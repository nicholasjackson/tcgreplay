package game

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// startedManager is startedGame that also returns the manager: p1 to move,
// turn 1, play mode.
func startedManager(t *testing.T) (*Manager, *Game) {
	t.Helper()
	m, g := newTestGame(t)
	hu, cl := state.ActorHuman, state.ActorClaude
	if l, _ := g.State().Find("p1-1"); l.Zone != state.Hand {
		must(t)(g.Move(state.P1, hu, "p1-1", state.Loc{Zone: state.Hand}, "", ""))
	}
	if l, _ := g.State().Find("p2-1"); l.Zone != state.Hand {
		must(t)(g.Move(state.P2, cl, "p2-1", state.Loc{Zone: state.Hand}, "", ""))
	}
	must(t)(g.PlaceActive(state.P1, hu, "p1-1"))
	must(t)(g.PlaceActive(state.P2, cl, "p2-1"))
	must(t)(g.StartGame(state.P1, hu))
	return m, g
}

type waitResult struct {
	out WaitOutcome
	err error
	// took is how long the call blocked.
	took time.Duration
}

// startWait runs a Wait in a goroutine and returns once the call has
// registered (its session is set and it is in flight), so a change made
// after it lands under the call.
func startWait(t *testing.T, ctx context.Context, m *Manager, req WaitRequest) <-chan waitResult {
	t.Helper()
	p := m.presenceRef()
	p.mu.Lock()
	gen := p.callGen
	p.mu.Unlock()
	done := make(chan waitResult, 1)
	go func() {
		start := time.Now()
		out, err := m.Wait(ctx, req)
		done <- waitResult{out, err, time.Since(start)}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		p.mu.Lock()
		registered := p.callGen > gen
		p.mu.Unlock()
		if registered {
			return done
		}
		if time.Now().After(deadline) {
			t.Fatal("wait call never registered")
		}
		time.Sleep(time.Millisecond)
	}
}

func recv(t *testing.T, done <-chan waitResult, within time.Duration) waitResult {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(within):
		t.Fatalf("wait did not return within %v", within)
		return waitResult{}
	}
}

func waitNow(t *testing.T, m *Manager, req WaitRequest) WaitOutcome {
	t.Helper()
	out, err := m.Wait(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func intp(n int) *int { return &n }

// TestWaitTurnEndedPromptly: a wait on the player's seat returns within a
// fraction of a second of that seat ending its turn, well under the cap,
// with the new turn, whose turn it is, the hand-off text and where the
// player's moves start.
func TestWaitTurnEndedPromptly(t *testing.T) {
	m, g := startedManager(t)
	m.Timing = Timing{WaitCap: 5 * time.Second, CheckIn: time.Minute}
	must(t)(g.EndTurn(state.P1, state.ActorHuman))  // turn 1 p1 -> p2
	must(t)(g.EndTurn(state.P2, state.ActorClaude)) // the assistant ends turn 1 p2 -> turn 2 p1
	since := g.TurnStartedAt(state.P1)

	done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
	ended := time.Now()
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
	r := recv(t, done, 2*time.Second)
	if lag := time.Since(ended); lag > 500*time.Millisecond {
		t.Errorf("turn_ended came %v after end_turn, want under 500ms", lag)
	}
	if r.err != nil {
		t.Fatal(r.err)
	}
	o := r.out
	if o.Kind != WaitTurnEnded {
		t.Fatalf("kind %q, want %q (%+v)", o.Kind, WaitTurnEnded, o)
	}
	if o.Seat != state.P1 || o.Turn != 2 || o.TurnSeat != state.P2 {
		t.Errorf("seat %s turn %d turn seat %s, want p1 2 p2", o.Seat, o.Turn, o.TurnSeat)
	}
	if o.Since != since {
		t.Errorf("since %d, want %d", o.Since, since)
	}
	if head := g.Head(); o.Next != head {
		t.Errorf("next %d, want head %d", o.Next, head)
	}
	want := fmt.Sprintf("Your turn in tcgreplay (game t1, branch main). Call get_log since %d and play turn 2 (p2's turn).", since)
	if o.Prompt != want {
		t.Errorf("prompt\n got %q\nwant %q", o.Prompt, want)
	}
}

// TestWaitFreshDuringAssistantTurn: a fresh play-mode wait during the
// assistant's own turn is refused when it has acted, but reports the
// player's turn as ended at once when nothing but the turn change
// happened since.
func TestWaitFreshDuringAssistantTurn(t *testing.T) {
	m, g := startedManager(t)
	m.Timing = Timing{WaitCap: 5 * time.Second, CheckIn: time.Minute}
	since := g.TurnStartedAt(state.P1)
	must(t)(g.EndTurn(state.P1, state.ActorHuman))

	start := time.Now()
	o := waitNow(t, m, WaitRequest{Seat: state.P1})
	if took := time.Since(start); took > time.Second {
		t.Errorf("took %v, want immediate", took)
	}
	if o.Kind != WaitTurnEnded || o.Since != since || o.Turn != 1 || o.TurnSeat != state.P2 {
		t.Fatalf("got %+v, want turn_ended since %d turn 1 p2", o, since)
	}

	must(t)(g.Draw(state.P2, state.ActorClaude, 1))
	if o, err := m.Wait(context.Background(), WaitRequest{Seat: state.P1}); err == nil {
		t.Fatalf("fresh wait during p2's turn after a move: %+v, want an error", o)
	}
}

// TestWaitReviewOtherSeatEnd: in review mode, the other seat ending its
// turn does not end a wait on the handed-over seat; that seat ending its
// own turn does.
func TestWaitReviewOtherSeatEnd(t *testing.T) {
	m, g := startedManager(t)
	m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute}
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	// p1 is to move; hand p2 over.
	done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P2})
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
	select {
	case r := <-done:
		t.Fatalf("p1 ending its turn ended the wait on p2: %+v", r.out)
	case <-time.After(150 * time.Millisecond):
	}
	must(t)(g.EndTurn(state.P2, state.ActorClaude))
	r := recv(t, done, 2*time.Second)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.out.Kind != WaitTurnEnded || r.out.Seat != state.P2 || r.out.Turn != 2 || r.out.TurnSeat != state.P1 {
		t.Fatalf("got %+v, want turn_ended for p2, turn 2 p1", r.out)
	}
}

