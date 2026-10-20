---
created_date: "2026-10-19"
document_status: final
closed_date: "2026-10-19"
---

# Context: 20261019170829-bfb31628-turn-numbering-by-round

## Current State Analysis

- The playmat counts one turn per player: `State.Turn` and `Event.Turn` go 1, 2, 3… alternating seats, with `State.First` recording who went first (`internal/state/state.go:73-76`, set on turn 1 start in `internal/state/apply.go` ~415).
- Rules read that counter directly: evolve warnings `internal/game/verbs.go:218-223`, first-player attack warning `:406`, first-player Supporter warning `internal/game/reads.go:394`, effect expiry `internal/state/apply.go:425-427`, `EnteredTurn` `apply.go:235`, draws-like-main `internal/game/game.go:247-279`.
- Every surface prints the raw counter: board view `internal/state/view.go:156`, log lines `internal/game/ui.go:81` and `internal/game/reads.go:165`, text `reads.go:101,111,131`, branch info `reads.go:425` / `ui.go:106`, action label `verbs.go:171`, errors `verbs.go:161`, `game.go:431`, prompt `internal/webui/actions.go:261`, UI `web/src/components/{Strip,Timeline,PlaybackBar}.svelte`, skills `skills/tcg-{review-game,before-you-move,replay-drill}/SKILL.md`.
- Only game records (battle log + seed + decks) are persisted (`internal/store/store.go:1-4`, `internal/game/record.go:33-50`); everything else is rebuilt on load.

## Per-Task Technical Notes

### Task: Turn conversion helpers

Requirement attribution: "One turn number covers both players", "A turn is always shown with whose turn it is" → repo `tcgreplay`.

- `internal/state/turns.go` (new) — `Round(turn int) int` = `(turn+1)/2` (0 stays 0); `(*State).TurnSeat(turn int) Seat`: `""` if `turn <= 0` or `s.First == ""`, `s.First` if odd, `s.First.Other()` if even; `(*State).TurnOf(round int, seat Seat) (int, error)`: error if `s.First == ""` ("the game has not started"), if `round < 1`, or seat not P1/P2; returns `2*round-1` when `seat == s.First`, else `2*round`. Doc comment states the rule: stored and rule data stay per-player, only numbers shown to or taken from people/assistant go through these.
- `internal/state/turns_test.go` (new) — table tests for all three, with `First` = P1 and = P2; there is no existing test file in `internal/state`, so follow the table style in `internal/game/*_test.go`.
- `First` is set when turn 1 starts (`internal/state/apply.go` turn "start" branch, ~line 415-417); turns alternate because `apply.go:410-411` requires `Number == Turn+1` and `internal/game/verbs.go:170-173` / `internal/importer/importer.go:304` start the next turn for the other seat.

Complexity: Low. Token estimate: ~8k. Agent strategy: single agent, sequential.

### Task: Board view reports combined turns

Requirement attribution: "Board state reports the combined turn", "Lingering effects expire by combined turn and player" (display side) → `tcgreplay`.

- `internal/state/view.go:60-70` — `BoardView.Effects` becomes `[]EffectView`; add `type EffectView struct { Effect; UntilSeat Seat \`json:"until_seat,omitempty"\` }`. Keep JSON field `turn` on `BoardView`.
- `internal/state/view.go:155-158` — set `Turn: Round(s.Turn)`; build `Effects` by copying each `Effect`, and when `UntilTurn > 0` set `UntilSeat = s.TurnSeat(e.UntilTurn)` and `UntilTurn = Round(e.UntilTurn)`. Do not mutate `s.Effects`.
- Check other readers of `BoardView.Turn`/`.Effects` in Go (`grep -rn 'BoardView\|\.Effects' internal`) and adjust; `State.Effects` itself is unchanged.
- Tests: `internal/game/game_test.go` — new test (or extend `TestFullGameRewindAndBranch` ~line 103-226) projecting the board during P2's first turn (turn 1, current p2) and P1's second turn (turn 2); effect projection check.

Complexity: Low. Token estimate: ~10k. Agent strategy: single agent.

### Task: Logs and branches use combined turns

Requirement attribution: "Game log uses the combined turn", "Timeline groups both players under one turn" (data), "Existing saved games show the combined numbering" → `tcgreplay`.

