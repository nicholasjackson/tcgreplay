---
created_date: "2026-10-19"
document_status: final
closed_date: "2026-10-19"
---

# Research: 20261019170829-bfb31628-turn-numbering-by-round

## Alternatives considered and rejected

- **Renumber the internal counter to combined turns.** Rejected: spec constraint 1, and rules key off the per-player count — `internal/game/verbs.go:218-223` (evolve warnings, `Turn <= 2`), `internal/game/verbs.go:406` (first-player attack warning, `Turn == 1`), `internal/game/reads.go:394` (first-player Supporter warning), `internal/state/apply.go:425-427` (effect expiry `UntilTurn <= s.Turn`), `internal/state/apply.go:235` (`EnteredTurn`), `internal/game/game.go:247-279` (draws_like_main matches by `e.Turn`).
- **Keep every output field raw and add a parallel `round` field.** Rejected: the assistant reads `get_state`/`list_branches` JSON directly (`internal/mcpserver/server.go:459,749`), so a raw `turn` would stay visible next to the combined one, against the spec's "no surface shows a per-player turn count" metric, and invites the assistant to use the wrong one.
- **Derive the turn player from turn parity in the UI.** Rejected: duplicates the rule in JS and Go; the server already knows `First` (`internal/state/state.go:76`, set on turn 1 start in `internal/state/apply.go` near line 417).

## Chosen approach — evidence

