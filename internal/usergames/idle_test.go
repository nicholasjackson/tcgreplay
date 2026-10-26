package usergames

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// idleRegistry is newRegistry with an idle time of idle.
func idleRegistry(t *testing.T, idle time.Duration, now func() time.Time) *Registry {
	t.Helper()
	r := newRegistry(t, t.TempDir(), now)
	r.cfg.Idle = idle
	t.Cleanup(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, e := range r.entries {
			e.stop()
		}
	})
	return r
}

// checkExpired fails unless m's game has expired with a notice naming the
// idle time and asking for the record.
func checkExpired(t *testing.T, m *game.Manager, idleText string) {
	t.Helper()
	g, err := m.Current()
	if err == nil {
		t.Fatalf("game %s still open, want it expired", g.ID)
	}
	if !errors.Is(err, game.ErrExpired) {
		t.Fatalf("Current() error %q does not match game.ErrExpired", err)
	}
	for _, want := range []string{"expired", idleText, "record", "import_log"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expiry notice %q does not contain %q", err, want)
		}
	}
	if m.Notice() != err.Error() {
		t.Errorf("Notice() = %q, want the Current() error %q", m.Notice(), err)
	}
}

// checkOpen fails unless m's game is the game want.
func checkOpen(t *testing.T, m *game.Manager, want *game.Game) {
	t.Helper()
	g, err := m.Current()
	if err != nil {
		t.Fatalf("game gone: %v", err)
	}
	if g != want {
		t.Fatalf("current game is %s, want the original %s", g.ID, want.ID)
	}
}

// TestIdleGameExpires: with an idle time of 2 minutes, a game left alone
// for exactly 2 minutes is still open, and one second more makes the
// account's next action find it expired, with a notice naming 2 minutes
// and asking for the record. The account holds no place afterwards.
func TestIdleGameExpires(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := idleRegistry(t, 2*time.Minute, clock.Now)
	e := use(t, r, alice)
	startGame(t, e.M, "a-game")
	g, _ := e.M.Current()

	clock.Advance(2 * time.Minute)
	checkOpen(t, r.Get(alice).M, g)

	clock.Advance(time.Second)
	again := use(t, r, alice)
	if again != e {
		t.Fatal("expiry replaced the account's entry")
	}
	checkExpired(t, again.M, "2 minutes")
}

// TestOpenBoardDoesNotKeepGame: lookups that are not activity (Get, as an
// open board's live connection and card lookups make) every 30 seconds do
// not keep a game; the lookup after the idle time finds it expired.
func TestOpenBoardDoesNotKeepGame(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := idleRegistry(t, 2*time.Minute, clock.Now)
	e := use(t, r, alice)
	startGame(t, e.M, "a-game")
	g, _ := e.M.Current()

	for i := 0; i < 4; i++ { // 30s, 1m, 1m30s, 2m
		clock.Advance(30 * time.Second)
		checkOpen(t, r.Get(alice).M, g)
	}
	clock.Advance(30 * time.Second)
	checkExpired(t, r.Get(alice).M, "2 minutes")
}

// TestActivityKeepsGame: a game acted on every minute for 5 minutes, with
// sweeps in between, is still the same game with its moves intact.
func TestActivityKeepsGame(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := idleRegistry(t, 2*time.Minute, clock.Now)
	e := use(t, r, alice)
	startGame(t, e.M, "a-game")
	g, _ := e.M.Current()
	if _, err := g.Move(state.P1, state.ActorHuman, "p1-1", state.Loc{Seat: state.P1, Zone: state.Discard}, "", ""); err != nil {
		t.Fatalf("move: %v", err)
	}
	head := g.Head()

	for i := 0; i < 5; i++ {
		clock.Advance(time.Minute)
		r.Sweep()
		checkOpen(t, use(t, r, alice).M, g)
		r.Sweep()
	}
	if got := g.Head(); got != head {
		t.Errorf("head is %d after 5 minutes of activity, want %d", got, head)
	}
	if loc, err := g.State().Find("p1-1"); err != nil || loc.Zone != state.Discard {
		t.Errorf("p1-1 is at %+v after 5 minutes of activity, want the discard", loc)
	}
}

