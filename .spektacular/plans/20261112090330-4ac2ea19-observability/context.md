---
created_date: "2026-11-12"
document_status: final
closed_date: "2026-11-12"
---

# Context: 20261112090330-4ac2ea19-observability

## Current State Analysis

- The hosted playmat (`cmd/tcgreplay/hosted.go`) logs free text with `log.Printf` to stderr. Ten lines carry a player's email address:
  - `internal/hostauth/google.go:93,231`, `internal/hostauth/board.go:132`, `internal/hostauth/admin.go:42,51`, `internal/hostauth/limit.go:86`;
  - `internal/usergames/usergames.go:123,134,193,267`, `internal/usergames/board.go:34`;
  - the cache admin's `by` in `internal/cacheadmin/cacheadmin.go:189,203,243,245`.
- Nothing records tool calls, imports, new games or errors.
- The only MCP receiving middleware is the rate limiter (`internal/hostauth/limit.go:92-119`, added at `cmd/tcgreplay/hosted.go:265`).
- The assistant name is derived from the MCP client's name (`internal/mcpserver/server.go:159-172`).
- go-sdk v1.6.0 turns handler errors into `IsError` tool results (`mcp/server.go:343-353` in the module cache).
- `.do/app.yaml` runs a single `apps-s-1vcpu-0.5gb` instance with `TCGREPLAY_MAX_USERS=10`, no alerts and no log destinations. It has uncommitted domain-move edits in the working tree.
- App Platform's Runtime Logs tab keeps no history. DigitalOcean Insights (public preview) does show the playmat's runtime logs, as the maintainer confirmed in the console (`gotchas/app-platform-logs-only-in-insights.md`).
- Admin pages are `internal/cacheadmin`, behind `hostauth.RequireAdmin` (`internal/hostauth/admin.go`).
- Requirement-to-repo resolution: every requirement lands in the `tcgreplay` repo (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`). The per-requirement files are listed at the top of Per-Task Technical Notes.

## Per-Task Technical Notes

### Requirement-to-repo resolution

Every requirement lands in the one registered repo, `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`):
- Usage recorded, errors recorded and records never identifying a player: `internal/telemetry` (new), `internal/usergames`, `internal/webui`, `cmd/tcgreplay/hosted.go`.
- Existing log lines switch to the ID: `internal/hostauth`, `internal/usergames`, `internal/cacheadmin`.
- Same player, same ID: `internal/telemetry/ids.go`, `cmd/tcgreplay/hosted.go`.
- Maintainer lookup: `internal/cacheadmin`.
- Traffic, resources and alerts: `.do/app.yaml`, plus DigitalOcean (person).
- Capacity: a measurement test in `internal/game` or `internal/usergames`, `.do/app.yaml`, `docs/observability.md`.
- Retention and the daily-usage method: `docs/observability.md`.

### Task: Add the telemetry package

**File changes:**
- `internal/telemetry/ids.go` (new) — `IDs`, `NewIDs(secret)`:
  - Refuse a secret under 32 characters, matching `minSigningKey` at `internal/hostauth/seal.go:13`.
  - The key is `sha256("tcgreplay player-id\x00"+secret)`.
  - `ID(email)`: trim, lower-case, then `hmac.New(sha256.New, key)` over `"tcgreplay player\x00"+email`, written as `"p_"+hex(sum)[:16]`. An empty email gives `""`.
- `internal/telemetry/recorder.go` (new) — event name constants (the table in plan.md § Data Structures).
  - The `Event` struct has the email tagged `json:"-"`. Its line struct fields are, in order: `Time string "time"`, `Level "level"`, `Event "event"`, `Player "player,omitempty"`, `Tool`, `Action`, `Assistant`, `Message`, `Reason` and `Idle`, each `omitempty`.
  - `NewRecorder(w io.Writer, ids *IDs, now func() time.Time)`. `Record` marshals the line struct with `json.Marshal` plus `\n`, under a mutex; it never uses `log`. Level is `error` for `error` events, else `info`.
  - `Active(email)`: when the UTC date (`now().UTC().Format("2006-01-02")`) differs from the stored day, reset the `map[string]struct{}`. On a player's first sighting, `Record(player_day)`.
  - Every method on a nil `*Recorder` is a no-op. `ID` on a nil recorder returns `""`.
- `internal/telemetry/redact.go` (new) — `Redact(msg string, sensitive []string) string`:
  1. Replace each `"…"`-quoted segment (Go `%q` output, honouring escaped quotes) with `"…"`.
  2. Tokenise the rest into words, separated by spaces and punctuation, with each word's span kept. For each start position, take the longest run of words (at most 12) whose joined text appears, case-sensitive and word-bounded, in any sensitive string. Replace that run with `…`, unless every word of the run is in `commonWords`.
  3. Collapse adjacent `…`.
  - `commonWords` is a small lower-case set of playmat vocabulary: articles, prepositions, `no`, `not`, `has`, `is`, `already`, `only`, `line`, `turn`, `hand`, `deck`, `bench`, `active`, `discard`, `prize(s)`, `card(s)`, `energy`, `setup`, `section`, `player(s)`, `log`, `decklist`. Matching against it is case-insensitive.
  - `SensitiveFromJSON(raw json.RawMessage) []string` walks the arguments JSON and returns every string value, splitting multi-line strings into lines, then decklist lines into their name parts (removing a leading count and trailing set code and number, as in `internal/deck` parsing).
- `internal/telemetry/ids_test.go`, `recorder_test.go`, `redact_test.go` (new) — the tests in plan.md § Testing Approach.
  - The redaction tests read real logs and decks from `testdata/logs/*.txt` and `testdata/decks`. They provoke real errors by calling `game.Manager.Import` with corrupted logs: a line before Setup (`internal/importer/parse.go:137`), missing card copies (`internal/importer/replay.go:214`), and a wrong player (`internal/importer/replay.go:315`), using `cardstest` or the test catalogue.
  - Each test asserts that no card name, player name or log line from the inputs survives. Keep this in `internal/telemetry` with a test-only import of `internal/game`, or put the real-error cases in `internal/usergames` tests if an import cycle appears. telemetry must not import game in non-test code.

**Complexity:** Medium
**Token estimate:** ~35k tokens
**Agent strategy:** 2 parallel agents: one for IDs and the recorder, one for Redact and its real-error corpus. Integrate sequentially.

### Task: Switch the existing log lines to player IDs

**File changes:**
- `cmd/tcgreplay/hosted.go:117-134` — `hostedAuth` gains a `rec *telemetry.Recorder` parameter, which it passes as `hostauth.Config.Recorder`.
- `cmd/tcgreplay/hosted.go:236-266` — `newHosted`:
  - Read `TCGREPLAY_PLAYER_ID_KEY` from the environment only. Return an error naming the variable when it is missing or under 32 characters.
  - Build `telemetry.NewRecorder(c.events, ids, c.now)`, where `c.events` is a new `config` field (`io.Writer`) that defaults to `os.Stdout`; tests set a buffer.
  - Pass the recorder to `usergames.Config.Recorder`.
  - Return the recorder, or keep it on the registry, so `runHosted` and the admin wiring can reach it.
- `cmd/tcgreplay/hosted.go:166-175` — `hostedAdmin` sets `svc.PlayerID = rec.ID`. This is used by the lookup task, but wire it here so the admin log lines can use it.
- `cmd/tcgreplay/hosted.go:382` — the start-up line adds "player IDs on". Never print the key.
- `cmd/tcgreplay/main.go:49` — `config` gains `events io.Writer`.
- `internal/hostauth/hostauth.go:26-62` — `Config` gains `Recorder *telemetry.Recorder`.
- `internal/hostauth/google.go:93` — replace the line with `Record(sign_in_refused, Email, Reason: err.Error())`. The busy message names only the cap, which is safe.
- `internal/hostauth/google.go:231` — `Record(sign_in, Email)`.
- `internal/hostauth/board.go:132` — `Record(sign_out, Email)`.
- `internal/hostauth/limit.go:86` — `Record(limited, Email, Reason: "rate", Message: err.Error())`.
- `internal/hostauth/admin.go:42,51` — `Record(admin_refused, Email, Reason: "not admin" | "not sent from the playmat")`. The 403 page may still show the email to that user, since it is a page, not a log.
- `internal/usergames/usergames.go:25-41` — `Config` gains `Recorder`.
- `internal/usergames/usergames.go:123` — `Record(limited, Reason: "places", Message)`.
- `internal/usergames/usergames.go:134,267` — `Record(account_removed, Reason: signed_out | idle)`.
- `internal/usergames/usergames.go:193` — `Record(game_expired, Idle: humanDuration(...))`.
- `internal/usergames/board.go:34` — `Record(board_refused)`.
- `internal/usergames/usergames.go:281` — the summary line has no email; keep it as it is.
- `internal/cacheadmin/handler.go:40-43` — add `by(r)`, which returns `s.PlayerID(email)` when set, else `"admin"`. Use it for the `by` passed to `CheckNow` and `Start`, so `internal/cacheadmin/cacheadmin.go:189,203,243,245` log the ID. The page's own display of the signed-in email (`render(..., email(r), ...)`) stays.
- `internal/cacheadmin/cacheadmin.go` — `Service` gains `PlayerID func(email string) string`. Also record `admin_action` (`Action: "check" | "clear"`) through an optional `Recorder` field, or keep the text log keyed by the ID; prefer the recorder for consistency.
- Tests:
  - Extend `internal/hostauth/*_test.go`, `internal/usergames/*_test.go` and `internal/cacheadmin/cacheadmin_test.go` to build a recorder on a buffer and assert each record has the ID and no email.
  - Add `cmd/tcgreplay/hosted_events_test.go`, covering: start-up refused without the key or with a short key; sign-in, sign-out, rate limit and expiry records; stable ID across a sign-out and a second `newHosted` with the same key.
  - Add `cmd/tcgreplay/noemail_test.go` (or one under `internal/`). It parses the non-test `.go` files of `internal/hostauth`, `internal/usergames`, `internal/cacheadmin`, `internal/webui` and `cmd/tcgreplay` with `go/parser`, and fails on any call to `log.Print*`, `log.Fatal*` or a `Logf` whose arguments contain a selector ending in `.Email`.
  - Existing hosted tests must set `TCGREPLAY_PLAYER_ID_KEY`: update the helpers in `cmd/tcgreplay/hosted_test.go` and the other `hosted_*_test.go` files that call `newHosted` or `runHosted`, including `TestHostedMainHelper`.

**Complexity:** Medium
**Token estimate:** ~45k tokens
**Agent strategy:** 2-3 parallel agents (hostauth; usergames and cacheadmin; cmd wiring and the guard test), then a sequential integration pass running `go test ./...`.

### Task: Record tool calls, daily players and errors

**File changes:**
- `internal/usergames/telemetry.go` (new) — `func (r *Registry) MCPMiddleware() mcp.Middleware`:
  - For `method == "tools/call"`, read `req.(*mcp.CallToolRequest)`, the account from `hostauth.AccountFromToken(req.GetExtra().TokenInfo)`, and the assistant from `mcpserver.AssistantName(req.Session.InitializeParams().ClientInfo.Name)`. Guard every nil.
  - Call `next`, then always `Record(tool_call, Tool: params.Name, Assistant)`.
  - On a result with `!IsError`, record `import` for `import_log` and `new_game` for `new_game`.
  - On `IsError`, or a Go error, record `error` with `Tool` and `Message: telemetry.Redact(text, sensitive)`. The text is the result's first `TextContent` (or `err.Error()`). `sensitive` is `telemetry.SensitiveFromJSON(params.Arguments)` plus `r.gameNames(acct)`.
  - The middleware must never change the result.
- `internal/usergames/telemetry.go` — `gameNames(acct)` takes `r.Get`, without counting it as activity: under the registry's lock, look up `r.entries[acct.ID]` directly. It returns the current game's player names and the names of every card in its state, through the game's card DB.
  - Check the read API in `internal/game/reads.go` and `internal/game/game.go`. If there is no game, return nil.
  - Do not call `Use`: this lookup must not count as activity.
- `internal/usergames/telemetry.go` — `func (r *Registry) BoardError(ctx context.Context, action string, err error)`: the account comes from `hostauth.AccountFrom(ctx)`. Record `error` with `Action` and the message redacted against `gameNames`. Skip `hostauth.ErrRateLimited` and `hostauth.ErrBusy`, which are already recorded as `limited`.
- `internal/usergames/usergames.go:101-115` (`Use`) — after a successful use, call `r.cfg.Recorder.Active(acct.Email)`. A busy refusal must not call it.
- `internal/webui/server.go:30-46` — add the `OnError func(ctx context.Context, action string, err error)` field.
- `internal/webui/actions.go:89-112` (`dispatch`) — when `s.do` returns an error and `OnError != nil`, call `OnError(ctx, req.Type, err)`. Use the request's verb field; check the `request` struct. Limit refusals at `:92-97` do not call it.
- `cmd/tcgreplay/hosted.go:269-279` (`hostedBoard`) — take an `onError` and set `ui.OnError`. `newHosted` passes `reg.BoardError`.
- `cmd/tcgreplay/hosted.go:264-265` — after `srv.AddReceivingMiddleware(a.MCPMiddleware())`, add `srv.AddReceivingMiddleware(reg.MCPMiddleware())`. Confirm in go-sdk v1.6.0 `mcp/shared.go` (`addMiddleware`) that a later middleware runs inside an earlier one; if the order is reversed, pass both in one call in the right order.
- Tests:
  - `internal/usergames/telemetry_test.go` (new): the middleware with fake requests (success, IsError, import and new_game), assistant mapping, and redaction drawing on the game's card and player names. `Active` is called from `Use` but not from `Get`, `ResolveMCPQuiet` or a busy refusal.
  - `internal/webui` test: `OnError` is called for a failing action and not for a limit refusal.
  - `cmd/tcgreplay/hosted_usage_test.go` (new): end-to-end over the real hosted handler with the `hostauthtest` fake Google and an MCP client whose `ClientInfo.Name` is `claude-ai`. Model it on `cmd/tcgreplay/hosted_test.go:165` (`TestHostedHandlerServesMCPAndBoard`) and `:346-361`. It covers:
    - usage records (sign-in, import with a `testdata/logs` log and `testdata/decks` list, then new game);
    - daily players (two accounts, one acting twice) giving exactly two `player_day`;
    - a malformed import giving an `error` record with `tool:"import_log"`;
    - no identifying data: grep the whole buffer for the email, the fake Google's issued `access_token` and `id_token` values and the playmat's own bearer and refresh tokens, every non-empty log line of the imported battle log, and every card name in both decks. All must be absent.
  - The Google display name is not requested (scopes `openid email`, `internal/hostauth/hostauth.go:150`), so assert that the fake's `name` claim is absent too.

**Complexity:** High
**Token estimate:** ~55k tokens
**Agent strategy:** Parallel analysis: one agent for the middleware and redaction wiring, one for the board hook. Integrate sequentially, then write the end-to-end test.

### Task: Let admins look up a player's ID

**File changes:**
- `internal/cacheadmin/handler.go:18-26` — register `POST /admin/player`. Parse the form, take `email := strings.TrimSpace(r.PostForm.Get("email"))`, and refuse an empty one with a message. Render the page with a `Lookup{Email, ID}` result. Use `Cache-Control: no-store`. Never log the email; record `admin_action` with `Action: "lookup"` keyed by the admin.
- `internal/cacheadmin/page.go` — add a "Find a player's ID" section, shown only when `s.PlayerID != nil`. It is a POST form with one email input, and shows the result: the email entered and its ID in a `<code>` element that can be copied. Follow the existing form markup and its error message pattern.
- `cmd/tcgreplay/hosted.go:190-193` — no route change is needed: `/admin/` already goes to `adminPage` behind `RequireAdmin`.
- Tests:
  - `internal/cacheadmin/cacheadmin_test.go`: the lookup returns the injected function's ID, and the page carries no form when `PlayerID` is nil.
  - `cmd/tcgreplay/hosted_admin_test.go`: an admin POST returns the same ID that the records carry for that account; a non-admin gets 403; a signed-out caller gets 401; a cross-site POST with the wrong `Origin` gets 403; the recorder buffer does not contain the looked-up email.

**Complexity:** Low
**Token estimate:** ~15k tokens
**Agent strategy:** Single agent, sequential execution.

### Task: Declare alerts and the secret, and write the runbook

**File changes:**
- `.do/app.yaml` (services[0].envs) — add `TCGREPLAY_PLAYER_ID_KEY` with `scope: RUN_TIME`, `type: SECRET`. Its comment says: at least 32 characters (`openssl rand -base64 48`); hosted mode will not start without it; changing it changes every player's ID, so old records no longer match; set it in the control panel, encrypted.
- `.do/app.yaml` (services[0]) — add `alerts:`:
  - `RESTART_COUNT` `GREATER_THAN` `0` over `FIVE_MINUTES`;
  - `CPU_UTILIZATION` `GREATER_THAN` `80` over `TEN_MINUTES`;
  - `MEM_UTILIZATION` `GREATER_THAN` `80` over `TEN_MINUTES`.

  The comment says the thresholds are the maintainer's to change, that recipients are set in the control panel (Settings > Alerts, or the Insights notification channels) because the app spec cannot name them, and that changes need `doctl apps update`.
  - Keep the domain-move edits already in the working tree. Touch only these keys.
- `.do/app.yaml` (header comment) — one line saying records go to stdout and are read in DigitalOcean Insights, pointing to `docs/observability.md`.
- `docs/observability.md` (new) — sections:
  - **What is recorded**: the event table with every field. Include the exact JSON line shape.
  - **What is never recorded**: email, name, Google tokens, battle logs, decklists, card data. Explain how error messages are redacted.
  - **Player IDs**: the keyed hash, the secret, rotation, and that the IDs cannot be reversed.
  - **Reading daily usage in Insights**: Control Panel > Insights > Logs, region LON1, an absolute From and To for one UTC day. Text-search `"event":"player_day"` and read the match count, which is the distinct players (a restart that day can over-count). Repeat with `"event":"tool_call"` plus `"assistant":"Claude"` (and each other assistant), `"event":"import"` and `"event":"new_game"`. If Insights parses the JSON into attributes, use the filters panel instead. Note the 7-day limit on a single query.
  - **Finding a player's errors**: email → /admin lookup → ID → search `"player":"p_…"` together with `"event":"error"`.
  - **Graphs**: where the request rate, P95, status codes, CPU, memory and restarts are, in Insights and in the app's Insights tab.
  - **Alerts**: the three rules and where recipients are set.
  - **Capacity** and **Retention**: headings with a line saying they are filled from the measurement and from DigitalOcean (by the "Record capacity and retention" task). Write no figures yet.
  - Link the runbook from `README.md` only if the README has an operations or hosting section; otherwise leave the README alone.
- Check that the website build that reads limits from `.do/app.yaml` (see the `Makefile` `site` target comment, and the reader under `site/scripts`) still parses the file with the new `alerts:` block.

**Complexity:** Low
**Token estimate:** ~20k tokens
**Agent strategy:** Single agent, sequential execution.

### Task: Add the memory-per-game measurement

**File changes:**
- `internal/usergames/memory_test.go` (new), or `internal/game/memory_test.go` if usergames is not needed — `TestMemoryPerGame`:
  - `t.Skip` unless `TCGREPLAY_MEASURE_MEMORY=1`.
  - For N (default 10) iterations, build a fresh `game.Manager` (as `newRegistry` does at `internal/usergames/usergames_test.go:40-54`), import a full `testdata/logs/*.txt` log with its deck from `testdata/decks` (see `testdata/index.json`) and an opponent list, then create two branches by rewinding and replaying a few moves.
  - Use `runtime.GC()` and `runtime.ReadMemStats` before and after (HeapInuse), keeping the managers referenced.
  - Log the average bytes per game, and how many games fit in 512 MiB and 1 GiB at 70% use.
  - Use the card source the existing import tests use; check `internal/game/*import*_test.go` for the catalogue fixture. A real game holds a pinned card snapshot, so share one source between games, as hosted mode does, and do not count the snapshot per game.

**Complexity:** Low
**Token estimate:** ~15k tokens
**Agent strategy:** Single agent, sequential execution.

### Task: Set up the secret and alert recipients, deploy and measure

A person, the maintainer, with DigitalOcean control panel access:
1. Generate a key (`openssl rand -base64 48`), add `TCGREPLAY_PLAYER_ID_KEY` as an encrypted variable on the playmat component, and keep a copy in their password manager.
2. Apply the app definition with `doctl apps update <app-id> --spec .do/app.yaml`, keeping the domain-move changes. Then deploy, by pushing to main or starting the deploy workflow.
3. In the control panel, set the alert recipients for the three rules to the maintainer's email: the app's Settings > Alerts, or an Insights email notification channel.
4. Run `TCGREPLAY_MEASURE_MEMORY=1 go test -run TestMemoryPerGame -v ./internal/usergames/` (or wherever the test landed) and note the figure.
5. On the deployed playmat, open several full imported games with branches from test accounts. Note the memory percentage Insights shows before and after.
6. Find out from DigitalOcean (the Insights console, docs or support) how long Insights keeps logs and metrics during the preview, and whether that will change at general availability.
7. Put the figures from steps 4-6 into the next task's brief (or a PR comment).

**Checked by:** records with player IDs visible in Insights; the three alerts listed on the app with the maintainer's email; the figures written down.

### Task: Record capacity and retention, and set the launch limits

**File changes:**
- `docs/observability.md` (Capacity) — the measured bytes per game (test and deployed), the calculation `limit = floor(0.7 × instance memory − baseline) / per-game`, and the chosen `TCGREPLAY_MAX_USERS` and `instance_size_slug`.
- `docs/observability.md` (Retention) — usage records and error records (both Insights logs: the confirmed period), and platform metrics (Insights and App Platform metrics: the confirmed period), with the date confirmed and the source.
- `.do/app.yaml` — `instance_size_slug` (currently `apps-s-1vcpu-0.5gb`) and `TCGREPLAY_MAX_USERS` (currently `"10"`) set from the calculation, with a comment citing `docs/observability.md`.
- Confirm that the website build still reads `TCGREPLAY_MAX_USERS` from `.do/app.yaml` and builds.

**Complexity:** Low
**Token estimate:** ~8k tokens
**Agent strategy:** Single agent, sequential execution.

### Task: Apply the launch limits

A person with production access applies `.do/app.yaml` (`doctl apps update <app-id> --spec .do/app.yaml`) and deploys.

**Checked by:** the app's Settings page shows the chosen instance size, and the start-up log line in Insights shows the chosen user limit.

## Testing Strategy

Per task:
- **Add the telemetry package**: table-driven unit tests for IDs (stability, case, keys, unkeyed hash differs, short key refused), the recorder (JSON per line, field order, no email, `player_day` once per UTC day using a fake clock, nil-safe) and Redact (real failing-import messages from `testdata/logs` and `testdata/decks`; no card or player name or log line survives).
- **Switch the existing log lines to player IDs**:
  - Component tests in hostauth, usergames and cacheadmin with a recorder on a buffer.
  - Hosted tests: start-up refused without the key; sign-in, sign-out, rate-limit and expiry records carry the ID and no email; stable ID across a sign-out and a second instance with the same key.
  - The guard test that parses the hosted packages' sources and fails on an email-formatting logger call.
  - Existing hosted tests set `TCGREPLAY_PLAYER_ID_KEY`.
- **Record tool calls, daily players and errors**:
  - Middleware unit tests with fake requests.
  - A webui test for `OnError`.
  - The end-to-end hosted usage test with the fake Google and a named MCP client: usage records, two `player_day` records for two players, an error record for a malformed import, and no identifying data in the whole buffer.
- **Let admins look up a player's ID**: cacheadmin page test and hosted admin tests covering admin, non-admin, signed-out and cross-site, plus no email in the records.
- **Declare alerts and the secret, and write the runbook**: the website build still parses `.do/app.yaml`, and `go test ./...` still passes.
- **Add the memory-per-game measurement**: the test is skipped by default and reports a figure when `TCGREPLAY_MEASURE_MEMORY=1`.
- **Human tasks and manual checks**: every item marked "Manual — captured in the implementation test plan" in plan.md § Testing Approach. These are the three success metrics and the manual reviews: records visible and filterable in Insights, daily usage read in Insights, no identifying data in Insights, a stable ID across a real redeploy, the lookup on the deployed admin page, the graphs, alert delivery, capacity on the deployed instance, retention written down, and the runbook review.

## Project References

- Spec: `20261112090330-4ac2ea19-observability` (epic `20261111093348-e1b5c2b4-public-launch`).
- Downstream spec: `20261112083238-29247b36-privacy-terms-disclaimer` (reads `docs/observability.md`).
- Sibling spec sharing `.do/app.yaml`: `20261111093353-8ff3589f-domain-move`.
- Design documents: none.
- Knowledge entries:
  - `repo/tcgreplay` `conventions/skills-text-and-packaging.md`;
  - `repo/tcgreplay` `gotchas/go-sdk-sessions-and-auth.md`;
  - `repo/tcgreplay` `gotchas/app-platform-logs-only-in-insights.md`, written during this plan.
- Repo root: tcgreplay at `/home/nicj/code/github.com/nicholasjackson/tcgreplay`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

## Migration Notes

- A new required secret, `TCGREPLAY_PLAYER_ID_KEY`, must be set in the App Platform control panel **before** the first deploy that contains the "Switch the existing log lines to player IDs" task. Otherwise hosted mode refuses to start and the deploy fails its health check. The "Set up the secret and alert recipients, deploy and measure" task sets it. If the code merges earlier, set the secret first.
- `doctl apps update --spec .do/app.yaml` must be run for the alerts, the secret declaration and the new limits to take effect: the deploy workflow does not apply app-definition edits.
- Log lines that used to carry email addresses are now JSON records. Anything searching for the old text (`hostauth: signed in …`) must search for `"event":"sign_in"` instead.
- Records written before this work still hold emails, but App Platform does not retain them, and Insights keeps them only for its retention period.

## Performance Considerations

- Each record is one small `json.Marshal` and one write, under a mutex. At the configured rate limit (300 requests a minute per account) and with tens of players, this is negligible.
- The `player_day` set grows by one entry per distinct player a day and is cleared at midnight UTC.
- Redaction runs only on errors. Gathering card names walks the current game's cards once per error.
- The memory measurement decides `TCGREPLAY_MAX_USERS` and the instance size, keeping at least 30% of memory free with every place filled.
