---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Context: 20261017112555-30c99ab1-memory-store-record

## Current State Analysis

All work is in the single registered repo `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`).

- **Event store** (`internal/store/store.go`): append-only event tree with branches, actions, annotations and a read audit. File mode (`Create` `:90-108`, `Open` `:129-173`, JSONL `write` `:225-238`) and memory mode (`NewMemory` `:110-116`); every mutation calls `write`, a no-op without a file.
- **Game manager** (`internal/game/manager.go`): one current game; writes `current.txt` on every `setCurrent` (`:97-112`) and reopens it with `Resume` (`:162-180`); `List`/`Load` over `<data>/games/*.jsonl` (`:114-199`); `New` creates a file-backed play game (`:249-282`); `assistant.txt` label (`:40-66`).
- **Import** (`internal/game/import.go`): `ImportRequest{ID, Log, Deck, OppDeck, Me, Seed}` (`:23-36`); reuses `last-deck.txt` when no deck is given (`:51-58`, `:72-76`); writes the game file and `<id>.import.json` with the seed used (`:98-135`); `SetOpponentDeck` re-imports from that file, refusing games with branches unless `new_id` (`:162-224`).
- **Importer** (`internal/importer/importer.go`): deterministic given log, decks, owner name and seed (single PCG, `:209`); random seed when 0 (`:72-75`). Verified during planning across all 10 corpus logs.
- **MCP tools** (`internal/mcpserver/server.go`): `import_log` (`:325-336`), `set_opponent_deck` (`:337-356`), `list_games` (`:357-359`), `load_game` (`:360-366`); inputs at `:140-175`. Tests use an in-memory client (`server_test.go:17-47`).
- **Board** (`internal/webui`): `list_games`/`load_game` verbs (`actions.go:103-113`), `games` in the no-game state message (`hub.go:124-129`); Svelte landing buttons (`web/src/App.svelte:74-82`), `app.games` (`web/src/lib/ws.js:41`, `store.svelte.js:14`), `list_games` in `READS` (`ws.js:66`).
- **Run modes** (`cmd/tcgreplay`): in-process and daemon call `m.Resume()` (`main.go:111`, `daemon.go:164`); web-mcp-docker's planned `runHosted` (`cmd/tcgreplay/hosted.go`) also will.
- **Skills**: drills use `list_games`/`load_game` (`skills/tcg-replay-drill/SKILL.md:12`, `tcg-numbers-drill/SKILL.md:12`, `tcg-before-you-move/SKILL.md:12`); `tcg-opponent-deck/SKILL.md:57` advises `new_id`. `skills/skills_test.go` forbids "Claude"/"Nic " in skills.

**Requirement → task → files**

| Requirement | Task(s) | Files |
|---|---|---|
| Nothing kept between sessions | Keep games in memory only | `internal/game/manager.go`, `cmd/tcgreplay/main.go`, `cmd/tcgreplay/daemon.go`, `cmd/tcgreplay/hosted.go` |
| No lasting game storage | Keep games in memory only | `internal/store/store.go`, `internal/game/manager.go`, `internal/game/import.go` |
| Record with every import | Define the game record format; Build and restore games from records; Give the assistant the record through the tools | `internal/game/record.go`, `internal/game/import.go`, `internal/mcpserver/server.go` |
| Record follows deck changes | Build and restore games from records; Give the assistant the record through the tools; Keep games in memory only | `internal/game/import.go`, `internal/mcpserver/server.go` |
| Record on request | Give the assistant the record through the tools | `internal/mcpserver/server.go`, `internal/game/manager.go` |
| No record without a battle log | Build and restore games from records; Give the assistant the record through the tools | `internal/game/manager.go`, `internal/mcpserver/server.go` |
| Exact restore | Build and restore games from records | `internal/game/import.go`, `internal/game/record_roundtrip_test.go` |
| Damaged records never restore wrong | Define the game record format | `internal/game/record.go`, `internal/game/record_test.go` |
| New game replaces current | Keep games in memory only | `internal/game/manager.go`, `internal/game/import.go` |
| No reopening earlier games | Keep games in memory only; Remove stored games from the board page | `internal/mcpserver/server.go`, `internal/webui/actions.go`, `internal/webui/hub.go`, `web/src/App.svelte`, `web/src/lib/ws.js` |
| Portable record | Define the game record format; Tell the player about the record when importing | `internal/game/record.go`, `skills/tcg-import-game/SKILL.md` |
| Drills use the current game | Point the drills at the game open now | `skills/tcg-replay-drill/SKILL.md`, `skills/tcg-numbers-drill/SKILL.md`, `skills/tcg-before-you-move/SKILL.md`, `skills/skills_test.go` |
| Both modes; existing files untouched; import still takes a plain log | all Milestone 2 tasks; Build and restore games from records | as above |

