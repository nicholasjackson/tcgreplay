---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Plan: 20261017112555-30c99ab1-memory-store-record

<!-- Metadata -->
<!-- Created: 2026-10-17T15:05:36Z -->
<!-- Commit: 77e6a6a -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

Makes the playmat keep games only for the session, on the desktop and the hosted server alike, and hands the assistant a complete, portable record of every imported game instead: the battle log, the player's name, both decklists and the value that fixes how hidden cards are dealt, as JSON text. The player keeps records wherever they like (a note, for instance) and brings any game back exactly, card for card, by importing its record; a record damaged on the way is repaired or refused, never restored as a different game. This lets the hosted playmat store nothing at all, removes the "list and load saved games" path everywhere, and keeps the drills working on the game open now.

## Conventions

No project conventions apply to this feature: the `tcgreplay` knowledge store holds no entries (the always-applied load for the repo returned nothing, and topic searches for store, persistence, import, record, seed and mcp found no entries). The plan follows the patterns already in the code instead: the repo's vet-and-test Makefile target, standard-library `testing` beside the code with the `testdata/` battle-log corpus, doc comments on every exported identifier, and the skills format test (`skills/skills_test.go`), which forbids naming a specific assistant or player in a skill.

## Architecture & Design Decisions

All work lands in the single registered repo, `tcgreplay` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`), and applies identically to every run mode (shim/daemon, `-no-daemon`, and the `-hosted` mode added by the web-mcp-docker plan), because all of them build the same game manager through `newServer` (`cmd/tcgreplay/main.go:100-106`). The game manager stops using files for games: every game, imported or started with `new_game`, is held in the event store's existing in-memory mode (`store.NewMemory`, `internal/store/store.go:110-116`), whose writes are already no-ops without a file. The store's file mode (`Create`, `Open`) is removed, together with the manager's game-file plumbing (`List`, `Load`, `Resume`, `games/`, `current.txt`) and the import's side files (`<id>.import.json`, `last-deck.txt`). Starting or importing a game replaces the current one, which is simply dropped; restarting the process starts with no game open. Files already in the data directory are left exactly where they are and never opened. The data directory keeps only non-game files (the process log, the daemon's socket and lock, the assistant label); the card cache is unchanged.

In place of storage, each imported game carries its **game record**: a small, versioned JSON object holding exactly the raw game — the battle log text, the player's name (as detected or given), the player's decklist, the opponent's list as passed, and the seed the importer used — plus a checksum over that content. The import already gathers all of these (`internal/game/import.go:49-135`); the record is that request, completed and held in memory on the `Game` rather than written beside a file. Because the importer's only randomness is a PCG seeded from that seed (`internal/importer/importer.go:72-75`, `:209`), feeding a record back through `import_log` reproduces the identical game: every event, card identity and position on the imported timeline. Branches, notes and findings are deliberately not in the record (spec constraint), so they last only for the session. Holding the record on the game, not as a single manager field, keeps it attached when per-user-games later looks games up per account.

The record is returned in three places: the `import_log` result, the `set_opponent_deck` result (which now rebuilds the current game from its in-memory record with the new list), and a new `get_record` tool for the current game. A game started with `new_game` has no record, and `get_record` says so. Restoring goes through `import_log`, as the spec prefers: its `log` argument accepts either a TCG Live battle log, as today, or a record pasted back as text (a log never starts with `{`). Records survive a trip through a note because the checksum covers field values, not JSON layout, so reflowed whitespace between tokens is harmless; a value altered in transit (curly quotes, non-breaking spaces, CRLF) fails the check, the server retries with exactly those known substitutions undone, and accepts only if the check then matches. Anything else is rejected with a message saying the record was damaged, so a record is never silently restored as a different game. The player's last decklist is remembered in the manager's memory for the session, so a later import can still omit `deck`.

`list_games` and `load_game` leave the MCP tool list, and the board loses its own list/load path (the landing page's stored-game buttons and the `list_games`/`load_game` board verbs), so the only way a game comes back is importing its record. `set_opponent_deck` loses `id` and `new_id` (there is no other game to name, and no way to keep the old one) and refuses to rebuild a game with branches unless the caller passes `discard_branches: true`. The web-mcp-docker plan's hosted switches are reused unchanged: `BoardURL` still adds `board` to the remaining game-opening results (`new_game`, `import_log`, `set_opponent_deck`), and the hosted mode's "reopen the last game" call disappears with `Resume`. The three drill skills work from the game open now and ask for an import when none is open; the import and opponent-deck skills explain the record and the new rebuild rule. This beats keeping files in a wiped temp directory, keeping file mode dormant in the store, or putting the full event log in the record (which breaks the spec's "raw game only" constraint); the evidence is in `research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Event store (changed, `internal/store`)** — owns one game's event tree (events, branches, actions, notes, read audit). It becomes memory-only: the file-backed create and open paths are removed, so nothing in the process can write or read a game file. Every other behaviour (append, branch, checkout, annotate) is unchanged, which is why the game engine above it needs no change beyond how a log is created.
- **Game record (new, in `internal/game`)** — owns the record format: building a record from a finished import (log, player name, deck, opponent list as passed, seed), sealing it with its checksum, rendering it as JSON, and reading one back from pasted text, including recognising that the text is a record, undoing the known note-app substitutions and refusing a record whose check does not match or whose version is newer than supported. It is a value type with no I/O, used by the game manager and the MCP tool server; keeping it in the game package beside the import request it extends avoids a new package for one small type.
- **Game (changed, `internal/game`)** — gains the record it was imported from (none for a `new_game` game), so the record travels with the game wherever it is held. Today's single current game, and per-user-games' later per-account games, both carry it for free.
- **Game manager (changed, `internal/game`)** — owns the current game for the session, entirely in memory. It creates play and imported games on memory logs, replaces the current game on every new game or import, remembers the player's last decklist in memory for the next import, rebuilds the current imported game with a new opponent list from its record, and answers "what is the current game's record". It no longer lists, loads, resumes or writes games, and leaves any files already in the data directory alone. It still keeps the assistant label, which is not game storage.
- **MCP tool server (changed, `internal/mcpserver`)** — exposes the record to the assistant: `import_log` accepts a record as well as a battle log and returns the record; `set_opponent_deck` acts on the current game, returns the updated record, and takes an explicit opt-in to discard branches; a new `get_record` returns the current game's record or says it has none. `list_games` and `load_game` are removed. The web-mcp-docker plan's `board` link stays on the remaining game-opening results; the "no game" message points at importing instead of loading.
- **Board server and board UI (changed, `internal/webui`, `web/`)** — the board stops offering stored games: the list and load verbs and the landing page's game buttons go, and the landing text asks for an import or a new game instead. Live play on the current game is untouched.
- **Command run modes (changed, `cmd/tcgreplay`)** — every mode (shim/daemon, in-process, hosted) stops reopening the last game at start-up, so each session starts empty. The data-directory flag's help text stops calling it the home of game files.
- **Skills (changed, `skills/`)** — the replay drill, numbers drill and before-you-move skills work from the game open now and ask for an import (a battle log or a saved record) when none is open, with no mention of listing or loading games. The import skill tells the player the import returns a record they can keep in their notes and paste back to restore the game exactly, and that branches and notes last only for the session. The opponent-deck skill explains the new rebuild rule in place of `new_id`.
- **Design document (changed)** — the runtime and import sections describe memory-only games, the record and its restore, and drop saved games, `.import.json` and "reopen the last game".

## Data Structures & Interfaces

One new persisted-by-the-player format (the game record) and a handful of changed in-process and tool contracts. No file format is written by the playmat any more: the game JSONL, `<id>.import.json`, `current.txt` and `last-deck.txt` stop being produced or read. The card cache and the board's websocket messages keep their shapes, except that the board's state message no longer carries a `games` list.

**Game record (new, the portable format)** — the complete raw game, as JSON text the player can keep anywhere and paste back. Small fields first so a note shows them above the long log; the marker field both identifies a record and versions it; the seed is a decimal string so 64-bit seeds survive JSON tools that use doubles; `check` is a SHA-256 over the canonical content (every field but `check`, in a fixed order), so JSON layout does not matter but any change to a value does.

```json
{
  "tcgreplay_record": 1,
  "me": "PlayerName",
  "seed": "11400714819323198485",
  "deck": "Pokémon: 12\n4 Riolu MEG 76\n…",
  "opp_deck": "Pokémon: 12\n4 Abra\n…",
  "log": "Setup\nPlayerName chose heads for the opening coin flip.\n…",
  "check": "sha256:9f2c…"
}
```

```go
// Record is the portable form of an imported game: enough to rebuild it exactly.
type Record struct {
    Version int    `json:"tcgreplay_record"`
    Me      string `json:"me"`
    Seed    uint64 `json:"seed,string"`
    Deck    string `json:"deck"`
    OppDeck string `json:"opp_deck,omitempty"`
    Log     string `json:"log"`
    Check   string `json:"check"`
}

// ParseRecord reports whether text is a record and, if so, returns it verified.
// Damage that undoing known note-app substitutions repairs is accepted; any other
// mismatch is ErrRecordDamaged.
func ParseRecord(text string) (rec Record, isRecord bool, err error)
```

**Import request and result (changed, game manager)** — `ImportRequest` keeps its fields; its `Log` may now hold a record's text, in which case the deck, opponent list, name and seed come from the record and must not also be given. `ImportResult` gains the record of the game it built.

```go
type ImportResult struct {
    Game     *Game
    Report   importer.Report
    Warnings []string
    Record   Record // new
}
```

**Game (changed)** — carries its record; a game started with `new_game` has none.

```go
func (g *Game) Record() (Record, bool)
```

**Game manager (changed)** — memory-only. Removed: `List`, `Load`, `Resume`, `GameInfo`. Changed and new:

```go
// SetOpponentDeck rebuilds the current imported game from its record with a new list.
func (m *Manager) SetOpponentDeck(ctx context.Context, oppDeck string, discardBranches bool) (*ImportResult, error)

// Record returns the current game's record; an error if no game is open or it has none.
func (m *Manager) Record() (Record, error)
```

**Event store (changed)** — `Create`, `Open` and the log's file handle and path are removed; `NewMemory` is the only constructor.

**MCP tools (changed contract seen by the assistant)**

| Tool | Change |
|---|---|
| `import_log` | `log` accepts a battle log **or** a game record; result gains `record` (and keeps `board` when hosted). |
| `set_opponent_deck` | Inputs `id` and `new_id` removed; `discard_branches` (bool) added. Acts on the current game; result gains `record`. |
| `get_record` (new) | No input. Returns the current game's record, or an error saying no game is open or that the game has no record. |
| `list_games`, `load_game` | Removed. |
| `new_game` | Unchanged; its game has no record. |

```json
{ "game": "import-20261017-153000-ledjo23", "head": 412, "mode": "review",
  "report": { "…": "…" }, "warnings": [], "record": { "tcgreplay_record": 1, "…": "…" },
  "board": "https://playmat.example.com/" }
```

**Board verbs and state message (changed)** — the `list_games` and `load_game` board verbs are removed, and the state message sent when no game is open no longer includes `games`.

## Implementation Detail

**Mostly a removal.** The largest code-shape change is subtraction: the event store loses its file mode, and the game manager loses its file layout (games directory, side files, last-game pointer) and the list/load/resume operations built on it. A developer reading the manager afterwards sees a small in-memory holder of "the current game" plus the import and rebuild logic, with no paths in it. Because the store's in-memory mode already exists and every write is already a no-op without a file, the game engine (verbs, branches, rewind, notes, reads) is untouched and its existing tests keep passing unchanged. The compiler finds every caller of the removed operations (the MCP server, the board's verbs, the run modes), which makes the removal safe to do in one pass.

