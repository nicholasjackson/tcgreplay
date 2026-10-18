DONE summary for 20261017143118-b05aa6b1-per-user-games (kept for Step 6 of plan-epic)

Approach: new hosted-only package internal/usergames: registry with one entry per Google account; each entry has its own game.Manager, own webui.Server (webui.New), stable board handle, last-activity time, whether it holds a place. Tool calls go through one shared *mcp.Server; each tool call resolves caller's game from token via new optional mcpserver.Server.Resolve hook in the single `add` helper. Board per account: routes behind google-login's RequireSession go to the account's own board server; live updates scoped by construction, hub unchanged. Rejected: one *mcp.Server per account (claude.ai session tied to server, keeps acting on dropped game); shared hub tagging tabs (hub rewrite); deleting entry on expiry (tabs can't show expiry); place count in hostauth (second structure). Desktop untouched: hooks nil.

M1 Every signed-in account has its own game and board:
- Keep one game per account
- Route Claude's tool calls to the caller's own game
- Give each account its own board (owner-only "not your game"; page load counts as activity, open ws does not)
- Turn on per-account games in hosted mode (wiring, e2e tests with two accounts)
- Show the player's own name on the hosted board
M2 Idle games cleared and free their place:
- Let a game expire with a notice (manager expiry state + board notice)
- Drop games left idle (-idle-timeout, background sweep, check on every lookup)
- Free a place when a game expires (user cap moves into registry)
- Document per-account games and idle drop
- HUMAN TASK: Apply the updated app definition to the deployed app (doctl apps update)

Out of scope: saving games across restarts/idle drop; sharing/spectating; >1 instance; warnings/countdown before expiry; per-account limits beyond google-login's two; idle expiry of sign-ins; Google profile names; desktop change; skills packaging.

Drafting assumptions: per-account registry of managers & board servers, Resolve hook, Places interface in hostauth config; cap counting moves to registry, hostauth.Config.MaxUsers replaced by Config.Places (Admit/Release), setting/default 10/0=off/ErrBusy/busy page unchanged; -idle-timeout / TCGREPLAY_IDLE_TIMEOUT Go duration default 60m, 0 disables, invalid stops start-up; on expiry entry kept, game dropped, place released, placeless entries purged after 24h or when last sign-in ends; after expiry actions report expiry and ask for record, import_log/new_game proceed and clear notice; own name = game's name for seat else email local part; caller's game looked up every tool call; activity = tool calls (incl reads), board actions, page loads, not ws connect/reconnect or card lookups; board link <public>/?board=<handle>, handle = hash of Google account id, stable across restarts; / with no handle opens own board; resolve failures return tool error result not protocol error; person applies app definition (push doesn't re-apply); cap move lands with idle drop.

Project-wide rules:
- Hosted mode & routing: reuses web-mcp-docker's -hosted, one router, -public-url only address source.
- Hosted-only switches: optional fields, zero = desktop unchanged; adds mcpserver.Server.Resolve and hostauth.Config.Places.
- Board link (DELIBERATE CHANGE from web-mcp-docker): `board` = <public>/?board=<owner handle>; still hosted-only on new_game, import_log, set_opponent_deck.
- User cap (CHANGES google-login): counts places not signed-in accounts; place taken at sign-in or first action after losing one; freed by idle expiry, last sign-in ending, restart; counting in usergames registry via Places; same setting/default/ErrBusy/busy page; sign-ins still last until sign-out/restart.
- Identity: reuses google-login AccountFrom(ctx), AccountFromToken(TokenInfo), UserID = Google sub; own AS, board cookie, in-memory sessions, request limit unchanged.
- Storage: no game storage any mode; Resume/load_game stay gone; record on Game, last decklist in manager — now per account.
- Instances: exactly one, now documented in .do/app.yaml; push to main, app-spec changes via doctl apps update.
- New setting: -idle-timeout / TCGREPLAY_IDLE_TIMEOUT.
- Tests: make test, stdlib testing + httptest, injected clock; hosted tests use hostauthtest.
- Changelog: none hand-edited.
- Shared interfaces changed: hostedHandler(srv, reg *usergames.Registry, a *hostauth.Auth); game.Manager gains Expire, Notice, OnClear, ErrExpired; board state message gains optional notice field.

Manual checks:
- Success metric: two real Google accounts concurrently for full session via claude.ai and board, neither sees other's game (also automated).
- Success metric: a week of App Platform memory metrics with idle drop on stays within allowance.
- Same account two devices: phone move shows on laptop board and laptop conversation's next read; import on one replaces game on other.
- Owner-only link: account B opening A's link sees "not your game", no cards; signed-out private window asked to sign in, no cards.
- Idle drop deployed with idle 2 min: Claude and board report expiry; open tab alone doesn't keep it; acting every minute for 5 min keeps it.
- Expired game frees a place with cap temporarily 1.
- Own name: each account's board labels player's side with own name.
- Desktop regression: Claude Code & Desktop keep one game, no sign-in, root board link.

Discoveries:
- go-sdk v1.6.0 Streamable handler calls getServer only at session creation; per-user state must be looked up per request via RequestExtra.TokenInfo (mcp/streamable.go:1142-1164). Tags: mcp, go-sdk, hosted, multi-tenant.
- Board client sends no verb on its own and reconnects ws itself; only verbs & page loads are reliable activity signals. Tags: webui, board, activity.
