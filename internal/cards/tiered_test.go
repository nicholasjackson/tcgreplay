package cards

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// logBuffer collects log lines from a tiered store.
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

type tieredFixture struct {
	dir   string
	local Store
	srv   *spacestest.Server
	store *tieredStore
	clock *fakeClock
	log   *logBuffer
}

// newTiered builds a tiered store over a fresh disk store and a stand-in
// Spaces, with a fake clock, a 200ms shared timeout and captured logs.
func newTiered(t *testing.T) *tieredFixture {
	t.Helper()
	f := &tieredFixture{dir: t.TempDir(), srv: spacestest.New(t), clock: &fakeClock{t: testEpoch}, log: &logBuffer{}}
	f.local = NewDiskStore(f.dir)
	f.store = NewTieredStore(f.local, NewSpacesStore(f.srv.Client(t))).(*tieredStore)
	f.store.now = f.clock.Now
	f.store.timeout = 200 * time.Millisecond
	f.store.logf = f.log.logf
	return f
}

// seedShared puts an entry in the stand-in Spaces only.
func (f *tieredFixture) seedShared(t *testing.T, key string, e Entry) {
	t.Helper()
	b, err := encodeEnvelope(key, e)
	if err != nil {
		t.Fatal(err)
	}
	f.srv.SetObject(objectKey(key), b)
}

var oldFetch = time.Date(2026, 10, 1, 8, 15, 30, 987654321, time.UTC)

func TestTieredSharedOnlyEntryIsCopiedToLocalWithItsFetchTime(t *testing.T) {
	f := newTiered(t)
	ctx := context.Background()
	f.seedShared(t, "cards/tst-1", Entry{Data: []byte(`{"id":"tst-1"}`), Fetched: oldFetch})

	e, err := f.store.Get(ctx, "cards/tst-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(e.Data) != `{"id":"tst-1"}` || !e.Fetched.Equal(oldFetch) {
		t.Errorf("Get = %s @ %v, want the shared entry @ %v", e.Data, e.Fetched, oldFetch)
	}
	le, err := f.local.Get(ctx, "cards/tst-1")
	if err != nil {
		t.Fatalf("local copy: %v", err)
	}
	if !le.Fetched.Equal(oldFetch) || string(le.Data) != `{"id":"tst-1"}` {
		t.Errorf("local copy = %s @ %v, want fetched %v", le.Data, le.Fetched, oldFetch)
	}
}