// TestIdleZeroNeverExpires: with an idle time of 0, a game left alone for
// two days, through sweeps, is still open and its entry is kept.
func TestIdleZeroNeverExpires(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := idleRegistry(t, 0, clock.Now)
	e := use(t, r, alice)
	startGame(t, e.M, "a-game")
	g, _ := e.M.Current()

	clock.Advance(48 * time.Hour)
	r.Sweep()
	again := r.Get(alice)
	if again != e {
		t.Fatal("with no idle time, a sweep two days on removed the account's entry")
	}
	checkOpen(t, again.M, g)
}

// TestSweepExpiresIdleGame: a sweep expires a game nobody touches, and an
// open board connection receives the notice without a reload; a sweep
// before the idle time leaves it open.
func TestSweepExpiresIdleGame(t *testing.T) {
	clock := newClock()
	reg, ts := boardSite(t, clock)
	reg.cfg.Idle = 2 * time.Minute
	e := use(t, reg, alice) // after the write above, under reg.mu
	startGame(t, e.M, "a-game")
	g, _ := e.M.Current()

	a := dialBoard(t, ts, "alice")
	var st boardState
	a.next(t, "state", &st, 5*time.Second)
	a.drain(200 * time.Millisecond)

	clock.Advance(2 * time.Minute)
	reg.Sweep()
	checkOpen(t, e.M, g)

	clock.Advance(time.Second)
	reg.Sweep()
	var msg map[string]any
	a.next(t, "state", &msg, 2*time.Second)
	notice, _ := msg["notice"].(string)
	if msg["snapshot"] != nil || !strings.Contains(notice, "2 minutes") || !strings.Contains(notice, "record") {
		t.Errorf("open board after the sweep: snapshot %v notice %q, want no snapshot and the expiry notice", msg["snapshot"], notice)
	}
	checkExpired(t, e.M, "2 minutes")
}

// TestSweepPurgesIdleAccounts: an account whose place was freed by expiry
// is removed a day after its last activity, closing its open board, and a
// later lookup gets a fresh entry; an account still holding a place is
// kept.
func TestSweepPurgesIdleAccounts(t *testing.T) {
	clock := newClock()
	reg, ts := boardSite(t, clock)
	reg.cfg.Idle = 2 * time.Minute
	start := clock.Now()
	ea := use(t, reg, alice)
	startGame(t, ea.M, "a-game")

	a := dialBoard(t, ts, "alice")
	var st boardState
	a.next(t, "state", &st, 5*time.Second)

	clock.Advance(2*time.Minute + time.Second)
	reg.Sweep() // expires alice's game, freeing her place
	checkExpired(t, ea.M, "2 minutes")

	// Bob acts just before alice's day is up, so he still holds a place.
	clock.Advance(24*time.Hour - 3*time.Minute)
	eb := use(t, reg, bob)
	startGame(t, eb.M, "b-game")
	gb, _ := eb.M.Current()

	// A day after alice's last activity exactly: not yet removed.
	clock.Advance(start.Add(24 * time.Hour).Sub(clock.Now()))
	reg.Sweep()
	reg.mu.Lock()
	kept := reg.entries[alice.ID] == ea
	reg.mu.Unlock()
	if !kept {
		t.Fatal("alice's entry was removed at exactly a day")
	}

	clock.Advance(time.Second)
	reg.Sweep()
	select {
	case <-a.err:
	case <-time.After(2 * time.Second):
		t.Fatal("alice's open board stayed connected after her entry was removed")
	}
	fresh := reg.Get(alice)
	if fresh == ea || fresh.M == ea.M {
		t.Fatal("a lookup after the purge returned alice's old entry")
	}
	if n := fresh.M.Notice(); n != "" {
		t.Errorf("alice's fresh entry carries notice %q", n)
	}
	if fresh.Handle != ea.Handle {
		t.Errorf("alice's handle changed across the purge: %q then %q", ea.Handle, fresh.Handle)
	}

	if again := reg.Get(bob); again != eb {
		t.Fatal("the sweep removed bob, who still holds a place")
	}
	checkOpen(t, eb.M, gb)
}

