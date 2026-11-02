---
created_date: "2026-11-02"
document_status: final
closed_date: "2026-11-02"
---

# Research: 20261102110545-5c84659c-touch-friendly-board

## Alternatives considered and rejected

### Input handling

- **Polyfill HTML5 drag and drop for touch** (for example `mobile-drag-drop`).
  - Rejected for three reasons. It keeps `dataTransfer`, whose payload is unreadable during `dragover`. It adds no tap or target mode. And it conflicts with the design's single Pointer Events path (design `mobile-board/design.md`, "Principles" 1, "Dragging").
  - The current DnD code it would keep: `web/src/lib/actions.js:26-42` (`startDrag`/`readDrag` on `dataTransfer`).
- **Separate touch handlers beside the existing mouse DnD.**
  - Rejected: it doubles every drop zone's handlers (`Stack.svelte:26-39`, `Side.svelte:20-33`, `Strip.svelte:14-28`, `Hand.svelte:58`), and the design asks for one implementation.
- **Long press to open menus.** Ruled out by the spec's Non-Goals and the design ("Long press is not used").

### Menus

- **Keep flat item lists and restyle the container for touch.**
  - Rejected: `stackMenu` produces 25+ items (`actions.js:189-275`) and `handMenu` one item per target (`actions.js:148-186`). The design and spec require grouped sections and board targeting.
- **One component per container** (phone sheet, tablet panel and desktop popover each rendering items themselves).
  - Rejected: the design says "One renderer serves all three".

### Browser test backend

- **Mock `/ws` in Playwright** (`page.routeWebSocket`) with JSON snapshot fixtures. Hermetic and fast, but rejected:
  - it can only assert the verb the board sends, not that "the board and timeline show the same move" (spec AC "Hand to bench by touch", "An attack completes with no dialog");
  - hand-written snapshot fixtures drift from `game.UISnapshot`.
- **Drive the real binary through MCP** (spawn `tcgreplay -no-daemon -ui-addr …` and write JSON-RPC to stdin to import a game).
  - Rejected: it needs TCGdex or a committed card cache for real card data, plus an MCP handshake in test setup, and starting from a random shuffle is not deterministic.

## Chosen approach — evidence

### Pointer Events, one path

- Every DnD site funnels into two functions: `dropOnStack` and `dropOnZone` (`actions.js:69-124`). Each takes the payload `{kind, iid, def, from, role}` (`Card.svelte:32-34`; `Stack.svelte:36-38` uses `{kind:'stack', from}`).
- A pointer drag that hit-tests drop zones by `data-drop` can call those same two functions. This keeps the drop results identical, as the spec requires.
- `app.dragging` already exists in the store (`store.svelte.js:44`). `Strip.svelte:83` reads it to show the play area while dragging, so a pointer drag that sets it keeps that behaviour.

### Menus as data

- Every menu is opened by setting `app.menu = {x, y, title, items}` (`Stack.svelte:22-25`, `Hand.svelte:73`, `Side.svelte:44-46`, `Stack.svelte` attachment `onmenu`), and one component renders it (`ContextMenu.svelte`).
- Changing the builders to return the sections model, and replacing `ContextMenu.svelte` with one renderer that takes a container mode, touches only those sites.

### Prompts

- `window.prompt` is used through `ask()` (`actions.js:142-145`) in four places:
  - the attack, twice (`actions.js:200-205`, `211-218`);
  - Set damage (`actions.js:254-260`);
  - Draw N and Look at top N (`actions.js:300-312`).

  All four map to the design's panels.

### Hover-only controls the design does not name

These must stay reachable by touch (spec requirement "Everything stays reachable on a phone", AC "Tap targets…"):

- `Stack.svelte` `.controls` (−, +10, +50, ⋯) shows only on `.main:hover` (`Stack.svelte:169`). Their actions are covered by the sheet's Damage stepper, so on a coarse pointer they can stay hidden.
- `Timeline.svelte` `.acts` (play from here, rewind) and `.rw` show only on hover (`Timeline.svelte:115-117`, `175-195`). The timeline is the only place for these, so they need to be visible on coarse pointers.
- `Hand.svelte:105-110` and `Card.svelte:108-116` enlarge on `:hover`. The design gates these behind `(hover: hover)`.
- `Modal.svelte:98,123,142,173` set the large preview on `mouseenter`. On touch, the pile view needs a tap to preview, or the action sheet.

