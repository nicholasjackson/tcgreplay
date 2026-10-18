---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Research: 20261017123240-21493a95-google-login

## Alternatives considered and rejected

### Option: Point claude.ai straight at Google as the authorization server

List `https://accounts.google.com` as the authorization server in the protected resource metadata, so claude.ai signs in with Google directly.

**Rejected**: Google has no Dynamic Client Registration or Client ID Metadata Document support, and claude.ai's custom-connector flow needs one of them (or Anthropic-held credentials, which require emailing mcp-review and are not offered for custom connectors) — claude.com/docs/connectors/building/authentication "DCR and CIMD details". The spec's Technical Approach already says Google cannot play that role directly.

### Option: Static bearer token (`static_headers`) instead of OAuth

Static bearer token (`static_headers`) instead of OAuth.

**Rejected**: beta, limited organisations, one credential per organisation (cannot identify the Google account), and the spec requires Google sign-in.

### Option: Pre-registered client entered at connection time (`custom_connection`)

Pre-registered client entered at connection time (`custom_connection`).

**Rejected**: each user would have to type a client ID; DCR works out of the box. DCR is kept as the only registration method.

### Option: Client ID Metadata Document (CIMD) support

Client ID Metadata Document (CIMD) support.

**Rejected** (for now): claude.ai falls back to DCR when the metadata does not advertise `client_id_metadata_document_supported` (same doc). CIMD means fetching and validating remote JSON documents; DCR alone is less code and fully supported.

### Option: Signed, self-contained tokens (JWTs) instead of opaque in-memory tokens

Signed, self-contained tokens (JWTs) instead of opaque in-memory tokens.

**Rejected**: the spec requires sign-ins to end on restart and not be stored. Opaque random tokens in a process-memory map end on restart by construction, with no signing key to manage. A JWT signed with a per-process random key would also die on restart but adds a library or crypto code for no benefit.

### Option: Reusing the board's Google session cookie to skip Google in the connector's `/authorize`

Reusing the board's Google session cookie to skip Google in the connector's `/authorize`.

**Rejected**: an attacker who registers a client through open DCR could get a code issued silently in a victim's browser (the confused-deputy risk the MCP authorization spec warns about for proxies with a static upstream client). Every connector authorization goes through Google's account chooser instead, and redirect URIs are restricted to Claude's callbacks and loopback.

### Option: A third-party OAuth server library (e.g. ory/fosite, go-oauth2/oauth2)

A third-party OAuth server library (e.g. ory/fosite, go-oauth2/oauth2).

**Rejected**: large dependency trees and storage abstractions for what is four small endpoints with in-memory maps; the go-sdk already supplies the resource-server side (`auth.RequireBearerToken`, `auth.ProtectedResourceMetadataHandler`, `oauthex` types).

### Option: Rate limiting at HTTP level only (429 for `/mcp`)

Rate limiting at HTTP level only (429 for `/mcp`).

**Rejected** (for MCP): a 429 surfaces in claude.ai as a generic connector error, but the spec wants a message naming the limit. Tool calls over the limit return a tool error result with the message instead (go-sdk receiving middleware), board verbs return the message as the verb's error (shown as a toast). HTTP 429 is kept only for `/api/card`, which has no message channel.

### Option: `golang.org/x/time/rate` for the token bucket

`golang.org/x/time/rate` for the token bucket.

**Rejected**: a new dependency for ~30 lines; a small per-account bucket with an injectable clock is simpler to test.

### Option: A bypass flag to run hosted mode without Google credentials

A bypass flag to run hosted mode without Google credentials.

**Rejected**: hosted mode must refuse anonymous use; starting without credentials fails closed with a clear error. Local container runs use a Google client with a `http://localhost:8080` redirect.

## Chosen approach — evidence

