---
created_date: "2026-11-23"
document_status: final
closed_date: "2026-11-23"
---

# Test plan: Grafana Cloud observability

Manual procedures for the success metrics and reviews that automated tests cannot cover, because they need the live Grafana Cloud stack, the App Platform console or the Cloudflare dashboard. Everything else is covered by the Go suite (`go test -tags nodynamic ./...`) and the site browser suite (`cd site && npm run test:e2e`).

All steps run against production (`https://tcgreplay.app`) after this branch is merged and deployed. Run them in the order below: steps 1 and 2 are the two plan tasks skipped in the implement run, and every later step depends on them.

**Who**: the maintainer, unless a step says otherwise.

**Before you start**: you need a test Google account that is not your own. Its player ID comes from `/admin` > Find a player's ID. You also need an assistant (Claude) connected to the playmat with that account.

## Setup (the two skipped human tasks)

### 1. Turn on sending to the Grafana stack (plan task fcc30e32, skipped)

- **When**: once the privacy page naming Grafana Cloud is live on `https://tcgreplay.app/privacy`.
- **How**: follow `docs/observability.md` > "Setting up Grafana Cloud", steps 1 to 6:
  1. check the region;
  2. create the token, with scopes `metrics:read metrics:write logs:write traces:write`;
  3. copy the OTLP and Prometheus addresses;
  4. add the five `TCGREPLAY_GRAFANA_*` app-level settings in the App Platform console, with the token encrypted, and never paste `.do/app.yaml`;
  5. save;
  6. import `docs/grafana/tcgreplay-usage.json`.
- **Expected**:
  - the stack's region is `prod-eu-west-2` or `prod-eu-west-4` (Germany);
  - the new deployment's start-up line in Runtime Logs contains `telemetry export: on (Grafana Cloud otlp-gateway-` and `usage history: on (prometheus-`;
  - no `warning:` line about telemetry appears;
  - within 5 minutes, Explore shows `{service_name="tcgreplay"}` lines in Loki, traces in Tempo, and `tcgreplay_` metrics in the Prometheus data source;
  - the dashboard shows the last-14-days panels with today's raw counts.
- **Pass**: every expected item holds. If queries from the usage history are refused while export works (a `history: rejected (HTTP 401)` warning after five minutes), stop and raise it before adding a second token setting (plan Open Question).

### 2. Check the metric names Grafana stored

- **When**: right after step 1, once at least one sign-in and one tool call have happened.
- **How**: in Explore with the Prometheus data source, open the metric browser and search `tcgreplay`.
- **Expected**: exactly these names:
  - `tcgreplay_player_days_total`
  - `tcgreplay_sign_ins_total`
  - `tcgreplay_tool_calls_total`
  - `tcgreplay_imports_total`
  - `tcgreplay_new_games_total`
  - `tcgreplay_errors_total`
  - `tcgreplay_limited_total`
  - `tcgreplay_usage_daily`, which appears after the first hourly refresh that finds a finished day.

  The labels are `provider`, `assistant`, `tool`, `kind` and `reason` on the counters, and `day`, `count`, `provider` and `assistant` on the gauge.
- **Pass**: the names match. If Grafana stored different names (an added unit suffix such as `_ratio`, for example), correct them in three places to the stored names:
  - the dashboard JSON;
  - the runbook's query tables;
  - `historyQueries` / `historyReadBack` / `historyPresent` in `internal/observe/history.go`, and the hand list in `internal/observe/dashboard_test.go`.

  If a name collides with another metric, stop and raise it.

### 3. Turn on Cloudflare edge traces (plan task 30bb8630, skipped)

- **When**: after step 1.
- **How**: follow `docs/observability.md` > "Turning on edge traces", steps 1 to 4:
  - Free plan check: no upgrade;
  - add a Traces destination at `<OTLP URL>/v1/traces` with the Basic header;
  - turn persistence off;
  - turn Forward to origin on;
  - leave incoming context at Reject;
  - change no routing, proxy or DNS setting.
