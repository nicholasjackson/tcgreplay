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

func loadReport(t *testing.T, dir, id string) Report {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "reports", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var r Report
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// item finds a report item by card and kind.
func item(r Report, card, kind string) (ReportItem, bool) {
	for _, it := range r.Items {
		if it.Card == card && it.Kind == kind {
			return it, true
		}
	}
	return ReportItem{}, false
}

// A clean set raises nothing.
func TestReportCleanSet(t *testing.T) {
	r := buildReport(reportInput{set: testSet(), cmp: comparison{}})
	// testSet's Trainer and special Energy have effects, its Stage 1 has
	// evolves-from, its Pokémon have attacks; only image gaps remain.
	s := testSet()
	for i := range s.Prints {
		s.Prints[i].Images = []Image{{Source: "tcgdex", URL: "u"}}
	}
	r = buildReport(reportInput{set: s})
	if len(r.Items) != 0 {
		t.Errorf("clean set raised %+v", r.Items)
	}
	if b, _ := json.Marshal(r); !bytes.Contains(b, []byte(`"items":[]`)) {
		t.Errorf("clean report %s, want an empty items list", b)
	}
}

// Every kind of gap names its card.
func TestReportEveryKind(t *testing.T) {
	s := testSet()
	for i := range s.Prints {
		s.Prints[i].Images = []Image{{Source: "tcgdex", URL: "u"}}
	}
	trainer, _ := s.Print("tst-010")
	en := trainer.Text["en"]
	en.Effect = ""
	trainer.Text["en"] = en // missing-effect
	lucario, _ := s.Print("tst-002")
	len2 := lucario.Text["en"]
	len2.EvolveFrom = ""
	lucario.Text["en"] = len2 // missing-evolves-from
	riolu, _ := s.Print("tst-001")
	riolu.Attacks = nil
	ren := riolu.Text["en"]
	ren.Attacks = nil
	riolu.Text["en"] = ren // no-attacks
	jet, _ := s.Print("tst-TG01")
	jet.Images = nil // no-image
	r := buildReport(reportInput{
		set:           s,
		disagreements: map[string][]disagreement{"tst-002": {{Field: "/hp", Values: map[string]any{"tcgdex": 130, "malie": 140}}}},
		applied:       []applied{{Correction: corr("tst-001", "/hp", "70"), Redundant: true}},
		cmp:           comparison{changed: []string{"tst-002"}, gone: []string{"tst-TG01"}},
	})
	for _, want := range []struct{ card, kind, field string }{
		{"tst-010", KindMissingEffect, "/text/en/effect"},
		{"tst-002", KindMissingEvolvesFrom, "/text/en/evolveFrom"},
		{"tst-001", KindNoAttacks, "/attacks"},
		{"tst-TG01", KindNoImage, "/images"},
		{"tst-002", KindDisagreement, "/hp"},
		{"tst-002", KindTextChanged, "/text/en"},
		{"tst-TG01", KindPrintGone, ""},
		{"tst-001", KindCorrectionRedundant, "/hp"},
	} {
		it, ok := item(r, want.card, want.kind)
		if !ok {
			t.Errorf("no %s item for %s in %+v", want.kind, want.card, r.Items)
			continue
		}
		if it.Field != want.field || it.Status != StatusOpen || it.Name == "" {
			t.Errorf("%s %s = %+v, want field %q, open, with a name", want.card, want.kind, it, want.field)
		}
	}
	if d, _ := item(r, "tst-002", KindDisagreement); !reflect.DeepEqual(d.Values, map[string]any{"tcgdex": 130, "malie": 140}) {
		t.Errorf("disagreement values %v", d.Values)
	}
	if r.open() != 8 {
		t.Errorf("open = %d, want 8", r.open())
	}
	// Sorted by collector number, then kind, then field.
	var order []string
	for _, it := range r.Items {
		order = append(order, it.Card+" "+it.Kind)
	}
	want := []string{
		"tst-001 correction-redundant", "tst-001 no-attacks",
		"tst-002 disagreement", "tst-002 missing-evolves-from", "tst-002 text-changed",
		"tst-010 missing-effect",
		"tst-TG01 no-image", "tst-TG01 print-gone-upstream",
	}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order %v, want %v", order, want)
	}
}

