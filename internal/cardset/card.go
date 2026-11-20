package cardset

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// Card turns the print into the playmat's runtime card definition in one
// language. The set reference is left empty: Set.Card fills it.
//
// The card carries the print's whole ordered image list (Images), the
// order the board tries them in: a TCGdex image under
// https://assets.tcgdex.net/ is a base path (the board appends the size
// file), an https address of a complete image file (a malie.io jpg, say)
// is a file entry used as is, and anything else is left out. Image is the
// first entry's address. A Basic Energy with no image borrows the art of
// its type (cards.EnergyArt): the borrowed art is never stored in the
// built set.
func (p Print) Card(lang string) (*cards.Card, error) {
	t, ok := p.Text[lang]
	if !ok {
		return nil, fmt.Errorf("print %s has no %q text", p.ID, lang)
	}
	if len(t.Attacks) != len(p.Attacks) || len(t.Abilities) != len(p.Abilities) {
		return nil, fmt.Errorf("print %s: %q text does not line up with the card", p.ID, lang)
	}
	c := &cards.Card{
		ID:             p.ID,
		LocalID:        p.LocalID,
		Name:           t.Name,
		Category:       p.Category,
		HP:             p.HP,
		Types:          slices.Clone(p.Types),
		Stage:          p.Stage,
		EvolveFrom:     t.EvolveFrom,
		Suffix:         p.Suffix,
		Weaknesses:     slices.Clone(p.Weaknesses),
		Resistances:    slices.Clone(p.Resistances),
		Retreat:        p.Retreat,
		TrainerType:    p.TrainerType,
		EnergyType:     p.EnergyType,
		Effect:         t.Effect,
		RegulationMark: p.RegulationMark,
		PlayedAsPair:   p.PlayedAsPair,
	}
	c.Images = cardImages(p.Images)
	if len(c.Images) > 0 {
		c.Image = c.Images[0].URL
	}
	cards.EnergyArt(c)
	for i, a := range p.Abilities {
		c.Abilities = append(c.Abilities, cards.Ability{Type: a.Type, Name: t.Abilities[i].Name, Effect: t.Abilities[i].Effect})
	}
	for i, a := range p.Attacks {
		c.Attacks = append(c.Attacks, cards.Attack{Cost: slices.Clone(a.Cost), Name: t.Attacks[i].Name, Effect: t.Attacks[i].Effect, Damage: a.Damage})
	}
	return c, nil
}

// tcgdexImages is where TCGdex serves card images; an address under it
// with no file extension is a base path.
const tcgdexImages = "https://assets.tcgdex.net/"

// cardImages turns a print's image sources, in order, into the image list
// the board can show, leaving out any it cannot.
func cardImages(imgs []Image) []cards.ImageRef {
	var out []cards.ImageRef
	for _, img := range imgs {
		switch {
		case !strings.HasPrefix(img.URL, "https://"):
		case cards.ImageIsFile(img.URL):
			out = append(out, cards.ImageRef{URL: img.URL, File: true})
		case img.Source == SourceTCGdex && strings.HasPrefix(img.URL, tcgdexImages):
			out = append(out, cards.ImageRef{URL: img.URL})
		}
	}
	return out
}

// Card returns the print with the given id as a runtime card definition in
// one language, with its set reference filled in (the set's name in that
// language, else in English).
func (s *Set) Card(id, lang string) (*cards.Card, error) {
	p, ok := s.Print(id)
	if !ok {
		return nil, fmt.Errorf("set %s has no print %s: %w", s.Set.ID, id, cards.ErrNotFound)
	}
	c, err := p.Card(lang)
	if err != nil {
		return nil, err
	}
	name := s.Set.Text[lang].Name
	if name == "" {
		name = s.Set.Text[English].Name
	}
	c.Set = cards.SetRef{ID: s.Set.ID, Name: name}
	return c, nil
}

// Upstream source names, as recorded in Print.Sources and Image.Source.
const (
	SourceTCGdex = "tcgdex"
	SourceMalie  = "malie"
)
