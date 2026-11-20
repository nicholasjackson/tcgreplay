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

// benchedGame is a test game in review mode, started with p1 going first:
// each player has an Active Pokémon and p2 a Benched one.
func benchedGame(t *testing.T) *Game {
	t.Helper()
	_, g := newTestGame(t)
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	basic := func(seat state.Seat) state.IID {
		st := g.State()
		for _, iid := range st.Sides[seat].Hand {
			if c := g.card(st, iid); c != nil && c.IsBasic() {
				return iid
			}
		}
		for _, iid := range st.Sides[seat].Deck {
			if c := g.card(st, iid); c != nil && c.IsBasic() {
				must(t)(g.Move(seat, state.ActorSystem, iid, state.Loc{Seat: seat, Zone: state.Hand}, "", ""))
				return iid
			}
		}
		t.Fatalf("%s has no Basic Pokémon", seat)
		return ""
	}
	for _, seat := range state.Seats {
		must(t)(g.PlaceActive(seat, state.ActorSystem, basic(seat)))
	}
	must(t)(g.Bench(state.P2, state.ActorSystem, basic(state.P2), -1))
	must(t)(g.StartGame(state.P1, state.ActorHuman))
	return g
}

func handCard(t *testing.T, g *Game, seat state.Seat, name string) state.IID {
	t.Helper()
	st := g.State()
	for _, iid := range st.Sides[seat].Hand {
		if st.Name(iid) == name {
			return iid
		}
	}
	for _, iid := range st.Sides[seat].Deck {
		if st.Name(iid) == name {
			must(t)(g.Move(seat, state.ActorSystem, iid, state.Loc{Seat: seat, Zone: state.Hand}, "", ""))
			return iid
		}
	}
	t.Fatalf("no %s for %s", name, seat)
	return ""
}

func hasWarning(ws []string, part string) bool {
	for _, w := range ws {
		if strings.Contains(w, part) {
			return true
		}
	}
	return false
}

func TestCostNeed(t *testing.T) {
	for text, want := range map[string]int{
		"You can use this card only if you discard 2 other cards from your hand. Search your deck": 2,
		"you may discard a Basic {F} Energy card from your hand in order to use this Ability.":     1,
		"Discard your hand and draw 7 cards.":                                                      0,
		"Draw 3 cards.":                                                                            0,
	} {
		if got := costNeed(text); got != want {
			t.Errorf("%q: cost %d, want %d", text, got, want)
		}
	}
}

// A card played while Ultra Ball's cost is unpaid is a play, not a
// discard: closing Ultra Ball warns. Paying the cost does not.
func TestUnpaidCostWarns(t *testing.T) {
	g := benchedGame(t)
	ball := handCard(t, g, state.P1, "Ultra Ball")
	must(t)(g.PlayTrainer(state.P1, state.ActorHuman, ball))
	if snap, err := g.UISnapshot(state.P1, nil); err != nil || snap.OpenCost == nil || snap.OpenCost.Need != 2 || snap.OpenCost.Done != 0 {
		t.Fatalf("snapshot open cost %+v (%v), want 0 of 2", snap.OpenCost, err)
	}
	iono := handCard(t, g, state.P1, "Iono")
	res := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, iono))
	if !hasWarning(res.Warnings, "Ultra Ball asks for 2 cards discarded") {
		t.Fatalf("playing Iono over an unpaid Ultra Ball warned %v", res.Warnings)
	}
	g.EndAction()

	ball2 := handCard(t, g, state.P1, "Ultra Ball")
	must(t)(g.PlayTrainer(state.P1, state.ActorHuman, ball2))
	for _, iid := range g.State().Sides[state.P1].Hand[:2] {
		must(t)(g.Move(state.P1, state.ActorHuman, iid, state.Loc{Seat: state.P1, Zone: state.Discard}, "", ""))
	}
	if _, warn := g.EndActionChecked(); len(warn) > 0 {
		t.Fatalf("a paid Ultra Ball warned %v", warn)
	}
}

// The turn cannot end while a player must promote or is owed prizes; taking
// the last prize ends the game.
func TestEndTurnChecksAndGameOver(t *testing.T) {
	g := benchedGame(t)
	must(t)(g.KnockOut(state.P1, state.ActorHuman, state.Loc{Seat: state.P2, Zone: state.Active}))
	if _, err := g.EndTurn(state.P1, state.ActorHuman); err == nil || !strings.Contains(err.Error(), "promote") {
		t.Fatalf("end turn with p2's Active empty: %v, want a refusal", err)
	}
	must(t)(g.Switch(state.P2, state.ActorHuman, 0))
	if _, err := g.EndTurn(state.P1, state.ActorHuman); err == nil || !strings.Contains(err.Error(), "owed 1 prize card") {
		t.Fatalf("end turn with a prize owed: %v, want a refusal", err)
	}
	if _, err := g.EndTurnWith(state.P1, state.ActorHuman, EndTurnOptions{IgnorePrizes: true}); err != nil {
		t.Fatalf("end turn ignoring prizes: %v", err)
	}

	// Taking every prize ends the game.
	var last *Result
	for range g.State().Sides[state.P2].PrizesLeft() {
		last = must(t)(g.TakePrize(state.P2, state.ActorHuman, -1))
	}
	if !hasWarning(last.Warnings, "Game over: p2 wins (took all their prize cards)") {
		t.Fatalf("last prize warned %v", last.Warnings)
	}
	if w, _ := g.State().Winner(); w != state.P2 {
		t.Fatalf("winner %q, want p2", w)
	}
}
