package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

func connect(t *testing.T) (*mcp.ClientSession, *game.Manager) {
	t.Helper()
	ctx := context.Background()
	m := game.NewManager(t.TempDir(), nil)
	srv := (&Server{M: m, Version: "test"}).MCP()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "codex-mcp-client"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, m
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), res.IsError
}

func TestMCPRoundTrip(t *testing.T) {
	cs, m := connect(t)
	if out, _ := call(t, cs, "ping", nil); out != "pong" {
		t.Fatalf("ping: %q", out)
	}
	// The client's name labels the assistant's seat on the playmat.
	for i := 0; m.Assistant() != "Codex"; i++ {
		if i == 100 {
			t.Fatalf("assistant %q, want Codex", m.Assistant())
		}
		time.Sleep(10 * time.Millisecond)
	}
	deck, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Fatal(err)
	}
	out, isErr := call(t, cs, "new_game", map[string]any{"deck_p1": string(deck), "deck_p2": string(deck), "id": "mcp-1"})
	if isErr {
		t.Fatal(out)
	}

	out, isErr = call(t, cs, "get_state", nil)
	if isErr {
		t.Fatal(out)
	}
	var st struct {
		Board state.BoardView `json:"board"`
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatal(err)
	}
	if len(st.Board.Sides[state.P2].Hand) != 7 || len(st.Board.Sides[state.P1].Hand) != 0 || st.Board.Sides[state.P1].HandCount != 7 {
		t.Fatalf("seat view wrong: p2 hand %d, p1 visible %d", len(st.Board.Sides[state.P2].Hand), len(st.Board.Sides[state.P1].Hand))
	}

	// Reveal is refused in play mode; acting as p1 is refused too.
	if out, isErr = call(t, cs, "get_state", map[string]any{"view": "reveal"}); !isErr {
		t.Fatal("reveal allowed in play mode")
	}
	if out, isErr = call(t, cs, "draw", map[string]any{"seat": "p1"}); !isErr {
		t.Fatal("acting as p1 allowed in play mode")
	}

	// Place an active: pull a Riolu (p2-1) into hand first via move.
	g, _ := m.Current()
	if l, _ := g.State().Find("p2-1"); l.Zone != state.Hand {
		if out, isErr = call(t, cs, "move", map[string]any{"iid": "p2-1", "to": "hand"}); isErr {
			t.Fatal(out)
		}
	}
	out, isErr = call(t, cs, "place_active", map[string]any{"iid": "p2-1"})
	if isErr || !strings.Contains(out, "Riolu") {
		t.Fatalf("place_active: %s", out)
	}
	if out, isErr = call(t, cs, "bench", map[string]any{"iid": "p2-1"}); !isErr {
		t.Fatal("benching a card already in play should fail")
	}
	if !strings.Contains(out, "not p2's hand") {
		t.Fatalf("unhelpful error: %s", out)
	}

	out, _ = call(t, cs, "get_log", map[string]any{"since": 17})
	if !strings.Contains(out, "p2-1 Riolu p2:hand → p2:active") {
		t.Fatalf("log: %s", out)
	}

	out, isErr = call(t, cs, "set_mode", map[string]any{"mode": "review"})
	if isErr {
		t.Fatal(out)
	}
	if out, isErr = call(t, cs, "get_state", map[string]any{"view": "reveal"}); isErr {
		t.Fatal(out)
	}
}

func TestImportLogTool(t *testing.T) {
	cs, _ := connect(t)
	deck, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Fatal(err)
	}
	logText, err := os.ReadFile("../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt")
	if err != nil {
		t.Fatal(err)
	}
	if out, isErr := call(t, cs, "import_log", map[string]any{"log": string(logText)}); !isErr {
		t.Fatalf("import without any deck should fail: %s", out)
	}
	out, isErr := call(t, cs, "import_log", map[string]any{"log": string(logText), "deck": string(deck), "id": "imp-1"})
	if isErr {
		t.Fatal(out)
	}
	var res struct {
		Game   string `json:"game"`
		Mode   string `json:"mode"`
		Report struct {
			Opponent string `json:"opponent"`
			Problems []any  `json:"problems"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatal(err)
	}
	if res.Game != "imp-1" || res.Mode != "review" || res.Report.Opponent == "" || len(res.Report.Problems) > 0 {
		t.Fatalf("unexpected import result: %s", out)
	}
	// The deck is remembered for the next import.
	if out, isErr := call(t, cs, "import_log", map[string]any{"log": string(logText), "id": "imp-2"}); isErr {
		t.Fatalf("import reusing the previous deck: %s", out)
	}
	if out, isErr := call(t, cs, "get_state", map[string]any{"view": "reveal"}); isErr {
		t.Fatal(out)
	}

	// Sample the opponent's hidden cards from a proposed list.
	out, isErr = call(t, cs, "get_profile", nil)
	if isErr || !strings.Contains(out, `"seen_so_far"`) {
		t.Fatalf("get_profile: %s", out)
	}
	if out, isErr := call(t, cs, "sample_hidden", map[string]any{"deck": "Pokémon: 4\n4 Abra\n\nEnergy: 56\n56 Basic Psychic Energy\n\nTotal Cards: 60", "seed": 3}); isErr || !strings.Contains(out, `"hand"`) {
		t.Fatalf("sample_hidden: %s", out)
	}
	out, isErr = call(t, cs, "get_profile", nil)
	if isErr || !strings.Contains(out, `"Basic Psychic Energy"`) {
		t.Fatalf("profile not stored: %s", out)
	}
}

func TestAssistantName(t *testing.T) {
	for in, want := range map[string]string{
		"claude-code": "Claude", "claude-ai": "Claude", "codex-mcp-client": "Codex",
		"gemini-cli-mcp-client": "Gemini", "cursor-vscode": "cursor-vscode",
	} {
		if got := AssistantName(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}
