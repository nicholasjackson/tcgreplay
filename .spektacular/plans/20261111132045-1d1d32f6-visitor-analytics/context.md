---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Context: 20261111132045-1d1d32f6-visitor-analytics

## Current State Analysis

- **Two page shells, one server.** The website is Astro (`site/src/layouts/Base.astro`), the board is Vite and Svelte (`web/index.html`). They share no layout. On the hosted playmat both are served by `hostedHandler` (`cmd/tcgreplay/hosted.go:188-210`):
  - the website on `/` via `site.Handler()` (`internal/site/site.go:79-110`);
  - the board page on `GET /board` via `a.RequireSession(atRoot(board))`, where `board` is the per-account front door (`internal/usergames/board.go:23-57`) and the board server's `http.FileServerFS` (`internal/webui/server.go:95-100`).
- **No analytics, no CSP, no cookies from the playmat.** Nothing in the code loads anything from Cloudflare today. No `Content-Security-Policy` is set anywhere. The website sets no cookie (`internal/site/site.go:1-8`). The live response carries a `__cf_bm` cookie from Cloudflare's edge, not the playmat (`gotchas/live-site-cf-bm-cookie.md`). The live home page carried no Cloudflare beacon on 2026-11-12.
- **The board pushes history.** `web/src/components/ActionSheet.svelte:37` calls `history.pushState({ sheet: true }, '')` when the phone action sheet opens, and `history.back()` when it closes. Nothing else in `web/src` or `site/src` changes the URL.
- **The board stores four settings.** `replaySpeed` and `autoplay` (`web/src/lib/playback.svelte.js:18-44`), and `panelAt` and `prizeReminders` (`web/src/lib/store.svelte.js:70-121`).
- **Settings pattern.** Hosted settings are flags defaulting to `TCGREPLAY_*` variables (`cmd/tcgreplay/main.go:152-175`). Optional settings warn and carry on (`cmd/tcgreplay/hosted.go:300-343`, `hostedImages`). `.do/app.yaml:120-127` declares `TCGREPLAY_ADMINS` without a value, for the control panel.
- **Test servers.**
  - The site suite runs `internal/site/sitetest/cmd/serve` (`site/playwright.config.js:22-28`; `/board` etc. answer 503).
  - The board suite runs `internal/webui/webtest/cmd/boardtest` (`web/playwright.config.js`), the desktop board server over a fixed game, shared by every spec (`workers: 1`).
  - The site suite's third-party check is `site/e2e/site.spec.js:162-180`.
- **Sibling plans touching the same files.**
  - The domain move adds a moved-host guard around `hostedHandler` and a literal `TCGREPLAY_PUBLIC_URL` in `.do/app.yaml` (partly present as uncommitted edits in the main tree).
  - Observability edits hosted start-up and `.do/app.yaml`.
  - Privacy-terms edits `Base.astro` and the privacy page, adding `data-retention="visits"`.

## Per-Task Technical Notes

### Requirement-to-repo resolution

All requirements land in the one registered repo, `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`):
- **Website visits are counted.** `cmd/tcgreplay/hosted.go` (wrap `site.Handler()`), with the snippet and wrapper in `internal/webanalytics`.
- **Board visits are counted, once per session.** `cmd/tcgreplay/hosted.go` (wrap the `GET /board` route), plus `"spa":false` in `internal/webanalytics`.
- **The maintainer can read the numbers.** Cloudflare dashboard; a person's task, no code.
- **Nothing identifies a visitor.** `internal/webanalytics` (token-only snippet, no page data), and the browser specs in `site/e2e` and `web/e2e`.
- **Blocking analytics changes nothing.** A deferred external script with no app dependency; covered by the browser specs.
- **Only the live playmat is counted.** Hosted-only wiring in `cmd/tcgreplay`; public-host check in `internal/webanalytics`; test servers opt-in only.
- **Privacy page comes first.** `internal/site/site.go` (`DescribesVisitorCounting`), the gate in `internal/webanalytics.Setup`, and the person's task order.

### Task: Add the visitor analytics package and the privacy page check

