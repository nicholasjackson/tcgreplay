---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Research: 20261017143118-b05aa6b1-per-user-games

## Alternatives considered and rejected

### Option: One `*mcp.Server` per account, chosen by the Streamable handler's `getServer(r)`

The go-sdk calls `getServer` only when a session is created (initialize), so each claude.ai session would be pinned to the server instance, and so to the manager, it started with. When an idle game is dropped or an account's entry is rebuilt, live sessions would keep acting on a stale manager. It would also register every tool's schema once per account.

**Rejected**: In favour of one shared `*mcp.Server` that resolves the caller's game on each tool call through `RequestExtra.TokenInfo` (go-sdk v1.6.0 `mcp/shared.go:477-490`). Every tool already goes through one helper, `add` (`internal/mcpserver/server.go:82-91`), so that is the single place to resolve it.

### Option: A multi-tenant hub inside one `webui.Server`, with each client tagged by account

The hub keeps one `pending` events list and one `watched` set for a single manager, and pushes the same message to every client (`internal/webui/hub.go:21-110`). Making it per-account would mean rewriting the push loop, its caching and the frames.

**Rejected**: Building one `webui.Server` per account through the existing `webui.New(m, human)` (`internal/webui/server.go:36-45`) scopes live updates by construction and leaves the hub's code unchanged.

### Option: Deleting an account's entry when its game expires and keeping only a tombstone

Open tabs would lose their hub, so they could not show the expiry or follow the next import without a reload. The "expired" notice would need storage of its own, and the remembered decklist would be lost.

**Rejected**: The entry, with its manager and board server, is kept and only the game is dropped. Entries that hold no place are purged after a day so memory stays bounded.

### Option: A board link that names the game id

For example `?game=import-…`. The id changes with every import, but all of an account's tabs must follow the account's one game (spec: "importing in one replaces it for all").

**Rejected**: In favour of a link that identifies the owner (`/?board=<handle>`).

### Option: Keeping the board link at the public root

This is the link web-mcp-docker set. Each account would then see its own board at `/`, but account B opening A's link would see B's own game rather than the "not your game" message the spec requires.

**Rejected**: In favour of `/?board=<owner handle>`.

### Option: Ending an account's sign-ins when its game expires

The aim would be to let google-login's existing "a place frees when the last sign-in ends" rule frees the place.

**Rejected**: The account's next action must say the game expired and ask for the record, not force a new sign-in. claude.ai's behaviour after a sign-in ends is also google-login's open question.

### Option: Keeping the user-cap counter in `hostauth` and exporting `Admit`/`Release` for the game registry to call

An account that signs in and never acts would hold a place until restart, unless `hostauth` also told the registry about every sign-in. That would leave two structures tracking places.

**Rejected**: The per-account registry becomes the one owner of places, and `hostauth` asks it at the Google callback and tells it when an account's last sign-in ends. It is one limit mechanism, with the same setting and the same "server busy" page.

### Option: Counting every MCP request, or the board's websocket connection, as activity

claude.ai may open sessions and list tools without acting, and the board client reconnects on its own after any drop (`web/src/lib/ws.js:17-23`). Counting either would let an open tab or an idle connector keep a game alive, against "a board tab simply left open does not count".

**Rejected**: Only tool calls, board verbs (all are user-triggered: `web/src/lib/actions.js`, `web/src/App.svelte:18-24`, `web/src/lib/playback.svelte.js:74`) and board page loads count.

### Option: Saving games across the idle drop or restarts

Keep games beyond the idle time or a restart.

**Rejected**: This is a spec non-goal, and memory-store-record forbids game storage.

## Chosen approach — evidence

- Every MCP tool is registered through `add` (`internal/mcpserver/server.go:82-91`, 46 registrations). Every tool body reaches the game through `s.M` (`internal/mcpserver/server.go:121, 316, 326, 340-368, 380-744`). One resolution step in `add`, storing the caller's manager and board link on the context, plus a mechanical `s.M` → `s.m(ctx)` change, therefore covers every tool.
- go-sdk v1.6.0 hands every server request `Extra.TokenInfo` (`mcp/shared.go:477-490`), and google-login's verifier sets `TokenInfo.UserID` to the Google subject, with `hostauth.AccountFromToken` reading it. The Streamable handler binds sessions to `UserID`, so a session cannot cross accounts.
- The board server is a thin shell around one manager: `webui.New` subscribes the hub to `m.OnLoad` (`internal/webui/server.go:36-45`), and the hub reads only `h.s.M` (`internal/webui/hub.go:121-126`). One `webui.Server` per account therefore gives scoped live updates with no change to the hub.
- Board verbs already pass through one hook, google-login's `webui.Server.Limit(ctx)`, called in `dispatch` with the websocket request's context (`internal/webui/actions.go:83`), which carries the account. Wrapping that hook in hosted wiring makes "a move on the board" count as activity without changing webui.
- google-login's `RequireSession` cancels a websocket's context when its session ends. The same context-merge trick closes an account's tabs when its entry is purged.
- `Manager` already holds exactly the per-session state that has to become per-account: the current game, the assistant label and, after memory-store-record, the last decklist and the record on `Game` (`internal/game/manager.go:22-33`). One manager per account makes all of it per-account at once.
- Seat labels come from the game's side names, falling back to "You" for the human seat (`web/src/App.svelte:26-29`). An imported game names the owner's side with the account's own `me`, so per-account games already give each account its own name. Only the fallback needs the signed-in account.

