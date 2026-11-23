---
created_date: "2026-11-23"
document_status: draft
project: tcgreplay
spec: 20261123114225-5cf4d0df-grafana-observability
plan: 20261123114225-5cf4d0df-grafana-observability
---

# Grafana Cloud observability

The hosted playmat can now keep its usage and error records, its log lines, a trace of every request and daily usage counts in Grafana Cloud. It names players only by their pseudonymous ID and never slows them down when Grafana is unreachable. Sending stays off until the maintainer sets the new optional Grafana settings. The privacy page now says diagnostics are kept in Grafana Cloud, in Germany, for 14 days, with daily totals kept for six weeks. The desktop playmat sends nothing.

> Derived from project tcgreplay, spec/plan 20261123114225-5cf4d0df-grafana-observability. See the project-level record for the full feature.

## What changed in this repo

- **New `internal/observe` package** (hosted mode only).
  - Builds OTLP/HTTP exporters with bounded, non-blocking queues and a health watch. It reads the optional settings and reports their status on the start-up line.
  - Adds the record sink and the log line writer.
  - Adds the HTTP server wrapper and the tracing clients, with a span privacy filter.
  - Adds the MCP header carrier and tool-call middleware, and board action spans.
  - Adds the usage counters and the six-week usage history gauge.
  - Adds a fake OTLP receiver for tests (`internal/observe/observetest`).
- **`internal/telemetry`.** The recorder gained a sink, `RecordContext`/`ActiveContext`, and a `provider` field on sign-in records.
- **Context threading.** `internal/hostauth`, `internal/usergames` and `internal/cacheadmin` thread request contexts so records carry their trace. `Places.Admit` and `Places.Release` take a context.
- **`internal/webui`.** Gained a board action trace hook and a context-taking image interface.
- **`internal/cards`.** Queued image copies run under the card request's span.
- **`cmd/tcgreplay`.** Hosted wiring starts telemetry first, tees log output, sets the sink, wraps the handler, injects tracing clients and places the MCP middleware outermost. It reports export and usage history status, and flushes on shutdown.
- **Tests.** Hosted, desktop and end-to-end telemetry tests, and the no-email static check extended to span and log attributes.
- **Deploy files.** `.do/app.yaml` declares the five `TCGREPLAY_GRAFANA_*` settings (the token is a secret), and the `Dockerfile` settings note lists them.
- **`docs/observability.md`.** Rewritten for Grafana Cloud. `docs/grafana/tcgreplay-usage.json` is the new usage dashboard.
- **Site.** `site/src/pages/privacy.astro`, `site/e2e/privacy.spec.js`, `site/src/data/whats-new.yaml` and the privacy and What's new screenshots are updated.
- **Dependencies.** OpenTelemetry Go v1.47.0 (API, SDK, logs, metrics, OTLP/HTTP exporters) and otelhttp v0.72.0 are added to `go.mod`.

## Why

The playmat's logs used to vanish on every deploy, and there was no way to see how a request moved through it. This change gives the maintainer searchable history across updates, request timing out to the services the playmat calls, and daily usage counts. Errors link to their traces. Players stay pseudonymous and unaffected.

## After merging

The maintainer turns sending on by following `docs/observability.md`, "Setting up Grafana Cloud" and then "Turning on edge traces", once the privacy page is live. The stored metric names should then be checked against the dashboard and runbook queries.
