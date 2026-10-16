package importer

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// corpusGame is one game in the corpus index. Observed spot-checks
// minimum opponent counts the log proves.
type corpusGame struct {
	File     string         `json:"file"`
	Player   string         `json:"player"`
	Expected oracle         `json:"expected"`
	Observed map[string]int `json:"observed,omitempty"`
}

func loadCorpus(t *testing.T) ([]deck.Entry, []corpusGame) {
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
		Games []corpusGame `json:"games"`
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
			copies := map[string]int{}
			for iid, c := range st.Cards {
				if c.Owner == MeSeat && c.Name == "" {
					t.Errorf("%s has no identity", iid)
				}
				if c.Owner == OppSeat && c.Name != "" {
					copies[c.Name]++
				}
			}
			// No deck holds more than four of a card, so neither the
			// reconstruction nor the proven minimums may.
			for name, n := range copies {
				if limited(name) && n > MaxCopies {
					t.Errorf("reconstruction has %d %s", n, name)
				}
			}
			for name, n := range rep.Observed {
				if limited(name) && n > MaxCopies {
					t.Errorf("observed %d %s", n, name)
				}
				if n > copies[name] {
					t.Errorf("observed %d %s but the reconstruction has %d", n, name, copies[name])
				}
			}
			for name, n := range g.Observed {
				if rep.Observed[name] != n {
					t.Errorf("observed %d %s, want %d", rep.Observed[name], name, n)
				}
			}
		})
	}
}

