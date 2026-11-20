---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Plan: 20261111132045-1d1d32f6-visitor-analytics

<!-- Metadata -->
<!-- Created: 2026-11-12T10:11:55Z -->
<!-- Commit: 6ecc1aa -->
<!-- Branch: main -->
<!-- Repository: tcgreplay (/home/nicj/code/github.com/nicholasjackson/tcgreplay) -->

## Overview

When TCG Replay launches publicly, the maintainer needs to know whether anyone is using it. This plan has the hosted playmat add Cloudflare's cookieless Web Analytics to every public website page and to the board page. The maintainer can then read visits, page views, top pages, referrers, countries and devices in Cloudflare's dashboard, and each board session counts once. Visitors get no cookies, no consent banner and no record of who they are, and pages work the same when analytics are blocked. Counting stays off in local runs and tests, can be switched off by configuration alone, and cannot start until the playmat's own privacy page describes it.

## Conventions

- **Never write "Claude" or "Nic " in an `internal/guides` guide; never exclude `internal/guides` from a build context** — this work touches the hosted settings comment in the `Dockerfile` and must leave the `site` stage's `COPY internal/guides` and `.dockerignore` untouched; no guide text changes.

## Architecture & Design Decisions

All of the work lands in the one registered repo, `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`), and almost all of it in Go. The website (Astro, `site/`) and the board (Vite and Svelte, `web/`) are built separately and share no page shell. But on the live playmat both are served by the same hosted router (`cmd/tcgreplay/hosted.go`, `hostedHandler`): the website on every public path, and the board page at `GET /board` behind sign-in. That router is where Cloudflare's script is added, once, for both. A new small package, `internal/webanalytics`, owns the snippet and an HTTP wrapper. Hosted mode wraps only the website routes and the `/board` page route with it. Sign-in, admin, "Not your game" and refusal pages are not wrapped. The wrapper inserts Cloudflare's manual snippet before `</body>`, and only into a successful `GET` HTML response. The snippet is `<script defer src="https://static.cloudflareinsights.com/beacon.min.js" data-cf-beacon='{"token":"…","spa":false}'></script>`. Redirects (signed-out board loads go to Google), 404 pages, 503 refusals, `HEAD` and range responses pass through untouched. No front-end source changes, and the shared layout `Base.astro`, which belongs to the privacy-terms work, is left alone.

**Counting is off unless four things hold.** Running the playmat locally, in desktop mode or in the test suites never goes through the hosted router, so it never counts. On the hosted playmat, counting is on only when all of these hold:
- **A token is set.** A new setting, `TCGREPLAY_WEB_ANALYTICS_TOKEN` (flag `-web-analytics-token`), names the Cloudflare Web Analytics site token. It follows the existing pattern of a flag whose default is its `TCGREPLAY_*` variable.
- **The token is well formed.** It is 16 to 64 ASCII letters and digits. This is checked once at start, and it also guarantees nothing written into the page can break out of the attribute.
- **The playmat's own privacy page describes counting.** At start-up the playmat checks its embedded privacy page for the `data-retention="visits"` marker that the privacy-terms work puts on its visitor-counting section. A binary whose privacy page does not describe counting cannot count, whatever the setting says. This makes the spec's "privacy page first" hold by construction, not only by the order a person does things in.
- **The request is for the public address.** The wrapper injects only when the request's host matches `TCGREPLAY_PUBLIC_URL`'s host, ignoring case and port. The App Platform default address, the old `tcgreplay.demo.gs` alias (which the domain move redirects anyway) and a local `-hosted` run therefore add nothing to the dashboard. Cloudflare's own hostname matching backs this up.

If anything is missing, the playmat still starts and says in its start-up log that visitor analytics are off and why. A missing token, a malformed token and a privacy page that does not describe counting each have their own reason. It follows the project's warn-and-carry-on pattern for optional settings, so an analytics typo never takes the playmat down. Clearing the setting in the App Platform control panel restarts the playmat with counting off: configuration alone, no release. `.do/app.yaml` declares the setting without a value, as it does `TCGREPLAY_ADMINS`. A person sets it once the privacy page is live, because app spec and control panel changes take effect only when a person applies them.

**One board session is one page view.** The board changes views without loading pages, and the phone action sheet pushes a history entry each time it opens (`web/src/components/ActionSheet.svelte:37`). Cloudflare's single-page-app mode patches `pushState` and would count those as page views. So the snippet turns it off (`"spa": false`), which Cloudflare only allows with the manual snippet. The board is then counted exactly once per page load. In the dashboard a board session is a page view of `/board`. It is not necessarily a Cloudflare "visit": Cloudflare counts a visit only when the referrer is another site, so a player who comes from the website's home page makes a page view but no new visit. The website keeps its plain page loads, one page view each. Neither page sends a content security policy today, so none needs changing. The design doc records what a future one must allow (`script-src https://static.cloudflareinsights.com`, `connect-src https://cloudflareinsights.com`).

**Why this direction.** Cloudflare's automatic edge injection needs no code, but it cannot turn single-page-app tracking off. It also cannot be gated on the privacy page, and it is unclear whether the maintainer's zone proxies the site at all. Writing the snippet into both builds would duplicate it in two shells, pull the token into two Node builds (so local builds and test runs would count unless gated twice), need a rebuild to switch off, and edit the privacy-terms work's shared layout. Options on `site.Handler` and the board server would leave silence one forgotten option away, because the test servers and desktop mode build those handlers directly. The evidence for each is in `research.md#alternatives-considered-and-rejected`.