### A real server for browser tests

- `internal/webui/webui_test.go:83-105` already builds a full board server in-process from `game.NewManager(dir, fakeDB)` + `m.New(...)` + `webui.New(m, P1)` + `s.Handler()`.
- `fakeDB` (`webui_test.go:28-61`) serves cards with no network.
- `/api/card` reads `s.M.DB` (`server.go:137-150`), so the fake pool also serves card data to the page.
- A small Go program built from the same pieces can serve the built board on a fixed loopback port with a known position set up in review mode (`move` by iid). It would expose a reset hook so each test starts clean. Playwright starts it through `webServer`.
- Loopback is fine for this: the board refuses non-loopback Host and Origin (knowledge `gotchas/board-loopback-locks.md`), and Playwright on `127.0.0.1` satisfies both.

### Build and test wiring

- `web/package.json` has no test script. Its dev dependencies are vite 8.3.1, svelte 5.57.1 and @sveltejs/vite-plugin-svelte 7.3.1.
- `Makefile:27-29` `test` runs only Go. `Makefile:20-25` builds the web app into `internal/webui/dist/app` (`vite.config.js:9-12`).
- There are no CI workflows in the repo (`.github/workflows` is absent), so "the project's checks" means `make test`.

## Files examined

- `tcgreplay:web/src/lib/actions.js:1-368`:
  - DnD helpers (26-42);
  - `dropOnStack`/`dropOnZone` (69-124);
  - `ownStacks` (131-140);
  - `ask` prompt (142-145);
  - `handMenu` (148-186), `stackMenu` (189-275), `attachmentMenu` (279-290), `deckMenu` (294-320), `pileCardActions` (333-344);
  - `firstEmptyBench` (347-352).
- `tcgreplay:web/src/lib/store.svelte.js:1-117`:
  - `app` state: `menu`, `modal`, `zoom`, `dragging`, `highlight` (18-58);
  - `cardInfo` fetch (90-107);
  - `canAct`/`oppActing` (109-117).
- `tcgreplay:web/src/lib/ws.js:1-101`: a single websocket; `act(verb,args)` resolves on the result; state pushes go to `showSnapshot`.
- `tcgreplay:web/src/components/Card.svelte:1-200`: `draggable` + `ondragstart`, click to zoom, `contextmenu` to `onmenu`, hover lift (105-116).
- `tcgreplay:web/src/components/Stack.svelte`:
  - drop zone (26-35);
  - stack drag (36-38);
  - `.controls` shown on hover (169);
  - attachment menus inline.
- `tcgreplay:web/src/components/Hand.svelte`: hand drop zone (58); fan geometry; hover enlarge (105-110); `handMenu` via `onmenu` (73).
- `tcgreplay:web/src/components/Side.svelte:1-189`: prizes grid; piles column with DnD; deck click and right-click both open `deckMenu`; bench slots, including the 8-slot expansion (`expands`).
- `tcgreplay:web/src/components/Strip.svelte:14-28,48-90`: stadium and play-area zones; turn buttons; play area shown while dragging.
- `tcgreplay:web/src/components/Modal.svelte`: pile, search, look and decklist views; draggable cards; `mouseenter` previews; `pileCardActions` buttons.
- `tcgreplay:web/src/components/ContextMenu.svelte:1-66`: fixed popover clamped to the viewport; items `{label, run}`/`{sep}`.
- `tcgreplay:web/src/components/Zoom.svelte:1-67`: full card view from `app.zoom`.
- `tcgreplay:web/src/components/Timeline.svelte:86-117,175-195`: rewind and play buttons revealed on hover.
- `tcgreplay:web/src/App.svelte:1-299`:
  - header (branch, mode, reveal, warnings, ☰ dropdown);
  - board order: Hand, Side, Strip, Side, Hand;
  - `--cw` formulas (`.app.wide`, `.panel-bottom`) and `@media (max-width: 900px)`.
