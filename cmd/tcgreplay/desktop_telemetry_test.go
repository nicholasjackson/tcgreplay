package main

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/observe/observetest"
)

// grafanaEnv points every TCGREPLAY_GRAFANA_* setting at r, trusting its
// certificate, for the rest of the test.
func grafanaEnv(t *testing.T, r *observetest.Receiver) {
	t.Setenv("SSL_CERT_FILE", r.CertFile(t))
	t.Setenv("TCGREPLAY_GRAFANA_OTLP_URL", r.URL)
	t.Setenv("TCGREPLAY_GRAFANA_INSTANCE_ID", "123456")
	t.Setenv("TCGREPLAY_GRAFANA_TOKEN", grafanaToken)
	t.Setenv("TCGREPLAY_GRAFANA_METRICS_URL", strings.TrimSuffix(r.URL, "/otlp")+"/api/prom")
	t.Setenv("TCGREPLAY_GRAFANA_METRICS_USER", "654321")
}

// TestDesktopSendsNothing: the in-process playmat and the daemon, run with
// every Grafana setting pointing at a receiver and used (a game, the board
// page and a board action), make no request to it.
func TestDesktopSendsNothing(t *testing.T) {
	captureSyncLog(t)
	r := observetest.New(t)
	grafanaEnv(t, r)
	deckText := readTestdata(t, testDeckPath)

	t.Run("in-process", func(t *testing.T) {
		_, cs := desktopOn(t, config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true})
		callObject(t, cs, "new_game", map[string]any{"deck_p1": deckText, "deck_p2": deckText, "id": "desk"})
	})

	t.Run("daemon", func(t *testing.T) {
		ui := "127.0.0.1:" + freePort(t)
		c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true, uiAddr: ui}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- runDaemon(ctx, c, false) }()
		defer func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("runDaemon: %v", err)
			}
		}()
		var conn net.Conn
		deadline := time.Now().Add(5 * time.Second)
		for {
			var err error
			if conn, err = net.Dial("unix", socketPath(c)); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("daemon socket: %v", err)
			}
			time.Sleep(10 * time.Millisecond)
		}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "daemon-test"}, nil).Connect(ctx, &mcp.IOTransport{Reader: conn, Writer: conn}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cs.Close()
		callObject(t, cs, "new_game", map[string]any{"deck_p1": deckText, "deck_p2": deckText, "id": "desk"})

		for {
			if code, _ := get(t, "http://"+ui+"/"); code == 200 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("desktop board page never served")
			}
			time.Sleep(10 * time.Millisecond)
		}
		wctx, wcancel := context.WithTimeout(ctx, 5*time.Second)
		defer wcancel()
		ws, _, err := websocket.Dial(wctx, "ws://"+ui+"/ws", nil)
		if err != nil {
			t.Fatalf("desktop board connection: %v", err)
		}
		defer ws.CloseNow()
		ws.SetReadLimit(-1)
		b, _ := json.Marshal(map[string]any{"id": 1, "verb": "move", "args": map[string]any{"iid": "p1-1", "to": "nowhere"}})
		if err := ws.Write(wctx, websocket.MessageText, b); err != nil {
			t.Fatal(err)
		}
		var res boardResult
		if err := readBoard(wctx, ws, "result", &res); err != nil || res.ID != 1 {
			t.Fatalf("board action result %+v: %v", res, err)
		}
	})

	if got := r.Paths(); len(got) != 0 {
		t.Fatalf("the desktop playmat sent %d requests to the receiver: %v", len(got), got)
	}
}

// TestOnlyHostedStartsTelemetry: of the playmat's own code, only hosted
// mode's wiring (hosted.go) calls into the telemetry package; main.go may
// name its Settings type for the config, and nothing else touches it.
func TestOnlyHostedStartsTelemetry(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "observe" {
				switch {
				case f == "hosted.go":
				case f == "main.go" && sel.Sel.Name == "Settings":
				default:
					t.Errorf("%s uses observe.%s; only hosted.go may", f, sel.Sel.Name)
				}
			}
			return true
		})
	}
}
