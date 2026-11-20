---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Plan: 20261112090330-4ac2ea19-observability

<!-- Metadata -->
<!-- Created: 2026-11-12T09:41:40Z -->
<!-- Commit: 6ecc1aa -->
<!-- Branch: main -->
<!-- Repository: tcgreplay (/home/nicj/code/github.com/nicholasjackson/tcgreplay) -->

## Overview

Once TCG Replay is public, the maintainer needs to know how much it is used and to catch and diagnose problems before players report them, without any record identifying a player. This plan makes the hosted playmat write structured usage and error records, keyed by a stable, keyed one-way pseudonymous player ID in place of the email address. It gives admins a way to find a player's ID from their email. The maintainer reads the records, graphs and alerts in DigitalOcean Insights. The plan also sets the launch player limit and instance size from a memory measurement, and writes down how long each kind of record is kept for the privacy page.

## Conventions

- **Never write "Claude" or "Nic " in an `internal/guides` guide; never exclude `internal/guides` from a build context** — this work changes neither the guides nor the Dockerfile or `.dockerignore`. It is kept on the list only because the capacity task may change `.do/app.yaml` and the build. No change here may exclude `internal/guides` from the image.

## Architecture & Design Decisions

All of the work lands in the one registered repo, `tcgreplay`, and only in hosted mode: the desktop, daemon and dev paths build no recorder and behave exactly as today. The playmat gains a small new package, `internal/telemetry`, with two parts.

- **Pseudonymous IDs.** `IDs` turns an email address into a player ID. The ID is a keyed HMAC-SHA256 of the lower-cased, trimmed address, domain-separated with the prefix `"tcgreplay player\x00"`, truncated to 16 hex characters and written `p_<hex>`.
  - The key comes only from a new secret setting, `TCGREPLAY_PLAYER_ID_KEY`: at least 32 characters, never a flag, declared `type: SECRET` in `.do/app.yaml`. Hosted mode refuses to start without it, as it already does without the Google client.
  - So the same account keeps the same ID across sign-ins, assistants, restarts and redeploys. Hashing a list of known emails without the secret matches nothing, and changing the secret changes every ID.
- **The event recorder.** `Recorder` writes **one JSON object per line to stdout**, with a fixed field order: `time` (UTC, RFC 3339), `level` (`info` or `error`), `event`, `player`, then the event's own fields (`tool`, `assistant`, `action`, `message`, `reason`, `idle`).
  - App Platform passes stdout to DigitalOcean Insights, whose Logs tab keeps the playmat's runtime logs across redeploys (confirmed by the maintainer in the console; see `gotchas/app-platform-logs-only-in-insights.md`). There the maintainer filters by event and reads the match count.
  - Callers hand the recorder an email address, never an ID. The recorder hashes the address and never writes it out: the event struct carries the email in a field tagged `json:"-"`.
  - A nil `*Recorder` is a no-op, so packages and tests that build none keep working.

Usage is recorded at the points that already define it, not inside each of the 40-odd tools.

- **Tool calls, imports and new games.** A second MCP receiving middleware, built by the per-account registry (`internal/usergames`), wraps `tools/call`. It is added after `hostauth.MCPMiddleware` in `cmd/tcgreplay/hosted.go`, so requests the rate limit refuses are recorded once, as `limited`, and never as tool calls.
  - It writes a `tool_call` record for every call. The tool comes from the request; the assistant comes from the session's `ClientInfo.Name` through the existing `mcpserver.AssistantName`; the player comes from the token through `hostauth.AccountFromToken`.
  - It adds `import` or `new_game` when `import_log` or `new_game` succeeds.
  - It writes `error` when the result is an error. go-sdk v1.6.0 turns every handler error into an `IsError` result, so this one place sees every tool failure.
- **Distinct players a day.** The registry's activity clock (`Registry.Use`: sign-in, tool call, board page load or board action) writes a `player_day` record the first time each player is active in a UTC day. Insights has no distinct count, so the day's `player_day` count is the number of distinct players. The set of players seen is held in memory and cleared at midnight UTC; after a restart a player active again that day is counted again, a small over-count the runbook states.
- **Sign-ins and the existing log lines.** The sign-in service gets the recorder through `hostauth.Config`. Every log line that carries an email today becomes a structured record keyed by player ID: `sign_in`, `sign_in_refused`, `sign_out`, `limited` (both the rate limit and the user cap), `game_expired`, `account_removed`, `board_refused`, `admin_refused`. The cache admin's log lines name the admin by ID too. After this work no log line anywhere carries an email address.
- **Board actions.** A board action that fails is recorded as `error` with the action's name, through a new `OnError` hook on `webui.Server` that hosted mode wires to the recorder.

Error messages are the one place user text can reach a record. The playmat's errors quote battle-log lines (`internal/importer/parse.go:137`), name cards (`internal/importer/replay.go:207-219`) and name TCG Live players (`internal/importer/replay.go:315`). So every error message goes through `telemetry.Redact` before it is written.

- `Redact` replaces every double-quoted segment with `"…"`.
- It also replaces every maximal word run found, word-bounded, in the call's *sensitive text* with `…`, unless the run is made only of words from a short built-in list of the playmat's own common words (`the`, `no`, `in`, `hand`, `deck`, `bench`, `turn`, `line`, …).
- The sensitive text is every string value in the tool call's arguments, plus the player names and card names of the caller's current game.
- Over-redaction is preferred to a leak. The message keeps its shape (which check failed and where) without the player's data, which satisfies both "the error message" and "no battle log, decklist or card data".

The maintainer-only lookup is a form on the existing admin page (`/admin`, behind `hostauth.RequireAdmin`). It POSTs an email to `/admin/player` and shows that address's player ID. Using POST keeps the address out of URLs, and `RequireAdmin` already refuses non-admins (403), signed-out callers (401 or a redirect to sign-in) and cross-site posts.

Traffic, resources and alerts come from the platform, not the playmat.

- Insights and the app's own Insights tab already chart request rate, P95 response time, requests by status code, CPU, memory and restarts.
- Component alert rules for `RESTART_COUNT`, `CPU_UTILIZATION` and `MEM_UTILIZATION` are declared under `alerts:` in `.do/app.yaml`. The thresholds there are the maintainer's to edit. The app spec cannot name who an alert goes to, so pointing alerts at the maintainer's email is a person's task in the control panel.

Two more pieces of the work are partly people's tasks:

- **Capacity.** A Go test imports a full game with branches and reports how much memory it adds. Then the maintainer confirms the figure on the deployed instance, and sets `TCGREPLAY_MAX_USERS` and `instance_size_slug` from it with headroom.
- **Retention.** The maintainer confirms how long Insights keeps logs and metrics.

Both results, with the event catalogue and the exact Insights filters for each daily number, are written in a new runbook, `docs/observability.md`. The privacy spec reads that runbook.

This direction beats the alternatives because:
- one middleware sees every tool call and every tool failure without touching 40 handlers;
- the recorder adds no storage of its own;
- nothing it writes is read back, so a restart loses nothing that matters.

