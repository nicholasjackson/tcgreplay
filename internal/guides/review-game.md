---
name: review-game
description: Use when the player asks to review, go through, analyse or coach a game on the playmat, or accepts the offer of a review after importing.
---

# Review a game

A review has two parts. First a **turn-by-turn analysis**: every one of the player's turns, what they did and your verdict, with real mistakes kept apart from plays that simply did not work out. Then **practice**: the player chooses the turns they want to work on, and for each you put the board there, let them play it again, and test the scenarios on the board. The player learns most by replaying the position and seeing the alternatives played out, and leaves with one principle per mistake.

## 0. Ground rules

- **Rate decisions, not outcomes.** Distinguish a **poor decision** from a **bad outcome**: a correct play that lost to variance is not an error, and a lucky one is still a mistake.
- **Put the board where you are talking.** When you practise a turn, `rewind` to the start of it on a branch named for the turn and the player whose turn it is (`review-t2-p2`), so the playmat and its timeline show that turn rather than the end of the game. Never just tell the player which timeline header to click. During the analysis itself, leave the board alone: the findings you record on the main branch (step 3) put each verdict on the timeline.
- **Name turns as players do.** Turn N counts both players: it is the first player's Nth turn and the second player's Nth turn, the way the timeline and the log show it (`T2 p2`). Always say whose turn it is: "turn 2, your turn" or "turn 2, <opponent>'s turn".
- **Check the current branch before you move anything.** Branch switches are shared with the player's browser; `list_branches` first, and never make test moves on a branch the player is using. Never `checkout` another branch while the player is on the board, even to read it: the switch shows in their browser. Read another branch with `get_log` and its `branch` argument instead.
- **Card text from `card_text`**, never memory. Quote the numbers you rely on: HP, attack costs, damage, Weakness, Resistance, Tool effects. When an effect scales or targets, restate in your own words whose cards it counts or hits ("counts the cards in your opponent's hand") before you build advice on it.
- **The log shows actions, not knowledge or intent.** Before marking a `Mistake`, check what the player could have known: prizes they had seen, cards already revealed. If a play looks odd, ask why in one line rather than assume. Credit deliberate plays as deliberate. A play with a sound reason that was wrong in this spot is still a mistake, but its principle should address the reason ("check their Active's cheapest attack before hedging"), not the action.
- **Hidden information is an assumption.** The opponent's unseen cards come from an assumed list: say "they had" only for cards the log proves, "they likely had" otherwise.

## 1. Read the game

Read the game with `get_log` (view `coach` shows what the player could see; `reveal` shows the assumed hidden cards too). For each of the player's turns, look for the decision points:

- Turns that ended without an attack when an attacker could have been made ready.
- Supporters played when another Supporter in hand would have done more (a shuffle-draw that threw away a gust or a search).
- Resources committed before information: Energy attached, or a Pokémon retreated or benched, before a draw Ability or search that could change the plan.
- Damaged or multi-prize Pokémon left where the opponent's gust could reach them.
- Prize-race swings: the turn where the prizes needed to win changed by two or more.
- Attacks short of a knockout by a small amount (a Tool, a boost, Weakness misapplied).
- Plays that added to the board (evolving, benching, attaching to a Benched Pokémon). Ask what the opponent's gust does to it next turn: a Mega that cannot attack this turn is 3 prizes on a plate for Boss's Orders, and a 2-prize Benched Pokémon is the same at lower HP. When the opponent needs exactly the prizes that Pokémon gives up, the play loses.

### Check a line is playable

Before you call anything a mistake or suggest a line, prove it was playable that turn with only what the player had. The other guides use this same check.

- **Energy:** one attachment per turn. Count what is already on the attacker, plus that one.
- **Getting the attacker Active:** the retreat cost paid from attached Energy, at most one retreat per turn; or Switch, Air Balloon or a gust still in hand or reachable. Check effects that stop retreating.
- **Supporter:** one per turn, and none for the player going first on their first turn. If the line uses a Supporter, nothing else in it can be one.
- **Locks:** "can't use this attack next turn" unless the attacker went to the Bench and back; Item lock; protection on the target (no damage from Basic Pokémon, from ex, and the like).
- **Stadium:** one per turn.
- **The numbers:** HP from `card_text`, plus HP Tools; damage in order: attacker bonuses, then Weakness doubles, then Resistance subtracts.

If any of these fails, the line does not exist: say so instead of suggesting it. If you cannot show a concrete better line, the turn is at most worth examining.

## 2. Turn-by-turn analysis

In one reply, go through every one of the player's turns in game order. For each turn:

- **A heading** with the turn, whose it is and a few words of verdict: "Turn 3, your turn: good recovery".
- **What they did**, in a sentence or two: the plays that mattered, not every card.
- **Your assessment**: why it was right, or what was better and why, naming the alternative concretely (the card, the target, the order). Test the alternative against the opponent's best reply before you recommend it. Mention the opponent's turn only where it set up the player's choice.
- **A verdict**, one of: `Strong play`, `Good`, `Reasonable`, `Worth examining`, `Mistake`, `Dealt` (forced by the draw or the prizes, not a mistake).

