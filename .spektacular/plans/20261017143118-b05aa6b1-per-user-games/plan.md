---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Plan: 20261017143118-b05aa6b1-per-user-games

<!-- Metadata -->
<!-- Created: 2026-10-17T15:21:39Z -->
<!-- Commit: 77e6a6a -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

Gives every Google account signed in to the hosted playmat its own game, so several people can share one server without seeing or changing each other's boards. All of an account's claude.ai conversations, board tabs and devices share that one game, and board links open only for their owner: anyone else sees "not your game", and a signed-out visitor is asked to sign in first. Games left idle for longer than an operator-set time are cleared and free their place under the signed-in user limit, so the owner's hosted trial can keep serving everyone in bounded memory. The desktop keeps its single game with no accounts.

## Conventions

No project conventions apply to this feature: the `tcgreplay` knowledge store holds no entries (the always-applied load for the repo returned nothing). The plan follows the patterns already in the code and in the three upstream plans instead: the repo's vet-and-test Makefile target, standard-library `testing` with `httptest` beside the code, doc comments on every exported identifier, hosted-only behaviour switched on by optional fields whose zero value leaves the desktop unchanged (web-mcp-docker), the public URL as the only source of addresses (web-mcp-docker), and an injectable clock for time-based tests (google-login).

## Architecture & Design Decisions

All work lands in the single registered repo, `tcgreplay` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`), on top of the three upstream plans: web-mcp-docker's `tcgreplay -hosted` (one router serving `/mcp` and the board, `-public-url` as the only source of addresses, hosted-only optional fields), memory-store-record's memory-only game manager (the record on `Game`, the last decklist in the manager), and google-login's signed-in account on every request (`hostauth.AccountFrom` for the board, `AccountFromToken` for `/mcp`) with its request limit and user cap. As the spec's Technical Approach prefers, **the single shared current game becomes a lookup by signed-in account, used by both the connector and the board's live connection**. A new hosted-only package, `internal/usergames`, holds a registry with one entry per Google account. Each entry holds that account's own `game.Manager`, its own board server (a `webui.Server` built with the existing `webui.New`, so its live-update hub watches only that account's manager), the time of its last activity, and whether it holds one of the server's places. Because the manager already owns everything that was "the session" (the current game and its record, the last decklist, the assistant label), one manager per account makes all of it per-account at once, and nothing in `internal/game` learns about accounts.

**The connector resolves the caller's game on every tool call.** The MCP tool server gains one optional hook (`Resolve`, nil on the desktop, where `M` and `BoardURL` are used exactly as today). The single place every tool is registered through looks up the caller's manager and board link from the request's token and passes them on to the tool, so every tool body acts on the caller's game and nothing else. Resolution is per call rather than per MCP session (one `*mcp.Server` per account would pin each claude.ai session to a stale manager after an idle drop). All of an account's conversations and devices carry tokens for the same Google account, so they reach the same manager, and an import in one replaces the game for all of them. **The board is per account.** In hosted mode the board's routes, behind google-login's session check, go to the signed-in account's own board server, so a tab only ever reads and receives its owner's game. The `board` key on game-opening results changes deliberately from web-mcp-docker's public root to `<public URL>/?board=<handle>`. The handle is a stable, non-reversible tag derived from the Google account id, so it survives restarts and imports. Opening a link whose handle is not the signed-in account's own gets a plain "not your game" page with no board and a Sign out button. A signed-out visitor is sent to Google sign-in by google-login's guard before anything is served. Where a game does not name the player's side, the board labels it with the signed-in account's name rather than "You".

**Idle games are dropped, and places follow games.** Activity is any tool call, any board action, or a board page load. A websocket connecting or reconnecting does not count, so a tab left open never keeps a game. An entry whose last activity is older than the operator's idle time (`-idle-timeout` / `TCGREPLAY_IDLE_TIMEOUT`, default 60 minutes, `0` = never) has its game dropped from memory. Its manager remembers that the game expired: the account's next tool call that needs the game, and its board, say so and ask for the saved game record to be imported again. Importing or starting a game clears that notice. The entry is kept, so open tabs show the expiry and follow the next import, but it gives up its place. Entries without a place and with no activity for a day are removed. google-login's user cap moves into this registry so that there is still exactly one limit mechanism, with the same `-max-users` setting and the same "server busy" page. A place is held from sign-in (or from the first activity after a place was given up) until the idle time passes, the account's last sign-in ends, or the server restarts. `hostauth` asks the registry for a place at its Google callback and tells it when an account's last sign-in ends, through a small `Places` interface in its config. An account acting after its place was freed while the server is full is told the server is busy. Expiry is checked both by a periodic sweep and on every lookup, so an expired game is reported, and its place freed, on time even between sweeps.

**The desktop is untouched by construction.** The registry is built only in `runHosted`. The MCP server's new hook and the board's existing hooks are nil on the desktop, which keeps its single manager, single board server and no sign-in. Everything stays in memory: a restart clears every account's game, as memory-store-record requires. The hosted server must run as one instance, which web-mcp-docker's `.do/app.yaml` already declares; this plan documents the reason there. This direction beats a per-account `*mcp.Server`, a multi-tenant hub inside one board server, deleting entries on expiry, keeping the cap counter in `hostauth`, and ending sign-ins on expiry. The evidence is in `research.md#alternatives-considered-and-rejected`.

## Component Breakdown

