package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/mcpserver"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

func TestHostedListen(t *testing.T) {
	tests := []struct {
		name, flag, port, want string
	}{
		{"default", "", "", ":8080"},
		{"port env", "", "3000", ":3000"},
		{"flag wins over port env", "127.0.0.1:9999", "3000", "127.0.0.1:9999"},
		{"flag without port env", ":9000", "", ":9000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PORT", tt.port)
			if got := hostedListen(tt.flag); got != tt.want {
				t.Fatalf("hostedListen(%q) with PORT=%q = %q, want %q", tt.flag, tt.port, got, tt.want)
			}
		})
	}
}

// captureLog redirects the standard logger for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func TestHostedPublicURL(t *testing.T) {
	tests := []struct {
		name, raw, listen string
		want              string // empty means an error is expected
		warn              bool
	}{
		{"fallback default port", "", ":8080", "http://localhost:8080", true},
		{"fallback follows listen port", "", ":3000", "http://localhost:3000", true},
		{"fallback ignores listen host", "", "127.0.0.1:4321", "http://localhost:4321", true},
		{"https host", "https://play.example.com", ":8080", "https://play.example.com", false},
		{"http host with port", "http://play.example.com:8443", ":8080", "http://play.example.com:8443", false},
		{"trailing slash dropped", "https://play.example.com/", ":8080", "https://play.example.com", false},
		{"no scheme", "example.com", ":8080", "", false},
		{"host and port without scheme", "localhost:8080", ":8080", "", false},
		{"path", "https://x/y", ":8080", "", false},
		{"ftp scheme", "ftp://x", ":8080", "", false},
		{"no host", "https://", ":8080", "", false},
		{"query", "https://x?a=1", ":8080", "", false},
		{"fragment", "https://x#top", ":8080", "", false},
		{"userinfo", "https://user:pass@x", ":8080", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLog(t)
			u, err := hostedPublicURL(tt.raw, tt.listen)
			if tt.want == "" {
				if err == nil {
					t.Fatalf("hostedPublicURL(%q) = %v, want an error", tt.raw, u)
				}
				if !strings.Contains(err.Error(), "public URL") || !strings.Contains(err.Error(), tt.raw) {
					t.Fatalf("error %q does not name the public URL %q", err, tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("hostedPublicURL(%q): %v", tt.raw, err)
			}
			if got := u.String(); got != tt.want {
				t.Fatalf("hostedPublicURL(%q, %q) = %q, want %q", tt.raw, tt.listen, got, tt.want)
			}
			warned := strings.Contains(logs.String(), "warning")
			if warned != tt.warn {
				t.Fatalf("warning logged = %v, want %v (log: %q)", warned, tt.warn, logs.String())
			}
			if tt.warn && !strings.Contains(logs.String(), tt.want) {
				t.Fatalf("warning %q does not name the fallback %s", logs.String(), tt.want)
			}
		})
	}
}

// toolSchemas lists every tool's input schema as JSON, by name.
func toolSchemas(t *testing.T, cs *mcp.ClientSession) map[string]string {
	t.Helper()
	out := map[string]string{}
	for tool, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		out[tool.Name] = string(b)
	}
	return out
}

func callText(t *testing.T, cs *mcp.ClientSession, name string) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	if res.IsError {
		t.Fatalf("%s: %s", name, b.String())
	}
	return b.String()
}

