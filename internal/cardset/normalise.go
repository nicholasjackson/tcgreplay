package cardset

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// candidate is one source's normalised print: a Print with English text
// only, from one source.
type candidate struct {
	Print
	source string
}

// tcgdexCard mirrors the definition fields of a TCGdex card body. Fields
// that churn (pricing, updated) or that the playmat never reads (rarity,
// illustrator, variants, legal, thirdParty) are not decoded, so they never
// reach a built set.
type tcgdexCard struct {
	ID         string   `json:"id"`
	LocalID    string   `json:"localId"`
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	Image      string   `json:"image"`
	HP         flexInt  `json:"hp"`
	Types      []string `json:"types"`
	Stage      string   `json:"stage"`
	EvolveFrom string   `json:"evolveFrom"`
	Suffix     string   `json:"suffix"`
	Abilities  []struct {
		Type   string `json:"type"`
		Name   string `json:"name"`
		Effect string `json:"effect"`
	} `json:"abilities"`
	Attacks []struct {
		Cost   []string     `json:"cost"`
		Name   string       `json:"name"`
		Effect string       `json:"effect"`
		Damage cards.Damage `json:"damage"`
	} `json:"attacks"`
	Weaknesses     []cards.TypeValue `json:"weaknesses"`
	Resistances    []cards.TypeValue `json:"resistances"`
	Retreat        flexInt           `json:"retreat"`
	TrainerType    string            `json:"trainerType"`
	EnergyType     string            `json:"energyType"`
	Effect         string            `json:"effect"`
	RegulationMark string            `json:"regulationMark"`
}

// fromTCGdex normalises a TCGdex card body. Text is cleaned of markup;
// nothing is borrowed from other prints (the playmat's Basic Energy art is
// run-time behaviour, not data).
func fromTCGdex(raw json.RawMessage) (candidate, error) {
	var c tcgdexCard
	if err := json.Unmarshal(raw, &c); err != nil {
		return candidate{}, fmt.Errorf("decoding TCGdex card: %w", err)
	}
	if c.ID == "" {
		return candidate{}, fmt.Errorf("TCGdex card has no id")
	}
	p := Print{
		ID:             c.ID,
		LocalID:        c.LocalID,
		Category:       c.Category,
		HP:             int(c.HP),
		Types:          c.Types,
		Stage:          c.Stage,
		Suffix:         c.Suffix,
		Weaknesses:     cleanTypeValues(c.Weaknesses),
		Resistances:    cleanTypeValues(c.Resistances),
		Retreat:        int(c.Retreat),
		TrainerType:    c.TrainerType,
		EnergyType:     c.EnergyType,
		RegulationMark: c.RegulationMark,
		Sources:        []string{SourceTCGdex},
	}
	t := PrintText{Name: cleanText(c.Name), EvolveFrom: cleanText(c.EvolveFrom), Effect: cleanText(c.Effect)}
	for _, a := range c.Abilities {
		p.Abilities = append(p.Abilities, AbilityCore{Type: a.Type})
		t.Abilities = append(t.Abilities, MoveText{Name: cleanText(a.Name), Effect: cleanText(a.Effect)})
	}
	for _, a := range c.Attacks {
		p.Attacks = append(p.Attacks, AttackCore{Cost: a.Cost, Damage: cards.Damage(strings.TrimSpace(string(a.Damage)))})
		t.Attacks = append(t.Attacks, MoveText{Name: cleanText(a.Name), Effect: cleanText(a.Effect)})
	}
	if c.Image != "" {
		p.Images = []Image{{Source: SourceTCGdex, URL: c.Image}}
	}
	p.Text = map[string]PrintText{English: t}
	return candidate{Print: p, source: SourceTCGdex}, nil
}

func cleanTypeValues(tv []cards.TypeValue) []cards.TypeValue {
	for i := range tv {
		tv[i].Value = strings.TrimSpace(tv[i].Value)
	}
	return tv
}

// malieWords maps the export's upper-case enums to TCGdex's spellings.
var malieWords = map[string]string{
	"POKEMON": "Pokemon", "TRAINER": "Trainer", "ENERGY": "Energy",
	"SUPPORTER": "Supporter", "ITEM": "Item", "STADIUM": "Stadium", "TOOL": "Tool",
	"BASIC": "Basic", "STAGE1": "Stage1", "STAGE2": "Stage2",
}