// TestRunSweeps: Run sweeps on its own, so a game nobody touches expires,
// and it stops when its context ends.
func TestRunSweeps(t *testing.T) {
	clock := newClock()
	r := idleRegistry(t, 2*time.Second, clock.Now) // sweeps every second
	e := use(t, r, alice)
	startGame(t, e.M, "a-game")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	clock.Advance(3 * time.Second)
	deadline := time.After(5 * time.Second)
	for e.M.Notice() == "" {
		select {
		case <-deadline:
			t.Fatal("Run did not expire the idle game within 5 seconds")
		case <-time.After(50 * time.Millisecond):
		}
	}
	checkExpired(t, e.M, "2 seconds")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after its context ended")
	}
}

func TestHumanDuration(t *testing.T) {
	for _, tt := range []struct {
		d    time.Duration
		want string
	}{
		{time.Second, "1 second"},
		{30 * time.Second, "30 seconds"},
		{time.Minute, "1 minute"},
		{2 * time.Minute, "2 minutes"},
		{60 * time.Minute, "60 minutes"},
		{119 * time.Minute, "119 minutes"},
		{2 * time.Hour, "2 hours"},
		{3 * time.Hour, "3 hours"},
	} {
		if got := humanDuration(tt.d); got != tt.want {
			t.Errorf("humanDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestSweepEvery(t *testing.T) {
	for _, tt := range []struct {
		idle, want time.Duration
	}{
		{0, 30 * time.Second},
		{2 * time.Second, time.Second},
		{8 * time.Second, 2 * time.Second},
		{time.Minute, 15 * time.Second},
		{2 * time.Minute, 30 * time.Second},
		{60 * time.Minute, 30 * time.Second},
	} {
		r := New(Config{Idle: tt.idle})
		if got := r.sweepEvery(); got != tt.want {
			t.Errorf("sweepEvery with idle %v = %v, want %v", tt.idle, got, tt.want)
		}
	}
}

// TestWaitingDoesNotKeepGame: a game whose only traffic is an assistant
// waiting on the player (resolving quietly every 30 seconds, as each
// wait_for_turn call does) expires after the same idle time as a game with
// no assistant, and the expiry ends the wait in progress with
// game_expired.
func TestWaitingDoesNotKeepGame(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := idleRegistry(t, 2*time.Minute, clock.Now)
	e := use(t, r, alice)
	startGame(t, e.M, "a-game")
	g, _ := e.M.Current()
	if err := g.SetMode(game.ModeReview); err != nil {
		t.Fatal(err)
	}
	e.M.Timing = game.Timing{WaitCap: 10 * time.Second, CheckIn: time.Hour}

	type result struct {
		o   game.WaitOutcome
		err error
	}
	done := make(chan result, 1)
	go func() {
		o, err := e.M.Wait(context.Background(), game.WaitRequest{Seat: state.P1})
		done <- result{o, err}
	}()

	quiet := func() {
		t.Helper()
		m, _, err := r.ResolveMCPQuiet(context.Background(), tokenExtra(alice.ID, alice.Email))
		if err != nil || m != e.M {
			t.Fatalf("quiet lookup: %v, %v", m, err)
		}
	}
	for i := 0; i < 4; i++ { // 30s, 1m, 1m30s, 2m
		clock.Advance(30 * time.Second)
		quiet()
		r.Sweep()
		checkOpen(t, e.M, g)
	}
	select {
	case res := <-done:
		t.Fatalf("wait returned before the game expired: %+v, %v", res.o, res.err)
	default:
	}

	clock.Advance(time.Second)
	quiet()
	r.Sweep()
	checkExpired(t, e.M, "2 minutes")
	select {
	case res := <-done:
		if res.err != nil || res.o.Kind != game.WaitEnded || res.o.Reason != game.EndGameExpired {
			t.Fatalf("wait ended with %+v, %v; want ended, game_expired", res.o, res.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the expiry did not end the wait")
	}
}
