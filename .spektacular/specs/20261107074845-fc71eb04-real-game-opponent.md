---
created_date: "2026-11-07"
document_status: final
closed_date: "2026-11-07"
---

# Feature: 20261107074845-fc71eb04-real-game-opponent

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

When a player takes their turn on the board while the assistant plays the other side, the assistant's side should respond the way a real opponent across the table would. Cards that make the opponent shuffle their hand into their deck and draw (Judge, Red Card, Iono and similar) take effect on the opponent's side straight away, with no one asking. The assistant is only interrupted when it has a real decision to make, such as which Pokémon to promote after a knockout, or when a card affects its side in a way the playmat can't apply itself. While it decides, the player keeps playing. The player benefits most: they no longer have to wait for, or instruct, the assistant during their own turn, so playing against it or practising a turn feels like a real game.

<!--
  REQUIREMENTS
  Specific, testable behaviours the feature must deliver.
  Format: bold title on the checkbox line, detail indented below.
  Rules:
    - Use active voice: "Users can...", "The system must..."
    - Each requirement should be independently verifiable
    - Focus on WHAT, not HOW — avoid prescribing implementation
    - Keep each item atomic — one behaviour per line
-->
## Requirements

- [x] **No-choice opponent effects apply themselves**
  When the player uses a card whose effect on the opponent leaves the opponent no decision, the system must carry out that effect on the assistant's side at once, as part of the player's card, without stopping the wait or asking anyone. This covers any card whose effect moves the opponent's whole hand into their deck (shuffled in, or onto the bottom) and then has them draw a number of cards that is fixed or can be counted from the board, with no choices; Judge, Special Red Card, Iono and Unfair Stamp are the minimum set. The player resolves their own side of such a card as they do today.
