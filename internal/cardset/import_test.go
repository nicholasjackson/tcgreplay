package cardset

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// fakeSources serves a three-print set "tst" (decklist code TST) from a
// fake TCGdex and a fake malie export: a Basic Pokémon both sources list
// (TCGdex text with an energy span, no image or regulation mark), a
// Supporter TCGdex lists without effect text, and a print only malie lists.
// The set list also holds a Pocket set and an unrelated set.
type fakeSources struct {
	tcgdex, malie *fakeUpstream
}

func newFakeSources(t *testing.T) *fakeSources {
	t.Helper()
	td, ml := newFakeUpstream(t), newFakeUpstream(t)
	td.put("/sets", `[{"id":"tst","name":"Test Set"},{"id":"other","name":"Other"},{"id":"A1","name":"Genetic Apex"}]`)
	td.put("/sets/tst", `{"id":"tst","name":"Test <i>Set</i>","releaseDate":"2026-01-02","serie":{"id":"me","name":"Mega Evolution"},"abbreviation":{"official":"TST"},
		"cards":[{"id":"tst-001","localId":"001","name":"Riolu"},{"id":"tst-002","localId":"002","name":"Aarune"}]}`)
	td.put("/sets/other", `{"id":"other","name":"Other","serie":{"id":"sv"},"abbreviation":{"official":"OTH"},"cards":[]}`)
	td.put("/sets/A1", `{"id":"A1","name":"Genetic Apex","serie":{"id":"tcgp"},"cards":[]}`)
	td.put("/cards/tst-001", `{"id":"tst-001","localId":"001","name":"Riolu","category":"Pokemon","hp":70,"types":["Fighting"],"stage":"Basic",
		"attacks":[{"cost":["Fighting"],"name":"Punch","effect":"Discard a <span class=\"energy-symbol Fighting\" title=\"Fighting\">Fighting</span> Energy.","damage":30}],
		"weaknesses":[{"type":"Psychic","value":"×2"}],"retreat":1,"pricing":{"avg":1},"updated":"2026-11-08T00:00:00Z","rarity":"Common"}`)
	td.put("/cards/tst-002", `{"id":"tst-002","localId":"002","name":"Aarune","category":"Trainer","trainerType":"Supporter","image":"https://assets.tcgdex.net/en/me/tst/002"}`)
	ml.put("/index.json", `{"en-US":{"tst1":{"path":"v1/tst1.en-US.json","name":"<i>Test Set</i>","num":5,"abbr":"TST"}}}`)
	ml.put("/v1/tst1.en-US.json", `[
		{"name":"Riolu","card_type":"POKEMON","stage":"BASIC","hp":70,"types":["FIGHTING"],"retreat":1,"regulation_mark":"J",
		 "weakness":{"types":["PSYCHIC"],"operator":"×","amount":2},
		 "text":[{"kind":"ATTACK","name":"Punch","text":"Discard a {F} Energy.","cost":["FIGHTING"],"damage":{"amount":30}}],
		 "collector_number":{"numeric":1},"ext":{"tcgl":{"cardID":"tst1_1","longFormID":"Riolu_tst1_1_std_Common"}},
		 "images":{"tcgl":{"jpg":{"front":"https://malie.example/tst_001.jpg"}}}},
		{"name":"Riolu","card_type":"POKEMON","stage":"BASIC","hp":70,"collector_number":{"numeric":1},"ext":{"tcgl":{"cardID":"tst1_1_ph","longFormID":"Riolu_tst1_1_ph_Common"}}},
		{"name":"Aarune","card_type":"TRAINER","subtype":"SUPPORTER","regulation_mark":"J",
		 "text":[{"kind":"TEXT_BOX","text":"Search your deck for up to 3 cards."},{"kind":"REMINDER","text":"You may play only 1 Supporter card during your turn."}],
		 "collector_number":{"numeric":2},"ext":{"tcgl":{"cardID":"tst1_2","longFormID":"Aarune_tst1_2_std_Uncommon"}},
		 "images":{"tcgl":{"jpg":{"front":"https://malie.example/tst_002.jpg"}}}},
		{"name":"Lucario","card_type":"POKEMON","stage":"STAGE1","stage_text":"Evolves from Riolu","hp":130,"types":["FIGHTING"],"retreat":2,"regulation_mark":"J",
		 "text":[{"kind":"ATTACK","name":"Aura Sphere","cost":["FIGHTING","COLORLESS"],"damage":{"amount":130}}],
		 "collector_number":{"numeric":3},"ext":{"tcgl":{"cardID":"tst1_3","longFormID":"Lucario_tst1_3_std_Rare"}},
		 "images":{"tcgl":{"jpg":{"front":"https://malie.example/tst_003.jpg"}}}}
	]`)
	return &fakeSources{tcgdex: td, malie: ml}
}

