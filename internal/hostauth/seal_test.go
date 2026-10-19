package hostauth

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

const testSigningKey = "a-signing-key-long-enough-for-tests-0123"

// TestSealer shows a sealed value opens only as the kind it was sealed as,
// only with its own key, and not once altered.
func TestSealer(t *testing.T) {
	s, err := newSealer(testSigningKey)
	if err != nil {
		t.Fatal(err)
	}
	in := grantClaims{ID: "x", Sub: "sub-1", Email: "one@example.com", Exp: 42}
	sealed := s.seal(sealRefresh, in)
	if strings.Contains(sealed, "one@example.com") {
		t.Error("sealed value shows the email")
	}
	var out grantClaims
	if !s.open(sealRefresh, sealed, &out) || out != in {
		t.Fatalf("opened %+v, want %+v", out, in)
	}
	if s.open(sealAccess, sealed, &out) {
		t.Error("a refresh token opened as an access token")
	}
	same, _ := newSealer(testSigningKey)
	if !same.open(sealRefresh, sealed, &out) {
		t.Error("the same key did not open it")
	}
	other, _ := newSealer(testSigningKey + "-other")
	if other.open(sealRefresh, sealed, &out) {
		t.Error("another key opened it")
	}
	b := []byte(sealed)
	b[len(b)/2] ^= 1
	if s.open(sealRefresh, string(b), &out) {
		t.Error("an altered value opened")
	}
	if _, err := newSealer("short"); err == nil {
		t.Error("a short signing key was accepted")
	}
}

// TestRestartKeepsConnectorWithKey shows that with a signing key, after a
// restart at the same address the old access token still works, the old
// refresh token renews it for the same client, and the old client can
// still start a sign-in.
func TestRestartKeepsConnectorWithKey(t *testing.T) {
	c := newConnector(t)
	c.cfg.SigningKey = testSigningKey
	c.restart(t)
	tok := hostauthtest.Connect(t, c.ts.URL, c.ts.URL, c.g)
	c.checkMCPAccepted(t, tok.AccessToken)

	c.restart(t)

	c.checkMCPAccepted(t, tok.AccessToken)
	resp, m := c.token(t, refreshForm(tok.ClientID, tok.RefreshToken), "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh after restart: %d %v, want 200", resp.StatusCode, m)
	}
	c.checkMCPAccepted(t, m["access_token"].(string))
	// Used once, it is spent on this instance.
	resp, m = c.token(t, refreshForm(tok.ClientID, tok.RefreshToken), "", "")
	checkOAuthError(t, resp, m, http.StatusBadRequest, "invalid_grant")

	b := c.browser(t)
	b.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	r, err := b.Get(c.authorizeURL(tok.ClientID, hostauthtest.LoopbackCallback, nil))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusFound || !strings.HasPrefix(r.Header.Get("Location"), c.g.URL) {
		t.Errorf("authorize for the old client: %d to %q, want a redirect to Google", r.StatusCode, r.Header.Get("Location"))
	}
}

// TestRestartKeepsBoardSessionWithKey shows that with a signing key a board
// cookie from before a restart is a session again, holding a live sign-in,
// until it is signed out or expires; and that with another key it is not.
func TestRestartKeepsBoardSessionWithKey(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	newAuth := func(key string) *Auth {
		t.Helper()
		h := newHarness(t, func(c *Config) { c.SigningKey = key; c.Now = clock })
		return h.a
	}
	acct := Account{ID: "sub-1", Email: "one@example.com"}
	cookie := newAuth(testSigningKey).st.addSession(now, acct)

	a := newAuth(testSigningKey)
	s := a.st.session(cookie)
	if s == nil || s.account != acct {
		t.Fatalf("after restart the cookie names %+v, want a session for %+v", s, acct)
	}
	if a.st.accounts[acct.ID] == nil || a.st.accounts[acct.ID].live != 1 {
		t.Errorf("the taken-up session holds no live sign-in")
	}
	if a.st.session(cookie) != s {
		t.Error("the cookie names a second session")
	}
	a.st.endSession(cookie)
	if a.st.session(cookie) != nil {
		t.Error("the cookie works after signing out")
	}
	select {
	case <-s.done:
	default:
		t.Error("signing out did not end the session's live connections")
	}

	if newAuth(testSigningKey+"-other").st.session(cookie) != nil {
		t.Error("another key took the cookie up")
	}
	later := newAuth(testSigningKey)
	now = now.Add(sessionLifetime + time.Minute)
	if later.st.session(cookie) != nil {
		t.Error("an expired cookie was taken up")
	}
}
