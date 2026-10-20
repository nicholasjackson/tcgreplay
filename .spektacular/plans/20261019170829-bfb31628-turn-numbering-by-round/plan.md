---
created_date: "2026-10-19"
document_status: final
closed_date: "2026-10-19"
---

# Plan: 20261019170829-bfb31628-turn-numbering-by-round

<!-- Metadata -->
<!-- Created: 2026-10-19T18:28:12Z -->
<!-- Commit: 82a0ab3 -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

Change every turn number the playmat shows or accepts so that "Turn N" means both players' Nth turns, shown with whose turn it is, matching how Pokémon players count. Today each player's turn gets its own number, so the player and the assistant translate between two counts in every review, drill and log. The playmat UI, the assistant's tools and the coaching skills all switch; game rules and saved games behave exactly as before.

## Conventions

- **Skill text and packaging: never write "Claude" or "Nic " in a `skills/tcg-*` skill, and never exclude them from a build context** — this plan edits three tcg skills; `skills/skills_test.go` fails on either word, and the binary embeds the skills.

## Architecture & Design Decisions

The per-player counter stays exactly as it is in stored and rule-bearing data: `State.Turn`, `Event.Turn`, `Effect.UntilTurn`, `Stack.EnteredTurn` and `TurnPayload.Number` keep counting one turn per player, so every rule check (`internal/game/verbs.go:218-223,406`, `internal/game/reads.go:394`, `internal/state/apply.go:425-427`) and the draws-like-main matcher (`internal/game/game.go:247-279`) are untouched. The combined turn exists only at the edges: where the server hands numbers out (board view, log lines, branch list, log text, messages, prompts, import report) and where it takes one in (effect expiry). One small set of helpers in `internal/state` owns the conversion: `Round(turn)` gives `(turn+1)/2`, `(*State).TurnSeat(turn)` gives the seat that played a per-player turn (`First` for odd turns, the other seat for even, empty for setup), and `(*State).TurnOf(round, seat)` gives the per-player turn back for a combined turn and seat. Nothing else does the arithmetic, so the playmat, the log and the tools cannot disagree.

Output types carry the combined number under the field names they already use, rather than gaining a parallel field. `BoardView.turn`, `LogLine.turn` and `BranchInfo.turn` become the combined turn. `LogLine` and `BranchInfo` gain `turn_seat`, the seat whose turn the event or branch head falls in; `BoardView` already says this with `current`. Effects in the board view are projected into a view type whose `until_turn` is combined and which adds `until_seat`. Reusing the field names keeps the web UI and the assistant on one field each, and means no raw per-player number is left in anything they read, which a parallel `round` field would not achieve (see `research.md#alternatives-considered-and-rejected`). Text follows the same rule: `turn 3 start (p2)`, `(until end of turn 4 (p2))`, `#12 T3 p2 …`, the start-turn action label `Turn 3`, wrong-seat errors, the draws-like-main warning and the end-turn prompt all print the combined number with a seat. The `turn N start` shape is kept so the timeline's start-of-turn detection keeps working.

Input is converted once, at `Game.AddEffect`: the MCP `add_effect` tool and the web `add_effect` action take `until_turn` as a combined turn plus `until_seat`, and the game converts them with `TurnOf` before storing the effect. `until_seat` is required whenever `until_turn` is set, so an expiry can never land on the wrong half silently. Saved games need no migration: a record is the battle log text, seed and decks (`internal/game/record.go:33-50`), and the whole event tree, labels included, is rebuilt from it on load, so old records come back with the new numbering for free. The import report's `total_turns` becomes the number of combined turns (the higher of the two per-player counts); the per-player `turns` map is unchanged.

The web UI follows the data: the Strip header and PlaybackBar print the combined turn with the player's name, effect chips print `until end of T4 · <name>`, and the Timeline groups lines by `turn` into one section per combined turn with a sub-heading per `turn_seat`, each sub-heading carrying its own rewind (to branch `turn-<N>-<seat>`) and play buttons. The tcg-review-game, tcg-before-you-move and tcg-replay-drill skills drop the "name turns both ways" translation, speak "turn N, <player>'s turn", and name branches `review-t<N>-<seat>`, `bym-t<N>-<seat>` and `drill-<game>-t<N>-<seat>`, keeping to the skills-text convention (no "Claude" or "Nic ").

