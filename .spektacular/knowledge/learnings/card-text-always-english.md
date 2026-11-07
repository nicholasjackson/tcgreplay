---
tags: [cards, tcgdex, game]
---

# Card text is always English, whatever the card's printed language

The card catalogue reads every card definition from TCGdex's English API. `cards.NewClient` sets `Base: "https://api.tcgdex.net/v2/en"` (`internal/cards/cards.go`), and only tests override it (`cardBase` in `cmd/tcgreplay`).

Cards on the playmat are identified by their TCGdex id (set and number, resolved from TCG Live logs and decklists), not by printed name or language. A French or Japanese print of a card is therefore the same definition, with English `Effect`, Ability and attack text.

Consequences:
- Anything that reads card text (the `joinIf` word check, the turn wait's card text, an effect reader) only ever sees English wording, and may be written for English alone.
- A print with no English entry in TCGdex (a set that exists only in another language) has no definition text. Treat missing text as "unknown, ask", never as "the card does nothing". A Trainer with empty `Effect` is missing data, whereas an attack with empty text is normal (damage only).
