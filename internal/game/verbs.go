package game

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// Opening hand size.
const HandSize = 7

func loc(seat state.Seat, z state.Zone) state.Loc { return state.Loc{Seat: seat, Zone: z} }

// inHand checks iid is in seat's hand.
func inHand(st *state.State, seat state.Seat, iid state.IID) error {
	l, err := st.Find(iid)
	if err != nil {
		return err
	}
	if l.Zone != state.Hand || l.Seat != seat {
		return fmt.Errorf("%s (%s) is in %s, not %s's hand", iid, st.Name(iid), l, seat)
	}
	return nil
}

// Setup starts a game: loads both decks in list order, shuffles them and
// draws opening hands.
func (g *Game) Setup(cardsList []state.Card, opts Options) (*Result, error) {
	if opts.BenchSize == 0 {
		opts.BenchSize = 5
	}
	if opts.PrizeCount == 0 {
		opts.PrizeCount = 6
	}
	return g.run("", state.ActorSystem, actionSpec{label: "Setup", kind: "setup", anyTurn: true}, func(t *tx) error {
		if err := t.emit("", state.EvSetup, state.SetupPayload{
			BenchSize: opts.BenchSize, PrizeCount: opts.PrizeCount, Names: opts.Names, Cards: cardsList,
		}); err != nil {
			return err
		}
		for _, seat := range state.Seats {
			if err := t.shuffle(seat); err != nil {
				return err
			}
			if err := t.draw(seat, HandSize); err != nil {
				return err
			}
		}
		return nil
	})
}

// Mulligan shuffles seat's hand back and draws a new one.
func (g *Game) Mulligan(seat state.Seat, actor state.Actor) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Mulligan", kind: "setup"}, func(t *tx) error {
		if t.st.Phase != state.PhaseSetup {
			t.warn = append(t.warn, "mulligan outside setup")
		}
		for _, iid := range slices.Clone(t.st.Sides[seat].Hand) {
			if err := t.move(iid, loc(seat, state.Deck), "", ""); err != nil {
				return err
			}
		}
		if err := t.shuffle(seat); err != nil {
			return err
		}
		return t.draw(seat, HandSize)
	})
}

// PlaceActive puts a Pokémon from hand into an empty Active Spot.
func (g *Game) PlaceActive(seat state.Seat, actor state.Actor, iid state.IID) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Active " + g.nameOf(iid), source: iid}, func(t *tx) error {
		if err := inHand(t.st, seat, iid); err != nil {
			return err
		}
		if !t.st.Sides[seat].Active.Empty() {
			return fmt.Errorf("%s's Active Spot is occupied; use switch or move", seat)
		}
		t.warnNotBasic(iid)
		return t.move(iid, loc(seat, state.Active), state.AsPokemon, "")
	})
}

// Bench puts a Pokémon from hand onto the bench. slot < 0 picks the first
// empty slot.
func (g *Game) Bench(seat state.Seat, actor state.Actor, iid state.IID, slot int) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Bench " + g.nameOf(iid), source: iid, joinIf: "bench"}, func(t *tx) error {
		if err := inHand(t.st, seat, iid); err != nil {
			return err
		}
		bench := t.st.Sides[seat].Bench
		if slot < 0 {
			slot = slices.IndexFunc(bench, func(s *state.Stack) bool { return s.Empty() })
			if slot < 0 {
				return fmt.Errorf("%s's bench is full", seat)
			}
		}
		if slot >= len(bench) {
			return fmt.Errorf("bench slot %d out of range (bench size %d)", slot, len(bench))
		}
		if !bench[slot].Empty() {
			return fmt.Errorf("bench slot %d is occupied", slot)
		}
		t.warnNotBasic(iid)
		return t.move(iid, state.Loc{Seat: seat, Zone: state.Bench, Slot: slot}, state.AsPokemon, "")
	})
}

func (t *tx) warnNotBasic(iid state.IID) {
	if c := t.g.card(t.st, iid); c != nil && !c.IsBasic() {
		t.warn = append(t.warn, fmt.Sprintf("%s is not a Basic Pokémon", c.Name))
	}
}