It was chosen over forwarding to DigitalOcean Managed OpenSearch, in-memory or admin-page counts, and counts persisted in Spaces. The evidence for each rejection is in `research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Telemetry (new).**
  - Owns the player ID (a keyed one-way hash of an email address), the event recorder, and the redaction of error messages.
  - The recorder writes one JSON record per line to the process's standard output, with a fixed field order and a closed set of event names. It hashes the email address it is handed and never writes the address itself.
  - It holds, in memory, the set of players already seen in the current UTC day, so it can write each player's `player_day` record once a day.
  - It depends on no other playmat package, so the sign-in service, the account registry and the admin page can all use it. A missing recorder is a no-op, so the desktop builds none.
- **Sign-in service (changed).**
  - Takes the recorder in its configuration.
  - Writes `sign_in`, `sign_in_refused`, `sign_out`, `limited` (rate limit) and `admin_refused` records keyed by player ID, in place of the log lines that carry an email address today.
  - Its other log lines (refused tokens, client registration, callback state) carry no personal data and stay as they are.
- **Per-account registry (changed).**
  - Takes the recorder in its configuration.
  - Writes `player_day` from its activity clock, and `limited` (user cap), `game_expired`, `account_removed` and `board_refused` records in place of its email-bearing log lines.
  - Builds the **tool-call middleware**, a new MCP receiving middleware placed after the rate limiter. For every tool call it writes `tool_call`, naming the tool and the assistant. It adds `import` or `new_game` when those tools succeed, and `error` with the redacted message when a call fails.
  - It is the one component that knows the token's account, the session's assistant and the caller's current game, which the redaction draws its sensitive text from.
- **Board server (changed).** Gains an error hook that is told of each board action that fails, with the action's name and the error. Hosted mode wires the hook to the recorder through the registry, which redacts the message the same way.
- **MCP tool server (unchanged in behaviour).** Its existing assistant-name mapping is reused by the tool-call middleware, so a tool's record names the assistant exactly as the playmat already does.
- **Admin page (changed).**
  - Gains a "Find a player's ID" form that takes an email address and shows the player ID, using the same hashing as the recorder. It sits behind the existing admin guard.
  - Its card-set and image log lines name the admin by player ID instead of by email.
- **Hosted wiring (changed).**
  - Reads the new player-ID secret and refuses to start without it.
  - Builds one recorder and hands it to the sign-in service, the registry, the board servers and the admin page. Adds the tool-call middleware.
  - Says in the start-up log that player IDs are on, never the key itself.
- **App definition (changed).** Declares the player-ID secret and the restart, CPU and memory alert rules. After the capacity measurement, it carries the launch player limit and instance size, with comments saying where they came from.
- **Capacity measurement (new test).** A Go test that imports a full game with branches and reports the memory one open game adds. The launch player limit and instance size are set from its figure, checked against the deployed instance.
- **Observability runbook (new document).**
  - Lists every event and its fields.
  - Gives the Insights filters for each daily number and for finding a player's error records.
  - Lists the alert rules and how to point them at the maintainer.
  - Records the measured memory per game with the chosen limit and size, and how long Insights keeps logs and metrics.
  - The privacy page work reads it.

## Data Structures & Interfaces

**The event record (the serialization boundary).** It is one JSON object per line on standard output; Insights shows it as a log line. Fields always appear in this order, and an empty one is left out:

```json
{"time":"2026-11-12T09:30:00.123Z","level":"info","event":"tool_call","player":"p_3f9a1c0b7d2e4a68","tool":"import_log","assistant":"Claude"}
{"time":"…","level":"error","event":"error","player":"p_…","tool":"import_log","message":"line 3: \"…\" before the Setup section"}
```

- `time` is the UTC time in RFC 3339.
- `level` is `info`, or `error` for the `error` event.
- `event` is one of a closed set, with its own fields:

| event | fields beyond time/level/event/player | written when |
|---|---|---|
| `sign_in` | — | a Google sign-in completes (either flow) |
| `sign_in_refused` | `reason` | a completed Google sign-in is refused (user cap) |
| `sign_out` | — | a board session signs out |
| `player_day` | — | a player's first activity in a UTC day |
| `tool_call` | `tool`, `assistant` | every MCP tool call that reaches the tools |
| `import` | `assistant` | `import_log` succeeds |
| `new_game` | `assistant` | `new_game` succeeds |
| `error` | `tool` or `action`, `message` (redacted) | a tool call or board action fails |
| `limited` | `reason` (`rate` or `places`), `message` | the rate limit or user cap stops a request |
| `game_expired` | `idle` | an idle game is dropped |
| `account_removed` | `reason` (`signed_out` or `idle`) | an account's entry is removed |
| `board_refused` | — | a player opens another account's board link |
| `admin_refused` | `reason` | a non-admin or cross-site request reaches /admin |
| `admin_action` | `action` | an admin checks card sets, clears images or looks up a player |

- `player` is always a player ID, never an email; it is left out when no player is known.
- No record ever carries an email address, a display name, a token, a battle log, a decklist or a card name.

**Player ID.** A string `p_` followed by 16 lower-case hex characters. It is a keyed HMAC-SHA256 of the trimmed, lower-cased email address, domain-separated. The empty email has no ID.

**Telemetry package interfaces** (pseudocode):

```go
type IDs struct{ /* key */ }
func NewIDs(secret string) (*IDs, error) // error when secret is shorter than 32 characters
func (*IDs) ID(email string) string      // "" for an empty email

type Event struct {
    Name                         string // one of the event constants
    Email                        string `json:"-"` // hashed into Player, never written
    Tool, Action, Assistant      string
    Message, Reason, Idle        string
}

type Recorder struct{ /* writer, IDs, clock, today's seen set */ }
func NewRecorder(w io.Writer, ids *IDs, now func() time.Time) *Recorder
func (*Recorder) Record(e Event)           // nil receiver: no-op
func (*Recorder) Active(email string)      // writes player_day once per player per UTC day
func (*Recorder) ID(email string) string   // for the admin lookup

