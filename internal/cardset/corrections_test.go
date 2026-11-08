package cardset

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func corr(card, path, value string) Correction {
	return Correction{Card: card, Path: path, Value: json.RawMessage(value), Reason: "printed card"}
}

func TestCorrectionsChangeAnyField(t *testing.T) {
	tests := []struct {
		name  string
		c     Correction
		check func(t *testing.T, s *Set)
	}{
		{"English effect text", corr("tst-010", "/text/en/effect", `"You cannot play this card by itself."`), func(t *testing.T, s *Set) {
			if p, _ := s.Print("tst-010"); p.Text["en"].Effect != "You cannot play this card by itself." {
				t.Errorf("effect %q", p.Text["en"].Effect)
			}
		}},
		{"absent field created", corr("tst-001", "/text/en/evolveFrom", `"Egg"`), func(t *testing.T, s *Set) {
			if p, _ := s.Print("tst-001"); p.Text["en"].EvolveFrom != "Egg" {
				t.Errorf("evolveFrom %q", p.Text["en"].EvolveFrom)
			}
		}},
		{"number", corr("tst-002", "/hp", `140`), func(t *testing.T, s *Set) {
			if p, _ := s.Print("tst-002"); p.HP != 140 {
				t.Errorf("hp %d", p.HP)
			}
		}},
		{"list", corr("tst-002", "/types", `["Fighting","Metal"]`), func(t *testing.T, s *Set) {
			if p, _ := s.Print("tst-002"); !reflect.DeepEqual(p.Types, []string{"Fighting", "Metal"}) {
				t.Errorf("types %v", p.Types)
			}
		}},
		{"list element", corr("tst-002", "/attacks/0/damage", `"120+"`), func(t *testing.T, s *Set) {
			if p, _ := s.Print("tst-002"); p.Attacks[0].Damage != "120+" {
				t.Errorf("damage %q", p.Attacks[0].Damage)
			}
		}},
		{"image sources", corr("tst-TG01", "/images", `[{"source":"tcgdex","url":"https://assets.tcgdex.net/en/me/tst/TG01"}]`), func(t *testing.T, s *Set) {
			if p, _ := s.Print("tst-TG01"); !reflect.DeepEqual(p.Images, []Image{{Source: "tcgdex", URL: "https://assets.tcgdex.net/en/me/tst/TG01"}}) {
				t.Errorf("images %v", p.Images)
			}
		}},
		{"append to a list", corr("tst-010", "/images/-", `{"source":"malie","url":"https://malie.example/10.jpg"}`), func(t *testing.T, s *Set) {
			if p, _ := s.Print("tst-010"); len(p.Images) != 2 || p.Images[1].Source != "malie" {
				t.Errorf("images %v", p.Images)
			}
		}},
		{"absent pair mark created", corr("tst-010", "/playedAsPair", `true`), func(t *testing.T, s *Set) {
			if p, _ := s.Print("tst-010"); !p.PlayedAsPair {
				t.Error("playedAsPair not set")
			}
		}},
		{"escaped pointer token", corr("tst-001", "/text/a~1b~0c", `{"name":"Riolu"}`), func(t *testing.T, s *Set) {
			if p, _ := s.Print("tst-001"); p.Text["a/b~c"].Name != "Riolu" {
				t.Errorf("text %v, want a block under a/b~c", p.Text)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testSet()
			_, err := applyCorrections(s, []Correction{tt.c})
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, s)
			if err := s.ValidateShape(); err != nil && tt.name != "escaped pointer token" {
				t.Errorf("corrected set invalid: %v", err)
			}
		})
	}
}

func TestCorrectionsRefuseUnknownCardOrField(t *testing.T) {
	tests := []struct {
		c    Correction
		want []string
	}{
		{corr("tst-999", "/hp", `10`), []string{"tst-999", "no such card"}},
		{corr("tst-001", "/pairMark", `true`), []string{"tst-001", "/pairMark", "pairMark"}},
		{corr("tst-001", "/text/en/flavour", `"x"`), []string{"tst-001", "/text/en/flavour", "flavour"}},
		{corr("tst-001", "/attacks/5/damage", `"10"`), []string{"tst-001", "/attacks/5/damage", "no element"}},
		{corr("tst-001", "/hp", `"lots"`), []string{"tst-001", "/hp"}},
		{corr("tst-001", "hp", `1`), []string{"tst-001", "JSON Pointer"}},
		{corr("tst-001", "/id", `"tst-777"`), []string{"tst-001", "id"}},
	}
	for _, tt := range tests {
		s := testSet()
		before, _ := Marshal(s)
		_, err := applyCorrections(s, []Correction{tt.c})
		if err == nil {
			t.Errorf("%s %s: no error", tt.c.Card, tt.c.Path)
			continue
		}
		for _, w := range tt.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s %s: error %q lacks %q", tt.c.Card, tt.c.Path, err, w)
			}
		}
		if after, _ := Marshal(s); !bytes.Equal(before, after) {
			t.Errorf("%s %s: a refused correction changed the set", tt.c.Card, tt.c.Path)
		}
	}
}

