---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Context: 20261017123240-21493a95-google-login

## Current State Analysis

- **Hosted mode is planned, not built.** At commit 77e6a6a the binary has three modes — stdio shim (default), `-daemon`, `-no-daemon` (`cmd/tcgreplay/main.go:86-93`) — and no HTTP MCP transport. Plan `20261017105035-e3f64c1b-web-mcp-docker` adds `-hosted`, `cmd/tcgreplay/hosted.go` (`hostedListen`, `hostedPublicURL`, `hostedHandler(srv, ui)`, `runHosted`, `newHosted`), `cmd/tcgreplay/hosted_test.go`, `webui.Server.Public`, `mcpserver.Server.BoardURL`, `Dockerfile`, `.do/app.yaml` (health check on `/`, `TCGREPLAY_PUBLIC_URL=${APP_URL}`) and Makefile docker targets. This plan's tasks start from that code.
- **No authentication anywhere.** The board is guarded only by loopback `Host` and origin checks (`internal/webui/server.go:87-102`, `internal/webui/hub.go:162-164`); web-mcp-docker lifts these in hosted mode, leaving the hosted board and `/mcp` open by design for its trial.
- **Board request path.** `webui.Server.Handler()` (`internal/webui/server.go:65-71`) routes `GET /ws`, `GET /api/card`, `GET /` (embedded Svelte app). `serveWS` (`internal/webui/hub.go:161-213`) accepts the websocket, derives its context from `r.Context()` (`:168`) and sends every verb through `s.dispatch(r.Context(), c, req)` (`:206`, `internal/webui/actions.go:83`), whose `response.Error` the client shows as a toast (`web/src/lib/ws.js` `act`).
- **MCP server.** `mcpserver.Server.MCP()` (`internal/mcpserver/server.go:28-45`) builds one `*mcp.Server`; tool handlers never see HTTP. The go-sdk v1.6.0 delivers `RequestExtra.TokenInfo` to every server request (`mcp/shared.go:485-488`) and binds Streamable HTTP sessions to `TokenInfo.UserID` (`mcp/streamable.go:303-310, 494-498`).
- **Board client.** `web/src/main.js` calls `connect()` then mounts `App.svelte`; `ws.js` reconnects with backoff on close and reloads state on connect; the header (`web/src/App.svelte:33-56`) holds the connection dot, game, branch, toggles and buttons. No front-end tests exist.
- **Dependencies.** `go.mod`: go 1.27.2, go-sdk v1.6.0 direct; `golang.org/x/oauth2 v0.35.0` indirect (pulled by the go-sdk), so Google's endpoints (`golang.org/x/oauth2/endpoints`) need no new download.
- **Tests.** Go `testing` + `httptest`; `make test` = `go vet ./...` + `go test ./...`; live network tests gated by `TCGREPLAY_LIVE=1`. Board tests use `setup(t)` (`internal/webui/webui_test.go:78`); desktop guard test `TestRejectsForeignHostAndOrigin` (`:254-295`).
- **Knowledge and designs.** The `tcgreplay` knowledge store is empty; the spec references no design documents.

## Per-Task Technical Notes

Requirement-to-repo resolution: every requirement lands in the single registered repo `tcgreplay` (root `/home/nicj/code/github.com/nicholasjackson/tcgreplay`). Paths marked "(web-mcp-docker)" are created by plan `20261017105035-e3f64c1b-web-mcp-docker`, which must be implemented first; they have no line numbers yet, so entries name the function instead.

| Requirement | Task(s) | Files |
|---|---|---|
| Sign-in required when hosted | Require sign-in for the hosted connector; Guard the board with Google sign-in | `cmd/tcgreplay/hosted.go`, `internal/hostauth/oauth.go`, `internal/hostauth/board.go` |
| Any Google account | Sign users in with Google; Create the Google sign-in client and deployment secrets | `internal/hostauth/google.go` |
| Connector sign-in | Serve the connector sign-in endpoints; Require sign-in for the hosted connector | `internal/hostauth/oauth.go`, `cmd/tcgreplay/hosted.go` |
| Board sign-in, Live board guarded | Guard the board with Google sign-in; Show the account and Sign out on the board | `internal/hostauth/board.go`, `web/src/lib/ws.js` |
| Signed in until restart | Sign users in with Google; Serve the connector sign-in endpoints; Guard the board with Google sign-in | `internal/hostauth/store.go` |
| Sign out | Guard the board with Google sign-in; Show the account and Sign out on the board | `internal/hostauth/board.go`, `web/src/App.svelte` |
| Per-account request limit | Limit requests per account | `internal/hostauth/limit.go`, `internal/webui/server.go`, `internal/webui/hub.go` |
| Signed-in user limit | Cap the number of signed-in users | `internal/hostauth/limit.go`, `internal/hostauth/google.go` |
| Operator settings | Require sign-in for the hosted connector; Limit requests per account; Cap the number of signed-in users | `cmd/tcgreplay/main.go`, `cmd/tcgreplay/hosted.go`, `.do/app.yaml` |

