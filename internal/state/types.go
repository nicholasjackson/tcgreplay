// Package state holds the playmat's data model and the fold that builds it
// from events. It knows nothing about card text or rules: state is plain data,
// so any position can be built or rewound.
package state

import (
	"fmt"
	"strconv"
	"strings"
)

// Seat identifies a player.
type Seat string

const (
	P1 Seat = "p1"
	P2 Seat = "p2"
)

// Seats lists both seats in a stable order.
var Seats = []Seat{P1, P2}

// Other returns the opposing seat.
func (s Seat) Other() Seat {
	if s == P1 {
		return P2
	}
	return P1
}

// Valid reports whether s is a known seat.
func (s Seat) Valid() bool { return s == P1 || s == P2 }

// IID is a card instance ID, stable for the life of a game, e.g. "p1-37".
type IID string

// Card is one physical card. Def is a TCGdex card ID, or empty when the card
// is hidden information that has not yet been assumed.
type Card struct {
	IID   IID    `json:"iid"`
	Def   string `json:"def"`
	Name  string `json:"name,omitempty"`
	Owner Seat   `json:"owner"`
	// Basis records where an assumed identity came from: observed,
	// decklist, reference, filler, sampled or manual. Empty for cards from
	// a known decklist.
	Basis string `json:"basis,omitempty"`
}

// Zone names a place a card can be.
type Zone string

const (
	Deck    Zone = "deck"
	Prizes  Zone = "prizes"
	Hand    Zone = "hand"
	Discard Zone = "discard"
	Lost    Zone = "lost"
	Active  Zone = "active"
	Bench   Zone = "bench"
	Stadium Zone = "stadium"
)

// IsStack reports whether the zone holds Pokémon stacks.
func (z Zone) IsStack() bool { return z == Active || z == Bench }

// IsPublic reports whether cards in the zone are face up to both players.
func (z Zone) IsPublic() bool {
	switch z {
	case Discard, Lost, Active, Bench, Stadium:
		return true
	}
	return false
}

// Valid reports whether z is a known zone.
func (z Zone) Valid() bool {
	switch z {
	case Deck, Prizes, Hand, Discard, Lost, Active, Bench, Stadium:
		return true
	}
	return false
}

// Pos values for placing a card into the deck.
const (
	Top    = 0
	Bottom = -1
)

// Loc addresses a zone, and optionally a slot (bench or prize index) or a
// position in the deck.
type Loc struct {
	Seat Seat `json:"seat,omitempty"`
	Zone Zone `json:"zone"`
	// Slot is the bench or prize index, zero based.
	Slot int `json:"slot,omitempty"`
	// Pos places a card in the deck: 0 is the top, -1 the bottom. Nil means
	// the bottom.
	Pos *int `json:"pos,omitempty"`
}

func (l Loc) String() string {
	var b strings.Builder
	if l.Seat != "" {
		b.WriteString(string(l.Seat))
		b.WriteByte(':')
	}
	b.WriteString(string(l.Zone))
	if l.Zone == Bench || l.Zone == Prizes {
		fmt.Fprintf(&b, ":%d", l.Slot)
	}
	if l.Zone == Deck && l.Pos != nil {
		switch *l.Pos {
		case Top:
			b.WriteString(":top")
		case Bottom:
			b.WriteString(":bottom")
		default:
			fmt.Fprintf(&b, ":%d", *l.Pos)
		}
	}
	return b.String()
}

// SameStack reports whether two locations name the same stack.
func (l Loc) SameStack(o Loc) bool {
	if l.Seat != o.Seat || l.Zone != o.Zone {
		return false
	}
	return l.Zone != Bench || l.Slot == o.Slot
}