// TestOppDeck imports with an assumed opponent list that undercounts a
// card the log proves: the list is raised to match and fills every card.
func TestOppDeck(t *testing.T) {
	entries, _ := loadCorpus(t)
	text, err := os.ReadFile(filepath.Join(corpus, "logs/2026-10-11_ledjo23_alakazam-donk.txt"))
	if err != nil {
		t.Fatal(err)
	}
	opp := []deck.Entry{{Name: "Abra", Count: 3}, {Name: "Basic Psychic Energy", Count: 57}}
	res, err := Import(string(text), Options{MyDeck: entries, OppDeck: opp, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(res.Report.Warnings, "opponent list: raised Abra from 3 to the 4 seen") {
		t.Errorf("warnings: %v", res.Report.Warnings)
	}
	if !slices.ContainsFunc(res.Report.Warnings, func(w string) bool { return strings.Contains(w, "conflicts with the log") }) {
		t.Errorf("no fallback warning: %v", res.Report.Warnings)
	}
	if len(res.Report.Problems) > 0 {
		t.Errorf("problems after falling back: %v", res.Report.Problems)
	}
	st, err := state.Fold(res.Events())
	if err != nil {
		t.Fatal(err)
	}
	abra, unnamed := 0, 0
	for _, c := range st.Cards {
		if c.Owner != OppSeat {
			continue
		}
		switch c.Name {
		case "Abra":
			abra++
		case "":
			unnamed++
		}
	}
	// Reconcile adds the other cards seen, so the list overfills the deck
	// and every unnamed card gets an identity.
	if abra != 4 || unnamed != 0 {
		t.Errorf("%d Abra, %d unnamed", abra, unnamed)
	}
}

// TestCorpusSeeds replays every log with many seeds: the arbitrary choices
// a seed drives (which unseen card a draw takes) must never leave the
// replay stuck, and what the log proves must not depend on them.
func TestCorpusSeeds(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	entries, games := loadCorpus(t)
	for _, g := range games {
		t.Run(filepath.Base(g.File), func(t *testing.T) {
			text, err := os.ReadFile(filepath.Join(corpus, g.File))
			if err != nil {
				t.Fatal(err)
			}
			for seed := uint64(2); seed <= 25; seed++ {
				res, err := Import(string(text), Options{MyDeck: entries, Seed: seed})
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range res.Report.Problems {
					t.Errorf("seed %d: line %d %q: %s", seed, p.Line, p.Text, p.Err)
				}
				for name, n := range g.Observed {
					if got := res.Report.Observed[name]; got != n {
						t.Errorf("seed %d: observed %d %s, want %d", seed, got, name, n)
					}
				}
				if msg := oneToolEach(res.Events()); msg != "" {
					t.Errorf("seed %d: %s", seed, msg)
				}
			}
		})
	}
}

// TestOppDeckCounts holds the opponent to an assumed list that agrees with
// the log: the reconstruction then has exactly the list's counts.
func TestOppDeckCounts(t *testing.T) {
	entries, _ := loadCorpus(t)
	text, err := os.ReadFile(filepath.Join(corpus, "logs/2026-10-11_carlos0oz_dragapult.txt"))
	if err != nil {
		t.Fatal(err)
	}
	l, err := deck.Parse(`4 Dreepy
3 Drakloak
2 Dragapult ex
1 Budew
2 Munkidori
1 Fezandipiti ex
2 Dunsparce
2 Dudunsparce
4 Lillie's Determination
2 Crispin
1 Rosa's Encouragement
2 Boss's Orders
4 Buddy-Buddy Poffin
4 Poké Pad
4 Crushing Hammer
3 Ultra Ball
2 Night Stretcher
2 Rare Candy
2 Switch
1 Sparkling Crystal
2 Risky Ruins
4 Basic Fire Energy
4 Basic Psychic Energy
2 Basic Darkness Energy`)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{}
	for _, e := range l.Entries {
		want[e.Name] += e.Count
	}
	for seed := uint64(1); seed <= 10; seed++ {
		res, err := Import(string(text), Options{MyDeck: entries, OppDeck: l.Entries, Seed: seed})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range res.Report.Problems {
			t.Errorf("seed %d: line %d %q: %s", seed, p.Line, p.Text, p.Err)
		}
		for _, w := range res.Report.Warnings {
			t.Errorf("seed %d: warning: %s", seed, w)
		}
		st, err := state.Fold(res.Events())
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int{}
		for _, c := range st.Cards {
			if c.Owner == OppSeat {
				got[c.Name]++
			}
		}
		if !maps.Equal(got, want) {
			t.Errorf("seed %d: opponent deck %v, want %v", seed, got, want)
		}
	}
}

// oneToolEach replays events and reports the first Pokémon holding more
// than one Tool, which the rules never allow.
func oneToolEach(events []state.Event) string {
	st := state.New()
	for i := range events {
		if err := st.Apply(&events[i]); err != nil {
			return err.Error()
		}
		for _, seat := range state.Seats {
			for _, l := range st.StackLocs(seat) {
				if s, _ := st.Stack(l); len(s.Tools) > 1 {
					return fmt.Sprintf("event %d: %s holds %d Tools", events[i].ID, l, len(s.Tools))
				}
			}
		}
	}
	return ""
}

// TestToolCredits places a Tool on the copy the damage breakdown proves
// held it: the Night Joker that counted Binding Mochi came from a Zoroark
// holding one.
func TestToolCredits(t *testing.T) {
	entries, _ := loadCorpus(t)
	text, err := os.ReadFile(filepath.Join(corpus, "logs/2026-10-11_absolutezere_ns-zoroark.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for seed := uint64(1); seed <= 25; seed++ {
		res, err := Import(string(text), Options{MyDeck: entries, Seed: seed})
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range res.Report.Warnings {
			t.Errorf("seed %d: %s", seed, w)
		}
		st := state.New()
		evs := res.Events()
		checked := false
		for i := range evs {
			e := &evs[i]
			if err := st.Apply(e); err != nil {
				t.Fatal(err)
			}
			if e.Type != state.EvDamage || e.Seat != OppSeat {
				continue
			}
			// Damage events hold the new total; this is the 290 hit on Nic's
			// Mega Lucario ex.
			if p, _ := e.Decode(); p.(*state.DamagePayload).Value != 290 || p.(*state.DamagePayload).Target.Seat != MeSeat {
				continue
			}
			a := st.Sides[OppSeat].Active
			if !slices.ContainsFunc(a.Tools, func(x state.IID) bool { return st.Cards[x].Name == "Binding Mochi" }) {
				t.Errorf("seed %d: the Zoroark that hit for 290 with Binding Mochi holds %v", seed, a.Tools)
			}
			checked = true
		}
		if !checked {
			t.Fatalf("seed %d: no 290 damage found", seed)
		}
	}
}