func TestReportStatuses(t *testing.T) {
	s := testSet()
	r := buildReport(reportInput{
		set: s,
		disagreements: map[string][]disagreement{
			"tst-002": {
				{Field: "/hp", Values: map[string]any{"tcgdex": 130, "malie": 140}},
				{Field: "/attacks/0/damage", Values: map[string]any{"tcgdex": "130", "malie": "120"}},
				{Field: "/retreat", Values: map[string]any{"tcgdex": 2, "malie": 3}},
			},
		},
		applied: []applied{
			{Correction: corr("tst-002", "/hp", "140")},     // covers /hp exactly
			{Correction: corr("tst-002", "/attacks", "[]")}, // covers a field inside it
		},
		acks: []Ack{
			{Card: "tst-001", Kind: KindNoImage, Reason: "No art yet"},
			{Card: "tst-002", Kind: KindDisagreement, Field: "/retreat", Reason: "TCGdex is right"},
			{Card: "tst-010", Kind: KindDisagreement, Field: "/hp", Reason: "wrong card"}, // matches nothing
		},
	})
	for _, want := range []struct {
		card, kind, field, status, reason string
	}{
		{"tst-002", KindDisagreement, "/hp", StatusCorrected, ""},
		{"tst-002", KindDisagreement, "/attacks/0/damage", StatusCorrected, ""},
		{"tst-002", KindDisagreement, "/retreat", StatusAcknowledged, "TCGdex is right"},
		{"tst-001", KindNoImage, "/images", StatusAcknowledged, "No art yet"},
		{"tst-TG01", KindNoImage, "/images", StatusOpen, ""},
	} {
		found := false
		for _, it := range r.Items {
			if it.Card == want.card && it.Kind == want.kind && it.Field == want.field {
				found = true
				if it.Status != want.status || it.Reason != want.reason {
					t.Errorf("%s %s %s: status %q reason %q, want %q %q", want.card, want.kind, want.field, it.Status, it.Reason, want.status, want.reason)
				}
			}
		}
		if !found {
			t.Errorf("no item %s %s %s", want.card, want.kind, want.field)
		}
	}
	// Corrected and acknowledged items are not open: only the one
	// unacknowledged image gap remains.
	if r.open() != 1 {
		t.Errorf("open = %d, want 1", r.open())
	}
}

