package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
)

// TestToolAnnotations: every tool carries behaviour hints. The reads are
// read-only (so clients need not ask before them), the tools that replace
// the game are destructive, every other write is not, and none is
// open-world. Every name in the read-only and replacing sets is a real
// tool, so a rename cannot silently drop a hint.
func TestToolAnnotations(t *testing.T) {
	forEachTransport(t, func(t *testing.T, cs *mcp.ClientSession, _ *game.Manager) {
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
	})
}
