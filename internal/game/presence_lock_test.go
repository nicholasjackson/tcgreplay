package game

import (
	"context"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// eventually polls cond until it holds, failing after within.
func eventually(t *testing.T, what string, within time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("%s: not within %v", what, within)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// wantPresence checks whether a hold is set and the board's opponent
// status.
func wantPresence(t *testing.T, m *Manager, when string, locked bool, opponent string) {
	t.Helper()
	if got := m.Hold() != HoldNone; got != locked {
		t.Errorf("%s: Hold() = %q, want a hold %v", when, m.Hold(), locked)
	}
	if got := m.Opponent(); got != opponent {
		t.Errorf("%s: Opponent() = %q, want %q", when, got, opponent)
	}
}

// setWaitingGrace shortens how long "waiting" outlives a still_waiting
// call, for this test only.
func setWaitingGrace(t *testing.T, d time.Duration) {
	t.Helper()
	old := waitingGrace
	waitingGrace = d
	t.Cleanup(func() { waitingGrace = old })
}

// lockByTurnEnded hands the turn to the assistant through the wait: the
// player ends p1's turn under a wait, which reports turn_ended and so sets
// the acting lock.
func lockByTurnEnded(t *testing.T, m *Manager, g *Game) {
	t.Helper()
	done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
	r := recv(t, done, time.Second)
	if r.err != nil || r.out.Kind != WaitTurnEnded {
		t.Fatalf("hand-off: %+v, %v; want turn_ended", r.out, r.err)
	}
}

// TestOutcomesSetActingLock: a wait that reports the turn ended, a card to
// answer or a promotion locks the board for the assistant (status
// "acting"); while the call is in flight the status is "waiting" and the
// board is not locked.
func TestOutcomesSetActingLock(t *testing.T) {
	p2Active := state.Loc{Seat: state.P2, Zone: state.Active}
	for _, tt := range []struct {
		name   string
		kind   string
		action string
		act    func(t *testing.T, g *Game, judge state.IID)
	}{
		{"turn ended", WaitTurnEnded, "", func(t *testing.T, g *Game, _ state.IID) {
			must(t)(g.EndTurn(state.P1, state.ActorHuman))
		}},
		{"answer card", WaitDecide, DecideAnswerCard, func(t *testing.T, g *Game, eri state.IID) {
			must(t)(g.PlayTrainer(state.P1, state.ActorHuman, eri))
		}},
		{"promote", WaitDecide, DecidePromote, func(t *testing.T, g *Game, _ state.IID) {
			must(t)(g.KnockOut(state.P1, state.ActorHuman, p2Active))
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, g, _, _ := respondGame(t)
			eri := eriCard(t, m, g) // stops the wait: the playmat does not apply it
			m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute, LockRelease: time.Minute}
			wantPresence(t, m, "before any wait", false, "")
			done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
			wantPresence(t, m, "while the wait is in flight", false, "waiting")
			tt.act(t, g, eri)
			r := recv(t, done, time.Second)
			if r.err != nil || r.out.Kind != tt.kind || r.out.Action != tt.action {
				t.Fatalf("got %+v, %v; want %s %s", r.out, r.err, tt.kind, tt.action)
			}
			status, hold := "acting", HoldFull
			if tt.kind == WaitDecide {
				status, hold = "deciding", HoldDecision
			}
			wantPresence(t, m, "after "+tt.kind, true, status)
			if got := m.Hold(); got != hold {
				t.Errorf("after %s: Hold() = %q, want %q", tt.kind, got, hold)
			}
			if got := m.Decision(); got != tt.action {
				t.Errorf("after %s: Decision() = %q, want %q", tt.kind, got, tt.action)
			}
		})
	}
}

// TestQuietOutcomesDoNotLock: still_waiting, check_in and ended leave the
// board unlocked.
func TestQuietOutcomesDoNotLock(t *testing.T) {
	t.Run("still waiting", func(t *testing.T) {
		m, _ := startedManager(t)
		m.Timing = Timing{WaitCap: 30 * time.Millisecond, CheckIn: time.Minute}
		if o := waitNow(t, m, WaitRequest{Seat: state.P1}); o.Kind != WaitStillWaiting {
			t.Fatalf("kind %q, want still_waiting", o.Kind)
		}
		if m.Hold() != HoldNone {
			t.Error("still_waiting locked the board")
		}
	})
	t.Run("check in", func(t *testing.T) {
		m, _ := startedManager(t)
		m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: 30 * time.Millisecond}
		if o := waitNow(t, m, WaitRequest{Seat: state.P1}); o.Kind != WaitCheckIn {
			t.Fatalf("kind %q, want check_in", o.Kind)
		}
		wantPresence(t, m, "after check_in", false, "")
	})
	t.Run("ended", func(t *testing.T) {
		m, g := startedManager(t)
		m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute}
		done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
		if err := g.SetMode(ModeReview); err != nil {
			t.Fatal(err)
		}
		if r := recv(t, done, time.Second); r.out.Kind != WaitEnded {
			t.Fatalf("got %+v, want ended", r.out)
		}
		wantPresence(t, m, "after ended", false, "")
	})
}

