package webui

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cards/cardstest"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// swapSource is a card source whose catalogue the test replaces, as the
// catalogue loader does on a publish.
type swapSource struct {
	mu  sync.Mutex
	cur cards.DB
}

func (s *swapSource) Snapshot() cards.DB {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

func (s *swapSource) swap(db cards.DB) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur = db
}

// TestCardEndpointReadsTheOpenGamesCatalogue: /api/card answers from the
// catalogue the open game started with (text, HP, image), even after a
// newer one arrives, and from the current one with no game open.
func TestCardEndpointReadsTheOpenGamesCatalogue(t *testing.T) {
	old, updated := cardstest.New(), cardstest.New()
	old["tst-2"].Image = "https://assets.tcgdex.net/en/old/2"
	updated["tst-2"].HP = 150
	updated["tst-2"].Image = "https://assets.tcgdex.net/en/new/2"
	src := &swapSource{cur: old}
	m := game.NewManager(t.TempDir(), src)
	ts := httptest.NewServer(New(m, state.P1).Handler())
	t.Cleanup(ts.Close)

	if got := cardJSON(t, ts, "tst-2"); got["hp"] != float64(130) {
		t.Errorf("no game open: hp %v", got["hp"])
	}
	if _, _, err := m.New(context.Background(), game.NewGameRequest{ID: "before", DeckP1: testDeck, DeckP2: testDeck}); err != nil {
		t.Fatal(err)
	}
	src.swap(updated)
	got := cardJSON(t, ts, "tst-2")
	if got["hp"] != float64(130) || firstImage(t, got)["image"] != "https://assets.tcgdex.net/en/old/2" {
		t.Errorf("open game after a newer catalogue: hp %v, image %v; want the old 130 and old art", got["hp"], firstImage(t, got)["image"])
	}
	m.Expire("idle")
	if got := cardJSON(t, ts, "tst-2"); got["hp"] != float64(150) {
		t.Errorf("no game open after the swap: hp %v, want 150", got["hp"])
	}
}
