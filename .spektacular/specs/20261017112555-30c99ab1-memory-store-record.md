---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
epic: 20261017105033-91f90842-hosted-playmat
---

# Feature: 20261017112555-30c99ab1-memory-store-record

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

The playmat stops keeping games and instead hands Claude a complete, portable record of each imported game: the battle log, both decks, and what's needed to deal the hidden cards the same way. That lets the hosted playmat store nothing at all. The player keeps their records wherever they like and can bring any game back exactly as it was.

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

- [x] **Nothing kept between sessions**
  A session is one run of the playmat, from start to stop. When a session ends, every game goes with it, and a new session starts with no game open.
- [x] **No lasting game storage**
  The playmat keeps no game in lasting storage. Saved games and their import data are never written or read; the card cache is not game storage.
- [x] **Record with every import**
  An import's result includes the game record: transcript, the player's name, the player's deck, the opponent's deck as used, and the value that fixes how hidden cards are dealt.
- [x] **Record follows deck changes**
  Setting the opponent's deck returns the updated record.
- [x] **Record on request**
  Claude can ask for the current game's record at any time.
- [x] **No record without a battle log**
  Asking for the record of a game that was started without a battle log reports that it has no record.
- [x] **Exact restore**
  Importing a record reproduces the identical game, with the same cards in the same places at every point of the timeline.
- [x] **Damaged records never restore wrong**
  A record altered in transit (for example curly quotes or changed whitespace from a note app) is either restored correctly or rejected with a clear message; it is never silently restored as a different game.
- [x] **New game replaces current**
  Importing or starting a game replaces the current one.
- [x] **No reopening earlier games**
  There is no way to list or reopen earlier games; a game comes back only by importing its record.
- [x] **Portable record**
  The record is text that can be pasted into a note and pasted back to restore the game.
- [x] **Drills use the current game**
  The replay drill, numbers drill and before-you-move skills work from the game open now, and say so when none is open.

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

- Must apply to both desktop and hosted modes.
- Must leave game files already on disk untouched.
- The record must hold only the raw game (transcript, player's name, both decks, seed); no branches, notes or findings.
- Import must continue to accept a plain battle log, as well as a full record.

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

- [ ] **Restart forgets**
  Import a game and restart the playmat, desktop and hosted. No game is open afterwards.
- [x] **Nothing written**
  After importing and playing a game, the data directory has no new or changed saved games or import data (the card cache is excluded), and existing files are byte-for-byte unchanged.
- [x] **Import returns record**
  The import result's record has a transcript identical to the log passed in, the player's name, both decklists, and the information needed to deal hidden cards identically.
- [x] **Deck change updates record**
  After setting the opponent's deck, both the result's record and the record tool show the new list.
- [x] **Record tool**
  Asking for the record returns the same record as the most recent import or deck change.
- [x] **Play-mode game**
  A game started without a battle log has no record, and asking for its record says so.
- [x] **Exact restore**
  Take an imported game's full state (reveal view, so prizes, hands and decks are visible) at its start, middle and end. Restart, import its record, and the state at those points matches exactly.
- [ ] **Paste round trip**
  A record copied through a plain-text note (e.g. a Notion code block) and pasted back restores the same game.
- [x] **Damaged paste**
  A record with straight quotes turned curly and whitespace altered either restores to the identical game or is rejected with a message saying it was damaged.
- [x] **Second import replaces first**
  Importing a second game replaces the first; only the second is open.
- [x] **Tools removed**
  The tool list no longer offers listing or loading saved games.
- [ ] **Drills**
  With a game open, each drill skill runs on it. With none open, each one asks for an import. Each completes with no error and no mention of listing or loading saved games.

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

- Prefer the store's existing in-memory mode in place of game files.
- Prefer basing the record on the import data the playmat saves alongside each imported game today (log, decks, seed, player name), made complete and returned as JSON.
- Prefer restoring through import accepting a full record including its seed, over a separate restore operation.
- The player's last deck may be remembered in memory for the session, so a later import can reuse it.
- Risk: desktop users lose "restart reopens my last game"; branches and notes vanish on restart.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- Over the owner's first 10 real games after delivery, every saved record restores with no differences.
- The owner saves records to Notion after games without hand-editing them.

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

- No drilling across many past games; that is left for later.
- No records for games started in play mode.
- The card cache stays on disk and is not made memory-only.
- Per-user games are out of scope; covered by the per-user-games spec.
- No Notion integration and no packaging of skills for claude.ai.