func (f *fakeSources) opts(dir, set string) ImportOptions {
	return ImportOptions{Set: set, Dir: dir, TCGdex: &TCGdex{Base: f.tcgdex.URL, get: fastGetter}, Malie: &Malie{Base: f.malie.URL, get: fastGetter}}
}

func TestImportWritesBuiltSet(t *testing.T) {
	f := newFakeSources(t)
	for _, name := range []string{"tst", "TST"} { // by id, and by decklist code
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			sum, err := Import(context.Background(), f.opts(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if sum.Set != "tst" || sum.Prints != 3 {
				t.Errorf("summary %+v, want tst with 3 prints", sum)
			}
			got, err := Read(filepath.Join(dir, "sets", "tst.json"))
			if err != nil {
				t.Fatal(err)
			}
			want := &Set{Format: 1,
				Set: SetHeader{ID: "tst", Serie: "me", ReleaseDate: "2026-01-02", Abbreviation: "TST", Text: map[string]SetText{"en": {Name: "Test Set"}}},
				Prints: []Print{
					{ID: "tst-001", LocalID: "001", Category: "Pokemon", HP: 70, Types: []string{"Fighting"}, Stage: "Basic", Retreat: 1, RegulationMark: "J",
						Attacks:    []AttackCore{{Cost: []string{"Fighting"}, Damage: "30"}},
						Weaknesses: []cards.TypeValue{{Type: "Psychic", Value: "×2"}},
						Images:     []Image{{Source: "malie", URL: "https://malie.example/tst_001.jpg"}},
						Sources:    []string{"malie", "tcgdex"},
						Text:       map[string]PrintText{"en": {Name: "Riolu", Attacks: []MoveText{{Name: "Punch", Effect: "Discard a Fighting Energy."}}}}},
					{ID: "tst-002", LocalID: "002", Category: "Trainer", TrainerType: "Supporter", RegulationMark: "J",
						Images:  []Image{{Source: "tcgdex", URL: "https://assets.tcgdex.net/en/me/tst/002"}, {Source: "malie", URL: "https://malie.example/tst_002.jpg"}},
						Sources: []string{"malie", "tcgdex"},
						Text:    map[string]PrintText{"en": {Name: "Aarune", Effect: "Search your deck for up to 3 cards."}}},
					{ID: "tst-003", LocalID: "003", Category: "Pokemon", HP: 130, Types: []string{"Fighting"}, Stage: "Stage1", Retreat: 2, RegulationMark: "J",
						Attacks: []AttackCore{{Cost: []string{"Fighting", "Colorless"}, Damage: "130"}},
						Images:  []Image{{Source: "malie", URL: "https://malie.example/tst_003.jpg"}},
						Sources: []string{"malie"},
						Text:    map[string]PrintText{"en": {Name: "Lucario", EvolveFrom: "Riolu", Attacks: []MoveText{{Name: "Aura Sphere"}}}}},
				}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("built set\n got %+v\nwant %+v", got, want)
			}
			b, _ := os.ReadFile(filepath.Join(dir, "sets", "tst.json"))
			for _, churn := range []string{"pricing", "updated", "rarity", "<"} {
				if bytes.Contains(b, []byte(churn)) {
					t.Errorf("built set holds %q", churn)
				}
			}
		})
	}
}

