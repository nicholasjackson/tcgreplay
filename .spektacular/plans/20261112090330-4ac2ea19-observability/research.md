---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Research: 20261112090330-4ac2ea19-observability

## Alternatives considered and rejected

- **Reading daily usage from DigitalOcean's own runtime log viewer alone** (the spec's Technical Approach preference). DigitalOcean's docs say runtime logs are not retained: "To retain runtime logs, configure log forwarding to an external provider" (docs.digitalocean.com/products/app-platform/how-to/view-logs/); the 90-day retention there is for build and deploy logs only. Community reports say run logs are cleared on redeploy, and this app redeploys on every push to main outside cardsets/ (`.do/app.yaml:1-40`, `.github/workflows/deploy.yml`). So a week of individual records cannot be read back from the log viewer. **Resolved: the user chose DigitalOcean Insights, whose Logs tab does show the playmat's runtime logs (user-confirmed), so records are read there rather than in the App Platform log viewer.**
- **A running count in the playmat's memory, shown on /admin or written as a daily summary record.** Lost on every restart and redeploy (spec Technical Approach); undercounts distinct players across a deploy.
- **Persisting daily counts in the existing Spaces bucket** (`internal/spaces`, `TCGREPLAY_SPACES_*`). Contradicts the spec constraint "no database or other persistent storage; recording usage and errors must not add any".
- **Third-party log forwarding (Better Stack, Datadog, Papertrail) or Sentry.** Rejected by the maintainer in the spec's Constraints.
- **Pseudonymous ID from the Google `sub` instead of the email.** The spec's Constraints fix it as a one-way keyed hash of the email address.
- **Reusing `TCGREPLAY_SIGNING_KEY` as the hash key.** It is optional (a random key is made up when unset, `internal/hostauth/seal.go:30-36`), and rotating it to sign everyone out would also re-key every player's ID; a dedicated secret keeps the two concerns apart.
- **Logging tool error messages verbatim.** Playmat error messages interpolate battle-log lines (`internal/importer/parse.go:137` quotes the line with %q), card names (`internal/importer/replay.go:207-219`, `internal/importer/handlers.go:725,780,850`) and TCG Live player names (`internal/importer/replay.go:315,364-366`). Verbatim messages would break "Records never identify a player" and the "No identifying data" acceptance criterion.
- **Emitting usage events from inside each tool handler.** 40+ tools in `internal/mcpserver/server.go`; a single MCP receiving middleware sees every `tools/call` (name, arguments, session client info, token, result) in one place, as `hostauth.MCPMiddleware` already does (`internal/hostauth/limit.go:92-119`).

- **DigitalOcean Insights as the store for usage and error records** (the user's pointer, 2026-11-12). Insights (public preview since 2026-10-01, free during preview, priced at GA) gives metrics, logs, traces, dashboards and metric alerts for *supported* resources. Its launch post says "DOKS and App Platform report their metrics into the same views, along with logs for DOKS", so App Platform contributes metrics, not runtime logs; custom telemetry from your own applications is only planned ("We plan to continue expanding coverage ... including custom telemetry from your own applications"). No public log-ingest or OTLP endpoint exists (the API covers alert rules, dashboards, notification channels, PromQL metric queries, logs/traces search and signal enablement only). Log search spans at most 7 days per query, and there is no documented group-by/distinct; retention for logs and metrics is not documented (only "Alert history is available for up to 30 days"). The limits page says it "should not be used as the primary observability system for production workloads". **Resolved (user, 2026-11-12): the playmat's own runtime log lines DO appear in the maintainer's Insights console (Logs tab, LON1), despite the docs. Insights is the chosen home for records, metrics, graphs and alerts; preview status and charging at GA are accepted.**

## Chosen approach — evidence

- MCP receiving middleware is already the per-request choke point: `srv.AddReceivingMiddleware(a.MCPMiddleware())` in `cmd/tcgreplay/hosted.go:265`; the middleware reads the account from `req.GetExtra().TokenInfo` (`internal/hostauth/limit.go:101-105`).
- The assistant name is already derived from the MCP client's `ClientInfo.Name` via `mcpserver.AssistantName` (`internal/mcpserver/server.go:126-133,159-172`), reachable from `req.Session.InitializeParams()`.
- go-sdk v1.6.0 turns a handler's plain error into a `CallToolResult` with `IsError=true` (`mcp/server.go:343-353`), so a middleware wrapping `tools/call` sees every tool failure as an IsError result whose text content is the error message; bind failures (busy, sign-in) already come back the same way (`internal/mcpserver/server.go:211-215`).
- Every email-bearing log line today: `internal/hostauth/google.go:93,231`, `internal/hostauth/board.go:132`, `internal/hostauth/admin.go:42,51`, `internal/hostauth/limit.go:86`, `internal/usergames/usergames.go:123,134,193,267`, `internal/usergames/board.go:34`, and the cache admin's `by` (the admin's email, from `internal/cacheadmin/handler.go:40-43`) in `internal/cacheadmin/cacheadmin.go:189,203,243,245`. The token also carries the email in `TokenInfo.Extra` (`internal/hostauth/oauth.go:460`), which is not logged.
- Hosted settings are read from the environment in `cmd/tcgreplay/hosted.go:117-134` (secrets only from env, never flags), and declared in `.do/app.yaml` with `type: SECRET`.
- The admin list and guard exist: `Auth.IsAdmin` / `RequireAdmin` (`internal/hostauth/admin.go:10-58`); the admin page is `internal/cacheadmin` served at `/admin` and `/admin/` behind `RequireAdmin` (`cmd/tcgreplay/hosted.go:190-193`), with POSTs already checked same-site.
- An existing keyed/domain-separated hash pattern: `usergames.Handle` (`internal/usergames/usergames.go:322-329`) and `sealer` key derivation (`internal/hostauth/seal.go:35`).
- App Platform alert rules can be declared in the app spec at component level: `CPU_UTILIZATION`, `MEM_UTILIZATION`, `RESTART_COUNT` with `operator`, `value`, `window` (FIVE_MINUTES..ONE_HOUR); destinations are not in the spec and are set in the control panel (docs.digitalocean.com/products/app-platform/reference/app-spec/).
- Instance today: `apps-s-1vcpu-0.5gb`, `instance_count: 1`, `TCGREPLAY_MAX_USERS=10` (`.do/app.yaml`). The site build reads the limits from `.do/app.yaml` (`Makefile` site target comment), so a changed MAX_USERS flows to the website.

