package game_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cardset"
	"github.com/nicholasjackson/tcgreplay/internal/catalog"
	"github.com/nicholasjackson/tcgreplay/internal/game"
)

// committedCatalog publishes the repository's committed card sets to a
// directory and returns a catalogue loaded from it, or skips when there
// are none.
func committedCatalog(t *testing.T) *catalog.Loader {
	t.Helper()
	const dir = "../../cardsets"
	if _, err := os.Stat(filepath.Join(dir, "sets")); err != nil {
		t.Skip("no committed card sets")
	}
	pub := t.TempDir()
	if _, err := cardset.Publish(context.Background(), cardset.PublishOptions{
		Dir: dir, Prefix: "", Target: cardset.DirTarget{Dir: pub}, Logf: func(string, ...any) {},
	}); err != nil {
		t.Fatalf("publishing the committed card sets: %v", err)
	}
	l := &catalog.Loader{Base: pub, CacheDir: t.TempDir(), Logf: t.Logf}
	l.Start(context.Background(), time.Minute)
	if st := l.Status(); st.Sets == 0 || st.LastError != "" {
		t.Fatalf("committed card sets did not load: %+v", st)
	}
	return l
}

// wantDefined checks the card id resolves through db to a named card, and
// that a Trainer or special Energy has its effect text.
func wantDefined(t *testing.T, db cards.DB, id, what string) {
	t.Helper()
	c, err := db.Card(context.Background(), id)
	if err != nil {
		t.Errorf("%s: %s does not resolve: %v", what, id, err)
		return
	}
	if c.Name == "" {
		t.Errorf("%s: %s has no name", what, id)
	}
	if (c.Category == "Trainer" || (c.Category == "Energy" && c.EnergyType == "Special")) && c.Effect == "" {
		t.Errorf("%s: %s (%s, %s) has no effect text", what, id, c.Name, c.Category)
	}
}

// TestCorpusResolvesFromCommittedCardSets: every corpus battle log, with
// the corpus deck, imports against the committed card sets as published,
// and every card it names resolves to a named definition, with effect text
// for every Trainer and special Energy; so does every entry of the deck.
func TestCorpusResolvesFromCommittedCardSets(t *testing.T) {
	loader := committedCatalog(t)
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	logs, _ := filepath.Glob("../../testdata/logs/*.txt")
	if len(logs) == 0 {
		t.Skip("no test corpus")
	}

	t.Run("deck", func(t *testing.T) {
		m := game.NewManager(t.TempDir(), loader)
		entries, warnings, err := m.ParseDeck(context.Background(), string(deckText), "deck")
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range warnings {
			t.Errorf("warning: %s", w)
		}
		if len(entries) == 0 {
			t.Fatal("deck has no entries")
		}
		db := loader.Snapshot()
		for _, e := range entries {
			what := e.Name + " " + e.Set + " " + e.Number
			if e.Def == "" {
				t.Errorf("%s did not resolve", what)
				continue
			}
			wantDefined(t, db, e.Def, what)
		}
	})

	for _, path := range logs {
		t.Run(filepath.Base(path), func(t *testing.T) {
			text, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			m := game.NewManager(t.TempDir(), loader)
			res, err := m.Import(context.Background(), game.ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1})
			if err != nil {
				t.Fatal(err)
			}
			g := res.Game
			resolved := 0
			for iid, c := range g.State().Cards {
				if c.Def == "" {
					continue
				}
				wantDefined(t, g.DB(), c.Def, string(iid)+" ("+c.Name+")")
				resolved++
			}
			if resolved < 60 {
				t.Errorf("only %d cards resolved; the owner's 60-card deck alone should", resolved)
			}
		})
	}
}
