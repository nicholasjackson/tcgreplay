---
created_date: "2026-11-23"
document_status: final
closed_date: "2026-11-23"
---

# Research: 20261123114225-5cf4d0df-grafana-observability

## Alternatives considered and rejected

- **App Platform log forwarding (`log_destinations`) to Grafana instead of OTLP from the app.** App Platform's forwarding targets are Managed OpenSearch, Datadog, Logtail, Papertrail and the like; there is no Grafana destination, it carries no traces or metrics, and it loses log-to-trace correlation (knowledge `gotchas/app-platform-logs-only-in-insights.md`; spec Technical Approach). Rejected.
- **A collector or Grafana Alloy sidecar.** App Platform runs one container per component and the spec forbids self-hosted monitoring and new storage (spec Constraints). Rejected: the Go SDK's batch processors give the same batching with bounded queues in-process.
- **Standard `OTEL_EXPORTER_OTLP_*` environment variables as the configuration.** They work with Grafana (research: grafana.com/docs/grafana-cloud/send-data/otlp/send-data-otlp/), but the maintainer would have to base64 `instanceID:token` into a header by hand, the playmat could not report "on/off/incomplete" in its start-up line the way every other optional setting does (`cmd/tcgreplay/hosted.go:373-425`, `gotchas/app-spec-not-applied-by-deploy.md` "start-up line lists which optional settings are on"), and the SDK would pick them up in any mode that built an exporter. Rejected in favour of explicit `TCGREPLAY_GRAFANA_*` settings passed as exporter options.
- **Relying on Grafana Cloud metric retention for the 28-day daily counts.** Grafana Cloud Free keeps metrics for **14 days** ("14 days retention", grafana.com/pricing and grafana.com/products/cloud/free-tier); 13 months is Pro only. Rejected: it cannot meet the spec's 28-day requirement.
- **Keeping daily counts in the playmat (memory, Spaces object, disk).** Memory is lost on every deploy; a Spaces object or disk is new persistent storage the spec forbids ("No new storage"), and the prior observability plan rejected counts persisted in Spaces for the same reason. Rejected.
- **A scheduled GitHub Actions job that rolls up the counts.** It would put the Grafana credentials in GitHub secrets, against the spec's "Credentials are hosted secrets ... set as encrypted settings on the hosted app". Rejected.
- **Grafana annotations or dashboards as the store for daily totals.** Their retention on Grafana Cloud is undocumented (unverified), and writing them needs a Grafana service-account token on top of the stack token. Rejected.
- **otelhttp's built-in HTTP metrics.** `http.server.request.duration` is a histogram per route, method and status; with ~25 route patterns it would cost thousands of active series against the Free tier's 10k ("Limited to 10k active series per month", grafana.com/pricing). Rejected: latency is read from traces, and the server's metric provider is a no-op for otelhttp.
- **`ParentBased` sampling.** Cloudflare forwards a `traceparent` whose sampled flag reflects its own sampling rate; a parent-based sampler would drop the playmat's spans for every request Cloudflare did not sample, against "every request is traced". Rejected for an always-on sampler that still adopts the incoming trace ID.
- **Taking the span from the MCP handler's context.** go-sdk v1.6.0 connects the session with the *initialize* request's context (`go-sdk@v1.6.0/mcp/streamable.go:488`), so every later tool call's context descends from that first, long-ended request; `trace.SpanFromContext` in a middleware returns the wrong span. Rejected: the current request's headers do reach every call (`streamable.go:1162-1166`, `RequestExtra.Header`), so trace context is injected into the headers by an HTTP wrapper and extracted in the MCP middleware.
- **`log/slog` with the otelslog bridge for the plain log lines.** The code uses the standard `log` package everywhere (no slog anywhere); converting ~50 call sites buys nothing the line writer does not. otelslog is also pre-1.0 (v0.21.0). Rejected in favour of an `io.Writer` that turns each std-log line into a log record.
- **Exporting otelhttp's default span attributes as they are.** Server spans carry the client address and user agent, and client spans the full URL with its query; `/auth/google/callback?code=…` carries Google's authorization code and `/authorize?…` the PKCE challenge and state. Rejected for an allow-list filter at the span exporter.

## Chosen approach — evidence

