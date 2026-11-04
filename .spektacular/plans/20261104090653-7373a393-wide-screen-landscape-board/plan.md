---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Plan: 20261104090653-7373a393-wide-screen-landscape-board

<!-- Metadata -->
<!-- Created: 2026-11-04T09:28:55Z -->
<!-- Commit: be81383 -->
<!-- Branch: main -->
<!-- Repository: tcgreplay (/home/nicj/code/github.com/nicholasjackson/tcgreplay) -->

## Overview

On wide desktop screens (a board area 1600px wide or more), the board switches to a landscape mat. Both Active Pokémon sit side by side in the middle, each player's bench, fanned prizes and piles share one row, and the opponent's hand peeks from the top edge, so cards come out at least 1.6 times today's width. Alongside it, every desktop and tablet board gets hand counts beside their hands, playback controls centred over the board, and text and a timeline that grow on big screens. Players reviewing or playing on a big monitor can read every card at a glance; smaller screens and the phone keep today's layout.

## Conventions

No project conventions apply to this feature. The only always-applied convention (guide text and packaging in `internal/guides`) covers the assistant guides, which this work does not touch.

## Architecture & Design Decisions

All changes land in the `tcgreplay` repo's board UI (`web/src`) and its browser suite (`web/e2e`, `web/playwright.config.js`). The landscape mat follows the settled design `wide-board/design.md` (source `design`). This plan only decides how to build that shape inside the existing Svelte board.

**A measured `wide` flag, branched in the existing components.** A new `app.wide` flag in `web/src/lib/store.svelte.js` is true when the layout is not `phone` and the board area (`<main>`) is at least 1600px wide. `App.svelte` measures `<main>` with a `ResizeObserver`, as `fitBoard` already does for the phone. Because it measures the board area itself, resizing, moving the timeline and hiding it all flip the flag with no reload. `Side.svelte`, `Strip.svelte` and `Hand.svelte` each gain a `wide` branch, as they already have a `phone` branch:
- `Side` renders one row: piles, then bench, then a fanned prize row, mirrored for the opponent. It does not render the Active.
- `Strip` renders the Stadium, the player's Active, "vs", the opponent's Active, then the turn controls.
- The opponent's `Hand` is pinned to the top edge of the board and overlaps it, so only a strip of each card shows.

Every pile, prize, drop zone and sheet keeps its current handler, because the branches reuse the same `Stack`, `Card`, `dropZone` and `act` calls. That is what makes "Nothing is lost" cheap to keep true. Drop targets keep working wherever they move, because drags find zones by `data-drop` and `elementFromPoint` (`mobile-board/design.md`).

**One card size from the board area's own dimensions.** In the wide layout, `--cw` is set on a wrapper inside `<main>` from container units (`main` gets `container-type: size`). It is the smaller of what fits four rows in the height and the widest row (8 bench slots plus the prize fan and piles) in the width, so every zone scales together and nothing scrolls. Today's mat keeps its existing `--cw` formulas untouched. Prize and pile cards stay fractions of `--cw`, as today.

**Desktop-wide fixes are small, targeted CSS changes that leave today's mat otherwise unchanged:**
- **Hand counts:** they become a flex item after the hand's last card, instead of `position: absolute; right: 12px`.
- **UI text:** header and timeline text use `max(<today's px>, <vw>)`, so it is exactly today's size at 1920px and below and grows above.
- **Timeline width:** the `clamp` cap rises from 480px, so the column only widens above about 2670px.
- **Playback overlay:** it stays `position: fixed`, but its centre becomes the board area's centre. `App.svelte` exposes the timeline column's width as a variable when the timeline is beside the board (0 when it is below or hidden), and the overlay's `left` subtracts half of it.

**Why this beats the alternatives:**
- A CSS-only rearrangement would have to flatten three nested component levels and re-order the mirrored halves by grid areas.
- A separate landscape component tree would duplicate the pile, prize and deck behaviour.
- A window-width media query can't tell the timeline beside, below or hidden apart.

See `research.md#alternatives-considered-and-rejected`. The browser suite gains a `wide` Playwright project at 2560×1440, which runs the landscape checks. Existing tests that assert today's mat geometry skip on it, the way they skip on the phone profile today.

## Component Breakdown

