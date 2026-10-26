---
name: review-game
description: Use when the player asks to review, go through, analyse or coach a game on the playmat, or accepts the offer of a review after importing.
---

# Review a game

A review is a conversation about decisions, not a verdict. The player learns most by explaining a play, seeing the alternative played out on the board, and leaving with one principle per mistake. Your job is to find the decision points, put the board there, ask, test lines honestly, and record what was learned.

## 0. Ground rules

- **Ask before judging.** At each decision point, set the scene and ask what the player was thinking. Give your view only after they answer, or when they ask for it.
- **Put the board where you are talking.** Before asking about a turn, `rewind` to the start of it on a branch named for the turn and the player whose turn it is (`review-t2-p2`), so the playmat and its timeline show that turn rather than the end of the game. Never just tell the player which timeline header to click.
- **Name turns as players do.** Turn N counts both players: it is the first player's Nth turn and the second player's Nth turn, the way the timeline and the log show it (`T2 p2`). Always say whose turn it is: "turn 2, your turn" or "turn 2, <opponent>'s turn".
- **Check the current branch before you move anything.** Branch switches are shared with the player's browser; `list_branches` first, and never make test moves on a branch the player is using.
- **Card text from `card_text`**, never memory. Quote the numbers you rely on: HP, attack costs, damage, Weakness, Resistance, Tool effects.
- **Hidden information is an assumption.** The opponent's unseen cards come from an assumed list: say "they had" only for cards the log proves, "they likely had" otherwise.

## 1. Find the decision points

Read the game with `get_log` (view `coach` shows what the player could see; `reveal` shows the assumed hidden cards too). Look for:

- Turns that ended without an attack when an attacker could have been made ready.
- Supporters played when another Supporter in hand would have done more (a shuffle-draw that threw away a gust or a search).
- Resources committed before information: Energy attached, or a Pokémon retreated or benched, before a draw Ability or search that could change the plan.
- Damaged or multi-prize Pokémon left where the opponent's gust could reach them.
- Prize-race swings: the turn where the prizes needed to win changed by two or more.
- Attacks short of a knockout by a small amount (a Tool, a boost, Weakness misapplied).

Pick the two to four that mattered most, in game order. Ignore turns with no real choice.

## 2. Walk each point

1. `rewind` to the start of the turn (after the turn's draw) on a `review-t<N>-<seat>` branch (`<seat>` is `p1` or `p2`, the player whose turn it is).
2. Set the scene in a few lines: both boards, the player's hand, prizes, what the opponent threatens next turn.
3. Ask what they were thinking, with one or two specific questions ("you had Boss's Orders and Lillie's in hand: what made you pick Lillie's?").
4. Wait for the answer. If the player wants to play the turn themselves on the board, let them: call `wait_for_turn` with their seat, answer any card that asks your side to act, and read their moves from the result when the turn ends.
5. Then give your view, plainly: was it a mistake, and why? Distinguish a **poor decision** from a **bad outcome**: a correct play that lost to variance is not an error.

## 3. Test the alternative

When you claim a better line, show it:

- Play it on the branch with the normal verbs, one move per action, so the player can press Play on the playmat and watch it at their own speed. Use `annotate` on a key event to explain the reasoning; notes show as captions during playback.
- Check the line is legal and the numbers reach: count damage in order (attacker bonuses, then Weakness doubles, then Resistance subtracts; Tools and Stadiums change HP).
- Test the opponent's best reply, not their worst. `sample_hidden` after rewinding to the same event deals plausible hidden cards; try two or three samples on separate branches before claiming a line is safe.
- If the alternative turns out no better, say so. A review that finds the player was right is still a good review.
- To play a line out with the player taking one seat, play the other seat's turns yourself and, after each `end_turn`, call `wait_for_turn` on the player's seat; reply when it reports their turn ended. Never ask them to say "done".

## 4. Record the finding

On the event where the mistake happened (on the main branch, so it shows whenever the game is opened), `annotate` with:

`Finding (<category>, decision: <good|poor>, outcome: <good|bad>): <what happened>. Better: <the line>. See branch <name>. Principle: <one sentence to remember>.`

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

## 5. Close

Summarise in a few lines: the findings with their categories, what went well (name real good plays), and whether the result was earned, gifted, dealt or played. If the same principle showed up more than once, say so: that is the thing to practise. Offer the `replay-drill` guide to practise the position later without the answer showing.
