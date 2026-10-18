---
created_date: "2026-10-17"
---

# Planning summary: 20261017105033-91f90842-hosted-playmat

## Decisions
### Saved games

- Plans: web-mcp-docker, memory-store-record.
- web-mcp-docker keeps game files under the container's `$HOME`, reopens the last game on hosted start and puts a board link on `load_game`; memory-store-record removes saved games in every mode, with `list_games`, `load_game` and `Resume`.
- Outcome: memory-store-record's rule is the epic's rule. web-mcp-docker stays as written since it lands first; memory-store-record removes those parts, and no later spec brings them back.

### Board link

- Plans: web-mcp-docker, per-user-games.
- web-mcp-docker's `board` link is the public root URL; per-user-games changes it to `<public>/?board=<owner handle>`.
- Outcome: per-user-games' link stands once it lands; web-mcp-docker stays as written.

### Health check path

- Plans: web-mcp-docker, google-login.
- web-mcp-docker checks `/`; google-login moves the App Platform health check to an unauthenticated `/healthz` because `/` redirects to sign-in.
- Outcome: google-login's `/healthz` stands; web-mcp-docker stays as written.

### User cap

- Plans: google-login, per-user-games.
- google-login counts accounts holding any live sign-in; per-user-games counts places holding a game, freed by idle expiry, last sign-in ending or restart, with counting in the per-user registry.
- Outcome: per-user-games' meaning stands; the setting (`-max-users`, default 10, `0` = off) and the busy page are unchanged. google-login stays as written.

## Order added for shared files
- Added: `20261017123240-21493a95-google-login` now depends on `20261017112555-30c99ab1-memory-store-record`. Both change `tcgreplay:internal/webui/actions.go`, `tcgreplay:internal/webui/hub.go`, `tcgreplay:internal/webui/webui_test.go`, `tcgreplay:cmd/tcgreplay/main.go`, `tcgreplay:cmd/tcgreplay/hosted.go`, `tcgreplay:cmd/tcgreplay`, `tcgreplay:web/src/App.svelte`, `tcgreplay:web/src/lib/ws.js`, `tcgreplay:web/src/lib/store.svelte.js`.

## 20261017105035-e3f64c1b-web-mcp-docker
### Approach

A new run mode of the existing binary, `tcgreplay -hosted`, runs the game manager and MCP server in-process and serves everything from one plain-HTTP listener: `/mcp` through go-sdk v1.6.0's Streamable HTTP handler (stateful sessions, 4h idle timeout), and `/`, `/ws` and `/api/card` from the existing board handler. A single public address setting (`-public-url` / `TCGREPLAY_PUBLIC_URL`) drives board links and the board's allowed websocket origin; the request's Host header is never used. With it unset (every desktop mode) the desktop is unchanged. The image is a multi-stage Dockerfile (node, go, distroless non-root); `.do/app.yaml` deploys on push to `main` only, as one instance, with the public URL from `${APP_URL}`.

Rejected: a separate binary, the daemon behind a relay, the legacy SSE transport, stateless sessions, server keep-alive pings, Host-header links, `?game=<id>` links, a CI pipeline and registry, a baked card cache, an Alpine runtime.

### Milestones and tasks

