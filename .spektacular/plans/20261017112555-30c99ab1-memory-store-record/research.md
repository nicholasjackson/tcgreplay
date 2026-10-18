---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Research: 20261017112555-30c99ab1-memory-store-record

## Alternatives considered and rejected

### Option: Keep game files but write them to a temp directory that is wiped on start

**Rejected**: the spec forbids writing or reading saved games at all ("No lasting game storage"), and a wiped temp dir is still lasting storage between a crash and the next start. The store already has an in-memory mode (`internal/store/store.go:110-116`, `NewMemory`), and every write path is a no-op when the log has no file (`internal/store/store.go:225-228`), so memory-only costs nothing.

### Option: Keep `store.Create`/`store.Open` (file mode) in the store package, unused

**Rejected**: once the game manager stops calling them (`internal/game/manager.go:188`, `:264`, `internal/game/import.go:102`) they are dead code whose only purpose is the behaviour the spec removes. Removing them makes "never written or read" structural rather than a convention. The history keeps them if a later spec (cross-game drilling, a non-goal here) wants files back.

### Option: A separate `restore_game` tool taking a record

**Rejected**: the spec's Technical Approach prefers restoring "through import accepting a full record including its seed". `import_log` already takes a `seed`-capable request (`internal/game/import.go:23-36`); the only gap is that the MCP input does not expose it (`internal/mcpserver/server.go:149-155`).

### Option: A separate `record` input field on `import_log` (typed object)

**Rejected** in favour of accepting the record as the `log` text: the player pastes the record from a note, and the assistant passes it through verbatim; a typed object would make the assistant re-transcribe it into tool-argument JSON and move quote-damage repair out of the server's reach. A battle log never starts with `{`, so detection is unambiguous.

### Option: Base64 / compressed blob as the record

**Rejected**: the spec asks for the record "returned as JSON" and readable text that pastes into a note. A JSON object with a checksum survives reformatting (whitespace between tokens is irrelevant) and catches value damage.

### Option: Detecting damage by re-importing and comparing state

**Rejected**: there is nothing to compare against after a restart. A checksum over the record's canonical content is the only way to tell "same game" from "different game" from the record alone.

### Option: Rejecting any record whose checksum does not match, with no repair

**Partly adopted**: Kept as the fallback, but the plan first tries the two normalisations a note app is known to apply (curly quotes to straight, non-breaking spaces and CRLF to plain) and accepts only if the checksum then matches. This meets "restored correctly or rejected", never "silently different".

### Option: Storing the reconciled opponent profile in the record instead of the list as passed

**Rejected**: reconciliation is a deterministic function of the list and the log (`internal/importer/importer.go:575`, `internal/infer/infer.go:98`), so the list as passed plus the seed reproduces the game exactly; storing the reconciled profile would feed different counts back into `oppLimit` (`internal/importer/importer.go:79-84`) and could change the replay.

### Option: Keeping `set_opponent_deck`'s `id`/`new_id` parameters

**Rejected**: with no stored games there is only the current game to rebuild, and "build a separate game" can no longer keep the old one. Replaced by acting on the current game, with an explicit opt-in to discard branches.

## Chosen approach — evidence

- `internal/store/store.go:110-116` — `NewMemory` exists; `write` is a no-op without a file (`:225-228`), so every `Append`/`Branch`/`Annotate` works in memory unchanged.
- `internal/game/manager.go:249-282` — `Manager.New` is the only place play games are created; swapping `store.Create` for `store.NewMemory` and dropping the path/remove calls is local.
- `internal/game/import.go:49-140` — `Manager.Import` already assembles exactly the record's contents: log, resolved deck (falls back to `last-deck.txt`, `:51-58`), opp deck, `Me`, and the seed actually used (`req.Seed = res.Seed`, `:130`). It writes them as `<id>.import.json` (`:131-135`). The record is this struct, completed with the detected player name (`res.Report.Me`) and a checksum, held in memory on the game instead of written.
- `internal/importer/importer.go:72-75`, `:118`, `:209` — the importer's only randomness is a PCG seeded from `Seed`; given the same log, decks, `Me` and seed the replay is deterministic. `TestSetOpponentDeck` (`internal/game/import_test.go:97-160`) already relies on "same log and seed → owner side unchanged".
- `internal/deck/deck.go:133-166` — `Resolve` only fills `Def`; card names (which decide placement) come from the list text, so card-database availability does not move cards.
- `internal/game/import.go:168-224` — `SetOpponentDeck` reads `<id>.import.json` and re-imports; it becomes "take the current game's in-memory record, swap the opp deck, re-import".
- `internal/game/manager.go:97-112`, `:162-180` — `current.txt` write and `Resume`; called from `cmd/tcgreplay/main.go:111` and `cmd/tcgreplay/daemon.go:164`. Removing both gives "a new session starts with no game open".
- `internal/mcpserver/server.go:325-366` — `import_log`, `set_opponent_deck`, `list_games`, `load_game` tool registrations; `:149-169` their inputs.
- `internal/webui/actions.go:103-113`, `internal/webui/hub.go:124-128`, `web/src/App.svelte:74-80`, `web/src/lib/ws.js:41,66`, `web/src/lib/store.svelte.js:14` — the board's own list/load-games path (landing-page buttons), which must go too for "No reopening earlier games".
- `internal/mcpserver/server_test.go:17-47` — `connect` helper with `game.NewManager(t.TempDir(), nil)`; tests use the corpus in `testdata/` with `Seed: 1`.
- `skills/skills_test.go:12-48` — skills must not mention "Claude" or "Nic "; any skill edits must respect that.

