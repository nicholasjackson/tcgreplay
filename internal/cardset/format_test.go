package cardset

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// testSet returns a small built set: a Basic, a Stage 1, an Item and a
// special Energy, deliberately out of collector-number order.
func testSet() *Set {
	return &Set{
		Format: Format,
		Set: SetHeader{
			ID: "tst", Serie: "me", ReleaseDate: "2026-01-01", Abbreviation: "TST",
			Text: map[string]SetText{"en": {Name: "Test Set"}},
		},
		Prints: []Print{
			{
				ID: "tst-010", LocalID: "010", Category: "Trainer", TrainerType: "Item", RegulationMark: "J",
				Images:  []Image{{Source: "tcgdex", URL: "https://assets.tcgdex.net/en/me/tst/010"}},
				Sources: []string{"malie", "tcgdex"},
				Text:    map[string]PrintText{"en": {Name: "Ultra Ball", Effect: "You can use this card only if you discard 2 other cards from your hand. Search your deck for a Pokémon & reveal it."}},
			},
			{
				ID: "tst-002", LocalID: "002", Category: "Pokemon", HP: 130, Types: []string{"Fighting"}, Stage: "Stage1", Retreat: 2,
				Attacks:    []AttackCore{{Cost: []string{"Fighting", "Colorless"}, Damage: "130"}},
				Weaknesses: []cards.TypeValue{{Type: "Psychic", Value: "×2"}},
				Images:     []Image{{Source: "malie", URL: "https://malie.example/2.jpg"}, {Source: "tcgdex", URL: "https://assets.tcgdex.net/en/me/tst/002"}},
				Text: map[string]PrintText{"en": {
					Name: "Lucario", EvolveFrom: "Riolu",
					Attacks: []MoveText{{Name: "Aura Sphere", Effect: "This attack also does 20 damage to 1 of your opponent's Benched Pokémon."}},
				}},
			},
			{
				ID: "tst-001", LocalID: "001", Category: "Pokemon", HP: 70, Types: []string{"Fighting"}, Stage: "Basic", Retreat: 1,
				Attacks:     []AttackCore{{Cost: []string{"Fighting"}, Damage: "10+"}},
				Abilities:   []AbilityCore{{Type: "Ability"}},
				Resistances: []cards.TypeValue{{Type: "Metal", Value: "-30"}},
				Text: map[string]PrintText{"en": {
					Name:      "Riolu",
					Attacks:   []MoveText{{Name: "Punch"}},
					Abilities: []MoveText{{Name: "Focus", Effect: "Once during your turn, you may draw 2 cards."}},
				}},
			},
			{
				ID: "tst-TG01", LocalID: "TG01", Category: "Energy", EnergyType: "Special",
				Text: map[string]PrintText{"en": {Name: "Jet Energy", Effect: "As long as this card is attached to a Pokémon, it provides {C} Energy."}},
			},
		},
	}
}

func writeTemp(t *testing.T, s *Set) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sets", s.Set.ID+".json")
	if err := Write(path, s); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWriteReadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sets", "tst.json")
	if err := Write(path, testSet()); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	want := testSet()
	SortPrints(want.Prints)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the set:\n got %+v\nwant %+v", got, want)
	}
	var ids []string
	for _, p := range got.Prints {
		ids = append(ids, p.ID)
	}
	if strings.Join(ids, ",") != "tst-001,tst-002,tst-010,tst-TG01" {
		t.Errorf("prints in order %v, want by collector number with lettered last", ids)
	}
}

func TestWriteIsDeterministic(t *testing.T) {
	a := writeTemp(t, testSet())
	b := writeTemp(t, testSet())
	if !bytes.Equal(a, b) {
		t.Fatal("writing the same set twice gave different bytes")
	}
	if !bytes.HasSuffix(a, []byte("}\n")) {
		t.Error("file does not end with a newline")
	}
	if !bytes.Contains(a, []byte("\n  \"prints\": [")) {
		t.Error("file is not two-space indented")
	}
	for _, lit := range []string{"×2", "Pokémon & reveal"} {
		if !bytes.Contains(a, []byte(lit)) {
			t.Errorf("file does not keep %q literal", lit)
		}
	}
}