Milestone 1: Claude in claude.ai can drive the hosted playmat (the epic's go/no-go).
- Serve the playmat over HTTP in a hosted mode
- Run the MCP tool tests over the hosted transport
- Build the hosted playmat as a container image
- Define the App Platform app in the repo
- Create the App Platform app and its domain (human)

Milestone 2: A board link opens a live board that follows Claude.
- Let the board serve a public address and stay connected (websocket ping every 30s)
- Add board links to game-opening tool results
- Wire the public address through hosted mode
- Document the hosted mode

### Tasks for a person

- Create the App Platform app and its domain: needs the owner's DigitalOcean account, GitHub authorisation and Cloudflare DNS. Replace the placeholder domain, create the app, set the CNAME DNS-only first, then proxied with Full (strict).

### Out of scope

Login; in-memory storage and the game record; per-user games; keeping games or the card cache across deploys; packaging the tcg-* skills for claude.ai; Notion; MCP Apps and chat inside the board; multiple instances; TLS inside the container; CI pipeline or registry; MCP keep-alive and stream resumption; any change to the Svelte board client.

### Drafting assumptions

- The board link is the public root URL, not `/?game=<id>`.
- Streamable HTTP is stateful with default SSE responses and no server KeepAlive; MCP sessions time out after 4h idle.
- Hosted mode turns off the SDK's localhost protection, skips the board's loopback Host guard and allows the public host's websocket origin.
- `-listen` defaults to `:$PORT`, else `:8080`; with no public URL it falls back to `http://localhost:<port>` and logs a warning; hosted mode logs to stderr.
- `set_opponent_deck` also returns the board link; the board-link instruction line is hosted-only and tool descriptions are unchanged.
- The websocket ping goes to every board client, desktop included.
- App Platform: region `lon`, size `apps-s-1vcpu-0.5gb`, health check `/`; the domain is a placeholder the human task fills in.
- Parity tests live in `internal/mcpserver`, hosted wiring tests in `cmd/tcgreplay`.

### Project-wide rules

- Decides the public address setting: one `-public-url` / `TCGREPLAY_PUBLIC_URL`, scheme and host, the only source of addresses.
- Decides the board link: `board` key on game-opening results equals the public root URL, hosted only (later changed by per-user-games).
- Decides the hosted vs desktop switch: optional fields (`webui.Server.Public`, `mcpserver.Server.BoardURL`) whose zero value leaves the desktop unchanged.
- MCP endpoint is `/mcp` on the same host as the board.
- Relies on game files and the card cache under the container's `$HOME` (game files later removed by memory-store-record).
- Deployment: `.do/app.yaml`, App Platform builds the Dockerfile on push to `main`, one instance; app-spec changes applied by the owner with doctl.
- Tests: `make test`; network tests behind `TCGREPLAY_LIVE=1`. No hand-edited changelog.

### Open for implementation

- Whether claude.ai accepts an authless Streamable HTTP connector. If not, the implementer stops and reports: needing OAuth would pull google-login forward.
- Whether Cloudflare and App Platform pass SSE and the websocket through. The fallback is JSON responses and a shorter ping; any change to the hosting arrangement goes to the user first.

### Manual checks

- The owner imports and reviews a real game end to end from claude.ai on a phone.
- A 30–60 minute review session has no reconnects or failed calls caused by the hosting.
- A move by Claude appears on the deployed board, behind Cloudflare, within 2s.
- Deploying needs only a push to `main`.
- The go/no-go outcome is recorded.
- claude.ai accepts `https://<domain>/mcp` and its tool list matches the desktop's.
- Import with an opponent deck, then move, rewind, branch and annotate through the connector, each read back.
- The board link opens the right board on a phone.
- Left idle 10 minutes behind Cloudflare, the board still shows Claude's next move without a reload.
- Changing the public address and restarting changes the links, with no rebuild.
- The image builds and starts with one command each, and the board loads.
- A push to `main` deploys; a push to any other branch does not.
- In a fresh container, `card_text` returns text and the board shows card images.
- Desktop regression: Claude Code and Claude Desktop launch the playmat locally, connect and import a log.

## 20261017112555-30c99ab1-memory-store-record
### Approach

Every game, imported or started with `new_game`, lives only in the store's existing in-memory mode, on desktop and hosted alike (the spec applies to both). The store's file mode and the manager's `List`/`Load`/`Resume`, `games/*.jsonl`, `<id>.import.json`, `current.txt` and `last-deck.txt` are removed; files already on disk are left alone and never read. Each imported game carries a record on `Game`: versioned JSON with the log, player name, deck, opponent list as passed, seed (decimal string) and a SHA-256 check over the values. `import_log`, `set_opponent_deck` and a new `get_record` tool return it; passing the record as `import_log`'s `log` restores the game. A failed check is retried after undoing curly quotes, non-breaking spaces and CRLF, and the record is refused as damaged unless the check then matches. All 10 corpus logs were verified to re-import identically during planning.

Rejected: a wiped temp directory, leaving file mode unused, events in the record, a separate restore tool.

### Milestones and tasks

Milestone 1: Every imported game comes with a record that brings it back exactly.
- Define the game record format
- Build and restore games from records
- Give the assistant the record through the tools
- Tell the player about the record when importing (import skill)

Milestone 2: The playmat keeps nothing between sessions.
- Keep games in memory only (removes `list_games`, `load_game` and `Resume`; `set_opponent_deck` drops `id`/`new_id` and gains `discard_branches`)
- Remove stored games from the board page
- Point the drills at the game open now
- Describe memory-only games in the design document

### Tasks for a person

None.

### Out of scope

Drilling across many past games; records for `new_game` games; keeping branches, notes or findings across sessions; moving the card cache to memory; per-user games; Notion and skill packaging; migrating or deleting old game files; moving `assistant.txt`; backward compatibility for removed tools and parameters.

### Drafting assumptions

- The record holds the five raw-game fields plus a version marker and a check.
- The seed is a decimal string so 64-bit values survive JSON tools.
- The store's file mode is deleted, not left unused.
- A record is detected in `import_log`'s `log` by a leading `{` and the `tcgreplay_record` marker; a record passed with deck, opp_deck or me is refused.
- The record keeps the opponent list as passed, not the reconciled one.
- `set_opponent_deck` acts on the current game only and refuses a game with branches unless `discard_branches: true`.
- Records land before storage removal, so games can always be brought back.
- Restoring a record sets the session's last deck.
- `assistant.txt` stays on disk; the "nothing written" test excludes it and the process log.

### Project-wide rules

- Decides: no game storage in any mode; later specs must not bring back `Resume` or `load_game`.
- Decides: the record lives on `Game`, the last deck in `Manager` memory.
- Decides the record format `{tcgreplay_record:1, me, seed(string), deck, opp_deck, log, check:"sha256:…"}`.
- Changes web-mcp-docker: its hosted `Resume` call, the `load_game` board link and its instruction mention go, and the container no longer writes game files under `$HOME`.
- Relies on web-mcp-docker's hosted mode, `BoardURL`/`board` switch and two-transport MCP tests; `board` stays on `new_game`, `import_log` and `set_opponent_deck`.
- Tests: `make test`. No hand-edited changelog. Deployment untouched.

### Manual checks

- Success metric: over the owner's first 10 real games, every saved record restores with no differences.
- Success metric: the owner saves records to Notion without hand-editing them.
- After an import, restarting the desktop daemon and the hosted container leaves no game open.
- On a real install, existing game files are unchanged byte for byte and no new game or import files appear.
- The reveal view at start, middle and end of a real game matches after a restart and a record import.
- A record copied into a Notion code block and pasted back restores the same game.
- A damaged paste through a real note app either restores identically or is refused with the damaged message.
- Drills run on the open game, ask for an import when none is open, and never mention listing or loading games.
- With no game open, the board landing page shows no stored-game buttons.

## 20261017123240-21493a95-google-login
### Approach

A new package, `internal/hostauth`, makes the playmat its own small OAuth 2.1 sign-in service for claude.ai and hands the login itself to Google (OpenID Connect through `golang.org/x/oauth2`). Towards claude.ai it serves RFC 9728 resource metadata, RFC 8414 server metadata, Dynamic Client Registration at `/register`, `/authorize` and `/token`, with S256 PKCE required. `/mcp` is protected by the go-sdk's `auth.RequireBearerToken`, with the token's `UserID` set to the Google account ID. The board (`/`, `/ws`, `/api/card`) sits behind a Google sign-in session cookie: signed-out pages redirect through Google, signed-out live connections and card requests get a 401, and signing out closes that session's live connections. All sign-in state is opaque random tokens in memory, so a restart signs everyone out; access tokens last 1 hour, refresh tokens until restart and rotate on use, and a post-restart refresh gets `401 invalid_client` so claude.ai registers and signs in again. Two limits: a per-account request budget (tool error, board message or 429) and a cap on signed-in accounts ("server busy" page). The desktop has no login.

Rejected: Google as the sign-in service directly (no client registration), a third-party OAuth library, JWT tokens, HTTP-only 429 limits, CIMD, reusing the board cookie at `/authorize`.

### Milestones and tasks

Milestone 1: Claude in claude.ai signs in with Google before using the playmat.
- Sign users in with Google
- Serve the connector sign-in endpoints
- Require sign-in for the hosted connector
- Create the Google sign-in client and deployment secrets (human)

Milestone 2: The board needs a Google sign-in, and the user can sign out.
- Guard the board with Google sign-in
- Show the account and Sign out on the board

Milestone 3: Limits keep one account from overloading the server.
- Limit requests per account
- Cap the number of signed-in users
- Document sign-in and limits

### Tasks for a person

- Create the Google sign-in client and deployment secrets: create the Google OAuth web client with redirect `https://<domain>/auth/google/callback` plus the localhost one; publish the consent screen as "In production"; set the two secrets in App Platform; apply the app spec with `doctl apps update`; remove the old authless connector from claude.ai; make sure Cloudflare does not challenge Anthropic's servers (`160.79.104.0/21`) on the sign-in paths.

### Out of scope

Per-account games; profiles or per-user records; billing or quotas beyond the two limits; sign-ins that survive a restart; providers other than Google and account allowlists; signing the connector out from the board; CIMD and Anthropic-held client credentials; a consent page of the playmat's own; idle expiry of sign-ins; any desktop login; packaging the tcg-* skills.

### Drafting assumptions

- Client registration is the only way claude.ai identifies itself.
- Google's ID token is checked for issuer, audience, expiry and nonce without a signature check, since it comes straight from Google's token endpoint over TLS.
- Registered callbacks are limited to the claude.ai and claude.com `/api/mcp/auth_callback` addresses and loopback on any port; every connector sign-in goes through Google's account chooser.
- Signing out on the board ends only the board session.
- Hosted mode refuses to start without Google credentials; there is no bypass flag.
- The user cap counts accounts holding any live sign-in, with no idle expiry (later changed by per-user-games).
- Defaults: 300 requests a minute per account, 10 signed-in accounts; 0 turns either off. The secret is read only from the environment.
- The request limit counts MCP requests, board actions and card lookups, not page or static loads.
- Sign-ins (with email) and refused requests are logged.
- One scope, `playmat`; refresh tokens always issued.
- The health check moves to an unauthenticated `/healthz`.

### Project-wide rules

- Relies on `-public-url` as the only address source: issuer, metadata, the `/mcp` resource and `/auth/google/callback` are built from it.
- Relies on the optional-field hosted switch; adds `webui.Server.Limit`, and `hostauth` is built only in hosted mode.
- Relies on all sign-in state being in memory and lost on restart, secrets from App Platform, and one instance.
- Tests use a fake Google from `internal/hostauth/hostauthtest`; nothing calls the real Google. No hand-edited changelog.
- Changes web-mcp-docker: the `.do/app.yaml` health check moves from `/` to `/healthz`.
- Decides settings `TCGREPLAY_GOOGLE_CLIENT_ID` (`-google-client-id`), `TCGREPLAY_GOOGLE_CLIENT_SECRET` (environment only), `TCGREPLAY_RATE_LIMIT` (`-rate-limit`, 300) and `TCGREPLAY_MAX_USERS` (`-max-users`, 10).
- Decides how later specs read the account: `hostauth.AccountFrom(ctx)` on the board, `hostauth.AccountFromToken(req.Extra.TokenInfo)` on MCP; the account ID is the Google `sub`.

### Open for implementation

Each stops and goes to the user if it goes wrong: whether claude.ai completes the flow (there are reports of it never calling `/token`); whether claude.ai asks for sign-in again after a restart or only shows errors (sign-ins surviving a restart are ruled out by the spec); whether Cloudflare's bot or WAF settings block Anthropic's calls or the cookies.

### Manual checks

- Success metric: the owner connects claude.ai on phone and laptop with one Google sign-in each.
- Success metric: a week of production logs shows zero unsigned requests reaching game actions.
- Success metric: signing back in after a deploy takes under a minute.
- Adding the connector opens Google sign-in; the tools are listed and a real import succeeds.
- Two personal Google accounts each sign in (consent screen published).
- A board link opened signed out on a phone goes through Google and lands on the board.
- A day of connector use and board reloads with no deploy never asks for sign-in.
- After a redeploy, both claude.ai and the board ask for sign-in and then work; record what claude.ai shows.
- Sign out on the board, then a reload, asks for sign-in.
- With limits set low on the deployed app, "too many requests" reaches the user through Claude and on the board, and a second account sees "server busy".
- Changing credentials or limits in App Platform and restarting takes effect with no rebuild.
- Claude Code and Claude Desktop still use the local playmat with no sign-in.
- Cookies and redirects work through Cloudflare and App Platform on the deployed domain.

## 20261017143118-b05aa6b1-per-user-games
### Approach

A new hosted-only package, `internal/usergames`, holds a registry with one entry per Google account: its own `game.Manager`, its own `webui.Server`, a stable board handle, the time of last activity, and whether it holds one of the server's places. Claude's tool calls go through one shared `*mcp.Server`; every call looks up the caller's own game from its token through a new optional `mcpserver.Server.Resolve` hook in the single `add` helper. Board routes, behind google-login's session check, go to the signed-in account's own board server, so live updates are scoped by construction and the hub is unchanged. Idle games expire with a notice and free their place. The desktop is untouched because every new hook is nil there.

Rejected: one `*mcp.Server` per account (a claude.ai session stays tied to the server it started on), a shared hub tagging tabs by account, deleting the entry on expiry, keeping the place count in hostauth.

### Milestones and tasks

Milestone 1: Every signed-in account has its own game and its own board.
- Keep one game per account
- Route Claude's tool calls to the caller's own game
- Give each account its own board (owner-only "not your game" page)
- Turn on per-account games in hosted mode
- Show the player's own name on the hosted board

Milestone 2: Idle games are cleared and free their place.
- Let a game expire with a notice
- Drop games left idle (`-idle-timeout`, background sweep, check on every lookup)
- Free a place when a game expires (user cap moves into the registry)
- Document per-account games and the idle drop
- Apply the updated app definition to the deployed app (human)

### Tasks for a person

- Apply the updated app definition to the deployed app: needs the owner's App Platform account to run `doctl apps update`.

### Out of scope

Saving games across restarts or the idle drop; sharing or spectating another account's board; more than one instance; warnings or a countdown before expiry; per-account limits beyond google-login's two; idle expiry of sign-ins; Google profile names; any desktop change; packaging the skills.

### Drafting assumptions

- `hostauth.Config.MaxUsers` is replaced by `Config.Places` (`Admit`, `Release`); the setting, default 10, `0` = off, `ErrBusy` and the busy page are unchanged.
- `-idle-timeout` / `TCGREPLAY_IDLE_TIMEOUT` is a Go duration, default 60m; `0` disables it; an invalid value stops start-up.
- On expiry the entry is kept and only the game dropped; its place is released; placeless entries are purged after 24 hours or when the account's last sign-in ends.
- After expiry, actions report it and ask for the saved record; `import_log` and `new_game` go ahead and clear the notice.
- Own name: the game's name for the seat, else the email's local part.
- The caller's game is looked up on every tool call.
- Activity is tool calls (reads included), board actions and board page loads; websocket connects and card lookups do not count.
- The board link is `<public>/?board=<handle>`, the handle a hash of the Google account ID, stable across restarts; `/` with no handle opens your own board.
- A failed lookup (busy, no account) is a tool error result, not a protocol error.

### Project-wide rules

- Reuses web-mcp-docker's hosted mode, one router and `-public-url`; adds `mcpserver.Server.Resolve` and `hostauth.Config.Places` as hosted-only switches.
- Changes web-mcp-docker's board link: `board` becomes `<public>/?board=<owner handle>`, still hosted-only, on `new_game`, `import_log` and `set_opponent_deck`.
- Changes google-login's user cap: it counts places, not signed-in accounts; a place is taken at sign-in or on the first action after losing one, and freed by idle expiry, the last sign-in ending, or a restart. Counting lives in the registry.
- Reuses google-login's identity rules (`AccountFrom`, `AccountFromToken`, Google `sub`).
- Keeps memory-store-record's no-storage rule; the record and last decklist are now per account.
- Exactly one instance, now documented in `.do/app.yaml`.
- New setting `-idle-timeout` / `TCGREPLAY_IDLE_TIMEOUT`.
- Tests: `make test`, stdlib `testing` with `httptest` and an injected clock; hosted tests use `hostauthtest`. No hand-edited changelog.
- Shared interfaces: `hostedHandler(srv, reg *usergames.Registry, a *hostauth.Auth)`; `game.Manager` gains `Expire`, `Notice`, `OnClear` and `ErrExpired`; the board state message gains an optional `notice`.

### Manual checks

- Success metric: two real Google accounts use the server at once for a full session through claude.ai and the board, and neither sees the other's game.
- Success metric: a week of App Platform memory metrics with the idle drop on stays within the container's allowance.
- Same account on two devices: a move from the phone shows on the laptop board and in a laptop conversation's next read; an import on one replaces the game on the other.
- Account B opening A's link sees "not your game" and no cards; a signed-out private window is asked to sign in.
- With the idle time at 2 minutes on the deployed app, Claude and the board report the expired game; an open tab alone does not keep it; acting every minute for 5 minutes does.
- With the user cap at 1, an expired game frees the place.
- Each account's board labels the player's side with their own name.
- Desktop regression: Claude Code and Claude Desktop keep one game, no sign-in, and the root board link.
