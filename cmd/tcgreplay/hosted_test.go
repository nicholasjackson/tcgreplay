package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
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

// TestHostedHandlerServesMCPAndBoard: a client signed in through the
// hosted handler reaches MCP at /mcp and sees the same tools and schemas
// as the desktop, and the board is served on the same address.
func TestHostedHandlerServesMCPAndBoard(t *testing.T) {
	h := startHosted(t, testPublic)
	ts := h.ts

	// MCP over Streamable HTTP at /mcp, signed in.
	cs := h.connect(t)
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

	// The board's routes on the same address, signed in.
	cookie := h.cookie(t)
	if resp, body := getWith(t, ts.URL+"/", cookie, nil); resp.StatusCode != http.StatusOK || !strings.Contains(body, "<title>tcgreplay</title>") {
		t.Fatalf("index: %d %q", resp.StatusCode, body)
	}
	// The board's card handler answers (its own 404), not the mux's.
	if resp, body := getWith(t, ts.URL+"/api/card", cookie, nil); resp.StatusCode != http.StatusNotFound || !strings.Contains(body, "unknown card") {
		t.Fatalf("/api/card without def: %d %q, want the board's 404", resp.StatusCode, body)
	}

	wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer wcancel()
	c, _, err := websocket.Dial(wctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Cookie": []string{cookie.String()}},
	})
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

// hostedConfig is a hosted-mode config with dummy Google credentials: the
// client ID in the config and the secret in the environment for the rest
// of the test. Nothing can sign in with them, as runHosted talks to the
// real Google.
func hostedConfig(t *testing.T, listen, publicURL string) config {
	t.Helper()
	t.Setenv("TCGREPLAY_GOOGLE_CLIENT_SECRET", "dummy-secret")
	return config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true, listen: listen, publicURL: publicURL, googleClientID: "dummy-client"}
}

// mcpPost sends one JSON-RPC message to an MCP endpoint without a sign-in
// (or with the given bearer token) and returns the response, body read.
func mcpPost(t *testing.T, endpoint, token string, msg any) (*http.Response, string) {
	t.Helper()
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

// initializeMsg is an MCP initialize request, as a connector's first call.
var initializeMsg = map[string]any{
	"jsonrpc": "2.0", "id": 1, "method": "initialize",
	"params": map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "anonymous", "version": "1"},
	},
}

// checkRefused fails unless resp is a 401 whose challenge points at the
// protected resource metadata under public.
func checkRefused(t *testing.T, resp *http.Response, body, public string) {
	t.Helper()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous MCP request: %d %q, want 401", resp.StatusCode, body)
	}
	want := `resource_metadata="` + public + `/.well-known/oauth-protected-resource/mcp"`
	if got := resp.Header.Get("WWW-Authenticate"); !strings.Contains(got, want) {
		t.Fatalf("WWW-Authenticate %q does not contain %s", got, want)
	}
}

