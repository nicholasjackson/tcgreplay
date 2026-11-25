---
tags: [grafana, observability, hosting, metrics]
---

# The usage history needs metrics:read on the access policy, and its failure never shows in Grafana

The playmat's six-week daily table (`tcgreplay_usage_daily`, `internal/observe/history.go`) is built by **reading back** from Grafana Cloud's Prometheus query API every hour, then re-exporting the gauge. The write-only scopes Grafana's "OpenTelemetry" setup tile creates (`metrics:write`, `logs:write`, `traces:write` and friends) are not enough: the token also needs **`metrics:read`**. Without it every read-back is refused, `refresh` stops before filling the table, and the gauge is exported with no rows, so the series simply never exists in Prometheus. Everything else (logs, traces, the raw `tcgreplay_*_total` counters) keeps arriving, so the stack looks healthy.

**Why Grafana shows nothing wrong.** The health watch writes its `warning: telemetry export: … (history: rejected (HTTP 401))` line to `os.Stderr` only (`cmd/tcgreplay/hosted.go` passes `os.Stderr`, never the log writer that feeds Loki), so it appears in App Platform's Runtime Logs and nowhere in Loki. Searching Grafana for warnings finds none.

**How to check:** in Explore (Prometheus), `count by (__name__) ({__name__=~"tcgreplay.*"})` should list `tcgreplay_usage_daily` a day after start-up. If it is missing, open the stack's access policy (grafana.com > Access Policies > Edit) and confirm **metrics: Read** is ticked; then check Runtime Logs for the history warning's HTTP code: 401/403 is the scope, 404 is the URL (see the `grafana-metrics-url-bare-host` gotcha). Policy changes take up to 15 minutes.

Found 2026-11-25: the policy `stack-1936126-otlp-write` had no metrics Read, and the daily series had never been written since export went live on 2026-11-23.
