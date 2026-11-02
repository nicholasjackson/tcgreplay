# Working context: touch-friendly board

## Problem and motivation

- The user's words: "we should have a mobile friendly ui. If I use my iPad things work as long as I am using the trackpad on the keyboard. But I can not do anything with touch".
- Cause:
  - The board moves cards with HTML5 drag and drop.
  - Menus open only on right-click (`contextmenu`).
  - Cards enlarge on hover, and pile previews use `mouseenter`.
  - Numbers are entered through `window.prompt`.
  - On an iPhone the mat overflows: `--cw` floors at 48px and the mat is about 10 card widths wide.

## The user's direction on menus

- The user's words: "On iPhone you would need the whole screen to show the menu and the items would probably need to scroll across the screen. On iPad you could probably show the card and the menu to the right of it. But again, some menus are really large so you would probably need to scroll".
- I proposed shrinking the menus as well as giving them new containers:
  - per-target items become one action plus a tap on the board (target mode);
  - damage becomes a stepper and conditions become toggles;
  - items are grouped into sections.
- The user accepted that and asked for a design with mockups.

## The design (settled, stored)

- Design source `design`:
  - `mobile-board/design.md` (authored, draft, 2026-11-02);
  - `mobile-board/mockups.html` (frames A–H, stored byte for byte).
- The user said "I think the designs look fantastic".
- The spec references the design as a constraint and does not repeat it.
- Choices I made in the design that the user didn't push back on:
  - phone landscape shows a "turn upright" notice;
  - 640px breakpoint between the phone sheet and the tablet panel;
  - Playwright as a new dev dependency for touch tests.

## Decisions on this spec

- One spec, no epic (the user answered "No epic"; they asked to "draft the spec").
- The user will review the spec when it's complete, so drive the workflow with minimal interruptions.
- Suggested order of work, all within this one spec:
  1. Pointer-events drag and target mode;
  2. the sections menu model and the three containers;
  3. panels that retire `window.prompt`;
  4. the compact phone layout;
  5. Playwright touch tests.

## Codebase facts

- Single repo, `tcgreplay`, root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`. The web app is Svelte 5 in `web/`.
- Menu builders live in `web/src/lib/actions.js`: `handMenu`, `stackMenu`, `attachmentMenu`, `deckMenu`, `pileCardActions`.
- `ContextMenu.svelte` renders the popover; `Zoom.svelte`, `Modal.svelte`, `Card.svelte`, `Stack.svelte`, `Hand.svelte`, `Side.svelte` and `Strip.svelte` make up the board.
- `MaxBench = 8` (Area Zero Underdepths), added in this session and still uncommitted.
- There is no frontend test suite today; `make test` runs only Go vet and tests.

## Review mode for this spec

The user said "draft the spec, I will review it when complete". Each section is drafted into its working file and the workflow advances without stopping for confirmation. The user reviews everything together at the verification step.

## Spec status

- The spec `20261102110545-5c84659c-touch-friendly-board` is written to the store. The user approved it ("This looks great").
- The design and mockups are attached as references.
- At the split check, no offer was made. Earlier the user chose one spec over the three-spec epic I'd suggested (input, menus, phone layout).
- The user asked to plan straight after the spec.

## Plan status

- The plan for the touch-friendly board is written: 4 milestones, 7 agent tasks, all in `tcgreplay`.
- The walkthrough is pending. The user must sign off before the plan workflow finishes.
- Flagged to the user: the uncommitted bench-of-8 change (and the decklist-required change) must be committed before implementing, because implement worktrees branch from HEAD.