- go-sdk v1.6.0 `auth/auth.go:69-97` — `RequireBearerToken(verifier, opts)` returns 401 with `WWW-Authenticate: Bearer resource_metadata="..."` when the token is missing or invalid; puts `*auth.TokenInfo` in the request context. Needs `Expiration` set (`auth.go:135-140`).
- go-sdk `auth/auth.go:154-183` — `ProtectedResourceMetadataHandler(*oauthex.ProtectedResourceMetadata)` serves RFC 9728 metadata with CORS.
- go-sdk `mcp/streamable.go:303-310, 494-498` — Streamable HTTP sessions are bound to `TokenInfo.UserID` at creation; later requests from another user get 403. Setting `UserID` to the Google subject gives session-hijack protection for free.
- go-sdk `mcp/shared.go:485-488, 518` — `RequestExtra.TokenInfo` reaches every server request (`req.GetExtra()`), so a receiving middleware can rate-limit per account and return a tool error.
- go-sdk `oauthex/auth_meta.go:28-116`, `oauthex/dcr.go:27-120` — field names for RFC 8414 metadata and RFC 7591 registration request/response.
- go-sdk `auth/authorization_code.go:67-217` + `mcp/streamable.go:1526` (`StreamableClientTransport.OAuthHandler`) — a spec-compliant MCP OAuth client (DCR + PKCE) usable in tests to drive the whole connector flow in-process against a fake Google.
- go-sdk `examples/auth/server/main.go:97-125` — the canonical wiring: PRM handler at `/.well-known/oauth-protected-resource`, `RequireBearerToken` around the Streamable handler.
- `golang.org/x/oauth2` v0.35.0 (already in `go.sum`, indirect) — `Config.AuthCodeURL`, `Exchange`, `GenerateVerifier`, `S256ChallengeOption`; `golang.org/x/oauth2/endpoints.Google` gives Google's endpoints with no extra dependencies.
- claude.com/docs/connectors/building/authentication — Claude's requirements: 401 with `resource_metadata`; PRM `resource` must equal the MCP URL exactly; first `authorization_servers` entry is used; RFC 8414 metadata at the issuer's `/.well-known/`; S256 PKCE always, advertise `code_challenge_methods_supported`; DCR registers a public client; refresh reactively on 401 and proactively before expiry; return `invalid_grant` for dead refresh tokens and rotate refresh tokens for public clients; `invalid_client` (401) from the token endpoint forces re-registration; token endpoint takes form-urlencoded, `/register` takes JSON; 10 s endpoint budget; callback `https://claude.ai/api/mcp/auth_callback` (may move to `https://claude.com/api/mcp/auth_callback`); Claude Code uses loopback `http://localhost:<any>/callback` and `http://127.0.0.1:<any>/callback`; consent should show the redirect host; `offline_access` appended to scopes when listed; egress `160.79.104.0/21`.
- `internal/webui/hub.go:162-213` — `serveWS` uses `r.Context()` as the connection's parent context, so cancelling the request context (when the board session ends) closes the live connection with no hub change; `h.s.dispatch(r.Context(), c, req)` is the single place every board verb passes, the hook point for the per-account limit.
- `internal/webui/server.go:65-71` — `Handler()` is the board's whole route set; wrapping it in a session middleware from the hosted handler guards `/`, `/ws` and `/api/card` without touching the desktop path.
- `web/src/lib/ws.js:17-23` — the client reconnects on close; the place to check the session and send a signed-out board to sign-in.
- `web/src/App.svelte:33-34` — the header with the connection dot; where the signed-in account and Sign out go.

## Files examined

