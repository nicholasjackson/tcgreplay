package cardset

import (
	"reflect"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

const malieJPG = "https://cdn.malie.io/file/malie-io/tcgl/cards/jpg/en/me6/me6_en_"

// mergeDeltaReign merges the Delta Reign fixtures. TCGdex's body for
// me06-063 is left out, so that print is listed by malie only.
func mergeDeltaReign(t *testing.T) map[string]merged {
	t.Helper()
	var tcgdex []candidate
	for _, id := range []string{"me06-001", "me06-003", "me06-014", "me06-023", "me06-080", "me06-089", "me06-132"} {
		tcgdex = append(tcgdex, tcgdexFixture(t, id))
	}
	out := map[string]merged{}
	for _, m := range merge(tcgdex, fromMalie(malieFixture(t, "me6"), me06Set(t))) {
		if _, dup := out[m.ID]; dup {
			t.Errorf("%s built twice", m.ID)
		}
		out[m.ID] = m
	}
	return out
}

func TestMergeFillsGapsFromMalie(t *testing.T) {
	got := mergeDeltaReign(t)
	tests := []struct {
		id   string
		want Print
	}{
		{"me06-003", Print{ // Stage 1 without evolves-from, regulation mark or image in TCGdex
			ID: "me06-003", LocalID: "003", Category: "Pokemon", HP: 110, Types: []string{"Grass"}, Stage: "Stage1",
			Attacks: []AttackCore{
				{Cost: []string{"Colorless"}, Damage: "30"},
				{Cost: []string{"Grass"}, Damage: "50×"},
			},
			Weaknesses:     []cards.TypeValue{{Type: "Fire", Value: "×2"}},
			Retreat:        1,
			RegulationMark: "J",
			Images:         []Image{{Source: "malie", URL: malieJPG + "003_std.jpg"}},
			Sources:        []string{"malie", "tcgdex"},
			Text: map[string]PrintText{"en": {
				Name: "Masquerain", EvolveFrom: "Surskit",
				Attacks: []MoveText{
					{Name: "Frightening Pattern", Effect: "During your opponent's next turn, the Defending Pokémon can't use attacks."},
					{Name: "Bug Out", Effect: "Reveal the bottom 7 cards of your deck, and this attack does 50 damage for each Pokémon you find there that has the Bug Out attack. Then, shuffle any revealed Pokémon back into your deck. Discard the other cards."},
				},
			}},
		}},
		{"me06-089", Print{ // Trainer without effect text in TCGdex
			ID: "me06-089", LocalID: "089", Category: "Trainer", TrainerType: "Supporter", RegulationMark: "J",
			Images:  []Image{{Source: "malie", URL: malieJPG + "089_std.jpg"}},
			Sources: []string{"malie", "tcgdex"},
			Text: map[string]PrintText{"en": {
				Name:   "Aarune",
				Effect: "Search your deck for up to 3 in any combination of Supporter and Stadium cards, reveal them, and put them into your hand. Then, shuffle your deck.",
			}},
		}},
		{"me06-132", Print{ // special Energy without effect or energy type in TCGdex; TCGdex's name kept
			ID: "me06-132", LocalID: "132", Category: "Energy", Types: []string{"Water"}, EnergyType: "Special", RegulationMark: "J",
			Images:  []Image{{Source: "malie", URL: malieJPG + "132_std.jpg"}},
			Sources: []string{"malie", "tcgdex"},
			Text: map[string]PrintText{"en": {
				Name:   "Bubbly Water Energy",
				Effect: "As long as this card is attached to a Pokémon, it provides {W} Energy.\n\nThe {W} Pokémon this card is attached to recovers from all Special Conditions and can't be affected by any Special Conditions.",
			}},
		}},
		{"me06-080", Print{ // no attacks in TCGdex: malie's attack is taken, TCGdex's ability kept
			ID: "me06-080", LocalID: "080", Category: "Pokemon", HP: 30, Types: []string{"Colorless"}, Stage: "Basic",
			Attacks:        []AttackCore{{}},
			Abilities:      []AbilityCore{{Type: "Ability"}},
			Weaknesses:     []cards.TypeValue{{Type: "Fighting", Value: "×2"}},
			RegulationMark: "J",
			Images:         []Image{{Source: "malie", URL: malieJPG + "080_std.jpg"}},
			Sources:        []string{"malie", "tcgdex"},
			Text: map[string]PrintText{"en": {
				Name:      "Azurill",
				Attacks:   []MoveText{{Name: "Bouncy-Bounce Charge", Effect: "Search your deck for an Energy card and attach it to 1 of your Benched Pokémon. Then, shuffle your deck."}},
				Abilities: []MoveText{{Name: "Bouncy-Bounce Charge", Effect: "Search your deck for an Energy card and attach it to 1 of your Benched Pokémon. Then, shuffle your deck."}},
			}},
		}},
		{"me06-014", Print{ // TCGdex's stage and suffix kept although malie differs
			ID: "me06-014", LocalID: "014", Category: "Pokemon", HP: 320, Types: []string{"Grass"}, Stage: "Basic", Suffix: "EX",
			Attacks:        []AttackCore{{Cost: []string{"Grass", "Grass"}, Damage: "120"}},
			Weaknesses:     []cards.TypeValue{{Type: "Fire", Value: "×2"}},
			Retreat:        2,
			RegulationMark: "J",
			Images:         []Image{{Source: "malie", URL: malieJPG + "014_std.jpg"}},
			Sources:        []string{"malie", "tcgdex"},
			Text: map[string]PrintText{"en": {
				Name: "Meowscarada ex", EvolveFrom: "Floragato",
				Attacks: []MoveText{{Name: "Magical Bullet", Effect: "This attack also does 120 damage to 1 of your opponent's Benched Pokémon that has any damage counters on it. (Don't apply Weakness and Resistance for Benched Pokémon.)"}},
			}},
		}},
		{"me06-063", Print{ // listed by malie only
			ID: "me06-063", LocalID: "063", Category: "Pokemon", HP: 120, Types: []string{"Darkness"}, Stage: "Basic",
			Attacks: []AttackCore{
				{Cost: []string{"Darkness"}},
				{Cost: []string{"Darkness", "Darkness", "Colorless"}, Damage: "110"},
			},
			Weaknesses:     []cards.TypeValue{{Type: "Grass", Value: "×2"}},
			Retreat:        2,
			RegulationMark: "J",
			Images:         []Image{{Source: "malie", URL: malieJPG + "063_std.jpg"}},
			Sources:        []string{"malie"},
			Text: map[string]PrintText{"en": {
				Name: "Darkrai",
				Attacks: []MoveText{
					{Name: "Shadow-Clad", Effect: "Search your deck for up to 2 Basic {D} Energy cards and attach them to this Pokémon. Then, shuffle your deck."},
					{Name: "Dark Cutter"},
				},
			}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			m, ok := got[tt.id]
			if !ok {
				t.Fatalf("%s not built", tt.id)
			}
			if !reflect.DeepEqual(m.Print, tt.want) {
				t.Errorf("got  %+v\nwant %+v", m.Print, tt.want)
			}
		})
	}
}

func TestMergeDeltaReignDisagreements(t *testing.T) {
	got := mergeDeltaReign(t)
	// Heat Rotom ex's effect reads "Basic Fire Energy" in TCGdex and "Basic
	// {R} Energy" in malie, and Bubbly Water Energy is "Bubbly {W} Energy"
	// in malie: neither is a disagreement. TCGdex writes the ex suffix "EX"
	// and has Meowscarada ex as a Basic.
	want := map[string][]disagreement{
		"me06-001": nil,
		"me06-003": nil,
		"me06-014": {
			{Field: "/stage", Values: map[string]any{"tcgdex": "Basic", "malie": "Stage2"}},
			{Field: "/suffix", Values: map[string]any{"tcgdex": "EX", "malie": "ex"}},
		},
		"me06-023": {
			{Field: "/suffix", Values: map[string]any{"tcgdex": "EX", "malie": "ex"}},
		},
		"me06-063": nil,
		// TCGdex lists Bouncy-Bounce Charge as an ability, malie as an
		// attack: the attack is filled in but the mismatch is recorded.
		"me06-080": {
			{Field: "/attacks", Values: map[string]any{"tcgdex": []string{}, "malie": []string{"Bouncy-Bounce Charge"}}},
		},
		"me06-089": nil,
		"me06-132": nil,
	}
	if len(got) != len(want) {
		t.Errorf("built %d prints, want %d", len(got), len(want))
	}
	for id, w := range want {
		if d := got[id].disagreements; !reflect.DeepEqual(d, w) {
			t.Errorf("%s disagreements = %v, want %v", id, d, w)
		}
	}
}

func TestMergeLeavesNoMarkup(t *testing.T) {
	for id, m := range mergeDeltaReign(t) {
		for _, s := range printTexts(m.Print) {
			if hasMarkup(s) {
				t.Errorf("%s: text still holds markup: %q", id, s)
			}
		}
	}
}

func TestMergeOrdersImagesTCGdexFirst(t *testing.T) {
	out := merge(
		[]candidate{tcgdexFixture(t, "sv01-001")},
		fromMalie(malieFixture(t, "sv1"), decodeJSON[TCGdexSet](t, `{"id":"sv01","cards":[{"id":"sv01-001","localId":"001"}]}`)),
	)
	if len(out) != 1 {
		t.Fatalf("built %d prints, want 1 (the reverse holo is not a print)", len(out))
	}
	want := []Image{
		{Source: "tcgdex", URL: "https://assets.tcgdex.net/en/sv/sv01/001"},
		{Source: "malie", URL: "https://cdn.malie.io/file/malie-io/tcgl/cards/jpg/en/sv1/sv1_en_001_std.jpg"},
	}
	if !reflect.DeepEqual(out[0].Images, want) {
		t.Errorf("images = %+v, want %+v", out[0].Images, want)
	}
	if want := []string{"malie", "tcgdex"}; !reflect.DeepEqual(out[0].Sources, want) {
		t.Errorf("sources = %v, want %v", out[0].Sources, want)
	}
	if out[0].RegulationMark != "G" {
		t.Errorf("regulation mark = %q, want G", out[0].RegulationMark)
	}
	if out[0].disagreements != nil {
		t.Errorf("disagreements = %v, want none", out[0].disagreements)
	}
}

func TestMergeUnionAndOrder(t *testing.T) {
	p := func(id, local, name, src string) candidate {
		return candidate{source: src, Print: Print{
			ID: id, LocalID: local, Category: "Trainer", Sources: []string{src},
			Text: map[string]PrintText{"en": {Name: name}},
		}}
	}
	out := merge(
		[]candidate{p("tst-010", "010", "Ten", "tcgdex"), p("tst-002", "002", "Two", "tcgdex")},
		[]candidate{p("tst-010", "010", "Ten", "malie"), p("tst-001", "001", "One", "malie")},
	)
	var got []string
	for _, m := range out {
		got = append(got, m.ID)
	}
	if want := []string{"tst-001", "tst-002", "tst-010"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	wantSources := [][]string{{"malie"}, {"tcgdex"}, {"malie", "tcgdex"}}
	for i, m := range out {
		if !reflect.DeepEqual(m.Sources, wantSources[i]) {
			t.Errorf("%s sources = %v, want %v", m.ID, m.Sources, wantSources[i])
		}
	}
}

// pokemon returns a print with every field set, for mergePrint tests to
// change one field at a time.
func pokemon(src string) Print {
	return Print{
		ID: "tst-001", LocalID: "001", Category: "Pokemon", HP: 120, Types: []string{"Fire"}, Stage: "Basic", Retreat: 2,
		Attacks: []AttackCore{
			{Cost: []string{"Fire"}, Damage: "30"},
			{Cost: []string{"Fire", "Colorless"}, Damage: "90"},
		},
		Abilities:      []AbilityCore{{Type: "Ability"}},
		Weaknesses:     []cards.TypeValue{{Type: "Water", Value: "×2"}},
		RegulationMark: "H",
		Images:         []Image{{Source: src, URL: "https://" + src + ".example/tst-001.jpg"}},
		Sources:        []string{src},
		Text: map[string]PrintText{"en": {
			Name: "Charmeleon",
			Attacks: []MoveText{
				{Name: "Ember", Effect: "Discard a {R} Energy from this Pokémon."},
				{Name: "Fire Fang", Effect: "Your opponent’s Active Pokémon is now Burned."},
			},
			Abilities: []MoveText{{Name: "Hot Body", Effect: "Once during your turn, you may draw a card."}},
		}},
	}
}

func TestMergePrintDisagreements(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(t, m *Print)
		check func(t *testing.T, got Print)
		want  []disagreement
	}{
		{
			name: "identical",
			edit: func(t, m *Print) {},
		},
		{
			name: "cosmetic differences only",
			edit: func(t, m *Print) {
				mt := m.Text["en"]
				mt.Attacks = []MoveText{
					{Name: "Ember", Effect: "Discard a  Fire Energy\nfrom this Pokémon."},
					{Name: "Fire Fang", Effect: "Your opponent's Active Pokémon is now Burned."},
				}
				m.Text["en"] = mt
				m.Attacks[1].Damage = "90 "
			},
		},
		{
			name: "Basic Energy name spelling",
			edit: func(t, m *Print) {
				*t = Print{ID: "tst-001", LocalID: "001", Category: "Energy", EnergyType: "Normal", Text: map[string]PrintText{"en": {Name: "Grass Energy"}}}
				*m = Print{ID: "tst-001", LocalID: "001", Category: "Energy", EnergyType: "Normal", Text: map[string]PrintText{"en": {Name: "Basic {G} Energy"}}}
			},
			check: func(tt *testing.T, got Print) {
				if got.Text["en"].Name != "Grass Energy" {
					tt.Errorf("name = %q, want TCGdex's", got.Text["en"].Name)
				}
			},
		},
		{
			name: "damage × spelled x",
			edit: func(t, m *Print) { t.Attacks[0].Damage = "30×"; m.Attacks[0].Damage = "30x" },
		},
		{
			name: "hp",
			edit: func(t, m *Print) { m.HP = 130 },
			check: func(tt *testing.T, got Print) {
				if got.HP != 120 {
					tt.Errorf("hp = %d, want TCGdex's 120", got.HP)
				}
			},
			want: []disagreement{{Field: "/hp", Values: map[string]any{"tcgdex": 120, "malie": 130}}},
		},
		{
			name: "attack effect",
			edit: func(t, m *Print) {
				mt := m.Text["en"]
				mt.Attacks = []MoveText{mt.Attacks[0], {Name: "Fire Fang", Effect: "Your opponent's Active Pokémon is now Paralyzed."}}
				m.Text["en"] = mt
			},
			check: func(tt *testing.T, got Print) {
				if e := got.Text["en"].Attacks[1].Effect; e != "Your opponent’s Active Pokémon is now Burned." {
					tt.Errorf("effect = %q, want TCGdex's", e)
				}
			},
			want: []disagreement{{Field: "/text/en/attacks/1/effect", Values: map[string]any{
				"tcgdex": "Your opponent’s Active Pokémon is now Burned.",
				"malie":  "Your opponent's Active Pokémon is now Paralyzed.",
			}}},
		},
		{
			name: "damage and cost",
			edit: func(t, m *Print) { m.Attacks[0] = AttackCore{Cost: []string{"Colorless"}, Damage: "30+"} },
			want: []disagreement{
				{Field: "/attacks/0/cost", Values: map[string]any{"tcgdex": []string{"Fire"}, "malie": []string{"Colorless"}}},
				{Field: "/attacks/0/damage", Values: map[string]any{"tcgdex": "30", "malie": "30+"}},
			},
		},
		{
			name: "types, weakness, retreat, regulation mark",
			edit: func(t, m *Print) {
				m.Types = []string{"Water"}
				m.Weaknesses = []cards.TypeValue{{Type: "Grass", Value: "×2"}}
				m.Retreat = 1
				m.RegulationMark = "G"
			},
			want: []disagreement{
				{Field: "/types", Values: map[string]any{"tcgdex": []string{"Fire"}, "malie": []string{"Water"}}},
				{Field: "/weaknesses", Values: map[string]any{"tcgdex": []string{"Water ×2"}, "malie": []string{"Grass ×2"}}},
				{Field: "/retreat", Values: map[string]any{"tcgdex": 2, "malie": 1}},
				{Field: "/regulationMark", Values: map[string]any{"tcgdex": "H", "malie": "G"}},
			},
		},
		{
			name: "name and ability effect",
			edit: func(t, m *Print) {
				mt := m.Text["en"]
				mt.Name = "Charizard"
				mt.Abilities = []MoveText{{Name: "Hot Body", Effect: "Once during your turn, you may draw 2 cards."}}
				m.Text["en"] = mt
			},
			want: []disagreement{
				{Field: "/text/en/name", Values: map[string]any{"tcgdex": "Charmeleon", "malie": "Charizard"}},
				{Field: "/text/en/abilities/0/effect", Values: map[string]any{
					"tcgdex": "Once during your turn, you may draw a card.",
					"malie":  "Once during your turn, you may draw 2 cards.",
				}},
			},
		},
		{
			name: "attack count",
			edit: func(t, m *Print) {
				m.Attacks = m.Attacks[:1]
				mt := m.Text["en"]
				mt.Attacks = []MoveText{{Name: "Ember", Effect: "Something else entirely."}}
				m.Text["en"] = mt
			},
			check: func(tt *testing.T, got Print) {
				want := pokemon("tcgdex")
				if !reflect.DeepEqual(got.Attacks, want.Attacks) || !reflect.DeepEqual(got.Text["en"].Attacks, want.Text["en"].Attacks) {
					tt.Errorf("attacks = %+v %+v, want TCGdex's", got.Attacks, got.Text["en"].Attacks)
				}
			},
			want: []disagreement{{Field: "/attacks", Values: map[string]any{
				"tcgdex": []string{"Ember", "Fire Fang"},
				"malie":  []string{"Ember"},
			}}},
		},
		{
			name: "ability count",
			edit: func(t, m *Print) {
				m.Abilities = append(m.Abilities, AbilityCore{Type: "Ability"})
				mt := m.Text["en"]
				mt.Abilities = append(mt.Abilities, MoveText{Name: "Extra", Effect: "More."})
				m.Text["en"] = mt
			},
			check: func(tt *testing.T, got Print) {
				if len(got.Abilities) != 1 || len(got.Text["en"].Abilities) != 1 {
					tt.Errorf("abilities = %+v, want TCGdex's one", got.Text["en"].Abilities)
				}
			},
			want: []disagreement{{Field: "/abilities", Values: map[string]any{
				"tcgdex": []string{"Hot Body"},
				"malie":  []string{"Hot Body", "Extra"},
			}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, mp := pokemon("tcgdex"), pokemon("malie")
			tt.edit(&tp, &mp)
			got := mergePrint(tp, mp)
			if !reflect.DeepEqual(got.disagreements, tt.want) {
				t.Errorf("disagreements = %v\nwant %v", got.disagreements, tt.want)
			}
			if tt.check != nil {
				tt.check(t, got.Print)
			}
		})
	}
}

func TestMergePrintKeepsTCGdexAndFillsGaps(t *testing.T) {
	// TCGdex has everything: every value is TCGdex's even where malie
	// differs (the differences are recorded elsewhere).
	tp := pokemon("tcgdex")
	mp := Print{
		ID: "tst-001", LocalID: "001", Category: "Pokemon", HP: 130, Types: []string{"Water"}, Stage: "Stage1", Suffix: "ex", Retreat: 1,
		Attacks:        []AttackCore{{Cost: []string{"Water"}, Damage: "10"}, {Cost: []string{"Water"}, Damage: "20"}},
		Abilities:      []AbilityCore{{Type: "Poké-Power"}},
		Weaknesses:     []cards.TypeValue{{Type: "Grass", Value: "×2"}},
		Resistances:    []cards.TypeValue{{Type: "Metal", Value: "-30"}},
		RegulationMark: "G",
		Images:         []Image{{Source: "malie", URL: "https://malie.example/tst-001.jpg"}},
		Sources:        []string{"malie"},
		Text: map[string]PrintText{"en": {
			Name: "Other", EvolveFrom: "Charmander",
			Attacks:   []MoveText{{Name: "A", Effect: "a"}, {Name: "B", Effect: "b"}},
			Abilities: []MoveText{{Name: "C", Effect: "c"}},
			Effect:    "Rule text.",
		}},
	}
	got := mergePrint(tp, mp).Print
	want := pokemon("tcgdex")
	// Fields TCGdex lacks come from malie; images list TCGdex's first.
	want.Suffix = "ex"
	want.Resistances = []cards.TypeValue{{Type: "Metal", Value: "-30"}}
	want.Images = []Image{{Source: "tcgdex", URL: "https://tcgdex.example/tst-001.jpg"}, {Source: "malie", URL: "https://malie.example/tst-001.jpg"}}
	want.Sources = []string{"malie", "tcgdex"}
	wt := want.Text["en"]
	wt.EvolveFrom = "Charmander"
	wt.Effect = "Rule text."
	want.Text = map[string]PrintText{"en": wt}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestMergePrintTakesMalieAttacksWhenTCGdexHasNone(t *testing.T) {
	tp := pokemon("tcgdex")
	tp.Attacks = nil
	tt := tp.Text["en"]
	tt.Attacks = nil
	tp.Text = map[string]PrintText{"en": tt}
	mp := pokemon("malie")
	mp.Attacks = []AttackCore{{Cost: []string{"Water"}, Damage: "10"}}
	mt := mp.Text["en"]
	mt.Attacks = []MoveText{{Name: "Splash", Effect: "Flip a coin."}}
	mp.Text = map[string]PrintText{"en": mt}

	m := mergePrint(tp, mp)
	if want := []AttackCore{{Cost: []string{"Water"}, Damage: "10"}}; !reflect.DeepEqual(m.Attacks, want) {
		t.Errorf("attacks = %+v, want %+v", m.Attacks, want)
	}
	if want := []MoveText{{Name: "Splash", Effect: "Flip a coin."}}; !reflect.DeepEqual(m.Text["en"].Attacks, want) {
		t.Errorf("attack text = %+v, want %+v", m.Text["en"].Attacks, want)
	}
	if m.disagreements != nil {
		t.Errorf("disagreements = %v, want none", m.disagreements)
	}
}

// Systematic spelling differences found across the Scarlet & Violet and
// Mega Evolution sets are cosmetic: weakness "x2" against "×2", and energy
// symbols in square brackets against braces.
func TestMergeIgnoresCatalogueSpellings(t *testing.T) {
	base := func() Print {
		return Print{ID: "x-001", LocalID: "001", Category: "Pokemon", HP: 60,
			Attacks:    []AttackCore{{Cost: []string{"Fire"}, Damage: "30"}},
			Weaknesses: []cards.TypeValue{{Type: "Water", Value: "×2"}},
			Text:       map[string]PrintText{"en": {Name: "Charmander", Attacks: []MoveText{{Name: "Ember", Effect: "Discard a {R} Energy from this Pokémon."}}}}}
	}
	td, ml := base(), base()
	td.Weaknesses = []cards.TypeValue{{Type: "Water", Value: "x2"}}
	td.Text["en"].Attacks[0].Effect = "Discard a [R] Energy from this Pokémon."
	if m := mergePrint(td, ml); len(m.disagreements) != 0 {
		t.Errorf("cosmetic spellings recorded as disagreements: %v", m.disagreements)
	}
	ml.Weaknesses = []cards.TypeValue{{Type: "Water", Value: "×3"}}
	if m := mergePrint(td, ml); len(m.disagreements) != 1 || m.disagreements[0].Field != "/weaknesses" {
		t.Errorf("a real weakness difference: %v", m.disagreements)
	}
}
