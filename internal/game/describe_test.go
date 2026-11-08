package game

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// A Stadium pair's move is one event, described in one line naming both
// halves, and both halves are highlighted.
func TestDescribeStadiumPairMove(t *testing.T) {
	var cards []state.Card
	for _, seat := range state.Seats {
		for i := 1; i <= state.DeckSize; i++ {
			cards = append(cards, state.Card{IID: state.IID(fmt.Sprintf("%s-%d", seat, i)), Def: "tst-10", Name: "Twin Peak", Owner: seat})
		}
	}
	st := state.New()
	id := 0
	ev := func(typ state.EventType, payload any) *state.Event {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		id++
		return &state.Event{ID: id, Parent: id - 1, Seat: state.P1, Type: typ, Payload: b}
	}
	step := func(e *state.Event) (before, after *state.State) {
		before = st.Clone()
		if err := st.Apply(e); err != nil {
			t.Fatal(err)
		}
		return before, st.Clone()
	}
	step(ev(state.EvSetup, state.SetupPayload{BenchSize: 5, PrizeCount: 6, Cards: cards}))
	step(ev(state.EvMove, state.MovePayload{IID: "p1-11", To: state.Loc{Zone: state.Hand}}))
	step(ev(state.EvMove, state.MovePayload{IID: "p1-12", To: state.Loc{Zone: state.Hand}}))

	pair := ev(state.EvMove, state.MovePayload{IID: "p1-11", Pair: "p1-12", From: &state.Loc{Seat: state.P1, Zone: state.Hand}, To: state.Loc{Zone: state.Stadium}})
	before, after := step(pair)
	if got, want := describe(pair, before, after, ""), "p1 moved p1-11 Twin Peak + p1-12 Twin Peak p1:hand → stadium"; got != want {
		t.Errorf("describe = %q, want %q", got, want)
	}
	if got := touched(pair, before); !slices.Equal(got, []state.IID{"p1-11", "p1-12"}) {
		t.Errorf("touched = %v, want both halves", got)
	}

	out := ev(state.EvMove, state.MovePayload{IID: "p1-11", Pair: "p1-12", From: &state.Loc{Zone: state.Stadium}, To: state.Loc{Seat: state.P1, Zone: state.Discard}})
	before, after = step(out)
	if got, want := describe(out, before, after, state.P2), "p1 moved p1-11 Twin Peak + p1-12 Twin Peak stadium → p1:discard"; got != want {
		t.Errorf("describe = %q, want %q", got, want)
	}

	// A single card joining a lone half reads "as pair".
	step(ev(state.EvMove, state.MovePayload{IID: "p1-11", To: state.Loc{Zone: state.Stadium}}))
	join := ev(state.EvMove, state.MovePayload{IID: "p1-12", From: &state.Loc{Seat: state.P1, Zone: state.Discard}, To: state.Loc{Zone: state.Stadium}, As: state.AsPair})
	before, after = step(join)
	if got, want := describe(join, before, after, ""), "p1 moved p1-12 Twin Peak p1:discard → stadium as pair"; got != want {
		t.Errorf("describe = %q, want %q", got, want)
	}
}
