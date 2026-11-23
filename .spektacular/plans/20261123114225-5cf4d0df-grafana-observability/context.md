---
created_date: "2026-11-23"
document_status: final
closed_date: "2026-11-23"
---

# Context: 20261123114225-5cf4d0df-grafana-observability

## Current State Analysis

- **Records.** `internal/telemetry/recorder.go` writes one JSON record per line to stdout (`cmd/tcgreplay/hosted.go:144-154`) for a closed catalogue of 14 events, naming players by `p_` HMAC IDs (`internal/telemetry/ids.go`) and redacting error messages (`internal/telemetry/redact.go`). App Platform shows stdout live but keeps no history, and Insights receives no App Platform logs (knowledge `gotchas/app-platform-logs-only-in-insights.md`), so records are lost on every deploy.
- **Logs.** Plain lines go through the standard `log` package to stderr (`cmd/tcgreplay/main.go:248-256`). There is no slog.
- **No tracing or metrics.** There is no OTel, Prometheus or traceparent anywhere. go.mod has three direct dependencies.
- **Request path.** `hostedHandler` (`cmd/tcgreplay/hosted.go:258-282`) is one ServeMux wrapped by `movedGuard` at `:491`, and the tests duplicate this in `serveHosted` (`cmd/tcgreplay/cardsets_test.go:292`). MCP tool calls get the initialize request's context (`go-sdk@v1.6.0/mcp/streamable.go:488`), while per-call headers arrive in `RequestExtra.Header` (`:1162-1166`). Board actions go through `webui/actions.go:90-121`.
- **Outbound clients** are all injectable but unset in hosted mode: hostauth `Config.HTTPClient`, catalog `Loader.HTTP`, `spaces.Config.HTTP`, `spaces.CDN.HTTP`, `cards.ImageCache.HTTP`.
- **Recorder call sites** without a context: `hostauth.signIn` (`google.go:217`), `allow` (`limit.go:75`), `usergames.Use`/`Admit`/`Release` (`usergames.go:118-156`) and background sweeps.
- **Privacy page** (`site/src/pages/privacy.astro:165-166,217-218`) says diagnostics are kept in Insights for 7 days. `site/e2e/privacy.spec.js:92-108` ties its retention figures to the runbook's Retention section.
- **Deploy.** The app spec (`.do/app.yaml`) is not applied by deploys, and settings are set in the console (knowledge `gotchas/app-spec-not-applied-by-deploy.md`). The runtime image is distroless static with CA certificates (`Dockerfile:68-71`).

## Requirement → repo and files

All work is in the single registered repo `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`).

| requirement | files |
|---|---|
| Logs kept across updates | `internal/telemetry/recorder.go`, `internal/observe/{sink,logwriter}.go`, `cmd/tcgreplay/hosted.go` |
| Logs still live | `cmd/tcgreplay/hosted.go` (MultiWriter beside stderr; recorder still writes stdout) |
| Every request traced | `internal/observe/{http,filter,mcp,board}.go`, `cmd/tcgreplay/hosted.go`, `internal/webui/{server,actions}.go`, `internal/cards/images.go` |
| Traces start at the edge | `internal/observe/http.go` (TraceContext propagator, AlwaysSample), Cloudflare dashboard (human), `docs/observability.md` |
| Logs link to their trace | `internal/telemetry/recorder.go` (RecordContext), `internal/hostauth/{google,limit,admin,board}.go`, `internal/usergames/{usergames,board,mcp,telemetry}.go`, `internal/cacheadmin/handler.go` |
| Usage counted over time | `internal/observe/{metrics,labels,history}.go`, `docs/grafana/tcgreplay-usage.json` |
| Sign-ins by provider | `internal/telemetry/recorder.go` (`Provider`), `internal/hostauth/google.go`, `internal/observe/sink.go` |
| Nothing identifies a player | `internal/observe/filter.go`, `cmd/tcgreplay/noemail_test.go`, `cmd/tcgreplay/hosted_telemetry_e2e_test.go` |
| Never slowed or stopped | `internal/observe/observe.go` (bounded batchers, timeout, no retry) |
| Export state visible | `internal/observe/{settings,health}.go`, `cmd/tcgreplay/hosted.go` start-up line |
| Runbook | `docs/observability.md` |
| Privacy page | `site/src/pages/privacy.astro`, `site/e2e/privacy.spec.js`, `site/src/data/whats-new.yaml` |
| Hosted only | `cmd/tcgreplay/hosted.go` is the only caller of `observe.Start`; `cmd/tcgreplay/desktop_telemetry_test.go` |

## Per-Task Technical Notes

### Task: Telemetry export foundation

**Requirement → repo/files:** "Players are never slowed or stopped by telemetry", "The maintainer can tell whether export is on", the "Settings are optional" and "Credentials are hosted secrets" constraints → `tcgreplay`, new package `internal/observe`.

**File changes:**
- `go.mod` / `go.sum` — add `go.opentelemetry.io/otel` v1.47.0, `otel/sdk` v1.47.0, `otel/sdk/metric` v1.47.0, `otel/log` v1.47.0, `otel/sdk/log` v1.47.0, `otel/exporters/otlp/otlptrace/otlptracehttp` v1.47.0, `otel/exporters/otlp/otlpmetric/otlpmetrichttp` v1.47.0, `otel/exporters/otlp/otlplog/otlploghttp` v0.23.0, `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` v0.72.0. Run `go mod tidy`. The binary stays `CGO_ENABLED=0` (`Dockerfile:66`); all are pure Go.
- `internal/observe/settings.go` (new):
  - `Settings`, `SettingsFromEnv(getenv)`, `Status()`.
  - Validation: the OTLP URL is https with a host, and a trailing `/` is dropped. The instance ID is digits. The token is non-empty with no whitespace.
  - Status texts, modelled on `hostedImages` (`cmd/tcgreplay/hosted.go:373-425`) and `webanalytics.Setup` status:
    - `telemetry export: on (Grafana Cloud <host>)`;
    - `telemetry export: off (no TCGREPLAY_GRAFANA_* settings)`;
    - `warning: telemetry export: off (TCGREPLAY_GRAFANA_INSTANCE_ID is not a number)` and similar;
    - `usage history: on|off (…)`.
  - Never format the token.
