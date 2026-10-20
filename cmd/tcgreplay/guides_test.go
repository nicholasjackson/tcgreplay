package main

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// desktopSession serves the desktop MCP server over in-memory transports,
// as the stdio caller does, and returns a client session on it.
func desktopSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
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
	return cs
}

// guideNames reads the guide tool's list and returns the names in it.
func guideNames(t *testing.T, list string) []string {
	t.Helper()
	var entries []struct{ Name, Description string }
	if err := json.Unmarshal([]byte(list), &entries); err != nil {
		t.Fatalf("guide list: %v: %s", err, list)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	return names
}

// servedGuides gathers everything cs is served about guides, keyed by what
// was asked: the guide list, each listed guide's text, and each prompt's
// description and text.
func servedGuides(t *testing.T, cs *mcp.ClientSession) map[string]string {
	t.Helper()
	ctx := context.Background()
	got := map[string]string{"guide list": callTool(t, cs, "guide", nil)}
	for _, name := range guideNames(t, got["guide list"]) {
		got["guide "+name] = callTool(t, cs, "guide", map[string]any{"name": name})
	}
	for p, err := range cs.Prompts(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		got["prompt "+p.Name+" description"] = p.Description
		res, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: p.Name})
		if err != nil {
			t.Fatalf("prompt %s: %v", p.Name, err)
		}
		var b strings.Builder
		for _, m := range res.Messages {
			if tc, ok := m.Content.(*mcp.TextContent); ok {
				b.WriteString(tc.Text)
			}
		}
		got["prompt "+p.Name+" text"] = b.String()
	}
	return got
}

// TestHostedGuidesMatchDesktop: listing the guides, reading each one, and
// listing and choosing each prompt give the same results on the hosted
// playmat as on the desktop; the hosted instructions are the desktop ones
// (guide block included) plus only the board-link line.
func TestHostedGuidesMatchDesktop(t *testing.T) {
	hosted := startHosted(t, testPublic).connect(t)
	desktop := desktopSession(t)

	want := servedGuides(t, desktop)
	names := guideNames(t, want["guide list"])
	if len(names) != 6 {
		t.Fatalf("desktop lists %d guides %v, want 6", len(names), names)
	}
	for _, name := range names {
		if want["guide "+name] == "" || want["prompt "+name+" text"] == "" {
			t.Errorf("desktop guide or prompt %s is empty or missing", name)
		}
	}
	got := servedGuides(t, hosted)
	keys := map[string]bool{}
	for k := range want {
		keys[k] = true
	}
	for k := range got {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		w, inDesktop := want[k]
		g, inHosted := got[k]
		switch {
		case !inHosted:
			t.Errorf("hosted lacks %s", k)
		case !inDesktop:
			t.Errorf("hosted has %s, the desktop does not", k)
		case g != w:
			t.Errorf("%s differs:\nhosted:  %q\ndesktop: %q", k, g, w)
		}
	}

	dIns, hIns := desktop.InitializeResult().Instructions, hosted.InitializeResult().Instructions
	for _, name := range names {
		if !strings.Contains(dIns, name) {
			t.Errorf("desktop instructions lack guide %s: %q", name, dIns)
		}
	}
	if !strings.HasPrefix(hIns, dIns) {
		t.Fatalf("hosted instructions do not start with the desktop ones:\nhosted:  %q\ndesktop: %q", hIns, dIns)
	}
	extra := hIns[len(dIns):]
	if !strings.Contains(extra, "board link") || !strings.HasPrefix(extra, "\n- ") || strings.Count(extra, "\n") != 1 {
		t.Errorf("hosted instructions add %q, want the board-link line", extra)
	}
	for _, name := range names {
		if strings.Contains(extra, name) {
			t.Errorf("hosted-only instructions %q name guide %s", extra, name)
		}
	}
}

// checkGuidesWhileBusy fails unless who, whose game calls are refused as
// busy, can still list the guides and read review-game.
func checkGuidesWhileBusy(t *testing.T, who string, cs *mcp.ClientSession) {
	t.Helper()
	if out, isErr := toolResult(t, cs, "guide", nil); isErr || !strings.Contains(out, "review-game") {
		t.Errorf("%s's guide list while busy: %q (error %v), want the list", who, out, isErr)
	}
	if out, isErr := toolResult(t, cs, "guide", map[string]any{"name": "review-game"}); isErr || out == "" {
		t.Errorf("%s's review-game guide while busy: %q (error %v), want its text", who, out, isErr)
	}
}
