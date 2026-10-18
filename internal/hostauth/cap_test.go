package hostauth

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

// capA and capB are the two accounts the user cap tests sign in: A takes
// the only place, B is the newcomer.
var (
	capA = hostauthtest.DefaultAccount
	capB = hostauthtest.Account{ID: "sub-2", Email: "two@example.com"}
)

// newCapped is the board harness with its sign-in service capped at
// maxUsers signed-in accounts.
func newCapped(t *testing.T, maxUsers int) *board {
	t.Helper()
	b := newBoard(t)
	b.cfg.MaxUsers = maxUsers
	b.restart(t)
	return b
}

// captureHostLog redirects the standard logger for the rest of the test.
func captureHostLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &logs
}

// boardAs signs g's current account in to the board through /auth/login
// in a fresh browser and returns the final response, its body and the
// browser.
func (b *board) boardAs(t *testing.T) (*http.Response, string, *http.Client) {
	t.Helper()
	br := b.browser(t)
	resp, body := get(t, br, b.ts.URL+"/auth/login?next="+url.QueryEscape("/"))
	return resp, body, br
}

// boardOK signs acct (g's current account) in to the board through
// /auth/login and checks it lands on the board as that account; it
// returns the session cookie value.
func (b *board) boardOK(t *testing.T, acct hostauthtest.Account) string {
	t.Helper()
	resp, body, br := b.boardAs(t)
	if resp.StatusCode != http.StatusOK || body != boardText(acct) {
		t.Fatalf("board sign-in as %s: %d %q, want the board page", acct.Email, resp.StatusCode, body)
	}
	id := b.sessionCookieIn(br)
	if id == "" {
		t.Fatalf("board sign-in as %s set no session cookie", acct.Email)
	}
	return id
}

// connectorAs runs g's current account through the connector's
// authorization request for clientID in a fresh browser, following it
// through Google, and returns the final response, its body and the
// browser. A sign-in that gets a code stops at the client's loopback
// callback with a 302.
func (b *board) connectorAs(t *testing.T, clientID string) (*http.Response, string, *http.Client) {
	t.Helper()
	br := b.browser(t)
	resp, body := get(t, br, b.authorizeURL(clientID, hostauthtest.LoopbackCallback, nil))
	return resp, body, br
}

// checkBusy checks a sign-in ended on the busy page naming the cap, with
// no session cookie in br and no redirect to an MCP client's callback.
func (b *board) checkBusy(t *testing.T, flow string, resp *http.Response, body string, br *http.Client, maxUsers int) {
	t.Helper()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("%s: status %d at %s, want 503", flow, resp.StatusCode, resp.Request.URL)
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		t.Errorf("%s: refused sign-in redirects to %q", flow, loc)
	}
	if resp.Request.URL.Host != b.cfg.Public.Host || resp.Request.URL.Path != googleCallbackPath {
		t.Errorf("%s: ended at %s, want the playmat's Google callback", flow, resp.Request.URL)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("%s: Content-Type %q, want an HTML page", flow, ct)
	}
	for _, want := range []string{"tcgreplay is busy", fmt.Sprintf("as many signed-in users as it can (%d)", maxUsers), "try again later"} {
		if !strings.Contains(body, want) {
			t.Errorf("%s: busy page does not say %q: %q", flow, want, body)
		}
	}
	if strings.Contains(body, "code=") {
		t.Errorf("%s: busy page carries a code: %q", flow, body)
	}
	if id := b.sessionCookieIn(br); id != "" {
		t.Errorf("%s: refused sign-in set a session cookie", flow)
	}
}

// accountCount is how many accounts hold a live sign-in.
func (b *board) accountCount() int {
	b.a.st.mu.Lock()
	defer b.a.st.mu.Unlock()
	return len(b.a.st.accounts)
}

// logout signs the browser holding session id out of the board from the
// playmat.
func (b *board) logout(t *testing.T, id string) {
	t.Helper()
	resp, _ := b.send(t, "POST", "/auth/logout", id, func(r *http.Request) { r.Header.Set("Origin", b.ts.URL) })
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sign-out: %d, want 303", resp.StatusCode)
	}
}

