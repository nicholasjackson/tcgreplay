package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

// eventRecords decodes the records in out, failing on a line that is not
// one JSON object or has no RFC 3339 time.
func eventRecords(t *testing.T, out string) []map[string]string {
	t.Helper()
	var recs []map[string]string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l == "" {
			continue
		}
		var r map[string]string
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatalf("record %q: %v", l, err)
		}
		if _, err := time.Parse(time.RFC3339, r["time"]); err != nil {
			t.Errorf("record %q: time: %v", l, err)
		}
		recs = append(recs, r)
	}
	return recs
}

// wantEvent fails unless recs hold a record of event for player whose
// other fields include want.
func wantEvent(t *testing.T, recs []map[string]string, event, player string, want map[string]string) {
	t.Helper()
next:
	for _, r := range recs {
		if r["event"] != event || r["player"] != player {
			continue
		}
		for k, v := range want {
			if !strings.HasPrefix(r[k], v) {
				continue next
			}
		}
		return
	}
	t.Errorf("no %s record for %s with %v in %v", event, player, want, recs)
}

// TestHostedRecordsUseIDs: signing in, being refused at the user cap,
// signing out, hitting the request limit and letting a game expire each
// write a record with the player's ID, and no email appears in any
// record or log line.
func TestHostedRecordsUseIDs(t *testing.T) {
	logs := captureLog(t)
	clock := &idleClock{t: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}
	h := startHostedWith(t, testPublic, func(c *config) {
		c.maxUsers = 1
		c.idleTimeout = 2 * time.Minute
		c.now = clock.Now
	})
	csA, cookieA := h.signInAs(t, userA)
	importFor(t, csA, testLogA, "game-a", "")

	h.g.SetAccount(userB)
	h.checkBoardBusy(t, "B", 1)

	h.logoutBoard(t, cookieA)

	clock.Advance(2*time.Minute + time.Second)
	toolResult(t, csA, "get_state", nil) // finds the game expired

	limited := startHostedWith(t, testPublic, func(c *config) { c.rateLimit = 1 })
	if _, isErr := toolResult(t, limited.connect(t), "ping", nil); !isErr {
		t.Fatal("ping over a limit of 1 allowed")
	}

	recs := eventRecords(t, h.events.String())
	wantEvent(t, recs, "sign_in", aID, nil)
	wantEvent(t, recs, "sign_in_refused", bID, map[string]string{"reason": "the server is busy"})
	wantEvent(t, recs, "limited", bID, map[string]string{"reason": "places"})
	wantEvent(t, recs, "sign_out", aID, nil)
	wantEvent(t, recs, "game_expired", aID, map[string]string{"idle": "2 minutes"})
	wantEvent(t, eventRecords(t, limited.events.String()), "limited", oneID, map[string]string{"reason": "rate", "message": "too many requests"})

	for name, out := range map[string]string{"records": h.events.String() + limited.events.String(), "log": logs.String()} {
		for _, email := range []string{userA.Email, userB.Email, hostauthtest.DefaultAccount.Email, "@example.com"} {
			if strings.Contains(out, email) {
				t.Errorf("%s contain %q:\n%s", name, email, out)
			}
		}
	}
}

// TestHostedPlayerIDStable: an account keeps its ID across a sign-out, a
// different assistant and a second hosted instance started with the same
// key (standing in for a redeploy); another account's ID differs.
func TestHostedPlayerIDStable(t *testing.T) {
	h := startHosted(t, testPublic)
	_, cookie := h.signInAs(t, userA)
	h.logoutBoard(t, cookie)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	other, err := mcp.NewClient(&mcp.Implementation{Name: "another-assistant"}, nil).Connect(ctx, hostauthtest.MCPTransport(t, h.public, h.ts.URL, h.g), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Close() })
	h.signInAs(t, userB)

	redeployed := startHosted(t, testPublic)
	redeployed.signInAs(t, userA)

	count := func(out, player string) int {
		n := 0
		for _, r := range eventRecords(t, out) {
			if r["event"] == "sign_in" && r["player"] == player {
				n++
			}
		}
		return n
	}
	// Board and connector, then the other assistant's connector.
	if n := count(h.events.String(), aID); n != 3 {
		t.Errorf("%d sign_in records for A's ID %s, want 3:\n%s", n, aID, h.events)
	}
	if n := count(h.events.String(), bID); n != 2 {
		t.Errorf("%d sign_in records for B's ID %s, want 2:\n%s", n, bID, h.events)
	}
	if n := count(redeployed.events.String(), aID); n != 2 {
		t.Errorf("%d sign_in records for A's ID on the second instance, want 2:\n%s", n, redeployed.events)
	}
	if aID == bID {
		t.Error("two accounts share an ID")
	}
}