// StartGame deals prizes to any seat without them and starts turn 1 for
// first, who draws a card.
func (g *Game) StartGame(first state.Seat, actor state.Actor) (*Result, error) {
	return g.run(first, actor, actionSpec{label: "Start game", kind: "turn", anyTurn: true}, func(t *tx) error {
		if t.st.Phase != state.PhaseSetup {
			return fmt.Errorf("the game has already started")
		}
		for _, seat := range state.Seats {
			side := t.st.Sides[seat]
			if side.Active.Empty() {
				t.warn = append(t.warn, fmt.Sprintf("%s has no Active Pokémon", seat))
			}
			if side.PrizesLeft() > 0 {
				continue
			}
			for i := range side.Prizes {
				if len(side.Deck) == 0 {
					return fmt.Errorf("%s's deck ran out while dealing prizes", seat)
				}
				if err := t.emit(seat, state.EvMove, state.MovePayload{
					IID: side.Deck[0], From: &state.Loc{Seat: seat, Zone: state.Deck},
					To: state.Loc{Seat: seat, Zone: state.Prizes, Slot: i}, Face: "down",
				}); err != nil {
					return err
				}
			}
		}
		if err := t.emit(first, state.EvTurn, state.TurnPayload{Phase: "start", Number: 1}); err != nil {
			return err
		}
		return t.draw(first, 1)
	})
}

// EndTurn ends seat's turn and starts the opponent's, who draws a card.
func (g *Game) EndTurn(seat state.Seat, actor state.Actor) (*Result, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	// Even in review mode, where the turn lock is lifted, only the turn
	// in progress can end.
	if g.st.Phase == state.PhasePlaying && g.st.TurnOpen && g.st.Current != seat {
		return nil, fmt.Errorf("it is %s's turn (turn %d), not %s's", g.st.Current, g.st.Turn, seat)
	}
	g.open = nil
	res, err := g.runLocked(seat, actor, actionSpec{label: "End turn", kind: "turn"}, func(t *tx) error {
		return t.emit(seat, state.EvTurn, state.TurnPayload{Phase: "end", Number: t.st.Turn})
	})
	if err != nil {
		return nil, err
	}
	next := seat.Other()
	label := fmt.Sprintf("Turn %d", g.st.Turn+1)
	res2, err := g.runLocked(next, state.ActorSystem, actionSpec{label: label, kind: "turn", anyTurn: true}, func(t *tx) error {
		if err := t.emit(next, state.EvTurn, state.TurnPayload{Phase: "start", Number: t.st.Turn + 1}); err != nil {
			return err
		}
		if len(t.st.Sides[next].Deck) == 0 {
			t.warn = append(t.warn, fmt.Sprintf("%s cannot draw at the start of their turn: deck is empty (they lose)", next))
			return nil
		}
		return t.draw(next, 1)
	})
	if err != nil {
		return nil, err
	}
	res.Events = append(res.Events, res2.Events...)
	res.Delta = append(res.Delta, res2.Delta...)
	res.Warnings = append(res.Warnings, res2.Warnings...)
	g.notify(res)
	return res, nil
}

// Draw draws n cards.
func (g *Game) Draw(seat state.Seat, actor state.Actor, n int) (*Result, error) {
	if n < 1 {
		return nil, fmt.Errorf("draw at least one card")
	}
	return g.run(seat, actor, actionSpec{label: fmt.Sprintf("Draw %d", n), kind: "draw", crossSeat: true}, func(t *tx) error {
		return t.draw(seat, n)
	})
}

