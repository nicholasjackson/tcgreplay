# Working context — spec 20261019170829-bfb31628-turn-numbering-by-round

## Problem
User: "for turns you get a turn per player but in Pokemon it would be a turn where both players take actions."
The playmat numbers each player's turn separately (Turn 1 = first player, Turn 2 = second player, ...). The user wants "Turn N" to mean both players' Nth turns.

## Decisions (user approved: "ok great this looks good")
- Keep the internal per-player counter `state.Turn` unchanged; rules depend on it (first-turn evolve/supporter rules in internal/game/verbs.go:218-221,406; effect expiry UntilTurn in internal/state/apply.go:426; EnteredTurn; draws_like_main; saved game records).
- Derive combined turn = (t+1)/2 plus whose half (the seat). Turn 1 = first player's turn + second player's first turn.
- Labels: "Turn 3 · Your turn" / "Turn 3 · Lukiees's turn". Timeline groups both halves under one "Turn 3" heading with a sub-heading per player.
- MCP surface also switches to combined numbering (get_state, log lines, until_turn on add_effect, end-turn prompt, errors). until_turn takes combined turn + which seat's turn it ends on; server converts.
- User asked to drive the spec to completion without per-step check-ins; they review everything at the end.
- No epic: the only epic (hosted-playmat) is unrelated.

## Learnings
- TCG Live battle logs carry no turn numbers ("X's Turn"), so the importer is unaffected by numbering.
- Branch-name conventions in skills use t<N> (review-t4, bym-t<N>, drill-<game>-t<N>) and the Timeline uses `turn-<N>`; with combined numbering two halves share N, so names must also distinguish the half.
- tcg-review-game SKILL.md:14 "Name turns both ways" rule becomes obsolete.
- state.First records who went first.
- All sections drafted from the approved proposal without per-step confirmation (user's instruction); non-goals and constraints are drafts awaiting the user's end review.
- No design document: one-line steers in Technical Approach suffice (design list was empty).
- Spec stored; reviewer findings 1-10 applied (opening-turn rules are warnings, not blocks). Split check: no offer (one atomic change across surfaces).

## Plan workflow (started 2026-10-19)
- User: "plan it" after reviewing the spec. Drafting steps run without section check-ins; walkthrough sign-off needed at the end.
- User (plan discovery): turns must equal the TCG Live definition, 1 turn = both players, everywhere. So the import report's total_turns becomes the combined count (max of the per-player counts); per-player `turns` map stays. Don't ask further questions of this kind: apply the rule.
- Persistence: only game records (battle log + seed + decks) are persisted; everything (labels, events) is rebuilt on load, so no migration.
- Plan committed (plan/context/research); in walkthrough awaiting sign-off.
- User signed off the plan at walkthrough (2026-10-19).
