package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"
)

// The closed catalogue of events. docs/observability.md lists each with
// its fields; a new event needs a constant here and a row there.
const (
	// SignIn: a sign-in completes (either flow, any provider; Provider).
	SignIn = "sign_in"
	// SignInRefused: a sign-in is refused (Reason): a completed sign-in at
	// the user cap, or a password attempt ("credentials", "throttled";
	// Provider "password", no player).
	SignInRefused = "sign_in_refused"
	// SignOut: a board session signs out.
	SignOut = "sign_out"
	// PlayerDay: a player's first activity in a UTC day (see Active).
	PlayerDay = "player_day"
	// ToolCall: an MCP tool call reaches the tools (Tool, Assistant).
	ToolCall = "tool_call"
	// Import: import_log succeeds (Assistant).
	Import = "import"
	// NewGame: new_game succeeds (Assistant).
	NewGame = "new_game"
	// Error: a tool call or board action fails (Tool or Action, Message
	// redacted). Its level is "error".
	Error = "error"
	// Limited: the rate limit or user cap stops a request (Reason "rate"
	// or "places", Message).
	Limited = "limited"
	// GameExpired: an idle game is dropped (Idle).
	GameExpired = "game_expired"
	// AccountRemoved: an account's entry is removed (Reason "signed_out"
	// or "idle").
	AccountRemoved = "account_removed"
	// BoardRefused: a player opens another account's board link.
	BoardRefused = "board_refused"
	// AdminRefused: a non-admin or cross-site request reaches /admin
	// (Reason).
	AdminRefused = "admin_refused"
	// AdminAction: an admin checks card sets, clears images or looks up a
	// player (Action).
	AdminAction = "admin_action"
)

// Event is one record to write. Callers hand over the player's email,
// never an ID: the recorder hashes it into the record's player field and
// never writes the address itself.
type Event struct {
	Name      string // one of the event constants
	Email     string `json:"-"` // hashed into the player ID, never written
	Tool      string
	Action    string
	Assistant string
	Message   string
	Reason    string
	Idle      string
	Provider  string // the sign-in provider ("google" or "password"), on sign_in and a refused password attempt
}

// line is the record as written, in its fixed field order.
type line struct {
	Time      string `json:"time"`
	Level     string `json:"level"`
	Event     string `json:"event"`
	Player    string `json:"player,omitempty"`
	Tool      string `json:"tool,omitempty"`
	Action    string `json:"action,omitempty"`
	Assistant string `json:"assistant,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Message   string `json:"message,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Idle      string `json:"idle,omitempty"`
}

// Line is a record as it was written, handed to the recorder's sink: its
// fields, already hashed and redacted, and the exact JSON line (with its
// newline) written to the recorder's writer.
type Line struct {
	Time                                                   time.Time
	Level, Event, Player, Tool, Action, Assistant, Message string
	Reason, Idle, Provider                                 string
	JSON                                                   []byte
}

// Sink receives every record after it is written, with the context of the
// request it was recorded in (context.Background for background records).
// It must not block: it is called on the request path.
type Sink interface {
	Emit(ctx context.Context, l Line)
}

// timeFormat is RFC 3339 in UTC with milliseconds.
const timeFormat = "2006-01-02T15:04:05.000Z07:00"

// Recorder writes events as one JSON object per line. It holds, in memory
// only, the players already seen in the current UTC day. Every method on
// a nil *Recorder does nothing.
type Recorder struct {
	w   io.Writer
	ids *IDs
	now func() time.Time

	mu   sync.Mutex
	day  string
	seen map[string]struct{}
	sink Sink
}

// NewRecorder returns a recorder writing to w, naming players by ids, on
// the clock now (time.Now when nil).
func NewRecorder(w io.Writer, ids *IDs, now func() time.Time) *Recorder {
	if now == nil {
		now = time.Now
	}
	return &Recorder{w: w, ids: ids, now: now, seen: map[string]struct{}{}}
}

// ID returns the player ID of email, as records carry it.
func (r *Recorder) ID(email string) string {
	if r == nil {
		return ""
	}
	return r.ids.ID(email)
}

// SetSink makes the recorder hand every record it writes to s as well (nil
// for none). Hosted mode sets it once, before serving.
func (r *Recorder) SetSink(s Sink) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.sink = s
	r.mu.Unlock()
}

// Record writes e, as RecordContext with no request.
func (r *Recorder) Record(e Event) { r.RecordContext(context.Background(), e) }

// RecordContext writes e, recorded while serving the request ctx belongs
// to, and hands it to the sink with ctx. The email it carries is written
// only as its player ID; should the address appear in the message or
// reason, it is cut out too.
func (r *Recorder) RecordContext(ctx context.Context, e Event) {
	if r == nil {
		return
	}
	level := "info"
	if e.Name == Error {
		level = "error"
	}
	now := r.now().UTC()
	l := line{
		Time:      now.Format(timeFormat),
		Level:     level,
		Event:     e.Name,
		Player:    r.ids.ID(e.Email),
		Tool:      e.Tool,
		Action:    e.Action,
		Assistant: e.Assistant,
		Provider:  e.Provider,
		Message:   cut(e.Message, e.Email),
		Reason:    cut(e.Reason, e.Email),
		Idle:      e.Idle,
	}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	b = append(b, '\n')
	r.mu.Lock()
	r.w.Write(b)
	sink := r.sink
	r.mu.Unlock()
	// Outside the lock, so a slow sink never holds up other records.
	if sink != nil {
		sink.Emit(ctx, Line{
			Time: now, Level: l.Level, Event: l.Event, Player: l.Player, Tool: l.Tool, Action: l.Action,
			Assistant: l.Assistant, Message: l.Message, Reason: l.Reason, Idle: l.Idle, Provider: l.Provider,
			JSON: b,
		})
	}
}

// Active notes that the player with email was active. Their first
// activity in each UTC day writes a player_day record, so a day's count of
// player_day records is its number of distinct players (a restart during
// the day forgets who was seen, so a player active again is counted
// again).
func (r *Recorder) Active(email string) { r.ActiveContext(context.Background(), email) }

// ActiveContext is Active for activity in the request ctx belongs to, so
// a player_day record carries that request's trace.
func (r *Recorder) ActiveContext(ctx context.Context, email string) {
	if r == nil {
		return
	}
	id := r.ids.ID(email)
	if id == "" {
		return
	}
	day := r.now().UTC().Format("2006-01-02")
	r.mu.Lock()
	if day != r.day {
		r.day, r.seen = day, map[string]struct{}{}
	}
	_, seen := r.seen[id]
	r.seen[id] = struct{}{}
	r.mu.Unlock()
	if !seen {
		r.RecordContext(ctx, Event{Name: PlayerDay, Email: email})
	}
}

// cut removes every case-insensitive occurrence of email from s.
func cut(s, email string) string {
	email = strings.TrimSpace(email)
	if s == "" || email == "" {
		return s
	}
	lower, target := strings.ToLower(s), strings.ToLower(email)
	if !strings.Contains(lower, target) {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(lower, target)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		b.WriteString("…")
		s, lower = s[i+len(target):], lower[i+len(target):]
	}
}
