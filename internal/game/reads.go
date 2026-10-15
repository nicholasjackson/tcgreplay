package game

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/store"
)

// cardLabel renders a card for logs, hiding it unless viewer may see it.
func cardLabel(st *state.State, iid state.IID, visible bool) string {
	if !visible {
		return string(iid) + " (hidden)"
	}
	return fmt.Sprintf("%s %s", iid, st.Name(iid))
}

func stackName(st *state.State, l state.Loc) string {
	s, err := st.Stack(l)
	if err != nil || s.Top() == "" {
		return l.String()
	}
	return fmt.Sprintf("%s (%s)", l, st.Name(s.Top()))
}

// describe renders one event as text. before and after are the states
// around it; viewer "" means no redaction.
func describe(e *state.Event, before, after *state.State, viewer state.Seat) string {
	p, err := e.Decode()
	if err != nil {
		return err.Error()
	}
	who := string(e.Seat)
	if who == "" {
		who = string(e.Actor)
	}
	switch p := p.(type) {
	case *state.SetupPayload:
		n := map[state.Seat]int{}
		for _, c := range p.Cards {
			n[c.Owner]++
		}
		return fmt.Sprintf("setup: p1 %d cards, p2 %d cards, bench %d, prizes %d", n[state.P1], n[state.P2], p.BenchSize, p.PrizeCount)
	case *state.MovePayload:
		from := "?"
		if p.From != nil {
			from = p.From.String()
		}
		owner := after.Cards[p.IID].Owner
		visible := viewer == "" ||
			(p.From != nil && p.From.Zone.IsPublic()) || p.To.Zone.IsPublic() ||
			after.KnownTo(p.IID, viewer) || before.KnownTo(p.IID, viewer)
		to := p.To
		if to.Seat == "" && to.Zone != state.Stadium {
			to.Seat = owner
		}
		if p.From != nil && p.From.Zone == state.Deck && p.From.Pos != nil && *p.From.Pos == state.Top && p.To.Zone == state.Hand {
			return fmt.Sprintf("%s drew %s", owner, cardLabel(after, p.IID, visible))
		}
		if p.From != nil && p.From.Zone == state.Prizes && p.To.Zone == state.Hand {
			return fmt.Sprintf("%s took prize %s", owner, cardLabel(after, p.IID, visible))
		}
		s := fmt.Sprintf("%s moved %s %s → %s", who, cardLabel(after, p.IID, visible), from, to)
		if p.As != "" && p.As != state.AsPokemon {
			s += " as " + p.As
		}
		if p.Face != "" {
			s += " face " + p.Face
		}
		return s
	case *state.SwapPayload:
		return fmt.Sprintf("%s swapped %s with %s", who, stackName(before, p.A), stackName(before, p.B))
	case *state.ShufflePayload:
		return fmt.Sprintf("%s shuffled %s's deck", who, p.Seat)
	case *state.CoinPayload:
		return fmt.Sprintf("%s flipped: %s", who, strings.Join(p.Results, ", "))
	case *state.DamagePayload:
		old := 0
		if s, err := before.Stack(p.Target); err == nil && s != nil {
			old = s.Damage
		}
		return fmt.Sprintf("%s set damage on %s: %d → %d", who, stackName(before, p.Target), old, p.Value)
	case *state.ConditionPayload:
		if p.On {
			return fmt.Sprintf("%s made %s %s", who, stackName(before, p.Target), p.Condition)
		}
		return fmt.Sprintf("%s cleared %s on %s", who, p.Condition, stackName(before, p.Target))
	case *state.EffectPayload:
		if p.Op == "remove" {
			return fmt.Sprintf("%s removed effect %s", who, p.Effect.ID)
		}
		s := fmt.Sprintf("%s added effect %s on %s: %s", who, p.Effect.ID, p.Effect.Seat, p.Effect.Text)
		if p.Effect.UntilTurn > 0 {
			s += fmt.Sprintf(" (until end of turn %d)", p.Effect.UntilTurn)
		}
		return s
	case *state.RevealPayload:
		names := make([]string, len(p.IIDs))
		for i, iid := range p.IIDs {
			names[i] = cardLabel(after, iid, true)
		}
		return fmt.Sprintf("%s revealed %s", who, strings.Join(names, ", "))
	case *state.TurnPayload:
		return fmt.Sprintf("turn %d %s (%s)", p.Number, p.Phase, e.Seat)
	case *state.AssumePayload:
		return fmt.Sprintf("assumed identities for %d hidden cards", len(p.Cards))
	}
	return string(e.Type)
}

// LogLine is one rendered event.
type LogLine struct {
	ID          int      `json:"id"`
	Turn        int      `json:"turn"`
	Seat        string   `json:"seat,omitempty"`
	Action      string   `json:"action,omitempty"`
	Label       string   `json:"label,omitempty"`
	Text        string   `json:"text"`
	Annotations []string `json:"annotations,omitempty"`
}

