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
	turnStart := map[state.IID]int{} // damage when the current turn began
	for i := range path {
		e := &path[i]
		before := st.Clone()
		if err := st.Apply(e); err != nil {
			return nil, err
		}
		if e.Type == state.EvTurn && st.TurnOpen {
			turnStart = st.StackDamage()
		}
		ids[e.ID] = true
		text := describe(e, before, st, viewer)
		line := LogLine{ID: e.ID, Turn: e.Turn, Seat: string(e.Seat), Action: e.Action, Text: text}
		if a := g.log.Action(e.Action); a != nil {
			line.Label = a.Label
		}
		snap.Log = append(snap.Log, line)
		if want[e.ID] {
			f := Frame{
				Event: e.ID, Actor: e.Actor, Text: text,
				Touched: touched(e, before), Board: st.Project(view, viewer, g.hp),
			}
			f.Board.SetTurnDamage(turnStart)
			snap.Frames = append(snap.Frames, f)
		}
	}
	snap.Board.SetTurnDamage(turnStart)
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
	case *state.DeclarePayload:
		return top(p.At)
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

// ReplayStep is one action in a replay: what it did, any notes on it, and
// the board after it.
type ReplayStep struct {
	Action  string          `json:"action,omitempty"`
	Label   string          `json:"label,omitempty"`
	Seat    state.Seat      `json:"seat,omitempty"`
	Actor   state.Actor     `json:"actor"`
	First   int             `json:"first"`
	Last    int             `json:"last"`
	Lines   []string        `json:"lines"`
	Notes   []string        `json:"notes,omitempty"`
	Touched []state.IID     `json:"touched,omitempty"`
	Board   state.BoardView `json:"board"`
}

// Replay is the board right after event from, then one step per action up
// to event to, on the current branch: for playing moves back at a pace the
// viewer can follow. viewer "" is the reveal view (review mode only).
type Replay struct {
	From  int             `json:"from"`
	To    int             `json:"to"`
	Start state.BoardView `json:"start"`
	Steps []ReplayStep    `json:"steps"`
}

// maxReplaySteps bounds a replay's size; each step carries a board.
const maxReplaySteps = 400

// Replay builds the replay of the current branch between from and to
// (event ids on it; 0 is the beginning, to 0 the head).
func (g *Game) Replay(viewer state.Seat, from, to int) (*Replay, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if viewer == "" && g.mode != ModeReview {
		return nil, fmt.Errorf("the reveal view is only available in review mode")
	}
	if to == 0 {
		to = g.log.Head()
	}
	path, err := g.log.PathTo(g.log.Head())
	if err != nil {
		return nil, err
	}
	view := state.ViewSeat
	if viewer == "" {
		view = state.ViewReveal
	}
	project := func(st *state.State, turnStart map[state.IID]int) state.BoardView {
		b := st.Project(view, viewer, g.hp)
		b.SetTurnDamage(turnStart)
		return b
	}
	notes := map[int][]string{}
	ids := map[int]bool{}
	for _, e := range path {
		if e.ID > from && e.ID <= to {
			ids[e.ID] = true
		}
	}
	for _, a := range g.log.Annotations(ids) {
		notes[a.Event] = append(notes[a.Event], a.Text)
	}

	r := &Replay{From: from, To: to}
	st := state.New()
	turnStart := map[state.IID]int{}
	started := from == 0
	if started {
		r.Start = project(st, turnStart)
	}
	var cur *ReplayStep
	for i := range path {
		e := &path[i]
		before := st.Clone()
		if err := st.Apply(e); err != nil {
			return nil, err
		}
		if e.Type == state.EvTurn && st.TurnOpen {
			turnStart = st.StackDamage()
		}
		if e.ID <= from {
			if e.ID == from {
				r.Start, started = project(st, turnStart), true
			}
			continue
		}
		if e.ID > to {
			break
		}
		if !started {
			return nil, fmt.Errorf("event %d is not on branch %s", from, g.log.Current())
		}
		// Consecutive events of one action are one step.
		if cur == nil || e.Action == "" || e.Action != cur.Action {
			if len(r.Steps) == maxReplaySteps {
				return nil, fmt.Errorf("more than %d moves between events %d and %d; replay a shorter stretch", maxReplaySteps, from, to)
			}
			r.Steps = append(r.Steps, ReplayStep{Action: e.Action, Seat: e.Seat, Actor: e.Actor, First: e.ID})
			cur = &r.Steps[len(r.Steps)-1]
			if a := g.log.Action(e.Action); a != nil {
				cur.Label, cur.Seat = a.Label, a.Seat
			}
		}
		cur.Last = e.ID
		cur.Lines = append(cur.Lines, describe(e, before, st, viewer))
		cur.Notes = append(cur.Notes, notes[e.ID]...)
		cur.Touched = append(cur.Touched, touched(e, before)...)
		cur.Board = project(st, turnStart)
	}
	if !started {
		return nil, fmt.Errorf("event %d is not on branch %s", from, g.log.Current())
	}
	return r, nil
}