- Recorder is the single point every usage and error record passes through, and already never writes an email (`internal/telemetry/recorder.go:104-135`; `TestRecordNeverWritesEmail`). A `Sink` on it turns each line into an OTel log record and a counter increment without a second event set (spec "Build on the existing records").
- All recorder call sites and the context available at each: hostauth `admin.go:44,53` (r), `board.go:134` (r), `google.go:95` (r), `google.go:233` signIn (no ctx; caller at `google.go:94` has r), `limit.go:86` allow (no ctx; callers `limit.go:110`, `:128`, `board.go:75` have one); usergames `board.go:34` (req), `usergames.go:134` Active in Use (no ctx), `:145` Admit, `:156` Release (under store lock), `:223`, `:297` background; `telemetry.go:50-64` MCP middleware (ctx stale, headers fresh), `telemetry.go:90` BoardError (ws upgrade ctx); cacheadmin `handler.go:36,63,97` (r).
- Std log is configured only in `cmd/tcgreplay/main.go:248-256` (`log.SetOutput(os.Stderr)`, file only for desktop), records go to stdout (`hosted.go:149-152`): one `io.MultiWriter` per stream keeps the live view unchanged.
- Whole-handler wrap point: `hosted.go:491` (`movedGuard(...)`), duplicated in tests at `cmd/tcgreplay/cardsets_test.go:292` (`serveHosted`), so the wrap belongs in a shared function. Go 1.27 sets `r.Pattern` on the outer mux's request (inner muxes see copies), so the route name is read after `mux.ServeHTTP` returns.
- Board actions: `webui/actions.go:90-121` `dispatch(ctx, c, req)` is the per-action point, named by `req.Verb`; ctx is the websocket upgrade's (`hub.go:260`).
- MCP: middleware type `func(next mcp.MethodHandler) mcp.MethodHandler`; wired `srv.AddReceivingMiddleware(a.MCPMiddleware(), reg.MCPMiddleware())` at `hosted.go:353`; first is outermost within one call (`gotchas/go-sdk-sessions-and-auth.md`).
- Outbound clients are all injectable: hostauth `Config.HTTPClient` (`hostauth.go:51-52`, unset in `hostedAuth`), catalog `Loader.HTTP` (`catalog/loader.go:69,423`), `spaces.Config.HTTP` (`spaces/config.go:33`), `spaces.CDN.HTTP` (`cdn.go:56-67`), `cards.ImageCache.HTTP` (`images.go:83`). Google token exchange uses the request ctx (`google.go:115-131`); image copies run on workers with the process ctx and `Image(src)` has no ctx (`images.go:235`), so the request's span context must be carried into the queued job for "the image store call as its own timed part".
- No OTel today (no hits for otel/traceparent/prometheus in code, go.mod or go.sum). Runtime image `gcr.io/distroless/static-debian12:nonroot` ships CA certificates (`Dockerfile:68-71`).
- Go SDK (v1.47.0, 2026-10-02): logs API and SDK stable; BatchSpanProcessor drops when its 2048 queue is full (non-blocking unless `WithBlocking`); log BatchProcessor drops the oldest at 2048; otlp*http retry is on by default for up to 1 minute and timeout 10s, both configurable; `otel.SetErrorHandler` sees export errors. Bounded, asynchronous, drop-under-pressure export comes from configuration, not new code.
- Grafana OTLP: Basic auth `instanceID:token` with scopes `metrics:write logs:write traces:write`; OTLP logs land in Loki with `trace_id`/`span_id` as structured metadata; counters land in Mimir as `<name>_total`; Go's default temporality is cumulative, which Mimir wants.
- Cloudflare Traces (open beta since 2026-10-02) can "Forward to origin" a `traceparent` (opt-in); it exports to a Grafana Cloud OTLP destination in JSON; Free plan from 2026-12-01: 0.5 GB/day shared ingestion, 7 days retention, export without persisting is possible.

## Files examined

