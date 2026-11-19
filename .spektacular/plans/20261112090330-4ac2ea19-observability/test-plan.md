---
created_date: "2026-11-13"
document_status: final
closed_date: "2026-11-13"
---

# Test plan: 20261112090330-4ac2ea19-observability

These are the manual checks for the observability work: the spec's three
success metrics, which need the live service, and the manual reviews listed
in the plan's Testing Approach. Everything else is covered by Go tests:

- `internal/telemetry`;
- `internal/hostauth`, `internal/usergames`, `internal/cacheadmin` and
  `internal/webui` (their `events_test.go`, `telemetry_test.go` and
  `onerror_test.go`);
- `cmd/tcgreplay`: `hosted_events_test.go`, `hosted_usage_test.go`,
  `hosted_admin_test.go` and `noemail_test.go`.

## Before any check: the deferred setup

The user deferred the plan's two person tasks to the maintainer: "Set up the
secret and alert recipients, deploy and measure" and "Apply the launch
limits". Every check below needs that setup done first:

1. Generate a key with `openssl rand -base64 48`. Set it as
   `TCGREPLAY_PLAYER_ID_KEY` on the playmat component (App Platform >
   tcgreplay > Settings > playmat > Environment Variables, encrypted).
   **Do this before the branch reaches main.** Hosted mode refuses to start
   without the key, and every push to main deploys.
2. After the merge, apply the app definition with
   `doctl apps update <app-id> --spec .do/app.yaml`. This adds the three alert
   rules and declares the secret. Then deploy.
3. Set the alert recipients to the maintainer's email: the app's
   Settings > Alerts, or an Insights email notification channel.
4. **Pass**: the start-up line in Insights > Logs (LON1) ends with
   `player IDs on`. The app's Settings page lists the alerts `RESTART_COUNT`,
   `CPU_UTILIZATION` and `MEM_UTILIZATION`, each sending to the maintainer.

The test accounts below are two Google accounts that are not admins (A and
B) and one admin account (in `TCGREPLAY_ADMINS`). Use one TCG Live battle log
and decklist, for example `testdata/logs/2026-10-11_ledjo23_alakazam-donk.txt`
and `testdata/decks/mega-lucario-ex-v12.txt`.

## Success metrics

### SM1: distinct players and assistants each day, from DigitalOcean alone

- **What**: a week after launch, the number of distinct players each day and
  which assistants they used can be read for each of the last 7 days.
- **How**: follow the "Reading daily usage in Insights" section of
  `docs/observability.md`. Go to Control Panel > Insights > Logs, region LON1,
  and set an absolute UTC day as the time range. For each of the last 7 days,
  text-search:
  - `"event":"player_day"` and note the match count;
  - `"event":"tool_call"` with `"assistant":"Claude"`, then with each of
    `ChatGPT`, `Codex` and `Gemini`.
- **Expected**: each of the 7 days returns counts. Every day with activity
  has `player_day` ≥ 1, and the by-assistant counts add up to at most the
  day's `"event":"tool_call"` count. No other service is used.
- **If records older than a few days are missing**, or vanish after a
  redeploy, the provisional 7-day retention is wrong. Record the real period
  (see M9) and raise it before launch.
- **Who / when**: the maintainer, 7 days after launch.

### SM2: from a player's email to their error records in under five minutes

- **What**: a timed walk-through, from a reported email address to the
  matching `error` records, takes under 5 minutes.
- **How**:
  1. As account A, import a malformed battle log through an assistant: put
     the line `Hello` before `Setup`.
  2. Start a timer.
  3. As the admin, open `https://tcgreplay.app/admin`. Under "Find a player's
     ID", enter A's email and copy the ID shown in `<code>`.
  4. In Insights > Logs (LON1), with the last hour as the range, search
     `"player":"<id>"` and `"event":"error"`.
- **Expected**: in under 5:00, one record with `"tool":"import_log"`, A's
  ID, and a message such as `line 1: "…" before the Setup section`.
- **Who / when**: the maintainer, once after deploy and before launch.

### SM3: no crash or resource exhaustion in the first month goes unnoticed

- **What**: every restart, and every sustained CPU or memory spike, reaches
  the maintainer's email.
- **How**: run M7 (alert delivery) once before launch. During the first
  month, compare the alert emails with the app's Insights tab: its restart
  count, CPU and memory over the last 30 days. Alert history is kept for
  30 days.
- **Expected**: every restart, and every CPU or memory period above 80% for
  10 minutes or more, has a matching alert email. No player report names a
  crash the maintainer had not already been alerted to.
- **Who / when**: the maintainer, at the end of the first month after launch.

## Manual reviews and checks

### M1: records appear in Insights and can be filtered

- **Where**: Insights > Logs (LON1), on the deployed playmat.
- **Steps**: as account A, sign in to the board, connect an assistant, call
  `import_log` with the log and deck, then call `new_game`.