func (l LogLine) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "#%d T%d", l.ID, l.Turn)
	if l.Action != "" {
		fmt.Fprintf(&b, " [%s %s]", l.Action, l.Label)
	}
	b.WriteString(" " + l.Text)
	for _, a := range l.Annotations {
		b.WriteString("\n    ✎ " + a)
	}
	return b.String()
}

// Log renders events on the current branch after event since, redacted for
// viewer ("" for none).
func (g *Game) Log(since int, viewer state.Seat, tool string, actor string) ([]LogLine, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.audit(tool, viewer, actor)
	path, err := g.log.PathTo(g.log.Head())
	if err != nil {
		return nil, err
	}
	st := state.New()
	var out []LogLine
	ids := map[int]bool{}
	for i := range path {
		e := &path[i]
		before := st.Clone()
		if err := st.Apply(e); err != nil {
			return nil, err
		}
		if e.ID <= since {
			continue
		}
		ids[e.ID] = true
		line := LogLine{ID: e.ID, Turn: e.Turn, Seat: string(e.Seat), Action: e.Action, Text: describe(e, before, st, viewer)}
		if a := g.log.Action(e.Action); a != nil {
			line.Label = a.Label
		}
		out = append(out, line)
	}
	notes := map[int][]string{}
	for _, a := range g.log.Annotations(ids) {
		notes[a.Event] = append(notes[a.Event], a.Author+": "+a.Text)
	}
	for i := range out {
		out[i].Annotations = notes[out[i].ID]
	}
	return out, nil
}

func (g *Game) audit(tool string, viewer state.Seat, actor string) {
	view := "seat"
	if viewer == "" {
		view = "reveal"
	}
	g.log.LogRead(store.Read{Tool: tool, View: view, Viewer: viewer, Actor: actor, Head: g.log.Head(), Time: time.Now().UTC()})
}

// View projects the current state. viewer "" is the reveal view, which is
// only available in review mode.
func (g *Game) View(viewer state.Seat, tool, actor string) (state.BoardView, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if viewer == "" && g.mode != ModeReview {
		return state.BoardView{}, fmt.Errorf("the reveal view is only available in review mode")
	}
	g.audit(tool, viewer, actor)
	v := state.ViewSeat
	if viewer == "" {
		v = state.ViewReveal
	}
	return g.st.Project(v, viewer, g.hp), nil
}

// DeckCard is a card found by searching or looking at a deck.
type DeckCard struct {
	IID      state.IID `json:"iid"`
	Name     string    `json:"name"`
	Def      string    `json:"def,omitempty"`
	Category string    `json:"category,omitempty"`
	Kind     string    `json:"kind,omitempty"` // stage, trainer type or energy type
}

func (g *Game) deckCard(iid state.IID) DeckCard {
	c := g.st.Cards[iid]
	dc := DeckCard{IID: iid, Name: c.Name, Def: c.Def}
	if cc := g.card(g.st, iid); cc != nil {
		dc.Category = cc.Category
		dc.Kind = cmp.Or(cc.Stage, cc.TrainerType, cc.EnergyType)
		if cc.Category == "Pokemon" {
			dc.Kind += " " + strings.Join(cc.Types, "/")
		}
	}
	return dc
}

// SearchDeck lists whose deck sorted by category and name, never in true
// order. filter matches name, category, stage, trainer type, energy type or
// Pokémon type, case-insensitively; several words must all match.
func (g *Game) SearchDeck(whose state.Seat, filter, tool, actor string) ([]DeckCard, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	side, ok := g.st.Sides[whose]
	if !ok {
		return nil, fmt.Errorf("unknown seat %q", whose)
	}
	g.audit(tool, whose, actor)
	words := strings.Fields(strings.ToLower(filter))
	var out []DeckCard
	for _, iid := range side.Deck {
		dc := g.deckCard(iid)
		hay := strings.ToLower(strings.Join([]string{dc.Name, dc.Category, dc.Kind}, " "))
		match := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				match = false
				break
			}
		}
		if match {
			out = append(out, dc)
		}
	}
	order := map[string]int{"Pokemon": 0, "Trainer": 1, "Energy": 2}
	slices.SortFunc(out, func(a, b DeckCard) int {
		return cmp.Or(cmp.Compare(order[a.Category], order[b.Category]), cmp.Compare(a.Name, b.Name), cmp.Compare(a.IID, b.IID))
	})
	return out, nil
}