### Task: Sign users in with Google

**File changes**:
- `internal/hostauth/hostauth.go` (new) — package doc ("sign-in for the hosted playmat: an OAuth authorization server for MCP clients that delegates login to Google, and board sessions; everything in memory"). `Config` as in plan.md's Data Structures (`Public *url.URL`, `GoogleClientID`, `GoogleSecret`, `RateLimit`, `MaxUsers`, `Google oauth2.Endpoint` defaulting to `endpoints.Google` from `golang.org/x/oauth2/endpoints`, plus unexported `now func() time.Time` and `httpClient *http.Client` settable by tests through options). `Account{ID, Email string}`. `New(cfg Config) (*Auth, error)`: validate `Public` (scheme + host), client ID and secret non-empty. `(*Auth).Routes(mux *http.ServeMux)` registers routes owned by each later task; this task registers `GET /auth/google/callback` and `GET /healthz` (`200 ok`, `Cache-Control: no-store`). `AccountFrom(ctx)` / `withAccount(ctx, Account)` context helpers.
- `internal/hostauth/store.go` (new) — `store` with one `sync.Mutex` and maps: `pending map[string]*pendingLogin` (10 min), `sessions map[string]*session` (until restart), `accounts map[string]*accountState` (ID → email, count of live sign-ins: sessions + refresh tokens), later tasks add clients/codes/tokens/buckets. Keys of token-like values are `sha256` hex of the random value; random values are 32 bytes from `crypto/rand`, base64url (`rand.Text()` is acceptable). `sweep(now)` deletes expired entries, called at the start of every write. `session` has `id, account, done chan struct{}` (closed when it ends) — used by the board task.
- `internal/hostauth/google.go` (new) —
  - `startGoogle(w, r, p *pendingLogin)`: store `p` (fields: `kind` = `loginBoard`|`loginConnector`, `next string`, connector fields filled by the OAuth task, `nonce`, PKCE `verifier` via `oauth2.GenerateVerifier()`), set cookie `tcgreplay_login` = pending id (`HttpOnly`, `SameSite=Lax`, `Path=/auth/google/callback`, `Max-Age=600`, `Secure` when `Public.Scheme == "https"`), redirect 302 to `oauthCfg.AuthCodeURL(id, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("nonce", nonce), oauth2.SetAuthURLParam("prompt", "select_account"))`. `oauthCfg`: `ClientID`, `ClientSecret`, `Endpoint: cfg.Google`, `RedirectURL: Public + "/auth/google/callback"`, `Scopes: ["openid", "email"]`.
  - `serveGoogleCallback`: require `state` query == `tcgreplay_login` cookie value and a live pending entry (consume it, clear the cookie); on `error` query (user cancelled) render the error page; `Exchange(ctx, code, oauth2.VerifierOption(p.verifier))` with a 10 s timeout and `a.httpClient` via `oauth2.HTTPClient` context key; read `tok.Extra("id_token")`, split JWT, base64url-decode the payload and check `iss` ∈ {`https://accounts.google.com`, `accounts.google.com`}, `aud` == client ID (string or array), `exp` > now, `nonce` == `p.nonce`, `sub` non-empty (no signature check: received directly over TLS, OIDC Core §3.1.3.7). Then `a.signIn(w, Account{ID: sub, Email: email})`: create a board session, set cookie `tcgreplay_session` (`HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` on https, no `Max-Age`), log `hostauth: signed in <email>`. Continue: `loginBoard` → 302 to `p.next` (validated earlier as a local path starting with `/` and not `//`); `loginConnector` → the OAuth task's continuation (leave a `continueConnector` hook that returns 500 until then).
  - Pages (`pages.go`, new): minimal `html/template` pages — `errorPage(w, status, msg)`, later `signedOutPage`, `busyPage`. All with `Cache-Control: no-store`.
