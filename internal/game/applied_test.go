package game

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cards/cardstest"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// appliedCards are p1's family cards in hand after appliedGame.
type appliedCards struct {
	judge, stamp, red, iono state.IID
}

// rename gives p1's four family cards their defs and names. The swap is
// made on the live state only (as respondGame does), so anything that
// rebuilds the state from the log, such as an undo, reverts it: call
// rename again after one.
func (c appliedCards) rename(g *Game) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, x := range []struct {
		iid       state.IID
		def, name string
	}{
		{c.judge, "sv01-176", "Judge"},
		{c.stamp, "sv06-165", "Unfair Stamp"},
		{c.red, "me04-082", "Special Red Card"},
		{c.iono, "sv02-185", "Iono"},
	} {
		g.st.Cards[x.iid].Def, g.st.Cards[x.iid].Name = x.def, x.name
	}
}

// undo undoes the last action and renames p1's family cards again.
func (c appliedCards) undo(t *testing.T, g *Game) {
	t.Helper()
	if _, err := g.Undo(1); err != nil {
		t.Fatal(err)
	}
	c.rename(g)
}

// appliedGame is respondGame with two more of p1's fixture Trainers
// turned into a Special Red Card (me04-082) and an Iono with its real
// wording (sv02-185), all four in p1's hand, and p2 (the assistant)
// holding 6 cards. It is p1's turn 1, in play mode.
func appliedGame(t *testing.T) (*Manager, *Game, appliedCards) {
	t.Helper()
	m, g, judge, stamp := respondGame(t)
	db := m.CardDB().(cardstest.FakeDB)
	db["me04-082"] = &cards.Card{ID: "me04-082", Name: "Special Red Card", Category: "Trainer", TrainerType: "Item", Effect: textSpecialRedCard}
	db["sv02-185"] = &cards.Card{ID: "sv02-185", Name: "Iono", Category: "Trainer", TrainerType: "Supporter", Effect: textIono}

	red := spareTrainer(t, g, state.P1)
	iono := spareTrainer(t, g, state.P1, red)
	c := appliedCards{judge: judge, stamp: stamp, red: red, iono: iono}
	c.rename(g)
	for _, iid := range []state.IID{red, iono} {
		if l, _ := g.State().Find(iid); l.Zone != state.Hand {
			must(t)(g.Move(state.P1, state.ActorHuman, iid, state.Loc{Zone: state.Hand}, "", ""))
		}
	}
	setHand(t, g, state.P2, 6)
	return m, g, c
}

// spareTrainer finds one of seat's fixture Trainers (an Iono, else an
// Ultra Ball) not in play, in the prizes or one of except, to turn into
// another card.
func spareTrainer(t *testing.T, g *Game, seat state.Seat, except ...state.IID) state.IID {
	t.Helper()
	st := g.State()
	side := st.Sides[seat]
	pool := append(append([]state.IID{}, side.Hand...), side.Deck...)
	for _, def := range []string{"tst-4", "tst-3"} {
		for _, iid := range pool {
			if st.Cards[iid].Def == def && !contains(except, iid) {
				return iid
			}
		}
	}
	t.Fatalf("%s has no spare Trainer", seat)
	return ""
}

func actorOf(seat state.Seat) state.Actor {
	if seat == state.P1 {
		return state.ActorHuman
	}
	return state.ActorClaude
}

// setHand makes seat hold n cards, moving its last hand cards to the
// bottom of its deck or drawing from the top, in review mode; the mode is
// put back afterwards.
func setHand(t *testing.T, g *Game, seat state.Seat, n int) {
	t.Helper()
	mode := g.Mode()
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	a := actorOf(seat)
	for {
		side := g.State().Sides[seat]
		switch {
		case len(side.Hand) > n:
			must(t)(g.Move(seat, a, side.Hand[len(side.Hand)-1], state.Loc{Seat: seat, Zone: state.Deck}, "", ""))
			continue
		case len(side.Hand) < n:
			must(t)(g.Move(seat, a, side.Deck[0], state.Loc{Seat: seat, Zone: state.Hand}, "", ""))
			continue
		}
		break
	}
	if err := g.SetMode(mode); err != nil {
		t.Fatal(err)
	}
}

// takePrizes has seat take n prize cards, in review mode; the mode is put
// back afterwards.
func takePrizes(t *testing.T, g *Game, seat state.Seat, n int) {
	t.Helper()
	mode := g.Mode()
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		must(t)(g.TakePrize(seat, actorOf(seat), -1))
	}
	if err := g.SetMode(mode); err != nil {
		t.Fatal(err)
	}
}

