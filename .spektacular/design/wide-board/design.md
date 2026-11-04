---
created_date: "2026-11-04"
document_status: draft
spec: 20261104090653-7373a393-wide-screen-landscape-board
specs:
    - 20261104090653-7373a393-wide-screen-landscape-board
---

# Wide-screen board: the landscape mat

The board's layout for wide desktop screens, chosen from the "Large-screen board
layouts" canvas (https://claude.ai/artifact/HK5VjHeETLarQsFE5d5p5n, artboard
"C · Landscape mat"). Today's mat stacks seven rows (opponent's hand, opponent's
bench, opponent's active, strip, player's active, player's bench, player's
hand), so its cards are sized by the screen's height. The landscape mat stacks
four rows, so cards can be much larger: at least 1.6 times today's width at 2560×1440.

## When it applies

- Only when the board area (the space beside or above the timeline) is
  1600px wide or more. The board switches the moment it crosses that
  width: window resize, moving the timeline, hiding it.
- Below that width, desktop and tablet keep today's mat. Phones keep the
  phone layout (mobile-board/design.md).

## Layout, top to bottom

```
┌──────────────────────────────────────────────────────────────────────┐
│ Bob · p2  (Opponent is waiting)    ▔▔▔▔▔▔▔▔  ← opponent's hand peeks  │
│                                                                      │
│  [Lost][Discard][Deck]   [B1][B2][B3][B4][B5]   [prize fan ▭▭▭▭▭▭]   │  opponent's row
│                                                                      │
│  [Stadium]   [Alice's Active]  vs  [Bob's Active]   Turn 1 · Draw/End │  middle row
│                                                                      │
│  [prize fan ▭▭▭▭▭▭]   [B1][B2][B3][B4][B5]   [Deck][Discard][Lost]   │  player's row
│                                                                      │
│              [ player's hand, overlapping  ]  8 in hand              │
│                           Alice · p1                                 │
└──────────────────────────────────────────────────────────────────────┘
```

- **Opponent's hand.** It hangs off the top edge of the board, card backs
  overlapping, with only the bottom strip of each card on screen (about a
  third of a card). The player can count the cards roughly without the hand
  taking a row. Screen readers still get the hand and its count.
- **Opponent's name and status.** Small, in the board's top-left corner,
  clear of the hand. No title bar across the board.
- **The two player rows mirror each other**, the way opposite seats do on a
  real mat: the opponent's piles at the left end and prizes at the right; the
  player's prizes at the left and piles at the right. Each bench sits in the
  middle of its row.
- **Prizes** form one overlapping fan per player, not a 2×3 grid. Each
  remaining prize stays visible, countable and takeable. Taken prizes close
  up, so the fan shrinks rather than leaving gaps.
- **Middle row.** The Stadium sits at the left. Then the player's Active and
  the opponent's Active side by side, each labelled with whose it is (border
  colour and caption), with "vs" between them. The turn pill and turn
  controls sit at the right. Attached energy and tools sit under each Active
  as on today's mat.
- **Player's hand** runs along the bottom, overlapping as today, with
  "N in hand" right beside it. The player's name sits centred below.

## Sizing

- One card size for the bench and both Actives, which are slightly larger.
  Piles and prize cards are smaller. That size is the largest that lets all
  four rows fit the board area's height and the widest row fit its width.
- The widest row is a player row at its largest bench (8, with a
  bench-expanding Stadium). As the bench grows, cards shrink to keep the row
  on screen.
- A big opponent hand overlaps more tightly rather than growing wider than
  the bench row.
- With the timeline below the board, the board area is wide but short, so
  height usually limits the card size.

## Everywhere on desktop, not just the landscape mat

- **Hand counts** sit beside their hands, never pinned to the board area's
  edge.
- **Text** (header, timeline, pile and seat labels) grows with the screen on
  screens wider than 1920px instead of staying at 12–13px; at 1920px and
  below it stays at today's size.
- **The timeline** may grow wider than today's 480px on big screens, in step
  with its text.
- **The playback bar and move caption** stay at the top, horizontally centred
  on the board area (not the window), with the timeline beside, below or
  hidden. They may cover the board.

## Not part of this design

- The phone layout
- New board actions
- Changes to the timeline's content
