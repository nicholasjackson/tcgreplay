package hostauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// signingKey is a signing key for the restart tests.
const signingKey = "provider-test-signing-key-0123456789-abcdef"

// passwordConnector is a connector with a signing key and the password
// accounts raw parses to.
func passwordConnector(t *testing.T, raw string) *connector {
	t.Helper()
	c := newConnector(t)
	c.cfg.SigningKey = signingKey
	c.setAccounts(t, raw)
	c.restart(t)
	return c
}

// setAccounts sets the configured password accounts for the next restart.
func (c *connector) setAccounts(t *testing.T, raw string) {
	t.Helper()
	p, err := ParsePasswordAccounts(raw)
	if err != nil {
		t.Fatal(err)
	}
	c.cfg.PasswordAccounts = p
}

// grants are a board cookie and connector tokens issued for one account.
type grants struct {
	clientID, cookie, access, refresh string
}

// issue gives acct a board session and connector tokens, as a finished
// sign-in does.
func (c *connector) issue(t *testing.T, acct Account) grants {
	t.Helper()
	id, _ := c.registerClient(t, "none", "http://127.0.0.1/callback")
	now := c.a.now()
	access, refresh := c.a.st.issueTokens(now, id, acct)
	return grants{clientID: id, cookie: c.a.st.addSession(now, acct), access: access, refresh: refresh}
}

// tokenAccount is the account a tool call with token acts for, as the MCP
// library hands it to a tool, or false if the token is refused.
func (c *connector) tokenAccount(t *testing.T, token string) (Account, bool) {
	t.Helper()
	ti, err := c.a.verify(context.Background(), token, httptest.NewRequest("POST", "/mcp", nil))
	if err != nil {
		return Account{}, false
	}
	return AccountFromToken(ti)
}

// opens reports which of g's grants the current service still accepts,
// and the accounts they open as.
func (c *connector) opens(t *testing.T, g grants) (session, access, refresh *Account) {
	t.Helper()
	if s := c.a.st.session(g.cookie); s != nil {
		session = &s.account
	}
	if a, ok := c.tokenAccount(t, g.access); ok {
		access = &a
	}
	if a, ok := c.a.st.takeRefresh(g.clientID, g.refresh); ok {
		refresh = &a
	}
	return
}

// TestPasswordGrantsCarryTheProvider shows a password account's board
// cookie and tokens are recognised as that password account, on page
// loads and on tool calls, before and after a restart with the same
// signing key.
func TestPasswordGrantsCarryTheProvider(t *testing.T) {
	c := passwordConnector(t, "alice@example.com:"+hashFor(t, "pw"))
	want := PasswordAccount("alice@example.com")
	g := c.issue(t, want)

	check := func(when string, refresh bool) {
		t.Helper()
		if s := c.a.st.session(g.cookie); s == nil || s.account != want {
			t.Errorf("%s: board cookie opens as %+v, want %+v", when, s, want)
		}
		if a, ok := c.tokenAccount(t, g.access); !ok || a != want {
			t.Errorf("%s: tool call acts for %+v (%v), want %+v", when, a, ok, want)
		}
		if refresh {
			if a, ok := c.a.st.takeRefresh(g.clientID, g.refresh); !ok || a != want {
				t.Errorf("%s: refresh token opens as %+v (%v), want %+v", when, a, ok, want)
			}
		}
	}
	check("before a restart", false)
	c.restart(t)
	check("after a restart", true)
	// The board's session information names the provider.
	b := &board{connector: c}
	resp, body := b.send(t, "GET", "/auth/session", g.cookie, nil)
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(body) != `{"email":"alice@example.com","provider":"password"}` {
		t.Errorf("/auth/session for a password account: %d %s", resp.StatusCode, body)
	}
}

// oldGrant is the shape every grant was sealed in before the provider and
// fingerprint were kept.
type oldGrant struct {
	ID       string `json:"i,omitempty"`
	Client   string `json:"c,omitempty"`
	Sub      string `json:"u"`
	Email    string `json:"e"`
	Verified bool   `json:"v,omitempty"`
	Exp      int64  `json:"x"`
}

// TestGrantsFromBeforeOpenAsGoogle shows a board cookie and tokens sealed
// before the provider was kept open as the same Google account, with
// password accounts configured or not.
func TestGrantsFromBeforeOpenAsGoogle(t *testing.T) {
	for name, raw := range map[string]string{"no password accounts": "", "password accounts": "alice:" + hashFor(t, "pw")} {
		t.Run(name, func(t *testing.T) {
			c := passwordConnector(t, raw)
			id, _ := c.registerClient(t, "none", "http://127.0.0.1/callback")
			exp := c.a.now().Add(sessionLifetime).Unix()
			old := oldGrant{Sub: "1234567890", Email: "player@example.com", Verified: true, Exp: exp}
			g := grants{clientID: id}
			s := old
			s.ID = "old-session"
			g.cookie = c.a.st.seal.seal(sealSession, s)
			a := old
			a.Client = clientKey(id)
			g.access = c.a.st.seal.seal(sealAccess, a)
			r := a
			r.ID = "old-refresh"
			g.refresh = c.a.st.seal.seal(sealRefresh, r)
			c.restart(t)

			want := Account{ID: "1234567890", Email: "player@example.com", EmailVerified: true}
			session, access, refresh := c.opens(t, g)
			for kind, got := range map[string]*Account{"board cookie": session, "access token": access, "refresh token": refresh} {
				if got == nil || *got != want || got.Password() || got.providerName() != ProviderGoogle {
					t.Errorf("%s from before opens as %+v, want Google account %+v", kind, got, want)
				}
			}
		})
	}
}