// ParseLoc parses "[seat:]zone[:slot|top|bottom]", e.g. "active",
// "p1:bench:2", "deck:top", "prizes:5". def fills a missing seat.
func ParseLoc(s string, def Seat) (Loc, error) {
	parts := strings.Split(strings.TrimSpace(strings.ToLower(s)), ":")
	l := Loc{Seat: def}
	if len(parts) > 0 && Seat(parts[0]).Valid() {
		l.Seat = Seat(parts[0])
		parts = parts[1:]
	}
	if len(parts) == 0 || parts[0] == "" {
		return Loc{}, fmt.Errorf("location %q has no zone", s)
	}
	l.Zone = Zone(parts[0])
	if !l.Zone.Valid() {
		return Loc{}, fmt.Errorf("location %q: unknown zone %q", s, parts[0])
	}
	if l.Zone == Stadium {
		l.Seat = ""
	}
	if len(parts) > 2 {
		return Loc{}, fmt.Errorf("location %q has too many parts", s)
	}
	if len(parts) == 2 {
		switch {
		case l.Zone == Deck && parts[1] == "top":
			p := Top
			l.Pos = &p
		case l.Zone == Deck && parts[1] == "bottom":
			p := Bottom
			l.Pos = &p
		default:
			n, err := strconv.Atoi(parts[1])
			if err != nil || n < 0 {
				return Loc{}, fmt.Errorf("location %q: bad index %q", s, parts[1])
			}
			switch l.Zone {
			case Bench, Prizes:
				l.Slot = n
			case Deck:
				l.Pos = &n
			default:
				return Loc{}, fmt.Errorf("location %q: zone %s takes no index", s, l.Zone)
			}
		}
	}
	return l, nil
}

// Attachment roles within a stack.
const (
	AsPokemon = "pokemon"
	AsEnergy  = "energy"
	AsTool    = "tool"
)

// Special Conditions.
var Conditions = []string{"asleep", "burned", "confused", "paralyzed", "poisoned"}

// ValidCondition reports whether c is a Special Condition.
func ValidCondition(c string) bool {
	for _, k := range Conditions {
		if k == c {
			return true
		}
	}
	return false
}

// Stack is a Pokémon in play: its evolution chain (last is the current
// Pokémon), attachments, damage and Special Conditions.
type Stack struct {
	Pokemon    []IID    `json:"pokemon"`
	Energy     []IID    `json:"energy,omitempty"`
	Tools      []IID    `json:"tools,omitempty"`
	Damage     int      `json:"damage,omitempty"`
	Conditions []string `json:"conditions,omitempty"`
	// EnteredTurn is the turn the current Pokémon was put into play or
	// evolved, for evolution warnings.
	EnteredTurn int `json:"entered_turn,omitempty"`
}

// Empty reports whether the stack holds no cards.
func (s *Stack) Empty() bool {
	return s == nil || len(s.Pokemon)+len(s.Energy)+len(s.Tools) == 0
}

// Top returns the current Pokémon, or "" if none.
func (s *Stack) Top() IID {
	if s == nil || len(s.Pokemon) == 0 {
		return ""
	}
	return s.Pokemon[len(s.Pokemon)-1]
}

// Cards returns every card in the stack.
func (s *Stack) Cards() []IID {
	if s == nil {
		return nil
	}
	out := make([]IID, 0, len(s.Pokemon)+len(s.Energy)+len(s.Tools))
	out = append(out, s.Pokemon...)
	out = append(out, s.Energy...)
	out = append(out, s.Tools...)
	return out
}

func (s *Stack) clone() *Stack {
	if s == nil {
		return nil
	}
	c := *s
	c.Pokemon = append([]IID(nil), s.Pokemon...)
	c.Energy = append([]IID(nil), s.Energy...)
	c.Tools = append([]IID(nil), s.Tools...)
	c.Conditions = append([]string(nil), s.Conditions...)
	return &c
}

// HasCondition reports whether the stack has condition c.
func (s *Stack) HasCondition(c string) bool {
	if s == nil {
		return false
	}
	for _, k := range s.Conditions {
		if k == c {
			return true
		}
	}
	return false
}