- `cmd/tcgreplay/hosted.go:115-154` — hostedAuth and hostedRecorder; settings read from env; recorder to stdout.
- `cmd/tcgreplay/hosted.go:240-282` — hostedHandler route table (auth, `/mcp`, `/admin`, `/board`, `/assets/`, `/ws`, `/api/`, site).
- `cmd/tcgreplay/hosted.go:318-356` — newHosted; recorder, registry, auth, MCP middleware order.
- `cmd/tcgreplay/hosted.go:373-425` — hostedImages: optional settings are warnings, start-up status lines.
- `cmd/tcgreplay/hosted.go:458-507` — runHosted; start-up line at 489; server composition at 491; shutdown.
- `cmd/tcgreplay/main.go:50-103` — config struct (`events io.Writer`, `now`).
- `cmd/tcgreplay/main.go:248-273` — log output setup and the mode switch; only `-hosted` reaches runHosted.
- `cmd/tcgreplay/moved.go:78-102` — movedGuard, identity without moved hosts.
- `cmd/tcgreplay/cardsets_test.go:273-295` — serveHosted test helper duplicates the handler composition.
- `cmd/tcgreplay/hosted_test.go:71-87,553-598,671-772` — TestMain key, captureLog, subprocess helper, startHosted helpers.
- `cmd/tcgreplay/hosted_test.go:1294-1324` — TestAppSpecHealthCheckAndSecrets list of SECRET keys.
- `cmd/tcgreplay/hosted_events_test.go:19-103,195-215` — record decoding helpers; no-email test; "player IDs on" start-up test.
- `cmd/tcgreplay/noemail_test.go:13-95` — AST scan for `.Email` in log calls (does not cover OTel attributes).
- `internal/telemetry/recorder.go:1-189` — event catalogue, Event, line, Record, Active, cut.
- `internal/telemetry/ids.go:1-51` — player IDs (HMAC, `p_` + 16 hex).
- `internal/telemetry/redact.go:1-80` — Redact for error messages.
- `internal/usergames/telemetry.go:20-90` — MCP recorder middleware and BoardError.
- `internal/hostauth/hostauth.go:73-140` — Account (ID, Email, EmailVerified), client defaulting to http.DefaultClient.
- `internal/hostauth/google.go:66-131,217-235` — callback, token exchange with request ctx, signIn records SignIn.
- `internal/webui/actions.go:85-121` — dispatch per board action.
- `go-sdk@v1.6.0/mcp/streamable.go:488,1162-1166` — session context is the initialize request's; per-message `RequestExtra.Header`.
- `site/src/pages/privacy.astro:1-20,138-230` — diagnostics section names Insights; `data-retention="logs|metrics"` 7 days; outside services list.
- `site/e2e/privacy.spec.js:69-108` — diagnostics wording test; logs/metrics figures must appear in the runbook's Retention section.
- `docs/observability.md` (whole) — runbook built on Insights; record catalogue; retention provisional 7 days.
- `.do/app.yaml:1-230` — app-level envs, secrets `type: SECRET` with no value, `instance_count: 1`.
- `Dockerfile:1-71` — distroless static runtime with CA certificates; env comment lists every setting.
- `internal/appspec/appspec.go` — reads app.yaml envs for the site build.

## External references