// TestCapRefusesNewAccountBothFlows shows that at a cap of 1, with A
// signed in, B is shown the busy page whichever way it signs in: through
// the board's /auth/login and through a connector's /authorize, where its
// browser is not sent back to the client with a code. Neither sets a
// session cookie, B holds no place, and each refusal is logged.
func TestCapRefusesNewAccountBothFlows(t *testing.T) {
	logs := captureHostLog(t)
	b := newCapped(t, 1)
	b.boardOK(t, capA)

	b.g.SetAccount(capB)
	logs.Reset()
	resp, body, br := b.boardAs(t)
	b.checkBusy(t, "board", resp, body, br, 1)
	if want := "hostauth: refused sign-in for " + capB.Email; !strings.Contains(logs.String(), want) {
		t.Errorf("board refusal: log %q does not record %q", logs.String(), want)
	}
	if !strings.Contains(logs.String(), "the server is busy") {
		t.Errorf("board refusal: log %q does not say the server is busy", logs.String())
	}

	clientID, _ := b.registerClient(t, "none", hostauthtest.LoopbackCallback)
	logs.Reset()
	resp, body, br = b.connectorAs(t, clientID)
	b.checkBusy(t, "connector", resp, body, br, 1)
	if want := "hostauth: refused sign-in for " + capB.Email; !strings.Contains(logs.String(), want) {
		t.Errorf("connector refusal: log %q does not record %q", logs.String(), want)
	}
	if _, err := hostauthtest.FollowSignIn(b.browser(t), b.authorizeURL(clientID, hostauthtest.LoopbackCallback, nil)); err == nil {
		t.Error("connector sign-in for B over the cap reached the client's callback")
	}

	if got := b.liveCount(capB.ID); got != 0 {
		t.Errorf("refused account holds %d live sign-ins, want 0", got)
	}
	if got := b.accountCount(); got != 1 {
		t.Errorf("%d accounts signed in, want 1", got)
	}
}

// TestCapLetsSignedInAccountBackIn shows that at the cap, the account
// already signed in can sign in again in other browsers, through the
// board and through a connector all the way to tokens, without taking
// another place.
func TestCapLetsSignedInAccountBackIn(t *testing.T) {
	b := newCapped(t, 1)
	b.boardOK(t, capA)

	b.boardOK(t, capA)
	clientID, _ := b.registerClient(t, "none", hostauthtest.LoopbackCallback)
	code := b.code(t, clientID, hostauthtest.LoopbackCallback)
	if resp, m := b.token(t, exchange(clientID, code, hostauthtest.LoopbackCallback, testVerifier), "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("A's code exchange at the cap: %d %v", resp.StatusCode, m)
	}
	if tok := hostauthtest.Connect(t, b.ts.URL, b.ts.URL, b.g); tok.AccessToken == "" || tok.RefreshToken == "" {
		t.Errorf("A's second connector sign-in at the cap got tokens %+v", tok)
	}

	if got := b.accountCount(); got != 1 {
		t.Errorf("%d accounts signed in after A's repeat sign-ins, want 1", got)
	}
	// Two board sessions, two connector browsers' sessions and two
	// refresh tokens.
	if got := b.liveCount(capA.ID); got != 6 {
		t.Errorf("A holds %d live sign-ins, want 6", got)
	}
}

// TestCapPlaceFreedAtLastSignOut shows a place is held until the
// account's last sign-in ends: with A signed in to the board in two
// browsers, B stays busy after the first sign-out and gets in after the
// second, after which A is the one kept out.
func TestCapPlaceFreedAtLastSignOut(t *testing.T) {
	b := newCapped(t, 1)
	one := b.boardOK(t, capA)
	two := b.boardOK(t, capA)

	b.g.SetAccount(capB)
	resp, body, br := b.boardAs(t)
	b.checkBusy(t, "B before A signs out", resp, body, br, 1)

	b.logout(t, one)
	resp, body, br = b.boardAs(t)
	b.checkBusy(t, "B after A's first sign-out", resp, body, br, 1)

	b.logout(t, two)
	if got := b.accountCount(); got != 0 {
		t.Fatalf("%d accounts signed in after A's last sign-out, want 0", got)
	}
	b.boardOK(t, capB)

	b.g.SetAccount(capA)
	resp, body, br = b.boardAs(t)
	b.checkBusy(t, "A after B took the place", resp, body, br, 1)
	clientID, _ := b.registerClient(t, "none", hostauthtest.LoopbackCallback)
	resp, body, br = b.connectorAs(t, clientID)
	b.checkBusy(t, "A's connector after B took the place", resp, body, br, 1)
}