## Component Breakdown

- **Turn conversion helpers (state package, new)** — own the only arithmetic between per-player turns and combined turns: combined number from a per-player turn, the seat that played a per-player turn, and the per-player turn for a combined turn and seat. Every other component calls these; none computes parity itself.
- **Board view projection (state package, changed)** — reports the combined turn in the board view and projects lingering effects with a combined expiry turn and the seat it ends on. Stored state is unchanged.
- **Log rendering (game package, changed)** — builds log lines for the web snapshot and for get_log with the combined turn and the turn's seat, prints `T<N> <seat>` in text lines, and describes turn start/end and effect expiry in combined terms. Write results' deltas come from the same descriptions, so they follow automatically.
- **Branch listing (game package, changed)** — reports each branch head's combined turn and seat, for both the snapshot and list_branches.
- **Game verbs and messages (game package, changed)** — the start-turn action label, wrong-seat errors and the draws-like-main warning name the combined turn; adding an effect converts a combined expiry turn plus seat into the stored per-player turn, and rejects an expiry turn without a seat.
- **MCP tool surface (mcpserver, changed)** — add_effect takes `until_turn` plus `until_seat` and its description and the server instructions explain combined numbering; get_state, get_log and list_branches need no code change beyond the game package.
- **Web action surface (webui, changed)** — the web add_effect action takes the same combined expiry, and the end-turn prompt copied for the assistant names the assistant's turn by combined number.
- **Import report (importer, changed)** — total turns counts combined turns; per-player counts are unchanged.
- **Playmat UI (web, changed)** — Strip header, effect chips and PlaybackBar show the combined turn with the player's name; the Timeline shows one section per combined turn with a sub-section per player, each with its own rewind and play.
- **Coaching skills (skills, changed)** — tcg-review-game, tcg-before-you-move and tcg-replay-drill name turns by combined number and player and use seat-qualified branch names.

## Data Structures & Interfaces

No stored type changes: `State`, `Event`, `Effect`, `TurnPayload` and game records keep their shape and per-player meaning. The changes are new helpers and changed output/input contracts.

**Turn conversion helpers (new, state package)** — the single conversion point.

```go
func Round(turn int) int                     // (turn+1)/2; 0 for setup
func (s *State) TurnSeat(turn int) Seat      // seat that played per-player turn; "" for 0 or before First is known
func (s *State) TurnOf(round int, seat Seat) (int, error) // inverse; error before the game has started or for a bad seat/round
```

**Board view (changed output)** — `turn` now holds the combined turn; `current` already names whose turn it is. Effects are projected:

```go
type EffectView struct {
    Effect                    // id, text, source, seat; UntilTurn overwritten with the combined turn
    UntilSeat Seat `json:"until_seat,omitempty"`
}
// BoardView.Effects []EffectView
```

**Log line (changed output)** — `turn` holds the combined turn; new `turn_seat` names the seat whose turn the event falls in (empty during setup). Text form: `#<id> T<turn> <turn_seat> [<action> <label>] <text>`, `#<id> T0` during setup.

**Branch info (changed output)** — `turn` holds the head's combined turn; new `turn_seat`.

**add_effect input (changed, MCP tool and web action)** — `until_turn` is a combined turn; new `until_seat` (seat id) is required when `until_turn` is set. `Game.AddEffect` takes the combined turn and seat and stores the converted per-player turn.

**Import report (changed output)** — `total_turns` is the number of combined turns (max of the per-player counts in `turns`).

## Implementation Detail

