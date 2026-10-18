package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// idleClock is a settable clock safe to read from server goroutines.
type idleClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *idleClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *idleClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// startIdleHosted is hosted mode with an idle time of 2 minutes on a
// settable clock.
func startIdleHosted(t *testing.T) (*hostedServer, *idleClock) {
	t.Helper()
	clock := &idleClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	h := startHostedWith(t, testPublic, func(c *config) {
		c.idleTimeout = 2 * time.Minute
		c.now = clock.Now
	})
	return h, clock
}

// rawBoardFeed dials h's board with cookie and reads every state message,
// undecoded beyond JSON, in the background until the test ends.
func rawBoardFeed(t *testing.T, h *hostedServer, cookie *http.Cookie) <-chan map[string]any {
	t.Helper()
	c, _, err := dialBoard(t, h.ts, testPublic, cookie)
	if err != nil {
		t.Fatalf("board from %s: %v", testPublic, err)
	}
	c.SetReadLimit(-1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch := make(chan map[string]any, 64)
	go func() {
		defer close(ch)
		for {
			_, b, err := c.Read(ctx)
			if err != nil {
				return
			}
			var msg map[string]any
			if json.Unmarshal(b, &msg) != nil || msg["type"] != "state" {
				continue
			}
			select {
			case ch <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

// waitNotice waits up to within for a state message on feed with no game
// and the expiry notice, and returns the notice.
func waitNotice(t *testing.T, feed <-chan map[string]any, within time.Duration) string {
	t.Helper()
	timeout := time.After(within)
	for {
		select {
		case msg, open := <-feed:
			if !open {
				t.Fatal("board connection closed waiting for the expiry notice")
			}
			if n, _ := msg["notice"].(string); n != "" {
				if msg["snapshot"] != nil {
					t.Errorf("state with notice %q still has a snapshot", n)
				}
				return n
			}
		case <-timeout:
			t.Fatalf("no expiry notice on the board within %v", within)
		}
	}
}

// checkNotice fails unless notice says the game expired after 2 minutes
// and asks for its record.
func checkNotice(t *testing.T, where, notice string) {
	t.Helper()
	for _, want := range []string{"expired", "2 minutes", "record"} {
		if !strings.Contains(notice, want) {
			t.Errorf("%s: %q does not contain %q", where, notice, want)
		}
	}
}

// TestHostedIdleDrop: with the idle time at 2 minutes, an account that
// imports a game and leaves it 2 minutes and 1 second is told by its next
// tool call and its next board load that the game expired, and to import
// its record again; another account active in the meantime keeps its game.
func TestHostedIdleDrop(t *testing.T) {
	h, clock := startIdleHosted(t)
	csA, cookieA := h.signInAs(t, userA)
	csB, _ := h.signInAs(t, userB)
	importFor(t, csA, testLogA, "game-a", "")
	importFor(t, csB, testLogB, "game-b", "")

	clock.Advance(time.Minute)
	if got := stateGame(t, csB); got != "game-b" {
		t.Fatalf("B's get_state reads %q, want game-b", got)
	}
	clock.Advance(time.Minute + time.Second)

	out, isErr := toolResult(t, csA, "get_state", nil)
	if !isErr {
		t.Fatalf("A's get_state after 2m1s idle: %q, want an error", out)
	}
	checkNotice(t, "A's get_state", out)
	if _, err := h.manager(userA).Current(); !errors.Is(err, game.ErrExpired) {
		t.Errorf("A's manager after expiry: %v, want game.ErrExpired", err)
	}

	// A's board load gets the page, then the notice on its live connection.
	if resp, body := getWith(t, h.ts.URL+"/", cookieA, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("A's board page after expiry: %d %q", resp.StatusCode, body)
	}
	checkNotice(t, "A's board", waitNotice(t, rawBoardFeed(t, h, cookieA), 5*time.Second))

	if got := stateGame(t, csB); got != "game-b" {
		t.Errorf("B's get_state after A's expiry reads %q, want game-b", got)
	}

	// Importing again carries on.
	importFor(t, csA, testLogA, "game-a", "")
	if got := stateGame(t, csA); got != "game-a" {
		t.Errorf("A's get_state after importing again reads %q, want game-a", got)
	}
}

// TestHostedOpenTabDoesNotKeepGame: a board tab left open with no activity
// does not keep its game; the sweep expires it and the open tab shows the
// notice without a reload.
func TestHostedOpenTabDoesNotKeepGame(t *testing.T) {
	h, clock := startIdleHosted(t)
	csA, cookieA := h.signInAs(t, userA)
	importFor(t, csA, testLogA, "game-a", "")

	feed := rawBoardFeed(t, h, cookieA)
	select {
	case msg := <-feed:
		if msg["snapshot"] == nil {
			t.Fatalf("A's board opened with no game: %v", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("A's board sent no state")
	}

	clock.Advance(2*time.Minute + time.Second)
	h.reg.Sweep()
	checkNotice(t, "A's open board", waitNotice(t, feed, 5*time.Second))
	if _, err := h.manager(userA).Current(); !errors.Is(err, game.ErrExpired) {
		t.Errorf("A's game after the sweep: %v, want game.ErrExpired", err)
	}
}

// TestHostedActivityKeepsGame: a game acted on every minute for 5 minutes,
// through sweeps, is still there with its moves intact.
func TestHostedActivityKeepsGame(t *testing.T) {
	h, clock := startIdleHosted(t)
	csA, cookieA := h.signInAs(t, userA)
	importFor(t, csA, testLogA, "game-a", "")
	_, snap := boardSnapshot(t, h, cookieA)
	iid := firstHand(t, snap, state.P1)
	callTool(t, csA, "move", map[string]any{"seat": "p1", "iid": string(iid), "to": "discard"})
	g, err := h.manager(userA).Current()
	if err != nil {
		t.Fatal(err)
	}
	head := g.Head()

	for i := 0; i < 5; i++ {
		clock.Advance(time.Minute)
		h.reg.Sweep()
		if out, isErr := toolResult(t, csA, "get_state", nil); isErr {
			t.Fatalf("get_state at minute %d: %s", i+1, out)
		}
	}
	h.reg.Sweep()

	now, err := h.manager(userA).Current()
	if err != nil {
		t.Fatalf("A's game after 5 active minutes: %v", err)
	}
	if now != g || now.ID != "game-a" || now.Head() != head {
		t.Errorf("A's game is %s at head %d, want game-a at %d", now.ID, now.Head(), head)
	}
	if loc, err := now.State().Find(iid); err != nil || loc.Zone != state.Discard {
		t.Errorf("%s is at %+v (%v), want the discard", iid, loc, err)
	}
}

// TestEnvDuration: a duration setting falls back to its default when
// unset; 0 is a value; an invalid or negative value is an error naming the
// setting and its value.
func TestEnvDuration(t *testing.T) {
	const key = "TCGREPLAY_IDLE_TIMEOUT"
	for _, tt := range []struct {
		name, value string
		want        time.Duration
		bad         bool
	}{
		{"unset", "", 60 * time.Minute, false},
		{"zero", "0", 0, false},
		{"minutes", "2m", 2 * time.Minute, false},
		{"hours", "1h", time.Hour, false},
		{"not a duration", "abc", 0, true},
		{"bare number", "5", 0, true},
		{"negative", "-1m", 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(key, tt.value)
			got, err := envDuration(key, 60*time.Minute)
			if tt.bad {
				if err == nil {
					t.Fatalf("envDuration with %q = %v, want an error", tt.value, got)
				}
				if !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), tt.value) {
					t.Fatalf("error %q does not name %s and %q", err, key, tt.value)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("envDuration with %q = %v, %v, want %v", tt.value, got, err, tt.want)
			}
		})
	}
}

// TestRunHostedRejectsBadIdleTimeout: an invalid idle setting or a negative
// idle time stops start-up with an error, before anything listens.
func TestRunHostedRejectsBadIdleTimeout(t *testing.T) {
	port := freePort(t)
	for _, tt := range []struct {
		name string
		mod  func(*config)
		want string
	}{
		{"invalid env", func(c *config) {
			c.idleErr = errors.New(`TCGREPLAY_IDLE_TIMEOUT: invalid duration "abc"`)
		}, `TCGREPLAY_IDLE_TIMEOUT: invalid duration "abc"`},
		{"negative", func(c *config) { c.idleTimeout = -time.Minute }, "-idle-timeout"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLog(t)
			c := hostedConfig(t, "127.0.0.1:"+port, "https://play.example.com")
			tt.mod(&c)
			done := make(chan error, 1)
			go func() { done <- runHosted(context.Background(), c) }()
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), "hosted mode") || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("runHosted: %v, want a hosted mode error containing %q", err, tt.want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("runHosted with a bad idle setting did not stop")
			}
			ln, err := net.Listen("tcp", "127.0.0.1:"+port)
			if err != nil {
				t.Fatalf("port %s in use after a rejected start: %v", port, err)
			}
			ln.Close()
			if strings.Contains(logs.String(), "listening") {
				t.Errorf("logged listening with a bad idle setting: %q", logs.String())
			}
		})
	}
}

// TestMainHostedIdleTimeoutSetting: the binary drops idle games after
// 60 minutes when TCGREPLAY_IDLE_TIMEOUT is unset, follows the variable
// (0 turns it off) and the -idle-timeout flag over it, even over an
// invalid variable, each named in the start-up log.
func TestMainHostedIdleTimeoutSetting(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  []string
		want string
	}{
		{"default", []string{"TCGREPLAY_IDLE_TIMEOUT="}, "idle games dropped after 1h0m0s"},
		{"env", []string{"TCGREPLAY_IDLE_TIMEOUT=2m"}, "idle games dropped after 2m0s"},
		{"off", []string{"TCGREPLAY_IDLE_TIMEOUT=0"}, "idle games never dropped"},
		{"flag", []string{"TCGREPLAY_IDLE_TIMEOUT=2m", "HELPER_ARGS=-idle-timeout 5m"}, "idle games dropped after 5m0s"},
		{"flag over invalid env", []string{"TCGREPLAY_IDLE_TIMEOUT=abc", "HELPER_ARGS=-idle-timeout 5m"}, "idle games dropped after 5m0s"},
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

// TestMainHostedRejectsBadIdleTimeout: the binary exits non-zero, without
// listening, with a message naming the setting and its value when
// TCGREPLAY_IDLE_TIMEOUT is invalid or negative, or -idle-timeout is
// negative.
func TestMainHostedRejectsBadIdleTimeout(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  []string
		want []string
	}{
		{"invalid env", []string{"TCGREPLAY_IDLE_TIMEOUT=abc"}, []string{"TCGREPLAY_IDLE_TIMEOUT", `"abc"`}},
		{"negative env", []string{"TCGREPLAY_IDLE_TIMEOUT=-1m"}, []string{"TCGREPLAY_IDLE_TIMEOUT", `"-1m"`}},
		{"negative flag", []string{"TCGREPLAY_IDLE_TIMEOUT=", "HELPER_ARGS=-idle-timeout -1m"}, []string{"-idle-timeout", "negative"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd, stderr, _ := helperCmd(t, append([]string{"PORT=" + freePort(t), "TCGREPLAY_PUBLIC_URL=https://play.example.com",
				"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret"}, tt.env...)...)
			done := make(chan error, 1)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatalf("hosted main started with a bad idle setting\n%s", stderr)
				}
			case <-time.After(10 * time.Second):
				cmd.Process.Kill()
				t.Fatalf("hosted main did not stop on a bad idle setting\n%s", stderr)
			}
			out := stderr.String()
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("stderr does not contain %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, "listening") {
				t.Errorf("hosted main listened with a bad idle setting:\n%s", out)
			}
		})
	}
}
