package mcpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// wantLock checks the manager's acting lock and the board's opponent
// status.
func wantLock(t *testing.T, m *game.Manager, when string, locked bool, opponent string) {
	t.Helper()
	if got := m.Locked(); got != locked {
		t.Errorf("%s: Locked() = %v, want %v", when, got, locked)
	}
	if got := m.Opponent(); got != opponent {
		t.Errorf("%s: Opponent() = %q, want %q", when, got, opponent)
	}
}

func mustCall(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	out, isErr := call(t, cs, name, args)
	if isErr {
		t.Fatalf("%s: %s", name, out)
	}
	return out
}

// TestOwnTurnWriteLocks: in play mode the assistant's write on its own
// turn locks the board even with no wait first; a refused write or a read
// does not; its end_turn unlocks the board.
func TestOwnTurnWriteLocks(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
		m.Timing = game.Timing{LockRelease: time.Minute}
		g := waitGame(t, cs, m)
		if out, isErr := call(t, cs, "draw", nil); !isErr {
			t.Fatalf("draw on the player's turn: %s", out)
		}
		wantLock(t, m, "after a refused draw", false, "")

		if _, err := g.EndTurn(state.P1, state.ActorHuman); err != nil {
			t.Fatal(err)
		}
		mustCall(t, cs, "get_state", nil)
		wantLock(t, m, "after get_state on its turn", false, "")
		mustCall(t, cs, "draw", nil)
		wantLock(t, m, "after its draw", true, "acting")
		mustCall(t, cs, "end_turn", nil)
		wantLock(t, m, "after its end_turn", false, "")
	})
}

// TestToolCallsKeepLock: while locked, any game tool call refreshes the
// lock's clock, so an assistant calling more often than the lock-release
// time stays locked; once it stops the lock releases itself and the board
// shows it stopped. A read does not lock again, an own-turn write does.
func TestToolCallsKeepLock(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
		const release = 150 * time.Millisecond
		m.Timing = game.Timing{LockRelease: release}
		g := waitGame(t, cs, m)
		if _, err := g.EndTurn(state.P1, state.ActorHuman); err != nil {
			t.Fatal(err)
		}
		mustCall(t, cs, "draw", nil)
		until := time.Now().Add(4 * release)
		for time.Now().Before(until) {
			time.Sleep(30 * time.Millisecond)
			if !m.Locked() {
				t.Fatalf("lock released while the assistant called get_state every 30ms (release %v)", release)
			}
			mustCall(t, cs, "get_state", nil)
		}
		deadline := time.Now().Add(2 * time.Second)
		for m.Locked() {
			if time.Now().After(deadline) {
				t.Fatal("lock never released after the calls stopped")
			}
			time.Sleep(5 * time.Millisecond)
		}
		wantLock(t, m, "after the release", false, "stopped")
		mustCall(t, cs, "get_state", nil)
		wantLock(t, m, "after a read", false, "stopped")
		mustCall(t, cs, "draw", nil)
		wantLock(t, m, "after an own-turn draw", true, "acting")
	})
}

// TestWaitIsNotAMove: calling wait_for_turn does not keep the lock: an
// assistant that only calls it is shown as stopped once the release time
// passes.
func TestWaitIsNotAMove(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
		const release = 150 * time.Millisecond
		m.Timing = game.Timing{WaitCap: 50 * time.Millisecond, CheckIn: time.Minute, LockRelease: release}
		g := waitGame(t, cs, m)
		if _, err := g.EndTurn(state.P1, state.ActorHuman); err != nil {
			t.Fatal(err)
		}
		mustCall(t, cs, "draw", nil)
		until := time.Now().Add(4 * release)
		for time.Now().Before(until) {
			time.Sleep(30 * time.Millisecond)
			// Mid-turn, after its own moves, the wait is refused.
			if _, errText := waitCall(cs, nil); !strings.Contains(errText, "end_turn first") {
				t.Fatalf("wait_for_turn during its own turn: %q, want refused", errText)
			}
		}
		wantLock(t, m, "after only wait_for_turn calls", false, "stopped")
	})
}

// TestReviewWritesDoNotLock: in review mode with no turn handed over, the
// assistant's writes and reads never lock the board.
func TestReviewWritesDoNotLock(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
		m.Timing = game.Timing{LockRelease: time.Minute}
		waitGame(t, cs, m)
		mustCall(t, cs, "set_mode", map[string]any{"mode": "review"})
		mustCall(t, cs, "draw", map[string]any{"seat": "p2"})
		mustCall(t, cs, "draw", map[string]any{"seat": "p1"})
		mustCall(t, cs, "end_turn", map[string]any{"seat": "p1"})
		mustCall(t, cs, "draw", nil) // p2's turn now, the assistant's seat
		mustCall(t, cs, "get_state", nil)
		wantLock(t, m, "after review moves", false, "")
	})
}
