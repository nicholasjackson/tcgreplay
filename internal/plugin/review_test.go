package plugin

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/appspec"
)

// The example game's page, where the reviewer copies the decklist and the
// battle log from, as the built package and the demo script name it.
const examplePage = "https://tcgreplay.app/docs/example"

// TestReviewMaterial: the built package carries five positive cases that
// together walk the journey (the first points at the example game, and the
// tools triggered include importing, reading the board, rewinding and
// waiting), three negative cases each saying what should happen, and
// release notes that give the tool list size and tell connected players to
// refresh.
func TestReviewMaterial(t *testing.T) {
	b, err := Build(realSources(t))
	if err != nil {
		t.Fatal(err)
	}
	_, files := unzip(t, b)
	var p builtPlugin
	if err := json.Unmarshal(files["plugin.json"], &p); err != nil {
		t.Fatal(err)
	}
	tc := p.Extensions.OpenAI.Review.TestCases
	if len(tc.Positive) != 5 || len(tc.Negative) != 3 {
		t.Fatalf("%d positive and %d negative cases, want 5 and 3", len(tc.Positive), len(tc.Negative))
	}
	if first := tc.Positive[0]; !strings.Contains(first["description"], examplePage) || !strings.Contains(first["description"], "Google") {
		t.Errorf("first positive case %q does not point at %s and the Google sign-in", first["description"], examplePage)
	}
	var tools []string
	for i, c := range tc.Positive {
		if strings.Contains(strings.ToLower(c["description"]+c["prompt"]), "placeholder") {
			t.Errorf("positive case %d is still a placeholder", i+1)
		}
		for _, tool := range strings.Split(c["tools_triggered"], ",") {
			tools = append(tools, strings.TrimSpace(tool))
		}
	}
	for _, want := range []string{"import_log", "get_state", "rewind", "wait_for_turn", "get_record", "annotate"} {
		if !contains(tools, want) {
			t.Errorf("no positive case triggers %s (tools %v)", want, tools)
		}
	}
	journey := strings.ToLower(tc.Positive[1]["expected_behavior"] + tc.Positive[2]["expected_behavior"] + tc.Positive[3]["expected_behavior"])
	for _, step := range []string{"board link", "coaching notes", "branch"} {
		if !strings.Contains(journey, step) {
			t.Errorf("positive cases 2 to 4 do not cover %q", step)
		}
	}
	for i, c := range tc.Negative {
		if strings.Contains(strings.ToLower(c["description"]+c["prompt"]), "placeholder") {
			t.Errorf("negative case %d is still a placeholder", i+1)
		}
		if !strings.Contains(c["description"], ":") {
			t.Errorf("negative case %d %q does not say what should happen", i+1, c["description"])
		}
	}
	if !strings.Contains(strings.ToLower(tc.Negative[0]["description"]), "decklist") {
		t.Errorf("first negative case %q is not the missing decklist", tc.Negative[0]["description"])
	}

	notes := p.Extensions.OpenAI.Publication.ReleaseNotes
	for _, want := range []string{"1.0.0", "Refresh", "new chat", "bytes", "Google"} {
		if !strings.Contains(notes, want) {
			t.Errorf("release notes do not mention %q:\n%s", want, notes)
		}
	}
	if strings.Contains(strings.ToLower(notes), "placeholder") {
		t.Error("release notes are still the placeholder")
	}
}

// TestDemoScriptAndChecklist: the demo script names the example game's
// page at the App Platform address and walks the positive cases; the
// submission checklist exists and keeps reviewer credentials out of the
// repository.
func TestDemoScriptAndChecklist(t *testing.T) {
	spec, err := os.ReadFile("../../.do/app.yaml")
	if err != nil {
		t.Fatal(err)
	}
	public, err := appspec.PublicURL(string(spec))
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile("../../plugin/demo-script.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), public.String()+"/docs/example") {
		t.Errorf("demo script does not name %s/docs/example", public)
	}
	for i := 1; i <= 5; i++ {
		if !strings.Contains(string(script), "positive case "+string(rune('0'+i))) {
			t.Errorf("demo script does not walk positive case %d", i)
		}
	}
	if !strings.Contains(string(script), "two-step verification") {
		t.Error("demo script does not say the reviewer account needs no two-step verification")
	}
	checklist, err := os.ReadFile("../../plugin/SUBMITTING.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"make plugin", "TCGREPLAY_OPENAI_CHALLENGE", "TCGREPLAY_REVIEWER", "MCP Inspector", "backwards compatible", "demo_recording_url"} {
		if !strings.Contains(string(checklist), want) {
			t.Errorf("submission checklist does not mention %q", want)
		}
	}
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}