## Per-Task Technical Notes

### Task: Define the game record format

**File changes**:
- `internal/game/record.go` (new) — `const RecordVersion = 1`; `type Record struct { Version int "json:\"tcgreplay_record\""; Me string "json:\"me\""; Seed uint64 "json:\"seed,string\""; Deck string "json:\"deck\""; OppDeck string "json:\"opp_deck,omitempty\""; Log string "json:\"log\""; Check string "json:\"check\"" }` (field order is the JSON order: small fields first, log last, check at the end).
  - `func (r Record) sum() string` — `"sha256:" + hex(sha256(json.Marshal(struct{V int; Me string; Seed string; Deck, OppDeck, Log string}{...})))` — canonical content excludes `Check`; Go struct marshalling is deterministic, so this is the canonical form. Seed formatted with `strconv.FormatUint`.
  - `func (r Record) Seal() Record` — sets `Version` (if 0) and `Check`.
  - `func (r Record) JSON() string` — `json.MarshalIndent(r, "", "  ")` with `SetEscapeHTML(false)` (an encoder) so `<`, `>`, `&` in logs are not turned into `<` noise.
  - `var ErrRecordDamaged = errors.New("this game record was damaged on the way (its check does not match), so it was not restored: copy it again unchanged from where it was saved, or import the original battle log")`.
  - `func LooksLikeRecord(text string) bool` — after `strings.TrimSpace` (also trimming a leading BOM and a surrounding Markdown code fence ```` ``` ````/```` ```json ````), the text starts with `{` and contains `tcgreplay_record` (also with curly quotes around it). A TCG Live log starts with "Setup" or a player line, never `{`.
  - `func ParseRecord(text string) (Record, bool, error)` — returns `isRecord=false, nil` when `!LooksLikeRecord`. Otherwise: strip fence; `json.Unmarshal`; if it fails, retry after replacing `“ ” „ ″` with `"` across the whole text (structural quotes curled by a note app); if still invalid, return `ErrRecordDamaged`. Version check: `Version > RecordVersion` → `fmt.Errorf("this game record is version %d; this playmat reads version %d or older", ...)`; `Version == 0` → damaged. Then verify `Check == r.sum()`; if not, try `repair(r)`: in every string field replace `‘ ’ ‚ ′` → `'`, `“ ” „ ″` → `"`, U+00A0/U+202F/U+2007 → space, `\r\n` → `\n`; also try each substitution set alone (quotes only; spaces only) — accept the first candidate whose `sum()` equals `Check`, returning it with `Check` intact. If none match → `ErrRecordDamaged`. Never return a record whose check did not match.
