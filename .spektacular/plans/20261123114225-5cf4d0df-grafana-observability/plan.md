---
created_date: "2026-11-23"
document_status: final
closed_date: "2026-11-23"
---

# Plan: 20261123114225-5cf4d0df-grafana-observability

<!-- Metadata -->
<!-- Created: 2026-11-23T12:40:14Z -->
<!-- Commit: 5d8311b -->
<!-- Branch: studio-design-alignment -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

The hosted playmat sends its existing usage and error records, its plain log lines, a trace of every request and a handful of usage counters to the maintainer's Grafana Cloud stack (Germany, free tier) over OpenTelemetry. This gives the maintainer searchable history that survives redeploys, request timing from Cloudflare's edge through the playmat to Google, the card data and the image store, and daily usage counts kept for six weeks. Players are still named only by their pseudonymous ID, are never slowed when Grafana is unreachable, and the desktop playmat sends nothing. The runbook and the privacy page move from DigitalOcean Insights to Grafana.

## Conventions

- **Update the privacy page with any new outside resource or remembered setting** — Grafana Cloud becomes a new outside service holding diagnostics, so the privacy page's diagnostics, retention and outside-services sections change in the same change.
- **Every player-visible change ships with a What's new entry** — the privacy page is a website page players read, so its change ships with a `site/src/data/whats-new.yaml` entry (`refresh: false`); the telemetry plumbing itself is internal and needs none.
- **Guide text and packaging** — the Docker build context must keep `internal/guides` (and gains nothing new to exclude); no guide text changes, so the guide word rule is untouched.

## Architecture & Design Decisions

**One telemetry package, built only by hosted mode, feeding Grafana Cloud over OTLP from the server.** A new `internal/observe` package owns everything OpenTelemetry: reading the optional `TCGREPLAY_GRAFANA_*` settings, building the trace, log and meter providers with OTLP/HTTP exporters to the Grafana Cloud stack, the export-health watch, and the small adapters the rest of the playmat uses (an HTTP server wrapper, an HTTP client transport, an MCP middleware, a board-action span hook). `runHosted` is its only caller. The desktop, daemon, shim and in-process modes never build it, so they hold no exporter and make no connection (spec "Hosted only"). With the settings unset or incomplete the package hands back no-op providers and the start-up line says export is off and why, so a deploy never depends on the console having been changed first (`gotchas/app-spec-not-applied-by-deploy.md`). Other packages see only interfaces, hooks or the OTel API (`webui`, `cards` and `hostauth` take an optional hook or an injected `*http.Client`), never the SDK.

**Logs: the records that exist today, sent twice.** The playmat's usage and error records keep their single source, `telemetry.Recorder`, which still writes each JSON line to standard output exactly as today, so App Platform's live view is unchanged. The recorder gains an optional sink, set by hosted mode, that receives each finished line (already hashed and redacted) together with the request's context. The sink emits it as an OTel log record whose body is the same JSON line and whose attributes are its fields, so the request's `trace_id` and `span_id` ride with it into Loki. The plain start-up, card-set and image lines written through the standard `log` package reach Grafana through a line writer added beside standard error. Nothing new is written about players: the sink sees only what the recorder already writes, the player ID and never an email. `Event` gains a `Provider` field (`google` today), the same field the sign-in page plan adds, so the two plans can land in either order. Record call sites that have a request in hand pass its context (`RecordContext`, `ActiveContext`). The few with none (`signIn`, `allow`, `Use`, `Admit`) gain a context parameter, and background sweeps keep the context-free calls.

**Traces: every request, with its outside calls, joined to the edge.** One `otelhttp` server wrapper goes around the whole hosted handler, in a function the tests share. It names each span by the mux pattern the request matched and accepts the W3C `traceparent` that Cloudflare forwards, so edge and playmat spans share a trace. The sampler is always-on rather than parent-based, so Cloudflare's own sampling never drops a playmat span (spec: no sampling at launch). The spans the spec counts are covered as follows:
- **Assistant tool calls.** go-sdk hands every tool call the initialize request's context, so a span taken from that context would be wrong. A thin HTTP wrapper inside `RequireToken` writes the current span's `traceparent` into the request headers. An outermost MCP middleware reads it back from `RequestExtra.Header` and starts a `tools/call <tool>` span. That span is the context for the rate limit, the recorder and the tool (`gotchas/go-sdk-sessions-and-auth.md`).
- **Board actions.** Each websocket message gets its own short `board <verb>` span, linked to the long-lived `/ws` connection span.
- **Outside calls.** Outbound calls get `otelhttp` client transports injected into the clients that already accept one: Google's token exchange, card set fetches, the image store, CDN purges and TCGdex image fetches.
- **Image copies.** A card request that queues an image copy carries its span context into the copy job, so the image store call shows as part of that request's trace.

A span exporter filter keeps an allow-list of attributes and strips query strings. Client addresses, user agents, Google's authorization `code`, OAuth `state` and board handles therefore never leave the process. Span error descriptions carry only the recorder's redacted message.

**Metrics: a handful of counters, and daily totals carried past Grafana's 14 days.** The recorder sink also increments counters derived from the same records: player-days, sign-ins by provider, tool calls by assistant and tool, imports and new games by assistant, errors by kind, and limits by reason. Assistant names outside the four known ones count as `other`, so a client's self-chosen name cannot multiply series. `otelhttp`'s own HTTP metrics are turned off, because per-route histograms would spend thousands of the Free tier's 10k active series. Grafana Cloud Free keeps metrics for only 14 days (research), so a usage-history job in `observe` keeps the spec's 28 days inside Grafana itself. Every hour, and at start-up, it reads back the latest `tcgreplay_usage_daily` values from the stack's metrics query API. It adds each completed UTC day still inside the raw window, computed by `increase()` over that day, and drops days older than six weeks. An observable gauge then re-exports the whole table, one series per day and count (about 800 series). Grafana is the only store, so a restart loses nothing and the playmat keeps no storage of its own (spec "No new storage"). A dashboard definition checked into the repo charts the counts per day.

**Bounded, never in the way.** Spans and log records go to the SDK's batch processors (queues of 2,048 that drop rather than block), and metrics to a 60-second periodic reader. Exporters use a 5-second timeout and no retries, so a slow or failing Grafana costs the request path nothing and memory stays flat. Every exporter is wrapped by a health watch. When exports have failed continuously for more than five minutes it writes one plain line to standard error, at most every five minutes, naming the signals and the failure kind (unreachable, timeout, HTTP status) and never the token or the error text. Shutdown flushes for at most two seconds. This direction reuses the existing records, the injectable HTTP clients and the existing optional-setting pattern. A collector, log forwarding, a slog migration, metric retention left to Grafana and hand-rolled OTLP were all weighed and rejected (see `research.md#alternatives-considered-and-rejected`).

**Edge, runbook and privacy page.** Cloudflare's part is configuration, not code: domain Traces on the tcgreplay.app zone, a Grafana Cloud OTLP destination, and "Forward to origin" on. The maintainer does this in the dashboard. If OTLP export is not offered on the zone's Free plan, the edge requirement is dropped and the reason recorded in the runbook, as the spec allows. `docs/observability.md` is rewritten around Grafana, keeping its record catalogue and player-ID sections. The privacy page names Grafana Cloud, Germany and the retention periods, which the browser suite ties to the runbook's Retention section (`conventions/privacy-page-tracks-outside-services.md`). A What's new entry announces the privacy change (`conventions/whats-new-entries.md`).

## Component Breakdown

- **Export settings and status (new, observe).** Reads the optional Grafana settings, decides whether export is on, off or incomplete, and produces the start-up status text. It never includes the token. Incomplete settings give a warning, never an error. Hosted wiring asks it first, and every other observe component is built from its result.
- **Telemetry providers (new, observe).** Builds the trace, log and meter providers and their OTLP exporters for the Grafana stack: an always-on sampler, bounded batch queues, a short timeout, no retries, the service resource (name, version, an instance ID per start), and a two-second flush on shutdown. When export is off it hands out no-op providers, so the adapters below cost nothing. The rest of hosted mode takes its tracer, logger and meter from here, never from OTel globals, so tests stay isolated.
- **Span privacy filter (new, observe).** Sits between the trace providers and the exporter. It passes only allow-listed span attributes, strips query strings from URLs and error descriptions, and drops library span events. It is the single guarantee that no client address, user agent, authorization code, state or board handle leaves in a trace.
- **Export health watch (new, observe).** Wraps every exporter to note successes and failures. After five minutes of unbroken failure it writes one plain line to standard error naming the signals and the kind of failure, and repeats it at most every five minutes. The line goes only to the live log view, never through export, and never carries credentials or raw error text.
- **Record sink (new, observe).** Implements the recorder's sink. Each finished usage or error record becomes a log record (the same JSON line as its body, its fields as attributes, the request's trace and span IDs) and increments the matching usage counter. It is the only bridge between the existing records and Grafana.
- **Log line writer (new, observe).** Sits beside standard error as a second destination of the standard `log` package. It turns each plain playmat log line (start-up, card sets, images, warnings) into a log record, at warning severity for lines that start "warning:".
- **Usage history (new, observe).** Keeps the daily totals readable for six weeks inside Grafana. Hourly and at start-up it reads back the carried daily table from the stack's metrics query API, adds each completed UTC day from the raw counters, drops days past six weeks, and re-exports the table as a day-labelled gauge. It is optional: without the metrics query settings it is off and says so at start-up. A failed read keeps the table it has.
- **Request tracing adapters (new, observe).** Three pieces cover the request path:
  - the server wrapper for the whole hosted handler, which names spans by route and accepts incoming W3C trace context;
  - the client transport for outbound calls;
  - for tool calls, the header carrier that puts the current span's trace context into the MCP request headers, and the outermost MCP middleware that starts each tool call's span from those headers, with the tool, assistant and player ID.
  
  Hosted wiring places them, and the rest of the playmat only sees ordinary handlers, transports and middleware.