// TestWaitCheckIn: with no board activity for the check-in time the wait
// reports a check-in; activity more often than that never does.
func TestWaitCheckIn(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		m, _ := startedManager(t)
		m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: 100 * time.Millisecond}
		start := time.Now()
		o := waitNow(t, m, WaitRequest{Seat: state.P1})
		took := time.Since(start)
		if o.Kind != WaitCheckIn {
			t.Fatalf("kind %q, want check_in", o.Kind)
		}
		if took < 90*time.Millisecond || took > time.Second {
			t.Errorf("check_in after %v, want about 100ms", took)
		}
	})
	t.Run("active", func(t *testing.T) {
		m, _ := startedManager(t)
		m.Timing = Timing{WaitCap: 500 * time.Millisecond, CheckIn: 120 * time.Millisecond}
		stop := make(chan struct{})
		ticked := make(chan struct{})
		go func() {
			defer close(ticked)
			tk := time.NewTicker(40 * time.Millisecond)
			defer tk.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tk.C:
					m.BoardActivity()
				}
			}
		}()
		o := waitNow(t, m, WaitRequest{Seat: state.P1})
		close(stop)
		<-ticked
		if o.Kind != WaitStillWaiting {
			t.Fatalf("kind %q with activity every 40ms (check-in 120ms), want still_waiting", o.Kind)
		}
	})
}

// TestWaitAfterCheckIn: the call after a check-in starts a fresh clock, or
// reports the turn ended when the player ended it meanwhile.
func TestWaitAfterCheckIn(t *testing.T) {
	t.Run("fresh clock", func(t *testing.T) {
		m, _ := startedManager(t)
		m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: 100 * time.Millisecond}
		o := waitNow(t, m, WaitRequest{Seat: state.P1})
		if o.Kind != WaitCheckIn {
			t.Fatalf("kind %q, want check_in", o.Kind)
		}
		start := time.Now()
		o = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(o.Next)})
		took := time.Since(start)
		if o.Kind != WaitCheckIn {
			t.Fatalf("continuing kind %q, want check_in", o.Kind)
		}
		if took < 90*time.Millisecond {
			t.Errorf("continuing call checked in after %v, want a fresh 100ms clock", took)
		}
	})
	t.Run("ended while away", func(t *testing.T) {
		m, g := startedManager(t)
		m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: 100 * time.Millisecond}
		since := g.TurnStartedAt(state.P1)
		o := waitNow(t, m, WaitRequest{Seat: state.P1})
		if o.Kind != WaitCheckIn {
			t.Fatalf("kind %q, want check_in", o.Kind)
		}
		must(t)(g.EndTurn(state.P1, state.ActorHuman))
		start := time.Now()
		o = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(o.Next)})
		if took := time.Since(start); took > 500*time.Millisecond {
			t.Errorf("took %v, want immediate", took)
		}
		if o.Kind != WaitTurnEnded || o.Since != since || o.Turn != 1 || o.TurnSeat != state.P2 {
			t.Fatalf("got %+v, want turn_ended since %d turn 1 p2", o, since)
		}
	})
}

