package state

// View names a projection of the state.
type View string

const (
	// ViewSeat shows what one seat could legitimately see.
	ViewSeat View = "seat"
	// ViewReveal applies no redaction.
	ViewReveal View = "reveal"
)

// CardView is a card as a viewer sees it. Hidden cards carry only their iid.
type CardView struct {
	IID    IID    `json:"iid"`
	Name   string `json:"name,omitempty"`
	Def    string `json:"def,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
	Basis  string `json:"basis,omitempty"`
}

// StackView is a Pokémon in play.
type StackView struct {
	Slot    string     `json:"slot"`
	Pokemon []CardView `json:"pokemon"`
	Energy  []CardView `json:"energy,omitempty"`
	Tools   []CardView `json:"tools,omitempty"`
	Damage  int        `json:"damage,omitempty"`
	// DamageTurn is the damage put on it since the current turn began.
	DamageTurn  int      `json:"damage_turn,omitempty"`
	HP          int      `json:"hp,omitempty"`
	HPLeft      *int     `json:"hp_left,omitempty"`
	Conditions  []string `json:"conditions,omitempty"`
	EnteredTurn int      `json:"entered_turn,omitempty"`
}

// PrizeView is one prize slot.
type PrizeView struct {
	Slot int       `json:"slot"`
	Card *CardView `json:"card,omitempty"`
}

// SideView is one seat's half of the mat.
type SideView struct {
	Name       string       `json:"name,omitempty"`
	DeckCount  int          `json:"deck_count"`
	Deck       []CardView   `json:"deck,omitempty"` // reveal view only, top first
	Prizes     []PrizeView  `json:"prizes"`
	PrizesLeft int          `json:"prizes_left"`
	HandCount  int          `json:"hand_count"`
	Hand       []CardView   `json:"hand"`
	Discard    []CardView   `json:"discard"`
	Lost       []CardView   `json:"lost,omitempty"`
	Active     *StackView   `json:"active,omitempty"`
	Bench      []*StackView `json:"bench"`
}

// BoardView is a projection of the whole state.
type BoardView struct {
	View     View              `json:"view"`
	Viewer   Seat              `json:"viewer,omitempty"`
	Phase    string            `json:"phase"`
	Turn     int               `json:"turn"`
	Current  Seat              `json:"current,omitempty"`
	TurnOpen bool              `json:"turn_open"`
	First    Seat              `json:"first,omitempty"`
	Head     int               `json:"head"`
	Stadium  *CardView         `json:"stadium,omitempty"`
	Effects  []EffectView      `json:"effects,omitempty"`
	Coins    []string          `json:"last_coins,omitempty"`
	Sides    map[Seat]SideView `json:"sides"`
}

// EffectView is a lingering effect as shown: UntilTurn is the combined turn
// and UntilSeat the player whose turn it ends on.
type EffectView struct {
	Effect
	UntilSeat Seat `json:"until_seat,omitempty"`
}

// StackDamage returns the damage on every Pokémon in play, keyed by the
// stack's first card, which stays put through evolution and switching.
func (s *State) StackDamage() map[IID]int {
	out := map[IID]int{}
	for _, seat := range Seats {
		for _, l := range s.StackLocs(seat) {
			st, _ := s.Stack(l)
			if len(st.Pokemon) > 0 {
				out[st.Pokemon[0]] = st.Damage
			}
		}
	}
	return out
}

// SetTurnDamage fills each stack's DamageTurn from the damage it had when
// the turn began (StackDamage then). A Pokémon put into play since counts
// from zero; healing counts as none.
func (b *BoardView) SetTurnDamage(start map[IID]int) {
	set := func(sv *StackView) {
		if sv == nil || len(sv.Pokemon) == 0 {
			return
		}
		sv.DamageTurn = max(sv.Damage-start[sv.Pokemon[0].IID], 0)
	}
	for seat, side := range b.Sides {
		set(side.Active)
		for _, sv := range side.Bench {
			set(sv)
		}
		b.Sides[seat] = side
	}
}

// HPFunc returns the maximum HP of a Pokémon whose current card is pokemon
// with tools attached (definition ids), or 0 if unknown.
type HPFunc func(pokemon string, tools []string) int

// Project builds a view. For ViewSeat, viewer selects the seat.
func (s *State) Project(view View, viewer Seat, hp HPFunc) BoardView {
	reveal := view == ViewReveal
	see := func(iid IID) CardView {
		c := s.Cards[iid]
		if reveal || s.KnownTo(iid, viewer) {
			return CardView{IID: iid, Name: c.Name, Def: c.Def, Basis: c.Basis}
		}
		return CardView{IID: iid, Hidden: true}
	}
	many := func(xs []IID) []CardView {
		out := make([]CardView, 0, len(xs))
		for _, x := range xs {
			out = append(out, see(x))
		}
		return out
	}
	stack := func(slot string, st *Stack) *StackView {
		if st.Empty() {
			return nil
		}
		v := &StackView{
			Slot:        slot,
			Pokemon:     many(st.Pokemon),
			Energy:      many(st.Energy),
			Tools:       many(st.Tools),
			Damage:      st.Damage,
			Conditions:  st.Conditions,
			EnteredTurn: st.EnteredTurn,
		}
		if hp != nil && st.Top() != "" {
			tools := make([]string, len(st.Tools))
			for i, t := range st.Tools {
				tools[i] = s.Cards[t].Def
			}
			if h := hp(s.Cards[st.Top()].Def, tools); h > 0 {
				left := h - st.Damage
				v.HP, v.HPLeft = h, &left
			}
		}
		return v
	}

	bv := BoardView{
		View: view, Phase: s.Phase, Turn: Round(s.Turn), Current: s.Current,
		TurnOpen: s.TurnOpen, First: s.First, Head: s.Head,
		Coins: s.Coins, Sides: map[Seat]SideView{},
	}
	for _, e := range s.Effects {
		ev := EffectView{Effect: e}
		if e.UntilTurn > 0 {
			ev.UntilTurn, ev.UntilSeat = Round(e.UntilTurn), s.TurnSeat(e.UntilTurn)
		}
		bv.Effects = append(bv.Effects, ev)
	}
	if !reveal {
		bv.Viewer = viewer
	}
	if s.Stadium != "" {
		c := see(s.Stadium)
		bv.Stadium = &c
	}
	for _, seat := range Seats {
		side := s.Sides[seat]
		sv := SideView{
			Name:       side.Name,
			DeckCount:  len(side.Deck),
			PrizesLeft: side.PrizesLeft(),
			HandCount:  len(side.Hand),
			Hand:       []CardView{},
			Discard:    many(side.Discard),
			Lost:       many(side.Lost),
			Active:     stack("active", side.Active),
			Bench:      make([]*StackView, len(side.Bench)),
			Prizes:     []PrizeView{},
		}
		if reveal {
			sv.Deck = many(side.Deck)
		}
		for i, p := range side.Prizes {
			pv := PrizeView{Slot: i}
			if p != "" {
				c := see(p)
				pv.Card = &c
			}
			sv.Prizes = append(sv.Prizes, pv)
		}
		// A hand is listed card by card only when the viewer can see some of
		// it; otherwise the count says enough.
		for _, h := range side.Hand {
			c := see(h)
			if !c.Hidden {
				sv.Hand = append(sv.Hand, c)
			}
		}
		for i, b := range side.Bench {
			sv.Bench[i] = stack(benchSlot(i), b)
		}
		bv.Sides[seat] = sv
	}
	return bv
}

func benchSlot(i int) string {
	return Loc{Zone: Bench, Slot: i}.String()
}
