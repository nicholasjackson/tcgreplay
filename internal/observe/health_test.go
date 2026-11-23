package observe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/observe/observetest"
)

// clock is a fake clock the tests move by hand.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) set(t time.Time) { c.mu.Lock(); c.t = t; c.mu.Unlock() }

// syncBuf is a goroutine-safe stderr stand-in.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }
func (s *syncBuf) lines() int                  { return strings.Count(s.String(), "\n") }

var t0 = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

// leakyErr carries the token and the Basic value in its text, as an
// exporter error can carry the endpoint and response body.
var leakyErr = fmt.Errorf("failed to send to https://x/otlp: 401 Unauthorized (body: token %s Basic MTIzNDU2OmdsY19TRUNSRVRUT0tFTnZhbHVlMTIz)", testToken)

func TestHealthWarnsAfterFiveMinutesAtMostEveryFive(t *testing.T) {
	c := &clock{t: t0}
	var out syncBuf
	h := newHealth(c.now, &out)
	h.sawStatus("logs", http.StatusUnauthorized)
	h.note("logs", leakyErr)

	for _, step := range []struct {
		at    time.Duration
		lines int
	}{
		{4*time.Minute + 59*time.Second, 0},
		{5 * time.Minute, 1},
		{5*time.Minute + 30*time.Second, 1},
		{9 * time.Minute, 1},
		{9*time.Minute + 59*time.Second, 1},
		{10 * time.Minute, 2},
		{14 * time.Minute, 2},
		{15 * time.Minute, 3},
	} {
		h.check(t0.Add(step.at))
		if got := out.lines(); got != step.lines {
			t.Fatalf("at %v: %d warning lines, want %d\n%s", step.at, got, step.lines, out.String())
		}
	}
	first := strings.SplitN(out.String(), "\n", 2)[0]
	const want = "warning: telemetry export: sending to Grafana Cloud has failed for 5m (logs: rejected (HTTP 401))"
	if first != want {
		t.Errorf("warning = %q\nwant      %q", first, want)
	}
	for _, secret := range []string{testToken, "MTIzNDU2OmdsY19TRUNSRVRUT0tFTnZhbHVlMTIz", "Unauthorized", "body:"} {
		if strings.Contains(out.String(), secret) {
			t.Errorf("warning carries %q:\n%s", secret, out.String())
		}
	}
}

func TestHealthStopsAfterASuccess(t *testing.T) {
	var out syncBuf
	h := newHealth(func() time.Time { return t0 }, &out)
	h.note("traces", context.DeadlineExceeded)
	h.check(t0.Add(6 * time.Minute))
	if out.lines() != 1 {
		t.Fatalf("want one warning before the success, got:\n%s", out.String())
	}
	h.note("traces", nil)
	h.check(t0.Add(12 * time.Minute))
	h.check(t0.Add(20 * time.Minute))
	if out.lines() != 1 {
		t.Fatalf("warnings continued after a successful send:\n%s", out.String())
	}
}

func TestHealthOnlyContinuousFailureCounts(t *testing.T) {
	c := &clock{t: t0}
	var out syncBuf
	h := newHealth(c.now, &out)
	h.note("logs", context.DeadlineExceeded)
	c.set(t0.Add(3 * time.Minute))
	h.note("logs", nil)
	c.set(t0.Add(4 * time.Minute))
	h.note("logs", context.DeadlineExceeded)
	h.check(t0.Add(8 * time.Minute)) // 4m of unbroken failure
	if out.lines() != 0 {
		t.Fatalf("warned before five minutes of unbroken failure:\n%s", out.String())
	}
	h.check(t0.Add(9 * time.Minute))
	if out.lines() != 1 {
		t.Fatalf("no warning after five minutes of unbroken failure")
	}
}

func TestHealthGroupsSignalsByKind(t *testing.T) {
	var out syncBuf
	h := newHealth(func() time.Time { return t0 }, &out)
	unreachable := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	h.note("traces", unreachable)
	h.note("logs", unreachable)
	h.note("metrics", context.DeadlineExceeded)
	h.check(t0.Add(7 * time.Minute))
	const want = "warning: telemetry export: sending to Grafana Cloud has failed for 7m (logs, traces: unreachable; metrics: timeout)\n"
	if out.String() != want {
		t.Fatalf("warning = %q\nwant      %q", out.String(), want)
	}
}

func TestKind(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		want   string
	}{
		{context.DeadlineExceeded, 0, "timeout"},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), 0, "timeout"},
		{&net.OpError{Op: "dial", Err: errors.New("refused")}, 0, "unreachable"},
		{&net.DNSError{Err: "no such host", Name: "x"}, 0, "unreachable"},
		{leakyErr, 401, "rejected (HTTP 401)"},
		{errors.New("retry-able request failure"), 503, "rejected (HTTP 503)"},
		{errors.New("something else"), 200, "failed"},
	} {
		if got := kind(tc.err, tc.status); got != tc.want {
			t.Errorf("kind(%v, %d) = %q, want %q", tc.err, tc.status, got, tc.want)
		}
	}
}

// TestHealthSeesRealExportFailures runs real exporters against a refusing
// receiver and a fake clock: the failure line appears only after five
// minutes, and names the HTTP status but neither the token nor the header.
func TestHealthSeesRealExportFailures(t *testing.T) {
	reject := observetest.New(t, observetest.Status(http.StatusUnauthorized))
	c := &clock{t: t0}
	var out syncBuf
	s := on(reject.URL, reject.Client())
	s.Now, s.Stderr = c.now, &out
	tel := Start(context.Background(), s)
	t.Cleanup(func() { tel.Shutdown(context.Background()) })

	sendOne(tel)
	tel.ForceFlush(context.Background())
	tel.health.check(t0.Add(4 * time.Minute))
	if out.lines() != 0 {
		t.Fatalf("warned before five minutes:\n%s", out.String())
	}
	tel.health.check(t0.Add(5 * time.Minute))
	got := out.String()
	if !strings.HasPrefix(got, "warning: telemetry export: sending to Grafana Cloud has failed for 5m (") ||
		!strings.Contains(got, "logs") || !strings.Contains(got, "traces") || !strings.Contains(got, "rejected (HTTP 401)") {
		t.Fatalf("warning = %q", got)
	}
	for _, secret := range []string{testToken, "MTIzNDU2OmdsY19TRUNSRVRUT0tFTnZhbHVlMTIz", reject.URL} {
		if strings.Contains(got, secret) {
			t.Errorf("warning carries %q: %q", secret, got)
		}
	}
}