// An import writes the report beside the built set: the sources'
// disagreement, a correction that covers it, an acknowledged gap, and the
// same bytes again when nothing upstream changed.
func TestImportWritesReport(t *testing.T) {
	f := newFakeSources(t)
	// Malie disagrees with TCGdex on Riolu's HP.
	f.malie.put("/v1/tst1.en-US.json", `[
		{"name":"Riolu","card_type":"POKEMON","stage":"BASIC","hp":80,"types":["FIGHTING"],"retreat":1,
		 "text":[{"kind":"ATTACK","name":"Punch","text":"Discard a {F} Energy.","cost":["FIGHTING"],"damage":{"amount":30}}],
		 "collector_number":{"numeric":1},"ext":{"tcgl":{"cardID":"tst1_1","longFormID":"Riolu_tst1_1_std"}}}]`)
	dir := t.TempDir()
	ctx := context.Background()
	sum, err := Import(ctx, f.opts(dir, "tst"))
	if err != nil {
		t.Fatal(err)
	}
	r := loadReport(t, dir, "tst")
	if !reflect.DeepEqual(r.Summary, sum) || r.Set != "tst" {
		t.Errorf("report summary %+v, want %+v", r.Summary, sum)
	}
	// Riolu: HP disagreement and no image; Aarune: no effect anywhere.
	for _, k := range []struct{ card, kind string }{{"tst-001", KindDisagreement}, {"tst-001", KindNoImage}, {"tst-002", KindMissingEffect}} {
		if it, ok := item(r, k.card, k.kind); !ok || it.Status != StatusOpen {
			t.Errorf("%s %s: %+v, want open", k.card, k.kind, it)
		}
	}
	if sum.Open != 3 || len(r.Items) != 3 {
		t.Errorf("open %d, items %+v; want 3", sum.Open, r.Items)
	}

	// The maintainer corrects the HP and acknowledges the missing image.
	cpath := filepath.Join(dir, "corrections", "tst.json")
	body := []byte(`{
  "corrections": [{"card": "tst-001", "path": "/hp", "value": 80, "reason": "Printed HP is 80"}],
  "acknowledged": [{"card": "tst-001", "kind": "no-image", "reason": "No source has art yet"}]
}
`)
	os.MkdirAll(filepath.Dir(cpath), 0o755)
	os.WriteFile(cpath, body, 0o644)
	sum, err = Import(ctx, f.opts(dir, "tst"))
	if err != nil {
		t.Fatal(err)
	}
	r = loadReport(t, dir, "tst")
	if it, _ := item(r, "tst-001", KindDisagreement); it.Status != StatusCorrected {
		t.Errorf("HP disagreement %+v, want corrected", it)
	}
	if it, _ := item(r, "tst-001", KindNoImage); it.Status != StatusAcknowledged || it.Reason != "No source has art yet" {
		t.Errorf("image gap %+v, want acknowledged with its reason", it)
	}
	if sum.Open != 1 {
		t.Errorf("open %d, want 1 (Aarune's effect)", sum.Open)
	}
	s, _ := Read(filepath.Join(dir, "sets", "tst.json"))
	if p, _ := s.Print("tst-001"); p.HP != 80 {
		t.Errorf("HP %d, want the corrected 80", p.HP)
	}
	if after, _ := os.ReadFile(cpath); !bytes.Equal(after, body) {
		t.Error("the import changed the corrections file")
	}

	// Unchanged sources: identical report bytes.
	rpath := filepath.Join(dir, "reports", "tst.json")
	first, _ := os.ReadFile(rpath)
	if _, err := Import(ctx, f.opts(dir, "tst")); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(rpath)
	if !bytes.Equal(first, second) {
		t.Errorf("report changed on an unchanged re-import:\n%s\n%s", first, second)
	}

	// The sources now agree with the correction: it is redundant.
	f.tcgdex.put("/cards/tst-001", `{"id":"tst-001","localId":"001","name":"Riolu","category":"Pokemon","hp":80,"types":["Fighting"],"stage":"Basic",
		"attacks":[{"cost":["Fighting"],"name":"Punch","effect":"Discard a Fighting Energy.","damage":30}],"retreat":1}`)
	if _, err := Import(ctx, f.opts(dir, "tst")); err != nil {
		t.Fatal(err)
	}
	r = loadReport(t, dir, "tst")
	if it, ok := item(r, "tst-001", KindCorrectionRedundant); !ok || it.Field != "/hp" || it.Status != StatusOpen {
		t.Errorf("redundant correction %+v, want an open item on /hp", it)
	}
}

// An attack or ability without an English name is reported, not refused.
func TestReportMissingName(t *testing.T) {
	s := testSet()
	p, _ := s.Print("tst-001")
	p.Text["en"].Attacks[0].Name = ""
	p.Text["en"].Abilities[0].Name = ""
	if err := s.ValidateShape(); err != nil {
		t.Fatalf("an unnamed English move made the set malformed: %v", err)
	}
	r := buildReport(reportInput{set: s})
	var fields []string
	for _, it := range r.Items {
		if it.Card == "tst-001" && it.Kind == KindMissingName {
			fields = append(fields, it.Field)
		}
	}
	if !reflect.DeepEqual(fields, []string{"/text/en/abilities/0/name", "/text/en/attacks/0/name"}) {
		t.Errorf("missing-name fields %v", fields)
	}
}
