package game

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

const (
	judgeText       = "Each player shuffles their hand into their deck and draws 4 cards."
	unfairStampText = "You can use this card only if any of your Pokémon were Knocked Out during your opponent's last turn.\n\nEach player shuffles their hand into their deck. Then, you draw 5 cards, and your opponent draws 2 cards."
)

// respondGame is startedManager with p2 (the assistant) holding a Benched
// Riolu, and p1's hand holding Judge and Unfair Stamp, which the card DB
// knows. It returns the iids of the Judge and the Unfair Stamp.
func respondGame(t *testing.T) (*Manager, *Game, state.IID, state.IID) {
	t.Helper()
	m, g := startedManager(t)
	db := m.DB.(fakeDB)
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

// shuffleHandAndDraw is the assistant's part of Judge and Unfair Stamp:
// p2 shuffles its hand into its deck and draws n. Each step must succeed.
func shuffleHandAndDraw(t *testing.T, g *Game, n int) {
	t.Helper()
	cl := state.ActorClaude
	for _, iid := range g.State().Sides[state.P2].Hand {
		must(t)(g.Move(state.P2, cl, iid, state.Loc{Seat: state.P2, Zone: state.Deck}, "", ""))
	}
	must(t)(g.Shuffle(state.P2, cl, state.P2))
	must(t)(g.Draw(state.P2, cl, n))
	if got := len(g.State().Sides[state.P2].Hand); got != n {
		t.Fatalf("p2 holds %d cards after drawing %d", got, n)
	}
}

// TestWaitPromote: knocking out the assistant's Active by attack, by
// Ability or by a plain knock-out reports a promotion once; after the
// assistant promotes, a continuing wait goes back to waiting (after
// reporting the attack or Ability that did it).
func TestWaitPromote(t *testing.T) {
	p2Active := state.Loc{Seat: state.P2, Zone: state.Active}
	for _, tt := range []struct {
		name string
		ko   func(t *testing.T, g *Game)
		// opener is the card reported after the promotion, if any.
		opener string
	}{
		{"attack", func(t *testing.T, g *Game) {
			must(t)(g.Attack(state.P1, state.ActorHuman, "Punch", p2Active, 200))
			must(t)(g.KnockOut(state.P1, state.ActorHuman, p2Active))
		}, "Riolu: Punch"},
		{"ability", func(t *testing.T, g *Game) {
			must(t)(g.UseAbility(state.P1, state.ActorHuman, state.Loc{Seat: state.P1, Zone: state.Active}, "Focus"))
			must(t)(g.KnockOut(state.P1, state.ActorHuman, p2Active))
		}, "Riolu: Focus"},
		{"knock out", func(t *testing.T, g *Game) {
			must(t)(g.KnockOut(state.P1, state.ActorHuman, p2Active))
		}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m, g, _, _ := respondGame(t)
			m.Timing = Timing{WaitCap: 150 * time.Millisecond, CheckIn: time.Minute}
			done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1})
			tt.ko(t, g)
			r := recv(t, done, time.Second)
			if r.err != nil || r.out.Kind != WaitPromote {
				t.Fatalf("after the knock-out: %+v, %v; want promote", r.out, r.err)
			}
			if r.took > 100*time.Millisecond {
				t.Errorf("promote took %v", r.took)
			}
			next := r.out.Next

			// Not reported again for the same vacancy.
			out := waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(next)})
			if tt.opener != "" {
				if out.Kind != WaitRespond || len(out.Cards) != 1 || out.Cards[0].Name != tt.opener {
					t.Fatalf("after promote: %+v; want respond with %s", out, tt.opener)
				}
				out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
			}
			if out.Kind != WaitStillWaiting {
				t.Fatalf("continuing before promoting: %+v; want still_waiting", out)
			}

			must(t)(g.Switch(state.P2, state.ActorClaude, 0))
			if g.State().Sides[state.P2].Active.Empty() {
				t.Fatal("p2 did not promote")
			}
			if out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)}); out.Kind != WaitStillWaiting {
				t.Fatalf("after promoting: %+v; want still_waiting", out)
			}
		})
	}
}

