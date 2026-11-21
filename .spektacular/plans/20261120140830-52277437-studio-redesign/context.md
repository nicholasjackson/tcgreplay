---
created_date: "2026-11-20"
document_status: final
closed_date: "2026-11-20"
---

# Context: 20261120140830-52277437-studio-redesign

## Current State Analysis

- **Styling:** each app styles itself. `web/src/app.css:1-43` holds 19 dark tokens and a light variant that nothing switches (no code sets `data-theme`). `site/src/styles/tokens.css:1-39` is a hand copy. Hard-coded values are spread through `web/src/components/*.svelte` (Card 33 hex, ActionSheet 15, Modal 15, Stack 14, App 18, …) and `site/src/components/Playmat.astro` (17).
- **Fonts:** system fonts only, on both apps (`web/src/app.css:47`, `site/src/styles/site.css:1,8`). The website's tests forbid any web font or outside host (`site/e2e/site.spec.js:150-207`, `analytics.spec.js:72-98`, `animation.spec.js:118-120`). They are changed here to allow only Google Fonts.
- **Layouts:** decided in `web/src/lib/store.svelte.js:77-84`: phone below 640px, tablet under a coarse pointer, otherwise desktop. The timeline sits beside or below (`panelAt`). The landscape mat for 1600px and wider is not on main: it is uncommitted in `../tcgreplay-wide-board` and must land first.
- **Timeline:** per-line ▶ and ↺ are hover-only on desktop; per-half rewind and play exist; per-turn rewind exists only for Setup (`web/src/components/Timeline.svelte:89-129`).
- **Menus:** one container, `ActionSheet.svelte`, with phone, tablet and popover modes, rendering the model from `web/src/lib/actions.js`.
- **Card back:** a CSS Poké Ball look (`web/src/components/Card.svelte:167-175`, copied in `Playmat.astro:269-271`).
- **Tests:** board and website Playwright suites with no screenshot tests. `go test ./...` and both suites run in `make test`. CI runs no tests.
- **Icons:** `site/public/logo.png` and `icon.png`, `docs/logo.png` (README) and the listing icon `plugin/assets/icon.svg` and `.png` (checked by `internal/plugin/icon_test.go`). The board favicon is an inline gold-card SVG (`web/index.html:7`).

## Per-Task Technical Notes

### Task: Land the wide-screen landscape mat
- **What the person does:** in `/home/nicj/code/github.com/nicholasjackson/tcgreplay-wide-board` (branch `spek/20261104090653-7373a393-wide-screen-landscape-board`), review and commit the uncommitted changes:
  - modified: `web/e2e/fixtures.js`, `web/e2e/phone.spec.js`, `web/playwright.config.js`, `web/src/App.svelte`, `web/src/components/Hand.svelte`, `Side.svelte`, `Strip.svelte`, `Timeline.svelte`, `web/src/lib/pointer.svelte.js`, `web/src/lib/store.svelte.js`;
  - new: `web/e2e/desktop.spec.js`, `web/e2e/wide.spec.js`.

  Then merge to main.
- **How it is checked:** on main, `web/playwright.config.js` has a `wide` project, `web/e2e/wide.spec.js` exists, and `make test` passes. The board at 2560×1440 shows the four-row landscape mat (see the `css-container-units-card-size` gotcha).
- **Complexity:** Low · **Token estimate:** n/a (person) · **Agent strategy:** n/a.

