---
created_date: "2026-11-12"
---

# Planning summary: 20261111093348-e1b5c2b4-public-launch

## Decisions
- **Privacy page cookie and other-site statement.** Plans: privacy-terms-disclaimer, visitor-analytics. The live site already gets a `__cf_bm` bot-protection cookie from Cloudflare's edge, and counting loads Cloudflare's beacon. Outcome: privacy-terms rewrites the sentence: the playmat sets no cookies of its own beyond the sign-in cookies, Cloudflare may set a short-lived `__cf_bm` cookie that is not used for counting or tied to an account, and the only thing loaded from another site is the visitor-counting script.
- **Where the visitor-counting retention figure is checked.** Plans: privacy-terms-disclaimer, observability. The observability runbook records only Insights' log and metric retention. Outcome: the browser test checks only the `logs` and `metrics` `data-retention` figures against `docs/observability.md`; the `visits` figure is checked against Cloudflare's documentation in the manual privacy review.
- **Use of the request Host.** Plans: domain-move, visitor-analytics. Outcome: the design doc says the request Host is never used to build an address; it is only compared against configured hosts (the moved-host guard and the analytics public-host check).
- **What's new entries during the launch.** Plans: launch-docs, chatgpt-submission-prep, privacy-terms-disclaimer. Outcome: there is one launch entry. A launch spec that lands after `site/src/data/whats-new.yaml` exists adds its player-visible changes to that entry (with `refresh: true` if it changes tools) rather than a new one; specs that land earlier are picked up by launch-docs' entry.

## Order added for shared files
- Added: `20261112090330-4ac2ea19-observability` now depends on `20261111093353-8ff3589f-domain-move`. Both change `tcgreplay:.do/app.yaml`, `tcgreplay:README.md`, `tcgreplay:internal/hostauth/limit.go`, `tcgreplay:cmd/tcgreplay/main.go`, `tcgreplay:internal/webui/server.go`, `tcgreplay:cmd/tcgreplay/hosted.go`, `tcgreplay:cmd/tcgreplay/hosted_test.go`.
- Added: `20261112090820-07d70cfb-chatgpt-submission-prep` now depends on `20261111132045-1d1d32f6-visitor-analytics`. Both change `tcgreplay:cmd/tcgreplay/main.go`, `tcgreplay:cmd/tcgreplay/hosted.go`, `tcgreplay:cmd/tcgreplay/hosted_test.go`, `tcgreplay:/privacy`, `tcgreplay:.do/app.yaml`.
- Added: `20261112091756-c5b6d1fd-launch-docs` now depends on `20261112083238-29247b36-privacy-terms-disclaimer`. Both change `tcgreplay:site/src/lib/facts.ts`, `tcgreplay:import.meta.url`, `tcgreplay:gotchas/astro-site-build.md`, `tcgreplay:site/e2e/facts.spec.js`, `tcgreplay:site/e2e/site.spec.js`, `tcgreplay:site/e2e/limits.spec.js`.
- Added: `20261112091756-c5b6d1fd-launch-docs` now depends on `20261111132045-1d1d32f6-visitor-analytics`. Both change `tcgreplay:internal/site/site.go`, `tcgreplay:site/astro.config.mjs`, `tcgreplay:/mcp`, `tcgreplay:gotchas/astro-site-build.md`, `tcgreplay:/docs/connect`, `tcgreplay:site/e2e/site.spec.js`, `tcgreplay:site.spec.js`.
- Added: `20261112091756-c5b6d1fd-launch-docs` now depends on `20261112090820-07d70cfb-chatgpt-submission-prep`. Both change `tcgreplay:https://tcgreplay.app/mcp`, `tcgreplay:site/src/data/whats-new.yaml`, `tcgreplay:chatgptDirectory.listed`.
- Added: `20261123114225-5cf4d0df-grafana-observability` now depends on `20261112083238-29247b36-privacy-terms-disclaimer`. Both change `tcgreplay:site/src/pages/privacy.astro`, `tcgreplay:.do/app.yaml`, `tcgreplay:docs/observability.md`, `tcgreplay:internal/hostauth/hostauth.go`, `tcgreplay:google.go`, `tcgreplay:site/e2e/privacy.spec.js`.
- Added: `20261123114225-5cf4d0df-grafana-observability` now depends on `20261111132045-1d1d32f6-visitor-analytics`. Both change `tcgreplay:cmd/tcgreplay/main.go`, `tcgreplay:cmd/tcgreplay/hosted.go`, `tcgreplay:/ws`, `tcgreplay:/mcp`, `tcgreplay:cmd/tcgreplay/cardsets_test.go`, `tcgreplay:cmd/tcgreplay/hosted_test.go`, `tcgreplay:hosted_test.go`, `tcgreplay:.do/app.yaml`, `tcgreplay:127.0.0.1`.
- Added: `20261123114225-5cf4d0df-grafana-observability` now depends on `20261112090820-07d70cfb-chatgpt-submission-prep`. Both change `tcgreplay:internal/mcpserver`, `tcgreplay:internal/mcpserver/server.go`, `tcgreplay:server.go`, `tcgreplay:cmd/tcgreplay`, `tcgreplay:cmd/tcgreplay/main.go`, `tcgreplay:cmd/tcgreplay/hosted.go`, `tcgreplay:site/src/pages/privacy.astro`, `tcgreplay:cmd/tcgreplay/hosted_test.go`, `tcgreplay:site/e2e/privacy.spec.js`, `tcgreplay:.do/app.yaml`, `tcgreplay:internal/hostauth`, `tcgreplay:internal/usergames`, `tcgreplay:internal/usergames/usergames.go`, `tcgreplay:site/src/data/whats-new.yaml`.
- Added: `20261123114225-5cf4d0df-grafana-observability` now depends on `20261112091756-c5b6d1fd-launch-docs`. Both change `tcgreplay:/mcp`, `tcgreplay:site/src/data/whats-new.yaml`.