func Redact(msg string, sensitive []string) string
```

**Changed configuration contracts:**
- `hostauth.Config` gains `Recorder *telemetry.Recorder`.
- `usergames.Config` gains `Recorder *telemetry.Recorder`.
- `usergames.Registry` gains `MCPMiddleware() mcp.Middleware`, the tool-call middleware, and `BoardError(ctx, action string, err error)` for the board hook.
- `webui.Server` gains `OnError func(ctx context.Context, action string, err error)`; nil means nothing is told.
- `cacheadmin.Service` gains `PlayerID func(email string) string`; nil hides the lookup form.

**New setting.** `TCGREPLAY_PLAYER_ID_KEY` is a secret of at least 32 characters, read only from the environment. Hosted mode will not start without it.

**App definition.** The `playmat` service gains an `alerts:` list of three rules, each with `operator`, `value` and `window`:
- `RESTART_COUNT` greater than 0 over five minutes;
- `CPU_UTILIZATION` above the maintainer's threshold;
- `MEM_UTILIZATION` above the maintainer's threshold.

No persistent data structure is added. The only state is the in-memory set of players seen today.

## Implementation Detail

**A new module boundary for what gets recorded.** Today the hosted packages call `log.Printf` with free text, much of it carrying an email address. After this work there are two kinds of output:
- **Operational lines** that name no player (start-up, card sets, images, refused tokens). These stay as `log.Printf` text.
- **Records**, which go through the telemetry recorder.

A developer adding an event adds a constant and, if it needs one, a field to the event type. They never format a line by hand, and the recorder is the only code that writes a player ID. The rule a reader can see in the code is that a `log.Printf` near an `Account` is a bug. A test sweeps the hosted packages so it cannot creep back in (see Testing Approach).

**Optional dependencies via nil-safe handles.** Following the existing pattern of optional hooks (the board's `Limit` and `Images`, the sign-in service's `Places`), the recorder, the board's error hook and the admin page's lookup function are all optional fields. A nil recorder silently records nothing. So the desktop, daemon and dev builds, and the many existing tests that build these packages directly, need no change. Hosted wiring is the one place that turns them on.

**Middleware composition on the MCP server.** The rate limiter is the only receiving middleware today. A second, the tool-call middleware, is added after it, so it runs inside it. The order is load-bearing: requests refused by the limit are recorded once, as `limited`, and never as tool calls, and the tool-call record is written after the handler returns, so it knows whether the call succeeded. The middleware follows the per-request lookup rule in `gotchas/go-sdk-sessions-and-auth.md`: it reads the account from each request's token and the assistant from the request's session, never from something cached at session creation.

**Redaction as a pure function.** `Redact` takes the message and a list of sensitive strings and knows nothing about games or tools, so it can be unit-tested in isolation. The middleware and the board hook gather the sensitive strings:
- the call's string arguments, walked out of the raw JSON;
- the current game's player names;
- the current game's card names.

Gathering is best-effort: if there is no game, only the arguments are used. When in doubt it redacts more.

**Activity, not requests, defines a day's player.** `player_day` is written from the registry's existing activity clock, the same place that decides idle expiry and places. So "a player used the playmat today" means exactly what "activity" already means: a sign-in, a tool call, a board page load or a board action. Board live connections and card lookups, which are not activity, do not count.

**Existing patterns followed:**
- Secrets are read only from the environment, with a minimum length, and their absence is a start-up error. This matches the Google client secret.
- The admin form and its POST handler follow the clear and check handlers: they render the same page with a message, under the same guard.
- Domain-separated hashing follows the board handle and the sealer key.
- The app definition's comments explain each new setting and alert, as they do for every existing setting.

**New pattern:** a closed catalogue of record types, written as JSON lines, read by people in an external log viewer. This is the playmat's first machine-readable output stream. The runbook is its user documentation.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Go standard library** (`crypto/hmac`, `crypto/sha256`, `encoding/json`, `io`): the player-ID hash and the JSON-line writer. No new third-party module.
- **`github.com/modelcontextprotocol/go-sdk` v1.6.0** (already a dependency): receiving middleware, the session's `InitializeParams().ClientInfo`, and its rule that a handler's error becomes an `IsError` tool result. No upgrade.
- **Sign-in service, per-account registry, board server, admin page and MCP tool server** (existing internal packages): each changes as the Component Breakdown describes. None needs to change before this work starts.
- **DigitalOcean Insights** (external, public preview; free during the preview, charged from general availability): it holds the playmat's runtime logs, charts the App Platform metrics and runs the alert rules. The maintainer has confirmed that the playmat's runtime logs appear in its Logs tab (LON1). It must stay opted in on the account.
- **DigitalOcean App Platform** (external): passes stdout to Insights; applies the `alerts:` rules from the app definition; holds the new secret.
  - A person sets the new secret in the control panel and points the alert rules at the maintainer's email, since the app definition cannot name recipients.
  - A person applies the edited app definition to the live app, because the deploy workflow does not apply app-definition edits.
- **Domain move spec (`20261111093353-8ff3589f-domain-move`)** (same epic, no ordering dependency): it also edits `.do/app.yaml`, which has uncommitted domain changes in the working tree. Merge this plan's `.do/app.yaml` edits onto whichever lands first; the two touch different keys.
- **Privacy, terms and disclaimer spec (`20261112083238-29247b36-privacy-terms-disclaimer`)** (downstream): it depends on this plan's runbook for what is recorded, what is never recorded, and the retention periods. Nothing here waits on it.
- **Knowledge entries relied on:**
  - `gotchas/go-sdk-sessions-and-auth.md`: look the account up per request from the token.
  - `gotchas/app-platform-logs-only-in-insights.md`, written during this plan's discovery: runtime logs are kept only in Insights.

## Testing Approach

The tests are standard Go tests beside the code, run by the project's usual test target, with the same table-driven style, fake clocks and in-process fakes the hosted packages already use: the fake Google, the fake card DB and test accounts. The recorder writes to an `io.Writer`, so every test reads the records it produced from a buffer and decodes each line as JSON. No test needs DigitalOcean.

**Unit tests (telemetry package; most coverage, because every privacy guarantee rests on it):**
- **Player IDs.** The same email gives the same ID across calls and across recorders built with the same key, whatever its case or surrounding spaces. Two emails give different IDs. Another key gives a different ID. A plain or unkeyed SHA-256 of the email (any common encoding) is not the ID. A key shorter than 32 characters is refused.
- **Records.** Every line is one valid JSON object with the fixed field order. The email given never appears in the output. `player_day` is written once per player per UTC day and again after midnight. A nil recorder writes nothing and does not panic.
- **Redaction.** Run against real failing-import messages built from the repo's test battle logs and decks: no quoted log line, card name, deck line or player name survives. The playmat's own wording ("before the Setup section", "is not on the current branch") does. Over-redaction is acceptable; a leak fails.

**Component tests (sign-in service, registry, board server, admin page):**
- Each existing email-bearing log path writes its structured record with the player ID and without the email: sign-in, sign-out, rate limit, user cap, expiry, removal, foreign board link and admin refusal.
- The tool-call middleware writes `tool_call` with the tool and the assistant mapped from the client name. It writes `import` and `new_game` only on success, and `error` with a redacted message on failure. It writes nothing for a request the rate limit refused (which writes `limited`).
- A failed board action writes `error` naming the action.
- Admin lookup: an admin gets the right ID for an email; a non-admin gets 403; a signed-out POST gets 401; a cross-site POST is refused.

**End-to-end acceptance tests (hosted wiring, through the real hosted handler with the fake Google and a recorder writing to a buffer).** These carry the spec's acceptance criteria:
- **Usage records appear.** Sign in, import a battle log over MCP as a named client, then start a new game. The buffer holds `sign_in`, `tool_call` (tool and assistant), `import` and `new_game` records, each with a time and the same player ID.
- **Daily usage readable** (logic only). Player A active twice and player B once in a day gives exactly two `player_day` records, plus the matching `tool_call`, `import` and `new_game` counts.
- **Errors recorded with context.** Importing a malformed battle log writes an `error` record naming `import_log`, a message and the player ID.
- **No identifying data.** Across a whole scripted session (sign-in with a known email and display name, import with a battle log and decks, a failing import, sign-out, an expiry), the raw output contains none of: the email, the display name, the Google token values the fake issued, any line of the battle log, any card name from the decks.
- **Existing log lines use the ID.** Sign-in, sign-out, hitting the rate limit and letting a game expire each write a record with the ID, and no email appears anywhere.
- **Stable, one-way ID.** The same account gets the same ID before and after a sign-out, with a different client name, and from a second hosted instance built with the same key (standing in for a redeploy). Two accounts differ. Hashing the email without the key does not give the ID.
- **Start-up.** Hosted mode refuses to start without `TCGREPLAY_PLAYER_ID_KEY`, or with one shorter than 32 characters.

**Regression guard.** A test scans the non-test Go sources of the hosted packages and fails on any `log.Printf` (or other logger call) whose arguments include an account's `.Email`. This keeps "no email address appears anywhere in the logs" true as code changes.

**Capacity measurement.** A Go test, skipped unless asked for by flag or environment variable, imports a full battle log with an opponent list, makes branches, and reports the heap one open game adds (memory statistics before and after, averaged over several games). It reports a figure; it never fails on one.

**Success metrics:**
- "A week after launch, the maintainer can say how many distinct players used the playmat each day and which assistants they used, from DigitalOcean alone": **Manual — captured in the implementation test plan.** The records that make it possible are covered by the end-to-end tests above. Reading them in Insights a week after launch needs the live service.
- "When a player reports a problem, the maintainer can find the matching error records from the player's email in under five minutes": **Manual — captured in the implementation test plan.** The lookup and the error records are covered by behavioural tests; the timed walk-through in Insights is manual.
- "No crash or resource exhaustion in the first month goes unnoticed until a player reports it": **Manual — captured in the implementation test plan.** Alerts fire on the real platform only.

**Manual reviews and checks:**
- Records appear in Insights: after a real sign-in, import and new game on the deployed playmat, each record is visible and filterable in Insights' Logs tab (LON1) with the same player ID, and the JSON fields can be filtered or text-searched. **Manual — captured in the implementation test plan.**
- Daily usage in Insights: on a day when two test accounts use the playmat (one twice), the `player_day` filter shows 2, and the tool calls by assistant, imports and new games can be read off. **Manual — captured in the implementation test plan.**
- No identifying data in Insights: searching a day's records in Insights for the test players' emails, display names, a token, a pasted battle log's text and a deck's card names finds nothing. **Manual — captured in the implementation test plan.**
- Stable ID across a real redeploy: the same account's ID matches before and after a deploy. **Manual — captured in the implementation test plan.**
- Maintainer-only lookup on the deployed admin page, as an admin, a non-admin and signed out. **Manual — captured in the implementation test plan.**
- Graphs available: Insights (and the app's Insights tab) show request rate, response time, responses by status code, CPU, memory and restart count for the last 24 hours and the last 7 days. **Manual — captured in the implementation test plan.**
- Alerts reach the maintainer: a forced restart triggers the restart alert email, and temporarily lowering the CPU and memory thresholds below current use triggers those alerts. **Manual — captured in the implementation test plan.**
- Capacity measured and set: with the chosen player limit's worth of full imported games open on the deployed instance, memory stays below the instance limit with the recorded headroom. **Manual — captured in the implementation test plan.**
- Retention written down: the runbook states how long Insights keeps logs and metrics, confirmed from DigitalOcean (console, docs or support). **Manual — captured in the implementation test plan.**
- Runbook review: a person checks that the runbook's event catalogue matches the code and that its Insights filters return what they claim. **Manual — captured in the implementation test plan.**

## Milestones & Tasks

### Milestone 1: Players appear in the logs only as a pseudonymous ID

**What changes**: Every line the hosted playmat writes about a player names them by a stable pseudonymous player ID instead of their email address. This covers sign-in, sign-out, a limit refusing a request, an idle game expiring, and the admin page's own lines. The lines become structured records, one JSON object per line, that DigitalOcean Insights can filter. The ID is the same across sign-ins, assistants, restarts and redeploys, and cannot be worked back to the email without the playmat's new secret. Hosted mode will not start without that secret. Nothing the player sees changes.

**Validation point**: Hosted tests show each of those lines as a record carrying the ID and no email. The ID is stable across a sign-out and a second instance built with the same key. The guard test finds no logger call that formats an account's email. Hosted mode refuses to start without the secret.

#### - [x] Task: Add the telemetry package
**Id:** fd47106e-01ae-4d4a-b378-b06a9d82beb1
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds a new package, used only by hosted mode, that holds the three privacy-critical pieces everything else builds on:
- the keyed one-way player ID made from an email address;
- the recorder that writes one JSON record per line with a fixed event catalogue, and writes each player's `player_day` record once a UTC day;
- the function that redacts user text out of error messages.

Doing these first, in isolation, lets their guarantees be tested thoroughly before anything calls them.

*Technical detail:* [context.md#task-add-the-telemetry-package](./context.md#task-add-the-telemetry-package)

**Acceptance criteria**:
- [x] The same email, in any case or with spaces around it, always gives the same player ID under the same key; other emails and other keys give other IDs; an unkeyed hash of the email never equals the ID; a key under 32 characters is refused.
- [x] Every record is a single valid JSON object on its own line, with time, level, event and player first, and the email handed to the recorder never appears in its output.
- [x] A player's `player_day` record is written once per UTC day, and again on the next day.
- [x] Redacting real failing-import messages built from the repo's test battle logs and decks leaves no battle-log line, card name or player name, while the playmat's own wording survives.
- [x] A missing recorder records nothing and never fails.

#### - [x] Task: Switch the existing log lines to player IDs
**Id:** 2d34fbef-6106-4d4e-a632-92ecf07ef83a
**Repo:** tcgreplay
**Depends on:**
- fd47106e-01ae-4d4a-b378-b06a9d82beb1 — Add the telemetry package
**Execution:** agent

Hosted mode reads a new secret for player IDs, refuses to start without it, builds one recorder, and hands it to the sign-in service, the account registry and the admin page. Every log line that carries an email address today becomes a structured record keyed by player ID: sign-in, refused sign-in, sign-out, rate limit, user cap, game expiry, account removal, a foreign board link, admin refusals, and the admin's card-set and image actions. A guard test stops an email-formatting log line from coming back.

*Technical detail:* [context.md#task-switch-the-existing-log-lines-to-player-ids](./context.md#task-switch-the-existing-log-lines-to-player-ids)

**Acceptance criteria**:
- [x] Signing in, signing out, hitting the request limit, being refused at the user cap and letting a game expire each write a record with the player ID and without the email address.
- [x] The admin page's own log lines and its refusal lines name the admin or caller by player ID, not email.
- [x] The same account has the same ID before and after a sign-out, and from a second hosted instance started with the same key.
- [x] Hosted mode will not start without the player-ID secret, or with one shorter than 32 characters, and its start-up line says player IDs are on without showing the key.
- [x] No logger call in the hosted packages formats an account's email address, and a test fails if one is added.

### Milestone 2: The maintainer can see who used the playmat, with what, and what went wrong

**What changes**: Every tool call is recorded with the tool and the assistant that made it, and so is every import and every new game. A player's first activity each day writes a `player_day` record, so the number of distinct players a day can be read in Insights. Every failed tool call or board action is recorded with the tool or action and its error message, with the player's battle log, decklist, card names and player names redacted. Signed in as an admin, the maintainer can enter a player's email on the admin page and get their player ID. That ID leads straight to the player's records.

**Validation point**: The end-to-end hosted tests pass for usage records, daily counts, errors with context, no identifying data, and the admin-only lookup.

#### - [x] Task: Record tool calls, daily players and errors
**Id:** c38bb8d1-1783-470f-9dc6-d72175ff321d
**Repo:** tcgreplay
**Depends on:**
- 2d34fbef-6106-4d4e-a632-92ecf07ef83a — Switch the existing log lines to player IDs
**Execution:** agent

Adds the tool-call middleware to the MCP server, behind the rate limit. It records each tool call with the tool and the assistant, each successful import and new game, and each failed call with its redacted error message. The registry's activity clock writes each player's first activity of the day, so distinct players a day can be counted. A new board error hook does the same for failed board actions. End-to-end hosted tests prove the spec's usage, error and privacy acceptance criteria.

*Technical detail:* [context.md#task-record-tool-calls-daily-players-and-errors](./context.md#task-record-tool-calls-daily-players-and-errors)

**Acceptance criteria**:
- [x] After a sign-in, an import through a named assistant and a new game, there is a record for each sign-in, tool call (with the tool and assistant), import and new game, each with a time and the same player ID.
- [x] Two players, one active twice and one once in a day, produce exactly two `player_day` records for that day, alongside their tool-call, import and new-game records.
- [x] A failing import (a malformed battle log) writes an error record naming the tool, the error message and the player ID; a failing board action writes one naming the action.
- [x] Across a whole session, the records contain none of: the test player's email or display name, any Google token value, any line of the pasted battle log, any card name from the decks.
- [x] A request refused by the rate limit is recorded once, as a limit refusal, not as a tool call.

#### - [x] Task: Let admins look up a player's ID
**Id:** 441388c1-66e8-4a6a-80f8-fdea78463aca
**Repo:** tcgreplay
**Depends on:**
- 2d34fbef-6106-4d4e-a632-92ecf07ef83a — Switch the existing log lines to player IDs
**Execution:** agent

Adds a "Find a player's ID" form to the existing admin page. An admin enters a player's email address and sees the player ID that their records carry. The form posts the address, so it never appears in a URL. It sits behind the admin guard that already refuses non-admins, signed-out callers and cross-site posts.

*Technical detail:* [context.md#task-let-admins-look-up-a-players-id](./context.md#task-let-admins-look-up-a-players-id)

**Acceptance criteria**:
- [x] Signed in as an admin, entering a player's email shows the same player ID their records carry.
- [x] A signed-in non-admin is refused, and a signed-out caller is refused or sent to sign in, without seeing any ID.
- [x] A lookup posted from another site is refused.
- [x] The looked-up email address appears in no log line or record.

### Milestone 3: The platform watches the playmat, and its limits and retention are known

**What changes**: DigitalOcean alerts the maintainer by email when the playmat restarts unexpectedly or its CPU or memory stays above a threshold. Insights charts traffic, response times, status codes, CPU, memory and restarts.
- How much memory a full game uses is measured, and the launch player limit and instance size are set from it with headroom.
- How long Insights keeps logs and metrics is confirmed.
- A runbook records the event catalogue, the Insights filters for each daily number and for a player's errors, the alert rules, the capacity figures and the retention periods. The privacy page work builds on that runbook.

**Validation point**:
- The app definition carries the alert rules, the secret and the chosen limit and size, and has been applied.
- The runbook exists with measured figures and confirmed retention.
- The manual checks in the test plan pass on the deployed playmat: records visible in Insights, alerts delivered, graphs present, memory within the limit.

#### - [x] Task: Declare alerts and the secret, and write the runbook
**Id:** ee78d4b7-f570-4962-bbfc-e4f236151fe6
**Repo:** tcgreplay
**Depends on:**
- c38bb8d1-1783-470f-9dc6-d72175ff321d — Record tool calls, daily players and errors
- 441388c1-66e8-4a6a-80f8-fdea78463aca — Let admins look up a player's ID
**Execution:** agent

Adds three alert rules to the app definition (unexpected restarts, sustained high CPU, sustained high memory), with comments saying where to change their thresholds and that recipients are set in the control panel. It also declares the player-ID secret there. Writes the observability runbook:
- every record and its fields;
- what is never recorded;
- the Insights filters for each daily number and for finding a player's errors from their email;
- the alert rules;
- the graphs to watch;
- sections for the capacity figures and retention periods, which later tasks complete.

*Technical detail:* [context.md#task-declare-alerts-and-the-secret-and-write-the-runbook](./context.md#task-declare-alerts-and-the-secret-and-write-the-runbook)

**Acceptance criteria**:
- [x] The app definition declares restart, CPU and memory alert rules for the playmat and the player-ID secret, each explained in a comment.
- [x] The runbook lists every record the playmat writes, matching the code, and states what records never contain.
- [x] The runbook explains how to read, for any day, distinct players, tool calls by assistant, imports and new games in Insights, and how to go from a player's email to their error records.
- [x] The website build still reads its limits from the app definition and still succeeds.

#### - [x] Task: Add the memory-per-game measurement
**Id:** a867b923-58c2-4ae1-92ea-1c428c383d12
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds a measurement, run only when asked for, that imports full battle logs from the repo's test data with an opponent list, makes branches, and reports how much memory one open game adds on average. It gives the maintainer a repeatable figure to set the launch player limit and instance size from, and to re-check after future changes.

*Technical detail:* [context.md#task-add-the-memory-per-game-measurement](./context.md#task-add-the-memory-per-game-measurement)

**Acceptance criteria**:
- [x] Asking for the measurement reports the average memory a full imported game with branches adds, and how many such games would fit in a given memory budget.
- [x] The measurement is skipped in ordinary test runs, so it never slows or fails them.

#### - [x] Task: Set up the secret and alert recipients, deploy and measure
**Id:** 8a4c8b44-c64e-4ba0-ac34-d4c1346072d7
**Repo:** tcgreplay
**Depends on:**
- ee78d4b7-f570-4962-bbfc-e4f236151fe6 — Declare alerts and the secret, and write the runbook
- a867b923-58c2-4ae1-92ea-1c428c383d12 — Add the memory-per-game measurement
**Execution:** human — needs the DigitalOcean control panel, production secrets and a production deploy

The maintainer:
- creates the player-ID secret in the App Platform control panel;
- applies the updated app definition, then deploys;
- points the three alert rules at their email.

They then measure on the deployed playmat: the memory figure from the measurement, and the memory Insights shows with several full games open. They also find out from DigitalOcean (console, docs or support) how long Insights keeps logs and metrics. The figures are handed to the next task.

*Technical detail:* [context.md#task-set-up-the-secret-and-alert-recipients-deploy-and-measure](./context.md#task-set-up-the-secret-and-alert-recipients-deploy-and-measure)

**Acceptance criteria**:
- [ ] The deployed playmat starts with the player-ID secret set and writes records with player IDs to Insights.
- [ ] The restart, CPU and memory alerts exist on the app and send to the maintainer's email.
- [ ] The memory used per full game (measured and as seen on the deployed instance) and Insights' retention for logs and metrics are written down for the next task.

#### - [x] Task: Record capacity and retention, and set the launch limits
**Id:** 0dd1d664-6c2d-4d6e-b8d5-8983e785892c
**Repo:** tcgreplay
**Depends on:**
- 8a4c8b44-c64e-4ba0-ac34-d4c1346072d7 — Set up the secret and alert recipients, deploy and measure
**Execution:** agent

Writes the maintainer's measured memory per game and Insights' confirmed retention periods into the runbook. Then sets the launch player limit and instance size in the app definition from them, keeping at least 30% of the instance's memory free with every place filled, with comments showing the calculation. The website, which shows the player limit, picks up the new value from the app definition.

*Technical detail:* [context.md#task-record-capacity-and-retention-and-set-the-launch-limits](./context.md#task-record-capacity-and-retention-and-set-the-launch-limits)

**Acceptance criteria**:
- [ ] The runbook states the measured memory per full game with branches, the chosen player limit and instance size, and the headroom they leave.
- [x] The runbook states how long usage records, error records and the hosting platform's metrics are each kept.
- [x] The app definition's player limit and instance size match the runbook, and the website build still succeeds with them.

#### - [x] Task: Apply the launch limits
**Id:** 9964965a-4f48-4dd8-9380-d395053d3399
**Repo:** tcgreplay
**Depends on:**
- 0dd1d664-6c2d-4d6e-b8d5-8983e785892c — Record capacity and retention, and set the launch limits
**Execution:** human — needs production access to the DigitalOcean app

The maintainer applies the app definition with the new player limit and instance size to the live app, and deploys. The deploy workflow does not apply edits to the app definition, so a person must apply it.

*Technical detail:* [context.md#task-apply-the-launch-limits](./context.md#task-apply-the-launch-limits)

**Acceptance criteria**:
- [ ] The live app runs on the chosen instance size with the chosen player limit.

## Open Questions

- **How long Insights keeps logs and metrics, and whether that keeps a week of records.** This depends on DigitalOcean: the public docs give no retention for Insights logs or metrics during the preview. The "Set up the secret and alert recipients, deploy and measure" task finds out.
  - If logs are kept for less than seven days, or are dropped on redeploy, the spec's "a week after launch" success metric cannot be met. The implementer must STOP and ask the user before recording it.
- **Whether Insights parses the JSON fields of an App Platform log line into filterable attributes.** This is only discoverable on the deployed app. If it does not, the runbook's text-search filters on the raw line (`"event":"player_day"`) still work, so the runbook uses them either way and mentions the attribute filters only if they exist. No STOP is needed unless neither works, in which case STOP and ask the user.
- **Whether the measured memory per game forces an instance size above the current one.** This depends on the measurement. Choosing a larger size changes the hosting cost. If the 70% rule needs more than the next size up (`apps-s-1vcpu-1gb`) for at least today's 10 players, STOP and ask the user before setting it.

## Out of Scope

- **The privacy page's wording.** This plan only provides the runbook it draws on. The wording belongs to the privacy, terms and disclaimer spec (`20261112083238-29247b36-privacy-terms-disclaimer`).
- **Visitor analytics for the website and board pages.** Covered by the visitor-analytics spec (`20261111132045-1d1d32f6-visitor-analytics`).
- **Alerts for failed deploys, and for domain or certificate problems.** These are a non-goal of the spec, even though App Platform offers those rules.
- **Public usage numbers or a stats page.** A non-goal; daily numbers are read only by the maintainer in Insights.
- **Log forwarding to any other service, a second monitoring vendor, or storage of our own for records** (Managed OpenSearch, Sentry, Better Stack, a Spaces object, a database). Excluded by the spec's constraints and the user's choice of Insights.
- **Usage counts kept in memory or shown on the admin page.** Rejected: they would be lost on every restart. The admin page gains only the ID lookup.
- **Recording board actions as usage.** Only failed board actions are recorded, as errors. A successful board move counts only towards that day's `player_day`, through the activity clock.
- **Tracing (OpenTelemetry spans) and custom metrics.** Insights offers no custom-telemetry ingestion for App Platform yet, and the spec asks for neither.
- **Changing the desktop app, daemon or dev mode.** They build no recorder and keep their current logging.
- **Automating alert recipients or Insights setup through the API.** The recipients are set once by hand in the control panel.

## Changelog

### 2026-11-12 — Task: Add the telemetry package

**What was done**: Added `internal/telemetry`: keyed pseudonymous player IDs (`IDs`, `NewIDs`, `p_` + 16 hex of a domain-separated HMAC-SHA256), the JSON-line `Recorder` with the closed event catalogue and once-per-UTC-day `player_day`, and `Redact`/`SensitiveFromJSON` for error messages. Every method is nil-safe.

**Deviations**:
- `Redact` keeps common playmat words at either end of a matched run in place (e.g. `no Solrock in play` → `no … in play`, not `no … play`), and treats a word made only of digits as common, so `line 3:` survives. The plan's rule redacted the whole longest run; trimming common edge words keeps the message's shape and reveals nothing of the player's.
- `commonWords` gained `and`, `or`, `have`, `are`, `was`, `current`, `branch`, `game` and the plan's prepositions (`in`, `on`, `at`, `to`, `of`, `for`, `from`, `by`, `with`, `into`, `onto`, `before`, `after`, `as`).
- `Recorder.Record` also cuts the event's own email out of `Message` and `Reason` (case-insensitive), as a defence in depth.
- The real-error redaction test uses import `Report.Problems` messages (replay failures are problems, not import errors) as well as the import's own errors; the "missing copies" case is a decklist with 1 Basic Fighting Energy and no `Total Cards` line.

**Files changed**:
- `internal/telemetry/ids.go`
- `internal/telemetry/recorder.go`
- `internal/telemetry/redact.go`
- `internal/telemetry/ids_test.go`
- `internal/telemetry/recorder_test.go`
- `internal/telemetry/redact_test.go`

**Discoveries**:
- go-sdk v1.6.0 `addMiddleware` applies each call's list in `slices.Backward`, so a second `AddReceivingMiddleware` call wraps OUTSIDE the first. To run the tool-call middleware inside the rate limiter, pass both in one call: `AddReceivingMiddleware(a.MCPMiddleware(), reg.MCPMiddleware())`.
- `game.Manager.Import` reports most replay failures as `Report.Problems` (with the log line in `Text`), not as an error; only parse-level failures (line before Setup, unknown owner, decklist count) return an error.

### 2026-11-12 — Task: Switch the existing log lines to player IDs

**What was done**: Hosted mode now reads `TCGREPLAY_PLAYER_ID_KEY` (environment only, at least 32 characters) and refuses to start without it, builds one recorder writing JSON lines to stdout, and hands it to the sign-in service, the registry and the admin page. Every email-bearing log line became a record keyed by player ID (`sign_in`, `sign_in_refused`, `sign_out`, `limited` rate/places, `game_expired`, `account_removed`, `board_refused`, `admin_refused`), the admin's card-set and image log lines name the admin by player ID and write `admin_action`, and a guard test parses the hosted packages' sources for an email-formatting logger call.

**Deviations**:
- `newHosted` keeps its four return values; the recorder is reached through a new `usergames.Registry.Recorder()` accessor, and `hostedAdmin` takes it as a third argument (`svc.PlayerID = rec.ID`, `svc.Recorder = rec`).
- The cache admin keeps its text log lines (now naming the admin by ID via `Service.by(r)`, or "admin" without a `PlayerID`) **and** writes an `admin_action` record (`check`, `clear`). The admin page's "Last check now by …" and "Images … by …" lines now show the player ID instead of the email; "Signed in as <email>" stays.
- Tests get the key from a new `TestMain` in `cmd/tcgreplay/hosted_test.go` that sets it only when the variable is unset (so a helper process given an empty key keeps it), instead of editing each helper. `serveHosted` gives each test server an `events` buffer (`hostedServer.events`).
- Superseded log assertions were rewritten as record assertions: `internal/hostauth/cap_test.go` (`TestCapRefusesNewAccountBothFlows`), `internal/usergames/places_test.go` (`TestAdmitLogsRefusal` → `TestAdmitRecordsRefusal`), `cmd/tcgreplay/hosted_cap_test.go`, and the admin tests' "by one@example.com" expectations (now the golden ID).

**Files changed**:
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/hosted_test.go`
- `cmd/tcgreplay/cardsets_test.go`
- `cmd/tcgreplay/hosted_admin_test.go`
- `cmd/tcgreplay/hosted_cap_test.go`
- `cmd/tcgreplay/hosted_events_test.go`
- `cmd/tcgreplay/noemail_test.go`
- `internal/hostauth/hostauth.go`
- `internal/hostauth/google.go`
- `internal/hostauth/board.go`
- `internal/hostauth/limit.go`
- `internal/hostauth/admin.go`
- `internal/hostauth/cap_test.go`
- `internal/hostauth/events_test.go`
- `internal/usergames/usergames.go`
- `internal/usergames/board.go`
- `internal/usergames/places_test.go`
- `internal/usergames/events_test.go`
- `internal/cacheadmin/cacheadmin.go`
- `internal/cacheadmin/handler.go`
- `internal/cacheadmin/cacheadmin_test.go`
- `internal/cacheadmin/events_test.go`

