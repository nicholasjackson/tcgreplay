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
			// Guard the fixture: the expected total is the combined turn
			// count, the higher of the two players' counts.
			most := 0
			for _, n := range g.Expected.Turns {
				most = max(most, n)
			}
			if g.Expected.TotalTurns != most {
				t.Errorf("index expects total_turns %d, but the most turns either player took is %d (%v)", g.Expected.TotalTurns, most, g.Expected.Turns)
			}

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

// TestTotalTurnsCombined checks the report's total is the combined turn
// count, the higher of the two players' counts, not their sum.
func TestTotalTurnsCombined(t *testing.T) {
	entries, _ := loadCorpus(t)
	cases := []struct {
		name  string
		file  string
		turns map[string]int
		total int
	}{
		{"five each", "logs/2026-10-11_haagensen_ns-zoroark-rematch.txt", map[string]int{"NicJackson": 5, "Haagensen": 5}, 5},
		{"six and five", "logs/2026-10-11_chef0221_alakazam-dusclops.txt", map[string]int{"NicJackson": 6, "CHEF0221": 5}, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, err := os.ReadFile(filepath.Join(corpus, c.file))
			if err != nil {
				t.Fatal(err)
			}
			res, err := Import(string(text), Options{MyDeck: entries, Seed: 1})
			if err != nil {
				t.Fatal(err)
			}
			if !maps.Equal(res.Report.Turns, c.turns) {
				t.Errorf("turns = %v, want %v", res.Report.Turns, c.turns)
			}
			if res.Report.TotalTurns != c.total {
				t.Errorf("total turns = %d, want %d", res.Report.TotalTurns, c.total)
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

// TestAttackDiscardsStadium discards the Stadium in play when an attack's
// subline names it: Chi-Yu's Ground Melter ends "discard that Stadium",
// logged as "T4TSUYA discarded Team Rocket's Watchtower." under the attack.
// It must not take another copy from the hand or deck instead.
func TestAttackDiscardsStadium(t *testing.T) {
	entries, _ := loadCorpus(t)
	text, err := os.ReadFile(filepath.Join(corpus, "logs/2026-10-20_t4tsuya_dragapult-blaziken.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for seed := uint64(1); seed <= 10; seed++ {
		res, err := Import(string(text), Options{MyDeck: entries, Seed: seed})
		if err != nil {
			t.Fatal(err)
		}
		st, err := state.Fold(res.Events())
		if err != nil {
			t.Fatal(err)
		}
		if len(st.Stadium) != 0 {
			t.Errorf("seed %d: %s is still in play", seed, st.Cards[st.Stadium[0]].Name)
		}
		n := 0
		for _, iid := range st.Sides[OppSeat].Discard {
			if st.Cards[iid].Name == "Team Rocket's Watchtower" {
				n++
			}
		}
		if n != 1 {
			t.Errorf("seed %d: %d Team Rocket's Watchtower in the discard, want 1", seed, n)
		}
	}
}

// TestDefOfGetsMoves asks for an opponent card's print with the moves the
// log shows it using, so a Chi-Yu that used Ground Melter gets the print
// that has it. An assumed list naming Chi-Yu without a set code does not
// pin the print; it is the fallback when the lookup finds none.
func TestDefOfGetsMoves(t *testing.T) {
	entries, _ := loadCorpus(t)
	text, err := os.ReadFile(filepath.Join(corpus, "logs/2026-10-20_t4tsuya_dragapult-blaziken.txt"))
	if err != nil {
		t.Fatal(err)
	}
	chiYuDef := func(opts Options) string {
		t.Helper()
		res, err := Import(string(text), opts)
		if err != nil {
			t.Fatal(err)
		}
		st, err := state.Fold(res.Events())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range st.Cards {
			if c.Owner == OppSeat && c.Name == "Chi-Yu" {
				return c.Def
			}
		}
		t.Fatal("no Chi-Yu")
		return ""
	}
	var asked []string
	defOf := func(name string, moves []string) string {
		if name != "Chi-Yu" {
			return ""
		}
		asked = moves
		if slices.Contains(moves, "Ground Melter") {
			return "sv06-039"
		}
		return "me05-059"
	}
	if got := chiYuDef(Options{MyDeck: entries, Seed: 1, DefOf: defOf}); got != "sv06-039" {
		t.Errorf("Chi-Yu is %s, want sv06-039", got)
	}
	if !slices.Equal(asked, []string{"Ground Melter"}) {
		t.Errorf("DefOf got moves %v, want [Ground Melter]", asked)
	}

	byName := []deck.Entry{{Count: 1, Name: "Chi-Yu", Def: "me05-059"}}
	if got := chiYuDef(Options{MyDeck: entries, OppDeck: byName, Seed: 1, DefOf: defOf}); got != "sv06-039" {
		t.Errorf("with a by-name list, Chi-Yu is %s, want sv06-039", got)
	}
	offline := func(string, []string) string { return "" }
	if got := chiYuDef(Options{MyDeck: entries, OppDeck: byName, Seed: 1, DefOf: offline}); got != "me05-059" {
		t.Errorf("with no lookup, Chi-Yu is %s, want the list's me05-059", got)
	}
	pinned := []deck.Entry{{Count: 1, Name: "Chi-Yu", Set: "TWM", Number: "39", Def: "sv06-039-pinned"}}
	if got := chiYuDef(Options{MyDeck: entries, OppDeck: pinned, Seed: 1, DefOf: defOf}); got != "sv06-039-pinned" {
		t.Errorf("with a set code in the list, Chi-Yu is %s, want the list's print", got)
	}
}

// TestPlaysTheRevealedCopy plays the copy the other player saw: when a
// hand holds a revealed card and an unrevealed copy of it, playing that
// name must not leave the revealed one face up in hand. The Dragapult game
// reveals Dudunsparce with Poké Pad while a copy named at the mulligan
// sits unseen in hand.
func TestPlaysTheRevealedCopy(t *testing.T) {
	entries, games := loadCorpus(t)
	for _, g := range games {
		t.Run(filepath.Base(g.File), func(t *testing.T) {
			text, err := os.ReadFile(filepath.Join(corpus, g.File))
			if err != nil {
				t.Fatal(err)
			}
			for seed := uint64(1); seed <= 10; seed++ {
				res, err := Import(string(text), Options{MyDeck: entries, Seed: seed})
				if err != nil {
					t.Fatal(err)
				}
				if msg := staleReveal(res.Events()); msg != "" {
					t.Errorf("seed %d: %s", seed, msg)
				}
			}
		})
	}
}

// staleReveal reports a card played face up from hand unseen while a copy
// of it the other player had seen stayed behind in that hand. Cards
// returned to the deck are not checked: a whole hand goes back one card at
// a time.
func staleReveal(events []state.Event) string {
	st := state.New()
	for i := range events {
		e := &events[i]
		if p, _ := e.Decode(); e.Type == state.EvMove {
			m := p.(*state.MovePayload)
			if m.From != nil && m.From.Zone == state.Hand && m.To.Zone.IsPublic() {
				seat := m.From.Seat
				name := st.Cards[m.IID].Name
				if !st.KnownBy(m.IID, seat.Other()) {
					for _, x := range st.Sides[seat].Hand {
						if x != m.IID && st.Cards[x].Name == name && st.KnownBy(x, seat.Other()) {
							return fmt.Sprintf("event %d: %s played unseen %s; the seen %s stayed in hand", e.ID, seat, m.IID, x)
						}
					}
				}
			}
		}
		if err := st.Apply(e); err != nil {
			return err.Error()
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

// TestSingleDamageCounters reads a lone counter written as "a damage
// counter", whether a player put it or an effect placed it between turns
// (Freezing Shroud at Pokémon Checkup).
func TestSingleDamageCounters(t *testing.T) {
	entries, _ := loadCorpus(t)
	b, err := os.ReadFile(filepath.Join(corpus, "logs/2026-10-11_ledjo23_alakazam-donk.txt"))
	if err != nil {
		t.Fatal(err)
	}
	extra := "NicJackson drew Hariyama.\n" +
		"1 damage counter was placed on NicJackson's Solrock for the effect of Freezing Shroud.\n" +
		"LeDjo23 put a damage counter on NicJackson's Solrock.\n"
	text := strings.Replace(string(b), "NicJackson drew Hariyama.\n", extra, 1)
	res, err := Import(text, Options{MyDeck: entries, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Report.Problems {
		if strings.Contains(p.Text, "damage counter") {
			t.Errorf("line %d not imported: %q: %s", p.Line, p.Text, p.Err)
		}
	}
	// Itchy Pollen left 20; the two lines add 10 each.
	st := state.New()
	evs := res.Events()
	var got []int
	for i := range evs {
		e := &evs[i]
		if err := st.Apply(e); err != nil {
			t.Fatal(err)
		}
		if p, _ := e.Decode(); e.Type == state.EvDamage && p.(*state.DamagePayload).Target.Seat == MeSeat {
			got = append(got, p.(*state.DamagePayload).Value)
		}
	}
	if len(got) < 3 || got[1] != 30 || got[2] != 40 {
		t.Errorf("Solrock's damage went %v, want 20, 30, 40 before the knockout", got)
	}
}

// TestMoveFromUndamagedWarns warns when the log moves counters off a
// Pokémon the board shows without them: lines that put them there were
// missed.
func TestMoveFromUndamagedWarns(t *testing.T) {
	entries, _ := loadCorpus(t)
	b, err := os.ReadFile(filepath.Join(corpus, "logs/2026-10-11_ledjo23_alakazam-donk.txt"))
	if err != nil {
		t.Fatal(err)
	}
	extra := "NicJackson drew Hariyama.\n" +
		"LeDjo23 moved 3 damage counters from LeDjo23's Budew to NicJackson's Solrock.\n"
	text := strings.Replace(string(b), "NicJackson drew Hariyama.\n", extra, 1)
	res, err := Import(text, Options{MyDeck: entries, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range res.Report.Warnings {
		found = found || strings.Contains(w, "moved 30 damage from Budew, which had only 0")
	}
	if !found {
		t.Errorf("no warning about moving damage off an undamaged Budew; warnings %q", res.Report.Warnings)
	}
}

// TestBenchPastFive replays a bench of more than five, as Area Zero
// Underdepths allows: the log is the authority, so no Pokémon is refused.
func TestBenchPastFive(t *testing.T) {
	entries, _ := loadCorpus(t)
	b, err := os.ReadFile(filepath.Join(corpus, "logs/2026-10-11_ledjo23_alakazam-donk.txt"))
	if err != nil {
		t.Fatal(err)
	}
	extra := "- LeDjo23 drew 3 cards.\n" +
		"LeDjo23 played Duskull to the Bench.\n" +
		"LeDjo23 played Duskull to the Bench.\n"
	text := strings.Replace(string(b), "- LeDjo23 drew 3 cards.\n", extra, 1)
	res, err := Import(text, Options{MyDeck: entries, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Report.Problems {
		if strings.Contains(p.Text, "to the Bench") {
			t.Errorf("line %d not imported: %q: %s", p.Line, p.Text, p.Err)
		}
	}
	st := state.New()
	evs := res.Events()
	most := 0
	for i := range evs {
		if err := st.Apply(&evs[i]); err != nil {
			t.Fatal(err)
		}
		if st.Phase == state.PhaseEmpty {
			continue
		}
		n := 0
		for _, s := range st.Sides[OppSeat].Bench {
			if !s.Empty() {
				n++
			}
		}
		most = max(most, n)
	}
	if most <= 5 {
		t.Errorf("opponent's bench peaked at %d, want more than 5", most)
	}
}

// TestRetreatToSameName: a Pokémon that retreats in favour of a benched
// copy of itself goes to the Bench with its damage and Energy, and the
// fresh copy comes in, rather than the damaged one staying Active.
func TestRetreatToSameName(t *testing.T) {
	entries, _ := loadCorpus(t)
	b, err := os.ReadFile(filepath.Join(corpus, "logs/2026-10-11_ledjo23_alakazam-donk.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// Itchy Pollen has left the Active Solrock with 20 damage and an
	// Energy; a second Solrock is drawn, benched and swapped in.
	extra := "NicJackson drew Solrock.\n" +
		"NicJackson played Solrock to the Bench.\n" +
		"NicJackson retreated Solrock to the Bench.\n" +
		"- NicJackson discarded Basic Fighting Energy.\n" +
		"NicJackson's Solrock is now in the Active Spot.\n"
	text := strings.Replace(string(b), "NicJackson drew Hariyama.\n", extra, 1)
	res, err := Import(text, Options{MyDeck: entries, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Report.Problems {
		if strings.Contains(p.Text, "Solrock") && p.Line < 50 {
			t.Errorf("line %d not imported: %q: %s", p.Line, p.Text, p.Err)
		}
	}
	st := state.New()
	evs := res.Events()
	for i := range evs {
		e := &evs[i]
		if err := st.Apply(e); err != nil {
			t.Fatal(err)
		}
		if e.Type != state.EvSwap {
			continue
		}
		if p, _ := e.Decode(); p.(*state.SwapPayload).A.Seat != MeSeat {
			continue
		}
		active := st.Sides[MeSeat].Active
		var benched *state.Stack
		for _, s := range st.Sides[MeSeat].Bench {
			if !s.Empty() && st.Name(s.Top()) == "Solrock" {
				benched = s
			}
		}
		if st.Name(active.Top()) != "Solrock" || benched == nil {
			t.Fatalf("after the swap: Active %s, no benched Solrock", st.Name(active.Top()))
		}
		if active.Damage != 0 || benched.Damage != 20 {
			t.Errorf("after the retreat the Active Solrock has %d damage and the benched one %d; want 0 and 20", active.Damage, benched.Damage)
		}
		return
	}
	t.Fatal("no swap event: the retreat was not replayed")
}

// pairLog is the synthetic Legendary Summit pair log (not a real TCG Live
// log: no real log with a pair has been seen, so its line shapes are
// assumed). LeDjo23 plays two different prints in turn, then NicJackson's
// Gravity Mountain replaces the pair.
const pairLog = "logs/synthetic_2026-11-08_ledjo23_legendary-summit-pair.txt"

const (
	pairLines   = "LeDjo23 played (me6_96) Legendary Summit to the Stadium spot.\nLeDjo23 played (me6_97) Legendary Summit to the Stadium spot.\n"
	pairDiscard = "- LeDjo23 discarded (me6_96) Legendary Summit.\n- LeDjo23 discarded (me6_97) Legendary Summit.\n"
)

// importPairLog imports the synthetic pair log with the edits applied,
// checking it applies cleanly, and returns the result and its events.
func importPairLog(t *testing.T, edits ...[2]string) (*Result, []state.Event) {
	t.Helper()
	entries, _ := loadCorpus(t)
	b, err := os.ReadFile(filepath.Join(corpus, pairLog))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, e := range edits {
		if !strings.Contains(text, e[0]) {
			t.Fatalf("the pair log has no %q", e[0])
		}
		text = strings.Replace(text, e[0], e[1], 1)
	}
	res, err := Import(text, Options{MyDeck: entries, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Report.Problems {
		t.Errorf("line %d %q: %s", p.Line, p.Text, p.Err)
	}
	return res, res.Events()
}

// stadiumMoves lists the moves into or out of the Stadium spot.
func stadiumMoves(t *testing.T, evs []state.Event) []*state.MovePayload {
	t.Helper()
	var out []*state.MovePayload
	for i := range evs {
		p, err := evs[i].Decode()
		if err != nil {
			t.Fatal(err)
		}
		if mv, ok := p.(*state.MovePayload); ok && (mv.To.Zone == state.Stadium || mv.From != nil && mv.From.Zone == state.Stadium) {
			out = append(out, mv)
		}
	}
	return out
}

// pairOnBoard folds events until the Stadium spot holds two cards and
// returns that state, or nil if it never does.
func pairOnBoard(t *testing.T, evs []state.Event) *state.State {
	t.Helper()
	st := state.New()
	for i := range evs {
		if err := st.Apply(&evs[i]); err != nil {
			t.Fatal(err)
		}
		if len(st.Stadium) == 2 {
			return st
		}
	}
	return nil
}

// countIn counts the cards named name in seat's discard.
func countIn(st *state.State, seat state.Seat, name string) int {
	n := 0
	for _, iid := range st.Sides[seat].Discard {
		if st.Cards[iid].Name == name {
			n++
		}
	}
	return n
}

// TestStadiumPairFromLog: two consecutive Stadium-spot lines from the
// same player for two prints of one Stadium put both in the spot as one
// pair (the second joins as pair), and a Stadium played over the pair
// sends both halves to their owner's discard in one move, its two
// "discarded" sublines already done. Every seed agrees.
func TestStadiumPairFromLog(t *testing.T) {
	for seed := uint64(1); seed <= 5; seed++ {
		entries, _ := loadCorpus(t)
		b, err := os.ReadFile(filepath.Join(corpus, pairLog))
		if err != nil {
			t.Fatal(err)
		}
		res, err := Import(string(b), Options{MyDeck: entries, Seed: seed})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Report.Problems) > 0 || len(res.Report.Warnings) > 0 {
			t.Fatalf("seed %d: problems %v, warnings %v", seed, res.Report.Problems, res.Report.Warnings)
		}
		evs := res.Events()
		mid := pairOnBoard(t, evs)
		if mid == nil {
			t.Fatalf("seed %d: the Stadium spot never held the pair", seed)
		}
		for _, iid := range mid.Stadium {
			if c := mid.Cards[iid]; c.Owner != OppSeat || c.Name != "Legendary Summit" {
				t.Errorf("seed %d: %s in the spot is %s's %q", seed, iid, c.Owner, c.Name)
			}
		}
		mv := stadiumMoves(t, evs)
		if len(mv) != 4 || mv[1].As != state.AsPair || mv[2].Pair == "" || mv[2].To.Zone != state.Discard || mv[3].To.Zone != state.Stadium {
			t.Fatalf("seed %d: Stadium moves %+v, want a play, a join as pair, one pair move to the discard and Gravity Mountain", seed, mv)
		}
		st, err := state.Fold(evs)
		if err != nil {
			t.Fatal(err)
		}
		if n := countIn(st, OppSeat, "Legendary Summit"); n != 2 {
			t.Errorf("seed %d: %d Legendary Summit in LeDjo23's discard, want 2", seed, n)
		}
		if len(st.Stadium) != 1 || st.Cards[st.Stadium[0]].Name != "Gravity Mountain" {
			t.Errorf("seed %d: Stadium spot %v, want Gravity Mountain", seed, st.Stadium)
		}
	}
}

// TestStadiumPairOneLine: one line naming the same Stadium twice plays
// both halves together in one move.
func TestStadiumPairOneLine(t *testing.T) {
	res, evs := importPairLog(t, [2]string{pairLines, "LeDjo23 played Legendary Summit and Legendary Summit to the Stadium spot.\n"})
	if len(res.Report.Warnings) > 0 {
		t.Errorf("warnings: %v", res.Report.Warnings)
	}
	if pairOnBoard(t, evs) == nil {
		t.Fatal("the Stadium spot never held the pair")
	}
	mv := stadiumMoves(t, evs)
	if len(mv) != 3 || mv[0].Pair == "" || mv[0].To.Zone != state.Stadium || mv[1].Pair == "" {
		t.Errorf("Stadium moves %+v, want one pair play, one pair discard and Gravity Mountain", mv)
	}
}

// TestStadiumPairDiscardedTwoCards: the replaced pair's halves listed
// under one "discarded 2 cards" line are the halves already moved, not
// two more cards from the hand.
func TestStadiumPairDiscardedTwoCards(t *testing.T) {
	_, evs := importPairLog(t, [2]string{pairDiscard, "- LeDjo23 discarded 2 cards.\n   • Legendary Summit, Legendary Summit\n"})
	st, err := state.Fold(evs)
	if err != nil {
		t.Fatal(err)
	}
	if n := countIn(st, OppSeat, "Legendary Summit"); n != 2 {
		t.Errorf("%d Legendary Summit in LeDjo23's discard, want 2", n)
	}
}

// TestStadiumSamePrintNotAPair: two plays of the same print (the log's
// card codes say so) are not a pair: the second replaces the first, as
// before, with a warning.
func TestStadiumSamePrintNotAPair(t *testing.T) {
	res, evs := importPairLog(t,
		[2]string{"(me6_97) Legendary Summit to the Stadium spot.", "(me6_96) Legendary Summit to the Stadium spot."},
		[2]string{"- LeDjo23 discarded (me6_97) Legendary Summit.\n", ""})
	if !slices.ContainsFunc(res.Report.Warnings, func(w string) bool { return strings.Contains(w, "same print (me6_96)") }) {
		t.Errorf("no same-print warning: %v", res.Report.Warnings)
	}
	if pairOnBoard(t, evs) != nil {
		t.Error("two copies of one print were played as a pair")
	}
	st, err := state.Fold(evs)
	if err != nil {
		t.Fatal(err)
	}
	if n := countIn(st, OppSeat, "Legendary Summit"); n != 2 {
		t.Errorf("%d Legendary Summit in LeDjo23's discard, want 2", n)
	}
}

// TestStadiumLoneHalf: one half played alone imports as a single Stadium,
// as before (the playmat's own standing warning flags it, from card data).
func TestStadiumLoneHalf(t *testing.T) {
	res, evs := importPairLog(t,
		[2]string{pairLines, "LeDjo23 played (me6_96) Legendary Summit to the Stadium spot.\n"},
		[2]string{pairDiscard, "- LeDjo23 discarded (me6_96) Legendary Summit.\n"})
	if len(res.Report.Warnings) > 0 {
		t.Errorf("warnings: %v", res.Report.Warnings)
	}
	if mv := stadiumMoves(t, evs); len(mv) != 3 || mv[0].Pair != "" || mv[0].As != "" {
		t.Errorf("Stadium moves %+v, want a single play and its replacement", mv)
	}
}