- **Usage and error recorder (changed, telemetry).** Keeps writing every record to standard output unchanged. It gains an optional sink, context-taking variants of recording and activity so records carry their request's trace, and a provider on events (sign-ins say `google`). It stays free of any OTel dependency.
- **Sign-in service (changed, hostauth).** Its HTTP client, used for the Google token exchange, now comes from hosted wiring with a tracing transport. The sign-in completion and the request limit take the request's context, so sign-in, refusal and limit records carry their trace. Sign-in records name the provider.
- **Per-account registry (changed, usergames).** Activity, admission, board refusals and board errors record with the request's or action's context. The MCP recorder middleware records within the tool call's span context, and marks the span failed with the redacted message when a tool fails.
- **Board server (changed, webui).** Gains an optional hook that wraps each board action in its own span, named by the action and linked to the live connection. The action's context reaches the limit and the error report. With no hook (desktop) nothing changes.
- **Card image cache (changed, cards).** A card request that queues an image copy passes its context, and the copy job runs under that request's span context, so the source fetch and the image store write appear in its trace. Its HTTP client gets the tracing transport in hosted mode.
- **Card catalogue loader, image store client and CDN purge client (changed by wiring only, catalog and spaces).** Hosted wiring hands each one an HTTP client with the tracing transport. Their own code does not change.
- **Hosted wiring (changed, cmd).** Builds the export settings and providers, sets the recorder's sink, tees the standard log output, wraps the handler, injects tracing clients, places the MCP middleware outermost, sets the board and image hooks, starts the usage history and health watch, adds the export and usage-history status to the start-up line, and flushes on shutdown. Desktop modes are untouched.
- **Usage dashboard (new, docs).** A Grafana dashboard definition kept in the repo. It charts per day the distinct players, sign-ins by provider, tool calls by assistant, imports, new games and errors from the carried daily table, plus the last 14 days from the raw counters. The maintainer imports it once.
- **Observability runbook (changed, docs).** Rewritten around Grafana Cloud. It covers setting up the stack token and settings, reading daily usage, finding a player's errors from their email through the admin lookup, and opening a request's trace and its logs. It also covers turning on Cloudflare Traces (or recording why the edge part was dropped), and the retention table the privacy page is checked against. The record catalogue and player-ID sections stay, gaining the `provider` field.
- **Privacy page and What's new (changed, site).** The diagnostics section names Grafana Cloud, Germany, and how long records and traces (14 days) and daily totals (six weeks) are kept. It no longer says diagnostics are kept in DigitalOcean Insights. The outside-services list gains Grafana. The browser suite checks the new figures against the runbook. A What's new entry announces the change.
- **App definition and image notes (changed, deploy files).** The app spec declares the new settings, with the token as a secret and no value, and the Dockerfile's settings comment lists them. Neither applies anything by itself: the maintainer sets the values in the console.

## Data Structures & Interfaces

**Settings (new, hosted environment).** All are optional and hosted-only. Export is on only when the first three are set and well-formed. The usage history is on only when export is on and the last two are set too.

| setting | role | secret |
|---|---|---|
| `TCGREPLAY_GRAFANA_OTLP_URL` | the stack's OTLP endpoint, as shown on its OpenTelemetry tile (`https://…grafana.net/otlp`) | no |
| `TCGREPLAY_GRAFANA_INSTANCE_ID` | the stack's instance ID (the Basic-auth user for OTLP) | no |
| `TCGREPLAY_GRAFANA_TOKEN` | an access policy token with `metrics:read`, `metrics:write`, `logs:write`, `traces:write` | yes |
| `TCGREPLAY_GRAFANA_METRICS_URL` | the stack's Prometheus query endpoint (`https://prometheus-…grafana.net/api/prom`) | no |
| `TCGREPLAY_GRAFANA_METRICS_USER` | the stack's metrics instance ID (the Basic-auth user for queries) | no |

**`observe.Settings` and `observe.Telemetry` (new).** The contract between hosted wiring and the telemetry package.

```go
type Settings struct { OTLPURL, InstanceID, Token, MetricsURL, MetricsUser string; Version string; Now func() time.Time; Stderr io.Writer }
func SettingsFromEnv(getenv func(string) string) Settings
func (s Settings) Status() (export, history string) // start-up text: on/off/incomplete and why; never the token

func Start(ctx context.Context, s Settings) *Telemetry // never fails: bad or missing settings give no-op providers
type Telemetry struct{ /* providers, health watch, usage history */ }
func (t *Telemetry) Enabled() bool
func (t *Telemetry) Sink() telemetry.Sink                       // for Recorder.Sink
func (t *Telemetry) LogWriter() io.Writer                        // second destination of the std log package
func (t *Telemetry) Handler(h http.Handler) http.Handler         // server spans, route names, incoming W3C trace context
func (t *Telemetry) CarryContext(h http.Handler) http.Handler    // inside RequireToken: writes traceparent into r.Header for MCP
func (t *Telemetry) MCPMiddleware(acct func(mcp.Request) (playerID, assistant string)) mcp.Middleware
func (t *Telemetry) Client(base *http.Client) *http.Client       // copy with a tracing transport (nil base: a 30s-timeout client)
func (t *Telemetry) BoardSpan(ctx context.Context, verb string) (context.Context, func(err error))
func (t *Telemetry) Shutdown(ctx context.Context) error          // flush, at most 2s
```

**`telemetry.Sink`, `telemetry.Line` and Recorder additions (changed).** The recorder's only outward contract. `telemetry` keeps no OTel dependency.

```go
type Line struct{ Time time.Time; Level, Event, Player, Tool, Action, Assistant, Message, Reason, Idle, Provider string; JSON []byte } // as written to stdout
type Sink interface{ Emit(ctx context.Context, l Line) }

type Event struct{ /* existing fields */; Provider string }  // "google" on sign_in; written as "provider", omitted when empty
func (r *Recorder) SetSink(s Sink)
func (r *Recorder) RecordContext(ctx context.Context, e Event) // Record(e) == RecordContext(context.Background(), e)
func (r *Recorder) ActiveContext(ctx context.Context, email string)
```

**Log records (wire, OTLP to Loki).** Each record has body = the stdout JSON line, severity INFO or ERROR (WARN for std-log "warning:" lines), and attributes `event`, `player`, `tool`, `action`, `assistant`, `reason`, `provider` (each only when set), plus `log.source` = `record` or `log`. Trace and span IDs come from the record's context. Resource: `service.name=tcgreplay`, `service.version`, `service.instance.id`, `deployment.environment.name=production`.

**Spans (wire, OTLP to Tempo).**

| span | kind | attributes beyond the allow-listed HTTP ones |
|---|---|---|
| `<METHOD> <route pattern>` (for example `GET /board`, `POST /mcp`, `GET /ws`) | server | `http.route` |
| `tools/call <tool>` | internal | `tcgreplay.tool`, `tcgreplay.assistant`, `tcgreplay.player` |
| `board <verb>` | internal, linked to its `/ws` span | `tcgreplay.board.verb`, `tcgreplay.player` |
| outbound HTTP (`POST oauth2.googleapis.com`, card set GET, image store PUT/HEAD, CDN purge, TCGdex GET) | client | `server.address`, `url.full` without its query |

Allow-list for HTTP attributes: `http.request.method`, `http.route`, `http.response.status_code`, `url.scheme`, `url.path`, `url.full` (query stripped), `server.address`, `server.port`, `network.protocol.version`, `error.type`, and anything under `tcgreplay.`. A failed span's status description is the redacted record message, or the HTTP status text.

**Metrics (wire, OTLP to Mimir; names as Grafana stores them).** All are cumulative counters except the daily gauge. Label values are closed sets.

| metric | labels |
|---|---|
| `tcgreplay_player_days_total` | none |
| `tcgreplay_sign_ins_total` | `provider` (`google`; `password` once the sign-in page lands) |
| `tcgreplay_tool_calls_total` | `assistant` (`Claude`, `ChatGPT`, `Codex`, `Gemini`, `other`), `tool` |
| `tcgreplay_imports_total`, `tcgreplay_new_games_total` | `assistant` |
| `tcgreplay_errors_total` | `kind` (`tool`, `board`) |
| `tcgreplay_limited_total` | `reason` (`rate`, `places`) |
| `tcgreplay_usage_daily` (gauge) | `day` (`YYYY-MM-DD`), `count` (`players`, `sign_ins`, `tool_calls`, `imports`, `new_games`, `errors`), plus `provider` or `assistant` where the count is split |

**Hooks into existing packages (changed).**

```go
// webui.Server
Trace func(ctx context.Context, verb string) (context.Context, func(error)) // nil: no spans (desktop)
// cards.ImageCache
func (c *ImageCache) ImageContext(ctx context.Context, src string) ImageRef // Image(src) == ImageContext(context.Background(), src)
// hostauth.Config.HTTPClient (existing field, now set by hosted wiring)
```

**Usage dashboard (new file format).** A Grafana dashboard JSON model whose panel queries name only the metrics above, checked by a Go test.

## Implementation Detail

**A new module boundary: `observe` is the only package that imports the OTel SDK.** Today the playmat has three direct dependencies (the websocket library, the MCP SDK and the OAuth client). This plan adds the OTel API and SDK, the three OTLP/HTTP exporters and the `otelhttp` contrib package, and keeps all of them behind `internal/observe`. A few packages touch the OTel API at all, and only where the request path crosses them:
- `cards` imports only the trace API, to carry a span context into a queued copy job;
- `webui` and `hostauth` take a plain hook or an injected `*http.Client`;
- `telemetry` keeps its rule of depending on nothing else in the playmat, and gains only a small sink interface.

