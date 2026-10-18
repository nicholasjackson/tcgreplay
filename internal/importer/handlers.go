package importer

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// buildHandlers compiles the line table for the two player names. Order
// matters: specific patterns come before general ones.
func buildHandlers(a, b string) []handler {
	P := "(" + regexp.QuoteMeta(a) + "|" + regexp.QuoteMeta(b) + ")"
	h := func(pat string, fn func(r *replay, m []string, n, parent *Node) error) handler {
		pat = strings.ReplaceAll(pat, "{P}", P)
		return handler{re: regexp.MustCompile("^" + pat + "$"), fn: fn}
	}
	keep := func(x handler) handler { x.keepKO = true; return x }
	noop := func(r *replay, m []string, n, parent *Node) error { return nil }

	return []handler{
		// Setup.
		h(`{P} chose (heads|tails) for the opening coin flip\.`, noop),
		h(`{P} won the coin toss\.`, func(r *replay, m []string, n, _ *Node) error {
			r.rep.CoinTossWinner = m[1]
			return nil
		}),
		h(`{P} decided to go (first|second)\.`, noop),
		h(`{P} drew (\d+) cards for the opening hand\.`, func(r *replay, m []string, n, _ *Node) error {
			var names []string
			for _, c := range n.Children {
				if b := c.Bullets(); len(b) > 0 {
					names = b
				}
			}
			r.consumed = n // the "- 7 drawn cards." subline is consumed here
			if mull := r.mulligans[m[1]]; len(names) == 0 && len(mull) > 0 {
				names = mull[0] // the first mulligan reveals this hand
			}
			return r.draw(r.seat(m[1]), count(m[2]), names)
		}),
		h(`{P} took (a|\d+) mulligans?\.`, noop),
		h(`Cards revealed from Mulligan (\d+)`, func(r *replay, m []string, n, parent *Node) error {
			pm := regexp.MustCompile(`^(.+) took (a|\d+) mulligans?\.$`).FindStringSubmatch(parent.Text)
			if pm == nil {
				return fmt.Errorf("mulligan reveal outside a mulligan line")
			}
			seat := r.seat(pm[1])
			r.rep.Mulligans[pm[1]]++
			hand := slices.Clone(r.st.Sides[seat].Hand)
			if err := r.bindGroup(hand, n.Bullets()); err != nil {
				return err
			}
			if err := r.emit(seat, state.EvReveal, state.RevealPayload{IIDs: hand, To: state.Seats}); err != nil {
				return err
			}
			for _, iid := range hand {
				if err := r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Deck}, "", ""); err != nil {
					return err
				}
			}
			if err := r.shuffle(seat); err != nil {
				return err
			}
			var next []string
			if k := count(m[1]); k < len(r.mulligans[pm[1]]) {
				next = r.mulligans[pm[1]][k]
			}
			return r.draw(seat, 7, next)
		}),
		h(`{P} drew (\d+) more cards? because {P} took at least \d+ mulligans?\.`, noop),

		// Putting cards into play.
		h(`{P} played (.+) to the Active Spot\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			iid, err := r.take(seat, []state.Zone{state.Hand}, m[2])
			if err != nil {
				return err
			}
			return r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Active}, state.AsPokemon, "")
		}),
		h(`{P} played (.+) to the Bench\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			iid, err := r.take(seat, []state.Zone{state.Hand}, m[2])
			if err != nil {
				return err
			}
			return r.toBench(seat, iid)
		}),
		h(`{P} played (.+) to the Stadium spot\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			iid, err := r.take(seat, []state.Zone{state.Hand}, m[2])
			if err != nil {
				return err
			}
			if old := r.st.Stadium; old != "" {
				owner := r.st.Cards[old].Owner
				if err := r.move(seat, old, state.Loc{Seat: owner, Zone: state.Discard}, "", ""); err != nil {
					return err
				}
				r.lastStadium = old
			}
			return r.move(seat, iid, state.Loc{Zone: state.Stadium}, "", "")
		}),
		h(`{P} drew (\d+) cards and played them to the Bench\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			names := n.Bullets()
			for i := 0; i < count(m[2]); i++ {
				name := ""
				if i < len(names) {
					name = names[i]
				}
				iid, err := r.take(seat, []state.Zone{state.Deck}, name)
				if err != nil {
					return err
				}
				if err := r.toBench(seat, iid); err != nil {
					return err
				}
			}
			return nil
		}),

		h(`{P} drew (.+) and played it to the Bench\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			iid, err := r.take(seat, []state.Zone{state.Deck}, m[2])
			if err != nil {
				return err
			}
			return r.toBench(seat, iid)
		}),

		// Drawing.
		h(`{P} drew a card\.`, func(r *replay, m []string, n, _ *Node) error {
			return r.draw(r.seat(m[1]), 1, nil)
		}),
		h(`{P} drew (\d+) cards\.`, func(r *replay, m []string, n, _ *Node) error {
			return r.drawShown(r.seat(m[1]), count(m[2]), n.Bullets())
		}),
		h(`(\d+) drawn cards\.`, noop),
		h(`{P} drew (.+)\.`, func(r *replay, m []string, n, _ *Node) error {
			return r.drawShown(r.seat(m[1]), 1, []string{m[2]})
		}),

		// Trainers, attachments and evolution.
		h(`{P} played (.+)\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			// "played Abra." is an in-play Pokémon's effect, not a card
			// leaving the hand.
			if l, err := r.stackNamed(seat, m[2], ""); err == nil {
				r.lastRef = &l
				return nil
			}
			iid, err := r.take(seat, []state.Zone{state.Hand}, m[2])
			if err != nil {
				return err
			}
			return r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Discard}, "", "")
		}),
		h(`{P} attached (.+) to (.+) (in the Active Spot|on the Bench)\.`, func(r *replay, m []string, n, parent *Node) error {
			seat := r.seat(m[1])
			role := state.AsTool
			if strings.Contains(m[2], "Energy") {
				role = state.AsEnergy
			}
			r.adding = []string{m[2]}
			r.toolSlot = role == state.AsTool
			l, err := r.stackNamed(seat, m[3], zoneHint(m[4]))
			r.adding, r.toolSlot = nil, false
			if err != nil {
				return err
			}
			zones := []state.Zone{state.Hand}
			if n.Depth == 1 {
				zones = []state.Zone{state.Discard, state.Hand, state.Deck}
				if deckish(parent) {
					zones = []state.Zone{state.Deck, state.Discard, state.Hand}
				}
			}
			iid, err := r.take(seat, zones, m[2])
			if err != nil {
				return err
			}
			return r.move(seat, iid, l, role, "")
		}),
		h(`{P} evolved (.+) to (.+) (in the Active Spot|on the Bench)\.`, func(r *replay, m []string, n, parent *Node) error {
			seat := r.seat(m[1])
			l, err := r.stackNamed(seat, m[2], zoneHint(m[4]))
			if err != nil {
				return err
			}
			zones := []state.Zone{state.Hand}
			if n.Depth == 1 {
				zones = []state.Zone{state.Hand, state.Deck}
				if deckish(parent) {
					zones = []state.Zone{state.Deck, state.Hand}
				}
			}
			iid, err := r.take(seat, zones, m[3])
			if err != nil {
				return err
			}
			st, _ := r.st.Stack(l)
			hadConditions := len(st.Conditions) > 0
			if err := r.move(seat, iid, l, state.AsPokemon, ""); err != nil {
				return err
			}
			if hadConditions {
				return r.emit(seat, state.EvCondition, state.ConditionPayload{Target: l, Condition: "all"})
			}
			return nil
		}),
		h(`{P} devolved {P}'s (.+) to (.+) (in the Active Spot|on the Bench)\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[2])
			l, err := r.stackNamed(seat, m[3], zoneHint(m[5]))
			if err != nil {
				return err
			}
			// Devolving can strip several stages (Strange Timepiece returns
			// every Evolution card), but the log names only the top one, so
			// peel cards off until the named stage is on top.
			st, _ := r.st.Stack(l)
			top := st.Top()
			for {
				st, _ = r.st.Stack(l)
				if len(st.Pokemon) < 2 || (st.Top() != top && r.bound[st.Top()] == m[4]) {
					break
				}
				if err := r.move(r.seat(m[1]), st.Top(), state.Loc{Seat: seat, Zone: state.Hand}, "", ""); err != nil {
					return err
				}
			}
			r.lastDevolved = top
			return nil
		}),

		// Moving between Active and Bench.
		h(`{P} retreated (.+) to the Bench\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			if _, err := r.stackNamed(seat, m[2], "active"); err != nil {
				return err
			}
			r.pendingRetreat[seat] = true
			return nil
		}),
		h(`{P}'s (.+) is now in the Active Spot\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			r.pendingRetreat[seat] = false
			if _, err := r.stackNamed(seat, m[2], "active"); err == nil {
				return nil // already there, e.g. after a switch subline
			}
			b, err := r.stackNamed(seat, m[2], "bench")
			if err != nil {
				return err
			}
			return r.toActive(seat, b)
		}),
		h(`{P}'s (.+) was switched with {P}'s (.+) to become the Active Pokémon\.`, func(r *replay, m []string, n, _ *Node) error {
			// The first-named Pokémon is normally the one coming in, but
			// take whichever named Pokémon is on the Bench.
			seat := r.seat(m[3])
			b, err := r.stackNamed(seat, m[2], "bench")
			if err != nil {
				if b, err = r.stackNamed(seat, m[4], "bench"); err != nil {
					return err
				}
			}
			return r.toActive(seat, b)
		}),

		// Attacks, abilities and damage.
		h(`{P}'s (.+?) used (.+?) on {P}'s (.+?) for (\d+) damage\..*`, func(r *replay, m []string, n, _ *Node) error {
			actor := r.seat(m[1])
			l, err := r.target(state.Seat(""), r.seat(m[4]), m[5])
			if err != nil {
				return err
			}
			dmg := count(m[6])
			if dmg > 0 {
				if err := r.addDamage(actor, l, dmg); err != nil {
					return err
				}
			}
			r.checkToolCredits(n, actor, l)
			return nil
		}),
		h(`{P}'s (.+?) used (.+?)\.`, noop),
		h(`{P} put (an?|\d+) damage counters? on {P}'s (.+)\.`, func(r *replay, m []string, n, _ *Node) error {
			actor := r.seat(m[1])
			l, err := r.target(actor, r.seat(m[3]), m[4])
			if err != nil {
				return err
			}
			return r.addDamage(actor, l, 10*count(m[2]))
		}),
		h(`{P} moved (a|\d+) damage counters? from {P}'s (.+) to {P}'s (.+)\.`, func(r *replay, m []string, n, _ *Node) error {
			actor := r.seat(m[1])
			amount := 10 * count(m[2])
			// Only a copy carrying that much damage can give it up, which
			// settles identical copies the log can't tell apart.
			var hurt []state.Loc
			for _, l := range r.st.StackLocs(r.seat(m[3])) {
				if st, _ := r.st.Stack(l); r.bound[st.Top()] == m[4] && st.Damage >= amount {
					hurt = append(hurt, l)
				}
			}
			var from state.Loc
			if len(hurt) == 1 {
				from = hurt[0]
			} else {
				var err error
				if from, err = r.target(state.Seat(""), r.seat(m[3]), m[4]); err != nil {
					return err
				}
			}
			to, err := r.target(actor, r.seat(m[5]), m[6])
			if err != nil {
				return err
			}
			// The log moved counters the board never put there, so lines
			// that placed them earlier were missed; the totals still come out
			// right once they are moved off.
			if st, err := r.st.Stack(from); err == nil && st.Damage < amount {
				r.rep.Warnings = append(r.rep.Warnings, fmt.Sprintf("line %d: moved %d damage from %s, which had only %d; earlier damage on it was not imported", n.No, amount, m[4], st.Damage))
			}
			if err := r.addDamage(actor, from, -amount); err != nil {
				return err
			}
			return r.addDamage(actor, to, amount)
		}),
		h(`{P}'s (.+) took (\d+) damage\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			l, err := r.target(state.Seat(""), seat, m[2])
			if err != nil {
				return err
			}
			return r.addDamage(seat, l, count(m[3]))
		}),
		h(`{P}'s (.+) healed (\d+) damage\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			// Only a copy carrying at least that much damage can heal it,
			// which settles identical copies the log can't tell apart.
			var hurt []state.Loc
			for _, l := range r.st.StackLocs(seat) {
				if st, _ := r.st.Stack(l); r.bound[st.Top()] == m[2] && st.Damage >= count(m[3]) {
					hurt = append(hurt, l)
				}
			}
			l := state.Loc{}
			if len(hurt) == 1 {
				l = hurt[0]
			} else {
				var err error
				if l, err = r.stackNamed(seat, m[2], ""); err != nil {
					return err
				}
			}
			r.lastRef = &l
			return r.addDamage(seat, l, -count(m[3]))
		}),
		h(`(an?|\d+) damage counters? (?:was|were) placed on {P}'s (.+) for the Special Condition (\w+)\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[2])
			l, err := r.stackNamed(seat, m[3], "")
			if err != nil {
				return err
			}
			return r.addDamage(seat, l, 10*count(m[1]))
		}),
		// Counters an Ability or effect places between turns, such as
		// Freezing Shroud at Pokémon Checkup.
		h(`(an?|\d+) damage counters? (?:was|were) placed on {P}'s (.+?) (?:for|by|because of|due to) .+\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[2])
			l, err := r.stackNamed(seat, m[3], "")
			if err != nil {
				return err
			}
			return r.addDamage(seat, l, 10*count(m[1]))
		}),
		h(`{P}'s (.+) is now (Asleep|Burned|Confused|Paralyzed|Poisoned)\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			l, err := r.stackNamed(seat, m[2], "active")
			if err != nil {
				return err
			}
			cond := strings.ToLower(m[3])
			return r.emit(seat, state.EvCondition, state.ConditionPayload{Target: l, Condition: cond, On: true})
		}),

		// Knockouts and prizes.
		keep(h(`{P}'s (.+) was Knocked Out!`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			l, err := r.stackNamed(seat, m[2], "")
			if err != nil {
				return err
			}
			st, _ := r.st.Stack(l)
			cards := st.Cards()
			for _, iid := range cards {
				if err := r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Discard}, "", ""); err != nil {
					return err
				}
			}
			r.lastKO = &koInfo{seat: seat, name: m[2], iids: cards, top: st.Top()}
			return nil
		})),
		keep(h(`(\d+) cards were discarded from {P}'s (.+)\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[2])
			return r.discardFrom(seat, m[3], n.Bullets(), count(m[1]))
		})),
		keep(h(`(.+) was discarded from {P}'s (.+)\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[2])
			return r.discardFrom(seat, m[3], []string{m[1]}, 1)
		})),
		keep(h(`{P} took (a|\d+) Prize cards?\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			for i := 0; i < count(m[2]); i++ {
				side := r.st.Sides[seat]
				slot := slices.IndexFunc(side.Prizes, func(p state.IID) bool { return p != "" })
				if slot < 0 {
					return fmt.Errorf("%s has no prizes left", m[1])
				}
				iid := side.Prizes[slot]
				r.blind = true
				err := r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Hand}, "", "")
				r.blind = false
				if err != nil {
					return err
				}
				r.pendingPrize[seat] = append(r.pendingPrize[seat], iid)
			}
			return nil
		})),
		keep(h(`A card was added to {P}'s hand\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			if len(r.pendingPrize[seat]) > 0 {
				r.pendingPrize[seat] = r.pendingPrize[seat][1:]
				return nil
			}
			iid, err := r.take(seat, []state.Zone{state.Deck}, "")
			if err != nil {
				return err
			}
			r.blind = true
			defer func() { r.blind = false }()
			return r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Hand}, "", "")
		})),
		keep(h(`(.+) was added to {P}'s hand\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[2])
			if p := r.pendingPrize[seat]; len(p) > 0 {
				// Bind whichever taken prize can be this card.
				for i, iid := range p {
					if r.bindable(iid, m[1]) {
						r.pendingPrize[seat] = slices.Delete(slices.Clone(p), i, i+1)
						return r.bind(iid, m[1])
					}
				}
				if d, ok := r.repairAmong(seat, p, m[1]); ok {
					p = r.pendingPrize[seat]
					i := slices.Index(p, d)
					r.pendingPrize[seat] = slices.Delete(slices.Clone(p), i, i+1)
					return r.bind(d, m[1])
				}
				return fmt.Errorf("no taken prize can be %s", m[1])
			}
			iid, err := r.take(seat, []state.Zone{state.Deck}, m[1])
			if err != nil {
				return err
			}
			if err := r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Hand}, "", ""); err != nil {
				return err
			}
			return r.emit(seat, state.EvReveal, state.RevealPayload{IIDs: []state.IID{iid}, To: state.Seats})
		})),

		// Hand and deck manipulation.
		h(`{P} discarded (\d+) cards\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			return r.handTo(seat, count(m[2]), n.Bullets(), state.Loc{Seat: seat, Zone: state.Discard})
		}),
		h(`{P} discarded (.+)\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			if r.lastStadium != "" && r.bound[r.lastStadium] == m[2] && r.st.Cards[r.lastStadium].Owner == seat {
				r.lastStadium = "" // the replaced Stadium, already discarded
				return nil
			}
			iid, err := r.take(seat, []state.Zone{state.Hand, state.Deck}, m[2])
			if err != nil {
				return err
			}
			return r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Discard}, "", "")
		}),
		h(`{P} shuffled their deck\.`, func(r *replay, m []string, n, _ *Node) error {
			return r.shuffle(r.seat(m[1]))
		}),
		h(`{P} shuffled their hand\.`, noop),
		h(`{P} shuffled (a|\d+) cards? into their deck\.`, func(r *replay, m []string, n, parent *Node) error {
			seat := r.seat(m[1])
			if parent != nil && shuffleFromDiscardRe.MatchString(parent.Text) {
				if err := r.zoneTo(seat, state.Discard, count(m[2]), n.Bullets(), state.Loc{Seat: seat, Zone: state.Deck}); err != nil {
					return err
				}
				return r.shuffle(seat)
			}
			// An in-play Pokémon shuffling itself away (Dudunsparce's Run
			// Away Draw) lists its evolution chain; take the whole stack.
			if l := r.lastRef; l != nil && l.Seat == seat && r.stackIs(*l, n.Bullets()) {
				st, _ := r.st.Stack(*l)
				for _, iid := range st.Cards() {
					if err := r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Deck}, "", ""); err != nil {
						return err
					}
				}
				return r.shuffle(seat)
			}
			if err := r.handTo(seat, count(m[2]), n.Bullets(), state.Loc{Seat: seat, Zone: state.Deck}); err != nil {
				return err
			}
			return r.shuffle(seat)
		}),
		h(`{P} shuffled (.+) into their deck\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			deck := state.Loc{Seat: seat, Zone: state.Deck}
			if l, err := r.stackNamed(seat, m[2], ""); err == nil {
				st, _ := r.st.Stack(l)
				for _, iid := range st.Cards() {
					if err := r.move(seat, iid, deck, "", ""); err != nil {
						return err
					}
				}
				return r.shuffle(seat)
			}
			iid, err := r.boundIn(seat, state.Discard, m[2])
			if err != nil {
				if iid, err = r.take(seat, []state.Zone{state.Hand}, m[2]); err != nil {
					return err
				}
			}
			if err := r.move(seat, iid, deck, "", ""); err != nil {
				return err
			}
			return r.shuffle(seat)
		}),
		h(`{P} put (\d+) cards? on the bottom of their deck\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			bottom := state.Bottom
			return r.handTo(seat, count(m[2]), n.Bullets(), state.Loc{Seat: seat, Zone: state.Deck, Pos: &bottom})
		}),
		h(`{P} revealed (\d+) cards?\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[1])
			var iids []state.IID
			for _, name := range n.Bullets() {
				iid, err := r.pick(seat, []state.Zone{state.Hand}, name, iids)
				if err != nil {
					return err
				}
				if err := r.bind(iid, name); err != nil {
					return err
				}
				iids = append(iids, iid)
			}
			return r.emit(seat, state.EvReveal, state.RevealPayload{IIDs: iids, To: state.Seats})
		}),
		h(`{P} moved {P}'s (\d+) cards to their hand\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[2])
			names := n.Bullets()
			// Returning a Pokémon with its attachments (Tuck Tail) or the
			// Energy attached to one (Wally's Compassion).
			if l, ok := r.stackHolding(seat, names); ok {
				st, _ := r.st.Stack(l)
				group := slices.Clone(st.Cards())
				var chosen []state.IID
				for _, name := range names {
					i := slices.IndexFunc(group, func(x state.IID) bool { return r.bound[x] == name })
					chosen = append(chosen, group[i])
					group = slices.Delete(group, i, i+1)
				}
				for _, iid := range chosen {
					if err := r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Hand}, "", ""); err != nil {
						return err
					}
				}
				return nil
			}
			for _, name := range names {
				iid, err := r.take(seat, []state.Zone{state.Discard, state.Deck}, name)
				if err != nil {
					return err
				}
				if err := r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Hand}, "", ""); err != nil {
					return err
				}
			}
			return nil
		}),
		h(`{P} moved {P}'s (.+) to their hand\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[2])
			if d := r.lastDevolved; d != "" && r.bound[d] == m[3] {
				r.lastDevolved = ""
				return nil // already returned by the devolve line
			}
			if l := r.lastRef; l != nil && l.Seat == seat {
				st, _ := r.st.Stack(*l)
				for _, iid := range st.Cards() {
					if iid != st.Top() && r.bound[iid] == m[3] {
						return r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Hand}, "", "")
					}
				}
			}
			return r.moveNamed(seat, m[3], state.Loc{Seat: seat, Zone: state.Hand},
				[]state.Zone{state.Discard}, true, []state.Zone{state.Deck})
		}),
		h(`{P} moved {P}'s (.+) to the Bench\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[2])
			iid, err := r.take(seat, []state.Zone{state.Deck, state.Discard, state.Hand}, m[3])
			if err != nil {
				return err
			}
			return r.toBench(seat, iid)
		}),
		h(`{P} moved {P}'s (.+) to the discard pile\.`, func(r *replay, m []string, n, _ *Node) error {
			seat := r.seat(m[2])
			return r.moveNamed(seat, m[3], state.Loc{Seat: seat, Zone: state.Discard},
				nil, true, []state.Zone{state.Hand, state.Deck})
		}),

		// Coins, choices and triggers.
		h(`{P} flipped a coin and it landed on (heads|tails)\.`, func(r *replay, m []string, n, _ *Node) error {
			return r.emit(r.seat(m[1]), state.EvCoin, state.CoinPayload{Results: []string{m[2]}})
		}),
		h(`{P} chose (.+)`, noop),
		h(`(.+) was activated\.`, noop),
		h(`Damage breakdown:`, noop),
		h(`{P} ended their turn\.`, func(r *replay, m []string, n, _ *Node) error {
			return r.endTurn()
		}),

		// Game end.
		h(`(All Prize cards taken|Opponent took all of their Prize cards|Opponent conceded|You conceded|Knocked Out with no Benched Pokémon)\. {P} wins\.`, func(r *replay, m []string, n, _ *Node) error {
			r.rep.Winner = m[2]
			r.rep.EndReason = map[string]string{
				"All Prize cards taken":                  "all_prizes_taken",
				"Opponent took all of their Prize cards": "all_prizes_taken",
				"Opponent conceded":                      "opponent_conceded",
				"You conceded":                           "self_conceded",
				"Knocked Out with no Benched Pokémon":    "bench_out",
			}[m[1]]
			return nil
		}),
	}
}

