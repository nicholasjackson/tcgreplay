package webtest

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// checkPosition reports every way g differs from the fixed position.
func checkPosition(t *testing.T, g *game.Game) {
	t.Helper()
	st := g.State()
	if g.Mode() != game.ModeReview {
		t.Errorf("mode = %s, want review", g.Mode())
	}
	if st.Turn != 1 || st.Current != state.P1 || !st.TurnOpen {
		t.Errorf("turn = %d %s (open %v), want p1's turn 1, open", st.Turn, st.Current, st.TurnOpen)
	}
	if st.Stadium != "p1-11" {
		t.Errorf("stadium = %q, want p1-11 (Arena)", st.Stadium)
	}
	want := map[state.Seat]struct {
		name    string
		active  []state.IID
		energy  []state.IID
		bench   [][]state.IID
		hand    []state.IID
		discard []state.IID
	}{
		state.P1: {
			name:    "Alice",
			active:  []state.IID{"p1-5"},
			energy:  []state.IID{"p1-15"},
			bench:   [][]state.IID{{"p1-1"}, {"p1-6"}, nil, nil, nil},
			hand:    []state.IID{"p1-2", "p1-3", "p1-17", "p1-18", "p1-9", "p1-7", "p1-12", "p1-13"},
			discard: []state.IID{"p1-16"},
		},
		state.P2: {
			name:   "Bob",
			active: []state.IID{"p2-5"},
			bench:  [][]state.IID{{"p2-1"}, nil, nil, nil, nil},
			hand:   []state.IID{"p2-2", "p2-7", "p2-15", "p2-16", "p2-17", "p2-18"},
		},
	}
	for seat, w := range want {
		side := st.Sides[seat]
		if side.Name != w.name {
			t.Errorf("%s name = %q, want %q", seat, side.Name, w.name)
		}
		if !slices.Equal(side.Active.Pokemon, w.active) || !slices.Equal(side.Active.Energy, w.energy) {
			t.Errorf("%s Active = %v with Energy %v, want %v with %v", seat, side.Active.Pokemon, side.Active.Energy, w.active, w.energy)
		}
		var bench [][]state.IID
		for _, b := range side.Bench {
			var p []state.IID
			if b != nil {
				p = b.Pokemon
			}
			bench = append(bench, p)
		}
		if !slices.EqualFunc(bench, w.bench, slices.Equal) {
			t.Errorf("%s bench = %v, want %v", seat, bench, w.bench)
		}
		if got := slices.Sorted(slices.Values(side.Hand)); !slices.Equal(got, slices.Sorted(slices.Values(w.hand))) {
			t.Errorf("%s hand = %v, want %v", seat, side.Hand, w.hand)
		}
		if !slices.Equal(side.Discard, w.discard) {
			t.Errorf("%s discard = %v, want %v", seat, side.Discard, w.discard)
		}
		if n := side.PrizesLeft(); n != 6 {
			t.Errorf("%s has %d prizes, want 6", seat, n)
		}
	}
	// Everything not placed is in the deck or the prizes: 60 cards a side.
	for _, seat := range state.Seats {
		side := st.Sides[seat]
		n := len(side.Deck) + side.PrizesLeft() + len(side.Hand) + len(side.Discard) + len(side.Lost)
		for _, s := range append([]*state.Stack{side.Active}, side.Bench...) {
			if s != nil {
				n += len(s.Pokemon) + len(s.Energy) + len(s.Tools)
			}
		}
		if seat == state.P1 {
			n++ // the Stadium
		}
		if n != 60 {
			t.Errorf("%s has %d cards in all, want 60", seat, n)
		}
	}
}

func TestResetBuildsThePosition(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := Reset(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	g, err := m.Current()
	if err != nil {
		t.Fatal(err)
	}
	if g.ID != GameID {
		t.Errorf("game = %q, want %q", g.ID, GameID)
	}
	checkPosition(t, g)
}

func TestResetRestoresThePosition(t *testing.T) {
	m := NewManager(t.TempDir())
	ctx := context.Background()
	if err := Reset(ctx, m); err != nil {
		t.Fatal(err)
	}
	g, _ := m.Current()
	// A test moves cards about: a hand Riolu to the bench and the Active's
	// Energy to the discard.
	if _, err := g.Bench(state.P1, state.ActorHuman, "p1-2", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Move(state.P1, state.ActorHuman, "p1-15", state.Loc{Seat: state.P1, Zone: state.Discard}, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := Reset(ctx, m); err != nil {
		t.Fatal(err)
	}
	g2, err := m.Current()
	if err != nil {
		t.Fatal(err)
	}
	if g2 == g {
		t.Error("reset kept the old game, want a new one")
	}
	checkPosition(t, g2)
}

// TestNotInShippedBinary: the shipped binary includes neither the board
// test server nor the fake card pool.
func TestNotInShippedBinary(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/nicholasjackson/tcgreplay/cmd/tcgreplay").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.Contains(pkg, "/webtest") || strings.HasSuffix(pkg, "/cardstest") {
			t.Errorf("the shipped binary depends on %s", pkg)
		}
	}
}
