---
name: tcg-before-you-move
description: Coach the pre-move routine on a real tcgreplay board position. Stops at a position and asks the player three questions in order (their turn, my kill, can I act), checking each answer against the actual HP, Tools, Weakness, Resistance and Energy on the board. Use when the player asks to practise their pre-move checklist, turn routine, or "what should I check before I move", or to quiz a position.
---

# Before you move

Most lost games contain a turn where the player acted before answering three questions. This skill practises asking them, in order, on a real board, until it is a habit. It checks answers; it does not hand them out.

## Choose a position

- Use the game open on the playmat: its current head, or ask which turn. Turns where the player was about to act with several options are best; turns with a recorded `Finding (` are good choices. If no game is open, ask the player to import one (a battle log, or a game record they saved) and stop.
- `rewind` to the start of that turn on a branch named `bym-t<N>-<seat>` (the turn and the player whose turn it is, e.g. `bym-t3-p1`). Name the turn as the timeline does, counting both players: "turn 3, your turn". Describe the position from the player's side (view `coach`): both boards with HP and damage, Tools, Energy, the player's hand, prizes each side, the Stadium.

## The three questions

Ask one at a time. Let the player answer before moving on, then check the answer against the board and `card_text`.

1. **Their turn: what dies, and for how many prizes?**
   Name the opponent's best attack next turn, apply the player's Weakness, and decide which of the player's Pokémon it knocks out and how many prizes that gives. Gust effects make every benched Pokémon reachable; a damaged multi-prize Pokémon on the Bench is one gust from a big prize turn.
2. **My kill: name the target, does the number reach?**
   Target HP: printed HP, plus HP Tools, minus Stadium or other HP effects. Damage: base, plus the attacker's boosts, then Weakness doubles, then Resistance subtracts. If it does not reach, do not swing for it and do not spend boosts trying.
3. **Can I act: is the attacker Active with its Energy?**
   Active already, or a switch effect (Switch, Air Balloon or another retreat cost reducer, already attached or in hand), and the Energy on it or going on this turn. If no, it is a setup turn: attach, bench, and save gusts and boosts.

Then: **spend last.** Gusts, boosts and hand-shuffling Supporters wait until all three questions have answers, and draw before committing an attachment unless the attachment enables the draw.

## Checking answers

- Verify each number against `card_text` and `get_state`. If the player's number is wrong, show the sum step by step in the damage order above.
- Count what the opponent could have from what is visible and proven; say "likely" for the rest. `sample_hidden` can show what a typical hand allows.
- When the player has answered all three, ask for the play. If it differs from what the answers imply, point at the question that disagrees.

## Session shape

Three to five positions, varied: at least one where the right answer is "setup turn", and one where the opponent's reply is the deciding factor. End with which question the player answered wrong most; that is the one to say aloud every turn in their next games.