## Files examined

- tcgreplay:internal/store/store.go:1-397 — append-only event log; file mode (`Create`/`Open`/`write`/`Close`) and memory mode (`NewMemory`); branches, actions, annotations, reads all in memory with optional file append.
- tcgreplay:internal/game/manager.go:22-282 — `Manager{Dir, DB, Rand}`; `assistant.txt` (label, not game storage); `current.txt` + `Resume`; `List`/`Load`/`path`/`gamesDir`; `New` creates file-backed play games.
- tcgreplay:internal/game/import.go:20-224 — `ImportRequest{ID, Log, Deck, OppDeck, Me, Seed}`; `last-deck.txt`; `.import.json`; `SetOpponentDeck` with branch guard and `new_id`.
- tcgreplay:internal/game/game.go:68-112 — `Options`, `Load(id, log, db, rng)`; review mode iff `Header.Source == "import"`; `Close` closes the log.
- tcgreplay:internal/importer/importer.go:19-130,195-240,560-600 — `Options{Me, MyDeck, OppDeck, Seed, DefOf}`; seed handling; PCG; reconciliation via `infer.Reconcile`.
- tcgreplay:internal/importer/parse.go:145-180 — `Players` detects the log owner; passing the detected name back is equivalent.
- tcgreplay:internal/deck/deck.go:133-166 — `Resolve` fills `Def` only.
- tcgreplay:internal/mcpserver/server.go:17-47,49-57,149-169,312-370,700-811 — tool server, instructions, inputs, game-management tools, `get_profile`/`sample_hidden`/`annotate`.
- tcgreplay:internal/mcpserver/server_test.go:1-186 — in-memory transport `connect`, `call`, round-trip and import tests (deck reuse is tested at `:159-162`).
- tcgreplay:internal/game/import_test.go:17-420 — corpus import tests, all with `NewManager(t.TempDir(), nil)` and explicit IDs; `TestSetOpponentDeck` exercises branch refusal and `new_id`.
- tcgreplay:internal/webui/actions.go:85-120 — board verbs `list_games`/`load_game` before the current-game check.
- tcgreplay:internal/webui/hub.go:115-130 — state message includes `games` when no game is loaded.
- tcgreplay:internal/webui/webui_test.go:60-90 — `Games []game.GameInfo` in the test's state-message struct.
- tcgreplay:web/src/App.svelte:74-82 — landing page: "load a stored game" buttons.
- tcgreplay:web/src/lib/ws.js:36-44,66 — `app.games`, `list_games` in READS.
- tcgreplay:cmd/tcgreplay/main.go:36-135 — flags (`-data` "directory for game files"), `newServer`, `runInProcess` calls `Resume`.
- tcgreplay:cmd/tcgreplay/daemon.go:160-170 — daemon calls `Resume`.
- tcgreplay:skills/tcg-replay-drill/SKILL.md:12 — finds findings via `list_games`/`load_game`.
- tcgreplay:skills/tcg-numbers-drill/SKILL.md:12 — opponents via `list_games`.
- tcgreplay:skills/tcg-before-you-move/SKILL.md:12 — positions via `list_games`/`load_game`.
- tcgreplay:skills/tcg-opponent-deck/SKILL.md:57 — tells the assistant to pass `new_id` when the game has branches.
- tcgreplay:skills/tcg-import-game/SKILL.md:30-52 — import steps and triage; place to tell the player about the record.
- tcgreplay:skills/skills_test.go:12-48 — format test; forbids "Claude" and "Nic " in skills.
- tcgreplay:TCG Review Playmat Design and Build Plan.md:289-299 — design doc describes `.import.json`, saved games and the daemon owning the games; needs updating.
- tcgreplay:Makefile:1-40 — `make test` = `go vet ./...` + `go test ./...`; `make build` rebuilds the board into `internal/webui/dist/app` (gitignored).