Keep it tight: a turn with no real choice gets one line. Then close the analysis in a few lines: your overall assessment, whether the result was earned, gifted, dealt or played, what went well (name real good plays), and any principle that showed up more than once, since that is the thing to practise.

## 3. Record the findings

For each turn marked `Mistake` or `Worth examining`, and each `Strong play`, `annotate` the event where the decision happened, on the main branch so it shows whenever the game is opened:

`Finding (<category>, decision: <good|poor>, outcome: <good|bad>): <what happened>. Better: <the line>. Principle: <one sentence to remember>.`

Categories, one per finding:

| Category | Meaning |
|---|---|
| `perception` | Missed card text, status, HP, Tool or Ability on the board |
| `arithmetic` | Miscounted damage or prizes; a knockout missed by a countable amount |
| `sequencing` | Right cards, wrong order (draw before committing, gust before attack) |
| `fetch` | Searched for the wrong card |
| `resource` | Wasted or discarded a resource needed later |
| `prize-trade` | Gave the opponent an easy multi-prize knockout |
| `engine` | Setup priorities wrong (evolutions, draw engine, bench) |
| `dealt` | Variance: prized key cards, dead draws, forced plays. Not a mistake |
| `none` | Reviewed and found sound |

## 4. Offer practice

End the analysis reply with a numbered list of the turns worth practising, the ones marked `Mistake` or `Worth examining`, each with a few words on what to try:

1. Turn 4, your turn: the Ultra Ball target.
2. Turn 2, your turn: Energy on Riolu or Makuhita.

Add that the player can pick any other turn, or stop here, and ask which they would like to practise. If no turn qualifies, say so and offer to practise any turn they choose. Wait for their choice in chat; do not start practising until they pick.

## 5. Practise a turn

1. `rewind` to the start of the turn (after the turn's draw) on a `review-t<N>-<seat>` branch (`<seat>` is `p1` or `p2`, the player whose turn it is).
2. Set the scene in a few lines: both boards, the player's hand, prizes, what the opponent threatens next turn. Do not repeat your verdict yet.
3. Ask: "It's turn N, your turn. What's your play? Play it on the board, or tell me.", with the board link from the import result in that same message (the player may be on another device or have closed the tab). Then, in that same reply, call `wait_for_turn` with their seat straight away. Keep calling it, acting on any `decide` outcome (a promotion or a card the playmat could not apply) and handing the turn back in the same reply, until it reports the turn ended, then read their moves from the result. The playmat carries out your side of shuffle-hand-and-draw cards the player uses (Judge, Iono and the like) by itself and lists them in `applied`: do not do them again. A move that does not end the turn is not a cue to post. Never ask them to say they are done or to end their turn. If they answer in chat instead, take their play from there.
4. Check each move as it arrives against [Check a line is playable](#check-a-line-is-playable). If a move is illegal, say which rule it breaks in one line, `undo` it, and keep waiting: an illegal line on a practice branch makes the comparison meaningless. If the turn ended with prizes untaken, the `wait_for_turn` result says so and how to take them; tell the player which cards they got.
5. Compare their play with the line from your analysis. If theirs is as good or better, say so: a practice that finds the player was right is still a good practice.

### Test the alternative

Then play the scenarios out on the board:

- Play the better line on its own branch (`review-t<N>-<seat>-alt`) with the normal verbs, one move per action, so the player can press Play on the playmat and watch it at their own speed. Use `annotate` on a key event to explain the reasoning; notes show as captions during playback.
- Test the opponent's best reply, not their worst. `sample_hidden` after rewinding to the same event deals plausible hidden cards; try two or three samples on separate branches before claiming a line is safe.
- If the alternative turns out no better, say so, and change your verdict.
- Every line you play yourself passes the same playable check as the player's.
- To play a line out with the player taking one seat, play the other seat's turns yourself and, after each `end_turn`, call `wait_for_turn` on the player's seat; reply when it reports their turn ended. Never ask them to say "done".
- Playing the opponent's side: while the branch matches the real game, play the opponent's real moves. Once it diverges, play their best line from what they could see: take the biggest prize swing, gust 2-prize and damaged Benched Pokémon, and finish the game when it can be finished. Enforce their rules as strictly as the player's (Bench size when a Stadium that raised it leaves, one Supporter, attack costs). A soft opponent teaches the wrong lesson.
- If your attack knocks out the player's Active, `end_turn` still goes through with a warning, and the player promotes at the start of their turn. In review mode, ask them which Pokémon to promote; they can do it on the board.

Finish the turn with its one principle, and `annotate` the finding's event on the main branch again with what the practice showed and the branch names to open. Then offer the turns left on the list, or to stop.

## 6. Close

When the player stops, sum up in a line or two: the turns practised and the principle from each. Offer the `replay-drill` guide to practise these positions later without the answer showing.
