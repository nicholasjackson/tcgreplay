---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Research: 20261112083238-29247b36-privacy-terms-disclaimer

## Alternatives considered and rejected

### Option A: Hand-kept copies of the notice, pinned by tests

Copy the footer's notice and credit (`site/src/layouts/Base.astro:52-57`) into the board (`web/src/App.svelte`, a separate Vite/Svelte build into `internal/webui/dist/app`, `web/vite.config.js:15-18`) and pin both copies with tests.

**Rejected**: Two copies drift between test runs, and the ChatGPT listing (submission-prep spec: "a description that includes the fan-project notice") would need a third. The spec's Technical Approach asks for one wording reused everywhere.

### Option B: Serve the notice from the Go server

Add an `/api/notice` route the board fetches at runtime.

**Rejected**: A route and a runtime fetch for static text. The website is static files built ahead of time (`internal/site/site.go:1-9`), so it would still need a build-time copy.

### Option C: A permanent notice strip on the board

Show the notice and credit in a strip always visible on the board.

**Rejected**: The spec's Technical Approach prefers on-demand placement to keep the phone layout uncluttered. The phone board is sized so nothing scrolls (`web/src/App.svelte:22-49`, `web/e2e/phone.spec.js:24-44`), so a strip would shrink the cards.

### Option D: Typing the limits into the terms by hand

Write the requests-a-minute, players-at-once and idle figures into the terms as text.

**Rejected**: `site/src/lib/facts.ts` already reads `TCGREPLAY_RATE_LIMIT`, `TCGREPLAY_MAX_USERS` and `TCGREPLAY_IDLE_TIMEOUT` from `.do/app.yaml` and fails the build if one is missing; the privacy and limits pages use it (`site/src/pages/privacy.astro:7`, `site/src/pages/docs/limits.astro:8`). The spec asks for figures from configuration. (Retention has no configuration value, being DigitalOcean's and Cloudflare's, so it alone is typed in, and pinned by a test against the runbook.)

### Option E: Parsing the observability runbook at site build time

Have the site build read the retention periods out of `docs/observability.md`.

**Rejected**: The runbook's wording is owned by the observability work and is free prose; a build that parses it breaks on any rewording and would need the Dockerfile's `site` stage to copy `docs/`. A browser-suite check that the page's marked retention values appear in the runbook's Retention section catches drift without coupling the build.

### Option F: A bespoke legal text for the terms

Commission or draft formal legal terms.

**Rejected**: The spec's Technical Approach says to model the terms on community tools such as Limitless; its Non-Goals exclude a lawyer's review and a governing law.

### Option G: Terms in the header navigation

Add Terms beside Privacy in the site's header navigation.

**Rejected**: The spec asks for the footer of every page to link both pages. The header is already full on a phone, and its list is pinned by `site/e2e/site.spec.js:13-20`.

### Option H: Adding token revocation so "disconnect" is true

Add an OAuth revocation endpoint so the privacy page's "disconnect your assistant and the playmat forgets you" becomes true.

**Rejected**: The spec asks for the page to be corrected where it differs, not for new behaviour; there is no revocation endpoint today (`internal/hostauth/oauth.go:53-71`). A separate spec could add it (`gotchas/disconnect-does-not-reach-playmat.md`).

## Chosen approach — evidence