- `tcgreplay:web/src/app.css:1-45`: `--cw` base formula, colour tokens, light-mode overrides.
- `tcgreplay:web/index.html`: the viewport meta is present.
- `tcgreplay:web/package.json`, `web/vite.config.js`: no test tooling; the build goes into the Go package for `go:embed`.
- `tcgreplay:Makefile:12-35`: `web`, `test` (Go only), `dev`, `dev-ui`.
- `tcgreplay:internal/webui/server.go:46-80,137-150`: `New`, the routes (`/ws`, `/api/card`, static), `serveCard` from `M.DB`.
- `tcgreplay:internal/webui/webui_test.go:28-105`: `fakeDB`, `testDeck`, `setupServer` (the in-process board server).
- `tcgreplay:internal/webui/actions.go:146-274`: the board verbs accepted over the websocket (all current verbs; nothing new needed).
- `tcgreplay:internal/game/manager.go:193-199`: `NewGameRequest` has no seed, so a deterministic position needs review-mode moves after `New`.
- `tcgreplay:cmd/tcgreplay/main.go:122-145`: binary flags (for reference; the test server won't use the binary).

## External references

- MDN, Pointer Events and `setPointerCapture`: the single input model, and capture keeps a drag's moves after the pointer leaves the card.
- MDN, `touch-action`: `pan-x` on the hand and `none` on mat cards decide scroll versus drag.
- MDN, `-webkit-touch-callout` and `user-select`: suppress the iOS callout and text selection on long touch.
- MDN, media features `hover`, `pointer` and `any-pointer`: gate hover enlargement; enlarge tap targets under coarse pointers.
- Playwright docs:
  - device descriptors ("iPhone 13", "iPad (gen 7)") with `hasTouch`/`isMobile`;
  - `locator.tap()`;
  - the touchscreen API;
  - `webServer` config.

  These are the profiles and tap and touch drag the tests need.
- Apple Human Interface Guidelines: a 44×44pt minimum hit target, which the spec's 44×44 CSS px follows.

## Prior plans / specs consulted

- Spec `20261102110545-5c84659c-touch-friendly-board`: the spec being planned.
- Design `design:mobile-board/design.md` and mockups `design:mobile-board/mockups.html`: binding. They fix gestures, target mode, the menu model and sections, containers, panels, phone layout, the 640px breakpoint and Playwright.
- No prior plan touches the web board's input model. The hosted-playmat and wait-for-turn plans changed the board's header and status badges only. The wait-for-turn plan recorded "no automated tests for the board app" as out of scope, which this plan now reverses, as the spec requires.

## Open assumptions

- **iOS Safari scroll versus drag in the hand.** The assumption is that `touch-action: pan-x` on the hand, plus direction detection after 8px, separates a sideways scroll from an upward drag on real iOS. If it does not on a real device, STOP and ask before changing the gesture model, which the design fixes.
- **Playwright's WebKit iPhone profile.** It is assumed to reproduce tap and touch drag well enough to exercise Pointer Events. Callout and selection suppression cannot be checked in emulation, so it is a manual check.
- **A deterministic test position in review mode.** The assumption is that `New` then `move` by iid builds one (the hand, Active, bench, deck and Stadium the tests need), without touching game code. If not, STOP before adding a seed to `NewGameRequest`, since the spec's non-goals leave the game model unchanged.
- **Hidden-but-clickable hover controls.** Assumed acceptable to keep hidden on coarse pointers where the sheet covers them (the Stack damage buttons), but not where they are the only route (Timeline rewind and play).
- **Playwright browsers in `make test`.** Assumed the developer can install them once (`npx playwright install`). `make test` should say so clearly if they are missing, rather than fail obscurely.

## Drafting assumptions

### Browser tests run against a real in-process board server (discovery)
- **Decision**: Playwright starts a small Go program that serves the built board, backed by a real game manager, the fake card pool and a fixed review-mode position, with a reset between tests.
- **Rationale**: The spec's acceptance criteria check that the board and the timeline change after an action. A real server shows that with no hand-written fixtures, and the pieces already exist in `webui_test.go`.
- **Rejected**: Mocking `/ws` in Playwright: it can only check the verb sent, and its fixtures drift. Driving the real binary over MCP: it needs network card data and a handshake, and the position isn't deterministic.

### Hover-only timeline controls become visible on coarse pointers (discovery)
- **Decision**: The Timeline's rewind and play buttons are always shown under a coarse pointer. The Stack's hover damage buttons stay hidden on touch, because the action sheet covers them.
- **Rationale**: The design doesn't mention these controls, but the spec requires everything to stay reachable. The timeline buttons are the only way to reach those actions; the damage buttons are not.
- **Rejected**: Leaving them hover-only (unreachable by touch). Adding them to a sheet (more surface than needed).

### Chosen direction: one pointer controller, target-mode store, sections model, one ActionSheet renderer, a layout flag (architecture)
- **Decision**: Build the design with:
  - a single Pointer Events drag controller exposed as Svelte attachments, calling the existing `dropOnStack`/`dropOnZone`;
  - a target-mode store and banner;
  - menu builders that return the design's sections model;
  - one `ActionSheet` renderer with phone, tablet and popover modes, replacing `ContextMenu` (and Zoom for touch);
  - a phone/tablet/desktop layout flag driving the compact phone pieces.

  Playwright runs against an in-process Go board server with the fake card pool.
- **Rationale**: It follows the design's "one input path" and "one renderer" principles directly. Because the drop and verb code stays unchanged, the "same result as the mouse" requirement holds. It touches the fewest components.
- **Rejected**:
  - Per-component pointer handlers: duplicate drag logic in five components.
  - A separate phone app shell: forks the board.
  - An HTML5 drag-and-drop polyfill: keeps `dataTransfer`, with no tap or target mode.

### Svelte attachments for the pointer layer (architecture)
- **Decision**: Expose the draggable and drop-zone behaviour as Svelte 5 attachments (`{@attach}`), which svelte 5.57 supports.
- **Rationale**: Attachments are reactive to their arguments and clean up on unmount, so the payload follows the card it belongs to.
- **Rejected**: Svelte actions (`use:`), which are the older API and don't re-run when their arguments change; wrapper components, which add DOM.

### No conventions apply (architecture)
- **Decision**: List no conventions in the plan.
- **Rationale**: The only convention covers guide text and guide packaging, and this work touches neither.
- **Rejected**: Listing it anyway, which would be padding.

### Share the fake card pool between the Go tests and the test server (implementation_detail)
- **Decision**: Move `fakeDB` and the test deck from the board-server tests into a small test-support package that both the Go tests and the board test server import.
- **Rationale**: Two copies of the card pool would drift. A test-support package keeps one copy, and nothing in the shipped binary imports it.
- **Rejected**: Copying `fakeDB` into the test server, which drifts; exporting it from the `webui` package, which leaks test code into a shipped package.

### Task order keeps one writer per file (tasks)
- **Decision**:
  - The harness comes first.
  - Pointer dragging depends on the harness.
  - The grouped action sheet depends on dragging, because both edit the card, stack, hand and side components.
  - Target mode and the touch controls each depend on the sheet.
  - The panels depend on target mode.
  - The phone board depends on the sheet and the touch controls.
- **Rationale**: Tasks that touch the same components never run in parallel, so worktree merges stay clean. Target mode and touch controls can run in parallel because they touch different areas.
- **Rejected**: Running dragging and the sheet in parallel, which would both rewrite `Card.svelte` and the menu-opening sites.

### Target mode lands after the grouped sheet, not with dragging (tasks)
- **Decision**: Build board selection on top of the sections model, rather than retrofitting it onto today's flat menus first.
- **Rationale**: Target items are part of the sections model (an item has `run`, `target` or `panel`). Retrofitting would rewrite the builders twice. The user's suggested order put dragging and target mode first, mainly so the iPad becomes usable early. Milestone 2 still delivers a fully usable iPad.
- **Rejected**: Target mode in milestone 1, which means two rewrites of the menu builders.

### The browser suite runs in one worker against one game (tasks)
- **Decision**: Playwright runs serially, with a reset before each test.
- **Rationale**: The test server holds one game, as the board does, and serial runs keep tests independent without multi-game support in test code.
- **Rejected**: A game per worker, which would need routing by game in the board and test server for little gain at this suite's size.

## Rehydration cues

- `spektacular spec file read 20261102110545-5c84659c-touch-friendly-board`
- `spektacular design read --data '{"source":"design","path":"mobile-board/design.md"}'` (and `mobile-board/mockups.html` for frames A–H)
- `spektacular knowledge read --data '{"tier":"repo","name":"tcgreplay","path":"gotchas/board-loopback-locks.md"}'`
- Re-read `web/src/lib/actions.js`, `web/src/components/{Card,Stack,Hand,Side,Strip,Modal,ContextMenu,Zoom,Timeline}.svelte`, `web/src/App.svelte` and `web/src/app.css`.
- Re-read `internal/webui/webui_test.go:28-105` for the in-process server pattern.