- **Per-account game registry (new, `internal/usergames`)** — the one new component, built only by hosted mode. It owns the mapping from a signed-in Google account to that account's playmat: its own game manager, its own board server, its stable board handle, the time of its last activity, and whether it holds one of the server's places. It answers four questions for the rest of the hosted server:
  - *which game does this tool call act on*: for the connector, an activity that may need a place;
  - *which board does this browser see*: for the board, with no activity, so an open tab keeps nothing;
  - *may this account take a place*: for sign-in, enforcing the user cap;
  - *this account's last sign-in ended*: free its place.
  It also runs the idle sweep, which drops games idle longer than the operator's setting, frees their places and purges long-idle entries. It is also the board front door for hosted mode: it routes the board's page, live connection and card requests to the signed-in account's board server, counts a page load as activity, turns away a link that names another account's board with a "not your game" page, and closes an entry's open tabs when the entry is removed. It reuses the existing game manager and board server unchanged and sits between google-login's sign-in service and the existing MCP and board servers.
- **MCP tool server (changed)** — gains one optional resolution hook. When set (hosted), every tool call first asks it for the caller's game manager and board link, and the tool acts on that manager. The `board` key on `new_game`, `import_log` and `set_opponent_deck` carries the caller's own board link instead of the shared root, and the assistant label is recorded on the caller's manager. When unset (every desktop mode), the server uses its single manager and board URL exactly as before. Tool names, inputs and descriptions do not change.
- **Game manager (changed, `internal/game`)** — learns to drop its current game because it expired. Expiring closes the game, frees its memory, and remembers a notice that every "no game open" answer then carries (the game expired after the idle time; import its saved record again) until a new game or import replaces it. It also tells its listeners that the game was cleared, so an open board shows the notice at once. It still knows nothing about accounts; the registry decides when to expire it.
- **Board server (changed, `internal/webui`)** — its state message carries the manager's notice when there is no game, and its hub refreshes open tabs when the manager clears its game. One board server is built per account by the registry, each with google-login's limit hook wrapped so that a board action also counts as that account's activity. The desktop board server is built and behaves as today.
- **Sign-in service (changed, `internal/hostauth`, from google-login)** — stops counting places itself. Its configuration takes a places provider (the registry), which it asks at the Google callback whether a newly signed-in account may take a place, and tells when an account's last sign-in ends. The busy page, its message and the per-account request limit are unchanged.
- **Hosted run mode and handler (changed, `cmd/tcgreplay`)** — reads the new idle-time setting beside the existing hosted settings, builds the registry from the shared card database, data directory, seat, public URL, user cap and idle time, hands it to the sign-in service as its places provider and to the MCP server as its resolver, mounts the registry's board front door behind the session check in place of the single board server, and runs the idle sweep for the life of the process. The shim, daemon and in-process modes are unchanged.
- **Board client (changed, `web/`)** — when the server sends a notice with no game (an expired game), the landing area shows it in place of the usual "start or import a game" text. When the game does not name the player's side, a hosted board labels it with the signed-in account's name rather than "You". The desktop board shows exactly what it shows today.
- **App Platform app definition and docs (changed)** — `.do/app.yaml` declares the idle-time setting and notes why the service must stay at one instance. The design document's hosted section describes per-account games, owner-only board links, the idle drop and its setting, and the new rule for when a place under the user cap is freed.

## Data Structures & Interfaces

No persisted format changes: nothing is written to disk. The game record, the card cache and the shapes of the board's websocket messages are unchanged, except for one optional field on the state message. The new contracts are an operator setting, the board link's form, the registry's in-process interface, two optional hooks (one on the MCP tool server, one in the sign-in service's configuration), and a small addition to the game manager.

**Hosted settings (addition)** — beside web-mcp-docker's and google-login's settings. `-max-users` / `TCGREPLAY_MAX_USERS` keeps its flag, default (10) and meaning of `0`, but now counts places as defined below.

| Setting | Flag | Environment default | Fallback |
|---|---|---|---|
| Idle time before a game is dropped | `-idle-timeout` | `TCGREPLAY_IDLE_TIMEOUT` (Go duration, e.g. `2m`, `1h`) | `60m`; `0` = never drop |

**Board link (changed)** — the `board` key on `new_game`, `import_log` and `set_opponent_deck` results, hosted only, now names its owner. `/` without `board` opens the signed-in account's own board; a `board` handle that is not the signed-in account's own gets the "not your game" page (403) and no board.

```json
{ "game": "import-20261017-153000-ledjo23", "record": { "…": "…" },
  "board": "https://playmat.example.com/?board=Qm9hcmQtaGFuZGxl" }
```

**Per-account registry (new, package `usergames`)** — the hosted server's lookup from account to playmat.

```go
type Config struct {
    NewManager func() *game.Manager               // an empty manager sharing the card DB and data dir
    NewBoard   func(*game.Manager) *webui.Server  // a board server for one account (seat, public URL, limit hook)
    Public     *url.URL                           // the -public-url value, for board links
    Idle       time.Duration                      // 0 = never drop
    MaxUsers   int                                // places; 0 = no cap
    Now        func() time.Time                   // tests inject a clock
}

func New(cfg Config) *Registry
func (r *Registry) Use(acct hostauth.Account) (*Entry, error) // activity: touch; take a place if none (hostauth.ErrBusy when full)
func (r *Registry) Get(acct hostauth.Account) *Entry          // no activity, no place (live connections, card lookups)
func (r *Registry) Admit(acct hostauth.Account) error         // hostauth.Places: sign-in takes a place
func (r *Registry) Release(id string)                         // hostauth.Places: last sign-in ended; remove the entry
func (r *Registry) Run(ctx context.Context)                   // idle sweep until ctx ends
func (r *Registry) ResolveMCP(ctx context.Context, extra *mcp.RequestExtra) (*game.Manager, string, error) // for mcpserver.Server.Resolve
func (r *Registry) Board() http.Handler                       // the hosted board front door (behind RequireSession)
func Handle(accountID string) string                          // stable board handle for an account

type Entry struct {
    M      *game.Manager
    UI     *webui.Server
    Handle string
    // unexported: last activity, holds-place flag, done channel closed on removal
}
```

