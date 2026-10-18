---
created_date: "2026-10-17"
document_status: draft
project: tcgreplay
spec: 20261017112555-30c99ab1-memory-store-record
plan: 20261017112555-30c99ab1-memory-store-record
---

# Memory-only playmat with portable game records

The playmat now keeps games only for the session, on the desktop and the hosted server alike: restarting it starts with no game open, and earlier games can no longer be listed or reopened. Instead, every imported game comes with a game record, a piece of JSON text you can keep in your notes and paste back to bring that exact game back, card for card. A record damaged on the way is repaired if a note app only curled its quotes or changed its spaces, and otherwise refused, never restored as a different game. Branches, notes and findings last only for the session.

> Derived from project tcgreplay (file), spec/plan 20261017112555-30c99ab1-memory-store-record. See the project-level record for the full feature.

## What changed in this repo

- `internal/game/record.go` (new): the game record format, its check, rendering and parsing with note-app repair.
- `internal/store`: the event store is memory-only; file create, open and writing are removed.
- `internal/game`: imports build and keep the record on the game and accept a record as input; the manager holds the current game and the last decklist in memory, with no list, load or resume; `SetOpponentDeck` rebuilds the current game from its record and needs `discardBranches` to drop branches.
- `internal/mcpserver`: `import_log` and `set_opponent_deck` return `record`; new `get_record`; `list_games` and `load_game` removed; `set_opponent_deck` takes `discard_branches` instead of `id`/`new_id`.
- `internal/webui` and `web/`: the board's list/load requests, its `games` state field and the landing page's stored-game buttons are removed; the landing page asks for an import or a new game.
- `cmd/tcgreplay`: no run mode reopens a game at start-up; the `-data` help no longer mentions game files.
- `skills/`: the import skill offers the record and restores pasted records; the drills use the open game; the opponent-deck skill asks before discarding branches; a test keeps removed tools out of all skills.
- `TCG Review Playmat Design and Build Plan.md`: describes memory-only games and records.
- Tests: record format, corpus-wide round trip, nothing-written and restart-forgets checks, tool list and schema, board no-game state.

## Why

The hosted playmat must store no games, and the desktop follows the same rule. The record gives the player a way to keep and restore any game without the playmat storing anything.