- tcgreplay:cmd/tcgreplay/main.go:35-135 — config struct, flags, mode switch, `newServer`, `startUI`; hosted mode (from web-mcp-docker) is not implemented yet at commit 77e6a6a.
- tcgreplay:internal/webui/server.go:28-102 — `Server{M, Human, hub}`, `New`, `Run`, `Handler` (`/ws`, `/api/card`, `/`), `localOnly`.
- tcgreplay:internal/webui/hub.go:160-213 — `serveWS`: loopback `OriginPatterns`, writer goroutine, read loop, `dispatch(r.Context(), ...)`.
- tcgreplay:internal/webui/actions.go:13-60 — websocket `request`/`response` shapes; `response.Error` is shown to the user as a toast.
- tcgreplay:internal/mcpserver/server.go:17-45 — `Server` fields, `MCP()` builds one `*mcp.Server`; tool handlers do not see HTTP.
- tcgreplay:web/src/lib/ws.js — connect/reconnect loop, `act` shows `r.error` as a toast.
- tcgreplay:web/src/main.js, web/src/App.svelte:33-34 — app bootstrap and header.
- tcgreplay:Makefile — `test` = `go vet ./...` + `go test ./...`.
- tcgreplay:go.mod — go 1.27.2; go-sdk v1.6.0 direct; `golang.org/x/oauth2 v0.35.0` indirect.
- tcgreplay:TCG Review Playmat Design and Build Plan.md:299 — the "Runtime" paragraph web-mcp-docker extends with hosted mode; this plan adds sign-in and limits beside it.
- go-sdk@v1.6.0:auth/auth.go, auth/authorization_code.go, oauthex/*.go, mcp/streamable.go, mcp/shared.go, examples/auth/server/main.go — see evidence above.

## External references

- claude.com/docs/connectors/building/authentication — Claude's OAuth client behaviour for remote MCP servers (the authoritative list of what the sign-in service must do).
- support.claude.com/en/articles/11503834 — "Building custom connectors via remote MCP servers": callback URL and client name "Claude", DCR support.
- MCP authorization spec 2025-11-25 (modelcontextprotocol.io/specification/2025-11-25/basic/authorization) — PRM discovery, PKCE, confused-deputy and redirect-URI guidance.
- RFC 9728 (protected resource metadata), RFC 8414 (authorization server metadata), RFC 7591 (dynamic client registration), RFC 7636 (PKCE), RFC 6749 §5.2 (error codes), RFC 8252 §7.3 (loopback redirects).
- OpenID Connect Core 1.0 §3.1.3.7 — an ID token received directly from the token endpoint over TLS may be validated by TLS server authentication instead of its signature; lets the plan skip JWKS handling.
- Google "Using OAuth 2.0 to access Google APIs" / OpenID Connect docs — endpoints `accounts.google.com/o/oauth2/v2/auth`, `oauth2.googleapis.com/token`; `iss` is `https://accounts.google.com` or `accounts.google.com`; consent screen "Testing" status restricts sign-in to listed test users, so it must be "In production" (scopes `openid email` are non-sensitive, no verification).
- github.com/anthropics/claude-ai-mcp issues #313, #215 — user reports of claude.ai not calling `/token` after a code is issued; unconfirmed, kept as a risk.

## Prior plans / specs consulted

- Plan `20261017105035-e3f64c1b-web-mcp-docker` (final, not yet implemented) — defines `tcgreplay -hosted`, `hostedHandler(srv, ui)`, `-public-url`/`TCGREPLAY_PUBLIC_URL` as the only address source, `webui.Server.Public`, `mcpserver.Server.BoardURL`, `.do/app.yaml` (health check on `/`, envs), tests over Streamable HTTP in `cmd/tcgreplay/hosted_test.go`. This plan builds on all of it; its health check moves to `/healthz` because `/` now redirects to sign-in.
- Spec `20261017143118-b05aa6b1-per-user-games` — later; needs the signed-in account as the identity for every request and "no second sign-in or limit mechanism". This plan exposes the account on request contexts and MCP requests for it.
- Spec `20261017112555-30c99ab1-memory-store-record` — planned in parallel; makes games memory-only. No overlap with sign-in code; consistent with keeping sign-ins in memory.

## Open assumptions

- claude.ai completes the DCR + PKCE flow against this server (reported issues of claude.ai not calling `/token` exist). If it does not, STOP and report what the server saw.
- After a restart, claude.ai's refresh gets `invalid_client`/`invalid_grant`, re-registers and asks the user to reconnect (or shows a "reconnect" prompt) rather than failing silently. Unverified; the spec lists it as a risk. If claude.ai only shows errors, record the exact behaviour and STOP for the user's decision.
- Cloudflare (proxied) does not challenge Anthropic's egress (`160.79.104.0/21`) on `/.well-known/*`, `/register`, `/token`, `/mcp`; if bot protection blocks them, the owner must add a WAF skip rule.
- Cookies set on the Google callback survive Cloudflare and App Platform unchanged (`Set-Cookie` on 302 responses, `SameSite=Lax`, `Secure`).
- go-sdk v1.6.0's `auth.AuthorizationCodeHandler` can drive the flow against an `httptest` server on 127.0.0.1 (loopback is allowed by its HTTPS checks). If not, tests drive the endpoints by hand with `net/http`.

## Drafting assumptions

### Chosen direction: playmat as its own OAuth sign-in service, Google upstream (architecture)
- **Decision**: New package `internal/hostauth`: minimal OAuth 2.1 authorization server for claude.ai (PRM, RFC 8414 metadata, DCR, /authorize, /token, PKCE S256) delegating login to Google OIDC; /mcp protected with go-sdk `auth.RequireBearerToken`; board guarded by a Google sign-in session cookie; all state in memory; per-account token bucket and distinct-account cap; hosted-only.
- **Rationale**: Matches the spec's Technical Approach exactly; claude.ai needs DCR/CIMD which Google lacks; go-sdk supplies the resource-server half; memory-only state meets "sign-ins end on restart".
- **Rejected**: Google as the authorization server directly; third-party OAuth server library; JWT tokens; HTTP-only 429 limits (see research.md alternatives).
- **Key trade-offs**: Access token 1 h + refresh token until restart, rotated on use (Claude requires rotation for public clients); unknown client at /token answers 401 `invalid_client` so claude.ai re-registers after a restart; endpoints at root defaults `/authorize` `/token` `/register`; Google callback `/auth/google/callback` shared by connector and board flows and always sets the board cookie.

### DCR only, no CIMD (discovery)
- **Decision**: The sign-in service supports Dynamic Client Registration only; it does not advertise Client ID Metadata Documents.
- **Rationale**: claude.ai falls back to DCR when CIMD is not advertised; DCR is less code than fetching and validating remote client documents.
- **Rejected**: CIMD (more code, no user benefit for one server); pre-registered client entered by the user (extra setup per user).

### Opaque in-memory tokens, ID token checked by TLS (discovery)
- **Decision**: Access and refresh tokens are random opaque strings held in process memory; Google's ID token is decoded and its `iss`, `aud`, `exp` and `nonce` checked, without JWKS signature verification, because it comes straight from Google's token endpoint over TLS (OIDC Core §3.1.3.7).
- **Rationale**: Meets "sign-ins not stored, end on restart" by construction; no key management or extra dependency.
- **Rejected**: JWTs with a per-process key; JWKS verification of Google's ID token.

### Redirect URIs restricted, every connector sign-in goes through Google (discovery)
- **Decision**: Registered redirect URIs must be `https://claude.ai/api/mcp/auth_callback`, `https://claude.com/api/mcp/auth_callback`, or loopback `http://localhost:<any port>/...` / `http://127.0.0.1:<any port>/...`. The connector's authorize step always sends the browser to Google (account chooser), never silently reuses a board cookie. No separate consent page of our own.
- **Rationale**: Open DCR plus a static upstream Google client is the MCP spec's confused-deputy case; restricting redirects to Claude's own callbacks and loopback removes the attack, and Google's account chooser is the user's explicit step.
- **Rejected**: Consent page naming the client (more UI, little extra safety once redirects are restricted); silent reuse of the board cookie.

### Board sign-out ends only the board session (architecture)
- **Decision**: Sign out on the board ends that browser's board session and closes its live connections; it does not revoke the account's connector tokens.
- **Rationale**: The spec asks for sign-out "from the board" and its check is that reloading the board asks for sign-in; revoking claude.ai's tokens would break the connector unexpectedly. Disconnecting the connector is done in claude.ai.
- **Rejected**: Sign out of everything for the account.

### Hosted mode fails closed without Google credentials (architecture)
- **Decision**: `tcgreplay -hosted` refuses to start when the Google client ID or secret is missing; there is no bypass flag.
- **Rationale**: "Sign-in required when hosted"; an accidental open deployment is worse than a failed start. Local container runs use a Google client with a localhost redirect.
- **Rejected**: Running open with a warning; an `-insecure-no-login` flag.

### Signed-in user count = accounts holding any live sign-in (architecture)
- **Decision**: The user cap counts distinct Google accounts holding at least one live board session or connector refresh token. An account already counted can always sign in again. A place frees when the account's last sign-in ends (board sign-out with no connector token, or restart). No idle expiry.
- **Rationale**: Sign-ins last until restart by requirement, so idle expiry would contradict "Signed in until restart"; per-user-games will add its own idle game clean-up.
- **Rejected**: Idle-based expiry of sign-ins; counting live connections.

### Defaults for limits and settings names (architecture)
- **Decision**: `-rate-limit` / `TCGREPLAY_RATE_LIMIT` = requests per minute per account, default 300, bucket burst equal to the per-minute value, 0 disables; `-max-users` / `TCGREPLAY_MAX_USERS` default 10, 0 disables; `-google-client-id` / `TCGREPLAY_GOOGLE_CLIENT_ID`; secret from `TCGREPLAY_GOOGLE_CLIENT_SECRET` only (no flag, so it never shows in process listings).
- **Rationale**: Follows the web-mcp-docker flag-with-environment-default pattern; 300/min covers a board load (~60 card lookups) plus a busy review session; 10 users suits an owner trial.
- **Rejected**: Separate limits per surface (more settings, no spec need).

### What the request limit counts (architecture)
- **Decision**: One per-account bucket counts every MCP request (all JSON-RPC methods, via go-sdk receiving middleware), every board websocket action and every `/api/card` lookup. Page and static asset loads are not counted.
- **Rationale**: Covers every path that does work for the account (card lookups can hit TCGdex); static files are cheap.
- **Rejected**: Counting HTTP requests at the router (MCP over-limit would surface as a generic error in claude.ai).

### Logging sign-ins and refusals (architecture)
- **Decision**: Hosted mode logs each sign-in (account email), each refused unauthenticated request to `/mcp`, `/ws` or `/api/card` (path and reason, no token), each rate-limit refusal and each busy refusal, to stderr.
- **Rationale**: The success metric "zero unsigned requests reaching game actions" is checked from the logs; logs are not stored records under the "no per-user records" non-goal.
- **Rejected**: Logging every tool call with its account (noisy).

### One scope, `playmat`, always granted (tasks)
- **Decision**: The resource advertises one scope, `playmat`, sent in the 401's `WWW-Authenticate` and granted on every token regardless of what the client asks for; `offline_access` is not listed but refresh tokens are always issued.
- **Rationale**: The playmat has one level of access; a fixed scope lets `RequireBearerToken` check it without per-client logic.
- **Rejected**: No scopes (the go-sdk client and claude.ai then fall back to guessing); multiple scopes (no use).

### Settings flags added with the connector task (tasks)
- **Decision**: The `-rate-limit` and `-max-users` flags are added in "Require sign-in for the hosted connector" with their defaults, and wired to behaviour by the Milestone 3 tasks.
- **Rationale**: One change to the command's settings and the app definition, instead of three edits to the same lines.
- **Rejected**: Each limit task adding its own flag (more merge churn in `main.go` and `.do/app.yaml`).

### Health check moves to `/healthz` (tasks)
- **Decision**: Add an unauthenticated `/healthz` and point `.do/app.yaml`'s health check at it, changing the web-mcp-docker plan's `/`.
- **Rationale**: `/` now redirects signed-out requests to Google, which a platform health check would treat as a failure.
- **Rejected**: Exempting `/` from sign-in (breaks "Board sign-in").

## Rehydration cues

- `spektacular spec file read 20261017123240-21493a95-google-login`
- `spektacular plan file read 20261017105035-e3f64c1b-web-mcp-docker plan` and `... context` (hosted mode tasks and file plan)
- Read `$(go env GOMODCACHE)/github.com/modelcontextprotocol/go-sdk@v1.6.0/auth/auth.go`, `examples/auth/server/main.go`, `mcp/streamable.go:290-310`
- WebFetch claude.com/docs/connectors/building/authentication
- Read `internal/webui/server.go`, `internal/webui/hub.go:160-213`, `web/src/lib/ws.js`, `cmd/tcgreplay/main.go`
