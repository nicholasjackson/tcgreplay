---
name: play-game
description: Use when the player wants to play a game against you on the playmat (new_game, play mode), so your opponent's play never uses the player's hidden cards.
---

# Play a game

In a played game you have two roles. As **coach** you may see whatever the player shows you: their hand in chat or a screenshot, a review, a reveal view. As **opponent** you must play only on what your own seat can see. Anything the coach has seen stays in your context and would steer the opponent's choices, so keep the two roles apart.

## Set up

- `new_game` with both decklists. A hand with no Basic Pokémon is mulliganed automatically. The result's warnings say how many times, and how many extra cards the other player may draw: ask the player whether they want theirs, then `draw` them.
- Place your seat's Active and Bench (`place_active`, `bench`). The player places theirs on the board. `start_game` refuses until both players have an Active Pokémon.
- Give the player the board link.

## Your turns: play them blind

If you can start sub-agents, play **each** opponent turn in a fresh one, and give it only:

- the opponent's decklist and its seat id;
- a short play-style brief: the archetype, its game plan, and how aggressively to take prizes;
- the rules: read the board only with `get_state` view `seat`, never `coach` or `reveal`; quote card text from `card_text`; play the whole turn with its seat's verbs; then `end_turn`;
- what to return: a short summary of the turn, made only of public information.

Never put anything only the coach knows into that brief: the player's hand, a reveal, earlier coaching, or your view of their plan. When it returns, relay the summary in a line and call `wait_for_turn` yourself.

If you cannot start sub-agents, decide the turn from `get_state` view `seat` alone, before you discuss the position with the player, and do not look back at anything the player has shown you.

## The player's turns

- Call `wait_for_turn` and do what its `next` says, again and again until the turn ends.
- The playmat carries out your side of shuffle-hand-and-draw cards the player uses (Judge, Iono and the like) by itself; the wait lists them in `applied`. A `decide` outcome means your seat must act now: promote after a Knock Out (pick from its `choices`), or answer a card it lists. Decide it on `get_state` view `seat` only, as above, then call `wait_for_turn` in the same reply.
- After a Knock Out, the result reminds whose prizes to take. The player can turn the reminder off on the board.
- `end_turn` refuses while a player with a Benched Pokémon has an empty Active Spot (promote first) or while prize cards are owed for the turn's Knock Outs (take them first). Pass `force` only when a card's effect changes the rule.
- Costs count: an Item or Ability that asks for cards discarded from the hand (Ultra Ball, Lunar Cycle) warns when it closes unpaid. Discard them with `move` inside its action, never by playing them.
- Stadium pairs: a Stadium whose `card_text` says it is played as a pair (Delta Reign's Legendary Stadiums) goes down with one `play_trainer` naming both halves, `iid` and `pair_iid`, two different prints. A lone half shows a warning until its partner joins it; `play_trainer` with the partner alone completes the pair. A Stadium played over a pair takes both halves to their owner's discard.

## Game over

When a player takes their last prize card or has no Pokémon in play, every result says "Game over" and `wait_for_turn` stops waiting. Tell the player the result, then offer a review of the game (guide `review-game`).

## Coaching during the game

Coach only when the player asks. Your coaching can use anything they showed you. Your opponent's play never does.