func headOf(g *Game) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.log.Head()
}

func eventsAfter(t *testing.T, g *Game, cursor int) []state.Event {
	t.Helper()
	g.mu.Lock()
	defer g.mu.Unlock()
	evs, ok := g.eventsSince(cursor)
	if !ok {
		t.Fatalf("cursor %d is off the current branch", cursor)
	}
	return evs
}

func appliedAfter(g *Game, cursor int, seat state.Seat) []AppliedEffect {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.appliedSince(cursor, seat)
}

// sameSet reports whether a and b hold the same iids, in any order.
func sameSet(a, b []state.IID) bool {
	if len(a) != len(b) {
		return false
	}
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

// p2Shuffle returns the order of the one shuffle of p2's deck among evs,
// or nil if there is none.
func p2Shuffle(t *testing.T, evs []state.Event) []state.IID {
	t.Helper()
	var order []state.IID
	n := 0
	for i := range evs {
		if evs[i].Type != state.EvShuffle {
			continue
		}
		p, err := evs[i].Decode()
		if err != nil {
			t.Fatal(err)
		}
		if sp := p.(*state.ShufflePayload); sp.Seat == state.P2 {
			order = sp.Order
			n++
		}
	}
	if n > 1 {
		t.Fatalf("p2's deck was shuffled %d times", n)
	}
	return order
}

// TestAppliedJudge: the player's Judge leaves the assistant (which held 6
// cards) 4 cards drawn from the top of its deck once shuffled with its
// old hand in it, all in the Judge's own action and Result; the
// assistant's events carry its seat and the system actor; appliedSince
// reports it once.
func TestAppliedJudge(t *testing.T) {
	_, g, c := appliedGame(t)
	before := g.State()
	oldHand := slices.Clone(before.Sides[state.P2].Hand)
	oldDeck := slices.Clone(before.Sides[state.P2].Deck)
	if len(oldHand) != 6 {
		t.Fatalf("p2 holds %d cards before the Judge, want 6", len(oldHand))
	}
	p1Hand := len(before.Sides[state.P1].Hand)
	cursor := headOf(g)

	r := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, c.judge))

	after := g.State()
	hand := after.Sides[state.P2].Hand
	if len(hand) != 4 {
		t.Fatalf("p2 holds %d cards after the Judge, want 4", len(hand))
	}
	if got := len(after.Sides[state.P2].Deck); got != len(oldDeck)+6-4 {
		t.Errorf("p2's deck has %d cards, want %d", got, len(oldDeck)+2)
	}
	// The player's own part is the player's to do: only the Judge left.
	if got := len(after.Sides[state.P1].Hand); got != p1Hand-1 {
		t.Errorf("p1 holds %d cards, want %d (only the Judge played)", got, p1Hand-1)
	}

	evs := eventsAfter(t, g, cursor)
	order := p2Shuffle(t, evs)
	if order == nil {
		t.Fatal("p2's deck was not shuffled")
	}
	if !sameSet(order, append(slices.Clone(oldDeck), oldHand...)) {
		t.Errorf("the shuffled deck %v is not p2's old deck plus its old hand %v", order, oldHand)
	}
	if !slices.Equal(hand, order[:4]) {
		t.Errorf("p2's hand %v is not the top 4 of the shuffled deck %v", hand, order[:4])
	}

	inResult := map[int]bool{}
	for _, id := range r.Events {
		inResult[id] = true
	}
	var toDeck, drew, shuffles, judgeMoves int
	for i := range evs {
		e := &evs[i]
		if e.Action != r.ActionID {
			t.Errorf("event %d (%s) is in action %q, want the Judge's %q", e.ID, e.Type, e.Action, r.ActionID)
		}
		if !inResult[e.ID] {
			t.Errorf("event %d (%s) is not in the Judge's Result", e.ID, e.Type)
		}
		if e.Seat == state.P1 {
			if e.Actor != state.ActorHuman {
				t.Errorf("p1 event %d has actor %s, want human", e.ID, e.Actor)
			}
			judgeMoves++
			continue
		}
		if e.Seat != state.P2 || e.Actor != state.ActorSystem {
			t.Errorf("event %d has seat %s actor %s, want p2 system", e.ID, e.Seat, e.Actor)
		}
		p, err := e.Decode()
		if err != nil {
			t.Fatal(err)
		}
		switch p := p.(type) {
		case *state.MovePayload:
			switch {
			case p.From != nil && p.From.Zone == state.Hand && p.To.Zone == state.Deck:
				toDeck++
			case p.From != nil && p.From.Zone == state.Deck && p.To.Zone == state.Hand:
				drew++
			default:
				t.Errorf("unexpected p2 move %+v", p)
			}
		case *state.ShufflePayload:
			shuffles++
		default:
			t.Errorf("unexpected p2 event %s", e.Type)
		}
	}
	if toDeck != 6 || shuffles != 1 || drew != 4 || judgeMoves != 1 {
		t.Errorf("events: %d hand-to-deck, %d shuffles, %d draws, %d p1 moves; want 6, 1, 4, 1", toDeck, shuffles, drew, judgeMoves)
	}
	if hasWarning(r.Warnings, "condition is not met") {
		t.Errorf("Judge has no condition, but warned: %v", r.Warnings)
	}

	got := appliedAfter(g, cursor, state.P1)
	want := []AppliedEffect{{
		IID: c.judge, Name: "Judge", Def: "sv01-176", Action: r.ActionID,
		What: "p2's hand of 6 cards was shuffled into p2's deck; p2 drew 4 cards",
	}}
	if !slices.Equal(got, want) {
		t.Errorf("appliedSince:\n got  %+v\n want %+v", got, want)
	}
	// Seen from the assistant's seat, nothing was applied to p1.
	if got := appliedAfter(g, cursor, state.P2); len(got) != 0 {
		t.Errorf("appliedSince for p2: %+v, want none", got)
	}
}

