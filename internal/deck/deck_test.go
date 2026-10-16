package deck

import "testing"

func TestParseLines(t *testing.T) {
	l, err := Parse(`Pokémon: 3
2 Riolu MEG 76
1 Lillie's Clefairy ex

Trainer: 2
1 Team Rocket's Petrel
1 Prime Catcher TEF 157

Energy: 12
4 Basic {F} Energy SVE 6
8 Basic Psychic Energy

Total Cards: 17`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Count: 2, Name: "Riolu", Set: "MEG", Number: "76"},
		{Count: 1, Name: "Lillie's Clefairy ex"},
		{Count: 1, Name: "Team Rocket's Petrel"},
		{Count: 1, Name: "Prime Catcher", Set: "TEF", Number: "157"},
		{Count: 4, Name: "Basic {F} Energy", Set: "SVE", Number: "6"},
		{Count: 8, Name: "Basic Psychic Energy"},
	}
	if len(l.Entries) != len(want) {
		t.Fatalf("got %d entries", len(l.Entries))
	}
	for i, w := range want {
		g := l.Entries[i]
		if g.Count != w.Count || g.Name != w.Name || g.Set != w.Set || g.Number != w.Number {
			t.Errorf("line %d: got %+v, want %+v", i, g, w)
		}
	}
}
