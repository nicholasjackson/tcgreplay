# Observability: usage, errors, traces, graphs and alerts

The maintainer's runbook for watching the hosted playmat at
`https://tcgreplay.app`. It covers:

- what the playmat records, what its traces carry, and what it never sends;
- how players are named;
- setting up Grafana Cloud and turning sending on;
- how to read each day's usage, find a player's errors from their email,
  and open a request's trace;
- turning on Cloudflare's edge traces;
- what the export status lines mean;
- the graphs to watch, the alerts, capacity and retention.

Where each thing is read:

- **Records, log lines, traces and usage metrics: Grafana Cloud.** The
  playmat sends them over OTLP to the maintainer's Grafana Cloud stack (a
  German region, Free tier): records and log lines to Loki, request traces
  to Tempo, usage counters and the daily totals to Mimir. They survive
  redeploys and restarts. Sending is off until the `TCGREPLAY_GRAFANA_*`
  settings are set (see [Setting up Grafana Cloud](#setting-up-grafana-cloud)).
- **Platform metrics and alerts: DigitalOcean.** CPU, memory, restarts,
  request rate and the alert rules stay in App Platform. Nothing is alerted
  from Grafana.
- **Live view: App Platform's Runtime Logs tab.** The playmat still writes
  every record to standard output and every log line to standard error,
  exactly as before, so a deploy can be watched there. That tab keeps no
  history. DigitalOcean Insights receives no App Platform logs, so it is
  not used for records.

## What is recorded

Each record is one line, with its fields always in this order. An empty
field is left out.

| field | meaning |
|---|---|
| `time` | UTC time, RFC 3339 with milliseconds, e.g. `2026-11-12T09:30:00.123Z` |
| `level` | `error` for the `error` event, otherwise `info` |
| `event` | one of the events below |
| `player` | the player's pseudonymous ID (`p_` and 16 hex characters); absent when no player is known |
| `tool` | the MCP tool called |
| `action` | the board action, or the admin action |
| `assistant` | the assistant that made the call, as the playmat names it (`Claude`, `ChatGPT`, `Codex`, `Gemini`, or the client's own name) |
| `provider` | the sign-in provider (`google`), on `sign_in` |
| `message` | an error or refusal message, redacted (see below) |
| `reason` | why a request was refused or an account removed |
| `idle` | the idle time an expired game went past (`TCGREPLAY_IDLE_TIMEOUT`, in words, e.g. `60 minutes`) |

Some example lines:

```json
{"time":"2026-11-12T09:30:00.123Z","level":"info","event":"sign_in","player":"p_3f9a1c0b7d2e4a68","provider":"google"}
{"time":"2026-11-12T09:30:00.456Z","level":"info","event":"player_day","player":"p_3f9a1c0b7d2e4a68"}
{"time":"2026-11-12T09:31:12.003Z","level":"info","event":"tool_call","player":"p_3f9a1c0b7d2e4a68","tool":"import_log","assistant":"Claude"}
{"time":"2026-11-12T09:31:12.004Z","level":"info","event":"import","player":"p_3f9a1c0b7d2e4a68","assistant":"Claude"}
{"time":"2026-11-12T09:35:40.871Z","level":"error","event":"error","player":"p_3f9a1c0b7d2e4a68","tool":"import_log","message":"line 1: \"…\" before the Setup section"}
{"time":"2026-11-12T09:40:02.310Z","level":"info","event":"limited","player":"p_3f9a1c0b7d2e4a68","message":"too many requests: this account is limited to 300 requests a minute; try again in 12s","reason":"rate"}
```

The events form a closed list:

| event | fields beyond time, level, event and player | written when |
|---|---|---|
| `sign_in` | `provider` (`google`) | a Google sign-in completes, for the board or an assistant connection |
| `sign_in_refused` | `reason` (the refusal, e.g. the server is busy) | a completed Google sign-in is refused at the user cap |
| `sign_out` | none | a board session signs out |
| `player_day` | none | a player's first activity in a UTC day: a sign-in, a tool call, a board page load or a board action |
| `tool_call` | `tool`, `assistant` | every MCP tool call that gets past the request limit |
| `import` | `assistant` | `import_log` succeeds |
| `new_game` | `assistant` | `new_game` succeeds |
| `error` | `tool` or `action`, `message` | a tool call or a board action fails |
| `limited` | `reason` (`rate` or `places`), `message` | the request limit or the user cap refuses a request |
| `game_expired` | `idle` (the idle time it went past) | an idle game is dropped |
| `account_removed` | `reason` (`signed_out` or `idle`) | an account's entry is removed: its last sign-in ended, or it went a day without activity |
| `board_refused` | none | a player opens another account's board link |
| `admin_refused` | `reason` (`not admin` or `not sent from the playmat`) | a non-admin reaches /admin, or an admin change comes from another site |
| `admin_action` | `action` (`check`, `clear` or `lookup`) | an admin checks for card sets, clears images or looks up a player's ID |

The playmat's other log lines name no player and stay plain text: start-up,
card sets, card images, refused tokens and client registrations. They reach
Grafana too, as log records marked `log.source="log"` (records are marked
`log.source="record"`), at `WARN` when they start `warning:`. The start-up
line includes `player IDs on`, then the export and usage history status (see
[Export status](#export-status)). The admin page's own text lines for
card-set checks and image clears name the admin by player ID.

In Grafana each record is a Loki log line whose body is the JSON line
above. Its fields are also structured metadata (`event`, `player`, `tool`,
`action`, `assistant`, `reason`, `provider`), and a record written while
serving a request carries that request's `trace_id` and `span_id`.
Background records (idle games, idle accounts) carry none.

## What traces carry

Every request to the playmat is traced, with no sampling. A trace holds:

- **A server span per request**, named by its route (`GET /board`,
  `POST /mcp`, `GET /ws`, `GET /auth/google/callback`, `GET /api/`, and
  `GET /` for website pages), with its method, route, status code, path and
  timing.
- **A span per assistant tool call**, `tools/call <tool>`, inside the trace
  of the `POST /mcp` request that carried it. It names the tool, the
  assistant (`Claude`, `ChatGPT`, `Codex`, `Gemini` or `other`) and the
  player ID. A failed call is marked failed with the redacted message of its
  error record.
- **A span per board action**, `board <verb>`, in a short trace of its own
  linked to the live connection's `GET /ws` span, naming the action and the
  player ID.
- **Client spans for outside calls**: Google's token exchange (inside the
  sign-in callback's trace), card set fetches, image store writes, CDN
  purges and image source fetches. Each is named by method and host, with
  the address but never its query. An image copy that a card request queues
  shows inside that card request's trace.

A request that arrives with Cloudflare's W3C trace context joins
Cloudflare's trace (see [Turning on edge traces](#turning-on-edge-traces)).

Traces never carry a client address, a browser's user agent, a query string
(so no Google authorization code, OAuth state or board link handle), an
email, a token, a battle log or card names. Span attributes pass an
allow-list in the playmat; anything else a library adds is dropped.

Usage metrics carry only closed sets of labels (provider, assistant, tool,
kind, reason, day, count), never a player ID.

## What is never recorded

No record, log line, trace or metric contains any of these:

- a player's email address or name;
- a Google token, or the playmat's own tokens and cookies;
- a battle log, a decklist, or card data.

Callers hand the recorder an email address, which it writes only as the
player ID. Should the address appear in a message, it is cut out.

Error messages are the one place a player's own text could reach a record:
the playmat's errors can quote battle-log lines and name cards and players.
So every error message is redacted before it is written:

- every double-quoted part becomes `"…"`;
- every run of words that also appears in the player's own text becomes `…`.
  The player's text means the tool call's arguments (the pasted log and
  decklists, names), the current game's player and card names, and an
  imported game's log and decklists.

The playmat's own words, such as `before the Setup section` or `has no … in
play`, are kept, so the message still says which check failed. Redaction
prefers to hide too much rather than too little.

## Player IDs

A player ID is `p_` followed by the first 16 hex characters of a keyed
HMAC-SHA256 (`internal/telemetry/ids.go`):

- the HMAC key is SHA-256 of `tcgreplay player-id`, a zero byte, and the
  `TCGREPLAY_PLAYER_ID_KEY` secret;
- the message is `tcgreplay player`, a zero byte, and the player's email
  address, trimmed and lower-cased.

The fixed prefixes keep these IDs apart from any other hash of the same
secret or email. The admin page's lookup works out the same value, so there
is no need to compute an ID by hand.

- **Stable.** The same account has the same ID across sign-ins, assistants,
  restarts and redeploys, for as long as the key stays the same.
- **One-way.** Someone with the records cannot turn an ID back into an
  email. Without the key, hashing a list of known emails matches nothing.
- **The key** is a secret of at least 32 characters (`openssl rand -base64
  48`). Set it as an encrypted variable on the playmat component in the App
  Platform control panel, and keep a copy in a password manager. Hosted mode
  will not start without it.
- **Rotating the key** gives every player a new ID, so records written before
  the change no longer match the same players. Rotate it only if it leaks.

## Setting up Grafana Cloud

Do this once, and only after the privacy page describing Grafana Cloud is
live. Diagnostics must never go to a service the privacy page does not name.

1. **Check the region.** In Grafana Cloud, open the stack's details. Its
   region must be a German one (`prod-eu-west-2` or `prod-eu-west-4`;
   `prod-eu-central-0` is Switzerland). The privacy page says Germany: if
   the stack is elsewhere, stop.
2. **Create the token.** Under Administration > Users and access > Cloud
   access policies, create a policy for this stack with the scopes
   `metrics:read`, `metrics:write`, `logs:write` and `traces:write`, then
   add a token to it. Copy the token; it is shown once.
3. **Find the addresses.** On the stack's page:
   - the **OpenTelemetry** tile (Configure) gives the OTLP endpoint
     (`https://otlp-gateway-….grafana.net/otlp`) and the instance ID;
   - the **Prometheus** tile (Details) gives the query URL
     (`https://prometheus-….grafana.net/api/prom`) and its user (the
     metrics instance ID).
4. **Set the settings.** In the App Platform console, open the app's
   Settings > App-Level Environment Variables and add:

   | setting | value | encrypt |
   |---|---|---|
   | `TCGREPLAY_GRAFANA_OTLP_URL` | the OTLP endpoint | no |
   | `TCGREPLAY_GRAFANA_INSTANCE_ID` | the instance ID | no |
   | `TCGREPLAY_GRAFANA_TOKEN` | the token | **yes** |
   | `TCGREPLAY_GRAFANA_METRICS_URL` | the Prometheus query URL | no |
   | `TCGREPLAY_GRAFANA_METRICS_USER` | the Prometheus user | no |

   Add only these entries in the console editor. Never paste
   `.do/app.yaml` into it: the file declares the settings but holds no
   values, and deploys do not apply it.
5. **Save.** The playmat restarts. In Runtime Logs, the start-up line
   should include `telemetry export: on (Grafana Cloud otlp-gateway-…)` and
   `usage history: on (prometheus-…)`. Within a few minutes, Explore shows
   the records in Loki (`{service_name="tcgreplay"}`), traces in Tempo, and
   `tcgreplay_` metrics in the Prometheus data source.
6. **Import the dashboard.** Go to Dashboards > New > Import, upload
   `docs/grafana/tcgreplay-usage.json`, and choose the stack's Prometheus
   data source. The daily totals fill in after the first full UTC day.

To stop sending, remove the settings (or just the token) and save.

## Reading daily usage in Grafana

The **tcgreplay usage** dashboard charts each count per UTC day:

- six weeks of daily totals: distinct players, sign-ins by provider, tool
  calls by assistant, imports, new games and errors;
- the last 14 days from the raw counters, including tool calls by tool and
  limit refusals by reason.

Grafana Cloud Free keeps metrics for only 14 days, so the playmat carries
the daily totals itself. Every hour it does the following:

1. reads back the totals it sent before;
2. adds each finished day, computed from the raw counters;
3. drops days older than six weeks;
4. sends the table again as the `tcgreplay_usage_daily` gauge.

It stores nothing on disk; Grafana holds the table.

To read one number in Explore, use an instant query on the Prometheus data
source:

| number | query |
|---|---|
| Distinct players | `max by (day) (last_over_time(tcgreplay_usage_daily{count="players"}[42d]))` |
| Sign-ins by provider | `max by (day, provider) (last_over_time(tcgreplay_usage_daily{count="sign_ins"}[42d]))` |
| Tool calls by assistant | `max by (day, assistant) (last_over_time(tcgreplay_usage_daily{count="tool_calls"}[42d]))` |
| Imports | `max by (day, assistant) (last_over_time(tcgreplay_usage_daily{count="imports"}[42d]))` |
| New games | `max by (day, assistant) (last_over_time(tcgreplay_usage_daily{count="new_games"}[42d]))` |
| Errors | `max by (day) (last_over_time(tcgreplay_usage_daily{count="errors"}[42d]))` |

To read one day, add `day="YYYY-MM-DD"` to the selector. Today's numbers are
not in the table yet. They come from the raw counters, for example
`sum by (assistant) (increase(tcgreplay_tool_calls_total[1d]))` or
`sum(increase(tcgreplay_player_days_total[1d]))`. Assistants other than
Claude, ChatGPT, Codex and Gemini count as `other`; their own names are in
the `tool_call` records.

The players count can be slightly high. The playmat remembers who it has
seen today only in memory, so after a restart or redeploy during the day, a
player who is active again is counted a second time.

## Finding a player's errors

When a player reports a problem:

1. Sign in to `https://tcgreplay.app/admin` as an admin (an account in
   `TCGREPLAY_ADMINS`).
2. Under **Find a player's ID**, enter the email address the player signed
   in with. The page shows their ID, for example `p_3f9a1c0b7d2e4a68`. The
   address is posted, so it never appears in a URL, and it is not logged.
3. In Grafana, open **Explore** with the Loki data source, set the time
   range around the report, and run:

   ```text
   {service_name="tcgreplay"} | player="p_3f9a1c0b7d2e4a68" | event="error"
   ```

   Leave out `| event="error"` to see everything the player did:
   - their tool calls, with the tool and the assistant;
   - their imports and new games;
   - any limits they hit, and their sign-ins.
4. **Open the error's trace.** Expand an error line: its `trace_id` links to
   Tempo. The trace shows the tool call or board action, how long each part
   took, and any outside call it made. If the line shows a `trace_id` but no
   link, add a derived field to the Loki data source: under Connections >
   Data sources > Loki > Derived fields, name it `trace_id`, match the
   label, and make it an internal link to the Tempo data source with query
   `${__value.raw}`.
5. **From a trace back to its logs.** In Tempo, a span's **Logs for this
   span** button opens the records written while serving it. If the button
   is missing, open the Tempo data source's **Trace to logs** setting,
   choose the Loki data source and filter by trace ID.

## Traces

In Explore with the Tempo data source, search by:

- **route**: `{ name = "POST /mcp" }` or `{ span.http.route = "/board" }`;
- **tool call**: `{ name = "tools/call import_log" }` or
  `{ span.tcgreplay.tool = "import_log" }`;
- **player**: `{ span.tcgreplay.player = "p_3f9a1c0b7d2e4a68" }`;
- **failures**: `{ status = error }`;
- **slow requests**: `{ duration > 2s }`.

A board action's trace is short and linked to its live connection's
`GET /ws` trace, which stays open as long as the board does.

## Turning on edge traces

Cloudflare's domain-level Traces can add the edge's part of a request
(Cloudflare's handling and its connection to the playmat) to the same trace
as the playmat's part. It is configured in the Cloudflare dashboard, not in
code, and it changes no routing.

1. In the Cloudflare dashboard for the tcgreplay.app zone, open
   Observability. Check that Traces and OTLP export are offered on the
   zone's current (Free) plan. If OTLP export needs a paid plan, do not
   upgrade: record that under **Status** below and stop.
2. Add a destination of type **Traces**, named Grafana Cloud, with:
   - the endpoint `<TCGREPLAY_GRAFANA_OTLP_URL>/v1/traces`;
   - the header `Authorization: Basic <base64 of instanceID:token>` (a
     separate traces-only token is fine).
3. Turn on Traces for the zone and attach the destination, then set it up
   as follows:
   - **Persistence**: off, so the free daily allowance is not spent on
     Cloudflare's own storage.
   - **Sampling**: start at 100%, and lower it only if the daily cap is hit.
   - **Forward to origin**: on.
   - **Incoming context**: leave at **Reject**.
4. Request `https://tcgreplay.app/board` and find the trace in Tempo. The
   playmat's `GET /board` span should sit in the same trace as Cloudflare's
   spans. If it does not, App Platform's own edge may be dropping the
   `traceparent` header: record that under **Status** as the reason. Do not
   add header rewriting or routing changes to work around it.

From 2026-12-01 Cloudflare's Free plan allows 0.5 GB of trace ingestion a
day. At the cap, ingestion stops until 00:00 UTC.

**Status:** not yet turned on (2026-11-23). Record here, with the date,
whether it is on or why the edge part was dropped.

## Export status

The start-up line includes two parts:

- **Export**: `telemetry export: on (Grafana Cloud <host>)`,
  `off (no TCGREPLAY_GRAFANA_* settings)` or
  `off (incomplete: <which setting is missing or wrong>)`. An incomplete
  setting also writes its own line,
  `warning: telemetry export: off (incomplete: …)`. The playmat starts and
  serves either way.
- **Usage history**: `usage history: on (<host>)` or `off (<why>)`. It is
  off without the two metrics settings, or while export is off. A wrong
  metrics setting writes a `warning: usage history: off (…)` line.

When sending has failed for more than five minutes without a break, the
live log view shows:

```text
warning: telemetry export: sending to Grafana Cloud has failed for 7m (logs, traces: unreachable)
```

The line names:

- **the signals**: `logs`, `traces`, `metrics` or `history`;
- **the kind of failure**: `unreachable`, `timeout`, `rejected (HTTP 401)`
  (a wrong token or instance ID) or `failed`.

It repeats at most every five minutes and stops after a send succeeds. It
never shows the token or the server's reply. Players are not affected: data
that cannot be sent is dropped from bounded queues, and no request waits on
Grafana.

## Graphs

App Platform charts these without any work in the playmat. Use the last
24 hours and the last 7 days.

- **The app's Insights tab** (App Platform > tcgreplay > Insights): CPU,
  memory, restart count, request rate, P95 response time, and CDN throughput
  and latency by status code.
- **Control Panel > Insights**: the same App Platform metrics, in
  dashboards.

For each request's own timing, use the traces in Grafana (see
[Traces](#traces)).

## Alerts

`.do/app.yaml` declares three alert rules on the playmat service:

| rule | fires when | window |
|---|---|---|
| `RESTART_COUNT` | the playmat restarts at all (above 0) | 5 minutes |
| `CPU_UTILIZATION` | CPU stays above 80% | 10 minutes |
| `MEM_UTILIZATION` | memory stays above 80% | 10 minutes |

- **Thresholds**: these are starting values. Change them in `.do/app.yaml`
  and make the same change in the console (the app's Settings > App Spec >
  Edit, Save). The deploy workflow does not apply edits to the app
  definition.
- **Recipients**: the app definition cannot name who an alert goes to. Set
  the maintainer's email once in the control panel, either in the app's
  **Settings > Alerts** or in an Insights email notification channel.
- **Testing an alert**: lower a CPU or memory threshold below current use
  for a few minutes, or restart the app. The alert should arrive by email.

## Capacity

**Pending the maintainer's measurement.** The launch limits stay as they
are until it is done:

- `TCGREPLAY_MAX_USERS` is `10`;
- `instance_size_slug` is `apps-s-1vcpu-0.5gb` (512 MiB);
- both are set in `.do/app.yaml`.

**The rule.** Choose the player limit so that, with every place filled,
memory stays at or below 70% of the instance's memory. This leaves at least
30% free and sits under the 80% memory alert:

```text
limit = floor((0.7 × instance memory − baseline) / memory per game)
```

Here *baseline* is the playmat's memory with no games open, and *memory per
game* is one full imported game with branches. If the rule needs a size
above `apps-s-1vcpu-1gb` for at least 10 players, decide that before
changing the size, because it changes the hosting cost.

**Measuring:**

1. **In a checkout**, run the measurement:

   ```sh
   TCGREPLAY_MEASURE_MEMORY=1 go test -run TestMemoryPerGame -v ./internal/game/
   ```

   It holds 10 games open (`TCGREPLAY_MEASURE_GAMES` changes the number).
   Each is a full imported battle log with an opponent list and two
   branches. It reports the heap before and after, the average per game,
   and how many games fit in 512 MiB and 1 GiB at 70%. It counts the Go
   heap only. A development run on 2026-11-12 gave about 0.31 MiB per game
   on a 27.5 MiB baseline. That run is not a launch figure.
2. **On the deployed playmat**, note the memory % that Insights (or the
   app's Insights tab) shows with no games open. Then open several full
   imported games with branches from test accounts, note the % again, and
   note how many games were open. The difference divided by the number of
   games is the memory per game as deployed. It includes what the heap
   figure leaves out.
3. **Record the results here**: both per-game figures, the calculation, and
   the chosen `TCGREPLAY_MAX_USERS` and `instance_size_slug`, with the date.
   Then set the same values in `.do/app.yaml` and in the console (Settings >
   App Spec > Edit, Save). The website shows the player limit, read from
   `.do/app.yaml` when it is built, so the two must match.

## Retention

The privacy page quotes these figures, and the site's browser suite checks
that each figure it shows appears in this section. Keep the wording exact
("14 days", "six weeks", "7 days", "six months").

| kind | where it is kept | retention |
|---|---|---|
| Records: usage records (`sign_in`, `player_day`, `tool_call`, `import`, `new_game` and the other non-error events), error records (`error`) and the plain log lines | Grafana Cloud Loki | **14 days**: the Free tier's log retention |
| Request traces | Grafana Cloud Tempo | **14 days**: the Free tier's trace retention |
| Raw usage metrics (`tcgreplay_*_total`) | Grafana Cloud Mimir | **14 days**: the Free tier's metric retention |
| Daily usage totals (`tcgreplay_usage_daily`, no player ID) | Grafana Cloud Mimir, carried by the playmat | **six weeks**: the playmat keeps re-sending the last 42 days |
| Platform metrics (request rate, response time, status codes, CPU, memory, restarts) | App Platform metrics and Insights | **7 days**, provisional: the longest range the dashboards are relied on for. To be confirmed by the maintainer. |
| Alert history | DigitalOcean Insights | 30 days, as documented by DigitalOcean (Insights limits page) |
| Cloudflare's edge traces (only if turned on and persisted) | Cloudflare | 7 days on the Free plan; the playmat's setup turns persistence off |
| Visitor counts (Cloudflare Web Analytics, no player ID, when counting is on) | Cloudflare | **six months**: the full detail for 7 days, then a sample of about a tenth |

- **Sources, checked on 2026-11-23**:
  - `grafana.com/pricing` and `grafana.com/products/cloud/free-tier`
    (Free: "14 days retention" for metrics, logs and traces; 50 GB logs,
    50 GB traces and 10k active metric series a month);
  - `developers.cloudflare.com/observability/pricing/` (Free: 7 days
    retention, 0.5 GB a day from 2026-12-01);
  - `developers.cloudflare.com/web-analytics/faq/` (visitor counts:
    unsampled for 7 days, then about a tenth, available for six months;
    checked on 2026-11-13).
- **Platform metrics**, checked on 2026-11-13: DigitalOcean's documentation
  gives no metric retention period while Insights is in public preview
  (`docs.digitalocean.com/products/insights/details/limits/`,
  `.../insights/details/features/`,
  `.../app-platform/how-to/view-insights/`,
  `.../app-platform/details/limits/`). When the real period is confirmed,
  replace the provisional figure with it, its date and its source, and
  update the privacy page at the same time.
- **App Platform's own Runtime Logs tab** keeps no history, so it holds no
  records.
- **Nothing is stored by the playmat itself.** It keeps today's set of seen
  player IDs and the daily table in memory only. The table is read back
  from Grafana after a restart.