**Discoveries**:
- The guard test matches only a direct `.Email` selector in a logger call's arguments; an email copied to a local first would slip past it.
- `internal/mcpserver` `TestInstructionsTeachTheLoop/inmemory` failed once with a `TempDir RemoveAll ... directory not empty` cleanup race and passed on three re-runs; the package is untouched by this work.

### 2026-11-12 — Task: Record tool calls, daily players and errors

**What was done**: Added the tool-call middleware (`usergames.Registry.MCPMiddleware`), placed inside the rate limit, which records `tool_call` (tool, assistant), `import`/`new_game` on success and `error` with a redacted message on failure; `Registry.Use` now writes each player's daily `player_day`; a new `webui.Server.OnError` hook, wired to `Registry.BoardError`, records failed board actions as `error` naming the action. Redaction draws on the call's arguments and the caller's current game (player names, card names, and an imported game's log and decklist lines).

**Deviations**:
- The middleware is added in the same `AddReceivingMiddleware` call as the rate limiter (`AddReceivingMiddleware(a.MCPMiddleware(), reg.MCPMiddleware())`), because go-sdk v1.6.0 wraps a later call outside earlier ones; the plan anticipated this case.
- `gameNames` also feeds the imported game's record (`g.Record()`: me, log, deck, opponent deck) through `SensitiveFromJSON`, so an error on a later call still redacts the battle log; player names come from `state.Side.Name`.
- `hostedBoard` gained an `onError` parameter before `images`.
- `go test -race ./internal/usergames` intermittently fails `TestWaitingDoesNotKeepGame` ("the expiry did not end the wait"); it fails the same way on the unmodified HEAD source (1 of 6 race runs), so it is a pre-existing timing flake, not caused by this work. Without `-race` the whole suite passes.

