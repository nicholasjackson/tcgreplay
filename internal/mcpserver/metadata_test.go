package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
)

// toolGaps lists what a tool, as it came over the wire, is missing from
// the metadata every tool must carry: a title, a description saying when
// to use it ("Use this when") and when not to ("Do not use"), a
// description for every input, and readOnlyHint, destructiveHint and
// openWorldHint stated as JSON booleans. Each gap names the tool and what
// is missing; a complete tool has none.
func toolGaps(tool rawTool) []string {
	n := tool.name()
	var gaps []string
	gap := func(format string, args ...any) {
		gaps = append(gaps, n+": "+fmt.Sprintf(format, args...))
	}

	var title string
	if err := json.Unmarshal(tool["title"], &title); err != nil || strings.TrimSpace(title) == "" {
		gap("no title")
	}

	var desc string
	json.Unmarshal(tool["description"], &desc)
	if !strings.Contains(desc, "Use this when") {
		gap(`description does not say when to use it ("Use this when")`)
	}
	if !strings.Contains(desc, "Do not use") {
		gap(`description does not say when not to use it ("Do not use")`)
	}

	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool["inputSchema"], &schema); err != nil {
		gap("inputSchema %s: %v", tool["inputSchema"], err)
	}
	for prop, p := range schema.Properties {
		if strings.TrimSpace(p.Description) == "" {
			gap("input %q has no description", prop)
		}
	}

	var labels map[string]any
	json.Unmarshal(tool["annotations"], &labels)
	for _, key := range []string{"readOnlyHint", "destructiveHint", "openWorldHint"} {
		if _, ok := labels[key].(bool); !ok {
			gap("label %s is not an explicit boolean (got %v)", key, labels[key])
		}
	}
	return gaps
}

// TestToolMetadataComplete: on the desktop and hosted servers, over both
// transports, every tool in the list a client receives has a title, a
// description saying when to use it and when not to, a description for
// every input, and all three behaviour labels as explicit booleans.
func TestToolMetadataComplete(t *testing.T) {
	servers := map[string]func(t *testing.T) *Server{
		"desktop": func(*testing.T) *Server { return &Server{Version: "test"} },
		"hosted":  hostedServer,
	}
	for name, mk := range servers {
		for _, tr := range transports {
			t.Run(name+"/"+tr.name, func(t *testing.T) {
				for _, tool := range rawToolList(t, tr, mk(t)) {
					for _, g := range toolGaps(tool) {
						t.Error(g)
					}
				}
			})
		}
	}
}

// TestToolGapsNamesWhatIsMissing: a tool added without a title, either
// sentence of its description, an input description or a label is
// reported, each gap naming the tool and what is missing, and a complete
// tool reports nothing. This is what makes TestToolMetadataComplete fail
// on a tool added later without them.
func TestToolGapsNamesWhatIsMissing(t *testing.T) {
	complete := func() rawTool {
		return rawTool{
			"name":        json.RawMessage(`"new_tool"`),
			"title":       json.RawMessage(`"New tool"`),
			"description": json.RawMessage(`"Does a thing. Use this when you need the thing. Do not use it for other things."`),
			"inputSchema": json.RawMessage(`{"type":"object","properties":{"iid":{"type":"string","description":"The card."}}}`),
			"annotations": json.RawMessage(`{"readOnlyHint":false,"destructiveHint":false,"openWorldHint":false}`),
		}
	}
	if gaps := toolGaps(complete()); len(gaps) != 0 {
		t.Fatalf("complete tool reported gaps: %v", gaps)
	}
	cases := []struct {
		name   string
		change func(rawTool)
		want   []string
	}{
		{"no title", func(r rawTool) { delete(r, "title") }, []string{"new_tool", "title"}},
		{"empty title", func(r rawTool) { r["title"] = json.RawMessage(`""`) }, []string{"new_tool", "title"}},
		{"no when to use", func(r rawTool) {
			r["description"] = json.RawMessage(`"Does a thing. Do not use it for other things."`)
		}, []string{"new_tool", "Use this when"}},
		{"no when not to use", func(r rawTool) {
			r["description"] = json.RawMessage(`"Does a thing. Use this when you need the thing."`)
		}, []string{"new_tool", "Do not use"}},
		{"undescribed input", func(r rawTool) {
			r["inputSchema"] = json.RawMessage(`{"type":"object","properties":{"iid":{"type":"string","description":"The card."},"seat":{"type":"string"}}}`)
		}, []string{"new_tool", `input "seat"`}},
		{"missing label", func(r rawTool) {
			r["annotations"] = json.RawMessage(`{"readOnlyHint":true,"openWorldHint":false}`)
		}, []string{"new_tool", "destructiveHint"}},
		{"no annotations", func(r rawTool) { delete(r, "annotations") }, []string{"new_tool", "readOnlyHint"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool := complete()
			c.change(tool)
			gaps := toolGaps(tool)
			if len(gaps) == 0 {
				t.Fatalf("no gap reported, want one naming %v", c.want)
			}
			found := false
			for _, g := range gaps {
				ok := true
				for _, w := range c.want {
					if !strings.Contains(g, w) {
						ok = false
					}
				}
				if ok {
					found = true
				}
			}
			if !found {
				t.Errorf("gaps %q: none names all of %q", gaps, c.want)
			}
		})
	}
}