**Sign-in service configuration (changed, google-login's `hostauth`)** — the user cap moves out; a places provider comes in. `ErrBusy` and the busy page are unchanged.

```go
type Places interface {
    Admit(acct Account) error // nil, or an error wrapping ErrBusy
    Release(id string)        // the account's last sign-in ended
}

type Config struct {
    // ... Public, GoogleClientID, GoogleSecret, RateLimit, Google as in google-login
    Places Places // nil = no cap (replaces MaxUsers)
}
```

**MCP tool server (changed)** — one optional hook beside web-mcp-docker's `BoardURL`.

```go
type Server struct {
    M          *game.Manager
    ClaudeSeat, HumanSeat state.Seat
    Version    string
    BoardURL   string
    // Resolve, if set, picks the manager and board link for each tool call (hosted: the caller's account).
    // nil = M and BoardURL for every call (desktop, unchanged).
    Resolve func(ctx context.Context, extra *mcp.RequestExtra) (m *game.Manager, board string, err error)
}
```

**Game manager (changed)** — expiry and its notice.

```go
var ErrExpired = errors.New("game expired")

// Expire drops the current game (if any) and makes Current report notice, wrapping ErrExpired,
// until a new game or import replaces it. Listeners registered with OnClear are told.
func (m *Manager) Expire(notice string)
func (m *Manager) Notice() string        // the expiry notice, or "" while a game is open or none expired
func (m *Manager) OnClear(f func())
```

**Board state message (additive)** — with no game open, the state message may carry `notice` (the expiry text), which the board's landing area shows.

```json
{ "type": "state", "human": "p1", "claude": "p2", "assistant": "Claude", "reveal": false,
  "snapshot": null, "notice": "Your game expired after 60 minutes without activity. Ask Claude to import its saved game record again." }
```

## Implementation Detail

**New module boundary: one package decides whose playmat a request reaches.** Everything about "which account owns which game" lives in the new `usergames` package. It depends on `game` and `webui` (to build a manager and a board server per account) and on `hostauth` (for the account on a request, `ErrBusy` and the `Places` contract). None of those packages learns about it: `hostauth` sees only a two-method interface, the MCP tool server sees only a resolution function, and the game manager and board server are used exactly as the desktop uses them. A developer reading the hosted handler sees three wirings into the registry: the places provider for sign-in, the resolver for `/mcp`, and the board front door behind the session check. Tracing an action means reading account → entry → manager, with no branches on "hosted" inside the game or board code.

**New pattern: per-call tenancy through the single tool-registration helper.** The MCP tool server already registers every tool through one generic helper. That helper becomes the one place a tool call is bound to its game: it asks the optional resolver for the caller's manager and board link and hands them to the tool body through the call's context. Tool bodies stop reading the server's single manager field directly and ask for "this call's manager" instead. This is a mechanical change across the tool file, which the compiler and the existing tool tests check. On the desktop the resolver is unset and "this call's manager" is the one manager, so behaviour and results are byte-for-byte the same.

**Reuse over rework for the board.** Rather than teaching the board server's live-update hub about accounts, the registry builds one ordinary board server per account with the existing constructor. Each one has its own hub watching only its manager, and is started and stopped with its entry. Scoping live updates is therefore a property of the structure, not of a filter that could be got wrong. The board front door is a thin router: find the signed-in account's entry, check the link's handle on page loads, count page loads as activity, tie live connections to the entry's lifetime (the same context-cancellation technique google-login uses to close tabs on sign-out), and hand the request to that account's board server. Board actions count as activity by wrapping google-login's existing per-action limit hook, so the board server gains no new hook for this.

**New pattern: an activity clock with two readers.** Each entry records its last activity. Every lookup that counts as activity first checks whether the entry has already passed its idle time. If it has, the lookup expires the game and frees the place before going on, so expiry is exact even if the periodic sweep has not run. The sweep runs in the background, does the same for entries nobody touches, and purges entries that have held no place for a day. Admission at sign-in also sweeps first, so a place freed by expiry is available to the next account at once. The clock is injected, as in google-login, so idle, keep-alive and cap tests run without sleeping.

**Expiry is a state of the manager, not an absence of the entry.** Dropping an idle game closes it and leaves the manager in an "expired" state whose notice replaces the usual "no game open" message. Every existing code path that asks for the current game (tools and board verbs alike) reports the expiry with no change of its own. A new game or import clears the state. The manager notifies its listeners when it clears its game, the same way it already notifies them when a game is loaded, so an open board refreshes to the notice.

**Following the upstream plans' conventions.** Hosted-only behaviour hangs on optional fields with desktop-preserving zero values (the resolver on the MCP server, the places provider on the sign-in service). The public URL is the only source of the board link's address. Settings are flags with environment defaults. Tests drive the real hosted handler with google-login's fake Google and its helpers, signing in two named accounts to prove isolation end to end.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Plan `20261017105035-e3f64c1b-web-mcp-docker` must land first (flagged).** It provides `tcgreplay -hosted`, the hosted handler with one router for `/mcp` and the board, the `-public-url` setting (reused here for board links), `webui.Server.Public`, `mcpserver.Server.BoardURL`, the hosted-handler tests and `.do/app.yaml` with one instance. This plan changes one of its outcomes deliberately: the hosted `board` link becomes the caller's own `/?board=<handle>` rather than the public root.
- **Plan `20261017112555-30c99ab1-memory-store-record` must land first (flagged).** It provides the memory-only game manager, the record on `Game`, the last decklist in manager memory and `get_record`, and removes `list_games`, `load_game` and `Resume`. Per-account managers make all of that per account. This plan must not bring back game storage, `Resume` or `load_game`.
- **Plan `20261017123240-21493a95-google-login` must land first (flagged).** It provides `internal/hostauth`: the signed-in account on board requests (`AccountFrom`) and on MCP tokens (`AccountFromToken`), `RequireToken`, `RequireSession` (with its websocket cancellation on sign-out), the per-account request limit and the `webui.Server.Limit` hook, `ErrBusy` and the busy page, `-max-users`, the injectable clock, and the `hostauthtest` helpers (fake Google, `MCPTransport`, `BoardCookie`, `SetAccount`). This plan changes it: the user cap's counting moves out of `hostauth` into the per-account registry, through a new `Places` field on `hostauth.Config` that replaces `MaxUsers` there. The setting, its default, the busy page and google-login's cap tests keep their meaning.
- **None of the three upstream plans is implemented at the time of planning** (HEAD `77e6a6a`). This plan's tasks start from the code those plans produce, and name the functions they introduce.
- **`github.com/modelcontextprotocol/go-sdk` v1.6.0 (existing)** — `RequestExtra.TokenInfo` on every tool call, used to find the caller's account, plus `auth.TokenInfo`. No version change.
- **`github.com/coder/websocket` (existing)** — used unchanged by the per-account board servers, and in tests to dial two accounts' boards.
- **Internal packages (changed)** — `internal/mcpserver` (resolver hook, per-call manager), `internal/game` (expiry and notice on the manager), `internal/webui` (notice in the state message, refresh on clear), `internal/hostauth` (places provider in place of its own cap counting), `cmd/tcgreplay` (idle setting, registry wiring), `web/` (expiry notice, own-name fallback).
- **Internal package (new)** — `internal/usergames`, hosted only.
- **Internal packages (used unchanged)** — `internal/store`, `internal/importer`, `internal/state`, `internal/cards` (one card database shared by every account, since card data is public and is not game data).
- **DigitalOcean App Platform (external, existing)** — runs the one instance this plan relies on, since accounts' games live in that process's memory. The app definition gains the idle setting; applying it to the running app is a one-time step for the owner.
- **Board UI build (existing)** — the Svelte changes ship through the existing Vite build embedded in the binary. No new front-end dependency.

## Testing Approach

Tests stay in Go's standard `testing` package with `httptest`, beside the code they cover, and run under the repo's vet-and-test Makefile target. Nothing talks to the real Google or claude.ai: google-login's fake Google and its helpers sign in two or more named test accounts through the real routes. An injected clock drives every idle, keep-alive and cap test without sleeping, so tests that would take minutes at the spec's "2 minutes" run instantly.

**Unit: the per-account registry (most coverage).** It decides whose game every hosted request reaches and when games and places go, so it gets the densest tests:
- two accounts get different managers and board servers, and the same account always gets the same ones;
- the board handle is stable for an account and differs between accounts;
- with a 2-minute idle time, an entry untouched for just over 2 minutes has its game dropped on the next lookup, and the manager reports the expiry; an entry used every minute for 5 minutes keeps its game with every move intact; lookups that are not activity (a board connecting) never keep a game alive;
- the background sweep expires untouched entries and frees their places, and purges entries that have held no place for a day;
- with the cap at its limit, a new account is refused as busy until another account's game expires, and is then admitted; an account that signed in and never acted gives up its place after the idle time; an account whose place was freed is admitted again on its next action, or told the server is busy when it is full;
- the last sign-in ending removes the entry, frees its place and closes its tabs; an idle time of 0 never drops, and a cap of 0 never refuses.

**Unit: the game manager's expiry.** Expiring drops and closes the game, makes every "current game" answer carry the expiry notice, and tells its listeners. A new game or import clears the notice. A manager that never expired behaves exactly as today.

**Unit: the MCP tool server and board server hooks.** With a resolver set, tool calls act on the manager it returns and the `board` key is the link it returns. With none (the desktop), results, tool list and instructions are unchanged, which the existing tool tests over both transports keep proving. The board's state message carries the notice when the game expired, and an open board is refreshed when its manager clears its game. With the notice unset, the message is unchanged.

**Integration: the hosted handler, end to end with two accounts.** Through the same handler hosted mode serves, with fake Google and two signed-in accounts A and B (connector and board):
- A and B each import a different game; each reads only their own through the connector and the board, and a move by A leaves B's game unchanged (**Isolation**);
- two connector sessions for A see each other's moves on their next read, and A's open board shows them (**Same account, two conversations**); an import in one of A's sessions replaces what A's other session and A's board see (**Import replaces for the account**);
- B opening A's board link gets the "not your game" page and no board or card data; a signed-out request for A's link is sent to sign in and gets nothing of the game (**Owner-only link**, **Signed-out link**);
- A's open board receives A's moves within 2 seconds and receives nothing while B acts on B's game (**Live updates scoped**);
- with a 2-minute idle time and the injected clock: after 2 minutes idle, A's next tool call and A's next board load report the expired game and ask for the record; an open tab with no activity does not stop the expiry; acting every minute for 5 minutes keeps the game with its moves (**Idle drop**, **Open tab does not keep a game**, **Activity keeps a game**);
- with the cap at 1: A holds the place, a new account C is refused as busy at sign-in, and once A's game expires C signs in and starts a game (**Expired game frees a place**);
- each account's board state names its own side from its own import (**Own name**, server half);
- the existing hosted tests from the upstream plans (tool parity, board links, the 2-second board latency, board moves reaching Claude, sign-in, rate limit, user cap) pass with the per-account wiring; their user-cap tests now exercise the registry.

**Unit: hosted settings.** The idle setting resolves from flag and environment as documented, with a default of 60 minutes, `0` meaning never, and a clear start-up error for an invalid value.

**Regression: the desktop.** The desktop modes build no registry and set no resolver. The full existing suite (MCP tools over both transports, board, shim, installer, importer, game) passes, and a desktop tool result's `board` key and the desktop board's state message are unchanged (**Desktop**, code level). The board client is not unit-tested (the repo has no front-end tests); its expiry notice and own-name label are checked by hand.

**Deliberate gaps.** No automated test runs two real Google accounts through claude.ai, waits real minutes on the deployed app, or measures container memory over a week. Those need real accounts and the deployed app. The in-process tests drive the same handler with the same protocol steps and an injected clock.

**Success metrics:**
- Two different signed-in accounts use the server at the same time for a full session, and neither sees the other's game — **Behavioural test** (the two-account isolation, scoped-updates and owner-only-link integration tests above), plus **Manual — captured in the implementation test plan** (two real Google accounts through claude.ai and the board on the deployed app).
- Over a week with the idle drop on, memory stays within what the container is given — **Manual — captured in the implementation test plan** (review a week of App Platform memory metrics). Its automated precursor is a **Behavioural test** that expiry drops each idle game and frees its place, and that entries without a place are purged.

**Manual reviews:**
- **Two accounts, real claude.ai**: two Google accounts each connect claude.ai and import different games; each sees only their own game in Claude and on the board — **Manual — captured in the implementation test plan.**
- **Same account on two devices**: a move from a conversation on the phone shows on the laptop's board tab and in a laptop conversation's next read; an import on one replaces it on the other — **Manual — captured in the implementation test plan.**
- **Owner-only link in a browser**: account B opening A's board link sees "not your game" and no cards; a private window (signed out) is asked to sign in and sees no cards — **Manual — captured in the implementation test plan.**
- **Idle drop on the deployed app** with the idle time set to 2 minutes: after 2 idle minutes Claude reports the expired game and asks for the record, and the board shows the notice; an open tab alone does not keep the game; acting every minute for 5 minutes keeps it — **Manual — captured in the implementation test plan.**
- **Expired game frees a place** on the deployed app with the user cap temporarily at 1 — **Manual — captured in the implementation test plan.**
- **Own name**: each account's board labels the player's side with their own name — **Manual — captured in the implementation test plan.**
- **Desktop regression**: Claude Code and Claude Desktop keep one game, no sign-in and the root board link — **Manual — captured in the implementation test plan.**

## Milestones & Tasks

### Milestone 1: Every signed-in account has its own game and its own board

**What changes**: Several people can now share the hosted playmat without seeing or changing each other's boards. Each Google account has its own game: everything Claude does in that person's conversations (imports, moves, rewinds, branches) acts only on their game, and all of their conversations, board tabs and devices see and act on that one game, so an import in one replaces it everywhere for them. The board link Claude hands out now belongs to its owner. Opening it signed in as that account shows their live board, which follows only their game. Opening it signed in as anyone else shows a "not your game" message and nothing of the game. Opening it signed out asks for sign-in first. The player's side of the board shows their own name. Games still last until the server restarts, and the signed-in user limit works as before. The desktop still has one game, no sign-in and no change.

**Validation point**: The hosted tests with two signed-in accounts pass: isolation through the connector and the board, two sessions of one account sharing a game, import replacing the game for the account, owner-only and signed-out board links, live updates reaching only the owner's board, and each account's own name. The full vet-and-test suite passes, desktop tests unchanged. On the deployed app, two real Google accounts each import a game through claude.ai and see only their own.

#### - [x] Task: Keep one game per account
**Id:** 53e9a4b1-d3d2-41e0-9c52-ad5e768c3658
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Adds a hosted-only registry that gives every signed-in Google account its own playmat: its own game manager, its own board server and a stable board handle, created the first time the account is seen and kept in memory. It records when each account was last active and distinguishes lookups that count as activity from ones that do not, which the idle drop builds on later. Nothing in the game or board code learns about accounts.

*Technical detail:* [context.md#task-keep-one-game-per-account](./context.md#task-keep-one-game-per-account)

**Acceptance criteria**:
- [x] Two different accounts always get different games and boards, and the same account always gets the same ones.
- [x] An account's board handle is the same every time, including after a restart, differs between accounts, and does not reveal the account's identity.
- [x] Looking up an account for an action records the time; looking it up for an open board does not.
- [x] Nothing about an account's game is written to disk.

#### - [x] Task: Route Claude's tool calls to the caller's own game
**Id:** 49450dbe-f627-40dd-bf66-047ec218bdab
**Repo:** tcgreplay
**Depends on:**
- 53e9a4b1-d3d2-41e0-9c52-ad5e768c3658 — Keep one game per account
**Execution:** agent

Lets the MCP tool server act on a different game for each caller. When hosted, every tool call first finds the caller's own game from their sign-in, and the board link in game-opening results points at the caller's own board. On the desktop nothing is set, and every tool behaves and answers exactly as before.

*Technical detail:* [context.md#task-route-claudes-tool-calls-to-the-callers-own-game](./context.md#task-route-claudes-tool-calls-to-the-callers-own-game)

**Acceptance criteria**:
- [x] With per-call lookup in place, every tool acts on the game the lookup returns, and nothing else.
- [x] Game-opening results carry the caller's own board link.
- [x] A tool call counts as activity for the caller's account.
- [x] Without per-call lookup (the desktop), tool results, the tool list and instructions are unchanged, and the existing tool tests pass over both transports.

#### - [x] Task: Give each account its own board
**Id:** 79556cbd-7c28-4686-9646-06b8d2c0b005
**Repo:** tcgreplay
**Depends on:**
- 53e9a4b1-d3d2-41e0-9c52-ad5e768c3658 — Keep one game per account
**Execution:** agent

Adds the hosted board's front door. Each signed-in browser is sent to its own account's board server, so a tab only ever shows, and receives live updates for, its owner's game. Opening a board link that belongs to another account shows a "not your game" page with nothing of the game and a way to sign out. Opening the board counts as activity; a tab staying connected does not. Board moves count as activity too.

*Technical detail:* [context.md#task-give-each-account-its-own-board](./context.md#task-give-each-account-its-own-board)

**Acceptance criteria**:
- [x] A signed-in browser sees only its own account's game on the board, and its live connection receives only that game's changes.
- [x] Opening another account's board link shows "not your game" and no board, cards or game data.
- [x] Opening the board without a link, or with one's own link, shows one's own board.
- [x] Loading the board page and making a board move count as activity; an open, idle live connection does not.

#### - [ ] Task: Turn on per-account games in hosted mode
**Id:** a11748d2-002f-48ff-8242-29c4ae5a77cc
**Repo:** tcgreplay
**Depends on:**
- 49450dbe-f627-40dd-bf66-047ec218bdab — Route Claude's tool calls to the caller's own game
- 79556cbd-7c28-4686-9646-06b8d2c0b005 — Give each account its own board
**Execution:** agent

Wires the registry into hosted mode. The connector looks up each caller's game through it, the board routes go through its front door behind the sign-in check, and per-account board servers carry the public address and the request limit. End-to-end tests with two signed-in accounts prove isolation, shared games across one account's sessions, owner-only links and scoped live updates. The desktop modes are unchanged.

*Technical detail:* [context.md#task-turn-on-per-account-games-in-hosted-mode](./context.md#task-turn-on-per-account-games-in-hosted-mode)

**Acceptance criteria**:
- [ ] Two accounts each import a different game and each sees only their own, through Claude and on the board; a move by one leaves the other's game unchanged.
- [ ] A move in one of an account's connector sessions is seen by its other session and on its open board; an import in one replaces the game for all of them.
- [ ] Another account opening a board link sees "not your game"; a signed-out visitor is sent to sign in; neither sees any cards.
- [ ] An account's open board shows its moves within 2 seconds and receives nothing when another account acts.
- [ ] The upstream hosted tests (tool parity, board latency, board moves reaching Claude, sign-in, limits) pass, and the desktop modes start and behave as before.

#### - [ ] Task: Show the player's own name on the hosted board
**Id:** 419fc203-010e-47ae-92df-9f8ffb6f60a3
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Makes the hosted board label the player's side with the signed-in person's name when the game itself does not name it, instead of "You". An imported game already names the player's side from that account's own import. The desktop board keeps its current labels.

*Technical detail:* [context.md#task-show-the-players-own-name-on-the-hosted-board](./context.md#task-show-the-players-own-name-on-the-hosted-board)

**Acceptance criteria**:
- [ ] On the hosted board, the player's side shows the game's name for that seat, or else the signed-in account's name.
- [ ] One account's name never appears on another account's board.
- [ ] The desktop board's labels are unchanged.

### Milestone 2: Idle games are cleared and free their place for someone else

**What changes**: The operator sets how long a game may sit unused (60 minutes unless changed; for example 2 minutes for testing). A game with no tool calls from Claude, no moves on the board and no board page loads for that long is cleared from memory. A board tab left open does not keep it. The account's next request to Claude, or its next board load, says the game expired and asks for the saved game record to be imported again, and an open board shows the same message. Importing brings the game back as usual. Clearing a game frees its place under the signed-in user limit, so once the server is full, a new person can sign in as soon as someone else's game expires. An account that signs in and never plays gives its place back after the same idle time. The setting appears in the App Platform app definition, and the design document describes per-account games, board links, the idle drop and the new rule for freeing places. The desktop never clears its game.

**Validation point**: The registry and hosted tests with an injected clock and a 2-minute idle time pass: expiry is reported to Claude and on the board, an open tab does not keep a game, a game used every minute for 5 minutes survives, and an expired game frees a place under a cap of 1. The idle setting's defaults and errors are tested, and the full suite passes. On the deployed app with the idle time at 2 minutes, an idle game expires and is restored by importing its record.

#### - [ ] Task: Let a game expire with a notice
**Id:** e509cf51-cff4-4442-9848-605bd943317f
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Teaches the game manager to drop its current game because it expired, freeing its memory, and to remember a notice saying so. Until a new game or import replaces it, every request that needs the game (from Claude or the board) gets that notice, which asks for the saved game record to be imported again. An open board refreshes and shows the notice at once. Nothing calls this on the desktop.

*Technical detail:* [context.md#task-let-a-game-expire-with-a-notice](./context.md#task-let-a-game-expire-with-a-notice)

**Acceptance criteria**:
- [ ] After a game expires, tools and board actions that need it report that it expired and ask for its saved record to be imported again.
- [ ] An open board switches to the expiry notice without a reload, and a newly loaded board shows it.
- [ ] Importing or starting a game clears the notice and works as usual.
- [ ] A manager whose game never expired behaves exactly as before.

#### - [ ] Task: Drop games left idle
**Id:** 50487526-2103-4092-9293-1ebf3bb5e899
**Repo:** tcgreplay
**Depends on:**
- a11748d2-002f-48ff-8242-29c4ae5a77cc — Turn on per-account games in hosted mode
- e509cf51-cff4-4442-9848-605bd943317f — Let a game expire with a notice
**Execution:** agent

Adds the operator's idle-time setting and the sweep that uses it. A game with no activity for longer than the setting expires, both when the background sweep finds it and when the account next acts, so expiry is reported on time. A tab left open does not count as activity. Accounts left without a game for a day are forgotten, so memory stays bounded however many people come and go.

*Technical detail:* [context.md#task-drop-games-left-idle](./context.md#task-drop-games-left-idle)

**Acceptance criteria**:
- [ ] With the idle time at 2 minutes, after 2 minutes without activity the account's next tool call or board load reports that the game expired and asks for its record.
- [ ] A board tab left open with no activity does not stop the game expiring.
- [ ] A game acted on every minute for 5 minutes is still there with its moves intact.
- [ ] The idle time is a setting with a 60-minute default; 0 turns the drop off; an invalid value stops start-up with a clear message.
- [ ] Accounts left without a game and without activity for a day are removed from memory.

#### - [ ] Task: Free a place when a game expires
**Id:** e213638b-5365-43ee-8ca3-ff258aba4813
**Repo:** tcgreplay
**Depends on:**
- 50487526-2103-4092-9293-1ebf3bb5e899 — Drop games left idle
**Execution:** agent

Moves the signed-in user limit's counting into the per-account registry, so a place follows activity. A place is taken at sign-in or at an account's first action after losing one, and freed when the account's game expires, when the account goes idle without a game, or when its last sign-in ends. The setting, its default and the "server busy" page are unchanged. There is still exactly one limit.

*Technical detail:* [context.md#task-free-a-place-when-a-game-expires](./context.md#task-free-a-place-when-a-game-expires)

**Acceptance criteria**:
- [ ] With the server at its user limit, once one account's game expires a new account can sign in and start a game.
- [ ] An account that signed in and never acted gives up its place after the idle time.
- [ ] An account whose place was freed gets it back on its next action, or is told the server is busy when it is full.
- [ ] Accounts already holding a place can always sign in again, and the last sign-in ending still frees a place.
- [ ] The limit is still one setting, defaulting to 10, with 0 meaning no limit.

#### - [ ] Task: Document per-account games and the idle drop
**Id:** f0451c41-419d-4282-a812-454641e29d47
**Repo:** tcgreplay
**Depends on:**
- 50487526-2103-4092-9293-1ebf3bb5e899 — Drop games left idle
- e213638b-5365-43ee-8ca3-ff258aba4813 — Free a place when a game expires
**Execution:** agent

Describes per-account games in the project's design document beside the hosted mode and sign-in: one game per account shared across conversations and devices, owner-only board links, the idle drop and its setting, how places under the user limit are freed, and why the server must run as one instance. The App Platform app definition declares the idle setting and notes the single-instance requirement.

*Technical detail:* [context.md#task-document-per-account-games-and-the-idle-drop](./context.md#task-document-per-account-games-and-the-idle-drop)

**Acceptance criteria**:
- [ ] The design document explains per-account games, board links, the idle drop, freeing places and the single-instance requirement.
- [ ] The idle setting is listed with its default and meaning, and the app definition declares it.
- [ ] The document states that the desktop keeps one game with no accounts.

#### - [ ] Task: Apply the updated app definition to the deployed app
**Id:** 60da0ec5-dc89-462a-9c89-97ec24e80605
**Repo:** tcgreplay
**Depends on:**
- f0451c41-419d-4282-a812-454641e29d47 — Document per-account games and the idle drop
**Execution:** human — needs the owner's DigitalOcean App Platform account to apply the app definition

The owner applies the updated app definition to the running App Platform app, so the idle setting appears in its settings and can be changed without a rebuild. The owner also confirms the app still runs a single instance.

*Technical detail:* [context.md#task-apply-the-updated-app-definition-to-the-deployed-app](./context.md#task-apply-the-updated-app-definition-to-the-deployed-app)

**Acceptance criteria**:
- [ ] The deployed app shows the idle setting and still runs exactly one instance.
- [ ] Changing the idle setting and restarting the app takes effect with no rebuild.

## Open Questions

- **Do the per-account games fit the container's memory under real use?** This depends on the real size of an imported game with its branches and notes, its board server, and the App Platform instance size. Only the deployed app shows it, during the week-long memory check. If memory nears the container's limit while the number of places held stays within `TCGREPLAY_MAX_USERS`, first record the number of places held and entries kept, which the registry logs on every sweep, alongside the memory graph. Then STOP and ask the user whether to lower the defaults for the user limit or the idle time, or to resize the instance. Do not add game storage or a second limit on your own: both are ruled out by the spec and the upstream plans.

## Out of Scope

- **Saving games so they survive a restart, a redeploy or the idle drop** — a spec non-goal. Games stay in memory only, and an expired or restarted game comes back by importing its saved record (memory-store-record).
- **Sharing a game between accounts, or letting another account view or play someone's board** (spectators, coaching, team games) — boards are owner-only by requirement. No sharing link or invite is offered.
- **More than one hosted instance or sharing games across copies** — the hosted server stays a single instance (spec constraint, web-mcp-docker's app definition). No external session or game store.
- **Warning the user before their game expires, or a countdown on the board** — the game simply expires after the idle time, and the next action or board load reports it.
- **Per-account limits beyond google-login's two** (for example a cap on games, branches or memory per account) — memory is bounded by the user cap and the idle drop, as the spec intends.
- **Idle expiry of sign-ins** — an expired game frees its place under the user cap, but the account's sign-ins (connector tokens, board session) still last until sign-out or restart, as google-login decided.
- **Google profile names** — the board's fallback label uses the signed-in email's name part. The sign-in scopes stay `openid email`.
- **Per-account assistant labels beyond what the connector reports** — the label still comes from the MCP client's name. All per-account managers share the hosted data directory's label file, which only ever records the assistant's name.
- **Any change to the desktop** — Claude Code and Claude Desktop keep one game, no accounts, no idle drop and the root board link (spec requirement).
- **Packaging the tcg-* skills for claude.ai** — a later spec; the skills already work on "the game open now", which is now the caller's own game.

## Changelog

### 2026-10-18 — Task: Keep one game per account

**What was done**: Added the hosted-only package `internal/usergames` with a `Registry` that gives each Google account its own `game.Manager` and `webui.Server` (hub started for the entry's lifetime), a stable opaque board handle (`Handle`: domain-separated SHA-256, 12 bytes, base64url) and the `<public>/?board=<handle>` link. `Use` counts as activity and records the time; `Get` does not.

**Deviations**: The plan's drift against the landed upstream code was adapted, as its Open assumptions allow: google-login's cap lives in `store.addSession` (called from `Auth.signIn` at the Google callback), not in an `admit` function in `limit.go`, and its cap tests are in `internal/hostauth/cap_test.go` and `cmd/tcgreplay/hosted_cap_test.go`. `LastActive` is a registry method (it reads under the registry's lock) rather than an `Entry` method.

**Files changed**:
- `internal/usergames/usergames.go`
- `internal/usergames/usergames_test.go`

**Discoveries**: `game.Manager.SetAssistant` writes `assistant.txt` to the data dir; it holds only the assistant's name, which the plan's Out of Scope accepts as shared across accounts. A memory-only `Game` has no `Close`, so "closing" a game is dropping the pointer.

### 2026-10-18 — Task: Route Claude's tool calls to the caller's own game

**What was done**: The MCP tool server gained an optional `Resolve` hook. The single registration helper `add` now binds every tool call to its game (`bind`): with a resolver, the caller's manager and board link; without, the server's `M` and `BoardURL`. Tool bodies read `s.m(ctx)` and `s.board(ctx)`, and a resolver error comes back as a tool error. `usergames.Registry.ResolveMCP` finds the account from the request's token, counts the call as activity and returns its manager and `/?board=<handle>` link.

**Deviations**: The per-call context type is named `boundCall` (the test file already has a `call` helper). With a resolver, the assistant label is set on the caller's manager on each call, and `InitializedHandler` sets it only on the desktop, as planned.

**Files changed**:
- `internal/mcpserver/server.go`
- `internal/mcpserver/server_test.go`
- `internal/usergames/mcp.go`
- `internal/usergames/mcp_test.go`

**Discoveries**: `card_text` reached the card database through `s.M.DB`; with a nil `M` in hosted mode it must go through the call's manager too, which the mechanical `s.M.` → `s.m(ctx).` sweep covers.

### 2026-10-18 — Task: Give each account its own board

**What was done**: Added the hosted board's front door, `usergames.Registry.Board()`, to mount behind `hostauth.RequireSession`. It sends each signed-in browser to its own account's board server. A page load counts as activity, and a `?board=` handle that is not the viewer's own gets a 403 "Not your game" page with a link to one's own board and a Sign out button. `/ws` is not activity and is tied to the entry's lifetime. Assets and card lookups are not activity. `Registry.BoardLimit(next)` wraps the sign-in service's per-action limit so that a board verb counts as activity.

**Deviations**: Exported `hostauth.WithAccount` (a wrapper over the existing unexported helper) so board handlers can be served with an account in tests and wiring. A busy refusal on a page load is a plain 503 carrying the error text; the cap task decides its final wording. `/index.html` is redirected to `/` by the board's file server, so it is not tested separately.

**Files changed**:
- `internal/usergames/board.go`
- `internal/usergames/board_test.go`
- `internal/hostauth/hostauth.go`

**Discoveries**: coder/websocket closes the connection when a read's context times out, so "receives nothing within N ms" tests must read in a background goroutine into a channel (`liveConn` in board_test.go). Server goroutines read the injected clock while tests advance it, so the board tests use a mutex-guarded clock (`lockedClock`).