**Files changed**:
- `internal/usergames/telemetry.go`
- `internal/usergames/usergames.go`
- `internal/usergames/telemetry_test.go`
- `internal/webui/server.go`
- `internal/webui/actions.go`
- `internal/webui/onerror_test.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_cache_test.go`
- `cmd/tcgreplay/hosted_usage_test.go`

**Discoveries**:
- The fake Google (`hostauthtest`) exposes no issued token values; the end-to-end privacy test rebuilds the unsigned ID tokens from its `Claims` hook (which also injects a `name` claim) and gets playmat tokens through `hostauthtest.Connect`.
- A go-sdk tool handler's error reaches a receiving middleware as an `IsError` `*mcp.CallToolResult` with a nil Go error; the hosted rate limiter likewise returns an `IsError` result without calling `next`.

### 2026-11-12 — Task: Let admins look up a player's ID

**What was done**: The admin page gained a "Find a player's ID" form (shown only when the service has a `PlayerID` function) that POSTs an email to `/admin/player`; the handler trims it, refuses an empty one with a message, shows the email and its ID in a copyable `<code>`, and records the lookup as the admin's `admin_action` (`lookup`). The looked-up address is never logged or recorded. The existing `RequireAdmin` guard refuses non-admins (403), signed-out callers (401) and cross-site posts (403).

