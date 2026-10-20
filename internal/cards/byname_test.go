package cards

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestByNameCached resolves names against the local TCGdex cache, offline.
// It skips when the cache has not been built (run the app once online).
func TestByNameCached(t *testing.T) {
	dir := DefaultCacheDir()
	if _, err := os.Stat(filepath.Join(dir, "sets.json")); err != nil {
		t.Skip("no TCGdex cache")
	}
	c := NewClient(dir)
	c.Offline = true
	ctx := context.Background()
	for _, name := range []string{
		"Dragapult ex", "Lillie's Determination", "Lillie’s Determination",
		"Dunsparce", "Crushing Hammer", "N's Zoroark ex", "Basic Fire Energy",
	} {
		id, err := c.ByName(ctx, name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		setID := id[:strings.LastIndex(id, "-")]
		s, err := c.set(ctx, setID)
		if err != nil {
			t.Fatal(err)
		}
		if s.Serie.ID == "tcgp" {
			t.Errorf("%s resolved to %s, a Pokémon TCG Pocket print", name, id)
		}
		found := false
		for _, card := range s.Cards {
			if card.ID == id {
				found = true
				if got := nameKey(card.Name); got != nameKey(name) && got != strings.TrimPrefix(nameKey(name), "basic ") {
					t.Errorf("%s resolved to %s, named %s", name, id, card.Name)
				}
				if card.Image == "" {
					t.Errorf("%s resolved to %s, which has no art", name, id)
				}
			}
		}
		if !found {
			t.Errorf("%s resolved to %s, not in set %s", name, id, setID)
		}
	}
}

func TestEnergyArt(t *testing.T) {
	c := Card{Name: "Basic Fighting Energy", Category: "Energy", EnergyType: "Normal"}
	energyArt(&c)
	if c.Image != "https://assets.tcgdex.net/en/swsh/swsh12.5/157" {
		t.Errorf("image %q", c.Image)
	}
	special := Card{Name: "Rocky Fighting Energy", Category: "Energy", EnergyType: "Special"}
	energyArt(&special)
	if special.Image != "" {
		t.Errorf("special energy got art %q", special.Image)
	}
}

// TestByNameMoves picks among same-named cards by the moves seen: the
// newest Chi-Yu is a Darkness Pokémon with Whirling Envy, but one seen
// using Ground Melter is the older Fire Chi-Yu.
func TestByNameMoves(t *testing.T) {
	dir := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		p := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("sets.json", `[{"id":"sv06"},{"id":"me05"}]`)
	write("sets/sv06.json", `{"id":"sv06","releaseDate":"2024-05-24","serie":{"id":"sv"},"cards":[{"id":"sv06-039","localId":"039","name":"Chi-Yu","image":"a"}]}`)
	write("sets/me05.json", `{"id":"me05","releaseDate":"2026-09-01","serie":{"id":"me"},"cards":[{"id":"me05-059","localId":"059","name":"Chi-Yu","image":"b"}]}`)
	write("cards/sv06-039.json", `{"id":"sv06-039","name":"Chi-Yu","attacks":[{"name":"Allure"},{"name":"Ground Melter"}]}`)
	write("cards/me05-059.json", `{"id":"me05-059","name":"Chi-Yu","attacks":[{"name":"Whirling Envy"}]}`)
	c := NewClient(dir)
	c.Offline = true
	ctx := context.Background()
	for _, tc := range []struct {
		moves []string
		want  string
	}{
		{nil, "me05-059"},                                 // no moves: the newest print
		{[]string{"Ground Melter"}, "sv06-039"},           // only the Fire Chi-Yu has it
		{[]string{"Whirling Envy"}, "me05-059"},           // the newest print has it
		{[]string{"Borrowed Attack"}, "me05-059"},         // no print has it: the newest
		{[]string{"Allure", "Whirling Envy"}, "me05-059"}, // a tie keeps the preferred print
	} {
		got, err := c.ByName(ctx, "Chi-Yu", tc.moves...)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("Chi-Yu using %v: %s, want %s", tc.moves, got, tc.want)
		}
	}
}
