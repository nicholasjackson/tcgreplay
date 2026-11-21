---
tags: [board, webui, css, layout, playwright, testing, phone, tablet, width]
---

# The board's card size follows the measured board area, its width as well as its height, a frame after any resize

The board never scrolls vertically in any layout. The app is a fixed `100dvh` column (header, any banners such as Reveal all or game over, then the body), and `App.svelte` measures `<main>`'s height with a ResizeObserver into `--main-h`. Every layout's `--cw` is written against `--main-h` less its own padding, minus `--fit` (from `fitBoard`, which now runs on today's mat too, not only when docked). Today's mat keeps its old `100vh - 115px` (or `- 140px`, `- 120px - dock`) term inside `min()` as a ceiling, so with no banners it sizes exactly as it was tuned (phone.spec pins that formula), and the measured room binds only when banners or a short screen take more. The landscape mat already sized from `main` with container units.

**The width binds too, on today's mat.** Its widest row is the player block, the bench and the top of the discard. If that row is wider than `<main>`, the bench, which comes later in the DOM, covers the player block's chips, and the Deck chip can no longer be tapped. This happened on a 1024×768 tablet once the timeline column was 362px wide. App.svelte measures `<main>`'s width (`--main-w`) and the wider player block's least width (`--block-w`: its prize row and pile chips, which keep one line). It adds a width term, `--cw-w`, to every today's-mat `--cw`: the bench is about 4.4 card widths plus 72px, and the discard card 0.62 of a width.
- In Side.svelte, the seat row's grid gives the block's column `minmax(min-content, 1fr)`. Not `auto`: the block's `min-width: 0` makes `auto` resolve to zero. The discard's column is at least its card.
- So when room is short, the bench moves off centre rather than covering anything.
- The top-of-discard block is an inline-size container that drops its caption below 140px.
- Shrinking the cards always wins over overlapping.
- mat.spec's "the bench never covers…" checks that the boxes don't intersect, and that `elementFromPoint` at the Deck chip's centre lands in the chip, on every non-phone project.

Two consequences:

- **Tests must poll after a resize or a banner.** `--main-h` updates a frame later, so checking `scrollHeight` straight after `setViewportSize` can see the old size. Use `expect.poll` (see `web/e2e/no-scroll.spec.js` and phone.spec's fit test).
- **When cards hit their 44px touch floor, `fitBoard` keeps growing `--fit`**, and today's mat's `main` padding (`max(0px, 14px - var(--fit) / 4)`) gives way so a busy tablet mat under a banner still fits. `fitBoard` sums fractional row heights (`getBoundingClientRect`), not `offsetHeight`, so a fraction of a pixel over still shrinks rather than leaving a 1px scroll.

**Attached cards.** A Basic Energy draws no card on any layout, only its dot. Off the phone, Tools and Special Energy are tucked behind their Pokémon, and every slot in a Bench row reserves the room of the row's most strips (gotcha tucked-attachments). That room is a real row height, so the card size pays for it. The phone draws no tucked cards. On every layout each attached card is reached through its Pokémon's sheet, whose **Attached** group comes last (`stackMenu` in `web/src/lib/actions.js`); a row opens the card's view (the zoom) with its Move to, To hand and Discard. Browser tests use the fixtures `expectAttached(page, region, title, testInfo)` (a tucked card on the mat, else the board state) and `attached(...)` (with a mouse a tucked card's strip, else the row in the opened sheet) instead of looking for the card inside the region.