**New pattern: a sealed, versioned value that round-trips through text.** The game record is the codebase's first format meant to leave the program and come back by hand. It follows three rules a developer will see in one small file: a marker field that both identifies and versions the format (and refuses newer versions); a checksum over the canonical content rather than the bytes on the wire, so pretty-printing, reflowing or re-indenting is harmless; and a deliberately narrow repair step that undoes only substitutions note apps are known to make and is accepted only when the checksum then matches. The record is built at one point (the end of a successful import, from the request actually used, with the detected player name and the seed the importer chose) and read at one point (the start of an import), so there is exactly one producer and one consumer.

**Restore is import.** No new restore path exists. Import recognises a record at its entry, expands it into the same request a battle-log import uses (seed included), and continues down the unchanged importer. Rebuilding with an opponent list is the same move: take the current game's record, swap the list, import again. A developer debugging a restore therefore reads the same code as an import. The determinism this relies on is already present (a single seeded generator in the importer) and is made a tested property by a round trip over the whole battle-log corpus, rather than an assumption.

**State that used to be files becomes manager fields.** The player's last decklist moves from a file into the manager's memory, and the record moves from a side file onto the game. Both follow the manager's existing locking. Putting the record on the game, not the manager, keeps the per-account lookup that per-user-games will add a pure change of "which game", not "which game and which record".