- `internal/hostauth/hostauthtest/google.go` (new, test helper package, not imported by production code) — `NewGoogle(t) *FakeGoogle`: an `httptest.Server` with `GET /authorize` that, given `?login_hint=` or the fake's `Account` field (default `{ID: "sub-1", Email: "one@example.com"}`), immediately redirects to `redirect_uri?code=<random>&state=<state>` remembering the code's `nonce`, `code_challenge` and account; `POST /token` that checks client ID/secret, `redirect_uri`, the PKCE verifier against the challenge, and returns `{"access_token":"x","token_type":"Bearer","expires_in":3600,"id_token":<unsigned JWT with iss https://accounts.google.com, aud client ID, sub, email, nonce, exp>}`. `Endpoint() oauth2.Endpoint`. A `SetAccount(Account)` method switches the next sign-in's account. `Browser(t) *http.Client` returns a client with a cookie jar that follows redirects except to non-test hosts.
- `internal/hostauth/google_test.go` (new) — board-kind sign-in through fake Google ends with a `tcgreplay_session` cookie and a redirect to `next`; mismatched state cookie, consumed state, wrong `aud`, wrong `iss`, wrong nonce and expired `exp` (via injected clock) are refused; Google URL host and `redirect_uri` come from `Public`, not the request `Host`; a fresh `New(...)` does not recognise the previous instance's session id; `next=//evil.example` is rejected.
- `go.mod` — move `golang.org/x/oauth2 v0.35.0` from the indirect block to the direct `require` (run `go mod tidy`).

**Complexity**: Medium
**Token estimate**: ~40k tokens
**Agent strategy**: Single agent, sequential execution (store, Google client, fake Google, tests).

### Task: Serve the connector sign-in endpoints

**File changes**:
- `internal/hostauth/oauth.go` (new) —
  - Metadata: `GET /.well-known/oauth-protected-resource` and `GET /.well-known/oauth-protected-resource/mcp` → `auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{Resource: Public + "/mcp", AuthorizationServers: []string{Public}, ScopesSupported: []string{"playmat"}, BearerMethodsSupported: []string{"header"}, ResourceName: "tcgreplay playmat"})` (go-sdk `auth/auth.go:154`). `GET /.well-known/oauth-authorization-server` and `GET /.well-known/openid-configuration` → JSON (own struct with RFC 8414 field names, omitting `jwks_uri`): `issuer` = Public (no trailing slash), `authorization_endpoint` /authorize, `token_endpoint` /token, `registration_endpoint` /register, `response_types_supported ["code"]`, `grant_types_supported ["authorization_code","refresh_token"]`, `code_challenge_methods_supported ["S256"]`, `token_endpoint_auth_methods_supported ["none","client_secret_post","client_secret_basic"]`, `scopes_supported ["playmat"]`. Do **not** list `offline_access` or `client_id_metadata_document_supported`. CORS `*` on all four.
  - `POST /register` (JSON, `oauthex.ClientRegistrationMetadata` shape, `oauthex/dcr.go:27`): require ≥1 `redirect_uris`, each allowed by `allowedRedirect(u)`: exactly `https://claude.ai/api/mcp/auth_callback` or `https://claude.com/api/mcp/auth_callback`, or `http` with host `localhost` / `127.0.0.1` / `[::1]` on any port and any path. `token_endpoint_auth_method` default `none`; if `client_secret_post`/`client_secret_basic` issue a secret. Respond 201 with `client_id`, optional `client_secret` (+ `client_secret_expires_at: 0`), `client_id_issued_at`, the echoed metadata. Error 400 `{"error":"invalid_redirect_uri"}` / `invalid_client_metadata`. Store `client{id, secretHash, redirects, authMethod, created}`; cap 1000 clients, evicting the oldest. Body limit 64 KiB.
  - `GET /authorize`: look up `client_id` (unknown → 400 error page, never redirect); `redirect_uri` must exactly match a registered one, except loopback where the port is ignored (RFC 8252 §7.3) (mismatch → 400 page); then errors redirect to the client with `error=`: `response_type` must be `code` (`unsupported_response_type`), `code_challenge` present and `code_challenge_method=S256` (`invalid_request`), `resource` if present must equal `Public + "/mcp"` (`invalid_target`). Then `startGoogle` with a `loginConnector` pending entry holding client id, redirect, client `state`, challenge, scope.
  - `continueConnector(w, r, p, acct)`: issue a code (`codes` map, 5 min, single use) bound to client, redirect, challenge and account; 302 to `redirect_uri?code=..&state=..` (preserving existing query on the redirect).
  - `POST /token` (form-encoded, `r.ParseForm`): authenticate the client (public: `client_id` form value; confidential: Basic or `client_secret_post`, constant-time compare of hashes); unknown client → 401 `{"error":"invalid_client"}` with `WWW-Authenticate: Basic`. `authorization_code`: code exists, unexpired, unused, same client and `redirect_uri`, `base64url(sha256(code_verifier)) == challenge` → else `invalid_grant`; delete the code. `refresh_token`: known, same client → rotate (delete old, issue new) → else `invalid_grant`. Issue access token (1 h) + refresh token (until restart), both bound to account and client; refresh tokens count as a live sign-in for the account (decrement when rotated away is net zero). Response JSON per plan.md, `Cache-Control: no-store`, `Pragma: no-cache`. Unknown grant → `unsupported_grant_type`.
  - `(*Auth).RequireToken(next)` = `auth.RequireBearerToken(a.verify, &auth.RequireBearerTokenOptions{ResourceMetadataURL: Public + "/.well-known/oauth-protected-resource/mcp", Scopes: []string{"playmat"}})(next)`, wrapped in a small handler that logs refusals (`hostauth: refused <method> <path>: <reason>`, never the token) by capturing a 401/403 status. `verify(ctx, token, r)`: look up `sha256(token)`; missing or expired → `auth.ErrInvalidToken`; return `&auth.TokenInfo{Scopes: ["playmat"], Expiration: exp, UserID: acct.ID, Extra: {"email": acct.Email}}` (go-sdk `auth/auth.go:21-41`; `UserID` binds the MCP session, `mcp/streamable.go:303-310`).
  - `AccountFromToken(ti *auth.TokenInfo) (Account, bool)`.
