package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/guides"
)

// GuideArg names the guide to read.
type GuideArg struct {
	Name string `json:"name,omitempty" jsonschema:"guide to read; omit to list the guides"`
}

// guideEntry is one line of the guide list.
type guideEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// registerGuides adds the guide tool and one prompt per guide. The tool is
// registered with mcp.AddTool
// rather than add: a guide is compiled-in text, so reading one needs no
// game, and in hosted mode must not take a place or be refused as busy (see
// bind).
func (s *Server) registerGuides(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{Name: "guide", Description: guideToolDescription(), Annotations: annotations("guide")}, func(_ context.Context, _ *mcp.CallToolRequest, in GuideArg) (*mcp.CallToolResult, any, error) {
		if in.Name == "" {
			var list []guideEntry
			for _, g := range guides.List() {
				list = append(list, guideEntry{Name: g.Name, Description: g.Description})
			}
			return out(list)
		}
		g, err := guides.Get(in.Name)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil, nil
		}
		return out(g.Text)
	})
	// Each guide is also a prompt, for clients that offer server prompts as
	// commands. Choosing one gives exactly the text the guide tool returns.
	for _, g := range guides.List() {
		srv.AddPrompt(&mcp.Prompt{Name: g.Name, Description: g.Description}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
			return &mcp.GetPromptResult{
				Description: g.Description,
				Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: g.Text}}},
			}, nil
		})
	}
}

// guideToolDescription says when to use each guide, built from the guide
// list. Some clients (ChatGPT) choose an app from its tool descriptions and
// pay little heed to the server instructions, so the description has to
// carry each guide's trigger itself.
func guideToolDescription() string {
	var b strings.Builder
	b.WriteString("Pokémon TCG coaching on the playmat. Read the matching guide (pass its name) before starting its workflow, even if the player does not mention TCG Replay; call with no name to list them.")
	for _, g := range guides.List() {
		fmt.Fprintf(&b, " %s: %s", g.Name, g.Description)
	}
	return b.String()
}

// guideInstructions is the instructions' guide block: one line per guide,
// built from the guide list so it cannot drift from it.
func guideInstructions() string {
	var b strings.Builder
	b.WriteString("\n- Coaching workflows have guides. Before starting one, read its guide with the guide tool (pass its name):")
	for _, g := range guides.List() {
		b.WriteString("\n  - " + g.Name + ": " + g.Description)
	}
	return b.String()
}