// The core facts are stored once per print, outside the wording.
func TestCoreKeptApartFromWording(t *testing.T) {
	b := writeTemp(t, testSet())
	var raw struct {
		Prints []map[string]json.RawMessage `json:"prints"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	lucario := raw.Prints[1]
	for _, k := range []string{"hp", "types", "attacks", "retreat", "stage"} {
		if _, ok := lucario[k]; !ok {
			t.Errorf("print core lacks %q", k)
		}
	}
	if got := string(lucario["attacks"]); !strings.Contains(got, `"130"`) || !strings.Contains(got, `"Colorless"`) || strings.Contains(got, "Aura Sphere") {
		t.Errorf("core attacks hold wording or lack facts: %s", got)
	}
	var text map[string]map[string]json.RawMessage
	if err := json.Unmarshal(lucario["text"], &text); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"hp", "types", "retreat", "stage"} {
		if _, ok := text["en"][k]; ok {
			t.Errorf("English text holds core fact %q", k)
		}
	}
	if got := string(text["en"]["attacks"]); strings.Contains(got, "130") || strings.Contains(got, "cost") {
		t.Errorf("English attacks hold core facts: %s", got)
	}
}

func TestOnePrintChangeDiffsOnlyThatPrint(t *testing.T) {
	before := writeTemp(t, testSet())
	s := testSet()
	for i := range s.Prints {
		if s.Prints[i].ID == "tst-010" {
			en := s.Prints[i].Text["en"]
			en.Effect = "Search your deck for a Pokémon."
			s.Prints[i].Text["en"] = en
		}
	}
	after := writeTemp(t, s)
	bl, al := strings.Split(string(before), "\n"), strings.Split(string(after), "\n")
	if len(bl) != len(al) {
		t.Fatalf("line count changed %d -> %d", len(bl), len(al))
	}
	var changed []string
	for i := range bl {
		if bl[i] != al[i] {
			changed = append(changed, al[i])
		}
	}
	if len(changed) != 1 || !strings.Contains(changed[0], `"effect": "Search your deck for a Pokémon."`) {
		t.Errorf("changed lines %q, want only the Ultra Ball effect line", changed)
	}
}

func TestSecondLanguageLeavesEnglishAndCoreUnchanged(t *testing.T) {
	s := testSet()
	before := writeTemp(t, s)
	b, err := Decode(before)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := b.Print("tst-002")
	p.Text["fr"] = PrintText{
		Name: "Lucario", EvolveFrom: "Riolu",
		Attacks: []MoveText{{Name: "Aura Sphère", Effect: "Cette attaque inflige aussi 20 dégâts."}},
	}
	after := writeTemp(t, b)
	got, err := Decode(after)
	if err != nil {
		t.Fatal(err)
	}
	orig, _ := Decode(before)
	op, _ := orig.Print("tst-002")
	gp, _ := got.Print("tst-002")
	enBefore, _ := json.Marshal(op.Text["en"])
	enAfter, _ := json.Marshal(gp.Text["en"])
	if !bytes.Equal(enBefore, enAfter) {
		t.Errorf("English wording changed:\n%s\n%s", enBefore, enAfter)
	}
	coreOf := func(p Print) []byte {
		p.Text = nil
		b, _ := json.Marshal(p)
		return b
	}
	if !bytes.Equal(coreOf(*op), coreOf(*gp)) {
		t.Errorf("core changed:\n%s\n%s", coreOf(*op), coreOf(*gp))
	}
	// Every line of the English-only file is still in the file, in order,
	// apart from the English block's closing brace, which gains a comma
	// because the French block follows it.
	bl, al := strings.Split(string(before), "\n"), strings.Split(string(after), "\n")
	j := 0
	for _, l := range al {
		if j < len(bl) && l == bl[j] {
			j++
		}
	}
	if j != len(bl) {
		t.Errorf("adding French removed or changed an existing line (matched %d of %d)", j, len(bl))
	}
}

func TestValidateRefusesMissingEnglish(t *testing.T) {
	tests := []struct {
		name   string
		change func(p *Print)
		want   string
	}{
		{"no English text", func(p *Print) { p.Text = map[string]PrintText{"fr": p.Text["en"]} }, "no English text"},
		{"no English name", func(p *Print) { en := p.Text["en"]; en.Name = " "; p.Text["en"] = en }, "no English name"},
		{"attack text missing", func(p *Print) { en := p.Text["en"]; en.Attacks = nil; p.Text["en"] = en }, "en text has 0 attacks, the card has 1"},
		{"ability text missing", func(p *Print) { en := p.Text["en"]; en.Abilities = nil; p.Text["en"] = en }, "en text has 0 abilities, the card has 1"},

		{"effect only in French", func(p *Print) {
			fr := p.Text["en"]
			fr.Abilities = []MoveText{{Name: "Focus", Effect: "Une fois pendant votre tour."}}
			en := p.Text["en"]
			en.Abilities = []MoveText{{Name: "Focus"}}
			p.Text["en"] = en
			p.Text["fr"] = fr
		}, "fr ability 0 has effect text, English has none"},
		{"card effect only in French", func(p *Print) {
			p.Text["fr"] = PrintText{Name: "Riolu", Attacks: []MoveText{{Name: "Coup"}}, Abilities: []MoveText{{Name: "Focus"}}, Effect: "Texte."}
		}, "fr text has an effect, English has none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testSet()
			p, _ := s.Print("tst-001")
			tt.change(p)
			err := s.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate() = %v, want error containing %q", err, tt.want)
			}
			if _, err := Decode(mustMarshal(t, s)); err == nil {
				t.Error("Decode accepted the invalid set")
			}
		})
	}
	if err := testSet().Validate(); err != nil {
		t.Errorf("valid set refused: %v", err)
	}
}

func TestValidateSetLevel(t *testing.T) {
	s := testSet()
	s.Set.Text = map[string]SetText{"en": {Name: ""}}
	s.Prints = append(s.Prints, s.Prints[0])
	err := s.Validate()
	if err == nil || !strings.Contains(err.Error(), "no English name") || !strings.Contains(err.Error(), "listed twice") {
		t.Errorf("Validate() = %v, want missing set name and duplicate print", err)
	}
}

func TestDecodeRefusesUnknownFields(t *testing.T) {
	b := bytes.Replace(mustMarshal(t, testSet()), []byte(`"hp": 70,`), []byte(`"hp": 70, "pricing": 1,`), 1)
	if _, err := Decode(b); err == nil || !strings.Contains(err.Error(), "pricing") {
		t.Errorf("Decode() = %v, want unknown field error", err)
	}
}

func mustMarshal(t *testing.T, s *Set) []byte {
	t.Helper()
	b, err := Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCardConversion(t *testing.T) {
	s := testSet()
	tests := []struct {
		id   string
		want cards.Card
	}{
		{"tst-001", cards.Card{
			ID: "tst-001", LocalID: "001", Name: "Riolu", Category: "Pokemon", Set: cards.SetRef{ID: "tst", Name: "Test Set"},
			HP: 70, Types: []string{"Fighting"}, Stage: "Basic", Retreat: 1,
			Abilities:   []cards.Ability{{Type: "Ability", Name: "Focus", Effect: "Once during your turn, you may draw 2 cards."}},
			Attacks:     []cards.Attack{{Cost: []string{"Fighting"}, Name: "Punch", Damage: "10+"}},
			Resistances: []cards.TypeValue{{Type: "Metal", Value: "-30"}},
		}},
		{"tst-002", cards.Card{
			ID: "tst-002", LocalID: "002", Name: "Lucario", Category: "Pokemon", Set: cards.SetRef{ID: "tst", Name: "Test Set"},
			Image: "https://assets.tcgdex.net/en/me/tst/002",
			HP:    130, Types: []string{"Fighting"}, Stage: "Stage1", EvolveFrom: "Riolu", Retreat: 2,
			Attacks:    []cards.Attack{{Cost: []string{"Fighting", "Colorless"}, Name: "Aura Sphere", Effect: "This attack also does 20 damage to 1 of your opponent's Benched Pokémon.", Damage: "130"}},
			Weaknesses: []cards.TypeValue{{Type: "Psychic", Value: "×2"}},
		}},
		{"tst-010", cards.Card{
			ID: "tst-010", LocalID: "010", Name: "Ultra Ball", Category: "Trainer", Set: cards.SetRef{ID: "tst", Name: "Test Set"},
			Image: "https://assets.tcgdex.net/en/me/tst/010", TrainerType: "Item", RegulationMark: "J",
			Effect: "You can use this card only if you discard 2 other cards from your hand. Search your deck for a Pokémon & reveal it.",
		}},
		{"tst-TG01", cards.Card{
			ID: "tst-TG01", LocalID: "TG01", Name: "Jet Energy", Category: "Energy", Set: cards.SetRef{ID: "tst", Name: "Test Set"},
			EnergyType: "Special", Effect: "As long as this card is attached to a Pokémon, it provides {C} Energy.",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, err := s.Card(tt.id, "en")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("Card()\n got %+v\nwant %+v", *got, tt.want)
			}
		})
	}
	if _, err := s.Card("tst-001", "fr"); err == nil {
		t.Error("Card in a missing language succeeded")
	}
	if _, err := s.Card("tst-999", "en"); err == nil {
		t.Error("Card for a missing print succeeded")
	}
}

// A print lacking English text for an ability, or for the effect of a
// Trainer or special Energy, is invalid; the set still decodes, so the
// import and checks can read it and report the gap.
func TestValidateRefusesMissingEnglishEffect(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		change func(p *Print)
		want   string
	}{
		{"Trainer without effect", "tst-010", func(p *Print) { en := p.Text["en"]; en.Effect = ""; p.Text["en"] = en }, "print tst-010: no English text for /text/en/effect"},
		{"special Energy without effect", "tst-TG01", func(p *Print) { en := p.Text["en"]; en.Effect = " "; p.Text["en"] = en }, "print tst-TG01: no English text for /text/en/effect"},
		{"ability without effect", "tst-001", func(p *Print) { p.Text["en"].Abilities[0].Effect = "" }, "print tst-001: no English text for /text/en/abilities/0/effect"},
		{"attack unnamed", "tst-001", func(p *Print) { p.Text["en"].Attacks[0].Name = "" }, "print tst-001: no English text for /text/en/attacks/0/name"},
		{"ability unnamed", "tst-001", func(p *Print) { p.Text["en"].Abilities[0].Name = " " }, "print tst-001: no English text for /text/en/abilities/0/name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testSet()
			p, _ := s.Print(tt.id)
			tt.change(p)
			err := s.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Validate() = %v, want error containing %q", err, tt.want)
			}
			if _, err := Decode(mustMarshal(t, s)); err != nil {
				t.Errorf("Decode refused a well-formed set with a text gap: %v", err)
			}
		})
	}
	// An attack without effect text and a Basic Energy without an effect
	// are complete.
	s := testSet()
	s.Prints = append(s.Prints, Print{ID: "tst-200", LocalID: "200", Category: "Energy", EnergyType: "Normal", Text: map[string]PrintText{"en": {Name: "Basic Fire Energy"}}})
	if err := s.Validate(); err != nil {
		t.Errorf("complete set refused: %v", err)
	}
}

// A Basic Energy print without an image converts with the art the
// playmat's card client borrows for it today.
func TestCardConversionBasicEnergyArt(t *testing.T) {
	p := Print{ID: "sve-002", LocalID: "002", Category: "Energy", EnergyType: "Normal", Text: map[string]PrintText{"en": {Name: "Basic Fire Energy"}}}
	got, err := p.Card("en")
	if err != nil {
		t.Fatal(err)
	}
	want := cards.Card{ID: "sve-002", LocalID: "002", Name: "Basic Fire Energy", Category: "Energy", EnergyType: "Normal", Image: "https://assets.tcgdex.net/en/swsh/swsh12.5/153"}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("Card()\n got %+v\nwant %+v", *got, want)
	}
}

// Each built print converts into exactly the card the playmat builds today
// from the same card's TCGdex body: the body decoded into cards.Card, with
// the client's borrowed Basic Energy art.
func TestCardMatchesPlaymatCard(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		print Print
	}{
		{"Pokémon", `{"category":"Pokemon","id":"me06-001","localId":"001","name":"Heracross","set":{"id":"me06","name":"Delta Reign"},"hp":130,"types":["Grass"],"stage":"Basic","attacks":[{"cost":["Grass"],"name":"Spike Draw","effect":"Draw 2 cards.","damage":20},{"cost":["Grass","Grass","Colorless"],"name":"High Horsepower","effect":"This Pokémon also does 30 damage to itself.","damage":130}],"weaknesses":[{"type":"Fire","value":"×2"}],"retreat":3,"regulationMark":"J","image":"https://assets.tcgdex.net/en/me/me06/001"}`,
			Print{ID: "me06-001", LocalID: "001", Category: "Pokemon", HP: 130, Types: []string{"Grass"}, Stage: "Basic", Retreat: 3, RegulationMark: "J",
				Attacks:    []AttackCore{{Cost: []string{"Grass"}, Damage: "20"}, {Cost: []string{"Grass", "Grass", "Colorless"}, Damage: "130"}},
				Weaknesses: []cards.TypeValue{{Type: "Fire", Value: "×2"}},
				Images:     []Image{{Source: "tcgdex", URL: "https://assets.tcgdex.net/en/me/me06/001"}, {Source: "malie", URL: "https://cdn.malie.io/x.jpg"}},
				Text:       map[string]PrintText{"en": {Name: "Heracross", Attacks: []MoveText{{Name: "Spike Draw", Effect: "Draw 2 cards."}, {Name: "High Horsepower", Effect: "This Pokémon also does 30 damage to itself."}}}}}},
		{"Stage 1 with ability", `{"category":"Pokemon","id":"sv01-002","localId":"002","name":"Forretress","set":{"id":"sv01","name":"Scarlet & Violet"},"hp":120,"types":["Grass"],"stage":"Stage1","evolveFrom":"Pineco","abilities":[{"type":"Ability","name":"Spiky Shell","effect":"Put 2 damage counters."}],"attacks":[{"cost":["Colorless","Colorless","Colorless"],"name":"Tackle","damage":"30+"}],"resistances":[{"type":"Water","value":"-30"}],"retreat":3}`,
			Print{ID: "sv01-002", LocalID: "002", Category: "Pokemon", HP: 120, Types: []string{"Grass"}, Stage: "Stage1", Retreat: 3,
				Abilities:   []AbilityCore{{Type: "Ability"}},
				Attacks:     []AttackCore{{Cost: []string{"Colorless", "Colorless", "Colorless"}, Damage: "30+"}},
				Resistances: []cards.TypeValue{{Type: "Water", Value: "-30"}},
				Text:        map[string]PrintText{"en": {Name: "Forretress", EvolveFrom: "Pineco", Abilities: []MoveText{{Name: "Spiky Shell", Effect: "Put 2 damage counters."}}, Attacks: []MoveText{{Name: "Tackle"}}}}}},
		{"Trainer", `{"category":"Trainer","id":"sv01-170","localId":"170","name":"Electric Generator","set":{"id":"sv01","name":"Scarlet & Violet"},"image":"https://assets.tcgdex.net/en/sv/sv01/170","effect":"Look at the top 5 cards of your deck and attach up to 2 Basic {L} Energy cards.","trainerType":"Item","regulationMark":"G"}`,
			Print{ID: "sv01-170", LocalID: "170", Category: "Trainer", TrainerType: "Item", RegulationMark: "G",
				Images: []Image{{Source: "tcgdex", URL: "https://assets.tcgdex.net/en/sv/sv01/170"}},
				Text:   map[string]PrintText{"en": {Name: "Electric Generator", Effect: "Look at the top 5 cards of your deck and attach up to 2 Basic {L} Energy cards."}}}},
		{"special Energy", `{"category":"Energy","id":"me06-132","localId":"132","name":"Bubbly Water Energy","set":{"id":"me06","name":"Delta Reign"},"energyType":"Special","effect":"As long as this card is attached to a Pokémon, it provides {W} Energy."}`,
			Print{ID: "me06-132", LocalID: "132", Category: "Energy", EnergyType: "Special",
				Images: []Image{{Source: "malie", URL: "https://cdn.malie.io/x.jpg"}},
				Text:   map[string]PrintText{"en": {Name: "Bubbly Water Energy", Effect: "As long as this card is attached to a Pokémon, it provides {W} Energy."}}}},
		{"Basic Energy without art", `{"category":"Energy","id":"sve-002","localId":"002","name":"Basic Fire Energy","set":{"id":"sve","name":"Scarlet & Violet Energies"},"energyType":"Normal"}`,
			Print{ID: "sve-002", LocalID: "002", Category: "Energy", EnergyType: "Normal", Text: map[string]PrintText{"en": {Name: "Basic Fire Energy"}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var want cards.Card
			if err := json.Unmarshal([]byte(tt.body), &want); err != nil {
				t.Fatal(err)
			}
			cards.EnergyArt(&want)
			s := &Set{Format: Format, Set: SetHeader{ID: want.Set.ID, Text: map[string]SetText{"en": {Name: want.Set.Name}}}, Prints: []Print{tt.print}}
			got, err := s.Card(tt.print.ID, "en")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*got, want) {
				t.Errorf("Card()\n got %+v\nwant %+v", *got, want)
			}
		})
	}
}
