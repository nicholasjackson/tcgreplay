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
	Slot        string     `json:"slot"`
	Pokemon     []CardView `json:"pokemon"`
	Energy      []CardView `json:"energy,omitempty"`
	Tools       []CardView `json:"tools,omitempty"`
	Damage      int        `json:"damage,omitempty"`
	HP          int        `json:"hp,omitempty"`
	HPLeft      *int       `json:"hp_left,omitempty"`
	Conditions  []string   `json:"conditions,omitempty"`
	EnteredTurn int        `json:"entered_turn,omitempty"`
}

// PrizeView is one prize slot.
type PrizeView struct {
	Slot int       `json:"slot"`
	Card *CardView `json:"card,omitempty"`
}

// SideView is one seat's half of the mat.
type SideView struct {
	Name      string       `json:"name,omitempty"`
	DeckCount int          `json:"deck_count"`
	Deck      []CardView   `json:"deck,omitempty"` // reveal view only, top first
	Prizes    []PrizeView  `json:"prizes"`
	PrizesLeft int         `json:"prizes_left"`
	HandCount int          `json:"hand_count"`
	Hand      []CardView   `json:"hand"`
	Discard   []CardView   `json:"discard"`
	Lost      []CardView   `json:"lost,omitempty"`
	Active    *StackView   `json:"active,omitempty"`
	Bench     []*StackView `json:"bench"`
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
	Effects  []Effect          `json:"effects,omitempty"`
	Coins    []string          `json:"last_coins,omitempty"`
	Sides    map[Seat]SideView `json:"sides"`
}

// HPFunc returns a card's printed HP, or 0 if unknown.
type HPFunc func(def string) int

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
			if h := hp(s.Cards[st.Top()].Def); h > 0 {
				left := h - st.Damage
				v.HP, v.HPLeft = h, &left
			}
		}
		return v
	}

	bv := BoardView{
		View: view, Phase: s.Phase, Turn: s.Turn, Current: s.Current,
		TurnOpen: s.TurnOpen, First: s.First, Head: s.Head,
		Effects: s.Effects, Coins: s.Coins, Sides: map[Seat]SideView{},
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
