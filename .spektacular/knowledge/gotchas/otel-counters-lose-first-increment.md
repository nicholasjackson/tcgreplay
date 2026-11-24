---
tags: [grafana, opentelemetry, observability, metrics, promql]
---

# OTel counters in Grafana lose their first increment to increase()

The playmat's usage counters (`tcgreplay_*_total`, from `internal/observe/metrics.go`) are OpenTelemetry
cumulative counters exported over OTLP to Grafana Cloud. A series is first stored **at its first value**
(usually 1), with no zero sample before it. PromQL's `increase()` and `rate()` only count growth *between*
samples, so that first increment is never counted.

**Why it matters here:** traffic is low, and new series appear often:

- every deploy starts a new process with a new `service_instance_id`, so every counter starts a new series;
- every new label value (a tool not called before, a new assistant, a new sign-in provider) starts a new series.

So at launch volumes `increase()` reads 0 for a count that is really 1 to 3, and every deploy silently drops one
event per series. Checked on 2026-11-23 against the live stack: raw `tcgreplay_tool_calls_total` was 1 for each of
three tools, `sum(increase(...[1h]))` gave 0, and the query below gave 3.

**Count per series, treating a series that is new in the window as starting from zero:**

- Total over a range `R`:
  `sum((last_over_time(M[R]) - last_over_time(M[5m] offset R)) or last_over_time(M[R]))`
- Per-step bars:
  `sum by (label) ((M - M offset $__interval) or (M unless M offset $__interval))`
  (keep the panel interval at least the export interval, 60 s).

These are exact; `increase()` also extrapolates, giving fractional counts.

**Where this bites:**

- `internal/observe/history.go` computes the six-week daily totals with `round(sum(increase(M[1d])))`, so the
  daily table (and the sign-in comparison built on it) undercounts by one per series per day that a series is new.
- `docs/grafana/tcgreplay-usage.json`'s raw-counter panels use `increase(...[1d])`.
- The `tcgreplay overview` dashboard in the Grafana stack (uid `tcgreplay-overview`) already uses the queries above.

**Related trap for latency panels:** `GET /mcp` server spans are the long-lived MCP event stream (about 125 s each)
and `/ws` is the board websocket. Leave both out of request-duration panels and slow-request tables
(`name!="GET /mcp" && name!~"GET /ws.*"`), or they swamp the p95.