// TestRemovedOrChangedAccountsGrantsRefused shows that after a restart in
// which a password account was removed, or its password changed, its
// board cookie and tokens are refused, while another account's and a
// Google account's still work.
func TestRemovedOrChangedAccountsGrantsRefused(t *testing.T) {
	aliceHash, bobHash := hashFor(t, "alice-pw"), hashFor(t, "bob-pw")
	c := passwordConnector(t, "alice:"+aliceHash+",bob:"+bobHash+",carol:"+hashFor(t, "carol-pw"))
	alice := c.issue(t, PasswordAccount("alice"))
	bob := c.issue(t, PasswordAccount("bob"))
	carol := c.issue(t, PasswordAccount("carol"))
	google := c.issue(t, Account{ID: "999", Email: "carol@example.com", EmailVerified: true})

	// alice keeps her hash, bob's password changes, carol is removed.
	c.setAccounts(t, "alice:"+aliceHash+",bob:"+hashFor(t, "bob-new-pw"))
	c.restart(t)

	for name, tc := range map[string]struct {
		g     grants
		stand bool
	}{
		"alice (unchanged)":      {alice, true},
		"bob (password changed)": {bob, false},
		"carol (removed)":        {carol, false},
		"a Google account":       {google, true},
	} {
		session, access, refresh := c.opens(t, tc.g)
		for kind, got := range map[string]*Account{"board cookie": session, "access token": access, "refresh token": refresh} {
			if (got != nil) != tc.stand {
				t.Errorf("%s: %s accepted = %v, want %v", name, kind, got != nil, tc.stand)
			}
		}
	}

	// With no password accounts configured at all, no password grant
	// opens.
	c.setAccounts(t, "")
	c.restart(t)
	if s, a, _ := c.opens(t, alice); s != nil || a != nil {
		t.Error("alice's grants open with no password accounts configured")
	}
}

// TestPasswordAccountsAreNeverAdmins shows a password account is refused
// by the admin pages, on reads and changes, even when its username is an
// admin's email.
func TestPasswordAccountsAreNeverAdmins(t *testing.T) {
	b := newAdminBoard(t, "boss@example.com")
	p, err := ParsePasswordAccounts("boss@example.com:" + hashFor(t, "pw"))
	if err != nil {
		t.Fatal(err)
	}
	b.a.usePasswordAccounts(p)
	acct := PasswordAccount("boss@example.com")
	if b.a.IsAdmin(acct) {
		t.Fatal("IsAdmin is true for a password account named as an admin")
	}
	cookie := b.a.st.addSession(b.a.now(), acct)
	for _, method := range []string{"GET", "POST"} {
		resp, body := b.send(t, method, "/admin/clear", cookie, b.fromPlaymat)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s /admin/clear as a password account: %d %q, want 403", method, resp.StatusCode, body)
		}
		if strings.Contains(body, "sign in again") {
			t.Errorf("%s refusal tells a password account to sign in again: %q", method, body)
		}
	}
	if b.reached != 0 {
		t.Errorf("the admin page was reached %d times", b.reached)
	}
	// The Google account with that email is still an admin.
	if !b.a.IsAdmin(Account{ID: "1", Email: "boss@example.com", EmailVerified: true}) {
		t.Error("the Google admin lost admin rights")
	}
}

// TestPasswordAccountsHaveTheirOwnPlayerID shows telemetry names a
// password account by its own player ID, never the one of the Google
// account with the same email, which keeps the ID it had before.
func TestPasswordAccountsHaveTheirOwnPlayerID(t *testing.T) {
	c := passwordConnector(t, "alice@example.com:"+hashFor(t, "pw"))
	rec, buf := recordTo(t, c.a)
	pw := PasswordAccount("alice@example.com")
	g := Account{ID: "555", Email: "alice@example.com", EmailVerified: true}
	if pw.PlayerKey() == g.PlayerKey() {
		t.Fatalf("both accounts have player key %q", g.PlayerKey())
	}
	if g.PlayerKey() != "alice@example.com" {
		t.Errorf("Google player key %q, want the email (unchanged player IDs)", g.PlayerKey())
	}
	for _, acct := range []Account{pw, g} {
		if err := c.a.signIn(context.Background(), httptest.NewRecorder(), acct); err != nil {
			t.Fatal(err)
		}
	}
	pwID, gID := rec.ID(pw.PlayerKey()), rec.ID("alice@example.com")
	if pwID == gID {
		t.Fatal("the password account's player ID is the Google account's")
	}
	wantRecord(t, buf, "sign_in", pwID, `,"provider":"password"`)
	wantRecord(t, buf, "sign_in", gID, `,"provider":"google"`)
	noEmails(t, buf)
}
