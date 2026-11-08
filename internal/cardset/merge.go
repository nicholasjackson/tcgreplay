package cardset

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// disagreement is a field both sources have with different values.
type disagreement struct {
	Field  string         // JSON Pointer into the print, "/hp", "/text/en/effect"
	Values map[string]any // source -> its value
}

// merged is one print built from every source that lists it, with the
// fields the sources disagree on.
type merged struct {
	Print
	disagreements []disagreement
}

// merge combines TCGdex's and malie's candidates into one definition per
// print over the union of both sources' prints. TCGdex wins every field it
// has; malie fills the rest. A field both have with different values (other
// than in spacing, quote style or energy-symbol spelling) is recorded as a
// disagreement. Images list TCGdex's first, then malie's. The result is
// sorted by collector number.
func merge(tcgdex, malie []candidate) []merged {
	byID := map[string]*candidate{}
	for i := range malie {
		byID[malie[i].ID] = &malie[i]
	}
	seen := map[string]bool{}
	var out []merged
	for i := range tcgdex {
		t := &tcgdex[i]
		seen[t.ID] = true
		if m, ok := byID[t.ID]; ok {
			out = append(out, mergePrint(t.Print, m.Print))
		} else {
			out = append(out, merged{Print: t.Print})
		}
	}
	for i := range malie {
		if !seen[malie[i].ID] {
			seen[malie[i].ID] = true
			out = append(out, merged{Print: malie[i].Print})
		}
	}
	ps := make([]Print, len(out))
	for i := range out {
		ps[i] = out[i].Print
	}
	order := map[string]int{}
	SortPrints(ps)
	for i, p := range ps {
		order[p.ID] = i
	}
	slices.SortFunc(out, func(a, b merged) int { return order[a.ID] - order[b.ID] })
	return out
}

// fieldMerger fills one print from two and records disagreements.
type fieldMerger struct {
	out []disagreement
}

func (f *fieldMerger) disagree(field string, t, m any) {
	f.out = append(f.out, disagreement{Field: field, Values: map[string]any{SourceTCGdex: t, SourceMalie: m}})
}

// str merges a text field compared by key.
func (f *fieldMerger) str(field, t, m string, key func(string) string) string {
	switch {
	case t == "":
		return m
	case m != "" && key(t) != key(m):
		f.disagree(field, t, m)
	}
	return t
}

// num merges a number field; 0 means the source does not have it.
func (f *fieldMerger) num(field string, t, m int) int {
	switch {
	case t == 0:
		return m
	case m != 0 && t != m:
		f.disagree(field, t, m)
	}
	return t
}

// list merges a list of words.
func (f *fieldMerger) list(field string, t, m []string) []string {
	switch {
	case len(t) == 0:
		return slices.Clone(m)
	case len(m) != 0 && listKey(t) != listKey(m):
		f.disagree(field, t, m)
	}
	return t
}

// typeValues merges weaknesses or resistances.
func (f *fieldMerger) typeValues(field string, t, m []cards.TypeValue) []cards.TypeValue {
	switch {
	case len(t) == 0:
		return slices.Clone(m)
	case len(m) != 0 && typeValueKey(t) != typeValueKey(m):
		f.disagree(field, typeValueStrings(t), typeValueStrings(m))
	}
	return t
}

func listKey(l []string) string {
	k := make([]string, len(l))
	for i, s := range l {
		k[i] = compareKey(s)
	}
	return strings.Join(k, "|")
}

func typeValueStrings(tv []cards.TypeValue) []string {
	out := make([]string, len(tv))
	for i, v := range tv {
		out[i] = v.Type + " " + v.Value
	}
	return out
}

// typeValueKey compares weaknesses and resistances ignoring the letter x
// for × in the value: TCGdex writes "x2" in some sets where malie writes
// "×2".
func typeValueKey(tv []cards.TypeValue) string {
	k := make([]string, len(tv))
	for i, v := range tv {
		k[i] = compareKey(v.Type) + " " + damageKey(v.Value)
	}
	return strings.Join(k, "|")
}

