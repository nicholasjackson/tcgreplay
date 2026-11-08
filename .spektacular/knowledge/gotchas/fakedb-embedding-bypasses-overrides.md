---
tags: [game, cards, testing]
---

# A test type embedding cardstest.FakeDB loses its overrides as a card source

`cardstest.FakeDB` has a `Snapshot() cards.DB` method returning itself, so it
satisfies `game.CardSource` and can be passed straight to `game.NewManager`.

A test type that embeds `FakeDB` to override `Card`, `Cached` or `ByName` (for
example `slowDB` in `internal/game/cardlock_test.go` or `artFirstDB` in
`internal/webui/borrowed_art_test.go`) inherits that promoted `Snapshot()`. It
compiles when passed to `game.NewManager`, but every game then pins the bare
embedded `FakeDB`, silently skipping the wrapper's overrides. The test passes or
fails for the wrong reason.

Wrap such types explicitly: `game.NewManager(dir, game.Static{DB: db})`.
`game.Static` gives the wrapper itself as each game's database.