// TestAppliedFamilyCounts: Special Red Card leaves the assistant 3 cards
// with its old hand at the bottom of its deck; Iono leaves as many cards
// as its remaining prizes (4 here) with its old hand at the bottom;
// Unfair Stamp leaves 2 after shuffling its old hand in.
func TestAppliedFamilyCounts(t *testing.T) {
	for _, tt := range []struct {
		name   string
		card   func(appliedCards) state.IID
		def    string
		prizes int // prizes p2 takes first
		draws  int
		bottom bool
		what   string
	}{
		{"Special Red Card", func(c appliedCards) state.IID { return c.red }, "me04-082", 3, 3, true,
			"p2's hand of 6 cards was put on the bottom of p2's deck; p2 drew 3 cards"},
		{"Iono", func(c appliedCards) state.IID { return c.iono }, "sv02-185", 2, 4, true,
			"p2's hand of 6 cards was put on the bottom of p2's deck; p2 drew 4 cards"},
		{"Unfair Stamp", func(c appliedCards) state.IID { return c.stamp }, "sv06-165", 0, 2, false,
			"p2's hand of 6 cards was shuffled into p2's deck; p2 drew 2 cards"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, g, c := appliedGame(t)
			takePrizes(t, g, state.P2, tt.prizes)
			setHand(t, g, state.P2, 6)
			before := g.State()
			oldHand := slices.Clone(before.Sides[state.P2].Hand)
			oldDeck := slices.Clone(before.Sides[state.P2].Deck)
			cursor := headOf(g)
			iid := tt.card(c)

			r := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, iid))

			after := g.State()
			hand, deck := after.Sides[state.P2].Hand, after.Sides[state.P2].Deck
			if len(hand) != tt.draws {
				t.Fatalf("p2 holds %d cards, want %d", len(hand), tt.draws)
			}
			order := p2Shuffle(t, eventsAfter(t, g, cursor))
			if tt.bottom {
				if order != nil {
					t.Errorf("p2's deck was shuffled; the hand should only go to the bottom")
				}
				k := len(oldDeck)
				if !slices.Equal(hand, oldDeck[:tt.draws]) {
					t.Errorf("p2 drew %v, want the old top %v", hand, oldDeck[:tt.draws])
				}
				if len(deck) != k-tt.draws+6 {
					t.Fatalf("p2's deck has %d cards, want %d", len(deck), k-tt.draws+6)
				}
				if !slices.Equal(deck[:k-tt.draws], oldDeck[tt.draws:]) {
					t.Errorf("the rest of p2's old deck moved: got %v, want %v", deck[:k-tt.draws], oldDeck[tt.draws:])
				}
				if !sameSet(deck[k-tt.draws:], oldHand) {
					t.Errorf("the bottom of p2's deck is %v, want its old hand %v", deck[k-tt.draws:], oldHand)
				}
			} else {
				if order == nil {
					t.Fatal("p2's deck was not shuffled")
				}
				if !sameSet(order, append(slices.Clone(oldDeck), oldHand...)) {
					t.Errorf("the shuffled deck is not p2's old deck plus its old hand")
				}
				if !slices.Equal(hand, order[:tt.draws]) {
					t.Errorf("p2's hand %v is not the top of the shuffled deck %v", hand, order[:tt.draws])
				}
			}
			want := []AppliedEffect{{IID: iid, Name: tt.name, Def: tt.def, Action: r.ActionID, What: tt.what}}
			if got := appliedAfter(g, cursor, state.P1); !slices.Equal(got, want) {
				t.Errorf("appliedSince:\n got  %+v\n want %+v", got, want)
			}
		})
	}
}