- `internal/game/reads.go:119-127` — `LogLine` gains `TurnSeat string \`json:"turn_seat,omitempty"\``; `Turn` holds the combined turn.
- `internal/game/reads.go:129-140` — `String()` prints `#%d T%d` then ` %s` with `TurnSeat` when non-empty (`#12 T3 p2 [...] text`).
- Add a small constructor, e.g. `newLogLine(e *state.Event, st *state.State, text string) LogLine`, setting `Turn: state.Round(e.Turn)`, `TurnSeat: string(st.TurnSeat(e.Turn))`, ID/Seat/Action; use it at `internal/game/ui.go:81` and `internal/game/reads.go:165` (both have the post-apply `st`).
- `internal/game/reads.go:99-102` — effect text `(until end of turn %d (%s))` with `state.Round(p.Effect.UntilTurn)` and `after.TurnSeat(p.Effect.UntilTurn)`.
- `internal/game/reads.go:110-111` — `turn %d %s (%s)` with `state.Round(p.Number)`; keep the `turn N start` prefix (the Timeline regex `/^turn \d+ start/` depends on it).
- `internal/game/verbs.go:171` — label `fmt.Sprintf("Turn %d", state.Round(g.st.Turn+1))`.
- `internal/game/reads.go:413-418` (`BranchInfo`) — add `TurnSeat string \`json:"turn_seat,omitempty"\``; at `reads.go:~425` and `ui.go:~106` set `bi.Turn = state.Round(e.Turn)`, `bi.TurnSeat` from the game's current state `First` (use `g.st.TurnSeat(e.Turn)`; `First` is the same on every branch once turn 1 has started; if the head is in setup it is empty).
- Tests: `internal/game/game_test.go` — play to P1's second turn and assert `get_log`-style lines (`g.Log(...)` / the reads.go function at ~line 150) have `Turn` 1,1,2 and `TurnSeat` p1,p2,p1, text contains `turn 1 start (p2)`, and the action label `Turn 2` appears for P1's second turn. `internal/game/record_roundtrip_test.go:188` (`TestRecordRestoresCorpus`) — add an assertion that the restored game's highest log `Turn` equals the import report's combined total.

Complexity: Medium. Token estimate: ~18k. Agent strategy: single agent (changes are in two files sharing a helper).

### Task: Effect expiry and messages use combined turns

Requirement attribution: "Lingering effects expire by combined turn and player", "Messages and prompts use the combined turn" (errors), "Game rules behave exactly as before" → `tcgreplay`.

- `internal/game/verbs.go:570-579` — `AddEffect(seat, actor, text, target, source, untilTurn int, untilSeat state.Seat)`: if `untilTurn > 0`, require `untilSeat` (error "until_turn needs until_seat: the seat whose turn the effect ends on"), convert with `t.st.TurnOf(untilTurn, untilSeat)` inside the tx and store the per-player value.
- `internal/mcpserver/server.go:372-378` — `AddEffect` args: `UntilTurn` jsonschema "expires at the end of this turn (turns count both players: turn 3 is each player's third turn); 0 means until removed", new `UntilSeat string \`json:"until_seat,omitempty" jsonschema:"seat whose turn the effect ends on (p1 or p2); required with until_turn"\``; `server.go:735-741` pass `state.Seat(in.UntilSeat)`; tool description example → "'Items locked' on p1 until the end of p1's turn 3".
- `internal/mcpserver/server.go:60-70` — add one instruction line: "Turns count both players: turn N is the first player's Nth turn and the second player's Nth turn; logs show `T<N> <seat>`."
- `internal/webui/actions.go:40-50,252` — add `UntilSeat string \`json:"until_seat,omitempty"\`` and pass it through.
- `internal/game/verbs.go:161` and `internal/game/game.go:431` — `"it is %s's turn (turn %d), not %s's"` with `state.Round(g.st.Turn)`.
- `internal/game/game.go:279` — warning uses `state.Round(turn)` and the seat.
- Tests: `internal/game/game_test.go:350-382` (`TestStadiumReplacesAndEffectsExpire`) — update the call from per-player `2` to `1, state.P2` (same expiry); add a case: effect until P2's turn 2 survives P1's turn 2 end, is gone after P2 ends turn 2, and missing seat is refused. Existing opening-turn warning test (`game_test.go:~215`) stays; add assertions that evolving in P2's first turn warns and in turn 2 (per-player 3/4) does not. `internal/mcpserver/server_test.go` (`testMCPRoundTrip` ~line 93) — call `add_effect` with `until_turn` + `until_seat`.

Complexity: Medium. Token estimate: ~15k. Agent strategy: single agent.

### Task: Import report counts combined turns

Requirement attribution: user decision during planning (turns always mean both players) → `tcgreplay`.

- `internal/importer/importer.go:334-336` — `TotalTurns = max` of `r.rep.Turns` values instead of the sum.
- `testdata/index.json` (`total_turns` at lines ~45, 95, 146, …) — set each to the max of that entry's per-player `turns` values; check every entry.
- `internal/importer/importer_test.go:83` — oracle unchanged; it now checks the new values.

Complexity: Low. Token estimate: ~5k. Agent strategy: single agent.

### Task: Playmat shows combined turns

Requirement attribution: "Playmat header shows the combined turn", "Timeline groups both players under one turn", "Timeline rewind and play work per player's half", "Playback position shows the combined turn" → `tcgreplay` (web).

