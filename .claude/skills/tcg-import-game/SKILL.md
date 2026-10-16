---
name: tcg-import-game
description: Import a Pokémon TCG Live battle log into the tcgreplay playmat as a reviewable game with two complete decks, then triage it. Use when Nic pastes a TCG Live battle log, or asks to import, load, replay or review a game, whenever the tcgreplay tools (import_log, set_opponent_deck, get_profile) are available.
---

# Import a game

The goal is a game on the playmat that is ready to review: Nic's side exact, the opponent's side a complete, plausible 60, and a short triage so Nic knows whether it is worth a full review.

## 0. Check the tools

This needs the tcgreplay MCP tools (`import_log`, `set_opponent_deck`, `get_profile`). If they are not available, say the playmat is not connected and stop. Do not reconstruct the game by hand.

If the `pokemon-game-ledger` skill is also active, it logs the result to Notion; do that as usual. This skill only adds the playmat game.

## 1. Read the log first

Before importing, read the log yourself:

- **Who and what:** Nic's opponent, who went first, the result and how it ended (prizes, concession, deck-out, bench-out).
- **The opponent's archetype**, from the Pokémon they evolved and attacked with.
- **Mulligans and opening hands:** Nic's is itemised; an opponent's mulligans reveal whole hands.

## 2. Build the opponent's list

Follow the `tcg-opponent-deck` skill to write a likely 60-card list for the opponent from what the log shows. Doing it before the import means the game is built complete in one step.

## 3. Import

Call `import_log` with:

- `log`: the battle log exactly as pasted.
- `opp_deck`: the list from step 2.
- `deck`: only if Nic gives a decklist or says he changed lists; otherwise omit it and the previous import's deck is reused. If the import fails for lack of a decklist, ask Nic for his export.
- `id`: omit to get one from the date and opponent.

## 4. Check the result

- **`report.problems`** lists log lines that could not be replayed. A few are tolerable; say which and what they affect (e.g. "Itchy Pollen's item lock is not shown"). Many mean the import is unreliable: tell Nic rather than reviewing on top of it.
- **`warnings` starting "opponent list:"** mean your list disagreed with the log and was corrected. If they show a misread archetype, fix the list and call `set_opponent_deck` now, before anyone makes branches.
- **"the assumed list conflicts with the log"** means the list could not be held to its counts. Fix the list the same way.
- Check the turns, prizes taken and winner against what you read in step 1.

## 5. Triage

Give Nic a short summary:

- Result, opponent, archetype, turns, prizes taken each way.
- The opponent list you assumed: archetype and confidence, and that unseen cards are dealt from it.
- **Worth reviewing?** Games with few real decisions where the loss traces to Nic's opening hand (a lone Basic, no Supporter, a dead draw) belong in the deck-building bucket, not the review queue: say so and why. Otherwise name the two or three decision points that look most worth a rewind (for example the turn a prize race swung).
- The game id, and that it is on the playmat with its timeline. Clicking a turn header rewinds to the start of that turn.

Then offer to review it. Do not start judging plays until Nic asks: the review asks why he made a play before giving a view.
