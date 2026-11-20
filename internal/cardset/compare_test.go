package cardset

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func ids(ps []Print) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

func TestCompareFirstImport(t *testing.T) {
	c := compare(nil, testSet())
	if want := []string{"tst-010", "tst-002", "tst-001", "tst-TG01"}; !reflect.DeepEqual(c.added, want) {
		t.Errorf("added %v, want %v", c.added, want)
	}
	if len(c.changed)+len(c.gone) != 0 {
		t.Errorf("changed %v gone %v on a first import", c.changed, c.gone)
	}
}

func TestCompareAddsNewPrintsAndKeepsOld(t *testing.T) {
	prev := testSet()
	next := testSet()
	next.Prints = append(next.Prints, Print{ID: "tst-150", LocalID: "150", Category: "Trainer", TrainerType: "Item",
		Text: map[string]PrintText{"en": {Name: "Secret Ball", Effect: "Search your deck."}}})
	c := compare(prev, next)
	if want := []string{"tst-001", "tst-002", "tst-010", "tst-150", "tst-TG01"}; !reflect.DeepEqual(ids(c.set.Prints), want) {
		t.Errorf("prints %v, want %v", ids(c.set.Prints), want)
	}
	if !reflect.DeepEqual(c.added, []string{"tst-150"}) || len(c.changed) != 0 || len(c.gone) != 0 {
		t.Errorf("added %v changed %v gone %v, want only tst-150 added", c.added, c.changed, c.gone)
	}
}

func TestCompareTakesUpstreamChanges(t *testing.T) {
	prev := testSet()
	next := testSet()
	p, _ := next.Print("tst-010")
	en := p.Text["en"]
	en.Effect = "Search your deck for a Pokémon."
	p.Text["en"] = en
	q, _ := next.Print("tst-TG01")
	q.Images = []Image{{Source: "tcgdex", URL: "https://assets.tcgdex.net/en/me/tst/TG01"}}

	// A field covered by a correction keeps the corrected value: the
	// import applies corrections to the fresh build before comparing.
	r, _ := prev.Print("tst-002")
	ren := r.Text["en"]
	ren.Name = "Lucario (corrected)"
	r.Text["en"] = ren
	nr, _ := next.Print("tst-002")
	nen := nr.Text["en"]
	nen.Name = "Lucari0" // upstream changed it again
	nr.Text["en"] = nen
	if _, err := applyCorrections(next, []Correction{corr("tst-002", "/text/en/name", `"Lucario (corrected)"`)}); err != nil {
		t.Fatal(err)
	}

	c := compare(prev, next)
	got, _ := c.set.Print("tst-010")
	if got.Text["en"].Effect != "Search your deck for a Pokémon." {
		t.Errorf("effect %q, want the new upstream text", got.Text["en"].Effect)
	}
	gotq, _ := c.set.Print("tst-TG01")
	if len(gotq.Images) != 1 || gotq.Images[0].Source != "tcgdex" {
		t.Errorf("images %v, want the new upstream image", gotq.Images)
	}
	gotr, _ := c.set.Print("tst-002")
	if gotr.Text["en"].Name != "Lucario (corrected)" {
		t.Errorf("name %q, want the corrected value", gotr.Text["en"].Name)
	}
	if !reflect.DeepEqual(c.changed, []string{"tst-010"}) {
		t.Errorf("changed %v, want only tst-010 (text), not the image or the corrected field", c.changed)
	}
}

func TestCompareKeepsGonePrints(t *testing.T) {
	prev := testSet()
	next := testSet()
	next.Prints = next.Prints[1:] // tst-010 vanished upstream
	c := compare(prev, next)
	got, ok := c.set.Print("tst-010")
	if !ok {
		t.Fatal("vanished print was dropped")
	}
	want, _ := testSet().Print("tst-010")
	if !reflect.DeepEqual(*got, *want) {
		t.Errorf("vanished print changed:\n got %+v\nwant %+v", *got, *want)
	}
	if !reflect.DeepEqual(c.gone, []string{"tst-010"}) {
		t.Errorf("gone %v, want tst-010", c.gone)
	}
}

func TestCompareCarriesOtherLanguages(t *testing.T) {
	prev := testSet()
	p, _ := prev.Print("tst-002")
	p.Text["fr"] = PrintText{Name: "Lucario", EvolveFrom: "Riolu", Attacks: []MoveText{{Name: "Aura Sphère"}}}
	prev.Set.Text["fr"] = SetText{Name: "Jeu d'essai"}
	enBefore, _ := json.Marshal(p.Text["en"])

	next := testSet() // a re-import writes English only
	c := compare(prev, next)
	got, _ := c.set.Print("tst-002")
	if !reflect.DeepEqual(got.Text["fr"], p.Text["fr"]) {
		t.Errorf("French text %+v, want it carried over", got.Text["fr"])
	}
	if enAfter, _ := json.Marshal(got.Text["en"]); !bytes.Equal(enBefore, enAfter) {
		t.Errorf("English text changed:\n%s\n%s", enBefore, enAfter)
	}
	if c.set.Set.Text["fr"].Name != "Jeu d'essai" {
		t.Errorf("set text %v, want French name carried over", c.set.Set.Text)
	}
	if len(c.changed) != 0 {
		t.Errorf("changed %v: carrying French over is not an English change", c.changed)
	}
	if err := c.set.ValidateShape(); err != nil {
		t.Errorf("result invalid: %v", err)
	}
}

