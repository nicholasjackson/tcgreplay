package game

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cards/cardstest"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// slowDB is the test card pool with one def that, once switched on, is
// never cached and whose fetch hangs, fails or succeeds after a delay.
type slowDB struct {
	cardstest.FakeDB
	def   string
	on    atomic.Bool
	mode  string        // "hang", "fail" or "delay"
	gate  chan struct{} // closed when the test ends, releasing hung fetches
	calls atomic.Int32
	mu    sync.Mutex
	done  bool // a delayed fetch has finished: the def is cached from now on
}

func (d *slowDB) Cached(id string) *cards.Card {
	if d.on.Load() && id == d.def {
		d.mu.Lock()
		done := d.done
		d.mu.Unlock()
		if !done {
			return nil
		}
	}
	return d.FakeDB.Cached(id)
}

func (d *slowDB) Card(ctx context.Context, id string) (*cards.Card, error) {
	if !d.on.Load() || id != d.def {
		return d.FakeDB.Card(ctx, id)
	}
	d.calls.Add(1)
	switch d.mode {
	case "fail":
		return nil, errors.New("card source down")
	case "delay":
		time.Sleep(50 * time.Millisecond)
		d.mu.Lock()
		d.done = true
		d.mu.Unlock()
		return d.FakeDB.Card(ctx, id)
	}
	select {
	case <-d.gate:
	case <-ctx.Done():
	}
	return nil, errors.New("hung")
}

// newSlowGame builds a review-mode game on a slowDB, then switches the
// slow def on, so building the game itself is unaffected.
func newSlowGame(t *testing.T, mode string) (*Game, *slowDB) {
	t.Helper()
	db := &slowDB{FakeDB: cardstest.New(), def: "tst-3", mode: mode, gate: make(chan struct{})}
	t.Cleanup(func() { close(db.gate) })
	m := NewManager(t.TempDir(), db)
	m.Rand = func() *rand.Rand { return rand.New(rand.NewPCG(7, 99)) }
	g, _, err := m.New(context.Background(), NewGameRequest{ID: "slow", DeckP1: testDeck, DeckP2: testDeck})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	db.on.Store(true)
	return g, db
}

// quick fails the test if f takes a second or more.
func quick(t *testing.T, what string, f func()) {
	t.Helper()
	start := time.Now()
	f()
	if d := time.Since(start); d >= time.Second {
		t.Errorf("%s took %v", what, d)
	}
}

// iidOf finds a card instance of def owned by seat.
func iidOf(t *testing.T, g *Game, seat state.Seat, def string) state.IID {
	t.Helper()
	for iid, c := range g.State().Cards {
		if c.Owner == seat && c.Def == def {
			return iid
		}
	}
	t.Fatalf("no %s card for %s", def, seat)
	return ""
}

// TestBoardReadsDoNotWaitOnHungFetch: with a card whose fetch hangs, board
// reads and a play that looks the card up each return within one second,
// and only one fetch of it is started however often it is looked up.
func TestBoardReadsDoNotWaitOnHungFetch(t *testing.T) {
	g, db := newSlowGame(t, "hang")
	for _, seat := range []state.Seat{state.P1, state.P2} {
		quick(t, "View "+string(seat), func() {
			if _, err := g.View(seat, "test", "test"); err != nil {
				t.Error(err)
			}
		})
		quick(t, "SearchDeck "+string(seat), func() {
			if _, err := g.SearchDeck(seat, "", "test", "test"); err != nil {
				t.Error(err)
			}
		})
	}
	quick(t, "Warnings", func() { g.Warnings() })

	// A play that infers how a card joins a stack from its definition.
	riolu := iidOf(t, g, state.P1, "tst-1")
	must(t)(g.Move(state.P1, state.ActorHuman, riolu, state.Loc{Seat: state.P1, Zone: state.Active}, "", ""))
	ball := iidOf(t, g, state.P1, "tst-3")
	quick(t, "Move onto a stack", func() {
		g.Move(state.P1, state.ActorHuman, ball, state.Loc{Seat: state.P1, Zone: state.Active}, "", "")
	})

	for range 20 {
		quick(t, "repeated View", func() { g.View(state.P1, "test", "test") })
		quick(t, "repeated SearchDeck", func() { g.SearchDeck(state.P1, "", "test", "test") })
	}
	if n := db.calls.Load(); n != 1 {
		t.Errorf("%d fetches of the hung card started, want 1 at a time", n)
	}
}

// TestBoardReadsWithFailingCard: repeated board reads of a game holding a
// card whose lookup fails each return within one second.
func TestBoardReadsWithFailingCard(t *testing.T) {
	g, db := newSlowGame(t, "fail")
	for range 20 {
		quick(t, "View", func() { g.View(state.P1, "test", "test") })
		quick(t, "SearchDeck", func() { g.SearchDeck(state.P1, "", "test", "test") })
	}
	if db.calls.Load() == 0 {
		t.Error("the missing card was never fetched in the background")
	}
}

// TestMissingCardWarmedInBackground: a card missing from the cache when it
// is first needed is fetched in the background, and later reads see it.
func TestMissingCardWarmedInBackground(t *testing.T) {
	g, db := newSlowGame(t, "delay")
	kindOf := func() string {
		cards, err := g.SearchDeck(state.P1, "", "test", "test")
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cards {
			if c.Def == "tst-3" {
				return c.Category
			}
		}
		return "absent"
	}
	if got := kindOf(); got == "absent" {
		t.Skip("no Ultra Ball in P1's deck with this seed")
	} else if got != "" {
		t.Fatalf("uncached card already known as %q", got)
	}
	deadline := time.Now().Add(5 * time.Second)
	for kindOf() != "Trainer" {
		if time.Now().After(deadline) {
			t.Fatal("background fetch never made the card visible")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := db.calls.Load(); n != 1 {
		t.Errorf("%d fetches, want 1", n)
	}
}