func connectHTTP(t *testing.T, endpoint string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "hosted-test"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestHostedHandlerServesMCPAndBoard(t *testing.T) {
	ts := startHosted(t, testPublic)

	// MCP over Streamable HTTP at /mcp.
	cs := connectHTTP(t, ts.URL+"/mcp")
	if err := cs.Ping(context.Background(), nil); err != nil {
		t.Fatalf("protocol ping: %v", err)
	}
	if out := callText(t, cs, "ping"); out != "pong" {
		t.Fatalf("ping tool: %q, want pong", out)
	}

	// The same tools and input schemas as a desktop (in-memory) client sees.
	desktop := (&mcpserver.Server{M: game.NewManager(t.TempDir(), nil), Version: "test"}).MCP()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := desktop.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	mem, err := mcp.NewClient(&mcp.Implementation{Name: "desktop-test"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mem.Close() })
	overHTTP, inMem := toolSchemas(t, cs), toolSchemas(t, mem)
	if len(overHTTP) == 0 {
		t.Fatal("no tools over HTTP")
	}
	for _, name := range []string{"ping", "new_game", "get_state"} {
		if _, ok := overHTTP[name]; !ok {
			t.Errorf("tool %s missing over HTTP", name)
		}
	}
	for name, schema := range inMem {
		got, ok := overHTTP[name]
		if !ok {
			t.Errorf("tool %s missing over HTTP", name)
			continue
		}
		if got != schema {
			t.Errorf("tool %s input schema differs:\nhttp:      %s\nin-memory: %s", name, got, schema)
		}
	}
	for name := range overHTTP {
		if _, ok := inMem[name]; !ok {
			t.Errorf("tool %s only over HTTP", name)
		}
	}

	// The board's routes on the same address.
	if code, body := get(t, ts.URL+"/"); code != http.StatusOK || !strings.Contains(body, "<title>tcgreplay</title>") {
		t.Fatalf("index: %d %q", code, body)
	}
	// The board's card handler answers (its own 404), not the mux's.
	if code, body := get(t, ts.URL+"/api/card"); code != http.StatusNotFound || !strings.Contains(body, "unknown card") {
		t.Fatalf("/api/card without def: %d %q, want the board's 404", code, body)
	}

	wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer wcancel()
	c, _, err := websocket.Dial(wctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatalf("dial /ws: %v", err)
	}
	defer c.CloseNow()
	_, b, err := c.Read(wctx)
	if err != nil {
		t.Fatalf("read /ws: %v", err)
	}
	var msg struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(b, &msg); err != nil || msg.Type != "state" {
		t.Fatalf("first /ws message %s, want type state", b)
	}
}

// freePort returns a loopback port nothing is listening on.
func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()
	return port
}

func hostedConfig(t *testing.T, listen, publicURL string) config {
	return config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true, listen: listen, publicURL: publicURL}
}

// TestRunHostedServesAndShutsDown runs hosted mode on one port, uses both
// MCP and the board, then stops it as a container stop would.
func TestRunHostedServesAndShutsDown(t *testing.T) {
	logs := captureLog(t)
	base := "http://127.0.0.1:" + freePort(t)
	c := hostedConfig(t, strings.TrimPrefix(base, "http://"), "https://play.example.com")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runHosted(ctx, c) }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get(base + "/")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		select {
		case err := <-done:
			t.Fatalf("runHosted returned before serving: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("hosted mode did not start serving /")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cs := connectHTTP(t, base+"/mcp")
	if out := callText(t, cs, "ping"); out != "pong" {
		t.Fatalf("ping tool: %q, want pong", out)
	}
	cs.Close()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runHosted after cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runHosted did not return after cancel")
	}
	if _, err := http.Get(base + "/"); err == nil {
		t.Fatal("still serving after shutdown")
	}
	out := logs.String()
	for _, want := range []string{"https://play.example.com", "stopped"} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q does not mention %q", out, want)
		}
	}
}

func TestRunHostedRejectsBadPublicURL(t *testing.T) {
	captureLog(t)
	port := freePort(t)
	for _, raw := range []string{"example.com", "https://x/y", "ftp://x"} {
		t.Run(raw, func(t *testing.T) {
			c := hostedConfig(t, "127.0.0.1:"+port, raw)
			done := make(chan error, 1)
			go func() { done <- runHosted(context.Background(), c) }()
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), raw) {
					t.Fatalf("runHosted with public URL %q: %v, want an error naming it", raw, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("runHosted with public URL %q did not stop", raw)
			}
			// Nothing was left listening on the port.
			ln, err := net.Listen("tcp", "127.0.0.1:"+port)
			if err != nil {
				t.Fatalf("port %s in use after a rejected start: %v", port, err)
			}
			ln.Close()
		})
	}
}

