---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Research: 20261104090653-7373a393-wide-screen-landscape-board

## Alternatives considered and rejected

- **CSS-only rearrangement of today's components** (a grid on `main`, with `display: contents` on each side's `.field` so the Active stacks can be placed in a shared middle row). Rejected: each Active is rendered inside its player's `Side` (`web/src/components/Side.svelte:89-99`, `.field` with `.active-row` and `.bench`), and the opponent's half is mirrored with `column-reverse` (`Side.svelte:161`, `:189`). Moving Actives across component boundaries with CSS alone means flattening three nested levels and re-ordering by grid areas, which is fragile with the existing mirrored ordering and the bench `flex-wrap`.
- **A separate landscape board component tree** (a new `LandscapeBoard.svelte` that reimplements piles, prizes and bench). Rejected: it would duplicate the pile, prize and deck behaviour in `Side.svelte:100-139` (drop zones, sheets, pile modal, `take_prize`), which the spec requires to work identically ("Nothing is lost").
- **Switching on window width with a media query.** Rejected: the spec requires switching on the board area's width (timeline beside, below or hidden). The window width alone can't tell those apart; `App.svelte:50-51` shows the timeline beside/below state is app state, not a media feature.

## Chosen approach — evidence

- A per-layout branch inside the existing components is already the house pattern. `Side.svelte:61` (`{#if app.layout === 'phone'}`) renders a different phone markup over the same data and actions, and `Strip.svelte` and `Hand.svelte` take `app.layout === 'phone'` classes. A `wide` flag can follow the same shape: `Side` renders its row without the Active, and `Strip` renders both Actives between the Stadium and the controls.
- The layout state lives in `web/src/lib/store.svelte.js:39-76` (`app.layout` from media queries plus `landscapePhone`). A measured `app.wide` flag sits beside it, set from `main`'s width by a `ResizeObserver`, as `App.svelte:27-48` (`fitBoard`) already does for the phone.
- Card size is one CSS variable, `--cw`, with `--fs` derived from it (`web/src/app.css:5-6`), overridden per layout in `App.svelte:238-257`. A landscape `--cw` from the board area's measured width and height fits that model.
- The playback overlay is `position: fixed; left: 50%` of the window (`App.svelte:401-411`), so it is centred on the window, not the board area. That is the off-centre bar in the user's screenshot.
- The hand count is `position: absolute; right: 12px` inside the full-width `.hand` (`Hand.svelte:145-151`), hence its drift to the board's edge.
- The timeline column is `clamp(320px, 18vw, 480px)` (`App.svelte:366`, `.body.panel`), so it is capped at 480px.
- The header and timeline fonts are fixed: 13px buttons and 14px body (`app.css`), and 12px timeline (`Timeline.svelte` `.timeline { font-size: 12px }`).

## Files examined

- web/src/App.svelte:17-51 — layout flags (phone, showTimeline, bottom), the phone `fitBoard` ResizeObserver pattern
- web/src/App.svelte:165-213 — body/main/aside markup: the board order is seat name, opponent's Hand (small), opponent's Side (mirrored), Strip, player's Side, player's Hand, seat name
- web/src/App.svelte:236-257 — `.app.wide` and `.app.panel-bottom` `--cw` overrides
- web/src/App.svelte:366-380 — `.body.panel` grid with the timeline width clamp; `.body.bottom` aside
- web/src/App.svelte:401-418 — `.overlay` fixed and window-centred; caption max-width 80vw
- web/src/app.css:1-27 — root `--cw`/`--fs` formula (about 11.1 card widths tall, 10 wide beside a 360px timeline)
- web/src/components/Side.svelte:1-240 — prizes as a 2-column grid of `.prize` slots that keep taken slots (`.taken` opacity 0.35); field (Active + bench, mirrored); piles column (Deck/Discard/Lost, mirrored); phone branch
- web/src/components/Strip.svelte:1-60 — Stadium drop zone and card, turn status, coin, buttons (setup: Mulligan / I go first; play: Draw/End turn etc.)
- web/src/components/Hand.svelte:40-152 — hand fan, `small` for the opponent, count positioned absolute at the right
- web/src/components/PlaybackBar.svelte:1-20 — the bar shows for a replay or held moves; rendered inside App's `.overlay`
- web/src/lib/store.svelte.js:39-76 — `app.layout` ('phone' | 'tablet' | 'desktop') from matchMedia; `landscapePhone`
- web/playwright.config.js:21-34 — projects phone 390×844, tablet 1024×768, desktop 1440×900; no wide profile
- web/e2e/fixtures.js:26-28, 192-194 — `isTouch`, `isPhone` project helpers
- web/e2e/phone.spec.js:183-237 — desktop/tablet mat assertions (aside beside main, `--cw` probe formula `(100vw - 360px) / 10`) that a wide profile must skip or adapt

## External references