**Tests never reach Cloudflare.** The site and board test servers gain an opt-in analytics flag. Each browser suite starts a second instance with it, and every test routes both Cloudflare hosts either to a stub or to a block. The suites can therefore show, on the real built pages, that the snippet is present only when asked for, that blocking it changes nothing, and that nothing new is stored, all without sending an analytics request. The site suite's "nothing fetched from other hosts" check is updated as the privacy-terms plan expects. It still requires that a default run asks no other host for anything. With analytics on, the only other host allowed is Cloudflare's beacon address, which the test stubs.

## Component Breakdown

- **Visitor analytics package (new).**
  - Owns everything about Cloudflare Web Analytics in one place: the snippet (beacon address, token, single-page-app tracking off), the token check, and an HTTP wrapper that adds the snippet to a page.
  - The wrapper adds the snippet only to a successful `GET` HTML response for the public host. Everything else passes through byte for byte.
  - It also decides whether counting is on from three inputs (the token, whether the privacy page describes counting, the public host) and gives a one-line reason for the start-up log.
  - It is new because the hosted router and the two test servers all need the same snippet and the same rules. Nothing existing is shared by those three, and the website and board builds cannot own it without two copies.
- **Website package (changed, small).**
  - Gains one question it can answer about its own embedded pages: does the privacy page carry the visitor-counting marker?
  - Otherwise unchanged. It still sets no cookie and is still served plain by the test server unless analytics are asked for.
- **Hosted router and start-up (changed).**
  - Hosted mode reads the new token setting and asks the visitor analytics package whether counting is on, using the public address and the website package's privacy check.
  - It wraps only the website routes and the board page route with the result, and says in its start-up log whether visitor analytics are on, or off and why.
  - Sign-in, admin, assistant connection, the board's files and live connection, and refusal pages are untouched.
  - The domain move's moved-host guard still sits in front of the whole router, so old-address requests are redirected before they reach the wrapper.
- **Website test server (changed).**
  - Gains an opt-in analytics token flag. With it, the server wraps the website the way hosted mode does, gated on the same privacy check, for its own loopback host. Without it, it is unchanged.
- **Board test server (changed).**
  - Gains the same opt-in flag, wrapping the board page. It has no website, so it treats the privacy check as passed and says so in its code.
- **Website browser suite (changed).**
  - Runs a second test server with analytics on. Its whole-site checks stay as they are for the default server; the third-party check is reworded to say a default run asks no other host for anything.
  - A new analytics spec checks the pages with analytics on, with Cloudflare's hosts stubbed or blocked:
    - every crawled page carries exactly one snippet, with single-page-app tracking off;
    - the only other host asked is the stubbed beacon address;
    - there are no cookies or storage;
    - blocking the beacon changes nothing a visitor sees.
- **Board browser suite (changed).**
  - Runs a second board test server with analytics on. A new spec checks, with Cloudflare blocked or stubbed:
    - the board page carries one snippet with single-page-app tracking off;
    - the board loads and plays normally;
    - opening the phone action sheet and card zooms loads no second beacon;
    - nothing beyond the four existing settings is stored.
- **App Platform definition (changed).** Declares the token setting without a value, with a comment on what it does, that it is public (it appears in every page), that it must be set only once the privacy page describing counting is live, and that clearing it stops counting.
- **Container build notes and design doc (changed).**
  - The Dockerfile's settings comment lists the new setting.
  - The design doc gains a visitor-counting paragraph and a row in its settings table:
    - what is counted and where;
    - the four conditions for counting;
    - how to switch counting off;
    - what a future content security policy must allow.
  - It also gains an operator step: create the Cloudflare site with the manual snippet, then set the token.
- **Maintainer's Cloudflare and App Platform setup (a person's task).**
  - Confirm the privacy page's visitor-counting section is live.
  - Add tcgreplay.app as a Web Analytics site using the manual snippet, making sure automatic injection is off so the page carries one beacon, and take its token.
  - Set the token on the playmat.
  - Check the dashboard against the spec's acceptance criteria.

## Data Structures & Interfaces

No persisted data and no wire format of the playmat's own changes. The plan adds one setting, one small Go package interface, one question on the website package, and one page contract read from the privacy-terms work.

**The setting.** `TCGREPLAY_WEB_ANALYTICS_TOKEN` (flag `-web-analytics-token`, hosted mode only) is the Cloudflare Web Analytics site token. Empty means no counting. It is public by nature, because it appears in every counted page, so it is a plain setting, not a secret.

**Visitor analytics package (`internal/webanalytics`).** It decides whether counting is on, says why, and supplies the wrapper:

```go
// Settings are what decide whether counting is on.
type Settings struct {
    Token             string // the Cloudflare site token; "" = off
    Host              string // the public host the snippet is added for (from the public URL)
    PrivacyDescribes  bool   // the embedded privacy page describes visitor counting
}

// Setup returns the wrapper to apply to counted pages (identity when off)
// and a one-line status for the start-up log, e.g.
// "visitor analytics: on for tcgreplay.app" or
// "visitor analytics: off (no TCGREPLAY_WEB_ANALYTICS_TOKEN)".
func Setup(s Settings) (wrap func(http.Handler) http.Handler, status string)

// Snippet is Cloudflare's manual beacon tag for token, with SPA tracking off.
func Snippet(token string) string

const BeaconURL = "https://static.cloudflareinsights.com/beacon.min.js"
```

The tag `Snippet` returns, inserted before `</body>`:

```html
<script defer src="https://static.cloudflareinsights.com/beacon.min.js" data-cf-beacon='{"token":"<token>","spa":false}'></script>
```

**Website package question.**

```go
// DescribesVisitorCounting reports whether the embedded privacy page
// carries the visitor-counting marker (data-retention="visits").
func DescribesVisitorCounting() bool
```

