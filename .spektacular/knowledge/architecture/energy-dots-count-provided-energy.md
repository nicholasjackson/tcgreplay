---
tags: [board, webui, cards, energy]
---

# The board's Energy dots count the Energy each card provides, read from its text

A Pokémon's Energy dots (its Bench readout, the Active panel, the menus' headers, the phone) are one per Energy *provided*, not one per attached card, and the Active panel's "N Energy" and every `aria-label` use the same count. A Double Colorless Energy is two dots. A Special Energy's dots are ringed (an outline in `mat-ink`, or `ink` in a menu), so they read apart from Basic ones.

How it works (`web/src/lib`):

- `isSpecialEnergy(card)` (`actions.js`) uses the card data's `energy_type` ("Special" or "Normal") once loaded, and until then (or when the data has none) the name: anything but a plain Basic name such as "Fighting Energy" counts as Special. Some sets list a Special Energy with no `energyType`, and old Darkness and Metal Special Energy share their name with the Basic cards, so the data must win once it arrives.
- `energyProvided(text, fallback)` (`energy.js`, pure, no imports) reads the card's text, which is `CardInfo.Text` (`cardInfo(def).text`: the name line, the kind line, then the effect). Only the first sentence holding "provides" counts, because later ones are conditions ("If this card is attached to an Evolution Pokémon, it provides {C}{C}{C} Energy instead"). Within that sentence:
  - each `{X}` symbol or type word before the word Energy is one Energy ("{C}{C}", "ColorlessColorless", "Colorless Colorless");
  - a leading number repeats the type ("provides 2 Darkness Energy"), and with "any combination" gives that many dots of every type;
  - "every type" (or "counts as every type") gives one dot of every type, or as many as "N Energy at a time" says;
  - two named types "but provides 2 Energy at a time" give one of each (Aqua Energy).
  Anything unreadable is one dot of `energyType(card)`, never zero, and readings are capped at four. Fairy and Dragon show as Colorless, and every type is the `any` dot (a conic gradient of the `energy-*` tokens).
- `providedEnergy(stack)` turns a stack's Energy into `{key, type, special}` dots. A Basic Energy is always one dot of its type and never parsed.

`web/e2e/energy.spec.js` unit-tests the parser (desktop project only) and runs it over every Special Energy in `cardsets/sets/*.json`. When this was written, 249 prints read as 1 to 3 Energy (209 one, 33 two, 7 three); the doubles and Boost or Triple Acceleration are pinned. A new set with odd wording fails that test rather than drawing a silly count. The board test pool has a Double Colorless Energy (tst-12, `webtest.DoubleColorless`, p1-58 and p2-58). It took the place of the last Fighting Energy, so the decks still shuffle exactly as before.
