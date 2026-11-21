---
tags: [board, webui, layout, design, playwright, mobile]
---

# The board's main view never scrolls vertically, on any screen

The maintainer's rule: on a phone, a tablet or iPad, a desktop, the narrow board with the timeline docked, and the wide landscape mat, the board's window never scrolls vertically. Everything fits the screen, and the cards shrink to make room.

That includes every strip that can appear above the board: the Reveal all banner ("REVEAL VIEW: both hands, prizes and decks are face up…"), the game-over banner, notices and held-move bars. A banner takes its own row, and the board gives up that height. A banner never pushes the hand off the bottom edge, and never covers the board.

So the app is a fixed-height column (header, banners, then the body filling what is left). The card size (`--cw`, `fitBoard`) follows the real space remaining, not a formula with a fixed header height. Any new full-width strip must be checked with Reveal all on and the game over, at phone, tablet and desktop sizes, and the no-scroll browser test covers it. Pile, deck and hand views open as modals or sheets and may scroll inside themselves.

## On mobile, nothing scrolls sideways except the hand

On a phone (and any narrow touch layout), the only things that may scroll horizontally are the row of cards in your hand, and the Bench when a Stadium lets it grow past five (the maintainer accepted that, 2026-11-21). Every other row wraps or reflows within the width: in the Pokémon sheet, the attacks (stacked one per row when two don't fit, with long names wrapping), the Conditions chips and the Other actions wrap onto further lines or sit in a grid, keeping 44px touch targets. The maintainer rejected sideways-scrolling chip rows in the sheet (2026-11-21). A sheet may still scroll vertically inside itself.


**Nothing scrolls horizontally on mobile except the hand, and the Bench when it grows past five** (a Stadium such as Area Zero Underdepths). The maintainer's rule from the attachments design. On the phone sheet (`ActionSheet.svelte`, phone mode) every row wraps instead of scrolling sideways: buttons share a row two to a line, three or more attacks stack one per line, a long label wraps, the Conditions are a three-column grid and Other is a two-column grid. Sheets and views may scroll vertically inside themselves. sheet.spec's phone test fails if any element in the open sheet has `scrollWidth > clientWidth`.
