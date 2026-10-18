package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// startPlacesHosted is hosted mode with a cap of maxUsers places and an
// idle time of idle (0 for never) on a settable clock.
func startPlacesHosted(t *testing.T, maxUsers int, idle time.Duration) (*hostedServer, *idleClock) {
	t.Helper()
	clock := &idleClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	h := startHostedWith(t, testPublic, func(c *config) {
		c.maxUsers = maxUsers
		c.idleTimeout = idle
		c.now = clock.Now
	})
	return h, clock
}

// checkBoardBusy fails unless the current account's board sign-in is
// refused with the busy page naming a cap of max.
func (h *hostedServer) checkBoardBusy(t *testing.T, who string, max int) {
	t.Helper()
	code, body := h.boardSignIn(t)
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "tcgreplay is busy") || !strings.Contains(body, fmt.Sprintf("(%d)", max)) {
		t.Errorf("%s's board sign-in: %d %q, want 503 busy naming the cap", who, code, body)
	}
}

// logoutBoard signs the browser holding cookie out of the board.
func (h *hostedServer) logoutBoard(t *testing.T, cookie *http.Cookie) {
	t.Helper()
	resp, _ := sendWith(t, http.MethodPost, h.ts.URL+"/auth/logout", cookie, func(r *http.Request) { r.Header.Set("Origin", testPublic) })
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sign-out: %d, want 303", resp.StatusCode)
	}
}

// TestHostedExpiredGameFreesPlace: at a cap of 1 with a 2 minute idle
// time, A signs in and imports a game; B's board and connector sign-ins
// get the busy page. Once A's game has gone 2 minutes and 1 second
// without activity, B signs in and its import succeeds.
func TestHostedExpiredGameFreesPlace(t *testing.T) {
	h, clock := startPlacesHosted(t, 1, 2*time.Minute)
	csA, _ := h.signInAs(t, userA)
	importFor(t, csA, testLogA, "game-a", "")

	h.g.SetAccount(userB)
	h.checkBoardBusy(t, "B at the cap", 1)
	if err := h.tryConnect(t); err == nil || !strings.Contains(err.Error(), "tcgreplay is busy") {
		t.Errorf("B's connector at the cap: %v, want the busy page", err)
	}

	clock.Advance(2*time.Minute + time.Second)
	csB, cookieB := h.signInAs(t, userB)
	importFor(t, csB, testLogB, "game-b", "")
	if got := stateGame(t, csB); got != "game-b" {
		t.Errorf("B's get_state reads %q, want game-b", got)
	}
	if resp, body := getWith(t, h.ts.URL+"/", cookieB, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("B's board page: %d %q", resp.StatusCode, body)
	}
}

// TestHostedNeverActedGivesUpPlace: an account that signs in to the board
// and does nothing gives up its place after the idle time.
func TestHostedNeverActedGivesUpPlace(t *testing.T) {
	h, clock := startPlacesHosted(t, 1, 2*time.Minute)
	h.g.SetAccount(userA)
	h.cookie(t)

	h.g.SetAccount(userB)
	h.checkBoardBusy(t, "B within A's idle time", 1)
	clock.Advance(2*time.Minute + time.Second)
	if code, body := h.boardSignIn(t); code != http.StatusOK || !strings.Contains(body, userB.Email) {
		t.Errorf("B's board sign-in after A sat idle: %d %q, want 200", code, body)
	}
}

// TestHostedFreedPlaceToldBusy: A's game expires and B takes the only
// place; A's next tool call is refused with the busy message and its
// board page with a 503, and A is not let back in until B's game expires
// in turn, when A's next call takes the place back, finds its game
// expired, and an import carries on.
func TestHostedFreedPlaceToldBusy(t *testing.T) {
	h, clock := startPlacesHosted(t, 1, 2*time.Minute)
	csA, cookieA := h.signInAs(t, userA)
	importFor(t, csA, testLogA, "game-a", "")
	clock.Advance(2*time.Minute + time.Second)
	csB, _ := h.signInAs(t, userB)
	importFor(t, csB, testLogB, "game-b", "")

	out, isErr := toolResult(t, csA, "get_state", nil)
	if !isErr {
		t.Fatalf("A's get_state with B holding the place: %q, want an error", out)
	}
	for _, want := range []string{"the server is busy", "(1)", "try again later"} {
		if !strings.Contains(out, want) {
			t.Errorf("A's refused get_state %q does not say %q", out, want)
		}
	}
	if resp, body := getWith(t, h.ts.URL+"/", cookieA, nil); resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "the server is busy") {
		t.Errorf("A's board page with B holding the place: %d %q, want 503 busy", resp.StatusCode, body)
	}

	clock.Advance(2*time.Minute + time.Second)
	out, isErr = toolResult(t, csA, "get_state", nil)
	if !isErr || !strings.Contains(out, "expired") {
		t.Fatalf("A's get_state after B's game expired: %q (error %v), want the expiry notice", out, isErr)
	}
	importFor(t, csA, testLogA, "game-a", "")
	if got := stateGame(t, csA); got != "game-a" {
		t.Errorf("A's get_state after importing again reads %q, want game-a", got)
	}
	out, isErr = toolResult(t, csB, "get_state", nil)
	if !isErr || !strings.Contains(out, "the server is busy") {
		t.Errorf("B's get_state with A holding the place again: %q (error %v), want busy", out, isErr)
	}
}

// TestHostedLastSignOutFreesPlace: at a cap of 1 with no idle drop, A
// signs in to the board only (no connector, so no refresh token holds a
// sign-in); B is busy, A can sign in again in a second browser at the
// cap, and B stays busy after A's first sign-out. A's last sign-out frees
// the place: B signs in, and A is the one kept out.
func TestHostedLastSignOutFreesPlace(t *testing.T) {
	h, _ := startPlacesHosted(t, 1, 0)
	h.g.SetAccount(userA)
	one := h.cookie(t)

	h.g.SetAccount(userB)
	h.checkBoardBusy(t, "B at the cap", 1)

	h.g.SetAccount(userA)
	two := h.cookie(t) // fails the test unless A gets in at the cap

	h.logoutBoard(t, one)
	h.g.SetAccount(userB)
	h.checkBoardBusy(t, "B after A's first sign-out", 1)

	h.logoutBoard(t, two)
	if code, body := h.boardSignIn(t); code != http.StatusOK || !strings.Contains(body, userB.Email) {
		t.Fatalf("B's board sign-in after A's last sign-out: %d %q, want 200", code, body)
	}
	h.g.SetAccount(userA)
	h.checkBoardBusy(t, "A after B took the place", 1)
}
