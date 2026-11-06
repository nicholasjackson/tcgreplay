package cards

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLogCodeID(t *testing.T) {
	for code, want := range map[string]string{
		"me1_86":       "me01-086",
		"me3_121":      "me03-121",
		"me2-5_172":    "me02.5-172",
		"me2_85_ph":    "me02-085",
		"mebsp_33":     "mep-033",
		"mee_14":       "mee-014",
		"sv5_154":      "sv05-154",
		"sv10_226":     "sv10-226",
		"zsv10-5_79":   "sv10.5b-079",
		"rsv10-5_1":    "sv10.5w-001",
		"swsh1_213":    "swsh1-213",
		"swsh12-5_157": "swsh12.5-157",
	} {
		if got, ok := LogCodeID(code); !ok || got != want {
			t.Errorf("%s: got %q, want %q", code, got, want)
		}
	}
	if _, ok := LogCodeID("Mega Absol ex"); ok {
		t.Error("a name is not a code")
	}
}

// TestLogCodesCached checks every code in a real log against the cache.
func TestLogCodesCached(t *testing.T) {
	dir := DefaultCacheDir()
	if _, err := os.Stat(filepath.Join(dir, "sets.json")); err != nil {
		t.Skip("no TCGdex cache")
	}
	c := NewClient(dir)
	c.Offline = true
	ctx := context.Background()
	for code, name := range map[string]string{
		"me1_86": "Mega Absol ex", "me3_121": "Meowth ex", "me2-5_172": "Mega Audino ex",
		"me2_85_ph": "Battle Cage", "mebsp_33": "Mega Lucario ex", "mee_15": "Basic Darkness Energy",
		"sv6_95": "Munkidori", "sv8_76": "Latias ex", "sv8_185": "Precious Trolley",
		"sv10_226": "Team Rocket's Petrel", "zsv10-5_79": "Air Balloon", "sv5_154": "Maximum Belt",
	} {
		id, _ := LogCodeID(code)
		s, err := c.set(ctx, id[:strings.LastIndex(id, "-")])
		if err != nil {
			t.Errorf("%s -> %s: %v", code, id, err)
			continue
		}
		i := slices.IndexFunc(s.Cards, func(x struct {
			ID      string `json:"id"`
			LocalID string `json:"localId"`
			Name    string `json:"name"`
			Image   string `json:"image"`
		}) bool {
			return x.ID == id
		})
		if i < 0 {
			t.Errorf("%s -> %s: no such card", code, id)
			continue
		}
		if got := s.Cards[i].Name; !SameName(got, name) && !SameName("Basic "+got, name) {
			t.Errorf("%s -> %s is %s, want %s", code, id, got, name)
		}
	}
}

func TestResolve30th(t *testing.T) {
	dir := DefaultCacheDir()
	if _, err := os.Stat(filepath.Join(dir, "sets", "30th.json")); err != nil {
		t.Skip("no TCGdex cache")
	}
	c := NewClient(dir)
	c.Offline = true
	for _, code := range []string{"30th", "30TH", "M30"} {
		if id, err := c.Resolve(context.Background(), code, "66"); err != nil || id != "30th-066" {
			t.Errorf("%s 66: got %q, %v", code, id, err)
		}
	}
}
