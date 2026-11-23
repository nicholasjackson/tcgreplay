---
tags: [observability, grafana, opentelemetry, cloudflare, hosting, retention, traces]
---

# Grafana Cloud Free and Cloudflare Traces: limits that shape the playmat's telemetry

Checked against the official docs on 2026-11-23 while planning the Grafana observability spec.

## Grafana Cloud Free (grafana.com/pricing, grafana.com/products/cloud/free-tier)

- **Every signal is kept 14 days on Free, metrics included.** Logs: 50 GB/month, 14 days. Traces: 50 GB/month, 14 days. Metrics: 10k active series, **14 days**. The 13-month metric retention is the Pro tier. So anything that must be readable for longer than two weeks (the spec's 28-day daily counts) needs its own answer.
- 3 active users a month. What happens once the 50 GB or 10k-series allowance is used up (dropped or rejected) is not documented.
- **Germany is `prod-eu-west-2` or `prod-eu-west-4`** (AWS; `prod-eu-west-5` is Azure). `prod-eu-central-0` is Switzerland. Copy the OTLP endpoint from the stack's OpenTelemetry tile rather than build it; stacks created from 2026-01-15 use a nested host name.
- **OTLP auth** is HTTP Basic: the stack's instance ID as user, an access policy token (scopes `metrics:write`, `logs:write`, `traces:write`) as password. A bare `glc_` token after "Basic" does not work.
- OTLP logs land in Loki with `trace_id`/`span_id` as structured metadata (dots become underscores); `service.name` becomes the `service_name` label. Counters land in Mimir as `<name>_total`. Send cumulative temporality (the Go SDK's default).
- Histograms cost one series per bucket per label combination, so per-route HTTP histograms eat the 10k-series allowance quickly.

## Cloudflare Traces (blog.cloudflare.com/cloudflare-tracing, developers.cloudflare.com/observability/)

- Domain-level tracing, open beta since 2026-10-02, exportable to an OTLP destination; the Grafana Cloud destination takes the OTLP endpoint plus `/v1/traces`, a Basic `Authorization` header, and JSON only.
- **Forwarding a `traceparent` to the origin is opt-in** ("Forward to origin"); it does not create origin spans by itself. Incoming trace context defaults to Reject.
- **From 2026-12-01** traces share the account's Observability allowance: Free gets 0.5 GB/day ingestion, 7 days retention, no overage (ingestion stops until 00:00 UTC). Persistence can be turned off to export only.
- No doc states whether domain-level OTLP export works on the Free plan (Workers trace export needed Workers Paid). Check it in the dashboard before relying on it.