// TestAppliedUnmetCondition: Unfair Stamp with no Knock Out of the
// player's Pokémon in the opponent's last turn, and Special Red Card
// while the opponent has 6 prizes, still apply and warn that the
// condition was not met.
func TestAppliedUnmetCondition(t *testing.T) {
	for _, tt := range []struct {
		name  string
		card  func(appliedCards) state.IID
		draws int
		why   string
	}{
		{"Unfair Stamp", func(c appliedCards) state.IID { return c.stamp }, 2, "Knocked Out"},
		{"Special Red Card", func(c appliedCards) state.IID { return c.red }, 3, "6 Prize cards"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, g, c := appliedGame(t)
			r := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, tt.card(c)))
			if got := len(g.State().Sides[state.P2].Hand); got != tt.draws {
				t.Errorf("p2 holds %d cards, want %d: the effect applies anyway", got, tt.draws)
			}
			prefix := tt.name + "'s condition is not met ("
			suffix := "); its effect was applied anyway"
			i := slices.IndexFunc(r.Warnings, func(w string) bool {
				return strings.HasPrefix(w, prefix) && strings.HasSuffix(w, suffix) && strings.Contains(w, tt.why)
			})
			if i < 0 {
				t.Errorf("warnings %q: want %q…%q mentioning %q", r.Warnings, prefix, suffix, tt.why)
			}
		})
	}
}

// TestAppliedConditionMet: Special Red Card with the opponent at 3 prizes,
// and Unfair Stamp after the assistant knocked out the player's Active
// during its previous turn, apply with no condition warning.
func TestAppliedConditionMet(t *testing.T) {
	t.Run("Special Red Card", func(t *testing.T) {
		_, g, c := appliedGame(t)
		takePrizes(t, g, state.P2, 3)
		setHand(t, g, state.P2, 6)
		r := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, c.red))
		if hasWarning(r.Warnings, "condition is not met") {
			t.Errorf("warned with p2 at 3 prizes: %v", r.Warnings)
		}
		if got := len(g.State().Sides[state.P2].Hand); got != 3 {
			t.Errorf("p2 holds %d cards, want 3", got)
		}
	})
	t.Run("Unfair Stamp", func(t *testing.T) {
		_, g, c := appliedGame(t)
		hu, cl := state.ActorHuman, state.ActorClaude
		// p1 benches a Riolu so it can promote after the Knock Out.
		riolu := cardIn(t, g, state.P1, "tst-1", false)
		if l, _ := g.State().Find(riolu); l.Zone != state.Hand {
			must(t)(g.Move(state.P1, hu, riolu, state.Loc{Zone: state.Hand}, "", ""))
		}
		must(t)(g.Bench(state.P1, hu, riolu, -1))
		must(t)(g.EndTurn(state.P1, hu))
		// p2's turn: it knocks out p1's Active and takes its prize.
		must(t)(g.KnockOut(state.P2, cl, state.Loc{Seat: state.P1, Zone: state.Active}))
		must(t)(g.TakePrize(state.P2, cl, -1))
		must(t)(g.Switch(state.P1, hu, 0))
		must(t)(g.EndTurn(state.P2, cl))

		r := must(t)(g.PlayTrainer(state.P1, hu, c.stamp))
		if hasWarning(r.Warnings, "condition is not met") {
			t.Errorf("warned after p1's Active was knocked out in p2's last turn: %v", r.Warnings)
		}
		if got := len(g.State().Sides[state.P2].Hand); got != 2 {
			t.Errorf("p2 holds %d cards, want 2", got)
		}
	})
}

