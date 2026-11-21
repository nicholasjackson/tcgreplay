---
created_date: "2026-11-20"
document_status: final
closed_date: "2026-11-20"
---

# Research: 20261120140830-52277437-studio-redesign

## Alternatives considered and rejected

- **Token source inside `web/src/app.css`, imported by the site across projects.** Rejected: the Dockerfile `site` stage copies `site/`, `internal/guides`, `internal/notice`, `.do/app.yaml` and `testdata/decks` only (Dockerfile:40-49), not `web/`. The hand copy in `site/src/styles/tokens.css:1-3` exists precisely because there is no shared place today.
- **A separate npm package or workspace for tokens.** Rejected as heavier than needed. The repo already shares data between the two apps with a plain file under `internal/` (`internal/notice/notice.json`, imported by `web/src/components/Modal.svelte:10` with `web/vite.config.js` `server.fs.allow`, and read by `site/src/lib/notice.ts:95`).
- **stylelint (or eslint plugins) for the stray-value check.** Rejected: neither app has any lint tooling (`web/package.json`, `site/package.json`: no lint scripts or configs), and CI runs no tests (`.github/workflows/deploy.yml` only deploys). A Go test runs inside the existing `go test ./...` in `make test` (Makefile:53-58).
- **A browser computed-style check for stray values.** Rejected: it sees resolved colours, not whether a source literal came from a token, so it cannot fail on a hard-coded value that happens to equal a token.
- **Embedding the real board in the home hero.** Rejected by the user. `site/e2e/home.spec.js` and `animation.spec.js` protect the self-drawn `Playmat.astro` scene (at least 5 `.text-card`, `.back-art`, `.token`, a radial-gradient `.stage`, no images or url() backgrounds, home page under 1.5 MB, `animation.spec.js:93-105`). The scene is restyled instead, and `studio/design.md` was revised to say so.
- **Self-hosting IBM Plex.** Rejected by the user ("web fonts are fine"). Fonts come from Google Fonts. Self-hosting would still have failed `site.spec.js:171-207`, which forbids any `@font-face` and any loaded font.
- **Rebuilding the landscape mat in this plan.** Rejected by the user. The wide-screen implementation exists, uncommitted, in the worktree `../tcgreplay-wide-board` (branch `spek/20261104090653-7373a393-wide-screen-landscape-board`, changes to App.svelte, Hand, Side, Strip, Timeline, store, pointer, fixtures, phone.spec, playwright.config, plus new `desktop.spec.js` and `wide.spec.js`). It must land on main first (spec constraint "The landscape mat lands first").
- **Ordering the guide groups Play / After a game / Practice to keep `workflows.spec.js:48-59` passing untouched.** Rejected: the design and spec fix the order as After a game, Practice, Play. The spec allows test changes forced by the guide grouping, so the order assertion changes to "guides.go order within each group".

## Chosen approach — evidence

- **Shared token directory under `internal/`:** precedent `internal/notice/notice.json` (Modal.svelte:10; vite.config.js `server.fs.allow: ['.', '../internal/notice']`; site/src/lib/notice.ts:95; Dockerfile:28-34 web stage and :40-49 site stage both copy `internal/notice`).
- **Token naming and values:** the TCG Replay Studio design system's `tokens.json` (https://claude.ai/artifact/8uqeTrkeXAc2YYTUvwtEjK). Themes are `light`/`dark`; mats are the `slate-*`, `felt-*` and `navy-*` sets plus `mat-*` aliases.
- **Both apps already switch on `data-theme`:** app.css:29-30, tokens.css:26-27 and site.css:18-19 use `:root:not([data-theme='dark'])`, but nothing sets the attribute yet. The board will set `data-theme` and `data-mat` on `<html>`.
- **Remembered settings pattern:** four localStorage keys, each wrapped in try/catch. `panelAt` and `prizeReminders` are in `web/src/lib/store.svelte.js:70-71,111,121`; `replaySpeed` and `autoplay` are in `web/src/lib/playback.svelte.js:18-44`. Theme keys follow the same pattern.
- **Source-scanning Go test model:** `internal/catalog/nofetch_test.go:19-62,83` walks files from the repo root with a per-file allowlist and tests its own scanner. `internal/plugin/icon_test.go` is the model for checking the listing icon.
- **Menu containers:** `web/src/components/ActionSheet.svelte` is the single container, with `phone`, `tablet` and `popover` modes (:14). The model is in `web/src/lib/actions.js:150-158`, with builders `handMenu` :161, `stackMenu` :229, `attachmentMenu` :350, `deckMenu` :378 and `pileCardActions` :421. The attack button label is built at `AttackPanel.svelte:75-77`.
- **Timeline:** `web/src/components/Timeline.svelte`.
  - Per line, ▶ and ↺ (:113-129) are shown on hover only on desktop and always at 44px on coarse pointers (:187-231).
  - Per half, rewind and play (:100-111).
  - Per turn, Setup has rewind, but "Turn N" headings have play only (:89-99). Per-turn rewind is new work.
  - `touch.spec.js:74,89,92` asserts opacity 1 on these controls.
