---
tags: [grafana, observability, hosting, opentelemetry]
---

# TCGREPLAY_GRAFANA_METRICS_URL must end in /api/prom; the bare host gives HTTP 404

The usage history reads back from Grafana Cloud's Prometheus query API at `TCGREPLAY_GRAFANA_METRICS_URL` + `/api/v1/query` (`internal/observe/history.go`). The setting must be the stack's **Query Endpoint**, `https://prometheus-prod-NN-<region>.grafana.net/api/prom`.

**The trap.** The hosted-metrics page on grafana.com (Stack > Prometheus > Details) shows the **bare host** in large type under the instance name, above the real Query Endpoint. Copying that, or the Remote Write Endpoint (`…/api/prom/push`), sends the query to a path that does not exist: Grafana answers **404** (a missing token answers 401, so 404 always means the path). Logs, traces and metrics keep exporting, because they use `TCGREPLAY_GRAFANA_OTLP_URL`; only the history fails, as `warning: telemetry export: sending to Grafana Cloud has failed for Nm (history: rejected (HTTP 404))`.

**Why it looks like it broke later.** The history queries once, 30 seconds after the playmat starts, then hourly. A wrong URL therefore surfaces only after a restart (a console save or a deploy), and one failed call keeps the warning repeating every 5 minutes until the next hourly attempt.

**Check from a terminal:** `curl -s -o /dev/null -w '%{http_code}\n' "<METRICS_URL>/api/v1/query?query=up"` without credentials: 401 means the path is right, 404 means it is wrong. Seen 2026-11-24 on the live playmat.