// TestAppliedUndo: undoing the card restores the assistant's hand and
// deck and the player's hand exactly, order included.
func TestAppliedUndo(t *testing.T) {
	for _, tt := range []struct {
		name string
		card func(appliedCards) state.IID
	}{
		{"Judge", func(c appliedCards) state.IID { return c.judge }},
		{"Iono", func(c appliedCards) state.IID { return c.iono }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, g, c := appliedGame(t)
			before := g.State()
			snap := snapshot(t, g)
			must(t)(g.PlayTrainer(state.P1, state.ActorHuman, tt.card(c)))
			if slices.Equal(g.State().Sides[state.P2].Hand, before.Sides[state.P2].Hand) {
				t.Fatal("p2's hand did not change: nothing to undo")
			}
			c.undo(t, g)
			after := g.State()
			for _, s := range []struct {
				what      string
				got, want []state.IID
			}{
				{"p2 hand", after.Sides[state.P2].Hand, before.Sides[state.P2].Hand},
				{"p2 deck", after.Sides[state.P2].Deck, before.Sides[state.P2].Deck},
				{"p1 hand", after.Sides[state.P1].Hand, before.Sides[state.P1].Hand},
			} {
				if !slices.Equal(s.got, s.want) {
					t.Errorf("%s after undo: %v, want %v", s.what, s.got, s.want)
				}
			}
			if got := snapshot(t, g); got != snap {
				t.Errorf("state after undo differs from before the card")
			}
		})
	}
}

// TestAppliedNotForAssistantCards: the assistant's own Judge, on its turn,
// changes nothing on the player's side.
func TestAppliedNotForAssistantCards(t *testing.T) {
	_, g, _ := appliedGame(t)
	cl := state.ActorClaude
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
	theirs := cardIn(t, g, state.P2, "tst-4", false)
	g.mu.Lock()
	g.st.Cards[theirs].Def, g.st.Cards[theirs].Name = "sv01-176", "Judge"
	g.mu.Unlock()
	if l, _ := g.State().Find(theirs); l.Zone != state.Hand {
		must(t)(g.Move(state.P2, cl, theirs, state.Loc{Zone: state.Hand}, "", ""))
	}
	before := g.State()
	cursor := headOf(g)

	r := must(t)(g.PlayTrainer(state.P2, cl, theirs))

	after := g.State()
	if !slices.Equal(after.Sides[state.P1].Hand, before.Sides[state.P1].Hand) || !slices.Equal(after.Sides[state.P1].Deck, before.Sides[state.P1].Deck) {
		t.Errorf("p1's hand or deck changed after p2's Judge")
	}
	for _, e := range eventsAfter(t, g, cursor) {
		if e.Seat != state.P2 || e.Actor != cl {
			t.Errorf("p2's Judge made event %d (%s) for seat %s actor %s", e.ID, e.Type, e.Seat, e.Actor)
		}
	}
	if len(r.Events) != 1 {
		t.Errorf("p2's Judge made %d events, want 1 (the Judge to the discard)", len(r.Events))
	}
	if got := appliedAfter(g, cursor, state.P2); len(got) != 0 {
		t.Errorf("appliedSince for p2: %+v, want none", got)
	}
}

// TestAppliedReviewNeedsHandedSeat: in review mode the player's Judge
// applies nothing to the assistant until the player's seat is handed
// over; handing the other seat does not count; once p1 is handed it
// applies; a mode switch clears the handover.
func TestAppliedReviewNeedsHandedSeat(t *testing.T) {
	_, g, c := appliedGame(t)
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	hu := state.ActorHuman
	tryJudge := func(want int) {
		t.Helper()
		before := g.State().Sides[state.P2]
		cursor := headOf(g)
		must(t)(g.PlayTrainer(state.P1, hu, c.judge))
		after := g.State().Sides[state.P2]
		if want == 6 {
			if !slices.Equal(after.Hand, before.Hand) || !slices.Equal(after.Deck, before.Deck) {
				t.Errorf("p2's hand or deck changed: %v → %v", before.Hand, after.Hand)
			}
			if got := appliedAfter(g, cursor, state.P1); len(got) != 0 {
				t.Errorf("appliedSince: %+v, want none", got)
			}
		} else if len(after.Hand) != want {
			t.Errorf("p2 holds %d cards, want %d", len(after.Hand), want)
		}
		c.undo(t, g)
	}

	tryJudge(6) // not handed
	g.setHanded(state.P2)
	tryJudge(6) // the other seat handed
	g.setHanded(state.P1)
	tryJudge(4) // p1 handed
	// The undo above moved the branch, which clears the handover.
	tryJudge(6)

	g.setHanded(state.P1)
	if err := g.SetMode(ModePlay); err != nil {
		t.Fatal(err)
	}
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	tryJudge(6) // the mode switch cleared it
}