// malieWord turns an upper-case enum into TCGdex's spelling: a known
// mapping, else title case ("DARKNESS" is "Darkness").
func malieWord(s string) string {
	if w, ok := malieWords[s]; ok {
		return w
	}
	if s == "" {
		return ""
	}
	parts := strings.Split(strings.ToLower(s), "_")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// pickMalieEntries keeps one entry per collector number: the standard one
// (its longFormID holds "_std_"), else the first. The export lists each
// print twice, once more as a reverse holo with the same text.
func pickMalieEntries(entries []MalieCard) []MalieCard {
	index := map[int]int{}
	var out []MalieCard
	for _, e := range entries {
		n := int(e.Number.Numeric)
		i, seen := index[n]
		switch {
		case !seen:
			index[n] = len(out)
			out = append(out, e)
		case !isMalieStd(out[i]) && isMalieStd(e):
			out[i] = e
		}
	}
	return out
}

func isMalieStd(e MalieCard) bool { return strings.Contains(e.Ext.TCGL.LongFormID, "_std_") }

// fromMalie normalises a malie set file into candidates with TCGdex ids
// (matchPrintID), one per print.
func fromMalie(entries []MalieCard, set TCGdexSet) []candidate {
	var out []candidate
	for _, e := range pickMalieEntries(entries) {
		id, local := matchPrintID(set, e)
		p := malieCandidate(e)
		p.ID, p.LocalID = id, local
		out = append(out, candidate{Print: p, source: SourceMalie})
	}
	return out
}

// malieCandidate maps one export entry to a Print without an id.
func malieCandidate(e MalieCard) Print {
	p := Print{
		Category:       malieWord(e.CardType),
		HP:             int(e.HP),
		Stage:          malieWord(e.Stage),
		Retreat:        int(e.Retreat),
		RegulationMark: strings.TrimSpace(e.RegulationMark),
		Sources:        []string{SourceMalie},
	}
	for _, t := range e.Types {
		p.Types = append(p.Types, malieWord(t))
	}
	for _, tag := range e.Tags {
		if tag == "EX_LOWER" {
			p.Suffix = "ex"
		}
	}
	switch p.Category {
	case "Trainer":
		p.TrainerType = malieWord(e.Subtype)
	case "Energy":
		if e.Subtype == "SPECIAL" {
			p.EnergyType = "Special"
		} else {
			p.EnergyType = "Normal"
		}
	}
	if e.Weakness != nil {
		for _, ty := range e.Weakness.Types {
			p.Weaknesses = append(p.Weaknesses, cards.TypeValue{Type: malieWord(ty), Value: e.Weakness.Operator + strconv.Itoa(int(e.Weakness.Amount))})
		}
	}
	if e.Resistance != nil {
		for _, ty := range e.Resistance.Types {
			p.Resistances = append(p.Resistances, cards.TypeValue{Type: malieWord(ty), Value: e.Resistance.Operator + strconv.Itoa(int(e.Resistance.Amount))})
		}
	}
	t := PrintText{Name: cleanText(e.Name)}
	if from, ok := strings.CutPrefix(strings.TrimSpace(e.StageText), "Evolves from "); ok {
		t.EvolveFrom = cleanText(from)
	}
	var effects []string
	for _, x := range e.Text {
		switch x.Kind {
		case "ATTACK":
			var cost []string
			for _, c := range x.Cost {
				if c != "FREE" { // a cost-free attack; TCGdex lists no cost
					cost = append(cost, malieWord(c))
				}
			}
			var dmg cards.Damage
			if x.Damage != nil && (x.Damage.Amount != 0 || x.Damage.Suffix != "") {
				dmg = cards.Damage(strconv.Itoa(int(x.Damage.Amount)) + x.Damage.Suffix)
			}
			p.Attacks = append(p.Attacks, AttackCore{Cost: cost, Damage: dmg})
			t.Attacks = append(t.Attacks, MoveText{Name: cleanText(x.Name), Effect: cleanText(x.Text)})
		case "ABILITY":
			p.Abilities = append(p.Abilities, AbilityCore{Type: "Ability"})
			t.Abilities = append(t.Abilities, MoveText{Name: cleanText(x.Name), Effect: cleanText(x.Text)})
		case "TEXT_BOX", "EFFECT":
			// TEXT_BOX is a Trainer's or special Energy's effect; EFFECT is
			// a Pokémon's own rule text (Tera). The ex rule (RULE_BOX) and
			// the one-Supporter reminder (REMINDER) are not card effects.
			if s := cleanText(x.Text); s != "" {
				effects = append(effects, s)
			}
		}
	}
	t.Effect = strings.Join(effects, "\n\n")
	if u := e.Images.TCGL.JPG.Front; u != "" {
		p.Images = []Image{{Source: SourceMalie, URL: u}}
	}
	p.Text = map[string]PrintText{English: t}
	return p
}
