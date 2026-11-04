---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
designs:
    - source: design
      path: wide-board/design.md
---

# Feature: 20261104090653-7373a393-wide-screen-landscape-board

<!--
  OVERVIEW
  A concise 2-3 sentence summary of the feature. Answer three questions:
    1. What is being built?
    2. What problem does it solve?
    3. Who benefits and why does it matter?
  Avoid implementation details — this should be readable by any stakeholder.
-->
## Overview

On a wide desktop screen, the board switches to a landscape layout: the Active Pokémon face each other across the middle, and each player's bench, prizes and piles share one row. Cards come out at least 1.6 times today's width, instead of sitting in a narrow column of empty felt. Players reviewing or playing on a big monitor can read every card and count at a glance, and the smaller fixes to hand counts, text size and the playback controls help every desktop and tablet screen.

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

- [ ] **Landscape layout on wide screens**
  When the board area beside or above the timeline is 1600px wide or more, the board must use the landscape layout. Below that width, desktop and tablet screens must keep today's mat, and phones their phone layout.
- [ ] **Actives face each other**
  In the landscape layout, both Active Pokémon must sit side by side in one middle row, each labelled with whose it is, with the Stadium on one side and the turn status and turn controls on the other.
- [ ] **One row per player**
  In the landscape layout, each player's bench, prizes and piles (Deck, Discard, Lost Zone) must share one row: the opponent's above the middle row, the player's below it. The two rows mirror each other the way opposite seats do on a real mat.
- [ ] **Prizes as a fanned row**
  In the landscape layout, each player's remaining prizes must show as one overlapping row in which every remaining prize can still be seen and counted, and taken individually.
- [ ] **Opponent's hand peeks from the top edge**
  In the landscape layout, the opponent's hand must hang off the top edge of the board, showing only a strip of each card, enough to count the cards. Its count must still be available as text to screen readers.
- [ ] **Opponent's name and status out of the way**
  In the landscape layout, the opponent's name and status must show in a corner of the board, not in a bar across it.
- [ ] **Cards use the space**
  In the landscape layout, cards must be sized so the board fills the board area's height or width, whichever runs out first. At 2560×1440 with the timeline beside the board, a benched card must be at least 1.6 times as wide as on today's mat.
- [ ] **Nothing is lost**
  Every card, pile, count, control and action reachable on today's mat (opening piles, taking prizes, drag and drop, the action sheet, turn controls) must be reachable in the landscape layout.
- [ ] **Hand counts beside their hands**
  On every desktop and tablet layout, each hand's count must sit beside that hand, not at the edge of the board area.
- [ ] **Text grows on big screens**
  On screens wider than 1920px, the header, timeline and board labels must grow with the screen instead of staying at a fixed small size, and must stay readable next to the cards. At 1920px wide and below, text must stay at today's size.
- [ ] **Wider timeline on big screens**
  On big screens, the timeline beside the board must be allowed to grow wider than today's 480px limit, in step with its larger text.
- [ ] **Playback controls centred over the board**
  The playback bar and its move caption must be centred horizontally over the board area, not over the whole window, whether the timeline sits beside the board, below it, or is hidden. They may cover the board.
- [ ] **Layout follows resizing**
  Resizing the window, or moving or hiding the timeline, must switch between the landscape layout and today's mat at once, without a reload.

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

- The wide-screen layout must implement the landscape mat settled in design source `design` at `wide-board/design.md`: when it applies, the zones' arrangement, the sizing rule, and the desktop-wide changes to hand counts, text, timeline width and playback centring.
- Below the landscape layout's width, today's desktop and tablet mat must not change, apart from those desktop-wide changes.
- The phone layout must not change. It has its own settled design at `mobile-board/design.md` in design source `design`.

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

- [ ] **Switches at wide sizes**
  At 2560×1440 with the timeline beside the board, the board shows the landscape layout. At 1440×900 with the timeline beside the board it shows today's mat, and on a 390×844 phone the phone layout.
- [ ] **Middle row holds both actives**
  In the landscape layout, both Active Pokémon sit in the same row, side by side, with the Stadium on one side of them and the turn status and controls on the other.
- [ ] **Player rows mirror each other**
  In the landscape layout, the opponent's bench, prizes and piles sit in one row above the actives, and the player's in one row below. Each row's prizes and piles are at opposite ends from the other row's.
- [ ] **Prizes countable and takeable**
  In the landscape layout with 6, 3 and 1 prizes left, every remaining prize card is visible, can be counted, and can be taken.
- [ ] **Opponent's hand peeks**
  In the landscape layout with the opponent holding 6 cards, 6 card backs are visible at the top edge, each showing only part of the card, and a screen reader announces the hand with its count.
- [ ] **Opponent's name in a corner**
  In the landscape layout, the opponent's name and status appear in a corner of the board, and no bar spans the board's width.
- [ ] **Bigger cards**
  At 2560×1440 with the timeline beside the board, a benched card in the landscape layout is at least 1.6 times the width a benched card had on today's mat at 2560×1440 before this change.
- [ ] **Board fits without scrolling**
  At 2560×1440 and at 3840×2160, the landscape board fits the board area with no scroll bars and nothing cut off, apart from the intended opponent's-hand peek.
- [ ] **Every action still works**
  In the landscape layout, opening each pile, taking a prize, dragging a card from hand to the bench, opening a card's action sheet, and using Draw and End turn all work as they do on today's mat.
- [ ] **Counts beside hands**
  On today's mat and in the landscape layout, each "N in hand" count sits next to its hand, within one card width of the hand's nearest card.
- [ ] **Text scales**
  At 3840×2160, the timeline text and header text are at least 1.5 times their size at 1920×1080. At 1920×1080 and 1440×900 they are the same size as before this change.
- [ ] **Timeline grows**
  At 3840×2160 with the timeline beside the board, the timeline is wider than 480px.
- [ ] **Playback centred**
  During a replay, with the timeline beside the board, below it, or hidden, the playback bar's and caption's horizontal centres are within 2px of the board area's horizontal centre.
- [ ] **Resizing switches layout live**
  Resizing a window from 2560 wide to 1440 wide and back switches from the landscape layout to today's mat and back with no reload. Hiding the timeline at a width where the board area becomes wide enough switches to the landscape layout, and so does moving the timeline from beside the board to below it at such a width.

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

- Switch layouts on the board area's own width (for example with a container query or a measured width), not the window's.
- Prefer one card-size variable for the landscape mat, derived from the board area's width and height the way today's mat derives its size from the viewport, so every zone scales together.
- Scale text with a clamped viewport-relative size.
- Centre the playback overlay on the board area by positioning it within that area, rather than fixed to the window.
- Expect two risks:
  - The board's existing e2e checks assume today's mat at desktop and tablet sizes, so a wider desktop test profile is likely needed to cover the landscape mat.
  - Drag-and-drop targets and the action sheet's placement are likely to need rechecking in the new arrangement.

<!--
  SUCCESS METRICS
  How you will know the feature is working well after delivery. Be specific:
    - Quantitative: "p99 latency < 200ms", "error rate < 0.1%"
    - Behavioural: "users complete the flow without support intervention"
  Format: one bullet point per metric.
  Leave blank if not applicable.
-->
## Success Metrics

- Every existing board e2e check passes on today's mat, and the equivalent checks pass on the landscape mat.
- The player's own 3840-wide monitor shows the whole game, timeline included, without zooming the browser.

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

- New board actions, or changes to what the timeline shows, are out of scope.
- A user setting to choose between the landscape mat and today's mat is out of scope.