// toolListCeiling caps the size in bytes of the hosted tools/list result.
// Measured on 2026-11-13 after the description pass: 45 tools, 37,217
// bytes hosted (35,057 desktop).
// Before that pass the desktop list was 45 tools, 26,149 bytes
// (2026-11-12). The ceiling is the measurement plus about 10%; it exists to
// stop the list growing silently, since every byte is sent to the model on
// every connection (OpenAI recommends keeping tool definitions small).
// Raise it deliberately, with a new measurement, if a change needs to.
const toolListCeiling = 41000

// TestToolListSize: the hosted tools/list result, as the server encodes
// it, stays under toolListCeiling bytes over both transports; the size and
// tool count are logged so a change in either is visible.
func TestToolListSize(t *testing.T) {
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			tools, raw := rawToolListBytes(t, tr, hostedServer(t))
			t.Logf("hosted tools/list: %d tools, %d bytes (ceiling %d)", len(tools), len(raw), toolListCeiling)
			dtools, draw := rawToolListBytes(t, tr, &Server{Version: "test"})
			t.Logf("desktop tools/list: %d tools, %d bytes", len(dtools), len(draw))
			if len(raw) > toolListCeiling {
				t.Errorf("hosted tools/list is %d bytes, over the %d-byte ceiling", len(raw), toolListCeiling)
			}
		})
	}
}

// TestToolAddedWithoutMetadataFails: a tool registered on the playmat's
// server without going through Server.tool (so with no title, no
// "Use this when" or "Do not use", no input description and no labels)
// still reaches the client without them through the label encoder, so
// the metadata guard names it and every missing piece, over both
// transports.
func TestToolAddedWithoutMetadataFails(t *testing.T) {
	type bareArg struct {
		Card string `json:"card"`
	}
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) {
			srv := (&Server{Version: "test", M: game.NewManager(t.TempDir(), nil)}).MCP()
			mcp.AddTool(srv, &mcp.Tool{Name: "bare_tool", Description: "Does a thing."}, func(context.Context, *mcp.CallToolRequest, bareArg) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{}, nil, nil
			})
			tools, _ := rawToolListFrom(t, tr, srv)
			var gaps []string
			for _, tool := range tools {
				if tool.name() == "bare_tool" {
					gaps = toolGaps(tool)
				} else if g := toolGaps(tool); len(g) != 0 {
					t.Errorf("registered tool has gaps: %v", g)
				}
			}
			for _, want := range []string{"no title", "Use this when", "Do not use", `input "card"`, "readOnlyHint", "destructiveHint", "openWorldHint"} {
				found := false
				for _, g := range gaps {
					if strings.HasPrefix(g, "bare_tool: ") && strings.Contains(g, want) {
						found = true
					}
				}
				if !found {
					t.Errorf("no gap naming bare_tool and %q; gaps: %v", want, gaps)
				}
			}
		})
	}
}