// TestWaitingAgainReleasesLock: once the assistant answered the card, its
// next wait call unlocks the board at once; after its own turn, ending it
// unlocks the board.
func TestWaitingAgainReleasesLock(t *testing.T) {
	t.Run("next wait", func(t *testing.T) {
		m, g, _, _ := respondGame(t)
		eri := eriCard(t, m, g)
		m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute, LockRelease: time.Minute}
		done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
		must(t)(g.PlayTrainer(state.P1, state.ActorHuman, eri))
		r := recv(t, done, time.Second)
		if r.out.Kind != WaitDecide || r.out.Action != DecideAnswerCard {
			t.Fatalf("got %+v, want decide/answer_card", r.out)
		}
		wantPresence(t, m, "answering the card", true, "deciding")

		ctx, cancel := context.WithCancel(context.Background())
		done = startWait(t, ctx, m, WaitRequest{Seat: state.P1, Since: intp(r.out.Next)})
		wantPresence(t, m, "waiting again", false, "waiting")
		// The player carries on with the same turn.
		must(t)(g.Draw(state.P1, state.ActorHuman, 1))
		cancel()
		recv(t, done, time.Second)
	})
	t.Run("end turn", func(t *testing.T) {
		m, g := startedManager(t)
		m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute, LockRelease: time.Minute}
		lockByTurnEnded(t, m, g)
		wantPresence(t, m, "the assistant's turn", true, "acting")
		m.AssistantMoved(true)
		must(t)(g.EndTurn(state.P2, state.ActorClaude))
		m.AssistantDone()
		wantPresence(t, m, "after the assistant ended its turn", false, "")
	})
}

// TestLockReleasesItself: with no move for the lock-release time the lock
// lets go by itself and the status says the assistant stopped; a tool call
// that is not a move on its own turn does not take it back, but an
// own-turn move does.
func TestLockReleasesItself(t *testing.T) {
	m, g := startedManager(t)
	const release = 80 * time.Millisecond
	m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute, LockRelease: release}
	start := time.Now()
	lockByTurnEnded(t, m, g)
	if !(m.Hold() != HoldNone) {
		t.Fatal("not locked after turn_ended")
	}
	eventually(t, "lock released", 2*time.Second, func() bool { return !(m.Hold() != HoldNone) })
	if took := time.Since(start); took < release-5*time.Millisecond {
		t.Errorf("lock released after %v, want at least %v", took, release)
	}
	wantPresence(t, m, "after the release", false, "stopped")

	m.AssistantMoved(false)
	wantPresence(t, m, "after a non-turn tool call", false, "stopped")
	m.AssistantMoved(true)
	wantPresence(t, m, "after an own-turn move", true, "acting")
	m.AssistantDone()
	wantPresence(t, m, "after end turn", false, "")
}

