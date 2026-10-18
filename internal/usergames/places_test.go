package usergames

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
)

// placesRegistry is idleRegistry with a cap of max places.
func placesRegistry(t *testing.T, max int, idle time.Duration, now func() time.Time) *Registry {
	t.Helper()
	r := idleRegistry(t, idle, now)
	r.cfg.MaxUsers = max
	return r
}

// checkBusyErr fails unless err is the busy refusal at a cap of max.
func checkBusyErr(t *testing.T, what string, err error, max int) {
	t.Helper()
	if !errors.Is(err, hostauth.ErrBusy) {
		t.Fatalf("%s: %v, want an error matching hostauth.ErrBusy", what, err)
	}
	if want := hostauth.BusyError(max).Error(); err.Error() != want {
		t.Errorf("%s: %q, want %q", what, err, want)
	}
}

// holding returns how many entries hold a place.
func (r *Registry) holding() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.entries {
		if e.holds {
			n++
		}
	}
	return n
}

// holdsPlace reports whether acct's entry holds a place.
func (r *Registry) holdsPlace(acct hostauth.Account) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[acct.ID]
	return e != nil && e.holds
}

// TestExpiredGameFreesPlace: at a cap of 1 with a 2 minute idle time, A
// takes the place and starts a game and B is refused as busy; once A's
// game has gone 2 minutes and 1 second without activity, B's next sign-in
// (Admit sweeps first) is let in and can start a game, and A's game has
// expired.
func TestExpiredGameFreesPlace(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := placesRegistry(t, 1, 2*time.Minute, clock.Now)
	if err := r.Admit(alice); err != nil {
		t.Fatalf("Admit(A) with a free place: %v", err)
	}
	a := use(t, r, alice)
	startGame(t, a.M, "game-a")

	checkBusyErr(t, "Admit(B) at the cap", r.Admit(bob), 1)
	clock.Advance(2 * time.Minute)
	checkBusyErr(t, "Admit(B) with A idle exactly 2 minutes", r.Admit(bob), 1)

	clock.Advance(time.Second)
	if err := r.Admit(bob); err != nil {
		t.Fatalf("Admit(B) after A's game expired: %v", err)
	}
	checkExpired(t, a.M, "2 minutes")
	if r.holdsPlace(alice) || !r.holdsPlace(bob) {
		t.Errorf("A holds a place: %v, B: %v; want only B", r.holdsPlace(alice), r.holdsPlace(bob))
	}
	b := use(t, r, bob)
	startGame(t, b.M, "game-b")
	if g, err := b.M.Current(); err != nil || g.ID != "game-b" {
		t.Errorf("B's game after taking the freed place: %v %v", g, err)
	}
}

// TestSweepFreesPlace: the background sweep frees an idle game's place
// on its own, before anyone else signs in.
func TestSweepFreesPlace(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := placesRegistry(t, 1, 2*time.Minute, clock.Now)
	startGame(t, use(t, r, alice).M, "game-a")
	clock.Advance(2*time.Minute + time.Second)
	r.Sweep()
	if n := r.holding(); n != 0 {
		t.Fatalf("%d places held after the sweep, want 0", n)
	}
	if err := r.Admit(bob); err != nil {
		t.Errorf("Admit(B) after the sweep: %v", err)
	}
}

// TestNeverActedGivesUpPlace: an account that signs in (Admit) and never
// acts holds its place for the idle time and gives it up after it.
func TestNeverActedGivesUpPlace(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := placesRegistry(t, 1, 2*time.Minute, clock.Now)
	if err := r.Admit(alice); err != nil {
		t.Fatal(err)
	}
	clock.Advance(2 * time.Minute)
	checkBusyErr(t, "Admit(B) within A's idle time", r.Admit(bob), 1)
	clock.Advance(time.Second)
	if err := r.Admit(bob); err != nil {
		t.Fatalf("Admit(B) after A sat idle past the idle time: %v", err)
	}
	if r.holdsPlace(alice) {
		t.Error("A still holds a place")
	}
}