A developer reading `cmd/tcgreplay/hosted.go` sees one `observe.Start` near the top of `runHosted`, then the handful of places its adapters are plugged in. That reads the same way as the existing optional services (card images, visitor counting) are wired: build, log status, plug in, run.

**Following the optional-setting pattern.** Export and the usage history behave like card image copies and visitor counting. They are decided once at start-up and reported in the start-up line ("telemetry export: on (Grafana Cloud otlp-gateway-…)", "off (no TCGREPLAY_GRAFANA_* settings)" or "off (incomplete: …)", plus "usage history: on/off"), and incomplete settings are a warning that never stops play. `Start` cannot fail: anything wrong gives no-op providers and an honest status. The token is read only from the environment, so it never shows in a process listing, and no status, error or health line ever contains it.

**One record, three destinations.** The recorder still owns what gets recorded and how it is redacted. After it writes a line to standard output it hands the same line to its sink, if one is set. This is the new seam: the sink is how logs and counters stay consistent with stdout by construction, and why no second event list is ever written. Record sites move from `Record(e)` to `RecordContext(ctx, e)` wherever a request or action is in hand. Where a record is made from inside a helper that never had a context (the sign-in completion, the request limit, account use and admission), the helper gains a context parameter, threaded from the request that already exists at its callers. Background sweeps (idle games, idle accounts, store expiry) keep context-free records, which simply carry no trace.

**Trace context crosses the MCP boundary through headers (new pattern).** The MCP SDK gives every tool call the session's original context. So the playmat treats MCP the way it would a remote hop: the HTTP layer writes the current span's `traceparent` into the request headers, and the outermost MCP middleware extracts it from the headers the SDK hands each call. Everything inside, from the rate limit to the recorder and the tools, then runs under the tool call's span. The middleware ordering rule the code already documents (one `AddReceivingMiddleware` call, outermost first) is followed: tracing, then the rate limit, then the recorder.

**Board actions and image copies get explicit spans (hooks, not imports).** The board server gains a nil-able trace hook around each websocket action, so a desktop board is byte-for-byte unchanged. The image cache gains a context-taking `Image` variant. The copy job remembers the requesting span's context and runs its fetch and store write under it, keeping the copy asynchronous.

**Privacy enforced at the edges of the process.** Three guards decide what leaves the process:
- **The recorder** decides what a record may say, exactly as today.
- **The span exporter filter** decides what a trace may say. It is an allow-list, so a new library attribute is dropped until someone adds it on purpose.
- **Metric labels** are closed sets.

The existing static check that no log call is handed an `.Email` is extended to span attributes and log attributes. An end-to-end test runs a real hosted session against a fake OTLP receiver and searches every byte received for the forbidden values.

**Never in the player's way.** Every export path is asynchronous and bounded: batch queues that drop, a periodic metric reader, exporters with a 5-second timeout and no retries. Neither ending a span nor emitting a record ever waits on the network. The health watch observes export results and only writes to standard error. The usage history's hourly read runs on its own goroutine with its own timeout, and a failure only keeps the table it already has.

**Usage history (new pattern: Grafana as the memory).** The playmat stores nothing. It treats its own previously exported `tcgreplay_usage_daily` series as the store, reads them back, merges in the newly completed days computed by PromQL from the raw counters, and exports the merged table again. Reading by maximum per day makes the merge idempotent, so two overlapping instances during a deploy write the same values.

