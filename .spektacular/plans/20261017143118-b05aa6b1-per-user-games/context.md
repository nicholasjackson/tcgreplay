---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Context: 20261017143118-b05aa6b1-per-user-games

## Current State Analysis

- **Upstream plans are planned, not built.** At commit 77e6a6a the binary has the shim, `-daemon` and `-no-daemon` modes only. web-mcp-docker adds `-hosted` (`cmd/tcgreplay/hosted.go`: `runHosted`, `newHosted`, `hostedHandler`; `webui.Server.Public`; `mcpserver.Server.BoardURL`; `.do/app.yaml` with one instance). memory-store-record makes the manager memory-only (record on `Game`, last deck in memory, no `List`/`Load`/`Resume`, no `list_games`/`load_game`). google-login adds `internal/hostauth` (`Account`, `AccountFrom`, `AccountFromToken`, `RequireToken`, `RequireSession`, `BoardLimit`, `MCPMiddleware`, cap at the Google callback, `ErrBusy`, busy page, `hostauthtest`) and `webui.Server.Limit`. This plan's tasks start from the code those plans produce.
- **One game for everyone.** `game.Manager` holds a single `current` game, `onLoad` hooks and the assistant label (`internal/game/manager.go:22-112`). The MCP tool server reads it through `s.M` in every tool (`internal/mcpserver/server.go:121, 316-744`), and every tool is registered through `add` (`internal/mcpserver/server.go:82-91`). The board server wraps the same manager, with one hub that pushes to every connected client (`internal/webui/server.go:28-45`, `internal/webui/hub.go:21-110`).
- **Seat labels.** The board names a side from the game (`app.board.sides[seat].name`), else "You" for the human seat (`web/src/App.svelte:26-29`). The landing text when there is no game is at `web/src/App.svelte:74-82`.
- **The board client sends no verb by itself.** Every verb is user-triggered (`web/src/lib/actions.js`, `web/src/App.svelte:18-24`, `web/src/lib/playback.svelte.js:74`); the websocket reconnects with backoff (`web/src/lib/ws.js:17-23`).
- **go-sdk v1.6.0** copies the bearer token's `TokenInfo` into every request's `RequestExtra` on Streamable HTTP (`mcp/streamable.go:1142-1164`; `mcp/shared.go:477-490`).
- **Knowledge and designs.** The `tcgreplay` knowledge store is empty. The spec references no design documents and has no interview notes.

**Requirement-to-repo resolution**: every requirement lands in the single registered repo `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`). Paths marked as upstream are created by the three upstream plans and have no line numbers yet, so entries name the function instead.

