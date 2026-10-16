package importer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

const corpus = "../../testdata"

type oracle struct {
	CoinTossWinner string         `json:"coin_toss_winner"`
	WentFirst      string         `json:"went_first"`
	Turns          map[string]int `json:"turns"`
	TotalTurns     int            `json:"total_turns"`
	PrizesTaken    map[string]int `json:"prizes_taken"`
	Winner         string         `json:"winner"`
	EndReason      string         `json:"end_reason"`
}

func loadCorpus(t *testing.T) ([]deck.Entry, []struct {
	File     string `json:"file"`
	Player   string `json:"player"`
	Expected oracle `json:"expected"`
}) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(corpus, "decks/mega-lucario-ex-v12.txt"))
	if err != nil {
		t.Skip("no test corpus")
	}
	l, err := deck.Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	var idx struct {
		Games []struct {
			File     string `json:"file"`
			Player   string `json:"player"`
			Expected oracle `json:"expected"`
		} `json:"games"`
	}
	b, err = os.ReadFile(filepath.Join(corpus, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		t.Fatal(err)
	}
	return l.Entries, idx.Games
}

func TestCorpus(t *testing.T) {
	entries, games := loadCorpus(t)
	for _, g := range games {
		t.Run(filepath.Base(g.File), func(t *testing.T) {
			text, err := os.ReadFile(filepath.Join(corpus, g.File))
			if err != nil {
				t.Fatal(err)
			}
			res, err := Import(string(text), Options{MyDeck: entries, Seed: 1})
			if err != nil {
				t.Fatal(err)
			}
			rep := res.Report
			for _, p := range rep.Problems {
				t.Errorf("line %d %q: %s", p.Line, p.Text, p.Err)
			}
			for _, w := range rep.Warnings {
				t.Errorf("warning: %s", w)
			}
			if rep.Me != g.Player {
				t.Errorf("me = %s, want %s", rep.Me, g.Player)
			}
			got := oracle{rep.CoinTossWinner, rep.WentFirst, rep.Turns, rep.TotalTurns, rep.PrizesTaken, rep.Winner, rep.EndReason}
			gb, _ := json.Marshal(got)
			wb, _ := json.Marshal(g.Expected)
			if string(gb) != string(wb) {
				t.Errorf("report mismatch\n got %s\nwant %s", gb, wb)
			}

			// The rewritten event log folds cleanly and holds every card.
			st, err := state.Fold(res.Events())
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range st.Check() {
				t.Errorf("invariant: %s", p)
			}
			for iid, c := range st.Cards {
				if c.Owner == MeSeat && c.Name == "" {
					t.Errorf("%s has no identity", iid)
				}
			}
		})
	}
}
