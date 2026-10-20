package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/guides"
)

// The six guides, in the order players meet them.
var guideNames = []string{"import-game", "opponent-deck", "review-game", "replay-drill", "before-you-move", "numbers-drill"}

// TestGuideList: guide with no name lists the six guides in order, each with
// a one-line "use when".
func TestGuideList(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, _ *game.Manager) {
		out, isErr := call(t, cs, "guide", nil)
		if isErr {
			t.Fatal(out)
		}
		var list []map[string]string
		if err := json.Unmarshal([]byte(out), &list); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		var names []string
		for _, e := range list {
			names = append(names, e["name"])
			if len(e) != 2 {
				t.Errorf("entry %v has fields other than name and description", e)
			}
			if d := e["description"]; d == "" || strings.Contains(d, "\n") {
				t.Errorf("%s description is not one line: %q", e["name"], d)
			}
		}
		if !reflect.DeepEqual(names, guideNames) {
			t.Errorf("guides %v, want %v", names, guideNames)
		}
	})
}

// TestGuideRead: a known name returns the guide's full text; an unknown one
// is a tool error naming the valid guides.
func TestGuideRead(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, _ *game.Manager) {
		out, isErr := call(t, cs, "guide", map[string]any{"name": "review-game"})
		if isErr {
			t.Fatal(out)
		}
		g, err := guides.Get("review-game")
		if err != nil {
			t.Fatal(err)
		}
		if out != g.Text {
			t.Errorf("review-game text differs from the guide (%d bytes, want %d)", len(out), len(g.Text))
		}
		// How to find decision points, and how to record findings.
		for _, want := range []string{"decision points", "Finding ("} {
			if !strings.Contains(out, want) {
				t.Errorf("review-game does not contain %q", want)
			}
		}

		out, isErr = call(t, cs, "guide", map[string]any{"name": "tcg-review"})
		if !isErr {
			t.Fatalf("unknown guide succeeded: %s", out)
		}
		for _, n := range guideNames {
			if !strings.Contains(out, n) {
				t.Errorf("unknown-guide error does not name %s: %q", n, out)
			}
		}
	})
}

// checkGuideInstructions checks that ins lists every guide with its "use
// when" line and asks for the guide to be read first, in a block of one line
// per guide plus its heading.
func checkGuideInstructions(t *testing.T, ins string) {
	t.Helper()
	if !strings.Contains(ins, "Before starting one, read its guide with the guide tool") {
		t.Errorf("instructions do not ask for a guide to be read first: %q", ins)
	}
	for _, g := range guides.List() {
		if line := "\n  - " + g.Name + ": " + g.Description; !strings.Contains(ins, line) {
			t.Errorf("instructions do not list %s: want %q", g.Name, line)
		}
	}
	if n := strings.Count(guideInstructions(), "\n"); n > 7 {
		t.Errorf("guide block is %d lines, want at most 7", n)
	}
}

// TestGuideInstructions: on the desktop and hosted, the instructions a new
// connection receives list the guides.
func TestGuideInstructions(t *testing.T) {
	t.Run("desktop", func(t *testing.T) {
		forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, _ *game.Manager) {
			checkGuideInstructions(t, cs.InitializeResult().Instructions)
		})
	})
	t.Run("hosted", func(t *testing.T) {
		for _, tr := range transports {
			t.Run(tr.name, func(t *testing.T) {
				cs, _ := connectServer(t, tr, &Server{Version: "test", BoardURL: "https://playmat.example/"})
				checkGuideInstructions(t, cs.InitializeResult().Instructions)
			})
		}
	})
}

// snakeWord matches a tool or argument name as guides write them.
var snakeWord = regexp.MustCompile(`\b[a-z]+(_[a-z]+)+\b`)

// unknownPlaymatWords returns each snake_case word in text that is not in
// known, once, in the order they appear.
func unknownPlaymatWords(text string, known map[string]bool) []string {
	var bad []string
	seen := map[string]bool{}
	for _, w := range snakeWord.FindAllString(text, -1) {
		if !known[w] && !seen[w] {
			seen[w] = true
			bad = append(bad, w)
		}
	}
	return bad
}

// playmatWords returns every tool name the server offers and every argument
// any tool takes, plus the record marker import-game tells the assistant to
// look for in a pasted record.
func playmatWords(t *testing.T, cs *mcp.ClientSession) map[string]bool {
	t.Helper()
	known := map[string]bool{"tcgreplay_record": true}
	for tool, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		known[tool.Name] = true
		b, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(b, &schema); err != nil {
			t.Fatal(err)
		}
		props, _ := schema["properties"].(map[string]any)
		for p := range props {
			known[p] = true
		}
	}
	return known
}

