package game

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/state"
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

// TestJumpTo reuses a branch already at the event and names new ones
// base, base-2, ... without moving the branch left behind.
func TestJumpTo(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	text, err := os.ReadFile("../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(context.Background(), ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1, ID: "g"})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Game
	end := g.Head()
	jump := func(ev int, base, want string) {
		t.Helper()
		name, err := g.JumpTo(ev, base)
		if err != nil {
			t.Fatal(err)
		}
		if name != want || g.Branch() != want || g.Head() != ev {
			t.Fatalf("JumpTo(%d, %s) = %s on %s at %d, want %s at %d", ev, base, name, g.Branch(), g.Head(), want, ev)
		}
	}
	jump(20, "turn-1", "turn-1")
	jump(20, "turn-1", "turn-1") // reused, not turn-1-2
	jump(25, "turn-1", "turn-1-2")
	jump(20, "turn-1", "turn-1")
	if err := g.Checkout("main"); err != nil || g.Head() != end {
		t.Fatalf("main moved: head %d, want %d (%v)", g.Head(), end, err)
	}
	if _, err := g.JumpTo(end+100, "x"); err == nil {
		t.Error("jumped to a missing event")
	}
}

// TestSetOpponentDeck rebuilds an import with an assumed opponent list: the
// owner's side is unchanged and every opponent card has an identity.
func TestSetOpponentDeck(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	text, err := os.ReadFile("../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(ctx, ImportRequest{Log: string(text), Deck: string(deckText), ID: "g"}) // random seed
	if err != nil {
		t.Fatal(err)
	}
	before := res.Game.State()
	opp := `Pokémon: 12
4 Abra
3 Kadabra
3 Alakazam
2 Duskull

Trainer: 4
4 Buddy-Buddy Poffin

Energy: 44
44 Basic Psychic Energy`

	res, err = m.SetOpponentDeck(ctx, "g", opp, "")
	if err != nil {
		t.Fatal(err)
	}
	after := res.Game.State()
	if len(res.Report.Problems) > 0 {
		t.Errorf("problems: %v", res.Report.Problems)
	}
	for iid, c := range after.Cards {
		b := before.Cards[iid]
		if c.Owner == state.P1 {
			if c.Name != b.Name {
				t.Errorf("%s was %s, now %s", iid, b.Name, c.Name)
			}
			continue
		}
		if c.Name == "" {
			t.Errorf("%s has no identity", iid)
		}
	}
	for _, seat := range state.Seats {
		bs, as := before.Sides[seat], after.Sides[seat]
		if !slices.Equal(bs.Hand, as.Hand) || !slices.Equal(bs.Prizes, as.Prizes) || !slices.Equal(bs.Deck, as.Deck) {
			t.Errorf("%s's cards moved in the rebuild", seat)
		}
	}

	// A game with branches is not replaced; a separate one can be built.
	if _, err := res.Game.JumpTo(10, "look"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetOpponentDeck(ctx, "g", opp, ""); err == nil {
		t.Error("replaced a game with branches")
	}
	if res, err := m.SetOpponentDeck(ctx, "g", opp, "g2"); err != nil || res.Game.ID != "g2" {
		t.Errorf("new_id: %v", err)
	}
}

// TestDeckList lists a whole deck with where each copy is: deck and prizes
// merge into hidden for the owner, and the reveal view places them.
func TestDeckList(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	text, err := os.ReadFile("../../testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), nil)
	res, err := m.Import(context.Background(), ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1, ID: "g"})
	if err != nil {
		t.Fatal(err)
	}
	g := res.Game
	sum := func(rows []DeckListRow, where string) (total, at int) {
		for _, r := range rows {
			total += r.Count
			at += r.Where[where]
		}
		return
	}
	seat, err := g.DeckList(state.P1, state.P1, "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	if total, prizes := sum(seat, "prizes"); total != 60 || prizes != 0 {
		t.Errorf("seat view: %d cards, %d shown as prizes; want 60 and 0", total, prizes)
	}
	reveal, err := g.DeckList(state.P1, "", "test", "test")
	if err != nil {
		t.Fatal(err)
	}
	st := g.State()
	if _, prizes := sum(reveal, "prizes"); prizes != st.Sides[state.P1].PrizesLeft() {
		t.Errorf("reveal view shows %d prizes, want %d", prizes, st.Sides[state.P1].PrizesLeft())
	}
	for _, r := range reveal {
		if r.Name == "Meowth ex" && r.Count != 1 {
			t.Errorf("Meowth ex count %d", r.Count)
		}
		if r.Where["hidden"] != 0 {
			t.Errorf("reveal view hides %s", r.Name)
		}
	}
	g.SetMode(ModePlay)
	if _, err := g.DeckList(state.P1, "", "test", "test"); err == nil {
		t.Error("reveal deck list in play mode")
	}
}
