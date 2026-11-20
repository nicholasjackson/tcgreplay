package game

import (
	"context"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards/cardstest"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// TestSetupWarnings checks that a Trainer played or a card attached before
// the game starts is flagged while the game is in setup, that placing Basic
// Pokémon is not, and that the warnings end once the game starts.
func TestSetupWarnings(t *testing.T) {
	m := NewManager(t.TempDir(), cardstest.FakeDB{})
	seed := uint64(1)
	m.Rand = func() *rand.Rand { seed++; return rand.New(rand.NewPCG(seed, 99)) }
	g, _, err := m.New(context.Background(), NewGameRequest{ID: "setup", DeckP1: pairDeck, DeckP2: pairDeck})
	if err != nil {
		t.Fatal(err)
	}
	find := func(seat state.Seat, def string) state.IID {
		st := g.State()
		for iid, c := range st.Cards {
			if c.Def == def && strings.HasPrefix(string(iid), string(seat)+"-") {
				if l, _ := st.Find(iid); l.Zone != state.Hand {
					must(t)(g.Move(seat, actorOf(seat), iid, state.Loc{Zone: state.Hand}, "", ""))
				}
				return iid
			}
		}
		t.Fatalf("no %s in %s's cards", def, seat)
		return ""
	}
	riolu1, riolu2 := find(state.P1, "tst-1"), find(state.P2, "tst-1")
	must(t)(g.PlaceActive(state.P1, state.ActorHuman, riolu1))
	must(t)(g.PlaceActive(state.P2, state.ActorClaude, riolu2))
	setupWarns := func() []string {
		var out []string
		for _, w := range g.Warnings() {
			if strings.Contains(w, "during setup") {
				out = append(out, w)
			}
		}
		return out
	}
	if w := setupWarns(); len(w) > 0 {
		t.Fatalf("placing Active Pokémon warned %v, want nothing", w)
	}

	res := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, find(state.P1, "tst-5")))
	want := "p1 played a Trainer during setup: that waits until the game starts (start_game)"
	if !slices.Contains(res.Warnings, want) {
		t.Errorf("play_trainer during setup warned %v, want %q", res.Warnings, want)
	}
	g.EndAction()
	must(t)(g.Attach(state.P2, state.ActorClaude, find(state.P2, "sve-006"), state.Loc{Seat: state.P2, Zone: state.Active}, "energy"))
	if got, want := setupWarns(), []string{
		"p1 played a Trainer during setup: that waits until the game starts (start_game)",
		"p2 attached a card during setup: that waits until the game starts (start_game)",
	}; !slices.Equal(got, want) {
		t.Errorf("setup warnings %v, want %v", got, want)
	}

	must(t)(g.StartGame(state.P1, state.ActorHuman))
	if w := setupWarns(); len(w) > 0 {
		t.Errorf("after start_game the setup warnings %v remain, want none", w)
	}
}
