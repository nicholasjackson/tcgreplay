# Working context: grafana-observability (20261123114225-5cf4d0df-grafana-observability)

## Problem and motivation
- User (2026-11-23): "no logs or traces are being captured by insights". Then: "I would like logs and traces to work, this will help diagnose, we need this before launch".
- Known cause (knowledge gotcha `gotchas/app-platform-logs-only-in-insights.md`): App Platform sends only metrics to DigitalOcean Insights. On 2026-11-19 Insights Logs showed 0 results over 7 days in AMS3, although Insights settings listed Logs and Traces as enabled. The app's Runtime Logs tab is live only and keeps no history across deploys (every push to main redeploys).
- There is no tracing at all today: no OpenTelemetry in the code or in go.mod.
- The earlier spec 20261112090330-4ac2ea19-observability (public-launch epic) built structured JSON usage and error records on stdout (internal/telemetry; player IDs keyed with TCGREPLAY_PLAYER_ID_KEY, never emails; event catalogue in docs/observability.md) and assumed Insights would hold them. This spec corrects that.
- The sign-in-page plan (20261122190321-c6d9d90a-sign-in-page, approved today) has a success metric (Google sign-ins two weeks before vs after release) that depends on logs being kept. Its plan notes the dependency on log forwarding. It adds a `provider` field to sign_in records.

## Decisions so far
- Provider: **Grafana Cloud** (user: "ok we do grafana cloud").
  - Logs, traces and metrics go via OpenTelemetry/OTLP straight from the Go server. No App Platform log_destinations.
  - Config as encrypted env vars, like the other secrets.
  - The free tier is believed to be about 50 GB logs and 50 GB traces with 14-day retention (unverified).
  - EU region available.
- Joins the public-launch epic (20261111093348-e1b5c2b4-public-launch), which this reopens (the user agreed).
- Optional: Cloudflare edge traces (Cloudflare Observability, open beta) if tcgreplay.app is the maintainer's own Cloudflare zone (unknown). Cloudflare can pass W3C trace context to the origin. Cloudflare cannot ingest logs or traces from a non-Cloudflare server: its OTel support is export only.

## Alternatives rejected (and why)
- DigitalOcean Insights: holds no App Platform logs, and its traces are for Action Gateway only.
- Better Stack: an App Platform forwarding destination, but the token is plain text in the app spec, forwarded lines are cut at 2,000 bytes, traces still need OTel, and its free tier is smaller.
- Self-hosting on DigitalOcean (Grafana stack on a Droplet, or Managed OpenSearch): monthly cost and ops burden for a one-maintainer project, and logs are lost when it falls over.
- A Cloudflare Worker log relay into Workers Logs: an extra service, no app traces.

## Points to settle in the spec (raised in discussion)
- Retention vs the 28-day sign-in comparison: count sign-ins as a metric, not only as log lines.
- What is traced (MCP tool calls, board requests, sign-in steps, card fetches) and the sample rate.
- Keep emails, card text, tokens and secrets out of spans and log attributes (same "player ID, never email" rule).
- Privacy page and docs/observability.md must name Grafana Cloud, what it receives and the retention (privacy convention). Same for the retention figures (gotcha says runbook and privacy page retention go together).
- Hosted only; the desktop playmat has no telemetry export.
- Needed before launch. It can run in parallel with the sign-in page work (different code).

## Interview answers (2026-11-23)
- Platform metrics and alerts stay in DigitalOcean (split). Grafana holds app logs, traces and app metrics.
- No Grafana alerts.
- tcgreplay.app is the maintainer's own Cloudflare zone, so Cloudflare edge traces are in scope.
- The Grafana Cloud stack already exists, in Germany (EU).
- Defaults accepted: keep stdout logs too; trace everything at launch (no sampling); a Grafana outage never hurts players; free-tier retention is fine (longer counts as metrics).
- Privacy page: the user is indifferent ("not sure I give much of a shit about the privacy page"), so keep only the minimal factual update the convention requires (it currently claims Insights keeps logs 7 days).
- Epic dependency recorded: grafana-observability depends on 20261112090330-4ac2ea19-observability.
- The earlier observability spec's constraint "everything in DigitalOcean, no second vendor" is reversed by the user's decision.
- User asked (2026-11-23) to draft the whole spec without per-section confirmations and give a summary at the end.
- Spec drafted and reviewed; reviewer fixes applied without asking (user delegated): edge tracing conditional on current Cloudflare plan (no paid plan), sampling allowed past free tier, distinct players added, 5% latency threshold, export-failure record after 5 min.
- Split check: not offered (one capability; logs, traces and metrics share one export path and link to each other; edge traces depend on app traces).

## Plan-epic run (2026-11-23)
- Orchestrating spek-plan-epic for epic 20261111093348-e1b5c2b4-public-launch. Only grafana-observability needs a plan (the other 6 are done).
- grafana-observability planned (DONE). Summary section written; no cross-plan disagreements. epic order added deps on privacy-terms-disclaimer, visitor-analytics (and maybe more).
- Repeat run (2026-11-23): status shows all 7 specs planned, nothing to start; summary already has a grafana-observability section. No review (no plan produced in this run).

## Implement-epic run (2026-11-23)
- Epic 20261111093348-e1b5c2b4-public-launch: 6 specs done; started child for grafana-observability in worktree .spektacular/worktrees/20261123114225-5cf4d0df-grafana-observability/tcgreplay (branch spek/20261123114225-5cf4d0df-grafana-observability, from HEAD of studio-design-alignment). Merge goes into the current branch.
- Child QUESTION (human tasks 10 Grafana sending, 11 Cloudflare edge traces): user chose option 1, skip both and finish; maintainer does them after deploy via docs/observability.md.
- Child asked to commit on spek branch at finished step: user approved commit and merge (message "Send records, traces and usage metrics to Grafana Cloud", Co-Authored-By, no Claude-Session trailer).
- Commit bb23b1c on spek branch (message had to name the spec; added "Spec:" line). Merged into studio-design-alignment as 408c046; worktree removed. Status still shows grafana implement "ready" (9/11) because human tasks 10 and 11 are unticked by design; no further child started.