- **Card back:** `web/src/components/Card.svelte:74-92` (markup) and `:167-186`. The back is a pure-CSS Poké Ball gradient (`#e9eef3`, `#232a33`, `#c8312b`, `#2a5bd7`, border `#1d3f9e`). The site's `Playmat.astro:269-271` copies it.
- **No screenshot tests exist** (no `toHaveScreenshot` in `web/e2e` or `site/e2e`). Projects: board `phone` 390×844, `tablet` 1024×768 and `desktop` 1440×900 (`web/playwright.config.js`), plus `wide` on the wide-board branch. Site `laptop` 1366×768 and `phone` 375×812 (`site/playwright.config.js`).

## Files examined

- `web/src/app.css:1-109` — 19 dark tokens plus a light variant, `--cw`/`--fs` sizing (:6-7), body font system-ui (:47), 37 hex literals.
- `web/src/App.svelte:50-51,89,111-161,172,208-227,268-282,310-342,386-466` — layout classes, ☰ menu, dock button, panel grid, many literal colours (badge, dot, toasts, caption).
- `web/src/lib/store.svelte.js:37,45-47,70-84,111-124` — `panelAt`, the layout decision (narrow <640px, coarse, sideways), localStorage.
- `web/src/lib/playback.svelte.js:18-44` — `replaySpeed` and `autoplay` keys.
- `web/src/components/*.svelte` — hex and px font counts: Card 33, ActionSheet 15/14, Modal 15/7, Stack 14, Strip 10, App 18/10, plus others. No rgb() literals and no inline style colours.
- `web/src/components/Timeline.svelte:12-24,89-129,177-231` — turn, half and line controls.
- `web/src/components/ActionSheet.svelte:14,103-152,168,186,214,241-268,300-308` — the three containers, sub-panels, danger and primary items.
- `web/src/components/AttackPanel.svelte:75-77` — the "Attack X for N" label.
- `web/src/components/Card.svelte:22,74-92,167-227` — back art, unknown card, text card, energy token colours.
- `web/index.html:7` — inline data-URI favicon (gold card).
- `web/playwright.config.js` and `web/e2e/*.spec.js` — 12 specs, the fixtures in `fixtures.js:5-192`, and the test server `internal/webui/webtest/cmd/boardtest` with fixed game `internal/webui/webtest/webtest.go:65,96`.
- `web/vite.config.js` — outDir `../internal/webui/dist/app`, `fs.allow` for `../internal/notice`.
- `site/src/styles/tokens.css:1-39`, `site.css:1-135` — the hand copy, "System fonts only" (:1), CopyBlock CSS (:86-106).
- `site/src/layouts/Base.astro:8-10,25-32,42,47-51,60-81` — the nav array, icon and logo, footer structure.
- `site/src/layouts/Docs.astro:16-71` — `nav.docs-nav aria-label="Guide"`, `article.prose`.
- `site/src/lib/docs-pages.ts:11-59` — sidebar order; `site/src/lib/workflows.ts:14` and `internal/guides/guides.go:17` — guide order.
- `site/src/components/Playmat.astro:1-402` — the self-drawn home scene with 17 hex literals.
- `site/src/pages/privacy.astro:40-45,71-74,109,142,150,173-200` — the outside-host sentence, "four display settings", data markers, Outside services.
- `site/e2e/site.spec.js:24-31,79-109,150-207,209-245`, `privacy.spec.js:29,32-55,111-154`, `analytics.spec.js:72-131`, `animation.spec.js:93-120`, `home.spec.js:14,35`, `workflows.spec.js:30-59`, `docs-index.spec.js:9-72`, `limits.spec.js:23-52`, `whats-new.spec.js:99-135`, `connect.spec.js:46,113-118` — what the restyle must keep or change (see Open assumptions).
- `internal/site/site.go:25,79,86-91` — embed, and the `data-retention="visits"` marker gate.
- `internal/plugin/icon_test.go` — square PNG ≥512, ≤1 MiB, SVG present, trademark word list. `internal/plugin/plugin.go:61,379-396,467-468` — icon path and checks.
- `plugin/assets/icon.svg:1-6` — current fanned-cards icon; rendered by hand with `rsvg-convert` (no Makefile or CI use).
- `README.md:1` (`docs/logo.png`); `docs/icon.png` is unreferenced; `site/public/logo.png` and `icon.png`.
- `internal/catalog/nofetch_test.go` — model for a source scanner.
- `Makefile:24-58` — `web`, `site`, `site-test`, `test`. `Dockerfile:28-49` — the `web` and `site` stages.