- `internal/observe/observe.go` (new):
  - `Start(ctx, s) *Telemetry`. Builds a resource (`service.name=tcgreplay`, `service.version=s.Version`, `service.instance.id`=random hex per start, `deployment.environment.name=production`).
  - Basic header `base64(instanceID:token)`, passed with `WithHeaders`. Endpoints `WithEndpointURL(<url>/v1/traces|metrics|logs)`, full paths because of the v1.47.0 change.
  - All exporters: `WithTimeout(5*time.Second)`, `WithRetry(RetryConfig{Enabled:false})`, gzip on by default (tests turn it off through an unexported option).
  - `sdktrace.NewTracerProvider(WithSampler(sdktrace.AlwaysSample()), WithBatcher(exp, WithMaxQueueSize(2048), WithExportTimeout(5s)))`. Not ParentBased: see assumptions.
  - `sdklog.NewLoggerProvider(WithProcessor(sdklog.NewBatchProcessor(exp, WithMaxQueueSize(2048), WithExportTimeout(5s))))`.
  - `sdkmetric.NewMeterProvider(WithReader(NewPeriodicReader(exp, WithInterval(60s), WithTimeout(5s))))`.
  - Off: `trace/noop`, `log/noop`, `metric/noop` providers and `Enabled()==false`. `Shutdown(ctx)` bounds each provider's Shutdown to 2s in total.
  - Never call `otel.SetTracerProvider` or the other global provider setters. Call `otel.SetErrorHandler` and `otel.SetLogger` only when enabled, routing them to the health watch, so the SDK's own stderr output does not duplicate the warning line.
