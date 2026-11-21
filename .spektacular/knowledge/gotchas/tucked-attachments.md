---
tags: [board, webui, css, layout, touch, bench, playwright]
---

# Tucked Tools and Special Energy: level Bench rows, mouse-only strips, and tests that look past the old attachment cards

Off the phone, a Pokémon's Tools and Special Energy are drawn tucked behind its card, a strip of each showing above it (`Stack.svelte`, the `.tucks`/`.tuck` boxes). A Basic Energy draws no card at all, only its dot. The phone draws no tucked cards. The rules that are easy to break:

- **At most two strips (`TUCK_CAP` in `actions.js`), with no "+n".** The first strip is 0.42 × the Pokémon card's width tall, the second a 0.2 band above it, and each tucked card is 84% of that width, centred. The Pokémon's menu lists every attached card in its **Attached** section, last, on every layout. That list is the only way to a Basic Energy's card. Each row opens the card's view (the zoom), whose foot offers `attachmentMenu`'s Move to, To hand and Discard (`Zoom.svelte` shows them for any zoomed card `attachmentOf` finds attached).
- **A Bench row stays level.** `Side.svelte` passes every Bench slot `reserve` = the most strips any slot in that row shows. Each slot then pads its `.pile` (or, when empty, its box) by `--tuck-r`, so every card top lines up, and a row with nothing tucked takes no extra height. The Active reserves for its own strips. The room is part of the measured rows, so `fitBoard` shrinks the cards to pay for it. Put new height there too, never as an overflow hanging outside the row (gotchas landscape-mat-fit-stall and board-card-size-follows-board-area).
- **Strips are targets only for a mouse.** Under `(pointer: coarse)` the `.tucks` box has `pointer-events: none`. A tap on a strip falls through to the `.pile`'s padding, and the pile's `onclick` (only when the event's target is the pile itself, the Pokémon has strips and the pointer isn't a mouse) opens the Pokémon's sheet. touch.spec's `tooSmall` skips elements with `pointer-events: none` for the same reason, because they are not targets. With a mouse, a click on a strip opens its view, a right-click its `attachmentMenu`, and it drags as a card. A tap test can't use `locator.tap()` on a strip (Playwright sees the pile intercept it), so use `page.touchscreen.tap(x, y)` at the strip's centre.
- **Tests:**
  - `fixtures.js` `expectAttached` checks a tucked card on the mat, but checks a Basic Energy (and anything on a phone) in the board state.
  - `attached()` returns what opens the card's view: with a mouse, the strip (`tucked()`); otherwise the card's row in the Pokémon's opened sheet.
  - `.att` and `.attachments` no longer exist.
  - `web/e2e/attach.spec.js` holds the behaviour tests, and mat.spec runs the bench-never-covers check with strips as well.
