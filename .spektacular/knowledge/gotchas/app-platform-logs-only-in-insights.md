---
tags: [hosting, digitalocean, logs, observability, insights]
---

# App Platform runtime logs are not reliably kept anywhere without log forwarding

- **App Platform's own log viewer keeps no history.** It shows the running deployment's runtime logs live. DigitalOcean's docs say "To retain runtime logs, configure log forwarding to an external provider", and the 90-day retention they mention covers build and deploy logs only. Every push to main outside `cardsets/` redeploys the playmat, so the app's Runtime Logs tab cannot answer "what happened yesterday".
- **Do not count on DigitalOcean Insights for App Platform logs.** On 2026-11-12 the maintainer reported the playmat's log lines in Insights Logs. On 2026-11-19 that did not hold: Insights > Logs showed **0 results over 7 days** in both AMS3 (where the app actually runs) and LON1, although Insights > Settings listed the app with Logs and Traces enabled and the app's Runtime Logs tab showed fresh start-up lines. This matches Insights' own docs and launch post (October 2026), which say App Platform sends only metrics and logs come from DOKS. Before relying on Insights for usage or error records, re-check that a fresh log line from the app actually shows up there; if it does not, the records need log forwarding (`log_destinations` on the component in the app spec: Managed OpenSearch, Datadog, Logtail and the like).
- **Search the region the app really runs in.** Insights shows one region at a time. The live app is in AMS3 (see `gotchas/app-spec-not-applied-by-deploy.md`), whatever `.do/app.yaml` says.
- **Insights' limits (when it does hold logs):**
  - A log search spans at most 7 days, in the console and in the API (`POST /v2/insights/query/{region}/logs/search`).
  - There is no group-by or distinct count: count things by filtering for one record type and reading the match count.
  - Retention for logs and metrics is not documented; only alert history (30 days) is.
  - There is no endpoint for sending our own telemetry, OpenTelemetry or otherwise: records get in only as stdout/stderr lines.
  - Billing starts at general availability (planned for 2026-11-19): platform metrics, dashboards and alerts stay free; logs and traces are billed beyond 5 GB a month per billing team, and extended retention and log forwarding are billed too.
- **App Platform metrics do arrive in Insights.** CPU, memory, restart count, crash-loop backoff, replicas, request rate and P95 duration show in the app's Insights tab and in DigitalOcean Insights > Metrics for the app's region (confirmed in AMS3 on 2026-11-19). Alert rules for `CPU_UTILIZATION`, `MEM_UTILIZATION` and `RESTART_COUNT` can be declared per component under `alerts:` in `.do/app.yaml`, but they reach the platform only when that file is applied. The app spec cannot name who the alerts go to: set the recipients in the control panel.
- **The retention figures in the runbook are provisional.** `docs/observability.md` (Retention) states 7 days for usage records, error records and platform metrics, marked "to be confirmed by the maintainer", on the assumption that Insights holds the logs. The privacy page's retention statements are tied to these figures: when where the logs live or how long they are kept changes, update the runbook and the privacy page together.