## External references

- None needed. Go standard library `crypto/sha256`, `encoding/json` (struct marshalling is deterministic in field order, which makes it usable as the checksum's canonical form).

## Prior plans / specs consulted

- Plan `20261017105035-e3f64c1b-web-mcp-docker` (final) — adds `tcgreplay -hosted` (in-process manager + MCP on one HTTP mux), `mcpserver.Server.BoardURL` adding a `board` key to `new_game`, `import_log`, `load_game`, `set_opponent_deck` results, and a hosted run mode that "reopens the last game" with game files under `$HOME`. This plan builds on it: `Resume` is removed so the hosted mode cannot reopen anything, `load_game` (and its board link) disappears, `board` stays on the remaining game-opening results, and game files are no longer written in any mode. Its Out of Scope names this spec as the owner of in-memory storage and the record.
- Spec `20261017143118-b05aa6b1-per-user-games` (overview and constraints only) — will key the current game by signed-in account and keep every account's game in memory. Hence the record and the "has no record" state live on the `Game`, not as a single field on the `Manager`, so a later per-account lookup carries them along unchanged.
- Spec `20261017123240-21493a95-google-login` — not read in detail; it shares the public-URL setting and does not touch game storage.

## Open assumptions

- **Verified during planning (experiment, since removed):** for all 10 corpus logs, with and without an assumed opponent list, three runs each with random seeds, re-importing with the saved seed and the detected player name passed back as `Me` produced deep-equal event paths and deep-equal final states (20 comparisons per run, all passing, card DB nil). Exact restore from log + decks + name + seed therefore holds for the corpus. If a future log does not round-trip, STOP: exact restore would need the events themselves in the record, which contradicts the spec's "raw game only" constraint.
- Card-database availability (online vs offline) only affects card `Def`s and art, never which card sits where. If a restore offline places cards differently, STOP and ask.
- No other process or test relies on `games/*.jsonl`, `*.import.json`, `current.txt` or `last-deck.txt` existing (grep found only the manager, import, daemon and main).
- The web-mcp-docker plan is implemented before this one (epic dependency); its hosted run mode calls `m.Resume()`, which this plan removes.

## Drafting assumptions

### Chosen direction: memory-only games with a seeded, checksummed record restored through import_log (architecture)
- **Decision**: Hold every game in `store.NewMemory`; remove game files, `.import.json`, `current.txt`, `last-deck.txt`, `Resume`, `List`, `Load` and the store's file mode. Each imported game carries a versioned JSON record (log, player name, deck, opp list as passed, seed as a string, SHA-256 check) on the `Game`; returned by `import_log`, `set_opponent_deck` and a new `get_record`; restored by passing it as `import_log`'s `log`.
- **Key design decisions**: record on the `Game` (forward-compatible with per-user-games); determinism of the seeded importer is relied on and proven by a corpus-wide round-trip test; checksum over values, not layout; repair only the known note-app substitutions and only when the check then matches; `set_opponent_deck` on current game only with `discard_branches`; last deck remembered in manager memory; board UI list/load removed too.
- **Rationale**: Uses the in-memory mode and the import data the spec points at; smallest change that satisfies every requirement in both desktop and hosted modes.
- **Rejected**: Files in a wiped temp dir (still storage); dormant file mode; full event log in the record (breaks "raw game only"); separate restore tool (spec prefers import).

### No conventions to apply (architecture)
- **Decision**: Conventions section states that none apply; existing code patterns followed.
- **Rationale**: The knowledge store is empty.
- **Rejected**: Inventing conventions.

### Seed encoded as a decimal string (architecture)
- **Decision**: The record's `seed` is a JSON string.
- **Rationale**: Seeds use the full 64-bit range; JSON tools built on doubles lose precision above 2^53.
- **Rejected**: A JSON number.

### Remove the store's file mode (discovery)
- **Decision**: Delete `store.Create`/`store.Open` and the file plumbing; games live only in `store.NewMemory` logs.
- **Rationale**: The spec forbids writing or reading saved games; dead file code invites accidental reuse. History keeps it.
- **Rejected**: Leaving the file functions unused (keeps a path back to the forbidden behaviour).

### Record restored through `import_log`'s `log` text (discovery)
- **Decision**: `import_log` accepts either a battle log or a record (JSON object text) in `log`; a record is detected by a leading `{` and its `tcgreplay_record` marker.
- **Rationale**: Spec prefers restore through import; the player pastes text, so the server receives it verbatim and can repair quote damage.
- **Rejected**: A separate restore tool; a typed `record` object parameter.

### Checksum plus known-damage repair (discovery)
- **Decision**: The record carries a SHA-256 check over its canonical content. On mismatch the server retries with curly quotes straightened and non-breaking spaces/CRLF normalised, and accepts only if the check then matches; otherwise it rejects with a "damaged" message.
- **Rationale**: Meets "restored correctly or rejected, never silently different".
- **Rejected**: No check (silent wrong restores); reject-only (fails the common Notion curly-quote case needlessly).

### Opponent deck stored as passed (discovery)
- **Decision**: The record's `opp_deck` is the list text as passed to the import or `set_opponent_deck`, not the reconciled profile.
- **Rationale**: With the seed, the list as passed reproduces the game exactly; the reconciled list could change the replay.
- **Rejected**: Storing the reconciled profile.

### `set_opponent_deck` acts on the current game; `discard_branches` replaces `new_id` (discovery)
- **Decision**: Drop `id` and `new_id`; refuse to rebuild a game with branches unless `discard_branches: true`.
- **Rationale**: Without stored games there is no other game to name and no way to keep the old one; an explicit opt-in keeps today's protection.
- **Rejected**: Keeping `new_id` (now meaningless); silently discarding branches.

### Record first, storage removal second (milestones)
- **Decision**: Milestone 1 adds the record while games are still saved; Milestone 2 removes storage.
- **Rationale**: Each milestone ships alone, and storage is never removed before there is a way to bring games back.
- **Rejected**: One milestone (no independent checkpoint); removal first (games unrecoverable in between).

### Interim: games loaded from disk have no record in Milestone 1 (tasks)
- **Decision**: In Milestone 1 only games imported or rebuilt in the session carry a record; `get_record` on a game loaded from disk says it has none.
- **Rationale**: Milestone 2 removes loading; deriving records from old `.import.json` files would be throwaway glue.
- **Rejected**: Back-filling records from old side files.

### Restoring a record sets the session's last deck (tasks)
- **Decision**: Importing a record remembers its deck as the last deck, as any import does.
- **Rationale**: Keeps one rule for "the deck of the last import"; the player can always pass a deck.
- **Rejected**: Excluding record restores from deck memory.

### Assistant label file stays (tasks)
- **Decision**: `assistant.txt` in the data dir is kept; the "nothing written" check excludes it and the process log.
- **Rationale**: It is a UI label, not a game or import data; the spec targets saved games and import data only.
- **Rejected**: Moving the label to memory (out of scope; would change labels after restart).

### Board landing-page task has no dependency (tasks)
- **Decision**: The Svelte landing-page change can land independently of the server change.
- **Rationale**: Removing buttons and the list read is harmless before or after the server stops sending games.
- **Rejected**: Serialising it behind the memory-only task.

### Version marker and check are not game content (verification)
- **Decision**: The record carries a version marker and a checksum alongside the five raw-game fields.
- **Rationale**: The spec's "raw game only" constraint excludes branches, notes and findings; the marker and check carry no game state and are what make "damaged records never restore wrong" possible.
- **Rejected**: A record of the five fields alone (damage would be undetectable).

## Rehydration cues

- `spektacular spec file read 20261017112555-30c99ab1-memory-store-record`
- `spektacular plan file read 20261017105035-e3f64c1b-web-mcp-docker plan`
- Re-read: `internal/game/manager.go`, `internal/game/import.go`, `internal/store/store.go`, `internal/mcpserver/server.go:140-370`, `internal/webui/actions.go:100-115`, `internal/webui/hub.go:115-130`, `web/src/App.svelte:70-82`, the three drill skills and `skills/tcg-opponent-deck/SKILL.md:57`.
- `grep -rn "list_games\|load_game\|Resume\|import.json\|last-deck\|current.txt" --exclude-dir=node_modules --exclude-dir=.spektacular .` to confirm every touch point is gone after implementation.