**Deviations**:
- Without a `PlayerID` function, `POST /admin/player` answers 404 rather than rendering the page.
- The render path gained `renderLookup(w, status, email, msg, found)`; `render` calls it with no result. `Lookup{Email, ID}` is the page's result type.
- The cacheadmin lookup tests went into `internal/cacheadmin/events_test.go` beside the other recorder tests, not `cacheadmin_test.go`.

**Files changed**:
- `internal/cacheadmin/handler.go`
- `internal/cacheadmin/page.go`
- `internal/cacheadmin/events_test.go`
- `cmd/tcgreplay/hosted_admin_test.go`

**Discoveries**: None.

### 2026-11-12 — Task: Declare alerts and the secret, and write the runbook

**What was done**: `.do/app.yaml` now declares the `TCGREPLAY_PLAYER_ID_KEY` secret and three alert rules on the playmat (`RESTART_COUNT` > 0 over 5 minutes, `CPU_UTILIZATION` and `MEM_UTILIZATION` > 80 over 10 minutes), each explained in comments, plus a header line pointing at the runbook. A new runbook, `docs/observability.md`, covers the record format and the full event catalogue, what is never recorded and how messages are redacted, player IDs and the key, the Insights searches for each daily number, the email-to-errors path through /admin, the graphs and the alerts. It has placeholder Capacity and Retention sections for the later task.