- **Expected**: either a request to `https://tcgreplay.app/board` gives one Tempo trace holding Cloudflare's spans and the playmat's `GET /board` span, or the runbook's Status line records why not (OTLP export not on the Free plan, or `traceparent` not reaching the playmat through App Platform's edge).
- **Pass**: the Status line is updated with the outcome and the date, and the zone is still on the Free plan.

## Success metrics

### 4. A player's errors and the failing trace, from their email, in under five minutes

- **What**: the time from "a player reports a problem" to "looking at the error record and its trace", without opening the App Platform console.
- **How**: with the test account, make an assistant import a malformed battle log (put a stray line before `Setup`). Start a timer. Then:
  1. open `/admin` and look up the test account's email under Find a player's ID;
  2. in Grafana Explore (Loki), run `{service_name="tcgreplay"} | player="p_…" | event="error"`;
  3. open the line's `trace_id` in Tempo.
- **Expected**: the `tools/call import_log` span is marked failed with the same redacted message as the record.
- **Pass**: under 5 minutes, with no hosting console used.

### 5. No deploy loses diagnostic history (first two weeks after launch)

- **What**: every day has usage and error records searchable in Grafana.
- **How**: every few days for the first two weeks, run `sum by (event) (count_over_time({service_name="tcgreplay", log_source="record"} [1d]))` in Loki over the last 14 days. In Loki the attribute may show as `log_source` or as structured metadata `log.source`; filter `| log_source="record"` if the label is not indexed. Also check that each deploy's start-up line is present.
- **Expected**: records on every day with play, including days with deploys.
- **Pass**: no day with known play shows zero records.

### 6. Telemetry adds no measurable latency

- **What**: median tool-call duration with telemetry on stays within 5% of the duration with it off.
- **How**:
  1. With sending on, make 50 `ping` tool calls through an assistant, or a scripted MCP client.
  2. In Tempo, read the median duration of `tools/call ping` spans: `{ name = "tools/call ping" }`, then the duration column, or the TraceQL metrics `quantile_over_time(duration, .5)`.
  3. Repeat the 50 calls with sending off: remove `TCGREPLAY_GRAFANA_TOKEN` and save. With no spans, measure client-side from the scripted client's timings, or from App Platform's P95 response time for `/mcp`.
  4. Compare the medians from the same client-side timing method.
- **Expected**: median(on) ≤ 1.05 × median(off).
- **Pass**: within 5%. Restore the token afterwards.

### 7. The sign-in page's release can be judged from daily Google sign-ins

- **What**: daily Google sign-in counts for the two weeks before and after the sign-in page ships, read from Grafana.
- **How**: on the dashboard, read the "Sign-ins per day by provider" panel, or run `max by (day, provider) (last_over_time(tcgreplay_usage_daily{count="sign_ins"}[42d]))` over 28 days around the release.
- **Expected**: one row per day with `provider="google"`, and a separate `password` row once the sign-in page ships.
- **Pass**: 28 consecutive days are readable at day 28 after the release window opens.

### 8. Monthly telemetry volume stays within the Grafana Cloud free tier

- **What**: 50 GB of logs a month, 50 GB of traces a month and 10k active metric series.
- **How**: after the first two weeks, and then monthly, open the stack's Usage / Billing page (Administration > Cost management, or the stack's usage dashboard).
- **Expected**: logs and traces on course to stay under 50 GB a month each, and active series under 10k. Expected series: about 800 daily-gauge series plus a few hundred counter series.
- **Pass**: all three are under the limits. If traces approach the limit, plan sampling (out of scope for this change).

## Manual reviews

### 9. Logs survive a redeploy

