package state

import (
	"fmt"
	"maps"
	"slices"
)

// Phase of the game.
const (
	PhaseEmpty   = ""      // no setup event yet
	PhaseSetup   = "setup" // decks loaded, before the first turn
	PhasePlaying = "playing"
)

// MaxBench is the most Pokémon a bench can hold, with a Stadium such as
// Area Zero Underdepths in play. The playmat allows it at any time and
// leaves the BenchSize limit to the warnings.
const MaxBench = 8

// Side is one player's half of the playmat.
type Side struct {
	Name string `json:"name,omitempty"`
	// Deck is ordered: index 0 is the top card.
	Deck []IID `json:"deck"`
	// Prizes has one entry per prize slot; "" is an empty slot.
	Prizes  []IID    `json:"prizes"`
	Hand    []IID    `json:"hand"`
	Discard []IID    `json:"discard"`
	Lost    []IID    `json:"lost"`
	Active  *Stack   `json:"active,omitempty"`
	Bench   []*Stack `json:"bench"`
}

func (s *Side) clone() *Side {
	c := *s
	c.Deck = slices.Clone(s.Deck)
	c.Prizes = slices.Clone(s.Prizes)
	c.Hand = slices.Clone(s.Hand)
	c.Discard = slices.Clone(s.Discard)
	c.Lost = slices.Clone(s.Lost)
	c.Active = s.Active.clone()
	c.Bench = make([]*Stack, len(s.Bench))
	for i, b := range s.Bench {
		c.Bench[i] = b.clone()
	}
	return &c
}

// PrizesLeft counts occupied prize slots.
func (s *Side) PrizesLeft() int {
	n := 0
	for _, p := range s.Prizes {
		if p != "" {
			n++
		}
	}
	return n
}

// Effect is a lingering effect, e.g. an Item lock.
type Effect struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Source IID    `json:"source,omitempty"`
	// Seat is the player the effect applies to.
	Seat Seat `json:"seat,omitempty"`
	// UntilTurn expires the effect at the end of that turn; 0 lasts until
	// removed.
	UntilTurn int `json:"until_turn,omitempty"`
}

// State is the board at one point in a game's history.
type State struct {
	Phase      string         `json:"phase"`
	BenchSize  int            `json:"bench_size"`
	PrizeCount int            `json:"prize_count"`
	Turn       int            `json:"turn"`
	Current    Seat           `json:"current,omitempty"`
	TurnOpen   bool           `json:"turn_open"`
	First      Seat           `json:"first,omitempty"`
	Cards      map[IID]*Card  `json:"cards"`
	Sides      map[Seat]*Side `json:"sides"`
	Stadium    IID            `json:"stadium,omitempty"`
	Effects    []Effect       `json:"effects,omitempty"`
	Coins      []string       `json:"coins,omitempty"` // results of the latest flip
	Known      map[IID]uint8  `json:"known,omitempty"` // bitmask of seats that know a hidden card
	Head       int            `json:"head"`
}

// New returns an empty state.
func New() *State {
	return &State{
		Cards: map[IID]*Card{},
		Sides: map[Seat]*Side{P1: {}, P2: {}},
		Known: map[IID]uint8{},
	}
}

// Clone deep-copies the state.
func (s *State) Clone() *State {
	c := *s
	c.Cards = make(map[IID]*Card, len(s.Cards))
	for k, v := range s.Cards {
		cc := *v
		c.Cards[k] = &cc
	}
	c.Sides = make(map[Seat]*Side, len(s.Sides))
	for k, v := range s.Sides {
		c.Sides[k] = v.clone()
	}
	c.Effects = slices.Clone(s.Effects)
	c.Coins = slices.Clone(s.Coins)
	c.Known = maps.Clone(s.Known)
	return &c
}

func seatBit(s Seat) uint8 {
	if s == P1 {
		return 1
	}
	return 2
}

// KnownTo reports whether seat can see card iid where it currently is.
func (s *State) KnownTo(iid IID, seat Seat) bool {
	loc, err := s.Find(iid)
	if err != nil {
		return false
	}
	if loc.Zone.IsPublic() {
		return true
	}
	return s.Known[iid]&seatBit(seat) != 0
}

