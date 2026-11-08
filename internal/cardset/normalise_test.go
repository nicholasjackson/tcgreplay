package cardset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// The fixtures under testdata are real upstream bodies, trimmed: TCGdex
// card bodies and set details for Delta Reign (me06) and Pineco (sv01-001),
// the matching entries of malie's me6 and sv1 set files (standard and
// reverse-holo entries both kept), and a cut of malie's index.

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// tcgdexFixture normalises testdata/tcgdex/<id>.json.
func tcgdexFixture(t *testing.T, id string) candidate {
	t.Helper()
	c, err := fromTCGdex(readFixture(t, "tcgdex/"+id+".json"))
	if err != nil {
		t.Fatalf("fromTCGdex(%s): %v", id, err)
	}
	return c
}

// malieFixture decodes testdata/malie/<name>.json.
func malieFixture(t *testing.T, name string) []MalieCard {
	t.Helper()
	var out []MalieCard
	if err := json.Unmarshal(readFixture(t, "malie/"+name+".json"), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// malieEntry returns the standard entry with collector number n.
func malieEntry(t *testing.T, entries []MalieCard, n int) MalieCard {
	t.Helper()
	for _, e := range entries {
		if int(e.Number.Numeric) == n && strings.Contains(e.Ext.TCGL.LongFormID, "_std_") {
			return e
		}
	}
	t.Fatalf("no standard malie entry %d", n)
	return MalieCard{}
}

func decodeJSON[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func me06Set(t *testing.T) TCGdexSet {
	t.Helper()
	return decodeJSON[TCGdexSet](t, string(readFixture(t, "tcgdex/set-me06.json")))
}

// printTexts lists every piece of English wording a print holds.
func printTexts(p Print) []string {
	en := p.Text[English]
	out := []string{en.Name, en.EvolveFrom, en.Effect}
	for _, m := range append(append([]MoveText{}, en.Attacks...), en.Abilities...) {
		out = append(out, m.Name, m.Effect)
	}
	return out
}

func TestFromTCGdexStripsMarkup(t *testing.T) {
	tests := []struct {
		id   string
		get  func(PrintText) string
		want string
	}{
		{
			"me06-023", func(t PrintText) string { return t.Attacks[0].Effect },
			"This attack does 30 damage for each Basic Fire Energy card in your discard pile. Then, shuffle those cards into your deck.",
		},
		{
			"me06-063", func(t PrintText) string { return t.Attacks[0].Effect },
			"Search your deck for up to 2 Basic Darkness Energy cards and attach them to this Pokémon. Then, shuffle your deck.",
		},
		{
			"me06-014", func(t PrintText) string { return t.Attacks[0].Effect },
			"This attack also does 120 damage to 1 of your opponent's Benched Pokémon that has any damage counters on it. (Don't apply Weakness and Resistance for Benched Pokémon.)",
		},
		{
			"me06-080", func(t PrintText) string { return t.Abilities[0].Effect },
			"Search your deck for an Energy card and attach it to 1 of your Benched Pokémon. Then, shuffle your deck.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := tt.get(tcgdexFixture(t, tt.id).Text[English]); got != tt.want {
				t.Errorf("got %q\nwant %q", got, tt.want)
			}
		})
	}
	for _, id := range []string{"me06-001", "me06-003", "me06-014", "me06-023", "me06-063", "me06-080", "me06-089", "me06-132", "sv01-001"} {
		for _, s := range printTexts(tcgdexFixture(t, id).Print) {
			if hasMarkup(s) {
				t.Errorf("%s: text still holds markup: %q", id, s)
			}
		}
	}
}

func TestFromTCGdex(t *testing.T) {
	tests := []struct {
		id   string
		want Print
	}{
		{"me06-023", Print{
			ID: "me06-023", LocalID: "023", Category: "Pokemon", HP: 190, Types: []string{"Fire"}, Stage: "Basic", Suffix: "EX",
			Attacks: []AttackCore{
				{Cost: []string{"Colorless", "Colorless"}, Damage: "30×"},
				{Cost: []string{"Fire", "Fire", "Colorless"}, Damage: "170"},
			},
			Weaknesses: []cards.TypeValue{{Type: "Water", Value: "×2"}},
			Retreat:    1,
			Sources:    []string{"tcgdex"},
			Text: map[string]PrintText{"en": {
				Name: "Heat Rotom ex",
				Attacks: []MoveText{
					{Name: "Rewarm", Effect: "This attack does 30 damage for each Basic Fire Energy card in your discard pile. Then, shuffle those cards into your deck."},
					{Name: "Strong Flare", Effect: "Discard 2 Energy from this Pokémon."},
				},
			}},
		}},
		{"sv01-001", Print{
			ID: "sv01-001", LocalID: "001", Category: "Pokemon", HP: 60, Types: []string{"Grass"}, Stage: "Basic",
			Attacks:        []AttackCore{{Cost: []string{"Colorless", "Colorless"}, Damage: "10"}},
			Weaknesses:     []cards.TypeValue{{Type: "Fire", Value: "×2"}},
			Retreat:        2,
			RegulationMark: "G",
			Images:         []Image{{Source: "tcgdex", URL: "https://assets.tcgdex.net/en/sv/sv01/001"}},
			Sources:        []string{"tcgdex"},
			Text: map[string]PrintText{"en": {
				Name:    "Pineco",
				Attacks: []MoveText{{Name: "Guard Press", Effect: "During your opponent's next turn, this Pokémon takes 30 less damage from attacks (after applying Weakness and Resistance)."}},
			}},
		}},
		{"me06-089", Print{
			ID: "me06-089", LocalID: "089", Category: "Trainer", TrainerType: "Supporter",
			Sources: []string{"tcgdex"},
			Text:    map[string]PrintText{"en": {Name: "Aarune"}},
		}},
		{"me06-132", Print{
			ID: "me06-132", LocalID: "132", Category: "Energy", Types: []string{"Water"},
			Sources: []string{"tcgdex"},
			Text:    map[string]PrintText{"en": {Name: "Bubbly Water Energy"}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			c := tcgdexFixture(t, tt.id)
			if c.source != "tcgdex" {
				t.Errorf("source = %q, want tcgdex", c.source)
			}
			if !reflect.DeepEqual(c.Print, tt.want) {
				t.Errorf("got  %+v\nwant %+v", c.Print, tt.want)
			}
		})
	}
}

func TestFromTCGdexErrors(t *testing.T) {
	for _, body := range []string{`not json`, `{"name":"No Id"}`} {
		if _, err := fromTCGdex(json.RawMessage(body)); err == nil {
			t.Errorf("fromTCGdex(%s): want an error", body)
		}
	}
}

func TestMalieWord(t *testing.T) {
	tests := []struct{ in, want string }{
		{"POKEMON", "Pokemon"},
		{"TRAINER", "Trainer"},
		{"ENERGY", "Energy"},
		{"BASIC", "Basic"},
		{"STAGE1", "Stage1"},
		{"STAGE2", "Stage2"},
		{"SUPPORTER", "Supporter"},
		{"ITEM", "Item"},
		{"STADIUM", "Stadium"},
		{"TOOL", "Tool"},
		{"DARKNESS", "Darkness"},
		{"COLORLESS", "Colorless"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := malieWord(tt.in); got != tt.want {
			t.Errorf("malieWord(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPickMalieEntries(t *testing.T) {
	entry := func(n int, longFormID string) MalieCard {
		var e MalieCard
		e.Number.Numeric = flexInt(n)
		e.Ext.TCGL.LongFormID = longFormID
		return e
	}
	in := []MalieCard{
		entry(3, "Masquerain_me6_3_ph_Common_FlatSilver_Reverse"),
		entry(3, "Masquerain_me6_3_std_Common_NonFoil_None"),
		entry(4, "Surskit_me6_4_std_Common_NonFoil_None"),
		entry(4, "Surskit_me6_4_ph_Common_FlatSilver_Reverse"),
		entry(5, "Promo_me6_5_ph_Common_FlatSilver_Reverse"),
	}
	want := []MalieCard{
		entry(3, "Masquerain_me6_3_std_Common_NonFoil_None"),
		entry(4, "Surskit_me6_4_std_Common_NonFoil_None"),
		entry(5, "Promo_me6_5_ph_Common_FlatSilver_Reverse"), // no standard entry: the first
	}
	if got := pickMalieEntries(in); !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}

	// The real file: 13 entries for 8 prints.
	picked := pickMalieEntries(malieFixture(t, "me6"))
	var got []string
	for _, e := range picked {
		got = append(got, e.Ext.TCGL.LongFormID)
	}
	wantIDs := []string{
		"Heracross_me6_1_std_Common_NonFoil_None",
		"Masquerain_me6_3_std_Common_NonFoil_None",
		"Meowscaradaex_me6_14_std_DoubleRare_SunPillar_Holo_CastAndCure_SouthernCross",
		"HeatRotomex_me6_23_std_DoubleRare_SunPillar_Holo_CastAndCure_SouthernCross",
		"Darkrai_me6_63_std_Uncommon_NonFoil_None",
		"Azurill_me6_80_std_Common_NonFoil_None",
		"Aarune_me6_89_std_Uncommon_NonFoil_None",
		"BubblyWEnergy_me6_132_std_UltraRare_SunPillar_Etched",
	}
	if !reflect.DeepEqual(got, wantIDs) {
		t.Errorf("picked %v\nwant   %v", got, wantIDs)
	}
}

func TestMalieCandidate(t *testing.T) {
	me6 := malieFixture(t, "me6")
	img := func(n string) []Image {
		return []Image{{Source: "malie", URL: "https://cdn.malie.io/file/malie-io/tcgl/cards/jpg/en/me6/me6_en_" + n + "_std.jpg"}}
	}
	tests := []struct {
		name string
		n    int
		want Print
	}{
		{"Stage 1 with evolves-from, × damage and weakness", 3, Print{
			Category: "Pokemon", HP: 110, Types: []string{"Grass"}, Stage: "Stage1",
			Attacks: []AttackCore{
				{Cost: []string{"Colorless"}, Damage: "30"},
				{Cost: []string{"Grass"}, Damage: "50×"},
			},
			Weaknesses:     []cards.TypeValue{{Type: "Fire", Value: "×2"}},
			Retreat:        1,
			RegulationMark: "J",
			Images:         img("003"),
			Sources:        []string{"malie"},
			Text: map[string]PrintText{"en": {
				Name: "Masquerain", EvolveFrom: "Surskit",
				Attacks: []MoveText{
					{Name: "Frightening Pattern", Effect: "During your opponent's next turn, the Defending Pokémon can't use attacks."},
					{Name: "Bug Out", Effect: "Reveal the bottom 7 cards of your deck, and this attack does 50 damage for each Pokémon you find there that has the Bug Out attack. Then, shuffle any revealed Pokémon back into your deck. Discard the other cards."},
				},
			}},
		}},
		{"ex suffix, rule box dropped, energy symbols kept", 23, Print{
			Category: "Pokemon", HP: 190, Types: []string{"Fire"}, Stage: "Basic", Suffix: "ex",
			Attacks: []AttackCore{
				{Cost: []string{"Colorless", "Colorless"}, Damage: "30×"},
				{Cost: []string{"Fire", "Fire", "Colorless"}, Damage: "170"},
			},
			Weaknesses:     []cards.TypeValue{{Type: "Water", Value: "×2"}},
			Retreat:        1,
			RegulationMark: "J",
			Images:         img("023"),
			Sources:        []string{"malie"},
			Text: map[string]PrintText{"en": {
				Name: "Heat Rotom ex",
				Attacks: []MoveText{
					{Name: "Rewarm", Effect: "This attack does 30 damage for each Basic {R} Energy card in your discard pile. Then, shuffle those cards into your deck."},
					{Name: "Strong Flare", Effect: "Discard 2 Energy from this Pokémon."},
				},
			}},
		}},
		{"Darkness type, attack without damage", 63, Print{
			Category: "Pokemon", HP: 120, Types: []string{"Darkness"}, Stage: "Basic",
			Attacks: []AttackCore{
				{Cost: []string{"Darkness"}},
				{Cost: []string{"Darkness", "Darkness", "Colorless"}, Damage: "110"},
			},
			Weaknesses:     []cards.TypeValue{{Type: "Grass", Value: "×2"}},
			Retreat:        2,
			RegulationMark: "J",
			Images:         img("063"),
			Sources:        []string{"malie"},
			Text: map[string]PrintText{"en": {
				Name: "Darkrai",
				Attacks: []MoveText{
					{Name: "Shadow-Clad", Effect: "Search your deck for up to 2 Basic {D} Energy cards and attach them to this Pokémon. Then, shuffle your deck."},
					{Name: "Dark Cutter"},
				},
			}},
		}},
		{"FREE cost dropped", 80, Print{
			Category: "Pokemon", HP: 30, Types: []string{"Colorless"}, Stage: "Basic",
			Attacks:        []AttackCore{{}},
			Weaknesses:     []cards.TypeValue{{Type: "Fighting", Value: "×2"}},
			RegulationMark: "J",
			Images:         img("080"),
			Sources:        []string{"malie"},
			Text: map[string]PrintText{"en": {
				Name:    "Azurill",
				Attacks: []MoveText{{Name: "Bouncy-Bounce Charge", Effect: "Search your deck for an Energy card and attach it to 1 of your Benched Pokémon. Then, shuffle your deck."}},
			}},
		}},
		{"Supporter: text box is the effect, reminder dropped", 89, Print{
			Category: "Trainer", TrainerType: "Supporter", RegulationMark: "J",
			Images:  img("089"),
			Sources: []string{"malie"},
			Text: map[string]PrintText{"en": {
				Name:   "Aarune",
				Effect: "Search your deck for up to 3 in any combination of Supporter and Stadium cards, reveal them, and put them into your hand. Then, shuffle your deck.",
			}},
		}},
		{"special Energy, malie blank line", 132, Print{
			Category: "Energy", EnergyType: "Special", RegulationMark: "J",
			Images:  img("132"),
			Sources: []string{"malie"},
			Text: map[string]PrintText{"en": {
				Name:   "Bubbly {W} Energy",
				Effect: "As long as this card is attached to a Pokémon, it provides {W} Energy.\n\nThe {W} Pokémon this card is attached to recovers from all Special Conditions and can't be affected by any Special Conditions.",
			}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := malieCandidate(malieEntry(t, me6, tt.n))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestMalieCandidateSynthetic(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		want  Print
	}{
		{
			"resistance, ability, Tera effect, Stage 2",
			`{"name":"Testmon","card_type":"POKEMON","stage":"STAGE2","stage_text":"Evolves from Midmon","hp":"140",
			  "types":["PSYCHIC"],"tags":["TERA"],
			  "weakness":{"types":["DARKNESS"],"operator":"×","amount":2},
			  "resistance":{"types":["FIGHTING"],"operator":"-","amount":30},"retreat":2,
			  "text":[
			    {"kind":"ABILITY","name":"Mind Trick","text":"Once during your turn, you may draw a card."},
			    {"kind":"ATTACK","name":"Psy Burst","text":"This attack does 20 more damage.","cost":["PSYCHIC","COLORLESS"],"damage":{"amount":60,"suffix":"+"}},
			    {"kind":"EFFECT","name":"Tera","text":"As long as this Pokémon is on your Bench, prevent all damage done to it by attacks."}
			  ]}`,
			Print{
				Category: "Pokemon", HP: 140, Types: []string{"Psychic"}, Stage: "Stage2", Retreat: 2,
				Attacks:     []AttackCore{{Cost: []string{"Psychic", "Colorless"}, Damage: "60+"}},
				Abilities:   []AbilityCore{{Type: "Ability"}},
				Weaknesses:  []cards.TypeValue{{Type: "Darkness", Value: "×2"}},
				Resistances: []cards.TypeValue{{Type: "Fighting", Value: "-30"}},
				Sources:     []string{"malie"},
				Text: map[string]PrintText{"en": {
					Name: "Testmon", EvolveFrom: "Midmon",
					Attacks:   []MoveText{{Name: "Psy Burst", Effect: "This attack does 20 more damage."}},
					Abilities: []MoveText{{Name: "Mind Trick", Effect: "Once during your turn, you may draw a card."}},
					Effect:    "As long as this Pokémon is on your Bench, prevent all damage done to it by attacks.",
				}},
			},
		},
		{
			"basic Energy is Normal",
			`{"name":"Basic {G} Energy","card_type":"ENERGY","subtype":"BASIC"}`,
			Print{
				Category: "Energy", EnergyType: "Normal", Sources: []string{"malie"},
				Text: map[string]PrintText{"en": {Name: "Basic {G} Energy"}},
			},
		},
		{
			"Item with markup in its text box",
			`{"name":"Test Ball","card_type":"TRAINER","subtype":"ITEM","regulation_mark":" H ",
			  "text":[{"kind":"TEXT_BOX","text":"Search your deck for a Pokémon.<br>Then, shuffle your deck."}]}`,
			Print{
				Category: "Trainer", TrainerType: "Item", RegulationMark: "H", Sources: []string{"malie"},
				Text: map[string]PrintText{"en": {Name: "Test Ball", Effect: "Search your deck for a Pokémon.\nThen, shuffle your deck."}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := malieCandidate(decodeJSON[MalieCard](t, tt.entry))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestFromMalie(t *testing.T) {
	got := fromMalie(malieFixture(t, "me6"), me06Set(t))
	var ids []string
	for _, c := range got {
		if c.source != "malie" {
			t.Errorf("%s: source = %q, want malie", c.ID, c.source)
		}
		ids = append(ids, c.ID+" "+c.LocalID)
	}
	// One candidate per print: the reverse-holo entries never make a second.
	want := []string{
		"me06-001 001", "me06-003 003", "me06-014 014", "me06-023 023",
		"me06-063 063", "me06-080 080", "me06-089 089", "me06-132 132",
	}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("ids %v\nwant %v", ids, want)
	}
}

func TestMatchPrintID(t *testing.T) {
	me06 := me06Set(t)
	entry := func(n int, cardID string) MalieCard {
		var e MalieCard
		e.Number.Numeric = flexInt(n)
		e.Ext.TCGL.CardID = cardID
		return e
	}
	tests := []struct {
		name      string
		set       TCGdexSet
		entry     MalieCard
		id, local string
	}{
		{"TCGdex print with the same number", me06, entry(89, "me6_89"), "me06-089", "089"},
		{"number wins over the log code", me06, entry(3, "me6_999"), "me06-003", "003"},
		{"log code naming this set", me06, entry(150, "me6_151"), "me06-151", "151"},
		{"log code when there is no number", me06, entry(0, "me6_150"), "me06-150", "150"},
		{"log code naming another set: padded number", me06, entry(150, "sv1_150"), "me06-150", "150"},
		{"no log code: padded number", me06, entry(7, ""), "me06-007", "007"},
		{
			"padding follows the set's numbers",
			decodeJSON[TCGdexSet](t, `{"id":"xy","cards":[{"id":"xy-TG01","localId":"TG01"},{"id":"xy-01","localId":"01"}]}`),
			entry(7, ""), "xy-07", "07",
		},
		{
			"no numbered prints: three digits",
			decodeJSON[TCGdexSet](t, `{"id":"xy","cards":[{"id":"xy-TG01","localId":"TG01"}]}`),
			entry(7, ""), "xy-007", "007",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, local := matchPrintID(tt.set, tt.entry)
			if id != tt.id || local != tt.local {
				t.Errorf("matchPrintID = %q, %q; want %q, %q", id, local, tt.id, tt.local)
			}
		})
	}
}
