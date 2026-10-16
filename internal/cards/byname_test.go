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