- https://grafana.com/pricing/ — Free: metrics 10k active series, 14 days; logs 50 GB/month, 14 days; traces 50 GB/month, 14 days; 3 users. Drove the carry-forward of daily counts.
- https://grafana.com/products/cloud/free-tier/ — "14 days retention for metrics, logs, traces"; "Always free".
- https://grafana.com/docs/grafana-cloud/billing-and-usage/usage-limits/ — default per-stack ingestion limits.
- https://grafana.com/docs/grafana-cloud/send-data/otlp/send-data-otlp/ — OTLP gateway endpoint, copy it from the stack's OpenTelemetry tile; standard env vars.
- https://grafana.com/docs/grafana-cloud/security-and-account-management/region-url-formats/ — Germany regions are `prod-eu-west-2`/`-4` (AWS) and `-5` (Azure); `prod-eu-central-0` is Switzerland.
- https://grafana.com/docs/grafana-cloud/security-and-account-management/authentication-and-permissions/access-policies/using-an-access-policy-token/ — Basic auth with instance ID and access policy token; scopes.
- https://grafana.com/docs/grafana-cloud/send-data/otlp/otlp-format-considerations/ — trace_id/span_id to Loki structured metadata; metric naming (`_total`); attribute limits.
- https://grafana.com/docs/loki/latest/send-data/otel/ — which resource attributes become Loki labels (service.name → service_name).
- https://blog.cloudflare.com/cloudflare-tracing — Cloudflare Traces open beta (2026-10-02), OTLP export, forwards a traceparent to the origin.
- https://developers.cloudflare.com/observability/traces/configuration/ — "Forward to origin" toggle; persistence off for export-only; incoming context default Reject.
- https://developers.cloudflare.com/observability/pricing/ — from 2026-12-01: Free 0.5 GB/day shared ingestion, 7 days retention, no overage; ingestion stops until 00:00 UTC at the limit.
- https://developers.cloudflare.com/observability/export/opentelemetry/grafana-cloud/ — Grafana Cloud destination: endpoint + `/v1/traces`, Basic auth header, JSON only.
- https://github.com/open-telemetry/opentelemetry-go/blob/v1.47.0/CHANGELOG.md — first stable logs API/SDK; `WithEndpointURL` no longer appends the signal path.
- go.opentelemetry.io/otel/sdk/trace batch_span_processor.go, sdk/log batch.go, exporters/otlp/*/internal/retry — queue sizes, drop behaviour, retry defaults.

## Prior plans / specs consulted

- `20261112090330-4ac2ea19-observability` (plan) — why records went to stdout and Insights; tracing and custom metrics were out of scope then because Insights had no custom ingestion; the privacy page's retention markers are tied to the runbook's Retention section.
- `20261122190321-c6d9d90a-sign-in-page` (plan, approved, not implemented) — adds `Provider` to `hostauth.Account`, `Account.PlayerKey()` for telemetry, a `provider` field on `sign_in` records, replaces `signIn(w, acct)` with `finishSignIn(w, r, pending, account)`, and changes the start-up line and privacy page. It assumed log forwarding was handled separately; this plan is that. Shared files: `internal/telemetry/recorder.go`, `internal/hostauth/google.go`, `internal/usergames/*`, `cmd/tcgreplay/hosted.go`, `docs/observability.md`, `site/src/pages/privacy.astro`, `site/src/data/whats-new.yaml`, `.do/app.yaml`, the hosted test helpers.

## Open assumptions

- The maintainer's Grafana Cloud stack is in a German region (spec Constraint). The privacy page states Germany; the runbook task confirms it from the stack's OpenTelemetry tile before the page ships.
- Grafana's Mimir Prometheus query API (`/api/prom/api/v1/query`) accepts the same access policy token when it also has `metrics:read`, with the Mimir instance ID as user (Grafana docs; not exercised).
- What Grafana Free does once the 50 GB or 10k-series allowance is used up is undocumented (dropped or rejected). The playmat treats either as an export failure and drops.
- Whether Cloudflare's domain-level Traces export works on the maintainer's Free plan is unverified; the edge requirement is dropped with the reason recorded if it does not.
- Whether a `traceparent` forwarded by the maintainer's Cloudflare zone survives App Platform's own edge to the container is unverified; checked by hand after deploy.
- Grafana Cloud's provisioned Tempo and Loki data sources may already link traces to logs; if not, the maintainer configures it (runbook step).
- If any of these turns out wrong in a way that changes a requirement (not just a runbook step), the implementer must STOP and ask.

## Drafting assumptions

### Daily counts outlive Grafana's 14-day metric retention by being carried forward inside Grafana (discovery)
- **Decision**: Grafana Cloud Free keeps metrics for 14 days, not 28+. The playmat keeps the daily usage counts for 6 weeks by re-exporting them as a small `day`-labelled gauge, reading the previous values back from Grafana's metrics query API at start-up and hourly, and adding each completed day computed from the raw counters. Grafana is the only store; the playmat keeps nothing on disk.
- **Rationale**: the spec says that if metrics are kept for less than 28 days "the plan must find a way to keep the daily counts that long within the free tier"; this is the only option found that stays inside the free tier, adds no storage to the playmat and keeps the credentials on the hosted app.
- **Rejected**: Pro tier (spec: free tier only); counts in memory, a Spaces object or disk (spec: no new storage; prior plan rejected Spaces counts); a GitHub Actions rollup (credentials would leave the hosted app); Grafana annotations (retention undocumented, extra token).

### Settings are `TCGREPLAY_GRAFANA_*`, not the standard OTEL_* variables (discovery)
- **Decision**: `TCGREPLAY_GRAFANA_OTLP_URL`, `TCGREPLAY_GRAFANA_INSTANCE_ID`, `TCGREPLAY_GRAFANA_TOKEN` (secret) for export, and `TCGREPLAY_GRAFANA_METRICS_URL` and `TCGREPLAY_GRAFANA_METRICS_USER` for reading the carried counts; one access policy token with `metrics:read`, `metrics:write`, `logs:write`, `traces:write`.
- **Rationale**: matches every other optional hosted setting (start-up line reports on/off/incomplete, partial settings warn and never stop play); the playmat builds the Basic header so the maintainer never base64-encodes a secret by hand.
- **Rejected**: OTEL_EXPORTER_OTLP_* (no on/off reporting, hand-built header); a second read-only token (one more secret for aggregate, player-free data).

### Chosen direction: OpenTelemetry Go SDK in hosted mode only, records reused through a recorder sink (architecture)
- **Decision**: a new `internal/observe` package builds OTLP/HTTP trace, log and meter providers for Grafana Cloud, only in `runHosted`. The existing `telemetry.Recorder` keeps writing stdout and also feeds a sink (log records and counters). Std-log lines reach Grafana through a line writer. otelhttp wraps the whole hosted handler and the injectable outbound clients. The MCP trace context is carried in request headers. Daily totals are carried forward inside Grafana.
- **Key design decisions**: the always-on sampler adopts the incoming trace ID but ignores its sampled flag; otelhttp metrics are off (series budget); a span exporter allow-list filter protects privacy; exporters run with a 5-second timeout and no retries, behind bounded batch queues that drop; a health watch prints one stderr line at most every 5 minutes after 5 minutes of failure; unknown assistant names count as `other` in metric labels; board actions get their own spans, linked to the ws connection span; image copy jobs carry the requesting span's context.
- **Rationale**: it meets every requirement with no new storage, no collector and no change to the live log view, and it reuses the code's existing seams (recorder, injectable clients, optional settings with start-up status).
- **Rejected**: log forwarding, a collector or Alloy sidecar, a slog migration, hand-rolled OTLP, parent-based sampling, and letting Grafana's retention keep the metrics (see research alternatives).

### Board actions are their own spans linked to the connection (architecture)
- **Decision**: each websocket message starts a new root span `board <verb>` with a link to the `/ws` span, rather than a child of it.
- **Rationale**: a board connection can stay open for hours. Children of one span that long would make each action hard to find and time in Tempo's search, while a link keeps the relationship visible.
- **Rejected**: child spans of the ws span (unsearchable traces hours long); no per-action spans (the spec counts board data requests).

### Image copy spans are children of the request that queued them (architecture)
- **Decision**: `ImageCache` gains a context-taking variant of `Image` that records the caller's span context on the queued copy job. The worker runs the copy (TCGdex fetch, image store PUT) under that context, so those spans join the card request's trace. Only the request that first triggers a copy shows the store call. Later requests are served from the existing copy, and no store call happens.
- **Rationale**: the spec's acceptance criterion "a request that fetches a card image shows the image store call as its own timed part" requires that join. The copy is asynchronous today and must stay so, to keep players unaffected.
- **Rejected**: making the copy synchronous (adds latency to players); links only (the store call would not appear inside the request's trace).

### Admin image clears are not joined to the admin request's trace (architecture)
- **Decision**: the admin "clear images" job keeps running on the server's own context, so its CDN purge spans are separate root traces. The admin "check for card sets" call already runs on the request's context and joins it.
- **Rationale**: the spec lists sign-in, tool calls, board and card-data requests and their outside calls. The admin clear is an operator action outside that list.
- **Rejected**: carrying the context into the clear job (extra plumbing for no spec requirement).

### Conventions selected (architecture)
- **Decision**: applies — the privacy page tracks outside services; What's new for the privacy page change; guide packaging (Docker context keeps internal/guides). Dropped — the board-never-scrolls, fast-design-iteration and signed-off-design-wins conventions (no board or design surface changes).
- **Rationale**: only the website's privacy page and the Docker build are touched among convention-bearing surfaces.
- **Rejected**: n/a

### Hosted mode passes providers explicitly instead of setting OTel globals (components)
- **Decision**: providers, propagator and meter go to each adapter as explicit options; `otel.SetTracerProvider` and the other global setters are not used. The only global touched is `otel.SetErrorHandler`/`SetLogger`, routed to the health watch and set only in hosted mode.
- **Rationale**: the hosted Go tests run many servers in one process; globals would leak spans between tests and could make a desktop code path export if a global was set.
- **Rejected**: global providers (test cross-talk; harder to prove desktop sends nothing).

### Metric label cardinality is closed (data_structures)
- **Decision**: assistant labels collapse unknown clients to `other`; `tool` is the registered tool name (a closed set), with unknown names as `other`; `day` is limited to six weeks.
- **Rationale**: the Free tier allows 10k active series; client names are chosen by the client and are unbounded.
- **Rejected**: raw client names as labels (unbounded series; they stay in the log records instead).

### Usage history keeps six weeks (data_structures)
- **Decision**: the carried daily table keeps 42 days.
- **Rationale**: the spec needs at least 28 days, and the sign-in success metric compares two weeks before and after a release. Six weeks gives a margin at roughly 800 series.
- **Rejected**: 28 days exactly (no margin); a year (about 7,000 series, too close to the 10k allowance).

### Latency criterion verified by mechanism test plus manual measurement (testing_approach)
- **Decision**: an automated test proves a hung or failing receiver adds no wait to requests and memory stays bounded. The spec's 5% median comparison is a manual measurement on the deployed playmat.
- **Rationale**: percentage latency assertions in CI are flaky on shared runners. The real risk is blocking, which the mechanism test catches deterministically.
- **Rejected**: a CI benchmark with a 5% threshold (noise larger than the threshold).

### Sending is turned on only after the privacy page describes Grafana (milestones)
- **Decision**: Milestones 1-3 ship code with export off on the live site (no settings yet). The maintainer sets the Grafana settings only in Milestone 4, after the privacy page and runbook describe Grafana.
- **Rationale**: every push to main deploys. Turning export on earlier would send diagnostics to an outside service the privacy page does not yet name (`conventions/privacy-page-tracks-outside-services.md`). Optional settings make the code safe to ship dark.
- **Rejected**: privacy page first (it would describe a service not yet in use); setting the settings as each milestone lands (privacy page inaccurate in between).

### Places.Admit and signIn gain a context parameter (tasks)
- **Decision**: thread the request context through `hostauth.Places.Admit`, `signIn`, `allow`, `usergames.Use`/`Admit` and `ResolveMCP` rather than leave those records untraced.
- **Rationale**: the spec requires that a log line written while serving a request carries its trace identity. These are the sign-in, limit and activity records.
- **Rejected**: storing the context on the account or in a goroutine-local (not idiomatic Go); leaving those records untraced (fails the requirement).

### The edge step is a human task with a recorded fallback (tasks)
- **Decision**: turning on Cloudflare Traces is the maintainer's task, and if OTLP export is not offered on the Free plan, or forwarded context does not reach the container through App Platform's edge, the runbook records why and the edge requirement is dropped.
- **Rationale**: the spec explicitly allows dropping the edge requirement with the reason recorded. Plan availability can only be seen in the maintainer's dashboard.
- **Rejected**: an agent task (no dashboard access); upgrading the plan (spec forbids paid plans).

## Rehydration cues

- `spektacular spec file read 20261123114225-5cf4d0df-grafana-observability`
- `spektacular plan file read 20261122190321-c6d9d90a-sign-in-page plan` (shared files, Provider and PlayerKey)
- `spektacular knowledge read --data '{"tier":"repo","name":"tcgreplay","path":"gotchas/app-platform-logs-only-in-insights.md"}'`, `gotchas/go-sdk-sessions-and-auth.md`, `gotchas/app-spec-not-applied-by-deploy.md`, `learnings/grafana-cloud-free-and-cloudflare-traces.md`
- Re-read `cmd/tcgreplay/hosted.go`, `internal/telemetry/recorder.go`, `internal/usergames/telemetry.go`, `internal/webui/actions.go`, `docs/observability.md`, `site/src/pages/privacy.astro`, `site/e2e/privacy.spec.js`.
