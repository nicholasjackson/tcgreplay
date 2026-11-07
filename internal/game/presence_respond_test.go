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

const (
	judgeText       = "Each player shuffles their hand into their deck and draws 4 cards."
	unfairStampText = "You can use this card only if any of your Pokémon were Knocked Out during your opponent's last turn.\n\nEach player shuffles their hand into their deck. Then, you draw 5 cards, and your opponent draws 2 cards."
	eriText         = "Your opponent reveals their hand, and you discard up to 2 Item cards you find there."
)

// eriCard turns one of p1's spare Trainers (an Ultra Ball, not one in
// except) into Eri (sv05-210), whose effect involves the opponent in a way
// the playmat does not apply itself, so it still stops the wait. It puts
// the card in p1's hand and returns it.
func eriCard(t *testing.T, m *Manager, g *Game, except ...state.IID) state.IID {
	t.Helper()
	return trainerCard(t, m, g, &cards.Card{ID: "sv05-210", Name: "Eri", Category: "Trainer", TrainerType: "Supporter", Effect: eriText}, except...)
}

// trainerCard adds c to the card DB and turns one of p1's spare Ultra
// Balls (not one in except) into it, in p1's hand. The swap is made on the
// live state only, so an undo or checkout reverts it.
func trainerCard(t *testing.T, m *Manager, g *Game, c *cards.Card, except ...state.IID) state.IID {
	t.Helper()
	m.DB.(cardstest.FakeDB)[c.ID] = c
	return renamedCard(t, g, c.ID, c.Name, except...)
}

// renamedCard turns one of p1's spare Ultra Balls (not one in except) into
// the card def (named name, whether or not the DB knows def), in p1's
// hand.
func renamedCard(t *testing.T, g *Game, def, name string, except ...state.IID) state.IID {
	t.Helper()
	iid := cardIn(t, g, state.P1, "tst-3", false, except...)
	g.mu.Lock()
	g.st.Cards[iid].Def, g.st.Cards[iid].Name = def, name
	g.mu.Unlock()
	toHand(t, g, state.P1, iid)
	return iid
}

// toHand moves one of seat's cards to its hand, unless it is there.
func toHand(t *testing.T, g *Game, seat state.Seat, iid state.IID) {
	t.Helper()
	if l, _ := g.State().Find(iid); l.Zone != state.Hand {
		must(t)(g.Move(seat, actorOf(seat), iid, state.Loc{Zone: state.Hand}, "", ""))
	}
}

// respondGame is startedManager with p2 (the assistant) holding a Benched
// Riolu, and p1's hand holding Judge and Unfair Stamp, which the card DB
// knows. It returns the iids of the Judge and the Unfair Stamp.
func respondGame(t *testing.T) (*Manager, *Game, state.IID, state.IID) {
	t.Helper()
	m, g := startedManager(t)
	db := m.DB.(cardstest.FakeDB)
	db["sv01-176"] = &cards.Card{ID: "sv01-176", Name: "Judge", Category: "Trainer", TrainerType: "Supporter", Effect: judgeText}
	db["sv06-165"] = &cards.Card{ID: "sv06-165", Name: "Unfair Stamp", Category: "Trainer", TrainerType: "Item", Effect: unfairStampText}

	// p2 benches a Riolu (in review, as it is p1's turn).
	if err := g.SetMode(ModeReview); err != nil {
		t.Fatal(err)
	}
	riolu := cardIn(t, g, state.P2, "tst-1", false)
	must(t)(g.Move(state.P2, state.ActorClaude, riolu, state.Loc{Zone: state.Hand}, "", ""))
	must(t)(g.Bench(state.P2, state.ActorClaude, riolu, -1))
	if err := g.SetMode(ModePlay); err != nil {
		t.Fatal(err)
	}

	// Two of p1's Ionos become a Judge and an Unfair Stamp, in hand.
	judge := cardIn(t, g, state.P1, "tst-4", false)
	stamp := cardIn(t, g, state.P1, "tst-4", false, judge)
	g.mu.Lock()
	g.st.Cards[judge].Def, g.st.Cards[judge].Name = "sv01-176", "Judge"
	g.st.Cards[stamp].Def, g.st.Cards[stamp].Name = "sv06-165", "Unfair Stamp"
	g.mu.Unlock()
	for _, iid := range []state.IID{judge, stamp} {
		if l, _ := g.State().Find(iid); l.Zone != state.Hand {
			must(t)(g.Move(state.P1, state.ActorHuman, iid, state.Loc{Zone: state.Hand}, "", ""))
		}
	}
	return m, g, judge, stamp
}