// TestBusyRefusalIsNotActivity: a refused account is not recorded: a new
// account gets no entry, and an existing entry without a place keeps its
// last activity.
func TestBusyRefusalIsNotActivity(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := placesRegistry(t, 1, 0, clock.Now)
	use(t, r, alice)
	if _, err := r.Use(bob); err == nil {
		t.Fatal("Use(B) at the cap succeeded")
	}
	r.mu.Lock()
	_, ok := r.entries[bob.ID]
	r.mu.Unlock()
	if ok {
		t.Error("a refused new account has an entry")
	}

	b := r.Get(bob) // an open board: an entry with no place
	before := r.LastActive(b)
	clock.Advance(time.Minute)
	checkBusyErr(t, "Use(B) with an entry but no place", func() error { _, err := r.Use(bob); return err }(), 1)
	if got := r.LastActive(b); !got.Equal(before) {
		t.Errorf("refused Use moved B's last activity from %v to %v", before, got)
	}
}

// TestAdmitLogsRefusal: a refused sign-in is logged with the account.
func TestAdmitLogsRefusal(t *testing.T) {
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })
	r := placesRegistry(t, 1, 0, nil)
	if err := r.Admit(alice); err != nil {
		t.Fatal(err)
	}
	r.Admit(bob)
	if want := "usergames: refused " + bob.Email; !strings.Contains(logs.String(), want) {
		t.Errorf("log %q does not record %q", logs.String(), want)
	}
}

// TestFreedPlaceBackOnNextAction: A's game expires and B takes the only
// place; A's next action is told the server is busy, and once B's place
// is released, A's next action takes it back, finding its game expired.
// With a place to spare, an expired account gets one back at once.
func TestFreedPlaceBackOnNextAction(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := placesRegistry(t, 1, 2*time.Minute, clock.Now)
	a := use(t, r, alice)
	startGame(t, a.M, "game-a")
	clock.Advance(2*time.Minute + time.Second)
	if err := r.Admit(bob); err != nil {
		t.Fatalf("Admit(B) after A's game expired: %v", err)
	}

	_, err := r.Use(alice)
	checkBusyErr(t, "A's next action with B holding the place", err, 1)
	if r.holdsPlace(alice) {
		t.Error("A took a place over the cap")
	}

	r.Release(bob.ID)
	again, err := r.Use(alice)
	if err != nil {
		t.Fatalf("A's next action after B's place was released: %v", err)
	}
	if again != a {
		t.Error("A's entry was replaced; want the same entry back")
	}
	if !r.holdsPlace(alice) {
		t.Error("A holds no place after taking it back")
	}
	checkExpired(t, a.M, "2 minutes")

	r2 := placesRegistry(t, 2, 2*time.Minute, clock.Now)
	use(t, r2, alice)
	clock.Advance(2*time.Minute + time.Second)
	r2.Sweep()
	if r2.holdsPlace(alice) {
		t.Fatal("A still holds a place after expiring")
	}
	use(t, r2, alice)
	if !r2.holdsPlace(alice) {
		t.Error("A's next action with a place to spare did not take one")
	}
}

// TestHolderAlwaysAdmitted: at the cap an account already holding a place
// is always admitted again and may act; Release frees its place for a
// newcomer and closes its entry; releasing an unknown account is a no-op.
func TestHolderAlwaysAdmitted(t *testing.T) {
	r := placesRegistry(t, 1, 0, nil)
	for i := 0; i < 3; i++ {
		if err := r.Admit(alice); err != nil {
			t.Fatalf("Admit(A) #%d holding the place: %v", i+1, err)
		}
	}
	a := use(t, r, alice)
	checkBusyErr(t, "Admit(B) at the cap", r.Admit(bob), 1)

	r.Release("no-such-account")
	checkBusyErr(t, "Admit(B) after releasing an unknown account", r.Admit(bob), 1)

	r.Release(alice.ID)
	select {
	case <-a.ctx.Done():
	default:
		t.Error("A's entry was not closed on Release")
	}
	if err := r.Admit(bob); err != nil {
		t.Fatalf("Admit(B) after A's last sign-in ended: %v", err)
	}
	checkBusyErr(t, "Admit(A) after B took the place", r.Admit(alice), 1)
}

// TestZeroPlacesIsOff: a cap of 0 never refuses anyone.
func TestZeroPlacesIsOff(t *testing.T) {
	r := placesRegistry(t, 0, 0, nil)
	for i := 0; i < 50; i++ {
		acct := hostauth.Account{ID: fmt.Sprintf("sub-%d", i), Email: fmt.Sprintf("%d@example.com", i)}
		if err := r.Admit(acct); err != nil {
			t.Fatalf("Admit for account %d with no cap: %v", i+1, err)
		}
	}
	if n := r.holding(); n != 50 {
		t.Errorf("%d places held, want 50", n)
	}
}