- `internal/game/record_test.go` (new) — table tests: seal → `JSON()` → `ParseRecord` equal; compacted (`json.Compact`) and re-indented with tabs equal; whole text with every `"` → `“`/`”` alternating equal; value with `'` → `’` equal; log with NBSP inserted where the original had spaces (seal with a log containing a normal space, damage by replacing it with NBSP) equal; log with a double space collapsed → `ErrRecordDamaged`; deck count edited → damaged; seed edited → damaged; `check` removed → damaged; `tcgreplay_record: 2` → version error; a real corpus log text (`testdata/logs/*.txt`) → `isRecord=false`; seed `math.MaxUint64` round-trips through JSON as a string; a record wrapped in a ```` ```json ```` fence parses.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential execution (type and sum, then parse/repair, then table tests).

### Task: Build and restore games from records

**File changes**:
- `internal/game/game.go:52-66` — add an unexported `record *Record` field to `Game`; add `func (g *Game) Record() (Record, bool)` returning a copy and whether one exists. Games built by `Manager.New` (`internal/game/manager.go:249-282`) leave it nil.
- `internal/game/import.go:38-43` — `ImportResult` gains `Record Record`.
- `internal/game/import.go:49-58` — at the top of `Import`: `rec, isRec, err := ParseRecord(req.Log)`; on error return it; if `isRec`: if any of `req.Deck`, `req.OppDeck`, `req.Me` is non-blank (or `req.Seed != 0`) return `errors.New("a game record already holds both decks, the player's name and the seed: import it on its own, then use set_opponent_deck to change the opponent's list")`; else set `req.Log, req.Deck, req.OppDeck, req.Me, req.Seed = rec.Log, rec.Deck, rec.OppDeck, rec.Me, rec.Seed`. Keep `req.ID`.
- `internal/game/import.go:126-140` — after `Load` succeeds, build `rec := Record{Me: res.Report.Me, Seed: res.Seed, Deck: req.Deck, OppDeck: req.OppDeck, Log: req.Log}.Seal()` (`req.Deck` is already the reused last deck when the caller omitted it, `:51-58`); set `g.record = &rec`; return it in `ImportResult.Record`. In this milestone the `.import.json` side file (`:130-135`) and `last-deck.txt` (`:51-58`, `:72-76`) are still written; Milestone 2 removes them.
- `internal/game/import.go:168-224` — `SetOpponentDeck` keeps its current signature in this milestone (reads `.import.json`); its result already comes from `m.Import`, so it carries the new record. Note: a game loaded from disk with `Manager.Load` has no in-memory record in this milestone (the interim message is the generic "has no record" below).
- `internal/game/manager.go:87-95` — add `func (m *Manager) Record() (Record, error)`: `Current()` error as is; if `g.record == nil` return `fmt.Errorf("game %s was not imported from a battle log in this session, so it has no record", g.ID)` (Milestone 2 trims "in this session").
- `internal/game/record_roundtrip_test.go` (new, package `game`) — `TestRecordRestoresCorpus`: for each `testdata/logs/*.txt` (skip if corpus missing, as `import_test.go:17-30` does), `m1 := NewManager(t.TempDir(), nil)`, `Import` with `Seed: 0` (random) and the corpus deck (`testdata/decks/mega-lucario-ex-v12.txt`); `m2 := NewManager(t.TempDir(), nil)`; `Import(ImportRequest{Log: res.Record.JSON()})`. Assert: `g1.log.PathTo(g1.log.Head())` and `g2`'s are deep-equal (events, ids, payloads); for event ids `{1, head/2, head}` call `JumpTo(id, "")` on both and compare `State()` deep-equal (hands, prizes, deck order, discard, card names); compare `res.Report` turns/prizes. Repeat for one log with an assumed opponent list (the `TestSetOpponentDeck` list at `import_test.go:111-121`). `TestRecordAfterSetOpponentDeck`: rebuild with the list, assert `res.Record.OppDeck == list`, restore it in a fresh manager, compare as above. `TestRecordRejectsExtras`: record plus `Deck` → error. `TestNewGameHasNoRecord`: `m.New(...)` then `m.Record()` errors.
- If any corpus log fails the round trip, STOP and report (see Open Questions): do not add events to the record.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential execution (game field and import wiring, then round-trip tests; run the corpus test early to surface any determinism gap).

### Task: Give the assistant the record through the tools

**File changes**:
- `internal/mcpserver/server.go:149-155` — `ImportLog.Log` jsonschema: `"the battle log text copied from Pokémon TCG Live, or a game record returned earlier by import_log, set_opponent_deck or get_record (pass it unchanged) to restore that exact game"`; `Deck`, `OppDeck`, `Me` descriptions add "omit when log is a record".
- `internal/mcpserver/server.go:325-336` — `import_log` description: add "Also restores a game from its record. The result's record holds the log, both lists, the player's name and the seed: give it to the player to keep; importing it later rebuilds the identical game (branches and notes are not kept)." Result map gains `"record": res.Record` (the `Record` struct marshals as the nested object). Keep the web-mcp-docker `board` key via its helper.
- `internal/mcpserver/server.go:337-356` — `set_opponent_deck` result map gains `"record": res.Record`; description adds "Returns the updated record."
- `internal/mcpserver/server.go` (after `set_opponent_deck`) — new tool `add(srv, "get_record", "The current game's record: the battle log, the player's name, both decklists and the seed, as JSON the player can keep in a note and later pass to import_log to restore the identical game. Games started with new_game have none.", func(ctx, _ Empty) (any, error) { rec, err := s.M.Record(); if err != nil { return nil, err }; return rec, nil })`.
- `internal/mcpserver/server_test.go:124-175` — extend `TestImportLogTool`: unmarshal `record` from the import result; `record.log == logText`, `me` non-empty, `deck` equals the deck passed, `seed` non-empty string; `get_record` returns the same record; then `set_opponent_deck` with a 60-card list → result's `record.opp_deck` equals it and `get_record` matches; then `import_log` with `log` = the record JSON (as text) → succeeds, `get_state view=reveal` equals the pre-restore reveal state taken right after the rebuild.
- `internal/mcpserver/server_test.go:49-122` — in `TestMCPRoundTrip` after `new_game`, `get_record` is an error mentioning "no record".
- `internal/mcpserver/server_test.go` — if web-mcp-docker's `TestBoardLinks` exists, assert `record` and `board` both present on `import_log` and `set_opponent_deck` results. Tests run under web-mcp-docker's `forEachTransport` when present.

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Tell the player about the record when importing

**File changes**:
- `skills/tcg-import-game/SKILL.md:3` — description: mention "or pastes a saved game record" as a trigger (stay under 1024 characters).
- `skills/tcg-import-game/SKILL.md:30-36` (step 3) — add: "If the player pastes a game record (JSON starting with `{` and containing `tcgreplay_record`), pass it to `import_log` as `log` exactly as pasted, with no other arguments, and skip steps 2 and 4's list fixing: the record already holds both lists."
- `skills/tcg-import-game/SKILL.md:47-52` (step 5) — add a bullet: "The game record from the result (`record`), in a code block, exactly as returned: the player can keep it in their notes and paste it back to bring this exact game back later. Branches and notes made during the review are not in it and last only for this session." Also after `set_opponent_deck` in step 4: offer the updated record instead.
- Do not write "Claude" or a player's name (`skills/skills_test.go:40-46`).

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Keep games in memory only

**File changes**:
- `internal/store/store.go:89-108`, `:128-173`, `:175-216`, `:225-249` — remove `Create`, `Open`, `replay`, `write`, `Close`, `Path`, the `record` line type (`:63-73`) and the `path`/`f` fields (`:77-79`); drop the `write` calls in `Append` (`:279-290`), `PutAction` (`:293-299`), `Annotate` (`:308-317`), `LogRead` (`:331-337`), `Branch` (`:358-370`), `Checkout` (`:373-382`), keeping their validation and in-memory effects (their `error` returns stay for API stability). Update the package comment (`:1-4`): an in-memory event tree; nothing is persisted. Remove now-unused imports (`bufio`, `os`, `path/filepath`).
- `internal/game/game.go:112` — remove `Close` (no file to close); remove its callers.
- `internal/game/manager.go:22-33` — `Manager` gains `lastDeck string` (guarded by `mu`); doc comment: "owns the current game for this session, in memory only; it never reads or writes game files".
- `internal/game/manager.go:87-95` — `Current` error: `"no game open: import a battle log or a saved game record with import_log, or start one with new_game"`.
- `internal/game/manager.go:97-112` — `setCurrent`: drop `m.current.Close()` and the `current.txt` write.
- `internal/game/manager.go:114-199` — remove `gamesDir`, `idRe`, `path`, `GameInfo`, `List`, `Resume`, `Load`.
- `internal/game/manager.go:249-282` — `New`: `l := store.NewMemory(store.Header{...})`; drop the path and `os.Remove`.
- `internal/game/manager.go` `Record()` — message becomes `"game %s was started with new_game, not imported from a battle log, so it has no record"`.
- `internal/game/import.go:49-58`, `:72-76` — replace `last-deck.txt` read/write with `m.lastDeck` under `m.mu` (error text unchanged: "no decklist given and no previous import to reuse one from"). Remember the deck only after a successful import.
- `internal/game/import.go:98-111` — `store.NewMemory` instead of `store.Create`; `fail` just returns the error. Remove the `.import.json` write (`:130-135`) and the now-unused `nonID`-free imports (`os`, `path/filepath`, `encoding/json` if unused). Keep generated ids (`import-<time>-<opp>`).
- `internal/game/import.go:162-224` — replace with `func (m *Manager) SetOpponentDeck(ctx context.Context, oppDeck string, discardBranches bool) (*ImportResult, error)`: `g := Current()`; `g.record == nil` → `fmt.Errorf("game %s was not imported from a battle log, so it cannot be rebuilt with an opponent list", g.ID)`; blank list → existing error; branches other than `main` and `!discardBranches` → `fmt.Errorf("game %s has branches %s that a rebuild would lose; ask the player, then pass discard_branches to rebuild anyway", ...)`; build `ImportRequest{ID: g.ID, Log, Deck, Me, Seed from g.record, OppDeck: oppDeck}` and `m.Import` (which replaces the current game; on failure the current game stays as it was because `setCurrent` only runs on success).
- `internal/game/import_test.go:97-160` — `TestSetOpponentDeck`: new signature; branch refusal then `discardBranches=true` succeeds; drop the `new_id` case; add refusal for a `new_game` game.
- `internal/game/memory_test.go` (new) — `TestNothingWritten`: temp data dir pre-seeded with `games/old.jsonl`, `games/old.import.json`, `current.txt` (naming `old`), `last-deck.txt`; snapshot every file (path → bytes); run `Import` (corpus log), a few verbs, `JumpTo` (branch), `Annotate`, `SetOpponentDeck(..., true)`, `New`; snapshot again excluding `assistant.txt` and `tcgreplay.log`; must be equal (no new, changed or removed files). `TestRestartForgets`: after that, `NewManager(sameDir, nil).Current()` errors. `TestImportReplaces`: second import replaces first (`Current().ID` is the second; first game unreachable). `TestLastDeckInMemory`: import with deck, import without deck succeeds; a new manager on the same dir importing without deck fails.
- `internal/mcpserver/server.go:166-168`, `:357-366` — remove `list_games`, `load_game` and the `GameID` input type.
- `internal/mcpserver/server.go:160-164`, `:337-356` — `OppDeck` input: remove `ID` and `NewID`; add `DiscardBranches bool "json:\"discard_branches,omitempty\" jsonschema:\"rebuild even though the game has branches, which are lost; only after the player agrees\""`; handler calls `s.M.SetOpponentDeck(ctx, in.Deck, in.DiscardBranches)`; description: "Rebuild the current imported game … Replaces the game; refuses when it has branches unless discard_branches is set. Returns the updated record."
- `internal/mcpserver/server.go` (web-mcp-docker's hosted instructions line) — drop `load_game` from "Results of import_log, new_game, load_game and set_opponent_deck carry a board link"; drop `load_game` from the board-link helper's callers.
- `internal/mcpserver/server_test.go` — `TestToolList` (new or added to the round trip): `ListTools` contains `get_record`, not `list_games`/`load_game`; `set_opponent_deck`'s input schema has `discard_branches` and no `id`/`new_id`. Update web-mcp-docker's `TestBoardLinks` to drop its `load_game` call.
- `internal/webui/actions.go:103-113` — remove the `list_games` and `load_game` cases (they fall through to "no game open" / unknown verb like any other verb).
- `internal/webui/hub.go:124-129` — remove `M.List()` and `msg["games"]`.
- `internal/webui/webui_test.go:64-69` — remove `Games`; add a test that with no game open the state message has no `games` key and a `load_game` request returns an error.
- `cmd/tcgreplay/main.go:55` — `-data` help: `"directory for the playmat's log and daemon files"`.
- `cmd/tcgreplay/main.go:111-113` — remove the `m.Resume()` call in `runInProcess`.
- `cmd/tcgreplay/daemon.go:163-166` — remove the `m.Resume()` call.
- `cmd/tcgreplay/hosted.go` (from web-mcp-docker; if present) — remove its `m.Resume()` call in `runHosted`.
- `cmd/tcgreplay/main.go:1-11` — package comment: the daemon "owns the current game (in memory) and the playmat UI".
- Verify with `grep -rn "list_games\|load_game\|Resume\|import.json\|last-deck\|current.txt\|store.Create\|store.Open" --include='*.go' .` → no hits outside tests asserting absence.

**Complexity**: High
**Token estimate**: ~60k tokens
**Agent strategy**: Parallel analysis, sequential integration — one agent does the store and game package (removal, memory-only manager, SetOpponentDeck, game tests); then, once `go build ./internal/game/...` passes, a second pass fixes callers in `internal/mcpserver`, `internal/webui` and `cmd/tcgreplay` with their tests; finish with `make test`.

### Task: Remove stored games from the board page

**File changes**:
- `web/src/App.svelte:74-82` — landing: keep `<h1>No game loaded</h1>` (or "No game open"); replace the paragraph with `Ask {app.assistant} to import a battle log or a saved game record, or to start a new game.`; remove the `{#each app.games …}` buttons.
- `web/src/lib/ws.js:41` — remove `app.games = msg.games || []`.
- `web/src/lib/ws.js:66` — remove `'list_games'` from `READS`.
- `web/src/lib/store.svelte.js:14` — remove `games: []`.
- Rebuild with the existing `make build` (the `dist/app` output is gitignored, `internal/webui/.gitignore`); no committed build artefacts change.

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Point the drills at the game open now

**File changes**:
- `skills/tcg-replay-drill/SKILL.md:12-14` — "Find a puzzle": use the game open on the playmat (`get_log`, view `reveal`, for annotations beginning `Finding (`). If no game is open, ask the player to import one (a battle log, or a saved game record) and stop. If the open game has no findings, say so and offer to review it first with `tcg-review-game`. Note that findings live only for the session, so a restored record needs a review before drilling. Description (`:3`): "in the open game" instead of "in the player's games".
- `skills/tcg-numbers-drill/SKILL.md:12` — "Use cards from the game open on the playmat: the player's deck (`get_profile` for their seat) and the opponent's (`get_profile`). If no game is open, ask the player to import one (a battle log or a saved game record)." Remove `list_games`. Description: "from the game open on the tcgreplay playmat".
- `skills/tcg-before-you-move/SKILL.md:12` — "Use the game open on the playmat: its current head, or ask which turn. If no game is open, ask the player to import one (a battle log or a saved game record) and stop." Remove `list_games`/`load_game`.
- `skills/tcg-opponent-deck/SKILL.md:57` — "For a game already imported: `set_opponent_deck(deck)`. It rebuilds the open game from its record (same log and seed), so the player's side is unchanged, and returns the updated record to offer the player. If the game has branches it refuses: they would be lost. Ask the player; only if they agree, call it again with `discard_branches: true`."
- `skills/skills_test.go` — add `TestSkillsUseCurrentTools`: no skill file contains `list_games`, `load_game` or `new_id`.
- No "Claude"/"Nic " in any skill (`skills/skills_test.go:40-46`).

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Describe memory-only games in the design document

**File changes**:
- `TCG Review Playmat Design and Build Plan.md:30` — the "restart rebuilds state from disk" sentence: state that a restart now starts with no game, and games come back by importing their record.
- `TCG Review Playmat Design and Build Plan.md:289` — the `import_log` "saves a log as a review-mode game" sentence: games are held in memory for the session; the result carries the game record.
- `TCG Review Playmat Design and Build Plan.md:296` — `set_opponent_deck` "(saved as `<id>.import.json`)": now rebuilds the current game from its in-memory record; refuses with branches unless `discard_branches`.
- `TCG Review Playmat Design and Build Plan.md:299` — runtime paragraph: the daemon (and hosted mode) owns the current game in memory; nothing about saved games; old files in the data dir are ignored.
- Add a short "Game records" paragraph near `:296`: the record's fields, its version marker and check, restoring through `import_log`, repair of curly quotes and non-breaking spaces, and that branches/notes/findings are not included.
- Check the whole document for `list_games`, `load_game`, `games/`, `current.txt`, `last-deck` and update any remaining mention.

**Complexity**: Low
**Token estimate**: ~10k tokens
**Agent strategy**: Single agent, sequential execution.

## Testing Strategy

Tests use Go's `testing`, beside the code, run by `make test`; the card DB is nil or the existing fake, so nothing needs network access.

- **Define the game record format** — `internal/game/record_test.go`: seal/parse round trip; compacted and re-indented JSON; curly structural and value quotes; NBSP and CRLF in values; damaged content (collapsed space, edited count, edited seed, missing check) rejected with `ErrRecordDamaged`; newer version refused; corpus log text not a record; max-uint64 seed; fenced record.
- **Build and restore games from records** — `internal/game/record_roundtrip_test.go`: every corpus log with a random seed, with and without an opponent list, restored in a fresh manager: deep-equal event paths and deep-equal `State()` at events 1, head/2 and head (via `JumpTo`); record after `SetOpponentDeck` restores exactly; record plus extras refused; `new_game` game has no record.
- **Give the assistant the record through the tools** — `internal/mcpserver/server_test.go`: `record` on `import_log` and `set_opponent_deck` results with the right fields; `get_record` equals the latest; `get_record` after `new_game` errors; restoring via `import_log` matches the reveal state; `record` and `board` together when hosted. Under web-mcp-docker's `forEachTransport`, all of this also runs over Streamable HTTP.
- **Tell the player about the record when importing** — `skills/skills_test.go` format test.
- **Keep games in memory only** — `internal/game/memory_test.go`: nothing written (pre-seeded dir byte-identical, excluding `assistant.txt` and `tcgreplay.log`), restart forgets, import replaces, last deck in memory only; `internal/game/import_test.go` `TestSetOpponentDeck` with `discardBranches`; `internal/mcpserver/server_test.go` tool list without `list_games`/`load_game`, `set_opponent_deck` schema; `internal/webui/webui_test.go` no `games` key and `load_game` refused.
- **Remove stored games from the board page** — no automated UI tests exist for the Svelte app; covered by the manual board landing-page check and by `make build` succeeding.
- **Point the drills at the game open now** — `skills/skills_test.go` `TestSkillsUseCurrentTools` (no `list_games`, `load_game`, `new_id`) plus the format test.
- **Describe memory-only games in the design document** — none (documentation).

Manual checks (captured in the implementation test plan): restart forgets on desktop and hosted; nothing written on a real install; exact restore by hand at start/middle/end; Notion paste round trip; damaged paste through a real note app; drills with and without an open game; board landing page; success metrics (10 real games restore without differences; records saved to Notion without hand-editing).

## Project References

- Spec `20261017112555-30c99ab1-memory-store-record` (final) — requirements, constraints, acceptance criteria and success metrics this plan implements.
- Plan `20261017105035-e3f64c1b-web-mcp-docker` (final, upstream) — hosted mode, `BoardURL`/`board` switch, two-transport MCP tests, `cmd/tcgreplay/hosted.go`; this plan removes its `Resume` call and `load_game` link.
- Spec `20261017143118-b05aa6b1-per-user-games` (downstream) — per-account games in memory; builds on the record living on the `Game`.
- Spec `20261017123240-21493a95-google-login` (parallel) — no shared files.
- Design documents: none referenced by the spec.
- Knowledge: the `tcgreplay` repo store has no entries (always-applied load and topic searches empty).
- Repo root: `tcgreplay` → `/home/nicj/code/github.com/nicholasjackson/tcgreplay`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Per task: Define the game record format ~25k (Medium); Build and restore games from records ~35k (Medium); Give the assistant the record through the tools ~20k (Low); Tell the player about the record ~8k (Low); Keep games in memory only ~60k (High); Remove stored games from the board page ~8k (Low); Point the drills at the game open now ~12k (Low); Describe memory-only games ~10k (Low). Total ~180k. Keep `internal/mcpserver/server.go` reads to the ranges named (it is ~800 lines).

## Migration Notes

- No data migration. Files already in the data directory (`games/*.jsonl`, `games/*.import.json`, `current.txt`, `last-deck.txt`) are left in place, never read and never deleted; a player who wants an old game back imports its battle log again (with the original seed lost, hidden cards may be dealt differently from the old file).
- Behaviour change for desktop users: restarting the playmat no longer reopens the last game; branches and notes vanish on restart (spec risk, accepted). The import skill tells the player to keep the record.
- Tool contract changes for any external caller: `list_games`, `load_game` removed; `set_opponent_deck` loses `id`/`new_id` and gains `discard_branches`; `import_log` results gain `record`. The bundled skills are updated in the same change; installed copies are refreshed by `tcgreplay install`.
- If web-mcp-docker lands after this plan's Milestone 2, its implementer must not reintroduce `m.Resume()` in `runHosted` or a `load_game` board link.

## Performance Considerations

- `import_log` and `set_opponent_deck` results now include the whole battle log inside the record (typically 10–40 KB), roughly doubling the import result's size in the assistant's context. Accepted: the spec requires the transcript in the result. `get_record` lets the assistant fetch it again later instead of keeping it in view.
- Memory: a game's event log was already fully in memory (the file was append-only alongside it); dropping the file removes per-event `fsync` calls (`internal/store/store.go:237`), so moves get slightly faster. One game per process, as today; per-user-games bounds memory for many accounts.
- The checksum is one SHA-256 over a few tens of KB per import or restore: negligible.