- Output types are separate from stored state: `BoardView` (`internal/state/view.go:63`, filled at `:156`), `LogLine` (`internal/game/reads.go:119-127`), `BranchInfo` (`internal/game/reads.go:~418`). Their `turn` fields can carry the combined number without touching `State.Turn` or `Event.Turn`.
- Log lines are built in two places from the same pattern: `internal/game/ui.go:81` (snapshot for the web UI) and `internal/game/reads.go:165` (get_log); both have the post-event state `st`, which holds `First`.
- Turn text: `describe` renders `turn %d %s (%s)` (`internal/game/reads.go:111`) and effect expiry `(until end of turn %d)` (`:101`); `LogLine.String` renders `#%d T%d` (`:131`). Write results' `delta` strings come from `describe`, so fixing `describe` fixes deltas.
- Effect expiry input: MCP `add_effect.until_turn` (`internal/mcpserver/server.go:377,735-741`) and web action `until_turn` (`internal/webui/actions.go:47,252`) both pass straight into `Game.AddEffect` (`internal/game/verbs.go:571`). The web UI never sends `until_turn` (no hit in `web/src`).
- Messages: wrong-seat errors `internal/game/verbs.go:161`, `internal/game/game.go:431`; start-turn action label `Turn %d` `internal/game/verbs.go:171`; draws_like_main warning `internal/game/game.go:279`; end-turn prompt `internal/webui/actions.go:261`.
- Persistence: nothing but game records is persisted (`internal/store/store.go:1-4`), and a record is the battle log text + seed + decks (`internal/game/record.go:33-50`), rebuilt through the importer. Action labels and event logs are regenerated on load, so old records pick up the new numbering with no migration.
- TCG Live logs carry no turn numbers ("X's Turn"), and the importer's own labels are `<player>'s Turn` (`internal/importer/importer.go:297`).
- Web UI: Strip header `Turn ${b.turn} · who` (`web/src/components/Strip.svelte:41-42`), effect chip `until end of T{e.until_turn}` (`:93`); Timeline groups by `l.turn`, finds turn start by regex `/^turn \d+ start/`, rewinds to branch `turn-${t.turn}` (`web/src/components/Timeline.svelte:9-35,79`); PlaybackBar `T{current.board.turn}` (`web/src/components/PlaybackBar.svelte:30`).
- Skills: `skills/tcg-review-game/SKILL.md:13-14,34` (`review-t4`, "Name turns both ways"), `skills/tcg-before-you-move/SKILL.md:13` (`bym-t<N>`), `skills/tcg-replay-drill/SKILL.md:19` (`drill-<game>-t<N>`). `.claude/skills` and `.agents/skills` are symlinks to `skills/`.

## Files examined

- `internal/state/state.go:55-80` — `Effect.UntilTurn`, `State.Turn/Current/First`.
- `internal/state/apply.go:400-430` — turn start/end; `First` set on turn 1; effect expiry on turn end.
- `internal/state/view.go:60-70,150-160` — `BoardView` fields and projection.
- `internal/state/events.go:42,154` — `Event.Turn`, `TurnPayload`.
- `internal/game/reads.go:95-135,160-175,410-440` — describe, LogLine, get_log lines, BranchInfo.
- `internal/game/ui.go:60-110,146-165` — snapshot log lines/branches; `TurnStartedAt`.
- `internal/game/verbs.go:155-180,210-225,400-410,570-580` — EndTurn, evolve/attack warnings, AddEffect.
- `internal/game/game.go:240-280,425-435` — draws_like_main, wrong-seat error.
- `internal/mcpserver/server.go:60-70,370-380,459-490,735-760` — instructions, AddEffect args, get_state/get_log/list_branches.
- `internal/webui/actions.go:40-50,250-262` — web add_effect, end-turn prompt.
- `internal/webui/webui_test.go:301` — asserts the end-turn prompt contains "turn 2" (internal turn 2 = combined turn 1); must change.
- `internal/game/game_test.go:143-226`, `internal/game/infer_test.go:43-55` — assert internal `st.Turn`/`TurnPayload.Number`; unaffected.
- `internal/importer/importer.go:297-335`, `internal/importer/replay.go:31-32` — import report `turns` (per player) and `total_turns` (sum of both players' turns); `importer_test.go:83` checks against `testdata/index.json`.
- `skills/skills_test.go:12,52` — skill format/tool tests; convention: no "Claude"/"Nic " in tcg skills.
- `web/src/components/{Strip,Timeline,PlaybackBar}.svelte`, `web/src/lib/store.svelte.js:85`.

## External references

- None needed; Pokémon TCG players count "turn N" per player in official rules, but the user wants the both-players count, which is what this plan delivers.

## Prior plans / specs consulted

- None relevant: the four prior plans cover hosting, Docker, Google login, per-user games and the memory store, not turn handling.

## Open assumptions

- Turns strictly alternate, so internal turn t belongs to `First` when t is odd and to the other seat when even; `apply.go` enforces `Number == Turn+1` and turn start seat is the next player (`verbs.go:170-173`, importer `:304`).
- Old saved records contain no turn numbers (battle log text only), so they need no conversion.
- Nothing outside this repo parses `LogLine` text or `turn` JSON except the web UI and the assistant.

## Drafting assumptions

### total_turns reports combined turns (discovery)
- **Decision**: the import report's `total_turns` becomes the higher of the two per-player counts; `turns` per player is unchanged.
- **Rationale**: the user confirmed a turn always means both players' turns, everywhere; this overrides the spec's importer non-goal for the report only (log parsing is untouched).
- **Rejected**: leaving the sum (10 for a 5-each game), which shows a per-player count.
### Chosen direction: combined turn at the output/input edges (architecture)
- **Decision**: keep the per-player counter in all stored and rule data; convert to the combined turn only where numbers go out (board view, log lines, branches, text, prompts, import report) and come in (effect expiry), through helpers in internal/state. Output fields keep their names (`turn`) but carry the combined value; `turn_seat` / `until_seat` added.
- **Rationale**: spec constraint 1 and the rules depend on the per-player count; reusing field names leaves no raw count visible to the UI or assistant.
- **Rejected**: parallel `round` fields (raw count stays visible); renumbering internally (breaks rules, violates constraint).

### until_seat is required with until_turn (architecture)
- **Decision**: add_effect rejects `until_turn` without `until_seat`.
- **Rationale**: a combined turn has two halves; guessing one could expire an effect a half-turn early or late with no sign.
- **Rejected**: defaulting to the affected seat or the end of the combined turn; both are wrong for common cases.

### Branch-name pattern uses seat ids (architecture)
- **Decision**: `turn-<N>-<seat>` for timeline rewinds, `review-t<N>-<seat>`, `bym-t<N>-<seat>`, `drill-<game>-t<N>-<seat>` in skills (seat = p1/p2).
- **Rationale**: seat ids are stable and short; player names can contain spaces and change between games.
- **Rejected**: player names in branch names.

### Conventions selected (architecture)
- **Decision**: only the skills-text-and-packaging convention applies; it is the only convention in the knowledge base.
- **Rationale**: the plan edits tcg skills.
- **Rejected**: none.

### Prompt and timeline wording (tasks)
- **Decision**: end-turn prompt "play turn N (<seat>'s turn)"; timeline sub-heading "<name>'s turn"; log text "T<N> <seat>".
- **Rationale**: matches the header wording approved in conversation ("Turn 3 · Lukiees's turn").
- **Rejected**: "3a/3b" half-turn labels, which players don't use.

## Rehydration cues

- `spektacular spec file read 20261019170829-bfb31628-turn-numbering-by-round`
- `grep -rn 'Turn\b\|turn %d\|until_turn' internal --include='*.go'` and `grep -rn 'turn' web/src/components` to re-find every surface.
- Re-read `internal/game/reads.go:95-135` (describe + LogLine) and `web/src/components/Timeline.svelte` before editing.
