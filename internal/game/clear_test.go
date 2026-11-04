package game

import (
	"context"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// clearedGame opens a game through the real card database (a tiered store
// over a fresh cache directory and a stand-in Spaces) against src, and
// returns it with the client, the stand-in and every card definition the
// game uses.
func clearedGame(t *testing.T, src *liveSource) (*Game, *cards.Client, *spacestest.Server, []string) {
	t.Helper()
	srv := spacestest.New(t)
	dir := t.TempDir()
	client := cards.NewClient(dir)
	client.Base = src.URL
	client.Timeout = 300 * time.Millisecond
	client.Store = cards.NewTieredStore(cards.NewDiskStore(dir), cards.NewSpacesStore(srv.Client(t)))
	m := NewManager(t.TempDir(), client)
	m.Rand = func() *rand.Rand { return rand.New(rand.NewPCG(5, 99)) }
	g, _, err := m.New(context.Background(), NewGameRequest{ID: "clear", DeckP1: testDeck, DeckP2: testDeck})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var defs []string
	for _, c := range g.State().Cards {
		if c.Def == "" {
			t.Fatalf("%s unresolved", c.Name)
		}
		if !seen[c.Def] {
			seen[c.Def] = true
			defs = append(defs, c.Def)
		}
	}
	return g, client, srv, defs
}

// snapshot is what the board shows of each definition.
func defsShown(t *testing.T, client *cards.Client, defs []string) map[string]cards.Card {
	t.Helper()
	out := map[string]cards.Card{}
	for _, d := range defs {
		c := client.Cached(d)
		if c == nil {
			t.Fatalf("%s not cached", d)
		}
		out[d] = *c
	}
	return out
}

// playOn reads the board for both seats (each read within a second) and
// makes a play: P1 puts a Basic from their hand into the Active Spot.
func playOn(t *testing.T, g *Game) {
	t.Helper()
	for _, seat := range []state.Seat{state.P1, state.P2} {
		quick(t, "View "+string(seat), func() {
			if _, err := g.View(seat, "test", "test"); err != nil {
				t.Error(err)
			}
		})
	}
	var basic state.IID
	for _, iid := range g.State().Sides[state.P1].Hand {
		if g.State().Cards[iid].Def == "tst-1" {
			basic = iid
			break
		}
	}
	if basic == "" {
		t.Fatal("no Riolu in P1's hand")
	}
	quick(t, "PlaceActive", func() {
		if _, err := g.PlaceActive(state.P1, state.ActorHuman, basic); err != nil {
			t.Fatalf("play after the clear: %v", err)
		}
	})
	if a := g.State().Sides[state.P1].Active; a == nil || len(a.Cards()) == 0 || a.Cards()[0] != basic {
		t.Errorf("Active = %+v, want %s", a, basic)
	}
}

// TestOpenGameSurvivesAClear: with a game open, clearing the whole card
// cache leaves every card's definition on the board, board reads quick and
// play working; the cleared cards are then fetched again from the card
// source in the background, bringing a correction made at the source.
func TestOpenGameSurvivesAClear(t *testing.T) {
	src := newLiveSource(t)
	g, client, srv, defs := clearedGame(t, src)
	before := defsShown(t, client, defs)

	// A correction at the card source, picked up only once cleared.
	src.mu.Lock()
	src.bodies["/cards/tst-4"] = []byte(strings.Replace(string(src.bodies["/cards/tst-4"]), "Each player shuffles their hand", "CORRECTED Each player shuffles their hand", 1))
	src.mu.Unlock()
	src.reset()

	ctx := context.Background()
	client.Clear(ctx, cards.Scope{})
	if keys := srv.Keys("tcgdex/"); len(keys) != 0 {
		t.Fatalf("shared store after the clear: %v", keys)
	}

	quick(t, "board reads after the clear", func() {
		after := defsShown(t, client, defs)
		for d, c := range before {
			a := after[d]
			if a.Name != c.Name || a.HP != c.HP || a.Text() != c.Text() || a.Image != c.Image {
				t.Errorf("%s changed on the board straight after the clear: %+v, was %+v", d, a, c)
			}
		}
	})
	playOn(t, g)

	// The board's reads warmed every cleared card: each is fetched again.
	waitFor(t, 5*time.Second, "cards fetched again", func() bool {
		for _, d := range defs {
			if src.count("/cards/"+d) == 0 {
				return false
			}
		}
		return strings.Contains(client.Cached("tst-4").Text(), "CORRECTED")
	})
	for _, d := range defs {
		if n := src.count("/cards/" + d); n != 1 {
			t.Errorf("%s fetched %d times after the clear, want 1", d, n)
		}
	}
	if c, err := client.Card(ctx, "tst-4"); err != nil || !strings.Contains(c.Text(), "CORRECTED") {
		t.Errorf("Card(tst-4) after the warm = %v, %v; want the corrected text", c, err)
	}
	quick(t, "View after the warm", func() { g.View(state.P1, "test", "test") })
}

// TestOpenGameSurvivesAClearWithTheSourceDown: with the card source
// unreachable during and after the clear, the game still shows and plays
// its cards, and lookups return the old copies with no error.
func TestOpenGameSurvivesAClearWithTheSourceDown(t *testing.T) {
	src := newLiveSource(t)
	g, client, _, defs := clearedGame(t, src)
	before := defsShown(t, client, defs)
	src.Close()

	ctx := context.Background()
	client.Clear(ctx, cards.Scope{})
	quick(t, "board reads after the clear", func() {
		after := defsShown(t, client, defs)
		for d, c := range before {
			if a := after[d]; a.Text() != c.Text() || a.Image != c.Image {
				t.Errorf("%s changed: %+v, was %+v", d, a, c)
			}
		}
	})
	playOn(t, g)
	for _, d := range defs {
		c, err := client.Card(ctx, d)
		old := before[d]
		if err != nil || c == nil || c.Text() != old.Text() {
			t.Errorf("Card(%s) with the source down = %v, %v; want the old copy", d, c, err)
		}
	}
	quick(t, "View after lookups", func() { g.View(state.P2, "test", "test") })
}

// waitFor polls cond until it holds or d passes.
func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// count is how many requests path has had.
func (s *liveSource) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}
