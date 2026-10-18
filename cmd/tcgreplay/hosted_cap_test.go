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
	if want := "refused sign-in for " + capSecond.Email; !strings.Contains(logs.String(), want) {
		t.Errorf("log does not record %q:\n%s", want, logs)
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