**Docs follow the code.** The runbook keeps its structure (records, never-recorded, player IDs, daily usage, a player's errors, graphs and alerts, capacity, retention), with every "Insights" step replaced by its Grafana equivalent. The record catalogue gains `provider`, and a "Turning on edge traces" section is added. The privacy page's retention figures stay tied to the runbook by the existing browser check, extended to the new figures.

## Dependencies

- **Design documents this plan was built on: none.** The spec references no design document.
- **OpenTelemetry Go (new external libraries).** `go.opentelemetry.io/otel`, `otel/sdk`, `otel/sdk/metric`, `otel/log` and `otel/sdk/log` (v1.47.0, stable; the logs API and SDK reached 1.0 in this release); exporters `otlptracehttp` and `otlpmetrichttp` (v1.47.0) and `otlploghttp` (v0.23.0, still pre-1.0); `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` (v0.72.0, pre-1.0). Added to `go.mod`; pin these versions. Note v1.47.0's `WithEndpointURL` no longer appends the signal path, so pass full `/v1/<signal>` URLs.
- **Grafana Cloud stack (external service, existing, Free tier).** The maintainer's stack in a German region (`prod-eu-west-2` or `-4`). It receives OTLP logs (Loki), traces (Tempo) and metrics (Mimir), and answers PromQL queries for the usage history. Free limits: 50 GB logs, 50 GB traces, 10k active metric series a month, 14 days retention for all three. It needs an access policy token and the settings set in the console before export turns on. The code does not depend on that order.
- **Cloudflare (external service, existing zone).** Domain-level Traces (open beta) with a Grafana Cloud OTLP destination and "Forward to origin" on, for the edge part. Configuration only. Whether OTLP export is offered on the zone's Free plan is unverified. From 2026-12-01 the Free plan allows 0.5 GB/day of trace ingestion.
- **DigitalOcean App Platform (existing).** It keeps platform metrics and alerts unchanged (spec constraint), keeps showing stdout/stderr live, and stores the new encrypted settings. The app spec file records them, but deploys do not apply it (`gotchas/app-spec-not-applied-by-deploy.md`).
- **`internal/telemetry` (changed).** The sink, context-taking record and activity calls, and the event `Provider` field. Still no OTel dependency.
- **`internal/hostauth`, `internal/usergames`, `internal/webui`, `internal/cards` (changed, small).** Context threading, the board trace hook, the image copy context and the injected HTTP client.
- **`internal/catalog`, `internal/spaces` (unchanged code).** They receive tracing HTTP clients through their existing injectable fields.
- **MCP go-sdk v1.6.0 (unchanged).** Its per-call `RequestExtra.Header` carries the trace context. Its session context is the initialize request's (`gotchas/go-sdk-sessions-and-auth.md`).
- **`site/` and `docs/` (changed).** Privacy page, What's new, runbook, usage dashboard. Needs a site rebuild (`internal/site/dist`) and the site browser suite.
- **Prior plan `20261112090330-4ac2ea19-observability` (landed).** The record catalogue, player IDs, redaction and runbook this plan builds on. Nothing in it must change first.
- **Plan `20261122190321-c6d9d90a-sign-in-page` (approved, not implemented; not in this epic).** Either can land first. Both add `Provider` to `telemetry.Event` and the `provider` record field. Whichever lands second keeps the existing field rather than adding another, and the sign-in plan's `password` value flows into the `provider` metric label with no change here. The sign-in plan replaces `signIn(w, acct)` with `finishSignIn(w, r, pending, account)`: if it lands first, this plan's context threading goes into `finishSignIn`; if this plan lands first, the sign-in plan keeps the context parameter. The sign-in plan makes recorder callers pass `Account.PlayerKey()` instead of `Account.Email`, which leaves this plan's sink untouched. Shared files to merge with care: `internal/telemetry/recorder.go`, `internal/hostauth/google.go` and `limit.go`, `internal/usergames/usergames.go` and `telemetry.go`, `cmd/tcgreplay/hosted.go` (start-up line, `hostedAuth`), `docs/observability.md`, `site/src/pages/privacy.astro`, `site/src/data/whats-new.yaml`, `.do/app.yaml`, `Dockerfile`, and the hosted test helpers.

## Testing Approach

**Kinds of tests.** Unit tests sit in the new `observe` package and in `telemetry`. Integration tests run the real hosted server in-process against a **fake OTLP receiver** (an `httptest` server that accepts and keeps every OTLP/HTTP request body, with compression off) and a **fake Prometheus query API** for the usage history. Static checks extend the existing AST scan. A site browser test covers the privacy page. Everything slots into the existing conventions: the hosted suites' `startHosted` helpers and fake Google, `captureLog` for log output, the `eventRecords`/`wantEvent` record helpers, the subprocess `helperCmd` for start-up lines, and the site Playwright suite for the privacy page. No test reaches a real Grafana or Cloudflare. Tests use in-memory or fake exporters and never OTel globals, so parallel hosted tests cannot see each other's spans.

**Most coverage goes to privacy, non-blocking export and the trace joins.** These are where a silent failure would break the spec's promises.

**Load-bearing assertions:**
- **Records reach Grafana unchanged, and stdout still has them.** Every record written to stdout during a hosted session also arrives as an OTLP log record whose body is the same line. Plain log lines (the start-up line, card-set lines, warnings) arrive too. Stdout and stderr output is byte-for-byte what it is with export off, so the live log view is unchanged.
- **Every request is traced.** In a scripted session (full fake-Google sign-in through to the first MCP call, an import, a new game, opening the board, the live connection, a board action, a card request that triggers an image copy, an admin card-set check), the receiver holds a server span for every HTTP request made. It also holds a `tools/call <tool>` span per tool call, a `board <verb>` span per action linked to the `/ws` span, and client spans for the Google token exchange (inside the callback's trace), the card set fetch and the image store write (inside the card request's trace). None are missing (spec "Every request traced").
- **Tool spans belong to the request that carried them.** Two tool calls on one MCP session land in two different traces, each the trace of its own `POST /mcp`. This guards the go-sdk context trap.
- **Edge context is adopted.** A request carrying a `traceparent` (sampled or not) produces playmat spans in that trace ID, parented on that span.
- **Logs link to traces.** The `error` record from a failed import (a malformed battle log) carries the trace and span IDs of that tool call's span, and that span is marked failed with the redacted message.
- **Nothing identifies a player.** After the scripted session, plus a failed import of a log containing card and player names, no byte received by the fake receiver (traces, logs, metrics, resource) contains any of the following:
  - the test account's email;
  - the fake Google tokens or authorization code;
  - the OAuth state;
  - the board cookie or access token;
  - the board handle;
  - the client address or user agent;
  - the pasted battle log's text;
  - the deck's card names.

  The player appears only as `p_…`. The static check fails if any span or log attribute call is handed an `.Email`.
- **Telemetry never slows or stops players.** With the receiver unreachable, with one that hangs until the test ends, and with one that answers 401, sign-in, import, new game and board actions all succeed. A hung receiver adds no wait to a request beyond a small fixed allowance, and heap growth over many requests stays within the batch queues' bound. With settings unset or incomplete the server starts, serves, and makes no outbound telemetry connection.
- **Export state is visible.** The start-up line says export is on, off or incomplete (and the same for the usage history) and never contains the token. With a failing receiver and a fake clock, no failure line appears before five minutes; one appears after five minutes, no more than one per five minutes, and contains neither the token nor the Basic header value. The line clears after a successful export.
- **Usage is counted.** Each usage event increments its counter with the right labels. Sign-ins carry `provider="google"`, unknown assistants count as `other`, and the series count stays inside the closed label sets. The usage history merges the read-back table with newly computed days, keeps six weeks, is idempotent over repeated runs, keeps its table when the query API fails, and re-exports it as the daily gauge.
- **Desktop sends nothing.** The in-process and daemon modes, run with every `TCGREPLAY_GRAFANA_*` variable pointing at the fake receiver and used (a game, a board action), cause zero requests to it.
- **The dashboard matches the code.** Every query in the checked-in dashboard names only metrics and labels the code emits.
- **Privacy page and runbook agree.** The privacy page names Grafana Cloud, Germany and the retention figures, and no longer says diagnostics are kept in Insights. Each `data-retention` figure (logs, traces, totals, metrics) appears in the runbook's Retention section.

**Deliberate gaps.**
- No automated check of the latency percentage against real Grafana. The "no added wait with a hung receiver" test guards the mechanism, and the 5% figure is measured by hand.
- No test of Cloudflare's side, which is configuration.
- No test against Grafana's real query API. The fake follows its documented response shape.

**Spec success metrics:**
- *Find a player's error records and the failing request's trace from their email in under five minutes, without the hosting console.* **Manual — captured in the implementation test plan** (needs the live stack; the error-to-trace link and the admin lookup are guaranteed by behavioural tests).
- *First two weeks after launch: no deploy loses diagnostic history; every day has usage and error records searchable in Grafana.* **Manual — captured in the implementation test plan** (production observation; behavioural tests guarantee every record is exported and nothing is held on the instance).
- *Median tool-call duration with telemetry on stays within 5% of off.* **Manual — captured in the implementation test plan** (measured on the deployed playmat, on and off; the hung-receiver test guards against blocking).
- *The sign-in page's release can be judged from daily Google sign-in counts two weeks before and after, read from Grafana.* **Manual — captured in the implementation test plan** (production data; behavioural tests guarantee the provider label and the six-week daily table).
- *Monthly telemetry volume stays within the Grafana Cloud free tier.* **Manual — captured in the implementation test plan** (read from the stack's usage page after the first weeks; the closed label sets and disabled HTTP metrics are tested).

**Manual reviews and checks before acceptance:**
- **Manual — captured in the implementation test plan.** After setting the Grafana settings in the console and deploying, the start-up line says export and the usage history are on, and records, traces and metrics appear in the stack.
- **Manual — captured in the implementation test plan.** Logs survive a redeploy: sign in, import through an assistant, redeploy, and find the sign-in, tool-call and import records and both start-up lines in Grafana.
- **Manual — captured in the implementation test plan.** The live log view during a deploy shows the start-up record and later records as before.
- **Manual — captured in the implementation test plan.** In Tempo, the traces for an import tool call, a board page, the live connection and a full sign-in, with the Google call and an image store call as their own timed parts.
- **Manual — captured in the implementation test plan.** From an error record in Loki to its trace, and from the trace back to its logs.
- **Manual — captured in the implementation test plan.** Edge joined: a board page requested through tcgreplay.app shows Cloudflare's spans and the playmat's in one trace. Otherwise the runbook records why the edge part was dropped (plan or forwarding unavailable).
- **Manual — captured in the implementation test plan.** With wrong credentials set, the failure line appears in the live log view after five minutes, no more than every five minutes, with no credential, and players are unaffected.
- **Manual — captured in the implementation test plan.** The usage dashboard charts daily counts, and they are still present after 28 days (checked at day 28 or later).
- **Manual — captured in the implementation test plan.** A search of the stack (logs, traces, metrics) after a test session finds none of the test player's email, a Google token, a pasted log's text or a deck's card names.
- **Manual — captured in the implementation test plan.** A runbook walk-through: a day's distinct players and tool calls by assistant, a player's errors via the admin lookup, and opening one error's trace.
- **Manual — captured in the implementation test plan.** A privacy page review on the live site: Grafana Cloud, Germany, the retention figures, and no Insights claim.

## Milestones & Tasks

### Milestone 1: The playmat's records and log lines can be kept in Grafana across updates

**What changes**: The hosted playmat can now send every usage record, error record and plain log line (start-up, card sets, images, warnings) to the maintainer's Grafana Cloud stack, where they would survive redeploys and restarts. App Platform's live log view shows exactly what it shows today. Sending is controlled by new optional settings. With them unset or incomplete, the playmat starts and serves as before, and its start-up line says sending is off and why. When sending fails for more than five minutes, a line on the live log view says so, at most every five minutes, without credentials. A slow, failing or unreachable Grafana never delays or breaks a player's request, and nothing piles up in memory. The desktop playmat sends nothing anywhere. The records keep their current content, naming players only by their pseudonymous ID. Sign-in records now also say which provider was used (Google). Nothing is sent from the live site yet. The maintainer turns sending on in Milestone 4, once the privacy page describes Grafana.

**Validation point**: The hosted integration tests against a fake OTLP receiver pass. Every stdout record and log line arrives with an identical body, and stdout and stderr are unchanged. Unset, incomplete, unreachable, hanging and rejecting receivers leave sign-in, import and play working with no added wait and bounded memory. The start-up line reports on, off and incomplete without the token. The five-minute failure line behaves as specified. Desktop modes make no connection. The full Go suite passes.

#### - [x] Task: Telemetry export foundation
**Id:** be924ef6-41e1-46b5-9de9-8d103ec70991
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds the new telemetry package that reads the optional Grafana settings and decides whether sending is on. It builds bounded, non-blocking senders for logs, traces and metrics, and watches whether sending is failing. With settings missing or incomplete it gives harmless no-op senders and an honest status line. After five minutes of failed sending it writes one warning line to the live log view, at most every five minutes, without credentials. Nothing in the playmat uses it yet.

*Technical detail:* [context.md#task-telemetry-export-foundation](./context.md#task-telemetry-export-foundation)

**Acceptance criteria**:
- [x] With all three export settings well-formed, sending is on, and the status text names the Grafana host and never the token.
- [x] With the settings unset, the status says sending is off. With some missing or malformed, it says sending is off and which setting is wrong, without echoing the token. Neither case is an error.
- [x] Against a receiver that is unreachable, hangs or rejects, handing over data returns immediately, and queued data stays bounded (oldest or newest dropped, never unbounded growth).
- [x] Failure warnings appear only after five minutes of continuous failure, at most once per five minutes, stop after a successful send, and never contain the token, the Basic header value or raw error text.
- [x] Shutdown flushes for no more than two seconds.

#### - [x] Task: Records and log lines sent to Grafana
**Id:** bdd9d6b9-d974-40de-9bd3-f41b95b45084
**Repo:** tcgreplay
**Depends on:**
- be924ef6-41e1-46b5-9de9-8d103ec70991 — Telemetry export foundation
**Execution:** agent

Connects hosted mode to the export foundation. Every usage and error record the playmat writes, and every plain log line, is also sent as a log record, while standard output and standard error stay exactly as they are. Sign-in records name their provider (Google). The start-up line reports whether sending is on. The app definition and image notes list the new settings, with the token as a secret. The desktop playmat never builds any of this.

*Technical detail:* [context.md#task-records-and-log-lines-sent-to-grafana](./context.md#task-records-and-log-lines-sent-to-grafana)

**Acceptance criteria**:
- [x] In a hosted session against a fake receiver, every record printed to standard output arrives as a log record with the identical line as its body. The start-up line, card-set lines and warnings arrive too.
- [x] Standard output and standard error are unchanged by turning sending on, so the live log view looks as before.
- [x] Sign-in records carry `"provider":"google"`, and the record catalogue in the runbook lists the field.
- [x] With the receiver unreachable, hanging or rejecting, a player can sign in, import a game and play, with no added wait.
- [x] The start-up line says whether sending is on, off or incomplete, and never shows the token.
- [x] The desktop playmat, run with the Grafana settings present and used, makes no connection to the receiver.
- [x] The app definition declares the new settings with the token as a secret with no value, and the image's settings note lists them.

### Milestone 2: Every request is traced, from the edge to the services it calls, and its logs link to it

**What changes**: Each assistant tool call, board page, card data request, live board connection, board action and sign-in step produces a trace showing what the playmat did and how long each part took. That includes its calls to Google sign-in, the card set data, the card image store and the image sources. A request that arrives with Cloudflare's trace context joins Cloudflare's trace, so edge time and playmat time show in one place. Every request is traced, with no sampling. A record written while serving a request carries that request's trace identity, so an error record leads to its trace and a trace to its records. Traces never carry a client address, browser identity, sign-in code, board link handle, email, token, battle log or card names.

**Validation point**: The scripted hosted session yields a trace for every request, with the expected spans:
- one span per tool call, inside its own request's trace (two calls on one session give two traces);
- board actions linked to the live connection;
- the Google token exchange inside the sign-in callback's trace;
- the image store write inside the card request's trace that queued it.

A request carrying a `traceparent` is joined. The failed import's error record carries its tool span's IDs. The privacy sweep over everything received finds no forbidden value. The full Go suite passes.

#### - [x] Task: Every HTTP request and outside call traced
**Id:** da2717c3-abb6-4fb7-bb30-249843078c80
**Repo:** tcgreplay
**Depends on:**
- bdd9d6b9-d974-40de-9bd3-f41b95b45084 — Records and log lines sent to Grafana
**Execution:** agent

Wraps the hosted playmat so every incoming request becomes a trace named by its route, joining Cloudflare's trace when the request carries one. Calls the playmat makes to Google sign-in, the card set data, the image store, CDN purges and image sources become timed parts. A privacy filter makes sure traces carry no client address, browser identity, query string, sign-in code or board link handle.

*Technical detail:* [context.md#task-every-http-request-and-outside-call-traced](./context.md#task-every-http-request-and-outside-call-traced)

**Acceptance criteria**:
- [x] Every request to the hosted playmat (website, board page, card data, live connection, sign-in steps, MCP, admin) produces a server span named by its route.
- [x] A request carrying a W3C `traceparent`, sampled or not, produces spans in that trace, parented on the incoming span.
- [x] The Google token exchange appears as a timed part inside the sign-in callback's trace. Card set fetches, image store calls and CDN purges appear as client spans.
- [x] No span leaving the playmat carries a client address, user agent, query string, Google authorization code, OAuth state or board handle.
- [x] The live connection and MCP streams still work through the wrapper (websocket upgrade and streaming are not broken).
- [x] No per-route HTTP metrics are sent.

#### - [x] Task: Tool calls and board actions traced with records linked to their trace
**Id:** 81d1208a-8e94-4cd6-91c6-705abd72ed84
**Repo:** tcgreplay
**Depends on:**
- da2717c3-abb6-4fb7-bb30-249843078c80 — Every HTTP request and outside call traced
**Execution:** agent

Gives each assistant tool call a span inside the trace of the request that carried it, despite the MCP library handing every call the session's first context. Gives each board action its own span, linked to the live connection. Records written while serving a request (sign-ins, refusals, limits, tool calls, imports, errors, board errors, admin actions) now carry that request's trace identity. A failed tool call marks its span failed with the redacted message.

*Technical detail:* [context.md#task-tool-calls-and-board-actions-traced-with-records-linked-to-their-trace](./context.md#task-tool-calls-and-board-actions-traced-with-records-linked-to-their-trace)

**Acceptance criteria**:
- [x] Each tool call has a span naming the tool, the assistant and the player ID, inside the trace of its own MCP request. Two calls on one session land in two traces.
- [x] Calls refused by the rate limit are traced too, and their `limited` record carries the trace.
- [x] Each board action has its own span naming the action, linked to the live connection's span.
- [x] The error record of a failed import carries the trace and span IDs of that tool call's span, and the span is marked failed with the same redacted message.
- [x] Sign-in, sign-out, refusal, limit, board refusal and admin records carry the trace of the request that caused them. Background records (idle games, idle accounts) carry none and are otherwise unchanged.
- [x] The desktop board works exactly as before.

#### - [x] Task: Image copies joined to their card request and the end-to-end privacy sweep
**Id:** c74ae6a5-ef04-4ae7-87f3-5ed7dc3bb98e
**Repo:** tcgreplay
**Depends on:**
- 81d1208a-8e94-4cd6-91c6-705abd72ed84 — Tool calls and board actions traced with records linked to their trace
**Execution:** agent

Makes an image copy triggered by a card request show inside that request's trace, so the image store call is its own timed part, while the copy stays in the background. Adds the end-to-end test that drives a full hosted session against a fake receiver. It proves every request is traced, and that nothing identifying a player is ever sent. It also extends the existing check that no email reaches a log call to cover span and log attributes.

*Technical detail:* [context.md#task-image-copies-joined-to-their-card-request-and-the-end-to-end-privacy-sweep](./context.md#task-image-copies-joined-to-their-card-request-and-the-end-to-end-privacy-sweep)

**Acceptance criteria**:
- [x] A card request for an image with no copy yet shows the source fetch and the image store write inside its own trace, and the request itself does not wait for the copy.
- [x] In a scripted session (sign-in through to the first tool call, import, new game, board page, live connection, board action, card request, admin check), every HTTP request, tool call and board action appears as a span. None are missing.
- [x] Across everything the receiver got in that session plus a failed import of a log naming cards and players, there is no trace of the email, the Google tokens or code, the OAuth state, the board cookie or access token, the board handle, the client address or user agent, the battle log's text or the deck's card names. Players appear only as `p_…` IDs.
- [x] The static check fails if an email is passed to a span or log attribute.

### Milestone 3: Daily usage is counted and kept for six weeks

**What changes**: The playmat reports counts of distinct players, sign-ins (by provider), tool calls (by assistant and tool), imports, new games, errors and limit refusals as metrics. Grafana Cloud's free tier keeps metrics for only 14 days, so the playmat also keeps a daily table of those counts alive inside Grafana for six weeks. It reads the table back, adds each finished day and sends it again, and stores nothing itself. A Grafana dashboard kept in the repository charts every count per day. Once the sign-in page with password accounts ships, the sign-in chart can show Google sign-ins on their own with no further change here.

**Validation point**: The counter tests show each record increments the right counter with closed label sets. Usage history tests against a fake query API show merging, the six-week limit, idempotent repeats, survival of a failed read, and the re-exported daily gauge. The dashboard check finds only emitted metrics. The start-up line reports the usage history on or off. The full Go suite passes.

#### - [x] Task: Usage counted as metrics
**Id:** 966424fd-271c-44c1-808a-0ab19a42ff17
**Repo:** tcgreplay
**Depends on:**
- bdd9d6b9-d974-40de-9bd3-f41b95b45084 — Records and log lines sent to Grafana
**Execution:** agent

Counts usage as metrics derived from the same records: player-days, sign-ins by provider, tool calls by assistant and tool, imports and new games by assistant, errors by kind and limit refusals by reason. Label values are closed sets, so an unknown assistant counts as "other" and the free tier's series allowance is safe. A future password provider flows through with no change.

*Technical detail:* [context.md#task-usage-counted-as-metrics](./context.md#task-usage-counted-as-metrics)

**Acceptance criteria**:
- [x] Each usage record increments exactly one matching counter with the right labels, and records that are not usage (for example game expiry) increment none.
- [x] Sign-ins carry `provider="google"`, and a sign-in record naming another provider carries that provider.
- [x] Assistants other than Claude, ChatGPT, Codex and Gemini are counted as `other`, and tool names outside the registered tools as `other`.
- [x] Counters carry no player ID or other per-player label.

#### - [x] Task: Daily usage kept for six weeks with a dashboard
**Id:** df59d5c7-d090-4069-a8ab-275b9c938fa4
**Repo:** tcgreplay
**Depends on:**
- 966424fd-271c-44c1-808a-0ab19a42ff17 — Usage counted as metrics
**Execution:** agent

Keeps the daily totals readable for six weeks inside Grafana, although its free tier keeps metrics for only 14 days. Hourly and at start-up, the playmat reads back the daily table it sent before, adds each finished day computed from the counters, drops days older than six weeks and sends the table again. It stores nothing itself. Adds the Grafana dashboard that charts every count per day, and reports in the start-up line whether this is on.

*Technical detail:* [context.md#task-daily-usage-kept-for-six-weeks-with-a-dashboard](./context.md#task-daily-usage-kept-for-six-weeks-with-a-dashboard)

**Acceptance criteria**:
- [x] Against a fake metrics query API, the table read back is merged with newly finished days, repeated runs give the same table, and days older than six weeks are dropped.
- [x] When the query API is unreachable or rejects the request, the playmat keeps the table it has and keeps serving players. Failures count towards the five-minute warning.
- [x] The table is sent as one daily gauge series per day and count, with distinct players, sign-ins by provider, tool calls by assistant, imports, new games and errors.
- [x] Without the metrics query settings the usage history is off, the start-up line says so, and sending of everything else is unaffected.
- [x] The dashboard in the repository charts each count per day, and its queries name only metrics and labels the playmat sends.

### Milestone 4: The maintainer reads everything in Grafana, and the privacy page says so

**What changes**: The maintainer's observability runbook is rewritten around Grafana Cloud. It covers creating the stack's token and setting the hosted settings, reading daily usage, finding a player's errors from their email through the admin page's player-ID lookup, and opening a request's trace and its logs. It also covers turning on Cloudflare's edge traces, or recording why that part was dropped. The privacy page names Grafana Cloud as where diagnostics records and traces are kept, says they are stored in Germany for 14 days and daily totals for six weeks, and no longer says they are kept in DigitalOcean Insights. A What's new entry announces the privacy change. The maintainer then sets the Grafana settings in the console, turns on Cloudflare Traces, and imports the dashboard, and the live playmat starts sending.

**Validation point**: The site browser suite passes, with the privacy page's figures found in the runbook's Retention section and no Insights claim left. The What's new build check passes. After the maintainer's setup, the live start-up line reports export and the usage history on. The manual checks in the test plan (records surviving a redeploy, traces in Tempo, the error-to-trace link, edge joining or its recorded absence, the failure line with wrong credentials, the privacy search, the runbook walk-through) are carried out.

#### - [x] Task: Observability runbook rewritten for Grafana
**Id:** 84978786-4b62-413a-991e-76e70c58a5d0
**Repo:** tcgreplay
**Depends on:**
- c74ae6a5-ef04-4ae7-87f3-5ed7dc3bb98e — Image copies joined to their card request and the end-to-end privacy sweep
- df59d5c7-d090-4069-a8ab-275b9c938fa4 — Daily usage kept for six weeks with a dashboard
**Execution:** agent

Rewrites the maintainer's runbook around Grafana Cloud, replacing every DigitalOcean Insights instruction. It covers setting up the stack token and the hosted settings, reading daily usage from the dashboard and queries, finding a player's errors from their email through the admin page's ID lookup, and opening a request's trace and its logs. It also covers turning on Cloudflare's edge traces (or recording why not), what the export status and failure lines mean, and the retention table. Platform metrics and alerts stay in DigitalOcean.

*Technical detail:* [context.md#task-observability-runbook-rewritten-for-grafana](./context.md#task-observability-runbook-rewritten-for-grafana)

**Acceptance criteria**:
- [x] The runbook no longer tells the maintainer to read records in Insights. It says platform metrics and alerts still come from App Platform.
- [x] It explains, step by step, how to read a day's distinct players and tool calls by assistant, find a player's error records from their email via the admin lookup, and open the trace of one of those errors and its other log lines.
- [x] It explains how to create the token (with the scopes needed), which settings to set where, how to import the dashboard and how to confirm sending from the start-up line.
- [x] It explains how to turn on Cloudflare Traces with the Grafana destination and forwarding to the origin, and where to record the reason if the edge part is unavailable.
- [x] Its Retention section states 14 days for records and traces, six weeks for daily totals and the platform metrics' figure, with sources and the date checked.

#### - [x] Task: Privacy page names Grafana Cloud
**Id:** 353bd387-1cd6-4d25-af3b-9fdd4a299bb2
**Repo:** tcgreplay
**Depends on:**
- 84978786-4b62-413a-991e-76e70c58a5d0 — Observability runbook rewritten for Grafana
**Execution:** agent

Corrects the privacy page. Diagnostics records and request traces are kept in Grafana Cloud, in Germany, for 14 days, and daily usage totals (with no player ID) for six weeks. The claim that they are kept in DigitalOcean Insights goes. Grafana Cloud joins the outside services list. The browser check that ties the page's retention figures to the runbook is extended to the new figures. A What's new entry tells players.

*Technical detail:* [context.md#task-privacy-page-names-grafana-cloud](./context.md#task-privacy-page-names-grafana-cloud)

**Acceptance criteria**:
- [x] The privacy page names Grafana Cloud as where diagnostics records and traces are kept, says they are stored in Germany, and gives 14 days for records and traces and six weeks for daily totals.
- [x] It no longer says diagnostics are kept in DigitalOcean Insights. DigitalOcean is still named for hosting and its own platform metrics.
- [x] The outside services list includes Grafana Cloud, saying what it receives and that it never receives emails, tokens, battle logs, decklists or card text.
- [x] Each retention figure on the page appears in the runbook's Retention section, checked by the browser suite.
- [x] A What's new entry dated the release day announces the privacy change, with refresh set to false.

#### - [ ] Task: Turn on sending to the Grafana stack
**Id:** fcc30e32-90b2-4d53-9207-97d384edc211
**Repo:** tcgreplay
**Depends on:**
- 353bd387-1cd6-4d25-af3b-9fdd4a299bb2 — Privacy page names Grafana Cloud
**Execution:** human — needs the maintainer's Grafana Cloud account and the App Platform console (production secrets)

Once the privacy page describing Grafana is live, the maintainer creates the stack's access policy token and sets the Grafana settings as encrypted app settings in the App Platform console. The playmat restarts. The maintainer confirms its start-up line reports sending and the usage history on. They also confirm the stack is in a German region and import the usage dashboard.

*Technical detail:* [context.md#task-turn-on-sending-to-the-grafana-stack](./context.md#task-turn-on-sending-to-the-grafana-stack)

**Acceptance criteria**:
- [ ] The live playmat's start-up line says telemetry export is on (Grafana Cloud) and the usage history is on.
- [ ] Records, traces and metrics from the live playmat appear in the Grafana stack.
- [ ] The stack's region is confirmed as Germany, matching the privacy page.
- [ ] The usage dashboard is imported and shows data.

#### - [ ] Task: Turn on Cloudflare edge traces
**Id:** 30bb8630-091c-447c-82bb-9f5c10f9deee
**Repo:** tcgreplay
**Depends on:**
- fcc30e32-90b2-4d53-9207-97d384edc211 — Turn on sending to the Grafana stack
**Execution:** human — needs the maintainer's Cloudflare dashboard (zone settings outside the repo)

The maintainer turns on Cloudflare Traces for the tcgreplay.app zone without changing routing. They add a Grafana Cloud OTLP destination for traces, turn on forwarding trace context to the origin, and turn persistence off so the free daily allowance is not spent on Cloudflare's own storage. If the zone's plan does not offer OTLP export, or forwarded context does not reach the playmat, they record that and the reason in the runbook's edge section, and the edge requirement is dropped as the spec allows.

*Technical detail:* [context.md#task-turn-on-cloudflare-edge-traces](./context.md#task-turn-on-cloudflare-edge-traces)

**Acceptance criteria**:
- [ ] Either Cloudflare Traces exports to the Grafana stack with forwarding to the origin on and traffic routing unchanged, or the runbook records why the edge part is unavailable on the current plan.
- [ ] No paid Cloudflare plan is taken on.

## Open Questions

- **Does Grafana's metrics query API accept the shared access policy token for the usage history?** This depends on the live stack: whether one token with `metrics:read` and the write scopes authenticates against the Prometheus query endpoint with the metrics instance ID as user. Only the real stack shows it. When implementing, build against the documented Basic-auth form and the fake API. If the maintainer's setup task finds queries refused while export works, STOP and ask the user before adding a second token setting.
- **What exactly does Mimir name the counters and the daily gauge?** This depends on Grafana's OTLP translation (unit suffixes, `_total`), which the docs describe but which only a real ingest confirms. When implementing, follow the documented rules (`tcgreplay_<name>_total` for counters with unit `1`). If the stack shows different names after the setup task, correct the dashboard, runbook and history queries to the stored names; that is a fix, not a question. STOP only if a name collides with another metric.
- **Does a `traceparent` forwarded by Cloudflare survive App Platform's own edge to the container?** This depends on the live edge chain. If it does not, the edge requirement is dropped with that reason recorded in the runbook, as the spec allows. Do not add header-rewriting or routing changes to work around it.

## Out of Scope

- **Telemetry from the board in players' browsers.** Front-end errors, page timing and session replay are excluded (spec Non-Goal).
- **Public usage numbers or a stats page.** Excluded (spec Non-Goal). The dashboard is the maintainer's, inside their Grafana stack.
- **Cloudflare request logs (Logpush).** Only Cloudflare's traces are in scope (spec Non-Goal).
- **Alerting in Grafana.** The maintainer decided against it (spec Constraint). Restart, CPU and memory alerts stay in App Platform.
- **Moving platform metrics (CPU, memory, restarts, request rate) to Grafana.** They stay in DigitalOcean (spec Constraint).
- **Sampling.** Every request is traced at launch. Introducing sampling if volume nears the free tier is a later change, decided from the "monthly volume" check.
- **A paid Grafana or Cloudflare plan, a collector, or self-hosted monitoring.** Excluded by the spec's constraints. If Cloudflare's OTLP export is not on the Free plan, the edge part is dropped rather than bought.
- **Joining admin image-clear jobs to the admin request's trace.** The clear runs on the server's own context, and its CDN purges appear as separate traces. The spec does not ask for it.
- **The sign-in page's password provider.** Owned by plan `20261122190321-c6d9d90a-sign-in-page`. This plan only makes the `provider` field and metric label ready for it.
- **Changing what the records say.** The record catalogue, player IDs and redaction stay as they are, apart from the `provider` field. No new events are added.

## Changelog

### 2026-11-23 — Task: Telemetry export foundation

**What was done**: Added the `internal/observe` package. It reads the five optional `TCGREPLAY_GRAFANA_*` settings and reports export and usage-history status (on, off, or a "warning:" text naming the wrong setting, never the token). `Start` builds OTLP/HTTP trace, log and metric providers for Grafana Cloud with an always-on sampler, 2,048-entry dropping batch queues, a 5s timeout, no retries and gzip, or no-op providers when the settings are off. A health watch writes one stderr line after five minutes of unbroken export failure, at most every five minutes, naming only signals and the failure kind. `Shutdown` is bounded to two seconds.

**Deviations**:
- `Settings` gained an `HTTPClient` field. Tests use it to pass a client that trusts their TLS fake receiver; production leaves it nil. The plan's unexported "compression off" test option was not needed, because the fake receiver un-gzips bodies.
- Added `Telemetry.ForceFlush` and `Telemetry.Run`, which runs the health watch; the usage history joins `Run` later. Added the reusable fake receiver `internal/observe/observetest`, which the hosted tests in later tasks will use.
- The failure kind takes the HTTP status from a small transport wrapper on each exporter's client, because the exporters' retryable errors do not carry it.
- The OTel global error handler and logger are set to discard, once and only when export is on. The plan said to route them to the health watch, but the exporter wrappers already note every outcome.
- `go mod tidy` drops `otelhttp` until it is imported in "Every HTTP request and outside call traced". `golang.org/x/oauth2` moved to v0.36.0, the minimum the OTel modules need.

**Files changed**:
- `go.mod`
- `go.sum`
- `internal/observe/settings.go`
- `internal/observe/observe.go`
- `internal/observe/health.go`
- `internal/observe/observetest/receiver.go`
- `internal/observe/settings_test.go`
- `internal/observe/observe_test.go`
- `internal/observe/health_test.go`

**Discoveries**:
- In otel/log v1.47.0, a record's body takes an `attribute.Value` (`attribute.StringValue`). There is no `log.StringValue`.
- The worktree needs `cd site && npm ci && npm run build` before `go test ./cmd/tcgreplay` passes. Otherwise `TestOpenAIChallengeNotSet` fails on the placeholder site.

### 2026-11-23 — Task: Records and log lines sent to Grafana

**What was done**: `telemetry.Recorder` gained an optional `Sink` (`SetSink`), `RecordContext`/`ActiveContext`, and a `provider` field on events. Every record is still written to stdout unchanged, then handed to the sink outside the lock. The `observe` record sink sends each record as a log record whose body is the same JSON line, with its fields as attributes. A line writer, added beside stderr as the std log package's output, sends each plain log line at INFO, or at WARN for "warning:" lines. Hosted mode starts telemetry first in `runHosted` (`hostedTelemetry`), sets the recorder's sink, ends the start-up line with the export status and flushes on shutdown. Sign-in records say `"provider":"google"`. The app spec, the Dockerfile note and the runbook catalogue list the new settings and the field.

**Deviations**:
- `main` now also copies the log lines written before the mode is known (setting warnings) into a buffer. Hosted mode sends them once telemetry starts. `main` switches back to plain stderr before the mode switch, so the buffer stops growing.
- `runHosted` starts telemetry at its very top, ahead of the public URL check, so that check's warning is sent too. The plan said "before newHosted".
- The export status for set-but-wrong settings reads `off (incomplete: …)`.
- The `warning: telemetry export: off …` line itself can never be sent, because export is off when it is written.
- The fake receiver gained OTLP log decoding (`Logs`) and `CertFile`. With `CertFile`, a subprocess trusts it through `SSL_CERT_FILE`, so the real `main` is tested end to end. This made `go.opentelemetry.io/proto/otlp` a direct dependency.
- Goal check: attempt 1 found that warnings logged before telemetry started were not sent, which led to the first two deviations above. At attempt 2 the reviewer judged every criterion met, but a denied shell command meant the stale attempt-1 reply was recorded, which cost a retry. Attempt 3, with no code change, was accepted.

**Files changed**:
- `internal/telemetry/recorder.go`
- `internal/telemetry/sink_test.go`
- `internal/observe/sink.go`
- `internal/observe/logwriter.go`
- `internal/observe/sink_test.go`
- `internal/observe/settings.go`
- `internal/observe/settings_test.go`
- `internal/observe/observetest/receiver.go`
- `internal/hostauth/google.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/cardsets_test.go`
- `cmd/tcgreplay/hosted_test.go`
- `cmd/tcgreplay/hosted_telemetry_test.go`
- `cmd/tcgreplay/desktop_telemetry_test.go`
- `.do/app.yaml`
- `Dockerfile`
- `docs/observability.md`
- `go.mod`
- `go.sum`

**Discoveries**:
- Lines a test helper logs before `serveHosted` starts telemetry (the catalogue's "card sets:" line) are not sent in tests, although they are in `runHosted`. The subprocess test (`TestMainHostedSendsStartupLines`) covers the production order.
- The test helper `serveHosted` starts telemetry only when `c.telemetry` is set, never from the environment, so other hosted tests cannot export by accident.

### 2026-11-23 — Task: Every HTTP request and outside call traced

**What was done**: Added `observe.Handler`, an otelhttp server wrapper with W3C trace context, a non-public endpoint and HTTP metrics off. It names spans `<METHOD> <route>` from the matched mux pattern and sets `http.route`. Added `observe.Client`, a tracing transport that names client spans `<METHOD> <host>`, and `filterExporter`, an attribute allow-list that strips queries from `url.full` and status descriptions and keeps only `tcgreplay.*` events. Hosted mode now serves through one `hostedServeHandler`, used by both `runHosted` and the test helper `serveHosted`. It passes tracing clients to the Google token exchange, the card set loader, the image store, the image source fetches and the CDN purges.

**Deviations**:
- `Client` sends no trace context to outside services: its propagator is empty. The plan said TraceContext. None of those services takes part in the trace, and sending it would only hand trace IDs to third parties.
- With export off, `Client(base)` returns `base` itself, nil included, and `Handler(h)` returns `h`. Desktop and export-off clients are therefore exactly as before. `Client(nil)` gives a client with a 30s timeout only when export is on.
- `newHosted`, `hostedAuth` and `hostedImages` gained a `tel *observe.Telemetry` parameter, which may be nil. Their test callers pass nil.

**Files changed**:
- `internal/observe/http.go`
- `internal/observe/filter.go`
- `internal/observe/observe.go`
- `internal/observe/http_test.go`
- `internal/observe/observetest/receiver.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/cardsets_test.go`
- `cmd/tcgreplay/hosted_admin_test.go`
- `cmd/tcgreplay/hosted_cache_test.go`
- `cmd/tcgreplay/hosted_test.go`
- `cmd/tcgreplay/hosted_telemetry_test.go`
- `cmd/tcgreplay/hosted_trace_test.go`
- `go.mod`
- `go.sum`

**Discoveries**:
- otelhttp v0.72 renames the server span after the handler returns if `r.Pattern` is set, using the span name formatter. It adds `http.route` only to metrics, so the playmat sets it on the span itself. `movedGuard` passes the request through unchanged, so the outer mux's pattern reaches otelhttp.
- A `/ws` server span ends only when the connection closes, so tests must close the board connection before they look for its span.
- In the OTel Go API, `codes.Error` is 1 and `codes.Ok` is 2. A status set with a raw `2` silently drops its description.

### 2026-11-23 — Task: Tool calls and board actions traced with records linked to their trace

**What was done**:
- **Tool call spans.** `observe.CarryContext`, inside `RequireToken`, writes each MCP request's span into its headers. `observe.MCPMiddleware`, the outermost MCP middleware, reads it back from `RequestExtra.Header` and starts `tools/call <tool>` with the tool, the assistant (known names, else `other`) and the `p_…` player. Each call therefore sits in the trace of the `POST /mcp` that carried it, even though the go-sdk hands calls the initialize request's context.
- **Board action spans.** `webui.Server.Trace` wraps each board action, and in hosted mode with telemetry on it calls `observe.BoardSpan`. That starts a new root span `board <verb>`, linked to the `/ws` span.
- **Records carry their trace.** Hosted request contexts are threaded through `hostauth` (`allow`, `signIn`, `Places.Admit`, `Places.Release`, the store's release path), `usergames` (`Use`, `Admit`, `Release`, `ResolveMCP`), the MCP recorder, `BoardError` and `cacheadmin`. Their records now carry the request's trace.
- **Failed calls and actions.** The recorder marks the span failed with the same redacted message as its error record. The tracing layer marks it failed with no message only when nothing else has, and never with raw error text.

**Deviations**:
- `BoardSpan` takes the player ID as a third argument, `(ctx, verb, player)`. Hosted wiring supplies it from the account in the context. The plan had `(ctx, verb)`.
- `labels.go` `collapse` landed here rather than in "Usage counted as metrics", because tool call spans already need it.
- `hostedHandler` gained a `tel` parameter, for `CarryContext`.
- Goal check: attempt 1 found that the sign-out `account_removed` record (reason `signed_out`) carried no trace. `Places.Release`, the store's `releaseLocked`, `endSession` and `release` now take a context. Sign-out and the token endpoint pass their request's context; expiry sweeps pass `context.Background`. Attempt 2 was accepted.
- The codebase research and the tests were done as the High-complexity task requires: the research in context while the earlier reviews ran, and the tests by a sub-agent.

**Files changed**:
- `internal/observe/mcp.go`
- `internal/observe/board.go`
- `internal/observe/labels.go`
- `internal/observe/mcp_test.go`
- `internal/observe/board_test.go`
- `internal/webui/server.go`
- `internal/webui/actions.go`
- `internal/webui/trace_test.go`
- `internal/hostauth/hostauth.go`
- `internal/hostauth/google.go`
- `internal/hostauth/limit.go`
- `internal/hostauth/admin.go`
- `internal/hostauth/board.go`
- `internal/hostauth/store.go`
- `internal/hostauth/oauth.go`
- `internal/hostauth/cap_test.go`
- `internal/hostauth/events_test.go`
- `internal/hostauth/google_test.go`
- `internal/hostauth/limit_test.go`
- `internal/hostauth/seal_test.go`
- `internal/usergames/usergames.go`
- `internal/usergames/board.go`
- `internal/usergames/mcp.go`
- `internal/usergames/telemetry.go`
- `internal/usergames/*_test.go`
- `internal/cacheadmin/handler.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_admin_test.go`
- `cmd/tcgreplay/hosted_trace_test.go`

**Discoveries**:
- A later layer's `SetStatus(codes.Error, "")` overwrites the description an inner layer set. Outer tracing layers must check the SDK span's current status (`sdktrace.ReadOnlySpan.Status()`) before marking a span failed.
- `mcp.Request.GetSession()` gives the `*mcp.ServerSession`, whose `InitializeParams().ClientInfo.Name` names the assistant for any method, not only tool calls.

### 2026-11-23 — Task: Image copies joined to their card request and the end-to-end privacy sweep

**What was done**:
- **Image copies.** `cards.ImageCache` gained `ImageContext(ctx, src)`, and `Image(src)` is a wrapper for it. Queuing a copy remembers the requesting span's context, and the worker runs that copy's source fetch and store writes under it, so they show in the card request's trace while the copy stays asynchronous. The webui `ImageSource` interface became `ImageContext`, and `serveCard` passes the request context.
- **End-to-end test.** `hosted_telemetry_e2e_test.go` drives a full hosted session against the fake receiver: connector sign-in, import naming cards and players, new game, malformed import, board page, live connection and action, card request, admin check. It proves the following:
  - every request served is a server span, and every outbound call is a client span, counted per service;
  - there is one span per tool call and per board action;
  - the store write and source fetch sit in the card request's trace;
  - nothing identifying is sent.
- **Static check.** The no-email check now also covers span and log attributes, and scans `internal/observe` and `internal/cards`.

**Deviations**:
- The e2e test builds hosted mode itself, mirroring `runHosted` with the image cache, because the shared `serveHosted` helper runs without one.
- Goal check attempt 1 found that outbound calls were not shown to be spans. The test now compares client spans with the requests each fake service counted: Google token exchanges, card set fetches, image store requests and image source fetches. Attempt 2 was accepted.

**Files changed**:
- `internal/cards/images.go`
- `internal/cards/images_trace_test.go`
- `internal/webui/server.go`
- `internal/webui/webui_test.go`
- `cmd/tcgreplay/hosted_telemetry_e2e_test.go`
- `cmd/tcgreplay/noemail_test.go`

**Discoveries**:
- A queued image copy's span context is dropped when the copy ends (`unpend`) or when the queue is full, so the map stays bounded.
- Behind a rerouting test transport, the client span is still named after the original host (`GET assets.tcgdex.net`). The tracing transport sits outside the reroute.

### 2026-11-23 — Task: Usage counted as metrics

**What was done**: The record sink now also increments usage counters on the playmat's meter. Each counter has a `{…}` unit, so Grafana stores it as `tcgreplay_<name>_total`:

| counter | label |
|---|---|
| `tcgreplay.player_days` | none |
| `tcgreplay.sign_ins` | `provider` |
| `tcgreplay.tool_calls` | `assistant`, `tool` |
| `tcgreplay.imports` | `assistant` |
| `tcgreplay.new_games` | `assistant` |
| `tcgreplay.errors` | `kind` (`tool`/`board`) |
| `tcgreplay.limited` | `reason` (`rate`/`places`, else `other`) |

Unknown assistants count as `other`. Tool names outside the registered set count as `other`; hosted wiring reads that set at start-up by listing the MCP server's tools over an in-memory session (`toolNames`, `Telemetry.SetTools`). Records that are not usage count nothing.

**Deviations**:
- The provider label is the recorded provider as it stands, or `google` when the record names none. It is not forced into a fixed list, because the playmat's code, never a client, names the provider.
- The tool set is read by listing tools in process. The plan suggested passing an `mcpserver` list, but go-sdk exposes none.

**Files changed**:
- `internal/observe/metrics.go`
- `internal/observe/labels.go`
- `internal/observe/sink.go`
- `internal/observe/observe.go`
- `internal/observe/metrics_test.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_telemetry_test.go`

**Discoveries**:
- go-sdk v1.6.0's `mcp.Server` has no accessor for its registered tools. Listing them takes a client session, which an in-memory transport pair makes cheap.

### 2026-11-23 — Task: Daily usage kept for six weeks with a dashboard

**What was done**: Added the usage history (`internal/observe/history.go`). It runs 30s after start-up and then every hour. Each run reads back the exported `tcgreplay_usage_daily` table from the stack's Prometheus query API (Basic auth with the metrics user and token) and merges it, keeping the larger value. It then computes each completed UTC day in the last 13 that the table lacks from the raw counters with `increase(…[1d])`, skipping days with no raw data. Finally it drops days older than 42. The table is exported as the observable gauge `tcgreplay.usage_daily` with labels `day`, `count`, and `provider` or `assistant` where the count is split. Query failures leave the table unchanged and feed the health watch as signal `history`. The start-up line now ends with the usage history status as well as the export status. Added the dashboard `docs/grafana/tcgreplay-usage.json`: six per-day bar charts from the daily gauge, plus last-14-days charts from the raw counters.

**Deviations**:
- The gauge's unit is `{count}`, not the plan's `1`. In Grafana's OTLP translation, unit `1` on a gauge adds a `_ratio` suffix, and an annotation keeps the stored name `tcgreplay_usage_daily`.
- A day is computed only when a presence query finds any `tcgreplay_*_total` series in it. This is how "a day with no raw data at all is skipped" is told apart from a day with zero usage.
- A failed query part-way through a day leaves that day out, so the next run computes it again.
- `Telemetry.HistoryOn()` was added. The tests were written by a sub-agent, as the High-complexity task requires.

**Files changed**:
- `internal/observe/history.go`
- `internal/observe/observe.go`
- `internal/observe/history_test.go`
- `internal/observe/dashboard_test.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_history_test.go`
- `docs/grafana/tcgreplay-usage.json`

**Discoveries**:
- Real stored names still need confirming against the live stack once the setup task turns sending on (plan Open Question). Expected: `tcgreplay_<counter>_total` for the counters and `tcgreplay_usage_daily` for the gauge. If they differ, fix the dashboard, the runbook and `historyQueries` to match.

### 2026-11-23 — Task: Observability runbook rewritten for Grafana

**What was done**: Rewrote `docs/observability.md` around Grafana Cloud.
- **Intro.** Records, logs, traces and usage metrics are read in Grafana; platform metrics and alerts stay in App Platform; Runtime Logs is the live view.
- **What is recorded, never recorded and player IDs.** Kept, and extended to say how records appear in Loki.
- **New sections:**
  - "What traces carry";
  - "Setting up Grafana Cloud": region check, token scopes, the five settings, dashboard import, the start-up line;
  - "Reading daily usage in Grafana";
  - "Traces": Tempo searches;
  - "Turning on edge traces": Cloudflare steps and a Status line;
  - "Export status": the start-up and failure lines.
- **Finding a player's errors.** Now goes admin lookup, then a Loki query, then the trace, then back to its logs.
- **Graphs, Alerts and Capacity.** Kept.
- **Retention.** Rewritten: 14 days for records, traces and raw metrics; six weeks for daily totals; a provisional 7 days for platform metrics; 30 days for alert history; 7 days for Cloudflare traces if persisted. Each has its source and the date checked.

Updated the knowledge entry `gotchas/app-platform-logs-only-in-insights.md` to say records now go to Grafana via OTLP.

**Deviations**: Added a Go content check of the runbook (`TestRunbookReadsFromGrafana` in `cmd/tcgreplay/hosted_telemetry_test.go`). The Retention section keeps "7 days" (platform metrics), so the privacy page's current figures still match until the next task changes them.

**Files changed**:
- `docs/observability.md`
- `cmd/tcgreplay/hosted_telemetry_test.go`
- knowledge store (repo tier, `tcgreplay`): `gotchas/app-platform-logs-only-in-insights.md`

**Discoveries**: None.

### 2026-11-23 — Task: Privacy page names Grafana Cloud

**What was done**: The privacy page's Diagnostics section now describes request traces. It says records and traces are sent to Grafana Cloud and stored in Germany, for 14 days each, that daily totals with no player ID are kept for six weeks, and that DigitalOcean keeps its own metrics for 7 days. The Insights claim is gone. The outside services list gains Grafana Cloud (what it receives, and that it never receives emails, tokens, battle logs, decklists or card text). DigitalOcean is named for hosting and its own metrics, and Cloudflare "may trace requests". The page's updated date moved to 23 November 2026. A What's new entry (2026-11-23, `refresh: false`) announces the change. The browser suite checks the Grafana and Germany wording, the absence of Insights, and the new outside services item. It also checks every `data-retention` figure on the page against the runbook's Retention section. The privacy and What's new screenshots were re-recorded.

**Deviations**: Goal check attempt 1 found that the visitor-count figure ("six months", `data-retention="visits"`) was neither in the runbook's Retention section nor checked. A visitor counts row, with its source, was added to the runbook's Retention table. The browser check now covers every `data-retention` figure the page shows and fails if a new one is not checked. Attempt 2 was accepted.

**Files changed**:
- `site/src/pages/privacy.astro`
- `site/e2e/privacy.spec.js`
- `site/src/data/whats-new.yaml`
- `site/e2e/__screenshots__/laptop/privacy.png`
- `site/e2e/__screenshots__/laptop/docs-whats-new.png`
- `site/e2e/__screenshots__/phone/privacy.png`
- `site/e2e/__screenshots__/phone/docs-whats-new.png`
- `docs/observability.md`

**Discoveries**: Changing the privacy page or What's new changes the site's recorded screenshots (`e2e/screenshots.spec.js`). Re-record them with `npx playwright test e2e/screenshots.spec.js --update-snapshots` after `npm run build`.

### 2026-11-23 — Task: Turn on sending to the Grafana stack

**What was done**: Nothing in the code. This is a human task: it needs the maintainer's Grafana Cloud account and the App Platform console (production secrets), and the privacy page describing Grafana must be live first.

**Deviations**: **Skipped**, by the user's decision through the epic orchestrator. No criterion is ticked, and none can be until after merge and deploy. The goal check ran three times and judged every criterion `not_met`, because each needs the live stack and console. The maintainer does this task after merge and deploy by following `docs/observability.md` "Setting up Grafana Cloud": region check, token, the five settings, the start-up line, the dashboard import. At the same time they confirm the stored metric names (plan Open Question). The implementation test plan lists it as manual steps.

**Files changed**: None.

**Discoveries**: None.

### 2026-11-23 — Task: Turn on Cloudflare edge traces

**What was done**: Nothing in the code. This is a human task: it needs the maintainer's Cloudflare dashboard (zone settings outside the repo), and it follows "Turn on sending to the Grafana stack".

**Deviations**: **Skipped**, by the user's decision through the epic orchestrator. No criterion is ticked. The goal check ran three times: it judged criterion 2 (no paid plan) met and criterion 1 `not_met`, because the runbook's Status line still says "not yet turned on". After merge, deploy and the Grafana setup, the maintainer follows `docs/observability.md` "Turning on edge traces". They then record on its Status line, with the date, either that edge traces are on or why the edge part was dropped (no OTLP export on the Free plan, or `traceparent` not reaching the playmat through App Platform's edge). The implementation test plan lists it as manual steps.

**Files changed**: None.

**Discoveries**: A human task in a full-plan run can be skipped only from a stopped goal check (`goal_decision_not_due` otherwise). The run walks it through the goal check until the attempts run out, then records the skip.
