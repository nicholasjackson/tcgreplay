---
tags: [webui, playwright, testing, touch, drag]
---

# Playwright touch tests: Chromium only, touch through CDP

The board's browser suite (`web/e2e`, run by `make test`) has three sharp edges.

**WebKit does not run on the developer's machine.** Playwright's WebKit build links against Ubuntu library versions (`libicu74`, an older `libxml2`, `libflite1`, `libbacktrace`). On Arch it downloads, then fails to launch with "Host system is missing dependencies". `sudo npx playwright install-deps` only works on Debian/Ubuntu. So the `phone` and `tablet` projects in `web/playwright.config.js` take the iPhone 13 and iPad sizes but set `browserName: 'chromium'`, using Chromium's touch emulation (`hasTouch`, `isMobile`, `pointer: coarse`, no hover). Only Chromium needs installing (`cd web && npx playwright install chromium`). Real iOS Safari behaviour, such as the callout, text selection and scroll-versus-drag, is a manual check on a device.

**Playwright's `touchscreen` only taps, so drags use CDP.** Synthetic `PointerEvent`s can't scroll natively and can't be captured. Because every profile is Chromium, `touchStart` in `web/e2e/fixtures.js` sends real touch through `Input.dispatchTouchEvent` on a CDP session instead. That input respects `touch-action`, scrolls natively and gets implicit pointer capture, as on a device. Use `touchStart`/`mouseStart` handles (`move`, `end`) to test a drag still in progress. A drag first lifts the card (the `LIFT` step): in the hand only an upward move lifts a card, and the play area exists only once a drag has started.

**Pointer capture on an element keeps it `:hover`.** If `lib/pointer.svelte.js` captured the dragged card, its desktop hover enlargement would stay applied for the whole drag. `elementFromPoint` would then hit the enlarged card, and a drop meant for the bench would land on the hand. So the document captures the pointer, the drag source has `pointer-events: none`, and hover enlargement is gated by `body:not(.dragging)`. Keep any new hover effect behind the same gate.


**The phone profile has a different board.** At 390×844 the board is the compact phone layout (`app.layout === 'phone'`):
- Piles and prizes are counter chips, so use `pile()` and `pileLabel()` from `fixtures.js`, not the side piles.
- The timeline is a closed sheet behind ☰, so `timelineLines` reads the board's log from its state when no `.line` is rendered.
- The board is about 966px tall, so scroll a target into view before reading its position.
- A no-sideways-scroll check must look at `main` as well as the document.


**Real iPhones behave differently from emulated touch.** Three bugs passed every Chromium test and only showed on a phone (`lib/pointer.svelte.js`):
- **The ghost must be positioned inline.** A global `.drag-ghost { position: fixed }` loses to the card's scoped `.card { position: relative }`, so the ghost sat off-screen below the app. A test that checks the ghost is "visible" passes regardless; check `toBeInViewport()` and its position against the pointer.
- **Don't mix the `scale` property with `transform: translate(...)`.** `scale` applies outside `transform` and scales the offset too, carrying the ghost away from the pointer. Position the ghost with the `translate` property, which applies before `scale`.
- **Finger drags run on Touch Events, not Pointer Events.** iOS Safari cancels a touch's pointer events part way through a drag (it takes the gesture for a scroll, or the touched card fades), so the card dropped straight away. `touchmove`/`touchend` are not cancelled once the page claims them with `preventDefault()`, after about 4px: always on the mat, upward in the hand, sideways in a pile view. Mouse and pen stay on Pointer Events, with capture on the document.