### Task: Record screenshots of today's board and website
- `web/e2e/screenshots.spec.js` (new) — `toHaveScreenshot` of the fixed game (reset via `fixtures.js:5-21`) in every project (`phone`, `tablet`, `desktop`, `wide`), plus `desktop` with `panelAt=bottom` set through localStorage before load (key `panelAt`, `web/src/lib/store.svelte.js:70`). Before each capture, park the pointer at (1,1) and await finite animations, as `web/e2e/about.spec.js` `boxes` does (gotcha `board-boxes-settle-before-comparing`). Mask nothing; disable caret. Set `animations: 'disabled'` in the capture options.
- `web/playwright.config.js` — add `expect.toHaveScreenshot` defaults (`maxDiffPixelRatio` ≤ 0.002, `scale: 'css'`) and keep the snapshot path per project (`{testDir}/__screenshots__/{projectName}/{arg}{ext}`).
- `site/e2e/screenshots.spec.js` (new) — for every page reached by `site/e2e/crawl.js` (pass the base URL explicitly per gotcha `playwright-second-server-baseurl`), a full-page `toHaveScreenshot` in `laptop` and `phone`. Pause the home scene by emulating `prefers-reduced-motion: reduce`, which shows its still state (`site/src/components/Playmat.astro:128-190`).
- `site/playwright.config.js` — the same screenshot defaults.
- Commit the `__screenshots__` folders under `web/e2e` and `site/e2e`.
- **Complexity:** Medium · **Token estimate:** ~40k · **Agent strategy:** 2 parallel agents (board, site), then one runs both twice to prove stability.

### Task: Create the shared design-token package
- `internal/design/tokens.json` (new) — today's values from `web/src/app.css:1-43`: dark defaults and the light variant as the `light`/`dark` themes, keeping today's names (`bg`, `felt`, `panel`, `strip`, `input`, `text`, `muted`, `line`, `slot-line`, `drop`, `accent`, `accent-glow`, `hover`, `fresh`, `note`, `p1`, `p2`, `reveal`, `card-bg`). Today the dark values are the default and light is the media-query variant: generate the same media-query structure in phase one so nothing changes.
- `internal/design/design.go` (new) — loads `tokens.json` (`//go:embed`) and `Generate() []byte` writes CSS. `internal/design/gen.go` (new) holds `//go:generate go run ./cmd/gen`, and `internal/design/cmd/gen/main.go` (new) writes `tokens.css`.
- `internal/design/tokens.css` (generated, committed).
- `internal/design/design_test.go` (new) — `TestGeneratedCurrent`: `Generate()` equals the committed `tokens.css` byte for byte, and the failure message says to run `go generate ./internal/design`.
- `web/src/main.js:2` — import `../../internal/design/tokens.css` before `./app.css`. `web/src/app.css:1-43` — delete the moved `:root` and light blocks; keep `--cw`/`--fs` (:6-7), since they are layout expressions.
- `web/vite.config.js` — `server.fs.allow` gains `'../internal/design'`.
- `site/src/layouts/Base.astro:8` — import `../../../internal/design/tokens.css` in place of `../styles/tokens.css`. Delete `site/src/styles/tokens.css`.
- `Dockerfile:28-34` (web stage) and `:40-49` (site stage) — `COPY internal/design internal/design`.
- **Check:** the screenshot comparisons from the previous task pass unchanged.
- **Complexity:** Medium · **Token estimate:** ~35k · **Agent strategy:** single agent, sequential.

### Task: Move the board's components onto the tokens
- **Files** (hex and px font-size counts from discovery):
  - `web/src/app.css`: 37 hex. `.primary` #1a1a1a at :52-64, code #0005, drag ghost #000c, body font :47 → `--font-ui`.
  - `web/src/App.svelte`: 18 hex, 10 px. Badge #ffb300/#222 :310, dot #c62828/#43a047 :336-337, warn :342, caption :438, toasts :460-462, banners :372,379, `.upright` :324, font shorthand :312.
  - `web/src/components/ActionSheet.svelte`: 15/14. Danger #ff8a80/#6b2b2b/#3a1f1f :214, backdrop #000a :186.
  - `web/src/components/Card.svelte`: 33 hex. Back art :167-175, unknown :176-186, text card :187-203, energy `--ec` :220-227, shadow #0007.
  - `web/src/components/Modal.svelte` 15/7, `Stack.svelte` 14, `Strip.svelte` 10/2, `CounterBar.svelte` 3/3, `TargetBanner.svelte` 3/1, `Hand.svelte` 2, `Side.svelte` 2, `PlaybackBar.svelte` 1/2, `Zoom.svelte` 1, `AttackPanel.svelte` 0/6, `CountPanel.svelte` 0/2, `Stepper.svelte` 0/2, `Timeline.svelte` 0/4.
  - `web/index.html:7`: the favicon data-URI is exempt until the mark task.