// TestAppliedReviewHandedByWait: in review mode the player's Judge
// applies nothing to the assistant before any turn wait; once a wait on
// p1 has handed the seat over, the Judge leaves p2 holding 4 cards; when
// p1 ends the turn the wait returns turn_ended and the handover is
// cleared.
func TestAppliedReviewHandedByWait(t *testing.T) {
	m, g, c := appliedGame(t)
	m.Timing = Timing{WaitCap: 100 * time.Millisecond, CheckIn: time.Minute}
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	hu := state.ActorHuman
	handed := func() state.Seat {
		g.mu.Lock()
		defer g.mu.Unlock()
		return g.handed
	}

	// No wait yet: nothing applies to p2.
	before := g.State().Sides[state.P2]
	cursor := headOf(g)
	must(t)(g.PlayTrainer(state.P1, hu, c.judge))
	after := g.State().Sides[state.P2]
	if !slices.Equal(after.Hand, before.Hand) || !slices.Equal(after.Deck, before.Deck) {
		t.Errorf("p2's hand or deck changed with no wait: %v → %v", before.Hand, after.Hand)
	}
	if got := appliedAfter(g, cursor, state.P1); len(got) != 0 {
		t.Errorf("appliedSince with no wait: %+v, want none", got)
	}
	c.undo(t, g)
	if n := len(g.State().Sides[state.P2].Hand); n != 6 {
		t.Fatalf("p2 holds %d cards after the undo, want 6", n)
	}

	// A wait on p1 hands the seat over, and keeps it handed when the call
	// returns still_waiting.
	out := waitNow(t, m, WaitRequest{Seat: state.P1})
	if out.Kind != WaitStillWaiting {
		t.Fatalf("first wait: %+v; want still_waiting", out)
	}
	if got := handed(); got != state.P1 {
		t.Fatalf("handed %q after the wait, want p1", got)
	}
	must(t)(g.PlayTrainer(state.P1, hu, c.judge))
	if n := len(g.State().Sides[state.P2].Hand); n != 4 {
		t.Errorf("p2 holds %d cards after the handed Judge, want 4", n)
	}
	g.EndAction()

	// The applied Judge does not stop the wait, and is reported.
	out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
	if out.Kind != WaitStillWaiting || len(out.Cards) != 0 {
		t.Fatalf("after Judge: %+v; want still_waiting with no card to answer", out)
	}
	if len(out.Applied) != 1 || out.Applied[0].Name != "Judge" || out.Applied[0].What != "p2's hand of 6 cards was shuffled into p2's deck; p2 drew 4 cards" {
		t.Fatalf("applied after the handed Judge: %+v", out.Applied)
	}

	// p1 ends the turn: turn_ended, and the handover is cleared.
	done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
	must(t)(g.EndTurn(state.P1, hu))
	r := recv(t, done, 2*time.Second)
	if r.err != nil || r.out.Kind != WaitTurnEnded {
		t.Fatalf("after end_turn: %+v, %v; want turn_ended", r.out, r.err)
	}
	if got := handed(); got != "" {
		t.Errorf("handed %q after turn_ended, want none", got)
	}
}

