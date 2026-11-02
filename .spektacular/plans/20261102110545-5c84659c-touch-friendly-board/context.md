---
created_date: "2026-11-02"
document_status: final
closed_date: "2026-11-02"
---

# Context: 20261102110545-5c84659c-touch-friendly-board

## Current State Analysis

The board is a Svelte 5 app in `web/`, built by Vite into `internal/webui/dist/app` and embedded in the binary (`web/vite.config.js:9-12`). It responds only to a mouse:

- **Moving cards** uses HTML5 drag and drop. The payload goes on `dataTransfer` (`web/src/lib/actions.js:26-42`). Drop zones are wired per component (`Stack.svelte:26-35`, `Hand.svelte:58`, `Side.svelte:20-33`, `Strip.svelte:14-28`). Drops are interpreted by `dropOnStack` and `dropOnZone` (`actions.js:69-124`), which send board verbs over the websocket (`web/src/lib/ws.js`).
- **Menus** open on `contextmenu` (`Card.svelte:40-45`; the deck also opens on click, `Side.svelte:81`). The builders `handMenu`, `stackMenu`, `attachmentMenu` and `deckMenu` (`actions.js:148-320`) produce flat `{label, run}` lists, and `ContextMenu.svelte` renders them as a fixed popover. `stackMenu` runs to 25+ items, and `handMenu` lists one item per target.
- **Numbers** are asked for with `window.prompt` through `ask()` (`actions.js:142-145`): for attacks, Set damage, Draw N and Look at top N.
- **Hover:**
  - cards enlarge on `:hover` (`Card.svelte:105-116`, `Hand.svelte:105-110`);
  - quick damage buttons show on hover (`Stack.svelte:169`);
  - the timeline's rewind and play buttons show on hover (`Timeline.svelte:175-195`);
  - pile previews use `mouseenter` (`Modal.svelte:98-173`).
- **Layout.** `--cw` (`app.css:6`, `App.svelte` `.app.wide`/`.panel-bottom`) sizes the mat at about 10 card widths wide with a 48px floor. A phone overflows sideways.
- **Tests.** There are no frontend tests. `make test` runs only `go vet` and `go test` (`Makefile:27-29`). The board server's Go tests build a real in-process server with a fake card pool (`internal/webui/webui_test.go:28-105`).
- **Bench.** The bench can hold 8 with a bench-expanding Stadium (`MaxBench`). The board shows an extra slot when the Stadium is in play (`Side.svelte:15-17`). This change is currently uncommitted and must be committed before implementation (see Dependencies in plan.md).

## Per-Task Technical Notes

### Task: Board test server and browser test harness

**Requirement attribution.** All of this task's work is in repo `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`). It covers the spec requirement "Touch behaviour is covered by automated tests" and the AC "Automated touch and mouse tests run with the project's checks". It also lays the groundwork for every later AC.

**File changes:**
- `internal/webui/webui_test.go:28-81` — move `fakeDB` (28-61) and `testDeck` (63-69) into a new test-support package and import them from there. Keep the test behaviour identical. Extend the pool with what the fixed position and later tests need:
  - a Basic with two attacks, damage values and a Retreat Cost;
  - a Basic with an Ability;
  - a Stage 1 that evolves from it;
  - a Pokémon Tool;
  - a Supporter;
  - a Stadium whose effect text matches the bench-expansion regex ("Each player can have 8 Pokémon on their Bench.");
  - Basic Fighting Energy.

  Card fields (`Attacks`, `Abilities`, `Retreat`, `Effect`) come from `internal/cards` types.
- `internal/webui/webtest/fakedb.go` (new) — the shared `FakeDB` and the decklists. It is an ordinary package, imported only by tests and the test server. The name may be adjusted, but it must sit under `internal/` and must not be imported by `cmd/tcgreplay`.
- `internal/webui/webtest/cmd/boardtest/main.go` (new):
  - builds `game.NewManager(tmpdir, webtest.FakeDB())`, then `m.New` with fixed decks;
  - switches to review mode and lays out the fixed position by iid with the existing verbs (`move` into hand, `place_active`, `bench`, `attach`, `move` to stadium), so the shuffle doesn't matter;
  - serves `webui.New(m, state.P1).Handler()` on `127.0.0.1:<port>` (flag, default 7790);
  - adds `POST /test/reset`, which rebuilds the manager's game and position.

  Model the construction on `internal/webui/webui_test.go:91-105`. It must serve the built board from `internal/webui/dist/app` (embedded by `webui`), so the board must be built first.
