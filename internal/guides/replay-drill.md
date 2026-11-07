---
name: replay-drill
description: Use when the player asks for a drill, a puzzle, practice, or to replay their losses or mistakes from reviewed games.
---

# Replay drill

Past mistakes make the best puzzles: the position is real, the player has been there, and the answer is known. The drill hides the answer, puts the board at the decision, and lets the player find the play.

## 1. Find a puzzle

- Use the game open on the playmat. If no game is open, ask the player to import one (a battle log, or a game record they saved) and stop.
- `get_log` (view `reveal`) and look for annotations beginning `Finding (`. Those come from the `review-game` guide; they name a category, the event, the better line and a principle. Findings last only for the session: a game just restored from its record has none until it is reviewed again.
- Prefer findings with `decision: poor` (real mistakes) over `dealt`. If the player names a category to practise ("sequencing"), pick findings with it. Avoid repeating a puzzle from the same session.
- If the open game has no findings yet, say so and offer to review it first with the `review-game` guide.

## 2. Set it up without the answer

- `rewind` to the start of the turn containing the finding (after that turn's draw), on a branch named `drill-<game>-t<N>-<seat>` (the turn and the player whose turn it is; add `-2`, `-3` for repeats). Turns count both players, as on the timeline: name one as "turn 3, your turn". The timeline then stops before the finding, so the annotation is not visible.
- Do not quote the finding, its principle, or the original play.
- Describe the position the way the player would have seen it: both boards, their hand, prizes each side, the opponent's visible threats. Use view `coach`, never `reveal`: the player must not see hidden cards.
- Ask: "It's your turn. What's your play? Play it on the board, or tell me." Then, in the same reply, call `wait_for_turn` with their seat straight away. Do not end your reply to wait for an answer first.
- Do what each result's `next` says, acting on any `decide` outcome (a promotion or a card the playmat could not apply) and handing the turn back in the same reply, and keep calling `wait_for_turn` until it reports the turn ended; then go straight to Compare. A move on the board that does not end the turn is not a cue to post: keep waiting. Never ask them to say they are done or to end their turn.
- The playmat carries out your side of shuffle-hand-and-draw cards the player uses (Judge, Iono and the like) by itself and lists them in `applied`: do not do them again.
- If they would rather answer in words, the wait's check-in ("Still playing?") hands the chat back to them; take their play from their reply and compare it the same way.

## 3. Compare

- Read their line: from what they say, or from the log in the `wait_for_turn` result (or `get_log` since the rewind point).
- Check it on its merits first: is it legal, do the numbers reach, what does it leave exposed? Use `card_text` for every number.
- Then compare with the finding's better line. Three outcomes:
  - **Found it:** same idea (it need not be move for move). Say so, and give the principle.
  - **Different but sound:** argue it honestly. If their line is as good or better, say so, and offer to update the finding with `annotate`.
  - **Missed it:** show the better line on a fresh branch so they can press Play and watch it, then give the principle.
- If the answer depends on hidden cards, test it with `sample_hidden` on a couple of branches rather than asserting.

## 4. Keep score

At the end of a session, give a short tally: puzzles tried, found, missed, and the categories missed most. That category is what to drill next. Leave the drill branches in place; they are cheap and the player may want to revisit them.