// cardIn finds one of seat's cards with def that is not in play and not
// one of except (in hand only, if inHand).
func cardIn(t *testing.T, g *Game, seat state.Seat, def string, inHand bool, except ...state.IID) state.IID {
	t.Helper()
	st := g.State()
	side := st.Sides[seat]
	pool := append(append([]state.IID{}, side.Hand...), side.Deck...)
	if inHand {
		pool = side.Hand
	}
	for _, iid := range pool {
		if st.Cards[iid].Def == def && !contains(except, iid) {
			return iid
		}
	}
	t.Fatalf("%s has no %s to use", seat, def)
	return ""
}

func contains(ids []state.IID, id state.IID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// respondBench is p2's Bench after respondGame: the Riolu it benched.
var respondBench = []BenchChoice{{Slot: 0, IID: "p2-3", Name: "Riolu"}}

// TestWaitPromote: knocking out the assistant's Active by attack, by
// Ability or by a plain knock-out reports one promotion decision listing
// its Bench; after the assistant promotes, a continuing wait goes back to
// waiting, without reporting the damage-only attack or the Ability that
// asks nothing of the opponent.
func TestWaitPromote(t *testing.T) {
	for _, tt := range []struct {
		name string
		ko   func(t *testing.T, g *Game)
	}{
		{"attack", func(t *testing.T, g *Game) {
			must(t)(g.Attack(state.P1, state.ActorHuman, "Punch", p2Active, 200))
			must(t)(g.KnockOut(state.P1, state.ActorHuman, p2Active))
		}},
		{"ability", func(t *testing.T, g *Game) {
			must(t)(g.UseAbility(state.P1, state.ActorHuman, p1Active, "Focus"))
			must(t)(g.KnockOut(state.P1, state.ActorHuman, p2Active))
		}},
		{"knock out", func(t *testing.T, g *Game) {
			must(t)(g.KnockOut(state.P1, state.ActorHuman, p2Active))
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, g, _, _ := respondGame(t)
			m.Timing = Timing{WaitCap: 150 * time.Millisecond, CheckIn: time.Minute}
			done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
			tt.ko(t, g)
			r := recv(t, done, time.Second)
			if r.err != nil || r.out.Kind != WaitDecide || r.out.Action != DecidePromote {
				t.Fatalf("after the knock-out: %+v, %v; want decide/promote", r.out, r.err)
			}
			if !slices.Equal(r.out.Choices, respondBench) || len(r.out.Cards) != 0 {
				t.Fatalf("promote choices %+v cards %+v; want %+v and no cards", r.out.Choices, r.out.Cards, respondBench)
			}
			if r.took > 100*time.Millisecond {
				t.Errorf("promote took %v", r.took)
			}

			// Not reported again for the same vacancy, and the attack or
			// Ability is not reported at all.
			out := waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(r.out.Next)})
			if out.Kind != WaitStillWaiting || len(out.Cards) != 0 {
				t.Fatalf("continuing before promoting: %+v; want still_waiting", out)
			}

			must(t)(g.Switch(state.P2, state.ActorClaude, 0))
			if g.State().Sides[state.P2].Active.Empty() {
				t.Fatal("p2 did not promote")
			}
			if out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)}); out.Kind != WaitStillWaiting || len(out.Cards) != 0 {
				t.Fatalf("after promoting: %+v; want still_waiting", out)
			}
		})
	}
}