- `internal/webui/webtest/position_test.go` (new) — a Go test that the fixed position has the expected cards in each zone and that reset restores it after a move.
- `web/package.json` — add the `@playwright/test` dev dependency, pinned to an exact release at least two weeks old, and a `"test:e2e": "playwright test"` script. Update `web/package-lock.json`.
- `web/playwright.config.js` (new):
  - three projects: `phone` (WebKit "iPhone 13" descriptor, `hasTouch`), `tablet` (WebKit "iPad (gen 7) landscape" or 1024×768 with `hasTouch`) and `desktop` (Chromium, 1440×900, mouse);
  - a `webServer` running `go run ./internal/webui/webtest/cmd/boardtest -addr 127.0.0.1:7790` from the repo root, with `baseURL` `http://127.0.0.1:7790`;
  - `fullyParallel: false` and one worker, because there is a single shared game.
- `web/e2e/fixtures.js` (new):
  - a Playwright fixture that POSTs `/test/reset` before each test;
  - a `page.on('dialog')` handler that fails the test;
  - helpers `tapCard(name)`, `touchDrag(from, to)` (using `page.touchscreen`, or CDP `Input.dispatchTouchEvent` on Chromium and `locator.dispatchEvent` pointer events on WebKit) and `timelineLines()`.
- `web/e2e/smoke.spec.js` (new) — loads `/`, waits for the connection dot, and checks the names, hand cards and Pokémon in play.
- `Makefile:27-29` — `test` keeps `go vet ./...` and `go test ./...`, then runs `$(MAKE) web` and `cd web && npx playwright test`. Before running, check for installed browsers (`npx playwright install --dry-run`, or test for the cache directory) and print "Run: cd web && npx playwright install webkit chromium" when they are missing.
- `.gitignore` — add `web/test-results/` and `web/playwright-report/`.

**Complexity:** Medium
**Token estimate:** ~45k tokens
**Agent strategy:** 2 parallel agents: one for the Go test-support package, the server and the Go test; one for the Playwright config, fixtures, smoke test and Makefile. Then integrate sequentially by running the suite.

### Task: Touch dragging through one pointer layer

**Requirement attribution.** All in repo `tcgreplay`, `web/`. It covers the spec requirements "Move cards by touch", "Scroll the hand without picking up a card" and "See where a card can go while dragging", and the ACs "Hand to bench by touch", "Every drop target accepts a touch drop", "A sideways swipe scrolls the hand" and "Drop targets are marked and a missed drop is undone".

**File changes:**
- `web/src/lib/pointer.svelte.js` (new) — the drag controller:
  - `draggable(payload, {onTap})` and `dropZone(spec, onDrop)` attachments (`{@attach}`);
  - pointerdown with `setPointerCapture`, an 8px threshold, and in the hand a direction test (|dy| > |dx| and upward), otherwise let the browser scroll;
  - a ghost element raised half a card above the pointer;
  - `document.elementFromPoint` → `closest('[data-drop]')`, lighting the zone under the pointer and adding a body class that outlines every `[data-drop]`;
  - pointerup over a zone calls its `onDrop(payload)`; Escape or pointercancel cancels;
  - sets `app.dragging` at the start and clears it at the end (read by `web/src/components/Strip.svelte:83`);
  - a tap (under the threshold) calls `onTap(event)` with `pointerType`.