// TestHostedMainHelper is not a test: the main-binary tests below re-run
// this test binary with TCGREPLAY_HOSTED_HELPER=1 so it calls main() in
// hosted mode with the environment they set.
func TestHostedMainHelper(t *testing.T) {
	if os.Getenv("TCGREPLAY_HOSTED_HELPER") != "1" {
		t.Skip("helper process only")
	}
	dir := os.Getenv("HELPER_DATA")
	os.Args = []string{"tcgreplay", "-hosted", "-data", dir, "-cache", dir, "-offline"}
	main()
	os.Exit(0)
}

func helperCmd(t *testing.T, env ...string) (*exec.Cmd, *bytes.Buffer, string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cmd := exec.Command(exe, "-test.run=^TestHostedMainHelper$")
	cmd.Env = append(os.Environ(), append([]string{"TCGREPLAY_HOSTED_HELPER=1", "HELPER_DATA=" + dir}, env...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	return cmd, &stderr, dir
}

// TestMainHostedFollowsEnvAndStopsOnSIGTERM runs the binary's main in
// hosted mode with PORT and TCGREPLAY_PUBLIC_URL set, then stops it the
// way a container platform does.
func TestMainHostedFollowsEnvAndStopsOnSIGTERM(t *testing.T) {
	port := freePort(t)
	cmd, stderr, dir := helperCmd(t, "PORT="+port, "TCGREPLAY_PUBLIC_URL=https://play.example.com/")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() { cmd.Process.Kill() })

	base := "http://127.0.0.1:" + port
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(base + "/")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		select {
		case err := <-exited:
			t.Fatalf("hosted main exited before serving: %v\n%s", err, stderr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("hosted main did not serve on PORT %s\n%s", port, stderr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cs := connectHTTP(t, base+"/mcp")
	if out := callText(t, cs, "ping"); out != "pong" {
		t.Fatalf("ping tool: %q, want pong", out)
	}
	cs.Close()

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("exit after SIGTERM: %v\n%s", err, stderr)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("hosted main did not stop after SIGTERM\n%s", stderr)
	}
	out := stderr.String()
	for _, want := range []string{"hosted", ":" + port, "public https://play.example.com,", "stopped"} {
		if !strings.Contains(out, want) {
			t.Errorf("stderr does not mention %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "warning") {
		t.Errorf("unexpected warning with TCGREPLAY_PUBLIC_URL set:\n%s", out)
	}
	// Hosted mode logs only to stderr, not to the data dir's log file.
	if _, err := os.Stat(filepath.Join(dir, "tcgreplay.log")); err == nil {
		t.Error("hosted mode wrote tcgreplay.log")
	}
}

// TestMainHostedRejectsBadPublicURLFromEnv: a public address from
// TCGREPLAY_PUBLIC_URL with a path stops start-up with an error.
func TestMainHostedRejectsBadPublicURLFromEnv(t *testing.T) {
	cmd, stderr, _ := helperCmd(t, "PORT="+freePort(t), "TCGREPLAY_PUBLIC_URL=https://play.example.com/board")
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("hosted main accepted a public URL with a path\n%s", stderr)
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Fatalf("hosted main did not stop on a bad public URL\n%s", stderr)
	}
	if out := stderr.String(); !strings.Contains(out, "https://play.example.com/board") || !strings.Contains(out, "public URL") {
		t.Fatalf("stderr does not explain the bad public URL:\n%s", out)
	}
}

// testPublic is the public address the hosted tests configure.
const testPublic = "https://playmat.example"

// startHosted wires hosted mode exactly as runHosted does, through
// hostedPublicURL and newHosted, for the given public address, and serves
// it on a test server.
func startHosted(t *testing.T, public string) *httptest.Server {
	t.Helper()
	u, err := hostedPublicURL(public, ":8080")
	if err != nil {
		t.Fatal(err)
	}
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true}
	_, srv, ui := newHosted(c, u)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ui.StartHub(ctx)
	ts := httptest.NewServer(hostedHandler(srv, ui))
	t.Cleanup(ts.Close)
	return ts
}

// callTool calls a tool with arguments and returns its text, failing the
// test on an error result.
func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
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
	if res.IsError {
		t.Fatalf("%s: %s", name, b.String())
	}
	return b.String()
}

// callObject calls a tool and decodes its result as a JSON object.
func callObject(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	out := callTool(t, cs, name, args)
	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("%s: %v: %s", name, err, out)
	}
	return res
}

func readTestdata(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const (
	testDeckPath = "../../testdata/decks/mega-lucario-ex-v12.txt"
	testLogPath  = "../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt"
)

// claudeState reads Claude's seat view of the board over MCP.
func claudeState(t *testing.T, cs *mcp.ClientSession) state.BoardView {
	t.Helper()
	var st struct {
		Board state.BoardView `json:"board"`
	}
	out := callTool(t, cs, "get_state", nil)
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("get_state: %v: %s", err, out)
	}
	return st.Board
}

func hasCard(cards []state.CardView, iid state.IID) bool {
	for _, c := range cards {
		if c.IID == iid {
			return true
		}
	}
	return false
}

// TestHostedBoardLinks: game-opening results over the hosted connection
// carry a board link at the configured public address, and a different
// setting gives different links from the same build.
func TestHostedBoardLinks(t *testing.T) {
	deck, logText := readTestdata(t, testDeckPath), readTestdata(t, testLogPath)
	for _, tt := range []struct{ public, want string }{
		{testPublic, "https://playmat.example/"},
		{"https://other.example", "https://other.example/"},
	} {
		t.Run(tt.public, func(t *testing.T) {
			ts := startHosted(t, tt.public)
			cs := connectHTTP(t, ts.URL+"/mcp")
			results := map[string]map[string]any{
				"import_log": callObject(t, cs, "import_log", map[string]any{"log": logText, "deck": deck, "id": "links-imp"}),
				"new_game":   callObject(t, cs, "new_game", map[string]any{"deck_p1": deck, "deck_p2": deck, "id": "links-new"}),
			}
			for tool, res := range results {
				if got := res["board"]; got != tt.want {
					t.Errorf("%s board = %v, want %s", tool, got, tt.want)
				}
			}
		})
	}
}

type boardState struct {
	Type     string           `json:"type"`
	Snapshot *game.UISnapshot `json:"snapshot"`
}

type boardResult struct {
	Type  string `json:"type"`
	ID    int    `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// dialBoard opens the board's live connection as a page on origin would.
func dialBoard(t *testing.T, ts *httptest.Server, origin string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{origin}},
	})
	if err == nil {
		t.Cleanup(func() { c.CloseNow() })
	}
	return c, resp, err
}

// readBoard reads board messages until one of type typ arrives or ctx ends.
func readBoard(ctx context.Context, c *websocket.Conn, typ string, out any) error {
	for {
		_, b, err := c.Read(ctx)
		if err != nil {
			return err
		}
		var head struct{ Type string }
		json.Unmarshal(b, &head)
		if head.Type == typ {
			return json.Unmarshal(b, out)
		}
	}
}

// openGame starts a review-mode game over MCP and opens the board from the
// public address, returning the board's first snapshot.
func openGame(t *testing.T) (*mcp.ClientSession, *websocket.Conn, *game.UISnapshot) {
	t.Helper()
	ts := startHosted(t, testPublic)
	cs := connectHTTP(t, ts.URL+"/mcp")
	deck := readTestdata(t, testDeckPath)
	callTool(t, cs, "new_game", map[string]any{"deck_p1": deck, "deck_p2": deck, "id": "live-1"})
	callTool(t, cs, "set_mode", map[string]any{"mode": "review"})

	// The test server's Host is 127.0.0.1, so only the public-host pattern
	// lets this origin in.
	c, _, err := dialBoard(t, ts, testPublic)
	if err != nil {
		t.Fatalf("board from %s: %v", testPublic, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var st boardState
	if err := readBoard(ctx, c, "state", &st); err != nil {
		t.Fatalf("initial state: %v", err)
	}
	if st.Snapshot == nil || st.Snapshot.Game != "live-1" {
		t.Fatalf("initial snapshot %+v, want game live-1", st.Snapshot)
	}
	return cs, c, st.Snapshot
}

// TestHostedBoardFollowsClaude: a move Claude makes over the hosted
// connection reaches an open board within 2 seconds.
func TestHostedBoardFollowsClaude(t *testing.T) {
	cs, c, _ := openGame(t)
	hand := claudeState(t, cs).Sides[state.P2].Hand
	if len(hand) == 0 {
		t.Fatal("Claude has no cards in hand")
	}
	iid := hand[0].IID

	start := time.Now()
	ctx, cancel := context.WithDeadline(context.Background(), start.Add(2*time.Second))
	defer cancel()
	callTool(t, cs, "move", map[string]any{"seat": "p2", "iid": string(iid), "to": "discard"})
	for {
		var st boardState
		if err := readBoard(ctx, c, "state", &st); err != nil {
			t.Fatalf("no board update with %s in Claude's discard within 2s: %v", iid, err)
		}
		if st.Snapshot != nil && hasCard(st.Snapshot.Board.Sides[state.P2].Discard, iid) {
			break
		}
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("board update took %v, want within 2s", elapsed)
	}
	t.Logf("Claude's move reached the board in %v", elapsed)
}

// TestHostedBoardMoveReachesClaude: a card moved from the board shows in
// the state Claude reads back over the hosted connection.
func TestHostedBoardMoveReachesClaude(t *testing.T) {
	cs, c, snap := openGame(t)
	hand := snap.Board.Sides[state.P1].Hand
	if len(hand) == 0 {
		t.Fatal("the board's player has no cards in hand")
	}
	iid := hand[0].IID
	if hasCard(claudeState(t, cs).Sides[state.P1].Discard, iid) {
		t.Fatalf("%s already in the discard", iid)
	}

	b, _ := json.Marshal(map[string]any{"id": 1, "verb": "move", "args": map[string]any{"seat": "p1", "iid": string(iid), "to": "discard"}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
	var r boardResult
	if err := readBoard(ctx, c, "result", &r); err != nil {
		t.Fatalf("move result: %v", err)
	}
	if r.ID != 1 || !r.OK {
		t.Fatalf("board move result %+v, want ok for id 1", r)
	}

	side := claudeState(t, cs).Sides[state.P1]
	if !hasCard(side.Discard, iid) {
		t.Fatalf("Claude's view of p1's discard %v does not show %s", side.Discard, iid)
	}
	if side.HandCount != len(hand)-1 {
		t.Fatalf("Claude sees p1 hand count %d, want %d", side.HandCount, len(hand)-1)
	}
}

// TestHostedBoardAcceptsPublicAddress: the board is served to requests for
// the public host and its live connection accepts pages on the public
// address, but not pages elsewhere.
func TestHostedBoardAcceptsPublicAddress(t *testing.T) {
	ts := startHosted(t, testPublic)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "playmat.example"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<title>tcgreplay</title>") {
		t.Fatalf("GET / with Host playmat.example: %d %q", resp.StatusCode, body)
	}

	if _, _, err := dialBoard(t, ts, testPublic); err != nil {
		t.Fatalf("board from %s refused: %v", testPublic, err)
	}
	if c, resp, err := dialBoard(t, ts, "https://evil.example"); err == nil {
		c.CloseNow()
		t.Fatal("board accepted a live connection from https://evil.example")
	} else if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("evil origin: %v (response %v), want 403", err, resp)
	}
}

// TestDesktopServerHasNoBoardLinks: the desktop callers pass no public
// address, so game-opening results carry no board link.
func TestDesktopServerHasNoBoardLinks(t *testing.T) {
	_, srv := newServer(config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true}, "")
	st, ct := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "desktop-test"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	deck := readTestdata(t, testDeckPath)
	res := callObject(t, cs, "new_game", map[string]any{"deck_p1": deck, "deck_p2": deck, "id": "desk-1"})
	if b, ok := res["board"]; ok {
		t.Fatalf("desktop new_game result has a board link: %v", b)
	}
	if ins := cs.InitializeResult().Instructions; strings.Contains(ins, "board link") {
		t.Fatalf("desktop instructions mention a board link: %q", ins)
	}
}
