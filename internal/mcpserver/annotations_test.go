package mcpserver

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
)

// TestToolAnnotations: every tool carries behaviour hints. The reads are
// read-only (so clients need not ask before them), the tools that replace
// the game are destructive, every other write is not, and none is
// open-world. Every name in the read-only and replacing sets is a real
// tool (get_account on a server with an Account hook), so a rename cannot
// silently drop a hint.
func TestToolAnnotations(t *testing.T) {
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			cs, _ := connectServer(t, tr, &Server{Version: "test", Account: fixedAccount("sub-1")})
			checkToolAnnotations(t, cs)
		})
	}
}

// fixedAccount is an Account hook that always answers id.
func fixedAccount(id string) func(context.Context, *mcp.RequestExtra) (string, error) {
	return func(context.Context, *mcp.RequestExtra) (string, error) { return id, nil }
}

// checkToolAnnotations checks the behaviour hints of every tool cs lists.
func checkToolAnnotations(t *testing.T, cs *mcp.ClientSession) {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, tool := range res.Tools {
		seen[tool.Name] = true
		a := tool.Annotations
		if a == nil {
			t.Errorf("%s: no annotations", tool.Name)
			continue
		}
		if a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Errorf("%s: open-world hint %v, want false", tool.Name, a.OpenWorldHint)
		}
		if readOnlyTools[tool.Name] {
			if !a.ReadOnlyHint {
				t.Errorf("%s: not marked read-only", tool.Name)
			}
			continue
		}
		if a.ReadOnlyHint {
			t.Errorf("%s: write marked read-only", tool.Name)
		}
		if a.DestructiveHint == nil || *a.DestructiveHint != replacingTools[tool.Name] {
			t.Errorf("%s: destructive hint %v, want %v", tool.Name, a.DestructiveHint, replacingTools[tool.Name])
		}
	}
	for _, set := range []map[string]bool{readOnlyTools, replacingTools} {
		for name := range set {
			if !seen[name] {
				t.Errorf("%s is annotated but is not a tool", name)
			}
		}
	}
}

// rawTool is one entry of a tools/list result as it came over the wire,
// each field kept as raw JSON so an omitted field shows as missing rather
// than as Go's zero value.
type rawTool map[string]json.RawMessage

// rawToolList connects to s through tr with a bare JSON-RPC connection,
// not the SDK's client (whose types drop an omitted false), and returns
// the tools/list result's tools exactly as the server encoded them.
func rawToolList(t *testing.T, tr transport, s *Server) []rawTool {
	t.Helper()
	tools, _ := rawToolListBytes(t, tr, s)
	return tools
}

// rawToolListBytes is rawToolList that also returns the whole tools/list
// result's JSON.
func rawToolListBytes(t *testing.T, tr transport, s *Server) ([]rawTool, []byte) {
	t.Helper()
	if s.M == nil && s.Resolve == nil {
		s.M = game.NewManager(t.TempDir(), nil)
	}
	return rawToolListFrom(t, tr, s.MCP())
}