func TestImportNoMalie(t *testing.T) {
	f := newFakeSources(t)
	dir := t.TempDir()
	o := f.opts(dir, "tst")
	o.NoMalie = true
	sum, err := Import(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Prints != 2 {
		t.Errorf("%d prints, want TCGdex's 2", sum.Prints)
	}
	if n := f.malie.hits("/index.json"); n != 0 {
		t.Errorf("malie was asked %d times with -no-malie", n)
	}
	got, err := Read(filepath.Join(dir, "sets", "tst.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got.Prints {
		if !reflect.DeepEqual(p.Sources, []string{"tcgdex"}) {
			t.Errorf("%s sources %v, want tcgdex only", p.ID, p.Sources)
		}
	}
	if aarune, _ := got.Print("tst-002"); aarune.Text["en"].Effect != "" {
		t.Errorf("Aarune effect %q, want TCGdex's empty effect", aarune.Text["en"].Effect)
	}
}

func TestImportIsDeterministic(t *testing.T) {
	f := newFakeSources(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "sets", "tst.json")
	if _, err := Import(context.Background(), f.opts(dir, "tst")); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	if _, err := Import(context.Background(), f.opts(dir, "tst")); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if !bytes.Equal(first, second) {
		t.Error("re-importing unchanged sources changed the built set's bytes")
	}
}

func TestImportRefusals(t *testing.T) {
	f := newFakeSources(t)
	for _, tt := range []struct{ set, want string }{
		{"A1", "Pocket"},
		{"nope", "no TCGdex set"},
		{"", "no set named"},
	} {
		if _, err := Import(context.Background(), f.opts(t.TempDir(), tt.set)); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("Import(%q) = %v, want an error containing %q", tt.set, err, tt.want)
		}
	}
	// A card the set lists but TCGdex cannot serve fails the import and
	// writes nothing.
	f.tcgdex.put("/sets/tst", `{"id":"tst","name":"Test","serie":{"id":"me"},"cards":[{"id":"tst-001","localId":"001"},{"id":"tst-404","localId":"404"}]}`)
	dir := t.TempDir()
	if _, err := Import(context.Background(), f.opts(dir, "tst")); err == nil || !strings.Contains(err.Error(), "tst-404") {
		t.Errorf("Import with a missing card = %v, want an error naming it", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sets", "tst.json")); !os.IsNotExist(err) {
		t.Error("a failed import wrote a built set")
	}
}

// An all-sets import builds every TCGdex set except Pocket ones, carries on
// past a set that fails, and lists the malie sets no TCGdex set matches.
func TestImportAll(t *testing.T) {
	f := newFakeSources(t)
	f.tcgdex.put("/sets", `[{"id":"tst"},{"id":"other"},{"id":"A1"},{"id":"broken"}]`)
	f.tcgdex.put("/sets/other", `{"id":"other","name":"Other","serie":{"id":"sv"},"abbreviation":{"official":"OTH"},"cards":[{"id":"other-001","localId":"001"}]}`)
	f.tcgdex.put("/cards/other-001", `{"id":"other-001","localId":"001","name":"Pikachu","category":"Pokemon","hp":60,"stage":"Basic","attacks":[{"name":"Zap","damage":20}]}`)
	f.tcgdex.put("/sets/broken", `{"id":"broken","name":"Broken","serie":{"id":"sv"},"cards":[{"id":"broken-001","localId":"001"}]}`) // its card 404s
	f.malie.put("/index.json", `{"en-US":{
		"tst1":{"path":"v1/tst1.en-US.json","abbr":"TST","num":5},
		"svalt":{"path":"v1/svalt.en-US.json","abbr":"SVALT","num":237}}}`)
	dir := t.TempDir()
	o := f.opts(dir, "")
	res, err := ImportAll(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	var built []string
	for _, s := range res.Summaries {
		built = append(built, s.Set)
	}
	if !reflect.DeepEqual(built, []string{"other", "tst"}) {
		t.Errorf("built %v, want other and tst", built)
	}
	for _, id := range []string{"other", "tst"} {
		if _, err := Read(filepath.Join(dir, "sets", id+".json")); err != nil {
			t.Errorf("%s: %v", id, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "reports", id+".json")); err != nil {
			t.Errorf("%s report: %v", id, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "sets", "A1.json")); !os.IsNotExist(err) {
		t.Error("a Pokémon TCG Pocket set was built")
	}
	if !reflect.DeepEqual(res.Skipped, []string{"A1"}) {
		t.Errorf("skipped %v, want A1", res.Skipped)
	}
	if len(res.Failed) != 1 || res.Failed["broken"] == nil || !strings.Contains(res.Failed["broken"].Error(), "broken-001") {
		t.Errorf("failed %v, want broken naming its missing card", res.Failed)
	}
	if !reflect.DeepEqual(res.Unmatched, []MalieUnmatched{{Key: "svalt", Abbr: "SVALT", Entries: 237}}) {
		t.Errorf("unmatched %+v, want svalt", res.Unmatched)
	}
}