// TestWaitEndReasons: a mode switch, a new game, an expired game, a branch
// checkout, a rewind, a jump and an undo each end the wait with their
// reason, whether during a call or between calls.
func TestWaitEndReasons(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(t *testing.T, g *Game)
		change func(t *testing.T, m *Manager, g *Game)
		reason string
	}{
		{"mode switch", nil, func(t *testing.T, m *Manager, g *Game) {
			if err := g.SetMode(ModeReview); err != nil {
				t.Fatal(err)
			}
		}, EndModeChanged},
		{"new game", nil, func(t *testing.T, m *Manager, g *Game) {
			if _, _, err := m.New(context.Background(), NewGameRequest{ID: "t2", DeckP1: testDeck, DeckP2: testDeck}); err != nil {
				t.Fatal(err)
			}
		}, EndGameReplaced},
		{"expired", nil, func(t *testing.T, m *Manager, g *Game) {
			m.Expire("game expired after idling")
		}, EndGameExpired},
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
		}, EndBranchSwitched},
		{"rewind", nil, func(t *testing.T, m *Manager, g *Game) {
			if _, err := g.Rewind(g.Head(), "what-if"); err != nil {
				t.Fatal(err)
			}
		}, EndRewound},
		{"jump", nil, func(t *testing.T, m *Manager, g *Game) {
			if _, err := g.JumpTo(g.TurnStartedAt(state.P1), "jump"); err != nil {
				t.Fatal(err)
			}
		}, EndRewound},
		{"undo", func(t *testing.T, g *Game) {
			must(t)(g.Draw(state.P1, state.ActorHuman, 1))
		}, func(t *testing.T, m *Manager, g *Game) {
			if _, err := g.Undo(1); err != nil {
				t.Fatal(err)
			}
		}, EndRewound},
	}
	for _, c := range cases {
		t.Run(c.name+" during", func(t *testing.T) {
			m, g := startedManager(t)
			m.Timing = Timing{WaitCap: 2 * time.Second, CheckIn: time.Minute}
			if c.setup != nil {
				c.setup(t, g)
			}
			done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
			c.change(t, m, g)
			r := recv(t, done, time.Second)
			if r.err != nil {
				t.Fatal(r.err)
			}
			if r.out.Kind != WaitEnded || r.out.Reason != c.reason {
				t.Fatalf("got %q/%q, want ended/%s", r.out.Kind, r.out.Reason, c.reason)
			}
		})
		t.Run(c.name+" between", func(t *testing.T) {
			m, g := startedManager(t)
			m.Timing = Timing{WaitCap: 30 * time.Millisecond, CheckIn: time.Minute}
			if c.setup != nil {
				c.setup(t, g)
			}
			o := waitNow(t, m, WaitRequest{Seat: state.P1})
			if o.Kind != WaitStillWaiting {
				t.Fatalf("first call %q, want still_waiting", o.Kind)
			}
			c.change(t, m, g)
			o = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(o.Next)})
			if o.Kind != WaitEnded || o.Reason != c.reason {
				t.Fatalf("continuing call got %q/%q, want ended/%s", o.Kind, o.Reason, c.reason)
			}
		})
	}
}

// TestWaitCap: no call blocks longer than the cap.
func TestWaitCap(t *testing.T) {
	m, _ := startedManager(t)
	m.Timing = Timing{WaitCap: 100 * time.Millisecond, CheckIn: time.Minute}
	start := time.Now()
	o := waitNow(t, m, WaitRequest{Seat: state.P1})
	took := time.Since(start)
	if o.Kind != WaitStillWaiting {
		t.Fatalf("kind %q, want still_waiting", o.Kind)
	}
	if took < 90*time.Millisecond || took > 400*time.Millisecond {
		t.Errorf("returned after %v, want about the 100ms cap", took)
	}
	// A continuing call is capped too.
	start = time.Now()
	o = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(o.Next)})
	if took := time.Since(start); o.Kind != WaitStillWaiting || took > 400*time.Millisecond {
		t.Errorf("continuing call %q after %v, want still_waiting by the 100ms cap", o.Kind, took)
	}
}