// waitHealthy polls base's health check until it answers 200, failing if
// exited reports the server stopped first or it takes longer than within.
func waitHealthy(t *testing.T, base string, within time.Duration, exited <-chan error, out fmt.Stringer) {
	t.Helper()
	deadline := time.Now().Add(within)
	for {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case err := <-exited:
			t.Fatalf("hosted mode stopped before serving: %v\n%s", err, out)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("hosted mode did not answer %s/healthz\n%s", base, out)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// googleClientIDAt registers a client with the playmat at base, starts its
// sign-in and returns the client ID the playmat sends the browser to
// Google with, failing unless it is sent to the real Google.
func googleClientIDAt(t *testing.T, base string) string {
	t.Helper()
	resp, err := http.Post(base+"/register", "application/json", strings.NewReader(`{"client_name":"hosted-test","redirect_uris":["`+hostauthtest.LoopbackCallback+`"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var reg struct {
		ClientID string `json:"client_id"`
	}
	json.NewDecoder(resp.Body).Decode(&reg)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || reg.ClientID == "" {
		t.Fatalf("register: %d", resp.StatusCode)
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {reg.ClientID},
		"redirect_uri":          {hostauthtest.LoopbackCallback},
		"state":                 {"st"},
		"code_challenge":        {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"},
		"code_challenge_method": {"S256"},
	}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err = noFollow.Get(base + "/authorize?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc, err := resp.Location()
	if err != nil {
		t.Fatalf("authorize: %d with no redirect: %v", resp.StatusCode, err)
	}
	if loc.Host != "accounts.google.com" {
		t.Fatalf("authorize sent the browser to %s, want Google", loc)
	}
	return loc.Query().Get("client_id")
}

// TestRunHostedServesAndShutsDown runs hosted mode on one port, checks it
// answers its health check and refuses an anonymous connector with the
// public address, then stops it as a container stop would.
func TestRunHostedServesAndShutsDown(t *testing.T) {
	logs := captureLog(t)
	base := "http://127.0.0.1:" + freePort(t)
	c := hostedConfig(t, strings.TrimPrefix(base, "http://"), "https://play.example.com")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runHosted(ctx, c) }()

	waitHealthy(t, base, 5*time.Second, done, logs)

	// Sign-in is on, and its addresses come from the public URL.
	resp, body := mcpPost(t, base+"/mcp", "", initializeMsg)
	checkRefused(t, resp, body, "https://play.example.com")

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
	for _, want := range []string{"https://play.example.com", "Google sign-in", "stopped"} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q does not mention %q", out, want)
		}
	}
}

// TestRunHostedRejectsBadPublicURL: with credentials present, a bad public
// URL stops start-up with an error naming it, before anything listens.
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
// hosted mode with the environment they set, plus any flags in
// HELPER_ARGS.
func TestHostedMainHelper(t *testing.T) {
	if os.Getenv("TCGREPLAY_HOSTED_HELPER") != "1" {
		t.Skip("helper process only")
	}
	dir := os.Getenv("HELPER_DATA")
	os.Args = append([]string{"tcgreplay", "-hosted", "-data", dir, "-cache", dir, "-offline"}, strings.Fields(os.Getenv("HELPER_ARGS"))...)
	main()
	os.Exit(0)
}

// helperCmd prepares a run of main in hosted mode in a subprocess with env
// added to (and overriding) this process's environment.
// syncBuffer is a bytes.Buffer safe to read while a subprocess writes it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func helperCmd(t *testing.T, env ...string) (*exec.Cmd, *syncBuffer, string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cmd := exec.Command(exe, "-test.run=^TestHostedMainHelper$")
	cmd.Env = append(os.Environ(), append([]string{"TCGREPLAY_HOSTED_HELPER=1", "HELPER_DATA=" + dir}, env...)...)
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	return cmd, stderr, dir
}

// TestMainHostedFollowsEnvAndStopsOnSIGTERM runs the binary's main in
// hosted mode with PORT, TCGREPLAY_PUBLIC_URL and the Google credentials
// set, then stops it the way a container platform does.
func TestMainHostedFollowsEnvAndStopsOnSIGTERM(t *testing.T) {
	port := freePort(t)
	cmd, stderr, dir := helperCmd(t, "PORT="+port, "TCGREPLAY_PUBLIC_URL=https://play.example.com/",
		"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() { cmd.Process.Kill() })

	base := "http://127.0.0.1:" + port
	waitHealthy(t, base, 10*time.Second, exited, stderr)
	resp, body := mcpPost(t, base+"/mcp", "", initializeMsg)
	checkRefused(t, resp, body, "https://play.example.com")
	if got := googleClientIDAt(t, base); got != "env-client" {
		t.Errorf("signs in with Google client %q, want env-client from TCGREPLAY_GOOGLE_CLIENT_ID", got)
	}

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
	cmd, stderr, _ := helperCmd(t, "PORT="+freePort(t), "TCGREPLAY_PUBLIC_URL=https://play.example.com/board",
		"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret")
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

// hostedServer is hosted mode served on a test server for one test.
type hostedServer struct {
	ts *httptest.Server
	// g is the fake Google its users sign in with.
	g *hostauthtest.FakeGoogle
	// m is its game manager.
	m *game.Manager
	// public is the address it is configured to be reached at.
	public string
}

// connect signs a connector in as g's current account, the way claude.ai
// does, and returns its MCP session.
func (h *hostedServer) connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "hosted-test"}, nil).Connect(ctx, hostauthtest.MCPTransport(t, h.public, h.ts.URL, h.g), nil)
	if err != nil {
		t.Fatalf("signed-in connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// cookie signs a browser in to the board as g's current account and
// returns its session cookie.
func (h *hostedServer) cookie(t *testing.T) *http.Cookie {
	t.Helper()
	return hostauthtest.BoardCookie(t, h.public, h.ts.URL, h.g)
}

// getWith fetches u with the board cookie (none if nil), without following
// redirects, and returns the response with its body read. mod, if set,
// may change the request first.
func getWith(t *testing.T, u string, cookie *http.Cookie, mod func(*http.Request)) (*http.Response, string) {
	t.Helper()
	return sendWith(t, http.MethodGet, u, cookie, mod)
}

// sendWith makes one request with the board cookie (none if nil), without
// following redirects, and returns the response with its body read.
func sendWith(t *testing.T, method, u string, cookie *http.Cookie, mod func(*http.Request)) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if mod != nil {
		mod(req)
	}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, u, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

// startHosted wires hosted mode exactly as runHosted does, through
// hostedPublicURL, hostedAuth and newHosted, for the given public address
// with a fake Google, and serves it on a test server.
func startHosted(t *testing.T, public string) *hostedServer {
	t.Helper()
	return startHostedWith(t, public, nil)
}

// startHostedWith is startHosted with mod, if set, changing the config
// first.
func startHostedWith(t *testing.T, public string, mod func(*config)) *hostedServer {
	t.Helper()
	u, err := hostedPublicURL(public, ":8080")
	if err != nil {
		t.Fatal(err)
	}
	g := hostauthtest.NewGoogle(t)
	t.Setenv("TCGREPLAY_GOOGLE_CLIENT_SECRET", g.ClientSecret)
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true, googleClientID: g.ClientID}
	if mod != nil {
		mod(&c)
	}
	a, err := hostedAuth(c, u, g.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	m, srv, ui := newHosted(c, u, a)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ui.StartHub(ctx)
	ts := httptest.NewServer(hostedHandler(srv, ui, a))
	t.Cleanup(ts.Close)
	return &hostedServer{ts: ts, g: g, m: m, public: u.String()}
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
			cs := startHosted(t, tt.public).connect(t)
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

// dialBoard opens the board's live connection as a page on origin would,
// with the board cookie (none if nil).
func dialBoard(t *testing.T, ts *httptest.Server, origin string, cookie *http.Cookie) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hdr := http.Header{"Origin": []string{origin}}
	if cookie != nil {
		hdr.Set("Cookie", cookie.String())
	}
	c, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/ws", &websocket.DialOptions{HTTPHeader: hdr})
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

// openGame starts a review-mode game over MCP and opens the board, signed
// in, from the public address, returning the board's first snapshot.
func openGame(t *testing.T) (*mcp.ClientSession, *websocket.Conn, *game.UISnapshot) {
	t.Helper()
	h := startHosted(t, testPublic)
	cs := h.connect(t)
	c, snap := openGameOn(t, h, cs, h.cookie(t))
	return cs, c, snap
}

// openGameOn starts a review-mode game over cs and opens h's board from
// the public address with the board cookie, returning the board's first
// snapshot.
func openGameOn(t *testing.T, h *hostedServer, cs *mcp.ClientSession, cookie *http.Cookie) (*websocket.Conn, *game.UISnapshot) {
	t.Helper()
	deck := readTestdata(t, testDeckPath)
	callTool(t, cs, "new_game", map[string]any{"deck_p1": deck, "deck_p2": deck, "id": "live-1"})
	callTool(t, cs, "set_mode", map[string]any{"mode": "review"})

	// The test server's Host is 127.0.0.1, so only the public-host pattern
	// lets this origin in.
	c, _, err := dialBoard(t, h.ts, testPublic, cookie)
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
	return c, st.Snapshot
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

// TestHostedBoardAcceptsPublicAddress: the board is served to signed-in
// requests for the public host and its live connection accepts pages on
// the public address, but not pages elsewhere, even signed in.
func TestHostedBoardAcceptsPublicAddress(t *testing.T) {
	h := startHosted(t, testPublic)
	ts := h.ts
	cookie := h.cookie(t)

	resp, body := getWith(t, ts.URL+"/", cookie, func(r *http.Request) { r.Host = "playmat.example" })
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "<title>tcgreplay</title>") {
		t.Fatalf("GET / with Host playmat.example: %d %q", resp.StatusCode, body)
	}

	if _, _, err := dialBoard(t, ts, testPublic, cookie); err != nil {
		t.Fatalf("board from %s refused: %v", testPublic, err)
	}
	if c, resp, err := dialBoard(t, ts, "https://evil.example", cookie); err == nil {
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

// credentialsError is the start-up message for missing Google credentials.
const credentialsError = "hosted mode needs Google sign-in: set TCGREPLAY_GOOGLE_CLIENT_ID and TCGREPLAY_GOOGLE_CLIENT_SECRET"

// TestEnvInt: a whole-number setting falls back to its default when unset,
// and when not a whole number with a warning naming it; 0 is a value.
func TestEnvInt(t *testing.T) {
	tests := []struct {
		name, value string
		want        int
		warn        bool
	}{
		{"unset", "", 300, false},
		{"zero", "0", 0, false},
		{"number", "25", 25, false},
		{"not a number", "abc", 300, true},
		{"negative", "-1", 300, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLog(t)
			t.Setenv("TCGREPLAY_TEST_INT", tt.value)
			if got := envInt("TCGREPLAY_TEST_INT", 300); got != tt.want {
				t.Fatalf("envInt with %q = %d, want %d", tt.value, got, tt.want)
			}
			out := logs.String()
			if warned := strings.Contains(out, "warning"); warned != tt.warn {
				t.Fatalf("warning logged = %v, want %v (log: %q)", warned, tt.warn, out)
			}
			if tt.warn && (!strings.Contains(out, "TCGREPLAY_TEST_INT") || !strings.Contains(out, tt.value)) {
				t.Fatalf("warning %q does not name the setting and its value %q", out, tt.value)
			}
		})
	}
}

// TestHostedAuthNeedsCredentials: hosted mode's sign-in takes the client
// ID from the config and the secret from the environment, and refuses to
// build with either missing.
func TestHostedAuthNeedsCredentials(t *testing.T) {
	public := &url.URL{Scheme: "https", Host: "playmat.example"}
	tests := []struct {
		name, id, secret string
		ok               bool
	}{
		{"both", "client-id", "client-secret", true},
		{"no client ID", "", "client-secret", false},
		{"no secret", "client-id", "", false},
		{"neither", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TCGREPLAY_GOOGLE_CLIENT_SECRET", tt.secret)
			a, err := hostedAuth(config{googleClientID: tt.id}, public, oauth2.Endpoint{})
			if tt.ok {
				if err != nil || a == nil {
					t.Fatalf("hostedAuth with credentials: %v, %v", a, err)
				}
				return
			}
			if err == nil || err.Error() != credentialsError {
				t.Fatalf("hostedAuth with ID %q and secret %q: %v, want %q", tt.id, tt.secret, err, credentialsError)
			}
		})
	}
}

// TestRunHostedNeedsCredentials: without Google credentials hosted mode
// stops at start-up with the message, before anything listens.
func TestRunHostedNeedsCredentials(t *testing.T) {
	port := freePort(t)
	for _, tt := range []struct{ name, id, secret string }{
		{"no client ID", "", "client-secret"},
		{"no secret", "client-id", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLog(t)
			t.Setenv("TCGREPLAY_GOOGLE_CLIENT_SECRET", tt.secret)
			c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true, listen: "127.0.0.1:" + port, publicURL: "https://play.example.com", googleClientID: tt.id}
			done := make(chan error, 1)
			go func() { done <- runHosted(context.Background(), c) }()
			select {
			case err := <-done:
				if err == nil || err.Error() != credentialsError {
					t.Fatalf("runHosted: %v, want %q", err, credentialsError)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("runHosted without credentials did not stop")
			}
			ln, err := net.Listen("tcp", "127.0.0.1:"+port)
			if err != nil {
				t.Fatalf("port %s in use after a start without credentials: %v", port, err)
			}
			ln.Close()
			if strings.Contains(logs.String(), "listening") {
				t.Errorf("logged listening without credentials: %q", logs.String())
			}
		})
	}
}

// TestMainHostedNeedsCredentials: the binary in hosted mode exits non-zero
// with the message on stderr when either credential is missing, whether
// the client ID would come from the environment or the flag.
func TestMainHostedNeedsCredentials(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  []string
	}{
		{"neither", []string{"TCGREPLAY_GOOGLE_CLIENT_ID=", "TCGREPLAY_GOOGLE_CLIENT_SECRET="}},
		{"no secret", []string{"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET="}},
		{"no client ID", []string{"TCGREPLAY_GOOGLE_CLIENT_ID=", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret"}},
		{"flag client ID, no secret", []string{"TCGREPLAY_GOOGLE_CLIENT_ID=", "TCGREPLAY_GOOGLE_CLIENT_SECRET=", "HELPER_ARGS=-google-client-id flag-client"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			port := freePort(t)
			cmd, stderr, _ := helperCmd(t, append([]string{"PORT=" + port, "TCGREPLAY_PUBLIC_URL=https://play.example.com"}, tt.env...)...)
			done := make(chan error, 1)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				if err == nil {
					t.Fatalf("hosted main started without credentials\n%s", stderr)
				}
			case <-time.After(10 * time.Second):
				cmd.Process.Kill()
				t.Fatalf("hosted main did not stop without credentials\n%s", stderr)
			}
			if out := stderr.String(); !strings.Contains(out, credentialsError) {
				t.Fatalf("stderr does not explain the missing credentials:\n%s", out)
			}
			if out := stderr.String(); strings.Contains(out, "listening") {
				t.Fatalf("hosted main listened without credentials:\n%s", out)
			}
		})
	}
}

// TestMainHostedGoogleClientIDFlag: -google-client-id overrides
// TCGREPLAY_GOOGLE_CLIENT_ID, and the playmat sends users to Google with it.
func TestMainHostedGoogleClientIDFlag(t *testing.T) {
	port := freePort(t)
	cmd, stderr, _ := helperCmd(t, "PORT="+port, "TCGREPLAY_PUBLIC_URL=https://play.example.com",
		"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret",
		"HELPER_ARGS=-google-client-id flag-client")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() { cmd.Process.Kill() })

	base := "http://127.0.0.1:" + port
	waitHealthy(t, base, 10*time.Second, exited, stderr)
	if got := googleClientIDAt(t, base); got != "flag-client" {
		t.Fatalf("signs in with Google client %q, want flag-client from -google-client-id", got)
	}
	if strings.Contains(stderr.String(), "env-secret") {
		t.Errorf("the secret shows in the log:\n%s", stderr)
	}
}

// TestHostedAnonymousConnectorRefused: in hosted mode a connector request
// without a sign-in is refused with a pointer to the public address's
// protected resource metadata, the refusal is logged, and no tool runs.
func TestHostedAnonymousConnectorRefused(t *testing.T) {
	h := startHosted(t, testPublic)
	logs := captureLog(t)
	deck := readTestdata(t, testDeckPath)
	newGame := map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": "new_game", "arguments": map[string]any{"deck_p1": deck, "deck_p2": deck, "id": "anon-1"}},
	}

	for _, tt := range []struct {
		name, token string
		msg         any
		log         string
	}{
		{"initialize", "", initializeMsg, "hostauth: refused POST /mcp: no bearer token"},
		{"new_game", "", newGame, "hostauth: refused POST /mcp: no bearer token"},
		{"new_game with an unknown token", "not-a-token", newGame, "hostauth: refused POST /mcp: unknown or expired token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logs.Reset()
			resp, body := mcpPost(t, h.ts.URL+"/mcp", tt.token, tt.msg)
			checkRefused(t, resp, body, testPublic)
			if !strings.Contains(logs.String(), tt.log) {
				t.Errorf("log %q does not record %q", logs.String(), tt.log)
			}
		})
	}

	if g, err := h.m.Current(); err == nil {
		t.Fatalf("an anonymous new_game opened game %s", g.ID)
	}
	// Signed in, the playmat still has no game open.
	res, err := h.connect(t).CallTool(context.Background(), &mcp.CallToolParams{Name: "get_state"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("get_state after anonymous requests found a game open")
	}
}

// TestHostedAccountsSignIn: two different Google accounts can each sign a
// connector in and use the playmat, the first still working after the
// second signs in.
func TestHostedAccountsSignIn(t *testing.T) {
	h := startHosted(t, testPublic)
	one := h.connect(t)
	h.g.SetAccount(hostauthtest.Account{ID: "sub-2", Email: "two@example.com"})
	two := h.connect(t)
	for name, cs := range map[string]*mcp.ClientSession{"second": two, "first": one} {
		if out := callText(t, cs, "ping"); out != "pong" {
			t.Errorf("%s account's ping: %q, want pong", name, out)
		}
	}
}

// TestHostedHealthz: the health check answers without a sign-in.
func TestHostedHealthz(t *testing.T) {
	h := startHosted(t, testPublic)
	code, body := get(t, h.ts.URL+"/healthz")
	if code != http.StatusOK || strings.TrimSpace(body) != "ok" {
		t.Fatalf("GET /healthz: %d %q, want 200 ok", code, body)
	}
}

// TestAppSpecHealthCheckAndSecrets: the App Platform definition checks
// health at /healthz and declares the Google credentials as secrets, with
// no value in the file.
func TestAppSpecHealthCheckAndSecrets(t *testing.T) {
	lines := strings.Split(readTestdata(t, "../../.do/app.yaml"), "\n")
	var health bool
	for i, l := range lines {
		if strings.TrimSpace(l) == "health_check:" {
			for _, next := range lines[i+1:] {
				if strings.TrimSpace(next) == "" || strings.HasPrefix(strings.TrimSpace(next), "#") {
					continue
				}
				health = strings.TrimSpace(next) == "http_path: /healthz"
				break
			}
		}
	}
	if !health {
		t.Error("app.yaml has no health_check with http_path: /healthz")
	}

	// fields returns the settings of the env entry for key.
	fields := func(key string) map[string]string {
		for i, l := range lines {
			if strings.TrimSpace(l) != "- key: "+key {
				continue
			}
			out := map[string]string{}
			indent := len(l) - len(strings.TrimLeft(l, " ")) + 2
			for _, next := range lines[i+1:] {
				trimmed := strings.TrimSpace(next)
				if trimmed == "" || strings.HasPrefix(trimmed, "#") {
					continue
				}
				if len(next)-len(strings.TrimLeft(next, " ")) != indent || strings.HasPrefix(trimmed, "-") {
					break
				}
				k, v, _ := strings.Cut(trimmed, ":")
				out[k] = strings.TrimSpace(v)
			}
			return out
		}
		return nil
	}
	for _, key := range []string{"TCGREPLAY_GOOGLE_CLIENT_ID", "TCGREPLAY_GOOGLE_CLIENT_SECRET"} {
		f := fields(key)
		if f == nil {
			t.Errorf("app.yaml declares no %s", key)
			continue
		}
		if f["type"] != "SECRET" {
			t.Errorf("%s has type %q, want SECRET", key, f["type"])
		}
		if v, ok := f["value"]; ok {
			t.Errorf("%s has a value in app.yaml: %q", key, v)
		}
	}
}

// TestHostedBoardSignInReturnsToPage: opening a board link while signed
// out is sent through Google sign-in, query and all, and a browser
// following it lands on the board page it asked for, signed in.
func TestHostedBoardSignInReturnsToPage(t *testing.T) {
	h := startHosted(t, testPublic)
	for path, next := range map[string]string{"/": "%2F", "/?x=1": "%2F%3Fx%3D1"} {
		resp, _ := getWith(t, h.ts.URL+path, nil, nil)
		if want := "/auth/login?next=" + next; resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != want {
			t.Errorf("GET %s signed out: %d to %q, want 302 to %s", path, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}

	browser := hostauthtest.Browser(t, h.public, h.ts.URL)
	resp, err := browser.Get(h.public + "/?x=1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<title>tcgreplay</title>") {
		t.Fatalf("board link after sign-in: %d %q, want the board", resp.StatusCode, body)
	}
	// The test browser serves the public host from the test server, so the
	// final request names the test server; its path is what counts.
	if got := resp.Request.URL.RequestURI(); got != "/?x=1" {
		t.Errorf("sign-in ended at %s, want /?x=1", got)
	}
	if len(h.g.Prompts()) != 1 {
		t.Errorf("Google asked %d times, want once", len(h.g.Prompts()))
	}
	u, _ := url.Parse(h.public)
	var signedIn bool
	for _, c := range browser.Jar.Cookies(u) {
		signedIn = signedIn || c.Name == "tcgreplay_session"
	}
	if !signedIn {
		t.Error("the browser holds no board session after signing in")
	}
}

// TestHostedBoardSignedOutRefused: without a sign-in, or with an unknown
// session, the board's live connection and card lookups are refused with
// 401, so no verb can reach the game; /auth/session says who is signed
// in, or 401.
func TestHostedBoardSignedOutRefused(t *testing.T) {
	h := startHosted(t, testPublic)
	cs := h.connect(t)
	deck := readTestdata(t, testDeckPath)
	callTool(t, cs, "new_game", map[string]any{"deck_p1": deck, "deck_p2": deck, "id": "anon-board"})
	callTool(t, cs, "set_mode", map[string]any{"mode": "review"})
	before, _ := json.Marshal(claudeState(t, cs))

	unknown := &http.Cookie{Name: "tcgreplay_session", Value: "not-a-session"}
	for name, cookie := range map[string]*http.Cookie{"no cookie": nil, "unknown cookie": unknown} {
		t.Run(name, func(t *testing.T) {
			if c, resp, err := dialBoard(t, h.ts, testPublic, cookie); err == nil {
				c.CloseNow()
				t.Fatal("board accepted a live connection without a sign-in")
			} else if resp == nil || resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("live connection without a sign-in: %v (response %v), want 401", err, resp)
			}
			if resp, body := getWith(t, h.ts.URL+"/api/card?def=x", cookie, nil); resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("/api/card without a sign-in: %d %q, want 401", resp.StatusCode, body)
			}
			if resp, body := getWith(t, h.ts.URL+"/auth/session", cookie, nil); resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("/auth/session without a sign-in: %d %q, want 401", resp.StatusCode, body)
			}
		})
	}
	if after, _ := json.Marshal(claudeState(t, cs)); !bytes.Equal(before, after) {
		t.Errorf("the game changed while the board was signed out:\nbefore %s\nafter  %s", before, after)
	}

	resp, body := getWith(t, h.ts.URL+"/auth/session", h.cookie(t), nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"email":"`+hostauthtest.DefaultAccount.Email+`"`) {
		t.Errorf("/auth/session signed in: %d %q, want 200 with the email", resp.StatusCode, body)
	}
}

// TestHostedSignOutClosesBoard: signing out from the playmat ends the
// session, closes its open live connection within 2 seconds, and the next
// page load with the old cookie is sent to sign in.
func TestHostedSignOutClosesBoard(t *testing.T) {
	h := startHosted(t, testPublic)
	cs := h.connect(t)
	cookie := h.cookie(t)
	c, _ := openGameOn(t, h, cs, cookie)

	resp, _ := sendWith(t, http.MethodPost, h.ts.URL+"/auth/logout", cookie, func(r *http.Request) { r.Header.Set("Origin", testPublic) })
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/auth/signed-out" {
		t.Fatalf("sign-out: %d to %q, want 303 to /auth/signed-out", resp.StatusCode, resp.Header.Get("Location"))
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for {
		if _, _, err := c.Read(ctx); err != nil {
			if ctx.Err() != nil {
				t.Fatalf("live connection still open 2s after sign-out")
			}
			break
		}
	}
	t.Logf("live connection closed %v after sign-out", time.Since(start))

	resp, _ = getWith(t, h.ts.URL+"/", cookie, nil)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/auth/login?next=%2F" {
		t.Errorf("GET / after sign-out: %d to %q, want 302 to sign-in", resp.StatusCode, resp.Header.Get("Location"))
	}
	if _, resp, err := dialBoard(t, h.ts, testPublic, cookie); err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("live connection after sign-out: %v (response %v), want 401", err, resp)
	}
	// Signing out of the board leaves the connector signed in.
	if out := callText(t, cs, "ping"); out != "pong" {
		t.Errorf("connector after board sign-out: ping %q, want pong", out)
	}
}

// toolResult calls a tool and returns its text and whether it is a tool
// error, failing the test only on a protocol error.
func toolResult(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
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

// spendUntilRefused calls ping until the account is refused, failing if
// that takes more than max calls, and returns the refusal's text.
func spendUntilRefused(t *testing.T, cs *mcp.ClientSession, max int) string {
	t.Helper()
	for i := 0; i < max; i++ {
		if out, isErr := toolResult(t, cs, "ping", nil); isErr {
			return out
		}
	}
	t.Fatalf("still not refused after %d calls", max)
	return ""
}

// checkLimitMessage fails unless msg is the over-the-limit message naming
// a limit of limit requests a minute.
func checkLimitMessage(t *testing.T, where, msg string, limit int) {
	t.Helper()
	for _, want := range []string{"too many requests", fmt.Sprintf("limited to %d requests a minute", limit), "try again in"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%s: %q does not contain %q", where, msg, want)
		}
	}
}

// TestHostedRateLimit: with a limit of 5, an account that has spent its
// allowance gets the limit's message from a tool call, from a board action
// (which does not run) and from a card request (429), while a second
// account keeps working.
func TestHostedRateLimit(t *testing.T) {
	h := startHostedWith(t, testPublic, func(c *config) { c.rateLimit = 5 })
	cs := h.connect(t)
	cookie := h.cookie(t)
	c, snap := openGameOn(t, h, cs, cookie)
	hand := snap.Board.Sides[state.P1].Hand
	if len(hand) == 0 {
		t.Fatal("the board's player has no cards in hand")
	}
	iid := hand[0].IID

	// Initialize, new_game and set_mode have counted too.
	checkLimitMessage(t, "tool call over the limit", spendUntilRefused(t, cs, 5), 5)

	// A board action over the limit is refused and the board is unchanged.
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
	if r.ID != 1 || r.OK {
		t.Fatalf("board move over the limit: %+v, want refused", r)
	}
	checkLimitMessage(t, "board move over the limit", r.Error, 5)
	g, err := h.m.Current()
	if err != nil {
		t.Fatal(err)
	}
	if loc, _ := g.State().Find(iid); loc.Zone != state.Hand {
		t.Fatalf("refused board move moved %s to %v", iid, loc)
	}

	// A card request over the limit gets 429 with the message.
	resp, body := getWith(t, h.ts.URL+"/api/card?def=x", cookie, nil)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("/api/card over the limit: %d %q, want 429", resp.StatusCode, body)
	}
	checkLimitMessage(t, "/api/card over the limit", body, 5)

	// A second account works while the first is still refused.
	h.g.SetAccount(hostauthtest.Account{ID: "sub-2", Email: "two@example.com"})
	two := h.connect(t)
	if out, isErr := toolResult(t, two, "ping", nil); isErr || out != "pong" {
		t.Errorf("second account's ping: %q (error %v), want pong", out, isErr)
	}
	if resp, body := getWith(t, h.ts.URL+"/api/card?def=x", h.cookie(t), nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("second account's /api/card: %d %q, want the board's 404", resp.StatusCode, body)
	}
	if out, isErr := toolResult(t, cs, "ping", nil); !isErr {
		t.Errorf("first account's ping after the second's: %q, want still refused", out)
	}
}

