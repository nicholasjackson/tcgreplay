---
created_date: "2026-10-19"
document_status: draft
project: tcgreplay
spec: 20261019170829-bfb31628-turn-numbering-by-round
plan: 20261019170829-bfb31628-turn-numbering-by-round
---

# Turns are numbered the way players count them

The playmat now numbers turns the way Pokémon players count them: "Turn 3" is the first player's third turn and the second player's third turn together, always shown with whose turn it is. The header, timeline, playback bar, effect chips, game log, board state, error messages and the coaching skills all use this numbering. The timeline gives each turn one section with a sub-section per player, and you can rewind to or play from either player's half. Game rules, warnings and saved games behave exactly as before.

> Derived from project tcgreplay (spektacular), spec/plan 20261019170829-bfb31628-turn-numbering-by-round. See the project-level record for the full feature.

## What changed in this repo

- `internal/state/turns.go` (new): `Round`, `(*State).TurnSeat` and `(*State).TurnOf` handle every conversion between the stored per-player turn and the combined turn. `internal/state/view.go`: the board view reports the combined `turn`, and effects are projected as `EffectView` with a combined `until_turn` and `until_seat`.
- `internal/game/reads.go`, `ui.go`, `verbs.go`, `game.go`: log lines are built by one constructor with combined `turn` and `turn_seat`. The text form is `#<id> T<N> <seat>`, and turn and effect descriptions use combined turns. The start-turn label is `Turn <N>`. Branch info carries `turn_seat`. `AddEffect` takes a combined turn plus a seat and refuses a turn without one. Wrong-seat errors and the draws-like-main warning name the combined turn.
- `internal/mcpserver/server.go`: `add_effect` gains `until_seat`, and the tool description and server instructions explain combined turns. `internal/webui/actions.go`: the web `add_effect` takes `until_seat`, and the end-turn prompt says `play turn N (<seat>'s turn)`.
- `internal/importer/importer.go`: `total_turns` counts combined turns. `testdata/index.json` and `testdata/README.md` were updated to match.
- `web/src/components/Timeline.svelte`, `PlaybackBar.svelte`, `Strip.svelte`: the timeline is grouped by turn and then by player, with per-half rewind (`turn-<N>-<seat>`) and play. The playback bar and effect chips show the combined turn with the player's name.
- `skills/tcg-review-game`, `tcg-before-you-move`, `tcg-replay-drill`: combined-turn wording and branch names that include the seat. `skills/skills_test.go` guards against the old forms.
- Tests were added or extended in `internal/state`, `internal/game`, `internal/importer`, `internal/mcpserver`, `internal/webui` and `skills`.

## Why

Before this change, each player turn got its own number, so the player and the assistant had to translate between two counts in every review, drill and log. All of the playmat's surfaces and tools live in this repo, so the switch to a single shared count happens here.

## Deviations

The playmat UI changes are checked by hand (there is no Svelte test harness), following the feature's test plan. The combined-turn heading on the timeline has only a play button; rewind is on each player's sub-heading.
