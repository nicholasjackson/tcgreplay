package telemetry

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// collect is a Sink that keeps what it is handed.
type collect struct {
	mu    sync.Mutex
	lines []Line
	ctxs  []context.Context
}

func (c *collect) Emit(ctx context.Context, l Line) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, l)
	c.ctxs = append(c.ctxs, ctx)
}

type ctxKey struct{}

// TestSinkGetsTheWrittenLine: the sink receives every record with exactly
// the bytes written, its fields as written and the caller's context.
func TestSinkGetsTheWrittenLine(t *testing.T) {
	c := &clock{time.Date(2026, 11, 23, 9, 30, 0, 123_000_000, time.UTC)}
	r, buf := newTestRecorder(t, c)
	var s collect
	r.SetSink(&s)
	ctx := context.WithValue(context.Background(), ctxKey{}, "request")
	r.RecordContext(ctx, Event{Name: SignIn, Email: "ash@example.com", Provider: "google"})
	r.Record(Event{Name: Error, Email: "ash@example.com", Tool: "import_log", Message: "bad log from ash@example.com"})
	r.ActiveContext(ctx, "misty@example.com")

	want := []string{
		`{"time":"2026-11-23T09:30:00.123Z","level":"info","event":"sign_in","player":"p_94f4d9148dc444be","provider":"google"}`,
		`{"time":"2026-11-23T09:30:00.123Z","level":"error","event":"error","player":"p_94f4d9148dc444be","tool":"import_log","message":"bad log from …"}`,
	}
	got := lines(buf)
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("written:\n%s\nwant first two:\n%s", buf, strings.Join(want, "\n"))
	}
	if len(s.lines) != 3 {
		t.Fatalf("sink got %d lines, want 3", len(s.lines))
	}
	for i, l := range s.lines {
		if string(l.JSON) != got[i]+"\n" {
			t.Errorf("sink line %d JSON = %q, want the written line %q", i, l.JSON, got[i]+"\n")
		}
		if strings.Contains(string(l.JSON)+l.Message+l.Reason+l.Player, "example.com") {
			t.Errorf("sink line %d carries an email: %+v", i, l)
		}
	}
	first := s.lines[0]
	if first.Event != SignIn || first.Level != "info" || first.Player != "p_94f4d9148dc444be" || first.Provider != "google" || !first.Time.Equal(c.t) {
		t.Errorf("sink line 0 fields = %+v", first)
	}
	if e := s.lines[1]; e.Level != "error" || e.Tool != "import_log" || e.Message != "bad log from …" {
		t.Errorf("sink line 1 fields = %+v", e)
	}
	if s.lines[2].Event != PlayerDay {
		t.Errorf("ActiveContext wrote %q, want player_day", s.lines[2].Event)
	}
	if s.ctxs[0].Value(ctxKey{}) != "request" || s.ctxs[2].Value(ctxKey{}) != "request" {
		t.Error("RecordContext and ActiveContext did not hand the sink their context")
	}
	if s.ctxs[1].Value(ctxKey{}) != nil {
		t.Error("Record handed the sink a request context")
	}
}

// TestNoSinkAndNilRecorder: without a sink records are written as before,
// SetSink(nil) removes it, and a nil recorder ignores every call.
func TestNoSinkAndNilRecorder(t *testing.T) {
	r, buf := newTestRecorder(t, &clock{time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)})
	var s collect
	r.SetSink(&s)
	r.SetSink(nil)
	r.RecordContext(context.Background(), Event{Name: SignOut, Email: "ash@example.com"})
	if len(s.lines) != 0 || !strings.Contains(buf.String(), `"event":"sign_out"`) {
		t.Fatalf("sink got %d lines, written %q", len(s.lines), buf)
	}
	var nilRec *Recorder
	nilRec.SetSink(&s)
	nilRec.RecordContext(context.Background(), Event{Name: SignIn})
	nilRec.ActiveContext(context.Background(), "ash@example.com")
}