// toBench puts a card from hand or deck into the first empty bench slot.
func (r *replay) toBench(seat state.Seat, iid state.IID) error {
	slot := slices.IndexFunc(r.st.Sides[seat].Bench, func(s *state.Stack) bool { return s.Empty() })
	if slot < 0 {
		return fmt.Errorf("%s's bench is full", r.players[seat])
	}
	return r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Bench, Slot: slot}, state.AsPokemon, "")
}

// bindGroup binds a set of cards to a list of names, in any order.
func (r *replay) bindGroup(iids []state.IID, names []string) error {
	free := slices.Clone(iids)
	var pending []string
	for _, name := range names {
		i := slices.IndexFunc(free, func(x state.IID) bool { return r.bound[x] == name })
		if i < 0 {
			pending = append(pending, name)
			continue
		}
		free = slices.Delete(free, i, i+1)
	}
	for _, name := range pending {
		i := slices.IndexFunc(free, func(x state.IID) bool { return r.bound[x] == "" && r.bindable(x, name) })
		if i < 0 {
			return fmt.Errorf("cannot match %s among the revealed cards", name)
		}
		if err := r.bind(free[i], name); err != nil {
			return err
		}
		free = slices.Delete(free, i, i+1)
	}
	return nil
}

// handTo moves n cards from hand to dest, binding names when listed.
// shuffleFromDiscardRe matches cards whose "shuffled N cards into their
// deck" subline takes from the discard pile rather than the hand.
var shuffleFromDiscardRe = regexp.MustCompile(` played (Sacred Ash|Super Rod|Pal Pad|Energy Recycler)\.$`)