// TestCapRefreshTokenHoldsPlace shows a connector's refresh token holds
// its account's place: after A signs a connector in and then signs out of
// the board, B is still busy, and stays busy after A's connector
// refreshes, since rotating the token keeps the count. A restart frees
// every place, letting B in.
func TestCapRefreshTokenHoldsPlace(t *testing.T) {
	b := newCapped(t, 1)
	clientID, _ := b.registerClient(t, "none", hostauthtest.LoopbackCallback)
	resp, _, br := b.connectorAs(t, clientID)
	loc, err := resp.Location()
	if resp.StatusCode != http.StatusFound || err != nil || loc.Query().Get("code") == "" {
		t.Fatalf("A's connector sign-in: %d to %v, want a code", resp.StatusCode, loc)
	}
	tr, m := b.token(t, exchange(clientID, loc.Query().Get("code"), hostauthtest.LoopbackCallback, testVerifier), "", "")
	if tr.StatusCode != http.StatusOK {
		t.Fatalf("A's code exchange: %d %v", tr.StatusCode, m)
	}
	refresh, _ := m["refresh_token"].(string)

	b.logout(t, b.sessionCookieIn(br))
	if got := b.liveCount(capA.ID); got != 1 {
		t.Fatalf("A holds %d live sign-ins after signing out of the board, want 1 (the refresh token)", got)
	}
	b.g.SetAccount(capB)
	resp, body, bb := b.boardAs(t)
	b.checkBusy(t, "B with A's refresh token live", resp, body, bb, 1)

	tr, m = b.token(t, refreshForm(clientID, refresh), "", "")
	if tr.StatusCode != http.StatusOK {
		t.Fatalf("A's refresh at the cap: %d %v", tr.StatusCode, m)
	}
	if got := b.liveCount(capA.ID); got != 1 {
		t.Errorf("A holds %d live sign-ins after a refresh, want 1", got)
	}
	resp, body, bb = b.boardAs(t)
	b.checkBusy(t, "B after A's refresh", resp, body, bb, 1)

	b.restart(t)
	b.boardOK(t, capB)
}

// TestCapStore shows the store's cap directly: at a cap of 3 the first
// three accounts get in, a fourth is refused with errFull and recorded
// nowhere, an account already in gets more sessions, and ending the last
// session of one account lets the fourth in.
func TestCapStore(t *testing.T) {
	s := newStore()
	now := time.Now()
	ids := map[string]string{}
	for i := 1; i <= 3; i++ {
		acct := Account{ID: fmt.Sprintf("sub-%d", i), Email: fmt.Sprintf("%d@example.com", i)}
		id, err := s.addSession(now, acct, 3)
		if err != nil {
			t.Fatalf("account %d under the cap: %v", i, err)
		}
		ids[acct.ID] = id
	}
	fourth := Account{ID: "sub-4", Email: "4@example.com"}
	if id, err := s.addSession(now, fourth, 3); !errors.Is(err, errFull) || id != "" {
		t.Fatalf("fourth account at a cap of 3: %q %v, want errFull", id, err)
	}
	if len(s.accounts) != 3 || len(s.sessions) != 3 {
		t.Errorf("after a refusal: %d accounts, %d sessions, want 3 and 3", len(s.accounts), len(s.sessions))
	}
	again, err := s.addSession(now, Account{ID: "sub-1", Email: "1@example.com"}, 3)
	if err != nil {
		t.Fatalf("signed-in account at the cap: %v", err)
	}
	s.endSession(ids["sub-1"])
	if _, err := s.addSession(now, fourth, 3); !errors.Is(err, errFull) {
		t.Errorf("fourth account with sub-1 still holding a session: %v, want errFull", err)
	}
	s.endSession(again)
	if _, err := s.addSession(now, fourth, 3); err != nil {
		t.Errorf("fourth account after sub-1's last session ended: %v", err)
	}
}

