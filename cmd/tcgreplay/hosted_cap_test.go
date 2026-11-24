package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

// capSecond is the account the user cap tests try to sign in second.
var capSecond = hostauthtest.Account{ID: "sub-2", Email: "two@example.com"}

// tryConnect signs a connector in as g's current account, the way
// claude.ai does, and returns the error if it fails.
func (h *hostedServer) tryConnect(t *testing.T) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "hosted-test"}, nil).Connect(ctx, hostauthtest.MCPTransport(t, h.public, h.ts.URL, h.g), nil)
	if err == nil {
		cs.Close()
	}
	return err
}

// boardSignIn signs a fresh browser in to the board through /auth/login
// and the sign-in page's Google button as g's current account and returns
// the final status and body.
func (h *hostedServer) boardSignIn(t *testing.T) (int, string) {
	t.Helper()
	b := hostauthtest.Browser(t, h.public, h.ts.URL)
	resp, err := b.Get(h.public + "/auth/login?next=/auth/session")
	if err != nil {
		t.Fatal(err)
	}
	if resp, err = hostauthtest.ChooseGoogle(b, resp); err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// TestHostedUserCapSetting: hosted mode takes its user cap from the
// config. At a cap of 1, with one account's connector signed in, a second
// account's connector sign-in fails and its board sign-in gets the busy
// page, while the first account can still connect again; at 0 both
// accounts connect.
func TestHostedUserCapSetting(t *testing.T) {
	logs := captureLog(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.maxUsers = 1 })
	h.connect(t)

	h.g.SetAccount(capSecond)
	if err := h.tryConnect(t); err == nil {
		t.Error("second account's connector signed in at a cap of 1")
	} else if !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "tcgreplay is busy") {
		t.Errorf("second account's connector at a cap of 1 failed with %v, want the busy page", err)
	}
	code, body := h.boardSignIn(t)
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "tcgreplay is busy") || !strings.Contains(body, "(1)") {
		t.Errorf("second account's board sign-in at a cap of 1: %d %q, want 503 busy naming the cap", code, body)
	}
	if want := `"event":"sign_in_refused","player":"` + twoID + `"`; !strings.Contains(h.events.String(), want) {
		t.Errorf("records lack %s:\n%s", want, h.events)
	}
	if strings.Contains(logs.String()+h.events.String(), capSecond.Email) {
		t.Errorf("the refused account's email was logged or recorded")
	}

	h.g.SetAccount(hostauthtest.DefaultAccount)
	if err := h.tryConnect(t); err != nil {
		t.Errorf("first account connecting again at the cap: %v", err)
	}

	off := startHostedWith(t, testPublic, func(c *config) { c.maxUsers = 0 })
	off.connect(t)
	off.g.SetAccount(capSecond)
	if err := off.tryConnect(t); err != nil {
		t.Errorf("second account's connector with no cap: %v", err)
	}
	if code, body := off.boardSignIn(t); code != http.StatusOK || !strings.Contains(body, capSecond.Email) {
		t.Errorf("second account's board sign-in with no cap: %d %q, want 200", code, body)
	}
}

// TestMainHostedUserCapSetting: the binary's user cap is 10 when
// TCGREPLAY_MAX_USERS is unset, follows the variable (0 turns it off) and
// the -max-users flag over it, each read at start-up and named in the
// start-up log.
func TestMainHostedUserCapSetting(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  []string
		want string
	}{
		{"default", []string{"TCGREPLAY_MAX_USERS="}, "per account, 10 users"},
		{"env", []string{"TCGREPLAY_MAX_USERS=3"}, "per account, 3 users"},
		{"off", []string{"TCGREPLAY_MAX_USERS=0"}, "per account, 0 users"},
		{"flag", []string{"TCGREPLAY_MAX_USERS=3", "HELPER_ARGS=-max-users 2"}, "per account, 2 users"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			port := freePort(t)
			cmd, stderr, _ := helperCmd(t, append([]string{"PORT=" + port, "TCGREPLAY_PUBLIC_URL=https://play.example.com",
				"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret"}, tt.env...)...)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			t.Cleanup(func() { cmd.Process.Kill() })
			waitHealthy(t, "http://127.0.0.1:"+port, 10*time.Second, exited, stderr)
			if out := stderr.String(); !strings.Contains(out, tt.want) {
				t.Errorf("start-up log does not say %q:\n%s", tt.want, out)
			}
		})
	}
}

// Website example game the reviewer imports (as OpenAI's reviewer will,
// from /docs/example).
const (
	siteExampleLog  = "../../site/src/content/example/battle-log.txt"
	siteExampleDeck = "../../testdata/decks/mega-lucario-ex-v12.txt"
)