**Deviations**:
- The goal check's first attempt found two runbook statements that did not match the code. The `idle` field is the configured idle time the game went past, not its actual idle time. The ID recipe now names the derived HMAC key and the message prefix. Both were fixed on retry.
- `README.md` has no operations or hosting section, so it does not link the runbook. The site's app-definition reader is `site/src/lib/facts.ts` (`loadLimits`), not something under `site/scripts`. `npm ci && npm run build` in `site/` succeeds; the Playwright browser suite was not run.
- Added `TCGREPLAY_PLAYER_ID_KEY` to `TestAppSpecHealthCheckAndSecrets`, and a new `TestAppSpecAlerts`, in `cmd/tcgreplay`.
- After the goal check accepted, a sentence was added to the runbook's usage table: calls from unnamed clients are the total less the four named assistants.

**Files changed**:
- `.do/app.yaml`
- `docs/observability.md`
- `cmd/tcgreplay/hosted_test.go`
- `cmd/tcgreplay/hosted_events_test.go`

**Discoveries**: None.

### 2026-11-12 — Task: Add the memory-per-game measurement

**What was done**: Added `TestMemoryPerGame` in `internal/game/measure_memory_test.go`, skipped unless `TCGREPLAY_MEASURE_MEMORY=1`. It holds N games open at once (default 10; `TCGREPLAY_MEASURE_GAMES`), each in its own manager. Each game is the corpus mirror log imported with the corpus deck as both lists, plus two branches with a few moves. Every game shares one catalogue loaded from the committed card sets, as hosted mode does. The test logs the heap before and after, the average per game, the baseline, and how many games fit in 512 MiB and 1 GiB at 70% use. Run it with `TCGREPLAY_MEASURE_MEMORY=1 go test -run TestMemoryPerGame -v ./internal/game/`.