## Files examined

- `tcgreplay:internal/game/manager.go:22-112` — Manager holds `current`, `onLoad` hooks and the assistant label (file in Dir); `setCurrent` closes the previous game and runs the hooks; `Current()` error text names `load_game`, which memory-store-record changes.
- `tcgreplay:internal/game/import.go:46-140` — import reads and writes the last deck from a file (memory after memory-store-record); the player name `me` is detected or given per import.
- `tcgreplay:internal/mcpserver/server.go:17-91` — `Server{M, ClaudeSeat, HumanSeat, Version}` (+ `BoardURL` from web-mcp-docker); `InitializedHandler` sets the assistant label from the client name; `add` is the only tool registration path.
- `tcgreplay:internal/mcpserver/server.go:308-370` — game-opening tools (`new_game`, `import_log`, `set_opponent_deck`) build their result maps here; the `board` key is added to them.
- `tcgreplay:internal/mcpserver/server_test.go:17-45` — `connect` builds a server on an in-memory transport; `call` helper.
- `tcgreplay:internal/webui/server.go:28-71` — `Server{M, Human, hub}`, `New`, `Handler` (`/ws`, `/api/card`, `/`), `localOnly` (lifted by web-mcp-docker's `Public`).
- `tcgreplay:internal/webui/hub.go:21-236` — single-manager hub; `stateMessage` reads `M.Current()` and, with no game, the games list (removed by memory-store-record); `serveWS` dispatches with `r.Context()`.
- `tcgreplay:internal/webui/actions.go:83-120` — `dispatch` → `do`; every board verb is user-initiated.
- `tcgreplay:internal/webui/webui_test.go:78-110` — `setup`, `dial` and `next` helpers for websocket tests.
- `tcgreplay:web/src/lib/ws.js` — auto-reconnect with backoff; `onState` reads `human`, `claude`, `assistant`, `reveal`, `snapshot`, `error`.
- `tcgreplay:web/src/App.svelte:26-29, 74-82` — `nameOf` fallback 'You'; the landing text when there is no game.
- `tcgreplay:web/src/lib/actions.js`, `web/src/lib/playback.svelte.js:74` — no verb is sent automatically.
- `tcgreplay:Makefile` — `make test` = `go vet ./...` + `go test ./...`.
- `tcgreplay:TCG Review Playmat Design and Build Plan.md:299` — the Runtime paragraph, extended by web-mcp-docker (hosted) and google-login (sign-in).
- `go-sdk@v1.6.0/mcp/shared.go:477-520` — `ServerRequest.Extra *RequestExtra{TokenInfo, Header}`; `GetExtra()`.

## External references

- go-sdk v1.6.0 `mcp` package (`RequestExtra.TokenInfo`, the Streamable HTTP handler's `getServer` called per session) — the reason resolution is per call, not per session.
- MCP authorization spec 2025-11-25, via google-login — the token identifies the user on every request, so the connector needs no further identity.

## Prior plans / specs consulted

- `20261017105035-e3f64c1b-web-mcp-docker` (plan) — hosted mode, `hostedHandler`, one router, `-public-url` as the only address source, hosted-only optional fields with a desktop zero value, `BoardURL` = public root on game-opening results (this plan deliberately changes it to a per-account link), one instance in `.do/app.yaml`, two-transport MCP tests.
- `20261017112555-30c99ab1-memory-store-record` (plan) — memory-only manager; the record on `Game`; the last deck in Manager memory; `get_record`; `list_games`/`load_game` removed; `board` on `new_game`, `import_log` and `set_opponent_deck`. One manager per account makes these per account with no further change.
- `20261017123240-21493a95-google-login` (plan + context) — `hostauth.Account{ID, Email}`, `AccountFrom(ctx)` for the board, `AccountFromToken` for MCP, `RequireSession` (which cancels websocket contexts on session end), `Limit` hook on `webui.Server`, `BoardLimit`, `MCPMiddleware`, the cap at the Google callback with `ErrBusy` and a busy page, places freed when an account's last sign-in ends (live sign-ins = sessions + refresh tokens), `-max-users`/`TCGREPLAY_MAX_USERS` default 10, `/auth/session` → `{"email"}`, `hostauthtest` (fake Google, `MCPTransport`, `BoardCookie`, `SetAccount`), and an injectable clock. Its out-of-scope list hands "idle game clean-up" to this spec.
- Spec `20261017143118-b05aa6b1-per-user-games` — the requirements, constraints and acceptance criteria this plan implements. No design references and no interview notes.

## Open assumptions

- The three upstream plans are implemented as written (function names `hostedHandler`, `newHosted`, `hostauth.Config`, `Places` insertion points in `serveGoogleCallback` and the store's live-sign-in counting). If the landed code differs materially, for example if google-login's cap is not at the callback or live sign-ins are not counted, the implementer adapts the hook points without changing behaviour. If a behaviour this plan relies on is missing, STOP and ask.
- Verified, not assumed: the Streamable HTTP handler copies `auth.TokenInfoFromContext(req.Context())` into each request's `RequestExtra.TokenInfo` (`go-sdk@v1.6.0/mcp/streamable.go:1142-1164`), so tool handlers behind `RequireBearerToken` see the caller's token.
- Google `sub` is stable per account, so a board handle derived from it is stable across restarts.
- A game, together with its board server and hub, is small enough (tens to hundreds of KB) that `MaxUsers` (default 10) games bound memory. To be confirmed by the week-long memory check.

## Drafting assumptions

### Chosen direction: a per-account registry of managers and board servers (architecture)
- **Decision**: Add a new hosted-only package `internal/usergames` whose registry holds, per Google account, a `game.Manager`, a `webui.Server`, the last-activity time and whether the account holds a place. The MCP tool server resolves the caller's entry on every tool call through an optional `Resolve` hook. The hosted board routes go to the signed-in account's own `webui.Server`. Idle entries lose their game and their place but keep their manager and board server. google-login's user cap moves into the registry, and `hostauth` reaches it through a `Places` interface.
- **Key design decisions**: Resolution happens per call, not per session. Live updates are scoped by having one board server per account, with no change to the hub. The board link names the owner, not the game. Expired entries are kept and purged a day after they lost their place. The cap has one owner, the registry. The desktop is untouched because every new hook is nil there.
- **Rationale**: It keeps `internal/game` and `internal/webui` unaware of accounts, reuses `webui.New` and every tool body unchanged except for how they reach the manager, and meets the "expired game frees a place" requirement with one limit mechanism.
- **Rejected**: Option B, one `*mcp.Server` per account through `getServer` (Medium effort; sessions are pinned to a stale manager after an idle drop, and the tool schemas are duplicated per account). Option C, a multi-tenant `game.Manager` and hub with context-aware `Current(ctx)` (High effort; it changes every manager and hub path the desktop uses, which puts "desktop unaffected" at risk).

### Cap counting moves from hostauth into the registry (architecture)
- **Decision**: `hostauth.Config.MaxUsers` and hostauth's own place counting are replaced by `hostauth.Config.Places` (`Admit(Account) error`, `Release(id)`), implemented by the registry, which owns `MaxUsers`. The setting, its default (10), `0` = off, `ErrBusy` and the busy page are unchanged.
- **Rationale**: The spec requires an expired game to free a place, and forbids a second limit mechanism. A place now follows activity, so the structure that tracks activity has to own the count.
- **Rejected**: Keeping the counter in hostauth with exported Admit/Release, which needs a sign-in hook as well and leaves two structures tracking places.

### Idle timeout default 60 minutes; 0 disables; Go duration syntax (architecture)
- **Decision**: `-idle-timeout` / `TCGREPLAY_IDLE_TIMEOUT`, parsed with `time.ParseDuration`, default `60m`, `0` = never drop. An invalid value stops start-up with a clear message.
- **Rationale**: A review session rarely goes an hour without a tool call or a board move, and memory is bounded mainly by the user cap (10 games). The acceptance checks set it to `2m`.
- **Rejected**: A shorter default (it would drop games mid-review) and no default (the operator should not have to set it).

### Expired entries kept; placeless entries purged after 24 hours (architecture)
- **Decision**: On expiry the game is dropped (closed) and the manager records the expiry notice. The entry stays without a place, and is removed after 24 hours with no activity or when the account's last sign-in ends. Removal closes its open tabs, which reconnect to a fresh entry.
- **Rationale**: Open tabs keep their hub, so they show the expiry and follow the next import. A leftover entry costs an empty manager and an idle hub goroutine, and the purge bounds those over a week of different accounts.
- **Rejected**: Deleting on expiry with tombstones (tabs lose their hub, and the notice needs separate storage) and never purging (unbounded over many distinct accounts).

### Expiry is reported by actions that need the game; import and new game just proceed (architecture)
- **Decision**: After expiry, every tool or board verb that needs the current game returns "your game expired after N minutes without activity; import its saved game record again". `import_log` and `new_game` proceed normally and clear the notice. The board's landing shows the notice.
- **Rationale**: This meets "the account's next action says the game expired and asks for the record" without refusing the very import it asks for.
- **Rejected**: Refusing the first action of any kind after expiry, which would refuse the import itself.

### Own name: game's side name, else the signed-in account's name (architecture)
- **Decision**: The board keeps using the game's name for the player's side (an import's `me`, now per account). When the game has none (for example `new_game` without names), the hosted board shows the part of the signed-in email before the `@` instead of "You".
- **Rationale**: Per-account games stop one account's import name reaching another account's board, and the fallback gives every hosted board its owner's name. This needs no new Google scope: google-login asks only for `openid email`.
- **Rejected**: Adding the Google `profile` scope for the display name (it changes google-login's scopes and the consent-screen setup for a small cosmetic gain).
### Resolve the caller's game per tool call, not per MCP session (discovery)
- **Decision**: One shared `*mcp.Server`. Each tool call resolves the caller's manager from its token through a hosted-only hook on `mcpserver.Server`.
- **Rationale**: The go-sdk pins an MCP session to its server instance at initialize. Per-call resolution survives idle drops and keeps a single tool registry.
- **Rejected**: One `*mcp.Server` per account through `getServer`, because live sessions would be pinned to stale managers.

### Activity is tool calls, board verbs and board page loads only (discovery)
- **Decision**: Any MCP `tools/call` (reads included), any board verb, and a `GET /` page load count. Other MCP methods, websocket (re)connects and `/api/card` lookups do not.
- **Rationale**: The spec says "any action through Claude, any move on the board, or opening the board; a board tab simply left open does not count". The board client reconnects by itself, so counting connects would let an open tab keep a game.
- **Rejected**: Counting every authenticated request, which is how google-login's rate limiter counts.

### Board link identifies the owner: `<public>/?board=<handle>` (discovery)
- **Decision**: The `board` key becomes the public URL plus `?board=` and a stable handle derived from the Google account id (SHA-256, domain-separated, truncated, base64url). `/` without the parameter opens the signed-in account's own board.
- **Rationale**: All of an account's tabs follow its one game across imports, B opening A's link can be told "not your game", and the handle is stable across restarts and reveals nothing usable.
- **Rejected**: The root URL (B would see their own game) and the game id (it changes on every import).

### Tool-call lookup errors come back as tool errors (tasks)
- **Decision**: If finding the caller's game fails (server busy, no account), the tool returns an error result carrying the message rather than a protocol error.
- **Rationale**: This matches google-login's choice for rate limits: Claude can tell the user, instead of claude.ai showing a generic connector failure.
- **Rejected**: Protocol-level errors, which claude.ai shows generically.

### A human task applies the updated app definition (tasks)
- **Decision**: Add a `human` task for the owner to run `doctl apps update` after the app definition gains the idle setting.
- **Rationale**: Pushes do not re-apply the App Platform app spec (google-login's migration note), and changing the idle time on the deployed app for the manual checks needs the setting to be present.
- **Rejected**: Relying on the binary's default alone, which leaves the deployed definition out of step with the repo.

### Two milestones; the cap move lands with the idle drop (tasks)
- **Decision**: Milestone 1 delivers isolation with google-login's cap unchanged. Milestone 2 adds expiry, the idle sweep and moves the cap into the registry.
- **Rationale**: Isolation is the epic's main risk and is useful on its own. The cap move only matters once games can expire.
- **Rejected**: A single milestone (too large to validate in one step) and three milestones (expiry without freeing places is not worth shipping separately).

## Rehydration cues

- `spektacular plan file read 20261017123240-21493a95-google-login plan` and `… context` (hostauth API, cap, hooks); `… 20261017105035-e3f64c1b-web-mcp-docker plan` (hosted wiring); `… 20261017112555-30c99ab1-memory-store-record plan` (manager after storage removal).
- `spektacular spec file read 20261017143118-b05aa6b1-per-user-games`.
- Re-read `internal/mcpserver/server.go:17-91`, `internal/webui/server.go:28-71`, `internal/webui/hub.go`, `internal/game/manager.go`, `web/src/App.svelte:26-29`, `web/src/lib/ws.js`.
- `grep -n "s\.M\b\|s\.M\." internal/mcpserver/server.go internal/webui/*.go` lists every manager access to convert.
