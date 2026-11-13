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

// onErrorCall is one call of the OnError hook.
type onErrorCall struct {
	action, err string
	live        bool // the call carried the live connection's context
}

// TestOnErrorHook: a failing board action (a bad move, an unknown verb)
// tells OnError its verb and error, with the connection's context; an
// action the Limit refuses and an action that succeeds do not.
func TestOnErrorHook(t *testing.T) {
	m := game.NewManager(t.TempDir(), cardstest.New())
	if _, _, err := m.New(context.Background(), game.NewGameRequest{ID: "ui-1", DeckP1: testDeck, DeckP2: testDeck}); err != nil {
		t.Fatal(err)
	}
	s := New(m, state.P1)
	var refuse atomic.Bool
	s.Limit = func(context.Context) error {
		if refuse.Load() {
			return errors.New("too many requests: this account is limited to 5 requests a minute; try again in 12s")
		}
		return nil
	}
	var mu sync.Mutex
	var calls []onErrorCall
	s.OnError = func(ctx context.Context, action string, err error) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, onErrorCall{action: action, err: err.Error(), live: ctx.Value(limitKey{}) == "live"})
	}
	got := func() []onErrorCall {
		mu.Lock()
		defer mu.Unlock()
		return append([]onErrorCall(nil), calls...)
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
	r := send(t, c, 1, "move", map[string]any{"iid": "p1-1", "to": "nowhere"})
	if r.OK || r.Error == "" {
		t.Fatalf("move to nowhere: %+v, want an error", r)
	}
	want := []onErrorCall{{action: "move", err: r.Error, live: true}}
	if c := got(); len(c) != 1 || c[0] != want[0] {
		t.Fatalf("after a failed move, OnError calls %+v, want %+v", c, want)
	}

	// An unknown verb fails, and is named as sent.
	r = send(t, c, 2, "frobnicate", nil)
	if r.OK || r.Error == "" {
		t.Fatalf("unknown verb: %+v, want an error", r)
	}
	want = append(want, onErrorCall{action: "frobnicate", err: r.Error, live: true})
	if c := got(); len(c) != 2 || c[1] != want[1] {
		t.Fatalf("after an unknown verb, OnError calls %+v, want %+v", c, want)
	}

	// A refusal by the limit is not a failure.
	refuse.Store(true)
	if r := send(t, c, 3, "move", map[string]any{"iid": "p1-1", "to": "nowhere"}); r.OK {
		t.Fatalf("refused move: %+v, want an error", r)
	}
	// Nor is a move that succeeds.
	refuse.Store(false)
	if r := send(t, c, 4, "move", map[string]any{"iid": "p1-1", "to": "discard"}); !r.OK {
		t.Fatalf("allowed move: %+v", r)
	}
	if c := got(); len(c) != 2 {
		t.Errorf("OnError calls %+v after a limit refusal and a good move, want only the 2 failures", c)
	}
}

// TestNoOnErrorOnDesktop: without an OnError hook a failing action still
// returns its error.
func TestNoOnErrorOnDesktop(t *testing.T) {
	ts, _ := setup(t)
	c := dial(t, ts)
	var st stateMsg
	next(t, c, "state", &st)
	if r := send(t, c, 1, "frobnicate", nil); r.OK || r.Error == "" {
		t.Errorf("unknown verb without OnError: %+v, want an error", r)
	}
}
