---
created_date: "2026-11-20"
document_status: final
closed_date: "2026-11-20"
---

# Plan: 20261120140830-52277437-studio-redesign

<!-- Metadata -->
<!-- Created: 2026-11-20T15:19:07Z -->
<!-- Commit: fae76c09b161ad6451166d4157ee76330aa3468e -->
<!-- Branch: main -->
<!-- Repository: tcgreplay -->

## Overview

This plan moves TCG Replay's board and website to the Studio design: one shared set of design tokens, a light interface around a dark mat, IBM Plex type, themes (light or dark interface; slate, felt or navy mat), a new logo and card back, clearer board readouts, always-visible replay controls, restyled menus and a restyled website. It solves the hand-copied, hard-coded styling that makes every look change touch every component, and it gives players a board that reads at a glance on every screen. It ships as one release in three checked milestones: tokens with no visual change, the board, then the website and logo.

## Conventions

- **Update the privacy page with any new outside resource or remembered setting** — Studio adds Google Fonts (a new outside host for the board and the website) and two browser-remembered theme settings, so the privacy page's outside-services section, its "only thing this website loads" sentence and its display-settings list change in the same release.
- **Every player-visible change ships with a What's new entry** — the redesign is visible on every board screen and website page, so the release adds a `site/src/data/whats-new.yaml` entry with `refresh: false`, since no tool, guide or server instruction changes.
- Dropped: **Guide text and packaging** — no guide text or guide packaging changes in this work.

## Architecture & Design Decisions

Studio is built on the settled designs (`studio/design.md`, with `mobile-board/design.md` and `wide-board/design.md` for behaviour and layout). The work therefore decides how to carry that design into the two apps, not what it looks like. The centre of the change is a new **`internal/design` package** that owns the design tokens for the whole repo.