## External references

- WCAG 2.2 contrast minimum (1.4.3) and non-text contrast (1.4.11): the 4.5:1 and 3:1 thresholds in the spec.
- Google Fonts CSS2 API (`fonts.googleapis.com/css2`, files from `fonts.gstatic.com`): the two hosts the site tests must now allow.
- Playwright `toHaveScreenshot` (visual comparisons, `maxDiffPixels`/threshold, per-project baselines): the screenshot checkpoint.
- IBM Plex (SIL Open Font License): the typeface licence.

## Prior plans / specs consulted

- `20261102110545-5c84659c-touch-friendly-board` (plan): established ActionSheet's three containers, the pointer layer and the touch tests that the Studio menus restyle.
- `20261104090653-7373a393-wide-screen-landscape-board` (status implemented, 7/7 tasks): its code is uncommitted in `../tcgreplay-wide-board`, not on main. It is the precondition for this plan.
- `20261106110254-f08e142d-project-website` and `20261112091756-c5b6d1fd-launch-docs`: website structure, the crawl, and the "nothing from elsewhere" stance the tests encode.
- Design documents: `studio/design.md` (revised for the home hero), `mobile-board/design.md`, `wide-board/design.md`.

## Open assumptions

- **Screenshot baselines are taken on the developer's machine** (Chromium only, per the `playwright-touch-tests` gotcha). With no CI, environment drift is not a concern. If baselines prove flaky across font rendering, STOP and ask before loosening thresholds broadly.
- **Google Fonts in tests.** The test servers have no fixed network. Screenshot runs must not depend on fetching Google Fonts live: route the font requests to locally kept IBM Plex files in the test fixtures. If that is not workable, STOP and ask.
- **The wide-board work lands on main unchanged** before Studio starts, and its `wide` Playwright project and `desktop.spec.js` and `wide.spec.js` become part of the existing suites. If it lands changed, re-read it before the board tasks.
- **Text-card and energy-token colours become tokens** (card colours are design values too). The design system gains `card-*` tokens for text-card faces, and the existing `energy-*` tokens replace `Card.svelte`'s `--ec` literals.
- **The logo file keeps the name `logo.png`** (rendered from the mark's SVG) so that `site.spec.js:79` and `analytics.spec.js:110` (`header img[src="/logo.png"]`) keep passing without a test change.
- **Exactly one "Get started" link stays inside `<main>`** on the home and connect pages (`home.spec.js:35`, `connect.spec.js:46`). The header button sits outside `<main>`, and the address block uses other wording.
- **New docs landmarks.** The docs sidebar keeps exactly its 12 links in the `Guide` navigation (`docs-index.spec.js:55-72`). The breadcrumb and On this page get their own landmarks, with names that don't substring-match "Site", "Guide" or "Footer".
- **The What's new timeline keeps its hooks:** `[data-entry] h2 time`, `[data-refresh]` and the link hooks (`whats-new.spec.js:99-129`). The Limits tiles keep `[data-limit]` elements whose text is the value alone (`limits.spec.js:23-31`).

## Drafting assumptions

### Chosen direction: a Go-owned token package with generated CSS (architecture)
- **Decision**: New `internal/design` package. `tokens.json` (mirroring the Studio design system) is the source; a Go generator writes a committed `tokens.css` that both apps import. Go tests check the generated file is current, contrast across themes and mats, and stray values in `web/src` and `site/src`. The migration runs in two phases in one release: today's values on tokens first, proven by new Playwright screenshot baselines, then Studio values and component restyles.
- **Rationale**: Follows the repo's existing sharing precedent (`internal/notice`), runs in `go test` (the only gate, since CI runs no tests), and keeps generated values visible to the checks.
- **Rejected**: Hand-written CSS as the source (the contrast check would have to parse theme and mat blocks), build-time Vite/Astro plugins (generated values invisible to the Go checks), stylelint (no lint tooling in the repo).

### Mats as data-mat blocks repointing mat-* properties (architecture)
- **Decision**: Interface themes use `[data-theme]` blocks; mats use `[data-mat]` blocks that set each `--mat-*` to the chosen set. Components only reference `--mat-*`.
- **Rationale**: Matches the design system's alias model, and the six combinations need no combinatorial CSS.
- **Rejected**: Six combined theme blocks (duplicated values; drifts from the design system).

### Theme applied before first paint by an inline script (architecture)
- **Decision**: `web/index.html` reads the `theme` and `mat` keys inside try/catch and sets the attributes on `<html>` before the app loads.
- **Rationale**: Avoids a flash of the default theme on reload; the "Themes switch and stick" criterion needs the choice to survive a reload.
- **Rejected**: Setting the attributes from the Svelte store only (a visible flash on load).

### Conventions selected (architecture)
- **Decision**: Apply the privacy-page and What's new conventions; drop guide text and packaging.
- **Rationale**: Studio adds an outside font host and remembered settings and is player-visible; no guides change.
- **Rejected**: None.

### Keep the logo's file name logo.png (discovery)
- **Decision**: Render the Studio mark's SVG to `site/public/logo.png` (and `icon.png`) rather than switching the header to `logo.svg`.
- **Rationale**: `site.spec.js:79` and `analytics.spec.js:110` pin `header img[src="/logo.png"]`. Keeping the name avoids a test change the spec does not force.
- **Rejected**: Serving `logo.svg` (sharper, but needs a test change no requirement forces).

### Guide group order with guides.go order inside each group (discovery)
- **Decision**: Groups render as After a game, Practice, Play. Within each group, guides keep the guides.go order. `workflows.spec.js:48-59` changes to assert that within-group order.
- **Rationale**: The design fixes the group order. The spec allows test changes forced by the guide grouping.
- **Rejected**: Putting Play first to keep the test untouched (contradicts the design).

### Card colours become tokens (discovery)
- **Decision**: Text-card faces (cream, Trainer grey, Energy border, HP red) and energy-token colours become tokens. `card-*` tokens are added; `energy-*` tokens are reused. The design system is updated to match.
- **Rationale**: The spec requires that no design value is defined outside the tokens, and the stray-value check would otherwise flag Card.svelte.
- **Rejected**: An allowlist exemption for card colours (weakens the check the spec asks for).

### Route Google Fonts to local files in browser tests (discovery)
- **Decision**: Browser tests (screenshots in particular) fulfil Google Fonts requests from IBM Plex files kept with the test fixtures, not the live network.
- **Rationale**: Deterministic screenshots, and the test servers must not depend on the network. Production still loads from Google Fonts.
- **Rejected**: Letting tests fetch live from Google (flaky and slow, and needs network access).

### Contrast check arrives with the Studio values (tasks)
- **Decision**: The stray-value check lands in milestone 1; the contrast check lands with the Studio token values in milestone 2.
- **Rationale**: Today's palette was never designed against the 4.5:1 rule, so enforcing contrast on it would force visual changes in the "no visual change" milestone.
- **Rejected**: Enforcing contrast in milestone 1 (it contradicts the screenshot checkpoint).

### Docked timeline height not remembered (tasks)
- **Decision**: The docked layout's drag handle keeps its height in memory only.
- **Rationale**: A remembered height would be a new browser setting that the privacy page and its test would have to list; the spec asks for a resize handle, not a remembered size.
- **Rejected**: Storing the height in localStorage (extra privacy surface for little value).

### Card-face token values chosen during the work (tasks)
- **Decision**: The implementer picks Studio-consistent card-face colours that pass contrast; the design system is then updated (a human task).
- **Rationale**: The design system has no card-face tokens yet, and the spec says the design system is corrected where the two differ.
- **Rejected**: Blocking on design work before planning.

## Rehydration cues

- `spektacular spec file read 20261120140830-52277437-studio-redesign`
- `spektacular design read -d '{"source":"design","path":"studio/design.md"}'` (plus mobile-board and wide-board)
- Artifact read of the design system https://claude.ai/artifact/8uqeTrkeXAc2YYTUvwtEjK (`project/tokens.json`, `project/README.md`) and the canvas https://claude.ai/artifact/5esddCvKjxHT9WiFw9u7MJ.
- `spektacular knowledge read` for `gotchas/css-container-units-card-size.md`, `gotchas/board-boxes-settle-before-comparing.md`, `gotchas/playwright-touch-tests.md`, `gotchas/astro-site-build.md`, `gotchas/astro-inline-whitespace.md`, `architecture/visitor-counting-serve-time.md`, and the conventions (always-applied).
- Re-read `site/e2e/site.spec.js`, `privacy.spec.js`, `workflows.spec.js`, `docs-index.spec.js`, `web/e2e/touch.spec.js`, `web/src/components/ActionSheet.svelte`, `Timeline.svelte`, `Card.svelte`, `web/src/lib/store.svelte.js`.
- `git -C ../tcgreplay-wide-board status` to check whether the landscape mat has landed.