- **One new pattern: "per-player inside, combined at the edge".** Code that enforces or records rules keeps reading the per-player counter. Code that renders a number for a person or the assistant, or accepts one from them, goes through the conversion helpers. A reader can tell which side of the line a function is on by whether it calls `Round`/`TurnSeat`/`TurnOf`; a comment on the helpers states the rule so new code follows it.
- **Log line construction is shared.** The web snapshot and get_log build log lines with the same few lines today; both gain the same `turn`/`turn_seat` computation. Fold that into one small constructor so the two paths cannot drift.
- **Describing text needs the post-event state**, which the describer already receives, so turn and effect text can resolve seats without new plumbing.
- **The UI keeps deriving from server data.** The Timeline's grouping changes from "by turn" to "by turn, then by turn seat"; the existing start-of-turn finder, rewind and play logic is reused per player sub-section rather than rewritten. No parity logic moves into the browser.
- **Existing patterns followed:** verbs keep emitting the same events; MCP and web argument structs gain one field each in the existing style; skills keep their structure and only change wording and branch-name templates.

## Dependencies

- **Spec `20261019170829-bfb31628-turn-numbering-by-round`** — the requirements this plan implements; nothing else must land first.
- **`internal/state`** — gains the conversion helpers and the effect view; stored types unchanged.
- **`internal/game`, `internal/mcpserver`, `internal/webui`, `internal/importer`** — consumers changed in place; no new packages.
- **Web UI (Svelte, Vite build already in the repo)** — components changed; no new libraries. The built bundle is embedded at build time, so the UI must be rebuilt for the binary to show the change.
- **`skills/tcg-*` and `skills/skills_test.go`** — skill text changes must keep passing the existing skill tests.
- **`testdata/index.json`** — expected `total_turns` values for the bundled logs change to combined counts.
- No external libraries or services added.
- Design documents this plan was built on: none.

## Testing Approach

- **Unit tests for the conversion helpers** (state package, Go `testing`, table-driven like the existing state tests): combined number for setup and turns 1–6, seat for odd/even turns with either seat going first, the inverse for both seats, and errors before the game has started. These carry the whole feature, so they get the most cases.
- **Game-level tests** (game package, following the existing game tests that start a game and play turns): after both players' first turns and into the first player's second turn, the board view reports turn 1/1/2 with the right `current`; log lines carry combined `turn` and `turn_seat`, text lines read `T<N> <seat>` and `turn N start (<seat>)`; branch info reports combined turn and seat; wrong-seat errors name the combined turn; an effect added until the second player's half of turn 2 survives the first player's turn 2 and is gone after the second player ends turn 2, and its description names turn 2 and the seat; `until_turn` without `until_seat` is rejected.
- **Regression for rules:** the existing tests that assert per-player `Turn` values and the opening-turn warnings stay unchanged and must pass, proving the internal counter and rule behaviour did not move. Add one assertion that evolving during the second player's turn 1 still warns and that the same in turn 2 does not.
- **Saved records:** the existing record round-trip tests must pass unchanged; add an assertion that a restored record's log reports combined turns (no line above half the per-player total, rounded up).
- **Importer:** the bundled-log oracle test checks `total_turns` against updated expected values (combined counts).
- **Surfaces:** the web action test for the end-turn prompt is updated to expect the combined turn; an MCP test covers `add_effect` with `until_turn`+`until_seat`.
- **Skills:** the existing skill tests keep the text conventions; add a check that no tcg skill still contains the "name turns both ways" translation or a `-t<N>` branch template without a seat.
- **Deliberate gap:** the Svelte UI has no test harness in this repo, so UI changes are checked by hand rather than by adding one.

Success metrics:
- *The assistant names turns by combined number without translation, and the player doesn't correct a turn reference* — **Manual — captured in the implementation test plan** (run a short review with the updated skills).
- *No surface a player or the assistant reads shows a per-player turn count, apart from pre-change notes and branch names* — **Behavioural test** for server surfaces (board view, log lines and text, branches, errors, prompt, import report, skills text, covered above) plus **Manual — captured in the implementation test plan** for the playmat UI (header, timeline, playback bar, effect chips).

Manual reviews:
- **Manual — captured in the implementation test plan** — check the playmat by hand: header, timeline sections and per-player sub-sections, rewind/play per half, playback bar and effect chips.

## Milestones & Tasks

### Milestone 1: The assistant reads and writes turns the way players count them