// TestWaitReportsAppliedJudgeAndUnfairStamp: the player's Judge and
// Unfair Stamp apply themselves to the assistant's side and do not stop
// the wait; the next result, a still_waiting one, lists each with what
// happened, a later result does not repeat it, the assistant's seat stays
// under the turn lock, and the turn-ended result lists both, with log
// lines for the assistant's draws under each card's name.
func TestWaitReportsAppliedJudgeAndUnfairStamp(t *testing.T) {
	m, g, judge, stamp := respondGame(t)
	m.Timing = Timing{WaitCap: 150 * time.Millisecond, CheckIn: time.Minute}

	// The first wait is fresh; the rest carry on from the previous result.
	var since *int
	start := 0
	ids := map[state.IID]string{}
	for _, tt := range []struct {
		iid       state.IID
		name, def string
		hand      int
		what      string
	}{
		{judge, "Judge", "sv01-176", 4, "p2's hand of 6 cards was shuffled into p2's deck; p2 drew 4 cards"},
		{stamp, "Unfair Stamp", "sv06-165", 2, "p2's hand of 4 cards was shuffled into p2's deck; p2 drew 2 cards"},
	} {
		done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1, Since: since})
		r := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, tt.iid))
		ids[tt.iid] = r.ActionID
		g.EndAction()
		got := recv(t, done, time.Second)
		if got.err != nil || got.out.Kind != WaitStillWaiting || len(got.out.Cards) != 0 {
			t.Fatalf("after %s: %+v, %v; want still_waiting with no card to answer", tt.name, got.out, got.err)
		}
		if n := len(g.State().Sides[state.P2].Hand); n != tt.hand {
			t.Fatalf("p2 holds %d cards after %s, want %d", n, tt.name, tt.hand)
		}
		want := AppliedEffect{IID: tt.iid, Name: tt.name, Def: tt.def, Action: r.ActionID, What: tt.what}
		if len(got.out.Applied) != 1 || got.out.Applied[0] != want {
			t.Fatalf("applied after %s: %+v, want [%+v]", tt.name, got.out.Applied, want)
		}
		if since == nil {
			start = got.out.Since
		}

		out := waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(got.out.Next)})
		if out.Kind != WaitStillWaiting || len(out.Applied) != 0 {
			t.Fatalf("continuing after %s: %+v; want still_waiting with nothing applied again", tt.name, out)
		}
		since = intp(out.Next)
		if _, err := g.Draw(state.P2, state.ActorClaude, 1); err == nil || !strings.Contains(err.Error(), "it is p1's turn") {
			t.Fatalf("assistant draw after %s: %v; want the turn lock", tt.name, err)
		}
	}

	done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1, Since: since})
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
	r := recv(t, done, time.Second)
	if r.err != nil || r.out.Kind != WaitTurnEnded {
		t.Fatalf("end of turn: %+v, %v; want turn_ended", r.out, r.err)
	}
	if len(r.out.Applied) != 2 || r.out.Applied[0].Name != "Judge" || r.out.Applied[1].Name != "Unfair Stamp" {
		t.Fatalf("turn_ended applied %+v, want Judge then Unfair Stamp", r.out.Applied)
	}
	lines, err := g.Log(start, state.P1, "", "")
	if err != nil {
		t.Fatal(err)
	}
	drew := map[string]int{}
	for _, l := range lines {
		// p2's own turn-start draw follows; only the cards' draws count.
		if l.Seat == string(state.P2) && strings.Contains(l.Text, "p2 drew") && (l.Action == ids[judge] || l.Action == ids[stamp]) {
			drew[l.Label]++
		}
	}
	if drew["Judge"] != 4 || drew["Unfair Stamp"] != 2 {
		t.Errorf("log lines for p2's draws by card: %v, want Judge 4 and Unfair Stamp 2", drew)
	}
}

