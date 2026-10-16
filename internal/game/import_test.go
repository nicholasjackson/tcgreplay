package game

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestImportCorpus writes every corpus log to a game file and checks it
// reloads to the same state with no invariant violations.
func TestImportCorpus(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	logs, _ := filepath.Glob("../../testdata/logs/*.txt")
	m := NewManager(t.TempDir(), nil)
	for _, path := range logs {
		t.Run(filepath.Base(path), func(t *testing.T) {
			text, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			res, err := m.Import(context.Background(), ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1, ID: "g-" + filepath.Base(path[:len(path)-4])})
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range res.Report.Problems {
				t.Errorf("line %d %q: %s", p.Line, p.Text, p.Err)
			}
			for _, w := range res.Game.State().Check() {
				t.Errorf("invariant: %s", w)
			}
			if res.Game.Mode() != ModeReview {
				t.Errorf("mode %s, want review", res.Game.Mode())
			}
			want := snapshot(t, res.Game)
			g, err := m.Load(res.Game.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got := snapshot(t, g); got != want {
				t.Error("reloaded state differs from imported state")
			}
		})
	}
}
