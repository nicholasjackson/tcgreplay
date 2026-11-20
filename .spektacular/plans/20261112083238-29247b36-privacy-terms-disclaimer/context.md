---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Context: 20261112083238-29247b36-privacy-terms-disclaimer

## Current State Analysis

All work is in the one registered repo, `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`).

- **Website** (`site/`, Astro 7) builds into `internal/site/dist/app`. The Go site handler serves every built page at its own path with no sign-in and no cookie (`internal/site/site.go:1-9,112-126`); hosted mode mounts it as the `/` catch-all (`cmd/tcgreplay/hosted.go:208`). Every page uses `site/src/layouts/Base.astro`, whose footer already carries the fan notice, the TCGdex and malie.io credit and a "Source on GitHub" link (`:49-59`); the header nav lists Privacy but not Terms (`:21-28`). There is no terms page.
- **Facts**: `site/src/lib/facts.ts` reads the hosted limits from `.do/app.yaml` (300 a minute, 10 players, 60m idle today) and fails the build if one is missing. The public address is hard-coded to `https://tcgreplay.demo.gs` at HEAD (`:15`); the domain-move plan moves it to `TCGREPLAY_PUBLIC_URL` in `.do/app.yaml` (already `https://tcgreplay.app` in the working tree).
- **Privacy page** (`site/src/pages/privacy.astro`, last updated 8 November 2026). Audited against the code (see research.md, audit):
  - true: sign-in scopes and kept fields; the two cookies; the four browser settings; games in memory only; the retention constants.
  - misleading: "sign-ins kept encrypted, in memory only" (the server records are plain in-memory maps; the copies handed out are sealed and survive restarts).
  - partly true: "disconnect your assistant ... the playmat forgets you" (no revocation endpoint, so the record lasts up to 30 days after last use); outside services (the server itself fetches card images from TCGdex and malie.io into DigitalOcean Spaces, which the page never names); logs (more email-bearing lines than listed today; none after observability).
  - true today, false once visitor counting lands: "the website loads nothing from other sites" and "no analytics of any kind".
- **Board** (`web/`, Svelte 5 + Vite) builds into `internal/webui/dist/app`. Its ☰ menu (`web/src/App.svelte:111-158`) holds Deck list, the timeline, Prize reminders, Autoplay and, hosted only, the account and Sign out. `web/src/components/Modal.svelte` renders views by `app.modal.kind` (pile, prizes, search, look, decklist, prompt) in a shell with a backdrop, `role="dialog"`, a touch ✕ and Escape (`:127-132`). The board has no notice, credit or help link today. Layout is chosen in JS (`web/src/lib/store.svelte.js:77-84`: phone at `max-width: 639px`, tablet on a coarse pointer, otherwise desktop).
- **Container build**: the `Dockerfile` `web` stage copies only `web/` (`:21-25`); the `site` stage also copies `internal/guides`, `.do/app.yaml` and `testdata/decks` (`:30-33`).
- **Tests**:
  - The website suite (`site/e2e`, laptop 1366 and phone 375) crawls every linked page and checks the footer notice and credit on each, no cookies and no third-party fetches.
  - The board suite (`web/e2e`; phone 390, tablet 1024, desktop 1440) drives the board test server, which shows no signed-in account.
  - `cmd/tcgreplay/hosted_routes_test.go:34` fetches `/`, `/docs/connect` and `/privacy` signed out.

## Per-Task Technical Notes

### Task: Share one notice wording and show it in the website footer