// TestResponseWindowOnlyWhileOpen: the assistant's seat cannot act during
// the player's turn before a response is open, nor after the player ends
// the turn; a card with nothing for the assistant to do opens none.
func TestResponseWindowOnlyWhileOpen(t *testing.T) {
	m, g, _, _ := respondGame(t)
	m.Timing = Timing{WaitCap: 100 * time.Millisecond, CheckIn: time.Minute}
	refused := func(when string) {
		t.Helper()
		if _, err := g.Draw(state.P2, state.ActorClaude, 1); err == nil {
			t.Fatalf("assistant drew during the player's turn %s", when)
		}
	}
	refused("with no response open")

	ball := cardIn(t, g, state.P1, "tst-3", false)
	if l, _ := g.State().Find(ball); l.Zone != state.Hand {
		must(t)(g.Move(state.P1, state.ActorHuman, ball, state.Loc{Zone: state.Hand}, "", ""))
	}
	out := waitNow(t, m, WaitRequest{Seat: state.P1})
	if out.Kind != WaitStillWaiting {
		t.Fatalf("first wait: %+v", out)
	}
	must(t)(g.PlayTrainer(state.P1, state.ActorHuman, ball))
	g.EndAction()
	// Ultra Ball asks nothing of the assistant: the wait does not stop and
	// no response opens.
	out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
	if out.Kind != WaitStillWaiting || len(out.Cards) != 0 {
		t.Fatalf("after Ultra Ball: %+v; want still_waiting", out)
	}
	refused("after Ultra Ball")

	// A response open when the player ends the turn closes with it.
	eri := eriCard(t, m, g, ball)
	must(t)(g.PlayTrainer(state.P1, state.ActorHuman, eri))
	out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
	if out.Kind != WaitDecide || out.Action != DecideAnswerCard || len(out.Cards) != 1 || out.Cards[0].Name != "Eri" || out.Cards[0].Need != NeedRead {
		t.Fatalf("after Eri: %+v; want decide/answer_card with Eri, need read", out)
	}
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
	must(t)(g.EndTurn(state.P2, state.ActorClaude))
	if _, err := g.Draw(state.P2, state.ActorClaude, 1); err == nil {
		t.Fatal("assistant drew on the player's next turn: the window outlived the turn")
	}
}

// TestAssistantJoinNeedsResponseWindow: in play mode the assistant cannot
// slip its part of an effect into the player's turn by joining an action
// the player left open; it can once the wait has reported the card. The
// player's own part of the assistant's card still joins as before.
func TestAssistantJoinNeedsResponseWindow(t *testing.T) {
	m, g, _, _ := respondGame(t)
	m.Timing = Timing{WaitCap: 100 * time.Millisecond, CheckIn: time.Minute}
	out := waitNow(t, m, WaitRequest{Seat: state.P1})

	eri := eriCard(t, m, g)
	must(t)(g.PlayTrainer(state.P1, state.ActorHuman, eri)) // left open
	if _, err := g.Draw(state.P2, state.ActorClaude, 1); err == nil || !strings.Contains(err.Error(), "it is p1's turn") {
		t.Fatalf("assistant joined the player's open Eri with no response open: %v", err)
	}
	if out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)}); out.Kind != WaitDecide || out.Action != DecideAnswerCard {
		t.Fatalf("after Eri: %+v; want decide/answer_card", out)
	}
	r := must(t)(g.Draw(state.P2, state.ActorClaude, 1))
	if r.ActionID != g.OpenAction() {
		t.Errorf("the assistant's draw (action %s) did not join the open Eri (%s)", r.ActionID, g.OpenAction())
	}

	// On the assistant's turn, the player's draws for its Iono join it.
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
	theirs := cardIn(t, g, state.P2, "tst-4", false)
	if l, _ := g.State().Find(theirs); l.Zone != state.Hand {
		must(t)(g.Move(state.P2, state.ActorClaude, theirs, state.Loc{Zone: state.Hand}, "", ""))
	}
	open := must(t)(g.PlayTrainer(state.P2, state.ActorClaude, theirs)).ActionID
	if r := must(t)(g.Draw(state.P1, state.ActorHuman, 1)); r.ActionID != open {
		t.Errorf("the player's draw (action %s) did not join the assistant's Iono (%s)", r.ActionID, open)
	}
}