// rawToolListFrom is rawToolListBytes for an already built MCP server.
func rawToolListFrom(t *testing.T, tr transport, srv *mcp.Server) ([]rawTool, []byte) {
	t.Helper()
	ctx := context.Background()
	conn, err := tr.dial(t, srv).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	request := func(id int64, method, params string) json.RawMessage {
		t.Helper()
		rid, err := jsonrpc.MakeID(float64(id))
		if err != nil {
			t.Fatal(err)
		}
		if err := conn.Write(ctx, &jsonrpc.Request{ID: rid, Method: method, Params: json.RawMessage(params)}); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		for {
			msg, err := conn.Read(ctx)
			if err != nil {
				t.Fatalf("%s: %v", method, err)
			}
			if r, ok := msg.(*jsonrpc.Response); ok && r.ID == rid {
				if r.Error != nil {
					t.Fatalf("%s: %v", method, r.Error)
				}
				return r.Result
			}
		}
	}
	request(1, "initialize", `{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"raw-test","version":"1"}}`)
	if err := conn.Write(ctx, &jsonrpc.Request{Method: "notifications/initialized", Params: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	raw := request(2, "tools/list", `{}`)
	var res struct {
		Tools []rawTool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("tools/list result: %v\n%s", err, raw)
	}
	if len(res.Tools) == 0 {
		t.Fatalf("tools/list listed no tools: %s", raw)
	}
	return res.Tools, raw
}

// name is the tool's name.
func (r rawTool) name() string {
	var n string
	json.Unmarshal(r["name"], &n)
	return n
}

// hostedServer is a Server configured as the hosted playmat is: every call
// resolves its game from the request (here, always one fresh manager), and
// get_account answers with an account ID.
func hostedServer(t *testing.T) *Server {
	m := game.NewManager(t.TempDir(), nil)
	return &Server{Version: "test", BoardURL: "https://playmat.example/board", Resolve: func(context.Context, *mcp.RequestExtra) (*game.Manager, string, error) {
		return m, "https://playmat.example/board", nil
	}, Account: fixedAccount("sub-1")}
}

// TestToolLabelsExplicitOnTheWire: in the tool list a client receives, on
// the desktop and hosted servers over both transports, every tool states
// readOnlyHint, destructiveHint and openWorldHint as JSON booleans: reads
// are read-only and not destructive, only the three game-replacing tools
// are destructive, writes say readOnlyHint false, and none is open-world.
func TestToolLabelsExplicitOnTheWire(t *testing.T) {
	servers := map[string]func(t *testing.T) *Server{
		"desktop": func(*testing.T) *Server { return &Server{Version: "test"} },
		"hosted":  hostedServer,
	}
	for name, mk := range servers {
		for _, tr := range transports {
			t.Run(name+"/"+tr.name, func(t *testing.T) {
				for _, tool := range rawToolList(t, tr, mk(t)) {
					n := tool.name()
					var labels map[string]any
					if err := json.Unmarshal(tool["annotations"], &labels); err != nil {
						t.Errorf("%s: annotations %s: %v", n, tool["annotations"], err)
						continue
					}
					want := map[string]bool{
						"readOnlyHint":    readOnlyTools[n],
						"destructiveHint": n == "new_game" || n == "import_log" || n == "set_opponent_deck",
						"openWorldHint":   false,
					}
					for key, w := range want {
						got, ok := labels[key].(bool)
						if !ok {
							t.Errorf("%s: %s is %v, want an explicit %v", n, key, labels[key], w)
						} else if got != w {
							t.Errorf("%s: %s %v, want %v", n, key, got, w)
						}
					}
				}
			})
		}
	}
}

// TestToolSecuritySchemes: in hosted mode every tool declares in its _meta
// that it needs the player's OAuth sign-in, as OpenAI asks; on the desktop
// no tool declares any.
func TestToolSecuritySchemes(t *testing.T) {
	for _, tr := range transports {
		t.Run("hosted/"+tr.name, func(t *testing.T) {
			for _, tool := range rawToolList(t, tr, hostedServer(t)) {
				var meta struct {
					Schemes []map[string]any `json:"securitySchemes"`
				}
				if err := json.Unmarshal(tool["_meta"], &meta); err != nil {
					t.Errorf("%s: _meta %s: %v", tool.name(), tool["_meta"], err)
					continue
				}
				if want := []map[string]any{{"type": "oauth2"}}; !reflect.DeepEqual(meta.Schemes, want) {
					t.Errorf("%s: securitySchemes %v, want %v", tool.name(), meta.Schemes, want)
				}
			}
		})
		t.Run("desktop/"+tr.name, func(t *testing.T) {
			for _, tool := range rawToolList(t, tr, &Server{Version: "test"}) {
				if m, ok := tool["_meta"]; ok {
					t.Errorf("%s: desktop tool has _meta %s", tool.name(), m)
				}
			}
		})
	}
}

// desktopToolNames are the 45 tools the desktop offered before the
// ChatGPT submission work (2026-11-12). Claude's connections rely on them,
// so labelling and describing the tools must not add, drop or rename one.
var desktopToolNames = []string{
	"add_effect", "annotate", "attach", "attack", "bench", "card_text",
	"checkout", "coin_flip", "draw", "end_action", "end_turn", "evolve",
	"get_log", "get_profile", "get_record", "get_state", "get_warnings",
	"guide", "import_log", "knock_out", "list_branches", "look_top", "move",
	"mulligan", "new_game", "ping", "place_active", "place_damage",
	"play_trainer", "remove_effect", "retreat", "reveal", "rewind",
	"sample_hidden", "search_deck", "set_condition", "set_mode",
	"set_opponent_deck", "shuffle", "start_game", "switch", "take_prize",
	"undo", "use_ability", "wait_for_turn",
}

// TestDesktopToolNamesUnchanged: the desktop lists exactly the tools it
// did before, over both transports, the guard against breaking Claude.
func TestDesktopToolNamesUnchanged(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, _ *game.Manager) {
		var got []string
		for tool, err := range cs.Tools(context.Background(), nil) {
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, tool.Name)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, desktopToolNames) {
			t.Errorf("desktop tools changed:\n got %v\nwant %v", got, desktopToolNames)
		}
	})
}