// TestHostedRateLimitSetting: hosted mode takes its limit from the config:
// a limit of 1 refuses the first tool call after connecting (initialize
// spent it), and 0 never refuses.
func TestHostedRateLimitSetting(t *testing.T) {
	one := startHostedWith(t, testPublic, func(c *config) { c.rateLimit = 1 }).connect(t)
	out, isErr := toolResult(t, one, "ping", nil)
	if !isErr {
		t.Fatalf("ping with a limit of 1 after initialize: %q, want refused", out)
	}
	checkLimitMessage(t, "limit of 1", out, 1)

	off := startHostedWith(t, testPublic, func(c *config) { c.rateLimit = 0 }).connect(t)
	for i := 0; i < 400; i++ {
		if out, isErr := toolResult(t, off, "ping", nil); isErr {
			t.Fatalf("ping %d with the limit off refused: %q", i+1, out)
		}
	}
}

// TestDesktopServerHasNoLimit: the desktop MCP server counts nothing, so
// more calls than the hosted default never get refused.
func TestDesktopServerHasNoLimit(t *testing.T) {
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
	for i := 0; i < 400; i++ {
		if out, isErr := toolResult(t, cs, "ping", nil); isErr {
			t.Fatalf("desktop ping %d refused: %q", i+1, out)
		}
	}
}