// p1Active and p2Active are the two Active Spots.
var (
	p1Active = state.Loc{Seat: state.P1, Zone: state.Active}
	p2Active = state.Loc{Seat: state.P2, Zone: state.Active}
)

// playBall plays one of p1's spare Ultra Balls (not one in except), which
// asks nothing of the opponent.
func playBall(t *testing.T, g *Game, except ...state.IID) {
	t.Helper()
	ball := cardIn(t, g, state.P1, "tst-3", false, except...)
	toHand(t, g, state.P1, ball)
	must(t)(g.PlayTrainer(state.P1, state.ActorHuman, ball))
	g.EndAction()
}

// attachEnergy attaches a Fighting Energy from p1's hand to its Active.
func attachEnergy(t *testing.T, g *Game) {
	t.Helper()
	must(t)(g.Attach(state.P1, state.ActorHuman, cardIn(t, g, state.P1, "sve-006", true), p1Active, state.AsEnergy))
}

// evolveActive evolves p1's Active Riolu into a Lucario.
func evolveActive(t *testing.T, g *Game) {
	t.Helper()
	luc := cardIn(t, g, state.P1, "tst-2", false)
	toHand(t, g, state.P1, luc)
	must(t)(g.Evolve(state.P1, state.ActorHuman, luc, p1Active))
}

// auraSphere is Lucario's damage-only attack on p2's Active, for less than
// its HP.
func auraSphere(t *testing.T, g *Game) {
	t.Helper()
	must(t)(g.Attack(state.P1, state.ActorHuman, "Aura Sphere", p2Active, 30))
}

// playTurn plays p1's turn one step at a time; after each step it waits
// (a continuing wait; the first one is fresh) until a result that is not a
// decision, promoting p2's first choice on a promotion as the assistant
// would. It returns every result in order.
func playTurn(t *testing.T, m *Manager, g *Game, steps ...func(t *testing.T, g *Game)) []WaitOutcome {
	t.Helper()
	var outs []WaitOutcome
	out := waitNow(t, m, WaitRequest{Seat: state.P1})
	outs = append(outs, out)
	for _, step := range steps {
		step(t, g)
		for {
			out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
			outs = append(outs, out)
			if out.Kind != WaitDecide {
				break
			}
			if out.Action == DecidePromote {
				must(t)(g.Switch(state.P2, state.ActorClaude, out.Choices[0].Slot))
			}
		}
	}
	return outs
}

func decisions(outs []WaitOutcome) []WaitOutcome {
	var d []WaitOutcome
	for _, o := range outs {
		if o.Kind == WaitDecide {
			d = append(d, o)
		}
	}
	return d
}

func endTurn(t *testing.T, g *Game) {
	t.Helper()
	must(t)(g.EndTurn(state.P1, state.ActorHuman))
}

// TestOrdinaryTurnNeverStops: a turn of Ultra Ball, attaching Energy,
// evolving and a damage-only attack gives only still_waiting results until
// the turn ends, then turn_ended; no result asks the assistant anything.
func TestOrdinaryTurnNeverStops(t *testing.T) {
	m, g, _, _ := respondGame(t)
	m.Timing = Timing{WaitCap: 100 * time.Millisecond, CheckIn: time.Minute}
	outs := playTurn(t, m, g,
		func(t *testing.T, g *Game) { playBall(t, g) },
		attachEnergy, evolveActive, auraSphere, endTurn)
	if len(outs) != 6 {
		t.Fatalf("%d results, want 6 (one per step and the first wait): %+v", len(outs), outs)
	}
	for i, o := range outs[:5] {
		if o.Kind != WaitStillWaiting || o.Action != "" || len(o.Cards) != 0 || len(o.Choices) != 0 {
			t.Errorf("result %d: %+v; want still_waiting", i, o)
		}
	}
	if last := outs[5]; last.Kind != WaitTurnEnded {
		t.Errorf("last result %+v; want turn_ended", last)
	}
}

