package cards

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTCGdex is a stand-in for the TCGdex API. It serves bodies by path
// ("/sets", "/sets/<id>", "/cards/<id>"), counts requests per path, and
// can be made slow, failing, held open or unreachable.
type fakeTCGdex struct {
	*httptest.Server

	mu     sync.Mutex
	bodies map[string]string
	count  map[string]int
	delay  time.Duration
	fail   bool
	gate   chan struct{} // while set, set-detail requests wait for it to close
	closed bool
}

func newFakeTCGdex(t *testing.T) *fakeTCGdex {
	t.Helper()
	f := &fakeTCGdex{bodies: map[string]string{}, count: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(func() {
		f.release()
		f.close()
	})
	return f
}

func (f *fakeTCGdex) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.count[r.URL.Path]++
	body, ok := f.bodies[r.URL.Path]
	delay, fail, gate := f.delay, f.fail, f.gate
	f.mu.Unlock()
	if gate != nil && strings.HasPrefix(r.URL.Path, "/sets/") {
		select {
		case <-gate:
		case <-r.Context().Done():
			return
		}
	}
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	switch {
	case fail:
		http.Error(w, "boom", http.StatusInternalServerError)
	case !ok:
		http.NotFound(w, r)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}
}

// put serves body at path.
func (f *fakeTCGdex) put(path, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies[path] = body
}

// hits is how many requests path has had.
func (f *fakeTCGdex) hits(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count[path]
}

// totalHits is how many requests the fake has had on any path.
func (f *fakeTCGdex) totalHits() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.count {
		n += c
	}
	return n
}

// setDelay makes every response wait d.
func (f *fakeTCGdex) setDelay(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delay = d
}

// setFail makes every response a 500.
func (f *fakeTCGdex) setFail(fail bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = fail
}

// hold makes set-detail requests wait until release.
func (f *fakeTCGdex) hold() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gate = make(chan struct{})
}

// release lets held set-detail requests through.
func (f *fakeTCGdex) release() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gate != nil {
		close(f.gate)
		f.gate = nil
	}
}

// close makes the fake unreachable: later requests fail to connect.
func (f *fakeTCGdex) close() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.closed = true
	f.mu.Unlock()
	f.Server.Close()
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

// newTestClient returns a client on a fresh cache directory, talking to f,
// with a fake clock and a short source timeout.
func newTestClient(t *testing.T, f *fakeTCGdex) (*Client, *fakeClock) {
	t.Helper()
	return newTestClientDir(t, t.TempDir(), f)
}

// newTestClientDir is newTestClient on a cache directory the test has
// already seeded.
func newTestClientDir(t *testing.T, dir string, f *fakeTCGdex) (*Client, *fakeClock) {
	t.Helper()
	clock := &fakeClock{t: testEpoch}
	c := NewClient(dir)
	c.Base = f.URL
	c.Now = clock.Now
	c.Timeout = 300 * time.Millisecond
	return c, clock
}

// seedEnvelope caches body under key as this release writes it, fetched at
// fetched.
func seedEnvelope(t *testing.T, dir, key, body string, fetched time.Time) {
	t.Helper()
	if err := NewDiskStore(dir).Put(context.Background(), key, Entry{Data: []byte(body), Fetched: fetched}); err != nil {
		t.Fatal(err)
	}
}

// seedLegacy caches body under key as the previous release wrote it: the
// bare body, with mtime as the file's modification time.
func seedLegacy(t *testing.T, dir, key, body string, mtime time.Time) {
	t.Helper()
	p := diskStore{dir: dir}.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}
