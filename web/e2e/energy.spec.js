// Unit tests for energyProvided (src/lib/energy.js), the reading of what
// an attached Energy card provides that the board's Energy dots count.
// They need no board, so they run once, in the desktop project, and also
// read every Special Energy in the card sets (cardsets/sets).
import { test, expect } from '@playwright/test';
import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { energyProvided, ANY } from '../src/lib/energy.js';

test.beforeEach(({}, testInfo) => {
  test.skip(testInfo.project.name !== 'desktop', 'plain functions: one project is enough');
});

const C = 'colorless';

const cases = [
  // The modern wordings.
  ['Provides {C}{C} energy. Doesn\'t count as a basic Energy card.', [C, C]],
  ['As long as this card is attached to a Pokémon, it provides {F} Energy. Prevent all effects of attacks used by your opponent\'s Pokémon done to the {F} Pokémon this card is attached to.', ['fighting']],
  ['As long as this card is attached to a Pokémon, it provides {R} Energy.\n\nIf this card is discarded by an effect of an attack used by the {R} Pokémon this card is attached to, put this card into your hand.', ['fire']],
  ['As long as this card is attached to a Pokémon, it provides ColorlessColorless Energy. The attacks of the Pokémon this card is attached to do 20 less damage.', [C, C]],
  // Older wordings: type words, a space between them, a leading number.
  ['Double Colorless Energy provides ColorlessColorless Energy.', [C, C]],
  ['Double Colorless Energy provides Colorless Colorless Energy', [C, C]],
  ['Boost Energy provides ColorlessColorlessColorless Energy. The Pokémon Boost Energy is attached to can\'t retreat.', [C, C, C]],
  ['While in play, R Energy provides 2 Darkness Energy. (Doesn\'t count as a basic Energy card.)', ['darkness', 'darkness']],
  ['Warp Energy provides 1 Colorless Energy. When you attach Warp Energy from your hand to your Active Pokémon, switch it.', [C]],
  ['As long as this card is attached to a Pokémon, it provides 2 in any combination of {P} Energy and {D} Energy.', [ANY, ANY]],
  // Every type: one at a time, or as many as it says.
  ['As long as this card is attached to a Pokémon, it provides every type of Energy but provides only 1 Energy at a time.', [ANY]],
  ['While in play, Double Rainbow Energy provides every type of Energy but provides 2 Energy at a time.', [ANY, ANY]],
  ['While in play, Rainbow Energy counts as every type of basic Energy but only provides 1 Energy at a time.', [ANY]],
  // Two named types at a time; one of two at a time.
  ['Aqua Energy provides Water and Darkness Energy but provides 2 Energy at a time.', ['water', 'darkness']],
  ['As long as this card is attached to a Pokémon, it provides Fighting and Darkness Energy but provides only 1 Energy at a time, and the attacks do 20 more damage.', [ANY]],
  // The first "provides" is the ordinary one; later ones are conditions.
  ['This card provides Colorless Energy. If the Pokémon this card is attached to is a Basic Pokémon, this card provides every type of Energy but provides only 1 Energy at a time.', [C]],
  ['As long as this card is attached to a Pokémon, it provides {C} Energy.\n\nIf this card is attached to an Evolution Pokémon, it provides {C}{C}{C} Energy instead.', [C]],
  // Fairy and Dragon Energy have no colour of their own on the board.
  ['This card provides Fairy Energy only while this card is attached to a Fairy Pokémon.', [C]],
  // The board's card text: name and kind lines first.
  ['Double Colorless Energy (tst-12)\nSpecial Energy\nProvides {C}{C} energy.', [C, C]],
];

for (const [text, want] of cases) {
  test(`reads "${text.slice(0, 70).replace(/\n/g, ' ')}"`, () => {
    expect(energyProvided(text, 'psychic')).toEqual(want);
  });
}

test('without text, or without "provides", a card gives one Energy of the fallback type', () => {
  expect(energyProvided('', 'fighting')).toEqual(['fighting']);
  expect(energyProvided(null, 'water')).toEqual(['water']);
  expect(energyProvided(undefined)).toEqual([C]);
  expect(energyProvided('Fighting Energy (sve-006)\nNormal Energy', 'fighting')).toEqual(['fighting']);
  expect(energyProvided('This card has an effect but names no Energy it provides here.', 'grass')).toEqual(['grass']);
  // An unknown fallback is Colorless.
  expect(energyProvided('', 'fairy')).toEqual([C]);
});

test('a reading it cannot make sense of is one Energy, never none and never a silly count', () => {
  expect(energyProvided('This card provides.', 'metal')).toEqual(['metal']);
  expect(energyProvided('This card provides 0 Energy.', 'metal')).toEqual(['metal']);
  expect(energyProvided('This card provides 9 Colorless Energy.', 'metal')).toHaveLength(4);
});

// specials lists every Special Energy print in the card sets: its id, name
// and effect text.
function specials() {
  const dir = fileURLToPath(new URL('../../cardsets/sets/', import.meta.url));
  const out = [];
  for (const f of readdirSync(dir).filter((n) => n.endsWith('.json'))) {
    const set = JSON.parse(readFileSync(join(dir, f), 'utf8'));
    for (const c of set.prints || []) {
      if (c.category !== 'Energy' || c.energyType === 'Normal') continue;
      out.push({ id: c.id, name: c.text?.en?.name ?? '', effect: c.text?.en?.effect ?? '' });
    }
  }
  return out;
}

test('every Special Energy in the card sets provides one to three Energy, and the doubles two', () => {
  const all = specials();
  expect(all.length).toBeGreaterThan(200);
  const bad = all.filter((c) => {
    const n = energyProvided(c.effect, 'colorless').length;
    return n < 1 || n > 3;
  });
  expect(bad.map((c) => c.id)).toEqual([]);
  // Named doubles and triples read as such.
  const n = (name) => [...new Set(all.filter((c) => c.name === name).map((c) => energyProvided(c.effect).length))];
  expect(n('Double Colorless Energy')).toEqual([2]);
  expect(n('Double Turbo Energy')).toEqual([2]);
  expect(n('Twin Energy')).toEqual([2]);
  expect(n('Double Rainbow Energy')).toEqual([2]);
  expect(n('Boost Energy')).toEqual([3]);
  expect(n('Triple Acceleration Energy')).toEqual([3]);
  // The modern single-type ones give one of their type.
  for (const [name, type] of [['Rocky Fighting Energy', 'fighting'], ['Growing Grass Energy', 'grass'], ['Nitro Fire Energy', 'fire'], ['Jet Energy', C], ['Luminous Energy', ANY]]) {
    expect(all.filter((c) => c.name === name).map((c) => energyProvided(c.effect)), name).toContainEqual([type]);
  }
});
