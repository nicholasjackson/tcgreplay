---
created_date: "2026-10-19"
document_status: final
closed_date: "2026-10-20"
---

# Turns are numbered the way players count them

## What was built

The playmat now counts turns the way Pokémon players do: "Turn N" means both the first player's Nth turn and the second player's Nth turn, and it is always shown with whose turn it is. Before, each player turn got its own number, so "Turn 4" was really the second player's 2nd turn.

- **One conversion point.** New helpers in `internal/state/turns.go` (`Round`, `TurnSeat`, `TurnOf`) are the only code that converts between the stored per-player count and the combined turn. Stored and rule-bearing data (`State.Turn`, `Event.Turn`, `Effect.UntilTurn`, `EnteredTurn`) still count one turn per player, so game rules, warnings and saved records behave exactly as before. Numbers are converted only where they go out to a person or the assistant, or come in from them.
- **Board state.** `get_state` and the web board report `turn` as the combined turn. Lingering effects carry a combined `until_turn` plus `until_seat`.
- **Log and branches.** Every log line (get_log and the playmat timeline) carries the combined `turn` plus `turn_seat`. Text lines read `#12 T3 p2 …`, turn lines read `turn 3 start (p2)`, and the start-turn action is labelled `Turn 3`. The branch list reports each head's combined turn and seat.
- **Effect expiry input.** `add_effect` (MCP tool and web action) takes `until_turn` as a combined turn plus a required `until_seat`, and refuses an expiry turn with no seat.
- **Messages.** Out-of-turn errors (`it is p2's turn (turn 1), not p1's`), the draws-like-main warning and the end-turn prompt copied for the assistant (`play turn 1 (p2's turn)`) name the combined turn. The MCP server instructions explain the numbering.
- **Import report.** `total_turns` is now the number of combined turns (the higher per-player count) instead of the sum of both players' turns. The bundled fixtures were updated to match.
- **Playmat UI.** The timeline shows one section per combined turn, with a sub-heading for each player's half. Each sub-heading has its own rewind (to branch `turn-<N>-<seat>`) and play. The playback bar shows `T<N> · <name>`, effect chips show `until end of T<N> · <name>`, and the header reads `Turn 3 · <name>'s turn`.
- **Coaching skills.** tcg-review-game, tcg-before-you-move and tcg-replay-drill drop the "name turns both ways" translation, name turns as "turn N, your turn", and use branch names that include the seat (`review-t<N>-<seat>`, `bym-t<N>-<seat>`, `drill-<game>-t<N>-<seat>`). A skills test stops the old forms from coming back.

## Why it matters

Players, the playmat and the assistant now share one way to name a turn, so reviews, drills and logs no longer translate between two counts ("turn 4 (your 2nd turn)"). Because each combined turn is always shown with the player, the two halves of one turn can't be confused. Branches made from either half get different names.

## Deviations from the plan

- The playmat's four acceptance criteria (header, timeline grouping, per-half rewind and play, playback bar and effect chips) have no automated harness. They are left for the manual test plan (M1–M5). The bundle builds with `make web`.
- On the timeline, the combined-turn heading has only a play button; rewind is on each player's sub-heading.
- The draws-like-main warning also names whose turn it was.
- `Round` maps negative turns to 0 as a defensive edge case.
- No dedicated test was added for the web `add_effect` action; the MCP tool's test covers the same conversion, and the web UI never sends `until_turn`.

## Notes

- The built web bundle (`internal/webui/dist`) is git-ignored, so release builds must run `make web` / `make build` to pick up the UI change.
- Skill copies installed outside the repository need re-uploading from `skills/`.
- Notes, annotations and branches recorded before this change keep their old per-player wording.