**File changes:**
- `internal/notice/notice.json` (new) — the shape in plan.md § Data Structures. `notice` is the footer's current two sentences from `site/src/layouts/Base.astro:52-55`, verbatim. `credit.lead` is "Card data and images from". `credit.sources` lists TCGdex `https://tcgdex.dev` and malie.io `https://malie.io`. `help` is `{ "label": "GitHub issues", "url": "https://github.com/nicholasjackson/tcgreplay/issues" }`. UTF-8, no trailing comments (plain JSON).
- `site/src/lib/notice.ts` (new) — modelled on `site/src/lib/facts.ts:1-60`:
  - `noticePath = join(process.cwd(), '..', 'internal', 'notice', 'notice.json')`. Never use `import.meta.url` (`gotchas/astro-site-build.md`).
  - `loadNotice(jsonText, source)` throws an `Error` prefixed `site notice:` when:
    - the JSON is invalid;
    - `notice`, `credit.lead` or `help.label` is empty;
    - `credit.sources` is empty;
    - any `url` is not `https://`;
    - `notice` lacks any of `Nintendo`, `The Pokémon Company`, `Creatures`, `Game Freak`.
  - Export `notice` = `loadNotice(readFileSync(noticePath))`, wrapping a read failure in the same error style.
- `site/src/layouts/Base.astro:1-63`:
  - Import `notice` from `../lib/notice` (so every page's build checks it).
  - Footer `.fan-notice` renders `{notice.notice}`. `.credit` renders `{notice.credit.lead}` followed by each source as a link, joined with " and ", ending with ".".
  - Replace the lone "Source on GitHub" paragraph with a `<nav class="footer-links" aria-label="Footer">` list:
    - Privacy (`/privacy`);
    - Get help (`notice.help.url`, accessible name "Get help on GitHub issues" or visible text "Get help");
    - Source on GitHub (`site.repoURL`).
  - The Terms link is added by the next task, so the crawl never meets a broken link.
  - Update the header comment.
- `site/src/styles/site.css:120-127` — style `.footer-links` as an inline, wrapping list with gaps; it must not cause sideways scroll at 375 px.
- `Dockerfile:30-33` (`site` stage) — add `COPY internal/notice /src/internal/notice` beside the existing `COPY internal/guides`.
- `Dockerfile:21-25` (`web` stage) — add `COPY internal/notice /src/internal/notice`, so the board build (the About panel task) can import it.
  - Do this here, so one task owns both Dockerfile edits.
  - Keep `COPY internal/guides` and `.dockerignore` untouched (convention).
- `Dockerfile:1-17` header comment — mention that both UI builds read `internal/notice/notice.json`.
- `site/e2e/notice.spec.js` (new, no browser page, like `site/e2e/facts.spec.js`):
  - `loadNotice` on the real file returns the four companies, both sources and the issues URL.
  - Fixtures with a dropped field, a notice missing "Game Freak" and an `http://` URL each throw an error naming the problem.
- `site/e2e/site.spec.js:103-126` — in the every-page test:
  - Assert `footer .fan-notice` text equals the shared file's `notice`, reading the file independently with `readFileSync(join(process.cwd(), '..', 'internal/notice/notice.json'))`.
  - Assert the `.credit` links TCGdex and malie.io.
  - Assert the footer navigation has Privacy → `/privacy` and a help link → the issues URL.
  - Keep the existing four-company checks.

**Complexity:** Low
**Token estimate:** ~15k tokens
**Agent strategy:** Single agent, sequential execution.

### Task: Publish the terms of service page

**File changes:**
- `site/src/pages/terms.astro` (new) — modelled on `site/src/pages/privacy.astro`:
  - Uses the `Base` layout, with `title="Terms of service"` and a description.
  - Top comment naming its sources: `.do/app.yaml` through `lib/facts`, and `internal/notice/notice.json` through `lib/notice`.
  - `const updated = '<implementation day, e.g. 20 November 2026>'`, rendered as `<p class="updated" data-updated>Last updated {updated}.</p>`.
  - One `<section aria-labelledby>` per heading, in this order:
    1. **Who runs tcgreplay**: "the tcgreplay project", an open-source fan project. Address `site.publicURL`. These terms cover the hosted playmat and this website.
    2. **Free, and provided as is**: free, provided as is with no warranty; no guarantee it stays available or works without interruption; the project is not liable for losses from using it, as far as the law allows.
    3. **Your games**: held in memory; can be lost at any time (idle, last sign-in ending, every update or restart). Keep the record (link `/docs/limits#keep-the-record`).
    4. **Hosted limits**:
       - `<strong data-limit="rate">{limits.requestsPerMinute}</strong>` requests a minute per account;
       - `<strong data-limit="users">{limits.maxUsers}</strong>` signed-in players at once;
       - games idle for `<strong data-limit="idle">{limits.idleTimeout}</strong>` are dropped.
       - Link `/docs/limits`. The limits may change.
    5. **Acceptable use**: no overloading or attacking the service (including attempts to break in or disrupt it), no automated scraping of the site or card data, no getting round the limits (for example several accounts to dodge them), and no use for anything unlawful. The project may block accounts that break these terms.
    6. **Age**: you must be at least 13 to use the hosted playmat.
    7. **Trademarks and card images**: `{notice.notice}`, then the credit from `notice.credit` with links. Card data and images belong to their owners and are shown for players' own study.
    8. **Privacy**: link `/privacy`.
    9. **Changes**: these terms and the service may change or end at any time. The date above shows the last change, and continuing to use it means accepting the terms.
    10. **Help**: open an issue on `notice.help.url`. There is no support email.
  - Never write `demo.gs` or `www.`. Use `site.publicURL` (domain-move whole-site scan).
  - Styles like privacy (`max-width: 46rem`).
- `site/src/layouts/Base.astro` (footer links added by the previous task) — add Terms (`/terms`) after Privacy.
- `site/e2e/terms.spec.js` (new):
  - `/terms` returns 200; h1 "Terms of service".
  - Each named region exists.
  - Content assertions per the acceptance criteria: "tcgreplay project", "as is", "lost", "13", "scraping", "unlawful", "change".
  - `[data-limit=*]` equal the app.yaml values, read with an independent reader copied from `site/e2e/limits.spec.js:11-20`.
  - The trademarks region contains the shared `notice` text exactly and links TCGdex and malie.io.
  - Links to `/privacy` and the issues URL.
  - `[data-updated]` matches `/Last updated \d{1,2} [A-Z][a-z]+ \d{4}\./`.
- `site/e2e/site.spec.js` — the every-page footer assertion adds Terms → `/terms`.
- `cmd/tcgreplay/hosted_routes_test.go:34` — add `"/terms"` to the signed-out website paths.
- `internal/site/site_test.go` — no change needed (it uses fixtures). Optionally add a `dist/app/terms/index.html` fixture row beside privacy at :17 and :83 if that keeps the table complete.

**Complexity:** Low
**Token estimate:** ~20k tokens
**Agent strategy:** Single agent, sequential execution.

### Task: Bring the privacy page up to date

**Before starting:** read `docs/observability.md` (written by the observability plan).
- If the file does not exist, or its **Retention** section has no figures yet, STOP and ask the user. The diagnostics retention cannot be stated truthfully without them.
- If the runbook's "What is never recorded" list contradicts this page's plan, the runbook wins. Report the difference.
- Also confirm on developers.cloudflare.com/web-analytics (limits / FAQ / data pages) what Web Analytics records and how long it keeps it. Secondary sources say about six months, cookieless, no localStorage. If Cloudflare documents no period, say "kept by Cloudflare under its own policy" and link its privacy policy.

**File changes:**
- `site/src/pages/privacy.astro:1-150`:
  - **Header comment**: add the runbook and Cloudflare's docs as sources, and the URL and date the Cloudflare figures were checked.
  - **`updated`** (line 9): the implementation day, in the same `D Month YYYY` format, rendered with `data-updated`.
  - **Intro** (lines 18-23): the playmat itself sets no cookies on the website. Cloudflare, which sits in front of the site, may set a short-lived bot-protection cookie (`__cf_bm`) that is not used for visitor counting and is not tied to an account (knowledge: `gotchas/live-site-cf-bm-cookie.md`). The only thing the website loads from another site is Cloudflare's visitor-counting script (below). Keep the desktop sentence. (Decided at epic planning.)
  - **When you sign in** (lines 25-52):
    - Keep the scopes, the three kept items, and the name/photo/password and token sentences (all true, `internal/hostauth/hostauth.go:134`, `google.go:120-142`).
    - Replace "Your sign-ins are kept encrypted, in the playmat's memory only" with: the playmat keeps your sign-ins in its memory. The copies it gives your browser (the cookie) and your assistant are encrypted, so you stay signed in when the playmat is updated (`internal/hostauth/seal.go`, `store.go:188-211`).
    - Keep the two cookies and the four browser settings (true).
  - **While you play** (lines 54-65): keep, including "gone when the playmat restarts" (true for games).
  - **How long it is kept** (lines 67-90):
    - Game bullet unchanged.
    - Account details: dropped when your last sign-in ends. Signing out ends the board's. The assistant's ends when its connection runs out, up to 30 days after it was last used. Disconnecting in the assistant does not tell the playmat, so the record runs out on its own within those 30 days (`gotchas/disconnect-does-not-reach-playmat.md`).
    - Sign-in durations unchanged.
    - Replace the **Logs** bullet with: the playmat's own records hold no email address; see Diagnostics. The hosting providers, DigitalOcean and Cloudflare, may keep their own request logs.
  - **New section `#visitor-counting`, "Visitor counting"**, after "While you play":
    - The website and the board count visits with Cloudflare Web Analytics.
    - It records the page, the referrer (the site you came from), your country and your device (browser, operating system, device type).
    - It sets no cookies and stores nothing in your browser.
    - It is never linked to your Google account, and never sent your email, games or cards.
    - Blocking it changes nothing.
    - Cloudflare keeps the counts for `<span data-retention="visits">…</span>`.
  - **New section `#diagnostics`, "Diagnostics"**:
    - (a) DigitalOcean's request and performance metrics: request counts and response times, status codes, CPU, memory, restarts. These are totals, not per person.
    - (b) The playmat's own usage and error records, from the runbook's event list in plain words: sign-ins and sign-outs, each request your assistant makes (which tool and which assistant), imports and new games, your first activity each day, limits reached, games expiring, and errors with the user's text removed.
    - Each record names you only by a player ID, a one-way code made from your email with a secret key. It cannot be turned back into your email. The maintainer can find your ID from your email when you ask for help.
    - They never contain your email, name, Google tokens, battle logs, decklists, card names or IP address.
    - Kept in DigitalOcean Insights: records for `<span data-retention="logs">…</span>`, metrics for `<span data-retention="metrics">…</span>`, with the runbook's figures.
  - **Outside services** (lines 92-115):
    - Card data sentence: the playmat reads only the project's published copy.
    - Card images: the playmat fetches them from TCGdex and malie.io and keeps copies in its image store on DigitalOcean Spaces. Your browser loads them from there, or from TCGdex or malie.io if a copy is missing. Nothing about you is sent to TCGdex or malie.io.
    - DigitalOcean bullet: hosting, Spaces (card data and images) and Insights (diagnostics).
    - Add Cloudflare Web Analytics to the Cloudflare bullet.
  - **What it never collects** (lines 117-125): replace "Analytics, tracking or advertising data of any kind" with "Advertising or cross-site tracking data, or anything that identifies you in visitor counts". Keep the others.
  - **Removing your data** (lines 127-133):
    - Sign out on the board ends the board's sign-in.
    - Disconnecting the assistant stops it using the playmat; the playmat's record of that connection runs out within 30 days.
    - Nothing is stored beyond that, and diagnostic records name you only by the player ID and run out after the retention above.
    - Keep "Questions" → issues.
- `site/e2e/privacy.spec.js:23-55`:
  - Reverse the `'Analytics'` check in the never section.
  - New regions "Visitor counting" (Cloudflare, page, referrer, country, device, cookies, account) and "Diagnostics" (DigitalOcean, player ID, never-list words, retention).
  - A test reading `docs/observability.md`: slice from the `Retention` heading to the next heading of the same level, and assert each `[data-retention="logs"|"metrics"]` text appears in that slice.
  - Absent strings: "loads nothing from other sites", "writes your email address".
  - Present strings: "encrypted", "30 days" in the removal/retention wording, "DigitalOcean Spaces", the four storage settings.
  - A link to the issues URL; `[data-updated]` format.
  - The Dockerfile does not need `docs/`: only the test reads the runbook, from the repo.

**Complexity:** Medium
**Token estimate:** ~25k tokens
**Agent strategy:** Single agent, sequential execution. The content is one page and must read as one voice.

### Task: Add an About panel to the board

**File changes:**
- `web/vite.config.js:13-28` — add `server.fs.allow: ['.', '../internal/notice']` so the dev server can serve the shared file. The build needs nothing.
- `web/src/App.svelte:146-151` — after the Autoplay label and before the `{#if app.account}` block, add `<div class="sep"></div>` and `<button role="menuitem" onclick={() => menu(() => (app.modal = { kind: 'about' }))}>About tcgreplay</button>`. The item is outside every `snap`, `phone` and `account` condition, so it shows at every layout and in desktop mode.
- `web/src/components/Modal.svelte`:
  - Import `notice from '../../../internal/notice/notice.json'`.
  - Add `{:else if m.kind === 'about'}` beside `prompt` (`Modal.svelte:240-242`), following its header pattern: `<header><h2>About tcgreplay</h2>{#if !touch}<button onclick={close}>Close</button>{/if}</header>`. The shell (`:129-132`) already gives the backdrop, `role="dialog"`, the touch ✕ (`aria-label="Close"`) and the Escape handler (`:127`). The body renders:
    - `<p class="fan-notice">{notice.notice}</p>`;
    - `<p class="credit">{notice.credit.lead} <a href target="_blank" rel="noopener">…</a> and <a…>…</a>.</p>`;
    - `<p><a href={notice.help.url} target="_blank" rel="noopener">Get help</a> (GitHub issues)</p>`.
  - Text must wrap at 390 px.
- `web/e2e/about.spec.js` (new), using `web/e2e/fixtures.js` like `phone.spec.js`:
  - For each project, and for an extra 820×1180 viewport on the tablet project:
    - record the bounding boxes of the hand, the active cards and the header controls;
    - open ☰ → "About tcgreplay";
    - assert the panel's notice text equals the shared file (read independently with `readFileSync`) and names the four companies;
    - assert the links point to tcgdex.dev, malie.io and the issues URL;
    - close with the button named "Close" (✕ on touch, Close on desktop), reopen and close with Escape;
    - assert the recorded boxes are unchanged and no `localStorage` key exists beyond `replaySpeed`, `autoplay`, `panelAt` and `prizeReminders`.
  - On phone, also assert the existing fit (no page or board scroll) after closing.
- `web/e2e/phone.spec.js:205-215` — if that test enumerates the desktop/tablet menu items exactly, add "About tcgreplay" to its list. Likewise the ☰ inventory at :131-165 for phone.

**Complexity:** Low
**Token estimate:** ~15k tokens
**Agent strategy:** Single agent, sequential execution.

## Testing Strategy

The tests extend the three existing suites; see plan.md § Testing Approach for the guarantees. Per task:

- **Share one notice wording and show it in the website footer**:
  - `site/e2e/notice.spec.js` (new, no browser): `loadNotice` on the real file; failure fixtures.
  - `site/e2e/site.spec.js`, every-page test: the footer notice equals the shared file (read independently), the credit links, and the Privacy and help links.
  - The container build still succeeds.
- **Publish the terms of service page**:
  - `site/e2e/terms.spec.js` (new): regions, required statements, `data-limit` values against an independent `.do/app.yaml` reader, the shared notice in the trademarks section, privacy and help links, the `data-updated` format.
  - `site/e2e/site.spec.js`: the footer Terms link on every page; the crawl reaches `/terms`, and the 375 px check covers it.
  - `cmd/tcgreplay/hosted_routes_test.go`: `/terms` signed out, with no redirect and no cookie.
- **Bring the privacy page up to date**: `site/e2e/privacy.spec.js`:
  - the new "Visitor counting" and "Diagnostics" regions;
  - "Analytics" no longer in the never list;
  - each `data-retention` value ("logs", "metrics") found in `docs/observability.md`'s Retention section;
  - the corrected statements present and the false ones absent;
  - the issues link;
  - the `data-updated` format.
- **Add an About panel to the board**:
  - `web/e2e/about.spec.js` (new): on phone 390, tablet (1024, plus an 820×1180 viewport) and desktop 1440, the panel opens from ☰ and shows the shared text and links. It closes with Close or ✕ and with Escape; the board's boxes are unchanged; no new `localStorage` keys; the phone fit still holds.
  - Existing menu-inventory tests in `web/e2e/phone.spec.js` are updated if they enumerate items exactly.
- **Manual items** (written into the implementation test plan): every item marked "Manual — captured in the implementation test plan" in plan.md § Testing Approach:
  - the ChatGPT submission accepting the links (success metric);
  - the live public addresses;
  - the privacy accuracy review against the runbook and Cloudflare's docs;
  - the last-updated dates equal to the go-live day;
  - the terms read-through;
  - the board on real devices.

## Project References

- Spec: `20261112083238-29247b36-privacy-terms-disclaimer` (epic `20261111093348-e1b5c2b4-public-launch`).
- Upstream plans: `20261112090330-4ac2ea19-observability` (runbook `docs/observability.md`, `p_` player IDs, no email in logs) and `20261111093353-8ff3589f-domain-move` (public address from `.do/app.yaml`; `demo.gs` only inside `data-moved-note`; never `www.tcgreplay.app`).
- Downstream specs: `20261111132045-1d1d32f6-visitor-analytics` (counting starts only after this page describes it) and `20261112090820-07d70cfb-chatgpt-submission-prep` (submits `/privacy`, `/terms` and the issues page; can read `internal/notice/notice.json`).
- Design documents: none.
- Knowledge entries:
  - `repo/tcgreplay` `conventions/skills-text-and-packaging.md`
  - `repo/tcgreplay` `gotchas/astro-site-build.md`
  - `repo/tcgreplay` `gotchas/playwright-touch-tests.md`
  - `repo/tcgreplay` `learnings/publishing-to-chatgpt.md`
  - `repo/tcgreplay` `gotchas/disconnect-does-not-reach-playmat.md`, written during this plan
- Requirement → repo and files:
  - privacy visitor counting, diagnostics, accuracy and date → `tcgreplay:site/src/pages/privacy.astro`, `site/e2e/privacy.spec.js`;
  - terms published, content and trademarks → `tcgreplay:site/src/pages/terms.astro`, `site/e2e/terms.spec.js`, `cmd/tcgreplay/hosted_routes_test.go`;
  - footer links and help links → `tcgreplay:site/src/layouts/Base.astro`, `site/src/lib/notice.ts`, `internal/notice/notice.json`, `site/e2e/site.spec.js`;
  - board notice and credit, board help → `tcgreplay:web/src/App.svelte`, `web/src/components/Modal.svelte`, `web/vite.config.js`, `web/e2e/about.spec.js`;
  - build → `tcgreplay:Dockerfile`.
- Repo root: tcgreplay at `/home/nicj/code/github.com/nicholasjackson/tcgreplay`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

## Migration Notes

- No data or configuration migration. Nothing in `.do/app.yaml` changes, so no `doctl apps update` is needed; the next push to main deploys the pages.
- Order across the epic: the observability work (with its Retention figures) lands before the privacy task. Visitor counting is switched on only after the privacy task is live.
- The Google consent screen's privacy URL keeps pointing at `/privacy` (the domain-move runbook moves its host to tcgreplay.app). The path does not change.

## Performance Considerations

- None at runtime: the pages are static, and the board panel renders only when opened. The notice file adds a few hundred bytes to the board bundle.

### What's new during launch (decided at epic planning)

- Applies to the terms task and the About panel task: if `site/src/data/whats-new.yaml` exists when the task lands (launch-docs merged first), add its player-visible change to the existing launch entry's `changes` ("Terms of service page" / "An About panel on the board with the fan-project notice and a help link"), not a new entry, and leave `refresh` as it is. If the file does not exist, skip: launch-docs' launch entry lists the launch changes already on main.
- The `visits` retention figure is not checked against the runbook; only `logs` and `metrics` are (see the retention test above). The manual privacy review checks `visits` against Cloudflare's documentation.