// TestCapSignInError shows signIn's refusal over the cap wraps ErrBusy,
// says the server is busy, names the cap and tells the user to try again
// later, and sets no cookie.
func TestCapSignInError(t *testing.T) {
	a, err := New(Config{
		Public:         &url.URL{Scheme: "https", Host: "playmat.example"},
		GoogleClientID: "client-id",
		GoogleSecret:   "client-secret",
		MaxUsers:       2,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if err := a.signIn(httptest.NewRecorder(), Account{ID: fmt.Sprintf("sub-%d", i)}); err != nil {
			t.Fatalf("account %d under the cap: %v", i, err)
		}
	}
	rec := httptest.NewRecorder()
	err = a.signIn(rec, Account{ID: "sub-3", Email: "three@example.com"})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("third account at a cap of 2: %v, want ErrBusy", err)
	}
	for _, want := range []string{"the server is busy", "(2)", "try again later"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if c := rec.Header().Values("Set-Cookie"); len(c) != 0 {
		t.Errorf("refused sign-in set cookies %v", c)
	}
}

// TestCapZeroIsOff shows a cap of 0 refuses no one: 50 accounts sign in,
// in the store and through signIn, and through the board flow over HTTP.
func TestCapZeroIsOff(t *testing.T) {
	s := newStore()
	a, err := New(Config{
		Public:         &url.URL{Scheme: "https", Host: "playmat.example"},
		GoogleClientID: "client-id",
		GoogleSecret:   "client-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		acct := Account{ID: fmt.Sprintf("sub-%d", i)}
		if _, err := s.addSession(time.Now(), acct, 0); err != nil {
			t.Fatalf("store: account %d refused with no cap: %v", i+1, err)
		}
		if err := a.signIn(httptest.NewRecorder(), acct); err != nil {
			t.Fatalf("signIn: account %d refused with no cap: %v", i+1, err)
		}
	}

	b := newCapped(t, 0)
	for i := 0; i < 5; i++ {
		acct := hostauthtest.Account{ID: fmt.Sprintf("sub-%d", i), Email: fmt.Sprintf("%d@example.com", i)}
		b.g.SetAccount(acct)
		b.boardOK(t, acct)
	}
	if got := b.accountCount(); got != 5 {
		t.Errorf("%d accounts signed in with no cap, want 5", got)
	}
}

// TestCapCodeHoldsPlace shows a connector code not yet exchanged keeps its
// account's place: at a cap of 1, A gets a code and signs out of the board
// before its client exchanges it; B is still busy, and the exchange does
// not let a second account in. A code left to expire gives the place back.
func TestCapCodeHoldsPlace(t *testing.T) {
	b := newCapped(t, 1)
	id, _ := b.registerClient(t, "none", hostauthtest.LoopbackCallback)

	resp, _, br := b.connectorAs(t, id)
	loc, err := resp.Location()
	if resp.StatusCode != http.StatusFound || err != nil || loc.Query().Get("code") == "" {
		t.Fatalf("A's authorize: %d %v, want a redirect with a code", resp.StatusCode, err)
	}
	code := loc.Query().Get("code")
	b.logout(t, b.sessionCookieIn(br))

	b.g.SetAccount(capB)
	resp, body, brB := b.boardAs(t)
	b.checkBusy(t, "B while A's code is pending", resp, body, brB, 1)

	tr, m := b.token(t, exchange(id, code, hostauthtest.LoopbackCallback, testVerifier), "", "")
	if tr.StatusCode != http.StatusOK {
		t.Fatalf("A's exchange: %d %v", tr.StatusCode, m)
	}
	if n := b.accountCount(); n != 1 {
		t.Fatalf("%d accounts signed in after A's exchange, want 1", n)
	}
	resp, body, brB = b.boardAs(t)
	b.checkBusy(t, "B after A's exchange", resp, body, brB, 1)

	// A fresh service: A gets a code it never exchanges, then signs out.
	b.restart(t)
	b.g.SetAccount(capA)
	id, _ = b.registerClient(t, "none", hostauthtest.LoopbackCallback)
	_, _, br = b.connectorAs(t, id)
	b.logout(t, b.sessionCookieIn(br))
	b.clock.Advance(codeLifetime + time.Minute)
	b.g.SetAccount(capB)
	b.boardOK(t, capB)
}
