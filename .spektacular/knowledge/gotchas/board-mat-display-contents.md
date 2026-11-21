---
tags: [board, webui, css, layout, playwright, testing]
---

# Today's mat halves and the phone's halves are `display: contents`; measure rows flattened

**Each half of today's mat has no box of its own.** To spread the mat's six rows evenly, as the Studio design does, each player's half (`.side.mat` and its `.field`) is `display: contents`. Its rows are placed in `main` with `order` 1–6. The phone board does the same: `.side.phone` is `display: contents`, and its eight rows (opponent's counters, Bench, Active, the strip, your Active, Bench, counters, then the hand) take `order` 1–8, with a `main::before` as an empty first row so the gap under the header equals the gaps between rows. Anything that measures the board's rows must flatten `display: contents` children, because those report 0 for `offsetHeight` and `getBoundingClientRect`. That includes `fitBoard` in `web/src/App.svelte`, and tests that sum row heights (phone.spec's `overflow` helper) or check that the board scrolls from its top. Region locators and `toBeVisible` on the half's section still work in Playwright.

**Where the timeline's actions live after the design alignment:**
- Beside the board, "Move timeline below the board" is in ☰, because the 380px control row has no room for it.
- In the docked and touch timelines, lines carry no buttons. Tapping or keying a line (`role=button`, `aria-expanded`) reveals Play and Rewind under it (the half headings' `head-btn` style, an icon and one word), keeping the aria-labels "Play from #N" and "Rewind to just after #N" and titles "Play from here" / "Rewind to here", 44px on touch.
- Tests reach these through the `lineButton` helper in `web/e2e/timeline.spec.js`.

**The phone's fit is tight.** On a 390×844 phone the rows sit 13px apart (the design's gaps), with a least gap of 4px (none below 740px tall). `fitBoard` counts that least `row-gap` and the `::before` row, so a new gap must be a `row-gap`, not a margin, or the fit stops adding up. On short phones (375×667, or under the Reveal all banner) the Bench cards sit at their 44px touch floor, so take any new space from inside the rows.
