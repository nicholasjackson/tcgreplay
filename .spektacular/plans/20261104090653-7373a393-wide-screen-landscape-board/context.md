---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Context: 20261104090653-7373a393-wide-screen-landscape-board

## Current State Analysis

- The desktop and tablet board stacks seven rows: the opponent's hand, the opponent's side (prizes grid | Active + bench | piles column, mirrored), the strip (Stadium + turn controls), the player's side, and the player's hand (`web/src/App.svelte:170-183`, `web/src/components/Side.svelte:77-142`, `web/src/components/Strip.svelte:40-92`).
- Card width `--cw` comes from the viewport: `clamp(48px, min((100vh - 115px) / 11.1, (100vw - 360px) / 10), 240px)` (`web/src/app.css:5`), with overrides for no timeline or the timeline below (`web/src/App.svelte:238-245`) and for the phone (`:253`). At 2560×1440 with the timeline beside, this gives about 119px; the board fills about half its column.
- "N in hand" is `position: absolute; right: 12px` in the full-width hand row (`web/src/components/Hand.svelte:73,145-151`), so it sits at the board area's edge.
- The playback overlay is `position: fixed; left: 50%` of the window (`web/src/App.svelte:401-411`), so it is off-centre over the board when the timeline is beside it.
- The timeline column is `clamp(320px, 18vw, 480px)` (`web/src/App.svelte:369`), and its text is a fixed 12px (`web/src/components/Timeline.svelte:142,153,196`). Header text is 13–14px.
- `app.layout` is 'phone' | 'tablet' | 'desktop' from media queries (`web/src/lib/store.svelte.js:39-77`). There is no notion of a wide board.
- The browser suite has phone (390×844), tablet (1024×768) and desktop (1440×900) Chromium profiles (`web/playwright.config.js:21-34`).

## Per-Task Technical Notes