// Re-importing after upstream changes: a new print is added and every
// earlier one kept, changed text and art replace the old unless a
// correction covers the field, a print gone upstream stays, and French
// wording added by hand survives with the English unchanged.
func TestReimportPicksUpUpstreamChanges(t *testing.T) {
	f := newFakeSources(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "sets", "tst.json")
	ctx := context.Background()
	sum, err := Import(ctx, f.opts(dir, "tst"))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Added != 3 || sum.Changed != 0 || sum.Gone != 0 {
		t.Errorf("first import summary %+v, want 3 added", sum)
	}

	// The maintainer adds French wording to Riolu and corrects its HP.
	s, _ := Read(path)
	p, _ := s.Print("tst-001")
	p.Text["fr"] = PrintText{Name: "Riolu", Attacks: []MoveText{{Name: "Coup de poing"}}}
	if err := Write(path, s); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "corrections"), 0o755)
	os.WriteFile(filepath.Join(dir, "corrections", "tst.json"), []byte(`{"corrections":[{"card":"tst-001","path":"/text/en/attacks/0/effect","value":"Discard a Fighting Energy from this Pokémon.","reason":"printed card"}]}`), 0o644)
	if _, err := Import(ctx, f.opts(dir, "tst")); err != nil {
		t.Fatal(err)
	}
	s, _ = Read(path)
	p, _ = s.Print("tst-001")
	enBefore, _ := json.Marshal(p.Text["en"])

	// Upstream: TCGdex gains a print and art for the Supporter, changes
	// Riolu's attack text (covered by the correction) and the Supporter's
	// name; malie drops Lucario.
	f.tcgdex.put("/sets/tst", `{"id":"tst","name":"Test Set","releaseDate":"2026-01-02","serie":{"id":"me"},"abbreviation":{"official":"TST"},
		"cards":[{"id":"tst-001","localId":"001"},{"id":"tst-002","localId":"002"},{"id":"tst-004","localId":"004"}]}`)
	f.tcgdex.put("/cards/tst-001", `{"id":"tst-001","localId":"001","name":"Riolu","category":"Pokemon","hp":70,"types":["Fighting"],"stage":"Basic",
		"attacks":[{"cost":["Fighting"],"name":"Punch","effect":"Discard all Energy.","damage":30}],"weaknesses":[{"type":"Psychic","value":"×2"}],"retreat":1}`)
	f.tcgdex.put("/cards/tst-002", `{"id":"tst-002","localId":"002","name":"Aarune's Help","category":"Trainer","trainerType":"Supporter","image":"https://assets.tcgdex.net/en/me/tst/002-new"}`)
	f.tcgdex.put("/cards/tst-004", `{"id":"tst-004","localId":"004","name":"Secret Ball","category":"Trainer","trainerType":"Item","effect":"Search your deck."}`)
	f.malie.put("/v1/tst1.en-US.json", `[]`)

	sum, err = Import(ctx, f.opts(dir, "tst"))
	if err != nil {
		t.Fatal(err)
	}
	s, err = Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"tst-001", "tst-002", "tst-003", "tst-004"}; !reflect.DeepEqual(ids(s.Prints), want) {
		t.Errorf("prints %v, want %v", ids(s.Prints), want)
	}
	aarune, _ := s.Print("tst-002")
	if aarune.Text["en"].Name != "Aarune's Help" || len(aarune.Images) != 1 || aarune.Images[0].URL != "https://assets.tcgdex.net/en/me/tst/002-new" {
		t.Errorf("Supporter %+v, want the new name and art", aarune)
	}
	riolu, _ := s.Print("tst-001")
	if got := riolu.Text["en"].Attacks[0].Effect; got != "Discard a Fighting Energy from this Pokémon." {
		t.Errorf("Riolu effect %q, want the corrected value", got)
	}
	if !reflect.DeepEqual(riolu.Text["fr"], PrintText{Name: "Riolu", Attacks: []MoveText{{Name: "Coup de poing"}}}) {
		t.Errorf("French text %+v, want it kept", riolu.Text["fr"])
	}
	if enAfter, _ := json.Marshal(riolu.Text["en"]); !bytes.Equal(enBefore, enAfter) {
		t.Errorf("Riolu English changed:\n%s\n%s", enBefore, enAfter)
	}
	lucario, ok := s.Print("tst-003")
	if !ok || lucario.Text["en"].Name != "Lucario" {
		t.Errorf("Lucario, gone upstream, was not kept: %+v", lucario)
	}
	if sum.Added != 1 || sum.Changed != 1 || sum.Gone != 1 || sum.Prints != 4 {
		t.Errorf("summary %+v, want 4 prints, 1 added, 1 changed, 1 gone", sum)
	}
}
