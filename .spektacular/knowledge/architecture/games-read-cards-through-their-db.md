---
tags: [game, cards, webui, mcp, catalog]
---

# Games read card definitions through their own card database, on every read

A game never stores card definitions. `state.Card` holds only the def id (the
TCGdex id) and a name (`internal/state/types.go`), events carry only def ids,
and a game record is text that is re-resolved when it is imported again
(`internal/game/record.go`, `import.go`). Every time the game needs a card's
facts (HP, evolves-from, a Trainer's effect, an attack's cost or text) it calls
`g.db.Cached(def)` (`Game.card`, `Game.hp` in `internal/game/game.go`) on the
`cards.DB` that `game.Load` was given. `g.db` is set once in `Load` and never
changed, and `Game.DB()` returns it.

How a game gets its database (snapshot pinning):

- `game.NewManager(dir, src)` takes a `game.CardSource` (`Snapshot() cards.DB`):
  the playmat's `*catalog.Loader`, a fixed database wrapped in
  `game.Static{DB: db}`, or `cardstest.FakeDB` (its own source). nil means no
  database.
- `Manager.New` and `Manager.Import` (and so `SetOpponentDeck` and reopening a
  record) call the source **once**, at the start, and use that one snapshot both
  to resolve the decklists and the log (`BuildCards`, `parseDeck`, the
  `DefOf`/`DefOfCode` closures) and as the game's database in `Load`. So one game
  never mixes two catalogue versions, and an open game keeps the catalogue it
  started with however often the loader swaps in a newer one.
- Paths outside the game read through `Manager.CardDB()`: the open game's own
  snapshot, or the source's current one with no game open. These are the
  `card_text` MCP tool (`internal/mcpserver/server.go`), the board's `/api/card`
  and its borrowed art (`internal/webui/server.go`). Any new path that shows card
  data must do the same, never reach for "the latest" catalogue while a game is
  open.

Consequences:

- **`openText` points into the cached card.** `play_trainer`, `use_ability` and
  `attack` keep a pointer to the card's effect text (`verbs.go`) for the open
  action. A database that mutates a cached `*cards.Card` in place would change
  the text under an open action. Return immutable cards (catalogue snapshots
  never change once built).
- A game never fetches a card: a def its snapshot lacks stays unknown. There is
  no background warming or prefetch any more.
- A nil source is normal in tests (`import_test`, `record_roundtrip_test`,
  the MCP round-trip tests): every lookup then answers nothing, and cards show by
  name only.