- CSS container queries and units (`container-type: size`, `cqw`/`cqh`), MDN — they let `--cw` be derived from the board area's size without JS. Container units resolve against the nearest ancestor container, so the variable must be set on a descendant of `main`.
- ResizeObserver, MDN — the board-area width that decides the layout switch, since component markup (not just CSS) changes.

## Prior plans / specs consulted

- spec 20261102110545-5c84659c-touch-friendly-board and design `mobile-board/design.md` — established the per-layout branch pattern (`app.layout`), Pointer Events drag with `data-drop` zones found by `elementFromPoint` (so moved zones keep working), 44px tap targets under `pointer: coarse`, and that the phone layout is settled and separate.

## Open assumptions

- Tablets can reach the landscape layout only when their board area is at least 1600px wide (no current iPad is). The switch uses the board-area width on any non-phone layout.
- The board test server's fixed position (`web/internal/webui/webtest`) has enough cards in play (a bench, prizes, Stadium, hand) to drive the landscape checks. If not, tests set up state with `act()` as other specs do.
- Taken prizes are not rendered in the landscape fan (it closes up), per the design. Today's mat keeps its dashed taken slots.
- `docs`: no user documentation describes the board layout, so none needs updating.

## Drafting assumptions

### Layout branch over a separate tree (discovery)
- **Decision**: Add a `wide` branch inside Side/Strip/Hand, as the phone layout does, rather than a new board component tree or CSS-only rearrangement.
- **Rationale**: It reuses every pile, prize and drop behaviour, and it matches the existing `app.layout === 'phone'` pattern.
- **Rejected**: A CSS-only grid with `display: contents` (fragile with the mirrored nesting); a separate component tree (duplicates behaviour).

### Chosen direction: measured wide flag with per-component branches (architecture)
- **Decision**: An `app.wide` flag from a ResizeObserver on `<main>` (non-phone, width ≥1600px). Side, Strip and Hand get `wide` branches; the wide `--cw` comes from container units on a wrapper inside `<main>`.
- **Rationale**: It switches on the board area, as the spec requires, reuses every handler, and follows the phone layout's pattern. Container units keep sizing in CSS.
- **Rejected**: A CSS-only grid rearrangement (fragile); a separate component tree (duplication); media queries (can't see the timeline state).

### Playback centring by timeline-width offset (architecture)
- **Decision**: Keep the overlay `position: fixed` and offset its `left` by half the timeline column's width when the timeline is beside the board.
- **Rationale**: Moving it inside `<main>` (which scrolls) would move it with the board's scroll and change its stacking. The offset is a small CSS change.
- **Rejected**: Placing the overlay inside `<main>`, or measuring `<main>` in JS on every layout change.

### Text scaling by max(px, vw) (architecture)
- **Decision**: Header and timeline text use `max(today's px, px/1920 × 100vw)`, for example `max(12px, 0.625vw)`, so it is unchanged at 1920px and below and doubles by 3840px.
- **Rationale**: It meets "at least 1.5× at 3840 vs 1920" and "unchanged at 1920 and below" exactly.
- **Rejected**: `clamp` with a lower vw slope (doesn't reach 1.5×); a rem-based root font change (would move today's mat).

### Desktop-wide fixes ship first (milestones)
- **Decision**: Milestone 1 delivers the hand counts, playback centring, text scaling and timeline width on today's mat. Milestone 2 adds the landscape mat.
- **Rationale**: The fixes are independently useful (the user's screenshot showed the off-centre playback bar on today's mat) and smaller. Landing them first means the landscape branch builds on centred, scaled chrome.
- **Rejected**: One milestone (too large to validate in one step); landscape first (it would carry the old count and overlay positions into new markup).

### Wide flag exposed as a data attribute for tests (tasks)
- **Decision**: `.app` carries `data-wide` and a `landscape` class, so tests can assert the switch before the landscape markup exists.
- **Rationale**: It lets the switch and profile task land and be verified on its own.
- **Rejected**: Testing the switch only through landscape geometry (it couples two tasks).

### Taken prizes hidden in the landscape fan only (tasks)
- **Decision**: The landscape fan renders only remaining prizes, while today's 2×3 grid keeps dashed taken slots.
- **Rationale**: The design says the fan closes up, and the spec constrains today's mat to stay unchanged.
- **Rejected**: Changing today's grid too (outside the agreed desktop-wide changes).

## Rehydration cues

- `spektacular spec file read 20261104090653-7373a393-wide-screen-landscape-board`
- `spektacular design read --data '{"source":"design","path":"wide-board/design.md"}'`
- `spektacular design read --data '{"source":"design","path":"mobile-board/design.md"}'`
- Mockup canvas: https://claude.ai/artifact/HK5VjHeETLarQsFE5d5p5n (artboard OptionC)
- Re-read web/src/App.svelte, web/src/components/{Side,Strip,Hand,PlaybackBar,Timeline}.svelte, web/src/app.css, web/playwright.config.js, web/e2e/fixtures.js
- Knowledge: gotchas/playwright-touch-tests.md (Chromium-only profiles, CDP touch drags, `isPhone`)