- **Board layout state (store), changed.** It owns the `wide` flag beside the existing `layout` and `landscapePhone`. `wide` is true when the layout is not phone and the board area is at least 1600px wide. Every board component reads it to pick its markup, as they read `layout` for the phone today.
- **App shell, changed.** It measures the board area and sets `wide`, applies the landscape card size (container-based `--cw`) in place of today's formulas when wide, and lays out the board's rows in the landscape order:
  - the opponent's hand peeking from the top edge;
  - the opponent's name and status in the top-left corner;
  - the opponent's row, the middle strip and the player's row;
  - the player's hand.

  It also exposes the timeline column's width so the playback overlay centres on the board area, scales header text on big screens, and raises the timeline's width cap.
- **Side (one player's half), changed.** Today it owns prizes, the Active with the bench, and piles. In the landscape branch it owns one row of piles, bench and a fanned prize row that closes up as prizes are taken, mirrored for the opponent. It hands the Active to the strip. Every pile, prize and drop behaviour is shared with today's mat.
- **Strip (the midline), changed.** Today it owns the Stadium and turn controls. In the landscape branch it also owns both Active Spots, side by side and each labelled with whose it is, with "vs" between them. It renders them through the same Stack component the sides use, so damage, attachments, drops and sheets behave the same.
- **Hand, changed.** On every non-phone layout it places "N in hand" right after the hand's cards. In the landscape branch the opponent's (small) hand is pinned to the board's top edge with only a strip of each card showing, and keeps its accessible list and count.
- **Timeline, changed.** Its text scales on screens wider than 1920px. Content and behaviour are unchanged.
- **Playback overlay (bar and caption), changed position only.** It is centred on the board area rather than the window, with the timeline beside, below or hidden.
- **Browser test suite, changed.** A new `wide` desktop profile at 2560×1440 runs the landscape checks. Existing checks that assert today's mat geometry skip on it, and new checks cover the spec's acceptance criteria.

## Data Structures & Interfaces

No server, wire-format or persisted data changes: the board's snapshot and actions are unchanged. The new contracts are all client-side:

- **`app.wide: boolean`** (board layout state). It is true when `app.layout !== 'phone'` and the measured board area is 1600px wide or more. App sets it and every board component reads it. It sits beside the existing `app.layout` and `app.landscapePhone`.

  ```js
  app = { ..., layout: 'phone' | 'tablet' | 'desktop', landscapePhone: false, wide: false }
  ```

- **CSS custom properties** (the contract between App and the board components):
  - `--cw` (card width): in the wide layout it comes from the board area's container size. Today's formulas are unchanged elsewhere. `--fs` keeps deriving from it.
  - `--panel-w`: the timeline column's width when it sits beside the board, otherwise `0px`. The playback overlay uses it to centre on the board area.
  - `--ui-scale` text sizes: header and timeline fonts are expressed as `max(<today's px>, <vw equivalent>)`, so they are unchanged at 1920px and below.

- **Component props:** no new props. Side, Strip and Hand read `app.wide` directly, as they read `app.layout`. Strip renders both Active Spots through the existing `Stack` component with today's `seat`, `slot` and `label` props (`slot="active"`, `label="<seat> Active"`), so accessibility labels and test selectors keep their names.

## Implementation Detail

**Patterns followed.**
- The landscape mat is a third layout branch beside the phone one. A developer reading Side, Strip or Hand sees `{#if app.layout === 'phone'} … {:else if app.wide} … {:else} … {/if}`: three markups over the same data, the same `Stack`/`Card` components and the same action helpers.
- Measuring the board area follows the phone's existing `fitBoard` attachment (a `ResizeObserver` on `<main>`, cleaned up on detach). The flag is a plain store field, like `layout`.

**Patterns introduced.**
- **Container-sized cards.** The wide layout is the first to size cards from the board area rather than the viewport. `<main>` becomes a size container, and a wrapper inside it sets `--cw` from container units. This keeps the timeline beside, below or hidden correct with no JS sizing maths. Today's mat and the phone keep viewport-based sizing.
- **Text that grows only above 1920px.** Header and timeline font sizes become `max(<today's px>, <vw>)`, so nothing changes at today's sizes.
- **An overlay anchored to the board area.** The fixed playback overlay subtracts half the timeline column's width (`--panel-w`) from its centre. `--panel-w` is set once on the app shell from the same state that places the timeline.

**Code-shape changes.**
- The Active Spot moves out of each side in the wide branch: `Side` stops rendering it, and `Strip` renders both, in the order the design fixes. On today's mat and the phone, Side still owns its Active, so the move is confined to the wide branch.
- The prize area gets a fanned-row variant that renders only remaining prizes, so it closes up. The 2×3 grid with dashed taken slots stays for today's mat.
- The opponent's small hand gets a "peek" variant: it is pinned to the top edge of the board, partly off it, with the board's top clipping the rest. Its list role and "N in hand" text stay for screen readers.
- The hand count moves from absolute positioning to the end of the hand's flex row on every non-phone layout.

**Tests.** A `wide` Playwright project at 2560×1440 joins phone, tablet and desktop, with an `isWide(testInfo)` helper beside `isPhone`. A new spec file covers the landscape acceptance criteria. Existing tests that assert today's mat geometry gain an `isWide` skip, as they already skip on the phone.

## Dependencies

- **`wide-board/design.md` from the `design` design source:** the settled landscape mat this plan implements, covering when it applies, the arrangement of zones, the sizing rule and the desktop-wide changes. No changes needed.
- **`mobile-board/design.md` from the `design` design source:** the phone layout and the shared drag model (Pointer Events, `data-drop` zones found by `elementFromPoint`). It must stay unchanged; this plan relies on its drag model so moved drop zones keep working.
- **Svelte 5 (runes) board UI**, already in use: per-layout branches, `$derived` flags and `{@attach}` attachments. No version change.
- **Browser CSS container queries and container units** (`container-type: size`, `cqw`/`cqh`): supported by current Chromium, Firefox and Safari, which the board already targets. No new library.
- **Playwright (`@playwright/test`)**, already used by `web/e2e`: gains a `wide` project. No version change.
- **The board test server** (`internal/webui/webtest`): serves the fixed position every e2e test starts from. Not expected to change. If the fixed position lacks a state a landscape check needs, tests set it up with `act()`.
- **Upstream work:** none needs to land first. The timeline dock button and the hidden rewind on the last timeline entry are already in the working tree and are compatible.

## Testing Approach

**Test types.** The work is presentational, so it is verified end to end in the board's existing Playwright suite against the board test server: real layout, real input, real game state. There are no new Go tests, because the server, wire format and game state don't change. No unit tests are added for the `wide` flag either: the flag only matters through what the board renders, and the end-to-end checks assert that directly at the sizes that matter.

**A new `wide` profile.** A fourth Playwright project, a 2560×1440 desktop, runs the landscape checks. The landscape behaviour gets the most coverage because it is the new layout, and its every action must match today's mat.

**What the new checks guarantee** (one or more per acceptance criterion):
- **The switch:** landscape at 2560×1440 with the timeline beside; today's mat at 1440×900; the phone layout at 390×844. The layout switches live, with no reload, on resize, on hiding the timeline, and on moving it from beside to below.
- **Zone placement**, asserted from element positions:
  - both Actives share one row, with the Stadium on one side and the turn controls on the other;
  - each player's bench, prizes and piles share a row, above or below the actives;
  - the prizes and piles sit at opposite ends in the two rows;
  - the opponent's name and status sit in a corner, and no bar spans the board.
- **Prizes:** the fan shows every remaining prize, countable and takeable, at 6, 3 and 1 left.
- **Opponent's hand peek:** 6 card backs at the top edge, each only partly in view, with the hand still exposed as a list with its count.
- **Card size:** a benched card at 2560×1440 is at least 1.6 times the width it had on today's mat at 2560×1440. The baseline is computed from today's `--cw` formula at that size, as the existing phone/desktop check already probes.
- **Fit:** at 2560×1440 and 3840×2160, no scroll bars and nothing cut off apart from the peek.
- **Nothing lost:** in the landscape layout, open each pile, take a prize, drag hand to bench, open a card's action sheet, Draw, End turn.
- **Hand counts:** within one card width of the hand's nearest card, on today's mat and in the landscape layout.
- **Text scales:** timeline and header text at 3840×2160 are at least 1.5× their size at 1920×1080, and unchanged at 1920×1080 and 1440×900.
- **Timeline width:** wider than 480px at 3840×2160.
- **Playback centred:** during a replay, the bar's and caption's horizontal centres are within 2px of the board area's centre, with the timeline beside, below and hidden.

**Regression.** The full existing suite keeps running on phone, tablet and desktop unchanged. That is the guarantee that today's mat and the phone layout are otherwise untouched. On the `wide` profile, existing tests that assert today's mat geometry skip, as they skip on the phone today. Every behavioural test (drag, sheet, target mode, panels) runs there too.

**Success metrics:**
- **"Every existing board e2e check passes on today's mat, and the equivalent checks pass on the landscape mat":** behavioural test. The full suite passes on all four profiles.
- **"The player's own 3840-wide monitor shows the whole game, timeline included, without zooming the browser":** the fit check at 3840×2160 covers it automatically. The real monitor is **Manual — captured in the implementation test plan**.

**Manual reviews:**
- **Visual review of the landscape mat against mockup C** on a large monitor: arrangement, peek, labels and readability. **Manual — captured in the implementation test plan.**
- **A replay on the landscape mat with the playback bar over the board**, checking it reads well where it covers cards. **Manual — captured in the implementation test plan.**

**Deliberate gaps.** Real iPad Safari is not re-tested for this change, because no iPad reaches the 1600px board-area width. Firefox and Safari desktop are not automated, because the suite is Chromium-only (see the knowledge entry on Playwright touch tests).

## Milestones & Tasks

### Milestone 1: Every desktop board reads better on big screens

**What changes**: On today's mat, at any desktop or tablet size, each "N in hand" count sits right beside its hand instead of at the far edge of the board. During a replay, the playback bar and caption are centred over the board, not the window, whether the timeline is beside, below or hidden. On screens wider than 1920px, the header and timeline text grow with the screen, and the timeline column can widen past 480px. At 1920px and below, today's sizes are unchanged. Nothing about the mat's arrangement changes yet.

**Validation point**: The existing browser suite passes on phone, tablet and desktop. New checks confirm the hand counts sit beside their hands, the playback bar is centred over the board area in all three timeline positions, the text scales at 3840×2160 and is unchanged at 1920×1080 and 1440×900, and the timeline exceeds 480px at 3840×2160.

#### - [x] Task: Hand counts beside their hands
**Id:** c2e07d78-2a6a-4bc3-b1ac-63523695f083
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Moves each "N in hand" count from the far right edge of the board area to sit right after the hand's last card, on every desktop and tablet layout. The count stays readable and keeps its text. The phone layout's hand is unchanged.

*Technical detail:* [context.md#task-hand-counts-beside-their-hands](./context.md#task-hand-counts-beside-their-hands)

**Acceptance criteria**:
- [x] On today's mat at desktop and tablet sizes, each player's "N in hand" sits within one card width of that hand's nearest card.
- [x] The phone layout's hand and counts look and behave as before.
- [x] The existing browser suite passes on phone, tablet and desktop.

#### - [x] Task: Centre the playback controls on the board
**Id:** 789415e5-a4f2-45e6-bfa4-d054d532a9f8
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Centres the playback bar and its move caption over the board area instead of the whole window, so they no longer sit off to one side when the timeline is beside the board. They stay at the top and may still cover the board. This also works with the timeline below the board or hidden.

*Technical detail:* [context.md#task-centre-the-playback-controls-on-the-board](./context.md#task-centre-the-playback-controls-on-the-board)

**Acceptance criteria**:
- [x] During a replay, the playback bar's and caption's horizontal centres are within 2px of the board area's centre with the timeline beside the board, below it, and hidden.
- [x] The playback bar's controls and caption work as before.

#### - [x] Task: Larger text and a wider timeline on big screens
**Id:** 7f86241a-c344-4969-a896-6e2be05777d4
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Makes the header and timeline text grow with the screen on screens wider than 1920px, so it stays readable next to the larger cards. At 1920px and below it stays exactly as today. The timeline column may now grow past today's 480px limit on very wide screens, in step with its text.

*Technical detail:* [context.md#task-larger-text-and-a-wider-timeline-on-big-screens](./context.md#task-larger-text-and-a-wider-timeline-on-big-screens)

**Acceptance criteria**:
- [x] At 3840×2160 the timeline and header text are at least 1.5 times their size at 1920×1080.
- [x] At 1920×1080 and 1440×900 the timeline and header text are the same size as before.
- [x] At 3840×2160 with the timeline beside the board, the timeline is wider than 480px.

### Milestone 2: The landscape mat on wide screens

**What changes**: When the board area is 1600px wide or more, the board switches to the landscape mat:
- Both Active Pokémon sit side by side in the middle with "vs" between them, the Stadium on one side and the turn controls on the other.
- Each player's bench, fanned prizes and piles share one row, mirrored like a real mat.
- The opponent's hand peeks in from the top edge, and their name and status sit in the top-left corner.
- Cards come out at least 1.6 times today's width at 2560×1440.

The board switches back and forth live as the window resizes or the timeline moves or hides. Every action works as it does on today's mat.

**Validation point**: On a new 2560×1440 test profile, checks confirm every landscape acceptance criterion: zone placement, the prize fan, the hand peek, card size, fit at 2560×1440 and 3840×2160, every action, and the live switching. The full suite still passes on phone, tablet and desktop.

#### - [x] Task: Wide-screen layout switch and test profile
**Id:** e4a89366-902b-4ede-b752-60044fa291b3
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds the board's knowledge of whether it is on a wide screen: true when the board area beside or above the timeline is 1600px wide or more, on any layout but the phone. It updates live as the window resizes or the timeline moves or hides. It also adds a 2560×1440 browser test profile. Existing checks that assert today's mat geometry skip on that profile, so the suite stays green while the landscape mat is built.

*Technical detail:* [context.md#task-wide-screen-layout-switch-and-test-profile](./context.md#task-wide-screen-layout-switch-and-test-profile)

**Acceptance criteria**:
- [x] The board knows it is wide at 2560×1440 with the timeline beside, and not at 1440×900. It changes with no reload when the window is resized, or the timeline is hidden or moved below the board at a width where that makes the board area wide enough.
- [x] A 2560×1440 test profile runs with the rest of the browser suite, and the whole suite passes on all four profiles.

#### - [x] Task: Landscape mat rows and sizing
**Id:** 0e01e2b6-d75d-489f-bd4b-8b3f2b1f6523
**Repo:** tcgreplay
**Depends on:**
- e4a89366-902b-4ede-b752-60044fa291b3 — Wide-screen layout switch and test profile
- c2e07d78-2a6a-4bc3-b1ac-63523695f083 — Hand counts beside their hands
**Execution:** agent

On wide screens, lays the board out as the landscape mat:
- each player's piles, bench and a fanned prize row share one row, mirrored for the opponent;
- both Active Pokémon sit side by side in the middle, labelled with whose they are, with "vs" between them, the Stadium on one side and the turn controls on the other;
- cards are sized from the board area so the four rows fill it without scrolling.

Every pile, prize, drop target, action sheet and turn control works as it does on today's mat.

*Technical detail:* [context.md#task-landscape-mat-rows-and-sizing](./context.md#task-landscape-mat-rows-and-sizing)

**Acceptance criteria**:
- [x] At 2560×1440, both Actives sit in one middle row with the Stadium on one side and the turn controls on the other, each labelled with whose it is.
- [x] Each player's bench, prizes and piles share one row above or below the Actives, with prizes and piles at opposite ends in the two rows.
- [x] With 6, 3 and 1 prizes left, every remaining prize is visible in one overlapping row and can be taken.
- [x] At 2560×1440 and 3840×2160 the board fits with no scroll bars.
- [x] Opening each pile, taking a prize, dragging from hand to bench, opening a card's action sheet, Draw and End turn all work in the landscape mat.

#### - [x] Task: Opponent's hand peeks from the top edge
**Id:** 9f912ef7-0eac-473e-bb0f-eca5c24f3be0
**Repo:** tcgreplay
**Depends on:**
- 0e01e2b6-d75d-489f-bd4b-8b3f2b1f6523 — Landscape mat rows and sizing
**Execution:** agent

On the landscape mat, the opponent's hand hangs off the top edge of the board, with only a strip of each card showing, enough to count them, instead of taking a row. Their name and status move to the board's top-left corner with no bar across the board. Screen readers still get the hand and its count.

*Technical detail:* [context.md#task-opponents-hand-peeks-from-the-top-edge](./context.md#task-opponents-hand-peeks-from-the-top-edge)

**Acceptance criteria**:
- [x] With the opponent holding 6 cards, 6 card backs show at the top edge, each only partly in view.
- [x] The opponent's hand is still announced as a list with its count.
- [x] The opponent's name and status sit in a corner of the board, and no bar spans the board's width.
- [x] Apart from the peek, nothing on the landscape board is cut off at 2560×1440 or 3840×2160.
- [x] A benched card at 2560×1440 with the timeline beside is at least 1.6 times the width it had on today's mat at that size (moved from "Landscape mat rows and sizing": height is the limit, and it only allows 1.6× once the opponent's hand stops taking a row).

#### - [x] Task: Landscape mat acceptance checks
**Id:** df3fc6d9-10c4-45c8-9114-5b86ad0e4e07
**Repo:** tcgreplay
**Depends on:**
- 0e01e2b6-d75d-489f-bd4b-8b3f2b1f6523 — Landscape mat rows and sizing
- 9f912ef7-0eac-473e-bb0f-eca5c24f3be0 — Opponent's hand peeks from the top edge
- 789415e5-a4f2-45e6-bfa4-d054d532a9f8 — Centre the playback controls on the board
**Execution:** agent

Adds browser checks covering every landscape acceptance criterion on the wide profile, so the layout can't regress. They cover:
- the switch, live, on resize and on moving or hiding the timeline;
- zone placement, the prize fan, the hand peek and the corner name;
- card size and fit, and every action;
- hand counts and playback centring on the landscape mat.

*Technical detail:* [context.md#task-landscape-mat-acceptance-checks](./context.md#task-landscape-mat-acceptance-checks)

**Acceptance criteria**:
- [x] Every acceptance criterion of the landscape spec has at least one browser check that passes on the wide profile.
- [x] The full browser suite passes on phone, tablet, desktop and wide.

## Open Questions

- **Can the 1.6× card-size target be met at 2560×1440 with the timeline beside, with a 5-card bench, without crowding the row?** With the timeline beside, the board area is about 2100px wide. Today's `--cw` there is about 119px, so the target is about 191px. A player row of 3 piles, 5 bench slots and the prize fan with today's gaps is roughly 10.7 card widths, which gives about 196px: feasible, but with little margin. The answer depends on the real rendered widths of piles, gaps and the prize fan. **If the target can't be met without shrinking gaps or piles below what reads well, STOP and ask the user** whether to relax the target or compact the piles (for example, a 2×2 pile block), rather than quietly missing it.

## Out of Scope

- **Phone layout changes:** the phone keeps its own settled design (`mobile-board/design.md`).
- **New board actions, or changes to what the timeline shows:** spec non-goal.
- **A setting to choose between the landscape mat and today's mat:** spec non-goal. The layout follows the board area's width.
- **Changing today's mat beyond the desktop-wide fixes:** the 2×3 prize grid, the pile column and the stacked actives stay as they are below 1600px.
- **Automated Firefox, Safari or real-iPad checks:** the suite is Chromium-only. No iPad reaches the 1600px board-area width.
- **The other items in the 2026-11-04 bug report:** the opponent as a fresh sub-agent, a revealed card staying face-up, automatic mulligans, heal-all, undo branches, game-over detection and prize reminders. They're tracked outside this plan, pending the user's choice of spec or direct fixes.

## Changelog

### 2026-11-04 — Task: Hand counts beside their hands

**What was done**: On every non-phone layout, each hand's "N in hand" count is now an in-flow item at the end of the hand's flex row, so it sits right after the hand's last card instead of being pinned to the board area's right edge. The phone keeps its absolutely positioned count.

**Deviations**: The count's left margin is `calc(var(--cw) * 0.7)`, not the plan's `0.15`: the fanned last card is rotated, so its rendered box overhangs its layout box, and a smaller margin let the count overlap it. The new tests sit in `web/e2e/phone.spec.js` beside the other mat checks, as the plan suggested. Work happens in a manually created git worktree (`/home/nicj/code/github.com/nicholasjackson/tcgreplay-wide-board`, branch `spek/20261104090653-7373a393-wide-screen-landscape-board`), because `implement new` did not create one; the orchestrator approved this.

**Files changed**:
- `web/src/components/Hand.svelte`
- `web/e2e/phone.spec.js`

**Discoveries**: A fanned hand's last card is rotated about a point below the card, so its bounding box reaches past its layout box; anything placed after the hand needs about 0.5–0.7 card widths of clearance. The opponent's small hand aligns its count to the top (`align-self: flex-start`), because that hand hangs down from the top edge.

### 2026-11-04 — Task: Centre the playback controls on the board

**What was done**: The timeline column's width is now one CSS variable, `--panel-col` on `.app`, used by the `.body.panel` grid. A `panel-side` class (timeline shown beside the board, not on a phone) sets `--panel-w` to it, and it is `0px` otherwise, including below 900px, where the timeline stacks. The fixed playback overlay's `left` is `calc(50% - var(--panel-w) / 2)`, so the bar and caption centre on the board area. Their max widths also leave out the timeline.

**Deviations**: `--panel-w` is set by a class, not by `style:--panel-w`, so the existing `max-width: 900px` media rule can zero it. The overlay uses `50%`/`100%` of the fixed containing block rather than `100vw`, so a page scrollbar can't shift it. The test is in a new `web/e2e/desktop.spec.js`, for the desktop-wide chrome checks.

**Files changed**:
- `web/src/App.svelte`
- `web/e2e/desktop.spec.js`

**Discoveries**: Starting a replay from a timeline line ("Play from here") autoplays it. Tests pause it, then step once with "Next move", so the caption shows and holds still while they measure.

### 2026-11-04 — Task: Larger text and a wider timeline on big screens

**What was done**: Header text is now `max(13px, 0.677vw)`, and the header's buttons and branch select inherit it. The ☰ button is `max(16px, 0.833vw)`, and the menu's check items and the timeline heading are `max(13px, 0.677vw)`. The timeline's 12px text is `max(12px, 0.625vw)` and its 11px turn headings are `max(11px, 0.5729vw)`, so all of it is unchanged at 1920px and below and doubles by 3840px. The timeline column, `--panel-col`, is now capped at 960px instead of 480px.

**Deviations**: The ☰ glyph and the menu's check items scale too, so the whole header grows together. The 11px headings use `0.5729vw`, not `0.573vw`, so they are exactly 11px at 1920px; the goal reviewer noticed the 0.0016px difference, and I changed it after the check. The tests are in `web/e2e/desktop.spec.js`.

**Files changed**:
- `web/src/App.svelte`
- `web/src/components/Timeline.svelte`
- `web/e2e/desktop.spec.js`

**Discoveries**: `app.css` sets every button to 13px, so header controls only follow the header's size through an explicit `font-size: inherit`. That rule must exclude `.burger`, because `header button` outranks the scoped `.burger` class.

### 2026-11-04 — Task: Wide-screen layout switch and test profile

**What was done**: `app.wide` was added to the board's layout state. App attaches a `measureBoard` ResizeObserver to `<main>` on non-phone layouts (the phone keeps `fitBoard`). It sets `app.wide` when the board area is at least 1600px wide and resets it on detach. `.app` carries `class:landscape` and `data-wide` for styles and tests. A `wide` Playwright project at 2560×1440 and an `isWide` fixture were added. Switch tests in `web/e2e/desktop.spec.js` cover 2560×1440 (wide), 1440×900 (not wide), resizing live on the same page, and, at 1800×1000, moving the timeline below or hiding it.

**Deviations**: No existing test failed on `wide` yet, because nothing visible changes until the landscape markup lands. Two of today's mat checks were made wide-aware ahead of it: "tablets and desktops keep the mat" skips on `wide`, and the `--cw` probe in "tablets and desktops use none of the phone board" returns early on `wide`. No `test.fixme` markers were needed.

**Files changed**:
- `web/src/lib/store.svelte.js`
- `web/src/App.svelte`
- `web/playwright.config.js`
- `web/e2e/fixtures.js`
- `web/e2e/phone.spec.js`
- `web/e2e/desktop.spec.js`

**Discoveries**: At 1800px the timeline column is 324px, so the board area is about 1476px: under the threshold beside the timeline, and over it with the timeline below or hidden. That makes it a good width to test the timeline-driven switch.

### 2026-11-04 — Task: Landscape mat rows and sizing

**What was done**: On wide screens the board is now the landscape mat:
- `Side.svelte` has a wide branch with one grid row: a prize fan (remaining prizes only, overlapping), the bench in the middle and the piles. The opponent's row is mirrored (`piles bench prizes`). The pile markup moved into a `piles()` snippet shared with today's mat.
- `Strip.svelte` has a wide branch with the Stadium, both Active Spots (through `Stack`, with the same `slot`/`label` props) edged in their seat's colour and captioned "<name>'s Active" on their top edge, "vs" between them, and the turn controls.
- `App.svelte` renders the board through a `board()` snippet. When wide, it wraps the snippet in a `.board` box inside `<main>`, which becomes a size container. `.board` sets `--cw` from container units: `min((100cqh - 90px) / 7.6, (100cqw - 80px) / (4.6 + 1.16 × bench slots))`.
- A shared `benchSlots()` helper in the store gives the longer bench's slot count, so a bench of 8 shrinks the cards.
- With the timeline below the board, the landscape layout gives the timeline a fixed-height row.

**Deviations**: The 1.6× benched-card criterion moved from this task to "Opponent's hand peeks from the top edge" (orchestrator decision, option 1), and the plan was updated through `plan file write`. Height limits the card size, not width. With the opponent's hand still taking a row, a benched card at 2560×1440 is 171.8px (1.44×), while width alone would allow about 194px. A CSS experiment with the peek and corner name gave 192.2px with uncramped rows. The size test in `web/e2e/wide.spec.js` is `test.fixme` until the peek lands. `store.svelte.js` gained the `benchSlots` helper, which wasn't in the plan's file list. A sub-agent wrote the tests, as this task is High complexity.

**Files changed**:
- `web/src/components/Side.svelte`
- `web/src/components/Strip.svelte`
- `web/src/App.svelte`
- `web/src/lib/store.svelte.js`
- `web/e2e/wide.spec.js`

**Discoveries**:
- Container units in an unregistered custom property resolve where the property is used, against that element's nearest size container. An element's own `font-size: var(--fs)` therefore resolves against its ancestor's container, not its own, which is why `--cw` sits on a `.board` wrapper inside `main` rather than on `main`.
- A size container in an `auto` grid row collapses to zero height, so the timeline-below layout needs explicit rows.
- In the fan, each prize is mostly covered by the next, so tests click each prize's uncovered strip.

### 2026-11-04 — Task: Opponent's hand peeks from the top edge

**What was done**: On the landscape mat, the opponent's hand (`Hand.svelte`, `peek` class) is absolutely positioned and hangs off the board's top edge. About a third of each card shows, tightly overlapped, and `<main>` clips the rest. It keeps its list role and label, and its "N in hand" is visually hidden but still read. The opponent's name and status sit in the board's top-left corner. `.board` reserves a 0.3-card strip at the top for the peek. The card size was re-tuned to `min((100cqh - 50px - fit) / 6.9, width term)`: a benched card is 194px at 2560×1440 with the timeline beside (1.625× today's 119.4px). Width is now the limit, and the row gaps are unchanged. The previously `test.fixme` size test now passes.

**Deviations**:
- The 1.6× criterion came over from "Landscape mat rows and sizing" (orchestrator decision).
- `fitBoard`, the phone's shrink-to-fit, now counts only in-flow rows plus padding, and is also attached to `.board`. A busy landscape board, with Energy and a Tool under benched Pokémon in both rows, shrinks a little instead of scrolling. A test covers this.
- Two drag fixes outside the plan's file list, found while testing the landscape mat:
  - `lib/pointer.svelte.js`: the drag ghost now takes `--cw` in pixels, because container units mean nothing on a ghost outside the board. With the timeline below, it was about 50% too big. It reads the computed width, because a fanned card's bounding box is narrowed by its rotation.
  - `Strip.svelte`: the "Drop here to play" area floats under the controls on the landscape mat, so it no longer grows the middle row and resizes every card mid-drag.

**Files changed**:
- `web/src/components/Hand.svelte`
- `web/src/App.svelte`
- `web/src/components/Strip.svelte`
- `web/src/lib/pointer.svelte.js`
- `web/e2e/wide.spec.js`

**Discoveries**:
- `getBoundingClientRect` on an element inside a rotated ancestor returns the rotated box. Use the computed `width` to read a length.
- Anything that appears in a landscape row during a drag changes the card size through the fit, so drag-only UI there must be out of flow.

### 2026-11-04 — Task: Landscape mat acceptance checks

**What was done**: Every spec acceptance criterion now has a browser check that passes on the `wide` profile. `web/e2e/wide.spec.js` covers the landscape-specific ones in 16 tests: the middle row, the mirrored rows, the prize fan at 6/3/1, the peek, the corner name, the card size, fit with the timeline beside/below/hidden, fit on a busy board, every action, the drag ghost's size, hand counts, and the switch between the landscape mat, today's mat and the phone board. The checks shared with today's mat also run on `wide`: hand counts in `phone.spec.js`, and the timeline-driven switch, playback centring and text/timeline scaling in `desktop.spec.js`. The header comment of `wide.spec.js` points to them. The full suite passes on phone, tablet, desktop and wide: 310 passed, 146 skipped (profile-scoped skips), 0 failed.

**Deviations**: The opponent's "N in hand" on the landscape mat is now shown beside the peeking cards' visible ends, not visually hidden. "Counts beside hands" asks for each count to sit within one card width of its hand on the landscape mat too, and the visible text still serves screen readers. This touched `web/src/components/Hand.svelte`, which isn't in this task's file list. The plan suggested reusing Milestone 1's hand-count and playback assertions in `wide.spec.js`. Instead those tests already run on the `wide` profile, and `wide.spec.js` adds its own landscape hand-count check.

**Files changed**:
- `web/e2e/wide.spec.js`
- `web/src/components/Hand.svelte`

**Discoveries**: On the `wide` profile the shared mat checks in `phone.spec.js` resize to 1920×1080, which is below the 1600px board-area threshold with the timeline beside. They still exercise today's mat there.