- **How**:
  1. With the test account, sign in to the board and through an assistant, then import a game.
  2. Trigger a redeploy (push or "Deploy" in the console).
  3. In Loki over the last hour, search `{service_name="tcgreplay"} | player="p_…"` and `{service_name="tcgreplay"} |= "hosted: listening on"`.
- **Pass**: you find the `sign_in` (with `provider="google"`), `tool_call` and `import` records, and both start-up lines, from before and after the redeploy.

### 10. The live log view is unchanged

- **How**: watch App Platform > Runtime Logs during a deploy.
- **Pass**: the start-up line and the JSON records appear live as before. The start-up line now ends with the export and usage history status, and no extra lines appear.

### 11. Traces in Tempo

- **How**: in Tempo, find:
  - a trace for an `import_log` tool call (`{ name = "tools/call import_log" }`);
  - a board page (`{ name = "GET /board" }`);
  - the live connection (`{ name = "GET /ws" }`);
  - a full sign-in (`{ name = "GET /auth/google/callback" }`);
  - a card request that copied an image (`{ name = "GET /api/" }` on a card not yet copied).
- **Pass**:
  - the sign-in trace shows `POST oauth2.googleapis.com` as its own timed part;
  - the card trace shows a `PUT <bucket>.ams3.digitaloceanspaces.com` part;
  - the tool call sits inside its `POST /mcp` trace.

### 12. From an error record to its trace and back

- **How**: from the error line in Loki (step 4), open its trace. From the failing span in Tempo, use Logs for this span. If either link is missing, set it up as `docs/observability.md` > "Finding a player's errors" steps 4 and 5 describe.
- **Pass**: both directions work.

### 13. The failure line with wrong credentials

- **How**:
  1. In the console, change `TCGREPLAY_GRAFANA_TOKEN` to a wrong value and save.
  2. Watch Runtime Logs for 12 minutes while using the playmat with the test account: sign in, import, play.
  3. Restore the token.
- **Pass**:
  - no failure line before 5 minutes;
  - `warning: telemetry export: sending to Grafana Cloud has failed for 5m (… rejected (HTTP 401))` (or similar) at about 5 minutes, and at most one more by 10 minutes;
  - no token or Basic value in any line;
  - every player action works normally.

### 14. Nothing identifying in the stack

- **How**: after a test session with the test account (sign-in, import of a log naming cards and players, board actions), search:
  - Loki for the test account's email, a deck card name and a line of the pasted log: `{service_name="tcgreplay"} |= "<email>"`;
  - Tempo for the same strings in span attributes (`{ span.url.full =~ ".*code=.*" }`, and searches for the email);
  - the metric browser for any label holding an email or a `p_` ID.
- **Pass**:
  - no email, Google token, pasted log text or card name is found anywhere;
  - the player appears only as `p_…`;
  - no `url.full` carries a query string.

### 15. Runbook walk-through

- **How**: following only `docs/observability.md`:
  1. read yesterday's distinct players and tool calls by assistant ("Reading daily usage in Grafana");
  2. find the test account's errors through the admin lookup ("Finding a player's errors");
  3. open one error's trace.
- **Pass**: each step works as written, with no step missing or wrong. Correct the runbook where it is not.

### 16. Privacy page on the live site

- **Who**: the maintainer, before step 1.
- **How**: open `https://tcgreplay.app/privacy` > Diagnostics and Outside services.
- **Pass**:
  - it names Grafana Cloud and Germany;
  - it gives 14 days for records and traces and six weeks for daily totals;
  - DigitalOcean is named for hosting and its own metrics;
  - there is no Insights claim;
  - the What's new page shows the 2026-11-23 "Privacy page: where diagnostics are kept" entry.

### 17. Daily totals still present after 28 days

- **When**: at day 28 or later after step 1.
- **How**: open the dashboard over the last 42 days, or run `count by (day) (last_over_time(tcgreplay_usage_daily{count="players"}[42d]))`.
- **Pass**: every finished day since step 1 is present, including days older than Grafana's 14-day metric retention.