// mergePrint combines one print listed by both sources.
func mergePrint(t, m Print) merged {
	var f fieldMerger
	tt, mt := t.Text[English], m.Text[English]
	out := Print{
		ID:             t.ID,
		LocalID:        t.LocalID,
		Category:       f.str("/category", t.Category, m.Category, compareKey),
		HP:             f.num("/hp", t.HP, m.HP),
		Types:          f.list("/types", t.Types, m.Types),
		Stage:          f.str("/stage", t.Stage, m.Stage, compareKey),
		Suffix:         f.str("/suffix", t.Suffix, m.Suffix, compareKey),
		Weaknesses:     f.typeValues("/weaknesses", t.Weaknesses, m.Weaknesses),
		Resistances:    f.typeValues("/resistances", t.Resistances, m.Resistances),
		Retreat:        f.num("/retreat", t.Retreat, m.Retreat),
		TrainerType:    f.str("/trainerType", t.TrainerType, m.TrainerType, compareKey),
		EnergyType:     f.str("/energyType", t.EnergyType, m.EnergyType, compareKey),
		RegulationMark: f.str("/regulationMark", t.RegulationMark, m.RegulationMark, compareKey),
	}
	text := PrintText{
		Name:       f.str("/text/en/name", tt.Name, mt.Name, nameKey),
		EvolveFrom: f.str("/text/en/evolveFrom", tt.EvolveFrom, mt.EvolveFrom, nameKey),
		Effect:     f.str("/text/en/effect", tt.Effect, mt.Effect, compareKey),
	}

	// Attacks, by position. When one source lists moves the other does not
	// (an attack in one, an ability in the other), the gap is filled but
	// still recorded: the print would otherwise carry the same move twice.
	switch {
	case len(t.Attacks) == 0:
		if len(m.Attacks) != 0 && len(t.Abilities) != len(m.Abilities) {
			f.disagree("/attacks", moveNames(tt.Attacks), moveNames(mt.Attacks))
		}
		out.Attacks, text.Attacks = slices.Clone(m.Attacks), slices.Clone(mt.Attacks)
	case len(m.Attacks) != 0 && len(m.Attacks) != len(t.Attacks):
		f.disagree("/attacks", moveNames(tt.Attacks), moveNames(mt.Attacks))
		fallthrough
	case len(m.Attacks) == 0:
		out.Attacks, text.Attacks = slices.Clone(t.Attacks), slices.Clone(tt.Attacks)
	default:
		for i := range t.Attacks {
			p := "/attacks/" + strconv.Itoa(i)
			tp := "/text/en/attacks/" + strconv.Itoa(i)
			out.Attacks = append(out.Attacks, AttackCore{
				Cost:   f.list(p+"/cost", t.Attacks[i].Cost, m.Attacks[i].Cost),
				Damage: cards.Damage(f.str(p+"/damage", string(t.Attacks[i].Damage), string(m.Attacks[i].Damage), damageKey)),
			})
			text.Attacks = append(text.Attacks, MoveText{
				Name:   f.str(tp+"/name", tt.Attacks[i].Name, mt.Attacks[i].Name, compareKey),
				Effect: f.str(tp+"/effect", tt.Attacks[i].Effect, mt.Attacks[i].Effect, compareKey),
			})
		}
	}

	// Abilities, by position.
	switch {
	case len(t.Abilities) == 0:
		if len(m.Abilities) != 0 && len(t.Attacks) != len(m.Attacks) {
			f.disagree("/abilities", moveNames(tt.Abilities), moveNames(mt.Abilities))
		}
		out.Abilities, text.Abilities = slices.Clone(m.Abilities), slices.Clone(mt.Abilities)
	case len(m.Abilities) != 0 && len(m.Abilities) != len(t.Abilities):
		f.disagree("/abilities", moveNames(tt.Abilities), moveNames(mt.Abilities))
		fallthrough
	case len(m.Abilities) == 0:
		out.Abilities, text.Abilities = slices.Clone(t.Abilities), slices.Clone(tt.Abilities)
	default:
		for i := range t.Abilities {
			p := "/abilities/" + strconv.Itoa(i)
			tp := "/text/en/abilities/" + strconv.Itoa(i)
			out.Abilities = append(out.Abilities, AbilityCore{Type: f.str(p+"/type", t.Abilities[i].Type, m.Abilities[i].Type, compareKey)})
			text.Abilities = append(text.Abilities, MoveText{
				Name:   f.str(tp+"/name", tt.Abilities[i].Name, mt.Abilities[i].Name, compareKey),
				Effect: f.str(tp+"/effect", tt.Abilities[i].Effect, mt.Abilities[i].Effect, compareKey),
			})
		}
	}

	out.Images = append(slices.Clone(t.Images), m.Images...)
	out.Sources = unionSorted(t.Sources, m.Sources)
	out.Text = map[string]PrintText{English: text}
	return merged{Print: out, disagreements: f.out}
}

// damageKey compares damage ignoring spaces and the letter x for ×.
func damageKey(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(compareKey(s), " ", ""), "x", "×")
}

func moveNames(ms []MoveText) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return out
}

func unionSorted(a, b []string) []string {
	out := append(slices.Clone(a), b...)
	slices.Sort(out)
	return slices.Compact(out)
}

// String describes a disagreement for logs.
func (d disagreement) String() string {
	return fmt.Sprintf("%s: tcgdex %v, malie %v", d.Field, d.Values[SourceTCGdex], d.Values[SourceMalie])
}
