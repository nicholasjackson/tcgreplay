---
created_date: "2026-11-23"
document_status: final
closed_date: "2026-11-23"
epic: 20261111093348-e1b5c2b4-public-launch
---

# Feature: 20261123114225-5cf4d0df-grafana-observability

## Overview

Today the hosted playmat's logs disappear on every update, and nothing records how a request moved through it, so when something goes wrong the maintainer has little to go on. This change keeps every log line and diagnostic record and traces each request from the moment it reaches the site's edge network, through the playmat, to the services it calls. It also counts sign-ins, assistant actions, imports, games and errors over time, all in one searchable place outside the hosting platform. That gives the maintainer what they need to diagnose problems before and after the public launch, without ever recording who a player is, and without slowing players down when that place is unavailable.

## Requirements

- [ ] **Logs are kept across updates**
  Every log line and diagnostic record the hosted playmat writes (start-up lines, usage records, error records, sign-in records, and records of requests stopped by the rate limit or the player limit) is kept in the observability service. The maintainer can search it after the playmat has been updated or restarted.
- [x] **Logs still show live on the hosting platform**
  The playmat keeps writing the same log lines to the hosting platform's live log view, so watching a deploy there works as it does today.
- [x] **Every request is traced**
  Each assistant tool call, board page or data request, live board connection and sign-in step produces a trace showing what the playmat did and how long each part took. That includes its calls to outside services (Google sign-in, card set data, the card image store). At launch every request is traced, with no sampling. If trace volume would exceed the observability service's free tier, sampling may be introduced.
- [ ] **Traces start at the edge**
  For requests that pass through the site's edge network, the trace includes the edge's part of the request (edge handling and the connection to the playmat) joined to the playmat's own part, so the maintainer can tell whether time was spent at the edge or in the playmat. This applies when the edge network's trace export is available on the maintainer's current plan (see Constraints). If it is not, this requirement is dropped and the reason recorded.
- [x] **Logs link to their trace**
  A log line written while serving a request carries that request's trace identity, so the maintainer can go from an error record to the trace it belongs to, and from a trace to its log lines.
- [ ] **Usage is counted over time**
  The playmat reports counts of distinct players, sign-ins, tool calls by assistant, imports, new games and errors as metrics. The maintainer can chart them per day for at least 28 days, longer than the logs are kept.
- [x] **Sign-ins are counted by provider**
  Once sign-in offers more than one provider, the sign-in count says which provider each sign-in used, so Google sign-ins can be compared before and after a release.
- [x] **Nothing identifies a player**
  No log, trace or metric sent to the observability service contains an email address, name, Google or playmat token, password, battle log, decklist or card text. Players appear only as their pseudonymous player ID.
- [x] **Players are never slowed or stopped by telemetry**
  If the observability service is unreachable, slow, rejecting data or not configured, the playmat keeps serving players with no added delay or errors. Telemetry that cannot be sent is dropped rather than queued without limit.
- [x] **The maintainer can tell whether export is on**
  The playmat's start-up record says whether sending to the observability service is on or off. When sending has been failing for more than five minutes, a record on the hosting platform's live log view says so, repeated at most once every five minutes, without exposing credentials.
