---
created_date: "2026-11-02"
document_status: final
closed_date: "2026-11-02"
---

# Plan: 20261102110545-5c84659c-touch-friendly-board

<!-- Metadata -->
<!-- Created: 2026-11-02T11:23:05Z -->
<!-- Commit: 19cd525 -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

Players can follow, play and review games on the board from an iPad or an iPhone by touch alone. Today the board responds only to a mouse: cards can't be dragged, menus can't be opened and numbers can't be entered by touch, and the board overflows a phone. This plan builds the agreed design in the web board. It adds touch dragging through one pointer layer, grouped card actions in a sheet sized for each screen, target selection by tapping the board, panels in place of browser dialogs, and a compact phone layout, all checked by the repo's first automated browser tests. Mouse use on the desktop keeps working as it does now.

## Conventions

No project conventions apply to this feature. The only recorded convention concerns guide text and keeping `internal/guides` in the build context. This work changes no guide, and it leaves packaging untouched: the built board still lands in `internal/webui/dist/app` for `go:embed`.

## Architecture & Design Decisions

All work lands in the single registered repo, `tcgreplay` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`), almost entirely in the Svelte board under `web/`. The shape is fixed by the design document `mobile-board/design.md` in the `design` source, with its mockups at `mobile-board/mockups.html`. This plan builds that design and does not re-derive it. Four shared pieces carry it:

1. **A pointer interaction layer.** One drag controller built on Pointer Events, exposed to components as Svelte attachments. One marks an element as draggable with today's payload `{kind, iid, def, from, role}`; the other marks an element as a drop zone carrying today's drop spec in a `data-drop` attribute. The controller:
   - tells a tap from a drag (8px), and in the hand tells a scroll from a drag by direction;
   - shows the ghost card;
   - finds the zone under the pointer with `document.elementFromPoint`, then calls the existing `dropOnStack` and `dropOnZone` unchanged.

   This replaces `startDrag`, `readDrag` and every `ondragover`/`ondrop` handler (`web/src/lib/actions.js:26-42`, `Stack.svelte`, `Hand.svelte`, `Side.svelte`, `Strip.svelte`, `Modal.svelte`). Mouse, pen and touch share it. The pointer type of the opening event decides whether a tap opens the actions (touch) or zooms the card (mouse).
2. **Target mode.** A small reactive store holds the pending action: its prompt, its candidates as drop specs, and the function to run. A banner component shows the prompt with Cancel, while the candidates glow through the same highlight the board already uses. Any tap or click that isn't a candidate cancels.
3. **The menu model.** The builders in `actions.js` (`handMenu`, `stackMenu`, `attachmentMenu`, `deckMenu`) return the design's sections model instead of flat items. Each item carries `run`, `target` or `panel`, and a section carries items, a stepper or toggles. `ask()` and every `window.prompt` are removed. The attack, Draw N and Look at top N panels are small components the sheet can show.
4. **One renderer, three containers.** An `ActionSheet` component replaces `ContextMenu.svelte` and the touch use of `Zoom.svelte`. It renders the model as:
   - the full-screen phone sheet, below 640px wide with a touch pointer;
   - the tablet panel, at 640px or wider with a touch pointer;
   - the desktop popover, for a mouse.

**Phone layout.** A layout flag (phone, tablet or desktop) derived from media queries lets `App.svelte` and `Side.svelte` swap in the compact pieces from frame A:
- counter bars instead of the piles column and the prize grid;
- a sideways-scrolling bench and hand;
- the header items moved into ☰;
- the timeline as a full-screen sheet.

Phone landscape shows the turn-upright notice instead of the board. Today's mat and its `--cw` sizing stay for tablet and desktop. CSS media features gate hover enlargement (`hover: hover`) and the 44px tap targets (`pointer: coarse`). The Timeline's hover-revealed rewind and play buttons are shown on coarse pointers, because the timeline is their only route; the design does not mention them, but the spec's "everything stays reachable" requires it.

**Browser tests.** Playwright is added to `web/`, with WebKit iPhone and iPad touch profiles and a Chromium desktop mouse profile. It runs against a real board server: a small Go test-support program under `internal/webui`. That program reuses the `fakeDB` and in-process `webui.New` pattern from `internal/webui/webui_test.go:28-105`. It serves the built board on loopback, which the loopback locks allow (knowledge `gotchas/board-loopback-locks.md`), and builds a fixed review-mode position by moving cards by iid. A reset endpoint lets each test start clean. The project's test command gains a step that builds the board and runs the suite. No server, MCP or game-model behaviour changes; the only Go code added is test support.

**Why this shape.** The alternatives were rejected (see `research.md#alternatives-considered-and-rejected`):
- **Per-component pointer handlers** would duplicate drag logic in five components and drift.
- **A separate phone app shell** would fork the board and double every fix.
- **Polyfilling HTML5 drag and drop** would keep `dataTransfer`, which can't be read during a drag and gives no tap or target mode.

The chosen shape has every gesture go through one controller and every menu through one renderer. Both drop handlers and every server verb stay as they are, which is what keeps the spec's "same result as the mouse" true by construction.

## Component Breakdown

- **Pointer interaction layer (new).**
  - Owns every pointer gesture on the board: telling a tap from a drag, telling a hand scroll from a drag, pointer capture, the floating ghost card, outlining drop zones and lighting the one underneath, and cancelling.
  - Components opt in through two attachments: one marks a card or a stack as draggable with its payload, the other marks a drop zone with its drop spec.
  - On a drop it calls the existing drop handlers.
  - On a tap it reports the pointer type to the component, which either opens the actions (touch) or zooms the card (mouse).
  - It replaces the HTML5 drag helpers and every component's dragover/drop handlers.
- **Drop handlers (unchanged in behaviour).** The two existing functions that turn a drop into a server verb stay as they are. The interaction layer calls them instead of the HTML5 drop events.
- **Target-mode store and banner (new).**
  - The store holds one pending action: its prompt, the card being played, its candidate locations and the function to run.
  - The banner shows the prompt with Cancel.
  - Candidates glow through the existing highlight mechanism, and the card being played is faded.
  - Taps on a candidate, which the drop-zone attachments already identify, complete the action; any other tap cancels.
  - The action sheet starts target mode, and the attack panel uses it to change its target and then returns.
- **Menu builders (changed).** The hand, Pokémon, attachment and deck builders return the design's grouped sections: items with run, target or panel; steppers; toggles; with primary and danger marks. The changes:
  - per-target items collapse into one target-mode item, and an item with a single candidate runs at once, labelled with that candidate;
  - prompts are removed;
  - Zoom items go.

  The pile-card buttons keep their row shape.
- **Action sheet renderer (new; replaces the context menu).**
  - Renders a menu model in one of three containers: the full-screen phone sheet, the tablet panel with the card beside its sections, or the desktop popover at the pointer.
  - Owns the sections, items, stepper, toggles, the "Card text" view, closing (✕, swipe down, backdrop, Escape, browser back) and showing panels.
  - It is opened by components in place of the context menu.
  - The Zoom view stays for mouse clicks only.