// KnownBy reports whether seat knows hidden card iid's identity. Cards in
// public zones are known to everyone regardless.
func (s *State) KnownBy(iid IID, seat Seat) bool {
	return s.Known[iid]&seatBit(seat) != 0
}

// Stack returns the stack at l, which may be nil if the slot is empty.
func (s *State) Stack(l Loc) (*Stack, error) {
	side, ok := s.Sides[l.Seat]
	if !ok {
		return nil, fmt.Errorf("unknown seat %q", l.Seat)
	}
	switch l.Zone {
	case Active:
		return side.Active, nil
	case Bench:
		if l.Slot < 0 || l.Slot >= MaxBench {
			return nil, fmt.Errorf("bench slot %d out of range (a bench holds at most %d)", l.Slot, MaxBench)
		}
		if l.Slot >= len(side.Bench) {
			return nil, nil
		}
		return side.Bench[l.Slot], nil
	}
	return nil, fmt.Errorf("%s is not a Pokémon slot", l)
}

func (s *State) setStack(l Loc, st *Stack) {
	side := s.Sides[l.Seat]
	if st != nil && st.Empty() {
		st = nil
	}
	if l.Zone == Active {
		side.Active = st
		return
	}
	// The bench has BenchSize slots, and grows past them to MaxBench only
	// while the extra slots are in use.
	for l.Slot >= len(side.Bench) {
		side.Bench = append(side.Bench, nil)
	}
	side.Bench[l.Slot] = st
	for len(side.Bench) > s.BenchSize && side.Bench[len(side.Bench)-1] == nil {
		side.Bench = side.Bench[:len(side.Bench)-1]
	}
}

// FreeBenchSlot returns seat's first empty bench slot, which is past
// BenchSize when those slots are all full, or -1 when MaxBench are.
func (s *State) FreeBenchSlot(seat Seat) int {
	bench := s.Sides[seat].Bench
	if i := slices.IndexFunc(bench, func(st *Stack) bool { return st.Empty() }); i >= 0 {
		return i
	}
	if len(bench) < MaxBench {
		return len(bench)
	}
	return -1
}

// StackLocs lists every occupied stack location for seat, active first.
func (s *State) StackLocs(seat Seat) []Loc {
	side := s.Sides[seat]
	var out []Loc
	if !side.Active.Empty() {
		out = append(out, Loc{Seat: seat, Zone: Active})
	}
	for i, b := range side.Bench {
		if !b.Empty() {
			out = append(out, Loc{Seat: seat, Zone: Bench, Slot: i})
		}
	}
	return out
}

// Find returns the location of card iid. For deck cards Pos is set to the
// card's index; for stacks the role is not reported.
func (s *State) Find(iid IID) (Loc, error) {
	if s.Stadium == iid && iid != "" {
		return Loc{Zone: Stadium}, nil
	}
	for _, seat := range Seats {
		side := s.Sides[seat]
		if i := slices.Index(side.Deck, iid); i >= 0 {
			return Loc{Seat: seat, Zone: Deck, Pos: &i}, nil
		}
		if i := slices.Index(side.Prizes, iid); i >= 0 {
			return Loc{Seat: seat, Zone: Prizes, Slot: i}, nil
		}
		if slices.Contains(side.Hand, iid) {
			return Loc{Seat: seat, Zone: Hand}, nil
		}
		if slices.Contains(side.Discard, iid) {
			return Loc{Seat: seat, Zone: Discard}, nil
		}
		if slices.Contains(side.Lost, iid) {
			return Loc{Seat: seat, Zone: Lost}, nil
		}
		if slices.Contains(side.Active.Cards(), iid) {
			return Loc{Seat: seat, Zone: Active}, nil
		}
		for i, b := range side.Bench {
			if slices.Contains(b.Cards(), iid) {
				return Loc{Seat: seat, Zone: Bench, Slot: i}, nil
			}
		}
	}
	if _, ok := s.Cards[iid]; ok {
		return Loc{}, fmt.Errorf("%s is nowhere on the board", iid)
	}
	return Loc{}, fmt.Errorf("no card %s", iid)
}

// Name returns a card's display name.
func (s *State) Name(iid IID) string {
	c, ok := s.Cards[iid]
	if !ok || c.Name == "" {
		return "?"
	}
	return c.Name
}