func (r *replay) handTo(seat state.Seat, n int, names []string, dest state.Loc) error {
	return r.zoneTo(seat, state.Hand, n, names, dest)
}

// zoneTo moves n cards from seat's zone to dest, binding names when given.
func (r *replay) zoneTo(seat state.Seat, from state.Zone, n int, names []string, dest state.Loc) error {
	var moved []state.IID
	for i := 0; i < n; i++ {
		name := ""
		if i < len(names) {
			name = names[i]
		}
		iid, err := r.pick(seat, []state.Zone{from}, name, moved)
		if err != nil {
			return err
		}
		if err := r.bind(iid, name); err != nil {
			return err
		}
		moved = append(moved, iid)
	}
	for _, iid := range moved {
		if err := r.move(seat, iid, dest, "", ""); err != nil {
			return err
		}
	}
	return nil
}

// boundIn finds a card already known to be name in zone.
func (r *replay) boundIn(seat state.Seat, z state.Zone, name string) (state.IID, error) {
	for _, iid := range r.zoneCards(seat, z) {
		if r.bound[iid] == name {
			return iid, nil
		}
	}
	return "", fmt.Errorf("no %s in %s's %s", name, r.players[seat], z)
}

// moveNamed moves a card named name to dest, looking first in the known
// zones, then (if inPlay) at Pokémon in play (the whole Pokémon moves its top
// card; attachments go to the discard), then in the fallback zones.
func (r *replay) moveNamed(seat state.Seat, name string, dest state.Loc, known []state.Zone, inPlay bool, fallback []state.Zone) error {
	for _, z := range known {
		if iid, err := r.boundIn(seat, z, name); err == nil {
			return r.move(seat, iid, dest, "", "")
		}
	}
	if inPlay {
		if l, err := r.stackNamed(seat, name, "bench"); err == nil {
			return r.moveStack(seat, l, dest)
		}
		if l, err := r.stackNamed(seat, name, "active"); err == nil {
			return r.moveStack(seat, l, dest)
		}
	}
	iid, err := r.take(seat, fallback, name)
	if err != nil {
		return err
	}
	return r.move(seat, iid, dest, "", "")
}

