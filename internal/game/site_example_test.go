package game

import (
	"context"
	"maps"
	"os"
	"strings"
	"testing"
)

// The website's example game (site/src/content/example/battle-log.txt) is a
// corpus game with the opponent's player name replaced, so it can be shared.
const (
	siteExamplePath     = "../../site/src/content/example/battle-log.txt"
	siteExampleOriginal = "../../testdata/logs/2026-10-11_gatorusoju_mewex-bronzong-lopunny.txt"
	siteExampleDeck     = "../../testdata/decks/mega-lucario-ex-v12.txt"
	siteExampleRealName = "GatoruSoju"
	siteExampleStandIn  = "Rival"
)

// TestSiteExampleGame: the website's example log is the corpus game with
// only the opponent's name changed, line for line, holds no trace of the
// real name, and imports with its decklist cleanly and with the original
// result (testdata/index.json: GatoruSoju won 6 prizes to 4, going first,
// 7 turns to 6, all prizes taken).
func TestSiteExampleGame(t *testing.T) {
	example, err := os.ReadFile(siteExamplePath)
	if err != nil {
		t.Skip("no site example game")
	}
	original, err := os.ReadFile(siteExampleOriginal)
	if err != nil {
		t.Skip("no test corpus")
	}
	deck, err := os.ReadFile(siteExampleDeck)
	if err != nil {
		t.Skip("no test corpus")
	}

	if strings.Contains(string(example), siteExampleRealName) || strings.Contains(strings.ToLower(string(example)), strings.ToLower(siteExampleRealName)) {
		t.Fatalf("the site example still names %s", siteExampleRealName)
	}
	want := strings.ReplaceAll(string(original), siteExampleRealName, siteExampleStandIn)
	if got := string(example); got != want {
		gl, wl := strings.Split(strings.TrimSuffix(got, "\n"), "\n"), strings.Split(strings.TrimSuffix(want, "\n"), "\n")
		if len(gl) != len(wl) {
			t.Fatalf("the site example has %d lines, want %d (the corpus game with only %q replaced)", len(gl), len(wl), siteExampleRealName)
		}
		for i := range wl {
			if gl[i] != wl[i] {
				t.Fatalf("site example line %d = %q, want %q (the corpus game with only %q replaced)", i+1, gl[i], wl[i], siteExampleRealName)
			}
		}
		t.Fatal("the site example differs from the corpus game in more than the opponent's name")
	}
	if !strings.HasPrefix(string(example), "Setup\n") || !strings.HasSuffix(string(example), "Opponent took all of their Prize cards. Rival wins.\n") {
		t.Error("the site example does not run from Setup to the result line")
	}

	res, err := NewManager(t.TempDir(), nil).Import(context.Background(), ImportRequest{Log: string(example), Deck: string(deck), Seed: 1, ID: "g-site-example"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Report.Problems {
		t.Errorf("line %d %q: %s", p.Line, p.Text, p.Err)
	}
	for _, w := range res.Game.State().Check() {
		t.Errorf("invariant: %s", w)
	}
	r := res.Report
	if r.Me != "NicJackson" || r.Opponent != siteExampleStandIn {
		t.Errorf("players %q vs %q, want NicJackson vs %s", r.Me, r.Opponent, siteExampleStandIn)
	}
	if r.Winner != siteExampleStandIn || r.EndReason != "all_prizes_taken" || r.WentFirst != siteExampleStandIn {
		t.Errorf("winner %q (%s), first %q; want %s, all_prizes_taken, %s first", r.Winner, r.EndReason, r.WentFirst, siteExampleStandIn, siteExampleStandIn)
	}
	if want := map[string]int{"NicJackson": 4, siteExampleStandIn: 6}; !maps.Equal(r.PrizesTaken, want) {
		t.Errorf("prizes taken %v, want %v", r.PrizesTaken, want)
	}
	if want := map[string]int{"NicJackson": 6, siteExampleStandIn: 7}; !maps.Equal(r.Turns, want) {
		t.Errorf("turns %v, want %v", r.Turns, want)
	}
}