**Page contract read from the privacy-terms work.** The privacy page's visitor-counting section carries Cloudflare's retention figure in an element marked `data-retention="visits"`, as the privacy-terms plan defines. This plan only reads that marker. It never writes it.

**Hosted router signature.** `hostedHandler` gains one argument, the wrapper for counted pages (`func(http.Handler) http.Handler`). Its callers pass the identity when counting is off.

**Test server flags.** Both the website test server and the board test server gain `-analytics-token <token>`. When it is unset they behave exactly as today.

## Implementation Detail

**A new pattern: serve-time page decoration in hosted mode.** Until now the playmat has served its pages exactly as the builds produced them. This work adds the first wrapper that changes a page body on its way out.
- **How it changes the body.** It captures only responses that are HTML, successful and answers to `GET`, adds one tag before the closing body tag, and fixes up the length. Everything else streams through untouched: assets, redirects, error pages, `HEAD`, range requests and anything not HTML. So the board's files, live connection and the website's hashed assets never pay for it.
- **Where it applies.** The wrapper is applied by hosted mode at the two routes that serve pages people read, never inside the website or board handlers themselves. A developer reading the website package or the board server finds no analytics code. A developer reading the hosted path table sees exactly which pages are counted.

**Off by default, on by deliberate configuration.** The gating follows the hosted settings pattern already in use:
- a flag whose default is its `TCGREPLAY_*` variable;
- one decision made at start-up;
- a line in the start-up log;
- warn and carry on when an optional setting is unusable, as partial card-image settings already do.