**Deviations**:
- The test lives in `internal/game`, not `internal/usergames`, so it can reuse `committedCatalog` from `corpus_test.go`. The test data has no opponent decklists, so it uses the mirror log (`2026-10-11_lukiees_mirror.txt`) with the corpus deck as the opponent's list.
- A first draft overwrote the existing `internal/game/memory_test.go`, which holds other tests (`TestNothingWritten`, `TestRestartForgets`, `TestImportReplaces`, `TestImportNeedsDeck`). The goal reviewer caught it. The original file was restored from git, and the measurement moved to the new file `measure_memory_test.go`. The package's tests pass.
- A first local run measured about 0.31 MiB of heap per game (27.5 MiB baseline with one game open). That is the Go heap only: it leaves out the process's other memory and board connections, which the deployed measurement covers.

**Files changed**:
- `internal/game/measure_memory_test.go`

**Discoveries**:
- `internal/game` already has a `memory_test.go`, for the in-memory store behaviour. Check that a file name is free before creating it.

### 2026-11-13 — Task: Set up the secret and alert recipients, deploy and measure

**What was done**: Nothing in this run. This is a person's task. The user acknowledged it and owns it: the maintainer will set `TCGREPLAY_PLAYER_ID_KEY` and the alert recipients when deploying, then measure and confirm retention once observability is live.

**Deviations**:
- Accepted shortfall: the user deferred this task to the maintainer, to do after deploy. The goal check judged all three criteria not met: the deployed playmat with the secret and player-ID records in Insights; the alerts sending to the maintainer's email; the memory per game and Insights retention written down. None is ticked.
- The measurement is deferred to the maintainer, to run after deploy. `docs/observability.md` records capacity as pending and lists the steps. It records Insights retention as provisional figures to be confirmed by the maintainer (see the next task).
- The secret must be set on the live app **before** this branch reaches main: hosted mode refuses to start without it, and every push to main deploys.

**Files changed**: None.

**Discoveries**: None.

### 2026-11-13 — Task: Record capacity and retention, and set the launch limits

**What was done**: The runbook's Capacity section now records capacity as pending the maintainer's measurement. It sets out the 70% headroom rule and its formula, and the three steps: run `TestMemoryPerGame`, measure on the deployed instance, and record the results and apply the limits. It also gives the development figure, marked as not a launch figure. The Retention section gives provisional figures marked "to be confirmed by the maintainer":
- usage records: 7 days;
- error records: 7 days;
- platform metrics: 7 days;
- alert history: 30 days, which DigitalOcean documents.

It also lists the DigitalOcean pages checked on 2026-11-13. `.do/app.yaml` keeps `TCGREPLAY_MAX_USERS` "10" and `instance_size_slug` `apps-s-1vcpu-0.5gb`, with comments pointing at the runbook's Capacity section.

**Deviations**:
- Accepted shortfall: the user deferred the measurement to the maintainer after deploy and asked to keep the current launch limits. Criterion 1 (measured memory per game, the chosen limit and size, and their headroom) was judged not met and is not ticked. Criteria 2 (retention stated) and 3 (limits match the runbook; site build succeeds) were met.
- DigitalOcean documents no retention period for Insights logs or metrics during the preview; it documents only alert history, at 30 days. So, as the coordinator instructed, the runbook uses 7 days as a provisional figure: the longest range the Insights log search offers. The privacy page's retention markers are tested against these figures, so they must be updated together once the maintainer confirms the real periods.

**Files changed**:
- `docs/observability.md`
- `.do/app.yaml`

**Discoveries**:
- DigitalOcean's public docs (Insights limits, features and view-logs pages; App Platform view-insights and limits pages) state no retention for Insights logs or metrics; only "Alert history is available for up to 30 days". The Insights log search offers relative ranges up to "Last 7 days".

### 2026-11-13 — Task: Apply the launch limits

**What was done**: Nothing in this run. This is a person's task, and the user deferred it to the maintainer. After deploying and measuring, the maintainer applies `.do/app.yaml` with `doctl apps update <app-id> --spec .do/app.yaml`, along with any new `TCGREPLAY_MAX_USERS` and `instance_size_slug` the Capacity section leads to, and redeploys.

**Deviations**:
- Accepted shortfall: deferred to the maintainer. The goal check judged the one criterion (the live app runs on the chosen size with the chosen player limit) not met, and it is not ticked. The current launch values (`apps-s-1vcpu-0.5gb`, 10 players) stay in `.do/app.yaml`.

**Files changed**: None.

**Discoveries**: None.