// TestHostedAdminRefusalUsesID: a signed-in non-admin reaching /admin is
// recorded as admin_refused by player ID, not email.
func TestHostedAdminRefusalUsesID(t *testing.T) {
	logs := captureLog(t)
	h := startHostedAdmin(t, false, false)
	h.g.SetAccount(capSecond)
	if resp, _ := getWith(t, h.ts.URL+"/admin", h.cookie(t), nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin /admin: %d, want 403", resp.StatusCode)
	}
	wantEvent(t, eventRecords(t, h.events.String()), "admin_refused", twoID, map[string]string{"reason": "not admin"})
	if strings.Contains(logs.String()+h.events.String(), capSecond.Email) {
		t.Errorf("the non-admin's email was logged or recorded:\n%s\n%s", logs, h.events)
	}
}

// TestMainHostedNeedsPlayerIDKey: the binary in hosted mode exits
// non-zero, naming the setting, without a player-ID key or with one under
// 32 characters, and never prints the key.
func TestMainHostedNeedsPlayerIDKey(t *testing.T) {
	for name, key := range map[string]string{"missing": "", "short": "a-key-of-only-31-characters-xyz"} {
		t.Run(name, func(t *testing.T) {
			cmd, stderr, _ := helperCmd(t, "PORT="+freePort(t), "TCGREPLAY_PUBLIC_URL=https://play.example.com",
				"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret", "TCGREPLAY_PLAYER_ID_KEY="+key)
			done := make(chan error, 1)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatalf("hosted main started without a usable player-ID key\n%s", stderr)
				}
			case <-time.After(10 * time.Second):
				cmd.Process.Kill()
				t.Fatalf("hosted main did not stop without a usable player-ID key\n%s", stderr)
			}
			out := stderr.String()
			if !strings.Contains(out, "TCGREPLAY_PLAYER_ID_KEY") || !strings.Contains(out, "at least 32 characters") {
				t.Errorf("stderr does not name the missing key:\n%s", out)
			}
			if strings.Contains(out, "listening") || (key != "" && strings.Contains(out, key)) {
				t.Errorf("hosted main listened or printed the key:\n%s", out)
			}
		})
	}
}

// TestMainHostedSaysPlayerIDsOn: the start-up line says player IDs are on
// and never shows the key.
func TestMainHostedSaysPlayerIDsOn(t *testing.T) {
	port := freePort(t)
	cmd, stderr, _ := helperCmd(t, "PORT="+port, "TCGREPLAY_PUBLIC_URL=https://play.example.com",
		"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret", "TCGREPLAY_PLAYER_ID_KEY="+testPlayerIDKey)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() { cmd.Process.Kill() })
	waitHealthy(t, "http://127.0.0.1:"+port, 10*time.Second, exited, stderr)
	cmd.Process.Signal(syscall.SIGTERM)
	<-exited
	out := stderr.String()
	if !strings.Contains(out, "player IDs on") {
		t.Errorf("start-up line does not say player IDs are on:\n%s", out)
	}
	if strings.Contains(out, testPlayerIDKey) {
		t.Errorf("the key was printed:\n%s", out)
	}
}

// TestAppSpecAlerts: the App Platform definition declares the restart,
// CPU and memory alert rules for the playmat, each with an operator, a
// value and a window.
func TestAppSpecAlerts(t *testing.T) {
	spec := readTestdata(t, "../../.do/app.yaml")
	i := strings.Index(spec, "\n    alerts:\n")
	if i < 0 {
		t.Fatal("app.yaml declares no alerts on the playmat service")
	}
	block := spec[i:]
	if j := strings.Index(block, "\n    envs:\n"); j >= 0 {
		block = block[:j]
	}
	for _, want := range []string{
		"- rule: RESTART_COUNT\n        operator: GREATER_THAN\n        value: 0\n        window: FIVE_MINUTES",
		"- rule: CPU_UTILIZATION\n        operator: GREATER_THAN\n        value: 80\n        window: TEN_MINUTES",
		"- rule: MEM_UTILIZATION\n        operator: GREATER_THAN\n        value: 80\n        window: TEN_MINUTES",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("alerts lack:\n%s\nin:\n%s", want, block)
		}
	}
}
