package game

import (
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// A card revealed from a hand stays face down there for the other player,
// marked with its name, and the mark lapses once it leaves the hand.
func TestRevealedHandCardStaysFaceDown(t *testing.T) {
	_, g := newTestGame(t)
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	card := g.State().Sides[state.P2].Hand[0]
	name := g.State().Name(card)
	must(t)(g.Reveal(state.P2, state.ActorClaude, []state.IID{card}))

	find := func(v state.BoardView) *state.CardView {
		for i, c := range v.Sides[state.P2].Hand {
			if c.IID == card {
				return &v.Sides[state.P2].Hand[i]
			}
		}
		return nil
	}
	p1, err := g.View(state.P1, "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	if c := find(p1); c == nil || !c.Hidden || c.Known != name || c.Name != "" {
		t.Fatalf("p1 sees the revealed card as %+v, want face down and known as %q", c, name)
	}
	p2, _ := g.View(state.P2, "test", "test")
	if c := find(p2); c == nil || c.Hidden || c.Name != name {
		t.Fatalf("its owner sees %+v, want it face up", c)
	}
	all, _ := g.View("", "test", "test")
	if c := find(all); c == nil || c.Hidden {
		t.Fatalf("the reveal view shows %+v, want it face up", c)
	}

	must(t)(g.Move(state.P2, state.ActorClaude, card, state.Loc{Seat: state.P2, Zone: state.Deck}, "", ""))
	p1, _ = g.View(state.P1, "test", "test")
	if c := find(p1); c != nil {
		t.Fatalf("p1 still sees %+v after it went back to the deck", c)
	}
}

func TestPrizeValue(t *testing.T) {
	for name, want := range map[string]int{
		"Riolu": 1, "Meowth ex": 2, "Mega Lucario ex": 3, "Mega Absol ex": 3,
		"Lugia V": 2, "Arceus VSTAR": 2, "Eternatus VMAX": 3, "Lillie's Clefairy ex": 2,
	} {
		if got := prizeValue(name); got != want {
			t.Errorf("%s: %d prizes, want %d", name, got, want)
		}
	}
}

// Stepping back moves the current branch's head; it makes no branch.
func TestStepBackToKeepsTheBranch(t *testing.T) {
	_, g := newTestGame(t)
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	mark := g.Head()
	hand := g.State().Sides[state.P1].Hand
	must(t)(g.Move(state.P1, state.ActorHuman, hand[0], state.Loc{Seat: state.P1, Zone: state.Discard}, "", ""))
	must(t)(g.Move(state.P1, state.ActorHuman, hand[1], state.Loc{Seat: state.P1, Zone: state.Discard}, "", ""))
	branch, before := g.Branch(), len(g.log.Branches())
	if err := g.StepBackTo(mark); err != nil {
		t.Fatal(err)
	}
	if g.Head() != mark || g.Branch() != branch || len(g.log.Branches()) != before {
		t.Fatalf("after stepping back: head %d on %s with %d branches, want %d on %s with %d", g.Head(), g.Branch(), len(g.log.Branches()), mark, branch, before)
	}
	if n := len(g.State().Sides[state.P1].Discard); n != 0 {
		t.Fatalf("discard has %d cards after stepping back over both moves", n)
	}
	if err := g.StepBackTo(g.Head()); err == nil {
		t.Fatal("stepping back to the head itself should be refused")
	}
}

// Setup mulligans a hand with no Basic Pokémon until it has one, and start
// refuses while a player has no Active Pokémon.
func TestSetupMulligansAndStartNeedsActives(t *testing.T) {
	_, g := newTestGame(t)
	st := g.State()
	for _, seat := range state.Seats {
		basic := false
		for _, iid := range st.Sides[seat].Hand {
			if c := g.card(st, iid); c != nil && c.IsBasic() {
				basic = true
			}
		}
		if !basic {
			t.Fatalf("%s's opening hand has no Basic Pokémon after setup", seat)
		}
	}
	if _, err := g.StartGame(state.P1, state.ActorHuman); err == nil || !strings.Contains(err.Error(), "no Active Pokémon") {
		t.Fatalf("start with no Actives: %v, want a refusal", err)
	}
	if n := g.State().Sides[state.P1].PrizesLeft(); n != 0 {
		t.Fatalf("prizes dealt (%d) by a refused start", n)
	}
}