## Files examined

- `cmd/tcgreplay/hosted.go:117-134` — hostedAuth reads secrets from env; `:182-205` route table; `:236-266` newHosted wires registry, auth, MCP server and middleware; `:350-400` runHosted start-up log.
- `internal/hostauth/hostauth.go` — Config, Account{ID, Email, EmailVerified}, AccountFrom/WithAccount.
- `internal/hostauth/google.go:62-112,212-233` — Google callback; sign-in log line with email.
- `internal/hostauth/board.go:124-136` — sign-out log line with email.
- `internal/hostauth/admin.go` — IsAdmin, RequireAdmin, admin refusal lines with email.
- `internal/hostauth/limit.go` — rate-limit log with email; MCPMiddleware pattern.
- `internal/hostauth/oauth.go:433-473` — RequireToken, verify, AccountFromToken (email in TokenInfo.Extra).
- `internal/hostauth/seal.go` — signing key handling, domain-separated key derivation.
- `internal/usergames/usergames.go` — Admit/Release/expire/sweep log lines with email; Handle hash.
- `internal/usergames/board.go:34` — "opened another account's board link" log line with email.
- `internal/usergames/mcp.go` — ResolveMCP from token.
- `internal/mcpserver/server.go:43-240,470-505` — MCP() setup, bind/assistant name, addWith, new_game and import_log handlers.
- `internal/mcpserver/guides.go` — guide tool registered directly with mcp.AddTool (not through add), so it is a tools/call too.
- `internal/cacheadmin/handler.go`, `cacheadmin.go:108-130,189-245` — admin page handler; Logf with admin email.
- `internal/importer/*.go`, `internal/game/*.go` — 156 fmt.Errorf sites; messages interpolate log lines, card names and player names.
- `.do/app.yaml` — single instance, 0.5 GB, envs, no alerts, no log destinations.
- `Makefile` — `make test` runs go vet, go test, web and site e2e.

- DigitalOcean Insights (docs.digitalocean.com/products/insights/, features, limits, pricing, availability, view-logs, quickstart, API reference /v2/insights/...; blog digitalocean.com/blog/introducing-insights, updated 2026-10-12) — App Platform metrics (CPU, memory, restarts, request rate, P95, CDN status codes) are in Insights; App Platform logs are not listed; metric alerts send to email, Slack or webhook channels; LON1 supported; no custom telemetry yet.
- docs.digitalocean.com/products/app-platform/how-to/view-insights/ — the app's own Insights tab: CPU, memory, restart count, request rate, P95 duration, CDN throughput and latency by status code.

## External references

