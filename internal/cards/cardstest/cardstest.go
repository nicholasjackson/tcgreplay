// Package cardstest provides a tiny in-memory card pool for tests and the
// board test server, so they need no network and share one copy of it.
package cardstest

import (
	"context"
	"fmt"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// FakeDB serves a tiny card pool with set code TST. Each New returns a
// fresh pool, so a test may add or change cards in its own copy.
type FakeDB map[string]*cards.Card

// New returns the pool.
func New() FakeDB {
	return FakeDB{
		"tst-1":   {ID: "tst-1", Name: "Riolu", Category: "Pokemon", Stage: "Basic", HP: 70, Types: []string{"Fighting"}, Retreat: 1, Attacks: []cards.Attack{{Name: "Punch", Damage: "10"}}, Abilities: []cards.Ability{{Name: "Focus", Effect: "Once during your turn, you may draw 2 cards."}}},
		"tst-2":   {ID: "tst-2", Name: "Lucario", Category: "Pokemon", Stage: "Stage1", EvolveFrom: "Riolu", HP: 130, Types: []string{"Fighting"}, Retreat: 2, Attacks: []cards.Attack{{Name: "Aura Sphere", Damage: "130"}}},
		"tst-3":   {ID: "tst-3", Name: "Ultra Ball", Category: "Trainer", TrainerType: "Item", Effect: "You can use this card only if you discard 2 other cards from your hand. Search your deck for a Pokémon, reveal it, and put it into your hand. Then, shuffle your deck."},
		"tst-4":   {ID: "tst-4", Name: "Iono", Category: "Trainer", TrainerType: "Supporter", Effect: "Each player shuffles their hand and puts it on the bottom of their deck. Then, each player draws a card for each of their remaining Prize cards."},
		"tst-5":   {ID: "tst-5", Name: "Arena", Category: "Trainer", TrainerType: "Stadium"},
		"tst-6":   {ID: "tst-6", Name: "Band", Category: "Trainer", TrainerType: "Tool"},
		"tst-7":   {ID: "tst-7", Name: "Makuhita", Category: "Pokemon", Stage: "Basic", HP: 80, Types: []string{"Fighting"}, Retreat: 2, Attacks: []cards.Attack{{Cost: []string{"Fighting"}, Name: "Slap", Damage: "20"}, {Cost: []string{"Fighting", "Colorless"}, Name: "Body Slam", Damage: "60"}}, Weaknesses: []cards.TypeValue{{Type: "Psychic", Value: "×2"}}},
		"tst-8":   {ID: "tst-8", Name: "Deep Zone", Category: "Trainer", TrainerType: "Stadium", Effect: "Each player can have 8 Pokémon on their Bench."},
		"sve-006": {ID: "sve-006", Name: "Fighting Energy", Category: "Energy", EnergyType: "Normal"},
	}
}

func (f FakeDB) Card(_ context.Context, id string) (*cards.Card, error) {
	if c, ok := f[id]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("%s: %w", id, cards.ErrNotFound)
}
func (f FakeDB) Cached(id string) *cards.Card { return f[id] }

// Snapshot makes the pool its own card source (game.CardSource): every
// game pins the same pool.
func (f FakeDB) Snapshot() cards.DB { return f }
func (f FakeDB) ByName(_ context.Context, name string, _ ...string) (string, error) {
	for id, c := range f {
		if c.Name == name {
			return id, nil
		}
	}
	return "", cards.ErrNotFound
}
func (f FakeDB) Resolve(_ context.Context, set, num string) (string, error) {
	if set != "TST" {
		return "", cards.ErrNotFound
	}
	return "tst-" + num, nil
}
