package game

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/infer"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// TestSampleHidden samples the opponent's hidden cards at a rewound
// position of an imported game and checks the sample respects what Nic
// could see.
func TestSampleHidden(t *testing.T) {
	deckText, err := os.ReadFile("../../testdata/decks/mega-lucario-ex-v12.txt")
	if err != nil {
		t.Skip("no test corpus")
	}
	text, err := os.ReadFile("../../testdata/logs/2026-10-11_carlos0oz_dragapult.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), nil)
	imp, err := m.Import(context.Background(), ImportRequest{Log: string(text), Deck: string(deckText), Seed: 1, ID: "g"})
	if err != nil {
		t.Fatal(err)
	}
	g := imp.Game
	opp, me := state.P2, state.P1

	// A list that holds exactly what the log proves, padded with Energy.
	var list []deck.Entry
	n := 0
	for name, c := range imp.Report.Observed {
		list = append(list, deck.Entry{Name: name, Count: c})
		n += c
	}
	list = append(list, deck.Entry{Name: "Basic Grass Energy", Count: state.DeckSize - n})

	// Rewind to the start of the opponent's third turn.
	at := 0
	path, _ := g.log.PathTo(g.Head())
	for _, e := range path {
		if e.Type == state.EvTurn && e.Seat == opp {
			if p, _ := e.Decode(); p.(*state.TurnPayload).Phase == "start" && p.(*state.TurnPayload).Number == 5 {
				at = e.ID
			}
		}
	}
	if at == 0 {
		t.Fatal("no turn 5")
	}
	sample := func(branch string, list []deck.Entry, seed uint64) (*state.State, *state.State, *Result) {
		t.Helper()
		if _, err := g.Rewind(at, branch); err != nil {
			t.Fatal(err)
		}
		before := g.State()
		res, err := g.SampleHidden(opp, state.ActorClaude, list, seed)
		if err != nil {
			t.Fatal(err)
		}
		return before, g.State(), res
	}
	listCount := map[string]int{}
	for _, e := range list {
		listCount[e.Name] += e.Count
	}
	var after7 *state.State
	for seed := uint64(1); seed <= 20; seed++ {
		before, after, res := sample(fmt.Sprintf("seed-%d", seed), list, seed)
		if seed == 7 {
			after7 = after
		}
		g.mu.Lock()
		ev, err := g.evidence(opp)
		g.mu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
		if ev.OutsidePrizes["Crushing Hammer"] != 2 {
			t.Errorf("by turn 5 Nic had seen %d Crushing Hammer outside the prizes, want 2", ev.OutsidePrizes["Crushing Hammer"])
		}
		if len(res.Warnings) > 0 {
			t.Errorf("warnings: %v", res.Warnings)
		}
		for _, p := range after.Check() {
			t.Errorf("invariant: %s", p)
		}

		total, prized := map[string]int{}, map[string]int{}
		for _, cl := range after.Locations() {
			c := after.Cards[cl.IID]
			if c.Owner != opp {
				continue
			}
			total[c.Name]++
			if cl.Loc.Zone.IsPublic() || after.KnownBy(cl.IID, me) {
				if b := before.Cards[cl.IID].Name; b != c.Name {
					t.Errorf("%s is visible to Nic but changed from %s to %s", cl.IID, b, c.Name)
				}
				continue
			}
			if c.Basis != "sampled" {
				t.Errorf("%s is hidden but has basis %q", cl.IID, c.Basis)
			}
			if cl.Loc.Zone == state.Prizes {
				prized[c.Name]++
			}
		}
		for name, want := range listCount {
			if total[name] != want {
				t.Errorf("%d %s in the sampled deck, list has %d", total[name], name, want)
			}
		}
		// Copies Nic saw at once outside the prizes after the deal are not
		// prizes. Mulligan reveals came before the deal, so they do not count.
		for name, n := range prized {
			if free := listCount[name] - ev.OutsidePrizes[name]; n > free {
				t.Errorf("%d %s prized, but %d of %d were seen outside the prizes", n, name, ev.OutsidePrizes[name], listCount[name])
			}
		}
	}

	// The list is stored: a second branch reuses it, and a seed repeats.
	_, a1, _ := sample("s2", nil, 7)
	_, a2, _ := sample("s3", nil, 7)
	for iid, c := range a1.Cards {
		if a2.Cards[iid].Name != c.Name {
			t.Fatalf("same seed, different sample: %s is %s then %s", iid, c.Name, a2.Cards[iid].Name)
		}
		if c.Owner == opp && c.Name != after7.Cards[iid].Name {
			t.Fatalf("reused list, different sample from the same seed: %s", iid)
		}
	}
	rep, err := g.Profile(opp)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Profile == nil || rep.Profile.Total() != state.DeckSize {
		t.Errorf("stored profile %+v", rep.Profile)
	}

	// Play mode refuses.
	g.SetMode(ModePlay)
	if _, err := g.SampleHidden(opp, state.ActorClaude, list, 1); err == nil {
		t.Error("sampled in play mode")
	}
}

func TestReconcile(t *testing.T) {
	list := []deck.Entry{{Name: "Abra", Count: 3}, {Name: "Kadabra", Count: 2}, {Name: "Basic Psychic Energy", Count: 55}}
	p, warnings := infer.Reconcile(list, map[string]int{"Abra": 4, "Budew": 1, "Kadabra": 1})
	got := map[string]infer.ProfileCard{}
	for _, c := range p.Cards {
		got[c.Name] = c
	}
	if c := got["Abra"]; c.Count != 4 || c.Basis != "observed" {
		t.Errorf("Abra %+v", c)
	}
	if c := got["Budew"]; c.Count != 1 || c.Basis != "observed" {
		t.Errorf("Budew %+v", c)
	}
	if c := got["Kadabra"]; c.Count != 2 || c.Basis != "reference" || c.MinObserved != 1 {
		t.Errorf("Kadabra %+v", c)
	}
	if len(warnings) != 3 { // raised Abra, added Budew, total 62
		t.Errorf("warnings %v", warnings)
	}
}