## 20261111093353-8ff3589f-domain-move
### Approach

Most of the move is already live (checked 2026-11-12): tcgreplay.app serves the playmat through Cloudflare in front of App Platform with a valid certificate, the OAuth metadata names only tcgreplay.app, plain HTTP goes to HTTPS, www does not resolve, and a Cloudflare redirect forwards every tcgreplay.demo.gs path to tcgreplay.app. The maintainer's uncommitted `.do/app.yaml` edits (public URL, primary domain, demo.gs alias) are restated by the plan and should be committed or discarded before implementation.

- One public address, `TCGREPLAY_PUBLIC_URL=https://tcgreplay.app`; the website reads it from `.do/app.yaml` and the build fails if it is missing, a placeholder, malformed or not the primary domain.
- Old pages keep forwarding at Cloudflare, but the rule stops catching `/mcp`: a cross-host redirect drops the assistant's token and a 401 would start a confusing re-sign-in.
- New `TCGREPLAY_MOVED_FROM` lists old hosts. On those hosts `/mcp` gets an unauthenticated MCP server mirroring the real tools and guides, each answering "moved, reconnect at https://tcgreplay.app/mcp"; every other path gets a 301/308 keeping path and query, as a backstop.
- Retiring the old address needs no release.
- Rejected: a dual-address server, a Cloudflare Worker speaking MCP, a 401, a single `moved` tool, treating any unknown host as moved, a hard-coded site address.

### Milestones and tasks

- M1, website, docs and README name tcgreplay.app: website reads the public address from the deployment definition (agent); site pages and README name the new address, with one marked move note on the connecting page and a whole-site check for stray old or www addresses (agent).
- M2, an assistant on the old address is told where the playmat moved: moved answer for MCP connections (agent); moved-host setting and guard in the hosted playmat, adding `TCGREPLAY_MOVED_FROM` to `.do/app.yaml` (agent).
- M3, the move goes live with a checked runbook: `docs/domain-move.md` runbook and design-doc notes (agent); carry out the console steps and put the move live (human).

### Tasks for a person

- Carry out the console steps and put the move live: apply the app spec by hand, narrow the Cloudflare rule to leave `/mcp` alone, update the Google consent-screen URLs, remove the old Google callback, and confirm the steps already live. Needs the Google Cloud, Cloudflare and DigitalOcean consoles and production access.

### Out of scope

- Keeping old connections or board sign-ins alive across the move; sign-in at more than one address; any www name.
- Analytics, observability, privacy, terms and disclaimer; ChatGPT plugin files, domain verification and directory docs.
- Changing hosting or DNS provider; actually retiring demo.gs; automated tests of Cloudflare, App Platform, Google or assistant display.

### Drafting assumptions

- Chosen direction as above; the server-side redirect is a backstop, not a second redirect layer.
- Old hosts are an explicit list, not "any host other than the public one".
- The runbook lives in `docs/domain-move.md`, a maintainer document outside the website.
- The only website mention of the old address is one marked note on `/docs/connect`; none in the README.
- The human task confirms console work already live rather than redoing it; the first agent task restates the uncommitted `.do/app.yaml` edits.
- Open for implementation (STOP and ask if either fails): whether App Platform passes the original Host to the container, and whether claude.ai and ChatGPT show the moved error text.

### Project-wide rules

