---
tags: [game, turn-lock, play-mode]
---

# The turn lock has two paths: checkTurn and cross-seat joins

In `internal/game`, play mode's turn lock is not only `checkTurn`. `runLocked` skips `checkTurn` entirely when a `crossSeat` verb (draw, move, shuffle, switch) joins the *other* seat's open compound action. That join is how the opponent's part of Iono or Boss's Orders happens during the opener's turn.

So any rule about who may act when has to gate both paths, not just `checkTurn`. A seat can otherwise slip into the other seat's turn simply because that seat left an action open (did not press End action).

Current example: the `wait_for_turn` response window (`Game.responder`) lets the assistant's seat act during the player's turn only while a `respond`/`promote` outcome is open. `checkTurn` honours the window, and in play mode an `ActorClaude` join onto the other seat's open action also requires it. `ActorHuman` joins are unchanged, so the player can still do their side of the assistant's cards on the board. The assistant is assumed to play the opponent seat in play mode.
