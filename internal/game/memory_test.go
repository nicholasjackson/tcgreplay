package game

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// seededDataDir returns a data directory holding what an older playmat
// left behind: a game file, its import request, the current-game pointer
// and the last deck (the corpus deck, so reusing it would work).
func seededDataDir(t *testing.T, deckText string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"games/old.jsonl":       `{"game":"old","created":"2026-10-01T00:00:00Z","source":"import"}` + "\n",
		"games/old.import.json": `{"id":"old","log":"","deck":"","seed":1}` + "\n",
		"current.txt":           "old",
		"last-deck.txt":         deckText,
	}
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// dirFiles maps every file under dir, by path relative to it, to its
// content, leaving out the assistant label and the process log, which are
// not game data.
func dirFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel == "assistant.txt" || rel == "tcgreplay.log" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = b
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestNothingWritten drives a game through import, verbs, a branch, a note,
// a rebuild with an opponent list and a new game, and checks the data
// directory is byte for byte as it was: no file added, changed or removed.
func TestNothingWritten(t *testing.T) {
	deckText := corpusDeck(t)
	logText := readText(t, donkLogPath)
	ctx := context.Background()
	dir := seededDataDir(t, deckText)
	before := dirFiles(t, dir)
	if len(before) != 4 {
		t.Fatalf("seeded data dir holds %d files, want 4", len(before))
	}

	m := NewManager(dir, nil)
	res, err := m.Import(ctx, ImportRequest{ID: "g", Log: logText, Deck: deckText, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Game
	if g.Mode() != ModeReview {
		t.Fatalf("mode %s, want review", g.Mode())
	}
	cur := g.State().Current
	if _, err := g.Draw(cur, state.ActorHuman, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := g.View("", "test", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Log(0, state.P1, "test", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.JumpTo(20, "try"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Draw(state.P1, state.ActorClaude, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Shuffle(state.P1, state.ActorClaude, state.P1); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Undo(1); err != nil {
		t.Fatal(err)
	}
	if err := g.Annotate(20, "test", "a note"); err != nil {
		t.Fatal(err)
	}
	if err := g.Checkout("main"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Record(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetOpponentDeck(ctx, assumedOppDeck, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.New(ctx, NewGameRequest{ID: "play", DeckP1: deckText, DeckP2: deckText}); err != nil {
		t.Fatal(err)
	}

	after := dirFiles(t, dir)
	for rel, b := range after {
		was, ok := before[rel]
		switch {
		case !ok:
			t.Errorf("%s was written", rel)
		case !bytes.Equal(was, b):
			t.Errorf("%s was changed", rel)
		}
	}
	for rel := range before {
		if _, ok := after[rel]; !ok {
			t.Errorf("%s was removed", rel)
		}
	}
}

// TestRestartForgets: a new manager on a data directory that holds older
// game files and a current-game pointer, and one a manager has just used,
// starts with no game open and no record.
func TestRestartForgets(t *testing.T) {
	deckText := corpusDeck(t)
	logText := readText(t, donkLogPath)
	dir := seededDataDir(t, deckText)
	check := func(when string) {
		t.Helper()
		m := NewManager(dir, nil)
		if g, err := m.Current(); err == nil || !strings.Contains(err.Error(), "no game open") {
			t.Errorf("%s: current game %v, err %v; want no game open", when, g, err)
		}
		if _, err := m.Record(); err == nil || !strings.Contains(err.Error(), "no game open") {
			t.Errorf("%s: record err %v, want no game open", when, err)
		}
	}
	check("old files")

	if _, err := NewManager(dir, nil).Import(context.Background(), ImportRequest{ID: "g", Log: logText, Deck: deckText, Seed: 1}); err != nil {
		t.Fatal(err)
	}
	check("after an import")
}

// TestImportReplaces: each import replaces the game open, and new_game
// replaces an import, leaving a game with no record.
func TestImportReplaces(t *testing.T) {
	deckText := corpusDeck(t)
	logText := readText(t, donkLogPath)
	ctx := context.Background()
	m := NewManager(t.TempDir(), nil)
	first, err := m.Import(ctx, ImportRequest{ID: "first", Log: logText, Deck: deckText, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Import(ctx, ImportRequest{ID: "second", Log: logText, Deck: deckText, Seed: 2})
	if err != nil {
		t.Fatal(err)
	}
	cur, err := m.Current()
	if err != nil {
		t.Fatal(err)
	}
	if cur.ID != "second" || cur != second.Game || cur == first.Game {
		t.Errorf("current game %s, want second", cur.ID)
	}
	if rec, err := m.Record(); err != nil || rec != second.Record {
		t.Errorf("record is not the second import's (%v)", err)
	}

	g, _, err := m.New(ctx, NewGameRequest{ID: "play", DeckP1: deckText, DeckP2: deckText})
	if err != nil {
		t.Fatal(err)
	}
	if cur, err := m.Current(); err != nil || cur != g || cur.ID != "play" {
		t.Errorf("current game after new_game: %v (%v), want play", cur, err)
	}
	if _, err := m.Record(); err == nil || !strings.Contains(err.Error(), "no record") {
		t.Errorf("record after new_game: err %v, want no record", err)
	}
}

// TestLastDeckInMemory: an import without a deck reuses the last one this
// manager imported, but a new manager does not pick one up from the data
// directory, even when an old last-deck.txt is there.
func TestLastDeckInMemory(t *testing.T) {
	deckText := corpusDeck(t)
	logText := readText(t, donkLogPath)
	ctx := context.Background()
	dir := seededDataDir(t, deckText)

	m := NewManager(dir, nil)
	if _, err := m.Import(ctx, ImportRequest{ID: "a", Log: logText, Deck: deckText, Seed: 1}); err != nil {
		t.Fatal(err)
	}
	res, err := m.Import(ctx, ImportRequest{ID: "b", Log: logText, Seed: 1})
	if err != nil {
		t.Fatalf("import reusing the last deck: %v", err)
	}
	if res.Record.Deck != deckText {
		t.Error("import without a deck did not reuse the last one")
	}

	_, err = NewManager(dir, nil).Import(ctx, ImportRequest{ID: "c", Log: logText, Seed: 1})
	if err == nil || !strings.Contains(err.Error(), "no decklist given") {
		t.Errorf("new manager import without a deck: err %v, want no decklist given", err)
	}
}