// moveStack moves a Pokémon's top card to dest; the rest of the stack goes
// to the discard pile.
func (r *replay) moveStack(seat state.Seat, l state.Loc, dest state.Loc) error {
	st, _ := r.st.Stack(l)
	top := st.Top()
	for _, iid := range st.Cards() {
		if iid == top {
			continue
		}
		if err := r.move(seat, iid, state.Loc{Seat: l.Seat, Zone: state.Discard}, "", ""); err != nil {
			return err
		}
	}
	return r.move(seat, top, dest, "", "")
}

// discardFrom handles "<cards> discarded from <Pokémon>": after a knockout
// it names cards already discarded; otherwise it moves them from the
// Pokémon in play.
func (r *replay) discardFrom(seat state.Seat, pokemon string, names []string, n int) error {
	if ko := r.lastKO; ko != nil && ko.seat == seat && ko.name == pokemon {
		group := slices.DeleteFunc(slices.Clone(ko.iids), func(x state.IID) bool { return x == ko.top })
		return r.bindGroup(group, names)
	}
	l, err := r.stackNamed(seat, pokemon, "")
	if err != nil {
		return err
	}
	st, _ := r.st.Stack(l)
	pool := append(slices.Clone(st.Energy), st.Tools...)
	pool = append(pool, st.Pokemon[:max(len(st.Pokemon)-1, 0)]...)
	if len(names) == 0 {
		return fmt.Errorf("%d unnamed cards discarded from %s", n, pokemon)
	}
	var chosen []state.IID
	for _, name := range names {
		i := slices.IndexFunc(pool, func(x state.IID) bool { return r.bound[x] == name && !slices.Contains(chosen, x) })
		if i < 0 {
			i = slices.IndexFunc(pool, func(x state.IID) bool {
				return r.bound[x] == "" && r.bindable(x, name) && !slices.Contains(chosen, x)
			})
		}
		if i < 0 {
			return fmt.Errorf("%s has no %s attached", pokemon, name)
		}
		if err := r.bind(pool[i], name); err != nil {
			return err
		}
		chosen = append(chosen, pool[i])
	}
	for _, iid := range chosen {
		if err := r.move(seat, iid, state.Loc{Seat: seat, Zone: state.Discard}, "", ""); err != nil {
			return err
		}
	}
	return nil
}