All work is in the `tcgreplay` repo (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`). Each requirement maps to files as follows:
- **Hand counts:** `web/src/components/Hand.svelte`
- **Playback centring:** `web/src/App.svelte`
- **Text and timeline:** `web/src/App.svelte`, `web/src/components/Timeline.svelte`, `web/src/app.css`
- **Wide switch:** `web/src/lib/store.svelte.js`, `web/src/App.svelte`
- **Landscape rows:** `web/src/components/Side.svelte`, `web/src/components/Strip.svelte`, `web/src/App.svelte`
- **Peek:** `web/src/components/Hand.svelte`, `web/src/App.svelte`
- **Tests:** `web/playwright.config.js`, `web/e2e/*`

Rebuild the board with `cd web && npm run build` before running e2e (`npx playwright test`). The suite drives the built UI.

### Task: Hand counts beside their hands

**File changes**
- `web/src/components/Hand.svelte:73` — keep the `<span class="count">` as the last child of the `.hand` flex row.
- `web/src/components/Hand.svelte:145-151` — replace `.count { position: absolute; right: 12px; bottom: 4px; }` with an in-flow flex item: `align-self: flex-end`, `margin-left: calc(var(--cw) * 0.15)`, `white-space: nowrap`. Keep the phone's count where it is (scope the change with `.hand:not(.phone) .count`; check the phone hand's markup first).
- `web/e2e/phone.spec.js` — new test (desktop and tablet; skip phone): for each seat's hand, the `.count` box's left edge is within one benched card width of the right edge of the hand's last `.copy`.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution

### Task: Centre the playback controls on the board

**File changes**
- `web/src/App.svelte:88` — set `style:--panel-w` on `.app`. When the timeline is beside the board (`showTimeline && !phone && !bottom`), use the timeline column's width expression (the same `clamp(...)` as `.body.panel`, after the next task's change if it lands first); otherwise `0px`. To keep one source of truth, define the column width as a CSS variable `--panel-col` on `.app` and use it in both `.body.panel` and `--panel-w`.
- `web/src/App.svelte:369` — `.body.panel { grid-template-columns: 1fr var(--panel-col); }`
- `web/src/App.svelte:401-411` — `.overlay { left: calc((100vw - var(--panel-w, 0px)) / 2); }` (keep `transform: translateX(-50%)`), and `max-width: calc(100vw - var(--panel-w, 0px) - 32px)`. Caption `max-width: 80vw` → `min(80vw, calc(100vw - var(--panel-w, 0px) - 32px))`.
- `web/e2e/` — new test in a suitable spec (playback is driven by `startReplay`, as the Timeline's ▶ button does). Click a timeline "Play from here" so `.overlay .bar` shows, then compare the centres of `.overlay .bar` (and `.caption` if present) with `main`'s horizontal centre: within 2px with the timeline beside, after the dock button moves it below, and after "Hide timeline" in ☰. Skip on phone (its timeline is a sheet).

**Complexity**: Low
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution

### Task: Larger text and a wider timeline on big screens

**File changes**
- `web/src/App.svelte:316` (`header`) — set the header's font size to `max(13px, 0.677vw)`, and the header buttons and labels to match (they inherit `font: inherit`; `app.css` sets buttons to 13px, so override within `header`).
- `web/src/App.svelte:388` — `aside h2 { font-size: max(13px, 0.677vw); }`
- `web/src/components/Timeline.svelte:142,153,196` — the 12px sizes become `max(12px, 0.625vw)`.
- `web/src/App.svelte:369` (or the `--panel-col` variable from the previous task) — the timeline column becomes `clamp(320px, 18vw, 960px)`. Below about 2667px wide this equals today's width; above it, it keeps growing.
- `web/e2e/` — new tests that open fresh pages at 3840×2160, 1920×1080 and 1440×900 (`page.setViewportSize`), and read `getComputedStyle` font sizes of `.timeline` and `header`: at least 1.5× at 3840 vs 1920, and equal to today's values (12px/13px) at 1920 and 1440. The `aside` width is greater than 480 at 3840×2160 with the timeline beside.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution

### Task: Wide-screen layout switch and test profile

**File changes**
- `web/src/lib/store.svelte.js:41-42` — add `wide: false` to `app`, with a comment: true when `layout !== 'phone'` and the board area is ≥1600px wide.
- `web/src/App.svelte:27-48,170` — add a `measureBoard` attachment on `<main>` for non-phone layouts: a `ResizeObserver` sets `app.wide = main.clientWidth >= 1600`, and cleanup resets `app.wide = false`. The phone keeps `fitBoard`, so attach one or the other: `{@attach phone ? fitBoard : measureBoard}`. `<main>` resizes on window resize, on the timeline moving beside or below, and on hiding it, so one observer covers every case.
- `web/src/App.svelte:88` — add `class:landscape={app.wide}` to `.app` for CSS hooks.
- `web/playwright.config.js:21-34` — add the project `{ name: 'wide', use: { ...devices['Desktop Chrome'], viewport: { width: 2560, height: 1440 } } }`.
- `web/e2e/fixtures.js:192-194` — add `export function isWide(testInfo) { return testInfo.project.name === 'wide'; }`.
- `web/e2e/*.spec.js` — run the suite on `wide`. Add `test.skip(isWide(testInfo), 'the landscape mat')` to tests that assert today's mat geometry, such as `phone.spec.js:183` ("tablets and desktops keep the mat"), `:197` (the `--cw` probe and aside-beside check), and any test that fails only because zones moved. Behaviour tests (drag, sheet, target, panels) must keep running on wide; if one fails there, that is a landscape bug for the next task, so mark it `test.fixme(isWide(...))` with a note and remove the note in "Landscape mat rows and sizing".
- Until "Landscape mat rows and sizing" lands, `app.wide` changes nothing visible; this task's tests check the flag through a `data-layout` attribute. Set `data-wide={app.wide}` on `.app` and assert it at 2560×1440 (true), at 1440×900 (false), and live after `page.setViewportSize` changes and after hiding the timeline or moving it below.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent; run the full suite on `wide` once to list the geometry tests to skip

### Task: Landscape mat rows and sizing

**File changes**
- `web/src/components/Side.svelte:60-77` — add `{:else if app.wide}`: a `<section class="side wide" class:mirrored aria-label="{seat} side">` with one flex row of three parts.
  - **Piles:** the same Deck/Discard/Lost markup as `:101-139`, laid out horizontally. Extract the piles block to a snippet (`{#snippet piles()}`) shared by both branches, so handlers aren't duplicated.
  - **Bench:** the `slots` loop of `Stack` with `slot="bench:{i}"`, no wrap.
  - **Prize fan:** remaining prizes only, in `side.prizes.filter(p => p.card)`, each a `.prize` holding `<Card size="sm" onclick={() => prize(p)}>` with a negative left margin of about `calc(var(--cw) * -0.4)`. Keep `title="{side.prizes_left} prizes left"` and the count.
  - **Order:** player = prizes, bench, piles; mirrored (opponent) = piles, bench, prizes, using `flex-direction: row-reverse` on `.mirrored`.
  - The Active is not rendered here.
- `web/src/components/Strip.svelte:40-60` — when `app.wide`, render `[stadium] [player Active] vs [opponent Active] [controls]`. Each Active is `<Stack seat={...} slot="active" stack={app.board.sides[seat].active} label="{seat} Active" />`, the same props as `Side.svelte:92`, so drops, sheets and selectors are unchanged. Add a caption under each ("<name>'s Active") and a seat-coloured border (`--p1`/`--p2`). Import `Stack`.
- `web/src/App.svelte:170-183` — the markup order is unchanged (opponent's Hand, opponent's Side, Strip, player's Side, player's Hand). The landscape rows come from the branches.
- `web/src/App.svelte` styles — when `.app.landscape`:
  - `main { container-type: size; }`
  - set `--cw` on main's children (container units resolve against `main`) as `min((100cqh - <fixed px for names and hand>) / <rows factor>, (100cqw - <padding>) / <widest-row factor>)`;
  - the widest row is 8 bench slots + 3 piles + the prize fan, about 13.5 `--cw` with gaps; the height is about 3 rows × 1.4 `--cw` + the hand at 1.4 `--cw` + gaps.

  Tune both factors so 2560×1440 gives ≥1.6× today's `--cw` (today at 2560×1440 with the timeline beside: `min((1440-115)/11.1, (2560-360)/10)` = 119.4px, so the target is ≥191px) and nothing scrolls at 3840×2160. Keep `--fs` derived from `--cw`, capped as today.
- `web/e2e/*.spec.js` — remove the `test.fixme(isWide…)` markers added by the previous task once the behaviour passes on wide.

**Complexity**: High
**Token estimate**: ~60k tokens
**Agent strategy**: Parallel analysis, sequential integration. One agent drafts the Side branch while another drafts the Strip branch. Integrate the sizing in App last, then tune `--cw` against screenshots at 2560×1440 and 3840×2160.

### Task: Opponent's hand peeks from the top edge

**File changes**
- `web/src/components/Hand.svelte:52-75` — when `app.wide && small`, add `class:peek`. Peek styles: `margin-top: calc(var(--cw) * -0.9)` (only about a third of each card stays inside `main`), with tighter overlap and no fan lift. Keep `role="list"` and `aria-label="{seat} hand"`, and add a visually hidden "N in hand" so the count stays readable to screen readers.
- `web/src/App.svelte:170-178` — in the landscape layout, `main` clips the overflow at the top (`overflow: hidden` on a wrapper, or `clip-path`), so the peek is cut by the board's edge, not the header. Move the opponent's `.seat-name.top` (name plus `.opp-status`) into an absolutely positioned corner: `position: absolute; top: 8px; left: 12px` inside a `position: relative` main wrapper. The phone and today's mat keep the current markup.
- `web/src/components/Hand.svelte` — make sure the peek's card backs don't open sheets (they already have `onmenu=null` when hidden).

**Complexity**: Medium
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution

### Task: Landscape mat acceptance checks

**File changes**
- `web/e2e/wide.spec.js` — new file; every test starts with `test.skip(!isWide(testInfo), 'the landscape mat')`. The tests:
  - **Switch:** at 2560×1440 `.app[data-wide=true]`. `page.setViewportSize({width:1440,height:900})` gives false, and back again gives true. Hide the timeline at 1800×1000 (board area ≥1600) and it flips to true. Moving the timeline below at 1800 wide does the same.
  - **Middle row:** the boxes of `[aria-label="p1 Active"]` and `[aria-label="p2 Active"]` share a vertical centre (±4px). The Stadium region is left of both, and `.controls` is right of both.
  - **Rows mirror:** each side's bench, prizes and piles have overlapping y ranges. The opponent's row is above the actives and the player's below. The player's prizes are left of their piles; the opponent's piles are left of their prizes.
  - **Prize fan:** take prizes with `act('take_prize', …)` to leave 3, then 1. The count of visible prize cards equals `prizes_left`, each is in the viewport, and clicking one takes it.
  - **Peek:** the 6 opponent hand cards have `boundingBox().y < main.y` and a bottom inside `main`. `getByRole('list', { name: 'p2 hand' })` exists, and its count text reads "6 in hand".
  - **Corner name:** `.seat-name.top` lies within 120px of main's top-left corner, and no element with the name spans more than 50% of main's width.
  - **Card size:** a benched card's width ≥ 1.6 × 119.4px (today's `--cw` at 2560×1440 with the timeline beside). Compute the baseline with the probe pattern from `phone.spec.js:219-225` using today's formula string.
  - **Fit:** at 2560×1440 and 3840×2160, `main.scrollHeight <= main.clientHeight + 1` and `scrollWidth` likewise. Every zone except the peek is inside main.
  - **Actions:** open the Deck, Discard and Lost piles (modal shows); take a prize; drag a hand card to a free bench slot with `mouseStart` from `fixtures.js`; open a card's action sheet (right-click); Draw; End turn. The board changes as on the desktop profile.
  - **Hand counts** and **playback centring** on the landscape mat: reuse the assertions from the Milestone 1 tasks.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential execution; build once, then iterate on the spec file

## Testing Strategy

- **Hand counts beside their hands:** an e2e geometry check on desktop and tablet (skipped on the phone), comparing the count's box with the hand's last card.
- **Centre the playback controls on the board:** an e2e check that starts a replay from the timeline and compares the centres of `.overlay .bar` and `.caption` with `main`'s, with the timeline beside, below and hidden.
- **Larger text and a wider timeline on big screens:** e2e checks of computed font sizes at 3840×2160, 1920×1080 and 1440×900, and the aside width at 3840×2160.
- **Wide-screen layout switch and test profile:** the new `wide` project. The `data-wide` flag is asserted at the switch sizes, live on resize, on hiding the timeline and on moving it. Today's-mat geometry tests skip on wide.
- **Landscape mat rows and sizing**, and **Opponent's hand peeks from the top edge:** covered by `wide.spec.js` in the final task, with local screenshot checks while tuning `--cw`.
- **Landscape mat acceptance checks:** `web/e2e/wide.spec.js`, one or more tests per acceptance criterion (listed in that task's notes).
- **Regression:** the full suite on all four profiles after each milestone.
- **Manual (captured in the implementation test plan):**
  - visual review against mockup C on a large monitor;
  - a replay with the bar over the landscape board;
  - the user's own 3840-wide monitor shows the whole game without browser zoom.

## Project References

- Spec: `20261104090653-7373a393-wide-screen-landscape-board` (`spektacular spec file read 20261104090653-7373a393-wide-screen-landscape-board`)
- Design: `wide-board/design.md` (source `design`) — the settled landscape mat
- Design: `mobile-board/design.md` (source `design`) — the phone layout and the shared drag model
- Mockups: https://claude.ai/artifact/HK5VjHeETLarQsFE5d5p5n (artboard OptionC)
- Knowledge: `gotchas/playwright-touch-tests.md` (Chromium-only profiles, CDP touch drags, `isPhone`, hover gating during drags)
- Repo root: tcgreplay at `/home/nicj/code/github.com/nicholasjackson/tcgreplay`

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

## Migration Notes

None. There is no stored data, wire format or setting to migrate. A viewer's saved timeline position (`localStorage` `panelAt`) keeps working.

## Performance Considerations

- One `ResizeObserver` on `<main>` sets a boolean. It only causes a re-render when the 1600px threshold is crossed.
- Container-unit sizing is pure CSS and adds no layout passes beyond today's.
