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
// as g's current account and returns the final status and body.
func (h *hostedServer) boardSignIn(t *testing.T) (int, string) {
	t.Helper()
	b := hostauthtest.Browser(t, h.public, h.ts.URL)
	resp, err := b.Get(h.public + "/auth/login?next=/auth/session")
	if err != nil {
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

// capReviewer is the reviewer account the reviewer test signs in with; the
// setting names it in another case.
var capReviewer = hostauthtest.Account{ID: "sub-r", Email: "Reviewer@Example.com"}

// Website example game the reviewer imports (as OpenAI's reviewer will,
// from /docs/example).
const (
	siteExampleLog  = "../../site/src/content/example/battle-log.txt"
	siteExampleDeck = "../../testdata/decks/mega-lucario-ex-v12.txt"
)

// TestHostedReviewerPastCap: at a cap of 1 held by another account, the
// configured reviewer's connector signs in, imports the website's example
// game and plays, and the reviewer's board sign-in succeeds; a third
// account is still turned away with the busy page, as is an account with
// the reviewer's address that Google has not verified. The reviewer's
// place leaves the cap's one place to the first account, and neither the
// logs nor the records name the reviewer.
func TestHostedReviewerPastCap(t *testing.T) {
	logs := captureLog(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.maxUsers = 1; c.reviewer = "reviewer@example.com" })
	h.connect(t)

	h.g.SetAccount(capReviewer)
	cs := h.connect(t)
	callObject(t, cs, "import_log", map[string]any{"log": readTestdata(t, siteExampleLog), "deck": readTestdata(t, siteExampleDeck), "id": "review-example"})
	playSomeMoves(t, cs)
	if code, body := h.boardSignIn(t); code != http.StatusOK {
		t.Errorf("reviewer's board sign-in at the cap: %d %q, want 200", code, body)
	}

	h.g.SetAccount(capSecond)
	if err := h.tryConnect(t); err == nil || !strings.Contains(err.Error(), "tcgreplay is busy") {
		t.Errorf("third account's connector with the reviewer in: %v, want the busy page", err)
	}
	if code, body := h.boardSignIn(t); code != http.StatusServiceUnavailable || !strings.Contains(body, "tcgreplay is busy") {
		t.Errorf("third account's board sign-in with the reviewer in: %d, want 503 busy", code)
	}

	// The reviewer's address, unverified, is any other account.
	h.g.SetAccount(hostauthtest.Account{ID: "sub-impostor", Email: "reviewer@example.com"})
	h.g.Claims = func(c map[string]any) { c["email_verified"] = false }
	if err := h.tryConnect(t); err == nil || !strings.Contains(err.Error(), "tcgreplay is busy") {
		t.Errorf("unverified reviewer address's connector at the cap: %v, want the busy page", err)
	}
	if code, _ := h.boardSignIn(t); code != http.StatusServiceUnavailable {
		t.Errorf("unverified reviewer address's board sign-in at the cap: %d, want 503 busy", code)
	}
	h.g.Claims = nil

	// The first account still holds the cap's one place, beside the reviewer.
	h.g.SetAccount(hostauthtest.DefaultAccount)
	if err := h.tryConnect(t); err != nil {
		t.Errorf("first account connecting again with the reviewer in: %v", err)
	}

	out := strings.ToLower(logs.String() + h.events.String())
	if strings.Contains(out, "reviewer@example.com") {
		t.Error("the reviewer's address was logged or recorded")
	}
}

// TestHostedReviewerUnsetKeepsTheCap: with no reviewer configured, the
// account that would be the reviewer is turned away at a full cap.
func TestHostedReviewerUnsetKeepsTheCap(t *testing.T) {
	h := startHostedWith(t, testPublic, func(c *config) { c.maxUsers = 1 })
	h.connect(t)
	h.g.SetAccount(capReviewer)
	if err := h.tryConnect(t); err == nil || !strings.Contains(err.Error(), "tcgreplay is busy") {
		t.Errorf("reviewer account with no reviewer configured: %v, want the busy page", err)
	}
}

func TestReviewerText(t *testing.T) {
	if s := reviewerText(""); s != "no reviewer account (TCGREPLAY_REVIEWER)" {
		t.Errorf("reviewerText(\"\") = %q", s)
	}
	if s := reviewerText("reviewer@example.com"); s != "reviewer account set" {
		t.Errorf("reviewerText(email) = %q", s)
	}
}

// TestHostedReviewerToolCallsPastCap: the reviewer is recognised on tool
// calls too, from the verified flag its access token carries: at a cap of
// 1, after the reviewer's place went idle and another account took the
// one place, the reviewer's next tool call is still served, not refused
// as busy, while a third account is still refused.
func TestHostedReviewerToolCallsPastCap(t *testing.T) {
	clock := &idleClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	h := startHostedWith(t, testPublic, func(c *config) {
		c.maxUsers = 1
		c.idleTimeout = 2 * time.Minute
		c.now = clock.Now
		c.reviewer = "reviewer@example.com"
	})
	h.g.SetAccount(capReviewer)
	cs := h.connect(t)
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