- **Panels (new).** The attack panel (damage stepper, target chip, confirm), and the Draw N and Look at top N panels (stepper bounded by the deck size, then Go). The sheet shows them; they act through the existing verbs and use target mode for the attack's target.
- **Layout flag (new, in the shared UI state).** Phone, tablet or desktop, from media queries on width and pointer, plus a phone-landscape state. The sheet and the board read it to choose their container and layout.
- **Board shell (changed).**
  - On a phone, the header keeps only the connection dot, the warnings and ☰. The branch, mode, reveal and timeline move into the ☰ menu, and the timeline opens as a full-screen sheet.
  - Phone landscape shows the turn-upright notice instead of the board.
  - Card sizing comes from width on a phone.
- **Player side (changed).**
  - On a phone, it renders counter bars (deck, discard, Lost Zone, prizes; the opponent's hand count) in place of the piles column and the prize grid. Tapping a counter opens the pile, the deck's actions or the prize view.
  - The bench becomes a sideways-scrolling row that reaches all 8 slots.
  - Its drop zones and the deck's actions use the new layers.
- **Card, Pokémon slot, hand, middle strip and pile views (changed).**
  - Each uses the interaction layer instead of HTML5 drag and drop.
  - Each opens the action sheet instead of the context menu.
  - Hover enlargement applies only where hover exists.
  - Each suppresses the iOS callout and text selection.
  - Each grows tap targets on coarse pointers.
  - The hand also scrolls sideways on a phone, and the pile views preview a card on tap.
- **Timeline (changed).** Its hover-revealed rewind and play buttons are always visible on coarse pointers.
- **Board test server (new, test support only).** A small Go program that serves the built board against a real game manager, using the fake card pool from the existing board-server tests. It builds a fixed review-mode position and offers a reset endpoint so each browser test starts from that position. Nothing in the shipped binary uses it.
- **Browser test suite (new).** Playwright tests at iPhone and iPad sizes by touch and at desktop size by mouse, started through the test server and run by the project's test command.

## Data Structures & Interfaces

Everything here is a front-end contract inside the board, except for the test server's reset endpoint. No server message, websocket verb or game type changes.

**DragPayload (unchanged shape).** What a draggable carries and what the drop handlers receive. It is today's object, so the drop handlers keep their logic.

```js
{ kind: 'card', iid, def, from, role }   // a card
{ kind: 'stack', from }                  // a whole Pokémon stack
```

**Drop spec (unchanged values).** Each drop zone is labelled with the same locations the board uses today: `p1:active`, `p1:bench:2`, `discard`, `deck`, `lost`, `stadium`, `play` and `hand`. Two attachments carry it, and each Pokémon slot carries its location so target mode can find it:

```js
draggable(payload)          // marks a card or a stack as draggable
dropZone(spec, onDrop)      // marks an element as a drop zone (data-drop = spec)
```

**MenuModel (new, replaces `{title, items}`).** What every menu builder returns and the action sheet renders:

```js
{
  title, card, summary,                 // header: the card shown, a one-line state
  sections: [ Section ],                // empty sections are omitted by the builder
}
Section = { label, items: [Item] }
        | { label, stepper: { value, step, quick: [n], min, max, set(v) } }
        | { label, toggles: [ { label, on, set(on) } ] }
Item    = { label, detail?, primary?, danger?,
            run? | target?: TargetRequest | panel?: Panel }
```

An item has exactly one of `run`, `target` or `panel`.

**TargetRequest (new).** The input to target mode:

```js
{ prompt, card, candidates: [dropSpec], run(spec) }
```

With exactly one candidate, the builder emits a `run` item labelled with that candidate instead.

**Panel (new).** A component plus its props, shown inside the sheet in place of the sections, with a way back. There are three: `AttackPanel {stack, attack}`, `CountPanel {verb: 'draw' | 'look', seat, max}`, and the Card text view.

**Layout (new, in the shared UI state).**

```js
app.layout: 'phone' | 'tablet' | 'desktop'
app.landscapePhone: boolean
```

Derived from width (below 640px is a phone) and from the pointer media features. The sheet also takes the opening event's pointer type, so a mouse always gets the popover.

**Sheet state (replaces `app.menu`).** What is open:

```js
app.sheet = { model: MenuModel, pointerType, x, y, panel? }   // or null
```

`x` and `y` place the desktop popover.

**Target-mode state (new).**

```js
app.target = TargetRequest   // or null
```

**Test server reset (new, test support only).** A loopback-only HTTP endpoint on the board test server, `POST /test/reset`. It discards the game and rebuilds the fixed position, so each browser test starts from the same board. It exists only in the test-support program, never in the shipped binary.

## Implementation Detail

- **New pattern: gestures through attachments, not event props.**
  - Today each component wires `draggable`, `ondragstart`, `ondragover`, `ondrop` and `oncontextmenu` itself. After this change a component declares intent once: this element drags with this payload, that element is a drop zone for this location, a tap here opens these actions.
  - The interaction layer owns everything stateful about a gesture: the pointer that went down, the threshold, capture, the ghost, the lit zone and cancellation.
  - A reader of a board component sees what a card does, not how a drag is tracked.
  - Drop results stay in the two existing drop handlers, so the "what happens when X lands on Y" logic is untouched and stays in one place.
- **New pattern: menus are data, containers are presentation.**
  - The menu builders already compute what a card can do. They now say it in the grouped model instead of a flat list of labels.
  - Every builder follows the design's section order, so a reviewer can check a builder against the design section by section.
  - A builder never decides how its actions look on a phone or a desktop; the sheet does.
  - Builders lose their prompts. A number or a target becomes a stepper, a panel or a target request.
- **New pattern: actions that finish on the board.** Target mode is a small global state alongside the existing highlight, toast and modal state: at most one pending action, cancelled by anything that isn't a candidate. It follows the existing store pattern (one shared reactive object, components read and write it) rather than introducing an event bus.
- **Layout by flag plus CSS, not by forked components.**
  - The compact phone pieces (counter bars, the scrolling bench and hand, the reduced header) are branches inside the existing shell and side components, chosen by the layout flag.
  - Fine-grained differences stay in CSS media features:
    - tap-target size under a coarse pointer;
    - hover enlargement only where hover exists;
    - `touch-action` per area;
    - callout and selection suppression on cards.
  - Tablet and desktop keep today's mat and `--cw` sizing.
- **Existing patterns kept:**
  - shared UI state in the one store;
  - server verbs through the one websocket `act` helper;
  - card data fetched lazily by definition;
  - the Zoom view for mouse clicks;
  - the modal views for piles, search, look and deck list (gaining tap preview and larger buttons).
- **Removed:** the HTML5 drag helpers and their data-transfer type, the context-menu component, the `ask` prompt helper, and every hover-only path that has no touch equivalent.
- **Test support follows the Go test pattern.** The board test server reuses the in-process server construction and the fake card pool from the board-server tests, moved into a small shared test-support package so the Go tests and the program use one copy. The browser suite is the repo's first JavaScript test suite. It lives beside the board, drives it the way a player would (taps, touch drags, clicks), and asserts what is visible on the board and in the timeline, not internal state.

## Dependencies

- **`mobile-board/design.md` from the `design` design source.** The settled design this plan implements. It fixes the gestures, target mode, the menu model and each menu's sections, the three containers, the panels, the compact phone layout, the 640px breakpoint and the test approach. No changes.
- **`mobile-board/mockups.html` from the `design` design source.** Frames A–H, the visual reference for each container, target mode, the attack panel, the phone board and touch dragging. No changes.
- **The bench-of-8 change (`MaxBench`, the advisory bench warning, the extra bench slot on the board).** It must be committed before implementation starts. The spec requires a bench of up to 8, and it is currently uncommitted in the working tree. Implementation runs in a git worktree branched from the last commit, so without that commit the worktree would not contain it.
- **Svelte 5 (5.57.1) and Vite (8.3.1)**, already in the board. Svelte's attachments (`{@attach}`) carry the pointer layer. No version change.
- **Playwright (`@playwright/test`, new dev dependency in `web/`).**
  - Provides the touch device profiles (WebKit iPhone and iPad), the desktop Chromium profile, `tap`, touch drags and `webServer`.
  - Pin an exact release, at least two weeks old, in `package.json` and the lockfile.
  - Its browsers are installed once per machine.
- **The board server, game manager and card interfaces (`internal/webui`, `internal/game`, `internal/cards`).** The board test server reuses them in-process with a fake card pool. No behaviour changes; the fake pool moves into a test-support package.
- **Every existing board verb over the websocket.** The new interactions send the same verbs with the same arguments. No server change.
- **Knowledge `gotchas/board-loopback-locks.md`.** The test server serves on `127.0.0.1`, which satisfies the board's Host and Origin checks.
- **Prior plans.** None must land first. The wait-for-turn plan left the board's opponent badges and lock (unchanged here). It also recorded "no automated tests for the board app" as out of scope, which this plan now changes, as the spec requires.

## Testing Approach

**Kinds of tests.**
- **End-to-end browser tests (new, the main coverage).** Playwright drives the real built board against the board test server, which runs a real game manager with the fake card pool from a fixed review-mode position, reset before each test. Three profiles:
  - an iPhone-sized touch device (WebKit);
  - an iPad-sized touch device (WebKit);
  - a desktop mouse (Chromium).

  Tests act as a player would (tap, touch drag, click, right-click) and assert what the player sees: cards in their new places, the damage and conditions on a Pokémon, new timeline lines, which container opened, and its scroll behaviour.
- **Go tests (existing, kept green).** The board server, game and MCP tests are unchanged apart from importing the fake card pool from its new shared home. A small Go test checks that the board test server builds its fixed position and that reset restores it.
- **No unit tests for the menu builders in isolation.** The builders are exercised through the sheet in the browser tests. The design's section order is asserted there, where it is visible.

**Where coverage concentrates, and the load-bearing assertions.**
- **The pointer layer:**
  - a touch drag from hand to bench, and to each kind of drop zone, produces the same timeline line as the same mouse drag;
  - a sideways swipe in an overflowing hand scrolls without moving a card;
  - a release outside any zone changes nothing.
- **Target mode:**
  - attaching an Energy highlights every Pokémon in play, and a tap attaches it;
  - Cancel and a stray tap leave the board and timeline unchanged;
  - with one benched Pokémon, Retreat names it and acts at once;
  - Evolve, Switch, moving an attachment and changing an attack's target all use board selection.
- **The containers:**
  - at 390×844 by touch, the sheet covers the screen and an overflowing row scrolls sideways;
  - at 1024×768 by touch, the card sits left of the actions and only the actions scroll;
  - at desktop by mouse, hover enlarges, click zooms, right-click opens the grouped popover at the pointer, and drag moves.
- **The menu model:**
  - for each kind of card, the touch sheet offers the same actions as the desktop popover, apart from Zoom and the per-target items;
  - groups appear in the design's order, with empty groups hidden;
  - no browser dialog opens anywhere: the tests fail on any `dialog` event.
- **The panels:**
  - an attack with adjusted damage and a changed target records that attack, damage and target;
  - the damage stepper and a typed total change the shown damage;
  - condition toggles mark and clear;
  - Draw N and Look at top N cannot exceed the deck.
- **The phone layout:**
  - no horizontal page scroll at 390×844;
  - counters visible and openable;
  - all 8 bench slots reachable;
  - header controls reachable through ☰;
  - the landscape notice at 844×390, cleared on rotating back;
  - every control and card target at least 44×44 CSS px.

**Fitting the project's conventions.** Go tests stay table-light and behaviour-first, as now. The browser suite is the repo's first, so its conventions (one spec file per area, a reset per test, selectors by role and accessible name rather than CSS classes) are set here. The project's test command runs Go vet and tests, builds the board, and runs the browser suite. If Playwright's browsers are missing, it says so clearly.

**Deliberate gaps.**
- iOS's own long-touch callout and text selection cannot be observed in emulation, so their suppression is a manual check.
- Emulated touch may not match real iOS for telling a scroll from a drag, so real devices get a manual pass.
- Visual polish against the mockups is a manual review.

**Success metrics.**
- *A full play-mode game on an iPad by touch alone*: **Manual — captured in the implementation test plan**. The browser suite covers each interaction a game needs at iPad size by touch.
- *Following and acting in a game on an iPhone in portrait, without zooming or sideways scrolling*: **Behavioural test** (no horizontal scroll, every action reachable at 390×844 by touch), plus **Manual — captured in the implementation test plan** for a real-device session.
- *No action opens a browser dialog on any device*: **Behavioural test**. Every browser test fails on any `dialog` event, across all three profiles.
- *Every desktop menu action reachable by touch on iPad and iPhone*: **Behavioural test**. For each kind of card, the touch sheet's actions are compared with the desktop popover's.
- *Attaching an Energy to a benched Pokémon in at most three taps, however many Pokémon are in play*: **Behavioural test**. The attach flow with a full bench is counted in taps.
- *The automated touch and mouse tests pass on every run of the project's test command*: **Behavioural test**. The suite runs inside the project's test command.

**Manual reviews.**
- **Manual — captured in the implementation test plan**: on a real iPhone and iPad, touching and holding a card shows no callout or text selection, and swiping the hand scrolls while dragging up lifts a card.
- **Manual — captured in the implementation test plan**: the phone sheet, tablet panel, desktop popover, target banner, attack panel and compact phone board match the design's mockups (frames A–H) closely enough to be accepted.
- **Manual — captured in the implementation test plan**: a full play-mode game against the assistant on an iPad by touch alone, and a game followed and acted in on an iPhone in portrait.

## Milestones & Tasks

### Milestone 1: Cards can be dragged by touch

**What changes:**
- On an iPad, or any touch screen, the player can drag cards with a finger to every place a mouse can drop them today, with the same result. While dragging, every place the card can go is outlined, and letting go elsewhere leaves it where it was.
- Mouse dragging works exactly as before, through the same code.
- The repo also gains its first automated browser tests, run by the project's test command against a real board server. Every later milestone is checked by them.

**Validation point:** the browser suite passes at iPhone and iPad sizes by touch and at desktop size by mouse, covering a hand-to-bench drag, every drop-zone kind, a sideways hand scroll and a missed drop. The project's test command runs the suite.

#### - [x] Task: Board test server and browser test harness
**Id:** 37b4af36-4707-4337-905c-f9d98208a377
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

This gives the repo its first browser tests, so every later task can be checked by touch and by mouse. A small Go test-support program serves the built board against a real game, using the fake card pool the board-server tests already use. It builds a fixed review-mode position (cards in hand, an Active, a bench, Energy, a deck and a Stadium) and resets it between tests. Playwright is added to the board with iPhone, iPad and desktop profiles, and the project's test command runs it.

*Technical detail:* [context.md#task-board-test-server-and-browser-test-harness](./context.md#task-board-test-server-and-browser-test-harness)

**Acceptance criteria**:
- [x] Running the project's test command builds the board and runs a browser suite at iPhone and iPad sizes by touch and at desktop size by mouse, after the existing Go checks.
- [x] Every browser test starts from the same known board, whatever earlier tests changed.
- [x] A smoke test in each profile loads the board and sees both players' names, the cards in hand and the Pokémon in play from the fixed position.
- [x] Any browser dialog opening during any test fails that test.
- [x] When the browsers Playwright needs are not installed, the test command says how to install them instead of failing obscurely.
- [x] The existing Go tests still pass, with the fake card pool shared from one place rather than copied.
- [x] Nothing in the shipped binary includes the test server or its reset endpoint.

#### - [x] Task: Touch dragging through one pointer layer
**Id:** 1cefbf09-35c1-47fd-a334-03cdf2c3bac6
**Repo:** tcgreplay
**Depends on:**
- 37b4af36-4707-4337-905c-f9d98208a377 — Board test server and browser test harness
**Execution:** agent

This replaces the board's HTML5 drag and drop with one Pointer Events layer that mouse, pen and touch all use, so cards can be dragged with a finger. Every card, Pokémon stack and pile view card becomes draggable through it. Every drop place (Active, bench slots, Pokémon, discard, deck, Lost Zone, Stadium, play area, hand) becomes a drop zone that hands the drop to the existing drop handlers. A ghost card follows the finger, zones are outlined while dragging, and a miss cancels. In the hand, a sideways swipe scrolls and an upward drag lifts a card.

*Technical detail:* [context.md#task-touch-dragging-through-one-pointer-layer](./context.md#task-touch-dragging-through-one-pointer-layer)

**Acceptance criteria**:
- [x] By touch at iPhone and iPad sizes, dragging a Pokémon from the hand to an empty bench slot benches it, and the timeline shows the same line a mouse drag produces.
- [x] By touch, a card can be dropped on the Active Spot, a bench slot, a Pokémon in play, the discard pile, the deck, the Lost Zone, the Stadium, the play area and the hand, each with the same result as the same mouse drop.
- [x] Dragging a benched Pokémon onto the Active Spot switches them, by touch and by mouse, as a stack drag does today.
- [x] While a card is being dragged, every drop place is outlined and the one under the pointer is highlighted. Releasing over none leaves the board and timeline unchanged.
- [x] On a phone-sized screen with an overflowing hand, a sideways swipe scrolls the hand and moves no card, while an upward drag from a card lifts it.
- [x] Mouse dragging on the desktop behaves as before, and the play area still appears while a card is being dragged.
- [x] A tap that barely moves is treated as a tap, not a drag.

### Milestone 2: Every card's actions open by touch, grouped and sized for the screen

**What changes:**
- Tapping a card opens its actions with the card and its text: full screen on a phone, beside the card on a tablet, and still a popover at the pointer with a mouse.
- Actions are grouped (attacks, abilities, moving, damage, conditions, other). Damage is a stepper, and conditions are one-tap toggles.
- Anything that needs a destination is one action followed by a tap on the board, with the candidates highlighted and Cancel always available.
- Touch also stops triggering the device's own callout and selection, controls grow to a comfortable tap size, and the timeline's rewind and play buttons become reachable without hover.
- With this milestone the iPad is fully usable by touch, apart from the attack and draw dialogs.

**Validation point:**
- The browser suite shows that a tap opens the right container at each size, and that the touch actions match the desktop popover's for every kind of card.
- Attach, Evolve, Retreat, Switch and moving an attachment work through board selection, a single candidate acts at once, and cancel changes nothing.
- The damage stepper and condition toggles work.
- Every control measures at least 44×44 CSS px.

#### - [x] Task: Grouped actions in one action sheet
**Id:** 74c41358-d09b-4a53-ad85-e86aaae51763
**Repo:** tcgreplay
**Depends on:**
- 1cefbf09-35c1-47fd-a334-03cdf2c3bac6 — Touch dragging through one pointer layer
**Execution:** agent

The menus for a card in hand, a Pokémon in play, an attached card and the deck are rebuilt as the design's grouped sections, and one action sheet renders them. On a phone the sheet fills the screen, with rows that scroll sideways. On a tablet the card sits beside its actions. With a mouse it is the popover at the pointer. Tapping a card by touch opens the sheet with the card and its text; with a mouse a click still zooms and a right-click opens the popover. Damage becomes a stepper and conditions become toggles, and the old context menu and Zoom items are removed. Actions that need a target still list their targets in this task; the next task turns them into board selection.

*Technical detail:* [context.md#task-grouped-actions-in-one-action-sheet](./context.md#task-grouped-actions-in-one-action-sheet)

**Acceptance criteria**:
- [x] By touch, tapping a card in hand, a Pokémon in play, an attached card, the deck and a card in a pile view each opens its actions, showing the card and offering its text.
- [x] At 390×844 by touch, the sheet covers the screen, and a group whose actions overflow scrolls sideways and shows that more are off screen.
- [x] At 1024×768 by touch, the card is on the left and the actions on the right, and only the actions scroll when they are taller than the panel.
- [x] With a mouse, hovering over a card enlarges it, clicking shows it enlarged with its text, and right-clicking opens the grouped actions at the pointer. A device with both touch and a mouse behaves each way as it is used.
- [x] The actions are grouped in the design's order, with the common action first and Knock Out last. An Active Pokémon with two attacks and no Ability shows no Ability group.
- [x] Damage can be raised by 10, 30 or 50, lowered by 10, or typed as a total, and the board shows the new damage.
- [x] Each Special Condition on an Active Pokémon toggles on and off with one tap, and the toggle shows its state.
- [x] For every kind of card, the touch sheet offers the same actions as the desktop popover, and both offer every action today's menus do, apart from Zoom.

#### - [x] Task: Choose targets on the board
**Id:** 46ddd96f-8e7f-416b-bb51-174dfb66facb
**Repo:** tcgreplay
**Depends on:**
- 74c41358-d09b-4a53-ad85-e86aaae51763 — Grouped actions in one action sheet
**Execution:** agent

Actions that need a destination become one action followed by a tap on the board. Choosing Attach, Evolve, Retreat, Switch or Move closes the sheet, shows a banner naming the action and card with Cancel, highlights the candidates and fades the card being played. A tap on a candidate completes the action; anything else cancels. When there is only one candidate, the action is labelled with it and acts at once. This works the same way with a mouse.

*Technical detail:* [context.md#task-choose-targets-on-the-board](./context.md#task-choose-targets-on-the-board)

**Acceptance criteria**:
- [x] Choosing Attach for an Energy in hand shows a banner naming the Energy, highlights every Pokémon the player has in play, and attaches it to the one tapped.
- [x] Cancelling, or tapping anything other than a highlighted candidate, attaches nothing and adds nothing to the timeline.
- [x] Evolve, Retreat, Switch (no cost), Tool attach and moving an attached card each pick their destination on the board, offering the same candidates today's menus list.
- [x] With exactly one Pokémon on the bench, the Active Pokémon's Retreat names that Pokémon and retreats to it straight away.
- [x] With a full bench, attaching an Energy to a benched Pokémon takes three taps: the Energy, Attach and the Pokémon.
- [x] Board selection works the same way with a mouse on the desktop.

#### - [x] Task: Touch-friendly controls across the board
**Id:** bee420d0-abcb-4806-92f9-5b24f8d94e70
**Repo:** tcgreplay
**Depends on:**
- 74c41358-d09b-4a53-ad85-e86aaae51763 — Grouped actions in one action sheet
**Execution:** agent

This finishes the touch experience outside the sheet:
- Hover enlargement and the hover-only damage buttons apply only where a hover exists.
- Every control and card target grows to at least 44 by 44 pixels on a touch screen.
- Cards suppress the device's long-touch callout and text selection.
- The timeline's rewind and play buttons stay visible on touch, because the timeline is the only place they live.
- Pile views preview a card on tap instead of on mouse-over.

*Technical detail:* [context.md#task-touch-friendly-controls-across-the-board](./context.md#task-touch-friendly-controls-across-the-board)

**Acceptance criteria**:
- [x] At iPhone and iPad sizes by touch, every button, toggle and card target the board shows measures at least 44×44 CSS pixels.
- [x] By touch, cards do not enlarge or reveal hover-only buttons when touched. With a mouse on the desktop, hover still enlarges cards and shows the Pokémon's quick damage buttons.
- [x] By touch, the timeline's rewind and play-from-here buttons are visible and work.
- [x] By touch, tapping a card in a discard, search, look or deck-list view shows its large preview.
- [x] Cards are marked so that touching and holding one selects no text and opens no system callout.

### Milestone 3: No browser dialogs anywhere

**What changes:**
- Attacking, setting a damage total, drawing several cards and looking at the top of the deck no longer open browser dialogs.
- An attack opens a panel with its printed damage to adjust, a target defaulting to the opponent's Active (changeable on the board), and a confirm button that names the result.
- Draw N and Look at top N pick a count on screen that cannot exceed the deck.

**Validation point:** the browser suite completes an attack with adjusted damage and a changed target and sees it in the timeline. It draws 3 and looks at the top 4 by count panels. Every browser test fails if any dialog opens, and none does.

#### - [x] Task: Panels replace the browser dialogs
**Id:** c78d2ada-7f25-4611-b6d3-1a2c4faa588c
**Repo:** tcgreplay
**Depends on:**
- 46ddd96f-8e7f-416b-bb51-174dfb66facb — Choose targets on the board
**Execution:** agent

The remaining browser dialogs are replaced with panels inside the action sheet:
- **Attack.** Choosing an attack opens a panel showing its printed damage (adjustable, with ×2 for Weakness). The target defaults to the opponent's Active and can be changed by tapping the board. One confirm button names the target and damage.
- **Draw N and Look at top N.** These pick a count on screen, limited to the cards left in the deck.

The prompt helper is removed, so no part of the board opens a browser dialog.

*Technical detail:* [context.md#task-panels-replace-the-browser-dialogs](./context.md#task-panels-replace-the-browser-dialogs)

**Acceptance criteria**:
- [x] Choosing an attack shows its printed damage. The player can change the damage, keep the opponent's Active or pick another opposing Pokémon on the board, and confirm. The timeline then records that attack with that damage and target.
- [x] A Pokémon with no attack data offers a way to name an attack and its damage inside the sheet.
- [x] From the deck's actions, the player can draw 3 cards and look at the top 4 by choosing the count on screen, and cannot choose more than the cards left in the deck.
- [x] No browser dialog opens anywhere on the board, at any size, by touch or by mouse.

### Milestone 4: The board fits a phone

**What changes:**
- On an iPhone held upright, the board reorganises to fit the screen with no sideways scrolling:
  - counter bars for each player's deck, discard, Lost Zone and prizes, which open their piles when tapped;
  - a bench row that scrolls to all 8 slots;
  - a larger hand strip along the bottom;
  - a header whose less-used controls and timeline live in the ☰ menu.
- Held sideways, the phone asks to be turned upright.
- Tablets and desktops keep today's mat.

**Validation point:** the browser suite shows the following at 390×844:
- no horizontal page scroll;
- counters visible and openable;
- all 8 bench slots reachable;
- every header control reachable through ☰;
- the landscape notice at 844×390, cleared when turned back.

The earlier milestones' tests still pass at phone size.

#### - [x] Task: Compact phone board
**Id:** a2ce16fe-f973-448a-ae28-39f1c9be3bbc
**Repo:** tcgreplay
**Depends on:**
- 74c41358-d09b-4a53-ad85-e86aaae51763 — Grouped actions in one action sheet
- bee420d0-abcb-4806-92f9-5b24f8d94e70 — Touch-friendly controls across the board
**Execution:** agent

On a phone held upright, the board switches to the design's compact layout:
- each player has a counter bar for deck, discard, Lost Zone and prizes (and the opponent's hand count) that opens the matching pile, deck actions or prize view;
- the bench is a sideways-scrolling row that reaches all 8 slots;
- the hand is a larger strip along the bottom;
- the header keeps only the connection dot, the warnings and ☰, with the branch, mode, reveal and timeline moved into the menu and the timeline opening full screen.

Held sideways, the phone shows a notice to turn it upright. Tablets and desktops keep today's mat.

*Technical detail:* [context.md#task-compact-phone-board](./context.md#task-compact-phone-board)

**Acceptance criteria**:
- [x] At 390×844 in portrait, the page has no horizontal scroll, and cards in play and in hand are large enough to read and tap.
- [x] Both players' deck, discard, Lost Zone and prize counts are visible, and tapping each opens that pile, the deck's actions or the prize view.
- [x] With 8 Pokémon on a bench (Area Zero Underdepths in play), all 8 can be reached by scrolling the bench row.
- [x] The branch selector, review mode, reveal all, the timeline, the deck list and, on the hosted playmat, sign out can each be reached and used on a phone.
- [x] At 844×390 on a phone, the board is replaced by a notice to turn the phone upright, and turning it back shows the board with nothing lost.
- [x] At tablet and desktop sizes, the board looks and behaves as before this task.
- [x] The touch drag, action sheet, target selection and panel tests all still pass at phone size.

## Open Questions

- **Can Playwright's WebKit touch emulation drive a pointer drag reliably?** This depends on how Playwright's touchscreen and dispatched pointer events reach Pointer Events handlers in WebKit, which only shows up by running the suite.
  - If touch drags can't be driven in WebKit, run the touch drag tests in Chromium with touch emulation (`hasTouch`, an iPhone-sized viewport) and keep WebKit for taps and layout. Note it in the implementation.
  - If neither engine can drive a touch drag, STOP and ask the user, because the spec's automated touch criteria would then rest on manual checks.
- **Does iOS Safari honour the scroll-versus-drag rule in the hand?** This depends on real-device behaviour of `touch-action: pan-x` together with pointer capture.
  - If on a real iPad or iPhone the hand won't scroll, or won't let an upward drag lift a card, try adjusting the threshold and capture timing within the design's gesture model.
  - If the gesture model itself has to change (for example to a long press), STOP and ask the user: the design and the spec's non-goals rule long press out.
- **Can the fixed test position be laid out with existing verbs alone?** This depends on whether review-mode `move`, `place_active`, `bench` and `attach` by iid can place every card the tests need (including a Stadium and 8 benched Pokémon), regardless of the shuffle.
  - If a needed position can't be reached without a game-model change (such as a seeded shuffle), STOP and ask the user, because the spec leaves the game model unchanged.

## Out of Scope

- **No long-press gestures** (spec Non-Goals; the design). Tap opens the actions, drag moves cards, and nothing waits on a hold.
- **No home-screen web app.** No manifest, service worker or offline mode (spec Non-Goals).
- **No layout for a phone held sideways.** Landscape on a phone shows only the turn-upright notice (spec requirement; the design).
- **No changes to the assistant's side.** The MCP tools, the coaching guides, the game model and the board server's behaviour are unchanged (spec Non-Goals). The only Go code added is test support that the shipped binary never includes.
- **No new card actions.** Every action the board offers today stays, reached a different way. Zoom's job moves to the card shown in the sheet (spec Non-Goals).
- **No rules enforcement in target selection.** Candidates are exactly those today's menus list (spec Constraints).
- **No desktop redesign** beyond the grouped popover, board target selection and the hover gating that touch requires (spec Non-Goals).
- **No visual regression or screenshot tests.** The browser suite asserts behaviour and measurements. How closely the containers match the mockups is a manual review in the implementation test plan.
- **No continuous integration setup.** The repo has no CI today; the suite runs in the project's test command. Adding a CI workflow would be its own piece of work.

## Changelog

### 2026-11-02 — Task: Board test server and browser test harness

**What was done**: Added the repo's first browser test suite. A Go test-support program (`boardtest`) serves the built board over the fake card pool from a fixed review-mode position, with `POST /test/reset` to rebuild it. Playwright 1.64.1 runs phone, tablet and desktop profiles against it from `make test`, after the Go checks. Every test resets the board first and fails if a browser dialog opens.

**Deviations**:
- **Touch profiles run in Chromium, not WebKit** (user decision). Playwright's WebKit build needs Ubuntu library versions (libicu74, an older libxml2, libflite1, libbacktrace) and won't launch on this Arch machine. The phone and tablet profiles keep the iPhone 13 and iPad sizes, `hasTouch` and `isMobile`, but use Chromium's touch emulation. Only Chromium needs installing, and the missing-browser hint says `npx playwright install chromium`. Real iOS Safari stays a manual check.
- **The fake card pool now lives in `internal/cards/cardstest`**, not `internal/webui/webtest/fakedb.go`. It was copied in three test packages (game, webui, usergames), not one, and all three now import the shared pool. It merges the copies and adds Makuhita (`tst-7`, a Basic with two attacks and no Ability) and Deep Zone (`tst-8`, a Stadium that expands the bench to 8). It has no image URLs, so browser tests make no network calls.
- **The position code lives in `internal/webui/webtest/webtest.go`, tested by `webtest_test.go`** (in place of `position_test.go`, to follow the repo's `foo_test.go` naming). The test also checks that the shipped binary depends on neither `webtest` nor `cardstest` (`go list -deps`).
- **The browser check is a script, `web/scripts/check-browsers.js`.** It runs first in `npm run test:e2e`, which `make test` calls, instead of living inline in the Makefile.
- **An extra `web/e2e/harness.spec.js`** proves that reset works between tests and that the dialog guard fails a test. The smoke spec also asserts that the touch profiles report `pointer: coarse` with no hover, and the desktop a fine pointer with hover.

**Files changed**:
- `Makefile`
- `.gitignore`
- `internal/cards/cardstest/cardstest.go` (new)
- `internal/webui/webtest/webtest.go` (new)
- `internal/webui/webtest/webtest_test.go` (new)
- `internal/webui/webtest/cmd/boardtest/main.go` (new)
- `internal/webui/webui_test.go`
- `internal/game/game_test.go`
- `internal/game/manager_test.go`
- `internal/game/presence_respond_test.go`
- `internal/usergames/usergames_test.go`
- `internal/usergames/board_test.go`
- `web/package.json`
- `web/package-lock.json`
- `web/playwright.config.js` (new)
- `web/scripts/check-browsers.js` (new)
- `web/e2e/fixtures.js` (new)
- `web/e2e/smoke.spec.js` (new)
- `web/e2e/harness.spec.js` (new)

**Discoveries**:
- Playwright's `touchscreen` API only has `tap()`, so a touch drag can't be driven natively. `fixtures.js` `touchDrag` dispatches synthetic `PointerEvent`s (`pointerType: 'touch'`) at `elementFromPoint` for each step, and they bubble. The pointer layer in the next task must therefore listen for `pointermove` and `pointerup` on `window`, and must tolerate `setPointerCapture` throwing for a synthetic pointer. Synthetic events can't trigger native scrolling, so the hand-swipe test will need another approach (for example, asserting `touch-action` plus `scrollLeft` after a programmatic scroll, or Chromium CDP touch input).
- The fixed position (iids by deck order) is documented in `webtest.go`. In the hand, p1-2 is Riolu, p1-3 Lucario, p1-17 and p1-18 Fighting Energy, p1-9 Band, p1-7 Iono, p1-12 Deep Zone and p1-13 Ultra Ball. In play are p1-5 (Makuhita, Active, with Energy p1-15), p1-1 (Riolu, bench 1) and p1-6 (Makuhita, bench 2). On p2's side, p2-5 is Active and p2-1 on the bench. Arena (p1-11) is in play, and it is turn 1 for p1, in review mode.
- `npm audit` already reports advisories against vite 8.3.1 and deepmerge (via vite-plugin-svelte), not caused by this change.

### 2026-11-02 — Task: Touch dragging through one pointer layer

**What was done**: Replaced the board's HTML5 drag and drop with one Pointer Events controller, `web/src/lib/pointer.svelte.js`. It exposes two Svelte attachments, `draggable(payload)` and `dropZone(spec, onDrop)`, and mouse, pen and touch all go through it:
- Cards, Pokémon stacks and pile-view cards drag through it.
- Every drop place (Active, bench, Pokémon, discard, deck, Lost Zone, Stadium, play area, hand) hands the payload to the unchanged `dropOnStack` and `dropOnZone`.
- A ghost follows the pointer, raised half a card for touch. Every zone is outlined and the one underneath is lit.
- A release elsewhere, Escape or a cancelled pointer changes nothing.
- In the hand, a sideways or downward finger scrolls and an upward one lifts the card.

**Deviations**:
- **The pointer is captured on the document, not on the dragged card.** The design only says "the pointer is captured". A test found that capturing the card kept its desktop hover enlargement during the drag, so `elementFromPoint` hit the enlarged card and a drop meant for the bench landed on the hand. As a further fix, all hover enlargement (`Card.svelte`, `Hand.svelte`) is now off while `body.dragging` is set, and the drag source has `pointer-events: none`.
- **Taps stay native clicks in this task.** There is no `onTap` callback yet; the sheet task adds tap routing by pointer type. After a real drag, the controller swallows the one click that follows.
- **The hand scrolls sideways only below 640px wide** (`overflow-x: auto; justify-content: safe center`). At wider sizes the hand still fans as before, and the desktop's hover enlargement isn't clipped.
- **Pile, search and look views fade and let the pointer through while a drag lasts.** A card dragged out of them lands on the board; before, the view's backdrop blocked it.
- **The test helpers now use real touch.** They drive touch through Chromium's DevTools protocol (`Input.dispatchTouchEvent`) rather than synthetic events, because every profile runs in Chromium. `act()` moved from `harness.spec.js` to `fixtures.js`.

**Files changed**:
- `web/src/lib/pointer.svelte.js` (new)
- `web/src/lib/actions.js`
- `web/src/components/Card.svelte`
- `web/src/components/Stack.svelte`
- `web/src/components/Hand.svelte`
- `web/src/components/Side.svelte`
- `web/src/components/Strip.svelte`
- `web/src/components/Modal.svelte`
- `web/src/app.css`
- `web/e2e/fixtures.js`
- `web/e2e/harness.spec.js`
- `web/e2e/drag.spec.js` (new)

**Discoveries**:
- **Pointer capture on an element keeps it `:hover` for the whole drag**, so any hover transform stays applied and can cover the drop zones. Capture the document instead, and switch hover effects off while dragging.
- **The play area mounts only once a card drag has started**, so drop zones must be hit-tested at move time and never cached at the start. Tests reach it by lifting the card first (the `LIFT` step in `fixtures.js`).
- **What each drop records** (handy for later tests):
  - an empty-bench drop from the hand is `p1 moved p1-2 Riolu p1:hand → p1:bench:2` (a move, not a bench verb);
  - an Energy drop is `… → p1:active as energy`;
  - a stack switch is `p1 swapped p1:bench:0 (Riolu) with p1:active (Makuhita)`.
- **Real CDP touch works with `isMobile` and `hasTouch` in Chromium**: `touch-action`, native scrolling and implicit capture all behave as on a device.


### 2026-11-02 — Task: Grouped actions in one action sheet

**What was done**:
- **The menu builders return groups.** `handMenu`, `stackMenu`, `attachmentMenu` and `deckMenu` now return `{title, card, text, summary, sections}`, grouped in the design's order. Empty groups are left out, Knock Out is last and marked danger, the Zoom items are gone, damage is a stepper (−10/+10/+30/+50, or type a total) and Special Conditions are toggles.
- **One `ActionSheet.svelte` replaces `ContextMenu.svelte`.** It shows the model as:
  - the full-screen phone sheet, whose rows scroll sideways with an edge fade;
  - the tablet panel, with the card and a Card | Text switch on the left and actions that scroll on their own;
  - the desktop popover at the pointer.

  It closes with ✕, the backdrop, Escape, a swipe down on the phone header and browser Back.
- **Taps and clicks.** A touch or pen tap on a card opens the sheet. A mouse click still zooms the card, and a right-click opens the popover.
- **Pile, search and look views.** Cards there open a sheet with one Move group that mirrors their buttons; the buttons stay for the mouse.
- **The store.** It gains `app.sheet`, `app.layout` (phone, tablet or desktop), `app.landscapePhone`, `openSheet` and `pointerTypeOf`.

**Deviations**:
- **The sheet state holds a builder, not a model.** `app.sheet` is `{build, pointerType, x, y}` rather than `{model, …}`, and the sheet re-runs `build()` as the board changes. The stepper and toggles therefore show the live damage and conditions while the sheet stays open. The sheet also closes itself if its card leaves play.
- **Taps are routed by the click's pointer type, not by an `onTap` callback from the drag controller.** The store records the last `pointerdown` type for browsers whose click carries no `pointerType`. A long touch that fires `contextmenu` is ignored, since long press is not a gesture.
- **The deck shows as its card back, with a short text.** The goal check's first attempt found the deck sheet showed no card or text (criterion 1). The model gained an optional `text` field for this.
- **Labels follow the mockups.** In hand: Deck top, Deck bottom. In play: Return to hand, Devolve (was "Return X to hand" and "Discard X (devolve)"), and Switch with X, no cost. Draw 1 is marked primary on the deck.
- **Attacks, Draw N and Look at top N still use `window.prompt`.** Per-target items (Evolve, Attach, Retreat, Switch, Move to) still list one item per target. The panels and target-mode tasks replace both.
- **A toggle's ✓ is decoration only** (`content: '✓ ' / ''`), so its accessible name stays the condition's name.

**Files changed**:
- `web/src/components/ActionSheet.svelte` (new)
- `web/src/components/ContextMenu.svelte` (deleted)
- `web/src/lib/actions.js`
- `web/src/lib/store.svelte.js`
- `web/src/App.svelte`
- `web/src/components/Card.svelte`
- `web/src/components/Stack.svelte`
- `web/src/components/Hand.svelte`
- `web/src/components/Side.svelte`
- `web/src/components/Modal.svelte`
- `web/e2e/sheet.spec.js` (new)

**Discoveries**:
- **In Chromium, `click` and `contextmenu` are `PointerEvent`s whose `pointerType` says mouse or touch.** That makes a per-event "tap or click" decision cheap. A fallback to the last `pointerdown` covers browsers where they are not.
- **The fixed position's two Fighting Energy cards stack in the hand, and the first copy is covered.** Tests must target the last copy.
- **A test-visible hook for the sheet:**
  - `.sheet[data-mode=phone|tablet|popover]` with `role=dialog` and the title as its name;
  - each group is `section[role=group]` named by its label;
  - the damage input is named "Damage total".

### 2026-11-02 — Task: Choose targets on the board

**What was done**:
- **Destination actions are single items that finish on the board.** Attach (Energy and Tool), Evolve, Retreat, Switch (no cost) and Move to (an attached card) are now one item each.
- **Choosing one starts target mode.** The sheet closes, and a `TargetBanner` names the action and the card, with Cancel. The candidates glow through `app.highlight` and the other cards dim. The card being played fades.
- **The next tap or click decides.** A tap or click on a candidate runs the action. Anything else, Cancel, Escape or a right-click ends target mode with nothing done.
- **One candidate acts at once.** Its label names the candidate, for example "Retreat to Riolu". Mouse and touch behave the same.

**Deviations**:
- **Target requests carry more than the plan's `{prompt, card, candidates, run}`.** They also have a `hint` line ("Tap one of your Pokémon"), and their candidates are `{spec, name, iid}` rather than bare drop specs. The name gives the single-candidate label, and the iid feeds the highlight.
- **Taps are intercepted by a window capture-phase `click` listener in `lib/target.js`, not in the pointer layer.** The pointer layer only ignores presses while a target is pending, so no drag starts.
- **The banner covers the header across the top** rather than sitting under it. That avoids measuring the header, which wraps.
- **`ownStacks` now returns `{spec, name, iid}`, and its old label form ("Bench 1 (Riolu)") is gone.**
- **Two sheet tests changed** because the target items collapsed the Move row:
  - the phone overflow test now uses the Conditions row;
  - the tablet "only the actions scroll" test shortens the window to 1024×560 so the actions overflow the panel.

**Files changed**:
- `web/src/lib/target.js` (new)
- `web/src/components/TargetBanner.svelte` (new)
- `web/src/lib/actions.js`
- `web/src/lib/store.svelte.js`
- `web/src/lib/pointer.svelte.js`
- `web/src/components/ActionSheet.svelte`
- `web/src/components/Card.svelte`
- `web/src/App.svelte`
- `web/src/app.css`
- `web/e2e/target.spec.js` (new)
- `web/e2e/sheet.spec.js`

**Discoveries**:
- **Retreat from the menu discards Energy for the Retreat Cost first.** Makuhita has a cost of 2 and holds 1 Energy, so the timeline shows the discard and then the swap. Tests should expect both lines.
- **What target-mode actions record:**
  - an Energy attach is `p1 moved p1-18 Fighting Energy p1:hand → p1:bench:0 as energy`;
  - an Evolve is recorded as a move, `p1 moved p1-3 Lucario p1:hand → p1:bench:0`;
  - a Tool attach is `… → p1:active as tool`.

### 2026-11-02 — Task: Touch-friendly controls across the board

**What was done**:
- **Hover effects only where hover exists.** Card and hand hover enlargement now sit under `@media (hover: hover)`. The Pokémon's quick damage buttons show on hover only with a mouse, and are removed on touch, where the sheet's Damage stepper covers them.
- **44px targets under a coarse pointer.** Buttons, selects, text inputs, a checkbox's label and every card (including Energy, Tools, prizes and pile cards) are at least 44×44 CSS px.
- **The timeline on touch.** Its rewind and play-from-here buttons show without a hover, at 44px.
- **Cards resist a long touch.** They set `user-select: none`, `-webkit-user-select: none` and `-webkit-touch-callout: none`.
- **The modal's side preview follows the mouse only.** It never sticks after a tap.

**Deviations**:
- **"Tapping a card in a discard, search, look or deck-list view shows its large preview" is met by the card's sheet and Zoom, not the modal's side preview.**
  - The side preview only fits beside a desktop dialog.
  - On touch, a discard, search or look card opens its action sheet, which shows the card and its text. Task 3 relies on this tap.
  - A deck-list card, which has no actions, opens Zoom, the large card with its text.
- **On touch, the board's cards grow to a 44px minimum** (a `max(44px, …)` on `--w` under `pointer: coarse`). Energy, Tools and prizes are therefore larger on a tablet than with a mouse.

**Files changed**:
- `web/src/components/Card.svelte`
- `web/src/components/Hand.svelte`
- `web/src/components/Stack.svelte`
- `web/src/components/Timeline.svelte`
- `web/src/components/Modal.svelte`
- `web/src/components/Side.svelte`
- `web/src/app.css`
- `web/e2e/touch.spec.js` (new)

**Discoveries**:
- **Chromium drops `-webkit-touch-callout` when it parses CSS** (the property is iOS-only), so `getComputedStyle` can't show it. The test reads the built stylesheet instead.
- **Opacity-0 controls still take taps.** Hover-revealed buttons must be `display: none` on touch, not just transparent, or they become invisible tap targets.

### 2026-11-02 — Task: Panels replace the browser dialogs

**What was done**: The last browser dialogs are gone, and `ask()` is removed.
- **Attacks open an `AttackPanel` inside the sheet.** It shows the attack's name and text, and a damage stepper that starts at the printed damage, with ×2 for Weakness. The target defaults to the opponent's Active; "Change ›" picks another opposing Pokémon on the board, then returns to the panel. The primary button reads "Attack <target> for <n>".
- **A Pokémon without attack data gets a name field.** Its "Attack…" opens the same panel with an attack-name field.
- **Draw N and Look at top N open a `CountPanel`.** The count runs from 1 up to the cards left in the deck, then Go.
- **One `Stepper.svelte` serves every stepper.** The sheet's Damage group and both panels use it, and the sheet shows a panel with "‹ Actions" to go back.

**Deviations**:
- **"Change ›" keeps the sheet mounted but hidden** (`app.sheet.hidden`) while the target is tapped, so the panel keeps its damage and target. Target requests gained an optional `done()`, called once target mode ends either way, which brings the sheet back. While a target is pending, Escape ends only target mode (`stopImmediatePropagation`), so the hidden sheet stays.
- **Panels are local state in `ActionSheet`, not `app.sheet.panel`.** An item carries `panel: {component, props}`.
- **"Change ›" is disabled when the opponent has only one Pokémon in play.** There is nothing else to pick.

**Files changed**:
- `web/src/components/AttackPanel.svelte` (new)
- `web/src/components/CountPanel.svelte` (new)
- `web/src/components/Stepper.svelte` (new)
- `web/src/components/ActionSheet.svelte`
- `web/src/lib/actions.js`
- `web/src/lib/target.js`
- `web/e2e/panels.spec.js` (new)

**Discoveries**:
- **A panel that closes the sheet is unmounted at once.** Its props and derived values read after `close()` come back empty, which is how Look at top N lost its seat. Read everything first, then close. The sheet renders the panel null-safely (`panel?.component`), because closing clears `panel` while it is still on screen.
- **An attack records two lines:**
  - `p1 used attack Body Slam from p1:active (Makuhita)`;
  - `p1 set damage on p2:bench:0 (Riolu): 0 → 140`.

### 2026-11-02 — Task: Compact phone board

**What was done**: On a phone held upright (narrower than 640px), the board switches to a compact layout:
- **Card size.** `--cw` is set from the width.
- **Counter bars.** Each player has one: chips for Deck, Discard, Lost and Prizes, plus the opponent's hand count. A chip opens the deck's actions, the pile view or a new prize view where a prize can be taken. The pile chips also take dropped cards.
- **Bench.** The Active is centred, and the bench is one row scrolling sideways to all 8 slots, with a "+n ›" hint for the slots off screen.
- **Hand and middle strip.** The hand is a flat strip of 72px cards. The middle strip is one row, which the play area covers while a card is dragged.
- **Header.** It keeps the dot, warnings and ☰. The game name, branch, Review mode and Reveal all move into ☰, and the timeline opens there as a full-screen sheet.
- **Held sideways.** A phone held sideways shows a "Turn your phone upright" notice in place of the board, and loses nothing.

Tablets and desktops keep the mat.

**Deviations**:
- **The sideways-phone test is `(orientation: landscape) and (max-height: 499px) and (pointer: coarse)`, not "phone width in landscape".** A short tablet window must never show the notice.
- **The phone timeline has its own closed-by-default flag** (`timelineOpen` in `App.svelte`). Opening the board on a phone shows the board, not the timeline. `app.panelOpen` still drives tablet and desktop.
- **The phone board is taller than the screen at 390×844 and scrolls vertically.** The spec rules out only sideways scrolling.
- **Cards in a pile view keep `touch-action: pan-y`** so a finger can scroll the list. A card is dragged out of the view by starting sideways.
- **Test helpers became layout-aware for the phone:**
  - `pile()` and `pileLabel()` find the side pile or the counter chip;
  - `isPhone` is added;
  - `timelineLines` reads the board's log from its state while the phone's timeline sheet is closed.

  Several phone tests now scroll their targets into view.
- **Timeline lines on touch now contain their floated buttons** (`display: flow-root`). Without it, the 44px buttons from the touch-controls task stacked into the next line.
- **The first goal check found criterion 6 (tablet and desktop unchanged) unproven.** A test now shows that at tablet and desktop sizes none of the phone layout applies and card sizing follows the unchanged formula.

**Files changed**:
- `web/src/components/CounterBar.svelte` (new)
- `web/src/App.svelte`
- `web/src/components/Side.svelte`
- `web/src/components/Hand.svelte`
- `web/src/components/Strip.svelte`
- `web/src/components/Modal.svelte`
- `web/src/components/Timeline.svelte`
- `web/src/lib/store.svelte.js`
- `web/e2e/phone.spec.js` (new)
- `web/e2e/fixtures.js`
- `web/e2e/smoke.spec.js`
- `web/e2e/drag.spec.js`
- `web/e2e/sheet.spec.js`
- `web/e2e/panels.spec.js`
- `web/e2e/touch.spec.js`

**Discoveries**:
- **A flex column with `align-items: center` lets a wide child grow past the screen.** `main` then scrolls sideways while the document doesn't, so a no-sideways-scroll test has to check `main` too. The phone column stretches its children so the bench row scrolls within itself.
- **A count of hidden children needs a `MutationObserver` as well as a `ResizeObserver`.** Adding a slot to a fixed-width row doesn't resize it.
- **Playwright's iPhone profile is 390×844 here, but the phone board runs about 966px tall.** Tests on a phone must scroll a target into view before reading its position.

