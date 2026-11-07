---
created_date: "2026-11-07"
document_status: final
closed_date: "2026-11-07"
---

# Changelog: the assistant reacts like a real opponent during the player's turn

## What was built

When a player takes their turn on the board while the assistant plays the other side, the assistant's side now behaves like a real opponent across the table.

- **No-choice effects happen by themselves.** Cards whose effect on the opponent is "shuffle your hand into your deck (or onto the bottom) and draw" are recognised from their English card text, not from a list of names: Judge, Special Red Card, Iono, Unfair Stamp, Red Card, Reset Stamp, Roxanne, Marnie and anything worded the same way. When the player uses one against the assistant, the playmat carries out the assistant's part inside the same move as the card: its hand goes into its deck (shuffled) or onto the bottom (in a random order), then it draws the card's own count (one per remaining Prize card for Iono). The events are recorded under the card's action, so undoing the card puts both sides back exactly. The assistant's new cards stay hidden from the player in the board, the log and the board's move results. If the card's own condition is not met (Unfair Stamp with no knockout last turn, Special Red Card above 3 prizes), the effect still applies and the player gets a warning. This works in play mode, and in review-mode practice turns while a turn wait has handed the seat to the player.
- **The wait stops only for real decisions.** The turn wait no longer stops for every card. It tells the assistant about every applied effect (an `applied` list on every result, including still-waiting, decision, ended and turn-ended results) and stops only with a `decide` outcome: `promote` after its Active is knocked out with a Bench (listing exactly the Bench slots), or `answer_card` for cards that leave it a choice (`choose`) or that it must read (`read`): cards the playmat cannot apply, cards with no text, and family cards that could not be applied. Ordinary cards (Ultra Ball, Energy, evolving, damage-only attacks) never stop it. Each decision result names the decision and the assistant's seat and gives numbered steps ending with the exact hand-back call.
- **The player keeps playing while the assistant decides.** The assistant's board lock now has two strengths. Its own turn still locks the board ("Opponent is acting"). While it answers a decision, the board shows "Opponent is deciding" and refuses only actions that depend on the decision (attacking, ending the turn, moving the board back, anything aimed at the opponent's side, and Trainers or Abilities whose text names the opponent), with a message saying what it is waiting for. The hold lifts when the assistant hands the turn back, or after the existing release time; then the board says the opponent stopped responding, tells the player once that they can carry on, and offers "Promote to Active" on the opponent's Benched Pokémon.
- **Guides and docs.** The play-game, replay-drill and review-game guides, the `wait_for_turn` description, the standing server instructions and the website's play-game page describe applied effects and decision stops, and none says the wait stops for every card.

ChatGPT keeps the tool list it last fetched: after deploying this release, ChatGPT players must open Plugins → TCG Replay → Refresh and start a new chat to get the new `wait_for_turn` behaviour.

## Why it matters

Before this change the assistant had to be woken for every card the player used and do its own side of Judge or Iono by hand, and the board locked the player out while it did. Players practising against the assistant had to wait for it, or tell it what to do, during their own turn. Now a turn of ordinary cards and no-choice effects never blocks the player, the assistant is interrupted only when it genuinely has to decide, and even then the player keeps playing everything that does not depend on that decision, so playing against it feels like a real game.

## Deviations from the plan

- **Starting point.** The plan built on uncommitted `wait_for_turn` improvements (15-second cap, `wait`/`waited`, `keep_waiting`, `action`, numbered steps) that were only in the main checkout. They were copied into the feature's worktree as a patch for six files before work began; the main checkout still holds its own uncommitted copy, which must be committed or discarded before merging.
- **Use conditions.** An uncheckable "You can use this card only if …" sentence does not stop a family card from applying (it is the player's responsibility); the effect reader's acceptance criterion was clarified to say so.
- **Handing the seat over** (review-mode practice) was wired in the "apply family effects" task rather than the wait-reporting task, because its acceptance criterion needed a real turn wait.
- **Ended waits report applied effects.** A wait that ends early (mode switch, game closed or expired) now still reports effects applied but not yet reported; this needed a change in the game package beyond the renderer task's scope.
- **Wording.** The still-waiting and check-in texts gained "keep waiting until the turn ends", which the plan had asked to keep byte-identical.
- **Board guard.** During the decision-hold task the board kept refusing under either hold, so behaviour stayed unchanged until the next task narrowed it. The presence also records the deciding seat (`Decider`) so review-mode practice holds the correct side, and `OpenerNamesOpponent` takes `(verb, iid, ability)`.
- **Tests.** Tests that used the fixture Iono as an ordinary card were moved to draw-only or Eri-text cards, since the fixture Iono is now applied. The board test client's websocket read limit was raised in the shared helper, which fixed random board-test failures.
- **Known pre-existing flakes.** `TestGuideInstructions` (MCP package, under `-race`) and, once, `TestOpponentStatusPushed` (board package) failed intermittently; both also fail or are timing-sensitive without this work.