// TestWaitRespondJudgeAndUnfairStamp: Judge and Unfair Stamp are reported
// with their id, name and text; in play mode the assistant can then
// shuffle its hand into its deck and draw during the player's turn, even
// after the player closed the action; a continuing wait does not report
// them again, and once the wait is called again the turn lock is back.
func TestWaitRespondJudgeAndUnfairStamp(t *testing.T) {
	m, g, judge, stamp := respondGame(t)
	m.Timing = Timing{WaitCap: 150 * time.Millisecond, CheckIn: time.Minute}

	// The first wait is fresh; the rest carry on from the previous result.
	var since *int
	for _, tt := range []struct {
		iid       state.IID
		name, def string
		text      string
		draws     int
	}{
		{judge, "Judge", "sv01-176", judgeText, 4},
		{stamp, "Unfair Stamp", "sv06-165", unfairStampText, 2},
	} {
		done := startWait(t, context.Background(), m, WaitRequest{Seat: state.P1, Since: since})
		must(t)(g.PlayTrainer(state.P1, state.ActorHuman, tt.iid))
		r := recv(t, done, time.Second)
		if r.err != nil || r.out.Kind != WaitRespond || len(r.out.Cards) != 1 {
			t.Fatalf("after %s: %+v, %v; want respond with one card", tt.name, r.out, r.err)
		}
		want := UsedCard{IID: tt.iid, Name: tt.name, Def: tt.def, Kind: "trainer", Text: tt.text}
		if got := r.out.Cards[0]; got != want {
			t.Fatalf("reported %+v, want %+v", got, want)
		}
		g.EndAction() // the player closes the action before the assistant answers
		shuffleHandAndDraw(t, g, tt.draws)

		out := waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(r.out.Next)})
		if out.Kind != WaitStillWaiting {
			t.Fatalf("continuing after %s: %+v; want still_waiting", tt.name, out)
		}
		since = intp(out.Next)
		if _, err := g.Draw(state.P2, state.ActorClaude, 1); err == nil || !strings.Contains(err.Error(), "it is p1's turn") {
			t.Fatalf("assistant draw after the response closed: %v; want the turn lock", err)
		}
	}
}

// TestResponseWindowOnlyWhileOpen: the assistant's seat cannot act during
// the player's turn before a response is open, nor after the player ends
// the turn; a card with nothing for the assistant to do is reported once.
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
	out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)})
	if out.Kind != WaitRespond || len(out.Cards) != 1 || out.Cards[0].Name != "Ultra Ball" {
		t.Fatalf("after Ultra Ball: %+v; want respond", out)
	}
	// Nothing to do: the assistant waits again straight away.
	if out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)}); out.Kind != WaitStillWaiting {
		t.Fatalf("Ultra Ball reported again: %+v", out)
	}
	refused("after the next wait")

	// A response open when the player ends the turn closes with it.
	judgeLike := cardIn(t, g, state.P1, "tst-4", false)
	if l, _ := g.State().Find(judgeLike); l.Zone != state.Hand {
		must(t)(g.Move(state.P1, state.ActorHuman, judgeLike, state.Loc{Zone: state.Hand}, "", ""))
	}
	must(t)(g.PlayTrainer(state.P1, state.ActorHuman, judgeLike))
	if out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)}); out.Kind != WaitRespond {
		t.Fatalf("after Iono: %+v; want respond", out)
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

	iono := cardIn(t, g, state.P1, "tst-4", false)
	if l, _ := g.State().Find(iono); l.Zone != state.Hand {
		must(t)(g.Move(state.P1, state.ActorHuman, iono, state.Loc{Zone: state.Hand}, "", ""))
	}
	must(t)(g.PlayTrainer(state.P1, state.ActorHuman, iono)) // left open
	if _, err := g.Draw(state.P2, state.ActorClaude, 1); err == nil || !strings.Contains(err.Error(), "it is p1's turn") {
		t.Fatalf("assistant joined the player's open Iono with no response open: %v", err)
	}
	if out = waitNow(t, m, WaitRequest{Seat: state.P1, Since: intp(out.Next)}); out.Kind != WaitRespond {
		t.Fatalf("after Iono: %+v; want respond", out)
	}
	r := must(t)(g.Draw(state.P2, state.ActorClaude, 1))
	if r.ActionID != g.OpenAction() {
		t.Errorf("the assistant's draw (action %s) did not join the open Iono (%s)", r.ActionID, g.OpenAction())
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