func TestCorrectionsRedundancy(t *testing.T) {
	s := testSet()
	got, err := applyCorrections(s, []Correction{
		corr("tst-002", "/hp", `130`),                 // the sources already say 130
		corr("tst-002", "/text/en/name", `"Lucario"`), // same text
		corr("tst-010", "/text/en/effect", `"You can use this card only if you discard 2 other cards from your hand.  Search your deck for a Pokémon & reveal it."`), // spacing only
		corr("tst-002", "/retreat", `3`),                                    // a real change
		corr("tst-001", "/text/en/evolveFrom", `"Egg"`),                     // field was absent
		corr("tst-002", "/weaknesses", `[{"type":"Psychic","value":"×2"}]`), // same list
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []bool{true, true, true, false, false, true}
	for i, a := range got {
		if a.Redundant != want[i] {
			t.Errorf("correction %d (%s): redundant %v, want %v (before %v)", i, a.Path, a.Redundant, want[i], a.Before)
		}
	}
	if got[3].Before == nil || got[3].Before.(json.Number) != "2" {
		t.Errorf("before = %v, want the merged value 2", got[3].Before)
	}
}

func TestReadCorrections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tst.json")
	body := `{
  "corrections": [
    {"card": "tst-010", "path": "/text/en/effect", "value": "Fixed.", "reason": "Printed card says so"}
  ],
  "acknowledged": [
    {"card": "tst-TG01", "kind": "no-image", "reason": "No source has art yet"},
    {"card": "tst-002", "kind": "disagreement", "field": "/hp", "reason": "TCGdex is right"}
  ]
}
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := ReadCorrections(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Corrections) != 1 || string(c.Corrections[0].Value) != `"Fixed."` || len(c.Acknowledged) != 2 || c.Acknowledged[1].Field != "/hp" {
		t.Errorf("read %+v", c)
	}
	// Applying corrections never changes the file.
	s := testSet()
	if _, err := applyCorrections(s, c.Corrections); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); string(after) != body {
		t.Error("applying corrections changed the corrections file")
	}
	if p, _ := s.Print("tst-010"); p.Text["en"].Effect != "Fixed." {
		t.Errorf("effect %q", p.Text["en"].Effect)
	}

	// A missing file is no corrections.
	if c, err := ReadCorrections(filepath.Join(dir, "none.json")); err != nil || len(c.Corrections)+len(c.Acknowledged) != 0 {
		t.Errorf("missing file: %+v, %v", c, err)
	}
	// Unknown fields and entries without a reason are refused.
	for name, bad := range map[string]string{
		"unknown field":          `{"corrections":[{"card":"a","path":"/hp","value":1,"reason":"r","why":"x"}]}`,
		"correction no reason":   `{"corrections":[{"card":"a","path":"/hp","value":1}]}`,
		"correction no value":    `{"corrections":[{"card":"a","path":"/hp","reason":"r"}]}`,
		"acknowledged no reason": `{"acknowledged":[{"card":"a","kind":"no-image"}]}`,
		"acknowledged no kind":   `{"acknowledged":[{"card":"a","reason":"r"}]}`,
	} {
		p := filepath.Join(dir, "bad.json")
		os.WriteFile(p, []byte(bad), 0o644)
		if _, err := ReadCorrections(p); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// An import applies the set's corrections to the built set on disk, for
// text, numbers, lists and image sources, and leaves the corrections file
// as it was; a correction to an unknown card or field fails the import.
func TestImportAppliesCorrections(t *testing.T) {
	f := newFakeSources(t)
	dir := t.TempDir()
	cpath := filepath.Join(dir, "corrections", "tst.json")
	body := `{
  "corrections": [
    {"card": "tst-002", "path": "/text/en/effect", "value": "You cannot play this card by itself.", "reason": "Rule printed on the card"},
    {"card": "tst-001", "path": "/hp", "value": 80, "reason": "Printed HP is 80"},
    {"card": "tst-001", "path": "/types", "value": ["Fighting", "Metal"], "reason": "Printed types"},
    {"card": "tst-003", "path": "/images", "value": [{"source": "tcgdex", "url": "https://assets.tcgdex.net/en/me/tst/003"}], "reason": "Art found"},
    {"card": "tst-002", "path": "/playedAsPair", "value": true, "reason": "Rule printed on the card"}
  ],
  "acknowledged": []
}
`
	os.MkdirAll(filepath.Dir(cpath), 0o755)
	if err := os.WriteFile(cpath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 { // and again: every import applies them
		if _, err := Import(context.Background(), f.opts(dir, "tst")); err != nil {
			t.Fatal(err)
		}
		s, err := Read(filepath.Join(dir, "sets", "tst.json"))
		if err != nil {
			t.Fatal(err)
		}
		p2, _ := s.Print("tst-002")
		p1, _ := s.Print("tst-001")
		p3, _ := s.Print("tst-003")
		if p2.Text["en"].Effect != "You cannot play this card by itself." || !p2.PlayedAsPair || p1.PlayedAsPair || p1.HP != 80 ||
			!reflect.DeepEqual(p1.Types, []string{"Fighting", "Metal"}) ||
			!reflect.DeepEqual(p3.Images, []Image{{Source: "tcgdex", URL: "https://assets.tcgdex.net/en/me/tst/003"}}) {
			t.Errorf("corrections not in the built set: %+v %+v %+v", p1, p2, p3)
		}
		if after, _ := os.ReadFile(cpath); string(after) != body {
			t.Fatal("the import changed the corrections file")
		}
	}

	os.WriteFile(cpath, []byte(`{"corrections":[{"card":"tst-001","path":"/flavour","value":"x","reason":"r"}]}`), 0o644)
	if _, err := Import(context.Background(), f.opts(dir, "tst")); err == nil || !strings.Contains(err.Error(), "tst-001") || !strings.Contains(err.Error(), "/flavour") {
		t.Errorf("import with a bad correction = %v, want an error naming card and path", err)
	}
}
