---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Wide-screen landscape board

## What was built

On wide screens the playmat now switches to a **landscape mat**: whenever the board area beside or above the timeline is at least 1600px wide, on any layout but the phone.
- Both Active Pokémon sit side by side in a middle row with "vs" between them, each edged in its seat's colour and captioned with whose it is. The Stadium is at the left and the turn status and controls at the right.
- Each player's prizes, bench and Deck/Discard/Lost Zone share one row, mirrored across the table: the opponent's piles at the left and prizes at the right, yours the other way round.
- Prizes are an overlapping fan that closes up as prizes are taken. Each remaining prize stays visible and can be clicked.
- The opponent's hand hangs off the top edge of the board, about a third of each card showing, with its count beside it. Their name and status sit small in the top-left corner.
- Cards are sized from the board area itself (CSS container units on a `.board` box inside `<main>`), so four rows fill the space. At 2560×1440 with the timeline beside, a benched card is 194px, against 119.4px on today's mat (1.625×). A longer bench, or a busy board with Energy under benched Pokémon, shrinks the cards a little rather than scrolling. The phone's existing shrink-to-fit was generalised for this.
- The layout switches live, with no reload, as the window resizes or the timeline moves or hides.
- Every pile, prize, drop target, action sheet and turn control works as on today's mat, because the landscape branches reuse the same components and handlers.

Every desktop and tablet board also gets four smaller fixes:
- Each "N in hand" count now sits right after its hand's last card instead of at the board's far edge.
- The playback bar and move caption centre on the board area, not the window, with the timeline beside, below or hidden.
- Header and timeline text grow with the screen above 1920px wide (doubling by 3840px) and are unchanged at 1920px and below.
- The timeline column can grow past 480px on very wide screens (up to 960px).

The phone layout is unchanged. Today's mat is unchanged below 1600px, apart from those fixes.

The browser suite gained a fourth Playwright profile, `wide` (2560×1440), and two spec files: `web/e2e/wide.spec.js` with 16 landscape checks and `web/e2e/desktop.spec.js` for the desktop-wide fixes and the switch. Every spec acceptance criterion has a check that passes on the `wide` profile. The full suite passes on phone, tablet, desktop and wide: 310 passed, 146 profile-scoped skips, 0 failed. A manual test plan covers the real 3840-wide monitor, a visual review against mockup C, and a replay with the bar over the landscape board.

## Why it matters

On a big monitor, today's mat stacks seven rows, so its cards are sized by the screen's height and sit in a narrow column of empty felt. Players reviewing or playing on a large screen now get cards at least 1.6 times larger, read every card and count at a glance, and see the whole game, timeline included, without zooming the browser. The hand-count, playback and text fixes help every desktop and tablet screen.

## Deviations from the plan

- **The 1.6× card-size criterion moved** from "Landscape mat rows and sizing" to "Opponent's hand peeks from the top edge" (orchestrator decision). Height, not width, limits the card size at 2560×1440, and 1.6× only fits once the opponent's hand stops taking a row. The criterion is met: 194px against the 191px needed.
- **Implementation worktree created by hand.** `implement new` did not create a git worktree. With the orchestrator's approval, the work was done in a manual worktree on branch `spek/20261104090653-7373a393-wide-screen-landscape-board` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay-wide-board`), which the user merges by hand.
- **Busy boards fit by shrinking.** `fitBoard` now counts only in-flow rows and also runs on the landscape board, so Energy and Tools under benched Pokémon shrink the cards slightly instead of scrolling.
- **Two drag fixes outside the planned file list.**
  - The drag ghost takes the card width in pixels (`web/src/lib/pointer.svelte.js`), because container units mean nothing outside the board.
  - The "Drop here to play" area floats under the controls on the landscape mat, so the row doesn't resize mid-drag.
- **Opponent's count shown.** On the landscape mat the opponent's "N in hand" is visible beside the peeking cards rather than visually hidden, so "Counts beside hands" holds there too.
- **Smaller adjustments:**
  - The hand count's margin is 0.7 card widths, to clear the rotated last card.
  - The ☰ button and the menu's check items scale with the header text.
  - A shared `benchSlots()` store helper gives the bench length to both Side and App.