// TestHostedPasswordAccountPastCap: at a cap of 1 held by a Google
// account, a configured password account (a directory reviewer) connects,
// imports the website's example game and plays, and its board sign-in
// succeeds; a further Google account is still turned away with the busy
// page, as is a Google account whose email is the reviewer's username at
// a domain. The password account's place leaves the cap's one place to
// the first account, and neither the logs nor the records name it.
func TestHostedPasswordAccountPastCap(t *testing.T) {
	logs := captureLog(t)
	h := startHostedWith(t, testPublic, func(c *config) {
		c.maxUsers = 1
		c.passwordAccounts = passwordSetting(t, "openai-reviewer", "review-pw")
	})
	h.connect(t)

	cs := h.connectPassword(t, "openai-reviewer", "review-pw")
	callObject(t, cs, "import_log", map[string]any{"log": readTestdata(t, siteExampleLog), "deck": readTestdata(t, siteExampleDeck), "id": "review-example"})
	playSomeMoves(t, cs)
	// PasswordBoardCookie fails the test unless the board sign-in ends 200.
	hostauthtest.PasswordBoardCookie(t, h.public, h.ts.URL, "openai-reviewer", "review-pw")

	h.g.SetAccount(capSecond)
	if err := h.tryConnect(t); err == nil || !strings.Contains(err.Error(), "tcgreplay is busy") {
		t.Errorf("another Google account's connector with the reviewer in: %v, want the busy page", err)
	}
	if code, body := h.boardSignIn(t); code != http.StatusServiceUnavailable || !strings.Contains(body, "tcgreplay is busy") {
		t.Errorf("another Google account's board sign-in with the reviewer in: %d, want 503 busy", code)
	}
	h.g.SetAccount(hostauthtest.Account{ID: "sub-lookalike", Email: "openai-reviewer@example.com"})
	if err := h.tryConnect(t); err == nil || !strings.Contains(err.Error(), "tcgreplay is busy") {
		t.Errorf("a Google account named like the reviewer at the cap: %v, want the busy page", err)
	}

	// The first account still holds the cap's one place, beside the reviewer.
	h.g.SetAccount(hostauthtest.DefaultAccount)
	if err := h.tryConnect(t); err != nil {
		t.Errorf("first account connecting again with the reviewer in: %v", err)
	}

	out := logs.String() + h.events.String()
	if strings.Contains(out, "openai-reviewer") || strings.Contains(out, "review-pw") {
		t.Error("the reviewer's username or password was logged or recorded")
	}
}

// TestHostedPasswordAccountOneBelowCap: with one place still free, a
// password account signs in and imports a game without taking it, so a
// new Google player can still join afterwards.
func TestHostedPasswordAccountOneBelowCap(t *testing.T) {
	h := startHostedWith(t, testPublic, func(c *config) {
		c.maxUsers = 2
		c.passwordAccounts = passwordSetting(t, "openai-reviewer", "review-pw")
	})
	h.connect(t)
	cs := h.connectPassword(t, "openai-reviewer", "review-pw")
	callObject(t, cs, "import_log", map[string]any{"log": readTestdata(t, siteExampleLog), "deck": readTestdata(t, siteExampleDeck), "id": "review-example"})
	h.g.SetAccount(capSecond)
	if err := h.tryConnect(t); err != nil {
		t.Errorf("a new Google player after the reviewer joined one below the cap: %v, want in", err)
	}
	if code, _ := h.boardSignIn(t); code != http.StatusOK {
		t.Errorf("the new Google player's board sign-in: %d, want 200", code)
	}
}

// TestHostedPasswordAccountToolCallsPastCap: a password account is
// recognised on tool calls too, from the provider its access token
// carries: at a cap of 1, after its place went idle and another account
// took the one place, its next tool call is still served, not refused as
// busy, while a further account is still refused.
func TestHostedPasswordAccountToolCallsPastCap(t *testing.T) {
	clock := &idleClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	h := startHostedWith(t, testPublic, func(c *config) {
		c.maxUsers = 1
		c.idleTimeout = 2 * time.Minute
		c.now = clock.Now
		c.passwordAccounts = passwordSetting(t, "openai-reviewer", "review-pw")
	})
	cs := h.connectPassword(t, "openai-reviewer", "review-pw")
	clock.Advance(2*time.Minute + time.Second)
	h.g.SetAccount(hostauthtest.DefaultAccount)
	h.connect(t)

	// With no game open the call answers "no game open"; refused, it would
	// be the busy message instead.
	if out, isErr := toolResult(t, cs, "get_state", map[string]any{}); isErr && strings.Contains(out, "busy") {
		t.Errorf("reviewer's get_state with the one place held by another account was refused: %s", out)
	}
	h.g.SetAccount(capSecond)
	if err := h.tryConnect(t); err == nil || !strings.Contains(err.Error(), "tcgreplay is busy") {
		t.Errorf("third account's connector: %v, want the busy page", err)
	}
}

// TestLeftoverReviewerSettingIgnored: a TCGREPLAY_REVIEWER left in the
// environment is no longer used: the playmat logs that it is ignored and
// starts normally, and the setting grants nothing.
func TestLeftoverReviewerSettingIgnored(t *testing.T) {
	port := freePort(t)
	cmd, stderr, _ := helperCmd(t, "PORT="+port, "TCGREPLAY_PUBLIC_URL=https://play.example.com/",
		"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret",
		"TCGREPLAY_REVIEWER=reviewer@example.com")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() { cmd.Process.Kill() })
	waitHealthy(t, "http://127.0.0.1:"+port, 10*time.Second, exited, stderr)
	out := stderr.String()
	if !strings.Contains(out, "TCGREPLAY_REVIEWER is no longer used") {
		t.Errorf("no warning about the leftover setting:\n%s", out)
	}
	if strings.Contains(out, "reviewer@example.com") {
		t.Error("the leftover setting's value was logged")
	}
	if !strings.Contains(out, "no password accounts (TCGREPLAY_PASSWORD_ACCOUNTS)") {
		t.Errorf("start-up line does not report the password accounts:\n%s", out)
	}
}
