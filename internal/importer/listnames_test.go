package importer

import (
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// tcgLiveDeck is a list exactly as TCG Live's deck editor exports it: the
// section headers count lines, not cards; Energy is written with symbols
// ("Basic {F} Energy"); and alternate prints of a card are separate lines.
const tcgLiveDeck = `Pokémon: 10
1 Riolu MEP 10
3 Riolu MEG 76
1 Mega Lucario ex MEP 12
1 Mega Lucario ex MEG 77
1 Mega Lucario ex MEG 179
3 Solrock MEG 75
2 Lunatone MEG 74
2 Makuhita MEG 72
2 Hariyama MEG 73
1 Meowth ex POR 121

Trainer: 10
4 Lillie's Determination MEG 119
4 Fighting Gong MEG 116
4 Poké Pad POR 81
4 Premium Power Pro MEG 124
4 Ultra Ball MEG 131
2 Judge POR 76
2 Boss's Orders MEG 114
2 Gravity Mountain SSP 177
1 Wally's Compassion MEG 132
1 Switch MEG 130

Energy: 2
2 Rocky {F} Energy POR 87
13 Basic {F} Energy MEE 14

Total Cards: 60`

// tcgLiveOppDeck is an assumed opponent list in the same export format.
const tcgLiveOppDeck = `Pokémon: 1
4 Abra MEG 54

Trainer: 1
4 Buddy-Buddy Poffin TEF 144

Energy: 2
4 Telepathic {P} Energy POR 88
48 Basic {P} Energy MEE 13

Total Cards: 60`

// exportLog is a short game whose log names the Energy in words, as TCG
// Live's battle log does, and draws every print of Riolu and Mega Lucario
// ex the list holds.
const exportLog = `Setup
NicJackson chose heads for the opening coin flip.
NicJackson won the coin toss.
NicJackson decided to go first.
NicJackson drew 7 cards for the opening hand.
- 7 drawn cards.
   • Riolu, Rocky Fighting Energy, Riolu, Basic Fighting Energy, Mega Lucario ex, Riolu, Wally's Compassion
LeDjo23 drew 7 cards for the opening hand.
- 7 drawn cards.
NicJackson played Riolu to the Active Spot.
NicJackson played Riolu to the Bench.
LeDjo23 played Abra to the Active Spot.

NicJackson's Turn
NicJackson drew Riolu.
NicJackson played Riolu to the Bench.
NicJackson attached Rocky Fighting Energy to Riolu in the Active Spot.
NicJackson ended their turn.

LeDjo23's Turn
LeDjo23 drew a card.
LeDjo23 attached Telepathic Psychic Energy to Abra in the Active Spot.
LeDjo23 ended their turn.

NicJackson's Turn
NicJackson drew Mega Lucario ex.
NicJackson evolved Riolu to Mega Lucario ex in the Active Spot.
NicJackson evolved Riolu to Mega Lucario ex on the Bench.
NicJackson attached Basic Fighting Energy to Mega Lucario ex in the Active Spot.
NicJackson ended their turn.

LeDjo23's Turn
LeDjo23 drew a card.
LeDjo23 attached Basic Psychic Energy to Abra in the Active Spot.
NicJackson's Mega Lucario ex took 130 damage.
LeDjo23 ended their turn.

NicJackson's Turn
NicJackson drew Mega Lucario ex.
NicJackson evolved Riolu to Mega Lucario ex on the Bench.
NicJackson played Wally's Compassion.
- NicJackson's Mega Lucario ex healed 130 damage.
NicJackson attached Basic Fighting Energy to Mega Lucario ex in the Active Spot.
NicJackson ended their turn.
`

// TestTCGLiveExportImports imports against lists exactly as TCG Live
// exports them. The log names cards as the card does ("Rocky Fighting
// Energy"), not as the export writes them ("Rocky {F} Energy"), so both
// lists must be keyed by the log's names: before, the whole replay failed
// from the opening hand and the owner's own list was blamed on the
// opponent's ("the assumed list conflicts with the log").
func TestTCGLiveExportImports(t *testing.T) {
	mine, err := deck.Parse(tcgLiveDeck)
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := deck.Parse(tcgLiveOppDeck)
	if err != nil {
		t.Fatal(err)
	}
	for seed := uint64(1); seed <= 5; seed++ {
		res, err := Import(exportLog, Options{MyDeck: mine.Entries, OppDeck: theirs.Entries, Seed: seed})
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
		got := map[state.Seat]map[string]int{MeSeat: {}, OppSeat: {}}
		for iid, c := range st.Cards {
			if c.Name == "" {
				t.Errorf("seed %d: %s has no identity", seed, iid)
			}
			got[c.Owner][c.Name]++
		}
		for seat, want := range map[state.Seat]map[string]int{
			MeSeat:  {"Riolu": 4, "Mega Lucario ex": 3, "Rocky Fighting Energy": 2, "Basic Fighting Energy": 13},
			OppSeat: {"Abra": 4, "Telepathic Psychic Energy": 4, "Basic Psychic Energy": 48},
		} {
			for name, n := range want {
				if got[seat][name] != n {
					t.Errorf("seed %d: %s has %d %s, want %d", seed, seat, got[seat][name], name, n)
				}
			}
		}
		for name := range got[MeSeat] {
			if strings.Contains(name, "{") {
				t.Errorf("seed %d: a card is named %q, the export's spelling", seed, name)
			}
		}
	}
}

// TestWallysCompassionReturnsEnergy: Wally's Compassion puts all Energy
// attached to the Pokémon it healed into the hand. Newer logs print only
// the heal, so the importer makes the move itself; older ones print the
// move too, which must not move the Energy twice.
func TestWallysCompassionReturnsEnergy(t *testing.T) {
	mine, err := deck.Parse(tcgLiveDeck)
	if err != nil {
		t.Fatal(err)
	}
	heal := "- NicJackson's Mega Lucario ex healed 130 damage.\n"
	for name, text := range map[string]string{
		"heal only":    exportLog,
		"move printed": strings.Replace(exportLog, heal, heal+"- NicJackson moved NicJackson's 2 cards to their hand.\n   • Rocky Fighting Energy, Basic Fighting Energy\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			res, err := Import(text, Options{MyDeck: mine.Entries, Seed: 1})
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range res.Report.Problems {
				t.Errorf("line %d %q: %s", p.Line, p.Text, p.Err)
			}
			evs := res.Events()
			st := state.New()
			returned := 0
			for i := range evs {
				if err := st.Apply(&evs[i]); err != nil {
					t.Fatal(err)
				}
				p, _ := evs[i].Decode()
				if mv, ok := p.(*state.MovePayload); ok && mv.From != nil && mv.From.Seat == MeSeat && mv.From.Zone == state.Active && mv.To.Zone == state.Hand {
					returned++
				}
			}
			if returned != 2 {
				t.Errorf("%d cards went from the Active Pokémon to the hand, want its 2 Energy", returned)
			}
			// The Energy attached after Wally's is the only one left.
			active := st.Sides[MeSeat].Active
			if st.Name(active.Top()) != "Mega Lucario ex" || active.Damage != 0 || len(active.Energy) != 1 {
				t.Errorf("Active %s has %d damage and %d Energy, want Mega Lucario ex with 0 and 1", st.Name(active.Top()), active.Damage, len(active.Energy))
			}
		})
	}
}