// TestPromotionDecision: knocking out the assistant's Active with Pokémon
// on its Bench gives exactly one promotion decision listing exactly those
// Bench slots; knocking out its last Pokémon ends the wait with game over
// and no promotion.
func TestPromotionDecision(t *testing.T) {
	t.Run("bench", func(t *testing.T) {
		m, g, _, _ := respondGame(t)
		m.Timing = Timing{WaitCap: 100 * time.Millisecond, CheckIn: time.Minute}
		// A second Benched Riolu, in slot 1.
		if err := g.SetMode(ModeReview); err != nil {
			t.Fatal(err)
		}
		second := cardIn(t, g, state.P2, "tst-1", false)
		toHand(t, g, state.P2, second)
		must(t)(g.Bench(state.P2, state.ActorClaude, second, -1))
		if err := g.SetMode(ModePlay); err != nil {
			t.Fatal(err)
		}
		want := []BenchChoice{{Slot: 0, IID: "p2-3", Name: "Riolu"}, {Slot: 1, IID: "p2-2", Name: "Riolu"}}

		out := waitNow(t, m, WaitRequest{Seat: state.P1})
		must(t)(g.KnockOut(state.P1, state.ActorHuman, p2Active))
		var got []WaitOutcome
		for range 3 {
			out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
			got = append(got, out)
		}
		d := decisions(got)
		if len(d) != 1 || got[0].Kind != WaitDecide {
			t.Fatalf("results after the knock-out: %+v; want one decision, first", got)
		}
		if d[0].Action != DecidePromote || !slices.Equal(d[0].Choices, want) || len(d[0].Cards) != 0 {
			t.Fatalf("decision %+v; want promote with %+v", d[0], want)
		}
	})
	t.Run("last Pokémon", func(t *testing.T) {
		m, g := startedManager(t)
		m.Timing = Timing{WaitCap: time.Second, CheckIn: time.Minute}
		done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
		must(t)(g.KnockOut(state.P1, state.ActorHuman, p2Active))
		r := recv(t, done, time.Second)
		if r.err != nil || r.out.Kind != WaitEnded || r.out.Reason != EndGameOver {
			t.Fatalf("after knocking out the last Pokémon: %+v, %v; want ended/game_over", r.out, r.err)
		}
		if r.out.Action != "" || len(r.out.Choices) != 0 {
			t.Fatalf("game over carried a decision: %+v", r.out)
		}
	})
}