func TestTieredLocalHitMakesNoSharedRequest(t *testing.T) {
	f := newTiered(t)
	ctx := context.Background()
	if err := f.local.Put(ctx, "sets", Entry{Data: []byte(`[]`), Fetched: oldFetch}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Get(ctx, "sets"); err != nil {
		t.Fatal(err)
	}
	if n := f.srv.Total(); n != 0 {
		t.Errorf("shared requests = %d, want 0", n)
	}
}

func TestTieredMissInBothIsNotCached(t *testing.T) {
	f := newTiered(t)
	if _, err := f.store.Get(context.Background(), "cards/none"); !errors.Is(err, ErrNotCached) {
		t.Errorf("Get = %v, want ErrNotCached", err)
	}
	if f.log.count("unavailable") != 0 {
		t.Error("a miss was logged as an outage")
	}
}

func TestTieredPutWritesBothTiers(t *testing.T) {
	f := newTiered(t)
	ctx := context.Background()
	e := Entry{Data: []byte(`{"id":"sv01"}`), Fetched: oldFetch}
	if err := f.store.Put(ctx, "sets/sv01", e); err != nil {
		t.Fatal(err)
	}
	if le, err := f.local.Get(ctx, "sets/sv01"); err != nil || !le.Fetched.Equal(oldFetch) {
		t.Errorf("local: %v @ %v", err, le.Fetched)
	}
	if _, ok := f.srv.Object("tcgdex/sets/sv01.json"); !ok {
		t.Error("shared store has no tcgdex/sets/sv01.json")
	}
}

func TestTieredLocalReturnsLocalTier(t *testing.T) {
	f := newTiered(t)
	if f.store.Local() != f.local {
		t.Error("Local() is not the local store")
	}
}

// outageModes are the ways the shared store can fail. Each sets the
// failure up and returns how to end it.
var outageModes = map[string]func(srv *spacestest.Server) (heal func()){
	"unreachable": func(srv *spacestest.Server) func() { srv.Close(); return nil },
	"refused":     func(srv *spacestest.Server) func() { srv.SetDeny(true); return func() { srv.SetDeny(false) } },
	"failing":     func(srv *spacestest.Server) func() { srv.SetFail(true); return func() { srv.SetFail(false) } },
	"hanging":     func(srv *spacestest.Server) func() { return srv.Hang() },
}

func TestTieredAbsorbsSharedOutages(t *testing.T) {
	for name, mode := range outageModes {
		t.Run(name, func(t *testing.T) {
			f := newTiered(t)
			ctx := context.Background()
			f.seedShared(t, "cards/tst-1", Entry{Data: []byte(`{"id":"tst-1"}`), Fetched: oldFetch})
			heal := mode(f.srv)
			if heal != nil {
				defer heal()
			}

			start := time.Now()
			_, err := f.store.Get(ctx, "cards/tst-1")
			if !errors.Is(err, ErrNotCached) {
				t.Errorf("Get = %v, want ErrNotCached", err)
			}
			if d := time.Since(start); d > time.Second {
				t.Errorf("Get took %v, want no more than the 200ms shared timeout", d)
			}
			e := Entry{Data: []byte(`{"id":"tst-2"}`), Fetched: testEpoch}
			if err := f.store.Put(ctx, "cards/tst-2", e); err != nil {
				t.Errorf("Put = %v, want nil", err)
			}
			if _, err := f.local.Get(ctx, "cards/tst-2"); err != nil {
				t.Errorf("Put did not reach disk: %v", err)
			}
			if n := f.log.count("shared store unavailable"); n != 1 {
				t.Errorf("unavailable logged %d times, want 1: %q", n, f.log.lines)
			}
		})
	}
}

func TestTieredHangDelaysOnlyTheFirstRequest(t *testing.T) {
	f := newTiered(t)
	ctx := context.Background()
	release := f.srv.Hang()
	defer release()

	start := time.Now()
	f.store.Get(ctx, "cards/a")
	if d := time.Since(start); d < 150*time.Millisecond || d > time.Second {
		t.Errorf("first Get took %v, want about the 200ms timeout", d)
	}
	start = time.Now()
	for _, k := range []string{"cards/b", "cards/c", "sets"} {
		f.store.Get(ctx, k)
		f.store.Put(ctx, k, Entry{Data: []byte(`{}`), Fetched: testEpoch})
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("later requests took %v, want no wait", d)
	}
}

func TestTieredBreakerSkipsSharedForAMinuteThenRetries(t *testing.T) {
	f := newTiered(t)
	ctx := context.Background()
	f.srv.SetFail(true)

	f.store.Get(ctx, "cards/a") // trips the breaker
	if n := f.srv.Total(); n != 1 {
		t.Fatalf("shared requests = %d, want 1", n)
	}
	f.clock.Advance(59 * time.Second)
	f.store.Get(ctx, "cards/b")
	f.store.Put(ctx, "cards/b", Entry{Data: []byte(`{}`), Fetched: testEpoch})
	if n := f.srv.Total(); n != 1 {
		t.Errorf("shared requests inside the window = %d, want still 1", n)
	}

	// A failed probe after the window re-arms it without logging again.
	f.clock.Advance(2 * time.Second)
	f.store.Get(ctx, "cards/c")
	if n := f.srv.Total(); n != 2 {
		t.Errorf("shared requests after the window = %d, want 2", n)
	}
	f.store.Get(ctx, "cards/d")
	if n := f.srv.Total(); n != 2 {
		t.Errorf("a failed probe did not re-arm the window: %d requests", n)
	}
	if n := f.log.count("shared store unavailable"); n != 1 {
		t.Errorf("unavailable logged %d times, want 1", n)
	}

	// The store recovers: the next probe succeeds and is logged once.
	f.srv.SetFail(false)
	f.clock.Advance(61 * time.Second)
	f.store.Get(ctx, "cards/e")
	if n := f.srv.Total(); n != 3 {
		t.Errorf("shared requests after recovery = %d, want 3", n)
	}
	f.store.Get(ctx, "cards/f")
	f.store.Put(ctx, "cards/f", Entry{Data: []byte(`{}`), Fetched: testEpoch})
	if n := f.srv.Total(); n != 5 {
		t.Errorf("shared requests once healthy = %d, want 5", n)
	}
	if n := f.log.count("reachable again"); n != 1 {
		t.Errorf("recovery logged %d times, want 1: %q", n, f.log.lines)
	}
	if n := f.log.count("shared store unavailable"); n != 1 {
		t.Errorf("unavailable logged %d times in all, want 1", n)
	}
}

func TestTieredCallerGivingUpDoesNotTripTheBreaker(t *testing.T) {
	f := newTiered(t)
	release := f.srv.Hang()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	f.store.Get(ctx, "cards/a")
	release()
	if f.log.count("unavailable") != 0 {
		t.Errorf("a caller's own deadline was logged as an outage: %q", f.log.lines)
	}
	if f.store.skip() {
		t.Error("a caller's own deadline tripped the breaker")
	}
}