- docs.digitalocean.com/products/app-platform/how-to/view-logs/ — runtime logs are not retained without forwarding; build/deploy logs kept 90 days. Decides whether the log viewer can hold daily usage.
- docs.digitalocean.com/products/app-platform/how-to/enable-log-forwarding/ — forwarding destinations: DigitalOcean Managed OpenSearch, OpenSearch, Datadog, Better Stack.
- docs.digitalocean.com/reference/pydo/reference/apps/get_logs_aggregate/ — RUN = live run-time logs, RUN_RESTARTED = crashed instances' logs; archived links for past deployments, no retention stated.
- docs.digitalocean.com/products/app-platform/reference/app-spec/ — component alerts (CPU_UTILIZATION, MEM_UTILIZATION, RESTART_COUNT), operators and windows; no destinations in the spec.
- go-sdk v1.6.0 `mcp/server.go:320-353` — handler errors become IsError tool results.

## Prior plans / specs consulted

- Spec `20261112083238-29247b36-privacy-terms-disclaimer` — depends on this spec for what is recorded and how long it is kept; the privacy page must state the retention periods this work sets.
- Knowledge `gotchas/go-sdk-sessions-and-auth.md` — sessions are bound to TokenInfo.UserID; look per-user state up on every request from RequestExtra.TokenInfo, never at session creation.

## Open assumptions

- App Platform's own log viewer keeps no runtime-log history; Insights does hold the playmat's runtime logs (user-confirmed in the console). Insights' retention for logs and metrics is undocumented: a person task confirms and records it. If Insights turns out to keep logs for less than a day or drop them on redeploy, the implement workflow must STOP and ask.
- Insights parses a JSON log line's fields into attributes that can be filtered (`LogAttributes['key']` in the API, "+ Add to filters" in the console). Not yet verified for App Platform lines: verified as part of the manual checks. If the body stays a plain string, the text search on `"event":"tool_call"` still filters.
- Distinct players a day are read as the count of `player_day` records for that UTC day (Insights has no distinct/group-by). A restart during the day may re-emit one for a player already counted, slightly over-counting.
- Error messages are recorded with user-supplied text redacted (call arguments, card names, player names), which is assumed to satisfy both "the error message" and "no card data / battle log".
- App Platform captures the container's stdout/stderr as runtime logs, one line per record, so JSON lines on stdout are filterable in the log viewer.

## Drafting assumptions

### Records are read in DigitalOcean Insights (discovery)
- **Decision**: The playmat's JSON records are read in DigitalOcean Insights' Logs tab (LON1); graphs and alerts use App Platform metrics and alert rules. Settled by the user through the orchestrator after two questions (App Platform's own log viewer keeps no history; the user confirmed Insights shows the playmat's runtime logs).
- **Rationale**: The only DigitalOcean place that keeps the playmat's runtime logs across redeploys without a forwarding service or our own storage.
- **Rejected**: Managed OpenSearch forwarding (cost, relaxes "no log forwarding service"), in-memory/admin counts only (lost on deploy), Spaces-persisted counts (breaks "no persistent storage").

### Error messages are redacted, not dropped (discovery)
- **Decision**: An error record carries the error message with user-supplied text redacted: every word run that also appears in the call's string arguments, the current game's player names or its card names is replaced by `…`.
- **Rationale**: Playmat error messages interpolate battle-log lines, card names and TCG Live player names; the spec requires both "the error message" and "no battle log, decklist or card data". Redaction keeps the message's shape (which check failed) while honouring the privacy requirement and its acceptance criterion.
- **Rejected**: Verbatim messages (break "Records never identify a player"); dropping the message (breaks "Errors recorded with context").

### Chosen direction: a telemetry package, one MCP middleware, JSON lines to stdout read in Insights (architecture)
- **Decision**: New `internal/telemetry` (keyed player IDs, JSON-line recorder, Redact). Tool calls, imports, new games and tool errors are recorded by one MCP receiving middleware built by `usergames.Registry`. `player_day` is written from `Registry.Use`. Existing email-bearing log lines become structured records keyed by player ID. Board action failures go through a new `webui.Server.OnError` hook. Alerts are declared under `alerts:` in `.do/app.yaml`, and a runbook goes in `docs/observability.md`.
- **Rationale**: A single choke point for all tool calls (the existing middleware pattern, `internal/hostauth/limit.go:92-119`); no storage of our own; the activity clock already defines "active".
- **Rejected**: Recording in each tool handler (40+ sites); slog's default JSON shape (`msg` would carry the event, and field order is not fixed); counts on the admin page (lost on restart).

### Player ID key is a new required secret (architecture)
- **Decision**: `TCGREPLAY_PLAYER_ID_KEY`, read only from the environment, at least 32 characters. Hosted mode refuses to start without it.
- **Rationale**: The spec requires IDs that do not change across restarts and redeploys. A random fallback would silently break that and the admin lookup. Requiring the key mirrors how hosted mode already refuses to run without the Google client.
- **Rejected**: Deriving it from `TCGREPLAY_SIGNING_KEY` (optional, and rotating it to sign everyone out would also re-key every ID); a random key with a warning (IDs change on every deploy).