The decision lives in the new package as a pure function of its inputs, so every combination is unit-testable without a server. Hosted mode only gathers the inputs (the setting, the public host, the website's privacy check) and applies the result.

**The privacy page as a precondition the binary checks itself.** The website package answers whether its embedded privacy page carries the visitor-counting marker. That ties the switch-on to the privacy-terms work's page contract, the same `data-retention` marker its browser suite pins, rather than to a date or a person remembering. Because the website is embedded at build time, the check is about the very pages this binary serves: a build without the section cannot count, and a build with it counts once the token is set. The hosted route tests swap the check through a package-level hook, because the Go tests embed whatever website happens to be built.

**Test servers opt in; suites never call out.** The website and board test servers keep their current behaviour and gain an opt-in flag that applies the same wrapper. That way the browser suites see exactly the snippet hosted mode would add, from the same code. Each suite starts a second server instance on its own port and points only its analytics spec at it, so every existing spec and the shared board game are untouched. Every analytics browser test routes Cloudflare's two hosts first: the beacon address to a tiny stub, or to an abort when testing blocked analytics, and the reporting address to an abort. A missing route therefore shows up as a test failure, not as traffic to Cloudflare.

**Existing patterns followed.**
- The test servers stay loopback-only.
- The website's whole-site crawl remains the source of the page list.
- The board's storage check reuses the four known settings keys.
- `.do/app.yaml` declares an optional, hand-set value the way it declares the admin list.
- The design doc gains a paragraph and a settings-table row in its existing style.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Privacy, terms and disclaimer plan (`20261112083238-29247b36-privacy-terms-disclaimer`), must land and be live first.**
  - Its privacy page gains the visitor-counting section, with Cloudflare's retention in an element marked `data-retention="visits"`. This plan's start-up check reads that marker, so counting cannot start in a build without it.
  - The person's switch-on task also checks that the section is live on `https://tcgreplay.app/privacy` before setting the token.
  - The code here can merge before or after it. Counting simply stays off, with the reason in the log, until a build carries the section and the token is set. The browser suite's analytics spec needs the section in the built site, though, so in practice this lands after it.
  - It also owns the shared layout `Base.astro`, which this plan does not touch. It expects this plan to update the site suite's "nothing fetched from other hosts" check, which this plan does.
- **Domain move plan (`20261111093353-8ff3589f-domain-move`), should land first.**
  - It makes `TCGREPLAY_PUBLIC_URL` `https://tcgreplay.app`, whose host is the only one the snippet is added for.
  - Its moved-host guard wraps the whole hosted router and redirects `tcgreplay.demo.gs` before this plan's wrapper is reached.
  - It also edits the hosted router file and `.do/app.yaml`, so merge onto whichever lands first. The edits touch different lines: one new argument to the router and one new env declaration.
  - This plan names no address in website text, so its whole-site address check is unaffected.
- **Observability plan (`20261112090330-4ac2ea19-observability`), no ordering dependency.**
  - It edits hosted start-up and `.do/app.yaml` too; merge onto whichever lands first.
  - This plan's start-up log line carries no email, account or token, in keeping with its rule that no log line carries an email.
  - Usage inside the board is its business, not this plan's (spec non-goal).
- **Launch docs plan (`20261112091756-c5b6d1fd-launch-docs`), no ordering dependency.** Its new docs pages are crawled like every other page, so they get the snippet automatically and pass the analytics spec without change.
- **Cloudflare Web Analytics (external service).**
  - Provides the beacon script at `https://static.cloudflareinsights.com/beacon.min.js`, the reporting address `https://cloudflareinsights.com/cdn-cgi/rum`, the site token and the dashboard.
  - A person creates the site in the maintainer's Cloudflare account, using the manual snippet with automatic injection off.
  - No account change is made by code.
- **DigitalOcean App Platform (external service).** Runs the playmat. The token is set on the playmat component in the control panel by a person, and clearing it there turns counting off. The declaration in `.do/app.yaml` takes effect only when a person applies the app spec.
- **Hosted router, hosted settings and start-up log (existing, `cmd/tcgreplay`).** Gain the setting, the decision and the wrapping of two routes.
- **Website package (existing).** Gains the privacy-page question. Its handler is unchanged.
- **Board server and board front door (existing).** Unchanged. The board page is wrapped from outside.
- **Website and board test servers and browser suites (existing).** Gain an opt-in flag, a second server instance and one analytics spec each.
- **Go standard library, Playwright (existing toolchain).** No new packages or libraries.
- **Knowledge entries relied on:**
  - `gotchas/astro-site-build.md`: anything that embeds the site embeds whatever was built, so the privacy check and the site suite need the website built first.
  - `gotchas/playwright-touch-tests.md`: the board suite's touch profiles run in Chromium.
  - `gotchas/live-site-cf-bm-cookie.md`, written during this plan: the live site's `__cf_bm` cookie comes from Cloudflare's edge, so the live devtools check must not mistake it for analytics.
  - `conventions/skills-text-and-packaging.md`: guides packaging in the container build.

## Testing Approach

The tests extend the project's existing suites in their own style, with no new framework:
- Go unit tests for the new package.
- Go hosted route tests.
- The website's Playwright suite (laptop and phone, against the Go site test server).
- The board's Playwright suite (phone, tablet and desktop, against the board test server).

No test anywhere contacts Cloudflare. Unit and route tests make no network calls. Every browser test that switches analytics on first routes Cloudflare's beacon address to a stub, or blocks it, and blocks its reporting address.

**Unit tests (visitor analytics package; most coverage, because every privacy and "only the live playmat" guarantee rests on its decision and its wrapper):**
- **The decision.** Counting is on only with a well-formed token, a privacy page that describes counting, and a public host. Each missing input gives "off" with its own reason, and an on status never contains the token. Tokens with quotes, angle brackets, spaces or the wrong length are refused.
- **The snippet.** It is exactly Cloudflare's manual tag for the beacon address, with the token and `"spa":false`, and it can never carry anything but letters and digits from the token.
- **The wrapper.** On a `GET` 200 HTML response for the public host (any case, any port), exactly one snippet goes before the last closing body tag, or at the end when there is none, and the length header matches the new body. Each of these passes through byte for byte, headers included:
  - any other host;
  - `HEAD`;
  - redirects, 404, 503 and range (206) responses;
  - CSS, JS, images and JSON;
  - a response that is already streaming.

**Website package test.** The privacy check answers yes for an embedded privacy page carrying the `data-retention="visits"` marker, and no for one without it or with no privacy page (the placeholder).

**Hosted route tests (end to end through the hosted router):**
- With the token set and the privacy check passing, signed-out website pages (home, a docs page, privacy) carry one snippet for the public host, and none for another host.
- A signed-in board page load at `/board` carries one snippet.
- A signed-out `/board` still redirects to sign-in with no snippet in the response.
- The board's files, live connection, card lookups, the assistant connection, sign-in pages and the admin page never carry it.
- With no token, a malformed token or a failing privacy check, every page is byte-for-byte what the website or board itself serves, as the route test already compares today. The start-up status names the reason.
- No response sets a cookie because of analytics.

**Website browser suite:**
- **Default server (existing checks, kept).** The whole-site crawl's "sets no cookie and asks no other host for anything" check still passes with no exception. Its wording now says it covers a run without the analytics token. No page carries the snippet.
- **Analytics server (new spec).**
  - Every crawled page carries exactly one beacon tag with single-page-app tracking off.
  - The only request to another host is the stubbed beacon script. Its address and the page's beacon tag carry nothing but the beacon address, the token and the single-page-app switch.
  - No cookie and no `localStorage` or `sessionStorage` entry exists after visiting every page.
  - **Blocked analytics.** With the beacon blocked, every page loads, shows its content and navigation, raises no page error, and copy buttons still work.

**Board browser suite (analytics board server, new spec):**
- The board page carries one beacon tag with single-page-app tracking off.
- With the beacon stubbed, opening a card zoom, switching branches and (on the phone profile) opening and closing the action sheet loads no second beacon. This is the browser-side half of "one board session, one board visit".
- With the beacon blocked, the board renders the fixed game, a card zoom opens, a move can be made, and no page error or error message appears.
- After those interactions, browser storage holds only the four existing settings keys, and there are no cookies.

**Deliberate gaps:**
- No automated test loads Cloudflare's real beacon or reads the real dashboard, because that would send analytics traffic. The real beacon's cookie and storage behaviour and the dashboard's numbers are manual checks on the live site.
- The browser suites test the test servers' wrapping, not hosted mode's. Both use the same package, and the hosted route tests cover hosted mode's routes and gating.

**Success metrics:**
- "A week after launch, the maintainer can answer 'how many visits did the site get, and how many board sessions, this week?' from the dashboard alone": **Manual — captured in the implementation test plan.** Board sessions are read as page views of `/board`. The automated tests guarantee one snippet per page load and no extra counting on the board.
- "The dashboard shows only tcgreplay.app traffic": **Manual — captured in the implementation test plan.** The behaviour behind it is covered by behavioural tests: no snippet for any other host, none from local runs, desktop mode or test servers without the flag.

**Manual reviews and checks:**
- Privacy page first: before the token is set, `https://tcgreplay.app/privacy` shows the visitor-counting section, and the start-up log said counting was off until then. **Manual — captured in the implementation test plan.**
- Website page views appear: within 30 minutes of visiting the home page and a docs page at `https://tcgreplay.app`, both appear in the dashboard's page views. **Manual — captured in the implementation test plan.**
- One board session, one board visit: signing in and moving around the board (card zooms, branch switches) shows as one `/board` page view within 30 minutes. **Manual — captured in the implementation test plan.**
- Totals, top pages and breakdowns: the last 7 days view shows visit and page-view totals, top pages, and referrer, country and device type. **Manual — captured in the implementation test plan.**
- No browser storage, no identity, on the live site: devtools shows no cookie or storage entry from analytics on the website or the board, and no analytics request carries the signed-in email, account, game or card data. Cloudflare's `__cf_bm` cookie comes from the hosting edge, not analytics. **Manual — captured in the implementation test plan.**
- Blocked analytics change nothing, live: with a content blocker on, the website and the board load and work with no error shown. **Manual — captured in the implementation test plan.**
- No banner: no consent banner or analytics prompt on any page. **Manual — captured in the implementation test plan.**
- Local and test runs are silent: after running the playmat locally and the test suites, the dashboard shows no `localhost`, `127.0.0.1` or other-host traffic. **Manual — captured in the implementation test plan.**
- One beacon only: the live page source on tcgreplay.app carries exactly one Cloudflare beacon tag, ours, with `"spa":false`, so Cloudflare's automatic injection is not adding a second. **Manual — captured in the implementation test plan.**

## Milestones & Tasks

### Milestone 1: The live playmat can count website and board visits, and does nothing until it is told to

**What changes**: The hosted playmat learns to add Cloudflare's cookieless visitor counting to every website page and to the board page. It only does so when the maintainer has given it a Cloudflare site token, when its own privacy page describes the counting, and only for visitors on the public address. Until all three hold, nothing changes for anyone, and the playmat's start-up log says counting is off and why. Running the playmat locally, in the desktop app or in tests never counts. The board is counted once each time it is opened, however much the player moves around it. The deployment definition, the container's settings notes and the design doc explain the new setting, how to switch counting on and off, and that the privacy page comes first.

**Validation point**: The Go tests pass. With a token and a privacy page that describes counting, website pages and a signed-in board page for the public address carry exactly one Cloudflare snippet with single-page-app tracking off. Other hosts, sign-in redirects, the board's files, sign-in pages and the admin page carry none. With no token, a malformed one, or a privacy page without the visitor-counting section, every page is exactly what it was, and the start-up log gives the reason.

#### - [x] Task: Add the visitor analytics package and the privacy page check
**Id:** 11c2ba66-ef41-4dfc-8cf0-93e1aa244855
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds a small package that owns Cloudflare Web Analytics for the playmat: the beacon tag, the token check, the decision whether counting is on (with a reason for the log), and a wrapper that adds the tag to a successful HTML page for the public address only. The website package learns to say whether its own embedded privacy page describes visitor counting, so counting can never start in a build whose privacy page does not mention it. Nothing uses either yet, so nothing changes for anyone.

*Technical detail:* [context.md#task-add-the-visitor-analytics-package-and-the-privacy-page-check](./context.md#task-add-the-visitor-analytics-package-and-the-privacy-page-check)

**Acceptance criteria**:
- [x] Counting is decided "on" only with a well-formed token, a privacy page that describes counting and a public address, and each missing piece gives "off" with its own plain reason that never shows the token.
- [x] A token holding anything other than 16 to 64 letters and digits is refused, so nothing unexpected can reach a page.
- [x] The tag added is Cloudflare's manual beacon tag with the token and single-page-app tracking off, placed once before the end of the page body.
- [x] Only successful page loads for the public address are changed; other hosts, redirects, error pages, assets and partial responses are passed on exactly as they were.
- [x] The website reports that its privacy page describes counting only when the page carries the visitor-counting marker the privacy work defines.

#### - [x] Task: Count website and board pages on the hosted playmat
**Id:** b32a7984-790c-4338-9628-e913f8a99f34
**Repo:** tcgreplay
**Depends on:**
- 11c2ba66-ef41-4dfc-8cf0-93e1aa244855 — Add the visitor analytics package and the privacy page check
**Execution:** agent

Gives the hosted playmat a new setting for the Cloudflare site token and applies the counting wrapper to the website and to the board page only. Sign-in, the admin page, the assistant connection and the board's own files stay untouched. The start-up log says whether visitor analytics are on, or off and why. The deployment definition declares the setting without a value. The container's settings notes and the design doc explain what is counted, the conditions for counting, how to switch it off, and that the privacy page comes first.

*Technical detail:* [context.md#task-count-website-and-board-pages-on-the-hosted-playmat](./context.md#task-count-website-and-board-pages-on-the-hosted-playmat)

**Acceptance criteria**:
- [x] With a token and a privacy page that describes counting, every website page and a signed-in board page on the public address carry exactly one Cloudflare beacon, and the board page is counted once per load.
- [x] A signed-out board load still goes straight to sign-in, and sign-in pages, the admin page, the assistant connection, the board's files and pages on any other address never carry the beacon.
- [x] With no token, a malformed token or a privacy page without the visitor-counting section, every page is exactly as it was before, and the start-up log names the reason.
- [x] No page sets a cookie because of counting, and the start-up log never shows the token, an email or an account.
- [x] The deployment definition, the container's settings notes and the design doc describe the setting, that it must be set only once the privacy page describing counting is live, and that clearing it stops counting.
- [x] Running the playmat locally or in the desktop app adds no beacon to any page.

### Milestone 2: The browser suites show that counting is invisible to visitors

**What changes**: The website's and the board's browser suites gain checks that run the real built pages with counting switched on, without ever contacting Cloudflare. They show that each page carries one beacon and nothing else from another site. They show that nothing is stored in the browser and no cookie is set, and that a visitor who blocks analytics sees and can do exactly the same as before. On the board, moving around (card zooms, branch switches, the phone action sheet) loads no second beacon. The site's existing "nothing fetched from other hosts" check is reworded to cover a run without the token, where it still allows nothing at all. This milestone changes nothing visitors see. It is worth its own milestone because it is the evidence behind the spec's "nothing identifies a visitor" and "blocking changes nothing" promises.

**Validation point**: Both browser suites pass, including the new analytics specs, at the website's laptop and phone sizes and the board's phone, tablet and desktop sizes. The existing whole-site checks still pass unchanged against the default server.

#### - [x] Task: Check counting in the website's browser suite
**Id:** 456866d0-d347-49a0-8404-b5e4eb0bc7d9
**Repo:** tcgreplay
**Depends on:**
- 11c2ba66-ef41-4dfc-8cf0-93e1aa244855 — Add the visitor analytics package and the privacy page check
**Execution:** agent

Lets the website's test server switch counting on when asked. The browser suite runs a second copy of the server with it on, and stubs or blocks Cloudflare so no test ever contacts it. A new spec walks every page and shows each carries one beacon and nothing else from another site, that nothing is stored and no cookie is set, and that blocking the beacon changes nothing a visitor sees. The existing "nothing fetched from other hosts" check is reworded to cover a run without the token, where it still allows nothing at all.

*Technical detail:* [context.md#task-check-counting-in-the-websites-browser-suite](./context.md#task-check-counting-in-the-websites-browser-suite)

**Acceptance criteria**:
- [x] Without the token, every website page still asks no other host for anything, sets no cookie and carries no beacon, as before.
- [x] With counting on, every website page carries exactly one beacon with single-page-app tracking off, and the only other address asked is Cloudflare's beacon script, which the test stubs.
- [x] With counting on, no cookie, local storage or session storage entry exists after visiting every page.
- [x] With the beacon blocked, every page loads with its content and navigation, raises no error, and its copy buttons still work.
- [x] No test in the suite sends anything to Cloudflare.

#### - [x] Task: Check counting in the board's browser suite
**Id:** b55d6c1e-1a4a-42a3-8887-70fb05eebc4d
**Repo:** tcgreplay
**Depends on:**
- 11c2ba66-ef41-4dfc-8cf0-93e1aa244855 — Add the visitor analytics package and the privacy page check
**Execution:** agent

Lets the board's test server switch counting on when asked. The board's browser suite runs a second copy with it on, with Cloudflare stubbed or blocked. A new spec shows the board page carries one beacon, that moving around the board loads no second one, and that the board plays normally when analytics are blocked. It also shows nothing beyond the board's four existing settings is stored. The existing board specs and their shared game are untouched.

*Technical detail:* [context.md#task-check-counting-in-the-boards-browser-suite](./context.md#task-check-counting-in-the-boards-browser-suite)

**Acceptance criteria**:
- [x] At phone, tablet and desktop sizes, the board page carries exactly one beacon with single-page-app tracking off.
- [x] Opening card zooms, switching branches and, on a phone, opening and closing the action sheet load no second beacon.
- [x] With the beacon blocked, the board shows the game, opens a card zoom and accepts a move, with no error shown or raised.
- [x] After moving around, the browser holds only the board's four existing settings and no cookie.
- [x] No test in the suite sends anything to Cloudflare.

### Milestone 3: Visitor counting is live for tcgreplay.app

**What changes**: Once the privacy page describing visitor counting is live, the maintainer creates tcgreplay.app as a Cloudflare Web Analytics site and gives its token to the playmat. Visits to the website and board sessions then appear in the Cloudflare dashboard, with visits, page views, top pages, referrers, countries and devices for any time range. Visitors see no banner and nothing else changes for them. Clearing the token switches counting off again without a release.

**Validation point**: The start-up log says visitor analytics are on for tcgreplay.app. The live page source carries exactly one beacon. Within 30 minutes, the home page, a docs page and a `/board` session show in the dashboard. The live devtools checks show no cookie or storage from analytics and no identity in its requests.

#### - [x] Task: Switch visitor counting on for tcgreplay.app
**Id:** b47cb586-3bb8-442e-b5bd-ab4539615cf6
**Repo:** tcgreplay
**Depends on:**
- b32a7984-790c-4338-9628-e913f8a99f34 — Count website and board pages on the hosted playmat
**Execution:** human — needs the maintainer's Cloudflare dashboard and the App Platform control panel, and must wait for the privacy page to be live

Once the counting code is deployed and the privacy page describing visitor counting is live at tcgreplay.app, the maintainer adds tcgreplay.app as a Cloudflare Web Analytics site using the manual snippet. They make sure Cloudflare's automatic injection is off, so each page carries only the playmat's beacon. They then set the site token on the playmat in the App Platform control panel, and the playmat restarts with counting on.

*Technical detail:* [context.md#task-switch-visitor-counting-on-for-tcgreplayapp](./context.md#task-switch-visitor-counting-on-for-tcgreplayapp)

**Acceptance criteria**:
- [ ] The privacy page at tcgreplay.app described visitor counting before the token was set.
- [ ] tcgreplay.app exists as a Cloudflare Web Analytics site set up for the manual snippet, with automatic injection off.
- [ ] The playmat's start-up log says visitor analytics are on for tcgreplay.app, and the live page source carries exactly one Cloudflare beacon.

## Open Questions

- **Does App Platform hand the playmat the public host (`tcgreplay.app`) in the request's `Host`?**
  - It depends on App Platform's ingress behind Cloudflare, which can only be observed on the live deployment. The domain-move plan's moved-host guard relies on the same behaviour.
  - When it matters: the start-up log says counting is on for tcgreplay.app but the live page source carries no beacon.
  - What to do: STOP and ask the user before changing code. A candidate fix is matching `X-Forwarded-Host` as well, but that changes which requests count, so it is the user's call.
- **Does Cloudflare's real beacon set a cookie or write browser storage on tcgreplay.app?**
  - It depends on Cloudflare's current beacon, which the tests never load. Cloudflare describes Web Analytics as cookieless, but the pages fetched while planning do not say so outright.
  - When it matters: the live devtools check.
  - What to do: if anything from `cloudflareinsights.com` appears in cookies, local storage or session storage, STOP and ask the user. It contradicts the spec's cookieless constraint, and the privacy-terms plan treats the same finding as a STOP. Cloudflare's `__cf_bm` cookie comes from the hosting edge, not from analytics (`gotchas/live-site-cf-bm-cookie.md`), and is not such a finding.
- **Does the maintainer's Cloudflare account already inject Web Analytics automatically on tcgreplay.app?**
  - Only the dashboard shows this. The live page carried no beacon on 2026-11-12.
  - When it matters: page source shows two beacon tags once the token is set.
  - What to do: the person switches automatic injection off in Cloudflare (no code change). If Cloudflare offers no way to do so, STOP and ask the user.

## Out of Scope

- **Usage inside the board and the assistant connection** (tool calls, imports, games), errors and HTTP metrics. These belong to the observability plan (`20261112090330-4ac2ea19-observability`). This plan counts page loads only and adds nothing to the playmat's own records.
- **The privacy page's wording**, including what it says about Cloudflare's retention and the `__cf_bm` edge cookie. This belongs to the privacy, terms and disclaimer plan (`20261112083238-29247b36-privacy-terms-disclaimer`). This plan only reads its `data-retention="visits"` marker.
- **A public stats page or any numbers shown on the site.** A spec non-goal.
- **Editing the website's shared layout (`Base.astro`) or the board's page shell.** The snippet is added by the hosted playmat at serve time, so neither build changes. The layout belongs to the privacy-terms work.
- **A content security policy.** Neither page sends one today. The design doc records what a future one must allow for Cloudflare.
- **Counting events inside the board** (card zooms, branch switches, moves) **or single-page-app route changes.** Deliberately off: one board session is one page view.
- **Counting the 404 page, sign-in, admin, "Not your game" and refusal pages.** Only successful website pages and the board page are counted.
- **Cloudflare's server-side edge analytics or any change to the Cloudflare zone** beyond creating the Web Analytics site and making sure automatic injection is off.
- **Counting on the old address `tcgreplay.demo.gs`, the App Platform default address or local runs.** These are excluded on purpose. The domain move redirects the old address.

## Changelog

### 2026-11-13 — Task: Add the visitor analytics package and the privacy page check

**What was done**: Added `internal/webanalytics`, which owns Cloudflare's manual beacon tag (`"spa":false`), the 16 to 64 letters-and-digits token check, `Setup` (the on/off decision with a plain status line that never shows the token) and `Inject`, a wrapper that adds the tag before the last `</body>` of a successful `GET` HTML response for the public host only. The website package gained `DescribesVisitorCounting`, true only when the embedded privacy page carries `data-retention="visits"`. Nothing calls either yet.

**Deviations**: Small additions beyond the plan's detail, all on the safe side: the wrapper also passes through HTML with a non-identity `Content-Encoding`, and drops `Accept-Ranges` from a page it altered (the altered page is no longer the file a range would be cut from). `Setup` lower-cases the host and drops any port from it too, so the on status reads "on for tcgreplay.app".

**Files changed**:
- `internal/webanalytics/webanalytics.go`
- `internal/webanalytics/webanalytics_test.go`
- `internal/site/site.go`
- `internal/site/site_test.go`

**Discoveries**: The `/board` "Not your game" page is a 403 and refusals are 503, so the 200-only rule keeps them out of the counts without any route special-casing. The tests use the standard library only (the repo has no testify), matching the existing suites.

### 2026-11-13 — Task: Count website and board pages on the hosted playmat

**What was done**: Hosted mode gained the `-web-analytics-token` flag (default `$TCGREPLAY_WEB_ANALYTICS_TOKEN`) and `hostedAnalytics`, which asks `webanalytics.Setup` whether counting is on (token, public host from `TCGREPLAY_PUBLIC_URL`, and the swappable `privacyDescribesCounting` hook over `site.DescribesVisitorCounting`) and logs the one-line status. `hostedHandler` takes the resulting wrapper and applies it only to the website mount and the `GET /board` route, outside `RequireSession`. `.do/app.yaml` declares the setting without a value, and the Dockerfile settings comment, the Makefile `docker-run` note and the design doc (a **Visitor counting** paragraph, a settings-table row and a new operator step 9) explain it.

**Deviations**: The plan put the test wiring in `startHostedWith`; the hosted handler is now built in `serveHosted` (`cmd/tcgreplay/cardsets_test.go`, after the domain move), so `hostedAnalytics(c, u)` is wired there, inside the domain move's `movedGuard`. The old operator step 9 (legacy card cleanup) is now step 10. The goal check's first attempt found the Dockerfile comment did not say "set only once the privacy page is live" or "clear it to stop counting"; both were added on the retry.

**Files changed**:
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_analytics_test.go`
- `cmd/tcgreplay/cardsets_test.go`
- `cmd/tcgreplay/hosted_admin_test.go`
- `.do/app.yaml`
- `Dockerfile`
- `Makefile`
- `TCG Review Playmat Design and Build Plan.md`

**Discoveries**: The hosted analytics tests pass both with the website unbuilt (only `/` is a 200 page, the rest are the placeholder's 404s) and built (`make site`, every page carries one beacon); they compare against `siteResponse` so they hold either way. The built privacy page carries `data-retention="visits"`, so with the privacy-terms work merged a token is all that is needed to count.

### 2026-11-13 — Task: Check counting in the website's browser suite

**What was done**: The website test server gained `-analytics-token`, which wraps the site with hosted mode's counting (same privacy-page gate) for its own loopback host and stops with the reason when counting would be off. The site suite starts a second server on 127.0.0.1:7792 with a token; the new `site/e2e/analytics.spec.js` routes every other-host request (the beacon to a stub, or an abort when blocked, everything else to an abort) and checks one beacon per crawled page with `{token, spa:false}`, the stubbed beacon as the only other address, no cookies or storage, an uncounted 404, and that blocking the beacon changes nothing (content, navigation, footer, copy buttons, no page errors). The crawl moved to `site/e2e/crawl.js`; the existing third-party check now says it covers a run without the token and also checks no page carries the beacon.

**Deviations**: `crawl` takes an optional `prepare(context)` hook so the analytics crawl can block Cloudflare before it loads any page (the plan's crawl opened its own unrouted context, which would have fetched the real beacon). The routing is one catch-all for every other host rather than two Cloudflare-only routes, and records anything it stopped, so an unexpected request fails the test instead of escaping. The server's switch is a small `analytics(token, host)` helper with Go tests for on, off and a refused bad token.

**Files changed**:
- `internal/site/sitetest/cmd/serve/main.go`
- `internal/site/sitetest/cmd/serve/main_test.go`
- `site/playwright.config.js`
- `site/e2e/crawl.js`
- `site/e2e/site.spec.js`
- `site/e2e/analytics.spec.js`

**Discoveries**: A spec's `test.use({ baseURL })` does not change `testInfo.project.use.baseURL`, which the existing `beforeAll` crawl reads, so a spec aimed at a second server must pass its address to the crawl itself. Site suite: 214 passed, 2 skipped (the existing phone-only check on the laptop profile).

### 2026-11-13 — Task: Check counting in the board's browser suite

**What was done**: The board test server gained `-analytics-token`, which wraps the board page (the `/` mount only, not `POST /test/reset`) with hosted mode's counting for its own loopback host, taking the privacy precondition as met because it has no website. The board suite starts a second server on 127.0.0.1:7789 with a token; the new `web/e2e/analytics.spec.js`, with its own fixture that resets that server and routes every other-host request first, checks at phone, tablet and desktop: one beacon with `{token, spa:false}`; card zooms, a branch switch (rewind onto a new branch, then main again in the branch picker) and, on touch profiles, the action sheet opened and closed by ✕ and by Back load no second beacon and keep the same page; only the four settings keys in `localStorage`, empty `sessionStorage`, no cookies; and, with the beacon blocked, the game shows, a zoom opens, a move lands, with no error toast, alert, page error or dialog.

**Deviations**: The action sheet is exercised on the tablet profile too, not only the phone, because it pushes a history entry on every touch profile (`ActionSheet.svelte` pushes unless in popover mode). Other-host requests that are not Cloudflare (card images from their sources) are aborted silently; only Cloudflare requests are recorded and asserted to be exactly the one stubbed or blocked beacon load. Added a small Go test for the server's `analytics` helper.

**Files changed**:
- `internal/webui/webtest/cmd/boardtest/main.go`
- `internal/webui/webtest/cmd/boardtest/main_test.go`
- `web/playwright.config.js`
- `web/e2e/analytics.spec.js`

**Discoveries**: Escape alone does not close the board's card zoom; the existing specs also click the backdrop, and so does this one. Board suite: 289 passed, 65 skipped, 0 failed (the harness's `test.fail()` dialog test shows as ✘ in the list output but counts as passed).

### 2026-11-13 — Task: Switch visitor counting on for tcgreplay.app

**What was done**: Nothing in code: this is the maintainer's task (`Execution: human`). It needs the code deployed, the Cloudflare dashboard and the App Platform control panel. The live steps are written out in the implementation test plan: check the privacy page is live; check the start-up log says counting is off; add tcgreplay.app in Cloudflare Web Analytics with the manual snippet and automatic injection off; set `TCGREPLAY_WEB_ANALYTICS_TOKEN`; check the log says `visitor analytics: on for tcgreplay.app` and that the page source carries one beacon.

**Deviations**: Skipped, and deferred to the maintainer: "deferred to the maintainer: live switch-on after deploy, per the user". The user has deferred live, human-only steps until after deploy, as they did for the domain-move and observability work. The goal check ran three times and the reviewer judged all three criteria not met each time, because live and dashboard state cannot be seen from files. The skip was recorded through `implement goal decide`. All three acceptance criteria are left unticked: the privacy page was live before the token was set; tcgreplay.app is a Cloudflare Web Analytics site with the manual snippet and automatic injection off; the start-up log says counting is on and the live page has exactly one beacon. Until the token is set, the deployed code counts nothing and the start-up log says why.

**Files changed**:
- None

**Discoveries**: The goal check has no route straight to "deferred" for a human task. It reaches a skip only after the round's three attempts are used up (`exhausted`), because the reviewer judges unverifiable live criteria `not_met`, not `impossible`.

**Closed as deferred** (orchestrator, 2026-11-13): the task heading is ticked so the specs that depend on this one can go ahead. This matches how observability's deferred maintainer tasks were closed. The work is not done: the three acceptance criteria stay unticked, and the switch-on remains the maintainer's job, following Part A of the test plan.
