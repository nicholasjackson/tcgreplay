package webui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards/cardstest"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// traceKey marks the context the Trace hook hands back.
type traceKey struct{}

// traceCall is one board action as the Trace hook and the hooks after it
// saw it.
type traceCall struct {
	verb      string
	live      bool   // Trace was handed the live connection's context
	endErr    string // what end was called with ("" for nil)
	ended     int    // how many times end was called
	limitSaw  bool   // Limit got the context Trace returned
	onErrorOK bool   // OnError (if called) got the context Trace returned
}

// TestTraceHook: with Trace set, each board action calls it with the live
// connection's context and the action's verb; Limit and OnError get the
// context it returned; and its end function is called once, with nil for
// an action that succeeds and with the action's error for one that fails
// or that the Limit refuses.
func TestTraceHook(t *testing.T) {
	m := game.NewManager(t.TempDir(), cardstest.New())
	if _, _, err := m.New(context.Background(), game.NewGameRequest{ID: "ui-1", DeckP1: testDeck, DeckP2: testDeck}); err != nil {
		t.Fatal(err)
	}
	s := New(m, state.P1)
	var mu sync.Mutex
	var calls []*traceCall
	current := func() *traceCall {
		mu.Lock()
		defer mu.Unlock()
		return calls[len(calls)-1]
	}
	s.Trace = func(ctx context.Context, verb string) (context.Context, func(error)) {
		c := &traceCall{verb: verb, live: ctx.Value(limitKey{}) == "live", onErrorOK: true}
		mu.Lock()
		calls = append(calls, c)
		mu.Unlock()
		return context.WithValue(ctx, traceKey{}, verb), func(err error) {
			mu.Lock()
			defer mu.Unlock()
			c.ended++
			if err != nil {
				c.endErr = err.Error()
			}
		}
	}
	var refuse atomic.Bool
	const refusal = "too many requests: this account is limited to 5 requests a minute; try again in 12s"
	s.Limit = func(ctx context.Context) error {
		c := current()
		mu.Lock()
		c.limitSaw = ctx.Value(traceKey{}) == c.verb
		mu.Unlock()
		if refuse.Load() {
			return errors.New(refusal)
		}
		return nil
	}
	var onErrors atomic.Int32
	s.OnError = func(ctx context.Context, action string, err error) {
		onErrors.Add(1)
		c := current()
		mu.Lock()
		c.onErrorOK = ctx.Value(traceKey{}) == action && action == c.verb
		mu.Unlock()
	}
	got := func() []traceCall {
		mu.Lock()
		defer mu.Unlock()
		var out []traceCall
		for _, c := range calls {
			out = append(out, *c)
		}
		return out
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.StartHub(ctx)
	h := s.Handler()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), limitKey{}, "live")))
	}))
	t.Cleanup(ts.Close)
	c := dial(t, ts)
	var st stateMsg
	next(t, c, "state", &st)

	// A move to a location that does not exist fails.
	bad := send(t, c, 1, "move", map[string]any{"iid": "p1-1", "to": "nowhere"})
	if bad.OK || bad.Error == "" {
		t.Fatalf("move to nowhere: %+v, want an error", bad)
	}
	// A refusal by the limit.
	refuse.Store(true)
	refused := send(t, c, 2, "move", map[string]any{"iid": "p1-1", "to": "discard"})
	if refused.OK || refused.Error != refusal {
		t.Fatalf("refused move: %+v, want the refusal", refused)
	}
	// A move that succeeds.
	refuse.Store(false)
	if r := send(t, c, 3, "move", map[string]any{"iid": "p1-1", "to": "discard"}); !r.OK {
		t.Fatalf("allowed move: %+v", r)
	}

	want := []traceCall{
		{verb: "move", live: true, endErr: bad.Error, ended: 1, limitSaw: true, onErrorOK: true},
		{verb: "move", live: true, endErr: refusal, ended: 1, limitSaw: true, onErrorOK: true},
		{verb: "move", live: true, endErr: "", ended: 1, limitSaw: true, onErrorOK: true},
	}
	g := got()
	if len(g) != len(want) {
		t.Fatalf("Trace called %d times (%+v), want %d", len(g), g, len(want))
	}
	for i := range want {
		if g[i] != want[i] {
			t.Errorf("action %d: %+v, want %+v", i+1, g[i], want[i])
		}
	}
	if n := onErrors.Load(); n != 1 {
		t.Errorf("OnError called %d times, want once (the bad move)", n)
	}
}

// TestNoTraceOnDesktop: with no Trace hook (the desktop) the board acts
// as before: a good move succeeds and a bad one returns its error.
func TestNoTraceOnDesktop(t *testing.T) {
	ts, _, s := setupServer(t, nil)
	if s.Trace != nil {
		t.Fatal("a new board server has a Trace hook")
	}
	c := dial(t, ts)
	var st stateMsg
	next(t, c, "state", &st)
	if r := send(t, c, 1, "move", map[string]any{"iid": "p1-1", "to": "nowhere"}); r.OK || r.Error == "" {
		t.Errorf("move to nowhere without Trace: %+v, want an error", r)
	}
	if r := send(t, c, 2, "move", map[string]any{"iid": "p1-1", "to": "discard"}); !r.OK {
		t.Errorf("good move without Trace: %+v", r)
	}
}
