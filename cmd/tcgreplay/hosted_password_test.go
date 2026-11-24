package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/crypto/bcrypt"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
	"github.com/nicholasjackson/tcgreplay/internal/usergames"
)

// passwordSetting builds a TCGREPLAY_PASSWORD_ACCOUNTS value for the
// usernames and passwords given in pairs, hashed at the lowest accepted
// cost.
func passwordSetting(t *testing.T, pairs ...string) string {
	t.Helper()
	var entries []string
	for i := 0; i+1 < len(pairs); i += 2 {
		h, err := bcrypt.GenerateFromPassword([]byte(pairs[i+1]), bcrypt.DefaultCost)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, pairs[i]+":"+string(h))
	}
	return strings.Join(entries, ",")
}

// connectPassword signs a connector in with a username and password on the
// sign-in page and returns its MCP session.
func (h *hostedServer) connectPassword(t *testing.T, user, pass string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "hosted-test"}, nil).Connect(ctx, hostauthtest.PasswordMCPTransport(t, h.public, h.ts.URL, user, pass), nil)
	if err != nil {
		t.Fatalf("%s's password connect: %v", user, err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// stateText is get_state's text over cs, error or not: an account with no
// game gets an error result rather than a game.
func stateText(t *testing.T, cs *mcp.ClientSession) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_state"})
	if err != nil {
		t.Fatalf("get_state: %v", err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// getAccountID is get_account's account ID over cs.
func getAccountID(t *testing.T, cs *mcp.ClientSession) string {
	t.Helper()
	var acct struct {
		ID string `json:"id"`
	}
	out := callTool(t, cs, "get_account", nil)
	if err := json.Unmarshal([]byte(out), &acct); err != nil {
		t.Fatalf("get_account: %v: %s", err, out)
	}
	return acct.ID
}

// TestHostedPasswordAccountsKeptApart: with two password accounts
// configured, each connects and signs in with its own username and
// password; a game one imports is not visible to the other, on MCP or on
// the board, and they are different accounts.
func TestHostedPasswordAccountsKeptApart(t *testing.T) {
	h := startHostedWith(t, testPublic, func(c *config) {
		c.passwordAccounts = passwordSetting(t, "alice", "alice-pw", "bob", "bob-pw")
	})
	alice := h.connectPassword(t, "alice", "alice-pw")
	bob := h.connectPassword(t, "bob", "bob-pw")

	importFor(t, alice, testLogA, "alice-game", "")
	if got := stateGame(t, alice); got != "alice-game" {
		t.Fatalf("alice reads game %q, want alice-game", got)
	}
	if s := stateText(t, bob); strings.Contains(s, "alice-game") {
		t.Errorf("bob sees alice's game: %s", s)
	}
	if a, b := getAccountID(t, alice), getAccountID(t, bob); a != "password:alice" || b != "password:bob" {
		t.Errorf("get_account = %q and %q, want password:alice and password:bob", a, b)
	}

	// bob's board refuses alice's board link.
	bobCookie := hostauthtest.PasswordBoardCookie(t, h.public, h.ts.URL, "bob", "bob-pw")
	link := "/board?board=" + usergames.Handle("password:alice")
	if resp, body := getWith(t, h.ts.URL+link, bobCookie, nil); resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "Not your game") {
		t.Errorf("bob opening alice's board link: %d, want 403 not your game", resp.StatusCode)
	}
	// alice's own board link opens for her.
	aliceCookie := hostauthtest.PasswordBoardCookie(t, h.public, h.ts.URL, "alice", "alice-pw")
	if resp, _ := getWith(t, h.ts.URL+link, aliceCookie, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("alice opening her board link: %d, want 200", resp.StatusCode)
	}
}

// TestHostedPasswordAccountsNoCrossover: a Google account whose email is
// exactly an email-shaped password username, or whose email or name
// matches a plain one, sees none of the password account's games, and the
// password account sees none of theirs; they are recorded under different
// player IDs; and a browser signed in as one is never signed in as the
// other.
func TestHostedPasswordAccountsNoCrossover(t *testing.T) {
	h := startHostedWith(t, testPublic, func(c *config) {
		c.passwordAccounts = passwordSetting(t, "alice", "alice-pw", "carol@example.com", "carol-pw")
	})
	pwAlice := h.connectPassword(t, "alice", "alice-pw")
	pwCarol := h.connectPassword(t, "carol@example.com", "carol-pw")
	importFor(t, pwAlice, testLogA, "pw-alice-game", "")
	importFor(t, pwCarol, testLogA, "pw-carol-game", "")

	googles := map[string]hostauthtest.Account{
		"carol (exact email)": {ID: "sub-carol", Email: "carol@example.com"},
		"alice@example.com":   {ID: "sub-alice", Email: "alice@example.com"},
		"alice (plain name)":  {ID: "sub-alice-plain", Email: "alice"},
	}
	for name, g := range googles {
		cs, _ := h.signInAs(t, g)
		if s := stateText(t, cs); strings.Contains(s, "pw-alice-game") || strings.Contains(s, "pw-carol-game") {
			t.Errorf("Google %s sees a password account's game: %s", name, s)
		}
		if id := getAccountID(t, cs); id != g.ID {
			t.Errorf("Google %s's get_account = %q, want %s", name, id, g.ID)
		}
		importFor(t, cs, testLogA, "google-"+g.ID, "")
	}
	for name, cs := range map[string]*mcp.ClientSession{"alice": pwAlice, "carol@example.com": pwCarol} {
		if s := stateText(t, cs); strings.Contains(s, "google-") {
			t.Errorf("password account %s sees a Google account's game: %s", name, s)
		}
	}

	// Telemetry: the password accounts' player IDs are their own.
	rec := h.reg.Recorder()
	pwIDs := map[string]bool{
		rec.ID(hostauth.PasswordAccount("alice").PlayerKey()):             true,
		rec.ID(hostauth.PasswordAccount("carol@example.com").PlayerKey()): true,
	}
	for _, g := range googles {
		if pwIDs[rec.ID(g.Email)] {
			t.Errorf("Google %s shares a player ID with a password account", g.Email)
		}
	}
	for id := range pwIDs {
		if !strings.Contains(h.events.String(), `"event":"sign_in","player":"`+id+`","provider":"password"`) {
			t.Errorf("no password sign_in recorded for player %s", id)
		}
	}
	if strings.Contains(h.events.String(), "carol@example.com") {
		t.Error("records contain a username")
	}

	// One browser: password carol, then Google carol: only the latest.
	b := hostauthtest.Browser(t, h.public, h.ts.URL)
	session := func() string {
		resp, err := b.Get(h.public + "/auth/session")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var s struct{ Email, Provider string }
		json.NewDecoder(resp.Body).Decode(&s)
		return s.Provider + ":" + s.Email
	}
	signIn := func(choose hostauthtest.Chooser) {
		resp, err := b.Get(h.public + "/auth/login?next=/auth/session")
		if err != nil {
			t.Fatal(err)
		}
		if resp, err = choose(b, resp); err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	signIn(hostauthtest.ChoosePassword("carol@example.com", "carol-pw"))
	if got := session(); got != "password:carol@example.com" {
		t.Fatalf("after the password sign-in the browser is %q", got)
	}
	h.g.SetAccount(googles["carol (exact email)"])
	signIn(hostauthtest.ChooseGoogle)
	if got := session(); got != "google:carol@example.com" {
		t.Errorf("after the Google sign-in the browser is %q, want only the Google account", got)
	}
}

// TestHostedPasswordAccountNotAdmin: a password account whose username is
// an admin's email is refused by the admin pages.
func TestHostedPasswordAccountNotAdmin(t *testing.T) {
	t.Setenv("TCGREPLAY_ADMINS", "boss@example.com")
	h := startHostedWith(t, testPublic, func(c *config) {
		c.passwordAccounts = passwordSetting(t, "boss@example.com", "boss-pw")
	})
	cookie := hostauthtest.PasswordBoardCookie(t, h.public, h.ts.URL, "boss@example.com", "boss-pw")
	resp, body := getWith(t, h.ts.URL+"/admin", cookie, nil)
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "Not an admin") {
		t.Errorf("password account at /admin: %d, want 403 not an admin", resp.StatusCode)
	}
}

// TestHostedBoardOpensAfterPasswordConnect: after connecting an assistant
// with a password account, the same browser opens the board without
// signing in again.
func TestHostedBoardOpensAfterPasswordConnect(t *testing.T) {
	h := startHostedWith(t, testPublic, func(c *config) {
		c.passwordAccounts = passwordSetting(t, "alice", "alice-pw")
	})
	hc := hostauthtest.Server(h.public, h.ts.URL)
	resp, err := hc.Post(h.public+"/register", "application/json", strings.NewReader(`{"client_name":"hosted-test","redirect_uris":["`+hostauthtest.LoopbackCallback+`"],"token_endpoint_auth_method":"none"}`))
	if err != nil {
		t.Fatal(err)
	}
	var reg struct {
		ClientID string `json:"client_id"`
	}
	json.NewDecoder(resp.Body).Decode(&reg)
	resp.Body.Close()
	sum := sha256.Sum256([]byte("test-verifier-0123456789-0123456789-0123456789"))
	q := url.Values{
		"response_type": {"code"}, "client_id": {reg.ClientID}, "redirect_uri": {hostauthtest.LoopbackCallback},
		"state": {"st"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
	}
	b := hostauthtest.Browser(t, h.public, h.ts.URL)
	loc, err := hostauthtest.FollowPasswordSignIn(b, h.public+"/authorize?"+q.Encode(), "alice", "alice-pw")
	if err != nil {
		t.Fatal(err)
	}
	if loc.Query().Get("code") == "" {
		t.Fatalf("connecting ended at %s, want a code", loc)
	}
	resp, err = b.Get(h.public + "/board")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Request.URL.Path != "/board" {
		t.Errorf("the board after connecting: %d at %s, want 200 at /board without signing in", resp.StatusCode, resp.Request.URL)
	}
}

// TestHostedPasswordFormFollowsTheSetting: with accounts set the sign-in
// page offers the username and password form; unset, only Google.
func TestHostedPasswordFormFollowsTheSetting(t *testing.T) {
	for name, tc := range map[string]struct {
		setting  string
		wantForm bool
	}{
		"set":   {passwordSetting(t, "alice", "alice-pw"), true},
		"unset": {"", false},
	} {
		h := startHostedWith(t, testPublic, func(c *config) { c.passwordAccounts = tc.setting })
		b := hostauthtest.Browser(t, h.public, h.ts.URL)
		resp, err := b.Get(h.public + "/auth/login?next=/board")
		if err != nil {
			t.Fatal(err)
		}
		var body strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			body.Write(buf[:n])
			if err != nil {
				break
			}
		}
		resp.Body.Close()
		hasForm := strings.Contains(body.String(), `action="/auth/password"`)
		if hasForm != tc.wantForm || !strings.Contains(body.String(), `data-provider="google"`) {
			t.Errorf("%s: sign-in page has the form = %v (want %v), and must always offer Google", name, hasForm, tc.wantForm)
		}
	}
}

// TestHostedPasswordSettingRefused: a malformed or plain-text setting
// stops hosted mode with an error naming the setting but not its value.
func TestHostedPasswordSettingRefused(t *testing.T) {
	for _, bad := range []string{"alice:plaintext", "alice", passwordSetting(t, "alice", "x") + ",alice:" + "$2a$10$abcdefghijklmnopqrstuuJp6w9wB9Eh2Kq4.2Lp8Kf8GQ9pQ4bQ8u"} {
		c := hostedConfig(t, "127.0.0.1:"+freePort(t), "https://play.example.com")
		c.passwordAccounts = bad
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := runHosted(ctx, c)
		cancel()
		if err == nil {
			t.Errorf("hosted mode started with TCGREPLAY_PASSWORD_ACCOUNTS=%q", bad)
			continue
		}
		msg := err.Error()
		if !strings.Contains(msg, "TCGREPLAY_PASSWORD_ACCOUNTS") {
			t.Errorf("error %q does not name the setting", msg)
		}
		for _, secret := range []string{"plaintext", "alice", "$2a$"} {
			if strings.Contains(msg, secret) {
				t.Errorf("error %q contains %q", msg, secret)
			}
		}
	}
}

// TestHostedLogsPasswordAccountCount: the start-up line says how many
// password accounts are configured, and never who.
func TestHostedLogsPasswordAccountCount(t *testing.T) {
	logs := captureLog(t)
	base := "http://127.0.0.1:" + freePort(t)
	c := hostedConfig(t, strings.TrimPrefix(base, "http://"), "https://play.example.com")
	c.passwordAccounts = passwordSetting(t, "alice", "alice-pw", "bob", "bob-pw")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runHosted(ctx, c) }()
	waitHealthy(t, base, 5*time.Second, done, logs)
	cancel()
	<-done
	out := logs.String()
	if !strings.Contains(out, "2 password accounts") {
		t.Errorf("start-up log does not count the password accounts:\n%s", out)
	}
	for _, name := range []string{"alice", "bob", "$2a$"} {
		if strings.Contains(out, name) {
			t.Errorf("start-up log contains %q", name)
		}
	}
	for n, want := range map[int]string{0: "no password accounts (TCGREPLAY_PASSWORD_ACCOUNTS)", 1: "1 password account", 3: "3 password accounts"} {
		if got := passwordAccountsText(n); got != want {
			t.Errorf("passwordAccountsText(%d) = %q, want %q", n, got, want)
		}
	}
}