- `internal/hostauth/oauth_test.go` (new) — table tests for every rule above using `httptest` and the fake Google; plus `TestConnectorFlowWithSDKClient`: serve `Routes` + `RequireToken(echo handler or mcp handler)` on `httptest.NewServer` with `Public` = the server URL; drive `auth.NewAuthorizationCodeHandler` (`go-sdk auth/authorization_code.go:67-175`) with `DynamicClientRegistrationConfig` (redirect `http://127.0.0.1/callback`) and an `AuthorizationCodeFetcher` that GETs the auth URL with `FakeGoogle.Browser`, stopping at the loopback redirect and returning its `code`/`state`; connect `mcp.StreamableClientTransport{Endpoint: ts.URL + "/mcp", OAuthHandler: h}` to a tiny `mcp.Server` and call a tool. Restart test: a second `New` with the same config refuses the first one's access token (401) and answers its refresh with 401 `invalid_client`. Expiry test with the injected clock: access token refused after 1 h, refresh succeeds and the old refresh token then gets `invalid_grant`. If `AuthorizationCodeHandler` cannot run against `httptest` (HTTPS checks), drive the same steps with plain `net/http` and note it in the test.
- `internal/hostauth/hostauthtest/client.go` (new) — `MCPTransport(t, baseURL string, g *FakeGoogle) mcp.Transport` (the SDK flow above, reusable by `cmd/tcgreplay` tests) and `BearerToken(t, baseURL, g) string` (manual flow returning an access token).

**Complexity**: High
**Token estimate**: ~60k tokens
**Agent strategy**: Parallel analysis, sequential integration — one agent writes metadata + registration + authorize, then token endpoint + verifier, then the SDK-driven end-to-end test; a second agent may draft the table tests against the stated rules in parallel.

### Task: Require sign-in for the hosted connector

**File changes**:
- `cmd/tcgreplay/main.go:35-39` — add `googleClientID string; rateLimit, maxUsers int` to `config` (rate limit and cap fields are used by Milestone 3 tasks; add the flags now with their defaults so settings land together). `config.args()` (`:42-44`) unchanged (daemon only).
- `cmd/tcgreplay/main.go:54-65` — flags: `-google-client-id` (default `os.Getenv("TCGREPLAY_GOOGLE_CLIENT_ID")`), `-rate-limit` (default `envInt("TCGREPLAY_RATE_LIMIT", 300)`), `-max-users` (default `envInt("TCGREPLAY_MAX_USERS", 10)`). The secret is read only from `TCGREPLAY_GOOGLE_CLIENT_SECRET` inside `runHosted`.
- `cmd/tcgreplay/hosted.go` (web-mcp-docker) —
  - `hostedAuth(c config, public *url.URL) (*hostauth.Auth, error)`: builds `hostauth.Config` from `c` and the env secret; missing ID or secret → error `hosted mode needs Google sign-in: set TCGREPLAY_GOOGLE_CLIENT_ID and TCGREPLAY_GOOGLE_CLIENT_SECRET`.
  - `runHosted`: call `hostedAuth` after resolving the public URL and before listening; return its error (start-up fails, `log.Fatal` in `main`).
  - `hostedHandler(srv, ui, a)`: `mux := http.NewServeMux(); a.Routes(mux); mux.Handle("/mcp", a.RequireToken(streamable)); mux.Handle("/", ui.Handler())` (the board guard is added by the board task). Keep the Streamable options from web-mcp-docker.
  - `newHosted(...)` helper (web-mcp-docker) gains the `*hostauth.Auth` so tests and production wire identically.
  - `envInt(key string, def int) int` helper (invalid value → log and use default).
