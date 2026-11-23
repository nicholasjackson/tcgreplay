package observe

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/nicholasjackson/tcgreplay/internal/observe/observetest"
	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// TestSinkSendsEachRecordAsALogRecord: a record becomes a log record whose
// body is the written line, with its fields as attributes, its severity
// from its level and the trace and span of the context it was recorded in.
func TestSinkSendsEachRecordAsALogRecord(t *testing.T) {
	r := observetest.New(t)
	tel := Start(context.Background(), on(r.URL, r.Client()))
	t.Cleanup(func() { tel.Shutdown(context.Background()) })

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36},
		SpanID:     trace.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	const errLine = `{"time":"2026-11-23T09:35:40.871Z","level":"error","event":"error","player":"p_6cef43a09af39824","tool":"import_log","message":"line 1: \"…\" before the Setup section"}`
	const signIn = `{"time":"2026-11-23T09:30:00.123Z","level":"info","event":"sign_in","player":"p_6cef43a09af39824","provider":"google"}`
	sink := tel.Sink()
	sink.Emit(ctx, telemetry.Line{Time: time.Now(), Level: "error", Event: "error", Player: "p_6cef43a09af39824", Tool: "import_log",
		Message: `line 1: "…" before the Setup section`, JSON: []byte(errLine + "\n")})
	sink.Emit(context.Background(), telemetry.Line{Time: time.Now(), Level: "info", Event: "sign_in", Player: "p_6cef43a09af39824",
		Provider: "google", JSON: []byte(signIn + "\n")})
	tel.ForceFlush(context.Background())

	logs := r.Logs(t)
	if len(logs) != 2 {
		t.Fatalf("receiver got %d log records, want 2: %+v", len(logs), logs)
	}
	e, s := logs[0], logs[1]
	if e.Body != errLine || s.Body != signIn {
		t.Errorf("bodies = %q, %q\nwant the written lines without their newline", e.Body, s.Body)
	}
	if e.Severity != "ERROR" || s.Severity != "INFO" {
		t.Errorf("severities = %q, %q, want ERROR, INFO", e.Severity, s.Severity)
	}
	if e.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" || e.SpanID != "00f067aa0ba902b7" {
		t.Errorf("error record trace %q span %q, want the recording context's", e.TraceID, e.SpanID)
	}
	if s.TraceID != "" {
		t.Errorf("a record with no request carries trace %q", s.TraceID)
	}
	wantE := map[string]string{"log.source": "record", "event": "error", "player": "p_6cef43a09af39824", "tool": "import_log"}
	wantS := map[string]string{"log.source": "record", "event": "sign_in", "player": "p_6cef43a09af39824", "provider": "google"}
	for _, c := range []struct {
		got, want map[string]string
	}{{e.Attrs, wantE}, {s.Attrs, wantS}} {
		if len(c.got) != len(c.want) {
			t.Errorf("attributes = %v, want %v", c.got, c.want)
		}
		for k, v := range c.want {
			if c.got[k] != v {
				t.Errorf("attribute %s = %q, want %q (all: %v)", k, c.got[k], v, c.got)
			}
		}
	}
	if e.Resource["service.name"] != "tcgreplay" || e.Resource["deployment.environment.name"] != "production" {
		t.Errorf("resource = %v", e.Resource)
	}
}

// TestLogWriterSendsEachLine: each std log line becomes a log record, its
// date prefix dropped, at WARN for "warning:" lines; partial lines wait
// for their newline and an endless line is cut.
func TestLogWriterSendsEachLine(t *testing.T) {
	r := observetest.New(t)
	tel := Start(context.Background(), on(r.URL, r.Client()))
	t.Cleanup(func() { tel.Shutdown(context.Background()) })
	w := tel.LogWriter()

	io.WriteString(w, "2026/11/23 09:30:00 tcgreplay v1 hosted: listening on [::]:8080\n")
	io.WriteString(w, "2026/11/23 09:30:01 warning: card images: TCGREPLAY_SPACES_BUCKET is not set\ncard sets: ")
	io.WriteString(w, "3 loaded\n")
	io.WriteString(w, strings.Repeat("x", 100<<10)+"\n")
	tel.ForceFlush(context.Background())

	logs := r.Logs(t)
	if len(logs) != 4 {
		t.Fatalf("receiver got %d log records, want 4", len(logs))
	}
	want := []struct{ body, sev string }{
		{"tcgreplay v1 hosted: listening on [::]:8080", "INFO"},
		{"warning: card images: TCGREPLAY_SPACES_BUCKET is not set", "WARN"},
		{"card sets: 3 loaded", "INFO"},
	}
	for i, w := range want {
		if logs[i].Body != w.body || logs[i].Severity != w.sev || logs[i].Attrs["log.source"] != "log" {
			t.Errorf("record %d = %q %s %v, want %q %s log.source=log", i, logs[i].Body, logs[i].Severity, logs[i].Attrs, w.body, w.sev)
		}
	}
	if n := len(logs[3].Body); n != 64<<10 {
		t.Errorf("a 100 KB line was sent as %d bytes, want it cut to 64 KB", n)
	}
}

// TestSinkAndWriterOffWhenExportOff: with export off there is no sink and
// the log writer discards.
func TestSinkAndWriterOffWhenExportOff(t *testing.T) {
	tel := Start(context.Background(), Settings{})
	if tel.Sink() != nil {
		t.Error("Sink() is not nil with export off")
	}
	if tel.LogWriter() != io.Discard {
		t.Error("LogWriter() is not io.Discard with export off")
	}
}
