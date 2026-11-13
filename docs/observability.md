# Observability: usage, errors, graphs and alerts

The maintainer's runbook for watching the hosted playmat at
`https://tcgreplay.app`. It covers:

- what the playmat records and what it never records;
- how players are named in the records;
- how to read each day's usage in DigitalOcean Insights;
- how to find a player's errors from their email;
- the graphs to watch and the alerts.

The capacity and retention sections are filled from measurements.

Everything is read in DigitalOcean. No log forwarding service and no second
monitoring vendor is used.

- **Records**: the playmat writes one JSON object per line to its standard
  output. App Platform passes that output to DigitalOcean Insights, whose
  Logs tab keeps it across redeploys. App Platform's own Runtime Logs tab
  keeps no history, so always read records in Insights.
- **Graphs and alerts**: these come from App Platform's metrics and alert
  rules, not from the playmat.

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
| `message` | an error or refusal message, redacted (see below) |
| `reason` | why a request was refused or an account removed |
| `idle` | the idle time an expired game went past (`TCGREPLAY_IDLE_TIMEOUT`, in words, e.g. `60 minutes`) |

Some example lines:

```json
{"time":"2026-11-12T09:30:00.123Z","level":"info","event":"sign_in","player":"p_3f9a1c0b7d2e4a68"}
{"time":"2026-11-12T09:30:00.456Z","level":"info","event":"player_day","player":"p_3f9a1c0b7d2e4a68"}
{"time":"2026-11-12T09:31:12.003Z","level":"info","event":"tool_call","player":"p_3f9a1c0b7d2e4a68","tool":"import_log","assistant":"Claude"}
{"time":"2026-11-12T09:31:12.004Z","level":"info","event":"import","player":"p_3f9a1c0b7d2e4a68","assistant":"Claude"}
{"time":"2026-11-12T09:35:40.871Z","level":"error","event":"error","player":"p_3f9a1c0b7d2e4a68","tool":"import_log","message":"line 1: \"…\" before the Setup section"}
{"time":"2026-11-12T09:40:02.310Z","level":"info","event":"limited","player":"p_3f9a1c0b7d2e4a68","message":"too many requests: this account is limited to 300 requests a minute; try again in 12s","reason":"rate"}
```

The events form a closed list:

| event | fields beyond time, level, event and player | written when |
|---|---|---|
| `sign_in` | none | a Google sign-in completes, for the board or an assistant connection |
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
card sets, card images, refused tokens and client registrations. The start-up
line ends with `player IDs on`. The admin page's own text lines for card-set
checks and image clears name the admin by player ID.

## What is never recorded

No record and no log line contains any of these:

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

## Reading daily usage in Insights

Insights has no "count distinct" or group-by. Each daily number is the
number of matching lines for that UTC day.

1. Open the Control Panel and go to **Insights > Logs**. Choose the region
   **LON1** and the playmat app.
2. Set an absolute time range covering one UTC day: From `YYYY-MM-DD 00:00`
   To `YYYY-MM-DD 23:59:59`, in UTC. A single query covers at most 7 days.
3. Run each search below as a text search, and read the number of matches:

| number | search |
|---|---|
| Distinct players | `"event":"player_day"` |
| Tool calls by assistant | `"event":"tool_call"` together with `"assistant":"Claude"`, then again with `"assistant":"ChatGPT"`, `"assistant":"Codex"` and `"assistant":"Gemini"` |
| All tool calls | `"event":"tool_call"`; the total less the four named assistants is the calls from other clients, which are named by their own client name |
| Imports | `"event":"import"` |
| New games | `"event":"new_game"` |
| Sign-ins | `"event":"sign_in"` |
| Errors | `"event":"error"` |
| Refusals at the limits | `"event":"limited"`, split with `"reason":"rate"` or `"reason":"places"` |

The `player_day` count can be slightly high. The playmat remembers who it
has seen today only in memory, so after a restart or redeploy during the day,
a player who is active again is counted a second time.