// Evolve puts an evolution card from hand onto the Pokémon at target.
func (g *Game) Evolve(seat state.Seat, actor state.Actor, iid state.IID, target state.Loc) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Evolve into " + g.nameOf(iid), kind: "evolve", source: iid, joinIf: "evolv"}, func(t *tx) error {
		if err := inHand(t.st, seat, iid); err != nil {
			return err
		}
		if target.Seat != seat {
			return fmt.Errorf("can only evolve your own Pokémon")
		}
		st, err := t.st.Stack(target)
		if err != nil {
			return err
		}
		if st.Top() == "" {
			return fmt.Errorf("no Pokémon in %s to evolve", target)
		}
		if st.EnteredTurn == t.st.Turn && t.st.Phase == state.PhasePlaying {
			t.warn = append(t.warn, fmt.Sprintf("%s came into play or evolved this turn", t.st.Name(st.Top())))
		}
		if t.st.Turn <= 2 && t.st.Phase == state.PhasePlaying {
			t.warn = append(t.warn, "evolving on a player's first turn")
		}
		if c, base := t.g.card(t.st, iid), t.g.card(t.st, st.Top()); c != nil && base != nil && c.EvolveFrom != "" && c.EvolveFrom != base.Name {
			t.warn = append(t.warn, fmt.Sprintf("%s evolves from %s, not %s", c.Name, c.EvolveFrom, base.Name))
		}
		hadConditions := len(st.Conditions) > 0
		if err := t.move(iid, target, state.AsPokemon, ""); err != nil {
			return err
		}
		if hadConditions {
			return t.emit(seat, state.EvCondition, state.ConditionPayload{Target: target, Condition: "all", On: false})
		}
		return nil
	})
}

// Attach attaches an Energy or Tool from hand. role may be empty to infer it
// from the card.
func (g *Game) Attach(seat state.Seat, actor state.Actor, iid state.IID, target state.Loc, role string) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Attach " + g.nameOf(iid), kind: "attach", source: iid, joinIf: "attach"}, func(t *tx) error {
		if err := inHand(t.st, seat, iid); err != nil {
			return err
		}
		if target.Seat != seat {
			return fmt.Errorf("attach targets your own Pokémon; use move for unusual effects")
		}
		if _, err := t.st.Stack(target); err != nil {
			return err
		}
		if role == "" {
			role = state.AsEnergy
			if c := t.g.card(t.st, iid); c != nil && c.Category == "Trainer" {
				role = state.AsTool
			}
		}
		if role != state.AsEnergy && role != state.AsTool {
			return fmt.Errorf("attach role must be energy or tool")
		}
		if role == state.AsTool {
			if st, _ := t.st.Stack(target); st != nil && len(st.Tools) > 0 {
				t.warn = append(t.warn, fmt.Sprintf("%s already has a Tool", t.st.Name(st.Top())))
			}
		}
		return t.move(iid, target, role, "")
	})
}

// PlayTrainer plays a Trainer from hand and opens a compound action. Items
// and Supporters go to the discard; a Stadium replaces the current one.
func (g *Game) PlayTrainer(seat state.Seat, actor state.Actor, iid state.IID) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: g.nameOf(iid), kind: "trainer", source: iid, opener: true}, func(t *tx) error {
		if err := inHand(t.st, seat, iid); err != nil {
			return err
		}
		c := t.g.card(t.st, iid)
		if c != nil {
			t.effect = &c.Effect
		}
		if c != nil && c.Category != "Trainer" {
			return fmt.Errorf("%s is a %s card, not a Trainer", c.Name, c.Category)
		}
		if c != nil && c.TrainerType == "Tool" {
			return fmt.Errorf("%s is a Pokémon Tool; use attach", c.Name)
		}
		if c != nil && c.TrainerType == "Stadium" {
			if old := t.st.Stadium; old != "" {
				owner := t.st.Cards[old].Owner
				if err := t.move(old, loc(owner, state.Discard), "", ""); err != nil {
					return err
				}
			}
			return t.move(iid, state.Loc{Zone: state.Stadium}, "", "")
		}
		return t.move(iid, loc(seat, state.Discard), "", "")
	})
}

