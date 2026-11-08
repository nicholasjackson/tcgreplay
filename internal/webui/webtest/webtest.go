// Package webtest builds the fixed game the board's browser tests start
// from. It is test support: only tests and the boardtest program import it,
// never the shipped binary.
package webtest

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"

	"github.com/nicholasjackson/tcgreplay/internal/cards/cardstest"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// GameID names the fixed game.
const GameID = "board-test"

// Names are the two players' names on the board.
var Names = map[state.Seat]string{state.P1: "Alice", state.P2: "Bob"}

// Deck is both players' list. Instances are numbered in list order, so
// p1-1 and p1-2 are Riolu, p1-3 and p1-4 Lucario, p1-5 and p1-6 Makuhita,
// p1-7 and p1-8 Iono, p1-9 and p1-10 Band, p1-11 Arena, p1-12 Deep Zone,
// p1-13 and p1-14 Ultra Ball, p1-15 to p1-58 Fighting Energy, and p1-59
// and p1-60 the two prints of Twin Peak (TST 10 and TST 11), a made-up
// Stadium played as a pair. The Twin Peaks come last so every other card
// keeps its number; they start in the deck (see TwinPeak).
const Deck = `Pokémon: 6
2 Riolu TST 1
2 Lucario TST 2
2 Makuhita TST 7

Trainer: 8
2 Iono TST 4
2 Band TST 6
1 Arena TST 5
1 Deep Zone TST 8
2 Ultra Ball TST 3

Energy: 44
44 Basic {F} Energy SVE 6

Trainer: 2
1 Twin Peak TST 10
1 Twin Peak TST 11

Total Cards: 60`

// TwinPeak is p1's two prints of Twin Peak, a pair-played Stadium. They
// start in the deck, so the fixed position is unchanged; pair tests move
// them where they need them.
var TwinPeak = [2]state.IID{"p1-59", "p1-60"}

// Layout is where each seat's cards start, by iid.
type Layout struct {
	Active  state.IID
	Bench   []state.IID // slots 0, 1, ...
	Energy  []state.IID // attached to the Active
	Hand    []state.IID
	Discard []state.IID
}

// Position is the fixed position: p1 (the board's player) has a Makuhita
// Active with one Energy, Riolu and Makuhita on the bench, eight cards in
// hand and an Energy in the discard; p2 has a Makuhita Active and a benched
// Riolu. Arena is in play, and it is p1's first turn, in review mode.
var Position = map[state.Seat]Layout{
	state.P1: {
		Active:  "p1-5",
		Bench:   []state.IID{"p1-1", "p1-6"},
		Energy:  []state.IID{"p1-15"},
		Hand:    []state.IID{"p1-2", "p1-3", "p1-17", "p1-18", "p1-9", "p1-7", "p1-12", "p1-13"},
		Discard: []state.IID{"p1-16"},
	},
	state.P2: {
		Active: "p2-5",
		Bench:  []state.IID{"p2-1"},
		Hand:   []state.IID{"p2-2", "p2-7", "p2-15", "p2-16", "p2-17", "p2-18"},
	},
}

// Stadium is the Stadium in play.
const Stadium state.IID = "p1-11"

// NewManager returns a game manager over the fake card pool whose games
// shuffle the same way every time.
func NewManager(dir string) *game.Manager {
	m := game.NewManager(dir, cardstest.New())
	m.Rand = func() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }
	return m
}

// Reset starts the fixed game afresh in m, replacing any current game.
func Reset(ctx context.Context, m *game.Manager) error {
	g, _, err := m.New(ctx, game.NewGameRequest{ID: GameID, DeckP1: Deck, DeckP2: Deck, Names: Names})
	if err != nil {
		return err
	}
	if err := g.SetMode(game.ModeReview); err != nil {
		return err
	}
	sys := state.ActorSystem
	for _, seat := range state.Seats {
		// Every card the layout places goes back to the deck first, so the
		// opening hand's shuffle does not matter.
		for _, iid := range slices.Clone(g.State().Sides[seat].Hand) {
			if _, err := g.Move(seat, sys, iid, state.Loc{Seat: seat, Zone: state.Deck}, "", ""); err != nil {
				return err
			}
		}
		l := Position[seat]
		for _, iid := range slices.Concat([]state.IID{l.Active}, l.Bench, l.Energy, l.Hand) {
			if _, err := g.Move(seat, sys, iid, state.Loc{Seat: seat, Zone: state.Hand}, "", ""); err != nil {
				return err
			}
		}
		if _, err := g.PlaceActive(seat, sys, l.Active); err != nil {
			return err
		}
		for i, iid := range l.Bench {
			if _, err := g.Bench(seat, sys, iid, i); err != nil {
				return err
			}
		}
		for _, iid := range l.Energy {
			if _, err := g.Attach(seat, sys, iid, state.Loc{Seat: seat, Zone: state.Active}, ""); err != nil {
				return err
			}
		}
		for _, iid := range l.Discard {
			if _, err := g.Move(seat, sys, iid, state.Loc{Seat: seat, Zone: state.Discard}, "", ""); err != nil {
				return err
			}
		}
	}
	if _, err := g.Move(state.P1, sys, Stadium, state.Loc{Zone: state.Stadium}, "", ""); err != nil {
		return err
	}
	// Starting the game deals prizes from the decks and draws p1 a card,
	// which goes back so the hand stays as laid out.
	if _, err := g.StartGame(state.P1, sys); err != nil {
		return err
	}
	for _, iid := range g.State().Sides[state.P1].Hand {
		if !slices.Contains(Position[state.P1].Hand, iid) {
			if _, err := g.Move(state.P1, sys, iid, state.Loc{Seat: state.P1, Zone: state.Deck}, "", ""); err != nil {
				return err
			}
			break
		}
	}
	if n := len(g.State().Sides[state.P1].Hand); n != len(Position[state.P1].Hand) {
		return fmt.Errorf("p1 hand has %d cards, want %d", n, len(Position[state.P1].Hand))
	}
	return nil
}
