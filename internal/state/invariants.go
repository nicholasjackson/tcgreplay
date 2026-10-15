package state

import (
	"fmt"
	"slices"
)

// DeckSize is the number of cards each seat must hold.
const DeckSize = 60

// Locations returns every card on the board with its location, in a stable
// order. A card appearing twice indicates corruption.
func (s *State) Locations() []struct {
	IID IID
	Loc Loc
} {
	type pair = struct {
		IID IID
		Loc Loc
	}
	var out []pair
	add := func(l Loc, xs ...IID) {
		for _, x := range xs {
			if x != "" {
				out = append(out, pair{x, l})
			}
		}
	}
	for _, seat := range Seats {
		side := s.Sides[seat]
		add(Loc{Seat: seat, Zone: Deck}, side.Deck...)
		for i, p := range side.Prizes {
			add(Loc{Seat: seat, Zone: Prizes, Slot: i}, p)
		}
		add(Loc{Seat: seat, Zone: Hand}, side.Hand...)
		add(Loc{Seat: seat, Zone: Discard}, side.Discard...)
		add(Loc{Seat: seat, Zone: Lost}, side.Lost...)
		add(Loc{Seat: seat, Zone: Active}, side.Active.Cards()...)
		for i, b := range side.Bench {
			add(Loc{Seat: seat, Zone: Bench, Slot: i}, b.Cards()...)
		}
	}
	add(Loc{Zone: Stadium}, s.Stadium)
	return out
}

// Check returns invariant violations: every card exactly once on the board,
// on its owner's side, and DeckSize cards per seat.
func (s *State) Check() []string {
	if s.Phase == PhaseEmpty {
		return nil
	}
	var problems []string
	seen := map[IID]int{}
	perSeat := map[Seat]int{}
	for _, p := range s.Locations() {
		seen[p.IID]++
		c, ok := s.Cards[p.IID]
		if !ok {
			problems = append(problems, fmt.Sprintf("unknown card %s in %s", p.IID, p.Loc))
			continue
		}
		perSeat[c.Owner]++
		if p.Loc.Seat != "" && p.Loc.Seat != c.Owner {
			problems = append(problems, fmt.Sprintf("%s (owner %s) is on %s's side", p.IID, c.Owner, p.Loc.Seat))
		}
	}
	ids := make([]IID, 0, len(s.Cards))
	for iid := range s.Cards {
		ids = append(ids, iid)
	}
	slices.Sort(ids)
	for _, iid := range ids {
		switch n := seen[iid]; {
		case n == 0:
			problems = append(problems, fmt.Sprintf("%s is missing from the board", iid))
		case n > 1:
			problems = append(problems, fmt.Sprintf("%s appears %d times", iid, n))
		}
	}
	for _, seat := range Seats {
		if n := perSeat[seat]; n != DeckSize {
			problems = append(problems, fmt.Sprintf("%s holds %d cards, not %d", seat, n, DeckSize))
		}
	}
	for _, seat := range Seats {
		for _, l := range s.StackLocs(seat) {
			st, _ := s.Stack(l)
			if len(st.Pokemon) == 0 {
				problems = append(problems, fmt.Sprintf("%s has attachments but no Pokémon", l))
			}
		}
	}
	return problems
}
