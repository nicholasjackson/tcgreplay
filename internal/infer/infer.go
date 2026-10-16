// Package infer reasons about cards one seat cannot see: what a game's
// history proves about the other seat's list, and plausible identities for
// the hidden cards at a position.
package infer

import (
	"fmt"
	"math/rand/v2"
	"sort"

	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// Profile is an assumed opponent list. It is stored in setup and assume
// events, so a game's assumptions are part of its history.
type Profile struct {
	Opponent  string `json:"opponent,omitempty"`
	Archetype string `json:"archetype,omitempty"`
	// Observed is the minimum count of each card the log proves.
	Observed map[string]int `json:"observed,omitempty"`
	Cards    []ProfileCard  `json:"cards,omitempty"`
}

// ProfileCard is one line of an assumed list.
type ProfileCard struct {
	Name        string `json:"name"`
	Def         string `json:"def,omitempty"`
	Count       int    `json:"count"`
	MinObserved int    `json:"min_observed,omitempty"`
	// Basis is observed (every copy was seen), reference (from a list) or
	// filler (added to reach a full deck).
	Basis string `json:"basis"`
}

// Total counts the cards in the profile.
func (p *Profile) Total() int {
	n := 0
	for _, c := range p.Cards {
		n += c.Count
	}
	return n
}

// Evidence is what viewer has seen of owner's cards along a branch.
// Counts are by name, never by card instance: when a seen card goes back
// into a hidden zone, nobody can tell whether a later copy is the same card.
type Evidence struct {
	// Min is, per card name, the most copies viewer could see at once: a
	// lower bound on the count in owner's list.
	Min map[string]int
	// OutsidePrizes is, per card name, the most copies viewer could see at
	// once outside the prize slots after the prizes were dealt. Prizes
	// only ever leave, so at most list count minus this many are prizes.
	OutsidePrizes map[string]int
}

// Gather folds events, recording what viewer saw of owner's cards after
// each one. It returns the final state too.
func Gather(events []state.Event, owner, viewer state.Seat) (*state.State, *Evidence, error) {
	st := state.New()
	ev := &Evidence{Min: map[string]int{}, OutsidePrizes: map[string]int{}}
	dealt := false
	for i := range events {
		if err := st.Apply(&events[i]); err != nil {
			return nil, nil, err
		}
		dealt = dealt || st.Sides[owner].PrizesLeft() > 0
		all, outside := map[string]int{}, map[string]int{}
		for _, cl := range st.Locations() {
			c := st.Cards[cl.IID]
			if c.Owner != owner || c.Name == "" {
				continue
			}
			if cl.Loc.Zone.IsPublic() || st.KnownBy(cl.IID, viewer) {
				all[c.Name]++
				if cl.Loc.Zone != state.Prizes {
					outside[c.Name]++
				}
			}
		}
		for name, n := range all {
			ev.Min[name] = max(ev.Min[name], n)
		}
		if dealt {
			for name, n := range outside {
				ev.OutsidePrizes[name] = max(ev.OutsidePrizes[name], n)
			}
		}
	}
	return st, ev, nil
}

// Reconcile turns a reference list into a profile consistent with the
// observed minimums: counts are raised to what was seen and cards seen but
// missing from the list are added. It does not trim back to 60; the
// returned warnings say when the total is off.
func Reconcile(list []deck.Entry, observed map[string]int) (Profile, []string) {
	p := Profile{Observed: observed}
	idx := map[string]int{}
	for _, e := range list {
		if i, ok := idx[e.Name]; ok {
			p.Cards[i].Count += e.Count
			continue
		}
		idx[e.Name] = len(p.Cards)
		p.Cards = append(p.Cards, ProfileCard{Name: e.Name, Def: e.Def, Count: e.Count, Basis: "reference"})
	}
	var warnings []string
	var missing []string
	for name := range observed {
		if _, ok := idx[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		idx[name] = len(p.Cards)
		p.Cards = append(p.Cards, ProfileCard{Name: name, Count: 0})
	}
	for i := range p.Cards {
		c := &p.Cards[i]
		c.MinObserved = observed[c.Name]
		if c.Count < c.MinObserved {
			if c.Count > 0 {
				warnings = append(warnings, fmt.Sprintf("raised %s from %d to the %d seen", c.Name, c.Count, c.MinObserved))
			} else {
				warnings = append(warnings, fmt.Sprintf("added %d %s, seen but not in the list", c.MinObserved, c.Name))
			}
			c.Count = c.MinObserved
		}
		if c.MinObserved > 0 && c.MinObserved >= c.Count {
			c.Basis = "observed"
		}
	}
	if n := p.Total(); n != state.DeckSize {
		warnings = append(warnings, fmt.Sprintf("the list has %d cards, not %d", n, state.DeckSize))
	}
	return p, warnings
}

// Sample assigns identities to every card of owner's that viewer cannot
// see now. Cards viewer can see keep their identities. The rest are drawn
// from the profile minus the visible cards, with no more of a card in the
// prizes than the evidence allows. With no profile, the hidden cards'
// current identities (some possibly unknown) are redistributed instead.
func Sample(st *state.State, ev *Evidence, prof *Profile, owner, viewer state.Seat, rng *rand.Rand) (state.AssumePayload, []string) {
	var warnings []string
	defs := map[string]string{}
	var prizes, rest []state.IID // hidden slots
	visible, knownPrizes, all := map[string]int{}, map[string]int{}, map[string]int{}
	var current []string // identities of hidden cards, for the no-profile case
	for _, cl := range st.Locations() {
		c := st.Cards[cl.IID]
		if c.Owner != owner {
			continue
		}
		all[c.Name]++
		if c.Def != "" {
			defs[c.Name] = c.Def
		}
		if cl.Loc.Zone.IsPublic() || st.KnownBy(cl.IID, viewer) {
			visible[c.Name]++
			if cl.Loc.Zone == state.Prizes {
				knownPrizes[c.Name]++
			}
			continue
		}
		current = append(current, c.Name)
		if cl.Loc.Zone == state.Prizes {
			prizes = append(prizes, cl.IID)
		} else {
			rest = append(rest, cl.IID)
		}
	}

	// The pool is the multiset of identities the hidden cards take.
	var pool []string
	listed := all
	if prof == nil || len(prof.Cards) == 0 {
		pool = current
	} else {
		listed = map[string]int{}
		for _, c := range prof.Cards {
			listed[c.Name] += c.Count
			if c.Def != "" {
				defs[c.Name] = c.Def
			}
			n := c.Count - visible[c.Name]
			if n < 0 {
				warnings = append(warnings, fmt.Sprintf("%d %s visible, the list has %d", visible[c.Name], c.Name, c.Count))
			}
			for range max(n, 0) {
				pool = append(pool, c.Name)
			}
		}
		for name, n := range visible {
			if name != "" && listed[name] == 0 {
				warnings = append(warnings, fmt.Sprintf("%d %s visible, the list has none", n, name))
			}
		}
	}
	hidden := len(prizes) + len(rest)
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	switch {
	case len(pool) > hidden:
		warnings = append(warnings, fmt.Sprintf("the list leaves %d cards for %d hidden slots; dropped %d at random", len(pool), hidden, len(pool)-hidden))
		pool = pool[:hidden]
	case len(pool) < hidden:
		warnings = append(warnings, fmt.Sprintf("the list leaves %d cards for %d hidden slots; %d stay unknown", len(pool), hidden, hidden-len(pool)))
		for len(pool) < hidden {
			pool = append(pool, "")
		}
	}

	// Fill the prize slots first, within each card's cap; the rest of the
	// pool goes to the hand and deck.
	var prizeNames, other []string
	took := map[string]int{}
	for _, name := range pool {
		limit := listed[name] - ev.OutsidePrizes[name] - knownPrizes[name]
		if len(prizeNames) < len(prizes) && (name == "" || took[name] < limit) {
			prizeNames = append(prizeNames, name)
			took[name]++
		} else {
			other = append(other, name)
		}
	}
	if short := len(prizes) - len(prizeNames); short > 0 {
		warnings = append(warnings, fmt.Sprintf("the list cannot fill the prizes without cards seen since they were dealt; %d prizes break that", short))
		prizeNames = append(prizeNames, other[len(other)-short:]...)
		other = other[:len(other)-short]
	}
	rng.Shuffle(len(other), func(i, j int) { other[i], other[j] = other[j], other[i] })

	out := state.AssumePayload{Cards: map[state.IID]state.Assumption{}}
	assign := func(iids []state.IID, names []string) {
		for i, iid := range iids {
			out.Cards[iid] = state.Assumption{Name: names[i], Def: defs[names[i]], Basis: basis(names[i])}
		}
	}
	assign(prizes, prizeNames)
	assign(rest, other)
	return out, warnings
}

func basis(name string) string {
	if name == "" {
		return ""
	}
	return "sampled"
}