// TestWaitSupersede: a second call takes over from one still in flight,
// which returns still_waiting at once.
func TestWaitSupersede(t *testing.T) {
	m, g := startedManager(t)
	m.Timing = Timing{WaitCap: 3 * time.Second, CheckIn: time.Minute}
	a := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := startWait(t, ctx, m, WaitRequest{Seat: state.P1})
	r := recv(t, a, time.Second)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if r.out.Kind != WaitStillWaiting {
		t.Fatalf("superseded call %q, want still_waiting", r.out.Kind)
	}
	if r.took > 500*time.Millisecond {
		t.Errorf("superseded call took %v, want prompt", r.took)
	}
	// The second call carries on and sees the turn end.
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
	rb := recv(t, b, time.Second)
	if rb.err != nil || rb.out.Kind != WaitTurnEnded {
		t.Fatalf("taking-over call got %+v, %v, want turn_ended", rb.out, rb.err)
	}
}

// TestWaitCancel: a cancelled call returns at once with an error.
func TestWaitCancel(t *testing.T) {
	m, _ := startedManager(t)
	m.Timing = Timing{WaitCap: 3 * time.Second, CheckIn: time.Minute}
	ctx, cancel := context.WithCancel(context.Background())
	done := startWait(t, ctx, m, WaitRequest{Seat: state.P1})
	cancel()
	r := recv(t, done, time.Second)
	if !errors.Is(r.err, context.Canceled) {
		t.Fatalf("err %v, want context.Canceled", r.err)
	}
	if r.took > 500*time.Millisecond {
		t.Errorf("cancelled call took %v, want prompt", r.took)
	}
}

// TestWaitNoLeaks: after many waits that time out, are cancelled, are
// superseded or end on mode switches, the game's listeners and the running
// goroutines are back where they started.
func TestWaitNoLeaks(t *testing.T) {
	m, g := startedManager(t)
	m.Timing = Timing{WaitCap: 3 * time.Millisecond, CheckIn: time.Minute}
	// One wait first, so any one-time setup is in the baseline.
	waitNow(t, m, WaitRequest{Seat: state.P1})
	listeners := func() int {
		g.mu.Lock()
		defer g.mu.Unlock()
		return len(g.listeners)
	}
	baseListeners := listeners()
	baseGoroutines := runtime.NumGoroutine()

	for i := range 200 {
		switch i % 4 {
		case 0: // times out
			if o := waitNow(t, m, WaitRequest{Seat: state.P1}); o.Kind != WaitStillWaiting {
				t.Fatalf("iteration %d: %q, want still_waiting", i, o.Kind)
			}
		case 1: // cancelled
			ctx, cancel := context.WithCancel(context.Background())
			out, err := func() (WaitOutcome, error) {
				cancel()
				return m.Wait(ctx, WaitRequest{Seat: state.P1})
			}()
			if err == nil && out.Kind != WaitStillWaiting {
				t.Fatalf("iteration %d: cancelled call %+v", i, out)
			}
		case 2: // superseded (the second call then times out)
			o1 := make(chan WaitOutcome, 1)
			p := m.presenceRef()
			p.mu.Lock()
			gen := p.callGen
			p.mu.Unlock()
			go func() {
				out, _ := m.Wait(context.Background(), WaitRequest{Seat: state.P1})
				o1 <- out
			}()
			for {
				p.mu.Lock()
				ok := p.callGen > gen
				p.mu.Unlock()
				if ok {
					break
				}
				runtime.Gosched()
			}
			waitNow(t, m, WaitRequest{Seat: state.P1})
			if o := <-o1; o.Kind != WaitStillWaiting {
				t.Fatalf("iteration %d: superseded call %q", i, o.Kind)
			}
		case 3: // ended by a mode switch, then switched back
			m.Timing.WaitCap = time.Second
			done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
			if err := g.SetMode(ModeReview); err != nil {
				t.Fatal(err)
			}
			r := recv(t, done, time.Second)
			if r.out.Kind != WaitEnded || r.out.Reason != EndModeChanged {
				t.Fatalf("iteration %d: %+v, want ended/mode_changed", i, r.out)
			}
			if err := g.SetMode(ModePlay); err != nil {
				t.Fatal(err)
			}
			m.Timing.WaitCap = 3 * time.Millisecond
		}
	}

	if got := listeners(); got != baseListeners {
		t.Errorf("game has %d listeners after the waits, want %d", got, baseListeners)
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > baseGoroutines {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines after the waits, want %d", runtime.NumGoroutine(), baseGoroutines)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