// UseAbility opens a compound action for an Ability of the Pokémon at slot,
// recording that it was used.
func (g *Game) UseAbility(seat state.Seat, actor state.Actor, slot state.Loc, name string) (*Result, error) {
	g.mu.Lock()
	st, err := g.st.Stack(slot)
	g.mu.Unlock()
	if err != nil {
		return nil, err
	}
	if st.Top() == "" {
		return nil, fmt.Errorf("no Pokémon in %s", slot)
	}
	top := st.Top()
	return g.run(seat, actor, actionSpec{label: fmt.Sprintf("%s: %s", g.nameOf(top), name), kind: "ability", source: top, opener: true}, func(t *tx) error {
		if slot.Seat != seat {
			t.warn = append(t.warn, "using an Ability of the opponent's Pokémon")
		}
		if c := t.g.card(t.st, top); c != nil {
			i := slices.IndexFunc(c.Abilities, func(a cards.Ability) bool { return strings.EqualFold(a.Name, name) })
			if i < 0 {
				t.warn = append(t.warn, fmt.Sprintf("%s has no Ability named %q", c.Name, name))
			} else {
				t.effect = &c.Abilities[i].Effect
			}
		}
		return t.emit(seat, state.EvDeclare, state.DeclarePayload{Kind: "ability", Name: name, At: slot})
	})
}

// Retreat discards Energy from the Active Pokémon and swaps it with a
// benched Pokémon.
func (g *Game) Retreat(seat state.Seat, actor state.Actor, to int, energy []state.IID) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Retreat", kind: "retreat", joinIf: "retreat"}, func(t *tx) error {
		active := loc(seat, state.Active)
		st, err := t.st.Stack(active)
		if err != nil {
			return err
		}
		if st.Top() == "" {
			return fmt.Errorf("no Active Pokémon")
		}
		for _, e := range energy {
			if !slices.Contains(st.Energy, e) {
				return fmt.Errorf("%s is not attached to the Active Pokémon", e)
			}
			if err := t.move(e, loc(seat, state.Discard), "", ""); err != nil {
				return err
			}
		}
		if c := t.g.card(t.st, st.Top()); c != nil && c.Retreat != len(energy) {
			t.warn = append(t.warn, fmt.Sprintf("%s's Retreat Cost is %d; %d Energy discarded", c.Name, c.Retreat, len(energy)))
		}
		if st.HasCondition("asleep") || st.HasCondition("paralyzed") {
			t.warn = append(t.warn, "an Asleep or Paralyzed Pokémon cannot retreat")
		}
		return t.switchActive(seat, to)
	})
}

func (t *tx) switchActive(seat state.Seat, to int) error {
	active := loc(seat, state.Active)
	bench := state.Loc{Seat: seat, Zone: state.Bench, Slot: to}
	b, err := t.st.Stack(bench)
	if err != nil {
		return err
	}
	if b.Top() == "" {
		return fmt.Errorf("no Pokémon in %s", bench)
	}
	a, _ := t.st.Stack(active)
	if a != nil && len(a.Conditions) > 0 {
		if err := t.emit(t.seat, state.EvCondition, state.ConditionPayload{Target: active, Condition: "all"}); err != nil {
			return err
		}
	}
	return t.emit(t.seat, state.EvSwap, state.SwapPayload{A: bench, B: active})
}

// Switch swaps seat's Active Pokémon with the one on bench slot to. It also
// promotes a benched Pokémon into an empty Active Spot, which is allowed on
// either player's turn.
func (g *Game) Switch(seat state.Seat, actor state.Actor, to int) (*Result, error) {
	g.mu.Lock()
	promote := g.st.Sides[seat] != nil && g.st.Sides[seat].Active.Empty()
	g.mu.Unlock()
	label := "Switch"
	if promote {
		label = "Promote"
	}
	return g.run(seat, actor, actionSpec{label: label, kind: "switch", anyTurn: promote, crossSeat: true}, func(t *tx) error {
		return t.switchActive(seat, to)
	})
}