- `web/src/lib/actions.js:6,26-42` — remove `DRAG_TYPE`, `startDrag`, `endDrag` and `readDrag`. Keep `dropOnStack` (69-108) and `dropOnZone` (111-124) unchanged.
- `web/src/components/Card.svelte:5,32-34,49-60` — replace `draggable`/`ondragstart`/`ondragend` with `{@attach draggable({kind:'card', iid, def, from, role})}`, enabled under the same condition as today (`draggable && !card.hidden && app.opponent !== 'acting'`). Clicks keep working through `onTap` (see the next task for touch).
- `web/src/components/Stack.svelte:5,26-38,47-54` — the slot root becomes `{@attach dropZone(spec, (d) => dropOnStack(d, seat, slot, !!stack))}`, and the pile becomes `{@attach draggable({kind:'stack', from: spec})}`. Remove `over`/`dragover`/`drop`/`dragStack`, and replace the `.over` styling with the controller's lit-zone class.
- `web/src/components/Hand.svelte:7,49-60` — the hand root becomes a drop zone for `'hand'`. Mark the hand container `data-scroll-x` so the controller applies the direction rule, with `touch-action: pan-x` on it.
- `web/src/components/Side.svelte:7,20-33,74-120` — deck, discard and Lost piles become `dropZone('deck'|'discard'|'lost', (d) => dropOnZone(d, zone))`. Remove the `dragover`/`drop` helpers and the `over` state.
- `web/src/components/Strip.svelte:4,14-28,49,84` — the `zone(name)` helper is replaced by `dropZone(name, (d) => dropOnZone(d, name))` for `stadium` and `play`.
- `web/src/components/Modal.svelte:99,124,144` — pile, search and look cards stay draggable through `Card`'s attachment (no change beyond what `Card` gives). Check that a drag out of the modal onto the board still lands, because the modal backdrop may need `pointer-events: none` while dragging.
- `web/src/app.css` — add the global styles: `.dragging [data-drop]` outline, `[data-drop].lit` highlight, the `.drag-ghost` style, and `touch-action: none` on `.card` inside the mat.
- `web/e2e/drag.spec.js` (new):
  - hand to empty bench by touch, compared with the same drag by mouse (timeline line text);
  - every drop-zone kind;
  - stack drag from bench to Active switches;
  - a miss changes nothing;
  - a sideways swipe scrolls the hand in the phone project;
  - a short tap doesn't drag.

**Complexity:** High
**Token estimate:** ~70k tokens
**Agent strategy:** parallel analysis of the five components' current DnD wiring, then sequential integration: controller first, then components one at a time with the suite run after each.

### Task: Grouped actions in one action sheet

**Requirement attribution.** All in repo `tcgreplay`, `web/`. It covers the spec requirements "Open a card's actions by touch", "Actions fit the screen they are shown on", "Mouse behaviour stays familiar", "Actions are grouped", "Set damage without typing in a dialog" and "Toggle Special Conditions in one tap each". ACs: "A tap opens the card's actions", "The phone shows actions full screen…", "The tablet shows the card beside its actions", "Mouse hover, click, right-click and drag still work", "Actions appear in groups…", "Damage changes with no dialog", "Conditions toggle in one tap…". The design sections followed are "Menu model", "Sections for each menu" and "Containers".

**File changes:**
- `web/src/lib/store.svelte.js:37-38` — replace `menu: null` with `sheet: null` (`{model, pointerType, x, y, panel}`). Add `layout` and `landscapePhone`, derived from `matchMedia('(max-width: 639px)')`, `(pointer: coarse)` and `(orientation: landscape)` with listeners. Keep `zoom`.
- `web/src/lib/actions.js:148-186` (`handMenu`) — return `{title, card, summary, sections}`:
  - **Play** (Set as Active / Bench / Evolve…, Play, Tool attach, Energy attach; the first marked `primary`);
  - **Move** (Discard, Deck top, Deck bottom, Lost Zone);
  - **Other** (Reveal).

  In this task, Evolve and Attach keep one item per target, now grouped inside Play. The next task collapses them. Drop Zoom (182).
