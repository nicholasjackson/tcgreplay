package usergames

import (
	"bytes"
	"context"
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
	if err := r.Admit(context.Background(), alice); err != nil {
		t.Fatalf("Admit(A) with a free place: %v", err)
	}
	a := use(t, r, alice)
	startGame(t, a.M, "game-a")

	checkBusyErr(t, "Admit(B) at the cap", r.Admit(context.Background(), bob), 1)
	clock.Advance(2 * time.Minute)
	checkBusyErr(t, "Admit(B) with A idle exactly 2 minutes", r.Admit(context.Background(), bob), 1)

	clock.Advance(time.Second)
	if err := r.Admit(context.Background(), bob); err != nil {
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
	if err := r.Admit(context.Background(), bob); err != nil {
		t.Errorf("Admit(B) after the sweep: %v", err)
	}
}

// TestNeverActedGivesUpPlace: an account that signs in (Admit) and never
// acts holds its place for the idle time and gives it up after it.
func TestNeverActedGivesUpPlace(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := placesRegistry(t, 1, 2*time.Minute, clock.Now)
	if err := r.Admit(context.Background(), alice); err != nil {
		t.Fatal(err)
	}
	clock.Advance(2 * time.Minute)
	checkBusyErr(t, "Admit(B) within A's idle time", r.Admit(context.Background(), bob), 1)
	clock.Advance(time.Second)
	if err := r.Admit(context.Background(), bob); err != nil {
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
	if _, err := r.Use(context.Background(), bob); err == nil {
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
	checkBusyErr(t, "Use(B) with an entry but no place", func() error { _, err := r.Use(context.Background(), bob); return err }(), 1)
	if got := r.LastActive(b); !got.Equal(before) {
		t.Errorf("refused Use moved B's last activity from %v to %v", before, got)
	}
}

// TestAdmitRecordsRefusal: a refused sign-in is recorded by the
// account's player ID, and its email is never logged.
func TestAdmitRecordsRefusal(t *testing.T) {
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })
	r := placesRegistry(t, 1, 0, nil)
	rec, events := withRecorder(t, r)
	if err := r.Admit(context.Background(), alice); err != nil {
		t.Fatal(err)
	}
	r.Admit(context.Background(), bob)
	if want := `"event":"limited","player":"` + rec.ID(bob.Email) + `"`; !strings.Contains(events.String(), want) {
		t.Errorf("records %q lack %s", events, want)
	}
	if strings.Contains(logs.String()+events.String(), bob.Email) {
		t.Errorf("bob's email was logged or recorded: %q %q", logs.String(), events)
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
	if err := r.Admit(context.Background(), bob); err != nil {
		t.Fatalf("Admit(B) after A's game expired: %v", err)
	}

	_, err := r.Use(context.Background(), alice)
	checkBusyErr(t, "A's next action with B holding the place", err, 1)
	if r.holdsPlace(alice) {
		t.Error("A took a place over the cap")
	}

	r.Release(context.Background(), bob.ID)
	again, err := r.Use(context.Background(), alice)
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
		if err := r.Admit(context.Background(), alice); err != nil {
			t.Fatalf("Admit(A) #%d holding the place: %v", i+1, err)
		}
	}
	a := use(t, r, alice)
	checkBusyErr(t, "Admit(B) at the cap", r.Admit(context.Background(), bob), 1)

	r.Release(context.Background(), "no-such-account")
	checkBusyErr(t, "Admit(B) after releasing an unknown account", r.Admit(context.Background(), bob), 1)

	r.Release(context.Background(), alice.ID)
	select {
	case <-a.ctx.Done():
	default:
		t.Error("A's entry was not closed on Release")
	}
	if err := r.Admit(context.Background(), bob); err != nil {
		t.Fatalf("Admit(B) after A's last sign-in ended: %v", err)
	}
	checkBusyErr(t, "Admit(A) after B took the place", r.Admit(context.Background(), alice), 1)
}

// TestZeroPlacesIsOff: a cap of 0 never refuses anyone.
func TestZeroPlacesIsOff(t *testing.T) {
	r := placesRegistry(t, 0, 0, nil)
	for i := 0; i < 50; i++ {
		acct := hostauth.Account{ID: fmt.Sprintf("sub-%d", i), Email: fmt.Sprintf("%d@example.com", i)}
		if err := r.Admit(context.Background(), acct); err != nil {
			t.Fatalf("Admit for account %d with no cap: %v", i+1, err)
		}
	}
	if n := r.holding(); n != 50 {
		t.Errorf("%d places held, want 50", n)
	}
}

// reviewer is the configured reviewer account, as Google verified it;
// carol is a third ordinary account.
var (
	reviewer = hostauth.Account{ID: "google-sub-reviewer", Email: "Reviewer@Example.com", EmailVerified: true}
	carol    = hostauth.Account{ID: "google-sub-carol", Email: "carol@example.com"}
)

// reviewerRegistry is placesRegistry with reviewer@example.com as the
// reviewer.
func reviewerRegistry(t *testing.T, max int) *Registry {
	t.Helper()
	r := placesRegistry(t, max, 0, nil)
	r.cfg.Reviewer = "reviewer@example.com"
	return r
}

// TestReviewerPastTheCap: at a cap of 1 held by A, the reviewer (email
// verified, matched without regard to case) is admitted and can act,
// while C is still refused with the usual busy error.
func TestReviewerPastTheCap(t *testing.T) {
	r := reviewerRegistry(t, 1)
	if err := r.Admit(context.Background(), alice); err != nil {
		t.Fatalf("Admit(A) with a free place: %v", err)
	}
	if err := r.Admit(context.Background(), reviewer); err != nil {
		t.Fatalf("Admit(reviewer) at the cap: %v", err)
	}
	e := use(t, r, reviewer)
	startGame(t, e.M, "review-game")
	checkBusyErr(t, "Admit(C) with the reviewer in", r.Admit(context.Background(), carol), 1)
	if _, err := r.Use(context.Background(), carol); err == nil {
		t.Error("Use(C) at the cap with the reviewer in was not refused")
	}
}

// TestUnverifiedReviewerEmailIsRefused: an account whose email matches the
// reviewer's but which Google has not verified is treated like any other
// account at the cap.
func TestUnverifiedReviewerEmailIsRefused(t *testing.T) {
	r := reviewerRegistry(t, 1)
	use(t, r, alice)
	impostor := hostauth.Account{ID: "google-sub-impostor", Email: "reviewer@example.com"}
	checkBusyErr(t, "Admit(unverified reviewer email) at the cap", r.Admit(context.Background(), impostor), 1)
	// A verified account with a different email is not the reviewer either.
	other := hostauth.Account{ID: "google-sub-other", Email: "reviewer@example.org", EmailVerified: true}
	checkBusyErr(t, "Admit(another verified email) at the cap", r.Admit(context.Background(), other), 1)
}

// TestReviewerPlaceNotCounted: the reviewer's place does not take one of
// the cap's: with the reviewer in, A leaving lets C in, and with the
// reviewer alone the one place is still free for A.
func TestReviewerPlaceNotCounted(t *testing.T) {
	r := reviewerRegistry(t, 1)
	use(t, r, reviewer)
	if err := r.Admit(context.Background(), alice); err != nil {
		t.Fatalf("Admit(A) with only the reviewer in: %v", err)
	}
	checkBusyErr(t, "Admit(C) with A and the reviewer in", r.Admit(context.Background(), carol), 1)
	r.Release(context.Background(), alice.ID)
	if err := r.Admit(context.Background(), carol); err != nil {
		t.Fatalf("Admit(C) after A left, the reviewer still in: %v", err)
	}
	if !r.holdsPlace(reviewer) || !r.holdsPlace(carol) {
		t.Errorf("reviewer holds a place: %v, C: %v; want both", r.holdsPlace(reviewer), r.holdsPlace(carol))
	}
}

// TestNoReviewerConfigured: with no reviewer set, the verified reviewer
// account is refused at the cap like any other.
func TestNoReviewerConfigured(t *testing.T) {
	r := placesRegistry(t, 1, 0, nil)
	use(t, r, alice)
	checkBusyErr(t, "Admit(reviewer account) with no reviewer configured", r.Admit(context.Background(), reviewer), 1)
}

// TestReviewerSettingNormalised: the reviewer setting is matched trimmed
// and without regard to case.
func TestReviewerSettingNormalised(t *testing.T) {
	r := New(Config{Reviewer: "  Reviewer@EXAMPLE.com \n"})
	if !r.isReviewer(reviewer) {
		t.Errorf("reviewer %q not matched by setting %q", reviewer.Email, r.cfg.Reviewer)
	}
	if r.isReviewer(alice) {
		t.Error("alice matched as the reviewer")
	}
}
