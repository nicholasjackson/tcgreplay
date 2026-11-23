---
created_date: "2026-11-23"
document_status: final
closed_date: "2026-11-23"
---

# Grafana Cloud observability for the hosted playmat

## What was built

The hosted playmat can now send its diagnostics to the maintainer's Grafana Cloud stack (Germany, Free tier) over OpenTelemetry. Sending is controlled by five optional settings, `TCGREPLAY_GRAFANA_*`, and stays off until the maintainer sets them.

- **One telemetry package, hosted mode only.** `internal/observe` builds OTLP/HTTP exporters for logs, traces and metrics.
  - It uses bounded batch queues that drop rather than block, a 5-second timeout, no retries and gzip.
  - With the settings unset or incomplete it gives no-op providers, and the start-up line says so: `telemetry export: on/off (…)`, `usage history: on/off (…)`.
  - A health watch writes one stderr line after five minutes of unbroken export failure, at most every five minutes. The line names the signals and the kind of failure, never the token.
  - Shutdown flushes for at most two seconds.
  - The desktop, daemon and in-process modes never build it.
- **Records and log lines.** The existing usage and error recorder still writes every record to stdout unchanged. It also hands each record, with its request's context, to a sink that sends it as a log record whose body is the same JSON line. Plain log lines go out too, through a writer beside stderr, including the setting warnings logged before telemetry starts. Sign-in records now carry `"provider":"google"`.
- **Traces of every request.** An otelhttp server wrapper names each span by its route and joins the W3C trace context Cloudflare forwards. The sampler is always on and HTTP metrics are off. Tracing clients cover Google's token exchange, card set fetches, the image store, CDN purges and image sources.
  - **Tool calls.** The go-sdk hands every tool call the context of the session's first request. So each MCP request's span is written into its headers and read back by an outermost MCP middleware, putting every `tools/call <tool>` span inside its own `POST /mcp` trace.
  - **Board actions** get their own `board <verb>` traces, linked to the live connection.
  - **Image copies** that a card request queues run under that request's span.
  - **Records carry their trace.** Request contexts are threaded through sign-in, the limits, the per-account registry, sign-out release and admin actions, so their records carry the request's trace.
  - **Privacy filter.** A span exporter allow-list keeps client addresses, user agents, query strings (Google's code, OAuth state, board handles) and library events out.
- **Usage metrics and six weeks of daily totals.**
  - **Counters.** They count player-days, sign-ins by provider, tool calls by assistant and tool, imports, new games, errors and limit refusals. Labels are closed sets (unknown assistants and tools count as `other`) and never carry a player ID.
  - **Daily totals.** Grafana Cloud Free keeps metrics for 14 days, so a usage history reads back the daily table it sent before, hourly and at start-up. It adds each finished UTC day computed from the raw counters, drops days past six weeks and re-sends the table as the `tcgreplay_usage_daily` gauge. The playmat stores nothing.
  - **Dashboard.** `docs/grafana/tcgreplay-usage.json` charts every count per day.
- **Runbook, privacy page and deploy files.**
  - `docs/observability.md` is rewritten around Grafana: setup, daily usage, a player's errors via the admin lookup, traces, Cloudflare edge traces, export status, and a Retention table with sources.
  - The privacy page names Grafana Cloud, Germany, 14 days for records and traces and six weeks for daily totals, and drops the Insights claim. A What's new entry announces it. The browser suite checks every retention figure on the page against the runbook.
  - `.do/app.yaml` declares the new settings, with the token as a secret, and the Dockerfile's settings note lists them.
- **Tests.**
  - Unit tests for every observe component.
  - Hosted integration tests against a fake OTLP receiver: identical record bodies, unchanged stdout and stderr, failing receivers adding no wait, the start-up lines, the desktop sending nothing.
  - An end-to-end session that proves every request, outbound call, tool call and board action is a span, and that nothing identifying is ever sent.
  - An AST check that no email reaches a log, span or log attribute.

## Why it matters

Before this change the hosted playmat's logs disappeared on every deploy, and nothing showed how a request moved through it. Now the maintainer can:

- search every record and log line after updates;
- follow a request from Cloudflare's edge through the playmat to Google, the card data and the image store;
- go from a player's error record to its trace and back;
- chart daily usage for six weeks, including Google sign-ins by provider, ready for judging the sign-in page's release.

All of this happens without recording who a player is, and without slowing players down when Grafana is unreachable.

## Deviations from the plan

- `observe.Settings` gained `HTTPClient` (tests trust a TLS fake receiver), and `Telemetry` gained `ForceFlush`, `Run`, `SetTools` and `HistoryOn`. Set-but-wrong settings report `off (incomplete: …)`.
- Outbound tracing clients send no trace context to outside services. Export health takes HTTP statuses from a transport wrapper.
- Telemetry starts first in `runHosted`, and `main` keeps the log lines written before the mode is known so hosted mode can send them.
- `BoardSpan` takes the player ID. `Places.Release` and the auth store's release path take a context, so the sign-out `account_removed` record carries its trace.
- The tool label's closed set is read by listing the MCP server's tools in process. The sign-in provider label passes through as recorded (`google`, or `password` later).
- The daily gauge uses unit `{count}`, so Grafana stores it as `tcgreplay_usage_daily` and not `…_ratio`. A day is computed only when the raw counters have data for it.
- The runbook's Retention table also lists the visitor-count figure (six months) that the privacy page already showed, so every figure on the page is checked.
- The two human tasks were skipped by the user's decision and are listed in the test plan for after merge and deploy:
  - "Turn on sending to the Grafana stack";
  - "Turn on Cloudflare edge traces".
- The stored metric names are still to be confirmed against the live stack (plan Open Question).
