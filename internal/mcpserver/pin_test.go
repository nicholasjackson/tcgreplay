package mcpserver

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cards/cardstest"
	"github.com/nicholasjackson/tcgreplay/internal/game"
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

const pinDeck = `Pokémon: 8
4 Riolu TST 1
4 Lucario TST 2

Trainer: 12
4 Ultra Ball TST 3
4 Iono TST 4
2 Arena TST 5
2 Band TST 6

Energy: 40
40 Basic {F} Energy SVE 6

Total Cards: 60`

// TestCardTextReadsTheOpenGamesCatalogue: card_text quotes the catalogue
// the open game started with, even after a newer one arrives; with no
// game open, and in a game started afterwards, it quotes the current one.
func TestCardTextReadsTheOpenGamesCatalogue(t *testing.T) {
	old, updated := cardstest.New(), cardstest.New()
	old["tst-8"].Effect = "Each player can have 8 Pokémon on their Bench."
	updated["tst-8"].Effect = "Each player can have 6 Pokémon on their Bench."
	src := &swapSource{cur: old}
	m := game.NewManager(t.TempDir(), src)
	s := &Server{Version: "test", M: m}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil).Connect(context.Background(), transports[0].dial(t, s.MCP()), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	text := func() string {
		t.Helper()
		out, isErr := call(t, cs, "card_text", map[string]any{"card": "tst-8"})
		if isErr {
			t.Fatalf("card_text: %s", out)
		}
		return out
	}

	if out := text(); !strings.Contains(out, "8 Pokémon") {
		t.Errorf("no game open: %q", out)
	}
	if _, _, err := m.New(context.Background(), game.NewGameRequest{ID: "before", DeckP1: pinDeck, DeckP2: pinDeck}); err != nil {
		t.Fatal(err)
	}
	src.swap(updated)
	if out := text(); !strings.Contains(out, "8 Pokémon") {
		t.Errorf("open game after a newer catalogue: %q, want the text it started with", out)
	}
	if _, _, err := m.New(context.Background(), game.NewGameRequest{ID: "after", DeckP1: pinDeck, DeckP2: pinDeck}); err != nil {
		t.Fatal(err)
	}
	if out := text(); !strings.Contains(out, "6 Pokémon") {
		t.Errorf("game started after the newer catalogue: %q", out)
	}
}
