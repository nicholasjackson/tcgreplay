package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

// accountID calls get_account and returns the id in its structured
// content, checking the text copy carries the same JSON.
func accountID(t *testing.T, cs *mcp.ClientSession) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_account"})
	if err != nil {
		t.Fatalf("get_account: %v", err)
	}
	if res.IsError {
		t.Fatalf("get_account refused: %+v", res.Content)
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("structured content %s: %v", b, err)
	}
	if len(out) != 1 {
		t.Errorf("structured content %s, want only id", b)
	}
	id, _ := out["id"].(string)
	if len(res.Content) != 1 {
		t.Fatalf("get_account content: %+v", res.Content)
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); !ok || strings.TrimSpace(tc.Text) != string(b) {
		t.Errorf("get_account text copy %+v, want %s", res.Content[0], b)
	}
	return id
}

// TestHostedGetAccount: a connector signed in through Google lists
// get_account as OpenAI's profile tool (the profile marker, read-only, no
// input, an output schema of a required id only), and calling it returns
// the account's Google ID: the same on a second connection for the same
// account, and a different one for another account.
func TestHostedGetAccount(t *testing.T) {
	h := startHosted(t, testPublic)
	cs := h.connect(t)

	var tool *mcp.Tool
	for tl, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		if tl.Name == "get_account" {
			tool = tl
		}
	}
	if tool == nil {
		t.Fatal("hosted playmat does not list get_account")
	}
	if tool.Meta["openai/profile"] != true {
		t.Errorf("get_account _meta %v, want openai/profile true", tool.Meta)
	}
	if a := tool.Annotations; a == nil || !a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint || a.OpenWorldHint == nil || *a.OpenWorldHint {
		t.Errorf("get_account annotations %+v, want read-only, not destructive, closed-world", a)
	}
	in, _ := json.Marshal(tool.InputSchema)
	var inSchema struct {
		Properties map[string]any `json:"properties"`
		Additional *bool          `json:"additionalProperties"`
	}
	json.Unmarshal(in, &inSchema)
	if len(inSchema.Properties) != 0 || inSchema.Additional == nil || *inSchema.Additional {
		t.Errorf("get_account input schema %s, want an empty object with no extra properties", in)
	}
	outB, _ := json.Marshal(tool.OutputSchema)
	var outSchema struct {
		Properties map[string]map[string]any `json:"properties"`
		Required   []string                  `json:"required"`
		Additional *bool                     `json:"additionalProperties"`
	}
	json.Unmarshal(outB, &outSchema)
	if len(outSchema.Properties) != 1 || outSchema.Properties["id"]["type"] != "string" || len(outSchema.Required) != 1 || outSchema.Required[0] != "id" || outSchema.Additional == nil || *outSchema.Additional {
		t.Errorf("get_account output schema %s, want only a required string id", outB)
	}

	if got := accountID(t, cs); got != hostauthtest.DefaultAccount.ID {
		t.Errorf("get_account id %q, want the Google account ID %q", got, hostauthtest.DefaultAccount.ID)
	}
	if got := accountID(t, h.connect(t)); got != hostauthtest.DefaultAccount.ID {
		t.Errorf("second connection's id %q, want the same %q", got, hostauthtest.DefaultAccount.ID)
	}
	h.g.SetAccount(userB)
	if got := accountID(t, h.connect(t)); got != userB.ID {
		t.Errorf("another account's id %q, want %q", got, userB.ID)
	}
}

// TestHostedGetAccountAtFullCap: at a cap of 1, an account whose place
// went to another account while it sat idle is refused a game tool as
// busy, yet get_account still answers it, and asking takes no place.
func TestHostedGetAccountAtFullCap(t *testing.T) {
	h, clock := startPlacesHosted(t, 1, 2*time.Minute)
	h.g.SetAccount(userB)
	csB := h.connect(t)
	clock.Advance(2*time.Minute + time.Second)
	h.g.SetAccount(userA)
	h.connect(t)

	busy := func() bool {
		res, err := csB.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_state", Arguments: map[string]any{}})
		if err != nil {
			t.Fatal(err)
		}
		var text strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				text.WriteString(tc.Text)
			}
		}
		return res.IsError && strings.Contains(text.String(), "busy")
	}
	if !busy() {
		t.Fatal("B's get_state at the full cap was not refused as busy")
	}
	if got := accountID(t, csB); got != userB.ID {
		t.Errorf("B's get_account at the full cap: %q, want %q", got, userB.ID)
	}
	if !busy() {
		t.Error("B's get_state after get_account was not refused: asking took a place")
	}
}