// TestMovesKeepLock: an assistant that moves more often than the
// lock-release time stays locked until it stops moving.
func TestMovesKeepLock(t *testing.T) {
	m, _ := startedManager(t)
	const release = 100 * time.Millisecond
	m.Timing = Timing{LockRelease: release}
	m.AssistantMoved(true)
	until := time.Now().Add(4 * release)
	for time.Now().Before(until) {
		time.Sleep(25 * time.Millisecond)
		if !(m.Hold() != HoldNone) {
			t.Fatalf("lock released while the assistant moved every 25ms (release %v)", release)
		}
		m.AssistantMoved(false)
	}
	stoppedAt := time.Now()
	eventually(t, "lock released after the moves stopped", 2*time.Second, func() bool { return !(m.Hold() != HoldNone) })
	if took := time.Since(stoppedAt); took > release+400*time.Millisecond {
		t.Errorf("released %v after the last move, want about %v", took, release)
	}
	wantPresence(t, m, "after the release", false, "stopped")
}

// TestOpponentWaitingStatus: "waiting" shows while a call is in flight and
// for the grace after a still_waiting result, then clears; after a
// check-in or an ended wait it clears at once. The real grace keeps the
// status inside a minute of the last call.
func TestOpponentWaitingStatus(t *testing.T) {
	if waitingGrace <= 0 || waitingGrace >= time.Minute {
		t.Fatalf("waitingGrace %v, want under a minute", waitingGrace)
	}
	setWaitingGrace(t, 150*time.Millisecond)

	t.Run("still waiting", func(t *testing.T) {
		m, _ := startedManager(t)
		m.Timing = Timing{WaitCap: 50 * time.Millisecond, CheckIn: time.Minute}
		wantPresence(t, m, "before the wait", false, "")
		done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
		wantPresence(t, m, "in flight", false, "waiting")
		if r := recv(t, done, time.Second); r.out.Kind != WaitStillWaiting {
			t.Fatalf("got %+v, want still_waiting", r.out)
		}
		returned := time.Now()
		wantPresence(t, m, "just after still_waiting", false, "waiting")
		eventually(t, "waiting status cleared", 2*time.Second, func() bool { return m.Opponent() == "" })
		if took := time.Since(returned); took > time.Second {
			t.Errorf("waiting status cleared %v after the call, want about 150ms", took)
		}
	})
	t.Run("renewed by the next call", func(t *testing.T) {
		m, _ := startedManager(t)
		m.Timing = Timing{WaitCap: 50 * time.Millisecond, CheckIn: time.Minute}
		o := waitNow(t, m, WaitRequest{Seat: state.P1})
		// Calls back to back keep the status for longer than one grace.
		until := time.Now().Add(400 * time.Millisecond)
		for time.Now().Before(until) {
			if got := m.Opponent(); got != "waiting" {
				t.Fatalf("between polls Opponent() = %q, want waiting", got)
			}
			o = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(o.Next)})
		}
	})
	t.Run("check in", func(t *testing.T) {
		m, _ := startedManager(t)
		m.Timing = Timing{WaitCap: 30 * time.Millisecond, CheckIn: time.Minute}
		o := waitNow(t, m, WaitRequest{Seat: state.P1})
		if o.Kind != WaitStillWaiting {
			t.Fatalf("got %q, want still_waiting", o.Kind)
		}
		// Within the grace of that poll, the next call checks in.
		m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: 20 * time.Millisecond}
		o = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(o.Next)})
		if o.Kind != WaitCheckIn {
			t.Fatalf("got %q, want check_in", o.Kind)
		}
		wantPresence(t, m, "straight after check_in", false, "")
	})
	t.Run("ended", func(t *testing.T) {
		m, g := startedManager(t)
		m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute}
		done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
		if err := g.SetMode(ModeReview); err != nil {
			t.Fatal(err)
		}
		if r := recv(t, done, time.Second); r.out.Kind != WaitEnded {
			t.Fatalf("got %+v, want ended", r.out)
		}
		wantPresence(t, m, "straight after ended", false, "")
	})
}