// TestGuidesNameRealTools: every tool or argument a guide names exists on
// the playmat, so a renamed or removed one fails here.
func TestGuidesNameRealTools(t *testing.T) {
	cs, _ := connect(t, transports[0])
	known := playmatWords(t, cs)
	// The check itself: a removed tool is flagged, a real one is not.
	if got := unknownPlaymatWords("call list_games, then import_log", known); !reflect.DeepEqual(got, []string{"list_games"}) {
		t.Errorf("check flagged %v, want [list_games]", got)
	}
	for _, g := range guides.List() {
		for _, w := range unknownPlaymatWords(g.Text, known) {
			t.Errorf("guide %s names %s, which the playmat does not have", g.Name, w)
		}
	}
}

// TestGuideNeedsNoGame: guide skips bind, so it works when the resolver
// refuses every call, as hosted mode does when no place is free.
func TestGuideNeedsNoGame(t *testing.T) {
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			s := &Server{Version: "test", Resolve: func(context.Context, *mcp.RequestExtra) (*game.Manager, string, error) {
				return nil, "", errors.New("busy")
			}}
			cs, err := mcp.NewClient(&mcp.Implementation{Name: "codex-mcp-client"}, nil).Connect(context.Background(), tr.dial(t, s.MCP()), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { cs.Close() })

			if out, isErr := call(t, cs, "ping", nil); !isErr || !strings.Contains(out, "busy") {
				t.Errorf("ping with a refusing resolver: error %v: %q, want refused", isErr, out)
			}
			if out, isErr := call(t, cs, "guide", nil); isErr {
				t.Errorf("guide list refused: %s", out)
			}
			if out, isErr := call(t, cs, "guide", map[string]any{"name": "review-game"}); isErr {
				t.Errorf("guide review-game refused: %s", out)
			}
		})
	}
}

// TestGuidePrompts: each guide is also a server prompt, named after the
// guide, described by its "use when" line and taking no arguments.
func TestGuidePrompts(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, _ *game.Manager) {
		if cs.InitializeResult().Capabilities.Prompts == nil {
			t.Error("server does not advertise prompts")
		}
		res, err := cs.ListPrompts(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, p := range res.Prompts {
			names = append(names, p.Name)
			g, err := guides.Get(p.Name)
			if err != nil {
				t.Errorf("prompt %s is not a guide", p.Name)
				continue
			}
			if p.Description != g.Description {
				t.Errorf("prompt %s description %q, want %q", p.Name, p.Description, g.Description)
			}
			if len(p.Arguments) != 0 {
				t.Errorf("prompt %s takes arguments %v", p.Name, p.Arguments)
			}
		}
		want := append([]string(nil), guideNames...)
		sort.Strings(names)
		sort.Strings(want)
		if !reflect.DeepEqual(names, want) {
			t.Errorf("prompts %v, want %v", names, want)
		}
	})
}

// TestGuidePromptText: choosing a prompt gives one user message holding
// exactly the text the guide tool returns; an unknown name is an error.
func TestGuidePromptText(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, _ *game.Manager) {
		for _, n := range guideNames {
			res, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: n})
			if err != nil {
				t.Errorf("%s: %v", n, err)
				continue
			}
			if len(res.Messages) != 1 {
				t.Errorf("%s: %d messages, want 1", n, len(res.Messages))
				continue
			}
			m := res.Messages[0]
			if m.Role != "user" {
				t.Errorf("%s: role %q, want user", n, m.Role)
			}
			tc, ok := m.Content.(*mcp.TextContent)
			if !ok {
				t.Errorf("%s: content is %T, want text", n, m.Content)
				continue
			}
			text, isErr := call(t, cs, "guide", map[string]any{"name": n})
			if isErr {
				t.Fatal(text)
			}
			if tc.Text != text {
				t.Errorf("%s prompt differs from the guide tool (%d bytes, want %d)", n, len(tc.Text), len(text))
			}
		}

		if _, err := cs.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "tcg-review"}); err == nil {
			t.Error("unknown prompt succeeded")
		}
	})
}

// TestGuidePromptsAddNoTools: prompts are not tools, so a client that ignores
// prompts sees no tool named after a guide.
func TestGuidePromptsAddNoTools(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, _ *game.Manager) {
		for tool, err := range cs.Tools(context.Background(), nil) {
			if err != nil {
				t.Fatal(err)
			}
			if _, err := guides.Get(tool.Name); err == nil {
				t.Errorf("tool %s is named after a guide", tool.Name)
			}
		}
	})
}