// LookTop returns the top n cards of whose deck, top first.
func (g *Game) LookTop(whose state.Seat, n int, tool, actor string) ([]DeckCard, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	side, ok := g.st.Sides[whose]
	if !ok {
		return nil, fmt.Errorf("unknown seat %q", whose)
	}
	g.audit(tool, whose, actor)
	n = min(n, len(side.Deck))
	out := make([]DeckCard, 0, n)
	for _, iid := range side.Deck[:n] {
		out = append(out, g.deckCard(iid))
	}
	return out, nil
}

// Warnings returns invariant violations and soft rule warnings for the
// current turn.
func (g *Game) Warnings() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append(g.st.Check(), g.turnWarnings()...)
}

// turnWarnings scans the current turn for soft rule breaches.
func (g *Game) turnWarnings() []string {
	if g.st.Phase != state.PhasePlaying {
		return nil
	}
	seat := g.st.Current
	var energy, retreats int
	supporters, stadiums := map[string]bool{}, map[string]bool{}
	for id := g.log.Head(); id != 0; {
		e := g.log.Event(id)
		if e == nil {
			break
		}
		id = e.Parent
		if e.Type == state.EvTurn {
			break
		}
		a := g.log.Action(e.Action)
		if a == nil || a.Seat != seat {
			continue
		}
		switch a.Kind {
		case "attach":
			if e.Type == state.EvMove && strings.Contains(string(e.Payload), `"as":"energy"`) {
				energy++
			}
		case "retreat":
			if e.Type == state.EvSwap {
				retreats++
			}
		case "trainer":
			if c := g.card(g.st, a.Source); c != nil {
				switch c.TrainerType {
				case "Supporter":
					supporters[a.ID] = true
				case "Stadium":
					stadiums[a.ID] = true
				}
			}
		}
	}
	var w []string
	if energy > 1 {
		w = append(w, fmt.Sprintf("%s attached %d Energy from hand this turn", seat, energy))
	}
	if len(supporters) > 1 {
		w = append(w, fmt.Sprintf("%s played %d Supporters this turn", seat, len(supporters)))
	}
	if len(supporters) > 0 && g.st.Turn == 1 {
		w = append(w, "the player going first cannot play a Supporter on their first turn")
	}
	if len(stadiums) > 1 {
		w = append(w, fmt.Sprintf("%s played %d Stadiums this turn", seat, len(stadiums)))
	}
	if retreats > 1 {
		w = append(w, fmt.Sprintf("%s retreated %d times this turn", seat, retreats))
	}
	return w
}

// BranchInfo describes a branch.
type BranchInfo struct {
	Name    string `json:"name"`
	Head    int    `json:"head"`
	Current bool   `json:"current"`
	Turn    int    `json:"turn"`
	Last    string `json:"last,omitempty"`
}

// Branches lists branches.
func (g *Game) Branches() []BranchInfo {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []BranchInfo
	for name, head := range g.log.Branches() {
		bi := BranchInfo{Name: name, Head: head, Current: name == g.log.Current()}
		if e := g.log.Event(head); e != nil {
			bi.Turn = e.Turn
			if a := g.log.Action(e.Action); a != nil {
				bi.Last = a.Label
			}
		}
		out = append(out, bi)
	}
	slices.SortFunc(out, func(a, b BranchInfo) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

// Rewind creates branch name at eventID and checks it out. Nothing is
// deleted: the previous branch keeps its head.
func (g *Game) Rewind(eventID int, name string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.log.Event(eventID) == nil {
		return "", fmt.Errorf("no event %d", eventID)
	}
	if name == "" {
		for i := 1; ; i++ {
			name = fmt.Sprintf("b-%d", i)
			if _, taken := g.log.Branches()[name]; !taken {
				break
			}
		}
	} else if _, taken := g.log.Branches()[name]; taken {
		return "", fmt.Errorf("branch %q already exists", name)
	}
	if err := g.log.Branch(name, eventID); err != nil {
		return "", err
	}
	if err := g.checkoutLocked(name); err != nil {
		return "", err
	}
	return name, nil
}

// Checkout switches to an existing branch.
func (g *Game) Checkout(name string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.checkoutLocked(name)
}

func (g *Game) checkoutLocked(name string) error {
	prev := g.log.Current()
	if err := g.log.Checkout(name); err != nil {
		return err
	}
	if err := g.refold(); err != nil {
		g.log.Checkout(prev)
		g.refold()
		return err
	}
	g.open = nil
	u := Update{Head: g.log.Head(), Branch: name}
	for _, f := range g.listeners {
		f(u)
	}
	return nil
}

// Annotate attaches a comment to an event.
func (g *Game) Annotate(eventID int, author, text string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.log.Annotate(store.Annotation{Event: eventID, Author: author, Text: text, Time: time.Now().UTC()})
}

// Head returns the current head event id.
func (g *Game) Head() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.log.Head()
}