- `web/src/components/Strip.svelte:39-42` — header already prints `Turn ${b.turn} · who`; `b.turn` is now combined, so no logic change beyond verifying.
- `web/src/components/Strip.svelte:93` — effect chip `until end of T{e.until_turn} · {nameOf(e.until_seat)}`.
- `web/src/components/PlaybackBar.svelte:30` — `T{current.board.turn} · <name of current.board.current>` (name from `board.sides[seat].name`, as Strip's `nameOf` at `Strip.svelte:38`).
- `web/src/components/Timeline.svelte:9-17` — group into `{turn, halves: [{seat, lines}]}`: new turn when `l.turn` changes, new half when `l.turn_seat` changes within it.
- `web/src/components/Timeline.svelte:21-35` — apply `startOf` per half (lines of that half).
- `web/src/components/Timeline.svelte:72-108` — one `<section>` per turn with heading `Setup`/`Turn N` (no rewind on the turn heading, or rewind to the first half), and a sub-heading per half `<name>'s turn` with ↺ (`jump(startOf(h), \`turn-${t.turn}-${h.seat}\`)`) and ▶ (`playFrom(h.lines[0].id)`); keep sticky-heading behaviour and styles; give the sub-heading a style alongside `h3 .turn`.
- Rebuild with `make web` (Vite) so `internal/webui/dist` embeds the change.

Complexity: Medium. Token estimate: ~15k. Agent strategy: single agent.

### Task: End-turn prompt names the combined turn

Requirement attribution: "Messages and prompts use the combined turn" (prompt) → `tcgreplay`.

- `internal/webui/actions.go:258-262` — prompt `"... Call get_log since %d and play turn %d (%s's turn)."` with `state.Round(st.Turn)` and `st.Current`.
- `internal/webui/webui_test.go:301` — expect `"turn 1"` instead of `"turn 2"` (after P1 ends the first turn, the assistant plays P2's turn 1).

Complexity: Low. Token estimate: ~4k. Agent strategy: single agent.

### Task: Coaching skills use combined turns

Requirement attribution: "Coaching skills speak the combined numbering" → `tcgreplay` (skills).

- `skills/tcg-review-game/SKILL.md:13` — branch example `review-t4` → `review-t2-p2`; `:14` replace "Name turns both ways" with "Name turns as players do: turn N counts both players, so say 'turn 2, your turn' or 'turn 2, <opponent>'s turn'. Log lines show `T<N> <seat>`."; `:34` `review-tN` → `review-t<N>-<seat>`.
- `skills/tcg-before-you-move/SKILL.md:13` — `bym-t<N>` → `bym-t<N>-<seat>`.
- `skills/tcg-replay-drill/SKILL.md:19` — `drill-<game>-t<N>` → `drill-<game>-t<N>-<seat>`.
- `skills/skills_test.go` — new test over every tcg skill: fails if text contains "both ways" or matches a branch template `-t<N>\`` / `-tN\`` / `-t\d+\`` without a following `-<seat>`/`-p[12]`.
- Convention: no "Claude" or "Nic " in tcg skills (`skills/skills_test.go` enforces).

Complexity: Low. Token estimate: ~6k. Agent strategy: single agent.

## Testing Strategy

- Turn conversion helpers: new table tests in `internal/state/turns_test.go`.
- Board view reports combined turns: game-level test projecting the board in P2's turn 1 and P1's turn 2; effect projection.
- Logs and branches use combined turns: game-level test on log lines (`turn`, `turn_seat`, text `T<N> <seat>`, `turn N start (<seat>)`, action label), branch info; corpus record test asserts combined maximum turn.
- Effect expiry and messages use combined turns: updated `TestStadiumReplacesAndEffectsExpire` call shape (same expiry), new expiry-by-half and missing-seat cases, opening-turn warning assertions, wrong-seat error text; MCP round-trip `add_effect` with `until_seat`.
- Import report counts combined turns: importer oracle with updated `testdata/index.json`.
- Playmat shows combined turns: no automated tests (no Svelte harness); manual check in the implementation test plan; `make web` must build.
- End-turn prompt names the combined turn: `internal/webui/webui_test.go:301` expects `turn 1`.
- Coaching skills use combined turns: new skills test rejecting the old translation rule and seatless branch templates; existing skill tests pass.
- Success metrics: server-surface "no per-player count" is covered by the tests above; UI surfaces and the "no translation in a review" metric are manual items for the implementation test plan.

## Project References

- Spec: `20261019170829-bfb31628-turn-numbering-by-round` (`spektacular spec file read 20261019170829-bfb31628-turn-numbering-by-round`).
- Design documents: none.
- Knowledge: `conventions/skills-text-and-packaging.md` (repo tier, `tcgreplay`).
- Repo root: `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (only registered repo).

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

## Migration Notes

None: stored data keeps its per-player meaning, and saved records are rebuilt from battle-log text with no turn numbers in them. In-memory games running across an upgrade are lost on restart anyway.

## Performance Considerations

None: the conversions are constant-time arithmetic on values already at hand.