// TestAnswerCardNeeds: a card the playmat does not carry out gives one
// answer_card decision naming it, with what it needs: a card making each
// player switch their Active is a choice; Eri and a Trainer with no text
// in the card data must be read (never a silent pass). None is reported
// twice.
func TestAnswerCardNeeds(t *testing.T) {
	for _, tt := range []struct {
		name string
		card *cards.Card
		need string
	}{
		{"each player switches", &cards.Card{ID: "tst-91", Name: "Escape Rope", Category: "Trainer", TrainerType: "Item", Effect: "Each player switches their Active Pokémon with 1 of their Benched Pokémon."}, NeedChoose},
		{"eri", &cards.Card{ID: "sv05-210", Name: "Eri", Category: "Trainer", TrainerType: "Supporter", Effect: eriText}, NeedRead},
		{"no text", &cards.Card{ID: "tst-92", Name: "Blank Supporter", Category: "Trainer", TrainerType: "Supporter"}, NeedRead},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, g, _, _ := respondGame(t)
			m.Timing = Timing{WaitCap: 100 * time.Millisecond, CheckIn: time.Minute}
			iid := trainerCard(t, m, g, tt.card)
			out := waitNow(t, m, WaitRequest{Seat: state.P1})
			must(t)(g.PlayTrainer(state.P1, state.ActorHuman, iid))
			g.EndAction()
			out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
			want := []UsedCard{{IID: iid, Name: tt.card.Name, Def: tt.card.ID, Kind: "trainer", Text: tt.card.Effect, Need: tt.need}}
			if out.Kind != WaitDecide || out.Action != DecideAnswerCard || !slices.Equal(out.Cards, want) || len(out.Choices) != 0 {
				t.Fatalf("after %s: %+v; want decide/answer_card with %+v", tt.card.Name, out, want)
			}
			if out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)}); out.Kind != WaitStillWaiting || len(out.Cards) != 0 {
				t.Fatalf("%s reported again: %+v", tt.card.Name, out)
			}
		})
	}
}

// TestLateFamilyCardNeedsRead: a family card whose text was not available
// when it was played (so the playmat applied nothing) gives one decision
// asking to read it once its text is known.
func TestLateFamilyCardNeedsRead(t *testing.T) {
	const def = "tst-late"
	if k := ReadEffect(judgeText).Kind; k != OppApply {
		t.Fatalf("Judge reads as %v, want apply", k)
	}
	m, g, _, _ := respondGame(t)
	m.Timing = Timing{WaitCap: 100 * time.Millisecond, CheckIn: time.Minute}
	iid := renamedCard(t, g, def, "Judge")
	out := waitNow(t, m, WaitRequest{Seat: state.P1})
	hand := len(g.State().Sides[state.P2].Hand)
	must(t)(g.PlayTrainer(state.P1, state.ActorHuman, iid))
	g.EndAction()
	if n := len(g.State().Sides[state.P2].Hand); n != hand {
		t.Fatalf("p2 holds %d cards after the unknown Judge, want %d: it was applied", n, hand)
	}

	// The card's text arrives once the background fetch the play started
	// (which found nothing) is over, so the DB is not written under it.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, running := g.warming.Load(def); !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background fetch of the unknown card never finished")
		}
		time.Sleep(time.Millisecond)
	}
	m.DB.(cardstest.FakeDB)[def] = &cards.Card{ID: def, Name: "Judge", Category: "Trainer", TrainerType: "Supporter", Effect: judgeText}

	out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
	want := []UsedCard{{IID: iid, Name: "Judge", Def: def, Kind: "trainer", Text: judgeText, Need: NeedRead}}
	if out.Kind != WaitDecide || out.Action != DecideAnswerCard || !slices.Equal(out.Cards, want) || len(out.Applied) != 0 {
		t.Fatalf("after the late Judge: %+v; want decide/answer_card with %+v and nothing applied", out, want)
	}
	if out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)}); out.Kind != WaitStillWaiting {
		t.Fatalf("the late Judge reported again: %+v", out)
	}
}

