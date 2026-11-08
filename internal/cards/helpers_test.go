package cards

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// waitFor polls cond until it holds, failing the test after d.
func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// fakeClock is a clock tests move by hand.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// testEpoch is the fake clock's starting time.
var testEpoch = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

// logBuffer collects log lines.
type logBuffer struct {
	mu    sync.Mutex
	lines []string
}

func (l *logBuffer) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

// count is how many lines contain s.
func (l *logBuffer) count(s string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, line := range l.lines {
		if strings.Contains(line, s) {
			n++
		}
	}
	return n
}

// outageModes are the ways the shared store can fail. Each sets the
// failure up and returns how to end it.
var outageModes = map[string]func(srv *spacestest.Server) (heal func()){
	"unreachable": func(srv *spacestest.Server) func() { srv.Close(); return nil },
	"refused":     func(srv *spacestest.Server) func() { srv.SetDeny(true); return func() { srv.SetDeny(false) } },
	"failing":     func(srv *spacestest.Server) func() { srv.SetFail(true); return func() { srv.SetFail(false) } },
	"hanging":     func(srv *spacestest.Server) func() { return srv.Hang() },
}