**File changes**
- `internal/webanalytics/webanalytics.go` (new) — package doc stating it is the playmat's one Cloudflare Web Analytics integration, used only by hosted mode and, opt-in, by the test servers. Contents:
  - `const BeaconURL = "https://static.cloudflareinsights.com/beacon.min.js"`.
  - `Snippet(token string) string` returns `<script defer src="` + BeaconURL + `" data-cf-beacon='{"token":"` + token + `","spa":false}'></script>`. Cloudflare's manual snippet goes before `</body>`, and `"spa": false` needs the manual embed (developers.cloudflare.com/web-analytics/get-started/web-analytics-spa/).
  - `validToken(s string) bool`: `^[A-Za-z0-9]{16,64}$`.
  - `type Settings struct{ Token, Host string; PrivacyDescribes bool }`.
  - `Setup(s Settings) (func(http.Handler) http.Handler, string)`. Trim the token first. The order of checks, each with its status:
    - empty token → identity, `"visitor analytics: off (no TCGREPLAY_WEB_ANALYTICS_TOKEN)"`;
    - invalid token → identity, `"warning: visitor analytics: off (TCGREPLAY_WEB_ANALYTICS_TOKEN is not a Cloudflare site token: 16 to 64 letters and digits)"`. Never echo the value.
    - empty host → identity, `"visitor analytics: off (no public host)"`;
    - `!PrivacyDescribes` → identity, `"visitor analytics: off (the privacy page does not describe visitor counting yet)"`;
    - otherwise → `func(next) { return Inject(next, Snippet(tok), host) }`, `"visitor analytics: on for <host>"`.
  - `Inject(next http.Handler, snippet, host string) http.Handler`.
    - Compare `r.Host` with `host` after stripping any port (`net.SplitHostPort`, falling back to the raw value) and lower-casing both. On a mismatch, or a method other than `GET`, call `next` directly.
    - Otherwise wrap `w` in a capturing writer. On `WriteHeader(code)`, capture only if code is 200 and `Content-Type` starts with `text/html`. Otherwise forward headers and status at once and stream every write. An implicit 200 counts on first `Write`, so sniff from the header set by then.
    - When capturing, buffer the body. After `next` returns, insert the snippet before the last case-insensitive `</body>`, or append it when there is none.
    - Delete `Content-Length`, then set it to the new length. Write status and body.
    - If `next` calls `Flush` while capturing, stop capturing: write what is buffered unmodified and stream the rest. A streamed page is never altered, and the board and site pages never flush.
    - Implement `http.Flusher` and `Unwrap() http.ResponseWriter` so `http.ResponseController` still works.
- `internal/webanalytics/webanalytics_test.go` (new) — table tests over `Setup` (every off reason, the on status, the status never containing the token, bad tokens: `"`, `<`, `'`, space, 15 and 65 characters, non-ASCII). Wrapper tests with `httptest`:
  - injected exactly once before `</body>`, including `</BODY>`, with no body tag, and with two `</body>` (last one wins);
  - the length header is right;
  - host match ignores case and port; another host is untouched;
  - each of these is byte-identical with headers unchanged: `HEAD`; 302; 404 HTML; 503; 206 range from `http.ServeContent`; `text/css`; `application/json`; a handler that flushes.