// TestDecisionsPerTurn: over a turn mixing ordinary cards, Judge (which
// the playmat applies), an opponent card it cannot (Eri) and a knock-out
// of the assistant's Active with a Bench, the wait stops exactly twice:
// first the promotion, then Eri. A turn of ordinary cards stops never.
func TestDecisionsPerTurn(t *testing.T) {
	t.Run("mixed", func(t *testing.T) {
		m, g, judge, _ := respondGame(t)
		m.Timing = Timing{WaitCap: 100 * time.Millisecond, CheckIn: time.Minute}
		eri := eriCard(t, m, g)
		outs := playTurn(t, m, g,
			func(t *testing.T, g *Game) { playBall(t, g, eri) },
			attachEnergy,
			func(t *testing.T, g *Game) {
				must(t)(g.PlayTrainer(state.P1, state.ActorHuman, judge))
				g.EndAction()
			},
			func(t *testing.T, g *Game) {
				must(t)(g.PlayTrainer(state.P1, state.ActorHuman, eri))
				g.EndAction()
				must(t)(g.KnockOut(state.P1, state.ActorHuman, p2Active))
			},
			evolveActive, auraSphere,
			func(t *testing.T, g *Game) { must(t)(g.TakePrize(state.P1, state.ActorHuman, -1)) },
			endTurn)
		d := decisions(outs)
		if len(d) != 2 {
			t.Fatalf("%d decisions, want 2: %+v", len(d), outs)
		}
		if d[0].Action != DecidePromote || !slices.Equal(d[0].Choices, respondBench) {
			t.Errorf("first decision %+v; want promote with %+v", d[0], respondBench)
		}
		if d[1].Action != DecideAnswerCard || len(d[1].Cards) != 1 || d[1].Cards[0].IID != eri || d[1].Cards[0].Need != NeedRead {
			t.Errorf("second decision %+v; want answer_card with Eri (%s), need read", d[1], eri)
		}
		if last := outs[len(outs)-1]; last.Kind != WaitTurnEnded {
			t.Errorf("last result %+v; want turn_ended", last)
		}
	})
	t.Run("ordinary", func(t *testing.T) {
		m, g, _, _ := respondGame(t)
		m.Timing = Timing{WaitCap: 100 * time.Millisecond, CheckIn: time.Minute}
		outs := playTurn(t, m, g,
			func(t *testing.T, g *Game) { playBall(t, g) },
			attachEnergy, evolveActive, auraSphere, endTurn)
		if d := decisions(outs); len(d) != 0 {
			t.Fatalf("an ordinary turn stopped %d times: %+v", len(d), d)
		}
	})
}

// TestEndedWaitCarriesApplied: an effect applied but not yet reported
// still reaches the assistant when the wait ends instead of continuing: a
// mode switch after the player's Judge ends the wait with Judge in
// Applied. An undo of the Judge ends it with nothing applied, because the
// Judge is no longer on the board.
func TestEndedWaitCarriesApplied(t *testing.T) {
	t.Run("mode switch", func(t *testing.T) {
		m, g, judge, _ := respondGame(t)
		m.Timing = Timing{WaitCap: 50 * time.Millisecond, CheckIn: time.Minute}
		out := waitNow(t, m, WaitRequest{Seat: state.P1})
		if out.Kind != WaitStillWaiting {
			t.Fatalf("first wait: %+v", out)
		}
		r := must(t)(g.PlayTrainer(state.P1, state.ActorHuman, judge))
		if err := g.SetMode(ModeReview); err != nil {
			t.Fatal(err)
		}
		out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
		if out.Kind != WaitEnded || out.Reason != EndModeChanged {
			t.Fatalf("after the mode switch: %+v; want ended (mode_changed)", out)
		}
		want := AppliedEffect{IID: judge, Name: "Judge", Def: "sv01-176", Action: r.ActionID, What: "p2's hand of 6 cards was shuffled into p2's deck; p2 drew 4 cards"}
		if len(out.Applied) != 1 || out.Applied[0] != want {
			t.Fatalf("ended applied %+v, want [%+v]", out.Applied, want)
		}
	})
	t.Run("undo", func(t *testing.T) {
		m, g, judge, _ := respondGame(t)
		m.Timing = Timing{WaitCap: 50 * time.Millisecond, CheckIn: time.Minute}
		out := waitNow(t, m, WaitRequest{Seat: state.P1})
		must(t)(g.PlayTrainer(state.P1, state.ActorHuman, judge))
		if _, err := g.Undo(1); err != nil {
			t.Fatal(err)
		}
		out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
		if out.Kind != WaitEnded || len(out.Applied) != 0 {
			t.Fatalf("after the undo: %+v; want ended with nothing applied", out)
		}
	})
}