- New website pages need no Go change: the site handler serves every built file without sign-in and never sets a cookie (`internal/site/site.go:1-9`, `Handler`), and `site/e2e/site.spec.js` crawls every same-origin link, so a footer link to `/terms` is crawled and checked automatically.
- Every website page uses `Base.astro` (the footer is there, `site/src/layouts/Base.astro:49-59`), so footer links and the shared notice reach every page in one edit.
- The board has a ☰ menu present at every layout (`web/src/App.svelte:111-158`) and a modal system (`web/src/components/Modal.svelte`), so an "About tcgreplay" menu item opening a modal is on demand, covers nothing until opened, and closes like every other view.
- The board suite already runs phone (390x844), tablet (1024x768) and desktop (1440x900) profiles (`web/playwright.config.js:21-34`); a test can set 820 px for the spec's tablet width with `page.setViewportSize`, as `web/e2e/phone.spec.js:27` does.
- The site's facts loader pattern (`site/src/lib/facts.ts`: `process.cwd()`-relative reads, a `FactError` that fails the build) is the model for reading the shared notice file (`gotchas/astro-site-build.md`: build-time paths from `process.cwd()`, never `import.meta.url`).
- The Dockerfile's `site` stage already copies files from outside `site/` that the build reads (`Dockerfile:30-33`: `internal/guides`, `.do/app.yaml`, `testdata/decks`); the `web` stage copies only `web/` (`Dockerfile:21-25`), so both need one more `COPY` for the notice file.
- The privacy page's storage statement matches the board: exactly four `localStorage` keys (`replaySpeed`, `autoplay`, `panelAt`, `prizeReminders`; `web/src/lib/store.svelte.js:70-121`, `web/src/lib/playback.svelte.js:18-44`). The About panel must add none.
- Observability plan (final): no log line carries an email; records keyed by `p_` IDs (keyed HMAC, `TCGREPLAY_PLAYER_ID_KEY`); closed event catalogue; error messages redacted; `docs/observability.md` sections "What is recorded", "What is never recorded", "Player IDs", "Retention" (Retention filled by its "Record capacity and retention" task after a person confirms Insights' retention).
- Domain-move plan (final): `site.publicURL` comes from `TCGREPLAY_PUBLIC_URL` in `.do/app.yaml`; a whole-site scan fails on `demo.gs` outside a `data-moved-note` element and on any `www.tcgreplay.app`. The new terms page must name the address only through `site.publicURL`.

### Audit of the current privacy page against the code (HEAD plus the working-tree `.do/app.yaml`)

- **Sign-in data: true.** Scopes `openid email` only (`internal/hostauth/hostauth.go:134`); keeps `sub`, `email`, `email_verified` (`google.go:132-142,189`; `hostauth.go:69-78`); Google's token response read once for the `id_token`, never stored (`google.go:120-128`).
- **"Sign-ins kept encrypted, in memory only": misleading — correct it.** Sessions, accounts and refresh tokens are plain in-memory maps (`store.go:20-45,86-100`); what is encrypted (AES-GCM, `seal.go:22-46`) is the copy handed to the browser cookie and the assistant's tokens. With `TCGREPLAY_SIGNING_KEY` set (`.do/app.yaml`), sign-ins survive a restart (`hostauth.go:1-11`; `store.go:188-211`); games do not.
- **Cookies: true.** Only `tcgreplay_login_<state>` (HttpOnly, path `/auth/google/callback`, 10 minutes; `google.go:43-51,74`) and `tcgreplay_session` (HttpOnly, 30 days; `google.go:222-230`; `board.go:134`). The website sets none (`internal/site/site.go:6-8`; `cmd/tcgreplay/hosted_routes_test.go:34-45`).
- **Browser settings: true.** Exactly four `localStorage` keys; no sessionStorage, IndexedDB or `document.cookie` in `web/src` or `site/src`.
- **Games in memory only: true for the game.** `internal/game/manager.go:20-22`; one game per account (`internal/usergames/usergames.go:57-60`); a foreign board handle gets "not your game" (`usergames/board.go:31-36`). Side finding (not personal data, out of scope): hosted mode shares one data directory, and `SetAssistant` writes the assistant's client name to `<dataDir>/assistant.txt` on each tool call (`manager.go:93-101`; `internal/mcpserver/server.go:132-135`; `cmd/tcgreplay/hosted.go:252-256`).
- **Retention constants: true.** Idle 60m (`.do/app.yaml`), board session 30 days (`store.go:15`), refresh 30 days renewed on refresh (`store.go:17,405`), access token 1 hour (`oauth.go:27`), Google sign-in in progress 10 minutes (`store.go:13`).
- **"Disconnect your assistant … once your sign-ins end, the playmat forgets you": partly — correct it.** There is no token revocation endpoint (`oauth.go:53-71`), so disconnecting in the assistant does not reach the playmat; the connection's record and the account entry stay in memory until the refresh token runs out, up to 30 days after its last use (`store.go:131-136`). Also, after a restart a still-valid access token is accepted without counting a sign-in, and that account's registry entry is removed by the 24-hour purge (`store.go:413-419`; `usergames.go:46,264-268`).
- **Logs: partly, and replaced by the observability work.** Today the email is logged on sign-in, sign-out, limits, expiry and removal (as the page says) and also on refused sign-ins (`google.go:93`), foreign board links (`usergames/board.go:34`), admin refusals (`admin.go:42,51`) and admin actions (`internal/cacheadmin/cacheadmin.go:189,203,243,245`). After observability no log line carries an email. No IP addresses are logged (no `RemoteAddr`, `X-Forwarded-For` or `CF-Connecting-IP`; only Go's own "panic serving <addr>", the platform proxy's address). No battle logs or decks are logged.
- **Outside services: partly — correct it.** Card data comes only from the published card sets (`internal/catalog/loader.go:263,417`), but the hosted server itself fetches card images from `assets.tcgdex.net` and `cdn.malie.io` at runtime to copy them into the DigitalOcean Spaces bucket (`internal/cards/images.go:137,149,469-604`) — no player data goes with them. The "playmat's own image store" is a DigitalOcean Spaces CDN (`digitaloceanspaces.com`), not tcgreplay.app (`internal/webui/server.go:42-61`). The page never names Spaces.
- **Website sets no cookies and loads nothing from other sites: true today.** Becomes untrue for "loads nothing" once visitor counting adds Cloudflare's beacon.
- **Removing data: partly.** Sign out ends only that browser's board session (`board.go:125-137`); disconnecting the assistant: see above. Nothing is stored per account on disk or in Spaces.
- **Address: the page names `site.publicURL`,** which the domain-move work makes `https://tcgreplay.app`.
- **Board:** the ☰ menu (`web/src/App.svelte:111-158`); natural place for an About item is after Autoplay and before the account block. The desktop board (`127.0.0.1:7777`) serves no website, so `/privacy` and `/terms` links would only work hosted; the GitHub issues link is absolute and works everywhere. Layout breakpoints are set in JS (`web/src/lib/store.svelte.js:77-84`: phone `max-width: 639px`, tablet `pointer: coarse`, desktop otherwise). Board tests never show the account block (the board test server serves no `/auth/session`).
- **Website routing:** `/` catch-all `site.Handler()` (`cmd/tcgreplay/hosted.go:208`), `site.resolve` maps `/x` to `x/index.html` (`internal/site/site.go:112-126`), no allow-list, `/terms` clashes with no route. `cmd/tcgreplay/hosted_routes_test.go:34` lists public website paths checked signed out.
- **Other links to /privacy:** `site/src/content/docs/sign-in.md:11`, `site/src/pages/docs/limits.astro:24`, `internal/site/site_test.go`; the design notes' consent-screen step (owned by domain-move).

## Files examined

- `tcgreplay:site/src/pages/privacy.astro:1-150` — current privacy page; dated 8 November 2026; intro claims the website "sets no cookies and loads nothing from other sites"; Logs bullet says emails are logged; "never collects" lists analytics; Questions section already links GitHub issues.
- `tcgreplay:site/src/layouts/Base.astro:1-63` — every page's layout; footer has `.fan-notice`, `.credit` (TCGdex, malie.io), "Source on GitHub"; nav includes Privacy, not Terms.
- `tcgreplay:site/src/lib/facts.ts:1-150` — `site` (publicURL hard-coded today, moving to app.yaml under domain-move), `limits` from `.do/app.yaml`, `FactError` build failure.
- `tcgreplay:site/src/pages/docs/limits.astro:1-80` — limits page wording to mirror in the terms (one game, dropped when idle/updated/last sign-in ends, players at once, requests a minute).
- `tcgreplay:site/e2e/privacy.spec.js:1-55` — privacy tests: region-by-heading assertions; asserts "Analytics" in the never section (must change); idle time from app.yaml.
- `tcgreplay:site/e2e/site.spec.js:1-200` — crawl of every page; footer notice must name the four companies; TCGdex credit link; no cookies, nothing fetched from other hosts; 375 px phone no sideways scroll.
- `tcgreplay:site/e2e/limits.spec.js:1-40` — independent app.yaml reader pattern for test-side facts.
- `tcgreplay:site/playwright.config.js` — site suite projects laptop 1366 and phone 375; served by `go run ./internal/site/sitetest/cmd/serve`.
- `tcgreplay:internal/site/site.go:1-80` — static handler, no sign-in, no cookies, any built page served.
- `tcgreplay:web/src/App.svelte:1-170` — board header and ☰ menu (Deck list, timeline, prize reminders, autoplay, account + Sign out); menu items close the menu via `menu(run)`.
- `tcgreplay:web/src/components/Modal.svelte:1-60` — board modals keyed by `app.modal.kind`; touch layouts close with a round ✕, phone fills the screen.
- `tcgreplay:web/playwright.config.js` — board suite projects phone 390, tablet 1024, desktop 1440; served by `go run ./internal/webui/webtest/cmd/boardtest`.
- `tcgreplay:web/e2e/phone.spec.js:1-60` — phone layout checks, `setViewportSize`, menu button by role.
- `tcgreplay:web/vite.config.js` — board build into `internal/webui/dist/app`; dev server proxies `/ws` and `/api`.
- `tcgreplay:Dockerfile:19-34` — `web` and `site` build stages and what they copy.
- `tcgreplay:.dockerignore` — `internal/` is not excluded, so a file under `internal/notice/` reaches both stages.
- `tcgreplay:.do/app.yaml` — limits (300 rpm, 10 users, 60m idle), public URL already tcgreplay.app in the working tree.
- `tcgreplay:internal/hostauth/{hostauth,google,store,seal,board,admin}.go`, `tcgreplay:internal/usergames/{usergames,board}.go`, `tcgreplay:internal/cards/images.go`, `tcgreplay:internal/catalog/loader.go`, `tcgreplay:internal/webui/server.go`, `tcgreplay:cmd/tcgreplay/hosted.go:186-256` — audited by a helper agent (findings above).
- `tcgreplay:cmd/tcgreplay/hosted_routes_test.go:25-50` — public paths fetched signed out with no redirect and no cookie; add `/terms`.
- `tcgreplay:site/src/content/docs/sign-in.md:1-30` — links /privacy; says the menu shows Sign out.
- `tcgreplay:Makefile:34-60` — `make site`, `make site-test`, `make test` (Go, board suite, site suite).

## External references

- Cloudflare Web Analytics docs (developers.cloudflare.com/web-analytics, its "limits" page) — the implementer confirms the retention period and the fields collected here; secondary sources (ctrl.blog review, competitor comparisons) say about six months and cookieless with no localStorage. Why it mattered: the privacy page states visitor-counting retention.
- Limitless TCG's terms of service (limitlesstcg.com) — the spec names it as the model for a community tool's terms. Why it mattered: tone and section set (as is, no warranty, acceptable use, IP notice).
- OpenAI plugin guidelines, as summarised in `learnings/publishing-to-chatgpt.md` — privacy policy must cover data categories, purposes, recipients, retention and user controls; support contact must be accurate; listing must not target under-13s. Why it mattered: the review the success metric depends on.

## Prior plans / specs consulted

- `20261112090330-4ac2ea19-observability` plan and context (final) — what diagnostics record and never record, `p_` IDs, the runbook `docs/observability.md` and its sections, retention confirmed by a person in its human task.
- `20261111093353-8ff3589f-domain-move` plan (final) — public URL from `.do/app.yaml`; `demo.gs` only inside `data-moved-note`; never `www.tcgreplay.app`; Google consent screen's privacy URL moves to tcgreplay.app (so `/privacy` must stay put).
- `20261111132045-1d1d32f6-visitor-analytics` spec (final, no plan yet; depends on this spec) — Cloudflare Web Analytics on website and board, cookieless, nothing identifying, counting starts only once the privacy page describes it.
- `20261112090820-07d70cfb-chatgpt-submission-prep` spec — needs website, privacy, terms and support links, and a listing description that includes the fan-project notice.
- `20261106110254-f08e142d-project-website` (listed, not reread) — origin of the site, footer and privacy page.

## Open assumptions

- `docs/observability.md` exists with filled Retention figures when this plan is implemented (the epic orders observability first, but its retention comes from a person's task). If the Retention section has no figures, STOP and ask.
- Cloudflare documents a retention period for Web Analytics; if not, the page says Cloudflare keeps the counts under its own policy and links it.
- The privacy page may describe visitor counting before visitor-analytics switches it on (the visitor-analytics spec requires the privacy page first).
- Correcting the privacy page's wording is what "any that differed is corrected" asks for; the playmat's behaviour (no token revocation, the shared `assistant.txt`) is not changed by this plan.
- The board is the same Svelte app in hosted and desktop modes; the About panel shows in both.

## Drafting assumptions

### Chosen direction: one shared wording file, static website pages, an on-demand board panel (architecture)
- **Decision**: Put the notice, credit and help address in `internal/notice/notice.json`, read by the website build (footer, terms) and the board build (an About panel opened from the ☰ menu). Add `site/src/pages/terms.astro`, update `privacy.astro` and the footer in `Base.astro`. No Go behaviour changes. The terms and privacy figures come from the existing facts loader (`.do/app.yaml`). Retention figures are typed in with `data-retention` markers that a browser test checks against `docs/observability.md`.
- **Rationale**: Meets the spec's Technical Approach on all four points: one wording, on-demand board placement, figures from configuration, terms modelled on community tools. The website handler already serves any new page publicly, and the board already has a menu and a modal system.
- **Rejected**: Hand-kept copies pinned by tests (drift; a third copy for the ChatGPT listing); serving the text from a Go route (runtime fetch for static text); a permanent board strip (the spec prefers on-demand; it would shrink the phone board); parsing the runbook at build time (brittle coupling to prose owned by another spec).

### Conventions selected (architecture)
- **Decision**: Keep the one always-applied convention (guides text and packaging), because the Dockerfile's build stages change.
- **Rationale**: The Dockerfile edit must not disturb the `internal/guides` copy; no other convention exists.
- **Rejected**: Listing none (the Dockerfile is touched, so the packaging half applies).

### Footer links, not a nav entry, for Terms (architecture)
- **Decision**: The footer of every page links to Privacy, Terms, Get help (GitHub issues) and Source on GitHub. The header navigation is unchanged (it keeps Privacy, gains no Terms).
- **Rationale**: The spec asks for the footer to link both; the header is already full on a phone, and its list is pinned by `site/e2e/site.spec.js`.
- **Rejected**: Adding Terms to the header nav (crowds the phone header for no requirement).

### One file holds the notice, credit and help address (discovery)
- **Decision**: The fan-project notice, the card-data credit (TCGdex, malie.io with their addresses) and the help address (the GitHub issues page) live in one JSON file, `internal/notice/notice.json`. The website footer, the terms page and the board all render it, and the ChatGPT submission work can read the same file.
- **Rationale**: The spec's Technical Approach prefers one wording reused everywhere so the copies can't drift apart. The website (Astro) and the board (Vite/Svelte) are separate builds, so neither can import the other's components. A JSON file both builds read is the smallest shared source. `internal/` already holds files the site build reads (`internal/guides`).
- **Rejected**: Copying the wording into the board and pinning both with tests (two copies still drift between test runs, and the ChatGPT listing would need a third); serving the wording from the Go server (adds a route and a runtime fetch for static text); keeping it in `site/` and importing it from `web/` (makes the board depend on the website's folder).

### Retention for visitor counting comes from Cloudflare's own documentation (discovery)
- **Decision**: The privacy page states how long Cloudflare Web Analytics keeps its counts, using the period Cloudflare documents, cited in a comment in the page's source. If Cloudflare documents none, the page says the counts are kept by Cloudflare under its own policy and links to it.
- **Rationale**: The visitor-analytics spec sets no retention period and has no plan yet, and the acceptance criteria require retention only for diagnostics. A public privacy policy should still cover retention for each kind of data (the ChatGPT guidance in `learnings/publishing-to-chatgpt.md` lists retention).
- **Rejected**: Leaving visitor-counting retention out (incomplete for the review); inventing a figure (untruthful).

### Correct the privacy page's wording, not the playmat's behaviour (discovery)
- **Decision**: Where the audit found the page wrong (sign-ins described as "encrypted, in memory only"; disconnecting the assistant said to make the playmat forget you at once; card images said not to involve the playmat contacting TCGdex or malie.io; the image store not named as DigitalOcean Spaces), the page is rewritten to match the code. No code changes: no token-revocation endpoint is added.
- **Rationale**: The spec's acceptance criterion is that each statement is checked and "any that differed is corrected"; it asks for a truthful page, not new behaviour. Adding OAuth revocation is a separate feature.
- **Rejected**: Adding a revocation endpoint so "disconnect" really forgets the player (new behaviour outside the spec, needs its own spec).

### The board gets an About panel from its menu, with notice, credit and help only (discovery)
- **Decision**: An "About tcgreplay" item in the board's ☰ menu, shown at every layout and in both hosted and desktop modes, opens a panel with the fan-project notice, the card-data credit (links to TCGdex and malie.io) and a "Get help" link to the GitHub issues page. No privacy or terms links on the board.
- **Rationale**: The spec prefers on-demand placement (menu or about panel); the menu exists at every width. The desktop board serves no website, so relative `/privacy` and `/terms` links would break there, and the spec asks the board only for the notice, the credit and the help link.
- **Rejected**: A permanent strip (spec prefers on-demand; the phone board is sized not to scroll); plain menu links (the notice is two sentences, too long for a menu row); privacy and terms links shown only when signed in (not required, and the board test server cannot show the signed-in state).

### Last-updated date is a manual check, not derived from the build (testing_approach)
- **Decision**: The privacy and terms pages carry a typed last-updated date. The implementer sets it to the implementation day. A manual check confirms it equals the go-live day, and the implementer or the maintainer adjusts it at release if not.
- **Rationale**: The acceptance criterion is "the date the updated page went live"; a build-time date would change on every deploy (every push to main redeploys), misreporting when the content last changed.
- **Rejected**: Build date (wrong meaning); git commit date of the page (unavailable in the container build, which excludes `.git`).

### The Terms footer link lands with the terms page (tasks)
- **Decision**: The footer task adds Privacy and Get help links; the terms task adds the Terms link with the page.
- **Rationale**: The website suite's crawl fails on any link to a missing page, so a Terms link before the page would break the build between tasks.
- **Rejected**: One task for footer and terms (larger task mixing the shared-file plumbing with page content).

### The privacy task depends on no other task in this plan (tasks)
- **Decision**: "Bring the privacy page up to date" has no in-plan dependency; its prerequisite is the observability runbook (cross-spec, guarded by a STOP if Retention is empty).
- **Rationale**: It touches only the privacy page and its suite; the footer and terms changes do not affect it.
- **Rejected**: Chaining it after the terms task (false ordering).

## Rehydration cues

- `spektacular spec file read 20261112083238-29247b36-privacy-terms-disclaimer`
- `spektacular plan file read 20261112090330-4ac2ea19-observability plan` (and `context`), `spektacular plan file read 20261111093353-8ff3589f-domain-move plan`
- Read `site/src/pages/privacy.astro`, `site/src/layouts/Base.astro`, `site/src/lib/facts.ts`, `site/e2e/privacy.spec.js`, `site/e2e/site.spec.js`, `web/src/App.svelte`, `web/src/components/Modal.svelte`, `web/playwright.config.js`, `Dockerfile`.
- `spektacular knowledge read` repo/tcgreplay `gotchas/astro-site-build.md`, `learnings/publishing-to-chatgpt.md`, `gotchas/playwright-touch-tests.md`.
- `make site-test` (site suite) and `cd web && npm run build && npm run test:e2e` (board suite), or `make test`.