- `internal/site/site.go:72-77` (beside `Built`) — add `DescribesVisitorCounting() bool`. Read `privacy/index.html` (Astro's directory build format, `site/astro.config.mjs` `build.format: 'directory'`) from `fs.Sub(fsys, "dist/app")`. Return true only if it contains `data-retention="visits"`, the marker the privacy-terms plan defines on the privacy page's Cloudflare retention figure. Return false for the placeholder or a read error. The doc comment names the privacy-terms contract.
- `internal/site/site_test.go:17,31` — using the existing `fsys` swap, three cases: a privacy page with `<span data-retention="visits">6 months</span>` → true; a privacy page without the marker (the existing fixture at line 17) → false; placeholder only → false.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential: package and tests first, then the site check.

### Task: Count website and board pages on the hosted playmat

**File changes**
- `cmd/tcgreplay/main.go:59-80` (config struct, hosted-only block) — add `webAnalyticsToken string` with a comment: the Cloudflare Web Analytics site token, hosted mode only; empty means no counting.
- `cmd/tcgreplay/main.go:163-166` (hosted flags) — add `flag.StringVar(&c.webAnalyticsToken, "web-analytics-token", os.Getenv("TCGREPLAY_WEB_ANALYTICS_TOKEN"), "with -hosted, the Cloudflare Web Analytics site token; pages are counted only with it, for the public address, once the privacy page describes counting (default $TCGREPLAY_WEB_ANALYTICS_TOKEN)")`.
- `cmd/tcgreplay/hosted.go:172-210` (`hostedHandler`) — add a final parameter `counted func(http.Handler) http.Handler`.
  - Wrap only `mux.Handle("GET /board", counted(a.RequireSession(atRoot(board))))` and `mux.Handle("/", counted(site.Handler()))`.
  - The wrapper sits outside `RequireSession`, so a signed-out redirect passes through untouched; it is a 302, not 200 HTML.
  - Extend the doc comment's route list: website pages and the board page carry Cloudflare's beacon when visitor analytics are on (see `hostedAnalytics`).
  - Leave `/assets/`, `/ws`, `/api/`, `/mcp`, `/admin` and `a.Routes` unwrapped.
- `cmd/tcgreplay/hosted.go` (new function beside `hostedImages`, ~line 343) — `hostedAnalytics(c config, public *url.URL) func(http.Handler) http.Handler`. It calls `webanalytics.Setup(webanalytics.Settings{Token: c.webAnalyticsToken, Host: public.Hostname(), PrivacyDescribes: privacyDescribesCounting()})`, logs the status with `log.Print` (the status already says "warning:" when it is one) and returns the wrapper.
  - Declare `var privacyDescribesCounting = site.DescribesVisitorCounting` so tests can swap it; the Go tests embed whatever site happens to be built (`gotchas/astro-site-build.md`).
- `cmd/tcgreplay/hosted.go:384` (`runHosted`) — `hostedHandler(srv, reg, a, admin, hostedAnalytics(c, public))`. If the domain move's `movedGuard` already wraps this call, keep it outermost.
- `cmd/tcgreplay/cardsets_test.go:283` and `cmd/tcgreplay/hosted_admin_test.go:102` — pass an identity wrapper (`func(h http.Handler) http.Handler { return h }`).
- `cmd/tcgreplay/hosted_test.go:646-680` (`startHostedWith`) — build the handler with `hostedAnalytics(c, u)` so `mod` can set `c.webAnalyticsToken`. Default tests have no token, so pages are unchanged and the existing byte-for-byte comparisons in `TestHostedRoutes` still hold.
- `cmd/tcgreplay/hosted_analytics_test.go` (new) — use `startHostedWith(t, testPublic, func(c *config){ c.webAnalyticsToken = "0123456789abcdef0123456789abcdef" })` with `privacyDescribesCounting` swapped to true (restored with `t.Cleanup`). Use `getWith`'s modifier to set `r.Host = "playmat.example"` (the host of `testPublic`, `cmd/tcgreplay/hosted_test.go:570`). Assert:
  - `/`, `/docs/connect` and `/privacy` (signed out) each contain `webanalytics.Snippet(token)` exactly once and set no cookie;
  - the same paths with `r.Host` left as the test server's address carry no snippet;
  - signed-in `GET /board` carries it once. Use the existing sign-in helpers in `hosted_test.go`;
  - signed-out `GET /board` (navigate) redirects with no snippet in the body;
  - `/assets/…`, `/api/card…`, `/.well-known/oauth-protected-resource/mcp`, `/healthz` and `/admin` (signed in as a non-admin) never contain `cloudflareinsights`;
  - with the privacy hook false, or with token `"bad token"`, every checked page equals `siteResponse` byte for byte.
  - Capture `log` output (`log.SetOutput` to a buffer, restored) to check the on and off status lines contain no token.
- `.do/app.yaml:120-127` (after `TCGREPLAY_ADMINS`) — add:
  ```yaml
      # Visitor counting: the Cloudflare Web Analytics site token for
      # tcgreplay.app (Cloudflare dashboard > Web Analytics > the site >
      # Manage site, manual snippet; automatic injection off). Public by
      # nature: it appears in every counted page. Set it in the control
      # panel only once the privacy page describing visitor counting is
      # live; the playmat also refuses to count until its own privacy page
      # describes it. Pages are counted only for TCGREPLAY_PUBLIC_URL's
      # host. Clear it to stop counting (needs only a restart).
      - key: TCGREPLAY_WEB_ANALYTICS_TOKEN
        scope: RUN_TIME
  ```
  Also extend the header comment's website paragraph (`.do/app.yaml:30-36`) with one sentence: with `TCGREPLAY_WEB_ANALYTICS_TOKEN` set, website pages and the board page are counted by Cloudflare Web Analytics, with no cookies. The main tree has uncommitted domain edits to this file; merge onto them.
- `Dockerfile:5-17` (settings comment) — add `TCGREPLAY_WEB_ANALYTICS_TOKEN (the Cloudflare Web Analytics site token: website and board pages are counted, cookieless, once the privacy page describes it)`. Do not touch the `site` stage's `COPY internal/guides` or `.dockerignore` (convention).
- `Makefile:80-101` — leave `docker-run` without the token, so local container runs never count. Add one comment line saying it is deliberately not passed.
- `TCG Review Playmat Design and Build Plan.md:307` (website paragraph) — add a "Visitor counting" paragraph after it covering:
  - hosted mode adds Cloudflare's manual beacon (`"spa":false`) to website pages and the `/board` page, only with `TCGREPLAY_WEB_ANALYTICS_TOKEN` set, for the public host, when the embedded privacy page carries `data-retention="visits"`;
  - redirects, errors, assets and sign-in and admin pages are never counted, and local, desktop and test runs never count;
  - a board session is one `/board` page view;
  - clearing the setting stops counting;
  - no CSP is sent today, and a future one must allow `script-src https://static.cloudflareinsights.com` and `connect-src https://cloudflareinsights.com`.
- `TCG Review Playmat Design and Build Plan.md:330` (settings table) — add a row: `TCGREPLAY_WEB_ANALYTICS_TOKEN` (Cloudflare Web Analytics site token) | `-web-analytics-token` | none: no counting; malformed → warning, no counting; privacy page without the visitor-counting section → no counting.
- `TCG Review Playmat Design and Build Plan.md:345` (operator steps) — add a step after 8: once the privacy page describing visitor counting is live, add tcgreplay.app in Cloudflare Web Analytics with the manual snippet (automatic injection off), set its token as `TCGREPLAY_WEB_ANALYTICS_TOKEN` on the component, restart, and check the start-up log says `visitor analytics: on for tcgreplay.app`.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential: hosted wiring and tests, then `.do/app.yaml`, Dockerfile and design doc.

### Task: Check counting in the website's browser suite

**File changes**
- `internal/site/sitetest/cmd/serve/main.go:23-41` — add flag `-analytics-token` (default empty). In `handler()`, when it is set, wrap `site.Handler()` with the wrapper from `webanalytics.Setup(webanalytics.Settings{Token: tok, Host: <host of addr>, PrivacyDescribes: site.DescribesVisitorCounting()})`. If `Setup` reports off, exit with that status, so a test run against a site whose privacy page lacks the section fails loudly instead of silently testing nothing. Update the package comment.
- `internal/site/sitetest/cmd/serve/main_test.go` — add a case: with a token, `/` contains the snippet; without one, it does not. Skip if the site is not built, as the existing tests do.
- `site/playwright.config.js:22-28` — make `webServer` an array: the existing server, and a second one on `127.0.0.1:7792` running `go run ./internal/site/sitetest/cmd/serve -addr 127.0.0.1:7792 -analytics-token 0123456789abcdef0123456789abcdef`. Export nothing new. The analytics spec sets its own `baseURL` with `test.use`.
- `site/e2e/crawl.js` (new) — move `pagePath` and `crawl` out of `site/e2e/site.spec.js:23-60` so both specs share the crawl. `site.spec.js` imports them unchanged.
- `site/e2e/site.spec.js:1-5,162` — reword the header comment and the test title to say a run without the analytics token fetches nothing from any other host. The assertion stays exactly as strict (`elsewhere` must be `[]`), plus a check that no page contains `cloudflareinsights`.
- `site/e2e/analytics.spec.js` (new) — `test.use({ baseURL: 'http://127.0.0.1:7792' })`. In a `beforeEach`:
  - route `https://static.cloudflareinsights.com/**` to fulfil a stub script (`window.__beacon = (window.__beacon || 0) + 1;`);
  - route `https://cloudflareinsights.com/**` to `abort()`.

  Tests:
  - every crawled page has exactly one `script[src="https://static.cloudflareinsights.com/beacon.min.js"]`, whose `data-cf-beacon` parses to `{token: <the test token>, spa: false}` with no other keys;
  - across all pages, the only request to another host is the beacon URL, with no query string;
  - after visiting all pages, `context.cookies()` is empty, `document.cookie` is `''`, and `localStorage.length` and `sessionStorage.length` are 0;
  - with a separate route that aborts the beacon, every page answers, shows its `h1`, header navigation and footer, raises no `pageerror`, and a copy button still copies its block (reuse the approach of `site/e2e/site.spec.js:220`).

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential.

### Task: Check counting in the board's browser suite

**File changes**
- `internal/webui/webtest/cmd/boardtest/main.go:23-60` — add flag `-analytics-token`. When it is set, wrap `s.Handler()` (only the `"/"` mount, not `POST /test/reset`) with `webanalytics.Setup(webanalytics.Settings{Token: tok, Host: <host of addr>, PrivacyDescribes: true})`'s wrapper. The comment says the board test server has no website, so the privacy precondition is the hosted playmat's business, not the board's. Exit if `Setup` reports off.
- `web/playwright.config.js` (`webServer`) — make it an array: the existing server, and a second on `127.0.0.1:7789` with `-analytics-token 0123456789abcdef0123456789abcdef`. The existing specs keep the default `baseURL`.
- `web/e2e/analytics.spec.js` (new) — `test.use({ baseURL: 'http://127.0.0.1:7789' })`. Do not import the shared `fixtures.js` `page` fixture, which resets the default server. Use Playwright's base `test` with a local fixture that posts `/test/reset` to the analytics server and waits for `header .dot.on`, mirroring `web/e2e/fixtures.js:5-21`. Route Cloudflare's hosts as in the site spec. Tests:
  - the page has exactly one beacon tag with `spa: false`;
  - with the stub counting loads: open a card zoom (reuse `card`/`tapCard` from `web/e2e/fixtures.js:31-60`), switch branches as `web/e2e/phone.spec.js` does, and on the phone profile open and close the action sheet (`web/src/components/ActionSheet.svelte:37` pushes history). Then `window.__beacon` is still 1 and the beacon URL was requested once;
  - with the beacon aborted: the board shows the fixed game, a card zoom opens, a move (via `act` or a drag) succeeds, no `pageerror`, and no element with an error role or class becomes visible;
  - storage: after those steps, `Object.keys(localStorage)` is a subset of `['replaySpeed','autoplay','panelAt','prizeReminders']` (`web/src/lib/playback.svelte.js:18-44`, `web/src/lib/store.svelte.js:70-121`), `sessionStorage` is empty, and there are no cookies.

**Complexity**: Medium
**Token estimate**: ~25k tokens
**Agent strategy**: Single agent, sequential. Board specs run one at a time (`workers: 1`); the second server has its own game, so no interference.

### Task: Switch visitor counting on for tcgreplay.app

**What the person does** (maintainer, after "Count website and board pages on the hosted playmat" is deployed and the privacy-terms plan's privacy page is live):
1. Open `https://tcgreplay.app/privacy` in a private window and confirm the visitor-counting section is there. If it is not, stop: counting must not start.
2. Confirm the playmat's start-up log in DigitalOcean Insights says `visitor analytics: off (no TCGREPLAY_WEB_ANALYTICS_TOKEN)`, which shows the code is deployed.
3. In the Cloudflare dashboard → Web Analytics → Add a site, enter `tcgreplay.app`.
   - If the zone is proxied and Cloudflare offers automatic setup, choose **Enable with manual snippet installation**, or otherwise make sure automatic injection is off, so the page carries only the playmat's beacon.
   - Copy the token from the snippet's `data-cf-beacon`.
4. In the App Platform control panel → the app → Settings → playmat → Environment Variables, set `TCGREPLAY_WEB_ANALYTICS_TOKEN` to the token and save. This restarts the playmat. If the declaration is not yet on the platform, applying `.do/app.yaml` with `doctl apps update` first adds it.
5. Check that the start-up log says `visitor analytics: on for tcgreplay.app`, and that the page source of `https://tcgreplay.app/` and of a signed-in `/board` contains exactly one `static.cloudflareinsights.com/beacon.min.js` tag with `"spa":false`.

**How the result is checked:** the log line and page source in step 5. The dashboard-side acceptance checks are in the Testing Approach's manual items.

**To switch counting off later:** clear the variable in the control panel (restart, no release). The log then says counting is off.


## Testing Strategy

Per task:
- **Add the visitor analytics package and the privacy page check.**
  - Go table tests over `Setup`: each off reason, the on status, the token never echoed, malformed tokens.
  - `httptest` tests over `Inject`: one snippet before the last `</body>`, length fixed, host match by hostname, and byte-for-byte pass-through for `HEAD`, 302, 404, 503, 206, non-HTML and flushed responses.
  - `internal/site` tests for `DescribesVisitorCounting` with the `fsys` swap.
- **Count website and board pages on the hosted playmat.** Hosted end-to-end tests through `startHostedWith`, with the privacy hook swapped:
  - website pages and the signed-in board carry one snippet for the public host;
  - other hosts, a signed-out `/board` redirect, assets, card lookups, sign-in metadata, `/healthz` and `/admin` never do;
  - every off case leaves pages byte-identical to `site.Handler()`'s;
  - the status log lines carry no token.
  - The existing `TestHostedRoutes` byte-for-byte comparisons keep passing unchanged, which proves the default is silent.
- **Check counting in the website's browser suite.**
  - The default server's whole-site checks are unchanged in strictness.
  - A new analytics spec on a second server, with Cloudflare routed to a stub or an abort, checks one beacon per crawled page with `spa:false`, no other host but the stubbed beacon, no cookies or storage, and that blocking the beacon changes nothing (content, navigation, copy buttons, no page errors).
- **Check counting in the board's browser suite.** A new analytics spec on a second board server, at phone, tablet and desktop, checks:
  - one beacon with `spa:false`;
  - no second beacon after card zooms, branch switches and the phone action sheet;
  - the board works with the beacon blocked;
  - only the four known storage keys, and no cookies.
- **Switch visitor counting on for tcgreplay.app.** No automated tests. The person checks the log line and the page source. The dashboard acceptance checks, the success metrics and the live devtools checks are manual items, flagged in plan.md's Testing Approach as captured in the implementation test plan.

No test contacts Cloudflare. Real-beacon behaviour (cookies, storage, request contents) and dashboard numbers are manual checks on the live site.

## Project References

- Spec: `20261111132045-1d1d32f6-visitor-analytics` (epic `20261111093348-e1b5c2b4-public-launch`).
- Plans built on:
  - `20261112083238-29247b36-privacy-terms-disclaimer`: the `data-retention="visits"` marker; `Base.astro` ownership; the third-party check update expected here.
  - `20261111093353-8ff3589f-domain-move`: the public URL and the moved-host guard.
  - `20261112090330-4ac2ea19-observability`: no email in any log line; shared edits to hosted start-up and `.do/app.yaml`.
  - `20261112091756-c5b6d1fd-launch-docs`: new docs pages are crawled.
- Design documents: none.
- Knowledge:
  - `gotchas/astro-site-build.md`;
  - `gotchas/playwright-touch-tests.md`;
  - `gotchas/live-site-cf-bm-cookie.md` (written during this plan);
  - `conventions/skills-text-and-packaging.md`.
- Repo root: `tcgreplay` at `/home/nicj/code/github.com/nicholasjackson/tcgreplay`.
- External: the Cloudflare Web Analytics docs pages (get started, SPA, FAQ, high-level metrics, data origin and collection), listed in `research.md`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Every agent task here is Medium (~25-35k) and runs as a single sequential agent. The two browser-suite tasks can run in parallel with each other once the package task is done; they touch disjoint files. The hosted wiring task also depends only on the package task.

## Migration Notes

- No data migration.
- Deploying the code changes nothing while `TCGREPLAY_WEB_ANALYTICS_TOKEN` is unset, and the start-up log says counting is off.
- The `.do/app.yaml` declaration reaches App Platform only when a person applies the app spec, or adds the variable in the control panel.
- Merge `.do/app.yaml` and `cmd/tcgreplay/hosted.go` edits onto the domain-move and observability changes, whichever lands first. The main tree has uncommitted `.do/app.yaml` domain edits.
- To switch counting off later, clear the variable (restart only).

## Performance Considerations

- The wrapper buffers only successful `GET` HTML responses on the two counted routes. Website pages and the board's `index.html` are a few kilobytes, so the copy is negligible. Assets, the websocket, card lookups and MCP are not wrapped at all.
- The beacon is a deferred external script, so it never blocks the page's render. Blocking it costs the visitor nothing.
- The privacy-page check runs once at start-up.
