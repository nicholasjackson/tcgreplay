---
created_date: "2026-11-04"
document_status: draft
project: tcgreplay
spec: 20261104090653-7373a393-wide-screen-landscape-board
plan: 20261104090653-7373a393-wide-screen-landscape-board
---

# Wide-screen landscape board

On a big monitor the playmat now becomes a landscape mat. Both Active Pokémon face each other across the middle, each player's prizes, bench and piles share one row, and the opponent's hand peeks in from the top edge, so cards come out at least 1.6 times larger than before. On every desktop and tablet, hand counts now sit beside their hands, the replay controls centre over the board, and the header and timeline text grow on screens wider than 1920px. Phones and smaller screens keep their current layouts.

> Derived from project tcgreplay (Spektacular), spec/plan 20261104090653-7373a393-wide-screen-landscape-board. See the project-level record for the full feature.

## What changed in this repo

- **Landscape mat** (`web/src/App.svelte`, `web/src/components/Side.svelte`, `web/src/components/Strip.svelte`, `web/src/components/Hand.svelte`, `web/src/lib/store.svelte.js`):
  - A measured `app.wide` flag switches the board when the board area is at least 1600px wide.
  - Side renders one mirrored row: prize fan, bench, piles.
  - Strip renders the Stadium, both captioned Active Spots with "vs", and the turn controls.
  - The opponent's hand peeks from the top edge, and their name sits in the corner.
  - Cards are sized from the board area with container units.
  - A busy board shrinks slightly to fit instead of scrolling.
- **Desktop-wide fixes:**
  - Hand counts follow their hand's last card (`Hand.svelte`).
  - The playback overlay centres on the board area through a `--panel-w` offset (`App.svelte`).
  - Header and timeline text use `max(<today's px>, <vw>)` (`App.svelte`, `Timeline.svelte`).
  - The timeline column can reach 960px.
- **Drag:**
  - The drag ghost takes the card width in pixels (`web/src/lib/pointer.svelte.js`).
  - The play area floats under the controls on the landscape mat.
- **Tests:**
  - A new `wide` Playwright profile at 2560×1440 (`web/playwright.config.js`, `isWide` in `web/e2e/fixtures.js`).
  - New `web/e2e/wide.spec.js` and `web/e2e/desktop.spec.js`.
  - Hand-count checks and wide-aware mat checks in `web/e2e/phone.spec.js`.

## Why

Today's mat sizes its cards by the screen's height, so on a wide monitor they sit small in a column of empty felt. The landscape mat puts that space to use, and the smaller fixes make the chrome read well at any desktop size.
