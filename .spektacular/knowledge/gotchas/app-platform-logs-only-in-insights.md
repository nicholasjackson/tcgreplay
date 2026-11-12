---
tags: [hosting, digitalocean, logs, observability, insights]
---

# App Platform runtime logs are kept only in Insights, whatever its docs say

- **App Platform's own log viewer keeps no history.** It shows the running deployment's runtime logs live. DigitalOcean's docs say "To retain runtime logs, configure log forwarding to an external provider", and the 90-day retention they mention covers build and deploy logs only. Every push to main outside `cardsets/` redeploys the playmat, so the app's Runtime Logs tab cannot answer "what happened yesterday".
- **DigitalOcean Insights does hold them.** The playmat's own runtime log lines appear in Insights (Control Panel > Insights > Logs, region LON1). The maintainer confirmed this in the console on 2026-11-12, while Insights was in public preview. Insights' docs and launch post (October 2026) say App Platform sends only metrics and that logs come from DOKS, so do not conclude from the docs that Insights has no App Platform logs.
- **Insights' limits:**
  - A log search spans at most 7 days, in the console and in the API (`POST /v2/insights/query/{region}/logs/search`).
  - There is no group-by or distinct count: count things by filtering for one record type and reading the match count.
  - Retention for logs and metrics is not documented; only alert history (30 days) is.
  - There is no endpoint for sending our own telemetry, OpenTelemetry or otherwise: records get in only as stdout/stderr lines.
  - It is free during the preview, charged from general availability, and DO says it "should not be used as the primary observability system for production workloads".
- **App Platform metrics are in both places.** CPU, memory, restart count, request rate, P95 duration and CDN requests by status code show in the app's Insights tab and in DigitalOcean Insights. Alert rules for `CPU_UTILIZATION`, `MEM_UTILIZATION` and `RESTART_COUNT` can be declared per component under `alerts:` in `.do/app.yaml`. The app spec cannot name who the alerts go to: set the recipients in the control panel.