- `cmd/tcgreplay/hosted_test.go` (web-mcp-docker) — build the handler with `hostauthtest.NewGoogle(t)`'s endpoint and `Public` = test server URL (start the `httptest.Server` unstarted or use a handler indirection so the URL is known before `hostauth.New`); replace every `StreamableClientTransport{Endpoint: ...}` with `hostauthtest.MCPTransport(t, ts.URL, g)`. Add: anonymous `POST /mcp` initialize → 401 with `WWW-Authenticate` containing `resource_metadata="<ts.URL>/.well-known/oauth-protected-resource/mcp"`; tool-list parity and `import_log` work signed in; two accounts (`g.SetAccount`) both connect; `GET /healthz` → 200 without sign-in; `hostedAuth` refuses a missing ID or secret; `envInt` parsing.
- `.do/app.yaml` (web-mcp-docker) — `health_check.http_path: /healthz`; add envs `TCGREPLAY_GOOGLE_CLIENT_ID` and `TCGREPLAY_GOOGLE_CLIENT_SECRET` with `scope: RUN_TIME`, `type: SECRET` and no value (comment: set in the App Platform control panel), and `TCGREPLAY_RATE_LIMIT: "300"`, `TCGREPLAY_MAX_USERS: "10"` (`scope: RUN_TIME`).
- `Makefile` — the web-mcp-docker `docker-run` target passes `-e TCGREPLAY_GOOGLE_CLIENT_ID -e TCGREPLAY_GOOGLE_CLIENT_SECRET` through from the caller's environment.

**Complexity**: Medium
**Token estimate**: ~35k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Create the Google sign-in client and deployment secrets

