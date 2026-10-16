package game

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// UISnapshot is the board and timeline as the playmat shows them. Unlike
// View and Log it is not audited: it is the human's own view of the mat.
type UISnapshot struct {
	Game       string          `json:"game"`
	Branch     string          `json:"branch"`
	Mode       Mode            `json:"mode"`
	Head       int             `json:"head"`
	OpenAction string          `json:"open_action,omitempty"`
	Board      state.BoardView `json:"board"`
	Log        []LogLine       `json:"log"`
	Branches   []BranchInfo    `json:"branches"`
	Warnings   []string        `json:"warnings,omitempty"`
	Frames     []Frame         `json:"frames,omitempty"`
}

// Frame is the board right after one event, for step-by-step animation.
type Frame struct {
	Event   int             `json:"event"`
	Actor   state.Actor     `json:"actor"`
	Text    string          `json:"text"`
	Touched []state.IID     `json:"touched,omitempty"`
	Board   state.BoardView `json:"board"`
}

// UISnapshot projects the current state for viewer ("" for the reveal view,
// review mode only). Frames are built for the listed events if they lie on
// the current branch.
func (g *Game) UISnapshot(viewer state.Seat, frameEvents []int) (*UISnapshot, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if viewer == "" && g.mode != ModeReview {
		return nil, fmt.Errorf("the reveal view is only available in review mode")
	}
	view := state.ViewSeat
	if viewer == "" {
		view = state.ViewReveal
	}
	snap := &UISnapshot{
		Game:     g.ID,
		Branch:   g.log.Current(),
		Mode:     g.mode,
		Head:     g.log.Head(),
		Board:    g.st.Project(view, viewer, g.hp),
		Warnings: append(g.st.Check(), g.turnWarnings()...),
	}
	if g.open != nil {
		snap.OpenAction = g.open.ID
	}
	want := map[int]bool{}
	for _, id := range frameEvents {
		want[id] = true
	}
	path, err := g.log.PathTo(g.log.Head())
	if err != nil {
		return nil, err
	}
	st := state.New()
	ids := map[int]bool{}
	for i := range path {
		e := &path[i]
		before := st.Clone()
		if err := st.Apply(e); err != nil {
			return nil, err
		}
		ids[e.ID] = true
		text := describe(e, before, st, viewer)
		line := LogLine{ID: e.ID, Turn: e.Turn, Seat: string(e.Seat), Action: e.Action, Text: text}
		if a := g.log.Action(e.Action); a != nil {
			line.Label = a.Label
		}
		snap.Log = append(snap.Log, line)
		if want[e.ID] {
			snap.Frames = append(snap.Frames, Frame{
				Event: e.ID, Actor: e.Actor, Text: text,
				Touched: touched(e, before), Board: st.Project(view, viewer, g.hp),
			})
		}
	}
	notes := map[int][]string{}
	for _, a := range g.log.Annotations(ids) {
		notes[a.Event] = append(notes[a.Event], a.Author+": "+a.Text)
	}
	for i := range snap.Log {
		snap.Log[i].Annotations = notes[snap.Log[i].ID]
	}
	for name, head := range g.log.Branches() {
		bi := BranchInfo{Name: name, Head: head, Current: name == g.log.Current()}
		if e := g.log.Event(head); e != nil {
			bi.Turn = e.Turn
			if a := g.log.Action(e.Action); a != nil {
				bi.Last = a.Label
			}
		}
		snap.Branches = append(snap.Branches, bi)
	}
	slices.SortFunc(snap.Branches, func(a, b BranchInfo) int { return cmp.Compare(a.Name, b.Name) })
	return snap, nil
}

// touched lists the cards an event affects, for highlighting.
func touched(e *state.Event, before *state.State) []state.IID {
	p, err := e.Decode()
	if err != nil {
		return nil
	}
	top := func(l state.Loc) []state.IID {
		if s, err := before.Stack(l); err == nil && s.Top() != "" {
			return []state.IID{s.Top()}
		}
		return nil
	}
	switch p := p.(type) {
	case *state.MovePayload:
		return []state.IID{p.IID}
	case *state.SwapPayload:
		return append(top(p.A), top(p.B)...)
	case *state.DamagePayload:
		return top(p.Target)
	case *state.ConditionPayload:
		return top(p.Target)
	case *state.RevealPayload:
		return p.IIDs
	}
	return nil
}

// TurnStartedAt returns the id of the event just before seat's latest turn
// started on the current branch, so a prompt can say "get_log since N" and
// cover that whole turn. It returns 0 if seat has not had a turn.
func (g *Game) TurnStartedAt(seat state.Seat) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	for id := g.log.Head(); id != 0; {
		e := g.log.Event(id)
		if e == nil {
			break
		}
		if e.Type == state.EvTurn && e.Seat == seat {
			if p, err := e.Decode(); err == nil && p.(*state.TurnPayload).Phase == "start" {
				return e.Parent
			}
		}
		id = e.Parent
	}
	return 0
}