- **Tokens:** add tokens with today's exact values to `internal/design/tokens.json` (for example `danger`, `danger-bg`, `toast-error`, `toast-warn`, `toast-info`, `live`, `offline`, `card-back-*`, `card-text-*`, `energy-*`, `shadow-card`, plus font sizes as `fs-*`), then regenerate.
- **Font sizes:** px sizes become `var(--fs-…)`. `calc(var(--cw) * k)` and `em` sizes stay; they are layout-relative.
- **Check:** board screenshots unchanged; `web/e2e` passes.
- **Complexity:** High · **Token estimate:** ~70k · **Agent strategy:** parallel analysis (2-3 agents splitting components), sequential integration into `tokens.json` by one agent, then screenshots.

### Task: Move the website onto the tokens
- `site/src/styles/site.css` — link colour #8a5d00 :15, header literals #e8efe9 :21,37,51, sizes :40-42, :66, CopyBlock :86-106, footer :120-135 → tokens.
- **Page `<style>` blocks:** `site/src/pages/index.astro:54-81` (`.button` #1d2420 :73), `docs/index.astro:26-31`, `docs/whats-new.astro:36-40`, `docs/workflows/index.astro:26-32`, `docs/limits.astro:89`, `terms.astro:118-123`, `privacy.astro:229-234`, `docs/workflows/[guide].astro:28-31`, `site/src/layouts/Docs.astro:51-71`.
- `site/src/components/Playmat.astro:191-402` — 17 hex literals (card back :269-271, text card :283-290, energy :300-306, log/coach :319-327, sweep #000a :344) → the same card and energy tokens the board uses.
- **Check:** website screenshots unchanged; `site/e2e` passes.
- **Complexity:** Medium · **Token estimate:** ~40k · **Agent strategy:** single agent.

### Task: Fail the build on design values outside the tokens
- `internal/design/scan_test.go` (new) — modelled on `internal/catalog/nofetch_test.go:19-62,83`. It walks `web/src` and `site/src` from the repo root (`filepath.Join("..","..")`).
  - **Files read:** `.css`, the `<style>` blocks of `.svelte` and `.astro` files, and `style=` attributes.
  - **Fails on:** hex colours, `rgb(`/`rgba(`/`hsl(`, named colours in colour properties, and `font-size`/`font:` px or rem values that are not `var(--…)`.
  - **Allowlist:** `transparent`, `currentColor`, `inherit`, `none`, values inside `var(--…, fallback)`, and `calc()` built on `--cw`/`--fs`.
  - **Exempt files:** `internal/design/tokens.css` and the `web/index.html` theme script.
- `TestScannerCatchesPlanted` — feeds the scanner a planted literal of each kind and checks it is reported, and that `var(--x)` is not.
- The failure lists `path:line: literal`.
- **Complexity:** Medium · **Token estimate:** ~30k · **Agent strategy:** single agent.

### Task: Switch the tokens to Studio and check contrast
- `internal/design/tokens.json`:
  - **Values:** replace them with the design system's `project/tokens.json` (https://claude.ai/artifact/8uqeTrkeXAc2YYTUvwtEjK): interface tokens (light first), `slate-*`/`felt-*`/`navy-*`, `mat-*` aliases, constants (`accent`, `seat-p1-mat`, `seat-p2-mat`, `hp`, `damage-*`, `energy-*`, `brand-*`, `turn-tag-ink`), type, spacing, radius, shadow and size.
  - **Card faces:** add `card-face`, `card-face-trainer`, `card-face-ink`, `card-border-pokemon`, `card-border-trainer`, `card-border-energy` and `card-hp`. Choose Studio-consistent values that meet contrast.
  - **Mats:** add a `mats` list.
  - **Legacy names:** keep them as aliases pointing at the closest Studio token (for example `felt`→`mat-surface`, `accent`→`accent`, `p1`→`seat-p1-mat`, `text`→`ink`, `muted`→`muted`).
  - **Pairs:** add `pairs` covering every text-on-ground use in the design system README and components.
- `internal/design/design.go` — the generator emits `:root` (light), `[data-theme="dark"]`, and `[data-mat="felt"|"navy"]` blocks repointing each `--mat-*` (slate is the default in `:root`). It drops the phase-one media query, because the board now sets `data-theme` and the site stays light.
- `internal/design/contrast_test.go` (new) — WCAG relative luminance and contrast ratio. For each pair × theme × (mat, when the pair names `mat-*`), assert ≥ `min`; a planted failing pair is reported.
- Regenerate `tokens.css`. Screenshots now differ; the restyle tasks re-record them. Phase-one baselines are kept until each restyle task replaces its own.
- **Complexity:** Medium · **Token estimate:** ~40k · **Agent strategy:** single agent.

### Task: Add the card-face tokens to the design system
- **What the person does:** in the TCG Replay Studio design system (https://claude.ai/artifact/8uqeTrkeXAc2YYTUvwtEjK), add the `card-*` tokens with the values in `internal/design/tokens.json`, and mention them in the README's colour rules (or ask Claude to do it in a session with the Artifact tool).
- **How it is checked:** the design system's tokens list names and values identical to the repo's `card-*` tokens.
- **Complexity:** Low · **Token estimate:** n/a · **Agent strategy:** n/a.

### Task: Create the new mark and redraw every icon
- `internal/design/mark.svg` (new) — the mark from the design system's Logos asset (navy #13285C square, cyan #5CC8F2 and white cards, replay arrow).
- **Rendered with `rsvg-convert`:**
  - `site/public/logo.png` (512×512, keeps its name; pinned by `site/e2e/site.spec.js:79` and `analytics.spec.js:110`);
  - `site/public/icon.png` (256×256);
  - `docs/logo.png` (512×512, used in `README.md:1`);
  - `docs/icon.png` (256×256).
- `plugin/assets/icon.svg` — redraw from the mark at 512 viewBox and keep the render comment (:5-6). Render `plugin/assets/icon.png` (512×512, ≤1 MiB). It must still pass `internal/plugin/icon_test.go` (square ≥512, ≤1 MiB, no trademark words) and `internal/plugin/plugin.go:379-396`.
- `web/index.html:7` — the favicon becomes the mark as an SVG data URI.
- **Complexity:** Low · **Token estimate:** ~15k · **Agent strategy:** single agent.

### Task: Load IBM Plex and add theme settings to the board
- `web/index.html`:
  - Google Fonts `<link rel="preconnect">` for both hosts, plus the css2 link for IBM Plex Sans 400/500/600/700 and Plex Mono 500/600 with `display=swap`.
  - An inline script before the app that reads localStorage `theme` and `mat` in try/catch and sets `document.documentElement.dataset.theme` and `.dataset.mat`.
- `internal/design/tokens.json` — `--font-sans`/`--font-mono` stacks with system fallbacks. `web/src/app.css:47` — body uses `var(--font-sans)`.
- `web/src/lib/store.svelte.js:37-47,70-71,111-124` — add `theme` and `mat` state, read at start (same try/catch pattern), plus `setTheme`/`setMat`, which write storage and the root attributes.
- `web/src/App.svelte:111-161` — ☰ dropdown gains Interface (Light/Dark) and Mat (Slate/Felt/Navy) as radio groups (44px under coarse pointer, `app.css:103-109`).
- **Font fixture:**
  - `testdata/fonts/` (new): IBM Plex woff2 files (OFL) and a stub css2 response.
  - `web/e2e/fixtures.js:5-21`: `page.route('https://fonts.googleapis.com/**')` and `fonts.gstatic.com/**` fulfil from those files.
- `web/e2e/themes.spec.js` (new):
  - default light+slate;
  - each combination via ☰ sets the attributes, and `getComputedStyle` of the mat surface changes;
  - board boxes unchanged across themes (settle per gotcha);
  - reload keeps the choice;
  - storage blocked (`addInitScript` throwing) still renders defaults.
- **Complexity:** Medium · **Token estimate:** ~45k · **Agent strategy:** 2 parallel agents (fonts and fixture; store, menu and tests).

### Task: Restyle the board's header and layouts
- `web/src/App.svelte`:
  - header markup and styles (:89-161, :326-368): mark, game breadcrumb, branch select, Play/Review segmented switch, Reveal all, warnings chip, connection dot plus "Live", ☰;
  - dropdown;
  - toasts (:455-462) and caption (:438);
  - layout CSS (:265-300, :386-466) using `--mat-surface` and panels; the `.wide` landscape rules from the wide-board landing.
- Add a timeline show/hide button in the header for `app.layout === 'tablet'` (it uses `app.panelOpen`, as the ☰ item does today).
- `web/e2e/screenshots.spec.js` — re-record header and layout baselines.
- **Complexity:** Medium · **Token estimate:** ~45k · **Agent strategy:** single agent.

### Task: Restyle the mat's player blocks, Bench, Active panel, turn strip and hand
- **Player blocks:** `web/src/components/Side.svelte` (`app.layout` at :60) — name and seat dot, prize pips (six slots filled for prizes left, with the count), pile chips (Hand, Deck, Discard, Lost as buttons opening today's pile views), top-of-discard thumbnail.
- **Bench and Active:** `web/src/components/Stack.svelte` —
  - Bench readout under each card: HP left in mono, `--damage-ink` when damaged, Energy dots in `--energy-*`;
  - Active panel: name, HP left/max, HP bar, damage tag, Energy count, outlined in `--seat-p1-mat`/`--seat-p2-mat`.
- **Turn strip:** `web/src/components/Strip.svelte` (:26, 49, 56, 64) — Stadium, turn tag, step, Flip coin, End action, End turn on `--accent`.
- **Hand:** `web/src/components/Hand.svelte` (:57, 64) — kind label on each card.
- **Phone:** `web/src/components/CounterBar.svelte` — counter chips; the Prizes chip is tinted with its player's colour.
- **Readout sources:** HP and damage come from the existing card info and state the components already read (no new state).
- `web/e2e` — update selectors only if a test targets moved markup; keep `fixtures.js` `pile`/`pileLabel` hooks working.
- **Complexity:** High · **Token estimate:** ~60k · **Agent strategy:** parallel agents per component (Side/CounterBar, Stack, Strip/Hand), then sequential integration and a screenshot pass.

### Task: Restyle cards and give face-down cards the Studio back
- `web/src/components/Card.svelte:74-92,167-227`:
  - the `.back-art` becomes a navy back (`--brand-navy` with an inset `--brand-cyan` frame) carrying the mark from `internal/design/mark.svg`, inlined as an SVG or a CSS mask;
  - `.back-art.unknown` keeps "?" over the back;
  - text card and Trainer and Energy borders use the `--card-*` tokens; `.token` uses `--energy-*`.
- `web/src/components/Zoom.svelte` — tokens.
- `site/src/components/Playmat.astro:269-271` — its copied back follows in the website pages task.
- `web/e2e/images.spec.js` — keep fallback assertions; add a check that a hidden card shows the Studio back (for example a `data-back="studio"` hook).
- **Complexity:** Low · **Token estimate:** ~20k · **Agent strategy:** single agent.

### Task: Restyle the timeline and add turn rewind
- `web/src/components/Timeline.svelte`:
  - **Headings (:89-111):** the "Turn N" h3 gains a Rewind button (jump to `startOf(turn)`, branch `turn-N`) beside ▶. The per-half h4 keeps Rewind/▶, restyled with labels.
  - **Lines (:113-129):** ▶ and ↺ always visible (remove the hover opacity at ~:177-197; keep 44px under coarse at ~:219-231). Current step `--current`, ahead dimmed, notes in note boxes.
  - Studio type: mono ids, `--seat-p1`/`--seat-p2` dots.
- `web/src/components/PlaybackBar.svelte` — Studio controls, plus a scrubber coloured by seat (segments from turn halves).
- **Docked layout** (`web/src/App.svelte` `.body.panel.bottom` :398, plus the container-unit gotcha: keep explicit rows):
  - a drag handle resizing the grid row, with the height stored in memory only (not remembered, so the privacy settings list is unchanged);
  - one control row (playback, scrubber, step, speed, Go live, dock);
  - a turn-list rail with per-half rewind buttons and the prize score after each turn, computed from the log's prize events.
- `web/e2e/touch.spec.js:74,89,92` — opacity assertions still hold (always 1).
- `web/e2e/timeline.spec.js` (new):
  - play from a line, rewind to just after it;
  - turn and half Rewind/Play act on their starts;
  - controls visible without hover on desktop;
  - a docked resize changes the board/timeline split;
  - a turn-list click rewinds.
- **Complexity:** High · **Token estimate:** ~60k · **Agent strategy:** 2 parallel agents (timeline and playback; docked rail and handle), sequential integration.

### Task: Restyle the menus and add the Knock Out hint and Card text
- `web/src/components/ActionSheet.svelte` — Studio styles for the `popover` (264px, header plus mono summary, uppercase section labels, 34px rows, cost dots, target icon), `tablet` (centred panel, card left with Card/Text switch) and `phone` (full-screen sheet) modes (:14, 103-152, 168, 186, 214, 241-268, 300-308). Knock Out is last, in `--danger`.
- `web/src/components/AttackPanel.svelte:75-77` — add the hint line "{damage} is enough to Knock Out {name}" when `damage >= target HP left` (from the target stack's HP minus damage). Keep the button label "Attack {name} for {damage}". Add ×2 Weakness and −30 Resistance toggles beside the stepper if they are not already present (×2 is), and keep Change ›.
- `web/src/lib/actions.js:161` (`handMenu`) — add `{ label: 'Card text', run }` opening the existing text view (Zoom or `cardInfo` text) in the Other section.
- `web/src/components/CountPanel.svelte`, `Stepper.svelte`, `TargetBanner.svelte`, `Modal.svelte:136-247`, `Zoom.svelte` — Studio styles.
- `web/e2e/sheet.spec.js` — the same content in three containers (keep its hooks).
- `web/e2e/panels.spec.js` — Knock Out hint at and below the threshold.
- New assertion that a hand card's menu has Card text.
- **Complexity:** High · **Token estimate:** ~55k · **Agent strategy:** parallel agents (ActionSheet; AttackPanel and actions; modal family), sequential integration.

### Task: Record the board's Studio screenshots
- `web/e2e/screenshots.spec.js` — re-record all baselines (phone, tablet, desktop, wide, docked) plus popover, panel and sheet captures (open via `fixtures.js` `tapCard` and right-click).
- `internal/design/tokens.json` — remove legacy aliases with no remaining `var(--legacy)` uses (grep `web/src`, `site/src`), and regenerate.
- Run `make test`.
- **Complexity:** Low · **Token estimate:** ~20k · **Agent strategy:** single agent.

### Task: Restyle the website's header, footer and docs layout
- `site/src/layouts/Base.astro`:
  - **Nav (:25-32):** Guides, Example, Limits, What's new.
  - **Header (:47-51):** `a.brand` keeps `img[src="/logo.png"]`, plus the wordmark "TCG Replay". Get started is an `a.button` in the header, outside `<main>` (`home.spec.js:35`, `connect.spec.js:46`).
  - **Footer (:60-78):** dark, with Docs and Project columns. Keep `p.fan-notice`, `p.credit` and `nav[aria-label="Footer"]` with exact links Privacy, Terms, Get help and Source on GitHub (`site.spec.js:87-109`). Extra Docs links go in a separate `nav aria-label="Docs links"`; check its name does not substring-match "Footer", "Site" or "Guide".
  - Google Fonts links in `<head>` (:38-43).
- `site/src/layouts/Docs.astro:16-71` — add `nav aria-label="Breadcrumb"` and `nav aria-label="On this page"` (headings from the article's h2s), outside `nav[aria-label="Guide"]`, which keeps exactly 12 links (`docs-index.spec.js:55-72`). Keep `article.prose`.
- `site/src/styles/site.css` — Studio styles for header, footer, prose, `.copy-block`/`.prompt` (chat bubble), steps, `.note` callouts and `pre` code blocks (keep `data-copy` structure; `site.spec.js:209-245`).
- **Tests:**
  - `site/e2e/site.spec.js:24-31` `navLinks` → Guides, Example, Limits, What's new, plus the header Get started button.
  - `:150-169` and `:171-207`: allow exactly `fonts.googleapis.com` and `fonts.gstatic.com`; `@font-face` and loaded fonts are permitted only from them.
  - `site/e2e/analytics.spec.js:72-98`: an allowed-hosts list adds both font hosts.
  - `site/e2e/animation.spec.js:118-120`: home `link[href]` may be Google Fonts.
  - `site/e2e/privacy.spec.js:29`: the Privacy link is found in the Footer nav.
  - Route both font hosts to `testdata/fonts` in a shared site fixture, so tests stay offline.
- **Complexity:** High · **Token estimate:** ~55k · **Agent strategy:** 2 parallel agents (layouts and styles; test updates), sequential integration.

### Task: Restyle the website's pages and group the guides
- **Home:** `site/src/pages/index.astro:1-81` — hero (badge "Works with Claude and ChatGPT", `h1`, `[data-pitch]` unchanged text, one "Get started" in `main`, See an example game), How it works (three steps), What it does on a slate band, four guide cards (prompts from `site/src/content/docs/workflows/*.md` frontmatter), and the address block with CopyBlock (button label "Copy The playmat's address", `site.spec.js:237-245`; it must not be a second "Get started" link).
- **Home scene:** `site/src/components/Playmat.astro` — Studio colours via tokens, keeping `.text-card` (≥5), `.back-art`, `.token`, a radial-gradient `.stage`, `.log li`, `.coach` "Turn 3", no images (`home.spec.js`, `animation.spec.js`). The card back matches the board's Studio back (CSS-drawn, no image).
- **Guide grouping:**
  - `site/src/content.config.ts:36-44` gains `group: z.enum(['after-a-game','practice','play'])`, set in each `site/src/content/docs/workflows/*.md`;
  - `site/src/lib/workflows.ts` groups `pairWorkflows()` output in group order, keeping the guides.go order (`internal/guides/guides.go:17`) within groups;
  - `site/src/pages/docs/workflows/index.astro:15-21` renders one `[data-workflows]` list per group under an h2;
  - `site/e2e/workflows.spec.js:48-59` asserts guides.go order within each group and the group order After a game, Practice, Play.
- **What's new:** `site/src/pages/docs/whats-new.astro` — timeline styles, keeping `[data-entry] h2 time`, `[data-refresh]` and its links (`whats-new.spec.js:99-129`).
- **Example:** `site/src/pages/docs/example.astro` — steps, prompt, and two copy panels (decklist light, log dark). Fix the known "see<a" whitespace bug (gotcha `astro-inline-whitespace`).
- **Limits:** `site/src/pages/docs/limits.astro` — stat tiles whose value elements are the existing `[data-limit]` spans containing only the value (`limits.spec.js:23-31`).
- **Terms and 404:** `site/src/pages/terms.astro` — text column with an On this page card. `site/src/pages/404.astro` — the stack-of-cards picture in CSS, Go to the home page and All docs.
- **Complexity:** High · **Token estimate:** ~60k · **Agent strategy:** parallel agents (home and scene; workflows grouping and test; what's-new, example, limits, terms, 404), sequential integration.

### Task: Update the privacy page and announce the redesign
- `site/src/pages/privacy.astro`:
  - **:40-45:** reword so Google Fonts and Cloudflare's script are the only outside loads. Keep the paragraph a direct child `p` of `main article` containing `__cf_bm`.
  - **:71-74:** "six display settings … (replay speed, autoplay, where the panel sits, prize reminders, the interface theme and the mat)".
  - **`section#outside-services` :173-200:** add Google Fonts (what it receives: the page's request and IP address, per Google Fonts' own privacy FAQ). Update `updated` (:26).
  - Keep the `data-retention`, `data-idle` and `data-assistant-id` markers (Go reads `data-retention="visits"`, `internal/site/site.go:79,86-91`).
- `site/e2e/privacy.spec.js`:
  - :111-144: "six display settings" plus the two new names;
  - Outside services contains Google Fonts;
  - :146-154: the new sentence.
- `site/src/data/whats-new.yaml` — a new top entry with the release date, title "A new look", `refresh: false`, and changes covering the Studio look, themes in ☰, the new logo and the website refresh (per the `whats-new-entries` convention).
- **Complexity:** Low · **Token estimate:** ~20k · **Agent strategy:** single agent.

### Task: Record the website's Studio screenshots
- `site/e2e/screenshots.spec.js` — re-record every page at `laptop` and `phone`.
- Run `make test` (Go, board suite, website suite).
- **Complexity:** Low · **Token estimate:** ~15k · **Agent strategy:** single agent.

## Testing Strategy

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

Per task, the context notes above name the test files each task adds or changes. Screenshot baselines live in `web/e2e/__screenshots__/` and `site/e2e/__screenshots__/`; Go checks live in `internal/design`.

## Project References

- **Spec:** `20261120140830-52277437-studio-redesign`.
- **Designs** (`design` source): `studio/design.md`, `mobile-board/design.md`, `wide-board/design.md`.
- **Design system:** https://claude.ai/artifact/8uqeTrkeXAc2YYTUvwtEjK. **Canvas:** https://claude.ai/artifact/5esddCvKjxHT9WiFw9u7MJ.
- **Knowledge:**
  - conventions `privacy-page-tracks-outside-services`, `whats-new-entries`;
  - gotchas `css-container-units-card-size`, `board-boxes-settle-before-comparing`, `playwright-touch-tests`, `playwright-second-server-baseurl`, `astro-site-build`, `astro-inline-whitespace`;
  - architecture `visitor-counting-serve-time`.
- **Repo:** `tcgreplay` at `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (every task). The wide-board worktree is at `/home/nicj/code/github.com/nicholasjackson/tcgreplay-wide-board`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

## Migration Notes

- **Precondition:** the wide-board work lands on main before task 2.
- **Order:** screenshots of today's look → tokens with today's values → components on tokens (screenshots must match) → stray-value check → Studio values and contrast → restyles, each re-recording its baselines → final full run.
- **Branch:** the work stays on one feature branch and merges to main once, because every push to main deploys.
- **Renames:** legacy token names survive as aliases until the board baselines task removes the unused ones.
- **Deleted:** `site/src/styles/tokens.css`. The Dockerfile's `web` and `site` stages gain `COPY internal/design`.

## Performance Considerations

- **Fonts:** Google Fonts adds two requests per first page load. `display=swap` and `preconnect` keep text visible immediately, with system-font fallback.
- **Home page:** it must stay under 1.5 MB (`animation.spec.js:93-105`). The Plex font files count toward the browser's transfer but not the page HTML; check the test's measure after adding fonts.
- **Rendering:** the theme script runs before the app with no rendering cost; theme switching is attribute-only (no re-render).