### Player ID format (architecture)
- **Decision**: `p_` + the first 16 hex characters (64 bits) of HMAC-SHA256(key, "tcgreplay player\x00" + lower(trim(email))).
- **Rationale**: Short enough to read and paste, with negligible collision risk at this scale. The prefix makes IDs easy to search for and to tell apart from board handles.
- **Rejected**: The full 64-character hex (unwieldy); base64 (characters that complicate searching).

### Admin lookup on the existing admin page (architecture)
- **Decision**: A "Find a player's ID" form on `/admin`, POSTing to `/admin/player`, served by `cacheadmin` behind `RequireAdmin`, with the hashing function injected.
- **Rationale**: The spec requires the existing Google sign-in and admin list; RequireAdmin already refuses non-admins, signed-out callers and cross-site posts; POST keeps emails out of URLs.
- **Rejected**: A separate admin page or package (more routing for a single form); GET with a query string (puts the email in URLs).

### Runbook location (architecture)
- **Decision**: A new `docs/observability.md` holds the event catalogue, the Insights filters for each daily number and for a player's errors, the alert rules, the capacity measurement and the retention periods. The capacity choice is also summarised in the `.do/app.yaml` comments beside `TCGREPLAY_MAX_USERS` and `instance_size_slug`.
- **Rationale**: The spec requires these to be "recorded by the project", and the privacy spec reads them. `docs/` holds no Markdown yet, but it is the repo's natural place for operator docs. `.do/app.yaml` already explains its settings in comments.
- **Rejected**: A knowledge entry only (not where an operator or the privacy page author would look first); the README (already long, and aimed at players).

### Conventions selected (architecture)
- **Decision**: Keep only the guides/packaging convention, as a guard on the build-context change risk; there is no logging or telemetry convention in the knowledge base.
- **Rationale**: It is the only loaded convention, and this plan touches neither guides nor the Dockerfile.
- **Rejected**: n/a.

### Closed event catalogue with an admin_action event (data_structures)
- **Decision**: 14 fixed event names (table in Data Structures), including `admin_action` for admin card-set checks, image clears and player lookups, and `sign_in_refused`, `account_removed` and `board_refused` for the other email-bearing lines.
- **Rationale**: The spec asks for a fixed event name and fields so the log viewer can filter. Every existing email-bearing line needs a home once emails are gone.
- **Rejected**: Free-form messages carrying `player=` (harder to filter); dropping the lines that are not usage (loses diagnostics the maintainer has today).

### Initial alert thresholds (data_structures)
- **Decision**: `RESTART_COUNT` > 0 over 5 minutes. `CPU_UTILIZATION` > 80% and `MEM_UTILIZATION` > 80% over 10 minutes, as starting values the maintainer edits in `.do/app.yaml`.
- **Rationale**: The spec leaves the thresholds to the maintainer. These are conventional starting points with headroom before the 0.5 GB instance runs out of memory.
- **Rejected**: Leaving the thresholds blank (the app spec requires a value).

### Headroom rule for launch capacity (tasks)
- **Decision**: Set the player limit so that, with every place filled, memory use stays at or below 70% of the instance's memory. That leaves at least 30% free and sits under the 80% memory alert.
- **Rationale**: The spec asks for headroom but gives no figure. 30% covers the Go runtime's heap growth and a burst during an import, and keeps the memory alert from firing at a normal full house.
- **Rejected**: 50% (wastes an instance size); 10% (an import burst could exhaust memory).

### Deploy, measure and apply are person tasks split around agent work (tasks)
- **Decision**: One person task sets the secret and alert recipients, applies the app definition, deploys, measures and confirms retention. An agent task records the figures and sets the limits. A final person task applies the new limits.
- **Rationale**: Secrets, the control panel and production deploys need a person; writing the runbook and editing the app definition do not. Mixed work is split, as the workflow requires.
- **Rejected**: One person task that also edits the repo (mixes executors).

## Rehydration cues

- `spektacular spec file read 20261112090330-4ac2ea19-observability`
- Re-read `cmd/tcgreplay/hosted.go`, `internal/hostauth/limit.go` (middleware pattern), `internal/mcpserver/server.go:100-240`, `internal/usergames/usergames.go`.
- `grep -rn 'Email' internal/hostauth internal/usergames internal/cacheadmin --include='*.go' | grep log` to re-list email-bearing log lines.
- `spektacular knowledge read --data '{"tier":"repo","name":"tcgreplay","path":"gotchas/go-sdk-sessions-and-auth.md"}'`
