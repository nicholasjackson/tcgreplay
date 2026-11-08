package webui

import (
	"context"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/webui/webtest"
)

// TestPlayTrainerPairAction: the board's play_trainer takes pair_iid and
// plays both halves of a pair-played Stadium as one Stadium, replacing the
// Arena in play; without it, a half dropped onto a lone half completes it.
func TestPlayTrainerPairAction(t *testing.T) {
	m := webtest.NewManager(t.TempDir())
	if err := webtest.Reset(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	s := New(m, state.P1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.StartHub(ctx)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	c := dial(t, ts)
	g, _ := m.Current()
	a, b := webtest.TwinPeak[0], webtest.TwinPeak[1]
	for i, iid := range []state.IID{a, b} {
		if r := send(t, c, i+1, "move", map[string]any{"iid": string(iid), "to": "hand"}); !r.OK {
			t.Fatal(r.Error)
		}
	}
	if r := send(t, c, 3, "play_trainer", map[string]any{"iid": string(a), "pair_iid": string(b)}); !r.OK {
		t.Fatal(r.Error)
	}
	st := g.State()
	if !slices.Equal(st.Stadium, []state.IID{a, b}) {
		t.Fatalf("stadium = %v, want [%s %s]", st.Stadium, a, b)
	}
	if !slices.Contains(st.Sides[state.P1].Discard, webtest.Stadium) {
		t.Errorf("Arena (%s) not replaced into the discard", webtest.Stadium)
	}

	// The pair back to the hand (both halves go), then one half and its
	// partner played in turn.
	if r := send(t, c, 4, "move", map[string]any{"iid": string(b), "to": "hand"}); !r.OK {
		t.Fatal(r.Error)
	}
	if st := g.State(); len(st.Stadium) != 0 {
		t.Fatalf("stadium = %v after moving a half to the hand, want empty", st.Stadium)
	}
	for i, iid := range []state.IID{a, b} {
		if r := send(t, c, 5+i, "play_trainer", map[string]any{"iid": string(iid)}); !r.OK {
			t.Fatal(r.Error)
		}
	}
	if st := g.State(); !slices.Equal(st.Stadium, []state.IID{a, b}) {
		t.Fatalf("stadium = %v after playing the halves in turn, want [%s %s]", st.Stadium, a, b)
	}
}