// TestAppliedHiddenFromPlayer: in play mode the player's view of the
// state and of the log never names the cards the assistant drew.
func TestAppliedHiddenFromPlayer(t *testing.T) {
	_, g, c := appliedGame(t)
	cursor := headOf(g)
	must(t)(g.PlayTrainer(state.P1, state.ActorHuman, c.judge))
	st := g.State()
	drawn := st.Sides[state.P2].Hand
	if len(drawn) != 4 {
		t.Fatalf("p2 holds %d cards, want 4", len(drawn))
	}

	v, err := g.View(state.P1, "get_state", "human")
	if err != nil {
		t.Fatal(err)
	}
	side := v.Sides[state.P2]
	if side.HandCount != 4 {
		t.Errorf("p1's view of p2's hand count is %d, want 4", side.HandCount)
	}
	// A hand the viewer cannot see any of is listed by count alone.
	if len(side.Hand) != 0 {
		t.Errorf("p1's view lists p2's hand cards %+v", side.Hand)
	}
	// The projection the view is built on hides each drawn card.
	g.mu.Lock()
	full := g.st.Project(state.ViewReveal, "", nil).Sides[state.P2].Hand
	g.mu.Unlock()
	for _, iid := range drawn {
		if st.KnownTo(iid, state.P1) {
			t.Errorf("p2's drawn %s is known to p1", iid)
		}
	}
	if len(full) != 4 {
		t.Errorf("the reveal projection lists %d p2 hand cards, want 4", len(full))
	}

	lines, err := g.Log(cursor, state.P1, "get_log", "human")
	if err != nil {
		t.Fatal(err)
	}
	draws := 0
	for _, l := range lines {
		if l.Seat != string(state.P2) {
			continue
		}
		if strings.Contains(l.Text, " drew ") {
			draws++
			if !strings.Contains(l.Text, "(hidden)") {
				t.Errorf("p1's log shows p2's draw: %q", l.Text)
			}
		}
		for _, iid := range drawn {
			if name := st.Name(iid); strings.Contains(l.Text, name) {
				t.Errorf("p1's log line %q names p2's drawn %s", l.Text, name)
			}
		}
	}
	if draws != 4 {
		t.Errorf("p1's log has %d p2 draw lines, want 4", draws)
	}
}

// TestAppliedAbilityAndAttack: an Ability or attack of the player's with
// the family's wording applies the opponent's part too.
func TestAppliedAbilityAndAttack(t *testing.T) {
	p1Active := state.Loc{Seat: state.P1, Zone: state.Active}
	p2Active := state.Loc{Seat: state.P2, Zone: state.Active}
	for _, tt := range []struct {
		name  string
		use   func(t *testing.T, g *Game) *Result
		label string
	}{
		{"ability", func(t *testing.T, g *Game) *Result {
			return must(t)(g.UseAbility(state.P1, state.ActorHuman, p1Active, "Focus"))
		}, "Riolu: Focus"},
		{"attack", func(t *testing.T, g *Game) *Result {
			return must(t)(g.Attack(state.P1, state.ActorHuman, "Punch", p2Active, 10))
		}, "Riolu: Punch"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, g, _ := appliedGame(t)
			db := m.CardDB().(cardstest.FakeDB)
			riolu := *db["tst-1"]
			riolu.Abilities = []cards.Ability{{Name: "Focus", Effect: textRedCard}}
			riolu.Attacks = []cards.Attack{{Name: "Punch", Damage: "10", Effect: textRedCard}}
			db["tst-1"] = &riolu
			cursor := headOf(g)

			r := tt.use(t, g)

			if got := len(g.State().Sides[state.P2].Hand); got != 4 {
				t.Errorf("p2 holds %d cards, want 4", got)
			}
			want := []AppliedEffect{{
				IID: g.State().Sides[state.P1].Active.Top(), Name: tt.label, Def: "tst-1", Action: r.ActionID,
				What: "p2's hand of 6 cards was shuffled into p2's deck; p2 drew 4 cards",
			}}
			if got := appliedAfter(g, cursor, state.P1); !slices.Equal(got, want) {
				t.Errorf("appliedSince:\n got  %+v\n want %+v", got, want)
			}
		})
	}
}

// TestAppliedDrawCondition: Special Red Card draws nothing when the
// assistant had no hand to put on the bottom; Iono still draws when only
// the player put cards there.
func TestAppliedDrawCondition(t *testing.T) {
	for _, tt := range []struct {
		name  string
		card  func(appliedCards) state.IID
		draws int
	}{
		{"Special Red Card", func(c appliedCards) state.IID { return c.red }, 0},
		{"Iono", func(c appliedCards) state.IID { return c.iono }, 6},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, g, c := appliedGame(t)
			setHand(t, g, state.P2, 0)
			deck := len(g.State().Sides[state.P2].Deck)
			must(t)(g.PlayTrainer(state.P1, state.ActorHuman, tt.card(c)))
			after := g.State().Sides[state.P2]
			if len(after.Hand) != tt.draws || len(after.Deck) != deck-tt.draws {
				t.Errorf("p2 holds %d cards with %d in its deck; want %d and %d", len(after.Hand), len(after.Deck), tt.draws, deck-tt.draws)
			}
		})
	}
}