// Attack opens a compound action for an attack and puts damage on target.
func (g *Game) Attack(seat state.Seat, actor state.Actor, name string, target state.Loc, damage int) (*Result, error) {
	g.mu.Lock()
	var attacker state.IID
	if side := g.st.Sides[seat]; side != nil {
		attacker = side.Active.Top()
	}
	g.mu.Unlock()
	label := fmt.Sprintf("%s: %s", g.nameOf(attacker), name)
	return g.run(seat, actor, actionSpec{label: label, kind: "attack", source: attacker, opener: true}, func(t *tx) error {
		if attacker == "" {
			return fmt.Errorf("no Active Pokémon to attack with")
		}
		if t.st.Turn == 1 {
			t.warn = append(t.warn, "the player going first cannot attack on their first turn")
		}
		if c := t.g.card(t.st, attacker); c != nil {
			i := slices.IndexFunc(c.Attacks, func(a cards.Attack) bool { return strings.EqualFold(a.Name, name) })
			if i < 0 {
				t.warn = append(t.warn, fmt.Sprintf("%s has no attack named %q", c.Name, name))
			} else {
				t.effect = &c.Attacks[i].Effect
			}
		}
		if err := t.emit(seat, state.EvDeclare, state.DeclarePayload{Kind: "attack", Name: name, At: loc(seat, state.Active)}); err != nil {
			return err
		}
		if damage == 0 {
			return nil
		}
		return t.addDamage(target, damage)
	})
}

func (t *tx) addDamage(target state.Loc, amount int) error {
	st, err := t.st.Stack(target)
	if err != nil {
		return err
	}
	if st.Top() == "" {
		return fmt.Errorf("no Pokémon in %s", target)
	}
	v := max(st.Damage+amount, 0)
	if err := t.emit(t.seat, state.EvDamage, state.DamagePayload{Target: target, Value: v}); err != nil {
		return err
	}
	if c := t.g.card(t.st, st.Top()); c != nil && c.HP > 0 && v >= c.HP {
		t.warn = append(t.warn, fmt.Sprintf("%s in %s has %d damage and %d HP: knock it out", c.Name, target, v, c.HP))
	}
	return nil
}

// PlaceDamage adds damage (negative heals) to the Pokémon at target.
func (g *Game) PlaceDamage(seat state.Seat, actor state.Actor, target state.Loc, amount int) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: fmt.Sprintf("Damage %+d on %s", amount, target), kind: "damage"}, func(t *tx) error {
		return t.addDamage(target, amount)
	})
}

// SetCondition adds or clears a Special Condition.
func (g *Game) SetCondition(seat state.Seat, actor state.Actor, target state.Loc, cond string, on bool) (*Result, error) {
	verb := "Clear"
	if on {
		verb = "Set"
	}
	return g.run(seat, actor, actionSpec{label: fmt.Sprintf("%s %s on %s", verb, cond, target), kind: "condition"}, func(t *tx) error {
		if on && target.Zone != state.Active {
			t.warn = append(t.warn, "only an Active Pokémon can have Special Conditions")
		}
		return t.emit(seat, state.EvCondition, state.ConditionPayload{Target: target, Condition: cond, On: on})
	})
}

// KnockOut moves every card of the Pokémon at target to its owner's discard.
func (g *Game) KnockOut(seat state.Seat, actor state.Actor, target state.Loc) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Knock Out " + target.String(), kind: "knockout"}, func(t *tx) error {
		st, err := t.st.Stack(target)
		if err != nil {
			return err
		}
		if st.Empty() {
			return fmt.Errorf("no Pokémon in %s", target)
		}
		for _, iid := range st.Cards() {
			if err := t.move(iid, loc(target.Seat, state.Discard), "", ""); err != nil {
				return err
			}
		}
		return nil
	})
}

// TakePrize moves a prize card to seat's hand. slot < 0 takes the first
// remaining prize.
func (g *Game) TakePrize(seat state.Seat, actor state.Actor, slot int) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Take prize", kind: "prize"}, func(t *tx) error {
		prizes := t.st.Sides[seat].Prizes
		if slot < 0 {
			slot = slices.IndexFunc(prizes, func(p state.IID) bool { return p != "" })
			if slot < 0 {
				return fmt.Errorf("%s has no prizes left", seat)
			}
		}
		if slot >= len(prizes) || prizes[slot] == "" {
			return fmt.Errorf("no prize in slot %d", slot)
		}
		if err := t.move(prizes[slot], loc(seat, state.Hand), "", ""); err != nil {
			return err
		}
		if t.st.Sides[seat].PrizesLeft() == 0 {
			t.warn = append(t.warn, fmt.Sprintf("%s has taken their last prize card and wins", seat))
		}
		return nil
	})
}

