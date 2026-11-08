package cardset

import (
	"fmt"
	"slices"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// Card turns the print into the playmat's runtime card definition in one
// language. The set reference is left empty: Set.Card fills it.
//
// The card's single Image is the print's TCGdex image, as the playmat
// shows today, and a Basic Energy without one borrows the art of its type
// exactly as the playmat's card client does (cards.EnergyArt): the borrowed
// art is never stored in the built set. Which other sources the board may
// fall back to is decided by the card-set-publishing spec.
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
	}
	for _, img := range p.Images {
		if img.Source == SourceTCGdex {
			c.Image = img.URL
			break
		}
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