// TestOnPresenceHook: the hook runs when a wait starts and returns, when
// the lock is set, cleared or releases itself, and when the waiting grace
// lapses; a tool call that changes nothing does not run it.
func TestOnPresenceHook(t *testing.T) {
	setWaitingGrace(t, 80*time.Millisecond)
	m, g := startedManager(t)
	m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute, LockRelease: time.Minute}
	fired := make(chan struct{}, 256)
	m.OnPresence(func() {
		select {
		case fired <- struct{}{}:
		default:
		}
	})
	drain := func() {
		for {
			select {
			case <-fired:
			default:
				return
			}
		}
	}
	expect := func(what string) {
		t.Helper()
		select {
		case <-fired:
		case <-time.After(time.Second):
			t.Fatalf("hook did not run on %s", what)
		}
	}

	drain()
	done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
	expect("the wait starting")
	drain()
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
	if r := recv(t, done, time.Second); r.out.Kind != WaitTurnEnded {
		t.Fatalf("got %+v, want turn_ended", r.out)
	}
	expect("turn_ended setting the lock")

	drain()
	m.AssistantMoved(false)
	select {
	case <-fired:
		t.Error("hook ran on a move that only refreshed the lock")
	case <-time.After(50 * time.Millisecond):
	}

	drain()
	m.AssistantDone()
	expect("the assistant ending its turn")
	drain()
	m.AssistantMoved(true)
	expect("an own-turn move locking the board")

	m.Timing.LockRelease = 40 * time.Millisecond
	m.AssistantMoved(false) // restart the clock with the short release
	drain()
	expect("the lock releasing itself")
	if got := m.Opponent(); got != "stopped" {
		t.Errorf("Opponent() = %q after the release, want stopped", got)
	}

	m.AssistantDone()
	m.Timing = Timing{WaitCap: 30 * time.Millisecond, CheckIn: time.Minute}
	must(t)(g.EndTurn(state.P2, state.ActorClaude))
	if o := waitNow(t, m, WaitRequest{Seat: state.P1}); o.Kind != WaitStillWaiting {
		t.Fatalf("got %q, want still_waiting", o.Kind)
	}
	drain()
	expect("the waiting grace lapsing")
	if got := m.Opponent(); got != "" {
		t.Errorf("Opponent() = %q after the grace, want empty", got)
	}
}

// TestGameChangesClearLock: a mode switch, checkout, rewind, undo, new
// game or expiry releases the lock and forgets a stop; new events do not.
func TestGameChangesClearLock(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(t *testing.T, g *Game)
		change func(t *testing.T, m *Manager, g *Game)
	}{
		{"mode switch", nil, func(t *testing.T, m *Manager, g *Game) {
			if err := g.SetMode(ModeReview); err != nil {
				t.Fatal(err)
			}
		}},
		{"checkout", func(t *testing.T, g *Game) {
			if _, err := g.Rewind(g.Head(), "alt"); err != nil {
				t.Fatal(err)
			}
			if err := g.Checkout("main"); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, m *Manager, g *Game) {
			if err := g.Checkout("alt"); err != nil {
				t.Fatal(err)
			}
		}},
		{"rewind", nil, func(t *testing.T, m *Manager, g *Game) {
			if _, err := g.Rewind(g.Head(), "what-if"); err != nil {
				t.Fatal(err)
			}
		}},
		{"undo", func(t *testing.T, g *Game) {
			must(t)(g.Draw(state.P1, state.ActorHuman, 1))
		}, func(t *testing.T, m *Manager, g *Game) {
			if _, err := g.Undo(1); err != nil {
				t.Fatal(err)
			}
		}},
		{"new game", nil, func(t *testing.T, m *Manager, g *Game) {
			if _, _, err := m.New(context.Background(), NewGameRequest{ID: "t2", DeckP1: testDeck, DeckP2: testDeck}); err != nil {
				t.Fatal(err)
			}
		}},
		{"expired", nil, func(t *testing.T, m *Manager, g *Game) {
			m.Expire("game expired after idling")
		}},
	}
	for _, c := range cases {
		t.Run(c.name+" acting", func(t *testing.T) {
			m, g := startedManager(t)
			m.Timing = Timing{LockRelease: time.Minute}
			if c.setup != nil {
				c.setup(t, g)
			}
			m.AssistantMoved(true)
			wantPresence(t, m, "locked", true, "acting")
			c.change(t, m, g)
			wantPresence(t, m, "after "+c.name, false, "")
		})
		t.Run(c.name+" stopped", func(t *testing.T) {
			m, g := startedManager(t)
			m.Timing = Timing{LockRelease: 20 * time.Millisecond}
			if c.setup != nil {
				c.setup(t, g)
			}
			m.AssistantMoved(true)
			eventually(t, "stopped", 2*time.Second, func() bool { return m.Opponent() == "stopped" })
			c.change(t, m, g)
			wantPresence(t, m, "after "+c.name, false, "")
		})
	}
	t.Run("new events", func(t *testing.T) {
		m, g := startedManager(t)
		m.Timing = Timing{LockRelease: time.Minute}
		m.AssistantMoved(true)
		must(t)(g.Draw(state.P1, state.ActorHuman, 1))
		wantPresence(t, m, "after a draw", true, "acting")
	})
}

