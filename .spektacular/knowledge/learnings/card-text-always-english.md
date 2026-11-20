---
tags: [cards, tcgdex, game, catalog]
---

# Card text is always English, whatever the card's printed language

The card catalogue converts every published print with its English text: the loader calls `cardset.Print.Card("en")` (`internal/catalog/snapshot.go`), and the card-set import writes English only (`text.en`, required for every print; other languages may be added by hand and are published but never shown).

Cards on the playmat are identified by their TCGdex id (set and number, resolved from TCG Live logs and decklists), not by printed name or language. A French or Japanese print of a card is therefore the same definition, with English `Effect`, Ability and attack text.

Consequences:
- Anything that reads card text (the `joinIf` word check, the turn wait's card text, an effect reader) only ever sees English wording, and may be written for English alone.
- A print with no English text upstream (a set that exists only in another language) is not in the built sets, so it has no definition. Treat missing text as "unknown, ask", never as "the card does nothing". A Trainer with empty `Effect` is missing data, whereas an attack with empty text is normal (damage only).