**Tools follow the existing registration shape.** `get_record` is one more entry in the tool server's existing table of tools with the same handler helper; the import and rebuild tools gain one result key each. The web-mcp-docker plan's hosted switch (`BoardURL`) is reused as is; this plan adds no new hosted-only behaviour, since the spec applies the same rules to desktop and hosted.

**Tests follow the existing shape.** Game-level tests use the existing corpus and temporary data directories; the new round-trip and no-files tests sit beside the import tests. Tool-level tests extend the existing in-memory MCP client tests, and under the web-mcp-docker plan they run over both transports automatically. A "nothing written" test snapshots the data directory before and after a session of imports, moves, branches and rebuilds and requires it unchanged, which guards the property structurally rather than by reading the code.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Upstream plan `20261017105035-e3f64c1b-web-mcp-docker` (must land first)** — the epic orders it before this spec. It adds the `-hosted` run mode, the `BoardURL` switch that puts `board` on game-opening results, and two-transport MCP tool tests. This plan changes three of its outcomes: the hosted run mode's "reopen the last game" call is removed (with `Resume`), `load_game` and its `board` link disappear, and the container no longer writes game files under `$HOME`. If this plan is implemented before web-mcp-docker lands, the hosted-mode parts simply have nothing to change, and web-mcp-docker's implementer must not reintroduce `Resume` or `load_game`.
- **Downstream spec `20261017143118-b05aa6b1-per-user-games`** — builds on this plan's memory-only manager and keeps every account's game in memory. The record living on the game, and the last decklist living in the manager, are the two pieces of session state it will need to make per-account.
- **Parallel spec `20261017123240-21493a95-google-login`** — no shared code; it touches sign-in and the public URL, not game storage.
- **Internal packages (changed)** — `internal/store` (file mode removed), `internal/game` (memory-only manager, record type, rebuild from record), `internal/mcpserver` (tool changes), `internal/webui` (board list/load verbs and state field removed), `cmd/tcgreplay` (no resume at start-up, flag text), `skills` (drill, import and opponent-deck skills).
- **Internal packages (used unchanged)** — `internal/importer` (the seeded replay that makes the record exact), `internal/deck`, `internal/infer`, `internal/state`, `internal/cards` (card cache stays on disk, a spec non-goal).
- **Board UI (`web/`, Svelte, changed)** — the landing page's stored-game buttons and the `list_games` read are removed; rebuilt into the embedded `dist/app` by the existing build target. No new npm dependency.
- **Go standard library** — `crypto/sha256` and `encoding/json` for the record and its check. No new module dependency.
- **`github.com/modelcontextprotocol/go-sdk` v1.6.0 (existing)** — tool registration for `get_record`; no version change.
- **Test corpus (`testdata/logs`, `testdata/decks`, existing)** — the ten real battle logs and the owner's deck drive the round-trip tests.