- [x] **Card conditions stay the player's responsibility**
  When the player uses one of these cards while its own condition is not met (for example Unfair Stamp with none of their Pokémon knocked out during the opponent's last turn), the system must still apply the effect and show the player a warning that the condition is not met.
- [x] **The assistant learns what happened to its side**
  The system must tell the assistant about every effect it applied to the assistant's side, in its next wait result and in the game log, so the assistant's picture of its hand and deck stays correct.
- [x] **Undo reverses an applied effect**
  When the player undoes a card whose effect was applied to the assistant's side, the system must reverse that effect along with the card.
- [x] **Cards that don't involve the opponent never stop the wait**
  The system must not stop the wait when the player uses a card, Ability or attack that does not involve the opponent's side (for example Ultra Ball, attaching Energy, or an attack that only deals damage).
- [x] **The wait stops when the assistant must decide**
  The system must stop the wait when the assistant's side has a decision to make: promoting after its Active Pokémon is knocked out, or a choice a card leaves to the opponent. A knockout that leaves the assistant with no Pokémon in play ends the game as it does today, with no promotion.
- [x] **A decision stop says exactly what to decide**
  When the wait stops for a decision, the result must name the decision, the assistant's seat, the valid choices where they are known (for a promotion, the Benched Pokémon it may promote), and how to hand the turn back and keep waiting.
- [x] **Unrecognised opponent cards still ask the assistant**
  When the player uses a card whose text involves the opponent but which the system cannot apply itself, the system must stop the wait and ask the assistant to read the card and do what it asks.
- [x] **The player keeps playing while the assistant decides**
  While the assistant is deciding, the player must be able to carry on with any action that does not depend on that decision. Only actions that depend on it are held until the assistant hands the turn back, for example a gust, an attack or ending the turn while the assistant's Active Spot is empty awaiting promotion.
- [x] **A hold never lasts forever**
  If the assistant has not handed the turn back within the board's existing release time, the system must lift the hold and tell the player, who can then carry on, including making the assistant's move themselves on the board.
- [x] **Held actions tell the player why**
  When the system holds a player action because the assistant is still deciding, the board must say so, and the player can retry once the assistant has finished.
- [x] **The turn-end summary covers the whole turn**
  When the player's turn ends, the wait result must still list everything the player did that turn, including effects applied to the assistant's side.
- [ ] **Works in play and in practice**
  The behaviour must apply wherever the assistant waits on the player's turn: a game against the assistant, and a practice turn in a review or drill.
- [x] **Applied effects respect hidden information**
  Cards the assistant's side draws or shuffles through an applied effect must stay hidden from the player wherever the assistant's hand is hidden today.
- [x] **Guides and docs describe the new behaviour**
  The assistant guides, the wait instructions, and any website page that describes waiting or responding during the player's turn must describe this behaviour and no longer say the wait stops for every card.

<!--
  CONSTRAINTS
  Hard boundaries the solution must operate within. These are non-negotiable.
  Format: one bullet point per constraint.
  Examples:
    - Must integrate with the existing authentication system
    - Cannot introduce breaking changes to the public API
    - Must support the current minimum supported runtime versions
  Leave blank if there are no constraints.
-->
## Constraints

- **Real decisions stay with the assistant.** The playmat may only carry out effects that leave the opponent no choice; anything that needs judgement stays with the assistant or the player.
- **Waits stay short polls.** Each wait call must return within HTTP timeout limits (at most 50 seconds, 15 by default), and the assistant learns of events only through those calls. Chat clients do not turn server notifications into assistant turns.
- **Every assistant client.** The behaviour must work for any MCP client, ChatGPT and Claude included, without relying on client-specific features.

<!--
  ACCEPTANCE CRITERIA
  The specific, binary conditions that define "done".
  Format: bold title on the checkbox line, verifiable detail indented below.
  Each criterion must be:
    - Independently verifiable (pass/fail, not subjective)
    - Traceable back to a requirement above
    - Testable by someone who didn't write the code
-->
## Acceptance Criteria

- [x] **Judge applies itself**
  With the assistant waiting on the player's turn and holding 6 cards, the player plays Judge on the board. Without the assistant calling any tool, the assistant's hand is shuffled into its deck, it then holds 4 cards, and the wait has not stopped.
- [x] **Each family card applies with the right count**
  With no wait stop, the assistant's hand afterwards holds: Special Red Card, 3 cards; Unfair Stamp, 2 cards; Iono, as many cards as the assistant has prize cards remaining (and its old hand is at the bottom of its deck). Each count matches the card's own text as the playmat shows it.
- [x] **Unmet condition warns but still applies**
  Playing Unfair Stamp when none of the player's Pokémon were knocked out during the opponent's last turn still leaves the assistant with 2 cards from its shuffled deck, and the board shows the player a warning that the card's condition was not met.
- [x] **The assistant is told**
  After an applied effect, the assistant's next wait result and the game log both name the card and what happened to its side, such as its hand being shuffled into its deck and 4 cards drawn.
- [x] **Undo reverses it**
  After the player plays Judge and then undoes it, the assistant's hand, deck and the player's hand match their state before Judge was played.
- [x] **Non-opponent cards don't stop**
  During one player turn of Ultra Ball, attaching Energy, evolving, and an attack that only deals damage without a knockout, the wait never stops for a decision or an unrecognised card: every result until the turn ends is still waiting.
- [x] **Knockout stops for promotion with the choices**
  When the player knocks out the assistant's Active Pokémon and it has Benched Pokémon, the wait stops with a promotion decision. The result names the assistant's seat, lists exactly the bench slots it may promote from, and says how to hand the turn back.
- [x] **Opponent-choice cards stop**
  When the player uses a card that leaves a choice to the opponent (for example one making each player switch their Active Pokémon), the wait stops with a decision naming the card and the assistant's seat.
- [x] **Unrecognised opponent cards ask**
  When the player uses a card whose text mentions the opponent but isn't in the set the playmat applies itself, the wait stops and asks the assistant to read that card and act. A card that mentions the opponent but needs nothing from them (such as a gust the player resolves themselves) costs at most one such stop.
- [x] **Player not locked out**
  While a promotion decision is pending, the player can still attach Energy, evolve, and play an Item that only touches their own side, and each takes effect at once.
- [x] **Dependent actions are held, with a message**
  While a promotion decision is pending, the player's attempts to attack and to end the turn are held, the board shows a message saying it's waiting for the opponent to promote, and the same actions succeed once the assistant has promoted and handed the turn back.
- [x] **A stalled hold lifts**
  When the assistant does not respond to a promotion decision, the hold lifts after the board's release time, the board tells the player, and the player can then promote the assistant's Pokémon themselves and attack.
- [x] **Knockout with no Pokémon left ends the game**
  When the player knocks out the assistant's last Pokémon in play, the game ends with the player as winner and no promotion is asked for.
- [x] **Turn summary complete**
  When the player's turn ends, the wait result lists every action the player took that turn, including any effect applied to the assistant's side.
- [ ] **Practice turns too**
  In review mode, when the player practises a turn in a review or a drill handed over by the assistant, playing Judge applies it to the other seat and a knockout stops for promotion, exactly as in play mode.
- [x] **Hidden cards stay hidden**
  In play mode, after Judge applies itself, the board shows the player the assistant's new hand only as face-down cards and its count, never the card identities.
- [x] **Guides and docs updated**
  No guide, wait instruction or website page says the wait stops for every card; the guides and the playing docs describe applied effects and decision stops.

<!--
  TECHNICAL APPROACH
  High-level technical direction to guide the planning agent. Include:
    - Key architectural decisions already made
    - Preferred patterns or technologies if known
    - Integration points with existing systems
    - Known risks or areas of uncertainty
  Format: one bullet point per direction/steer.
  Leave blank if you want the planner to propose the approach.
-->
## Technical Approach

- **Read the card text.** Decide from the text the playmat already holds whether a card involves the opponent and whether it is one of the no-choice effects it can apply itself. Recognise those by the wording of the effect (the shuffle-your-hand-and-draw family), not by a maintained list of card names.
- **Applied effects belong to the player's card.** Record each applied effect in the game history as part of the card that caused it, so undo and the turn summary treat them as one.
- **Replace the per-card stop with decision stops.** Keep using wait results, but turn the generic stop to check every card into a stop that names the decision, with numbered steps ending in handing the turn back, building on the action field and step-by-step instructions recently added to wait results. Instructions the assistant must follow at that moment belong in the tool result, not only in guides, because some clients pay little attention to guide or server instructions.
- **Narrow the existing lock.** Prefer narrowing the current whole-board lock to the pending decision's dependencies over adding a separate mechanism.
- **Known risks.** The text check will have false alarms and misses; the fallback to asking the assistant limits the damage. The family's wording varies (Iono draws one card per remaining prize; Unfair Stamp works only if one of your Pokémon was knocked out during your opponent's last turn), so the risk is applying a family-wide assumption where a card's own condition or count differs.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- During a player turn made only of cards that do not involve the opponent, or of no-choice opponent effects, the player's next legal move is never blocked: zero seconds of waiting on the assistant.
- When the assistant must decide, only actions that depend on that decision are blocked, and only for as long as the assistant takes to decide.
- The wait stops at most once per decision the assistant's side actually has to make, plus at most one check for each card the playmat cannot apply itself; a turn of ordinary cards produces no stops at all.
- In a played-out turn against ChatGPT that includes Judge and a knockout, the player never has to tell the assistant to act or to keep waiting.

<!--
  NON-GOALS
  Explicitly state what this spec does NOT cover. This is as important as
  the requirements — it prevents scope creep and sets clear expectations.
  Format: one bullet point per exclusion.
  Examples:
    - "Mobile support is out of scope (tracked in #456)"
    - "Internationalisation will be addressed in a follow-up spec"
  Leave blank if there are no explicit exclusions to call out.
-->
## Non-Goals

- General rules enforcement or new legality checks beyond today's warnings.
- A step where the player tells the opponent what to do.
- A setting to switch the automatic responses off.
- Keeping game records saved before this change working.
- Request ids or protection against a decision being resolved twice.