**What changes**: Everything the assistant reads from the playmat names a turn by the combined number with whose turn it is: the board state, the game log and its turn start and end lines, the branch list, the changes each move reports, out-of-turn errors and the import summary. Recording a lingering effect takes "until the end of the second player's turn 4" rather than a per-player count. Game rules, warnings and saved games behave exactly as before.

**Validation point**: The Go test suite passes, including new tests for combined numbering on every server surface, the unchanged rule tests, and the import oracle with updated turn totals.

#### - [x] Task: Turn conversion helpers
**Id:** 6f3d44dd-611e-4205-aa68-79caa69c20c7
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Add the one place that converts between the playmat's per-player turn count and the combined turn players use: the combined number for a per-player turn, the seat that played it, and the per-player turn for a combined turn and seat. Every later task uses these, so nothing else does the arithmetic.

*Technical detail:* [context.md#task-turn-conversion-helpers](./context.md#task-turn-conversion-helpers)

**Acceptance criteria**:
- [x] Per-player turns 1 and 2 both convert to turn 1, and 3 and 4 to turn 2; setup stays turn 0.
- [x] The seat for a per-player turn is the first player on odd turns and the other player on even turns, whichever seat went first.
- [x] Converting a combined turn and seat back gives the original per-player turn, and asking before the game has started gives an error.

#### - [x] Task: Board view reports combined turns
**Id:** 93d7549a-a486-4972-97ef-ba128ab1ce60
**Repo:** tcgreplay
**Depends on:**
- 6f3d44dd-611e-4205-aa68-79caa69c20c7 — Turn conversion helpers
**Execution:** agent

Make the board view that the playmat and the assistant read report the combined turn, and show each lingering effect's expiry as a combined turn plus the player whose turn it ends on. The stored state keeps its per-player count.

*Technical detail:* [context.md#task-board-view-reports-combined-turns](./context.md#task-board-view-reports-combined-turns)

**Acceptance criteria**:
- [x] During the second player's first turn the board reports turn 1 with that player as current; during the first player's second turn it reports turn 2.
- [x] An effect stored to end on the second player's second turn shows as ending in turn 2 on that player's turn.

#### - [x] Task: Logs and branches use combined turns
**Id:** 66d1fad3-aaa2-4b6d-b58b-8724ba7b842f
**Repo:** tcgreplay
**Depends on:**
- 6f3d44dd-611e-4205-aa68-79caa69c20c7 — Turn conversion helpers
**Execution:** agent

Give every log line the combined turn and the seat whose turn it falls in, for both the playmat's timeline and the assistant's log, and print them that way in text. Turn start and end lines and effect descriptions use the combined turn, the start-turn action is labelled with it, and the branch list reports each branch's combined turn and seat.

*Technical detail:* [context.md#task-logs-and-branches-use-combined-turns](./context.md#task-logs-and-branches-use-combined-turns)

**Acceptance criteria**:
- [x] In a game of ten player turns, no log line or branch shows a turn above 5, and each line names the seat whose turn it is.
- [x] The turn start line for the second player's third turn reads as turn 3 for that seat, and its action is labelled "Turn 3".
- [x] A game restored from a saved record shows the same board at every step as before and labels its turns with the combined numbering.

#### - [x] Task: Effect expiry and messages use combined turns
**Id:** 21bf9d51-f959-4e26-a08f-f4aa51074f25
**Repo:** tcgreplay
**Depends on:**
- 6f3d44dd-611e-4205-aa68-79caa69c20c7 — Turn conversion helpers
**Execution:** agent

Let the assistant (and the web action) record an effect "until the end of <seat>'s turn N" in combined terms, converted to the stored per-player turn, and refuse an expiry turn with no seat. Out-of-turn errors and the warning about repeating the real game's draws name the combined turn, and the tool descriptions and server instructions explain the numbering.

*Technical detail:* [context.md#task-effect-expiry-and-messages-use-combined-turns](./context.md#task-effect-expiry-and-messages-use-combined-turns)

**Acceptance criteria**:
- [x] An effect recorded until the end of the second player's turn 4 is still on the board during the first player's turn 4 and is gone once the second player ends turn 4.
- [x] Recording an effect with an expiry turn but no player is refused with a message saying which player is needed.
- [x] Acting for the wrong player in play mode gives an error naming the combined turn and whose turn it is.
- [x] Opening-turn warnings (Supporter or attack in the first player's turn 1, evolving in either player's turn 1) appear exactly as before, and not in turn 2.

#### - [x] Task: Import report counts combined turns
**Id:** a2aaa61f-eea2-4d45-a254-b6bbb8a64688
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Make the import summary's total turns count combined turns (the larger of the two players' counts) instead of adding both players' turns together, and update the expected totals for the bundled test logs. The per-player counts stay as they are.

*Technical detail:* [context.md#task-import-report-counts-combined-turns](./context.md#task-import-report-counts-combined-turns)

**Acceptance criteria**:
- [x] Importing a game where each player had five turns reports five total turns, with five for each player.
- [x] Every bundled log's expected totals match the combined count.

### Milestone 2: The playmat shows one turn for both players

**What changes**: The playmat header reads "Turn 3 · Your turn" or "Turn 3 · <name>'s turn", the timeline shows one section per turn with a sub-section for each player that can be rewound to or played from on its own, the playback bar and effect chips show the combined turn with the player, and the prompt copied for the assistant at the end of the player's turn names the turn the same way.

**Validation point**: The web bundle builds, the end-turn prompt test passes with the combined turn, and a manual check of the playmat shows the new header, timeline grouping, per-player rewind and playback labels.

#### - [x] Task: Playmat shows combined turns
**Id:** 1115abc8-b529-47de-8470-7ae0eabd30fd
**Repo:** tcgreplay
**Depends on:**
- 93d7549a-a486-4972-97ef-ba128ab1ce60 — Board view reports combined turns
- 66d1fad3-aaa2-4b6d-b58b-8724ba7b842f — Logs and branches use combined turns
**Execution:** agent

Update the playmat so the header, playback bar and effect chips show the combined turn with the player's name, and the timeline shows one section per combined turn with a sub-section per player. Each sub-section has its own rewind (to a branch named for the turn and seat) and play button.

*Technical detail:* [context.md#task-playmat-shows-combined-turns](./context.md#task-playmat-shows-combined-turns)

**Acceptance criteria**:
- [ ] On the second player's third turn the header reads "Turn 3" with that player's name, or "Your turn" for the viewer.
- [ ] After five turns each, the timeline shows Setup then Turn 1 to Turn 5, each with one sub-section per player in playing order.
- [ ] Rewinding to either player's half of a turn puts the board at the start of that player's turn, on branches with different names; play from either half starts there.
- [ ] The playback bar and effect chips show the combined turn and the player.

#### - [x] Task: End-turn prompt names the combined turn
**Id:** fe1c9633-72b9-46bf-90ad-f2637ec98cb6
**Repo:** tcgreplay
**Depends on:**
- 6f3d44dd-611e-4205-aa68-79caa69c20c7 — Turn conversion helpers
**Execution:** agent

When the player ends their turn in play mode, the prompt copied for the assistant names the assistant's turn by combined number and seat instead of the per-player count.

*Technical detail:* [context.md#task-end-turn-prompt-names-the-combined-turn](./context.md#task-end-turn-prompt-names-the-combined-turn)

**Acceptance criteria**:
- [x] After the player ends the first turn of the game, the copied prompt asks the assistant to play turn 1, not turn 2.

### Milestone 3: Coaching uses the same turn names as the player

**What changes**: The review, drill and pre-move coaching name turns as "turn 3, your turn" with no translation between two counts, and the branches they create say which player's half of a turn they start from, so reviewing both halves of one turn never reuses a branch name.

**Validation point**: The skill tests pass, including a check that no coaching skill still carries the old translation rule or a branch template without the player's seat.

#### - [x] Task: Coaching skills use combined turns
**Id:** f44180b9-d7f3-4629-97de-6f67969695bd
**Repo:** tcgreplay
**Depends on:**
- 66d1fad3-aaa2-4b6d-b58b-8724ba7b842f — Logs and branches use combined turns
**Execution:** agent

Rewrite the turn wording in the review, pre-move and drill skills: drop the "name turns both ways" translation, name turns as "turn N, <player>'s turn", and make every branch template include the seat so the two halves of a turn get different branches. Extend the skill tests so the old forms cannot come back.

*Technical detail:* [context.md#task-coaching-skills-use-combined-turns](./context.md#task-coaching-skills-use-combined-turns)

**Acceptance criteria**:
- [x] No coaching skill tells the assistant to translate between per-player and combined turn numbers.
- [x] Every branch name the skills prescribe includes both the turn and the seat.
- [x] The skill tests pass and fail if the old translation rule or a seatless turn branch template is reintroduced.

## Open Questions

None. Every question raised during planning was answered from the code or by the user (total_turns counts combined turns).

## Out of Scope

- Notes, findings and annotations already recorded in a game keep their text; old per-player turn mentions in them are not rewritten (spec non-goal).
- Branches created before the change keep their names (spec non-goal).
- Parsing TCG Live battle logs is unchanged; only the import summary's total-turns figure changes (spec non-goal, narrowed by the user's decision that turns always count both players).
- Game rules and their warnings are unchanged.
- Adding a test harness for the Svelte UI; the UI is checked by hand.
- Skill copies installed outside this repository (for example in a claude.ai skills library) are not updated by this plan; they need re-uploading from `skills/` after release.

## Changelog

### 2026-10-19 — Task: Turn conversion helpers

**What was done**: Added `internal/state/turns.go` with `Round`, `(*State).TurnSeat` and `(*State).TurnOf`, the single place that converts between the stored per-player turn count and the combined turn players use, with a comment stating the "per-player inside, combined at the edge" rule. Table tests cover the conversion both ways for either seat going first, and the errors.

**Deviations**: `Round` also maps negative turns to 0 (defensive; setup is the only non-positive turn in practice).

**Files changed**:
- `internal/state/turns.go`
- `internal/state/turns_test.go`

**Discoveries**: `internal/game` tests use plain `testing` (no testify); `internal/state` had no tests before this task.

### 2026-10-19 — Task: Board view reports combined turns

**What was done**: `State.Project` now reports `BoardView.Turn` as the combined turn and projects lingering effects into a new `EffectView` whose `until_turn` is combined and whose `until_seat` names the player whose turn the effect ends on. Stored state keeps its per-player counts.

**Deviations**: None.

**Files changed**:
- `internal/state/view.go`
- `internal/game/game_test.go`

**Discoveries**: Tests call `AddEffect` with a per-player `untilTurn` (4); the "Effect expiry and messages" task changes that signature and must update `TestBoardViewShowsEffectEndAsCombinedTurn` too. A gofmt alignment gotcha: a comment inside an aligned struct field block splits the alignment group, so avoid inline field comments there.

### 2026-10-19 — Task: Logs and branches use combined turns

**What was done**: Log lines for both get_log and the web snapshot are now built by one `Game.newLogLine` constructor that sets `turn` to the combined turn and adds `turn_seat`; `LogLine.String` prints `#<id> T<N> <seat>`. Turn start/end text reads `turn N start (<seat>)`, effect text `(until end of turn N (<seat>))`, the start-turn action is labelled with the combined turn, and `BranchInfo` reports the head's combined turn plus `turn_seat` in both the branch list and the snapshot.

**Deviations**: The corpus record test asserts the restored log's highest turn against the max of the report's per-player `turns` rather than `total_turns`, because `total_turns` is still a sum until the import-report task lands.

**Files changed**:
- `internal/game/reads.go`
- `internal/game/ui.go`
- `internal/game/verbs.go`
- `internal/game/game_test.go`
- `internal/game/record_roundtrip_test.go`

**Discoveries**: `Event.Turn` is stamped after the event is applied (`internal/game/game.go:202`, `internal/importer/replay.go:156`), so a turn-start event already carries its new turn number and the post-apply state's `First` is set for turn 1 — the turn seat can be resolved from the post-event state without extra plumbing.

### 2026-10-19 — Task: Effect expiry and messages use combined turns

**What was done**: `Game.AddEffect` now takes a combined `untilTurn` plus `untilSeat`, refuses an expiry turn without a seat, and stores the per-player turn via `TurnOf`. The MCP `add_effect` tool and web `add_effect` action gained `until_seat`; the tool description and server instructions explain combined numbering. Wrong-seat errors and the draws-like-main warning name the combined turn (the warning also names whose turn it is).

**Deviations**: Existing test call sites of `AddEffect` were updated to the new signature with the same per-player expiry (combined 1/p2 for per-player 2, combined 2/p2 for per-player 4). The web `add_effect` action has no dedicated test (the plan did not ask for one; the web UI never sends `until_turn`).

**Files changed**:
- `internal/game/verbs.go`
- `internal/game/game.go`
- `internal/mcpserver/server.go`
- `internal/webui/actions.go`
- `internal/game/game_test.go`
- `internal/mcpserver/server_test.go`

**Discoveries**: There were no existing tests for the opening-turn warnings (Supporter/attack/evolve in turn 1); `TestOpeningTurnWarnings` now pins them.

### 2026-10-19 — Task: Import report counts combined turns

**What was done**: The import report's `total_turns` is now the higher of the two per-player counts (combined turns) instead of their sum; `testdata/index.json` expected totals were updated to match, and `testdata/README.md` says what `total_turns` means. The oracle test now also guards that each fixture's total equals the max of its per-player turns.

**Deviations**: None.

**Files changed**:
- `internal/importer/importer.go`
- `internal/importer/importer_test.go`
- `internal/game/record_roundtrip_test.go`
- `testdata/index.json`
- `testdata/README.md`

**Discoveries**: None.

### 2026-10-19 — Task: Playmat shows combined turns

**What was done**: The Timeline groups the log into one section per combined turn (Setup unchanged) with a sub-heading per player's half, labelled "Your turn" or "<name>'s turn"; each sub-heading rewinds to the start of that player's turn on branch `turn-<N>-<seat>` and has its own ▶ play. The turn heading keeps ▶ play from the start of the turn. The PlaybackBar shows `T<N> · <name>` (or Setup), effect chips show `until end of T<N> · <name>`, and the Strip header needed no change because `board.turn` is now combined. The bundle builds with `make web`.

**Deviations**: The combined-turn heading itself has no rewind (only ▶); rewind lives on each player's sub-heading, as the plan allowed. The four acceptance criteria are UI behaviour with no automated harness, so they are left unchecked here and carried into the manual test plan.

**Files changed**:
- `web/src/components/Timeline.svelte`
- `web/src/components/PlaybackBar.svelte`
- `web/src/components/Strip.svelte`

**Discoveries**: `internal/webui/dist` is git-ignored apart from a placeholder, so the built bundle is not committed; a release build must run `make web` (or `make build`) to embed the change. A fresh worktree needs `npm ci` in `web/` before `make web`.

### 2026-10-19 — Task: End-turn prompt names the combined turn

**What was done**: The prompt copied for the assistant when the player ends their turn in play mode now reads "play turn N (<seat>'s turn)" with the combined turn, so after the opening turn it asks for turn 1 (p2's turn) instead of turn 2.

**Deviations**: None.

**Files changed**:
- `internal/webui/actions.go`
- `internal/webui/webui_test.go`

**Discoveries**: None.

### 2026-10-19 — Task: Coaching skills use combined turns

**What was done**: tcg-review-game replaces "Name turns both ways" with "Name turns as players do" (turn N counts both players; always say whose turn it is), and its branch templates become `review-t<N>-<seat>` (example `review-t2-p2`). tcg-before-you-move and tcg-replay-drill use `bym-t<N>-<seat>` and `drill-<game>-t<N>-<seat>` and tell the assistant to name turns as "turn 3, your turn". A new skills test rejects the old translation wording and any backtick-quoted turn branch template without a seat, with a table test proving the guard catches the old forms.

**Deviations**: None.

**Files changed**:
- `skills/tcg-review-game/SKILL.md`
- `skills/tcg-before-you-move/SKILL.md`
- `skills/tcg-replay-drill/SKILL.md`
- `skills/skills_test.go`

**Discoveries**: Copies of these skills installed outside the repo (e.g. a claude.ai skills library) still carry the old wording until they are re-uploaded from `skills/`.
