---
tags: [webui, cards, board, bench]
---

# The board's 8th-bench-slot hint never fires: `/api/card` sends no `effect`

`web/src/components/Side.svelte` offers a ninth drop target (an 8th bench slot) while a Stadium such as Area Zero Underdepths is in play. It tests `cardInfo(app.board.stadium?.def)?.effect` against `/\b8 Pokémon on (their|his or her) Bench/`. But `webui.CardInfo` (`internal/webui/server.go`, served at `/api/card`) has no `effect` field, only `text` (the card's whole `Text()` rendering), so `effect` is always undefined and the slot is never offered (found 2026-11-08).

The server side is unaffected: `game.benchWarnings` reads the Stadium's `Effect` from the card database directly, and the state allows up to `state.MaxBench` (8) at any time, so a player can still bench an eighth Pokémon by a move. To fix the hint, either add `effect` to `CardInfo`, or match the regex against `text`. Any other front-end code that reads a `cards.Card` field through `cardInfo` must check that the field is in `CardInfo`'s JSON (snake_case: `trainer_type`, `evolve_from`).