**What the person does**:
1. In Google Cloud console (any project the owner controls): APIs & Services → OAuth consent screen: user type **External**, app name "tcgreplay", support email, scopes `openid` and `email` only; **Publish app** so the status is "In production" (these scopes are non-sensitive, so no verification is needed; "Testing" would limit sign-in to listed test users).
2. Credentials → Create OAuth client ID → **Web application**; Authorised redirect URIs: `https://<domain>/auth/google/callback` and `http://localhost:8080/auth/google/callback` (local container). Note the client ID and secret.
3. In the App Platform app → Settings → the `playmat` component's environment variables: set `TCGREPLAY_GOOGLE_CLIENT_ID` and `TCGREPLAY_GOOGLE_CLIENT_SECRET` (encrypt the secret). Save; App Platform redeploys.
4. Apply the updated app definition (new secret keys, limits, `/healthz` health check) with `doctl apps update <app-id> --spec .do/app.yaml` — a push alone does not re-apply the app spec — then set the secret values as in step 3 if the update cleared them.
5. Remove the authless connector added during the web-mcp-docker trial from claude.ai, so the next add discovers sign-in.
6. In Cloudflare for the domain: check Security → Bots/WAF do not challenge requests to `/.well-known/*`, `/register`, `/token`, `/mcp` (Anthropic's egress is `160.79.104.0/21`); add a WAF skip rule for those paths if Bot Fight Mode or a challenge is enabled.

**How it is checked**: the deployed app starts (the health check passes, so the credentials were found); `https://<domain>/.well-known/oauth-authorization-server` returns the metadata with the domain as issuer from outside; opening `https://<domain>/` (once the board task is deployed) or adding the connector shows Google's account chooser for "tcgreplay" and a second personal account can sign in.

### Task: Guard the board with Google sign-in

**File changes**:
- `internal/hostauth/board.go` (new) —
  - `GET /auth/login?next=<path>`: validate `next` (local path, default `/`), `startGoogle` with a `loginBoard` pending entry.
  - `(*Auth).RequireSession(next http.Handler) http.Handler`: read `tcgreplay_session`, look up the session. Missing/unknown: for `/ws` and paths under `/api/` → 401 `sign in required` (logged as a refusal); otherwise (`GET`/`HEAD` page or asset) → 302 `/auth/login?next=<r.URL.RequestURI()>` with `Cache-Control: no-store`. Known: `ctx := withAccount(r.Context(), s.account)`; for `/ws` derive `ctx, cancel := context.WithCancel(ctx)` and `stop := context.AfterFunc`-style watch of `s.done` (goroutine `select { case <-s.done: cancel(); case <-ctx.Done(): }`), so ending the session closes the websocket (its parent context is `r.Context()`, `internal/webui/hub.go:168`); then `next.ServeHTTP(w, r.WithContext(ctx))`.
  - `GET /auth/session`: known session → `200 {"email": ...}`; else 401. `Cache-Control: no-store`.
  - `POST /auth/logout`: require `Origin` (or `Referer`) host == `Public.Host` (else 403); end the session (`close(done)`, delete, decrement the account's live sign-ins), expire the cookie (`Max-Age=-1`), 303 to `/auth/signed-out`.
  - `GET /auth/signed-out`: page "You have signed out of tcgreplay." with a "Sign in again" link to `/auth/login?next=/`.
- `internal/hostauth/google.go` — `signIn` already creates the board session for both flows (the connector flow's browser therefore also gets the board cookie).
- `cmd/tcgreplay/hosted.go` (web-mcp-docker, `hostedHandler`) — `mux.Handle("/", a.RequireSession(ui.Handler()))`; `/auth/*`, `/.well-known/*`, `/register`, `/authorize`, `/token`, `/healthz` stay outside (more specific patterns registered by `a.Routes`).
- `internal/hostauth/board_test.go` (new) — signed-out `GET /` → 302 to `/auth/login?next=%2F`, then through fake Google lands on `/` with a cookie; `GET /?x=1` keeps its query in `next`; signed-out `/ws` and `/api/card` → 401; `/auth/session` 200/401; logout without matching Origin → 403; logout ends the session (a handler blocked on the request context returns) and the next `GET /` redirects; a new `Auth` (restart) redirects the old cookie.
- `cmd/tcgreplay/hosted_test.go` (web-mcp-docker) — the board tests (`TestHostedBoardFollowsClaude`, `TestHostedBoardMoveReachesClaude`, links/`GET /` checks) obtain a session cookie via `hostauthtest` (add `BoardCookie(t, baseURL, g) *http.Cookie` to `internal/hostauth/hostauthtest/client.go`) and send it on the websocket dial (`websocket.DialOptions.HTTPHeader`); add: websocket dial without cookie fails with 401 and a board `move` verb cannot be sent; after `POST /auth/logout` the open websocket closes within 2 s.
- `internal/webui/webui_test.go:254-295` — unchanged (desktop guard stays).

**Complexity**: Medium
**Token estimate**: ~40k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Show the account and Sign out on the board

**File changes**:
- `web/src/lib/session.js` (new) — `export async function loadSession()`: `fetch('/auth/session', {credentials: 'same-origin', cache: 'no-store'})`; 200 → set `app.account = (await r.json()).email`; 401 → return `'signed-out'`; any other status (desktop 404) → `app.account = null`, return `'none'`. `export function signIn()`: `location.assign('/auth/login?next=' + encodeURIComponent(location.pathname + location.search))`.
- `web/src/lib/store.svelte.js` — add `account: null` to the `app` state.
- `web/src/main.js:1-7` — call `loadSession()` before `connect()` (don't block mount).
- `web/src/lib/ws.js:17-23` (`sock.onclose`) — before scheduling the reconnect, `loadSession()`; if it returns `'signed-out'` call `signIn()` and do not reconnect; otherwise reconnect with backoff as today.
- `web/src/App.svelte:33-56` (header) — after the timeline toggle: `{#if app.account}<span class="account">{app.account}</span><form method="POST" action="/auth/logout"><button type="submit">Sign out</button></form>{/if}`; a little CSS for `.account` matching the header's muted text.
- `internal/webui/dist/app` — rebuilt by `make build` (the Vite build), not edited by hand.

**Complexity**: Low
**Token estimate**: ~12k tokens
**Agent strategy**: Single agent, sequential execution; verify by building with `make build` and loading the board in hosted mode locally against the fake or real Google.

### Task: Limit requests per account

**File changes**:
- `internal/hostauth/limit.go` (new) — `bucket{tokens float64; last time.Time}` per account in the store; `allow(accountID) error`: refill at `RateLimit/60` per second up to `RateLimit` (burst), take one; empty → `fmt.Errorf("%w: this account is limited to %d requests a minute; try again in %ds", ErrRateLimited, RateLimit, wait)`, logged once per 10 s per account. `RateLimit == 0` → always allow. `var ErrRateLimited = errors.New("too many requests")`.
  - `(*Auth).MCPMiddleware() mcp.Middleware`: `func(next mcp.MethodHandler) mcp.MethodHandler` reading `req.GetExtra().TokenInfo` (go-sdk `mcp/shared.go:485-488, 518`); no token → pass (the HTTP layer already refused anonymous calls); over limit: for `tools/call` return `&mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil`; for other methods return the error (a JSON-RPC error with the message). `notifications/*` are not counted.
  - `(*Auth).BoardLimit(ctx) error`: `AccountFrom(ctx)` → `allow`; no account → nil.
  - In `RequireSession`, count `/api/` requests with `allow`; over limit → 429 with the message as plain text.
- `internal/webui/server.go:28-34` — add `Limit func(ctx context.Context) error` to `Server` with its doc comment (plan.md Data Structures).
- `internal/webui/actions.go:83-89` (`dispatch`) — first: `if s.Limit != nil { if err := s.Limit(ctx); err != nil { res.Error = err.Error(); return res } }`. `ctx` is the websocket request's context (`internal/webui/hub.go:206`), which carries the account.
- `cmd/tcgreplay/hosted.go` (web-mcp-docker, `runHosted`/`newHosted`) — `srv.AddReceivingMiddleware(a.MCPMiddleware())` on the hosted `*mcp.Server` only; `ui.Limit = a.BoardLimit`. The `config.rateLimit` flag from the connector task feeds `hostauth.Config.RateLimit`.
- `internal/hostauth/limit_test.go` (new) — bucket refusal at limit+1 with the message naming the limit, refill with the injected clock, two accounts independent, 0 disables.
- `cmd/tcgreplay/hosted_test.go` — with `RateLimit: 5`: the sixth tool call returns `IsError` with text containing "5 requests a minute" while a second account's call succeeds; a board verb over the limit returns `error` with the message; `/api/card` over the limit → 429.
- `internal/webui/webui_test.go` — `TestLimitHook`: with `s.Limit` returning an error, a `move` verb's response has that error and the board is unchanged; with `Limit` nil (existing tests) behaviour is unchanged.

**Complexity**: Medium
**Token estimate**: ~30k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Cap the number of signed-in users

**File changes**:
- `internal/hostauth/limit.go` — `admit(acct Account) error`: if `MaxUsers > 0`, the account is not already in `accounts` with a live sign-in, and the number of accounts with live sign-ins ≥ `MaxUsers` → `ErrBusy` ("the server is busy: it already has the most signed-in users it serves at once (N); try again later"), logged. `var ErrBusy = errors.New("server busy")`.
- `internal/hostauth/google.go` (`serveGoogleCallback`) — call `admit` after reading the ID token and before `signIn`/the connector continuation; on `ErrBusy` render `busyPage` with status 503 (both flows; the user is in the browser).
- `internal/hostauth/store.go` — live sign-in counting: increment on session creation and refresh-token issue, decrement on session end and refresh-token removal (rotation is net zero); remove the account entry at zero so its place frees. Authorization codes not yet exchanged do not hold a place (the account was admitted at the callback; the exchange re-checks nothing).
- `internal/hostauth/pages.go` — `busyPage` template: "tcgreplay is busy — it is serving as many signed-in users as it can. Please try again later."
- `cmd/tcgreplay/hosted.go` — `config.maxUsers` feeds `hostauth.Config.MaxUsers`.
- `internal/hostauth/limit_test.go` — with `MaxUsers: 1`: account A signs in; B gets 503 with "busy" through both the board and connector flows; A signs in again fine; A signs out of its only sign-in → B now gets in; `0` disables.

**Complexity**: Low
**Token estimate**: ~15k tokens
**Agent strategy**: Single agent, sequential execution.

### Task: Document sign-in and limits

**File changes**:
- `TCG Review Playmat Design and Build Plan.md:299` (the "Runtime" paragraph, as extended by web-mcp-docker's documentation task) — add a "Hosted sign-in" paragraph: connector sign-in (claude.ai custom connector → Google), board sign-in and Sign out, everything in memory so a restart or deploy signs everyone out, the request limit and user cap with their messages, the settings table (`TCGREPLAY_GOOGLE_CLIENT_ID`, `TCGREPLAY_GOOGLE_CLIENT_SECRET`, `TCGREPLAY_RATE_LIMIT` = 300, `TCGREPLAY_MAX_USERS` = 10, `0` disables), the Google client setup steps (from the human task), the `/healthz` health check, and that desktop modes never ask for sign-in.
- `cmd/tcgreplay/main.go:1-11` (package comment) — one sentence: hosted mode requires Google sign-in.
- `internal/hostauth/hostauth.go` — package doc already written in the first task; check it matches the final behaviour.

**Complexity**: Low
**Token estimate**: ~8k tokens
**Agent strategy**: Single agent, sequential execution.

## Testing Strategy

- **Sign users in with Google** — `internal/hostauth/google_test.go`: state/cookie binding, ID-token claim checks (issuer, audience, nonce, expiry via injected clock), addresses from `Public`, `next` validation, restart forgets sessions. Fake Google from `internal/hostauth/hostauthtest`.
- **Serve the connector sign-in endpoints** — `internal/hostauth/oauth_test.go`: metadata contents; registration redirect rules; authorize validation (client, redirect, PKCE, resource); token endpoint (single-use codes, PKCE, client binding, expiry, rotation, `invalid_grant`, 401 `invalid_client` for unknown clients, form encoding); bearer check (401 + `resource_metadata`, expiry, restart). End-to-end with the go-sdk `AuthorizationCodeHandler` and `StreamableClientTransport.OAuthHandler`.
- **Require sign-in for the hosted connector** — `cmd/tcgreplay/hosted_test.go`: anonymous `/mcp` refused and logged with no tool run; signed-in tool parity and `import_log`; two accounts; `/healthz` open; missing credentials stop start-up; settings parsing. All web-mcp-docker hosted tests run signed in through `hostauthtest.MCPTransport`.
- **Guard the board with Google sign-in** — `internal/hostauth/board_test.go` and `cmd/tcgreplay/hosted_test.go`: signed-out page redirect round trip to the original path; signed-out `/ws` and `/api/card` 401 and no verb reaches the game; session survives reloads; logout Origin check, session end closes the websocket within 2 s, next load redirects; restart redirects old cookies; connector sign-in also yields a board cookie. Desktop `TestRejectsForeignHostAndOrigin` unchanged.
- **Show the account and Sign out on the board** — no automated front-end tests (none exist in the repo); checked by hand in the test plan (header account + Sign out on hosted, nothing on desktop, redirect after restart).
- **Limit requests per account** — `internal/hostauth/limit_test.go` (bucket maths with injected clock, independence, 0 disables), `internal/webui/webui_test.go` `TestLimitHook` (refused verb leaves the board unchanged; nil hook unchanged), `cmd/tcgreplay/hosted_test.go` (tool error text names the limit, other account proceeds, board verb error, `/api/card` 429).
- **Cap the number of signed-in users** — `internal/hostauth/limit_test.go`: cap refuses a new account in both flows with 503 "busy", existing account re-signs in, place frees after the last sign-in ends, 0 disables.
- **Document sign-in and limits** — no tests; reviewed with the plan's manual checks.
- **Success metrics and manual reviews** — as listed in plan.md's Testing Approach; every "Manual — captured in the implementation test plan" item goes into the implementation test plan (connector flow in claude.ai, two accounts, phone board link, stays signed in, restart behaviour, sign out, deployed limits, settings without rebuild, desktop regression, Cloudflare cookies/redirects, a week of logs, under-a-minute re-sign-in).

## Project References

- Repo `tcgreplay`, root `/home/nicj/code/github.com/nicholasjackson/tcgreplay` (the only registered repo).
- Spec `20261017123240-21493a95-google-login` (this plan); epic `20261017105033-91f90842-hosted-playmat`.
- Plan `20261017105035-e3f64c1b-web-mcp-docker` — must be implemented first; supplies hosted mode, the public-URL setting, the hosted handler and tests, and `.do/app.yaml`.
- Specs `20261017112555-30c99ab1-memory-store-record` (parallel) and `20261017143118-b05aa6b1-per-user-games` (downstream, consumes the account on requests).
- Design documents: none. Knowledge entries: none (store empty).
- External: claude.com/docs/connectors/building/authentication; MCP authorization spec 2025-11-25; RFC 9728, 8414, 7591, 7636, 6749, 8252; OpenID Connect Core §3.1.3.7; go-sdk v1.6.0 `auth`, `oauthex`, `examples/auth/server`.

## Token Management Strategy

| Tier | Token Budget | Agent Strategy |
|------|-------------|----------------|
| Low | ~10k | Single agent, sequential |
| Medium | ~25k | 2-3 parallel agents |
| High | ~50k+ | Parallel analysis, sequential integration |

Per task: Sign users in with Google — Medium (~40k); Serve the connector sign-in endpoints — High (~60k); Require sign-in for the hosted connector — Medium (~35k); Guard the board with Google sign-in — Medium (~40k); Show the account and Sign out on the board — Low (~12k); Limit requests per account — Medium (~30k); Cap the number of signed-in users — Low (~15k); Document sign-in and limits — Low (~8k). The human task needs no agent budget. The `hostauth` tasks are sequential (each builds on the store and Google sign-in); the board client, rate limit and user cap tasks can run in parallel once the board guard lands.

## Migration Notes

- **Deployment change for the owner:** once Milestone 1 deploys, the hosted app will not start until `TCGREPLAY_GOOGLE_CLIENT_ID` and `TCGREPLAY_GOOGLE_CLIENT_SECRET` are set in App Platform — do the human task's Google client and secrets before (or immediately with) the first deploy of this code. The health check moves from `/` to `/healthz` in `.do/app.yaml`; apply the spec change with `doctl apps update <app-id> --spec .do/app.yaml` (pushes do not re-apply the app spec).
- The existing claude.ai connector added during the web-mcp-docker trial (authless) must be removed and re-added so claude.ai discovers sign-in.
- No data migration: nothing is persisted.

## Performance Considerations

- Every `/mcp` and board request now does one map lookup under a mutex (token or session) plus a bucket update — negligible next to game work. The store's mutex is never held across network calls (Google's token exchange happens outside it).
- Claude's OAuth endpoints budget 10 s (30 s for refresh); the only slow step is the Google code exchange in the callback (browser-facing), bounded by a 10 s timeout. `/token` and `/register` do no network I/O.
- Memory is bounded: registered clients capped at 1000 (oldest evicted), pending logins and codes expire in minutes and are swept on writes, rotated refresh tokens are deleted, and sessions/tokens are bounded in practice by the user cap.
