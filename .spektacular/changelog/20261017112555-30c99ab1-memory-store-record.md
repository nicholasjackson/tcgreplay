---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Memory-only playmat with portable game records

## What was built

The tcgreplay playmat no longer keeps games anywhere but memory, in every run mode (the desktop shim and daemon, `-no-daemon`, and the hosted server). Starting or importing a game replaces the current one, and restarting or redeploying the playmat starts with no game open. Game files, the saved import data beside them, the last-game pointer and the remembered decklist file are no longer written or read; files an older build left in the data directory are left untouched. The card cache is unchanged.

In place of storage, every imported game carries a **game record**: a small, versioned JSON object holding the raw game only (the player's name, the seed that fixes how hidden cards are dealt, the player's deck, the opponent list as passed, and the battle log) plus a SHA-256 check over those values. The record is returned by `import_log` and `set_opponent_deck`, and a new `get_record` tool returns it for the current game; a game started with `new_game` has none. Passing a record back to `import_log` as its `log` rebuilds the identical game, card for card at every point of the timeline. Re-indented or reflowed records are accepted unchanged; curly quotes, non-breaking spaces and Windows line ends added by a note app are undone only when the check then matches; anything else is refused with a message saying the record was damaged, so a record never restores as a different game. Records from a newer version are refused by number.

The ways to reopen earlier games are gone: the `list_games` and `load_game` tools, the board's equivalent requests and its landing-page buttons. `set_opponent_deck` acts on the current game from its record, replacing `id`/`new_id` with `discard_branches`, which is required to rebuild a game that has branches. The player's last decklist is remembered in memory for the session. The import skill offers the player the record to keep; the replay drill, numbers drill and before-you-move skills work on the game open now and ask for an import when none is; the opponent-deck skill asks before discarding branches. The design document describes the new behaviour.

Tests cover the record format (reformatting, note-app damage, real damage, versions, 64-bit seeds), a round trip of every battle log in the test corpus with and without an assumed opponent list, a pre-seeded data directory left byte-for-byte unchanged by a full session, a new manager starting empty, the tool list and schemas over both MCP transports, and the board's no-game state.

## Why it matters

The hosted playmat can now store nothing at all, while the player keeps a complete, portable record of each game wherever they like (a note, for instance) and can bring any game back exactly. Branches, notes and findings last only for the session.

## Deviations from the plan

- Record parsing is slightly more forgiving than planned within the same rule: structural decoding also retries with non-breaking spaces straightened, and value repair tries every combination of the four known substitutions. A record is still accepted only when its check matches.
- The board request's unused `game` argument was removed, game ids are no longer validated as file names, and the hosted run mode no longer keeps the manager it used only for reopening.
- Three existing assertions that only tested persistence to disk were removed, and `TestFullGameRewindAndReload` became `TestFullGameRewindAndBranch`.
- The skills check for removed tools was written alongside the skill edits; the Svelte landing page has no automated test (build, webui tests and a hosted smoke run stand in; manual check in the test plan).