// TestMainHostedRateLimitSetting: the binary's limit is 300 requests a
// minute when TCGREPLAY_RATE_LIMIT is unset, follows the variable (0 turns
// it off) and the -rate-limit flag over it, each read at start-up.
func TestMainHostedRateLimitSetting(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  []string
		want string
	}{
		{"default", []string{"TCGREPLAY_RATE_LIMIT="}, "300 requests a minute per account"},
		{"env", []string{"TCGREPLAY_RATE_LIMIT=40"}, "40 requests a minute per account"},
		{"off", []string{"TCGREPLAY_RATE_LIMIT=0"}, "0 requests a minute per account"},
		{"flag", []string{"TCGREPLAY_RATE_LIMIT=40", "HELPER_ARGS=-rate-limit 7"}, "7 requests a minute per account"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			port := freePort(t)
			cmd, stderr, _ := helperCmd(t, append([]string{"PORT=" + port, "TCGREPLAY_PUBLIC_URL=https://play.example.com",
				"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret"}, tt.env...)...)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			t.Cleanup(func() { cmd.Process.Kill() })
			waitHealthy(t, "http://127.0.0.1:"+port, 10*time.Second, exited, stderr)
			if out := stderr.String(); !strings.Contains(out, tt.want) {
				t.Errorf("start-up log does not say %q:\n%s", tt.want, out)
			}
		})
	}
}