func (r *replay) endTurn() error {
	if !r.st.TurnOpen {
		return nil
	}
	return r.emit(r.st.Current, state.EvTurn, state.TurnPayload{Phase: "end", Number: r.st.Turn})
}

// stackHolding finds a Pokémon in play whose cards include every name.
func (r *replay) stackHolding(seat state.Seat, names []string) (state.Loc, bool) {
	if len(names) == 0 {
		return state.Loc{}, false
	}
	for _, l := range r.st.StackLocs(seat) {
		st, _ := r.st.Stack(l)
		have := map[string]int{}
		for _, iid := range st.Cards() {
			have[r.bound[iid]]++
		}
		ok := true
		for _, name := range names {
			if have[name] == 0 {
				ok = false
				break
			}
			have[name]--
		}
		if ok {
			return l, true
		}
	}
	return state.Loc{}, false
}

var toolCreditRe = regexp.MustCompile(`^\(Pokémon Tool\) (.+): -?\d+ damage$`)

// checkToolCredits reads an attack's damage breakdown: a Tool it credits
// was attached to the attacker or the defender. If neither holds it, the
// replay attached that Tool to the wrong copy of a Pokémon earlier (the log
// rarely says which); recording a doubt lets the search try the others.
func (r *replay) checkToolCredits(n *Node, actor state.Seat, defender state.Loc) {
	for _, c := range n.Children {
		if c.Depth != 1 || c.Text != "Damage breakdown:" {
			continue
		}
		// Each breakdown line is its own bullet node.
		var items []string
		for _, b := range c.Children {
			if b.Depth == 2 {
				items = append(items, b.Items...)
			}
		}
		for _, item := range items {
			m := toolCreditRe.FindStringSubmatch(item)
			if m == nil {
				continue
			}
			holds := func(l state.Loc) bool {
				st, err := r.st.Stack(l)
				if err != nil || st == nil {
					return false
				}
				return slices.ContainsFunc(st.Tools, func(t state.IID) bool { return r.bound[t] == m[1] })
			}
			if !holds(state.Loc{Seat: actor, Zone: state.Active}) && !holds(defender) {
				r.doubts = append(r.doubts, Problem{Line: n.No, Text: n.Text,
					Err: fmt.Sprintf("the damage counts %s, but neither Pokémon holds it in the reconstruction", m[1])})
			}
		}
	}
}
