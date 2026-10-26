package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// transport is one way a client reaches the server: in-memory, as the
// desktop's stdio and daemon paths do, or Streamable HTTP, as the hosted
// connector does.
type transport struct {
	name string
	dial func(t *testing.T, srv *mcp.Server) mcp.Transport
}

var transports = []transport{
	{"inmemory", func(t *testing.T, srv *mcp.Server) mcp.Transport {
		st, ct := mcp.NewInMemoryTransports()
		if _, err := srv.Connect(context.Background(), st, nil); err != nil {
			t.Fatal(err)
		}
		return ct
	}},
	{"http", func(t *testing.T, srv *mcp.Server) mcp.Transport {
		ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, nil))
		t.Cleanup(ts.Close)
		return &mcp.StreamableClientTransport{Endpoint: ts.URL}
	}},
}

// connect starts a server on a fresh manager and connects a client to it
// through tr.
func connect(t *testing.T, tr transport) (*mcp.ClientSession, *game.Manager) {
	t.Helper()
	return connectServer(t, tr, &Server{Version: "test"})
}

// connectServer is connect for a caller-configured server; it gives s a
// fresh manager.
func connectServer(t *testing.T, tr transport, s *Server) (*mcp.ClientSession, *game.Manager) {
	t.Helper()
	m := game.NewManager(t.TempDir(), nil)
	s.M = m
	srv := s.MCP()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "codex-mcp-client"}, nil).Connect(context.Background(), tr.dial(t, srv), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs, m
}

// forEachTransport runs f as a subtest named after each transport, so a
// failure says which connection it came through.
func forEachTransport(t *testing.T, f func(t *testing.T, cs *mcp.ClientSession, m *game.Manager)) {
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			cs, m := connect(t, tr)
			f(t, cs, m)
		})
	}
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

func TestMCPRoundTrip(t *testing.T) { forEachTransport(t, testMCPRoundTrip) }

func testMCPRoundTrip(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
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
	// With no game open there is no record to give.
	if out, isErr := call(t, cs, "get_record", nil); !isErr {
		t.Fatalf("get_record with no game open: %s", out)
	}
	out, isErr := call(t, cs, "new_game", map[string]any{"deck_p1": string(deck), "deck_p2": string(deck), "id": "mcp-1"})
	if isErr {
		t.Fatal(out)
	}
	// A new_game game was not imported, so it has no record.
	if out, isErr := call(t, cs, "get_record", nil); !isErr || !strings.Contains(out, "no record") {
		t.Fatalf("get_record on a new_game game: error %v: %s", isErr, out)
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

	// Start the game with p1 going first, then record an effect that ends
	// on p2's turn 2: per-player turn 4 in storage, shown as turn 2 p2.
	if l, _ := g.State().Find("p1-1"); l.Zone != state.Hand {
		if out, isErr = call(t, cs, "move", map[string]any{"seat": "p1", "iid": "p1-1", "to": "hand"}); isErr {
			t.Fatal(out)
		}
	}
	if out, isErr = call(t, cs, "place_active", map[string]any{"seat": "p1", "iid": "p1-1"}); isErr {
		t.Fatal(out)
	}
	if out, isErr = call(t, cs, "start_game", map[string]any{"first": "p1"}); isErr {
		t.Fatal(out)
	}
	out, isErr = call(t, cs, "add_effect", map[string]any{"seat": "p1", "text": "Items locked", "on": "p2", "source": "p1-1", "until_turn": 2, "until_seat": "p2"})
	if isErr {
		t.Fatalf("add_effect: %s", out)
	}
	if !strings.Contains(out, "until end of turn 2 (p2)") {
		t.Errorf("add_effect result does not describe the end as turn 2 (p2): %s", out)
	}
	if effs := g.State().Effects; len(effs) != 1 || effs[0].UntilTurn != 4 {
		t.Fatalf("stored effects %+v, want one until per-player turn 4", effs)
	}
	out, isErr = call(t, cs, "get_state", nil)
	if isErr {
		t.Fatal(out)
	}
	st.Board = state.BoardView{}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatal(err)
	}
	if effs := st.Board.Effects; len(effs) != 1 || effs[0].Text != "Items locked" || effs[0].UntilTurn != 2 || effs[0].UntilSeat != state.P2 {
		t.Fatalf("board effects %+v, want Items locked until turn 2 p2", effs)
	}
	if !strings.Contains(out, `"until_turn": 2`) || !strings.Contains(out, `"until_seat": "p2"`) {
		t.Errorf("board JSON lacks until_turn 2 and until_seat p2: %s", out)
	}

	// An end turn without the seat it ends on is refused.
	out, isErr = call(t, cs, "add_effect", map[string]any{"seat": "p1", "text": "Retreat locked", "on": "p2", "until_turn": 2})
	if !isErr {
		t.Fatalf("add_effect without until_seat succeeded: %s", out)
	}
	if !strings.Contains(out, "until_seat") {
		t.Errorf("add_effect error does not ask for until_seat: %s", out)
	}
	if n := len(g.State().Effects); n != 1 {
		t.Fatalf("refused add_effect changed the effects: %d", n)
	}
}