- **Pass**: within a few minutes Insights shows `sign_in`, `player_day`,
  `tool_call` (with `"tool":"import_log"` and `"tool":"new_game"`, and an
  `assistant` field), `import` and `new_game` lines, all with the same
  `player` ID. The text search `"event":"tool_call"` filters to just the
  tool calls. Note whether Insights offers "+ Add to filters" on the JSON
  fields, and say so in the runbook (`docs/observability.md`) if it does.

### M2: daily usage read in Insights

- **Steps**: on one UTC day, account A uses the playmat twice (two sessions,
  some hours apart) and account B uses it once, and there is no redeploy in
  between. Read that day in Insights as in SM1.
- **Pass**: `"event":"player_day"` shows **2** for the day. The tool calls by
  assistant, imports and new games match what the accounts did.

### M3: no identifying data in Insights

- **Steps**: after M1 and M2, search the day's records in Insights for each
  of:
  - the test accounts' email addresses, and their local parts;
  - the accounts' Google display names;
  - a Google token value (copy it from the browser's network tab during
    sign-in);
  - a whole line of the imported battle log, e.g. `LeDjo23 played Abra to
    the Active Spot.`;
  - the TCG Live player names in the log, `NicJackson` and `LeDjo23`;
  - card names from the deck, e.g. `Mega Lucario ex` and `Lillie's
    Determination`.
- **Pass**: every search finds nothing.

### M4: a player's ID stays the same across a real redeploy

- **Steps**: note account A's `player` ID from a `sign_in` record. Redeploy:
  push a docs change to main, or start the deploy workflow. Sign in again as
  A, from a different assistant if you have one.
- **Pass**: the new `sign_in` record carries the same ID. The admin lookup
  for A's email shows that ID too.

### M5: the maintainer-only lookup on the deployed admin page

- **Steps**:
  - As the admin, open `/admin` and look up B's email.
  - As account A (not an admin), open `/admin`.
  - Signed out, in a private window, open `/admin`.
- **Pass**: the admin sees B's ID, the same one B's records carry. A gets
  the "Not an admin" page (403). The signed-out browser is sent to sign in.
  Neither A nor the signed-out browser sees any ID. Searching Insights for
  B's email finds nothing, and there is an `admin_action` record with
  `"action":"lookup"` for the admin's ID.

### M6: graphs available

- **Where**: the app's Insights tab (App Platform > tcgreplay > Insights) and
  Control Panel > Insights.
- **Pass**: request rate, P95 response time, responses by status code, CPU,
  memory and restart count are all charted for the playmat, over the last
  24 hours and the last 7 days.

### M7: alerts reach the maintainer

- **Steps**:
  1. Restart the playmat (App Platform > Actions > Restart, or a redeploy).
  2. In `.do/app.yaml`, set the `CPU_UTILIZATION` value and then the
     `MEM_UTILIZATION` value below the current use, for example 1. Apply each
     with `doctl apps update <app-id> --spec .do/app.yaml` and wait out the
     10-minute window.
  3. Restore both to 80 and apply again.
- **Pass**: one restart email, one CPU email and one memory email arrive at
  the maintainer's address.

### M8: capacity measured and set

- **Steps**: follow `docs/observability.md` > Capacity:
  1. Run `TCGREPLAY_MEASURE_MEMORY=1 go test -run TestMemoryPerGame -v
     ./internal/game/`.
  2. On the deployed instance, note the memory % with no games open. Open
     several full imported games with branches, from different test
     accounts, and note it again.
  3. Work out `limit = floor((0.7 × instance memory − baseline) / memory per
     game)`. Record the figures, the chosen `TCGREPLAY_MAX_USERS` and the
     chosen `instance_size_slug` in the runbook, and set them in
     `.do/app.yaml`.
  4. Apply with `doctl apps update` (the deferred "Apply the launch limits"
     task).
- **Pass**: with the chosen limit's worth of full games open on the deployed
  instance, Insights shows memory at or below 70% of the instance's memory,
  under the 80% alert. The app's Settings page shows the chosen size, and
  the start-up line shows the chosen user count. If 10 players would need a
  size above `apps-s-1vcpu-1gb`, decide that before changing the size.

### M9: retention written down

- **Steps**: confirm with DigitalOcean (the Insights console, the docs or
  support) how long Insights keeps logs and how long it keeps metrics during
  the preview, and whether that changes at general availability.
- **Pass**: `docs/observability.md` > Retention replaces each provisional
  "7 days, to be confirmed by the maintainer" with the confirmed period, the
  date and the source, for usage records, error records and platform
  metrics. The privacy page's retention statements are updated to match. If
  logs are kept for less than 7 days or are dropped on redeploy, raise it:
  SM1 cannot be met then.

### M10: runbook review

- **Where**: `docs/observability.md`, against `internal/telemetry/recorder.go`
  and the live Insights.
- **Pass**:
  - The event table lists exactly the 14 event constants in
    `recorder.go`, with the fields each actually carries.
  - Every Insights search in "Reading daily usage" and "Finding a player's
    errors" returns what the runbook says on the deployed app. M1, M2 and SM2
    exercise these searches.
  - The alert table matches `.do/app.yaml`.