- `web/src/lib/actions.js:189-275` (`stackMenu`) — return the sections:
  - **Attack** (items keep calling today's flow in this task; the panel arrives in a later task);
  - **Ability**;
  - **Move** (Retreat, Switch with each benched Pokémon, Switch into Active / Promote);
  - **Damage**: a stepper with `value: stack.damage`, step 10, quick [30, 50], `set(v)` → `place_damage` with the difference. It replaces the four damage items and "Set damage…" (249-261), and removes that `ask`;
  - **Conditions** (Active only): toggles from `CONDITIONS` with `on` from `stack.conditions` (262-267);
  - **Other** (Return to hand, Devolve, Knock Out marked `danger`, last).

  Drop Zoom (274). Omit empty sections.
- `web/src/lib/actions.js:279-290` (`attachmentMenu`) — **Move** (Move to each target, To hand, Discard). Drop Zoom.
- `web/src/lib/actions.js:294-320` (`deckMenu`) — **Draw** (Draw 1, Draw N…), **Look** (Look at top N…, Search…) and **Other** (Deck list…, Shuffle). The prompts stay until the panels task.
- `web/src/components/ActionSheet.svelte` (new) — renders `app.sheet` with mode `phone | tablet | popover`. Mode is `popover` when `pointerType === 'mouse'`, else `phone` when `app.layout === 'phone'`, else `tablet`. It contains:
  - the header (card image via `cardInfo`, title, summary, "Card text ›" toggle showing `info.text` as `Zoom.svelte:24-35` does, ✕);
  - the sections as rows: buttons, a stepper (−step, the value as an `inputmode="numeric"` input, +step, quick buttons) and toggle chips;
  - closing on ✕, the backdrop, Escape, a swipe down on the header in phone mode, and `popstate`. It pushes a history entry on open.

  Popover positioning and clamping come from `web/src/components/ContextMenu.svelte:8-13`. The style follows `mobile-board/mockups.html` frames B, E, F and G. Phone rows scroll with `overflow-x: auto` and an edge mask when overflowing; tablet rows wrap, and the right column scrolls.
- `web/src/components/ContextMenu.svelte` — delete, and replace its use in `web/src/App.svelte:11,153` with `ActionSheet`.
- `web/src/components/Card.svelte:15,35-45` — `onTap(ev)`:
  - `pointerType === 'touch'` or `'pen'` and an `onmenu` handler present → `onmenu(ev)`;
  - otherwise `onclick`, or zoom as today.

  `oncontextmenu` → `onmenu(ev)` with pointerType `mouse` (unchanged behaviour). A hidden card with no handler does nothing.
- `web/src/components/Stack.svelte:22-25,58,63,68` — `menu(ev)` sets `app.sheet = {model: stackMenu(...), pointerType: ev.pointerType || 'mouse', x, y}`, and the same for `attachmentMenu`.
- `web/src/components/Hand.svelte:73` — the same for `handMenu`.
- `web/src/components/Side.svelte:44-46,81,87` — the same for `deckMenu` on tap or right-click of the deck.
- `web/src/components/Modal.svelte:95-150` — tapping a card in a pile, search or look view opens a small sheet built from `pileCardActions` (`web/src/lib/actions.js:333-344`) as one Move section. The inline buttons stay for mouse.
- `web/e2e/sheet.spec.js` (new) — the container per profile; groups and order; no Ability group; the stepper (±, quick, typed total) changes the shown damage; condition toggles; parity, comparing the touch sheet's action labels with the desktop popover's per card kind (hand Pokémon, hand Energy, Active, bench, attachment, deck); and mouse hover, click and right-click.

**Complexity:** High
**Token estimate:** ~80k tokens
**Agent strategy:** 2 parallel agents: one for the builders' sections model, one for the `ActionSheet` component against a hand-written model. Then sequential integration through the open sites and the e2e tests.

### Task: Choose targets on the board

**Requirement attribution.** All in repo `tcgreplay`, `web/`. It covers the spec requirements "Choose a target by tapping the board" and "One possible target acts straight away", and the ACs "Attaching an Energy by tapping a Pokémon", "Every destination action uses board selection" and "A single target applies at once". It also covers the success metric "three taps". The design section followed is "Target mode", with mockup frame C.

**File changes:**
- `web/src/lib/store.svelte.js` — add `target: null` (`{prompt, card, candidates, run}`).
- `web/src/lib/target.js` (new):
  - `targetItem(label, prompt, card, candidates, run)` returns `{label, run}` labelled with the candidate's name when there is one candidate, else `{label, target: {...}}`;
  - `startTarget(req)` closes the sheet, sets `app.target` and sets `app.highlight` to the candidates' top-card iids;
  - `endTarget()`.
- `web/src/lib/actions.js:131-140` — `ownStacks` also returns the top card name for labels.
- `web/src/lib/actions.js:148-186` — `handMenu`: Evolve, Tool Attach and Energy Attach become single target items over `ownStacks(seat)`.
- `web/src/lib/actions.js:189-275` — `stackMenu`: "Retreat" becomes a target item over the occupied bench slots (cost Energy chosen as today, 234-239), and "Switch, no cost" the same.
- `web/src/lib/actions.js:279-290` — `attachmentMenu`: "Move to" becomes a target item over `ownStacks(seat)`.
- `web/src/components/TargetBanner.svelte` (new) — a fixed banner under the header showing the card thumbnail, the prompt and Cancel (frame C). Escape cancels.
- `web/src/lib/pointer.svelte.js` — when `app.target` is set:
  - a tap on an element whose `data-drop` is in `candidates` runs `app.target.run(spec)` then `endTarget()`;
  - any other tap calls `endTarget()` and swallows the event;
  - drags are disabled.
- `web/src/components/Card.svelte` — fade the card whose iid is `app.target.card.iid`.
- `web/src/App.svelte` — render `TargetBanner`, and dim the non-candidate areas with a body class.
- `web/e2e/target.spec.js` (new):
  - attach by tap;
  - cancel, and a stray tap, change nothing;
  - Evolve, Retreat, Switch, Tool and Move-attachment by tap;
  - a single bench Pokémon makes Retreat immediate and named;
  - a full bench attach counts three taps;
  - mouse board selection on desktop.

**Complexity:** Medium
**Token estimate:** ~45k tokens
**Agent strategy:** single agent, sequential: the store and helpers, then the builders, then the banner and the pointer hook, then the tests.

### Task: Touch-friendly controls across the board

**Requirement attribution.** All in repo `tcgreplay`, `web/`. It covers the spec requirements "Controls are large enough to tap", "Touch does not trigger the device's own gestures", "Everything stays reachable on a phone" (timeline controls) and "Mouse behaviour stays familiar" (hover kept for the mouse). ACs: "Tap targets measure at least 44×44 CSS pixels" and "Holding a card triggers no device gesture" (the CSS is in place; real-device confirmation is manual).

**File changes:**
- `web/src/components/Card.svelte:87-116` — wrap the `.card.lift…:hover` rules in `@media (hover: hover)`. Add `-webkit-touch-callout: none; -webkit-user-select: none;` to `.card`.
- `web/src/components/Hand.svelte:105-110` — wrap the hover enlargement in `@media (hover: hover)`.
- `web/src/components/Stack.svelte:150-170` — show the `.controls` only under `@media (hover: hover)` (on touch the sheet's stepper covers them). Wrap `.att:hover` likewise.
- `web/src/components/Timeline.svelte:175-195` — under `@media (pointer: coarse)` set `.rw` and `.acts` to `opacity: 1` and enlarge their buttons to 44px.
- `web/src/components/Modal.svelte:98,123,142,173` — add an `onclick`/tap that sets `preview` alongside `onmouseenter`. Make the inline action buttons (`pileCardActions` rows) 44px under `(pointer: coarse)`.
- `web/src/app.css` — a `@media (pointer: coarse)` block setting `min-height`/`min-width: 44px` on `button`, `select`, `label.toggle` and the strip's buttons. Card targets get at least a 44px hit box via padding on their wrapper where `--cw` scales a card below 44px (`.card.xs` attachments).
- `web/src/App.svelte:183-215` — header controls get 44px under coarse pointers.
- `web/e2e/touch.spec.js` (new):
  - in the phone and tablet projects, every visible `button`, `[role=button]`, `select`, `input` and card has a bounding box of at least 44×44;
  - no hover enlargement after a tap;
  - timeline rewind and play are visible and work;
  - tapping a pile card previews it;
  - in the desktop project, hover enlarges and the quick damage buttons appear;
  - the computed style of `.card` has `-webkit-touch-callout: none` (WebKit).

**Complexity:** Medium
**Token estimate:** ~35k tokens
**Agent strategy:** single agent, sequential, one component at a time, measured with the 44px test as it goes.

### Task: Panels replace the browser dialogs

**Requirement attribution.** All in repo `tcgreplay`, `web/`. It covers the spec requirements "Attack without browser dialogs" and "Draw or look at several cards without browser dialogs", and the success metric "No action opens a browser dialog". ACs: "An attack completes with no dialog" and "Draw N and Look at top N with no dialog". The design section followed is "Panels", with mockup frame D.

**File changes:**
- `web/src/components/AttackPanel.svelte` (new):
  - the attack name and text from `cardInfo(top.def).attacks`;
  - a damage stepper starting at `parseInt(attack.damage) || 0` with ±10 and ×2;
  - a target chip defaulting to the opponent's `active`, where "Change ›" calls `startTarget` over the opponent's active and bench specs and returns to the panel with the choice;
  - a confirm button "Attack <target name> for <n>" that sends `act('attack', {name, damage, target, ...seatArgs})` as `web/src/lib/actions.js:197-207` does today;
  - for a card with no attacks, a name field plus the stepper (replacing 209-219).
- `web/src/components/CountPanel.svelte` (new) — a stepper from 1 to `deck_count` and Go. `verb: 'draw'` sends `act('draw', {n})`. `verb: 'look'` sends `act('look_top', {n, whose})` and opens the look modal as `web/src/lib/actions.js:305-312` does.
- `web/src/lib/actions.js:142-145` — delete `ask`.
- `web/src/lib/actions.js:196-219` — attack items become `{panel: {component: AttackPanel, props}}`.
- `web/src/lib/actions.js:298-313` — Draw N and Look at top N become panel items.
- `web/src/components/ActionSheet.svelte` — render `app.sheet.panel` in place of the sections, with "‹ Actions" back. Keep the sheet open while the attack panel's target mode runs, restoring the panel after the tap.
- `web/e2e/panels.spec.js` (new):
  - an attack with adjusted damage and a changed target, asserting the timeline line names the attack, damage and target;
  - an attack-less card;
  - Draw 3 and Look at top 4;
  - the stepper capped at the deck count;
  - a grep-style check that the built bundle has no `prompt(`, `confirm(` or `alert(` calls (a unit-level guard alongside the global dialog listener).

**Complexity:** Medium
**Token estimate:** ~40k tokens
**Agent strategy:** single agent, sequential.

### Task: Compact phone board

**Requirement attribution.** All in repo `tcgreplay`, `web/`. It covers the spec requirements "The board fits a phone held upright", "A phone held sideways asks to be turned" and "Everything stays reachable on a phone". ACs: "Portrait phone board fits", "Landscape phone shows the turn notice" and "Header controls reachable on a phone". The design section followed is "Board layout → Phone", with mockup frame A. The constraint "a bench of up to 8" is covered.

**File changes:**
- `web/src/App.svelte:58-160` — when `app.layout === 'phone'`:
  - the header keeps the dot, warnings and ☰; the game name, branch `<select>`, Review mode and Reveal all move into the dropdown (77-110);
  - "Show timeline" opens the timeline as a full-screen sheet (`aside` positioned fixed, with a close control) instead of a grid column (227-240, 295-297);
  - the opponent's `Hand` is replaced by a count in their counter bar.

  When `app.landscapePhone`, render a "Turn your phone upright" notice instead of `main`, keeping state.
- `web/src/app.css:1-7` — under `@media (max-width: 639px) and (orientation: portrait)`, set `--cw: calc((100vw - 32px) / 5.6)`.
- `web/src/components/CounterBar.svelte` (new) — the player name and chips for Deck n, Discard n, Lost n and Prizes n (plus the hand count for the opponent). Tap opens `deckMenu` in the sheet, the pile modal (`app.modal = {kind:'pile'}`), or the prize view (a modal listing prize cards, using `take_prize` as `web/src/components/Side.svelte:35-41` does). The chips are also drop zones for deck, discard and Lost.
- `web/src/components/Side.svelte:51-125` — in phone layout, render `CounterBar`, the Active centred, and the bench as one `overflow-x: auto` row of all `slots` (including the expansion slot from `expands`, 15-17) with a "+n ›" edge hint. Hide the prizes grid and piles column.
- `web/src/components/Hand.svelte` — in phone layout, a flat row (no fan rotation) of 72px cards in an `overflow-x: auto` strip.
- `web/src/components/Strip.svelte:48-90` — in phone layout, one compact row: the Stadium thumbnail, the turn, the coin, End turn, and Draw / End action when they apply.
- `web/e2e/phone.spec.js` (new, phone project):
  - `document.documentElement.scrollWidth <= innerWidth`;
  - counters are visible and open the right views;
  - with the 8-bench Stadium in play and 8 benched, the last is reachable by scrolling;
  - every header control is reachable through ☰;
  - the landscape notice at 844×390, cleared at 390×844;
  - in the tablet and desktop projects, no counter bar is rendered and the piles column is present.

**Complexity:** High
**Token estimate:** ~60k tokens
**Agent strategy:** 2 parallel agents: one for the header, timeline sheet and landscape notice in `App.svelte`; one for `CounterBar`, `Side`, `Hand` and `Strip` in phone layout. Then sequential integration and the phone e2e run.

## Testing Strategy

The plan adds one end-to-end browser suite under `web/e2e/`. It runs with Playwright in three projects (phone: WebKit iPhone touch; tablet: WebKit iPad touch; desktop: Chromium mouse) against the board test server, with one worker, a reset before each test, and any `dialog` failing the test. Per task:

- **Board test server and browser test harness.** A Go test of the fixed position and reset. A smoke spec in each project. The dialog guard and reset fixture, which every later spec inherits.
- **Touch dragging through one pointer layer.** A drag spec:
  - hand to bench by touch, matched against the mouse;
  - every zone kind;
  - stack switch;
  - a miss;
  - a hand swipe scrolls (phone);
  - a tap isn't a drag.
- **Grouped actions in one action sheet.** A sheet spec:
  - the container per project;
  - group order and empty groups hidden;
  - the stepper and a typed total;
  - condition toggles;
  - touch versus popover action parity per card kind;
  - mouse hover, click and right-click (desktop).
- **Choose targets on the board.** A target spec:
  - attach by tap;
  - cancel and a stray tap;
  - Evolve, Retreat, Switch, Tool and Move-attachment;
  - an immediate single candidate;
  - three taps with a full bench;
  - mouse board selection.
- **Touch-friendly controls across the board.** A touch spec:
  - the 44×44 sweep (phone, tablet);
  - no hover effects after a tap;
  - timeline buttons visible;
  - pile tap preview;
  - desktop hover kept;
  - callout CSS present.
- **Panels replace the browser dialogs.** A panels spec:
  - an attack with adjusted damage and a changed target appears in the timeline;
  - an attack-less card;
  - Draw 3 and Look at top 4;
  - the cap at deck count;
  - no `prompt(`/`confirm(`/`alert(` in the built bundle.
- **Compact phone board.** A phone spec:
  - no horizontal scroll;
  - counters open the right views;
  - 8-bench reachable;
  - header controls through ☰;
  - the landscape notice and recovery;
  - tablet and desktop unchanged.

Existing Go tests stay green throughout. Manual checks (real-device gestures, matching the mockups, a full touch-only game) go to the implementation test plan.

## Project References

- **Spec:** `20261102110545-5c84659c-touch-friendly-board`.
- **Designs:** `mobile-board/design.md` and `mobile-board/mockups.html` in the `design` source (`spektacular design read --data '{"source":"design","path":"…"}'`).
- **Knowledge:** `gotchas/board-loopback-locks.md` (repo `tcgreplay`). The test server must serve on loopback.
- **Repo:** `tcgreplay`, root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`. All work is in `web/`, plus test support under `internal/webui/`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

## Migration Notes

There are no data migrations; nothing persisted changes. Developers need Playwright's browsers once per machine (`cd web && npx playwright install webkit chromium`) before `make test` runs the browser suite. The test command prints this when they are missing. The bench-of-8 change must be committed before implementation starts, so the implement worktrees contain it.

## Performance Considerations

- The pointer layer hit-tests with `document.elementFromPoint` on `pointermove`. Throttle it to one call per animation frame so dragging stays smooth on an older iPad.
- The ghost card should move with `transform` only, not layout properties.
- Rows that scroll sideways use native overflow scrolling (`-webkit-overflow-scrolling` is not needed on current iOS).
- The browser suite runs serially against one game, so its run time grows with the test count. At about 40 tests it should stay well under two minutes.
