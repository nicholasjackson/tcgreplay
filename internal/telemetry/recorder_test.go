package telemetry

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// clock is a settable fake clock.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTestRecorder(t *testing.T, c *clock) (*Recorder, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	return NewRecorder(&buf, mustIDs(t, testKey), c.now), &buf
}

func lines(buf *bytes.Buffer) []string {
	return strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
}

// TestRecordLine writes each record as one JSON object on its own line,
// with time, level, event and player first, empty fields left out.
func TestRecordLine(t *testing.T) {
	c := &clock{time.Date(2026, 11, 12, 9, 30, 0, 123_000_000, time.FixedZone("CET", 3600))}
	r, buf := newTestRecorder(t, c)
	r.Record(Event{Name: ToolCall, Email: "ash@example.com", Tool: "import_log", Assistant: "Claude"})
	r.Record(Event{Name: Error, Email: "ash@example.com", Tool: "import_log", Message: `line 3: "…" before the Setup section`})
	r.Record(Event{Name: GameExpired, Email: "Ash@Example.com", Idle: "2h"})
	r.Record(Event{Name: Limited, Reason: "places", Message: "the server is busy"})
	want := []string{
		`{"time":"2026-11-12T08:30:00.123Z","level":"info","event":"tool_call","player":"p_94f4d9148dc444be","tool":"import_log","assistant":"Claude"}`,
		`{"time":"2026-11-12T08:30:00.123Z","level":"error","event":"error","player":"p_94f4d9148dc444be","tool":"import_log","message":"line 3: \"…\" before the Setup section"}`,
		`{"time":"2026-11-12T08:30:00.123Z","level":"info","event":"game_expired","player":"p_94f4d9148dc444be","idle":"2h"}`,
		`{"time":"2026-11-12T08:30:00.123Z","level":"info","event":"limited","message":"the server is busy","reason":"places"}`,
	}
	got := lines(buf)
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), buf)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got %s\nwant %s", i, got[i], want[i])
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(got[i]), &v); err != nil {
			t.Errorf("line %d is not JSON: %v", i, err)
		}
	}
}

// TestRecordNeverWritesEmail keeps the email handed over out of the
// output, even when a message or reason quotes it.
func TestRecordNeverWritesEmail(t *testing.T) {
	r, buf := newTestRecorder(t, &clock{time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)})
	email := "Ash.Ketchum@example.com"
	for _, name := range []string{SignIn, SignInRefused, SignOut, ToolCall, Import, NewGame, Error, Limited, GameExpired, AccountRemoved, BoardRefused, AdminRefused, AdminAction} {
		r.Record(Event{Name: name, Email: email, Tool: "t", Action: "a", Assistant: "Claude", Message: "refused " + email + " and ash.ketchum@EXAMPLE.com", Reason: "for " + email})
	}
	r.Active(email)
	out := strings.ToLower(buf.String())
	for _, s := range []string{"ash.ketchum@example.com", "ash.ketchum", "@example.com"} {
		if strings.Contains(out, s) {
			t.Errorf("output contains %q:\n%s", s, buf)
		}
	}
	if n := strings.Count(buf.String(), `"player":"p_`); n != 14 {
		t.Errorf("%d records carry a player ID, want 14", n)
	}
}

// TestActiveOncePerDay writes player_day once per player per UTC day, and
// again on the next UTC day.
func TestActiveOncePerDay(t *testing.T) {
	c := &clock{time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}
	r, buf := newTestRecorder(t, c)
	r.Active("ash@example.com")
	r.Active("ASH@example.com")
	r.Active("misty@example.com")
	c.t = time.Date(2026, 11, 12, 23, 59, 59, 0, time.UTC)
	r.Active("ash@example.com")
	r.Active("") // no player, no record
	// 00:30 the next day in UTC, though still the 12th in New York.
	c.t = time.Date(2026, 11, 12, 19, 30, 0, 0, time.FixedZone("EST", -5*3600))
	r.Active("ash@example.com")
	r.Active("ash@example.com")
	want := []string{
		`{"time":"2026-11-12T09:00:00.000Z","level":"info","event":"player_day","player":"p_94f4d9148dc444be"}`,
		`{"time":"2026-11-12T09:00:00.000Z","level":"info","event":"player_day","player":"p_e356af17f70f9e00"}`,
		`{"time":"2026-11-13T00:30:00.000Z","level":"info","event":"player_day","player":"p_94f4d9148dc444be"}`,
	}
	got := lines(buf)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestRecorderID gives the admin lookup the same ID the records carry.
func TestRecorderID(t *testing.T) {
	r, _ := newTestRecorder(t, &clock{})
	if got := r.ID(" ASH@example.com"); got != "p_94f4d9148dc444be" {
		t.Errorf("ID = %q", got)
	}
}

// TestNilRecorder records nothing and never panics.
func TestNilRecorder(t *testing.T) {
	var r *Recorder
	r.Record(Event{Name: SignIn, Email: "ash@example.com"})
	r.Active("ash@example.com")
	if got := r.ID("ash@example.com"); got != "" {
		t.Errorf("nil recorder ID = %q", got)
	}
}
