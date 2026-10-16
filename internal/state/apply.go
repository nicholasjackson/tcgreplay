package state

import (
	"fmt"
	"slices"
)

// Fold builds a state from an ordered path of events.
func Fold(events []Event) (*State, error) {
	s := New()
	for i := range events {
		if err := s.Apply(&events[i]); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Apply applies one event in place. On error the state may be partially
// changed; callers that need atomicity apply to a Clone.
func (s *State) Apply(e *Event) error {
	p, err := e.Decode()
	if err != nil {
		return err
	}
	if s.Phase == PhaseEmpty && e.Type != EvSetup {
		return fmt.Errorf("event %d (%s): game has not been set up", e.ID, e.Type)
	}
	switch p := p.(type) {
	case *SetupPayload:
		err = s.applySetup(p)
	case *MovePayload:
		err = s.applyMove(p)
	case *SwapPayload:
		err = s.applySwap(p)
	case *ShufflePayload:
		err = s.applyShuffle(p)
	case *CoinPayload:
		s.Coins = slices.Clone(p.Results)
	case *DamagePayload:
		err = s.applyDamage(p)
	case *ConditionPayload:
		err = s.applyCondition(p)
	case *EffectPayload:
		err = s.applyEffect(p)
	case *RevealPayload:
		err = s.applyReveal(p)
	case *TurnPayload:
		err = s.applyTurn(e.Seat, p)
	case *AssumePayload:
		err = s.applyAssume(p)
	case *DeclarePayload:
		// Recorded for the log only.
	}
	if err != nil {
		return fmt.Errorf("event %d (%s): %w", e.ID, e.Type, err)
	}
	s.Head = e.ID
	return nil
}

func (s *State) applySetup(p *SetupPayload) error {
	if s.Phase != PhaseEmpty {
		return fmt.Errorf("game is already set up")
	}
	if p.BenchSize <= 0 || p.PrizeCount <= 0 {
		return fmt.Errorf("bench size and prize count must be positive")
	}
	s.Phase = PhaseSetup
	s.BenchSize = p.BenchSize
	s.PrizeCount = p.PrizeCount
	for _, seat := range Seats {
		s.Sides[seat] = &Side{
			Name:   p.Names[seat],
			Deck:   []IID{},
			Prizes: make([]IID, p.PrizeCount),
			Hand:   []IID{},
			Bench:  make([]*Stack, p.BenchSize),
		}
	}
	for _, c := range p.Cards {
		if c.IID == "" {
			return fmt.Errorf("card with empty iid")
		}
		if !c.Owner.Valid() {
			return fmt.Errorf("card %s has bad owner %q", c.IID, c.Owner)
		}
		if _, dup := s.Cards[c.IID]; dup {
			return fmt.Errorf("duplicate iid %s", c.IID)
		}
		cc := c
		s.Cards[c.IID] = &cc
		side := s.Sides[c.Owner]
		side.Deck = append(side.Deck, c.IID)
	}
	return nil
}

// remove takes a card out of its current location and returns where it was.
func (s *State) remove(iid IID) (Loc, error) {
	loc, err := s.Find(iid)
	if err != nil {
		return Loc{}, err
	}
	side := s.Sides[loc.Seat]
	del := func(xs []IID) []IID {
		return slices.DeleteFunc(xs, func(x IID) bool { return x == iid })
	}
	switch loc.Zone {
	case Stadium:
		s.Stadium = ""
	case Deck:
		side.Deck = del(side.Deck)
	case Prizes:
		side.Prizes[loc.Slot] = ""
	case Hand:
		side.Hand = del(side.Hand)
	case Discard:
		side.Discard = del(side.Discard)
	case Lost:
		side.Lost = del(side.Lost)
	case Active, Bench:
		st, _ := s.Stack(loc)
		st.Pokemon = del(st.Pokemon)
		st.Energy = del(st.Energy)
		st.Tools = del(st.Tools)
		if len(st.Pokemon) == 0 {
			st.Damage = 0
			st.Conditions = nil
		}
		s.setStack(loc, st)
	}
	return loc, nil
}

func (s *State) applyMove(p *MovePayload) error {
	card, ok := s.Cards[p.IID]
	if !ok {
		return fmt.Errorf("no card %s", p.IID)
	}
	to := p.To
	if to.Zone == Stadium {
		to.Seat = ""
	} else if to.Seat == "" {
		to.Seat = card.Owner
	}
	if !to.Zone.Valid() {
		return fmt.Errorf("unknown zone %q", to.Zone)
	}
	if to.Seat != "" && to.Seat != card.Owner {
		return fmt.Errorf("%s belongs to %s and cannot go to %s", p.IID, card.Owner, to)
	}
	cur, err := s.Find(p.IID)
	if err != nil {
		return err
	}
	if p.From != nil {
		f := *p.From
		if f.Seat == "" && f.Zone != Stadium {
			f.Seat = card.Owner
		}
		if f.Seat != cur.Seat || f.Zone != cur.Zone || (f.Zone == Bench || f.Zone == Prizes) && f.Slot != cur.Slot {
			return fmt.Errorf("%s is in %s, not %s", p.IID, cur, f)
		}
	}
	if cur.Zone.IsStack() && to.Zone.IsStack() && cur.SameStack(to) {
		return fmt.Errorf("%s is already in %s", p.IID, to)
	}
	// Validate the destination before removing anything.
	side := s.Sides[card.Owner]
	var dest *Stack
	switch to.Zone {
	case Prizes:
		if to.Slot < 0 || to.Slot >= len(side.Prizes) {
			return fmt.Errorf("prize slot %d out of range", to.Slot)
		}
		if side.Prizes[to.Slot] != "" {
			return fmt.Errorf("prize slot %d is occupied by %s", to.Slot, side.Prizes[to.Slot])
		}
	case Stadium:
		if s.Stadium != "" && s.Stadium != p.IID {
			return fmt.Errorf("stadium is occupied by %s (%s); discard it first", s.Stadium, s.Name(s.Stadium))
		}
	case Active, Bench:
		dest, err = s.Stack(to)
		if err != nil {
			return err
		}
		role := p.As
		if dest.Empty() {
			if role != "" && role != AsPokemon {
				return fmt.Errorf("%s is empty; only a Pokémon can start a stack", to)
			}
		} else if role != AsPokemon && role != AsEnergy && role != AsTool {
			return fmt.Errorf("%s is occupied; say whether %s goes on as pokemon, energy or tool", to, p.IID)
		}
	}

	wasPublic := cur.Zone.IsPublic()
	if _, err := s.remove(p.IID); err != nil {
		return err
	}

	switch to.Zone {
	case Deck:
		pos := Bottom
		if to.Pos != nil {
			pos = *to.Pos
		}
		if pos == Bottom || pos >= len(side.Deck) {
			side.Deck = append(side.Deck, p.IID)
		} else {
			side.Deck = slices.Insert(side.Deck, pos, p.IID)
		}
	case Prizes:
		side.Prizes[to.Slot] = p.IID
	case Hand:
		side.Hand = append(side.Hand, p.IID)
	case Discard:
		side.Discard = append(side.Discard, p.IID)
	case Lost:
		side.Lost = append(side.Lost, p.IID)
	case Stadium:
		s.Stadium = p.IID
	case Active, Bench:
		// The source removal may have changed the destination pointer if it
		// emptied a stack, so look it up again.
		dest, _ = s.Stack(to)
		if dest == nil {
			dest = &Stack{}
		}
		switch p.As {
		case "", AsPokemon:
			dest.Pokemon = append(dest.Pokemon, p.IID)
			dest.EnteredTurn = s.Turn
		case AsEnergy:
			dest.Energy = append(dest.Energy, p.IID)
		case AsTool:
			dest.Tools = append(dest.Tools, p.IID)
		}
		s.setStack(to, dest)
	}

	// Track who knows the card's identity.
	bits := s.Known[p.IID]
	switch {
	case to.Zone.IsPublic():
		bits = 0 // visible to all while there
	case to.Zone == Hand:
		if wasPublic {
			bits = 3
		}
		bits |= seatBit(card.Owner)
	default: // deck, prizes
		bits = 0
	}
	switch p.Face {
	case "up":
		bits = 3
	case "down":
		if !to.Zone.IsPublic() {
			bits = 0
		}
	}
	if bits == 0 {
		delete(s.Known, p.IID)
	} else {
		s.Known[p.IID] = bits
	}
	return nil
}

func (s *State) applySwap(p *SwapPayload) error {
	if !p.A.Zone.IsStack() || !p.B.Zone.IsStack() {
		return fmt.Errorf("swap needs two Pokémon slots")
	}
	if p.A.Seat != p.B.Seat {
		return fmt.Errorf("cannot swap across seats")
	}
	if p.A.SameStack(p.B) {
		return fmt.Errorf("cannot swap %s with itself", p.A)
	}
	a, err := s.Stack(p.A)
	if err != nil {
		return err
	}
	b, err := s.Stack(p.B)
	if err != nil {
		return err
	}
	if a.Empty() {
		return fmt.Errorf("%s is empty", p.A)
	}
	s.setStack(p.A, b)
	s.setStack(p.B, a)
	return nil
}

func (s *State) applyShuffle(p *ShufflePayload) error {
	side, ok := s.Sides[p.Seat]
	if !ok {
		return fmt.Errorf("unknown seat %q", p.Seat)
	}
	a, b := slices.Clone(side.Deck), slices.Clone(p.Order)
	slices.Sort(a)
	slices.Sort(b)
	if !slices.Equal(a, b) {
		return fmt.Errorf("shuffle order is not a permutation of %s's deck", p.Seat)
	}
	side.Deck = slices.Clone(p.Order)
	for _, iid := range side.Deck {
		delete(s.Known, iid)
	}
	return nil
}

func (s *State) pokemonAt(l Loc) (*Stack, error) {
	st, err := s.Stack(l)
	if err != nil {
		return nil, err
	}
	if st == nil || len(st.Pokemon) == 0 {
		return nil, fmt.Errorf("no Pokémon in %s", l)
	}
	return st, nil
}

func (s *State) applyDamage(p *DamagePayload) error {
	st, err := s.pokemonAt(p.Target)
	if err != nil {
		return err
	}
	if p.Value < 0 {
		return fmt.Errorf("damage cannot be negative")
	}
	st.Damage = p.Value
	return nil
}

func (s *State) applyCondition(p *ConditionPayload) error {
	st, err := s.pokemonAt(p.Target)
	if err != nil {
		return err
	}
	if p.Condition == "all" && !p.On {
		st.Conditions = nil
		return nil
	}
	if !ValidCondition(p.Condition) {
		return fmt.Errorf("unknown condition %q", p.Condition)
	}
	st.Conditions = slices.DeleteFunc(st.Conditions, func(c string) bool { return c == p.Condition })
	if p.On {
		// Asleep, Confused and Paralyzed replace one another.
		rot := []string{"asleep", "confused", "paralyzed"}
		if slices.Contains(rot, p.Condition) {
			st.Conditions = slices.DeleteFunc(st.Conditions, func(c string) bool { return slices.Contains(rot, c) })
		}
		st.Conditions = append(st.Conditions, p.Condition)
		slices.Sort(st.Conditions)
	}
	return nil
}

func (s *State) applyEffect(p *EffectPayload) error {
	switch p.Op {
	case "add":
		if p.Effect.ID == "" {
			return fmt.Errorf("effect needs an id")
		}
		for _, e := range s.Effects {
			if e.ID == p.Effect.ID {
				return fmt.Errorf("effect %s already exists", e.ID)
			}
		}
		s.Effects = append(s.Effects, p.Effect)
	case "remove":
		n := len(s.Effects)
		s.Effects = slices.DeleteFunc(s.Effects, func(e Effect) bool { return e.ID == p.Effect.ID })
		if len(s.Effects) == n {
			return fmt.Errorf("no effect %s", p.Effect.ID)
		}
	default:
		return fmt.Errorf("unknown effect op %q", p.Op)
	}
	return nil
}

func (s *State) applyReveal(p *RevealPayload) error {
	for _, iid := range p.IIDs {
		if _, ok := s.Cards[iid]; !ok {
			return fmt.Errorf("no card %s", iid)
		}
		for _, seat := range p.To {
			s.Known[iid] |= seatBit(seat)
		}
	}
	return nil
}

func (s *State) applyTurn(seat Seat, p *TurnPayload) error {
	if !seat.Valid() {
		return fmt.Errorf("turn event needs a seat")
	}
	switch p.Phase {
	case "start":
		if s.TurnOpen {
			return fmt.Errorf("turn %d is still open", s.Turn)
		}
		if p.Number != s.Turn+1 {
			return fmt.Errorf("next turn is %d, not %d", s.Turn+1, p.Number)
		}
		s.Phase = PhasePlaying
		s.Turn = p.Number
		s.Current = seat
		s.TurnOpen = true
		if s.First == "" {
			s.First = seat
		}
	case "end":
		if !s.TurnOpen || s.Current != seat {
			return fmt.Errorf("it is not %s's turn", seat)
		}
		s.TurnOpen = false
		s.Effects = slices.DeleteFunc(s.Effects, func(e Effect) bool {
			return e.UntilTurn != 0 && e.UntilTurn <= s.Turn
		})
	default:
		return fmt.Errorf("unknown turn phase %q", p.Phase)
	}
	return nil
}

func (s *State) applyAssume(p *AssumePayload) error {
	for iid, a := range p.Cards {
		c, ok := s.Cards[iid]
		if !ok {
			return fmt.Errorf("no card %s", iid)
		}
		c.Def, c.Name, c.Basis = a.Def, a.Name, a.Basis
	}
	return nil
}