## Testing Approach

Tests stay in Go's standard `testing` package beside the code, use the existing `testdata/` battle-log corpus and temporary data directories, and run under the repo's vet-and-test Makefile target. Nothing new needs network access: the card database is nil or the existing fake, as in today's tests, which also proves card data does not decide where cards go.

**Round trip over the corpus (most coverage, the load-bearing guarantee for Exact restore).** For every battle log in the corpus, import it with a random seed (and, for some, an assumed opponent list), take the record, then import the record into a fresh manager on a fresh data directory (the code-level equivalent of a restart). The two games must have identical event sequences, and identical reveal-view boards (every hand, prize, deck order and discard) at the start, the middle and the end of the timeline. The same holds after `set_opponent_deck`: the updated record restores the rebuilt game exactly. If any corpus log fails this, implementation stops (see the plan's open question), because it would mean the seed alone does not fix the game.

**Record format (unit).** A record survives being re-indented, compacted and having its whitespace between JSON tokens reflowed, and still restores the same game (**Paste round trip** at code level). A record whose straight quotes were all turned curly (structural and inside values), or whose values carry non-breaking spaces or CRLF line ends, is repaired and restores the identical game. A record whose content was otherwise altered (a space collapsed inside the log, a deck count changed, the seed edited) is rejected with the "damaged" message, never restored (**Damaged paste**). A record with a newer version is refused; plain battle-log text is never mistaken for a record; a record passed together with a deck, opponent list or name is refused with a clear message.

**Memory-only manager (integration at game level).**
- **Nothing written**: a data directory pre-seeded with game files, import files, a last-game pointer and a last-deck file is snapshotted (names and bytes); after imports, play, branches, notes, a rebuild and a `new_game`, it is byte-for-byte unchanged with no new game or import files (the assistant label and process log are not game data and are excluded).
- **Restart forgets**: a new manager on that same directory has no game open, even with the old pointer file present.
- **Second import replaces first**; `new_game` replaces an imported game; the last decklist is reused within the session and forgotten by a new manager.
- `set_opponent_deck` rebuilds the current game from its record (owner side unchanged, as today's test checks), refuses a game with branches unless branches are explicitly discarded, and refuses a `new_game` game.

**MCP tools (integration through the in-memory client, and over Streamable HTTP once web-mcp-docker's two-transport helper is in place).**
- **Import returns record**: `import_log`'s result has a record whose log equals the text passed in, with the player's name, both lists and a seed.
- **Deck change updates record**: after `set_opponent_deck`, its result's record and `get_record` both show the new list.
- **Record tool**: `get_record` returns the same record as the latest import or rebuild.
- **Play-mode game**: after `new_game`, `get_record` returns an error saying the game has no record.
- **Restore through the tool**: passing a record as `import_log`'s `log` restores the same game.
- **Tools removed**: the tool list contains `get_record` and no `list_games` or `load_game`, and `set_opponent_deck`'s schema has no `id` or `new_id`.
- **Hosted**: with a board URL set, `import_log` and `set_opponent_deck` results carry both `record` and `board`.

**Board (unit).** The board's state message carries no `games` list, and the `list_games` and `load_game` board verbs are refused as unknown.

**Skills.** The existing skills format test keeps guarding the skill files. A check that no skill mentions `list_games` or `load_game` guards the **Drills** text automatically; whether each drill actually runs on the open game, and asks for an import when none is open, is checked by hand.

**Regression.** The full existing suite (engine, importer, board, shim, installer) passes with only the mechanical changes forced by removed functions: tests that built games with explicit ids in a temp data dir keep working because ids are still accepted.

**Deliberate gaps.** No automated test restarts a real process or container, pastes through a real note app or drives a real assistant through the skills; those are covered by the manual checks below. The code-level restart (fresh manager on the same directory) and the code-level paste damage are the automated halves.

**Success metrics:**
- Over the owner's first 10 real games after delivery, every saved record restores with no differences — **Manual — captured in the implementation test plan** (the corpus round-trip test is its automated precursor).
- The owner saves records to Notion after games without hand-editing them — **Manual — captured in the implementation test plan.**

**Manual reviews:**
- **Restart forgets, desktop and hosted**: import a game, restart the desktop daemon and the hosted container; no game is open in either — **Manual — captured in the implementation test plan.**
- **Nothing written on a real install**: after a real session, the desktop data directory's existing game files are byte-for-byte unchanged and no new game or import files appear — **Manual — captured in the implementation test plan.**
- **Exact restore by hand**: reveal-view state at start, middle and end of a real game matches after restart and record import — **Manual — captured in the implementation test plan.**
- **Paste round trip through Notion**: a record copied into a Notion code block and pasted back restores the same game — **Manual — captured in the implementation test plan.**
- **Damaged paste through a real note app**: a record pasted into a note app that curls quotes either restores identically or is rejected with the damaged message — **Manual — captured in the implementation test plan.**
- **Drills**: with a game open, the replay drill, numbers drill and before-you-move skills each run on it; with none open, each asks for an import; none errors or mentions listing or loading games — **Manual — captured in the implementation test plan.**
- **Board landing page**: with no game open, the board shows no stored-game buttons and asks for an import or new game — **Manual — captured in the implementation test plan.**

## Milestones & Tasks

### Milestone 1: Every imported game comes with a record that brings it back exactly

**What changes**: When the player imports a battle log, the result now includes the game's record: the log, the player's name, both decklists and the value that fixes how hidden cards are dealt, as one piece of JSON text. Changing the opponent's list returns the updated record, and the assistant can ask for the current game's record at any time; a game started from two decklists says it has none. Pasting a record back into an import rebuilds the identical game, card for card at every point of the timeline, and a record mangled by a note app is either repaired to the same game or refused with a message saying it was damaged. The import skill tells the player they can keep the record in their notes. Games are still saved to disk as before in this milestone, so nothing is lost while the record proves itself; it lands first so that, when storage goes in Milestone 2, every game can already be brought back.

**Validation point**: The corpus round-trip tests pass (every corpus log, imported with a random seed, restores from its record with identical events and identical reveal-view boards at start, middle and end); the record-format tests for reformatting, curly quotes and damage pass; the MCP tests show `record` on `import_log` and `set_opponent_deck`, `get_record` matching the latest import or rebuild and refusing a `new_game` game; the full vet-and-test suite passes.

#### - [x] Task: Define the game record format
**Id:** 5620b4fe-a47a-4406-8c6e-1902eaaa809f
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds the game record: a small, versioned JSON object holding exactly the raw game (battle log, player's name, player's deck, opponent's list, seed) and a checksum over that content. It can be built and sealed from an import, rendered as JSON, and read back from pasted text, where it is recognised as a record rather than a battle log, repaired if a note app curled its quotes or swapped its spaces, and refused with a clear "damaged" message if anything else changed. This is the one format the playmat hands out and takes back, so its rules live in one place.

*Technical detail:* [context.md#task-define-the-game-record-format](./context.md#task-define-the-game-record-format)

**Acceptance criteria**:
- [x] A sealed record read back from its own JSON, re-indented or compacted, is accepted unchanged.
- [x] A record whose quotes were turned curly, or whose values gained non-breaking spaces or Windows line ends, is accepted and equals the original.
- [x] A record with any other change to its content (log, decks, name or seed) is refused with a message saying it was damaged.
- [x] A record from a newer version is refused with a message naming the version; ordinary battle-log text is never taken for a record.
- [x] The seed survives as text, so 64-bit values are not rounded by JSON tools.

#### - [x] Task: Build and restore games from records
**Id:** 26361422-d5a5-4355-82f0-c6626c01e9c7
**Repo:** tcgreplay
**Depends on:**
- 5620b4fe-a47a-4406-8c6e-1902eaaa809f — Define the game record format
**Execution:** agent

Makes every import produce a record and every import accept one. The record is built from what the import actually used (the deck even when reused from the previous import, the detected player name, the seed the importer chose) and kept with the game in memory. Importing a record's text rebuilds the same game from its seed; rebuilding with an opponent list returns the updated record. A round-trip test over every real battle log in the test corpus proves the restored game matches card for card at every point of its timeline.

*Technical detail:* [context.md#task-build-and-restore-games-from-records](./context.md#task-build-and-restore-games-from-records)

**Acceptance criteria**:
- [x] Importing a battle log yields a record whose log equals the text passed in, with the player's name, the deck used, the opponent list given and the seed used.
- [x] For every log in the test corpus, importing its record into a fresh playmat gives identical events and identical fully revealed boards at the start, middle and end of the game, with and without an assumed opponent list.
- [x] Rebuilding with an opponent list yields a record with the new list that also restores exactly.
- [x] A game started from two decklists has no record.
- [x] A record imported together with a deck, opponent list or player name is refused with a message saying a record already holds them.

#### - [x] Task: Give the assistant the record through the tools
**Id:** 47d4209b-9253-45c7-a7ec-b8683e1a7169
**Repo:** tcgreplay
**Depends on:**
- 26361422-d5a5-4355-82f0-c6626c01e9c7 — Build and restore games from records
**Execution:** agent

Exposes records to the assistant over MCP. The import tool's result and the opponent-deck tool's result each carry the record, the import tool's description says it accepts a saved record as well as a battle log, and a new `get_record` tool returns the current game's record at any time, or says the game has none. On the hosted server the board link stays on the results alongside the record.

*Technical detail:* [context.md#task-give-the-assistant-the-record-through-the-tools](./context.md#task-give-the-assistant-the-record-through-the-tools)

**Acceptance criteria**:
- [x] The import result contains the game's record, and the opponent-deck result contains the updated record.
- [x] `get_record` returns the same record as the most recent import or opponent-deck change.
- [x] For a game started from two decklists, `get_record` reports that the game has no record; with no game open it says so.
- [x] Passing a record to the import tool restores the same game.
- [x] With a hosted board address set, import and opponent-deck results carry both the record and the board link.

#### - [x] Task: Tell the player about the record when importing
**Id:** 19072054-7d6a-4ec3-80d0-b97112aab522
**Repo:** tcgreplay
**Depends on:**
- 47d4209b-9253-45c7-a7ec-b8683e1a7169 — Give the assistant the record through the tools
**Execution:** agent

Updates the import skill so that, after importing (or after fixing the opponent list), the assistant tells the player the game comes with a record they can keep in their notes, offers it exactly as returned, and explains that pasting it back restores the identical game while branches and notes last only for the session. It also tells the assistant to pass a pasted record to the import tool unchanged.

*Technical detail:* [context.md#task-tell-the-player-about-the-record-when-importing](./context.md#task-tell-the-player-about-the-record-when-importing)

**Acceptance criteria**:
- [x] The import skill offers the player the record after a successful import and after an opponent-list fix.
- [x] The import skill restores a game when the player pastes a record, passing it unchanged.
- [x] The skill still passes the skills format check and names no specific assistant or player.

### Milestone 2: The playmat keeps nothing between sessions

**What changes**: The playmat stops saving games, on the desktop and on the hosted server alike. Every game lives only in memory for the session: restarting the playmat (or redeploying the server) starts with no game open, and importing or starting a game replaces the current one. There is no longer a way to list or reopen earlier games, from the assistant or from the board; a game comes back only by importing its record. Game files already on disk are left exactly as they are and never read. The player's decklist is still remembered between imports within a session. Rebuilding a game with a new opponent list works from the game's record and asks for explicit consent before discarding branches. The replay drill, numbers drill and before-you-move skills work from the game open now and ask for an import when none is open, and the design document describes the new behaviour.

**Validation point**: The "nothing written" and "restart forgets" tests pass against a data directory pre-seeded with old game files; the tool list has `get_record` and no `list_games` or `load_game`; the board's state message has no game list and its landing page shows no stored-game buttons; no skill mentions listing or loading games; the full vet-and-test suite passes; and by hand, restarting the desktop daemon and the hosted container after an import leaves no game open.

#### - [x] Task: Keep games in memory only
**Id:** be8ac127-f2f5-4a47-9669-0383feefe1ce
**Repo:** tcgreplay
**Depends on:**
- 26361422-d5a5-4355-82f0-c6626c01e9c7 — Build and restore games from records
- 47d4209b-9253-45c7-a7ec-b8683e1a7169 — Give the assistant the record through the tools
**Execution:** agent

Stops the playmat writing or reading games in every mode. Games live only in memory; the event store's file mode, the saved-game list, loading and "reopen the last game" are removed, as are the `list_games` and `load_game` tools and the board's equivalent requests. The last decklist is remembered in memory for the session, and rebuilding with an opponent list works from the current game's record, refusing to discard branches unless asked to. Files already in the data directory are left untouched, and the data directory's help text no longer calls it the home of game files.

*Technical detail:* [context.md#task-keep-games-in-memory-only](./context.md#task-keep-games-in-memory-only)

**Acceptance criteria**:
- [x] After imports, moves, branches, notes, an opponent-list rebuild and a new game, a data directory that already held saved games is byte-for-byte unchanged, with no new game or import files.
- [x] A restarted playmat (desktop or hosted) starts with no game open, even when old saved games are present.
- [x] Importing or starting a game replaces the current one; the last decklist is reused within a session and forgotten after a restart.
- [x] The tool list no longer offers listing or loading games, and the board no longer answers requests to list or load them.
- [x] Rebuilding with an opponent list refuses a game with branches unless the caller asks to discard them, and refuses a game that was not imported.

#### - [x] Task: Remove stored games from the board page
**Id:** 6d0bea97-99fc-4fce-9d36-7f05c395b95d
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Updates the board's landing page, shown when no game is open, so it no longer lists stored games or offers to load one, and instead asks the player to have the assistant import a battle log or saved record, or start a new game. The board client also stops asking the server for a game list.

*Technical detail:* [context.md#task-remove-stored-games-from-the-board-page](./context.md#task-remove-stored-games-from-the-board-page)

**Acceptance criteria**:
- [x] With no game open, the board shows no stored-game buttons and tells the player how to open a game by import or new game.
- [x] The board client never requests a game list.
- [x] The built board still loads and follows a game when one is opened.

#### - [x] Task: Point the drills at the game open now
**Id:** 58d363f7-fbf1-4098-92a4-6ae70f229ad4
**Repo:** tcgreplay
**Depends on:**
- be8ac127-f2f5-4a47-9669-0383feefe1ce — Keep games in memory only
**Execution:** agent

Rewrites the replay drill, numbers drill and before-you-move skills to work from the game open on the playmat now, and to ask the player to import a battle log or a saved record when none is open, with no mention of listing or loading games. The opponent-deck skill replaces its "build a separate game" advice with the new rule: rebuilding replaces the game and needs the player's consent to discard branches. A check keeps any skill from mentioning the removed tools.

*Technical detail:* [context.md#task-point-the-drills-at-the-game-open-now](./context.md#task-point-the-drills-at-the-game-open-now)

**Acceptance criteria**:
- [x] Each drill skill starts from the open game and, when none is open, asks for an import.
- [x] No skill mentions listing or loading games or building a separate game by id.
- [x] The opponent-deck skill asks the player before discarding branches.
- [x] All skills pass the format check.

#### - [x] Task: Describe memory-only games in the design document
**Id:** 1652a272-374d-4c23-91f0-fad06f1bebff
**Repo:** tcgreplay
**Depends on:**
- be8ac127-f2f5-4a47-9669-0383feefe1ce — Keep games in memory only
**Execution:** agent

Updates the project's design document so its import and runtime descriptions match the new behaviour: games live only for the session, imports return a record that restores them exactly, saved games and side files are gone, and existing files on disk are ignored.

*Technical detail:* [context.md#task-describe-memory-only-games-in-the-design-document](./context.md#task-describe-memory-only-games-in-the-design-document)

**Acceptance criteria**:
- [x] The design document describes memory-only games, the record and restoring through import, for desktop and hosted alike.
- [x] Nothing in the document describes saved games, reopening the last game or a saved import file.

## Open Questions

- **Does a real note app (Notion, the owner's target) alter a pasted record in ways beyond curly quotes, non-breaking spaces and Windows line ends?** Depends on the app's real paste behaviour, only observable in the manual paste checks once the code exists. If it applies another reversible substitution (for example en dashes for hyphens or a zero-width character), add that substitution to the record's repair set: this is safe without asking because a repaired record is only ever accepted when its check then matches. If it applies a change that cannot be undone (for example trimming trailing spaces from log lines), do not weaken the check; STOP and ask the user whether to record the log in a form that survives it.
- **Does a corpus log ever fail the record round trip?** Planning verified all ten corpus logs round-trip (with and without an opponent list, random seeds), so none is expected. If the round-trip test fails on a log added later, STOP and ask the user: fixing it by putting events in the record would contradict the spec's "raw game only" constraint.

## Out of Scope

- **Drilling across many past games** — the drills work only on the game open now; drilling across a library of records is left for later (spec non-goal).
- **Records for games started with `new_game`** — play-mode games have no record and cannot be restored (spec non-goal).
- **Keeping branches, notes, findings or sampled lists across sessions** — the record holds only the raw game; everything made on top of it lasts for the session (spec constraint).
- **Moving the card cache to memory** — the TCGdex card cache stays on disk and is not game storage (spec non-goal).
- **Per-user games** — one current game per playmat, as today; separate games per signed-in account are tracked in `20261017143118-b05aa6b1-per-user-games`, which builds on this plan's memory-only manager.
- **Notion integration and packaging the skills for claude.ai** — the player copies and pastes the record themselves (spec non-goal).
- **Migrating, deleting or importing existing game files** — files already in the data directory are left exactly as they are and ignored; nothing converts them into records.
- **Moving the assistant label out of the data directory** — the label file is not game storage and stays as it is.
- **Backward compatibility for removed tools and parameters** — `list_games`, `load_game` and `set_opponent_deck`'s `id`/`new_id` are removed outright; the bundled skills are updated in the same change.

## Changelog

### 2026-10-17 — Task: Define the game record format

**What was done**: Added the game record type in `internal/game/record.go`: a versioned JSON object (`tcgreplay_record`, me, seed as a decimal string, deck, opp_deck, log, check) sealed with a SHA-256 over its canonical field values, rendered as indented JSON without HTML escaping, and parsed back from pasted text (code fences and BOM stripped). Parsing undoes curly quotes, non-breaking spaces and CRLF only when the check then matches, refuses newer versions by number, and otherwise returns `ErrRecordDamaged`. Table tests cover reformatting, note-app damage, real damage, versions, battle logs never being taken for records and 64-bit seeds.

**Deviations**: Minor widening within the planned repair rule: structural decoding also retries with non-breaking spaces straightened (so NBSP in JSON indentation still decodes), and value repair tries every combination of the four substitution sets (curly single quotes, curly double quotes, odd spaces, CRLF) rather than only "all" and "each alone". A record is still accepted only when its check matches.

**Files changed**:
- `internal/game/record.go`
- `internal/game/record_test.go`

**Discoveries**: The `sum()` canonical struct (json keys v, me, seed, deck, opp_deck, log) is now a frozen format: changing it invalidates every saved version-1 record. A record whose own content holds curly quotes or NBSP is returned as is, because the unrepaired check matches first.

### 2026-10-17 — Task: Build and restore games from records

**What was done**: Every import now builds a sealed record from what it actually used (log, detected player name, deck including a reused last deck, opponent list as passed, seed used), keeps it on the `Game` and returns it in `ImportResult.Record`. `Import` recognises a record in `req.Log`, refuses it with "already holds" when a deck, opponent list, name or seed is also given, and otherwise expands it into the same request a battle-log import uses. `Game.Record()` and `Manager.Record()` expose it; a `new_game` game has none. A corpus-wide round-trip test proves every log restores with identical events and boards at start, middle and end, with and without an assumed opponent list.

**Deviations**: None.

**Files changed**:
- `internal/game/game.go`
- `internal/game/import.go`
- `internal/game/manager.go`
- `internal/game/record_roundtrip_test.go`

**Discoveries**: All 10 corpus logs round-trip from the record alone (about 0.9s for 20 cases). In this milestone a game object that is no longer current has a closed log, so verbs on it fail with "file already closed"; tests compare games from separate managers.

### 2026-10-17 — Task: Give the assistant the record through the tools

**What was done**: The `import_log` and `set_opponent_deck` results now carry `record`, the `import_log` description and `log` schema say a saved record can be passed (alone, unchanged) to restore the exact game, and a new `get_record` tool returns the current game's record or an error saying no game is open or that the game has no record. The hosted `board` link stays on both results alongside the record. Tests cover the record fields, `get_record` matching the latest import and rebuild, restore through `import_log` giving an identical reveal view, the `new_game` and no-game errors, and `record` with `board` when hosted, over both transports.

**Deviations**: None.

**Files changed**:
- `internal/mcpserver/server.go`
- `internal/mcpserver/server_test.go`

**Discoveries**: Restoring a record through `import_log` with no id generates a new game id, so reveal-view comparisons must drop the `game` key.

### 2026-10-17 — Task: Tell the player about the record when importing

**What was done**: The import skill now triggers on a pasted game record, passes a pasted record to `import_log` as `log` unchanged with no other arguments (skipping list building and fixing), handles a "damaged" refusal by asking for a fresh copy or the original log, and in the triage offers the record (the updated one after a `set_opponent_deck` fix) in a code block, explaining it restores the exact game while branches and notes last only for the session.

**Deviations**: No new automated test: as the plan's testing strategy says, the existing skills format test covers this task, and it passes.

**Files changed**:
- `skills/tcg-import-game/SKILL.md`

**Discoveries**: None.

### 2026-10-17 — Task: Keep games in memory only

**What was done**: The event store is now memory-only (`Create`, `Open`, the JSONL writer, `Close` and `Path` are gone), and the game manager holds the current game and the last decklist in memory, dropping the previous game on every import or `new_game`. Saved-game listing, loading and reopening (`List`, `Load`, `Resume`, `GameInfo`, `current.txt`, `last-deck.txt`, `<id>.import.json`) are removed, as are the `list_games`/`load_game` MCP tools and board verbs and the board's `games` field. `set_opponent_deck` now rebuilds the current game from its record, takes `discard_branches` in place of `id`/`new_id`, and refuses games without a record. No run mode (in-process, daemon, hosted) reopens a game at start-up, and the `-data` help no longer mentions game files. New tests prove a pre-seeded data directory is left byte-for-byte unchanged, a new manager starts with no game, imports replace each other and the last deck is forgotten on restart.

**Deviations**: Also removed the board request's now-unused `game` argument (`internal/webui/actions.go`), and the hosted run mode discards the manager returned by `newHosted` since it no longer calls `Resume`. Game ids are no longer validated as file names, since nothing is written. Three existing assertions that only tested persistence to disk were removed (the reload check at the end of `TestUndo`, and the `.import.json` and game-file checks in the round-trip tests); `TestFullGameRewindAndReload` became `TestFullGameRewindAndBranch`. Restarting a real desktop daemon and hosted container is left to the manual test plan.

**Files changed**:
- `internal/store/store.go`
- `internal/game/game.go`
- `internal/game/manager.go`
- `internal/game/import.go`
- `internal/game/game_test.go`
- `internal/game/import_test.go`
- `internal/game/record_roundtrip_test.go`
- `internal/game/memory_test.go`
- `internal/mcpserver/server.go`
- `internal/mcpserver/server_test.go`
- `internal/webui/actions.go`
- `internal/webui/hub.go`
- `internal/webui/webui_test.go`
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/daemon.go`
- `cmd/tcgreplay/hosted.go`

**Discoveries**: Removed board verbs fall through to the normal path: with no game open they answer "no game open", otherwise "unknown verb". The store's `error` returns on `Append`, `Branch` and friends are kept for API stability though they can no longer fail on I/O.

### 2026-10-17 — Task: Remove stored games from the board page

**What was done**: The board's landing page now reads "No game open" and asks the player to have the assistant import a battle log or a saved game record, or start a new game; the stored-game buttons are gone, and the client no longer keeps `app.games` or treats `list_games` as a read. The board was rebuilt in the worktree (`npm ci`, `npm run build`); the built bundle holds no `list_games`/`load_game` and a hosted smoke run served the page and left its data directory empty.

**Deviations**: None. No automated UI tests exist for the Svelte app; the build, the webui Go tests and a hosted smoke run stand in, and the landing page is in the manual test plan.

**Files changed**:
- `web/src/App.svelte`
- `web/src/lib/ws.js`
- `web/src/lib/store.svelte.js`

**Discoveries**: None.

### 2026-10-17 — Task: Point the drills at the game open now

**What was done**: The replay drill, numbers drill and before-you-move skills now work from the game open on the playmat and, when none is open, ask the player to import one (a battle log or a saved game record) and stop; the replay drill notes that findings last only for the session, so a restored game needs a review first. The opponent-deck skill now says the rebuild replaces the open game, returns the updated record, and needs the player's agreement before calling again with `discard_branches: true`. A new `TestSkillsUseCurrentTools` keeps every skill free of `list_games`, `load_game` and `new_id`.

**Deviations**: The small skills test was written alongside the skill edits rather than by a separate test agent; it and the format test pass. Whether each drill actually runs on the open game is a manual check, as planned.

**Files changed**:
- `skills/tcg-replay-drill/SKILL.md`
- `skills/tcg-numbers-drill/SKILL.md`
- `skills/tcg-before-you-move/SKILL.md`
- `skills/tcg-opponent-deck/SKILL.md`
- `skills/skills_test.go`

**Discoveries**: None.

### 2026-10-17 — Task: Describe memory-only games in the design document

**What was done**: The project design document now says games live in memory for the session in every mode (a restart starts with no game open), `import_log` returns the game's record, `set_opponent_deck` rebuilds the current game from its record and needs `discard_branches` to drop branches, and old files in the data directory are ignored. A new "Game records" bullet describes the record's fields, version marker and check, restoring through `import_log`, the note-app repairs, and that branches, notes and findings are not kept. The hosted section no longer lists `load_game`.

**Deviations**: None.

**Files changed**:
- `TCG Review Playmat Design and Build Plan.md`

**Discoveries**: None.