| Requirement | Task(s) | Files |
|---|---|---|
| Games are isolated | Keep one game per account; Route Claude's tool calls to the caller's own game; Turn on per-account games in hosted mode | `internal/usergames/usergames.go`, `internal/usergames/mcp.go`, `internal/mcpserver/server.go`, `cmd/tcgreplay/hosted.go` |
| Shared across an account's conversations, tabs and devices | Keep one game per account; Turn on per-account games in hosted mode | `internal/usergames/usergames.go`, `cmd/tcgreplay/hosted.go` |
| Actions stay with their user | Route Claude's tool calls to the caller's own game | `internal/mcpserver/server.go`, `internal/usergames/mcp.go` |
| Owner-only board | Give each account its own board | `internal/usergames/board.go` |
| Signed-out visitors see nothing | Turn on per-account games in hosted mode (google-login's `RequireSession` in front) | `cmd/tcgreplay/hosted.go` |
| Board shows only its owner's game | Give each account its own board | `internal/usergames/board.go`, `internal/webui/server.go` |
| Own name on the board | Show the player's own name on the hosted board | `web/src/App.svelte` |
| Idle games dropped | Let a game expire with a notice; Drop games left idle | `internal/game/manager.go`, `internal/webui/hub.go`, `web/src/App.svelte`, `internal/usergames/usergames.go`, `cmd/tcgreplay/main.go` |
| Expired games free their place | Free a place when a game expires | `internal/usergames/usergames.go`, `internal/hostauth/hostauth.go`, `internal/hostauth/google.go`, `internal/hostauth/store.go`, `internal/hostauth/limit.go` |
| Desktop unaffected | all (nil hooks); Turn on per-account games in hosted mode | `internal/mcpserver/server.go`, `cmd/tcgreplay/hosted.go` |
| Single instance constraint | Document per-account games and the idle drop; Apply the updated app definition to the deployed app | `.do/app.yaml`, `TCG Review Playmat Design and Build Plan.md` |

## Per-Task Technical Notes

### Task: Keep one game per account

**File changes**:
- `internal/usergames/usergames.go` (new) — package doc: "per-account games for the hosted playmat: one game manager and board server per signed-in Google account, an activity clock, the idle drop and the server's places; all in memory". `Config` and `Entry` as in plan.md's Data Structures (M1 uses `NewManager`, `NewBoard`, `Public`, `Now`; `Idle`/`MaxUsers` are added by the later tasks). `New(cfg) *Registry`: `Now` defaults to `time.Now`. `Registry{mu sync.Mutex; entries map[string]*Entry}` keyed by `hostauth.Account.ID`. `Entry` unexported fields: `acct hostauth.Account`, `last time.Time`, `ctx context.Context`, `cancel context.CancelFunc` (entry lifetime; cancelled on removal by later tasks), `board http.Handler` (cached `UI.Handler()`).
  - `entry(acct, touch bool) *Entry` (internal, under lock): create if absent (`m := cfg.NewManager()`, `ui := cfg.NewBoard(m)`, `ctx, cancel := context.WithCancel(context.Background())`, `ui.StartHub(ctx)` (`internal/webui/server.go:74`), `Handle: Handle(acct.ID)`, `last: now` only if `touch`); if `touch`, `e.last = now`. `Use(acct) (*Entry, error)` = `entry(acct, true)` (error reserved for the cap task; nil here). `Get(acct) *Entry` = `entry(acct, false)`. Never hold `r.mu` while calling into a manager or a board server.
  - `Handle(id string) string` — `base64.RawURLEncoding.EncodeToString(sha256.Sum256([]byte("tcgreplay board\x00" + id))[:12])` (16 characters). Deterministic, so stable across restarts.
  - `(*Entry).Link(public *url.URL) string` — `public` with path `/` and query `board=<Handle>`.
- `internal/usergames/usergames_test.go` (new) — fake `NewManager` (`game.NewManager(t.TempDir(), nil)`) and `NewBoard` (`webui.New(m, state.P1)`, with `Public` set); two accounts → distinct `M`, `UI`, `Handle`; same account → same pointers; `Handle` deterministic across two `New` registries and does not contain the ID; `Use` sets `last` to the injected `Now`, and `Get` leaves it unchanged (expose `(*Entry).LastActive()` for tests or use an internal test).

**Complexity**: Medium
**Token estimate**: ~20k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Route Claude's tool calls to the caller's own game

**File changes**:
- `internal/mcpserver/server.go:17-24` — add `Resolve func(ctx context.Context, extra *mcp.RequestExtra) (*game.Manager, string, error)` to `Server`, with the doc comment from plan.md (beside web-mcp-docker's `BoardURL`).
- `internal/mcpserver/server.go:82-91` (`add`) — make it take the `*Server` (`func add[In any](s *Server, srv *mcp.Server, name, desc string, h ...)`) and update all 46 call sites in `register` (`:308` onward). In the wrapper, use the `*mcp.CallToolRequest` (currently `_`): `ctx, err := s.bind(ctx, req)`; on error return `&mcp.CallToolResult{IsError: true, Content: text(err.Error())}, nil, nil` so Claude sees the message (busy, sign-in).
- `internal/mcpserver/server.go` (new helpers near `verb`, `:120`) — `type callKey struct{}`; `type call struct{ m *game.Manager; board string }`; `bind(ctx, req)`: if `s.Resolve == nil` → `call{s.M, s.BoardURL}`; else `m, board, err := s.Resolve(ctx, req.Extra)`. Then, if `req.Session.InitializeParams()` has a client name, `m.SetAssistant(AssistantName(name))` (cheap no-op when unchanged), and store the call on ctx. `m(ctx) *game.Manager` and `board(ctx) string` read it back (falling back to `s.M`/`s.BoardURL`).
- `internal/mcpserver/server.go:39-43` (`InitializedHandler`) — only when `s.Resolve == nil` (desktop), as today. Hosted labels are set per call above.
- `internal/mcpserver/server.go:121, 316-370, 380-744` — replace every `s.M.` with `s.m(ctx).` (`verb` gains a `ctx` parameter; its callers already have `ctx`). Wherever web-mcp-docker / memory-store-record add `"board": s.BoardURL` to the `new_game`, `import_log` and `set_opponent_deck` results, use `s.board(ctx)` (omit the key when it is empty, as before). `card_text` keeps using the manager's `DB` (shared).
- `internal/usergames/mcp.go` (new) — `(*Registry).ResolveMCP(ctx, extra)`: `acct, ok := hostauth.AccountFromToken(extra.TokenInfo)` (nil `extra` or no token → error `sign in required`); `e, err := r.Use(acct)`; return `e.M, e.Link(r.cfg.Public), err`.
- `internal/mcpserver/server_test.go` — new `TestResolvePerCall`: a `Server` with `M: nil` and a `Resolve` that returns manager A or B depending on a value the test sets between calls (closure variable), plus link `https://x/?board=A`/`B`. `new_game` on A, then `get_state` on B → "no game"; `import_log`/`new_game` result `board` equals the resolver's link. A resolver error comes back as `IsError` with its message. Existing tests (desktop, `Resolve` nil) unchanged, including web-mcp-docker's two-transport runs.
- `internal/usergames/mcp_test.go` (new) — `ResolveMCP` with `&mcp.RequestExtra{TokenInfo: &auth.TokenInfo{UserID: "a", Extra: {"email": ...}}}` returns A's manager and link, touches A; nil `TokenInfo` → error.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential execution (the `s.M` → `s.m(ctx)` sweep is mechanical; the compiler finds every site).

### Task: Give each account its own board

**File changes**:
- `internal/usergames/board.go` (new) — `(*Registry).Board() http.Handler`, mounted behind `hostauth.RequireSession` (which puts the account on the context and cancels websocket contexts on sign-out):
  - `acct, ok := hostauth.AccountFrom(r.Context())`; if not ok → 401 (defensive; `RequireSession` already refuses).
  - **Page load** (`r.Method == GET|HEAD` and `r.URL.Path == "/"` or `"/index.html"`): if `q := r.URL.Query().Get("board"); q != "" && q != Handle(acct.ID)` → `notYoursPage(w, acct)` with status 403 and `Cache-Control: no-store`, and nothing else is served. Otherwise `e, err := r.Use(acct)` (activity). On `errors.Is(err, hostauth.ErrBusy)` → 503 with the busy message (the cap task makes this reachable). Then serve `e.board`.
  - **Live connection** (`/ws`): `e := r.Get(acct)` (no activity); `ctx, stop := mergeDone(r.Context(), e.ctx)` (a goroutine cancels the request context when the entry's context ends, as google-login's `RequireSession` does for sign-out); serve `e.board` with `r.WithContext(ctx)`.
  - **Everything else** (`/assets/*`, `/api/card`): `r.Get(acct)` and serve `e.board` (card lookups are not activity; they are still rate-limited by google-login's `RequireSession`).
  - `notYoursPage` — minimal `html/template` page: "Not your game. This board belongs to another account. You are signed in as {{email}}." with a `POST /auth/logout` Sign out button (google-login's route) and a link to `/` ("Open your own board"). No board app, no game data.
- `internal/usergames/board.go` — `(*Registry).BoardLimit(next func(ctx context.Context) error) func(ctx context.Context) error`: returns the hook to set as each per-account `webui.Server.Limit`: run `next` (google-login's `hostauth.BoardLimit`) and, if it passes, `r.Use(acct)` for `hostauth.AccountFrom(ctx)`, so every board verb counts as activity (`internal/webui/actions.go:83` `dispatch` calls `Limit` with the websocket's request context, `internal/webui/hub.go:213`).
- `internal/usergames/board_test.go` (new) — serve `Board()` behind a tiny test middleware that puts a chosen `hostauth.Account` on the context (no Google needed at this level): A `GET /` → board page (embedded app or placeholder) and A touched; B `GET /?board=<Handle(A)>` → 403, body contains "Not your game", body has no `<script` of the app; A `GET /?board=<Handle(A)>` → 200; A's websocket receives a state for A's game (`new_game` on `A.M`), and after a move on `B.M` A's socket receives nothing within 500 ms, while a move on `A.M` arrives within 2 s; dialling `/ws` does not touch A's activity; a board verb through the `BoardLimit` hook touches A; cancelling the entry context closes A's socket.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Turn on per-account games in hosted mode

**File changes**:
- `cmd/tcgreplay/hosted.go` (web-mcp-docker + google-login, `runHosted` / `newHosted`) — build the registry after the public URL and `hostauth.Auth`:
  - `NewManager: func() *game.Manager { return game.NewManager(c.dataDir, db) }` (the one shared card DB; the data dir holds only the assistant label after memory-store-record);
  - `NewBoard: func(m *game.Manager) *webui.Server { ui := webui.New(m, seat); ui.Public = public; ui.Limit = reg.BoardLimit(a.BoardLimit); return ui }`;
  - `Public: public`.
  The hosted `mcpserver.Server` gets `M: nil`, `BoardURL: public.String()` (kept for its instructions line) and `Resolve: reg.ResolveMCP`. Remove the single hosted `webui.Server` and its `ui.Limit` wiring; the hub-start call for it goes away (each entry starts its own).
- `cmd/tcgreplay/hosted.go` (`hostedHandler`) — signature becomes `hostedHandler(srv *mcp.Server, reg *usergames.Registry, a *hostauth.Auth) http.Handler`; `mux.Handle("/", a.RequireSession(reg.Board()))` replaces `a.RequireSession(ui.Handler())`. `/mcp`, the sign-in routes and `/healthz` unchanged.
- `cmd/tcgreplay/hosted_test.go` (upstream) — adapt the helper that builds the handler to build the registry. Add two-account tests using `hostauthtest.NewGoogle`, `g.SetAccount`, `hostauthtest.MCPTransport` and `hostauthtest.BoardCookie`:
  - `TestHostedIsolation`: A and B import different corpus logs (`testdata/logs`); A's `get_state` and board show A's game, B's show B's; a `move` by A leaves B's `get_state` unchanged.
  - `TestHostedSameAccountSessions`: two MCP sessions for A; a move in one is visible to the other's next `get_state` and arrives on A's board socket; an `import_log` in one replaces the game both sessions and the board see.
  - `TestHostedBoardLinkOwnerOnly`: the `board` key from A's `import_log` is `<ts.URL>/?board=<handle>`; fetching it with B's cookie → 403 "Not your game" and no game JSON; with no cookie → 302 to `/auth/login?next=...` and no game data.
  - `TestHostedLiveUpdatesScoped`: A's board socket gets A's move within 2 s, and nothing within 500 ms of B's move.
  - Existing upstream tests (tool parity, board link starts with the public URL, Claude move reaches the board in 2 s, board move reaches Claude, sign-in, rate limit, cap) pass; update their board-link assertions to the per-account form.
- `internal/mcpserver/server.go` instructions — unchanged (the hosted line about handing over the `board` link still applies).

**Complexity**: High
**Token estimate**: ~45k tokens
**Agent strategy**: Parallel analysis, sequential integration — one agent wires `hosted.go`, then the two-account tests are written against it; a second agent may draft the test helpers in parallel from `hostauthtest`.

### Task: Show the player's own name on the hosted board

**File changes**:
- `web/src/App.svelte:26-29` (`nameOf`) — `return n || (seat === app.human ? (ownName() || 'You') : app.assistant);` where `ownName()` returns `app.account ? app.account.split('@')[0] : ''` (`app.account` is set from `/auth/session` by google-login's `web/src/lib/session.js`; `null` on the desktop, so the desktop label stays "You").
- `internal/webui/dist/app` — rebuilt by `make build`, not edited by hand.

**Complexity**: Low
**Token estimate**: ~5k tokens
**Agent strategy**: Single agent, sequential execution; check by building and loading a hosted board locally with a `new_game` that names no players.

### Task: Let a game expire with a notice

**File changes**:
- `internal/game/manager.go:22-33` — add `notice string` and `onClear []func()` to `Manager`.
- `internal/game/manager.go` (new methods beside `OnLoad`, `:80-85`) — `var ErrExpired = errors.New("game expired")`. `Expire(notice string)`: under lock, close `m.current` if set, set it to nil, set `m.notice = notice`, clone `onClear`; unlock; run the hooks. `Notice() string` (under lock). `OnClear(f func())`.
- `internal/game/manager.go:87-95` (`Current`) — when `m.current == nil && m.notice != ""`, return `fmt.Errorf("%w: %s", ErrExpired, m.notice)`, so the text Claude and the board see is the notice. Otherwise the existing "no game" error (as memory-store-record words it).
- `internal/game/manager.go:97-112` (`setCurrent`) — clear `m.notice` when a game becomes current (covers `New`, `Import`, `SetOpponentDeck`).
- `internal/webui/server.go:36-45` (`New`) — `m.OnClear(s.hub.wake)` beside `m.OnLoad(s.hub.watch)`.
- `internal/webui/hub.go:115-128` (`stateMessage`, no-game branch) — add `msg["notice"] = h.s.M.Notice()` when non-empty (memory-store-record already removes `games` here).
- `web/src/lib/store.svelte.js` — add `notice: ''` to `app`. `web/src/lib/ws.js` `onState` — `app.notice = msg.notice || ''`.
- `web/src/App.svelte:74-82` (landing when no game) — when `app.notice` is set, show it as the landing text (a paragraph with the notice) in place of the default "start or import" text.
- `internal/game/game_test.go` or a new `internal/game/manager_test.go` — `Expire` on a manager with a game: `Current` errors with `ErrExpired` and the notice text; the old game is closed; `OnClear` hook ran; a `New` afterwards clears the notice; `Expire` with no game sets the notice; a fresh manager's `Notice()` is "".
- `internal/webui/webui_test.go` — `TestExpiryNotice`: with a socket open on `setup(t)`'s board, `m.Expire("x expired")` pushes a state with `snapshot: null` and `notice: "x expired"`; a board verb then returns the notice as its error; a new connection's first state carries the notice.
- `internal/webui/dist/app` — rebuilt by `make build`.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Drop games left idle

**File changes**:
- `internal/usergames/usergames.go` — `Config.Idle time.Duration`. `Entry` gains `holds bool` (set true on creation by `Use`; the cap task gives it its full meaning) and `game bool` tracking is read from the manager (`e.M.Current()` error). New internal `expireIfIdle(e, now) bool` (under lock; returns whether hooks must run): if `cfg.Idle > 0 && e.holds && now.Sub(e.last) > cfg.Idle` → `e.holds = false`, and if the manager has a current game, mark it to `e.M.Expire(notice)` outside the lock. Notice text: `fmt.Sprintf("Your game expired after %s without activity and was cleared. Import its saved game record again (import_log with the record) to carry on.", humanDuration(cfg.Idle))` where `humanDuration` renders `2m0s` as "2 minutes" and `1h0m0s` as "60 minutes" (minutes for anything under 2 hours).
  - `Use` calls `expireIfIdle` before touching, so a stale entry's game is expired before the action, and the action then sees the notice. `Get` calls it too (an open board learns of the expiry) but never touches.
  - `Run(ctx)`: ticker every `min(cfg.Idle/4, 30s)` (at least 1 s; no ticker when `Idle == 0`) calling `sweep(now)`: for each entry `expireIfIdle`; remove entries with `!e.holds && now.Sub(e.last) > 24h` (`purgeAfter` constant) — removal = delete from map, `e.cancel()` (closes its hub and, through `Board()`'s context merge, its open sockets). Run the managers' `Expire` calls after releasing the lock.
- `internal/usergames/usergames_test.go` — with `Idle: 2*time.Minute` and an injected clock: `Use`, advance 2m1s, `Use` → `M.Current()` errors with `game.ErrExpired` and a notice naming "2 minutes"; `Use` every 1m for 5m keeps the game and its moves (head unchanged); `Get` calls every 30 s do not keep it; `sweep` expires an untouched entry and pushes the notice to an open board socket; an entry without a place idle for 24h+ is removed and its socket closed; `Idle: 0` never expires.
- `internal/usergames/usergames.go` (logging) — `log.Printf("usergames: game for %s expired after %s idle", email, idle)` on expiry, `usergames: removed idle account %s` on purge, and one line per sweep that changed anything: `usergames: %d accounts, %d holding places, %d games`. Never log tokens or board handles.
- `cmd/tcgreplay/main.go:35-39, 54-65` (`config`, flags; google-login adds `rateLimit`, `maxUsers`) — add `idleTimeout time.Duration` and flag `-idle-timeout` with default from `envDuration("TCGREPLAY_IDLE_TIMEOUT", 60*time.Minute)`. `envDuration` (beside google-login's `envInt` in `cmd/tcgreplay/hosted.go`): an empty value → default; `"0"` → 0; otherwise `time.ParseDuration`, and an invalid value is returned as an error that `runHosted` reports at start-up (`hosted mode: TCGREPLAY_IDLE_TIMEOUT: invalid duration "x"`). A negative value is also an error.
- `cmd/tcgreplay/hosted.go` (`runHosted` / `newHosted`) — pass `Idle: c.idleTimeout` and run `go reg.Run(ctx)` for the life of the server. Log the idle setting at start-up.
- `cmd/tcgreplay/hosted_test.go` — build the hosted handler with an injected registry clock (the test helper exposes it) and `Idle: 2*time.Minute`: `TestHostedIdleDrop` (A imports; advance 2m1s; A's next `get_state` is `IsError` with "expired" and "record"; A's next board load gives a state with `notice`); `TestHostedOpenTabDoesNotKeepGame` (A's socket stays open while the clock advances 2m1s with sweeps; then the game is gone); `TestHostedActivityKeepsGame` (a `get_state` every simulated minute for 5 minutes; the moves are intact). Settings tests for `envDuration` (default 60m, "0", "2m", invalid, negative).

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Free a place when a game expires

**File changes**:
- `internal/hostauth/hostauth.go` (google-login, `Config`) — remove `MaxUsers`; add `Places Places` and the `Places` interface (plan.md Data Structures) with doc comments. nil = no cap.
- `internal/hostauth/limit.go` (google-login) — remove `admit` and the distinct-account counting it used. Keep `ErrBusy` and its message ("the server is busy: it already has the most signed-in users it serves at once (N); try again later") but have the message built by the places provider (export `BusyError(max int) error` returning an error wrapping `ErrBusy`, so the wording stays in one place).
- `internal/hostauth/google.go` (google-login, `serveGoogleCallback`) — where `admit(acct)` was called (after reading the ID token, before `signIn` and the connector continuation): `if a.cfg.Places != nil { if err := a.cfg.Places.Admit(acct); err != nil { if errors.Is(err, ErrBusy) { busyPage(w, 503); return }; errorPage(...) } }`.
- `internal/hostauth/store.go` (google-login, live sign-in counting) — keep the per-account count of live sign-ins (sessions + refresh tokens). At the point where the count reaches zero and the account entry is removed, call `Places.Release(id)` after unlocking the store.
- `internal/usergames/usergames.go` — `Config.MaxUsers int`. `holds` now means "holds a place". `countHeld()` under lock. `take(e) error`: if `!e.holds`, then `if MaxUsers > 0 && countHeld() >= MaxUsers → hostauth.BusyError(MaxUsers)`, else `e.holds = true`. `Admit(acct)`: `sweep(now)` first (so places freed by expiry are counted as free), then get or create the entry, `take`, and touch. `Use(acct)`: after `expireIfIdle`, `take` (returning the busy error to the caller) and touch. `Release(id)`: remove the entry (as the purge does: delete, cancel). Idle expiry already sets `holds = false`; for an entry that holds a place but has no game, expiry simply frees the place with no notice.
- `internal/usergames/usergames.go` — compile-time check `var _ hostauth.Places = (*Registry)(nil)`.
- `cmd/tcgreplay/hosted.go` (`hostedAuth` / `newHosted`) — build the registry before `hostauth.New` and pass `Places: reg`; `MaxUsers: c.maxUsers` moves from `hostauth.Config` to `usergames.Config`. `newHosted` must construct both: build the registry first (its `NewBoard` closure reads `a` lazily, or set `ui.Limit` through a variable assigned after `hostauth.New`).
- `internal/hostauth/limit_test.go` (google-login) — the cap tests move: keep a hostauth-level test with a fake `Places` (records Admit and Release calls; returns `BusyError` for a chosen account) proving the callback shows the busy page in both flows, and that `Release` is called when an account's last sign-in ends (board logout with no refresh tokens) and not before.
- `internal/usergames/usergames_test.go` — `MaxUsers: 1`: `Admit(A)` ok; `Admit(B)` busy; A's game expires (clock + sweep) → `Admit(B)` ok; A's next `Use` → busy while B holds the place; `Release(B)` → A's `Use` ok; an account that signs in (`Admit`) and never acts loses its place after `Idle`; `Admit` for an account already holding a place succeeds at the cap; `MaxUsers: 0` never refuses.
- `cmd/tcgreplay/hosted_test.go` — `TestHostedExpiredGameFreesPlace`: `MaxUsers: 1`, `Idle: 2m`; A signs in and imports; C's sign-in (fake Google `SetAccount`) gets 503 "busy"; advance 2m1s; C signs in, `import_log` succeeds. The upstream user-cap test still passes (same message, both flows).

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential execution (`hostauth` change first, then the registry, then wiring and tests).

### Task: Document per-account games and the idle drop

**File changes**:
- `TCG Review Playmat Design and Build Plan.md:299` (the Runtime paragraph, as extended by web-mcp-docker and google-login) — add a "Per-account games" paragraph covering:
  - one game per signed-in Google account, in memory, shared by all of that account's conversations, tabs and devices; an import replaces it for all of them;
  - board links of the form `/?board=<handle>`, owner-only ("not your game" for other accounts, sign-in first for visitors), and `/` opening one's own board;
  - the idle drop: what counts as activity (tool calls, board moves, board page loads; not an open tab), the expiry message, and restoring by importing the record;
  - `TCGREPLAY_IDLE_TIMEOUT` / `-idle-timeout` (default 60 minutes, `0` = never);
  - places under `TCGREPLAY_MAX_USERS`: taken at sign-in or on the first action after losing one, freed by idle expiry, the last sign-in ending, or a restart;
  - why the app must run as one instance;
  - the desktop keeping one game with no accounts.
- `.do/app.yaml` (web-mcp-docker / google-login) — add env `TCGREPLAY_IDLE_TIMEOUT: "60m"` (`scope: RUN_TIME`), and a comment on `instance_count: 1`: "must stay 1: each account's game lives in this instance's memory".
- `cmd/tcgreplay/main.go:1-11` (package comment) — one sentence: hosted mode keeps one game per signed-in account and drops idle games.
- `internal/usergames/usergames.go` — check the package doc matches the final behaviour.

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Apply the updated app definition to the deployed app

**What the person does**:
1. After the documentation task's change to `.do/app.yaml` is on `main`, run `doctl apps update <app-id> --spec .do/app.yaml` (a push does not re-apply the app spec). Re-enter the Google secret values in the App Platform control panel if the update cleared them, as in google-login's setup.
2. In the App Platform control panel, check that the `playmat` component lists `TCGREPLAY_IDLE_TIMEOUT` and that the instance count is 1.

**How it is checked**: The app redeploys and its health check passes. The start-up log line shows the idle setting. Changing `TCGREPLAY_IDLE_TIMEOUT` to `2m` in the control panel and letting the app restart changes the logged value with no rebuild; set it back to the chosen value afterwards. These values are also the setup for the manual idle checks in the test plan.

## Testing Strategy

- **Keep one game per account**: `internal/usergames/usergames_test.go`. Distinct managers, boards and handles per account; the same entry for the same account; a deterministic handle that is stable across registries and opaque; `Use` touches the activity time and `Get` does not.
- **Route Claude's tool calls to the caller's own game**: `internal/mcpserver/server_test.go` `TestResolvePerCall`. Calls act on the resolver's manager, `board` comes from the resolver, and a resolver error becomes a tool error. `internal/usergames/mcp_test.go` covers `ResolveMCP` with and without a token. The existing desktop tool tests, over both transports, stay unchanged.
- **Give each account its own board**: `internal/usergames/board_test.go`. Own board served; a foreign handle gets 403 "Not your game" with no app; a websocket receives only its own game's updates; a websocket connect is not activity; a board verb through the limit wrapper is; an entry's end closes its sockets.
- **Turn on per-account games in hosted mode**: `cmd/tcgreplay/hosted_test.go` with fake Google and two accounts. Isolation; same-account sessions; import replaces for the account; owner-only and signed-out links; scoped live updates within 2 s. The upstream hosted tests still pass.
- **Show the player's own name on the hosted board**: no front-end unit tests exist in the repo; checked by hand (manual review "Own name"). The server half, each account's state naming its own side, is covered in the hosted isolation test.
- **Let a game expire with a notice**: `internal/game` manager tests (`ErrExpired`, the notice text, the game closed, `OnClear` runs, a new game clears the notice) and `internal/webui/webui_test.go` `TestExpiryNotice` (pushed state carries `notice`; a verb error carries it; a new connection sees it).
- **Drop games left idle**: `internal/usergames/usergames_test.go` with an injected clock and a 2-minute idle time (expiry on lookup and on sweep, 5 minutes of activity keeps the game, `Get` never keeps it, 24-hour purge, `0` disables). `cmd/tcgreplay/hosted_test.go` covers `TestHostedIdleDrop`, `TestHostedOpenTabDoesNotKeepGame`, `TestHostedActivityKeepsGame` and the `envDuration` settings.
- **Free a place when a game expires**: `internal/hostauth/limit_test.go` with a fake `Places` (busy page in both flows; `Release` on the last sign-in ending). `internal/usergames/usergames_test.go` tests the cap at 1 (expiry frees a place, re-admission on the next action, an idle account that never acted, the already-holding account at the cap, `0` disables). `cmd/tcgreplay/hosted_test.go` has `TestHostedExpiredGameFreesPlace`.
- **Document per-account games and the idle drop**: no tests; reviewed with the manual checks.
- **Apply the updated app definition to the deployed app**: checked by the person as described in its entry.
- **Success metrics and manual reviews**: as listed in plan.md's Testing Approach. Every "Manual — captured in the implementation test plan" item goes into the implementation test plan: two real accounts through claude.ai, the same account on two devices, owner-only links in a browser, the idle drop at 2 minutes on the deployed app, an expired game freeing a place at cap 1, own name, desktop regression, and a week of memory metrics.

## Project References

- Repo `tcgreplay`, root `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (the only registered repo).
- Spec `20261017143118-b05aa6b1-per-user-games` (this plan); epic `20261017105033-91f90842-hosted-playmat` (this is its last spec).
- Plans that must be implemented first: `20261017105035-e3f64c1b-web-mcp-docker` (hosted mode, public URL, `BoardURL`, `.do/app.yaml`), `20261017112555-30c99ab1-memory-store-record` (memory-only manager, record), `20261017123240-21493a95-google-login` (`hostauth`, limits, `hostauthtest`).
- Design documents: none. Knowledge entries: none (the store is empty).
- External: go-sdk v1.6.0 `mcp` (`RequestExtra.TokenInfo`, Streamable HTTP session binding), `auth.TokenInfo`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Per task: Keep one game per account — Medium (~20k); Route Claude's tool calls to the caller's own game — Medium (~30k); Give each account its own board — Medium (~30k); Turn on per-account games in hosted mode — High (~45k); Show the player's own name on the hosted board — Low (~5k); Let a game expire with a notice — Low (~15k); Drop games left idle — Medium (~35k); Free a place when a game expires — Medium (~35k); Document per-account games and the idle drop — Low (~8k). The human task needs no agent budget. "Route…" and "Give each account its own board" can run in parallel after "Keep one game per account"; "Show the player's own name" and "Let a game expire with a notice" are independent and can run at any time.

## Migration Notes

- **google-login's cap moves.** `hostauth.Config.MaxUsers` is removed and replaced by `hostauth.Config.Places`. The `-max-users` / `TCGREPLAY_MAX_USERS` setting is unchanged for operators, but now feeds the registry. google-login's hostauth cap tests are rewritten against a fake `Places`, and the cap's behaviour tests move to the registry and the hosted tests.
- **The hosted `board` link changes form**, from `<public>/` to `<public>/?board=<handle>`. Links already handed out before deploy still open the signed-in account's own board, because `/` with no handle serves one's own board.
- **The deployed app needs the updated app definition applied** (`doctl apps update`) for the idle setting to appear. Until then the binary's 60-minute default applies.
- **No data migration**: nothing is persisted, and a deploy clears every account's game, as before.

## Performance Considerations

- Each tool call and board action adds one map lookup under the registry's mutex. Managers and board servers are never called while that mutex is held.
- Memory is bounded by places: at most `MaxUsers` (default 10) accounts hold a game at once. Accounts without a place keep only an empty manager and an idle hub goroutine, and are purged after a day. The idle sweep runs every `min(idle/4, 30s)` and only walks the entry map.
- Live updates fan out per account: a move wakes only its owner's hub, so other accounts' boards cost nothing.
- Per-account board servers each run one hub goroutine. With the cap at 10 and the purge, that is tens of goroutines at most.
