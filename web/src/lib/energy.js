// What an attached Energy card provides, read from its text: the board
// draws one dot per Energy provided, not one per card, so a Double
// Colorless Energy shows two. Pure functions, no board state, so the
// browser suite can test them on their own (e2e/energy.spec.js) and run
// them over every Special Energy in the card sets.

// TYPES are the Energy types the board has a colour for; Fairy and Dragon
// Energy show as Colorless. ANY is a dot for Energy of every type
// (Rainbow, Prism, Double Dragon), drawn in every type's colour.
export const TYPES = ['grass', 'fire', 'water', 'lightning', 'psychic', 'fighting', 'darkness', 'metal', 'colorless'];
export const ANY = 'any';

// SYMBOLS are the type letters inside {X} in card text.
const SYMBOLS = { G: 'grass', R: 'fire', W: 'water', L: 'lightning', P: 'psychic', F: 'fighting', D: 'darkness', M: 'metal', C: 'colorless', Y: 'colorless', N: 'colorless' };
const WORDS = {
  grass: 'grass',
  fire: 'fire',
  water: 'water',
  lightning: 'lightning',
  psychic: 'psychic',
  fighting: 'fighting',
  darkness: 'darkness',
  metal: 'metal',
  colorless: 'colorless',
  fairy: 'colorless',
  dragon: 'colorless',
};
const NUMBERS = { one: 1, two: 2, three: 3, four: 4 };

// MAX caps the dots one card gives: no card provides more than three (Boost
// Energy, Triple Acceleration Energy), so anything above four is a misread.
const MAX = 4;

const TYPE_RE = /\{([A-Z])\}|(grass|fire|water|lightning|psychic|fighting|darkness|metal|colorless|fairy|dragon)/gi;
const AT_A_TIME = /provides? (?:only )?(\d+|one|two|three|four) energy at a time/i;

// count reads a number ("2", "two"), or 0 for none.
function count(word) {
  if (!word) return 0;
  const w = word.toLowerCase();
  return NUMBERS[w] ?? (Number(w) || 0);
}

// energyProvided lists the Energy types one card provides, one entry per
// Energy, from its text (or effect). The first "provides" in the text is
// the card's ordinary provision (later ones are conditions: "If this card
// is attached to an Evolution Pokémon, it provides {C}{C}{C}"):
//
//   - "provides {C}{C} energy", "provides ColorlessColorless Energy",
//     "provides Colorless Colorless Energy": each {X} symbol or type word
//     before the word Energy is one Energy;
//   - "provides 2 Darkness Energy": a leading number repeats the type;
//     "provides 2 in any combination of {P} Energy and {D} Energy" gives
//     that many dots of every type;
//   - "provides every type of Energy but provides only 1 Energy at a
//     time" (and "counts as every type"): one dot of every type, or as
//     many as "N Energy at a time" says; "Water and Darkness Energy but
//     provides 2 Energy at a time" gives one of each.
//
// Anything else (no text, no "provides", a clause it cannot read) gives
// one dot of fallback, the type the card's name suggests.
export function energyProvided(text, fallback = 'colorless') {
  const one = [TYPES.includes(fallback) || fallback === ANY ? fallback : 'colorless'];
  if (!text) return one;
  // The sentence holding the first "provides" (sentences end at a full
  // stop followed by a space or the end).
  const at = text.search(/\bprovides?\b/i);
  if (at < 0) return one;
  const start = Math.max(text.lastIndexOf('. ', at), text.lastIndexOf('\n', at)) + 1;
  const stop = text.slice(at).search(/\.(\s|$)/);
  const sentence = text.slice(start, stop < 0 ? undefined : at + stop);
  const after = sentence.slice(at - start).replace(/^provides?\s*/i, '');
  const timed = count(AT_A_TIME.exec(sentence)?.[1]);

  if (/every type|all types|any type/i.test(sentence)) return clamp(Array(timed || 1).fill(ANY));

  // The clause names what it provides, up to the word Energy.
  const end = after.search(/energy/i);
  const clause = end < 0 ? after : after.slice(0, end);
  const lead = /^(\d+|one|two|three|four)\b/i.exec(clause.trim());
  const types = [...clause.matchAll(TYPE_RE)].map((m) => (m[1] ? SYMBOLS[m[1].toUpperCase()] : WORDS[m[2].toLowerCase()])).filter(Boolean);

  if (lead) {
    const n = count(lead[1]);
    if (n < 1) return one;
    if (/any combination/i.test(sentence)) return clamp(Array(n).fill(ANY));
    if (types.length) return clamp(Array(n).fill(types[0]));
    return n === 1 ? one : clamp(Array(n).fill(one[0]));
  }
  if (!types.length) return one;
  // "Water and Darkness Energy but provides 2 Energy at a time".
  if (timed) {
    if (/\b(and|or)\b/i.test(clause)) return clamp(timed === types.length ? types : Array(timed).fill(timed === 1 ? ANY : types[0]));
    return clamp(Array(timed).fill(types[0]));
  }
  return clamp(types);
}

// clamp keeps a reading between one and MAX Energy.
function clamp(list) {
  return list.length ? list.slice(0, MAX) : ['colorless'];
}
