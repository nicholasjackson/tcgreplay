---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Research: 20261111132045-1d1d32f6-visitor-analytics

## Alternatives considered and rejected

### Option A: Cloudflare's automatic setup (edge injection on a proxied zone)

No code; local and test runs are never proxied, so they would be silent.

**Rejected** because:
- SPA tracking is on by default and can only be turned off with the manual snippet's `"spa": false` (Cloudflare docs, `developers.cloudflare.com/web-analytics/get-started/web-analytics-spa/`: "This requires manual embedding").
- The board pushes a history entry each time the phone action sheet opens (`web/src/components/ActionSheet.svelte:37`, `history.pushState({ sheet: true }, '')`). With SPA tracking on, Cloudflare patches `pushState` and would count extra board page views, breaking "one board session, one board visit".
- It cannot be gated on the privacy page or switched off by the playmat's own configuration, and it is unclear whether the maintainer's zone proxies tcgreplay.app at all (the live response's `server: cloudflare` and `x-do-app-origin` headers show a Cloudflare edge in front of App Platform, but not whose zone it is).
- The spec's Technical Approach prefers a token supplied through configuration.

### Option B: Snippet written into the built pages (Astro `Base.astro` and `web/index.html`) at build time

**Rejected** because:
- The two builds share no page shell (`site/src/layouts/Base.astro` vs `web/index.html`), so the snippet and its gating would be written twice.
- The token would have to reach two Node builds (Dockerfile `web` and `site` stages). A local `make site`/`make web` or the browser suites would either embed it (and count local runs) or need a second gate.
- Switching counting off would need a rebuild and deploy, not configuration alone.
- `Base.astro` belongs to the privacy-terms plan in this epic (orchestrator rule); editing it here invites a conflict.

### Option C: Injection inside `site.Handler()` and `webui.Server` via options

**Rejected** because: both test servers (`internal/site/sitetest/cmd/serve/main.go:31-41`, `internal/webui/webtest/cmd/boardtest`) and desktop mode build these handlers directly, so analytics would be one forgotten option away from local runs. A wrapper applied only by hosted mode (and opt-in by the test servers) keeps the default silent.

### Option D: Wrapping the whole hosted router

**Rejected** because: it would inject into sign-in, admin and "Not your game" pages too (`internal/usergames/board.go:132`, `cmd/tcgreplay/hosted.go:197`). Only the website routes and the `/board` page route are wrapped.

### Option E: Refusing to start on a malformed token

**Rejected** because: an analytics typo would take the playmat down. The project's pattern for optional settings is warn and carry on (`.do/app.yaml` Spaces comment: "setting only some logs a warning and does the same"; `cmd/tcgreplay/hosted.go` `hostedImages`).

### Option F: Tests that load the real beacon from Cloudflare

**Rejected** because: the spec says running the tests sends no analytics request. Browser tests stub or block both Cloudflare hosts with Playwright routing.

## Chosen approach — evidence

- Both shells are served by the hosted Go router: `cmd/tcgreplay/hosted.go:188-210` mounts `GET /board` (via `a.RequireSession(atRoot(board))`) and the website (`mux.Handle("/", site.Handler())`). This is the one place both pages pass through, so a single HTML-injecting wrapper there is "added once, to the page shell the website and the board share".
- Hosted mode is the only mode that serves the website and the public board (`cmd/tcgreplay/hosted.go:348-399` `runHosted`); desktop mode (`startUI`, `cmd/tcgreplay/main.go:325`) and the test servers never go through `hostedHandler`.
- Settings pattern: flag whose default is a `TCGREPLAY_*` variable (`cmd/tcgreplay/main.go:152-174`); `.do/app.yaml` declares optional settings without values for the control panel (`TCGREPLAY_ADMINS`).
- Site pages are `text/html; charset=utf-8`, `no-cache`, no ETag, served with `http.ServeContent` (`internal/site/site.go:84-110`); the board page is served by `http.FileServerFS` (`internal/webui/server.go:99`). Neither sets a CSP (grep for `Content-Security-Policy` outside `.spektacular` finds nothing), so no CSP change is needed now.
- Cloudflare manual snippet: `<script defer src="https://static.cloudflareinsights.com/beacon.min.js" data-cf-beacon='{"token": "...", "spa": false}'></script>`, placed before `</body>` (get-started page); non-proxied reports go to `https://cloudflareinsights.com/cdn-cgi/rum` (data-origin page).
- Cloudflare matches the page's hostname to the site by suffix (FAQ), so a beacon fired on another host is not counted; the playmat additionally injects only for the public host.
- Cloudflare definitions (high-level metrics page): a page view is "a successful HTTP response with a content-type of HTML"; a visit is "a page view that originated from a different website or direct link" (referer host differs). So a board session is one page view of `/board`; whether it is also a "visit" depends on where the player came from.
- Privacy-first gate: the privacy-terms plan marks Cloudflare's retention figure `data-retention="visits"` on the privacy page (its Data Structures section). The website is embedded (`internal/site/site.go:22-27`), Astro builds `privacy/index.html` (`site/astro.config.mjs` `build.format: 'directory'`), so the same binary can check its own privacy page before it switches counting on.
- Test servers: site suite runs `go run ./internal/site/sitetest/cmd/serve` (`site/playwright.config.js:22-28`); board suite runs `boardtest` (`web/playwright.config.js`). The existing third-party check is `site/e2e/site.spec.js:162-180` ("a fresh visit to every page sets no cookie and asks no other host for anything"); its header comment (lines 1-5) says "no web fonts, analytics or third-party resources".
- Hosted test helpers: `startHostedWith(t, public, mod func(*config))` (`cmd/tcgreplay/hosted_test.go:653`) lets a test set config; `TestHostedRoutes` (`cmd/tcgreplay/hosted_routes_test.go:28`) compares hosted answers with `site.Handler()`'s.

## Files examined

- `cmd/tcgreplay/hosted.go:170-245` — hosted path table; `/board` page route and website mount; `atRoot`.
- `cmd/tcgreplay/hosted.go:348-399` — `runHosted`, start-up log line, `hostedHandler` call site.
- `cmd/tcgreplay/main.go:142-175,289-296` — flag/env pattern, `envOr`.
- `cmd/tcgreplay/hosted_routes_test.go:1-60` — route test style, `siteResponse`, `getWith` with request modifiers.
- `cmd/tcgreplay/hosted_test.go:646-670` — `startHosted`/`startHostedWith`.
- `cmd/tcgreplay/cardsets_test.go:283`, `cmd/tcgreplay/hosted_admin_test.go:102` — other `hostedHandler` call sites.
- `internal/site/site.go` — embedded site handler, content types, no cookies, `fsys` swappable in tests.
- `internal/site/sitetest/cmd/serve/main.go` — site suite's server; `/board` etc. answer 503.
- `internal/webui/server.go:95-145` — board handler, `FileServerFS`, placeholder.
- `internal/webui/webtest/cmd/boardtest/main.go` — board suite's server, loopback-only.
- `internal/usergames/board.go:23-57,95-140` — board front door; page loads; "not your game" page.
- `web/index.html` — board shell (Vite), no shared layout with the website.
- `web/src/components/ActionSheet.svelte:37-54` — `history.pushState` on the phone action sheet.
- `web/src/lib/store.svelte.js:70-121`, `web/src/lib/playback.svelte.js:18-44` — the board's four localStorage settings.
- `site/src/layouts/Base.astro` — website shell (owned by privacy-terms plan).
- `site/src/lib/facts.ts` — `site.publicURL` (domain move makes it read `.do/app.yaml`).
- `site/e2e/site.spec.js:1-21,162-180` — crawl, no-cookie and no-other-host checks.
- `site/playwright.config.js`, `web/playwright.config.js` — single `webServer`, profiles.
- `site/astro.config.mjs` — directory build format.
- `.do/app.yaml` — env declarations; `TCGREPLAY_ADMINS` declared without value; uncommitted domain edits in the main tree.
- `Dockerfile:1-17` — settings comment listing every hosted setting.
- `Makefile:85-101` — local container run passes selected env vars (analytics token deliberately not passed).
- `TCG Review Playmat Design and Build Plan.md:307,330,345` — website paragraph, settings table, operator steps.

## External references

- Cloudflare Web Analytics, Get started (`developers.cloudflare.com/web-analytics/get-started/`) — manual snippet before `</body>`; automatic setup for proxied sites; `Cache-Control: no-transform` blocks automatic injection.
- Cloudflare Web Analytics, SPAs (`.../web-analytics/get-started/web-analytics-spa/`) — three SPA tracking methods incl. patching `pushState`; `"spa": false` requires manual embedding.
- Cloudflare Web Analytics, FAQ (`.../web-analytics/faq/`) — CSP: `script-src` beacon URL and `connect-src cloudflareinsights.com` for manual embeds; hostname suffix matching; 7 days unsampled then ~10% sample; six months of data accessible.
- Cloudflare Web Analytics, High-level metrics (`.../web-analytics/data-metrics/high-level-metrics/`) — page view and visit definitions.
- Cloudflare Web Analytics, Data origin and collection (`.../web-analytics/data-metrics/data-origin-and-collection/`) — collects timing metrics and Core Web Vitals; reporting endpoint `cloudflareinsights.com/cdn-cgi/rum` for non-proxied sites.
- Live probe `curl -D - https://tcgreplay.app/` (2026-11-12) — no beacon on the page today; a `__cf_bm` cookie is set by Cloudflare's edge, not the playmat (knowledge `gotchas/live-site-cf-bm-cookie.md`).

## Prior plans / specs consulted

- `20261112083238-29247b36-privacy-terms-disclaimer` (final plan) — privacy page gains a visitor-counting section with `data-retention="visits"`; this spec updates the site suite's third-party check; `Base.astro` is theirs; Cloudflare must not set cookies or storage (else STOP).
- `20261111093353-8ff3589f-domain-move` (final plan) — `TCGREPLAY_PUBLIC_URL` becomes `https://tcgreplay.app`; a moved-host guard wraps `hostedHandler` and redirects `tcgreplay.demo.gs`; `facts.ts` reads the public URL from `.do/app.yaml`; it also edits `hosted.go` and `.do/app.yaml`.
- `20261112090330-4ac2ea19-observability` (final plan) — no log line carries an email; edits `hosted.go` start-up and `.do/app.yaml`; runbook `docs/observability.md`.
- `20261112091756-c5b6d1fd-launch-docs` (final plan) — stays out of `Base.astro`; new docs pages pass the crawl's no-third-party check.

## Open assumptions

- App Platform passes the public host (`tcgreplay.app`) to the container in `Host`. The domain-move plan relies on the same for its moved-host guard. If live page views never appear and the page source on tcgreplay.app has no beacon while the start-up log says counting is on, STOP and ask (candidate fix: match `X-Forwarded-Host`).
- The privacy-terms work keeps the `data-retention="visits"` marker on the visitor-counting section. If it uses another marker, the gate in this plan follows that marker; if the privacy page has no such section, counting stays off by design.
- Cloudflare's beacon sets no cookie and writes no browser storage (Cloudflare markets it as cookieless; not stated on the pages fetched). If the live devtools check finds otherwise, STOP and ask: it contradicts the spec's cookieless constraint.
- The maintainer's Cloudflare account can add tcgreplay.app as a Web Analytics site with the manual snippet, and automatic injection is not already on for it (the live page had no beacon on 2026-11-12).
- Cloudflare site tokens are short alphanumeric strings (typically 32 hex characters); the playmat accepts 16 to 64 ASCII letters and digits.

## Drafting assumptions

### Chosen direction: hosted router adds Cloudflare's manual snippet at serve time (architecture)
- **Decision**: A new `internal/webanalytics` package supplies the snippet (`"spa": false`) and an HTML-injecting wrapper; hosted mode wraps only the website routes and the `/board` page route. Counting needs a well-formed `TCGREPLAY_WEB_ANALYTICS_TOKEN`, an embedded privacy page carrying `data-retention="visits"`, and a request for the public host. Otherwise counting is off, with the reason in the start-up log.
- **Rationale**: One place for both shells; silent by default everywhere but the live playmat; switched off by configuration alone; privacy page first by construction; no edits to `Base.astro`.
- **Rejected**: Cloudflare automatic injection; snippet in both builds; options on `site.Handler`/`webui.Server`; wrapping the whole router (see research.md alternatives).

### Conventions selected (architecture)
- **Decision**: Keep only the guides-packaging convention (the Dockerfile settings comment is edited).
- **Rationale**: It is the project's only convention, and it applies because the Dockerfile is touched; no guide text changes.
- **Rejected**: None to drop.

### Inject the snippet at serve time in hosted mode (discovery)
- **Decision**: The hosted Go router inserts Cloudflare's manual snippet into the website's and the board page's HTML responses, only when a token is configured.
- **Rationale**: The website (Astro) and the board (Vite) share no page shell; the hosted router is the one place both pass through. Local runs, desktop mode and tests never go through it, so they stay silent by default, and counting is switched off by configuration alone.
- **Rejected**: Cloudflare's automatic edge injection (cannot turn SPA tracking off); writing the snippet into both builds (two copies, token in Node builds, rebuild to switch off, `Base.astro` belongs to privacy-terms).

### Turn SPA tracking off (discovery)
- **Decision**: The snippet carries `"spa": false`.
- **Rationale**: The phone action sheet calls `history.pushState`; Cloudflare's SPA mode patches `pushState` and would count extra board page views. Neither the website nor the board changes page without a load.
- **Rejected**: Leaving SPA mode on (inflates board counts).

### Count a board session as one `/board` page view (discovery)
- **Decision**: "One visit to the board" is read in the dashboard as one page view of the path `/board`, filtered by path.
- **Rationale**: Cloudflare counts a "visit" only when the referer host differs, so a player who arrives from the website home page makes a `/board` page view but not a new Cloudflare "visit". A page view per board load is the honest per-session number.
- **Rejected**: Cloudflare's "visits" figure filtered to `/board` (misses players who came from the website).

### Gate counting on the embedded privacy page (discovery)
- **Decision**: The playmat switches counting on only if its own embedded privacy page carries the `data-retention="visits"` marker the privacy-terms plan defines; otherwise it logs that counting is off and why.
- **Rationale**: The spec and the epic require the privacy page first. Checking the binary's own page makes that hold mechanically, whatever order the token and the deploy happen in.
- **Rejected**: Relying only on the person's task order (one mistake starts counting early).

### Inject only for the public host (discovery)
- **Decision**: The wrapper injects only when the request's host (ignoring case and port) equals `TCGREPLAY_PUBLIC_URL`'s host.
- **Rationale**: Keeps the App Platform default `ondigitalocean.app` address, the old alias and any local `-hosted` run out of the dashboard ("only tcgreplay.app traffic"); Cloudflare's own hostname matching backs it up.
- **Rejected**: Injecting for every host (relies on Cloudflare alone).

### A malformed token warns and leaves counting off (discovery)
- **Decision**: A token that is not 16 to 64 ASCII letters and digits is not used; the start-up log says counting is off and why. The playmat still starts.
- **Rationale**: Follows the optional-settings pattern (partial Spaces settings warn). The check also stops anything reaching the page's HTML that could break out of the attribute.
- **Rejected**: Refusing to start (an analytics typo would take the playmat down).

### Token declared in `.do/app.yaml` without a value, set in the control panel (discovery)
- **Decision**: `TCGREPLAY_WEB_ANALYTICS_TOKEN` is declared (RUN_TIME, no value) like `TCGREPLAY_ADMINS`; the maintainer sets its value in the control panel once the privacy page is live, and clears it to stop counting.
- **Rationale**: Follows the existing pattern for optional settings set by hand; a value change needs only a restart, not a release.
- **Rejected**: Writing the value into `.do/app.yaml` (needs a commit and a `doctl apps update` for every on/off, and would be easy to apply before the privacy page is live).

### Inject only into successful GET HTML responses (discovery)
- **Decision**: Only `GET` responses with status 200 and an HTML content type get the snippet, inserted before the last `</body>` (or appended when there is none). Redirects, 404 pages, 503 refusals, `HEAD` and range responses pass through unchanged.
- **Rationale**: Matches Cloudflare's definition of a page view and keeps sign-in redirects and refusals out of the counts.
- **Rejected**: Injecting into the 404 page (counts mistyped addresses as pages).

### Browser tests use opt-in test servers and never contact Cloudflare (discovery)
- **Decision**: The site and board test servers gain an opt-in analytics flag; the suites start a second instance with it and route both Cloudflare hosts to a stub or a block.
- **Rationale**: Exercises the real built pages with the snippet in place (blocked analytics, no storage, no cookies) while keeping "tests send nothing" true.
- **Rejected**: Loading the real beacon in tests (sends analytics traffic); Go tests only (no browser evidence that blocking changes nothing).

### No content security policy work (discovery)
- **Decision**: No CSP is added or changed.
- **Rationale**: Neither the website nor the board sends a CSP today (no `Content-Security-Policy` anywhere in the code). The design doc notes what a future CSP must allow.
- **Rejected**: Adding a CSP now (out of the spec's scope).

### Local container runs never count (tasks)
- **Decision**: `make docker-run` does not pass `TCGREPLAY_WEB_ANALYTICS_TOKEN`, with a comment saying so.
- **Rationale**: Spec: running the playmat locally adds nothing to the dashboard. The public-host check would also stop it, but not passing the token is the plainer guarantee.
- **Rejected**: Passing it like the other optional settings.

### The website test server refuses to start with analytics on if the privacy page lacks the section (tasks)
- **Decision**: With `-analytics-token`, the site test server applies the same privacy gate as hosted mode and exits with the reason if counting would be off.
- **Rationale**: A silent "off" would make the analytics spec fail confusingly or test nothing. Exiting ties the suite to the privacy-terms work having landed, which this spec depends on anyway.
- **Rejected**: Bypassing the gate in the test server (the suite would no longer exercise it).

### Second test-server instances on their own ports (tasks)
- **Decision**: The site suite adds a server on 127.0.0.1:7792 and the board suite one on 127.0.0.1:7789; only the analytics specs use them.
- **Rationale**: Existing specs, and the board's single shared game, stay untouched; Playwright supports several web servers.
- **Rejected**: A runtime toggle endpoint on the existing servers (shared state across parallel site tests).

### Switching on is one person's task, after the privacy page is live (tasks)
- **Decision**: A single `human` task covers creating the Cloudflare site (manual snippet, automatic injection off) and setting the token in the control panel, starting with a check that the privacy page is live.
- **Rationale**: Both need consoles an agent cannot reach; app spec and control-panel changes take effect only when a person applies them. The dashboard checks are manual items in the Testing Approach, not tasks.
- **Rejected**: Splitting the Cloudflare and App Platform steps (no agent work between them).

### "The page shell the website and the board share" read as the hosted server (verification)
- **Decision**: The spec's preference for adding the script once to the shared page shell is met by adding it once in the hosted server, which serves both shells.
- **Rationale**: No shared page shell exists in the code: the website's layout is an Astro component and the board's is Vite's `index.html`. The hosted router is the one place both pass through, and it keeps the script in one place as the spec intends.
- **Rejected**: Creating a shared shell file both builds read (a new cross-build mechanism for one tag, with the token in the builds).

## Rehydration cues

- `spektacular spec file read 20261111132045-1d1d32f6-visitor-analytics`
- `spektacular plan file read 20261112083238-29247b36-privacy-terms-disclaimer plan` (Data Structures: `data-retention`), and the same for `20261111093353-8ff3589f-domain-move` (moved-host guard) and `20261112090330-4ac2ea19-observability`.
- Re-read `cmd/tcgreplay/hosted.go:170-245,348-399`, `internal/site/site.go`, `internal/site/sitetest/cmd/serve/main.go`, `internal/webui/webtest/cmd/boardtest/main.go`, `site/e2e/site.spec.js:162-180`, `web/src/components/ActionSheet.svelte:30-60`.
- `spektacular knowledge read --data '{"tier":"repo","name":"tcgreplay","path":"gotchas/astro-site-build.md"}'` and `gotchas/live-site-cf-bm-cookie.md`.
