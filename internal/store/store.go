// Package store holds one game's event tree in memory: events form a tree
// through parent links, and branches are named pointers to head events.
// Nothing is persisted; a game lasts only as long as the process. Nothing is
// ever deleted; rewinding just moves a branch pointer or creates a new one.
package store

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// Schema is the current header schema version.
const Schema = 1

// MainBranch is the branch a new game starts on.
const MainBranch = "main"

// Header describes a game's log.
type Header struct {
	Schema  int       `json:"schema"`
	Game    string    `json:"game"`
	Created time.Time `json:"created"`
	Source  string    `json:"source,omitempty"` // play, import or scenario
}

// Action groups the events of one player action.
type Action struct {
	ID     string      `json:"id"`
	Label  string      `json:"label"`
	Kind   string      `json:"kind,omitempty"` // trainer, ability, attack, verb, ...
	Seat   state.Seat  `json:"seat,omitempty"`
	Actor  state.Actor `json:"actor"`
	Source state.IID   `json:"source,omitempty"`
}

// Annotation is a comment on an event. It never alters state.
type Annotation struct {
	Event  int       `json:"event"`
	Author string    `json:"author"`
	Text   string    `json:"text"`
	Time   time.Time `json:"time"`
}

// Read audits a read of hidden information: which tool used which view.
type Read struct {
	Tool   string     `json:"tool"`
	View   string     `json:"view"`
	Viewer state.Seat `json:"viewer,omitempty"`
	Actor  string     `json:"actor"`
	Head   int        `json:"head"`
	Time   time.Time  `json:"time"`
}

// Log is one game's event tree.
type Log struct {
	Header      Header
	events      []state.Event // index id-1
	actions     map[string]*Action
	actionOrder []string
	branches    map[string]int
	current     string
	annotations []Annotation
	reads       []Read
}

// NewMemory returns a new, empty log held only in memory.
func NewMemory(h Header) *Log {
	if h.Schema == 0 {
		h.Schema = Schema
	}
	return &Log{
		Header:   h,
		actions:  map[string]*Action{},
		branches: map[string]int{MainBranch: 0},
		current:  MainBranch,
	}
}

func (l *Log) addAction(a *Action) {
	if _, ok := l.actions[a.ID]; !ok {
		l.actionOrder = append(l.actionOrder, a.ID)
	}
	l.actions[a.ID] = a
}

// Current returns the checked-out branch name.
func (l *Log) Current() string { return l.current }

// Head returns the head event id of the current branch (0 before any event).
func (l *Log) Head() int { return l.branches[l.current] }

// Branches returns a copy of the branch pointers.
func (l *Log) Branches() map[string]int {
	out := make(map[string]int, len(l.branches))
	for k, v := range l.branches {
		out[k] = v
	}
	return out
}

// NextID returns the id the next appended event will get.
func (l *Log) NextID() int { return len(l.events) + 1 }

// Event returns the event with id, or nil.
func (l *Log) Event(id int) *state.Event {
	if id < 1 || id > len(l.events) {
		return nil
	}
	return &l.events[id-1]
}

// Append adds events to the current branch. Each event's ID and Parent are
// assigned here; the input slice is updated in place.
func (l *Log) Append(evs []state.Event) error {
	for i := range evs {
		evs[i].ID = l.NextID()
		evs[i].Parent = l.Head()
		l.events = append(l.events, evs[i])
		l.branches[l.current] = evs[i].ID
	}
	return nil
}

// PutAction records an action's metadata.
func (l *Log) PutAction(a Action) error {
	l.addAction(&a)
	return nil
}

// Action returns an action by id.
func (l *Log) Action(id string) *Action { return l.actions[id] }

// NextActionID returns a fresh action id.
func (l *Log) NextActionID() string { return fmt.Sprintf("a-%d", len(l.actionOrder)+1) }

// Annotate adds an annotation.
func (l *Log) Annotate(a Annotation) error {
	if l.Event(a.Event) == nil {
		return fmt.Errorf("no event %d", a.Event)
	}
	l.annotations = append(l.annotations, a)
	return nil
}

// Annotations returns annotations on the given events (all if ids is nil).
func (l *Log) Annotations(ids map[int]bool) []Annotation {
	var out []Annotation
	for _, a := range l.annotations {
		if ids == nil || ids[a.Event] {
			out = append(out, a)
		}
	}
	return out
}

// LogRead appends a read audit record.
func (l *Log) LogRead(r Read) error {
	l.reads = append(l.reads, r)
	return nil
}

// Reads returns the read audit trail.
func (l *Log) Reads() []Read { return slices.Clone(l.reads) }

// PathTo returns the events from the root to head, in order.
func (l *Log) PathTo(head int) ([]state.Event, error) {
	var out []state.Event
	for id := head; id != 0; {
		e := l.Event(id)
		if e == nil {
			return nil, fmt.Errorf("no event %d", id)
		}
		out = append(out, *e)
		id = e.Parent
	}
	slices.Reverse(out)
	return out, nil
}

// Branch creates or moves a branch pointer to head.
func (l *Log) Branch(name string, head int) error {
	if name == "" {
		return errors.New("branch needs a name")
	}
	if head != 0 && l.Event(head) == nil {
		return fmt.Errorf("no event %d", head)
	}
	l.branches[name] = head
	return nil
}

// Checkout switches the current branch.
func (l *Log) Checkout(name string) error {
	if _, ok := l.branches[name]; !ok {
		return fmt.Errorf("no branch %q", name)
	}
	l.current = name
	return nil
}

// OnPath reports whether event id is an ancestor of (or equal to) head.
func (l *Log) OnPath(id, head int) bool {
	for cur := head; cur != 0; {
		if cur == id {
			return true
		}
		e := l.Event(cur)
		if e == nil {
			return false
		}
		cur = e.Parent
	}
	return false
}