// Shuffle shuffles whose deck.
func (g *Game) Shuffle(seat state.Seat, actor state.Actor, whose state.Seat) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Shuffle " + string(whose) + " deck", kind: "shuffle", crossSeat: true}, func(t *tx) error {
		return t.shuffle(whose)
	})
}

// CoinFlip flips n coins. It is allowed at any time, e.g. to decide who goes
// first.
func (g *Game) CoinFlip(seat state.Seat, actor state.Actor, n int) (*Result, error) {
	if n < 1 {
		n = 1
	}
	return g.run(seat, actor, actionSpec{label: fmt.Sprintf("Flip %d", n), kind: "coin", anyTurn: true}, func(t *tx) error {
		res := make([]string, n)
		for i := range res {
			res[i] = "tails"
			if t.g.rng.IntN(2) == 0 {
				res[i] = "heads"
			}
		}
		t.data = res
		return t.emit(seat, state.EvCoin, state.CoinPayload{Results: res})
	})
}

// Move is the escape hatch: any card to any zone.
func (g *Game) Move(seat state.Seat, actor state.Actor, iid state.IID, to state.Loc, face, as string) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Move " + g.nameOf(iid), kind: "move", source: iid, crossSeat: true}, func(t *tx) error {
		c, ok := t.st.Cards[iid]
		if !ok {
			return fmt.Errorf("no card %s", iid)
		}
		if to.Zone != state.Stadium {
			to.Seat = c.Owner
		}
		if as == "" && to.Zone.IsStack() {
			if st, _ := t.st.Stack(to); !st.Empty() {
				if cc := t.g.card(t.st, iid); cc != nil {
					switch cc.Category {
					case "Energy":
						as = state.AsEnergy
					case "Trainer":
						as = state.AsTool
					case "Pokemon":
						as = state.AsPokemon
					}
				}
			}
		}
		return t.move(iid, to, as, face)
	})
}

// Reveal shows cards to both players.
func (g *Game) Reveal(seat state.Seat, actor state.Actor, iids []state.IID) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Reveal", kind: "reveal", anyTurn: true}, func(t *tx) error {
		return t.emit(seat, state.EvReveal, state.RevealPayload{IIDs: iids, To: state.Seats})
	})
}

// AddEffect records a lingering effect on target seat.
func (g *Game) AddEffect(seat state.Seat, actor state.Actor, text string, target state.Seat, source state.IID, untilTurn int) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Effect: " + text, kind: "effect"}, func(t *tx) error {
		id := fmt.Sprintf("e-%d", t.g.log.NextID())
		t.data = id
		return t.emit(seat, state.EvEffect, state.EffectPayload{Op: "add", Effect: state.Effect{
			ID: id, Text: text, Seat: target, Source: source, UntilTurn: untilTurn,
		}})
	})
}

// RemoveEffect removes a lingering effect.
func (g *Game) RemoveEffect(seat state.Seat, actor state.Actor, id string) (*Result, error) {
	return g.run(seat, actor, actionSpec{label: "Remove effect " + id, kind: "effect", anyTurn: true}, func(t *tx) error {
		return t.emit(seat, state.EvEffect, state.EffectPayload{Op: "remove", Effect: state.Effect{ID: id}})
	})
}

func (g *Game) nameOf(iid state.IID) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.st == nil {
		return string(iid)
	}
	return g.st.Name(iid)
}

// PrefetchCards loads every card definition in the game into the cache.
func (g *Game) PrefetchCards(ctx context.Context) []error {
	g.mu.Lock()
	defs := map[string]bool{}
	for _, c := range g.st.Cards {
		if c.Def != "" {
			defs[c.Def] = true
		}
	}
	g.mu.Unlock()
	var errs []error
	for d := range defs {
		if g.db == nil {
			break
		}
		if _, err := g.db.Card(ctx, d); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}
