package game

import (
	"fmt"
	"slices"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// AppliedEffect is one effect the playmat applied to the other side as
// part of a card the player used: the opponent's part of a
// shuffle-hand-and-draw card such as Judge or Iono.
type AppliedEffect struct {
	IID    state.IID // the card the player used
	Name   string    // its action label ("Judge")
	Def    string
	Action string // the action id, so a wait reports each once
	What   string // what happened, in seat names
}

// opponentPart finishes an opener's transaction with the opponent's part
// of its effect, when the effect is the shuffle-hand-and-draw family and
// the playmat plays that part itself (see autoOpponentLocked). label names
// the card in the unmet-condition warning. The caller holds g.mu.
func (t *tx) opponentPart(label string) error {
	if t.effect == nil || !t.g.autoOpponentLocked(t.seat, t.actor) {
		return nil
	}
	eff := ReadEffect(*t.effect)
	if eff.Kind != OppApply {
		return nil
	}
	if why := t.g.unmetConditionLocked(t.st, t.seat, eff.Condition); why != "" {
		t.warn = append(t.warn, fmt.Sprintf("%s's condition is not met (%s); its effect was applied anyway", label, why))
	}
	return t.applyOpponent(t.seat.Other(), eff, len(t.st.Sides[t.seat].Hand))
}

// autoOpponentLocked reports whether the playmat carries out the
// opponent's part of seat's card: only for the human's own cards, always
// in play mode (the other side is the assistant's), and in review mode
// only while a turn wait has handed seat to the player.
func (g *Game) autoOpponentLocked(seat state.Seat, actor state.Actor) bool {
	return actor == state.ActorHuman && (g.mode == ModePlay || g.handed == seat)
}

// applyOpponent moves opp's whole hand into its deck (shuffled) or onto
// the bottom of it (in a random order), then draws eff's count for opp.
// playerHandAfter is the player's hand size once the card has left it,
// for draws that depend on either player having put cards on the bottom.
// Its events carry opp's seat and the system actor, inside the card's own
// action, so the card and its effect commit and undo as one.
func (t *tx) applyOpponent(opp state.Seat, eff OppEffect, playerHandAfter int) error {
	actor := t.actor
	t.actor = state.ActorSystem
	defer func() { t.actor = actor }()

	hand := slices.Clone(t.st.Sides[opp].Hand)
	if eff.HandTo == HandToBottom {
		t.g.rng.Shuffle(len(hand), func(i, j int) { hand[i], hand[j] = hand[j], hand[i] })
	}
	for _, iid := range hand {
		from, err := t.st.Find(iid)
		if err != nil {
			return err
		}
		// A deck location with no position is its bottom.
		if err := t.emit(opp, state.EvMove, state.MovePayload{IID: iid, From: &from, To: loc(opp, state.Deck)}); err != nil {
			return err
		}
	}
	if eff.HandTo == HandToDeck {
		if err := t.shuffle(opp); err != nil {
			return err
		}
	}
	n := eff.Draw
	if eff.PerPrize {
		n = t.st.Sides[opp].PrizesLeft()
	}
	switch eff.DrawIf {
	case DrawIfOpponentPut:
		if len(hand) == 0 {
			n = 0
		}
	case DrawIfEitherPut:
		if len(hand) == 0 && playerHandAfter == 0 {
			n = 0
		}
	}
	if n == 0 {
		return nil
	}
	return t.draw(opp, n)
}

// unmetConditionLocked checks a card's use condition for seat, the
// player using it. It returns why the condition is not met, or "" when it
// is met or the playmat cannot check it.
func (g *Game) unmetConditionLocked(st *state.State, seat state.Seat, c Condition) string {
	switch c.Kind {
	case CondKOLastTurn:
		if !g.knockedOutDuringLocked(seat, st.Turn-1) {
			return "none of your Pokémon were Knocked Out during your opponent's last turn"
		}
	case CondOppPrizesAtMost:
		if n := st.Sides[seat.Other()].PrizesLeft(); n > c.N {
			return fmt.Sprintf("your opponent has %d Prize cards remaining, not %d or fewer", n, c.N)
		}
	}
	return ""
}

// knockedOutDuringLocked reports whether any of victim's Pokémon were
// Knocked Out (by a knock_out action) during per-player turn turn on the
// current branch. The caller holds g.mu.
func (g *Game) knockedOutDuringLocked(victim state.Seat, turn int) bool {
	if turn <= 0 {
		return false
	}
	path, err := g.log.PathTo(g.log.Head())
	if err != nil {
		return false
	}
	for i := len(path) - 1; i >= 0; i-- {
		e := &path[i]
		if e.Turn < turn {
			break
		}
		if e.Turn != turn || e.Type != state.EvMove {
			continue
		}
		p, err := e.Decode()
		if err != nil {
			continue
		}
		if mv := p.(*state.MovePayload); g.isKnockoutMove(e, mv) && mv.From.Seat == victim {
			return true
		}
	}
	return false
}

// isKnockoutMove reports whether a move takes a card out of a Pokémon
// stack as part of a knock_out action. The caller holds g.mu.
func (g *Game) isKnockoutMove(e *state.Event, mv *state.MovePayload) bool {
	if mv.From == nil || !mv.From.Zone.IsStack() {
		return false
	}
	a := g.log.Action(e.Action)
	return a != nil && a.Kind == "knockout"
}

// appliedAfter is appliedSince for callers that do not hold g.mu.
func (g *Game) appliedAfter(cursor int, seat state.Seat) []AppliedEffect {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.appliedSince(cursor, seat)
}

// appliedSince lists the effects the playmat applied to seat's opponent
// after cursor, as part of seat's own cards: one entry per card, in order.
// It is nil when cursor is off the current branch. The caller holds g.mu.
func (g *Game) appliedSince(cursor int, seat state.Seat) []AppliedEffect {
	events, ok := g.eventsSince(cursor)
	if !ok {
		return nil
	}
	opp := seat.Other()
	type tally struct {
		hand, drew int
		shuffled   bool
	}
	var order []string
	tallies := map[string]*tally{}
	for i := range events {
		e := &events[i]
		if e.Actor != state.ActorSystem || e.Seat != opp {
			continue
		}
		a := g.actionOf(e.Action)
		if a == nil || a.Seat != seat || a.Actor != state.ActorHuman || !openerKinds[a.Kind] {
			continue
		}
		tl := tallies[a.ID]
		if tl == nil {
			tl = &tally{}
			tallies[a.ID] = tl
			order = append(order, a.ID)
		}
		p, err := e.Decode()
		if err != nil {
			continue
		}
		switch p := p.(type) {
		case *state.MovePayload:
			switch {
			case p.From != nil && p.From.Zone == state.Hand && p.To.Zone == state.Deck:
				tl.hand++
			case p.From != nil && p.From.Zone == state.Deck && p.To.Zone == state.Hand:
				tl.drew++
			}
		case *state.ShufflePayload:
			tl.shuffled = true
		}
	}
	var out []AppliedEffect
	for _, id := range order {
		a, tl := g.actionOf(id), tallies[id]
		var what string
		switch {
		case tl.hand == 0:
			what = fmt.Sprintf("%s had no cards in hand", opp)
		case tl.shuffled:
			what = fmt.Sprintf("%s's hand of %s was shuffled into %s's deck", opp, cardCount(tl.hand), opp)
		default:
			what = fmt.Sprintf("%s's hand of %s was put on the bottom of %s's deck", opp, cardCount(tl.hand), opp)
		}
		if tl.drew == 0 {
			what += fmt.Sprintf("; %s drew no cards", opp)
		} else {
			what += fmt.Sprintf("; %s drew %s", opp, cardCount(tl.drew))
		}
		ae := AppliedEffect{IID: a.Source, Name: a.Label, Action: a.ID, What: what}
		if c := g.st.Cards[a.Source]; c != nil {
			ae.Def = c.Def
		}
		out = append(out, ae)
	}
	return out
}

func cardCount(n int) string {
	if n == 1 {
		return "1 card"
	}
	return fmt.Sprintf("%d cards", n)
}