- `internal/observe/health.go` (new):
  - `health` holds per-signal `lastOK`, `failingSince`, `lastWarn` and `lastKind`, behind a mutex.
  - Wrapper types `healthSpanExporter`, `healthLogExporter` and `healthMetricExporter` call through and note the result.
  - `kind(err)` classifies: `timeout` (context deadline / net timeout), `unreachable` (`*net.OpError`, DNS), `rejected (HTTP 401)` (parse the status from otlp's error type or the "HTTP 4xx/5xx" prefix), else `failed`. It never includes `err.Error()`.
  - `run(ctx)` ticks every 30s on `s.Now`. When any signal has been failing for over 5 minutes and `now-lastWarn >= 5m`, it writes `warning: telemetry export: sending to Grafana Cloud has failed for 7m (logs, traces: unreachable)` to `s.Stderr` (os.Stderr in production). It writes directly, never through `log`, so the line is not re-exported. Success clears the state.
- `internal/observe/observe_test.go`, `health_test.go`, `settings_test.go` (new):
  - Settings matrix, and status text never contains the token.
  - A fake receiver (`httptest.Server` collecting bodies). Also an unreachable address (a closed listener), a hanging handler (blocks until test cleanup) and a 401 handler. Assert that `End()`/`Emit` return in under 10ms with the hanging receiver, and that queue length is capped (emit 10k spans; heap delta bounded).
  - The fake clock drives health: no warning at 4m59s, one at 5m, none at 9m, one at 10m; the warning contains neither the token nor the base64 header.
  - Shutdown with a hanging receiver returns within ~2s.

**Complexity:** Medium · **Token estimate:** ~45k · **Agent strategy:** single agent, sequential (settings → providers → health → tests).

### Task: Records and log lines sent to Grafana

**Requirement → repo/files:** "Logs are kept across updates", "Logs still show live on the hosting platform", "The maintainer can tell whether export is on" (start-up record), "Hosted only" → `tcgreplay`: `internal/telemetry`, `internal/observe`, `internal/hostauth`, `cmd/tcgreplay`, `.do/app.yaml`, `Dockerfile`, `docs/observability.md`.

**File changes:**
- `internal/telemetry/recorder.go:52-62` — add `Provider string` to `Event`. `line` (`:65-76`) gains `Provider string \`json:"provider,omitempty"\`` after `Assistant`, keeping field order with provider last before message. Coordinate with the sign-in plan: if it has already added the field, keep its.
- `internal/telemetry/recorder.go` — add `Line` (exported view of a written record plus its JSON bytes), `Sink` interface, `SetSink`, `RecordContext(ctx, e)` and `ActiveContext(ctx, email)`. `Record`/`Active` become wrappers with `context.Background()`. The sink call happens after `r.w.Write` and outside `r.mu` (copy the bytes), so a slow sink can never block other records. A nil sink does nothing.
- `internal/telemetry/recorder_test.go` — sink receives identical bytes, `Provider` written, nil sink and nil recorder safe, email never in `Line`.
- `internal/observe/sink.go` (new) — `recordSink.Emit(ctx, l)` emits a `log.Record`:
  - body: string of `l.JSON` without its newline;
  - severity: INFO, or ERROR when `l.Level=="error"`;
  - attributes: `event`, `player`, `tool`, `action`, `assistant`, `reason`, `provider` (each only when non-empty), and `log.source="record"`;
  - timestamp `l.Time`, emitted with ctx so trace and span IDs attach.

  Counters are added in the "Usage counted as metrics" task.
- `internal/observe/logwriter.go` (new) — `lineWriter` implements `io.Writer`, splitting on `\n` and buffering partial lines (cap 64 KB, truncating). For each line it strips the std `log` date/time prefix `2006/01/02 15:04:05 ` and emits a record: body = the rest, severity WARN if it starts with `warning:` else INFO, `log.source="log"`. Writes never block: the batch processor drops.
- `internal/hostauth/google.go:233` — `Record(telemetry.Event{Name: telemetry.SignIn, Email: acct.Email, Provider: "google"})` (context threading comes in the tool-call task).
- `cmd/tcgreplay/hosted.go:144-154` `hostedRecorder` — unchanged signature; the sink is set in runHosted.
- `cmd/tcgreplay/hosted.go:458-507` `runHosted`:
  - Before `newHosted`: `tel := observe.Start(ctx, observe.SettingsFromEnv(os.Getenv) + Version, Now: c.now, Stderr: os.Stderr)`. When enabled, `log.SetOutput(io.MultiWriter(os.Stderr, tel.LogWriter()))`.
  - After `newHosted`: `reg.Recorder().SetSink(tel.Sink())`.
  - Append `exportStatus, historyStatus` to the start-up line (`:489`). Log a warning line first when settings are incomplete.
  - On return: `tel.Shutdown(context.Background())` after `<-stopped`, before the "stopped" line is printed (so that line still reaches stderr).
- `cmd/tcgreplay/main.go:248-256` — unchanged (hosted keeps stderr). Desktop paths never call observe.
- `cmd/tcgreplay/hosted_test.go:749-772` / `cardsets_test.go:273-295` — let `startHostedWith` take an optional `observe.Settings` (pointing at a fake receiver) through a `config` field `telemetry *observe.Settings` (tests only), so the in-process server is wired like production.
- `cmd/tcgreplay/hosted_telemetry_test.go` (new):
  - every stdout record during a session (sign-in, import, error) arrives as a log record with an identical body;
  - log lines arrive;
  - stdout and stderr bytes are equal with sending on and off (compare via `c.events` and `captureLog`);
  - unreachable, hanging and 401 receivers: sign-in, import and a board action succeed, and each request finishes within the off-case duration plus 50ms;
  - subprocess start-up line (`helperCmd`, `hosted_test.go:553-598`) says on, off or incomplete and never contains the token, following `TestMainHostedSaysPlayerIDsOn` (`hosted_events_test.go:195-215`).
- `cmd/tcgreplay/desktop_telemetry_test.go` (new) — set `TCGREPLAY_GRAFANA_*` to a fake receiver's URL, run the in-process mode (`runInProcess`) and the daemon with a game and a board action, and assert the receiver got zero requests.
- `cmd/tcgreplay/hosted_test.go:1294-1324` — add `TCGREPLAY_GRAFANA_TOKEN` to the SECRET-keys list.
- `.do/app.yaml:116-230` — add the five `TCGREPLAY_GRAFANA_*` envs (`scope: RUN_TIME`; TOKEN `type: SECRET`, no value; others with no value), with a comment that they are set in the console and are optional.
- `Dockerfile:5-24` — add the Grafana settings to the settings comment.
- `docs/observability.md` "What is recorded" table — add `provider` to the field list and to `sign_in`'s row (full rewrite comes later).

**Complexity:** Medium · **Token estimate:** ~50k · **Agent strategy:** 2 parallel agents (telemetry recorder + observe sink/writer; then sequential hosted wiring and tests).

### Task: Every HTTP request and outside call traced

**Requirement → repo/files:** "Every request is traced", "Traces start at the edge" (origin half), "Nothing identifies a player" (spans) → `tcgreplay`: `internal/observe`, `cmd/tcgreplay`.

**File changes:**
- `internal/observe/http.go` (new) — `Handler(h)`:
  - `otelhttp.NewHandler(routeNamer(h), "http", WithTracerProvider(tp), WithMeterProvider(metricnoop.NewMeterProvider()), WithPropagators(propagation.TraceContext{}), WithPublicEndpointFn(func(*http.Request) bool { return false }))`. Public-endpoint off, so the incoming context is the parent rather than a link.
  - `routeNamer` serves `h` then reads `r.Pattern`. The outer mux sets it on the same `*http.Request` (Go 1.22+); inner muxes see copies (research). `trace.SpanFromContext(r.Context()).SetName(method + " " + pathPart(pattern))` and sets `http.route`. An empty pattern (moved redirects) uses the method only.
  - `Client(base)` copies base (or `&http.Client{Timeout: 30s}`) with `Transport: otelhttp.NewTransport(base.Transport or http.DefaultTransport, WithTracerProvider, WithMeterProvider(noop), WithPropagators(TraceContext))`.
  - `otelhttp` keeps `http.Hijacker`/`http.Flusher` through httpsnoop; test the websocket and SSE.
- `internal/observe/filter.go` (new) — `filterExporter` wraps the span exporter. Each `ReadOnlySpan` is replaced by a struct embedding it with:
  - `Attributes()` filtered to the allow-list (data_structures), with `url.full` and `http.target` stripped of `?…`;
  - `Events()` returning only events named `tcgreplay.*`;
  - `Status()` with its description passed through a regexp that cuts `?[^ "]*` from URLs.

  It sits between the batcher and the health wrapper: `WithBatcher(health(filter(exp)))`.
- `cmd/tcgreplay/hosted.go:491` — compose `tel.Handler(movedGuard(...))` inside a new `hostedServeHandler(c, tel, …) http.Handler`. Use it in runHosted and in `serveHosted` (`cmd/tcgreplay/cardsets_test.go:292`) so the tests run the same wrapper.
- `cmd/tcgreplay/hosted.go:115-137` `hostedAuth` — add `HTTPClient: tel.Client(nil)` to `hostauth.Config` (field `internal/hostauth/hostauth.go:51-52`). Google's token exchange already passes the request ctx via `oauth2.HTTPClient` (`internal/hostauth/google.go:115-131`), so its client span is a child of the callback span. Tests use `oauth2.Endpoint` for the fake Google (`hostauthtest/google.go:75`), which is unaffected.
- `cmd/tcgreplay/main.go:328-330` `newCatalog` — hosted passes `HTTP: tel.Client(&http.Client{Timeout: 30 * time.Second})` (`internal/catalog/loader.go:69,423`). Add a parameter, or set the field in runHosted after `newCatalog`.
- `cmd/tcgreplay/hosted.go:373-425` `hostedImages` — set `cfg.HTTP = tel.Client(...)` before `spaces.New(cfg)` (`internal/spaces/config.go:33`, `client.go:43-52`), `cdn.HTTP = tel.Client(&http.Client{Timeout: 10s})` (`spaces/cdn.go:56-67`) and `images.HTTP = tel.Client(images.HTTP)` (`internal/cards/images.go:83`). `hostedImages` gains the `tel` parameter.
- `internal/observe/http_test.go`, `filter_test.go` (new), using `tracetest.NewInMemoryExporter` behind the filter:
  - spans named by route for `GET /board`, `/mcp`, `/ws`, `/api/`, `/`;
  - an incoming `traceparent` (flag 00 and 01) gives the same trace ID and parent;
  - the filter drops `client.address`, `network.peer.address`, `user_agent.original` and `url.query`, and strips `?code=…&state=…`;
  - a websocket echo and an SSE flush work through `Handler`.
- `cmd/tcgreplay/hosted_trace_test.go` (new):
  - the fake-Google sign-in gives a callback trace containing a client span to the fake token endpoint;
  - the admin card-set check has a client span for the manifest fetch;
  - no span has the forbidden attributes;
  - no `http.server.*` metrics reach the fake receiver.

**Complexity:** Medium · **Token estimate:** ~45k · **Agent strategy:** 2 parallel agents (observe http+filter; hosted wiring of clients), then sequential integration tests.

### Task: Tool calls and board actions traced with records linked to their trace

**Requirement → repo/files:** "Every request is traced" (tool calls, board actions), "Logs link to their trace" → `tcgreplay`: `internal/observe`, `internal/hostauth`, `internal/usergames`, `internal/webui`, `internal/cacheadmin`, `cmd/tcgreplay`.

**File changes:**
- `internal/observe/mcp.go` (new):
  - `CarryContext(h)`: `propagation.TraceContext{}.Inject(r.Context(), propagation.HeaderCarrier(r.Header))`, overwriting any incoming `traceparent` with the current server span's, then serve h.
  - `MCPMiddleware(acct)`: for `tools/call` only, extract the trace context from `req.GetExtra().Header` into the handler's ctx (replacing the stale span context it carries). Start `tools/call <tool>` (internal) with `tcgreplay.tool`, `tcgreplay.assistant` (closed set via `mcpserver.AssistantName` + collapse) and `tcgreplay.player` (from `acct`, a `func(mcp.Request) (playerID, assistant string)` closure built in hosted wiring from `hostauth.AccountFromToken` + `rec.ID`). Call next with the new ctx. On error, or `CallToolResult.IsError`, set status Error. The redacted message is set by usergames, so here only the code is set.
  - Background: go-sdk gives every call the initialize request's ctx (`go-sdk@v1.6.0/mcp/streamable.go:488`); headers are per call (`:1162-1166`); knowledge `gotchas/go-sdk-sessions-and-auth.md`.
- `cmd/tcgreplay/hosted.go:262` — `a.RequireToken(tel.CarryContext(mcp.NewStreamableHTTPHandler(...)))`.
- `cmd/tcgreplay/hosted.go:353` — `srv.AddReceivingMiddleware(tel.MCPMiddleware(acctFn), a.MCPMiddleware(), reg.MCPMiddleware())` in one call, so tracing is outermost (knowledge: middleware order).
- `internal/hostauth/limit.go:75-119` — `allow(ctx, acct)`. Record `Limited` with `RecordContext(ctx, …)` (`:86`). Callers: `MCPMiddleware` passes its ctx (now the tool span's), `BoardLimit(ctx)` (`:123-128`) passes ctx, and `RequireSession` (`board.go:74-79`) passes `r.Context()`.
- `internal/hostauth/google.go:217-235` — `signIn(ctx, w, acct)`. Record `SignIn` with ctx. The caller at `:94` passes `r.Context()`. `Places.Admit(acct)` becomes `Admit(ctx, acct)` in the `Places` interface (`internal/hostauth/hostauth.go` ~60-71). SignInRefused at `google.go:95` uses `RecordContext(r.Context(), …)`. If the sign-in plan has landed, this goes into `finishSignIn` instead.
- `internal/hostauth/admin.go:44,53`, `board.go:134` — `RecordContext(r.Context(), …)`.
- `internal/usergames/usergames.go:118-150` — `Use(ctx, acct)` and `Admit(ctx, acct)` (implements the changed `Places`). `Active` → `ActiveContext(ctx, …)` (`:134`), Limited "places" → `RecordContext(ctx, …)` (`:145`). Callers:
  - `board.go:38` passes `req.Context()`;
  - `board.go:70` (`BoardLimit`) passes ctx;
  - `mcp.go:20-49` `ResolveMCP`/`ResolveMCPQuiet` stop ignoring their ctx (`_ context.Context`) and pass it;
  - `usergames.go:143` (Admit → Use) passes ctx.

  `Release`, `expireIfIdle` and `sweepLocked` (`:152-156,223,297`) stay context-free.
- `internal/usergames/board.go:34` — BoardRefused with `req.Context()`.
- `internal/usergames/telemetry.go:24-69` — use `RecordContext(ctx, …)` for ToolCall, Error, Import and NewGame (ctx is the tool span's because tracing is outermost). On failure also `trace.SpanFromContext(ctx).SetStatus(codes.Error, redacted)`, using the same redacted string as the record. That needs only the OTel trace API (`go.opentelemetry.io/otel/trace`, `codes`), no SDK.
- `internal/usergames/telemetry.go:85-90` `BoardError(ctx, …)` — `RecordContext(ctx, …)`, and set the span status with the redacted message.
- `internal/cacheadmin/handler.go:36,63,97` — `RecordContext(r.Context(), …)` through the service's recorder (`cacheadmin.go` Recorder field).
- `internal/webui/server.go` — add `Trace func(ctx context.Context, verb string) (context.Context, func(error))` to `Server`. `internal/webui/actions.go:90-121` `dispatch`: `if s.Trace != nil { ctx, end = s.Trace(ctx, req.Verb); defer end(err) }` before `Limit`, so `Limit`, `do` and `OnError` get the action ctx.
- `internal/observe/board.go` (new) — `BoardSpan(ctx, verb)` starts a new root span `board <verb>` (`trace.WithNewRoot()`) with `trace.WithLinks(trace.LinkFromContext(ctx))` (the `/ws` server span from the upgrade ctx, `webui/hub.go:199,260`) and `tcgreplay.board.verb`. End sets Error on err.
- `cmd/tcgreplay/hosted.go:358-370` `hostedBoard` — set `ui.Trace = tel.BoardSpan` (nil-safe when off: `BoardSpan` with noop providers is cheap; still set).
- Tests:
  - `internal/hostauth/events_test.go`, `internal/usergames/events_test.go`, `internal/usergames/telemetry_test.go` and `internal/cacheadmin/events_test.go` move to the new signatures.
  - `cmd/tcgreplay/hosted_trace_test.go`:
    - two tool calls on one MCP session give two traces, each the trace of its own `POST /mcp`, each with a `tools/call` child;
    - a rate-limited call has a span and its `limited` log record carries that trace ID;
    - a failed `import_log` (malformed log) gives an error log record whose trace and span IDs equal the tool span's, and the span status is Error with the redacted message;
    - a board action gives a `board <verb>` root span linked to the `/ws` span;
    - sign-in, sign-out and admin records carry trace IDs, and idle-expiry records carry none.
  - The webui test: with `Trace` nil, dispatch is unchanged.

**Complexity:** High · **Token estimate:** ~70k · **Agent strategy:** parallel analysis (hostauth+usergames context threading; webui hook; observe MCP/board), sequential integration in cmd and the integration tests.

### Task: Image copies joined to their card request and the end-to-end privacy sweep

**Requirement → repo/files:** "Every request is traced" (image store call as its own part), "Nothing identifies a player" → `tcgreplay`: `internal/cards`, `internal/webui`, `cmd/tcgreplay`.

**File changes:**
- `internal/cards/images.go:231-272` — add `ImageContext(ctx, src)`. `Image(src)` calls it with `context.Background()`. When it queues `rel` (`:264-266`), record `ic.spanCtx[rel] = trace.SpanContextFromContext(ctx)` if valid (new map beside `origin`, guarded by `ic.mu`, deleted when the job finishes or is dropped).
- `internal/cards/images.go:420-540` — the worker, on taking `rel` (`:430`), builds `jctx := trace.ContextWithSpanContext(ctx, sc)` when present, and passes `jctx` to the source fetch (`:576-583`) and the store PUT/HEAD, so their client spans (tracing transports from the previous task) become children of the card request's span. Imports only `go.opentelemetry.io/otel/trace`.
- `internal/webui/server.go:66` — the `Images` interface method becomes `ImageContext(ctx context.Context, src string) (image, fallback, version string)`. Call sites `server.go:240,250` (in `serveCard`) pass the request ctx. Update `internal/webui` test fakes.
- `internal/cards/images_test.go` — a queued copy carries the caller's span context into its fetch and put (using a recording transport), and `Image(src)` without context still works.
- `cmd/tcgreplay/hosted_telemetry_e2e_test.go` (new) — the scripted session against the fake receiver, compression off, with fake Google, fake Spaces (`cmd/tcgreplay/hosted_cache_test.go` helpers) and a TCGdex stub:
  - Steps: Google sign-in through the connector, then MCP initialize, `import_log` with a battle log naming cards and players and an opponent decklist, `new_game`, an `import_log` with a malformed log, the board page, `/ws` with one action, `/api/card` for an uncopied image, and the admin check.
  - Assert the receiver's spans include a server span for each HTTP request the test made (count by route), a `tools/call` per call, a `board` span per action, and a client span to the fake Spaces PUT inside the `/api/card` trace.
  - Then search every received body (raw bytes) for: the email, the fake Google ID token/access token/code, the OAuth state, the session cookie value, the access token, `usergames.Handle(acct.ID)`, `127.0.0.1` client address strings in attributes, the test User-Agent marker, distinctive battle log lines, the decklist card names, and the player names. None may appear. `p_` IDs do appear.
- `cmd/tcgreplay/noemail_test.go:13-95` — extend the AST scan's call matcher to `attribute.String`/`attribute.StringSlice`, `(*Span).SetAttributes`, `log.String`/`KeyValue` constructors and `trace.WithAttributes` arguments containing an `.Email` selector. Add `internal/observe` to the scanned packages.

**Complexity:** Medium · **Token estimate:** ~50k · **Agent strategy:** 2 parallel agents (image cache context; e2e test + static check), sequential final run.

### Task: Usage counted as metrics

**Requirement → repo/files:** "Usage is counted over time", "Sign-ins are counted by provider" → `tcgreplay`: `internal/observe`, `internal/mcpserver` (read-only use).

**File changes:**
- `internal/observe/metrics.go` (new) — instruments from the meter `tcgreplay`:
  - `tcgreplay.player_days` (Int64Counter, unit `{player}`), `tcgreplay.sign_ins` (`{sign_in}`, attr `provider`), `tcgreplay.tool_calls` (`assistant`, `tool`), `tcgreplay.imports` and `tcgreplay.new_games` (`assistant`), `tcgreplay.errors` (`kind`), `tcgreplay.limited` (`reason`).
  - Mimir stores them as `tcgreplay_<name>_total`. Pass unit `1` or a `{…}` annotation so no unit suffix is appended; verify the stored names against the format doc and fix the dashboard and runbook queries to match.
- `internal/observe/sink.go` — `Emit` also increments, by `l.Event`:
  - `player_day` → player_days;
  - `sign_in` → sign_ins{provider: l.Provider or "google"};
  - `tool_call` → tool_calls{assistant: collapse(l.Assistant), tool: closedTool(l.Tool)};
  - `import`, `new_game` → by assistant;
  - `error` → errors{kind: "tool" if l.Tool != "" else "board"};
  - `limited` → limited{reason}.

  Other events increment nothing.
- `internal/observe/labels.go` (new):
  - `collapse(name)`: keep `Claude`, `ChatGPT`, `Codex`, `Gemini` (the names `mcpserver.AssistantName` returns, `internal/mcpserver/server.go:179-191`), else `other`.
  - `closedTool(name)`: keep names in the registered tool set (pass the list from hosted wiring: `mcpserver` tool names), else `other`.
- `internal/observe/metrics_test.go` (new) — with `sdkmetric.NewManualReader`: each event increments the right counter with the right attributes; `sign_in` without a provider counts `google` and with `password` counts `password`; an unknown assistant gives `other`; no attribute key is `player`.

**Complexity:** Low · **Token estimate:** ~20k · **Agent strategy:** single agent.

### Task: Daily usage kept for six weeks with a dashboard

**Requirement → repo/files:** "Usage is counted over time" (28 days) under Grafana Free's 14-day metric retention → `tcgreplay`: `internal/observe`, `cmd/tcgreplay`, `docs/grafana/`.

**File changes:**
- `internal/observe/history.go` (new):
  - `history` holds `table map[dayKey]float64`, where `dayKey{Day, Count, Provider, Assistant string}`, behind a mutex.
  - `query(ctx, promql, at)` makes a GET to `<MetricsURL>/api/v1/query` with Basic auth (`MetricsUser:Token`) and a 10s timeout, parsing the Prometheus `vector` JSON. Its own HTTP client gets the health wrapper's notion: failures are noted as signal `history`.
  - `refresh(ctx)`:
    1. `max by (day,count,provider,assistant) (last_over_time(tcgreplay_usage_daily[14d]))` merges into the table with max.
    2. For each completed UTC day d in the last 13 days not in the table, at `time=<d+1 00:00 UTC>`, run:
       - `round(sum(increase(tcgreplay_player_days_total[1d])))` → players;
       - `round(sum by (provider)(increase(tcgreplay_sign_ins_total[1d])))` → sign_ins;
       - `round(sum by (assistant)(increase(tcgreplay_tool_calls_total[1d])))` → tool_calls;
       - the same for imports and new_games;
       - `round(sum(increase(tcgreplay_errors_total[1d])))` → errors.

       An empty result for a day the playmat exported metrics records 0. A day with no raw data at all is skipped.
    3. Drop days older than 42.
  - The `run` loop: refresh at start (after 30s, letting raw metrics exist) and then hourly.
  - An observable gauge `tcgreplay.usage_daily` (unit `1`) has a callback that observes every table entry with attributes `day`, `count` and `provider`/`assistant`.
  - Off unless export is on and both metrics settings are set; the status text says why.
- `cmd/tcgreplay/hosted.go` `runHosted` — `go tel.Run(ctx)` (runs the health watch and history), and the start-up line includes the history status.
- `internal/observe/history_test.go` (new) — a fake Prometheus API (`httptest`) returning canned vectors per query and time:
  - the merge keeps the max;
  - computed days are added;
  - running twice gives the same table;
  - day 43 is dropped;
  - a 500 or unreachable API keeps the table and notes a failure;
  - the gauge callback emits one point per entry (ManualReader).
- `docs/grafana/tcgreplay-usage.json` (new) — dashboard JSON model. Panels: daily distinct players, sign-ins by provider, tool calls by assistant, imports, new games, errors (bar charts over `tcgreplay_usage_daily{count="…"}` by `day`, Grafana table/bar with `day` label as x). Plus "last 14 days" time series from `increase(...[1d])` on the raw counters.
- `internal/observe/dashboard_test.go` (new) — read the JSON (path relative to the repo), extract every `expr`, and assert each metric name is one the code defines (a list exported from `metrics.go`/`history.go`) and each label in a `{…}` selector is one of the defined labels.

**Complexity:** High · **Token estimate:** ~55k · **Agent strategy:** parallel analysis (history logic; dashboard JSON + test), sequential integration.

### Task: Observability runbook rewritten for Grafana

**Requirement → repo/files:** "The runbook reads from the new place" → `tcgreplay`: `docs/observability.md`.

**File changes:**
- `docs/observability.md`:
  - **Intro.** Replace "Everything is read in DigitalOcean…" with: records, logs, traces and app metrics in Grafana Cloud (Germany, Free tier); platform metrics and alerts in App Platform; live view in Runtime Logs.
  - **What is recorded / never recorded / Player IDs.** Keep, add `provider`, and add "what traces carry" (route, duration, status, tool, assistant, player ID, outside calls without query strings) and "what traces never carry".
  - **New "Setting up Grafana Cloud".**
    - Create an access policy token with `metrics:read metrics:write logs:write traces:write`.
    - The OTLP endpoint and instance ID come from the stack's OpenTelemetry tile; the Prometheus query URL and user from the stack's Prometheus details.
    - Set the five settings at app level in the console (token encrypted) and never paste the app spec file (knowledge `gotchas/app-spec-not-applied-by-deploy.md`).
    - Saving restarts the playmat. Check the start-up line. Import `docs/grafana/tcgreplay-usage.json`.
  - **"Reading daily usage in Grafana"** replaces the Insights section. Covers the dashboard, the Explore query per number (`tcgreplay_usage_daily{count="players"}` etc.), and the raw-counter queries for the last 14 days. Note the player-day double count after a restart.
  - **"Finding a player's errors"**, step 3 replaced: in Grafana Explore → Loki, `{service_name="tcgreplay"} | player="p_…" | event="error"`; open the `trace_id` link to Tempo; from a Tempo span, "Logs for this span". Note how to enable the derived-field/trace-to-logs link if the stack's data sources lack it.
  - **New "Traces".** Search by route, tool or player in Tempo.
  - **New "Turning on edge traces"** with Cloudflare steps:
    - turn on Traces for the zone;
    - add a destination of type Traces with the endpoint `<OTLP URL>/v1/traces` and header `Authorization: Basic …`;
    - enable it on the zone; turn persistence off; turn "Forward to origin" on;
    - leave incoming context at Reject.

    Note the Free plan's 0.5 GB/day from 2026-12-01, and a "Status" line recording whether it is on or why it was dropped.
  - **New "Export status".** The start-up line's export and usage-history parts, and the five-minute failure line.
  - **Graphs / Alerts / Capacity.** Keep (DigitalOcean).
  - **Retention.** A table:
    - records (Grafana Loki), **14 days**, source grafana.com/pricing (checked 2026-11-23);
    - traces (Tempo), **14 days**;
    - raw app metrics, 14 days;
    - daily usage totals, **six weeks** (carried by the playmat);
    - platform metrics (App Platform), keep the existing figure and note;
    - alert history, 30 days;
    - Cloudflare traces (if on), 7 days in Cloudflare if persisted.

    State the figures exactly as the privacy page will quote them ("14 days", "six weeks", "7 days").
- Update the knowledge entry `gotchas/app-platform-logs-only-in-insights.md` through `spek-knowledge` (not by hand) to say records now go to Grafana via OTLP; the implementer does this at the end of the task.

**Complexity:** Medium · **Token estimate:** ~30k · **Agent strategy:** single agent.

### Task: Privacy page names Grafana Cloud

**Requirement → repo/files:** "The privacy page is accurate" → `tcgreplay`: `site/src/pages/privacy.astro`, `site/e2e/privacy.spec.js`, `site/src/data/whats-new.yaml`.

**File changes:**
- `site/src/pages/privacy.astro:1-20` — the header comment's sources now name the Grafana retention (from the runbook) and the four figures checked.
- `site/src/pages/privacy.astro:138-168` (Diagnostics):
  - Platform metrics stay with DigitalOcean (`data-retention="metrics"` with the runbook's figure).
  - The playmat's records and request traces (route, timing, outside calls, player ID; never the never-list) are sent to Grafana Cloud and stored in Germany: records `<span data-retention="logs">14 days</span>`, traces `<span data-retention="traces">14 days</span>`, daily totals with no player ID `<span data-retention="totals">six weeks</span>`.
  - Remove "Both are kept in DigitalOcean's Insights service".
  - Extend the never-contain sentence with "passwords" if the sign-in plan has landed; otherwise leave it.
- `site/src/pages/privacy.astro:210-225` (Outside services):
  - DigitalOcean no longer "keeps the diagnostics (Insights)"; it keeps its platform metrics.
  - Add `<li><strong>Grafana Cloud</strong> (Grafana Labs) stores the diagnostics records and request traces, in Germany …</li>`.
  - If Cloudflare Traces is turned on, Cloudflare's item gains "and traces requests": write that conditional wording now as "may trace requests" so it holds either way.
- `site/e2e/privacy.spec.js:69-108`:
  - the diagnostics test expects "Grafana Cloud", "Germany" and "traces", and not "Insights";
  - the outside-services test expects Grafana Cloud;
  - the retention cross-check loops over `logs`, `traces`, `totals` and `metrics`.
- `site/src/data/whats-new.yaml` — new top entry: `date` the release day, `title: Privacy page: where diagnostics are kept`, `refresh: false`, `changes` ("The [privacy page](/privacy) now says diagnostics records and request traces are kept in Grafana Cloud, in Germany, for 14 days."). Quote lines containing `: `.
- Rebuild the site (`internal/site/dist`) and run the site browser suite.

**Complexity:** Low · **Token estimate:** ~20k · **Agent strategy:** single agent.

### Task: Turn on sending to the Grafana stack

**Human task. What to do:**
1. In Grafana Cloud, open the stack and confirm its region is a German one (`prod-eu-west-2` or `-4`; `prod-eu-central-0` is Switzerland). If it is not, stop: the privacy page says Germany.
2. Create an access policy with `metrics:read`, `metrics:write`, `logs:write`, `traces:write` for this stack and generate a token.
3. Copy the OTLP endpoint and instance ID from the OpenTelemetry tile, and the Prometheus query URL and user from the stack's Prometheus details.
4. Only after the privacy page change is live, open the App Platform console, Settings > App-Level Environment Variables, and add `TCGREPLAY_GRAFANA_OTLP_URL`, `TCGREPLAY_GRAFANA_INSTANCE_ID`, `TCGREPLAY_GRAFANA_TOKEN` (Encrypt), `TCGREPLAY_GRAFANA_METRICS_URL` and `TCGREPLAY_GRAFANA_METRICS_USER`. Do not paste `.do/app.yaml`. Save (the playmat restarts).
5. In Runtime Logs, check the start-up line says `telemetry export: on (Grafana Cloud …)` and `usage history: on`.
6. Import `docs/grafana/tcgreplay-usage.json` as a dashboard.

**How it is checked:** the start-up line, then records in Loki, traces in Tempo and `tcgreplay_*` metrics in Mimir within a few minutes, and the dashboard showing today's raw counts (daily totals appear after the first full UTC day).

### Task: Turn on Cloudflare edge traces

**Human task. What to do:**
1. In the Cloudflare dashboard for the tcgreplay.app zone, open Observability. Check that Traces and OTLP export are offered on the zone's current (Free) plan. If OTLP export needs a paid plan, do not upgrade: write in the runbook's "Turning on edge traces" status line that the edge part is dropped because export is not on the Free plan (date checked), and stop.
2. Add a destination (type Traces) named Grafana Cloud. Endpoint `<TCGREPLAY_GRAFANA_OTLP_URL>/v1/traces`, header `Authorization` = `Basic <base64(instanceID:token)>` (a separate traces-only token is fine).
3. Turn on Traces for the zone, attach the destination, turn persistence off, set a baseline sampling rate the 0.5 GB/day Free allowance covers (start at 100%, lower it if the daily cap is hit), turn "Forward to origin" on, and leave incoming context at Reject. Change no routing, proxy or DNS setting.
4. Request `https://tcgreplay.app/board` and find the trace in Tempo. Check whether the playmat's `GET /board` span is in the same trace as Cloudflare's spans. If not, App Platform's edge may be dropping the header: record that in the runbook status line as the reason the edge part is dropped.

**How it is checked:** one trace in Tempo holding Cloudflare's and the playmat's spans for a board page, or the runbook's status line recording why not.

## Testing Strategy

Per task, the tests to add are listed under each task above. Across the plan:

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

## Project References

- Spec: `20261123114225-5cf4d0df-grafana-observability` (epic `20261111093348-e1b5c2b4-public-launch`).
- Design documents: none.
- Knowledge used:
  - `gotchas/app-platform-logs-only-in-insights.md` (update it at the end of the runbook task);
  - `gotchas/app-spec-not-applied-by-deploy.md`;
  - `gotchas/go-sdk-sessions-and-auth.md` (session context and middleware order);
  - `gotchas/disconnect-does-not-reach-playmat.md` (privacy wording);
  - `gotchas/live-site-cf-bm-cookie.md`;
  - `architecture/visitor-counting-serve-time.md` (optional-setting pattern);
  - `architecture/published-card-sets.md` (catalogue and image clients);
  - `learnings/grafana-cloud-free-and-cloudflare-traces.md`;
  - conventions `privacy-page-tracks-outside-services`, `whats-new-entries`, `skills-text-and-packaging`.
- Plans:
  - `20261112090330-4ac2ea19-observability` (landed);
  - `20261122190321-c6d9d90a-sign-in-page` (approved, not landed; shared files listed in plan.md Dependencies).
- Repo root: `tcgreplay` at `/home/nicj/code/github.com/nicholasjackson/tcgreplay`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

The tool-call/board-action task (High, ~70k) touches four packages' signatures at once. Do the context threading in hostauth and usergames first, so `go build ./...` stays green, then the hooks, then the integration tests. The usage-history task (High) separates the pure merge logic (unit-testable) from the HTTP query client.

## Migration Notes

- **No data migration.** Nothing is stored by the playmat.
- **Settings.** Five new optional app-level settings, set in the console by the maintainer (Turn on sending task) only after the privacy page change is live. Add only the new entries in the console editor; never paste `.do/app.yaml` (it carries no secret values).
- **Records.** `sign_in` records gain a `provider` field. Anything reading the stdout records (only the maintainer, by eye) is unaffected.
- **Sign-in page plan interplay.** Whichever plan lands second merges `Event.Provider` (keep one field) and moves this plan's context parameter into `finishSignIn` (or keeps it there).
- **Signatures changed** (internal only): `hostauth.Places.Admit(ctx, acct)`, `usergames.Registry.Use(ctx, acct)`, `cards.ImageCache.ImageContext`, and the webui `Images` interface. All callers are in this repo.

## Performance Considerations

- **Request path cost.** One server span per request, a tool or board span, and log emission into a non-blocking batch queue. Ending a span or emitting a record never waits on the network. The no-op providers when export is off cost near nothing.
- **Memory.** Bounded by the two 2,048-entry batch queues, the metric aggregation state (closed label sets) and the usage-history table (about 800 entries). The SDK drops data under pressure rather than growing.
- **Long-lived requests.** `/ws` and MCP SSE streams keep one open server span for their lifetime. That is cheap: one span object, exported on close.
- **Volume against the free tier.** Every request is traced. At launch scale (10 players) this is far below 50 GB/month, but the "monthly volume" manual check watches it, and sampling is the later lever (out of scope).
- **Series budget.** About 800 daily-gauge series plus at most a few hundred counter series, with otelhttp HTTP metrics off, sit well under 10k.
- **Latency check.** The spec's 5% median criterion is measured by hand on the deployed playmat. The hung-receiver test guards the mechanism.
