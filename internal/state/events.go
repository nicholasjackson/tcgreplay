package state

import (
	"encoding/json"
	"fmt"
)

// Actor is who caused an event.
type Actor string

const (
	ActorHuman  Actor = "human"
	ActorClaude Actor = "claude"
	ActorSystem Actor = "system"
	ActorImport Actor = "import"
)

// EventType names a primitive event.
type EventType string

const (
	EvSetup     EventType = "setup"
	EvMove      EventType = "move"
	EvSwap      EventType = "swap"
	EvShuffle   EventType = "shuffle"
	EvCoin      EventType = "coin"
	EvDamage    EventType = "damage"
	EvCondition EventType = "condition"
	EvEffect    EventType = "effect"
	EvReveal    EventType = "reveal"
	EvTurn      EventType = "turn"
	EvAssume    EventType = "assume"
	EvDeclare   EventType = "declare"
)

// Event is one primitive change to the state. Parent links make the log a
// tree; a branch is a pointer to a head event.
type Event struct {
	ID      int             `json:"id"`
	Parent  int             `json:"parent"`
	Action  string          `json:"action,omitempty"`
	Turn    int             `json:"turn"`
	Actor   Actor           `json:"actor"`
	Seat    Seat            `json:"seat,omitempty"`
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Decode unmarshals the payload into the type for e.Type.
func (e *Event) Decode() (any, error) {
	var p any
	switch e.Type {
	case EvSetup:
		p = &SetupPayload{}
	case EvMove:
		p = &MovePayload{}
	case EvSwap:
		p = &SwapPayload{}
	case EvShuffle:
		p = &ShufflePayload{}
	case EvCoin:
		p = &CoinPayload{}
	case EvDamage:
		p = &DamagePayload{}
	case EvCondition:
		p = &ConditionPayload{}
	case EvEffect:
		p = &EffectPayload{}
	case EvReveal:
		p = &RevealPayload{}
	case EvTurn:
		p = &TurnPayload{}
	case EvAssume:
		p = &AssumePayload{}
	case EvDeclare:
		p = &DeclarePayload{}
	default:
		return nil, fmt.Errorf("event %d: unknown type %q", e.ID, e.Type)
	}
	if err := json.Unmarshal(e.Payload, p); err != nil {
		return nil, fmt.Errorf("event %d (%s): %w", e.ID, e.Type, err)
	}
	return p, nil
}

// SetupPayload loads both decks. Cards are placed in each seat's deck in
// the order given; a shuffle event normally follows.
type SetupPayload struct {
	BenchSize  int             `json:"bench_size"`
	PrizeCount int             `json:"prize_count"`
	Names      map[Seat]string `json:"names,omitempty"`
	Cards      []Card          `json:"cards"`
	// Profile is the opponent deck profile for imported games.
	Profile json.RawMessage `json:"profile,omitempty"`
}

// MovePayload moves one card. From is recorded for readability and checked
// against the card's real location.
type MovePayload struct {
	IID  IID  `json:"iid"`
	From *Loc `json:"from,omitempty"`
	To   Loc  `json:"to"`
	// Face is "up" or "down"; it overrides the zone default for knowledge.
	Face string `json:"face,omitempty"`
	// As is the role in a stack: pokemon, energy or tool.
	As string `json:"as,omitempty"`
}

// SwapPayload exchanges two stacks of the same seat (e.g. a retreat). If B
// is empty, A simply moves there. Damage and conditions travel with the
// stack.
type SwapPayload struct {
	A Loc `json:"a"`
	B Loc `json:"b"`
}

// ShufflePayload records the resulting deck order, not a seed.
type ShufflePayload struct {
	Seat  Seat  `json:"seat"`
	Order []IID `json:"order"`
}

// CoinPayload records flip results: "heads" or "tails".
type CoinPayload struct {
	Results []string `json:"results"`
}

// DamagePayload sets the damage on a stack.
type DamagePayload struct {
	Target Loc `json:"target"`
	Value  int `json:"value"`
}

// ConditionPayload adds or clears a Special Condition. Condition "all" with
// On false clears every condition.
type ConditionPayload struct {
	Target    Loc    `json:"target"`
	Condition string `json:"condition"`
	On        bool   `json:"on"`
}

// EffectPayload adds or removes a lingering effect.
type EffectPayload struct {
	Op     string `json:"op"` // add or remove
	Effect Effect `json:"effect"`
}

// RevealPayload shows cards to seats.
type RevealPayload struct {
	IIDs []IID  `json:"iids"`
	To   []Seat `json:"to"`
}

// TurnPayload starts or ends a turn. The envelope seat is the turn player.
type TurnPayload struct {
	Phase  string `json:"phase"` // start or end
	Number int    `json:"number"`
}

// Assumption fills hidden information for one card.
type Assumption struct {
	Def   string `json:"def"`
	Name  string `json:"name"`
	Basis string `json:"basis"`
}

// AssumePayload fills hidden card identities, with provenance.
type AssumePayload struct {
	Cards map[IID]Assumption `json:"cards"`
	// Profile, if set, replaces the assumed opponent list the cards were
	// drawn from.
	Profile json.RawMessage `json:"profile,omitempty"`
}

// DeclarePayload records that a Pokémon used an Ability or attack, so the
// log shows it even when its effects are moved by hand or there are none.
// It changes nothing on the board.
type DeclarePayload struct {
	Kind string `json:"kind"` // ability or attack
	Name string `json:"name"`
	At   Loc    `json:"at"`
}
