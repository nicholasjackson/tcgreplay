package mcpserver

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const movedMCPURL = "https://tcgreplay.app/mcp"

// connectMoved connects a client through tr to a moved server: a playmat
// server with no game manager or resolver and the Moved middleware.
func connectMoved(t *testing.T, tr transport) *mcp.ClientSession {
	t.Helper()
	srv := (&Server{Version: "test", BoardURL: "https://x/board"}).MCP()
	srv.AddReceivingMiddleware(Moved(movedMCPURL))
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "moved-test"}, nil).Connect(context.Background(), tr.dial(t, srv), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func toolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func promptNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range res.Prompts {
		names = append(names, p.Name)
	}
	return names
}

func TestMovedTextNamesTheNewAddress(t *testing.T) {
	got := MovedText(movedMCPURL)
	want := "The tcgreplay playmat has moved to a new address. This connection no longer works: remove it, then add the playmat again at https://tcgreplay.app/mcp and sign in."
	if got != want {
		t.Fatalf("MovedText = %q, want %q", got, want)
	}
	for _, brand := range []string{"Claude", "ChatGPT"} {
		if strings.Contains(got, brand) {
			t.Errorf("MovedText names %s; it is shown in every assistant", brand)
		}
	}
}

// TestMovedServer: with Moved in front, a server with no game manager
// lists the same tools and guide prompts as the real server, and every
// tool call, prompt and the instructions give the moved message. Nothing
// panics, so no tool handler (which would need the manager) ran.
func TestMovedServer(t *testing.T) {
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			real, _ := connect(t, tr)
			cs := connectMoved(t, tr)
			want := MovedText(movedMCPURL)

			if got := cs.InitializeResult().Instructions; got != want {
				t.Errorf("instructions = %q, want the moved message", got)
			}

			tools := toolNames(t, cs)
			if len(tools) == 0 {
				t.Fatal("moved server lists no tools")
			}
			if realTools := toolNames(t, real); !reflect.DeepEqual(tools, realTools) {
				t.Errorf("moved tools %v, want the real server's %v", tools, realTools)
			}
			prompts := promptNames(t, cs)
			if len(prompts) == 0 {
				t.Fatal("moved server lists no prompts")
			}
			if realPrompts := promptNames(t, real); !reflect.DeepEqual(prompts, realPrompts) {
				t.Errorf("moved prompts %v, want the real server's %v", prompts, realPrompts)
			}

			for _, name := range tools {
				res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if !res.IsError {
					t.Errorf("%s: not an error result", name)
				}
				if len(res.Content) != 1 {
					t.Fatalf("%s: %d content items, want 1", name, len(res.Content))
				}
				if tc, ok := res.Content[0].(*mcp.TextContent); !ok || tc.Text != want {
					t.Errorf("%s: content %#v, want the moved message", name, res.Content[0])
				}
			}

			for _, name := range prompts {
				res, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: name})
				if err != nil {
					t.Fatalf("prompt %s: %v", name, err)
				}
				if len(res.Messages) != 1 {
					t.Fatalf("prompt %s: %d messages, want 1", name, len(res.Messages))
				}
				if tc, ok := res.Messages[0].Content.(*mcp.TextContent); !ok || tc.Text != want {
					t.Errorf("prompt %s: content %#v, want the moved message", name, res.Messages[0].Content)
				}
			}
		})
	}
}