If Insights has parsed the JSON fields into attributes (a field shows
**+ Add to filters**), filter on `event`, `assistant` and `player` in the
filters panel instead of text searches. The text searches work either way.

## Finding a player's errors

When a player reports a problem:

1. Sign in to `https://tcgreplay.app/admin` as an admin (an account in
   `TCGREPLAY_ADMINS`).
2. Under **Find a player's ID**, enter the email address the player signed
   in with. The page shows their ID, for example `p_3f9a1c0b7d2e4a68`. The
   address is posted, so it never appears in a URL, and it is not logged.
3. In **Insights > Logs** (LON1), set the time range around the report and
   search `"player":"p_3f9a1c0b7d2e4a68"` together with `"event":"error"`.
   Leave out the event to see everything the player did:
   - their tool calls, with the tool and the assistant;
   - their imports and new games;
   - any limits they hit, and their sign-ins.

## Graphs

App Platform charts these without any work in the playmat. Use the last
24 hours and the last 7 days.

- **The app's Insights tab** (App Platform > tcgreplay > Insights): CPU,
  memory, restart count, request rate, P95 response time, and CDN throughput
  and latency by status code.
- **Control Panel > Insights**: the same App Platform metrics, beside the
  logs, in dashboards.

## Alerts

`.do/app.yaml` declares three alert rules on the playmat service:

| rule | fires when | window |
|---|---|---|
| `RESTART_COUNT` | the playmat restarts at all (above 0) | 5 minutes |
| `CPU_UTILIZATION` | CPU stays above 80% | 10 minutes |
| `MEM_UTILIZATION` | memory stays above 80% | 10 minutes |

- **Thresholds**: these are starting values. Change them in `.do/app.yaml`,
  then apply the file with `doctl apps update <app-id> --spec .do/app.yaml`.
  The deploy workflow does not apply edits to the app definition.
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
   Then set the same values in `.do/app.yaml` and apply them with
   `doctl apps update <app-id> --spec .do/app.yaml`. The website shows the
   player limit, and the next deploy picks it up from `.do/app.yaml`.

## Retention

**Provisional: to be confirmed by the maintainer.** DigitalOcean's
documentation does not state how long Insights keeps logs or metrics while
Insights is in public preview. It documents only alert history, which is
kept for up to 30 days. The figures below are the conservative working
values until the maintainer confirms the real periods with DigitalOcean (in
the Insights console, the docs or support):

| kind | where it is kept | retention |
|---|---|---|
| Usage records (`sign_in`, `player_day`, `tool_call`, `import`, `new_game` and the other non-error events) | Insights logs | **7 days**, provisional: the longest range the Insights log search offers ("Last 7 days"). To be confirmed by the maintainer. |
| Error records (`error`) | Insights logs | **7 days**, provisional, as above. To be confirmed by the maintainer. |
| Platform metrics (request rate, response time, status codes, CPU, memory, restarts) | Insights and App Platform metrics | **7 days**, provisional: the longest range the dashboards are relied on for. To be confirmed by the maintainer. |
| Alert history | Insights | 30 days, as documented by DigitalOcean (Insights limits page) |

- **Sources checked on 2026-11-13**: these DigitalOcean docs pages, none of
  which gives a log or metric retention period:
  - `docs.digitalocean.com/products/insights/details/limits/`;
  - `.../insights/details/features/`;
  - `.../insights/how-to/view-logs/`;
  - `.../app-platform/how-to/view-insights/`;
  - `.../app-platform/details/limits/`.
- **App Platform's own Runtime Logs tab** keeps no history, so it holds no
  records.
- **Nothing is stored by the playmat itself.** It keeps today's set of seen
  player IDs in memory only, and that set is lost on restart.
- **When confirmed**, replace each provisional figure above with the
  confirmed period, the date it was confirmed and its source. The privacy
  page states these periods, so update it at the same time.
- **If Insights keeps logs for less than 7 days, or drops them on
  redeploy**, a week of daily usage cannot be read back. Raise that before
  launch.