// TestNoLockInReviewWithoutWait: in review mode with no turn handed over,
// the assistant's tool calls never lock the board or show a status.
func TestNoLockInReviewWithoutWait(t *testing.T) {
	m, g := startedManager(t)
	m.Timing = Timing{LockRelease: time.Minute}
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		m.AssistantMoved(false)
		must(t)(g.Draw(state.P2, state.ActorClaude, 1))
	}
	wantPresence(t, m, "after the assistant's review moves", false, "")
}

// holdByKnockOut knocks out p2's Active (it has a Bench) under a wait, so
// the wait asks the assistant to promote and sets a decision hold. It
// returns the outcome.
func holdByKnockOut(t *testing.T, m *Manager, g *Game) WaitOutcome {
	t.Helper()
	done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
	must(t)(g.KnockOut(state.P1, state.ActorHuman, state.Loc{Seat: state.P2, Zone: state.Active}))
	r := recv(t, done, time.Second)
	if r.err != nil || r.out.Kind != WaitDecide || r.out.Action != DecidePromote {
		t.Fatalf("knock-out: %+v, %v; want decide/promote", r.out, r.err)
	}
	return r.out
}

// TestDecisionHold: a decision stop sets a decision hold ("deciding"),
// never a full one, naming the decision; the assistant's next wait ends
// it. A decision hold left for the release time lapses to "stopped" and
// keeps its decision readable until the next wait, which clears both.
func TestDecisionHold(t *testing.T) {
	t.Run("next wait ends it", func(t *testing.T) {
		m, g, _, _ := respondGame(t)
		m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute, LockRelease: time.Minute}
		out := holdByKnockOut(t, m, g)
		if m.Hold() != HoldDecision || m.Opponent() != "deciding" || m.Decision() != DecidePromote {
			t.Fatalf("after the decision stop: hold %q, opponent %q, decision %q; want decision, deciding, promote", m.Hold(), m.Opponent(), m.Decision())
		}
		// The assistant's own moves while deciding keep the hold a
		// decision hold.
		m.AssistantMoved(false)
		if m.Hold() != HoldDecision {
			t.Errorf("a move while deciding changed the hold to %q", m.Hold())
		}
		must(t)(g.Switch(state.P2, state.ActorClaude, 0))
		ctx, cancel := context.WithCancel(context.Background())
		done := startWait(t, ctx, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
		if m.Hold() != HoldNone || m.Decision() != "" || m.Opponent() != "waiting" {
			t.Errorf("waiting again: hold %q, decision %q, opponent %q; want none, none, waiting", m.Hold(), m.Decision(), m.Opponent())
		}
		cancel()
		recv(t, done, time.Second)
	})
	t.Run("lapses", func(t *testing.T) {
		m, g, _, _ := respondGame(t)
		m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute, LockRelease: 40 * time.Millisecond}
		out := holdByKnockOut(t, m, g)
		eventually(t, "the decision hold lapsed", 2*time.Second, func() bool { return m.Opponent() == "stopped" })
		if m.Hold() != HoldNone || m.Decision() != DecidePromote {
			t.Fatalf("after the lapse: hold %q, decision %q; want none and promote", m.Hold(), m.Decision())
		}
		m.Timing.WaitCap = 30 * time.Millisecond
		waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
		if m.Decision() != "" || m.Opponent() == "stopped" {
			t.Errorf("after the next wait: decision %q, opponent %q; want both cleared", m.Decision(), m.Opponent())
		}
	})
	t.Run("own turn is a full hold", func(t *testing.T) {
		m, g := startedManager(t)
		m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute, LockRelease: time.Minute}
		lockByTurnEnded(t, m, g)
		if m.Hold() != HoldFull || m.Opponent() != "acting" || m.Decision() != "" {
			t.Fatalf("the assistant's turn: hold %q, opponent %q, decision %q; want full, acting, none", m.Hold(), m.Opponent(), m.Decision())
		}
		m.AssistantMoved(true)
		if m.Hold() != HoldFull {
			t.Errorf("an own-turn move: hold %q, want full", m.Hold())
		}
	})
}

