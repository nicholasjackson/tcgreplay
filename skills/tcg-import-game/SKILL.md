---
name: tcg-import-game
description: Import a Pokémon TCG Live battle log into the tcgreplay playmat as a reviewable game with two complete decks, then triage it. Use when the player pastes a TCG Live battle log or a saved game record, or asks to import, load, replay or review a game, whenever the tcgreplay tools (import_log, set_opponent_deck, get_profile) are available.
---

# Import a game

The goal is a game on the playmat that is ready to review: the player's side exact, the opponent's side a complete, plausible 60, and a short triage so the player knows whether it is worth a full review.

## 0. Check the tools

This needs the tcgreplay MCP tools (`import_log`, `set_opponent_deck`, `get_profile`). If they are not available, say the playmat is not connected and stop. Do not reconstruct the game by hand.

If another skill records results (a game ledger, a notes vault), let it do its part as usual. This skill only adds the playmat game.

## 1. Read the log first

Before importing, read the log yourself:

- **Who and what:** which seat is the player, the opponent's name, who went first, the result and how it ended (prizes, concession, deck-out, bench-out).
- **The opponent's archetype**, from the Pokémon they evolved and attacked with.
- **Mulligans and opening hands:** the player's is itemised; an opponent's mulligans reveal whole hands.

## 2. Build the opponent's list

Follow the `tcg-opponent-deck` skill to write a likely 60-card list for the opponent from what the log shows. Doing it before the import means the game is built complete in one step.

## 3. Import

If the player pastes a game record (JSON starting with `{` and containing `tcgreplay_record`) instead of a battle log, pass it to `import_log` as `log` exactly as pasted, with no other arguments, and skip step 2 and the list fixing in step 4: the record already holds both lists, the player's name and the seed, so the import rebuilds the identical game. If the import says the record was damaged, ask the player to copy it again unchanged from where they saved it, or to paste the original battle log.

Otherwise call `import_log` with:

- `log`: the battle log exactly as pasted.
- `opp_deck`: the list from step 2.
- `deck`: only if the player gives a decklist or says they changed lists; otherwise omit it and the previous import's deck is reused. If the import fails for lack of a decklist, ask the player for their TCG Live export.
- `id`: omit to get one from the date and opponent.

## 4. Check the result

- **`report.problems`** lists log lines that could not be replayed. A few are tolerable; say which and what they affect (e.g. "Itchy Pollen's item lock is not shown"). Many mean the import is unreliable: tell the player rather than reviewing on top of it.
- **`warnings` starting "opponent list:"** mean your list disagreed with the log and was corrected. If they show a misread archetype, fix the list and call `set_opponent_deck` now, before anyone makes branches. Its result carries the updated record: offer that one in step 5, not the first.
- **"the assumed list conflicts with the log"** means the list could not be held to its counts. Fix the list the same way.
- Check the turns, prizes taken and winner against what you read in step 1.

## 5. Triage

Give the player a short summary:

- Result, opponent, archetype, turns, prizes taken each way.
- The opponent list you assumed: archetype and confidence, and that unseen cards are dealt from it.
- **Worth reviewing?** Games with few real decisions where the loss traces to the opening hand (a lone Basic, no Supporter, a dead draw) belong in the deck-building bucket, not the review queue: say so and why. Otherwise name the two or three decision points that look most worth a rewind (for example the turn a prize race swung).
- The game id, and that it is on the playmat with its timeline.
- The game record from the result (`record`), in a code block, exactly as returned (after a `set_opponent_deck` fix, the updated one). The playmat keeps nothing after this session: the player can keep the record in their notes and paste it back later to bring this exact game back. Branches and notes made during the review are not in it and last only for this session. `get_record` returns it again at any time.

Then offer to review it with the `tcg-review-game` skill. Do not start judging plays until the player asks.
