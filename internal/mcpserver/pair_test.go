package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/cards/cardstest"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// pairDeck holds two prints of Twin Peak (TST 10 and TST 11), a made-up
// pair-played Stadium in the test pool.
const pairDeck = "Pokémon: 8\n4 Riolu TST 1\n4 Lucario TST 2\nTrainer: 12\n4 Ultra Ball TST 3\n4 Iono TST 4\n2 Twin Peak TST 10\n2 Twin Peak TST 11\nEnergy: 40\n40 Basic {F} Energy SVE 6"

// pairServer connects a client through tr to a server reading the test
// pool, with a review-mode game started, p1 to move, holding one Twin
// Peak of each print and an Ultra Ball; it returns those three.
func pairServer(t *testing.T, tr transport) (*mcp.ClientSession, *game.Game, state.IID, state.IID, state.IID) {
	t.Helper()
	m := game.NewManager(t.TempDir(), cardstest.New())
	s := &Server{Version: "test", M: m}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test-client"}, nil).Connect(context.Background(), tr.dial(t, s.MCP()), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	if _, _, err := m.New(context.Background(), game.NewGameRequest{ID: "pairs", DeckP1: pairDeck, DeckP2: pairDeck}); err != nil {
		t.Fatal(err)
	}
	g, err := m.Current()
	if err != nil {
		t.Fatal(err)
	}
	if out, isErr := call(t, cs, "set_mode", map[string]any{"mode": "review"}); isErr {
		t.Fatal(out)
	}
	find := func(def string) state.IID {
		st := g.State()
		for _, iid := range append(append([]state.IID{}, st.Sides[state.P1].Hand...), st.Sides[state.P1].Deck...) {
			if st.Cards[iid].Def == def {
				return iid
			}
		}
		t.Fatalf("p1 has no %s in hand or deck", def)
		return ""
	}
	a, b, ball := find("tst-10"), find("tst-11"), find("tst-3")
	for _, seat := range state.Seats {
		basic := state.IID(string(seat) + "-1")
		for _, iid := range []state.IID{basic, a, b, ball} {
			if g.State().Cards[iid].Owner != seat {
				continue
			}
			if l, _ := g.State().Find(iid); l.Zone != state.Hand {
				if _, err := g.Move(seat, state.ActorSystem, iid, state.Loc{Zone: state.Hand}, "", ""); err != nil {
					t.Fatal(err)
				}
			}
		}
		if _, err := g.PlaceActive(seat, state.ActorSystem, basic); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := g.StartGame(state.P1, state.ActorHuman); err != nil {
		t.Fatal(err)
	}
	return cs, g, a, b, ball
}

// TestPlayTrainerPair: play_trainer with pair_iid plays both halves of a
// pair-played Stadium in one action, shown as one Stadium with its second
// half nested and logged in one line.
func TestPlayTrainerPair(t *testing.T) {
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			cs, g, a, b, _ := pairServer(t, tr)
			out, isErr := call(t, cs, "play_trainer", map[string]any{"seat": "p1", "iid": string(a), "pair_iid": string(b)})
			if isErr {
				t.Fatal(out)
			}
			if st := g.State().Stadium; len(st) != 2 || st[0] != a || st[1] != b {
				t.Fatalf("stadium = %v, want [%s %s]", st, a, b)
			}
			out, isErr = call(t, cs, "get_state", map[string]any{"view": "reveal"})
			if isErr {
				t.Fatal(out)
			}
			var got struct {
				Board state.BoardView `json:"board"`
			}
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatal(err)
			}
			if s := got.Board.Stadium; s == nil || s.IID != a || s.Pair == nil || s.Pair.IID != b || s.Pair.Name != "Twin Peak" {
				t.Fatalf("board stadium = %+v, want %s with pair %s", s, a, b)
			}
			out, _ = call(t, cs, "get_log", nil)
			want := "p1 moved " + string(a) + " Twin Peak + " + string(b) + " Twin Peak p1:hand → stadium"
			if n := strings.Count(out, "→ stadium"); n != 1 || !strings.Contains(out, want) {
				t.Errorf("log should have the one line %q:\n%s", want, out)
			}
		})
	}
}

// TestCardTextPairRule: card_text for a pair-played card is its data text,
// pair rule included, with no line added for the pair mark.
func TestCardTextPairRule(t *testing.T) {
	cs, _, a, _, _ := pairServer(t, transports[0])
	out, isErr := call(t, cs, "card_text", map[string]any{"card": string(a)})
	if isErr {
		t.Fatal(out)
	}
	want := "Twin Peak (tst-10)\nTrainer: Stadium\nYou cannot play this card by itself. You must combine 2 different Twin Peak from your hand to play as 1 Stadium card. Each player's Pokémon get +10 HP."
	if !strings.Contains(out, want) {
		t.Errorf("card_text = %q, want the data text %q", out, want)
	}
	if strings.Count(out, "pair") != 0 || strings.Count(out, "combine 2 different") != 1 {
		t.Errorf("card_text adds to the data text: %q", out)
	}
}

// TestPlayTrainerSingle: play_trainer without pair_iid plays one Trainer
// as before.
func TestPlayTrainerSingle(t *testing.T) {
	cs, g, _, _, ball := pairServer(t, transports[0])
	out, isErr := call(t, cs, "play_trainer", map[string]any{"seat": "p1", "iid": string(ball)})
	if isErr {
		t.Fatal(out)
	}
	if l, _ := g.State().Find(ball); l.Zone != state.Discard {
		t.Errorf("Ultra Ball is in %s, want the discard", l)
	}
	if !strings.Contains(out, "Ultra Ball p1:hand → p1:discard") {
		t.Errorf("result lacks the play: %s", out)
	}
}
