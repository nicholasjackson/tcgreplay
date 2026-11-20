---
tags: [game, testing, cards, record]
---

# Card swaps in game tests revert on undo and checkout

Game tests often turn a fixture card into another card by rewriting it on the live state, under the game lock: `g.st.Cards[iid].Def, g.st.Cards[iid].Name = "sv01-176", "Judge"` (see `respondGame` in `internal/game/presence_respond_test.go`). That change lives only in `g.st`. The log still records the original card.

`Undo`, `StepBackTo`, `Checkout`, `Rewind` and anything else that goes through `checkoutLocked` rebuild the state by refolding the log. The swapped card silently becomes the fixture card again (for example the cardstest Iono `tst-4`), with that card's text.

- A test that undoes or moves the board, then plays the swapped card again, must re-apply the swap first. `appliedCards.rename` in `internal/game/applied_test.go` does this.
- If a test behaves as though a card has the wrong text after an undo, suspect a reverted swap before suspecting the game code. One test author first took this for a bug in the review-mode handover.
- Log lines name swapped cards by their fixture names, because the log was written with them.

The fixture Iono `tst-4` is itself a shuffle-hand-and-draw card. Since the playmat applies that family to the assistant's side, a human play-mode Iono in a test changes p2's hand. Swap it to a draw-only def when a test only needs "a Supporter".