// TestGameChangesClearDecisionHold: a mode switch, a branch move, an undo,
// a new game and expiry each clear a decision hold and its decision, live
// or lapsed.
func TestGameChangesClearDecisionHold(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(t *testing.T, m *Manager, g *Game)
	}{
		{"mode switch", func(t *testing.T, _ *Manager, g *Game) {
			if err := g.SetMode(ModeReview); err != nil {
				t.Fatal(err)
			}
		}},
		{"branch move", func(t *testing.T, _ *Manager, g *Game) {
			if _, err := g.Rewind(g.Head()-1, "alt"); err != nil {
				t.Fatal(err)
			}
		}},
		{"undo", func(t *testing.T, _ *Manager, g *Game) {
			if _, err := g.Undo(1); err != nil {
				t.Fatal(err)
			}
		}},
		{"new game", func(t *testing.T, m *Manager, _ *Game) {
			if _, _, err := m.New(context.Background(), NewGameRequest{ID: "t2", DeckP1: testDeck, DeckP2: testDeck}); err != nil {
				t.Fatal(err)
			}
		}},
		{"expired", func(t *testing.T, m *Manager, _ *Game) {
			m.Expire("game expired after idling")
		}},
	} {
		for _, lapsed := range []bool{false, true} {
			name := tt.name + " deciding"
			if lapsed {
				name = tt.name + " lapsed"
			}
			t.Run(name, func(t *testing.T) {
				m, g, _, _ := respondGame(t)
				release := time.Minute
				if lapsed {
					release = 30 * time.Millisecond
				}
				m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute, LockRelease: release}
				holdByKnockOut(t, m, g)
				if lapsed {
					eventually(t, "lapsed", 2*time.Second, func() bool { return m.Opponent() == "stopped" })
				}
				tt.change(t, m, g)
				if m.Hold() != HoldNone || m.Decision() != "" || m.Opponent() != "" {
					t.Errorf("after %s: hold %q, decision %q, opponent %q; want all cleared", tt.name, m.Hold(), m.Decision(), m.Opponent())
				}
			})
		}
	}
}

// TestOnPresenceHookOnDeciding: the board's presence hook runs when a
// decision stop sets the deciding status.
func TestOnPresenceHookOnDeciding(t *testing.T) {
	m, g, _, _ := respondGame(t)
	m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute, LockRelease: time.Minute}
	statuses := make(chan string, 256)
	m.OnPresence(func() {
		select {
		case statuses <- m.Opponent():
		default:
		}
	})
	holdByKnockOut(t, m, g)
	deadline := time.After(time.Second)
	for {
		select {
		case s := <-statuses:
			if s == "deciding" {
				return
			}
		case <-deadline:
			t.Fatal("the hook did not run with the deciding status")
		}
	}
}