- **`tokens.json`** is the one source. Its names and values mirror the TCG Replay Studio design system: the interface colour roles for `light` and `dark`, the `slate`, `felt` and `navy` mat sets with `mat-*` aliases, the constant game colours (players, accent, HP, damage, Energy), new `card-*` colours for text-card faces, and type, spacing, radius, shadow and size scales.
- **A small Go generator**, run through Go's code generation, writes **`tokens.css`** from it, and the CSS is committed. Interface themes are `[data-theme]` blocks. Each mat is a `[data-mat]` block that repoints the `mat-*` custom properties at its set.
- **Both apps import that one CSS file:** the board from `web/src/main.js` (with `internal/design` added to Vite's `fs.allow`) and the website from `Base.astro`. The Dockerfile's `web` and `site` stages copy `internal/design`, exactly as they already do for `internal/notice`. The hand-copied `site/src/styles/tokens.css` is deleted.

This beats the alternatives (see `research.md#alternatives-considered-and-rejected`). Keeping tokens in `web/src/app.css` would leave the site without a buildable path to them. A tokens package or a lint stack would add tooling the repo deliberately does not have. Build-time CSS plugins would hide the generated values from the Go checks.

The **checks are Go tests in `internal/design`**, so they run inside the project's existing Go test run, part of the full project test run. CI runs no tests, so that full run is the gate.

- **Generated file current:** `tokens.css` matches what `tokens.json` generates.
- **Contrast:** every text-and-background pairing declared in `tokens.json` meets 4.5:1 (3:1 for large text, borders, focus rings and markers) in both interface themes and all three mats, computed with the WCAG relative-luminance formula.
- **No stray values:** a scanner modelled on `internal/catalog/nofetch_test.go` walks `web/src` and `site/src` and fails on any colour literal (hex, `rgb()`, `hsl()`, named colours in colour properties) or font size in px or rem that is not a `var(--…)` reference. The token file itself is exempt, along with a short, reviewed allowlist (for example `transparent`, `currentColor`, and sizes derived from `--cw`).

The **migration runs in two phases inside one release**, with the spec's constraint ("The landscape mat lands first") as a precondition: the wide-board work must be on main before any task starts.

- **Phase one (tokens, no visual change).** First add Playwright `toHaveScreenshot` baselines of today's board (phone, tablet, desktop, wide, docked timeline) and every website page, taken on the developer's machine with Chromium. Then introduce `internal/design` with tokens set to *today's* values, plus legacy aliases where today's names differ. Move every component onto tokens, and prove nothing moved by re-running the screenshots and turning the stray-value check on.
- **Phase two (Studio).** Swap the token values for Studio's, then restyle component by component: board chrome, the mat parts (player blocks, Bench readouts, Active panel, turn strip, hand), the card back, the timeline (adding per-turn rewind and always-visible line controls), the menus and the attack panel's Knock Out hint, the theme settings in ☰, and finally every website page. Each restyle replaces its screenshot baselines.

IBM Plex loads from Google Fonts in `web/index.html` and `Base.astro`. The website tests that forbade any outside host or web font are narrowed to allow exactly `fonts.googleapis.com` and `fonts.gstatic.com`. Browser tests fulfil those requests from IBM Plex files kept with the test fixtures, so screenshots stay deterministic and offline.

Themes live entirely in the board. Two new localStorage keys, `theme` and `mat`, follow the existing try/catch pattern in `store.svelte.js`. A tiny inline script in `web/index.html` applies `data-theme` and `data-mat` before first paint, so the board never flashes the default theme. The website stays light-only and stores nothing, which keeps `analytics.spec.js`'s empty-storage check true. Per the `privacy-page-tracks-outside-services` convention, the privacy page gains Google Fonts and the two theme settings in the same change.

The new mark's SVG (`internal/design/mark.svg`) is the source for every icon. The PNGs (`site/public/logo.png`, `icon.png`, `docs/logo.png`, `plugin/assets/icon.png` from a redrawn `plugin/assets/icon.svg`) are rendered from it at their existing paths, so the tests that pin those paths are unaffected. The board's favicon and card back use the mark directly. Per the `whats-new-entries` convention, the release ships with a What's new entry (`refresh: false`), because players see the change but no tool changes.

## Component Breakdown

- **Design tokens (new, `internal/design`).** Owns every design value in the repo. It holds:
  - the token source;
  - the generator that turns it into the shared stylesheet;
  - the mark's vector source.

  Its tests guard three things: that the stylesheet is current, contrast across both interface themes and all three mats, and the absence of stray colour and type-size literals in the board and the website. The board and the website depend on it; it depends on nothing in them.
- **Board shell (changed).** The board's page and its root layout. It loads the shared tokens and IBM Plex, applies the saved interface and mat before first paint, and owns the header (the mark, game, branch, mode, Reveal all, warnings, connection dot, ☰). The ☰ menu gains the interface and mat choices. Layout modes (phone, tablet, desktop, the landscape mat from the wide-board work, the docked timeline) are unchanged in behaviour and restyled.
- **Display settings store (changed).** The board's remembered settings gain the interface theme and the mat, alongside replay speed, autoplay, panel position and prize reminders, with the same storage-may-fail handling.
- **Mat parts (changed): player blocks, Bench, Active panel, turn strip, hand.**
  - Player blocks show prize pips, pile chips and the top of the discard.
  - The Bench shows an HP and Energy readout under each card.
  - The Active panel shows HP left over maximum, an HP bar, a damage tag and Energy, outlined in the player's colour.
  - The turn strip shows the Stadium, the turn tag and the turn controls (End turn in the accent colour).
  - The hand labels each card's kind.

  They read the same game state as today and change only presentation. They use only `mat-*` and game tokens, so every mat works.
- **Card (changed).** Draws a card face, a text card or an Energy token, now from tokens. Face-down and unknown cards show the new Studio back with the mark. Used by every mat part, the menus and the zoom view.
- **Timeline and playback bar (changed).**
  - Every line's Play from here and Rewind to here are always visible.
  - Each turn heading gains Rewind beside Play, and each player's half keeps Rewind and Play.
  - The scrubber is coloured by player; the docked layout gets a control row and a turn list with the prize score.

  Controls call the existing rewind and replay actions.
- **Menu containers and panels (changed): the action sheet in its popover, panel and sheet modes, the attack panel, the count panels, the target banner, the stepper, the modal views, zoom.** They render the existing menu model in Studio styling.
  - The attack panel adds the Knock Out hint, computed from the chosen damage and the target's HP left.
  - The hand-card popover adds Card text.
  - The menu model itself is unchanged except for that item.
- **Website shell (changed): the base layout, the docs layout and the shared pieces.** The base layout owns the header (the mark, Guides, Example, Limits and What's new links, Get started as a button), the dark footer (the notice, credit and link columns), the shared stylesheet and IBM Plex. The docs layout adds a breadcrumb and an On this page list, each in its own landmark, beside the existing sidebar. Copy blocks, prompt bubbles, numbered steps, callouts and code blocks are restyled.
- **Website pages (changed):**
  - home: the restyled animated scene, How it works, What it does, guide cards, address block;
  - Coaching workflows: grouped into After a game, Practice and Play, keeping the playmat's order within each group;
  - What's new: a timeline;
  - Example game;
  - Limits: stat tiles from the site facts;
  - Privacy: Google Fonts and the theme settings added;
  - Terms and 404.

  Content still comes from the site facts, the notice, the What's new data and the docs content.
- **Icons and listing assets (changed).** The website's logo and favicon, the README's logo and the ChatGPT listing icon are all redrawn from the mark's source, keeping their file names and the listing icon's checks.
- **Browser test suites (changed).**
  - The board and website suites gain screenshot baselines (first today's look, then Studio's) and a font fixture that serves IBM Plex locally.
  - Theme, replay-control, menu and Knock Out hint tests are added.
  - Existing tests change only where a requirement forces it: the nav, the guide grouping, the outside-font allowance, and the privacy wording.

## Data Structures & Interfaces

**Token source (`tokens.json`).** This is the contract between the design system, the generator and the checks. It uses the design system's list shape: each family is a list of named tokens, and colours carry a value per interface theme. Two additions serve the checks: mat sets, and declared contrast pairs.

```
{
  "color":   { "themes": ["light","dark"],
               "tokens": [ { "name": "ink", "value": {"light": "#14171C", "dark": "#E6E9EE"}, "usage": "…" },
                           { "name": "slate-surface", "value": "#252A33" },
                           { "name": "mat-surface", "value": "{slate-surface}" }, … ] },
  "mats":    [ "slate", "felt", "navy" ],          // each names a <mat>-* set the mat-* aliases can point at
  "type":    { "families": { "sans": "…", "mono": "…" }, "styles": [ { "name", "fontSize", "lineHeight", "fontWeight" } ] },
  "spacing": { "tokens": [ … ] }, "radius": { "tokens": [ … ] },
  "shadow":  { "tokens": [ … ] }, "size":   { "tokens": [ … ] },
  "pairs":   [ { "text": "muted", "background": "panel", "min": 4.5 },
               { "text": "mat-muted", "background": "mat-zone", "min": 4.5 },
               { "text": "control-line", "background": "panel", "min": 3.0 }, … ]
}
```

- `pairs` lists every text-and-background (or marker-and-background) pairing the design uses. The contrast check evaluates each pair in both interface themes and, for `mat-*` names, under each mat.
- Phase one holds today's values, with legacy names kept as aliases (for example `felt`, `accent`, `p1`). Phase two replaces the values with Studio's and drops aliases that are no longer used.

**Generated stylesheet (`tokens.css`).** This is the contract between the token package and both apps. It holds:

- every token as a custom property `--<name>` on `:root` (light values, `color-scheme: light`);
- `[data-theme="dark"]` overrides;
- one `[data-mat="<mat>"]` block per mat that sets each `--mat-*` to that set;
- the type styles as custom properties (`--font-sans`, `--font-mono`, `--fs-<style>`), and spacing, radius, shadow and size properties.

Components reference only custom properties from this file. Nothing else defines a design value.

**Board display settings.** Two new remembered settings sit beside the existing four:

```
theme: "light" | "dark"           (default "light")   storage key "theme"
mat:   "slate" | "felt" | "navy"  (default "slate")   storage key "mat"
```

They apply as `data-theme` and `data-mat` attributes on the document root. An unknown or unreadable stored value falls back to the default.

**Menu model.** Unchanged in shape. The hand-card builder adds one item, `{ label: "Card text", run }`, that opens the card's text. The attack panel derives a display-only value, `knockOut = damage >= target.hpLeft`; it feeds the hint line and is never used to block the attack.

**Workflow grouping on the website.** Each coaching guide gains a group, one of `after-a-game`, `practice` or `play`. Groups render in that order, and within a group guides keep the playmat's own guide order.

No server, tool, wire-format or MCP interface changes.

## Implementation Detail

- **A new module boundary for design values.** Today each app owns its own colours and the website keeps a hand copy. After this, design values live in one Go package and arrive in both apps as a generated stylesheet. A developer changing a colour edits `tokens.json` and reruns the generator; the tests then show whether the generated file is current and whether contrast still holds. The package follows the precedent of the shared notice data already imported by both apps, and needs the same Vite dev-server allowance and Dockerfile copies.
- **Generated-and-committed, checked by tests.** The stylesheet is committed so that neither front-end build needs Go. A test regenerates it in memory and fails on any difference. This copies the existing pattern for the listing icon (an editable source beside a derived artefact, checked by a test), but makes the derivation automatic.
- **Source scanning as a guard rail.** The stray-value check is a Go test that walks the two front-end source trees, in the style of the existing no-fetch scanner. It has an explicit allowlist and tests its own scanner. A failure names the file, the line and the literal, so fixing it is mechanical.
- **Components speak only in tokens.** Svelte and Astro component styles reference `var(--…)` properties and never raw values. Sizes tied to the card width (`--cw`-derived lengths) remain expressions over tokens. Board layout geometry (grid tracks, card ratios) is not a design value and stays in components.
- **Two phases, one release, screenshots as the hinge.** Phase one introduces screenshot baselines first (today's look), then tokens with today's values, so the baselines prove the move changed nothing. Phase two changes token values and component styles and re-records baselines deliberately, component by component, so each review is small. Screenshot tests become a new pattern in both suites. They run on Chromium only, with animations settled and the pointer parked, per the existing gotchas on board measurements.
- **Theme application without a framework.** The interface theme and the mat are attributes on the document root, set once before first paint and then by the ☰ menu through the existing settings store. No component knows which theme is active: they read the same tokens, whose values the attributes switch.
- **Fonts as the only outside host.** Google Fonts is added in both page shells. The website's outside-host and web-font tests become allowlists naming Google's two font hosts, and browser tests serve the font files locally so runs are offline and deterministic.
- **Website structure stays data-driven.** Guide grouping is data attached to each guide, not hard-coded markup, and the group order lives in one place. Limits stat tiles keep reading the site facts. New docs landmarks (breadcrumb, On this page) sit outside the existing sidebar landmark, so the sidebar's contract is untouched.
- **Restyle in place, not rewrite.** The menu model, layout modes, pointer layer and the home page's animated scene are reused as they are. Only their markup classes and styles change, and the hooks existing tests select on stay.

## Dependencies

- **Must land first: the wide-screen landscape mat** (spec `20261104090653-7373a393-wide-screen-landscape-board`). Its implementation is uncommitted in the worktree `../tcgreplay-wide-board`. The maintainer commits and merges it to main before any task here starts. This plan restyles it and adds its `wide` test project to the screenshot baselines.
- **`studio/design.md` from the `design` design source** — the settled Studio look: colour roles, themes, the board's parts, screen sizes, menus and website. Revised during planning so the home hero restyles the animated scene.
- **`mobile-board/design.md` from the `design` design source** — the settled touch behaviour, the menu model and its three containers, and the phone layout that Studio restyles without changing.
- **`wide-board/design.md` from the `design` design source** — the settled landscape-mat layout for board areas 1600px and wider, restyled here.
- **TCG Replay Studio design system** (https://claude.ai/artifact/8uqeTrkeXAc2YYTUvwtEjK) — token names and values, the mark, and component guidance. Needs `card-*` tokens added for text-card faces. Where it disagrees with the design document, the design system is corrected.
- **Studio design canvas** (https://claude.ai/artifact/5esddCvKjxHT9WiFw9u7MJ) — the mockups each restyled screen is reviewed against.
- **Google Fonts** (external service) — serves IBM Plex Sans and Mono to the board and the website. No account or key needed.
- **IBM Plex font files** (SIL Open Font License) — a local copy kept with the browser test fixtures, so tests serve fonts offline.
- **`internal/notice`** — an existing shared package. Unchanged; it is the precedent for sharing `internal/design` with both front-end builds.
- **Playwright (`@playwright/test`, already a dev dependency of both apps)** — gains screenshot comparisons. No new package.
- **`rsvg-convert` (librsvg)** — renders the mark and the listing icon to PNG. Already the documented way to render the listing icon. Used by hand; not added to the build.
- **Dockerfile `web` and `site` stages** — need `internal/design` copied in, as they already copy `internal/notice`.
- No server, MCP tool or Go runtime dependency changes.

## Testing Approach

**Test types**

- **Go unit tests in the token package** carry the load-bearing guarantees, and they run in the project's existing Go test run, part of the full project test run (the gate):
  - the generated stylesheet matches the token source;
  - every declared pair meets its contrast minimum in both interface themes and under each mat;
  - no colour literal or px/rem font size exists in the board or website sources outside the token file and the reviewed allowlist. The scanner has its own tests (it catches a planted literal and ignores `var()` references and allowlisted values), as the existing no-fetch scanner does.
- **Screenshot regression (Playwright visual comparison)** on both suites. These are the first in the repo, Chromium only, with animations settled and the pointer parked before capture (per the board-measurement gotcha).
  - Phase one records today's board (phone, tablet, desktop, wide, docked timeline) and every website page (laptop and phone) before any change, and must still match after the move onto tokens. That proves the token step changed nothing visible.
  - Phase two re-records baselines component by component as Studio lands.
  - Fonts are served from local fixture files so captures are offline and stable.
- **Board end-to-end tests** (existing Playwright suite and fixtures, all profiles):
  - **Themes:** a first visit shows light with slate; each of the six combinations can be chosen from ☰; element boxes are unchanged across themes; the choice survives a reload.
  - **Replay controls:** play from a line and rewind to just after it; turn and half Rewind/Play act on their start; the controls are visible without hover, and 44px on touch.
  - **Attack panel:** the Knock Out hint appears exactly when damage reaches the target's HP left, and Attack still runs below it.
  - **Menus:** a mouse right-click opens the popover, touch opens the panel on a tablet and the sheet on a phone; the hand popover offers Card text.
  - **Card back:** face-down cards show the Studio back.
- **Website end-to-end tests** (existing Playwright suite and crawl):
  - the header's links and Get started button, Home and Privacy as footer links, the dark footer content;
  - guides grouped into After a game, Practice and Play, in playmat order within each group;
  - Limits tiles showing the facts' values;
  - the privacy page naming Google Fonts and both theme settings;
  - Google's two font hosts as the only outside hosts allowed.
- **Existing tests** are changed only where a requirement makes an old expectation untrue. The plan's context lists each one: nav links, the Privacy link's landmark, the guide order, the outside-host and web-font checks, and the privacy wording.

**Most coverage** goes to the token package (it guards every future change) and to the board's interaction tests (themes, replay controls, menus), because those are where behaviour, not just looks, could regress.

**Success metrics from the spec**

- *Changing one token value restyles the board and the website together, with no component edits* — **behavioural test**: a Go test asserts both apps import the single generated stylesheet and that neither defines design values elsewhere (via the stray-value scanner). A **Manual — captured in the implementation test plan** check changes one token, rebuilds both apps and sees both change.
- *The stray-value and contrast checks stay green on every later change* — **behavioural test**: both checks are Go tests, so the full project test run fails on any regression.

**Manual reviews**

- **Manual — captured in the implementation test plan:** the maintainer reviews and accepts the Studio screenshots for all five board screen sizes (desktop, 1600px and wider, docked timeline, touch tablet or touch PC, phone) against the design canvas.
- **Manual — captured in the implementation test plan:** the maintainer reviews and accepts every website page's Studio screenshots against the canvas.
- **Manual — captured in the implementation test plan:** a check on a real iPhone and iPad that menus, the timeline sheet and themes behave as in the emulated tests (WebKit cannot run on the development machine).
- **Manual — captured in the implementation test plan:** a check that the live board and website load IBM Plex from Google Fonts and fall back cleanly to system fonts when the fonts are blocked.
- **Manual — captured in the implementation test plan:** a check that no shipped logo or icon shows Poké Ball imagery (beyond the automated trademark-word check on the listing icon's source).

**Deliberate gaps:** no cross-browser screenshot baselines (Chromium only, per the existing touch-test gotcha), and no automated pixel check that each PNG icon matches the mark's SVG. Icons are rendered by hand and reviewed.

## Milestones & Tasks

All three milestones land on the feature branch and reach players together in one merge to main, because every push to main deploys and the spec requires one release. Each milestone still ends at a checked, working state on the branch.

### Milestone 1: One set of design values, with nothing looking different

**What changes**: Nothing a player can see, on purpose. The board and the website stop keeping their own colours and sizes and draw every design value from one shared set of tokens. The website's hand-copied colour file goes away, and a check now fails if anyone hard-codes a colour or font size. Screenshots of every board layout and every website page, recorded before the change, still match afterwards. This internal milestone earns its place because it makes the Studio restyle a matter of changing values and styles, and the screenshots prove the groundwork broke nothing.

**Validation point:** the full project test run passes, including the new token checks. The screenshot comparisons recorded from today's board and website match after the move onto tokens.

#### - [x] Task: Land the wide-screen landscape mat
**Id:** 0bdc7a80-31e7-4a68-8ab4-123fb7acef4b
**Repo:** tcgreplay
**Depends on:** none
**Execution:** human — the maintainer's own uncommitted work in another worktree; only they can decide what to commit and merge

The landscape mat for board areas 1600px and wider was built under its own spec but never committed: it sits in the separate `tcgreplay-wide-board` worktree. The maintainer commits and merges it to main so Studio can restyle it rather than rebuild it.

*Technical detail:* [context.md#task-land-the-wide-screen-landscape-mat](./context.md#task-land-the-wide-screen-landscape-mat)

**Acceptance criteria**:
- [x] The board on main shows the landscape mat when the board area is 1600px or wider.
- [x] Its wide-screen and desktop browser tests are on main and pass.

#### - [x] Task: Record screenshots of today's board and website
**Id:** 02236978-8907-4171-b142-142de5471e08
**Repo:** tcgreplay
**Depends on:**
- 0bdc7a80-31e7-4a68-8ab4-123fb7acef4b — Land the wide-screen landscape mat

**Execution:** agent

Before anything changes, capture what the board and the website look like today. Baselines cover:
- the board on a phone, a tablet, a desktop, a wide screen, and a desktop with the timeline docked below;
- every website page at laptop and phone widths.

These become the proof that moving onto tokens changes nothing, and later the baselines Studio replaces. Captures wait for animations to settle and park the pointer, so they are repeatable.

*Technical detail:* [context.md#task-record-screenshots-of-today's-board-and-website](./context.md#task-record-screenshots-of-today's-board-and-website)

**Acceptance criteria**:
- [x] Screenshots of the board in all five layouts and of every website page are recorded and committed.
- [x] Running the screenshot comparisons twice in a row with no changes passes both times.

#### - [x] Task: Create the shared design-token package
**Id:** 78b11a6e-933e-49b2-bd46-cdb33daa5312
**Repo:** tcgreplay
**Depends on:**
- 02236978-8907-4171-b142-142de5471e08 — Record screenshots of today's board and website

**Execution:** agent

Add one shared home for design values: a token source holding today's colours, sizes and fonts, plus a generator that writes the stylesheet both apps load. A test fails when the stylesheet is out of date with the source. The board and the website both load it, the website's hand-copied colour file is deleted, and the container build copies the package into both front-end stages.

*Technical detail:* [context.md#task-create-the-shared-design-token-package](./context.md#task-create-the-shared-design-token-package)

**Acceptance criteria**:
- [x] Both the board and the website load their design values from the one shared stylesheet.
- [x] The website's hand-copied colour file is gone.
- [x] Editing the token source without regenerating makes the tests fail.
- [x] The board and website screenshots still match today's.

#### - [x] Task: Move the board's components onto the tokens
**Id:** 60a73948-7175-4270-a597-ff9bb67df23f
**Repo:** tcgreplay
**Depends on:**
- 78b11a6e-933e-49b2-bd46-cdb33daa5312 — Create the shared design-token package

**Execution:** agent

Replace every hard-coded colour and font size in the board's styles with the shared tokens, adding tokens with today's values where one is missing (toasts, badges, card faces, Energy colours and the like). The board must look exactly as it does today.

*Technical detail:* [context.md#task-move-the-board's-components-onto-the-tokens](./context.md#task-move-the-board's-components-onto-the-tokens)

**Acceptance criteria**:
- [x] No board component defines its own colour or px/rem font size.
- [x] The board's screenshots in all five layouts match today's.
- [x] The board's existing browser tests pass, with only the new screenshot test's capture made steadier.

#### - [x] Task: Move the website onto the tokens
**Id:** 53c7320f-cc62-4693-a32c-f6c52821203a
**Repo:** tcgreplay
**Depends on:**
- 78b11a6e-933e-49b2-bd46-cdb33daa5312 — Create the shared design-token package

**Execution:** agent

Replace every hard-coded colour and font size in the website's styles, page styles and the home page's animated scene with the shared tokens. The website must look exactly as it does today.

*Technical detail:* [context.md#task-move-the-website-onto-the-tokens](./context.md#task-move-the-website-onto-the-tokens)

**Acceptance criteria**:
- [x] No website page or component defines its own colour or px/rem font size.
- [x] Every website page's screenshots match today's.
- [x] The website's existing browser tests pass unchanged.

#### - [x] Task: Fail the build on design values outside the tokens
**Id:** 22e5086d-920f-475c-854f-99282b183d1c
**Repo:** tcgreplay
**Depends on:**
- 60a73948-7175-4270-a597-ff9bb67df23f — Move the board's components onto the tokens
- 53c7320f-cc62-4693-a32c-f6c52821203a — Move the website onto the tokens

**Execution:** agent

Add a check that scans the board and website sources and fails on any colour or font size that does not come from the tokens, naming the file, the line and the value. A short, reviewed list of allowed exceptions (such as `transparent` and sizes derived from the card width) keeps it practical, and the check is tested against planted examples so it cannot silently stop working.

*Technical detail:* [context.md#task-fail-the-build-on-design-values-outside-the-tokens](./context.md#task-fail-the-build-on-design-values-outside-the-tokens)

**Acceptance criteria**:
- [x] Adding a hard-coded colour or px/rem font size to any board or website component makes the tests fail with its location.
- [x] The check passes on the board and the website as they now stand.

### Milestone 2: The board in Studio, with themes

**What changes**:
- **Look:** the board takes on the Studio look on every screen (desktop, the landscape mat at 1600px and wider, a narrow screen with the timeline docked below, tablets and touch PCs, phones), with IBM Plex type and the new card back.
- **Readouts:** the Bench, Active panel and player blocks show HP left, damage, Energy and prizes at a glance.
- **Themes:** players choose a light or dark interface and a slate, felt or navy mat from ☰, and the choice is remembered.
- **Timeline:** every line keeps Play from here and Rewind to here, always visible, and each turn and each player's half has Rewind and Play.
- **Menus:** they appear in their Studio form, with the Knock Out hint in the attack panel and Card text on a card in hand.

**Validation point:** the board's suite passes with the new theme, replay-control, menu, Knock Out hint and card-back tests. Studio screenshot baselines exist for all five board screen sizes and are ready for the maintainer's review.

#### - [x] Task: Switch the tokens to Studio and check contrast
**Id:** ed9872d5-c5a1-40b7-86aa-7abd91026197
**Repo:** tcgreplay
**Depends on:**
- 22e5086d-920f-475c-854f-99282b183d1c — Fail the build on design values outside the tokens

**Execution:** agent

Replace the token values with the Studio design system's: both interface themes, the slate, felt and navy mats, the game colours, card faces, and the type, spacing, radius, shadow and size scales. Add a contrast check over every declared text-and-background pairing, in both themes and under each mat. Legacy token names stay as aliases until the components that use them are restyled.

*Technical detail:* [context.md#task-switch-the-tokens-to-studio-and-check-contrast](./context.md#task-switch-the-tokens-to-studio-and-check-contrast)

**Acceptance criteria**:
- [x] The tokens carry the Studio values for both interfaces and all three mats.
- [x] Every declared text-and-background pairing meets its contrast minimum in every interface-and-mat combination, and a failing pair makes the tests fail.

#### - [x] Task: Add the card-face tokens to the design system
**Id:** aa9cf918-985b-4656-b9bb-2679d179d399
**Repo:** tcgreplay
**Depends on:**
- ed9872d5-c5a1-40b7-86aa-7abd91026197 — Switch the tokens to Studio and check contrast

**Execution:** human — edits the TCG Replay Studio design system on claude.ai, outside the repo

The repo's tokens gain card-face colours that the design system does not have yet. The maintainer adds them to the TCG Replay Studio design system so the two stay in step. It also takes the felt mat's corrected muted colour, `felt-muted` #B4CBBD (was #AFC7B8, which fell short of 4.5:1 on felt pile chips).

*Technical detail:* [context.md#task-add-the-card-face-tokens-to-the-design-system](./context.md#task-add-the-card-face-tokens-to-the-design-system)

**Acceptance criteria**:
- [x] The design system lists the same card-face tokens, with the same values, as the repo.
- [x] The design system's `felt-muted` is #B4CBBD, as in the repo.

#### - [x] Task: Create the new mark and redraw every icon
**Id:** 370ad6c6-e03d-4337-a2fc-b98539f31f54
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Add the card-stack mark's vector source to the token package and render every logo and icon from it, keeping each file's current name and size: the website's logo and favicon, the README's logo and the ChatGPT listing icon (whose source is redrawn from the mark). The board's favicon becomes the mark.

*Technical detail:* [context.md#task-create-the-new-mark-and-redraw-every-icon](./context.md#task-create-the-new-mark-and-redraw-every-icon)

**Acceptance criteria**:
- [x] The website header, the website and board favicons, the README and the listing icon all show the card-stack mark.
- [x] No logo or icon shows Poké Ball imagery.
- [x] The listing icon still passes its size, format and trademark checks.

#### - [x] Task: Load IBM Plex and add theme settings to the board
**Id:** 4cb436ce-b503-4ded-8011-d307ce80ff43
**Repo:** tcgreplay
**Depends on:**
- ed9872d5-c5a1-40b7-86aa-7abd91026197 — Switch the tokens to Studio and check contrast

**Execution:** agent

- **Type:** the board loads IBM Plex Sans and Mono from Google Fonts, with system fallbacks.
- **Themes:** players choose the interface (light or dark) and the mat (slate, felt or navy) from ☰. The choice is remembered like the other display settings and applied before the board first appears, so it never flashes the default.
- **Tests:** browser tests serve the fonts from local files, so they stay offline and repeatable. New tests cover the default, all six combinations, unchanged layout across themes and remembering the choice.

*Technical detail:* [context.md#task-load-ibm-plex-and-add-theme-settings-to-the-board](./context.md#task-load-ibm-plex-and-add-theme-settings-to-the-board)

**Acceptance criteria**:
- [x] A first visit shows the light interface with the slate mat.
- [x] Each interface and mat can be chosen from ☰, changes only colours, and is still chosen after a reload.
- [x] The board's text is set in IBM Plex, and falls back to system fonts if Google Fonts is unreachable.

#### - [x] Task: Restyle the board's header and layouts
**Id:** 376fb958-82ec-48e2-ad38-11476611af01
**Repo:** tcgreplay
**Depends on:**
- 4cb436ce-b503-4ded-8011-d307ce80ff43 — Load IBM Plex and add theme settings to the board
- 370ad6c6-e03d-4337-a2fc-b98539f31f54 — Create the new mark and redraw every icon

**Execution:** agent

Restyle the board's frame in Studio:
- the header, with the mark, game, branch, mode switch, Reveal all, warnings, connection dot and ☰;
- the ☰ dropdown;
- toasts and the caption;
- the mat surface in each layout: phone, tablet, desktop, the landscape mat and the docked timeline.

On tablets and touch PCs, a header button hides and shows the timeline. Layout behaviour is unchanged.

*Technical detail:* [context.md#task-restyle-the-board's-header-and-layouts](./context.md#task-restyle-the-board's-header-and-layouts)

**Acceptance criteria**:
- [x] The board's header, menus, notices and mat surface match the Studio design in every layout.
- [x] On a tablet or touch PC the timeline can be hidden and shown from the header.
- [x] Layout switching between phone, tablet, desktop, wide and docked works as before.

#### - [x] Task: Restyle the mat's player blocks, Bench, Active panel, turn strip and hand
**Id:** c0e22301-e550-49ba-ba6f-a3b6a641be3c
**Repo:** tcgreplay
**Depends on:**
- 376fb958-82ec-48e2-ad38-11476611af01 — Restyle the board's header and layouts

**Execution:** agent

Give the parts of the mat their Studio readouts:
- **Player blocks:** prize pips, pile chips and the top of the discard.
- **Bench:** HP left and Energy under each card.
- **Active panel:** HP left over maximum, an HP bar, a damage tag and Energy, outlined in the player's colour.
- **Turn strip:** the Stadium, the turn tag and End turn in the accent colour.
- **Hand:** each card's kind.

The phone's counter bars get the same treatment. Only presentation changes; every action works as before.

*Technical detail:* [context.md#task-restyle-the-mat's-player-blocks,-bench,-active-panel,-turn-strip-and-hand](./context.md#task-restyle-the-mat's-player-blocks,-bench,-active-panel,-turn-strip-and-hand)

**Acceptance criteria**:
- [x] HP left, damage, Energy and prizes can be read on the mat without opening a card.
- [x] The mat matches the Studio design on every mat colour and in every layout.
- [x] Dragging, tapping and the existing board tests behave as before.

#### - [x] Task: Restyle cards and give face-down cards the Studio back
**Id:** da26c686-870b-46df-bb47-3faf972b3a20
**Repo:** tcgreplay
**Depends on:**
- 370ad6c6-e03d-4337-a2fc-b98539f31f54 — Create the new mark and redraw every icon
- ed9872d5-c5a1-40b7-86aa-7abd91026197 — Switch the tokens to Studio and check contrast

**Execution:** agent

Face-down and unknown cards show the new navy back with the mark, in place of the red and blue back. Text cards, Energy tokens and the full card view take their colours from the Studio tokens.

*Technical detail:* [context.md#task-restyle-cards-and-give-face-down-cards-the-studio-back](./context.md#task-restyle-cards-and-give-face-down-cards-the-studio-back)

**Acceptance criteria**:
- [x] Every face-down card on the board shows the navy back with the mark.
- [x] Text cards and Energy tokens use the Studio card colours.

#### - [x] Task: Restyle the timeline and add turn rewind
**Id:** 3209e87b-c8bf-4b61-9484-b025ff467c37
**Repo:** tcgreplay
**Depends on:**
- 376fb958-82ec-48e2-ad38-11476611af01 — Restyle the board's header and layouts

**Execution:** agent

Restyle the timeline and playback bar:
- **Controls:** every line shows Play from here and Rewind to here at all times. Each turn heading gains Rewind beside Play, and each player's half keeps Rewind and Play.
- **Scrubber:** coloured by player.
- **Docked layout:** a drag handle, one control row, and a turn list with a rewind for each player's half and the prize score after each turn.

*Technical detail:* [context.md#task-restyle-the-timeline-and-add-turn-rewind](./context.md#task-restyle-the-timeline-and-add-turn-rewind)

**Acceptance criteria**:
- [x] From any line, Play from here plays on from that event, and from any line but the newest, Rewind to here rewinds to just after it.
- [x] Each turn's and each half's Rewind and Play act on its start.
- [x] The controls are visible without hovering, and at least 44px on touch devices.
- [x] With the timeline docked below, the board and timeline can be resized and the turn list jumps to any player's half.

#### - [x] Task: Restyle the menus and add the Knock Out hint and Card text
**Id:** e62b7ca2-5d42-4ba5-818e-50abef0ef315
**Repo:** tcgreplay
**Depends on:**
- da26c686-870b-46df-bb47-3faf972b3a20 — Restyle cards and give face-down cards the Studio back

**Execution:** agent

Restyle the menus and their panels in Studio:
- the popover, the touch panel and the phone sheet;
- the attack panel, the Draw N and Look at top N panels, the stepper, the target banner, the pile and deck views, and the card zoom.

Two additions:
- **Knock Out hint:** the attack panel shows "N is enough to Knock Out X" whenever the chosen damage is at least the target's HP left, without ever blocking the attack.
- **Card text:** the menu for a card in hand gains a Card text item.

*Technical detail:* [context.md#task-restyle-the-menus-and-add-the-knock-out-hint-and-card-text](./context.md#task-restyle-the-menus-and-add-the-knock-out-hint-and-card-text)

**Acceptance criteria**:
- [x] A mouse right-click opens the popover, a tap opens the panel on a tablet or touch PC and the sheet on a phone, each in Studio styling.
- [x] The Knock Out hint appears exactly when the damage reaches the target's HP left, and attacking still works below it.
- [x] A card in hand offers Card text, which shows the card's text.
- [x] Target mode, the deck menu and the count panels match the Studio design.

#### - [x] Task: Record the board's Studio screenshots
**Id:** be0df998-bbaf-4ad2-91b0-565faadef60b
**Repo:** tcgreplay
**Depends on:**
- c0e22301-e550-49ba-ba6f-a3b6a641be3c — Restyle the mat's player blocks, Bench, Active panel, turn strip and hand
- 3209e87b-c8bf-4b61-9484-b025ff467c37 — Restyle the timeline and add turn rewind
- e62b7ca2-5d42-4ba5-818e-50abef0ef315 — Restyle the menus and add the Knock Out hint and Card text

**Execution:** agent

Replace the board's screenshot baselines with Studio ones for all five layouts, plus the menus in their three containers. Remove token aliases that nothing uses any more. Confirm the whole board suite passes.

*Technical detail:* [context.md#task-record-the-board's-studio-screenshots](./context.md#task-record-the-board's-studio-screenshots)

**Acceptance criteria**:
- [x] Studio screenshots exist for the board on desktop, wide, docked, tablet and phone, and for each menu container.
- [x] No legacy token name is left in the board, and the tokens keep only the legacy names the website still uses, until its pages are restyled.
- [x] The board's full browser suite and the Go tests pass.

### Milestone 3: The website and the new mark in Studio

**What changes**:
- **Pages:** every website page takes on the Studio look: the new header with Get started as a button, the dark footer, the restyled home scene with How it works, the docs layout with breadcrumbs and On this page, guides grouped into After a game, Practice and Play, a What's new timeline, Limits stat tiles, and restyled Example, Privacy, Terms and 404 pages.
- **The mark:** the card-stack mark replaces the old logo on the website, as the favicon, in the README and as the ChatGPT listing icon.
- **Privacy and What's new:** the privacy page names Google Fonts and the theme settings, and a What's new entry announces the redesign.

**Validation point:** the full project test run passes, both browser suites and the Go tests. Studio screenshot baselines exist for every website page. The listing icon test passes with the redrawn icon.

#### - [x] Task: Restyle the website's header, footer and docs layout
**Id:** 81234e4b-ffec-45b7-a1e6-827d43e6ac1c
**Repo:** tcgreplay
**Depends on:**
- ed9872d5-c5a1-40b7-86aa-7abd91026197 — Switch the tokens to Studio and check contrast
- 370ad6c6-e03d-4337-a2fc-b98539f31f54 — Create the new mark and redraw every icon

**Execution:** agent

Restyle the website's frame in Studio:
- **Header:** the mark, Guides, Example, Limits and What's new links, and Get started as a button. Home and Privacy move out of the header (the logo goes home; Privacy is in the footer).
- **Footer:** dark, with link columns, the notice and the credit.
- **Docs layout:** a breadcrumb and an On this page list, each in its own landmark.
- **Shared pieces:** copy blocks, prompt bubbles, steps, callouts and code blocks.

The website loads IBM Plex from Google Fonts, and its tests allow exactly Google's two font hosts and serve the fonts locally.

*Technical detail:* [context.md#task-restyle-the-website's-header,-footer-and-docs-layout](./context.md#task-restyle-the-website's-header,-footer-and-docs-layout)

**Acceptance criteria**:
- [x] Every page shows the new header with Get started as a button, and the dark footer with Privacy, Terms, Get help and the source link.
- [x] Docs pages show a breadcrumb, an On this page list when they have more than one section, and a sidebar with the same links in the same order.
- [x] The website loads nothing from other sites except Google Fonts (and Cloudflare's counting script where it is switched on).

#### - [x] Task: Restyle the website's pages and group the guides
**Id:** 17e1dbba-2205-4c78-baf0-d776be255795
**Repo:** tcgreplay
**Depends on:**
- 81234e4b-ffec-45b7-a1e6-827d43e6ac1c — Restyle the website's header, footer and docs layout

**Execution:** agent

Restyle every page in Studio:
- **Home:** the animated scene redrawn in Studio colours, plus How it works, What it does, guide cards and the address block.
- **Coaching workflows:** grouped into After a game, Practice and Play, keeping the playmat's order within each group.
- **What's new:** a timeline.
- **Example:** copy panels.
- **Limits:** stat tiles read from the site facts.
- **Terms** and the **404** page.

*Technical detail:* [context.md#task-restyle-the-website's-pages-and-group-the-guides](./context.md#task-restyle-the-website's-pages-and-group-the-guides)

**Acceptance criteria**:
- [x] Home, Coaching workflows, What's new, Example, Limits, Privacy and the 404 page match their Studio designs, and Terms, which has no design of its own, follows the docs text column.
- [x] Coaching workflows are grouped into After a game, Practice and Play.
- [x] The Limits tiles always show the current hosted limits.
- [x] The home page keeps its animated scene, now in Studio colours.

#### - [x] Task: Update the privacy page and announce the redesign
**Id:** 3134c162-0cd1-43f4-a1e7-5a51d2d25733
**Repo:** tcgreplay
**Depends on:**
- 4cb436ce-b503-4ded-8011-d307ce80ff43 — Load IBM Plex and add theme settings to the board
- 81234e4b-ffec-45b7-a1e6-827d43e6ac1c — Restyle the website's header, footer and docs layout

**Execution:** agent

Update the privacy page:
- name Google Fonts among its outside services;
- reword the sentence that says Cloudflare's script is the only thing the website loads from elsewhere;
- add the interface and mat to the display settings the board remembers.

Add a What's new entry announcing the redesign (no connection refresh needed).

*Technical detail:* [context.md#task-update-the-privacy-page-and-announce-the-redesign](./context.md#task-update-the-privacy-page-and-announce-the-redesign)

**Acceptance criteria**:
- [x] The privacy page lists Google Fonts as an outside service and names the theme settings among the board's remembered display settings.
- [x] What's new has an entry describing the new look and themes, marked as needing no refresh.

#### - [x] Task: Record the website's Studio screenshots
**Id:** 8ecbbeac-c51a-43e9-a05c-2354af01bb75
**Repo:** tcgreplay
**Depends on:**
- 17e1dbba-2205-4c78-baf0-d776be255795 — Restyle the website's pages and group the guides
- 3134c162-0cd1-43f4-a1e7-5a51d2d25733 — Update the privacy page and announce the redesign
- be0df998-bbaf-4ad2-91b0-565faadef60b — Record the board's Studio screenshots

**Execution:** agent

Replace the website's screenshot baselines with Studio ones for every page at laptop and phone widths, and confirm the whole project's tests pass, ready for the single release.

*Technical detail:* [context.md#task-record-the-website's-studio-screenshots](./context.md#task-record-the-website's-studio-screenshots)

**Acceptance criteria**:
- [x] Studio screenshots exist for every website page at laptop and phone widths.
- [x] The full project test run passes: Go tests, the board suite and the website suite.

## Open Questions

- **Do the screenshot comparisons stay stable run to run on the development machine?** This depends on Chromium's rendering of system fonts in milestone 1 and of the locally served IBM Plex later, and it can only be known by running them. If the recording task's double run differs, the implementer STOPS and asks the user before loosening thresholds or masking regions.
- **Does the landscape mat land on main as built in its worktree?** This depends on what the maintainer commits. If the merged landscape mat differs from what this plan's research saw (different files or projects), the implementer re-reads it and STOPS to ask before restyling it in a way the plan did not anticipate.
- **Can every token pairing meet contrast with the design system's values?** The design system was chosen to pass, but some on-mat pairs (for example muted text on the felt mat's zone) are close to 4.5:1 and only the check will tell. If a pair fails, the implementer STOPS and asks the user, because changing a Studio colour changes the agreed design.

## Out of Scope

- A dark theme for the website (spec Non-Goals). The website stays light and stores nothing in the browser.
- Changing the game's actions, gestures, dragging, target mode or menu model, beyond the controls the spec lists (spec Non-Goals).
- How card images are drawn. The board keeps the real card images; only text cards, Energy tokens and backs are restyled.
- Rewriting website copy, beyond the new navigation, the guide grouping and How it works (spec Non-Goals).
- Embedding the real board in the home page. The animated scene is restyled instead (decided during planning).
- Building the landscape mat. It lands first from its own spec (`20261104090653-7373a393-wide-screen-landscape-board`) and is only restyled here.
- Remembering the docked timeline's height between visits (decided during planning, to avoid a new browser setting).
- Cross-browser screenshot baselines (Chromium only) and an automated pixel check that each PNG icon matches the mark's source.
- Running the tests in CI. No test workflow exists; the full project test run stays the gate.
- Porting to React, syncing to Claude Design, or offering the other explored directions (spec Non-Goals).

## Changelog

### 2026-11-20 — Task: Land the wide-screen landscape mat

**What was done**: The maintainer asked the agent to land the uncommitted landscape-mat work. It was committed on its own branch (`spek/20261104090653-7373a393-wide-screen-landscape-board`, d120545), main was merged into it with three conflicts resolved (App.svelte imports, store.svelte.js `benchSlots` beside the card-image helpers, Strip.svelte paired-Stadium markup with the `wide` class), and it was merged to main locally as 4c38c9c (not pushed). The studio branch was fast-forwarded to it.

**Deviations**: A human task done by the agent at the maintainer's request, with each conflict resolution shown to them first. One addition beyond the landed work: a paired (Legendary) Stadium is drawn the size of a single Stadium on the landscape mat (`.wide .joined`), with a new `wide.spec.js` test, because main's 1.25-card pair spilled out of the landscape mat's Stadium spot.

**Amendments**: Design `design:wide-board/design.md` and `design:studio/design.md` gained a Legendary Stadium section (paired Stadiums reached main after both designs were written), with a mockup `design:studio/legendary-stadium.html`; both recorded on the spec with `spec amend`. Later restyle tasks (cards, turn strip, menus and zoom) build Legendary Stadiums to the Studio text.

**Files changed**:
- `web/e2e/desktop.spec.js`
- `web/e2e/wide.spec.js`
- `web/e2e/fixtures.js`
- `web/e2e/phone.spec.js`
- `web/playwright.config.js`
- `web/src/App.svelte`
- `web/src/components/Hand.svelte`
- `web/src/components/Side.svelte`
- `web/src/components/Strip.svelte`
- `web/src/components/Timeline.svelte`
- `web/src/lib/pointer.svelte.js`
- `web/src/lib/store.svelte.js`

**Discoveries**: In a fresh worktree `make test` fails `TestOpenAIChallengeNotSet` until `make site` has run once (the embedded site is only a placeholder). Running Playwright directly tests the last built board (`internal/webui/dist`), so rebuild with `make web` first. The landscape mat's `Timeline.svelte` uses `max(12px, 0.625vw)` font sizes, which the stray-value scanner (a later task) must allow or the token task must express through tokens.

### 2026-11-20 — Task: Record screenshots of today's board and website

**What was done**: Added Playwright screenshot comparisons to both browser suites, with baselines of today's look: the board in phone, tablet, desktop, wide and docked-timeline layouts, and every crawled website page plus the 404 at laptop and phone widths (22 pages each). Two comparison runs in a row with no changes both passed. Committed on the spec branch as 25e428e.

**Deviations**: None.

**Files changed**:
- `web/playwright.config.js`
- `site/playwright.config.js`
- `web/e2e/screenshots.spec.js`
- `site/e2e/screenshots.spec.js`
- `web/e2e/__screenshots__/` (5 baselines)
- `site/e2e/__screenshots__/` (44 baselines)

**Discoveries**: The site screenshot test crawls at run time, so a new page gets a baseline only on the next `--update-snapshots`; until then it fails as a missing snapshot. Snapshots use `snapshotPathTemplate` `{testDir}/__screenshots__/{projectName}/{arg}{ext}` with no platform suffix, since baselines are taken on the developer's Linux machine only. In the docked layout the timeline is scrolled so its SETUP heading is partly cut off; this is today's behaviour, captured as is.

### 2026-11-20 — Task: Create the shared design-token package

**What was done**: Added `internal/design`: `tokens.json` holds today's colours (dark default, light variant under `prefers-color-scheme`), a Go generator (`go generate ./internal/design`) writes the committed `tokens.css`, and both the board (`web/src/main.js`) and the website (`Base.astro`) import it. The website's hand-copied `tokens.css` is gone, and the Dockerfile copies the package into both front-end stages. Committed on the spec branch as 77063f9.

**Deviations**: Criterion 2 was reworded with the maintainer's approval from "The website no longer has its own copy of the colours" to "The website's hand-copied colour file is gone": the goal reviewer first read it as also covering the website's hard-coded colours, which task 5 (Move the website onto the tokens) replaces. Beyond the listed files, `site/astro.config.mjs` gained a dev-server `fs.allow` for `../internal/design`, as `web/vite.config.js` has, and the Dockerfile copies the package.

**Files changed**:
- `internal/design/tokens.json`
- `internal/design/tokens.css` (generated)
- `internal/design/design.go`
- `internal/design/gen.go`
- `internal/design/cmd/gen/main.go`
- `internal/design/design_test.go`
- `web/src/main.js`
- `web/src/app.css`
- `web/vite.config.js`
- `site/src/layouts/Base.astro`
- `site/astro.config.mjs`
- `site/src/styles/tokens.css` (deleted)
- `Dockerfile`

**Discoveries**: The generator already reads optional `type` (families and styles), `spacing`, `radius`, `shadow` and `size` families, so tasks 4 and 5 add tokens without generator changes. The board's light variant keeps one literal override in `app.css` (`main { color: #f2f6f3; --muted: #cfe0d5; }`) for task 4 to tokenise. The project's Go tests use the standard `testing` package, not testify.

### 2026-11-20 — Task: Move the board's components onto the tokens

**What was done**: Every hard-coded colour, shadow and px font size in the board's styles (`app.css`, `App.svelte` and 15 components) now reads a token with today's exact value: 78 colours (plain values, the same in both themes), 12 whole-value shadows, 14 font sizes (`--fs-9` to `--fs-40`) and the UI typeface (`--font-ui`). Screenshots in all five layouts still match. A new Go test checks every `var(--name)` the board uses resolves. Committed on the spec branch as 66ad261.

**Deviations**: Criteria 1 and 3 were reworded with the maintainer's approval: "colour or font size" became "colour or px/rem font size" (relative em sizes stay, as this task's technical note says), and "existing browser tests pass unchanged" became "pass, with only the new screenshot test's capture made steadier". The screenshot capture now waits for the timeline's smooth scroll to stop, after one full-suite run caught it 1px mid-scroll on the wide layout. `internal/design/refs_test.go` was added beyond the listed files.

**Files changed**:
- `internal/design/tokens.json`
- `internal/design/tokens.css` (generated)
- `internal/design/refs_test.go`
- `web/src/app.css`
- `web/src/App.svelte`
- `web/src/components/ActionSheet.svelte`, `AttackPanel.svelte`, `Card.svelte`, `CountPanel.svelte`, `CounterBar.svelte`, `Hand.svelte`, `Modal.svelte`, `PlaybackBar.svelte`, `Side.svelte`, `Stack.svelte`, `Stepper.svelte`, `Strip.svelte`, `TargetBanner.svelte`, `Timeline.svelte`, `Zoom.svelte`
- `web/e2e/screenshots.spec.js`

**Discoveries**: New tokens must be plain values, not `{dark, light}` pairs, or the light variant would change. The strip and the light-theme `main` override deliberately hold fixed colours (`strip-text`, `strip-muted`, `felt-ink`, `felt-muted`) rather than `--text`/`--muted`, which change by theme. `Strip.svelte` builds `var(--{seat})` at run time, so the seat tokens must stay named `p1`/`p2` until it changes. A literal `white` replacement also catches the `white-space` property: match colour keywords with a trailing `(?![\w-])`. `animations: 'disabled'` does not stop smooth scrolling, so screenshot captures must wait for scroll positions to settle. The goal reviewer reads criteria literally, so a criterion that could cover another task's work needs the owning task named in the verify summary.

### 2026-11-20 — Task: Move the website onto the tokens

**What was done**: The website's stylesheet, pages and the home page's animated scene read every colour and px/rem font size from the tokens with today's values, reusing the board's card, energy and shadow tokens where the values match. New tokens cover the text on the felt header, the light-theme link, the home button's ink, the scene's log line, coach box and sweep, 11 rem sizes (`--fs-r65` to `--fs-r230`) and the site typeface (`--font-site`). A new Go test checks every `var(--name)` the website uses resolves. Every website screenshot still matches. Committed on the spec branch as a674c7d.

**Deviations**: Criterion 1 was reworded before work started to "colour or px/rem font size", with the maintainer's approval. Criterion 3 was briefly reworded to match task 4's and then put back to its original "pass unchanged", which holds: no website test changed. Two board-test changes landed in this task's commits: the board's screenshot capture now pins the timeline to its bottom (in a674c7d), and the board's wide baseline was re-recorded (53622e9) from the pre-token board code (25e428e's `web/src`) with that steadier capture. Smooth scrolling had left the wide timeline at either of two positions 1px apart, so the full suite failed on it now and then. The tokenised board matches the re-recorded baseline in all five layouts, three runs in a row.

**Files changed**:
- `site/src/styles/site.css`
- `site/src/pages/index.astro`
- `site/src/pages/docs/index.astro`
- `site/src/pages/docs/whats-new.astro`
- `site/src/pages/docs/workflows/index.astro`
- `site/src/components/Playmat.astro`
- `internal/design/tokens.json`
- `internal/design/tokens.css` (generated)
- `internal/design/refs_test.go`
- `web/e2e/screenshots.spec.js`
- `web/e2e/__screenshots__/wide/board.png`

**Discoveries**: `images.spec.js` "when every source fails, each is tried in order before the text stand-in" (desktop) failed once in a full board run with a duplicated last image request, then passed 220 of 220 on repeat. It is a timing race in image fallback, unrelated to styles. When a baseline needs re-recording during the token phase, record it from the pre-token code (`git checkout 25e428e -- web/src`, build, `--update-snapshots`, then `git checkout HEAD -- web/src`) so the comparison still proves the move changed nothing.

### 2026-11-20 — Task: Fail the build on design values outside the tokens

**What was done**: Added a stray-value scanner to `internal/design` (`scan.go`, `StrayValues`) and `TestNoStrayDesignValues`, which walks `web/src` and `site/src` (stylesheets, `<style>` blocks, `style` attributes and Svelte `style:` directives) and fails on any hex colour, colour function, named colour in a colour property, or px/rem font size (including `--fs` and `--fs-*` properties) that is not a `var(--name)` reference, naming the file, line and value. `TestStrayScanner` checks it against planted values. It passes on today's tree. Run over pre-token copies of six files, it found 161 values. Committed on the spec branch as c6e22bd.

**Deviations**: Criterion 1 was reworded before work started to "colour or px/rem font size", with the maintainer's approval. The scanner lives in `scan.go` (package code) with its tests in `scan_test.go`, not in the test file alone. No allowlist was needed: `transparent`, `currentColor`, token fallbacks inside `var()` and relative sizes pass by construction, and `web/index.html` and `tokens.css` sit outside the scanned folders.

**Files changed**:
- `internal/design/scan.go`
- `internal/design/scan_test.go`

**Discoveries**: The scanner reads only style text, so colours set from JavaScript (`.js`/`.ts` style objects or `el.style`) are not checked; none exist today. A `/18px` after a font size in a `font:` shorthand is a line-height and is skipped. CSS `#id` selectors are not mistaken for colours, because only declaration values are read. Milestone 1's validation point holds: the full project test run passes with the token checks, and the screenshot comparisons match.

### 2026-11-20 — Task: Switch the tokens to Studio and check contrast

**What was done**: `tokens.json` now carries the Studio design system's values: light (default, on `:root`) and dark (`[data-theme='dark']`) interfaces, the slate, felt and navy mats (`[data-mat]` blocks repoint the nine `mat-*` aliases), game, brand and Energy colours, seven new card-face colours, IBM Plex type styles (`--fs-`, `--lh-`, `--fw-`, `--ls-`), and the spacing, radius, shadow (now themed) and size scales. The generator gained `{token}` references (written as `var()`), themed values in every family, mat aliases and `Resolve`. A contrast test checks 53 declared pairs in both themes and under every mat, and a planted failing pair is reported. Legacy names stay as aliases. Mat text is now light in every theme (`main` uses `--mat-ink`/`--mat-muted`). Committed on the spec branch as 7c7fdc1.

**Deviations**: Contrast findings stopped the task for the maintainer. All text pairs passed except muted text on felt pile chips (4.41:1), so `felt-muted` became #B4CBBD (4.61:1). Studio's hairline borders (1.2 to 1.8:1) cannot reach 3:1, so the 3:1 rule for borders was narrowed to borders that are the only thing showing a control's edge. Legacy `felt-ink`, `felt-muted` and `warn-ink` uses were renamed (`mat-ink`, `mat-muted`, `warn-badge-ink`), because Studio owns those names. Legacy `felt` became a radial gradient of the chosen mat's colours, which keeps the home scene's gradient stage that `home.spec.js` checks. Screenshot comparisons now differ from the pre-Studio baselines, as planned; the restyle tasks re-record them.

**Amendments**: Spec, Constraint "Contrast" and Acceptance Criterion "Contrast holds in every theme": 3:1 applies to large text, focus rings, markers, and any border that is the only thing showing a control's edge, not to every control border. Design `design:studio/design.md`, colour table: felt mat muted #AFC7B8 → #B4CBBD. Both recorded with `spec amend`, and the human design-system task now carries the felt-muted change.

**Files changed**:
- `internal/design/tokens.json`
- `internal/design/tokens.css` (generated)
- `internal/design/design.go`
- `internal/design/design_test.go`
- `internal/design/contrast_test.go`
- `web/src/app.css`
- `web/src/App.svelte`
- `web/src/components/CounterBar.svelte`

**Discoveries**: Playwright's default `toHaveScreenshot` threshold (0.2) did not flag the whole green-to-slate mat change: it misses hue shifts between dark colours. A zero-threshold recheck of Milestone 1 found only rendering noise (website diffs at most 1/255; board tablet and docked diffs that also appear when the pre-token board is compared with itself). Studio baselines should use `threshold: 0` with the existing `maxDiffPixelRatio: 0.002`. `[data-mat]` overrides must sit on the document root, because legacy aliases such as `--felt: var(--mat-surface)` resolve where they are defined.

### 2026-11-20 — Task: Add the card-face tokens to the design system

**What was done**: At the maintainer's request the agent updated the TCG Replay Studio design system (https://claude.ai/artifact/8uqeTrkeXAc2YYTUvwtEjK), republishing it as version 5. Its `project/tokens.json` gained the seven `card-*` text-card colours with the repo's values, `felt-muted` became #B4CBBD, the README's colour rules gained a line on text cards, and the index's `lastChange` records the edit. The published file was read back and matches the repo.

**Deviations**: A human task done by the agent at the maintainer's request. It also carried the `felt-muted` correction from the contrast task.

**Files changed**:
- No repo files. Outside the repo: the design system's `project/tokens.json`, `project/README.md` and `project/design-system.json`.

**Discoveries**: The design system's legacy-free view does not carry the repo's `card-back-*` and `card-text-*` names; those are token-step aliases that the card restyle drops. Publishing to a Design System artifact needs a read of the artifact itself (not only of its files) in the session first.

### 2026-11-20 — Task: Create the new mark and redraw every icon

**What was done**: Added `internal/design/mark.svg`, the Studio mark from the design system's Logos group (with its render commands in a comment). Rendered it to `site/public/logo.png` and `docs/logo.png` (512px) and `site/public/icon.png` and `docs/icon.png` (256px). Redrew `plugin/assets/icon.svg` as the mark at 512px and rendered `icon.png`. The board's favicon is now the mark as an SVG data URI. Tests check that the listing icon and favicon draw the mark's shapes, that every PNG is its expected size, and that the mark names no game trademark. Committed on the spec branch as e6bf34f.

**Deviations**: None.

**Files changed**:
- `internal/design/mark.svg`
- `internal/design/mark_test.go`
- `plugin/assets/icon.svg`, `plugin/assets/icon.png`
- `site/public/logo.png`, `site/public/icon.png`
- `docs/logo.png`, `docs/icon.png`
- `web/index.html`

**Discoveries**: The favicon is a data URI whose SVG contains `>` characters, so finding the end of its `<link>` tag by the first `>` cuts inside the SVG. A first edit did this and left the rest of the old icon as text in the page, which made the phone and wide boards scroll by 20px; the layout tests caught it. `TestIconsComeFromTheMark` now requires the favicon line to end right after its link.

### 2026-11-20 — Task: Load IBM Plex and add theme settings to the board

**What was done**: `web/index.html` loads IBM Plex Sans (400 to 700) and Mono (500, 600) from Google Fonts with `display=swap`, starts as `data-theme="light" data-mat="slate"`, and an inline script applies the remembered `theme` and `mat` before the app loads. The settings store gained `theme` and `mat` with `setTheme`/`setMat`, using the same try/catch storage pattern as the other display settings, and ☰ gained Interface (Light, Dark) and Mat (Slate, Felt, Navy) radio groups, 44px under a coarse pointer. Browser tests serve IBM Plex from `testdata/fonts` (a stand-in css2 response, the woff2 files and the OFL) through a context route in the shared fixture. `themes.spec.js` covers the defaults, all six pairs with a reload each, unchanged boxes across themes, blocked storage and the font fallback. Committed on the spec branch as d866b14.

**Deviations**: `testdata/fonts` holds one Plex Sans file for every weight, because Google serves Plex Sans as one variable font. The coarse-pointer rule in `app.css` now also sizes radio labels.

**Files changed**:
- `web/index.html`
- `web/src/lib/store.svelte.js`
- `web/src/App.svelte`
- `web/src/app.css`
- `web/e2e/fixtures.js`
- `web/e2e/themes.spec.js`
- `testdata/fonts/` (`css2.css`, `plex-sans.woff2`, `plex-mono-500.woff2`, `plex-mono-600.woff2`, `OFL.txt`)

**Discoveries**: `document.fonts.check()` returns true for a family the page never declared, so to prove a web font loaded, look for a loaded `FontFace` in `document.fonts`. The board's build minifies `#FFFFFF` to `#FFF` inside custom property values, so tests that read `--name` values must normalise short hex. The board server sends no Content-Security-Policy, so the inline theme script and Google Fonts load as they are. The board analytics tests abort every outside host, fonts included, and so run on the system font stack.

### 2026-11-20 — Task: Restyle the board's header and layouts

**What was done**: `App.svelte` has the Studio header: the mark, the game name, a mono branch picker, a Play/Review segmented switch (two radios labelled Play mode and Review mode), Reveal all, the warnings chip, the connection dot with "Live", and ☰. On tablets and touch PCs a header button hides and shows the timeline. The ☰ menu, toasts and caption are raised panels. The mat and timeline sit as rounded panels on the page background, joined when the timeline is docked; the phone stays full-bleed. `app.css` gives the mat a flat `--mat-surface` and the body the interface ink and Plex. A new `header.spec.js` covers the timeline button, the mark and the mode switch. Committed on the spec branch as 751000f.

**Deviations**: The wordmark stays "tcgreplay" (`phone.spec.js` checks it), and the mockup's "Games /" breadcrumb is left out because there is no games page to point to. The phone keeps its overlaid dot and ☰ for now; the mockup's phone header bar with the turn tag goes to the mat-parts task. Screenshot baselines are re-recorded once, in "Record the board's Studio screenshots", not per restyle task. The maintainer saw the board mid-restyle and confirmed carrying on with the mat.

**Files changed**:
- `web/src/App.svelte`
- `web/src/app.css`
- `web/e2e/header.spec.js`

**Discoveries**: The landscape mat can stall for seconds when the card size allowed by its height and by its width nearly match, so small changes to the frame around it can make `wide.spec.js` fail. The Studio frame uses a 4px gutter around the landscape mat for this reason (knowledge gotcha `landscape-mat-fit-stall`). The build inlines the imported mark as a `data:` URI, so tests should compare its content rather than its path.

### 2026-11-20 — Task: Restyle the mat's player blocks, Bench, Active panel, turn strip and hand

**What was done**: The mat now shows its readouts in the Studio style:
- **Player blocks:** name and seat dot, six prize pips with the count, Deck/Discard/Lost chips, and a "Top of discard" block.
- **Bench:** HP left under each card, in the damage colour when hurt, with Energy dots.
- **Active panel:** outlined in the seat colour, with HP left over maximum, an HP bar, the damage tag and the Energy count.
- **Turn strip:** the Stadium (a Legendary Stadium pair as one card), the turn tag, and End turn in the accent colour.
- **Hand:** a kind label on each card.
- **Phone:** count-over-label counter chips, the player's tinted Prizes chip first and a Hand chip last, and the header bar with the turn tag. Below 740px tall the turn tag moves into the strip.

New `prize-chip-p1`/`prize-chip-p2` tokens with contrast pairs, and `web/e2e/mat.spec.js` covers the readouts and the painted colours on every mat in both themes. Committed as fd61e19 and fc6082d. The goal check passed on its second attempt, after fixes for the short-phone turn indicator, the discard layout and prize taps.

**Deviations**:
- Kept for existing tests and actions:
  - The opponent's hand stays a fanned row on desktop and tablet, not a Hand chip.
  - Your hand stays fanned.
  - Attached Energy and Tool cards stay visible over the Pokémon, alongside the dots.
  - The landscape mat's tag reads "Turn 1", not "T1".
- Today's mat's Active card is 1.1 card widths (not 1.25), so everything fits 1440×900.
- On the phone, the header bar shows only at 740px tall or more.
- On touch screens the player block follows the iPad mockup's two-column chip grid.
- **Prizes:** with a mouse, a click on a pip takes that prize, as clicking its card did, and the count opens the prize view. On a touch screen the pip row is one fingertip-sized button that opens the prize view (two taps instead of one), to keep the 44×44 touch-target rule. The maintainer accepts behaviour changes that are improvements.
- **Baselines:** the board's screenshot baselines were re-recorded here (the plan's approach: "Each restyle replaces its screenshot baselines"), so the goal check could run the full suite. "Record the board's Studio screenshots" still re-records them at the stricter threshold.

**Files changed**:
- `web/src/App.svelte`
- `web/src/components/Side.svelte`
- `web/src/components/Stack.svelte`
- `web/src/components/Strip.svelte`
- `web/src/components/Hand.svelte`
- `web/src/components/CounterBar.svelte`
- `internal/design/tokens.json`
- `internal/design/tokens.css`
- `web/e2e/mat.spec.js`
- `web/e2e/drag.spec.js`
- `web/e2e/fixtures.js`
- `web/e2e/__screenshots__/desktop/board.png`
- `web/e2e/__screenshots__/desktop/board-docked.png`
- `web/e2e/__screenshots__/tablet/board.png`
- `web/e2e/__screenshots__/phone/board.png`
- `web/e2e/__screenshots__/wide/board.png`

**Discoveries**:
- Letting attachments hang below cards on the landscape mat stalled its fit loop for about 7.5s after a resize. At 2560×1440 the landscape mat has about 1px of height to spare against `wide.spec.js`'s "1.6× today's width" check (knowledge gotcha `landscape-mat-fit-stall`, updated).
- At 1024×768 the tablet hand sits at the fold, so a taller seat row breaks drag.spec's "discard → hand" drop. touch.spec's 44×44 rule is better met by grouping small controls into one button than by loosening it (knowledge gotcha `playwright-touch-tests`, updated).
- `fixtures.js` gains `discardTop(page, seat)` for the top-of-discard block; `pile()`/`pileLabel()` now match the Discard chip.

### 2026-11-20 — Task: Restyle cards and give face-down cards the Studio back

**What was done**: Face-down cards show the Studio back: `--brand-navy` with a `--brand-cyan` inset frame and the TCG Replay mark (imported from `internal/design/mark.svg`), marked `data-back="studio"`. An unidentified card shows the same back, dimmed, under a "?". Text cards take the Studio `card-face`/`card-face-trainer` faces, `card-border-*` borders, `card-face-ink` and `card-hp`; Energy tokens gain Colorless; cards and the full card view use Plex and the radius and shadow tokens. `web/e2e/cards.spec.js` checks the back in every layout and the card colours. Committed as 13294fc; the goal check passed first time.

**Deviations**: The back check is a new `cards.spec.js` rather than an addition to `images.spec.js` (left untouched, its fallback assertions intact). The board's screenshot baselines were re-recorded for the new back, as each restyle now does. The website's copied back in `Playmat.astro` is left for the website pages task, as planned.

**Files changed**:
- `web/src/components/Card.svelte`
- `web/src/components/Zoom.svelte`
- `web/e2e/cards.spec.js`
- `web/e2e/__screenshots__/desktop/board.png`
- `web/e2e/__screenshots__/desktop/board-docked.png`
- `web/e2e/__screenshots__/tablet/board.png`
- `web/e2e/__screenshots__/wide/board.png`

**Discoveries**: The test board has no unidentified ("?") cards, so that back is checked only by its markup. The legacy `card-back-*` and `card-text-*` tokens are now used only by the website's `Playmat.astro`; drop them once the website pages task moves it onto the Studio names.


### 2026-11-20 — Task: Restyle the timeline and add turn rewind

**What was done**:
- **Lines:** every timeline line shows Play from here and Rewind to here without a hover, 44px on touch. The newest line has no Rewind, by design.
- **Headings:** each "Turn N" heading gains Rewind (branch `turn-N`) and Play. Each player's half keeps both, now labelled. All act on the turn's or half's start (`startOf`, now shared in `web/src/lib/timeline.js`).
- **Playback controls:** Previous step, Play/Pause, Next step, the position, a scrubber coloured by player (new `scrub-setup` token), Speed and Go live. They head the timeline beside or below the board, and float over the board only when the timeline is hidden or on the phone. Play with no replay running plays the current half from its start.
- **Docked below:**
  - A drag handle (`role="separator"`, pointer and keys, height in memory only), with the board's cards following the split.
  - One control row.
  - A turn list showing prizes left after each turn and a rewind for each player's half.
- **Board area:** it now scrolls from its top (`justify-content: safe center`) instead of clipping.
- **Tests and commits:** new `web/e2e/timeline.spec.js`. Committed as 5d77c81. The goal check passed on its second attempt, after the criterion rewording below.

**Deviations**:
- **Agents:** one implementer instead of two parallel agents, since both halves edit `App.svelte`.
- **Side panel:** keeps its "Timeline" h2 and dock button, which tests need. At the panel's 320px width, speed and Go live share the scrubber's row. The h4 drops the design's "T5" prefix because the turn has its own h3. Line text stays 12px (`desktop.spec` pins it).
- **Touch and docked lines:** per-line ▶/↺ are kept on tablet and in the docked lines, which the iPad and docked mockups drop; the plan's criterion 3 wins.
- **Speed control:** a select styled as the "1×" button.
- **Docked range:**
  - The timeline's minimum is 160px.
  - At 1440×900 the range is about 160–280px, because today's mat needs about 540px at its 40px card floor.
  - On a tablet the board keeps half the room and scrolls, since 48px touch cards can't fit under a docked timeline.
- **Existing tests:** the selectors in `desktop.spec`, `touch.spec` and `phone.spec` now use the new control names. The desktop playback test now expects the controls in the timeline panel when it's beside or below the board. The board's screenshot baselines were re-recorded.

**Amendments**: The plan's criterion 1 was narrowed, with the maintainer's approval, to "From any line, Play from here plays on from that event, and from any line but the newest, Rewind to here rewinds to just after it". The newest line deliberately has no Rewind, because rewinding there changes nothing, and `phone.spec.js` checks this.

**Files changed**:
- `web/src/components/Timeline.svelte`
- `web/src/components/PlaybackBar.svelte`
- `web/src/App.svelte`
- `web/src/lib/timeline.js`
- `web/src/lib/playback.svelte.js`
- `web/src/lib/store.svelte.js`
- `internal/design/tokens.json`
- `internal/design/tokens.css`
- `web/e2e/timeline.spec.js`
- `web/e2e/touch.spec.js`
- `web/e2e/desktop.spec.js`
- `web/e2e/phone.spec.js`
- `web/e2e/__screenshots__/desktop/board.png`
- `web/e2e/__screenshots__/desktop/board-docked.png`
- `web/e2e/__screenshots__/tablet/board.png`
- `web/e2e/__screenshots__/wide/board.png`

**Discoveries**:
- **Centred scrolling areas:** a flex-centred scrolling `<main>` cuts off an overflowing board's top out of reach unless it uses `safe center` (knowledge gotcha `board-area-safe-center`).
- **Docked card size:** the docked today's-mat `--cw` formula (÷11.1) runs short at small cards (≈9.2·cw + 143px), so the docked mat also relies on `fitBoard`/`--fit` (gotcha `css-container-units-card-size`, updated).
- **Tests:** notes can't be added from the browser (use `routeWebSocket`), and generic class names like `.body` collide with page-level test locators (gotcha `board-browser-test-blind-spots`).
- **Turns ended by tests:** a test that ends turns with `act()` must pass the seat once it's p2's turn. Moves sent over a test's own websocket count as another client's, so the board holds them until a reload.

### 2026-11-21 — Task: Restyle the menus and add the Knock Out hint and Card text

**What was done**:
- The action sheet takes the Studio look in all three containers:
  - **Mouse popover:** 264px, with a mono summary such as "Hand · Basic Pokémon · 70 HP", uppercase section labels, a grid of move buttons in the design's order, cost dots and mono damage on attacks, a target icon on target rows, and Knock Out last in `--danger`.
  - **Touch panel:** 900×660, with a 320px `--panel-sunk` column holding the card and a Card/Text switch, mono facts with a damage tag, and a 44px bordered Close.
  - **Phone sheet.**
- **Attack panel:**
  - Cost dots and the effect text.
  - "×2 Weakness" and "−30 Resistance" toggles (`aria-pressed`) applied to the stepper's base damage.
  - A target card with HP left.
  - The hint "N is enough to Knock Out X." whenever the final damage reaches the target's HP left, never blocking the attack.
- **Card text:** a card in hand offers Card text (`app.zoom = card`).
- **Studio family:** the target banner, count panels, stepper, pile and deck views and the zoom follow it.
- **Shared code:**
  - `hpOf`/`hpLeft`/`energyType` move into `lib/actions.js`, shared with `Stack.svelte`.
  - New `Icon.svelte`.
- **Tokens:** a new `scrim` token, and 19 legacy tokens nothing used any more are removed.
- **Tests:** new `web/e2e/menus.spec.js`.
- Committed as 4421c5c; the goal check passed first time.

**Deviations**:
- Kept because tests pin them:
  - The exact item labels (`Return to hand`, `Devolve`, `−10`/`+10` stepper buttons) rather than the design's longer or bare variants.
  - Rows stay buttons, not `menuitem`s.
  - Retreat shows no "2 Energy" detail (sheet.spec pins `Retreat ›`).
- **Label prefixes:** item labels stay the first child (icons and dots are shown first with CSS order), since sheet.spec reads `firstElementChild`.
- **Missing data:** the popover's primary action has no hint line ("Goes to the first free Bench spot…"); the menu model has no data for it.
- **Layout:**
  - The tablet attack panel keeps the Card/Text switch in its side column.
  - On a 1024px tablet the Weakness/Resistance toggles wrap below the stepper.
- **Changes from the old behaviour:**
  - ×2 became a toggle and the move order follows the design; sheet.spec and panels.spec are updated to match.
  - The touch close in pile views is a bordered X icon, and touch.spec is updated.
- **No mockup:** the target banner, count panels, deck menu, pile views and zoom were styled by analogy with the popover and panel.
- **Dark theme:** tokens.json's `line`/`control-line` values win over the mockups' slightly different ones.

**Files changed**:
- `web/src/components/ActionSheet.svelte`
- `web/src/components/AttackPanel.svelte`
- `web/src/components/CountPanel.svelte`
- `web/src/components/Stepper.svelte`
- `web/src/components/TargetBanner.svelte`
- `web/src/components/Modal.svelte`
- `web/src/components/Zoom.svelte`
- `web/src/components/Stack.svelte`
- `web/src/components/Icon.svelte`
- `web/src/lib/actions.js`
- `internal/design/tokens.json`
- `internal/design/tokens.css`
- `web/e2e/menus.spec.js`
- `web/e2e/sheet.spec.js`
- `web/e2e/panels.spec.js`
- `web/e2e/touch.spec.js`

**Discoveries**:
- Four Svelte and scoped-CSS traps (knowledge gotcha `svelte-css-sharp-edges`):
  - `{#if}` drops a leading space;
  - a flex-row input needs `width: 0`;
  - component CSS can undo the global 44px touch minimum;
  - an attribute selector outranks ActionSheet's per-mode rules.
- **Weakness/Resistance data:** `/api/card` doesn't send weaknesses or resistances, so the toggles are manual.

### 2026-11-21 — Task: Record the board's Studio screenshots

**What was done**:
- **Screenshots:** `web/e2e/screenshots.spec.js` gains a capture of a card's actions in each container: the desktop popover, the tablet panel and the phone sheet (`menu.png`). The board captures stay as before: phone, tablet, desktop, wide, docked. `web/playwright.config.js` compares at `threshold: 0` with `maxDiffPixelRatio` 0.002.
- **Legacy tokens:**
  - The board no longer uses any legacy token. Its uses moved to Studio tokens:
    - `text`→`ink`
    - `accent-ink`→`on-accent`
    - `code-bg`→`panel-sunk`
    - `card-bg`→`mat-zone`
    - `energy-special`→`energy-colorless`
    - the known badge moved from accent to `primary`/`on-primary`
  - Values with no Studio counterpart got Studio names: `drop-target`, `highlight-glow`, `glyph-ink`, `assumed-badge`, `condition-poisoned` and `condition-burned`.
  - Descriptive tokens kept their names with proper usage notes: `energy-ring`, `mask-fade` and the card shadows.
  - 28 unused aliases are deleted.
  - The 28 names the website still uses are marked "used only by the website".
  - A new Go test, `internal/design/legacy_test.go`, keeps `web/src` off legacy names.
- Committed as 0370cb8; the goal check passed first time.

**Deviations**:
- The wide layout has no `menu.png`, because its popover is the desktop's.
- The board baselines for desktop, wide, docked and tablet were recorded by the restyle tasks and still match at threshold 0. The phone board needed re-recording at the stricter setting.
- `make test` still exits non-zero, but only on the website's two screenshot comparisons, whose pre-Studio baselines "Record the website's Studio screenshots" re-records.

**Amendments**: The plan's criterion 2 was narrowed, with the maintainer's approval, to "No legacy token name is left in the board, and the tokens keep only the legacy names the website still uses, until its pages are restyled". Removing the website's legacy names now would break the website before its own restyle tasks.

**Files changed**:
- `web/e2e/screenshots.spec.js`
- `web/playwright.config.js`
- `web/e2e/__screenshots__/phone/board.png`
- `web/e2e/__screenshots__/desktop/menu.png`
- `web/e2e/__screenshots__/tablet/menu.png`
- `web/e2e/__screenshots__/phone/menu.png`
- `web/src/app.css`
- `web/src/components/Card.svelte`
- `web/src/components/Stack.svelte`
- `internal/design/tokens.json`
- `internal/design/tokens.css`
- `internal/design/legacy_test.go`

**Discoveries**:
- The website pages task must move `site/src` (`Playmat.astro`, `site.css`, `index.astro`) off the 28 tokens marked "used only by the website", then delete them.
- Once that is done, `legacy_test.go` could widen to `site/src`.

### 2026-11-21 — Task: Restyle the website's header, footer and docs layout

**What was done**:
- **Header:** the mark and "TCG Replay"; Guides, Example, Limits and What's new; and Get started as a button. Home and Privacy left the header.
- **Footer:** dark (`data-theme="dark"` scoping the Studio names), with a Docs links column, a Project column (`nav aria-label="Footer"`: Privacy, Terms, Get help, Source on GitHub), the fan notice and the credit.
- **Docs pages:** a Breadcrumb landmark, and an On this page landmark built from the article's h2s by the new `site/src/lib/headings.ts`, which also gives h2s missing an id a unique slug. Docs pages are one column on a phone, two from 52rem and three from 76rem. The Guide sidebar keeps its 12 links, restyled to the DocsNav mockup.
- **Shared pieces:** prose, step badges, `.note` callouts, code, and copy blocks in three looks (dark code box, address band, prompt bubble) take the Studio look.
- **Fonts:** IBM Plex loads from Google Fonts. The new `site/e2e/fixtures.js` serves both font hosts from `testdata/fonts`, every site spec imports it, and the outside-host checks allow exactly those two hosts.
- Committed as 6902e46. The goal check passed on its third attempt, after the criterion rewordings below; the code did not change.

**Deviations**:
- The Project nav keeps the name "Footer" and the sidebar keeps "Guide" (tests).
- Copy buttons stay text.
- The prompt block itself is the "Check it works" card, since page content is the pages task's.
- Example's decklist became a dark code box until the pages task.
- Callout text uses `warn-ink`.
- On the phone the header wraps rather than collapsing into a menu.
- The test that `document.fonts` is empty no longer holds and was replaced by host checks.

**Amendments**: The maintainer said contradictions between "unchanged" wording and deliberate improvements should be resolved in favour of the improvement. Criterion 2 was narrowed twice, from "…and the sidebar is unchanged" to "Docs pages show a breadcrumb, an On this page list when they have more than one section, and a sidebar with the same links in the same order". The sidebar is restyled per the design, and the list is left out on pages with fewer than two sections.

**Files changed**:
- `site/src/layouts/Base.astro`
- `site/src/layouts/Docs.astro`
- `site/src/styles/site.css`
- `site/src/lib/headings.ts`
- `site/src/components/CopyBlock.astro`
- `site/src/scripts/copy.ts`
- `site/e2e/fixtures.js`
- `site/e2e/crawl.js`
- `site/e2e/site.spec.js`
- `site/e2e/analytics.spec.js`
- `site/e2e/animation.spec.js`
- `site/e2e/privacy.spec.js`
- `site/e2e/home.spec.js`
- `site/e2e/docs-index.spec.js`
- `site/e2e/connect.spec.js`
- `site/e2e/example.spec.js`
- `site/e2e/facts.spec.js`
- `site/e2e/limits.spec.js`
- `site/e2e/notice.spec.js`
- `site/e2e/screenshots.spec.js`
- `site/e2e/terms.spec.js`
- `site/e2e/whats-new.spec.js`
- `site/e2e/workflows.spec.js`

**Discoveries**:
- An always-dark surface takes `data-theme="dark"`. Site specs must use the font fixture, and a later catch-all route must `route.fallback()` for the font hosts (knowledge gotcha `site-dark-surfaces-and-font-routing`).
- For later tasks:
  - The privacy page now wrongly says Cloudflare's script is the only outside load, and omits Google Fonts (privacy task).
  - Example, Terms and Privacy have Astro dropped-space bugs such as "Seeimporting a game" (gotcha `astro-inline-whitespace`; pages task).
  - Home keeps its legacy `.button` and Playmat styles (pages task).

### 2026-11-21 — Task: Restyle the website's pages and group the guides

**What was done**:
- **Home:**
  - the Studio hero (badge, display h1, the unchanged pitch, Get started and See an example game);
  - How it works in three steps;
  - a full-width slate What it does band;
  - four guide cards whose prompts come from the guides' frontmatter;
  - a navy address block.
- **Home scene (`Playmat.astro`):** redrawn in Studio colours, with a slate mat, panels, the turn strip and a note-box coach. It uses the board's navy card back, with the mark as an inline aria-hidden SVG.
- **Coaching workflows:** grouped into After a game (import-game, opponent-deck, review-game), Practice (replay-drill, before-you-move, numbers-drill) and Play (play-game).
  - A `group` field in each guide's frontmatter, enforced by the schema.
  - `groupWorkflows()` keeps the guides.go order within each group.
  - The page shows equal-column cards under group headings.
- **Other pages:**
  - What's new becomes a timeline.
  - Example has step cards, a light decklist panel and a dark log panel (a new optional `theme` prop on CopyBlock).
  - Limits gets stat tiles read from the site facts.
  - Terms and Privacy become a text column with an On this page card. Privacy's wording is unchanged.
  - The 404 page shows a CSS stack of cards with ways back.
- **Fixes:** dropped spaces on Example, Terms and Privacy.
- **Legacy tokens:** the last 28 are deleted. `legacy_test.go` guards `web/src` and `site/src` against their return.
- **Commits and review:** committed as edd13e6 and 995479b. The goal check passed on its second attempt, after the Privacy page was restyled.

**Deviations**:
- **Agents:** two parallel implementers, not three, plus a third agent for Privacy.
- **Privacy:** it wasn't on this task's page list, but it has a Studio design and the milestone names it, so it was restyled here. Its content changes stay with the next task.
- **Layout compromises** inside the Docs layout's three columns:
  - Example's panels stack.
  - The Limits tiles sit 2×2, and the value reads "60 minutes".
  - The guide cards also show each guide's description, which tests require.
  - The guides page keeps the docs sidebar.
- **Copy left out:** the home hero has no new mono line, which would be new copy.
- **Token rounding:** a few mockup colours with no token map to the nearest Studio token (badge text, address field), and section titles stop at the 28px token.
- **Fixed tile:** the "1 game per account" tile is literal, because it's a product rule, not a hosted setting.

**Amendments**: Following the maintainer's guidance on wording that contradicts the designs, criterion 1 was narrowed from "Each page matches its Studio design" to "Home, Coaching workflows, What's new, Example, Limits, Privacy and the 404 page match their Studio designs, and Terms, which has no design of its own, follows the docs text column".

**Files changed**:
- `site/src/pages/index.astro`
- `site/src/components/Playmat.astro`
- `site/src/content.config.ts`
- `site/src/content/docs/workflows/*.md`
- `site/src/lib/workflows.ts`
- `site/src/pages/docs/workflows/index.astro`
- `site/src/pages/docs/whats-new.astro`
- `site/src/pages/docs/example.astro`
- `site/src/pages/docs/limits.astro`
- `site/src/pages/terms.astro`
- `site/src/pages/privacy.astro`
- `site/src/pages/404.astro`
- `site/src/components/CopyBlock.astro`
- `internal/design/tokens.json`
- `internal/design/tokens.css`
- `internal/design/legacy_test.go`
- `site/e2e/home.spec.js`
- `site/e2e/workflows.spec.js`
- `site/e2e/whats-new.spec.js`
- `site/e2e/example.spec.js`
- `site/e2e/limits.spec.js`
- `site/e2e/terms.spec.js`
- `site/e2e/privacy.spec.js`
- `site/e2e/not-found.spec.js`

**Discoveries**:
- **Astro whitespace:** Astro also drops the space before a `{expression}` on the next line, and after a closing inline tag followed by text (gotcha `astro-inline-whitespace`, updated with a grep for both directions).
- **Astro scoping and Playwright** (gotcha `site-test-and-scoping`):
  - Astro scopes `*` too, so `.x > *` outranks a single-class rule.
  - Only one site Playwright run can happen per worktree.
- **Home page tricks** (gotcha `site-home-layout-tricks`): a full-width band inside `main` uses a spread shadow plus `clip-path`, and inline SVG in the scene needs `aria-hidden`.

### 2026-11-21 — Task: Update the privacy page and announce the redesign

**What was done**:
- **Privacy page:**
  - It now lists Google Fonts among the outside services. Google Fonts serves IBM Plex to both the website and the board, and sees the request and IP address but no cookies.
  - The cookie paragraph now says the website and the board load only the typeface from Google Fonts and Cloudflare's visitor-counting script from other sites.
  - The board's remembered display settings are now six, adding the interface theme and the mat.
  - The "last updated" date is 21 November 2026.
- **What's new:** a new top entry, "A new look" (`refresh: false`). It covers the new board look and readouts, light/dark and slate/felt/navy themes in ☰, the timeline's turn rewind and resizing, the Knock Out hint and Card text, the new logo, and the refreshed website.
- **Tests:** `privacy.spec.js` and `whats-new.spec.js` check both.
- Committed as 493e814. The goal check passed first time.

**Deviations**:
- The What's new entry is dated 2026-11-21, the day it was written. If the release merges on a later day, the maintainer may want to move it to that day, since the convention dates entries by the day they ship.
- `internal/site/site.go` needed no change: the `data-retention` markers are untouched.

**Files changed**:
- `site/src/pages/privacy.astro`
- `site/src/data/whats-new.yaml`
- `site/e2e/privacy.spec.js`
- `site/e2e/whats-new.spec.js`

**Discoveries**:
- A What's new change line containing `: ` parses as a YAML map, and the build stops with "has an empty change". Quote such lines (convention `whats-new-entries`, updated).

### 2026-11-21 — Task: Record the website's Studio screenshots

**What was done**: The website's screenshot baselines were re-recorded in the Studio look for all 22 pages the crawl reaches (the 404 page included), at laptop and phone widths: 44 images. `site/playwright.config.js` now compares at `threshold: 0`, as the board does, so a change between close shades counts. Two comparison runs after recording both passed. The full `make test` passes: Go vet and tests, the board suite (565 passed) and the website suite (298 passed). Committed as 7ecd699; the goal check passed first time.

**Deviations**: `site/playwright.config.js` changed (the threshold), beside the screenshot baselines.

**Files changed**:
- `site/playwright.config.js`
- `site/e2e/__screenshots__/laptop/*.png`
- `site/e2e/__screenshots__/phone/*.png`

**Discoveries**: None.

