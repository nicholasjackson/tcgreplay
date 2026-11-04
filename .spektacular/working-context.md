# Working context: wide-screen-landscape-board

## Problem
On large screens (the user's monitor is about 3840 wide; reproduced at 2560×1340
and 3840×2050), the desktop board is sized by height: card width comes from
the viewport height divided by the roughly 11 card heights the board stacks
(opponent's hand, opponent's bench, opponent's active, strip, my active, my
bench, my hand). The board fills about half its column, leaving wide empty
felt on both sides. Other measured problems:
- The "6 in hand" / "8 in hand" counts are pinned to the far right edge of
  the board area, far from their hands.
- Header, timeline and pile labels stay at 12–13px while the cards scale.
- The timeline width is capped at 480px.
- The playback overlay ("Claude is playing 2/2 · Skip to end · Speed") and
  its caption sit fixed at the top centre, covering the opponent's name,
  status and hand. In the user's words: "we need to check the positions".

User's words: "I would like to fix the layout a bit on large screens too".

## Mockups
Canvas: https://claude.ai/artifact/HK5VjHeETLarQsFE5d5p5n (four artboards at
2560×1440, drawn at 1600×900)
- Main = today
- A = same mat, tidied and scaled
- B = opponent's hand folded into a name bar (cards about 40% bigger)
- C = landscape mat (cards nearly twice today's size)

The user chose C: "I think my preference is on C but I would actually put my
opponents hand off the screen rather into the tile bar, just show the top of
the card so I can roughly see how many cards they have". After the revision:
"Yes this is a better layout."

Mockup C as revised:
- The opponent's hand hangs off the top edge of the board; only about 40px of
  each card back shows.
- The opponent's name and status are small, top-left.
- Opponent row: piles (Lost, Discard, Deck), then the bench of 5, then the
  prizes as a fanned, overlapping row.
- Middle row: Stadium, then Alice's Active, "vs", Bob's Active, then the turn
  pill and controls (Draw, End turn).
- Player row: the prizes as a fanned row, then the bench, then the piles
  (Deck, Discard, Lost).
- The player's hand along the bottom, with "8 in hand" beside it, and a name
  bar along the bottom.
- The timeline is about 400px at the 1600 scale (wider than today), with
  larger text.

## User decisions
- Use layout C only on wide screens: when the board area beside the timeline
  is about 1600px or wider. Tablets and smaller laptops keep today's mat;
  phones are unchanged.
- The playback bar and caption must never cover the opponent's hand, name or
  status.
- Capture as a spec, with mockup C stored as its design document (spek-design).
- Already done separately: the timeline dock icon (it moves the timeline
  between beside and below) replaced the ☰ "Move timeline" item. The rewind
  button is hidden on the last timeline entry.

## Related
- Design source "design" holds mobile-board/design.md and
  mobile-board/mockups.html (the touch-friendly board spec).
- The beside position for the timeline is on the right of the board.

## Interview answers (2026-11-04)
- The playback bar may cover the board but must be centred over it ("I don't
  mind them covering, they should just be centered"). This also applies on
  today's mat.
- Also on today's mat: hand counts beside their hands, and text that scales
  on big screens.
- The user reviews the final spec as a whole (same as the cache spec): draft
  the sections without per-section stops.

## Progress
- Every section is drafted and the spec is staged at
  .spektacular/tmp/spec_template.md. A fresh-eyes review is running.
- The design document draft is at .spektacular/tmp/wide-board-design.md
  (target: source `design`, path `wide-board/design.md`, authored with
  --spec). Waiting on the user's explicit OK before `design author`, then
  `design ref add`, then `spec file write`. Constraints already point at
  wide-board/design.md.
- Layout review: 18 findings, all applied: the switch at exactly 1600px; text
  scales only above 1920px; cards at least 1.6x; new criteria for the corner
  name and for moving the timeline; a constraint that today's mat is
  otherwise unchanged; duplicate metrics and non-goals dropped.
- Hidden-info bug report (~/Downloads/hidden-info-bug.md): bug 1 is FIXED.
  In play mode, MCP write results and new_game deltas are redacted for
  ClaudeSeat via Game.DeltaFor plus Server.redact; there is a test in
  server_test. Still open, waiting on the user: bug 2 (the opponent runs as a
  fresh sub-agent; a guide change), bug 3 (a revealed card stays face-up in
  the hand), automatic mulligans, and the UX notes (heal all, undo branches,
  game-over detection, prize reminders).

## Plan (2026-11-04)
The plan for 20261104090653-7373a393-wide-screen-landscape-board is committed
(plan, context, research), with 2 milestones and 7 agent tasks. It is at the
walkthrough step, waiting on the user's sign-off.