func TestImportLogTool(t *testing.T) { forEachTransport(t, testImportLogTool) }

func testImportLogTool(t *testing.T, cs *mcp.ClientSession, _ *game.Manager) {
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
	testRecordTools(t, cs, out, string(logText), string(deck))
	// The deck is not remembered for the next import.
	if out, isErr := call(t, cs, "import_log", map[string]any{"log": string(logText), "id": "imp-2"}); !isErr || !strings.Contains(out, "decklist is required") {
		t.Fatalf("import without a deck: %s", out)
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

// decodeRecord returns the "record" object of a tool result.
func decodeRecord(t *testing.T, tool, out string) map[string]any {
	t.Helper()
	var res struct {
		Record map[string]any `json:"record"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("%s: %v: %s", tool, err, out)
	}
	if len(res.Record) == 0 {
		t.Fatalf("%s result has no record: %s", tool, out)
	}
	return res.Record
}

// getRecord calls get_record and decodes the record it returns.
func getRecord(t *testing.T, cs *mcp.ClientSession) map[string]any {
	t.Helper()
	out, isErr := call(t, cs, "get_record", nil)
	if isErr {
		t.Fatalf("get_record: %s", out)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(out), &rec); err != nil {
		t.Fatalf("get_record: %v: %s", err, out)
	}
	return rec
}

// revealState returns get_state's reveal view of the current game, decoded,
// without the game id: a game restored from its record is stored under an
// id of its own, while everything else must be identical.
func revealState(t *testing.T, cs *mcp.ClientSession) map[string]any {
	t.Helper()
	out, isErr := call(t, cs, "get_state", map[string]any{"view": "reveal"})
	if isErr {
		t.Fatalf("get_state: %s", out)
	}
	var st map[string]any
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("get_state: %v: %s", err, out)
	}
	delete(st, "game")
	return st
}

// testRecordTools checks the record the tools give the assistant, starting
// from importOut, the result of importing logText with deck: import_log and
// set_opponent_deck carry it, get_record repeats it, and importing it as
// the log restores the identical game.
func testRecordTools(t *testing.T, cs *mcp.ClientSession, importOut, logText, deck string) {
	t.Helper()
	rec := decodeRecord(t, "import_log", importOut)
	if rec["log"] != logText {
		t.Errorf("record log differs from the log imported")
	}
	if me, _ := rec["me"].(string); me == "" {
		t.Errorf("record me = %v, want the player's name", rec["me"])
	}
	if rec["deck"] != deck {
		t.Errorf("record deck = %v, want the deck imported", rec["deck"])
	}
	if seed, ok := rec["seed"].(string); !ok || seed == "" {
		t.Errorf("record seed = %#v, want a non-empty JSON string", rec["seed"])
	}
	if check, _ := rec["check"].(string); !strings.HasPrefix(check, "sha256:") {
		t.Errorf("record check = %v, want sha256:…", rec["check"])
	}
	if _, ok := rec["opp_deck"]; ok {
		t.Errorf("record has opp_deck %v without one being given", rec["opp_deck"])
	}
	if got := getRecord(t, cs); !reflect.DeepEqual(got, rec) {
		t.Errorf("get_record = %v\nwant import_log's record %v", got, rec)
	}

	opp := `Pokémon: 12
4 Abra
3 Kadabra
3 Alakazam
2 Duskull

Trainer: 4
4 Buddy-Buddy Poffin

Energy: 44
44 Basic Psychic Energy`
	out, isErr := call(t, cs, "set_opponent_deck", map[string]any{"deck": opp})
	if isErr {
		t.Fatalf("set_opponent_deck: %s", out)
	}
	rebuilt := decodeRecord(t, "set_opponent_deck", out)
	if rebuilt["opp_deck"] != opp {
		t.Errorf("record opp_deck = %v, want the list passed", rebuilt["opp_deck"])
	}
	for _, k := range []string{"log", "me", "seed", "deck"} {
		if rebuilt[k] != rec[k] {
			t.Errorf("set_opponent_deck changed record %s: %v, was %v", k, rebuilt[k], rec[k])
		}
	}
	if got := getRecord(t, cs); !reflect.DeepEqual(got, rebuilt) {
		t.Errorf("get_record = %v\nwant set_opponent_deck's record %v", got, rebuilt)
	}
	before := revealState(t, cs)

	// The record, passed as the log, restores the identical game.
	text, err := json.Marshal(rebuilt)
	if err != nil {
		t.Fatal(err)
	}
	out, isErr = call(t, cs, "import_log", map[string]any{"log": string(text)})
	if isErr {
		t.Fatalf("import_log of a record: %s", out)
	}
	if got := decodeRecord(t, "import_log", out); !reflect.DeepEqual(got, rebuilt) {
		t.Errorf("restored record = %v\nwant %v", got, rebuilt)
	}
	if after := revealState(t, cs); !reflect.DeepEqual(after, before) {
		a, _ := json.Marshal(after)
		b, _ := json.Marshal(before)
		t.Errorf("restored board differs:\n got %s\nwant %s", a, b)
	}
}

// TestToolList: the stored-game tools are gone, get_record is offered, and
// set_opponent_deck acts on the current game, taking discard_branches
// rather than a game id.
func TestToolList(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, _ *game.Manager) {
		schemas := map[string]any{}
		for tool, err := range cs.Tools(context.Background(), nil) {
			if err != nil {
				t.Fatal(err)
			}
			schemas[tool.Name] = tool.InputSchema
		}
		for _, want := range []string{"get_record", "wait_for_turn"} {
			if _, ok := schemas[want]; !ok {
				t.Errorf("no %s tool", want)
			}
		}
		for _, gone := range []string{"list_games", "load_game"} {
			if _, ok := schemas[gone]; ok {
				t.Errorf("%s is still offered", gone)
			}
		}
		b, err := json.Marshal(schemas["set_opponent_deck"])
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Properties map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(b, &schema); err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"deck", "discard_branches"} {
			if _, ok := schema.Properties[want]; !ok {
				t.Errorf("set_opponent_deck has no %s argument: %s", want, b)
			}
		}
		for _, gone := range []string{"id", "new_id"} {
			if _, ok := schema.Properties[gone]; ok {
				t.Errorf("set_opponent_deck still takes %s: %s", gone, b)
			}
		}
	})
}

// TestSetOpponentDeckBranches: through the tool, a rebuild of a game with a
// branch is refused, naming discard_branches, and goes ahead with it.
func TestSetOpponentDeckBranches(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
		deck, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
		if err != nil {
			t.Skip("no test corpus")
		}
		logText, err := os.ReadFile("../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt")
		if err != nil {
			t.Fatal(err)
		}
		if out, isErr := call(t, cs, "import_log", map[string]any{"log": string(logText), "deck": string(deck), "id": "br"}); isErr {
			t.Fatal(out)
		}
		g, err := m.Current()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := g.JumpTo(10, "look"); err != nil {
			t.Fatal(err)
		}
		opp := "Pokémon: 4\n4 Abra\n\nEnergy: 56\n56 Basic Psychic Energy"
		out, isErr := call(t, cs, "set_opponent_deck", map[string]any{"deck": opp})
		if !isErr || !strings.Contains(out, "discard_branches") {
			t.Fatalf("set_opponent_deck on a game with a branch: error %v: %s", isErr, out)
		}
		if cur, _ := m.Current(); cur != g {
			t.Fatal("refused rebuild replaced the game")
		}
		out, isErr = call(t, cs, "set_opponent_deck", map[string]any{"deck": opp, "discard_branches": true})
		if isErr {
			t.Fatalf("set_opponent_deck with discard_branches: %s", out)
		}
		cur, err := m.Current()
		if err != nil || cur == g || cur.ID != "br" {
			t.Fatalf("game not rebuilt: %v", err)
		}
		for _, br := range cur.Branches() {
			if br.Name != "main" {
				t.Errorf("branch %s survived the rebuild", br.Name)
			}
		}
		if rec := decodeRecord(t, "set_opponent_deck", out); rec["opp_deck"] != opp {
			t.Errorf("record opp_deck = %v, want the list passed", rec["opp_deck"])
		}
	})
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

// A failure is reported under its transport's subtest name, so the names are
// part of the contract.
func TestTransportsNamed(t *testing.T) {
	want := []string{"inmemory", "http"}
	if len(transports) != len(want) {
		t.Fatalf("%d transports, want %d", len(transports), len(want))
	}
	for i, w := range want {
		if transports[i].name != w {
			t.Errorf("transports[%d] = %q, want %q", i, transports[i].name, w)
		}
	}
}

// boardResults runs the three game-opening tools and returns each result's
// decoded JSON object, keyed by tool name.
func boardResults(t *testing.T, cs *mcp.ClientSession) map[string]map[string]any {
	t.Helper()
	deck, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Fatal(err)
	}
	logText, err := os.ReadFile("../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt")
	if err != nil {
		t.Fatal(err)
	}
	run := func(tool string, args map[string]any) map[string]any {
		out, isErr := call(t, cs, tool, args)
		if isErr {
			t.Fatalf("%s: %s", tool, out)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatalf("%s: %v: %s", tool, err, out)
		}
		return res
	}
	res := map[string]map[string]any{}
	res["new_game"] = run("new_game", map[string]any{"deck_p1": string(deck), "deck_p2": string(deck), "id": "board-new"})
	res["import_log"] = run("import_log", map[string]any{"log": string(logText), "deck": string(deck), "id": "board-imp"})
	res["set_opponent_deck"] = run("set_opponent_deck", map[string]any{"deck": string(deck)})
	return res
}

func TestBoardLinks(t *testing.T) {
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			cs, _ := connectServer(t, tr, &Server{Version: "test", BoardURL: "https://playmat.example/"})
			for tool, res := range boardResults(t, cs) {
				if got := res["board"]; got != "https://playmat.example/" {
					t.Errorf("%s board = %v, want https://playmat.example/", tool, got)
				}
				if tool == "import_log" || tool == "set_opponent_deck" {
					if rec, ok := res["record"].(map[string]any); !ok || len(rec) == 0 {
						t.Errorf("%s record = %v, want a non-empty object", tool, res["record"])
					}
				}
			}
			ins := cs.InitializeResult().Instructions
			if !strings.Contains(ins, "board link") {
				t.Errorf("instructions do not mention the board link: %q", ins)
			}
			// The board line comes after the guide block.
			if want := instructions + guideInstructions() + boardInstruction; ins != want {
				t.Errorf("instructions = %q, want %q", ins, want)
			}
		})
	}
}

func TestNoBoardLinksOnDesktop(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, _ *game.Manager) {
		for tool, res := range boardResults(t, cs) {
			if _, ok := res["board"]; ok {
				t.Errorf("%s result has a board key: %v", tool, res["board"])
			}
		}
		if ins := cs.InitializeResult().Instructions; ins != instructions+guideInstructions() {
			t.Errorf("instructions changed without a board address: %q", ins)
		}
	})
}

// TestResolvePerCall checks that with a resolver every tool call acts on the
// manager and board link the resolver returns for it, and on nothing else:
// the server's own M is nil and must never be touched.
func TestResolvePerCall(t *testing.T) {
	deck, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			a, b := game.NewManager(t.TempDir(), nil), game.NewManager(t.TempDir(), nil)
			const linkA, linkB = "https://x/?board=A", "https://x/?board=B"

			var mu sync.Mutex
			target, calls := "A", 0
			pick := func(to string) {
				mu.Lock()
				defer mu.Unlock()
				target = to
			}
			resolved := func() int {
				mu.Lock()
				defer mu.Unlock()
				return calls
			}
			s := &Server{Version: "test", Resolve: func(_ context.Context, _ *mcp.RequestExtra) (*game.Manager, string, error) {
				mu.Lock()
				defer mu.Unlock()
				calls++
				switch target {
				case "A":
					return a, linkA, nil
				case "B":
					return b, linkB, nil
				}
				return nil, "", errors.New("no account for this call")
			}}
			cs, err := mcp.NewClient(&mcp.Implementation{Name: "codex-mcp-client"}, nil).Connect(context.Background(), tr.dial(t, s.MCP()), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { cs.Close() })

			// Every tool, even ping, goes through the resolver.
			if out, isErr := call(t, cs, "ping", nil); isErr || out != "pong" {
				t.Fatalf("ping: error %v: %s", isErr, out)
			}
			if n := resolved(); n != 1 {
				t.Errorf("resolver called %d times for ping, want 1", n)
			}

			out, isErr := call(t, cs, "new_game", map[string]any{"deck_p1": string(deck), "deck_p2": string(deck), "id": "per-call"})
			if isErr {
				t.Fatal(out)
			}
			var res map[string]any
			if err := json.Unmarshal([]byte(out), &res); err != nil {
				t.Fatalf("new_game: %v: %s", err, out)
			}
			if res["board"] != linkA {
				t.Errorf("new_game board = %v, want %s", res["board"], linkA)
			}
			if _, err := a.Current(); err != nil {
				t.Errorf("A has no game after new_game resolved to A: %v", err)
			}
			if _, err := b.Current(); err == nil {
				t.Error("B has a game after new_game resolved to A")
			}
			// The call's client labels the assistant on the manager it acted on.
			if got := a.Assistant(); got != "Codex" {
				t.Errorf("A assistant = %q, want Codex", got)
			}
			if got := b.Assistant(); got == "Codex" {
				t.Error("B was labelled by a call that resolved to A")
			}

			pick("B")
			if out, isErr := call(t, cs, "get_state", nil); !isErr {
				t.Errorf("get_state resolved to B, which has no game, succeeded: %s", out)
			}
			if _, err := b.Current(); err == nil {
				t.Error("B gained a game from get_state")
			}

			pick("A")
			if out, isErr := call(t, cs, "get_state", nil); isErr {
				t.Errorf("get_state resolved back to A failed: %s", out)
			}

			pick("none")
			out, isErr = call(t, cs, "get_state", nil)
			if !isErr || !strings.Contains(out, "no account for this call") {
				t.Errorf("resolver error: error %v: %q, want a tool error with its message", isErr, out)
			}
		})
	}
}

// TestExpiredGameTools: after the game expires, tools that need it report
// the expiry notice, and importing or starting a game works again.
func TestExpiredGameTools(t *testing.T) {
	deck, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	const notice = "game per-call expired: import its saved record again with import_log"
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, m *game.Manager) {
		if out, isErr := call(t, cs, "new_game", map[string]any{"deck_p1": string(deck), "deck_p2": string(deck), "id": "per-call"}); isErr {
			t.Fatal(out)
		}
		m.Expire(notice)
		for tool, args := range map[string]map[string]any{"get_state": nil, "get_log": nil, "draw": {"n": 1}} {
			out, isErr := call(t, cs, tool, args)
			if !isErr || !strings.Contains(out, notice) {
				t.Errorf("%s after expiry: error %v: %q, want a tool error with the notice", tool, isErr, out)
			}
		}
		if out, isErr := call(t, cs, "new_game", map[string]any{"deck_p1": string(deck), "deck_p2": string(deck), "id": "again"}); isErr {
			t.Fatalf("new_game after expiry: %s", out)
		}
		if out, isErr := call(t, cs, "get_state", nil); isErr {
			t.Errorf("get_state after a new game: %s", out)
		}
		if n := m.Notice(); n != "" {
			t.Errorf("Notice() after new_game = %q, want empty", n)
		}
	})
}
