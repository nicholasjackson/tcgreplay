// Package store persists a game as an append-only JSONL log: events form a
// tree through parent links, and branches are named pointers to head events.
// Nothing is ever deleted; rewinding just moves a branch pointer or creates a
// new one.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// Schema is the current file schema version.
const Schema = 1

// MainBranch is the branch a new game starts on.
const MainBranch = "main"

// Header is the first line of a game file.
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

// record is one JSONL line.
type record struct {
	Kind       string       `json:"kind"`
	Header     *Header      `json:"header,omitempty"`
	Event      *state.Event `json:"event,omitempty"`
	Branch     string       `json:"branch,omitempty"`
	Head       *int         `json:"head,omitempty"`
	Action     *Action      `json:"action,omitempty"`
	Annotation *Annotation  `json:"annotation,omitempty"`
	Read       *Read        `json:"read,omitempty"`
}

// Log is one game's event tree.
type Log struct {
	Header      Header
	path        string
	f           *os.File
	events      []state.Event // index id-1
	actions     map[string]*Action
	actionOrder []string
	branches    map[string]int
	current     string
	annotations []Annotation
	reads       []Read
}

// Create starts a new game file at path.
func Create(path string, h Header) (*Log, error) {
	if h.Schema == 0 {
		h.Schema = Schema
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	l := newLog(path, h)
	l.f = f
	if err := l.write(record{Kind: "header", Header: &h}); err != nil {
		f.Close()
		return nil, err
	}
	return l, nil
}

// NewMemory returns a log that is never persisted, for tests and scratch.
func NewMemory(h Header) *Log {
	if h.Schema == 0 {
		h.Schema = Schema
	}
	return newLog("", h)
}

func newLog(path string, h Header) *Log {
	return &Log{
		Header:   h,
		path:     path,
		actions:  map[string]*Action{},
		branches: map[string]int{MainBranch: 0},
		current:  MainBranch,
	}
}

// Open loads an existing game file and keeps it open for appending.
func Open(path string) (*Log, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var l *Log
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	line := 0
	for sc.Scan() {
		line++
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var r record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if l == nil {
			if r.Kind != "header" || r.Header == nil {
				return nil, fmt.Errorf("%s: first line is not a header", path)
			}
			if r.Header.Schema > Schema {
				return nil, fmt.Errorf("%s: schema %d is newer than supported %d", path, r.Header.Schema, Schema)
			}
			l = newLog(path, *r.Header)
			continue
		}
		if err := l.replay(r); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if l == nil {
		return nil, fmt.Errorf("%s: empty file", path)
	}
	l.f, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Log) replay(r record) error {
	switch r.Kind {
	case "event":
		if r.Event == nil {
			return errors.New("event record without event")
		}
		if r.Event.ID != len(l.events)+1 {
			return fmt.Errorf("event id %d out of sequence", r.Event.ID)
		}
		l.events = append(l.events, *r.Event)
		if r.Branch != "" {
			l.branches[r.Branch] = r.Event.ID
		}
	case "branch":
		if r.Head == nil {
			return errors.New("branch record without head")
		}
		l.branches[r.Branch] = *r.Head
	case "checkout":
		l.current = r.Branch
		if _, ok := l.branches[r.Branch]; !ok {
			return fmt.Errorf("checkout of unknown branch %q", r.Branch)
		}
	case "action":
		if r.Action == nil {
			return errors.New("action record without action")
		}
		l.addAction(r.Action)
	case "annotation":
		if r.Annotation == nil {
			return errors.New("annotation record without annotation")
		}
		l.annotations = append(l.annotations, *r.Annotation)
	case "read":
		if r.Read != nil {
			l.reads = append(l.reads, *r.Read)
		}
	default:
		return fmt.Errorf("unknown record kind %q", r.Kind)
	}
	return nil
}

func (l *Log) addAction(a *Action) {
	if _, ok := l.actions[a.ID]; !ok {
		l.actionOrder = append(l.actionOrder, a.ID)
	}
	l.actions[a.ID] = a
}

func (l *Log) write(r record) error {
	if l.f == nil {
		return nil
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := l.f.Write(b); err != nil {
		return err
	}
	return l.f.Sync()
}

// Close closes the underlying file.
func (l *Log) Close() error {
	if l.f == nil {
		return nil
	}
	return l.f.Close()
}

// Path returns the file path, empty for memory logs.
func (l *Log) Path() string { return l.path }

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
		if err := l.write(record{Kind: "event", Event: &evs[i], Branch: l.current}); err != nil {
			return err
		}
		l.events = append(l.events, evs[i])
		l.branches[l.current] = evs[i].ID
	}
	return nil
}

// PutAction records an action's metadata.
func (l *Log) PutAction(a Action) error {
	if err := l.write(record{Kind: "action", Action: &a}); err != nil {
		return err
	}
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
	if err := l.write(record{Kind: "annotation", Annotation: &a}); err != nil {
		return err
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
	if err := l.write(record{Kind: "read", Read: &r}); err != nil {
		return err
	}
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
	if err := l.write(record{Kind: "branch", Branch: name, Head: &head}); err != nil {
		return err
	}
	l.branches[name] = head
	return nil
}

// Checkout switches the current branch.
func (l *Log) Checkout(name string) error {
	if _, ok := l.branches[name]; !ok {
		return fmt.Errorf("no branch %q", name)
	}
	if err := l.write(record{Kind: "checkout", Branch: name}); err != nil {
		return err
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