- [x] **The runbook reads from the new place**
  The maintainer's observability runbook explains how to read daily usage, find a player's errors from their email (by first looking up the player's pseudonymous ID on the admin page), and open a request's trace in the observability service. These replace its current instructions for DigitalOcean Insights, the hosting platform's monitoring tool.
- [x] **The privacy page is accurate**
  The privacy page names the observability service, the country it stores data in (Germany) and how long it keeps diagnostics records and traces, in place of its current claim that records are kept in DigitalOcean Insights.

## Constraints

- **Grafana Cloud is the observability service.** Logs, traces and app metrics go to the maintainer's existing Grafana Cloud stack (Germany region), on its free tier: no paid tier, and no self-hosted monitoring. This supersedes the earlier rule that everything is read in DigitalOcean.
- **Platform metrics and alerts stay in DigitalOcean.** CPU, memory, restarts and request rate, and the restart/CPU/memory alerts, remain in App Platform. This change adds no alerting in Grafana (the maintainer's decision).
- **Players stay pseudonymous.** Everything sent names a player only by their existing player ID: a one-way hash of their email keyed with a secret kept out of the code and logs, stable across restarts and redeploys.
- **Credentials are hosted secrets.** The Grafana credentials are set as encrypted settings on the hosted app, like its other secrets. They never appear in the repository, the app spec file, logs or telemetry.
- **Settings are optional.** The playmat must start and serve normally with the Grafana settings unset, because deploying code does not apply settings, so the code must never depend on the console having been changed first.
- **No new storage.** The playmat stays a single instance with no database or disk-backed queue. Telemetry adds no persistent storage.
- **Cloudflare is the existing edge.** tcgreplay.app is served through the maintainer's own Cloudflare zone. Edge tracing uses Cloudflare's own trace support, must not change how traffic is routed, and must not require moving to a paid Cloudflare plan.
- **Hosted only.** The desktop playmat must not send telemetry anywhere.

## Acceptance Criteria

- [ ] **Logs survive a redeploy**
  After a player signs in, imports a game through an assistant and the playmat is then redeployed, searching the observability service shows that player's sign-in, tool call and import records, and the start-up records from before and after the redeploy.
- [x] **Live log view unchanged**
  During a deploy, the hosting platform's live log view shows the playmat's start-up record and subsequent records as before.
- [ ] **A tool call's trace**
  After an assistant imports a game, the observability service shows a trace for that tool call with its total duration and separate timed parts for the playmat's own work and each outside call it made (such as fetching card set data).
- [ ] **Board, live connection and sign-in traces**
  Opening a board page, the board's live connection and a full sign-in (to the point the assistant's connection completes) each appear as traces. The sign-in trace shows the call to Google as its own timed part.
- [ ] **Every request traced**
  Over a test session, every tool call, board request, live connection and sign-in step appears as a trace, with none missing. A request that fetches a card image shows the image store call as its own timed part.
- [ ] **Edge joined to the playmat**
  Where edge trace export is available on the maintainer's plan, for a board page requested through tcgreplay.app, a single trace in the observability service shows both Cloudflare's part of the request and the playmat's part.
- [ ] **From error to trace and back**
  When a tool call fails (for example an import of a malformed battle log), its error record in the observability service links to that tool call's trace, and the trace links back to the error record.
- [ ] **Usage charted over 28 days**
  The observability service charts, per day, distinct players, sign-ins, tool calls by assistant, imports, new games and errors. Those counts are still available 28 days after they happened.
- [x] **Sign-ins split by provider**
  The sign-in count carries the provider (Google today). Once the sign-in page with username-and-password accounts ships, the sign-in chart can show Google sign-ins on their own.
- [ ] **No identifying data sent**
  After a test session, searching all logs, traces and metrics in the observability service for the test player's email address, a Google token value, a password used, a pasted battle log's text and a deck's card names finds nothing. The player appears only by pseudonymous ID.
- [ ] **Telemetry outage harmless**
  With the observability service unreachable (or its credentials wrong), a player can still sign in, import a game and play. Median tool-call and board-request durations stay within 5% of those with telemetry off, and the playmat's memory does not keep growing while data cannot be sent.
- [x] **Export state visible**
  The start-up record says whether sending is on or off. With wrong credentials, after five minutes the live log view shows a record saying sending is failing (no more than one every five minutes), and that record contains no credential.
- [x] **Desktop sends nothing**
  Running the desktop playmat and using it makes no connection to the observability service or any other telemetry destination.
- [ ] **Runbook works**
  Following the runbook, the maintainer can read a day's distinct players and tool calls by assistant, find a player's error records from their email via the admin page's player-ID lookup, and open the trace for one of those errors, all in the observability service.
- [x] **Privacy page corrected**
  The privacy page names Grafana Cloud as where diagnostics records and traces are kept, says they are stored in Germany, gives how long they are kept, and no longer says they are kept in DigitalOcean Insights.

## Technical Approach

- **OpenTelemetry from the server.** Instrument the Go server with OpenTelemetry and send logs, traces and metrics over OTLP straight to Grafana Cloud's OTLP endpoint. Prefer sending from the app over the hosting platform's log forwarding, which has no Grafana destination and would lose trace and log correlation.
- **Build on the existing records.** Reuse the existing structured usage and error records as the log events rather than inventing a second event set.
- **Metrics for long-range counts.** Derive the daily usage counts as metrics so they outlast log retention. The sign-in count carries the sign-in provider once the sign-in page spec (20261122190321-c6d9d90a-sign-in-page) adds it.
- **Follow Cloudflare's trace context.** Accept the W3C trace context Cloudflare passes to the origin, so edge and playmat spans join one trace. Use Cloudflare's domain-level Traces with its "Export to Grafana Cloud" OTLP destination for the edge part.
- **Bounded, asynchronous export.** Batch and send in the background with bounded buffers and short timeouts, dropping data under pressure.
- **Known risks.**
  - Cloudflare Traces is in open beta, and its OTLP export may need a paid Cloudflare plan (one source says Workers Paid). Its pricing changes on 2026-12-01. If export is not available on the maintainer's current plan, the edge part is dropped (see the edge requirement).
  - Grafana's free-tier limits and retention (believed about 14 days for logs and traces) are unverified, and so is metric retention. If the free tier keeps metrics for less than 28 days, the plan must find a way to keep the daily counts that long within the free tier. With every request traced, check volume against the free tier after launch.

## Success Metrics

- When a player reports a problem after launch, the maintainer can find the matching error records and the trace of the failing request from the player's email in under five minutes, without opening the hosting platform's console.
- In the first two weeks after launch, no deploy loses diagnostic history: every day has usage and error records searchable in Grafana.
- Telemetry adds no measurable latency: median tool-call duration with telemetry on stays within 5% of the duration with it off.
- The sign-in page's release can be judged by comparing daily Google sign-in counts for the two weeks before and after it, read from Grafana.
- Monthly telemetry volume stays within the Grafana Cloud free tier.

## Non-Goals

- Telemetry from the board running in players' browsers (front-end errors, page timing, session replay).
- Public usage numbers or a stats page.
- Cloudflare request logs (Logpush). Only Cloudflare's traces are in scope.