- `.do/app.yaml` is the single source for the public address; the site build reads `TCGREPLAY_PUBLIC_URL` from it and checks it against the primary domain.
- Edits to `.do/app.yaml` reach App Platform only when a person applies the spec with `doctl apps update`; the deploy workflow never applies it.
- The request Host is never used to build an address; it is only compared against configured hosts (the moved-host guard, and visitor-analytics' public-host check). The design doc is amended to say so.
- The website may name `demo.gs` only inside a `data-moved-note` element and never `www.tcgreplay.app`; the browser suite enforces both.
- The changelog is left to the implement workflow.

### Manual checks

- `/`, `/docs/` and `/board` load at tcgreplay.app, and the sign-in and connection metadata name only tcgreplay.app.
- The certificate is accepted, `http://tcgreplay.app/docs/connect/` ends on https, and www does not resolve.
- demo.gs `/docs/connect/` and `/board?x=1` get a permanent redirect keeping path and query; `http://tcgreplay.demo.gs/docs/` ends at tcgreplay.app.
- From claude.ai, ChatGPT and Claude Code: Google sign-in completes, a tool call works, and the board link starts with `https://tcgreplay.app/` and opens signed in.
- claude.ai and ChatGPT with a connector still on demo.gs/mcp show a message naming `https://tcgreplay.app/mcp`.
- A sign-in started through demo.gs never completes there and ends at tcgreplay.app.
- With demo.gs removed and no new build, tcgreplay.app still serves the website, board and assistant.
- Every runbook step has a command or page and its expected result.
- Success metric: the ChatGPT submission names tcgreplay.app and never needs an address change.
- Success metric: no player asks for help reconnecting in the first two weeks.
- Success metric: no server errors at tcgreplay.app caused by the address change in the first week.
- Review: the maintainer reads the runbook against the live consoles.
- Review: the maintainer checks the moved-message wording as it appears in claude.ai and ChatGPT.

## 20261112090330-4ac2ea19-observability
### Approach

- A new `internal/telemetry` package, used by the hosted playmat only, writes one JSON record per line to stdout. The maintainer reads them in DigitalOcean Insights' Logs tab (LON1): App Platform's own log viewer keeps no history, and the user confirmed Insights shows the playmat's runtime logs.
- Players appear only as `p_<16 hex>`, an HMAC-SHA256 of the lower-cased email keyed by a new required secret `TCGREPLAY_PLAYER_ID_KEY` (at least 32 characters); hosted mode will not start without it.
- One MCP receiving middleware after the rate limiter, built by `usergames.Registry`, records `tool_call` (tool and assistant), `import` and `new_game` on success, and `error` with a redacted message.
- A `player_day` record from the registry's activity clock gives distinct players per day, since Insights has no distinct count.
- Every log line carrying an email becomes a structured record keyed by player ID; failed board actions are recorded through a new `webui.Server.OnError` hook; admins get an email-to-ID lookup on `/admin`.
- Restart, CPU and memory alerts are declared in `.do/app.yaml`; graphs come from Insights' App Platform metrics.
- Runbook `docs/observability.md`: event catalogue, Insights filters, capacity figures, retention.
- Rejected: Managed OpenSearch forwarding, in-memory or admin-page counts, counts kept in Spaces, deriving the key from the signing key.

### Milestones and tasks

- M1, players appear in the logs only as a pseudonymous ID: add the telemetry package (agent); switch existing log lines to player IDs, add the required secret and a guard test against logging emails (agent).
- M2, the maintainer can see who used the playmat, with what, and what went wrong: record tool calls, daily players and errors, with end-to-end hosted acceptance tests (agent); let admins look up a player's ID (agent).
- M3, the platform watches the playmat, and its limits and retention are known: declare alerts and the secret and write the runbook (agent); add the memory-per-game measurement (agent); set up the secret and alert recipients, deploy and measure (human); record capacity and retention and set the launch limits (agent); apply the launch limits (human).

### Tasks for a person

- Set up the secret and alert recipients, deploy and measure: DigitalOcean control panel, production secrets and a production deploy; measure memory on the deployed app and confirm Insights' retention.
- Apply the launch limits: production access to the DigitalOcean app.

### Out of scope

- The privacy page's wording; visitor analytics; public usage numbers or a stats page.
- Alerts for failed deploys and domain or certificate problems; traces and custom metrics.
- Any other log forwarding service, monitoring vendor or storage of our own; in-memory or admin-page counts.
- Recording successful board actions as usage; the desktop app, daemon or dev mode; setting up Insights or alert recipients through the API.

### Drafting assumptions

- Records are read in DigitalOcean Insights (settled by the user's two answers during planning).
- Error messages are redacted, not dropped; over-redaction is preferred to a leak.
- The player-ID key is a new required secret, not derived from `TCGREPLAY_SIGNING_KEY`, with no random fallback; ID = `p_` + first 16 hex of HMAC-SHA256 over the domain-separated, lower-cased, trimmed email.
- The admin lookup is a POST form on the existing `/admin` page; a closed catalogue of 14 events.
- Starting alert thresholds: restarts above 0 over 5 minutes; CPU and memory above 80% over 10 minutes. Capacity headroom: memory at or below 70% with every place filled.
- Open for implementation (STOP and ask): Insights keeping logs under 7 days or dropping them on redeploy; Insights unable to filter JSON fields; the 70% budget needing an instance over 1 GB for at least 10 players.

### Project-wide rules

- No log line anywhere carries an email address; players appear only as `p_` IDs keyed by `TCGREPLAY_PLAYER_ID_KEY`, enforced by a guard test.
- Structured records are JSON lines on stdout with a fixed field order (time, level, event, player, …) and a closed event set; plain `log.Printf` stays for lines that name no player.
- `.do/app.yaml` gains `TCGREPLAY_PLAYER_ID_KEY` (secret), an `alerts:` block, and later `TCGREPLAY_MAX_USERS` and `instance_size_slug` values; edits take effect only when a person applies them.
- The website build reads `TCGREPLAY_MAX_USERS` from `.do/app.yaml`, so a changed limit flows to the site and the privacy and terms pages.
- `docs/observability.md` is the source the privacy page uses for what is recorded, what never is, and how long logs and metrics are kept.
- Admin features live on the existing `/admin` page behind `hostauth.RequireAdmin`.

### Manual checks

- Success metric: a week after launch, distinct players per day and the assistants they used can be read from Insights alone.
- Success metric: starting from a player's email, their error records are found in under five minutes.
- Success metric: no crash or resource exhaustion in the first month goes unnoticed.
- Records appear in Insights, filterable and searchable, each carrying the player ID.
- On a day two test accounts play, Insights shows `player_day` = 2 plus tool calls by assistant, imports and new games.
- Searching Insights finds no email, display name, token, battle-log text or card name.
- A player's ID is the same before and after a real redeploy.
- The deployed admin lookup works for an admin and is refused to a non-admin and when signed out.
- Insights shows request rate, P95, status codes, CPU, memory and restarts over 24 hours and 7 days.
- Alert emails arrive for a forced restart and for temporarily lowered CPU and memory thresholds.
- With the player limit's worth of games open on the deployed instance, memory stays within the 70% budget.
- The runbook states how long Insights keeps logs and metrics, confirmed with DigitalOcean.
- Review: a person checks the runbook's event catalogue matches the code and its filters work.

## 20261112083238-29247b36-privacy-terms-disclaimer
### Approach

- The fan-project notice, the card-data credit (TCGdex, malie.io) and the help address (GitHub issues) are written once in `internal/notice/notice.json`, read by the website build (`site/src/lib/notice.ts`, which stops the build if the file is broken) and the board build (Vite).
- A new public `/terms` page (`site/src/pages/terms.astro`), served by the existing Go site handler; the only Go change is one route test.
- The footer in `Base.astro` shows the notice and credit and links to Privacy, Terms, Get help and Source on GitHub.
- The privacy page gains Visitor counting and Diagnostics sections, and every statement the code no longer bears out is corrected, including the cookie sentence (decided at epic planning: the playmat sets no cookies of its own beyond the sign-in cookies; Cloudflare may set a short-lived `__cf_bm` bot-protection cookie not used for counting or tied to an account; the only thing loaded from another site is the visitor-counting script).
- The board's ☰ menu gains "About tcgreplay", a panel in the existing modal system with the notice, credit and a help link.
- Limits come from the facts loader (`.do/app.yaml`). Retention figures are typed in inside `data-retention` elements; the `logs` and `metrics` figures are tested against `docs/observability.md`, and the `visits` figure is checked by hand against Cloudflare's documentation.
- Rejected: hand-kept copies pinned by tests, a Go route for static text, a permanent strip on the board, a build that parses the runbook.

### Milestones and tasks

- M1, terms published and every page links to them, the privacy page and help: share one notice wording and show it in the footer (agent); publish the terms page (agent, after the footer task).
- M2, the privacy page tells the truth about visitor counting and diagnostics: bring the privacy page up to date (agent; needs the observability runbook with its Retention figures, else STOP and ask).
- M3, the board shows the notice, the credit and a help link: add an About panel to the board (agent, after the footer task).

### Tasks for a person

None.

### Out of scope

- Building visitor counting (including updating the site suite's third-party check); building diagnostics or editing the runbook; writing the ChatGPT listing.
- A lawyer's review and a governing law; Terms in the header navigation; privacy and terms links on the board.
- Changing the playmat to make old wording true (a token revocation endpoint, the shared hosted `assistant.txt`).
- Moving pages from demo.gs to tcgreplay.app; fixing the stale "sign-ins lost on restart" comment in `.do/app.yaml`.

### Drafting assumptions

- One shared file, static website pages, an on-demand board panel, limits from facts, typed-in retention cross-checked.
- Terms is linked from the footer only.
- Visitor-counting retention comes from Cloudflare's documentation; if none is documented, the page says the counts are kept under Cloudflare's own policy, with a link.
- Wording is corrected to match the code, not the code to match the wording.
- The board panel holds only notice, credit and help link, in hosted and desktop modes.
- Last-updated dates are typed in; a manual check confirms they equal the go-live day.
- If `site/src/data/whats-new.yaml` exists when a task lands, the terms page and About panel are added to the existing launch entry, not a new one (decided at epic planning).
- Audit findings outside scope: in hosted mode every account writes the assistant's name to one shared `<dataDir>/assistant.txt`; the design notes' consent-screen step still names `tcgreplay.demo.gs/privacy` (domain-move owns that).

### Project-wide rules

- The notice, credit and help wording lives only in `internal/notice/notice.json`; the footer, terms, board and ChatGPT listing read it.
- The help channel is `https://github.com/nicholasjackson/tcgreplay/issues`, with no support email.
- The policy pages stay at `/privacy` and `/terms`.
- The privacy page describes visitor counting before counting is switched on; visitor analytics starts only after this page is live.
- The `logs` and `metrics` retention figures carry `data-retention` markers tested against the Retention section of `docs/observability.md`; the `visits` marker is checked by hand against Cloudflare's documentation.
- The website names addresses only through `site.publicURL`; the Dockerfile `web` and `site` stages each gain `COPY internal/notice`.

### Manual checks

- Success metric: the ChatGPT submission accepts the privacy, terms and support links, and its review raises no objection about the policy pages.
- After deploy, `https://tcgreplay.app/privacy`, `/terms` and the GitHub issues page open in a private window without signing in.
- Privacy accuracy review: a person reads the page against `docs/observability.md` and Cloudflare's documentation, confirming everything recorded is described, the retention periods (including `visits`) match, and the cookie statement is true on the live site.
- The last-updated dates on the privacy and terms pages equal the go-live day.
- Review: the maintainer reads the terms for tone and completeness against the spec's list (not a legal review).
- On a real phone and tablet, the board's About panel opens from ☰, is readable, and closes without covering or shifting any card or control.

## 20261111132045-1d1d32f6-visitor-analytics
### Approach

- The website (Astro) and the board (Vite) share no page shell, but the hosted Go router serves both, so Cloudflare's manual Web Analytics snippet (`"spa":false`) is added there at serve time by a new `internal/webanalytics` package.
- Only website routes and `GET /board` are wrapped; the snippet goes before `</body>` on GET 200 HTML responses only.
- Counting is on only when `TCGREPLAY_WEB_ANALYTICS_TOKEN` (flag `-web-analytics-token`) is well formed, the embedded privacy page carries `data-retention="visits"`, and the request is for the public host. Otherwise the playmat starts and logs why counting is off. Clearing the setting turns counting off with no release.
- Local runs, the desktop app and tests never count.
- Rejected: Cloudflare's automatic edge injection (cannot turn SPA tracking off; the phone action sheet's `pushState` would inflate board views), the snippet in both builds, options on the site handler or board server, wrapping the whole router.

### Milestones and tasks

- M1, the live playmat can count but does nothing until told to: add the visitor analytics package and the privacy page check (agent); count website and board pages on the hosted playmat, with start-up log line, `.do/app.yaml` setting, Dockerfile comment and design-doc notes (agent).
- M2, the browser suites show counting is invisible: website browser suite with an opt-in second server on 7792 and a reworded third-party check (agent); board browser suite with an opt-in second server on 7789 (agent).
- M3, counting is live: switch visitor counting on for tcgreplay.app (human).

### Tasks for a person

- Switch visitor counting on for tcgreplay.app: once the privacy page is live, add the Web Analytics site in Cloudflare using the manual snippet with automatic injection off, then set the token in the App Platform control panel.

### Out of scope

- Usage inside the board or assistant, errors and metrics; the privacy page's wording; a public stats page.
- Edits to `Base.astro` or the board shell; a content security policy.
- Counting in-board events or SPA route changes; counting 404, sign-in, admin and refusal pages.
- Cloudflare zone changes beyond creating the site; counting the old address, the App Platform default address or local runs.

### Drafting assumptions

- The spec's "shared page shell" is read as the hosted server that serves both.
- A board session counts as one page view of `/board`; Cloudflare counts a visit only with an external referrer, so home → board adds a page view but no visit (worth challenging at review).
- The snippet is added only for the public host, ignoring case and port; a malformed token warns and leaves counting off.
- The token is declared in `.do/app.yaml` with no value and set in the control panel; `make docker-run` does not pass it.
- With analytics on, the site test server exits if its privacy page lacks the marker, so the site suite depends on privacy-terms having landed.
- Open for implementation (STOP and ask): App Platform not passing `Host: tcgreplay.app`; the real beacon setting a cookie or storage; Cloudflare auto-injection already on and not removable.

### Project-wide rules

- Relies on privacy-terms' `data-retention="visits"` marker; counting cannot start without it.
- The site suite's "nothing fetched from other hosts" check stays strict without the token; with analytics on, only the stubbed `static.cloudflareinsights.com` beacon is allowed.
- `Base.astro` and the board shell are left untouched; no address is named in website text.
- `hostedHandler` gains one final argument (the counted-pages wrapper); domain-move's moved-host guard stays outermost.
- The request Host is only compared against configured hosts, never used to build an address (shared with domain-move).
- `.do/app.yaml` gains `TCGREPLAY_WEB_ANALYTICS_TOKEN` (RUN_TIME, no value); a new hosted setting follows the flag-plus-env, start-up-log, warn-and-carry-on pattern; the start-up log never carries the token, an email or an account.
- The board's browser storage stays at the four existing settings keys.

### Manual checks

- Success metric: a week after launch, visits and board sessions for the week can be answered from the dashboard alone.
- Success metric: the dashboard shows only tcgreplay.app traffic.
- The visitor-counting section is live at `https://tcgreplay.app/privacy` before the token is set, and the log said counting was off until then.
- The home page and a docs page appear in dashboard page views within 30 minutes.
- A signed-in board session with card zooms and branch switches shows as one `/board` page view within 30 minutes.
- The last 7 days view shows totals, top pages, and referrer, country and device breakdowns.
- Live devtools: no analytics cookie or storage on the website or board, and no email, account, game or card data in analytics requests (`__cf_bm` comes from Cloudflare's edge, not analytics).
- With a content blocker on, the website and board load and work with no error.
- No consent banner or analytics prompt on any page.
- Local and test runs add no traffic to the dashboard.
- The live page source carries exactly one beacon (ours, with `"spa":false`).

## 20261112090820-07d70cfb-chatgpt-submission-prep
### Approach

All work is in the one repo; nothing changes for Claude except a better-described tool list and, on the hosted playmat, one new tool.

- Tool labels: go-sdk v1.6.0 cannot send `readOnlyHint:false`, so the tool server gets a `tools/list` receiving middleware writing all three labels as explicit booleans on every tool, on every transport (domain-move's moved server inherits it).
- Titles and descriptions added where each tool is registered; a raw-JSON guard test enforces title, labels, "Use this when", "Do not use" and parameter descriptions; a size test caps the tool list.
- Instructions reordered so the first 512 bytes stand alone; `wait_for_turn` returns within 15 s by default and start-up refuses a cap over 50 s.
- New hosted-only `get_account` tool (OpenAI's profile tool) returning only the Google `sub`.
- `TCGREPLAY_OPENAI_CHALLENGE` served at `/.well-known/openai-apps-challenge`.
- `TCGREPLAY_REVIEWER` names the reviewer's Google-verified email, which gets a place outside the player limit.
- A Go package builder (`internal/plugin`, `cmd/tcgreplay-plugin`, `plugin` Make target) reading the address from app.yaml, notice and support from `notice.json`, and listing text from `plugin/listing.json`; a new listing icon without a Poké Ball.
- Rejected: upgrading or forking the SDK, rewriting HTTP bodies.

### Milestones and tasks

- M1, every tool labelled and described: explicit labels on every tool; titles and descriptions; instructions that stand alone and waits inside the limit (all agent).
- M2, identity, domain check and reviewer: tell ChatGPT who is signed in, with one privacy-page sentence (agent); answer OpenAI's domain verification (agent); let the reviewer in past the player limit (agent); set the domain token and reviewer on the hosted playmat (human).
- M3, package and review material: a listing icon without Pokémon trademarks; build the plugin package; write the review material (5 positive and 3 negative cases, release notes, demo script, `plugin/SUBMITTING.md`, and a What's new line added to the launch entry if the list exists) (all agent).

### Tasks for a person

- Set the domain token and reviewer on the hosted playmat: get the token from the OpenAI dashboard, set both secrets in the DigitalOcean App Platform console and apply the app spec by hand.

### Out of scope

- Skills in the plugin, widgets inside ChatGPT, a Pokémon licence, raising the player limit, the policy pages and directory docs.
- The maintainer's own submission steps (identity verification, the reviewer's Google account, the video, upload, submit, publish).
- Replacing the website and board icon, which still show a Poké Ball (worth the maintainer's attention, since reviewers will see the site).
- Renaming `get_profile`; output schemas for game tools; an automated backwards-compatibility snapshot; CIMD client registration; a country allow-list.

### Drafting assumptions

- "Destructive" covers only `new_game`, `import_log` and `set_opponent_deck`; reads that write an internal audit record, and `wait_for_turn`, stay read-only.
- `get_account` returns the Google `sub` and no email; the reviewer is matched by verified email and holds a place outside the cap.
- The size ceiling is the measurement after the description pass plus 10%; `securitySchemes: oauth2` on every tool in hosted mode only.
- The privacy page gains one sentence saying the assistant is given the player's Google account ID.
- The What's new line goes into the existing launch entry with `refresh: true` only if `site/src/data/whats-new.yaml` exists; otherwise launch-docs' launch entry covers it.
- The app.yaml env scanner moves into a shared `internal/appspec` package; package name `tcgreplay`, version 1.0.0, category Games, developer "the tcgreplay project"; countries left out; the listing source writes `{public}` instead of literal addresses; the sample battle log is `/docs/example`.

### Project-wide rules

- The notice, credit and help link are read from `internal/notice/notice.json`, never copied; the support link is GitHub issues.
- `TCGREPLAY_PUBLIC_URL` is the single address; the builder derives `/mcp`, `/privacy` and `/terms` from it.
- No built website page says "Plugin Directory" while unlisted; review material lives in `plugin/`.
- No log line carries an email; the start-up log only says whether the reviewer and challenge token are set.
- New hosted settings `TCGREPLAY_OPENAI_CHALLENGE` and `TCGREPLAY_REVIEWER` (environment only, `type: SECRET`, no value in `.do/app.yaml`).
- The tool server owns the `tools/list` labelling middleware; every new tool needs a title, "Use this when", "Do not use", explicit labels and parameter descriptions, enforced by test.
- OpenAI's profile tool is `get_account`, not `get_profile`; access and refresh tokens carry the verified-email flag, older tokens read as unverified.
- Listing name "TCG Replay"; package and connection name `tcgreplay`; after publication tool names, parameters and results stay backwards compatible.

### Manual checks

- Success metric: the submission passes OpenAI's automated checks and reaches human review.
- Success metric: if rejected, the reason is not missing metadata, missing material or a broken connection.
- Success metric: TCG Replay is listed in ChatGPT's Plugin Directory.
- The ZIP uploads without validation errors and shows the name, icon, notice and working links.
- The live challenge URL returns exactly the token, and OpenAI's dashboard accepts it.
- MCP Inspector: connect with Google sign-in, list the tools, and call `get_account`, `import_log` and `get_state`.
- End to end in ChatGPT: connect, sign in, import the example game, open the board link, review and replay; all 5 positive cases behave and all 3 negative cases are handled.
- The reviewer account gets in at the limit while another account is refused.
- claude.ai and Claude Code connect, import and play as before.
- Review: icon and listing have no Pokémon trademark, nothing aimed at under-13s, no comparisons or pricing.
- The tool list size is recorded in the release notes, and ChatGPT shows the tools after a refresh with no size error.

## 20261112091756-c5b6d1fd-launch-docs
### Approach

All work is in the website (`site/`); no Go changes. Builds on domain-move, which must be merged first.

- `connect.md` becomes `site/src/pages/docs/connect.astro` at the same URL, keeping domain-move's single `data-moved-note` (now anchored `#moved`) and taking every address from `site.*` facts. The ChatGPT section is rewritten to OpenAI's current custom-server flow and names the plans that allow it. The Plugin Directory steps are written now behind `site/src/lib/listing.ts` (`chatgptDirectory.listed = false`).
- One "Refresh after an update" section (`/docs/connect#refresh`) for claude.ai, ChatGPT and Claude Code, linked from each assistant's troubleshooting line.
- What's new: one YAML list `site/src/data/whats-new.yaml` read by `site/src/lib/whats-new.ts`, which sorts newest first and stops the build on bad entries; page `/docs/whats-new`.
- Docs index: the sidebar list moves to `site/src/lib/docs-pages.ts`, and a new `site/src/pages/docs/index.astro` renders it.
- `Base.astro` is left alone (privacy-terms owns it).
- Rejected: a remark plugin, hidden HTML, pasting the directory steps in later.

### Milestones and tasks

- M1, the docs address opens a docs index: docs index and shared docs page list (agent).
- M2, ChatGPT users can connect at launch and everyone can refresh: connecting page with current ChatGPT steps and refresh steps (agent; after domain-move tasks 336cf3bc and 92eaa296 are merged).
- M3, players can see what changed in each update: What's new page and launch entry (agent; also writes the What's new convention).
- M4, ChatGPT users find TCG Replay in the Plugin Directory: switch on the Plugin Directory route (human).

### Tasks for a person

- Switch on the Plugin Directory route: after OpenAI approves the listing and the maintainer chooses when, confirm the live directory wording with a ChatGPT account, set the listing flag to true and add a What's new entry.

### Out of scope

- Changing the docs' addresses; privacy and terms pages, help links, header or footer edits; the listing text, plugin package and submission.
- Listing in Claude's connector directory; rewriting the claude.ai and Claude Code connection steps beyond refresh links.
- An automated "every release adds an entry" check, an RSS feed, entries from before launch.

### Drafting assumptions

- ChatGPT plans drafted as Business, Enterprise and Edu on the web (on Business an admin or owner adds it); Plus and Pro left out because sources disagree.
- Refresh steps: claude.ai remove and re-add the connector; ChatGPT Plugins → tcgreplay → Refresh (directory installs update on their own); Claude Code `/mcp` → Reconnect, or restart.
- The launch entry links to `/docs/connect#moved` and never names the old address; it is dated the day it merges and lists the address move, the new ChatGPT steps, the refresh steps, What's new, and sibling launch changes already on main. Launch specs landing later add their player-visible changes to this one entry (decided at epic planning).
- The custom connection is named `tcgreplay`; the directory steps use "TCG Replay".
- Sidebar order: All docs first, then Get started 10, Refreshing the connection 15, …, Limits 80, What's new 85, For developers 90.
- Open for implementation: which ChatGPT plans can actually connect a writing custom server (STOP if no individual plan can); whether the claude.ai and Claude Code refresh steps work (STOP if nothing works in an assistant).

### Project-wide rules

- The website gets its address only from `site/src/lib/facts.ts`; `demo.gs` appears only inside the connecting page's single `data-moved-note` aside.
- Player-visible changes are recorded in `site/src/data/whats-new.yaml`; every player-visible change adds an entry in the same change, with `refresh: true` when a tool, tool description, guide or the server instructions change. During launch, specs add to the single launch entry instead of new entries. Written as a conventions knowledge entry during implementation.
- While `site/src/lib/listing.ts` says not listed, no built page may contain "Plugin Directory"; the whole-site scan enforces it.
- The docs sidebar and docs index share `site/src/lib/docs-pages.ts`; new hand-written docs pages are added there.
- `site/src/content/docs/connect.md` is replaced by `site/src/pages/docs/connect.astro`; other plans edit the Astro page.
- `Base.astro` belongs to privacy-terms.

### Manual checks

- At launch, the ChatGPT custom steps from an account on a named plan connect TCG Replay and its tools appear; the plan list matches OpenAI's help page.
- After a release that adds or changes a tool, the refresh steps in claude.ai, ChatGPT and Claude Code make it appear.
- Once the flag is switched, the directory steps find "TCG Replay" in the Plugin Directory and connect it.
- Success metric: in the first month, no GitHub issue asks a ChatGPT-connection or missing-tools question the docs already answer.
- Success metric: every player-visible release since launch has a What's new entry.
- Review: the maintainer reads the connecting page, refresh section, What's new page and launch entry for clarity, and confirms the launch entry lists what shipped.

## 20261123114225-5cf4d0df-grafana-observability
### Approach

- The OpenTelemetry Go SDK runs in the hosted server only, in a new `internal/observe` package. It sends logs, traces and metrics over OTLP/HTTP to Grafana Cloud (Germany, `prod-eu-west-2`/`-4`), controlled by optional `TCGREPLAY_GRAFANA_*` settings. Unset or incomplete settings mean nothing is sent, and the start-up line says so. Desktop modes never build it.
- Logs reuse the existing `telemetry.Recorder` records. Stdout is unchanged, and a sink adds trace IDs and counters. Standard `log` lines reach Grafana through a second writer beside stderr.
- Traces:
  - `otelhttp` wraps the hosted handler. It accepts Cloudflare's W3C `traceparent` with an always-on sampler, so Cloudflare's sampling never drops playmat spans.
  - Every outside client (Google, card sets, image store, CDN, TCGdex) is traced.
  - Tool-call spans take their trace context from request headers in an outermost MCP middleware. Tool handlers receive the initialize request's context, not their own.
  - Board actions are root spans linked to their `/ws` span.
  - An image copy is a child span of the card request that queued it.
- An allow-list filter keeps client IP, user agent, query strings, OAuth codes and state, and board handles out of spans.
- Grafana Free keeps metrics only 14 days. So an hourly job reads the playmat's own daily gauge back through Grafana's Prometheus API and re-exports six weeks of daily totals. Grafana is the only store: no new storage.
- Telemetry never blocks players: 2,048-item drop-not-block queues, 5-second export timeouts with no retries, and one stderr line after 5 minutes of failure, at most every 5 minutes.

### Milestones and tasks

- **M1, records and log lines kept in Grafana across updates** (ships with sending off): telemetry export foundation; records and log lines sent to Grafana.
- **M2, every request traced, edge to outside services, logs linked:**
  - every HTTP request and outside call traced;
  - tool calls and board actions traced, with records linked to their trace;
  - image copies joined to their card request, plus the end-to-end privacy sweep.
- **M3, daily usage counted and kept six weeks:** usage counted as metrics; six weeks of daily usage with a dashboard (`docs/grafana/tcgreplay-usage.json`).
- **M4, maintainer reads everything in Grafana:**
  - runbook rewritten for Grafana;
  - privacy page names Grafana Cloud, with a What's new entry;
  - turn on sending (human);
  - turn on Cloudflare edge traces (human).

### Tasks for a person

- **Turn on sending to the Grafana stack:** create the access policy token (metrics read/write, logs write, traces write) and set `TCGREPLAY_GRAFANA_*` in the App Platform console. Needs the Grafana Cloud account and production secrets.
- **Turn on Cloudflare edge traces:** enable domain Traces with "Forward to origin" and the Grafana OTLP export. Needs the Cloudflare dashboard. If export is not on the Free plan, or the trace context does not survive App Platform's edge, the runbook records why and the edge requirement is dropped, as the spec allows.

### Out of scope

- The spec's non-goals: browser telemetry, public stats, Cloudflare Logpush.
- Grafana alerting, and moving platform metrics out of DigitalOcean.
- Sampling, kept as a later lever if volume nears the free tier.
- Paid plans, a collector or self-hosted monitoring.
- Joining admin image-clear jobs to the admin request's trace.
- The password provider itself (sign-in page plan).
- New record events.

### Drafting assumptions

- Daily totals are carried forward in Grafana for 42 days, because Free keeps metrics 14 days.
- Settings are named `TCGREPLAY_GRAFANA_{OTLP_URL,INSTANCE_ID,TOKEN,METRICS_URL,METRICS_USER}`, not `OTEL_*`. One token holds all four scopes.
- OTel providers are passed explicitly and never set as globals; only the error handler and logger globals are set, in hosted mode.
- Metric labels are closed sets (unknown assistants and tools become `other`), and `otelhttp` HTTP metrics are off to stay within the 10k-series budget.
- The 5% latency criterion is guarded in CI by a hung-receiver test, and measured by hand on the live playmat.
- Sending is turned on only after the privacy page names Grafana (M4). M1 to M3 ship with sending off.
- `Places.Admit`, `signIn`, `allow`, `usergames.Use` and `ResolveMCP` take a context parameter, so their records carry traces.

### Project-wide rules

- The privacy page names Grafana Cloud, Germany, records and traces kept 14 days, and daily totals six weeks. `site/e2e/privacy.spec.js` ties each `data-retention` figure (logs, traces, totals, metrics) to the runbook's Retention section.
- There is one What's new entry for the privacy change (`refresh: false`).
- New settings are declared in `.do/app.yaml` (the token as SECRET, with no value) and set in the console. Never paste the whole file.
- Optional hosted settings report on or off in the start-up line and never stop play.
- `telemetry.Event.Provider` (record field `provider`) is shared with the sign-in page plan; whichever lands second keeps the one field.
- Recorder calls take a context (`RecordContext`, `ActiveContext`), and the sign-in plan's `finishSignIn` must keep that parameter.
- MCP receiving middleware is added in one `AddReceivingMiddleware` call: tracing, then the rate limit, then the recorder.
- The `noemail_test` AST scan is extended to span and log attributes.

### Manual checks

- From a player's email, find their errors and the failing request's trace in under five minutes, without the hosting console.
- In the first two weeks after launch, every day has usage and error records in Grafana, and no deploy loses history.
- Median tool-call duration with telemetry on is within 5% of off, on the deployed playmat.
- Daily Google sign-ins can be compared two weeks before and after the sign-in page release.
- Monthly telemetry volume stays within the Grafana Free tier.
- After the settings are set, the start-up line says export and usage history are on, and data appears in the stack.
- Logs survive a redeploy (sign-in, tool-call and import records, and both start-up lines).
- The live log view during a deploy looks as before.
- Tempo shows traces for an import, a board page, the live connection and a full sign-in, with the Google and image store calls as their own parts.
- In Grafana you can go from an error record to its trace and back.
- One trace holds Cloudflare's and the playmat's spans for a board page, or the runbook records why not.
- With wrong credentials, the failure line appears after five minutes, at most every five minutes, with no credential, and players are unaffected.
- The dashboard's daily counts are still there after 28 days.
- A search of the stack after a test session finds no email, token, pasted log text or card names.
- A walk-through of the runbook works.
- A privacy page review on the live site.
