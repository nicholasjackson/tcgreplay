---
created_date: "2026-10-19"
document_status: final
closed_date: "2026-10-19"
---

# Feature: 20261019170829-bfb31628-turn-numbering-by-round

## Overview

The playmat numbers turns the way Pokémon players count them: "Turn 3" means the first player's third turn and the second player's third turn together, always shown with whose turn it is. Today every player turn gets its own number, so "Turn 4" is really the second player's 2nd turn, and the player and the assistant have to translate between the two counts in every review, drill and log. Players reviewing or playing on the playmat, and the assistant coaching them, get one shared way to name a turn.

## Requirements

- [x] **One turn number covers both players**
  The system must number turns so that Turn N is the first player's Nth turn followed by the second player's Nth turn. Turn 1 is the first player's opening turn and the second player's first turn.
- [x] **A turn is always shown with whose turn it is**
  Wherever the system names a turn that is in progress or a single player's turn, it must also say whose turn it is, so the two players' halves of one turn can never be confused.
- [ ] **Playmat header shows the combined turn**
  Users see the current turn on the playmat as the combined number plus the player, e.g. "Turn 3 · Your turn" or "Turn 3 · Lukiees's turn".
- [ ] **Timeline groups both players under one turn**
  The timeline must show one section per combined turn, with a separate sub-section for each player's half, and the setup section unchanged.
- [ ] **Timeline rewind and play work per player's half**
  Users can rewind to, and play from, the start of either player's half of a turn from the timeline, as they can for a single turn today.
- [ ] **Playback position shows the combined turn**
  The playback bar must show the combined turn number and whose half the current step is in.
- [x] **Game log uses the combined turn**
  Every log line, including turn start and end lines, must carry the combined turn number and the player whose half it belongs to, for both the playmat and the assistant.
- [x] **Board state reports the combined turn**
  The board state the assistant reads must report the combined turn number and whose turn it is.
- [x] **Lingering effects expire by combined turn and player**
  The assistant can record a lingering effect that lasts until the end of a named player's turn in a given combined turn, and the effect's description shows its expiry in the same terms.
- [x] **Messages and prompts use the combined turn**
  Error messages about acting out of turn, and the prompt the playmat copies for the assistant when the player ends their turn, must name the turn by the combined number and the player.
- [x] **Coaching skills speak the combined numbering**
  The review, drill and pre-move skills must name turns by the combined number and the player, drop the instruction to translate between the two counts, and name the branches they create so that the two halves of one turn get different names.
- [x] **Game rules behave exactly as before**
  Turn-based rules and the warnings they raise (opening-turn limits on evolving, Supporters and attacking, effect expiry, the turn a Pokémon came into play, a branch replaying the same draws as the game it was taken from) must behave identically to today.
- [x] **Existing saved games show the combined numbering**
  Users who restore a game saved before the change see its turns labelled with the combined numbering.

## Constraints

- The playmat's existing per-player turn count must keep its current meaning underneath: game rules depend on it, and the combined turn is derived from it rather than replacing it.
- Game records already saved must keep loading without conversion: players hold them in their own notes and paste them back to restore a game, so their format is an existing contract.
- The assistant-facing tools must switch to the combined numbering along with the playmat, not only the visible UI.

## Acceptance Criteria

- [x] **Turn numbers advance once per pair of player turns**
  In a game started on the playmat, the first player's first turn and the second player's first turn both show as Turn 1; the first player's second turn and the second player's second turn both show as Turn 2; the first player's fifth turn shows as Turn 5.
- [ ] **Header names the combined turn and the player**
  On the second player's third turn, the playmat header reads "Turn 3" with that player's name (or "Your turn" when it is the viewer's seat), and never "Turn 6".
- [ ] **Timeline has one heading per combined turn**
  After five full turns on each side, the timeline shows Setup followed by headings Turn 1 to Turn 5, each holding one sub-section per player in the order they played; no heading reads "Turn 6" or higher.
- [ ] **Rewind and play from either half**
  Using the timeline's rewind on the second player's half of Turn 3 puts the board at the start of that player's third turn; using it on the first player's half of Turn 3 puts the board at the start of the first player's third turn; the two rewinds land on differently named branches. Play from either half starts playback at that half.
- [ ] **Playback bar shows the combined turn and player**
  While stepping through the second player's third turn in playback, the playback position shows turn 3 and identifies that player's half.
- [x] **Log lines carry combined turn and player**
  The log the assistant reads for the second player's third turn shows turn 3 and that player on every line, including the turn start and end lines; no line in a game of ten player turns (five each) shows a turn above 5.
- [x] **Board state reports combined turn and player**
  Reading the board state during the second player's third turn reports turn 3 and that player as the one whose turn it is.
- [x] **Effect expiry by combined turn and player**
  An effect recorded to last until the end of the second player's turn in Turn 4 is still on the board during the first player's half of Turn 4, is gone once the second player ends their Turn 4, and its log description names Turn 4 and that player.
- [x] **Out-of-turn error and end-turn prompt**
  Trying to act for the wrong player in play mode returns an error naming the combined turn and the player whose turn it is; ending the player's turn copies a prompt that names the assistant's turn by the combined number.
- [x] **Skills use combined numbering and distinct branch names**
  The review, drill and pre-move skill instructions refer to turns by combined number and player, contain no instruction to translate between per-player and combined counts, and specify branch names that differ between the two players' halves of the same turn.
- [x] **Opening-turn warnings unchanged**
  In a new game, playing a Supporter or attacking during the first player's half of Turn 1, or evolving during either player's half of Turn 1, raises the same warnings as before; the same actions in Turn 2 raise none of them.
- [x] **Saved record round-trips**
  A game record saved before the change loads without error, shows the same board at every step as before, and labels its turns with the combined numbering.

## Technical Approach

- Derive the combined turn from the per-player count as (count + 1) / 2, rounded down, paired with the seat whose turn it is; compute it in one place on the server so the playmat, logs and tools agree.
- For effect expiry, let the assistant give a combined turn plus the player whose turn it ends on, and have the server convert that to the per-player count it already expires effects by.
- In the coaching skills, name branches with both the turn and whose half it is, so the two halves of one turn get distinct branch names; the timeline's own rewind branches should follow the same pattern.
- Keep existing rule tests unchanged; update only tests that assert turn labels to the combined numbering.

## Success Metrics

- In a review session, the assistant names turns by the combined number without a "(your Nth turn)" translation, and the player does not need to correct a turn reference.
- No surface a player or the assistant reads (playmat, timeline, playback, log, board state, errors, prompts, skills) shows a per-player turn count after delivery, apart from notes, annotations and branch names recorded before the change.

## Non-Goals

- Rewriting notes, findings or annotations already recorded in games that mention turns by the old per-player numbering; they keep their text as written.
- Renaming branches already created under the old numbering.
- Changes to importing TCG Live battle logs, which carry no turn numbers.

