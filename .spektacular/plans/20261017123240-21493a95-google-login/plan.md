---
created_date: "2026-10-17"
document_status: final
closed_date: "2026-10-17"
---

# Plan: 20261017123240-21493a95-google-login

<!-- Metadata -->
<!-- Created: 2026-10-17T15:06:33Z -->
<!-- Commit: 77e6a6a -->
<!-- Branch: main -->
<!-- Repository: git@github.com:nicholasjackson/tcgreplay.git -->

## Overview

Puts Google sign-in in front of the hosted playmat, so only people with a Google account can use it and anonymous traffic is kept out. Adding the playmat as a custom connector in claude.ai, or opening a board link, takes the user through Google sign-in once; sign-ins last until the server restarts, the board has a Sign out button, and two operator settings cap requests per account and the number of signed-in accounts. It protects the owner's hosted trial from abuse and gives the later per-user-games work an account to key games on, while Claude Code and Claude Desktop keep running the playmat locally with no login.

## Conventions

No project conventions apply to this feature: the `tcgreplay` knowledge store holds no entries (the always-applied load for the repo returned nothing, and topic searches for auth, oauth, http, login, mcp, hosted, session and cookie found no entries). The plan follows the patterns already in the code and in the web-mcp-docker plan instead: the repo's vet-and-test Makefile target, standard-library `testing` with `httptest`, doc comments on every exported identifier, and hosted-only behaviour switched on by optional fields whose zero value leaves the desktop unchanged.

## Architecture & Design Decisions

All work lands in the single registered repo, `tcgreplay` (`/home/nicj/code/github.com/nicholasjackson/tcgreplay`), on top of the hosted mode planned in `20261017105035-e3f64c1b-web-mcp-docker` (`tcgreplay -hosted`, one plain-HTTP router serving `/mcp` and the board, `-public-url` as the only source of addresses). A new package, `internal/hostauth`, makes the playmat its **own small OAuth sign-in service for claude.ai and hands the actual login to Google**, as the spec's Technical Approach prefers. Towards claude.ai it is a minimal OAuth 2.1 authorization server at the public URL: RFC 9728 protected-resource metadata pointing at itself, RFC 8414 authorization-server metadata, Dynamic Client Registration (`/register`), `/authorize` and `/token` with S256 PKCE, all at the root paths the MCP specification uses as defaults. Towards Google it is an ordinary OpenID Connect web client (`openid email`, through `golang.org/x/oauth2`, already in `go.sum`). The `/mcp` endpoint is protected with the official MCP library's own resource-server support, `auth.RequireBearerToken` and `auth.ProtectedResourceMetadataHandler` (go-sdk v1.6.0), so an anonymous request gets the 401 with a `resource_metadata` pointer that claude.ai needs to start sign-in. The verified token carries the Google account's subject as `TokenInfo.UserID`, which the library already uses to bind each MCP session to its account.

The board uses a standard Google sign-in with a session cookie. In hosted mode the board's routes (`/`, `/ws`, `/api/card`) sit behind a session middleware in the hosted handler: a page request without a session is redirected to Google and back to the page it asked for; a live-connection or card request without one is refused with 401. Ending a session (sign-out) cancels the request context of that session's live connections, so open boards stop receiving updates at once. The board client learns its account from a small `/auth/session` endpoint, shows a Sign out button, and on a refused reconnect (after a sign-out elsewhere or a restart) sends the user back through sign-in. The Google callback is shared by both flows and always sets the board cookie, so a board link opened in the browser that just connected claude.ai needs no second sign-in. The connector's `/authorize`, however, always goes through Google's account chooser rather than reusing a board cookie, and registered redirect addresses are limited to Claude's callbacks and loopback; together these close the confused-deputy hole that open client registration with one upstream Google client would otherwise leave.

Everything the sign-in service keeps (registered clients, pending logins, authorization codes, access and refresh tokens, board sessions, signed-in accounts, rate-limit buckets) lives in maps in process memory, with opaque random tokens: nothing is written to disk, and a restart or redeploy ends every sign-in, as the spec requires and consistent with the epic's stateless direction. After a restart claude.ai's refresh is answered with `invalid_client` (the client registration is gone), the standard signal for claude.ai to register again and ask the user to sign in; the board's cookie is unknown and the user is redirected to Google. Access tokens last an hour and are refreshed by claude.ai; refresh tokens last until restart and are rotated on use, as Claude requires of public clients. Limits are hosted-only too: a per-account token bucket counts MCP requests, board actions and card lookups, and a cap on distinct signed-in accounts refuses a new account at the Google callback with a "server busy" page. Over the request limit, a tool call returns a tool error naming the limit (so Claude can tell the user, rather than claude.ai showing a generic connector failure), a board action returns the same message as its error toast, and a card lookup gets a 429.

The desktop is untouched by construction, following the hosted-versus-desktop switch the web-mcp-docker plan established: `internal/hostauth` is only built in `runHosted`, the board server gains one optional hook (`Limit`, nil on the desktop), and the board client's sign-in behaviour keys off `/auth/session`, which does not exist on the desktop. Google credentials and limits are settings (`TCGREPLAY_GOOGLE_CLIENT_ID`, `TCGREPLAY_GOOGLE_CLIENT_SECRET`, `TCGREPLAY_RATE_LIMIT`, `TCGREPLAY_MAX_USERS`), set as App Platform environment values; hosted mode refuses to start without credentials rather than running open. All sign-in addresses (issuer, metadata, Google redirect `/auth/google/callback`) are built from the configured public URL, never the request's `Host`. This beats pointing claude.ai at Google directly (Google offers no client registration), a third-party OAuth server library (heavy, storage-oriented), signed JWTs (key management for no gain when tokens must die on restart) and HTTP-only 429 limits (generic errors in claude.ai); the evidence is in `research.md#alternatives-considered-and-rejected`. Because claude.ai's acceptance of this flow is the spec's main risk, the first milestone ships connector sign-in end to end and proves it on the deployed app before the board and limits follow.

## Component Breakdown

- **Sign-in service (new, `internal/hostauth`)** — the one new component, owning everything about who is signed in on the hosted playmat. It is built only by hosted mode, from the public URL, the Google credentials and the limits, and keeps all its state in memory for the life of the process. It is made of five parts that share one in-memory store:
  - *Google sign-in* — sends a browser to Google's account chooser with a one-time state tied to that browser, takes Google's answer at the callback, exchanges the code and reads the Google account (subject and email) from the ID token. Shared by the connector and board flows; every successful sign-in also sets the board's session cookie in that browser.
  - *Connector authorization server* — what claude.ai talks to: protected-resource and authorization-server metadata, client registration (restricted to Claude's callback addresses and loopback), the authorize step (which hands over to Google sign-in and then returns a one-time code to the client), and the token endpoint (code exchange with PKCE, refresh with rotation, standard error codes, `invalid_client` for clients forgotten by a restart). It also supplies the token check the MCP endpoint uses.
  - *Board sessions* — the session cookie, a middleware that redirects signed-out page requests to Google and refuses signed-out live-connection and card requests, an endpoint the board reads its account from, sign-out, and the small signed-out, busy and error pages. A session's end cancels the live connections opened under it.
  - *Limits* — a per-account request bucket and the cap on distinct signed-in accounts. It answers "may this account make another request?" for the MCP middleware, the board's action hook and card lookups, and "may this new account sign in?" at the Google callback.
  - *Account on the request* — the signed-in account (Google subject and email) is attached to every authenticated request, for board requests on the context and for MCP requests as the token's user, so later specs (per-user games) can read whose request it is without a second mechanism.
- **Hosted run mode and handler (changed, in the `tcgreplay` command)** — reads the new settings (Google client ID and secret, request limit, user cap) beside the public URL, refuses to start without credentials, builds the sign-in service from the public URL, and composes it into the one router: sign-in routes and metadata unauthenticated, `/mcp` behind the bearer-token check with the per-account limit as MCP middleware, the board behind the session middleware, and an unauthenticated health check. The shim, daemon and in-process modes are unchanged.
- **Board server (changed, `internal/webui`)** — gains one optional hook called before every board action with the live connection's request context; a refusal becomes that action's error message. Nil on the desktop, where behaviour is exactly as today.
- **Board client (changed, `web/`)** — asks the server who is signed in; when hosted, shows the account and a Sign out button in the header; when a live connection is refused because the sign-in has ended (sign-out elsewhere, restart), sends the user through sign-in back to the same page. On the desktop the account endpoint does not exist and the client behaves as today.
- **App Platform app definition (changed, `.do/app.yaml`)** — declares the Google credentials as secret environment values and the limits as plain ones, and moves the health check to the unauthenticated health path, since the board root now redirects to sign-in.
- **Docs (changed)** — the design document's hosted-mode description gains sign-in, sign-out, the limits, the settings and the one-time Google and App Platform setup.

## Data Structures & Interfaces

No persisted data formats change: nothing about sign-in is written to disk, and game files, the card cache and the board's websocket messages keep their shapes. The new contracts are operator settings, the HTTP routes of the sign-in service, a few in-process interfaces, and one small JSON endpoint for the board.

**Hosted settings (additions)** — beside the web-mcp-docker plan's `-hosted`, `-public-url` and `-listen`:

| Setting | Flag | Environment default | Fallback |
|---|---|---|---|
| Google client ID | `-google-client-id` | `TCGREPLAY_GOOGLE_CLIENT_ID` | none: hosted mode will not start |
| Google client secret | — (environment only) | `TCGREPLAY_GOOGLE_CLIENT_SECRET` | none: hosted mode will not start |
| Requests per minute per account | `-rate-limit` | `TCGREPLAY_RATE_LIMIT` | `300`; `0` = no limit |
| Signed-in accounts at once | `-max-users` | `TCGREPLAY_MAX_USERS` | `10`; `0` = no cap |

The Google client's authorised redirect address is `<public URL>/auth/google/callback`.

**Routes added in hosted mode** (all addresses advertised are built from the public URL):

| Route | Auth | Purpose |
|---|---|---|
| `GET /.well-known/oauth-protected-resource`, `.../oauth-protected-resource/mcp` | none | RFC 9728: `resource` = `<public>/mcp`, `authorization_servers` = [`<public>`] |
| `GET /.well-known/oauth-authorization-server`, `/.well-known/openid-configuration` | none | RFC 8414 metadata: issuer, `/authorize`, `/token`, `/register`, `code` only, `authorization_code` + `refresh_token`, `S256`, auth methods `none` / `client_secret_post` / `client_secret_basic` |
| `POST /register` | none | RFC 7591 client registration (JSON) |
| `GET /authorize` | none | starts Google sign-in for a registered client; ends with a redirect to the client with `code` and `state` |
| `POST /token` | client | form-encoded `authorization_code` (with PKCE verifier) and `refresh_token` grants |
| `GET /auth/login?next=<path>` | none | starts Google sign-in for the board |
| `GET /auth/google/callback` | none | Google's redirect target for both flows |
| `GET /auth/session` | cookie | `200 {"email": "..."}` or `401` |
| `POST /auth/logout` | cookie | ends the board session, redirects to `/auth/signed-out` |
| `GET /auth/signed-out` | none | "signed out — sign in again" page |
| `GET /healthz` | none | `200 ok` for the platform health check |
| `/mcp` | bearer | MCP, unchanged except for the token check and per-account limit |
| `/`, `/ws`, `/api/card` | cookie | the board, unchanged except for the session check |

**Token response** — the standard OAuth shape:

```json
{ "access_token": "<opaque>", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "<opaque>", "scope": "playmat" }
```

Errors follow RFC 6749 §5.2 (`invalid_grant`, `invalid_client` with 401, `invalid_request`, `unsupported_grant_type`).

**Sign-in service (new package `hostauth`)** — its public face to the hosted command:

```go
type Config struct {
    Public                     *url.URL // the -public-url value
    GoogleClientID, GoogleSecret string
    RateLimit                  int      // requests per minute per account; 0 = none
    MaxUsers                   int      // distinct signed-in accounts; 0 = none
    Google                     oauth2.Endpoint // Google's endpoints; tests substitute a fake
}

// Account is the signed-in Google account a request acts for.
type Account struct { ID, Email string } // ID = Google "sub"

func New(cfg Config) (*Auth, error)
func (a *Auth) Routes(mux *http.ServeMux)                 // metadata, /register, /authorize, /token, /auth/*, /healthz
func (a *Auth) RequireToken(next http.Handler) http.Handler   // wraps /mcp (go-sdk RequireBearerToken)
func (a *Auth) RequireSession(next http.Handler) http.Handler // wraps the board
func (a *Auth) MCPMiddleware() mcp.Middleware             // per-account limit on MCP requests
func (a *Auth) BoardLimit(ctx context.Context) error      // per-account limit for board actions
func AccountFrom(ctx context.Context) (Account, bool)     // board requests
func AccountFromToken(ti *auth.TokenInfo) (Account, bool) // MCP requests (RequestExtra.TokenInfo)

var ErrRateLimited, ErrBusy error // messages name the limit hit / say the server is busy
```

**Board server (changed)** — one optional hook beside the web-mcp-docker plan's `Public` field:

```go
type Server struct {
    M      *game.Manager
    Human  state.Seat
    Public *url.URL                        // from web-mcp-docker
    Limit  func(ctx context.Context) error // hosted: called before each board action; an error refuses it with its message. nil = desktop, unchanged
}
```

**Hosted handler (changed)** — gains the sign-in service:

```go
func hostedHandler(srv *mcp.Server, ui *webui.Server, a *hostauth.Auth) http.Handler
```

**Browser cookies** — `tcgreplay_session` (board session, `HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` when the public URL is https, no expiry so it ends with the browser or the server) and `tcgreplay_login` (the one-time state of a sign-in in progress, ten minutes). Both hold only random identifiers.

## Implementation Detail

**New module boundary: one package owns identity.** Everything about sign-in lives in a new `hostauth` package that knows nothing about games, the board or tools. It exposes HTTP routes, two wrappers (bearer token for the connector, session cookie for the board), one MCP middleware, one limit function and two "which account is this?" accessors. The hosted command composes these around the existing MCP and board handlers; neither of those learns about Google or OAuth. A developer reading the hosted handler sees, in a dozen lines, which routes are public, which need a token and which need a cookie. Later specs (per-user games) read the account from the request rather than re-deriving it.

**New pattern: an in-memory store with explicit lifetimes.** The sign-in service keeps one mutex-guarded store of maps keyed by random identifiers (registered clients, pending logins, authorization codes, access tokens, refresh tokens, board sessions, signed-in accounts, limit buckets). Every entry has a stated lifetime: pending logins and codes minutes, access tokens an hour, refresh tokens and sessions until restart. Expired entries are swept lazily on writes, and the number of registered clients is bounded with the oldest evicted, so unauthenticated registration cannot grow memory without limit. Tokens are compared by their hash so a timing difference or a logged map key never leaks a usable token. The clock is injectable, so expiry, refresh and limit tests run without sleeping.

**New pattern: one Google sign-in, two continuations.** Both flows start the same way — record a pending login, tie it to the browser with a short-lived cookie, send the browser to Google with state, nonce and PKCE — and both end at the same callback. What happens next is the pending login's continuation: for the connector, issue a one-time code and redirect to the registered client; for the board, redirect to the page originally requested. The user cap is checked once, at this callback, so neither flow can bypass it. Login CSRF is handled by requiring the state to match the browser's cookie.

**Following the library where it fits.** The resource-server half is the official go-sdk's: its bearer middleware produces the 401 and `WWW-Authenticate` header claude.ai requires, its metadata handler serves RFC 9728, and its Streamable HTTP handler already binds sessions to `TokenInfo.UserID`. The authorization-server half reuses the go-sdk's `oauthex` metadata and registration types for the JSON shapes, and `golang.org/x/oauth2` for the Google client. Only the four small endpoints and the store are new code.

**Hosted-only switches, as before.** Following the web-mcp-docker plan's convention, the board server gains one optional field whose zero value means "desktop, unchanged", and everything else is composed only in hosted mode. The board client's new behaviour is keyed off whether the server answers the account endpoint, so the same built UI serves both modes with no build flag.

**Limits as messages, not status codes.** An over-limit MCP request returns a tool error result (or a JSON-RPC error for non-tool methods) whose text names the limit, so Claude can relay it; a board action returns it as the action's error, which the board already shows as a toast; a card lookup gets 429. The cap's refusal is a plain "server busy" page in the browser where the user is signing in, whichever flow brought them there.

**Tests drive the real flow against a fake Google.** A test helper package provides an in-process fake Google (authorization endpoint that signs in a named test account, token endpoint that returns an ID token) and helpers that sign a test client in through the real routes: one returns an MCP client transport authorised through the go-sdk's own OAuth client (registration, PKCE, code, token), the other returns a board session cookie. The existing hosted-handler tests from the web-mcp-docker plan switch to these helpers, so every hosted test exercises sign-in, while the desktop tests are untouched.

## Dependencies

- **Design documents this plan was built on: none.** The spec carries no design references.
- **Plan `20261017105035-e3f64c1b-web-mcp-docker` must land first (flagged).** It introduces everything this plan wraps: `tcgreplay -hosted`, the hosted handler serving `/mcp` and the board on one router, the `-public-url` / `TCGREPLAY_PUBLIC_URL` setting (reused here for every sign-in address), `webui.Server.Public`, the hosted-handler tests and `.do/app.yaml`. None of it is implemented at the time of planning; this plan's tasks start from the code that plan produces.
- **Spec `20261017112555-30c99ab1-memory-store-record` (planned in parallel)** — no ordering dependency; it makes games memory-only, consistent with this plan keeping sign-ins in memory. Both plans touch the hosted command's wiring, so the later one to land merges around the other.
- **Spec `20261017143118-b05aa6b1-per-user-games` (downstream)** — builds on this plan's signed-in account (exposed on board request contexts and as the MCP token's user) and its limits, with no second sign-in mechanism.
- **`github.com/modelcontextprotocol/go-sdk` v1.6.0 (existing)** — `auth.RequireBearerToken`, `auth.ProtectedResourceMetadataHandler`, `oauthex` metadata and registration types, session-to-user binding in the Streamable HTTP handler, receiving middleware, and (in tests) `auth.AuthorizationCodeHandler` as a spec-compliant OAuth client. No version change.
- **`golang.org/x/oauth2` v0.35.0 (existing, indirect → direct)** — Google's endpoints (`endpoints.Google`), authorization URL, code exchange and PKCE helpers. Already in `go.sum`; becomes a direct requirement in `go.mod`.
- **Internal packages** — `internal/webui` (one optional hook), `cmd/tcgreplay` (settings and wiring); `internal/mcpserver`, `internal/game` and the rest unchanged.
- **Google Cloud OAuth client (external service, new)** — a "Web application" OAuth client with `openid email` scopes, redirect `<public URL>/auth/google/callback` (plus `http://localhost:8080/auth/google/callback` for local runs), and the consent screen published "In production" so any Google account can sign in. Created once by the owner.
- **claude.ai custom connectors (external service)** — must complete DCR + PKCE against the playmat and re-prompt after a restart. Behaviour documented by Anthropic but unverified against this server; proven in Milestone 1.
- **DigitalOcean App Platform and Cloudflare (external, existing from web-mcp-docker)** — carry the secret settings and pass cookies, redirects and Anthropic's requests (egress `160.79.104.0/21`) through unchanged.
- **Board UI build (existing)** — the Svelte client changes ship through the existing Vite build embedded in the binary; no new front-end dependency.

## Testing Approach

Tests stay in Go's standard `testing` package with `httptest`, beside the code they cover, and run under the repo's existing vet-and-test Makefile target. Nothing talks to the real Google or claude.ai: an in-process fake Google stands in for the identity provider, and the go-sdk's own spec-compliant OAuth client stands in for claude.ai. An injectable clock drives expiry, refresh and rate-limit tests without sleeping.

**Unit: the sign-in service (most coverage).** It is new security code, so it gets the densest tests:
- metadata documents name the public URL's addresses, the `/mcp` resource exactly, S256 PKCE and the supported grants, and never derive anything from the request's `Host`;
- registration accepts Claude's callbacks and loopback redirects on any port, and refuses any other redirect;
- authorize refuses unknown clients, unregistered redirects, missing or non-S256 PKCE and a mismatched `resource`, and otherwise sends the browser to Google with state, nonce and PKCE;
- the Google callback refuses a state that does not match the browser's cookie, an ID token with the wrong issuer, audience, nonce or an expired one, and otherwise signs the account in and continues the right flow;
- the token endpoint exchanges a code only once, only for the client and redirect it was issued to, only with the matching PKCE verifier and only before it expires; refresh rotates the refresh token and the old one is refused with `invalid_grant`; an unknown client gets 401 `invalid_client`; errors use RFC 6749 codes; it accepts form-encoded bodies;
- access tokens are accepted until they expire and refused after; a fresh `Auth` (a restart) accepts none of the previous instance's tokens or sessions;
- the per-account bucket refuses the request after the limit with a message naming it, refills over time, and one account's exhaustion leaves another's untouched; the user cap refuses a new account with the busy message while an account already signed in still gets in, and a place frees when an account's last sign-in ends.

**Integration: the hosted handler, end to end.** Through the same handler hosted mode serves, with a fake Google:
- an anonymous `/mcp` request gets 401 with a `resource_metadata` pointer, and no tool runs (**Anonymous connector request refused**);
- the go-sdk OAuth client discovers the metadata, registers, signs in through fake Google, gets a token, lists the same tools as the desktop and imports a game (**Connector flow**, at code level); two different accounts can each do so (**Any account**);
- the existing hosted tests from the web-mcp-docker plan (tool parity, board links, a Claude move reaching the board within 2 seconds, a board move reaching Claude) pass with a signed-in client and board;
- a signed-out request for the board page redirects to Google and, after sign-in, lands on the page first asked for (**Board link**); a signed-out live connection and card lookup are refused, and a board action without a session never reaches the game (**Live board guarded**);
- a signed-in board keeps working across reloads, and the connector across access-token expiry by refreshing (**Stays signed in**);
- after a new `Auth` replaces the old one (a restart), the old token gets 401 and the old cookie is redirected to sign-in, and signing in again works (**Restart**, at code level);
- signing out closes that session's live connection and the next page load redirects to sign-in (**Sign out**);
- over the limit, a tool call returns a tool error naming the limit, a board action returns it as its error, and another account carries on (**Rate limit**); over the cap a new account sees "server busy" (**User cap**);
- `/healthz` answers without sign-in.

**Unit: hosted settings.** The new flags and environment values resolve as documented (defaults 300 and 10, `0` disables), and hosted mode refuses to start without a Google client ID or secret, with a clear message (**Settings**, code level).

**Regression: the desktop.** The board server's existing tests (including the foreign host and origin rejection) and the MCP tool tests pass unchanged, and with no limit hook set the board behaves exactly as today (**Desktop**, code level). The board client is not unit-tested (the repo has no front-end tests); its sign-in redirect and Sign out button are checked by hand.

**Deliberate gaps.** No automated test talks to the real Google, claude.ai, Cloudflare or App Platform; those need real accounts and the deployed app, and the in-process flow already exercises the same handler and the same protocol steps.

**Success metrics:**
- The owner connects claude.ai on phone and laptop with one Google sign-in each — **Manual — captured in the implementation test plan.**
- Server logs show zero unsigned requests reaching game actions during the first week live — **Behavioural test** (every connector and board route refuses an unsigned request before any tool or board action runs, and the refusal is logged), plus **Manual — captured in the implementation test plan** (review a week of production logs).
- Signing back in after a deploy takes under a minute — **Manual — captured in the implementation test plan.**

**Manual reviews:**
- **Connector flow in claude.ai**: adding the connector opens Google sign-in; afterwards the tools are listed and a real game import succeeds — **Manual — captured in the implementation test plan.**
- **Two personal Google accounts** each sign in (confirms the consent screen is published, not in testing) — **Manual — captured in the implementation test plan.**
- **Board link while signed out** on a phone goes through Google and lands on the board — **Manual — captured in the implementation test plan.**
- **Stays signed in**: a day of connector use and board reloads with no deploy never asks for sign-in — **Manual — captured in the implementation test plan.**
- **Restart and redeploy**: after a push to `main`, both claude.ai and the board ask for sign-in and then work; record exactly what claude.ai shows — **Manual — captured in the implementation test plan.**
- **Sign out** from the board, then reload asks for sign-in — **Manual — captured in the implementation test plan.**
- **Rate limit and user cap on the deployed app** (temporarily low settings): the "too many requests" message reaches the user through Claude and the board; a second account sees "server busy" — **Manual — captured in the implementation test plan.**
- **Settings without rebuild**: changing credentials or limits in App Platform and restarting takes effect — **Manual — captured in the implementation test plan.**
- **Desktop regression**: Claude Code and Claude Desktop use the local playmat with no sign-in — **Manual — captured in the implementation test plan.**
- **Cookies and redirects through Cloudflare and App Platform** work on the deployed domain (checked early, at the end of Milestone 1) — **Manual — captured in the implementation test plan.**

## Milestones & Tasks

### Milestone 1: Claude in claude.ai signs in with Google before using the playmat

**What changes**: Adding the hosted playmat as a custom connector in claude.ai now takes the user through Google sign-in, with any Google account, and afterwards the connector works without further prompts: Claude lists the tools and can import and play a game as before. Requests to the connector without a sign-in are refused. Sign-ins last until the server restarts; after a redeploy claude.ai asks the user to sign in again. The Google credentials are settings in App Platform, and the hosted playmat refuses to start without them. This settles the spec's biggest risk first — whether claude.ai completes this sign-in flow, and how it behaves after a restart — and checks cookies and redirects through Cloudflare early. The board is still open at this point (it is guarded in the next milestone), and the desktop is unchanged.

**Validation point**: The automated suite passes, including the in-process connector sign-in through a fake Google and the hosted tests running signed in; on the deployed app, claude.ai connects through Google sign-in, lists the tools and imports a real game; after a redeploy it asks for sign-in again (what claude.ai shows is recorded).

#### - [x] Task: Sign users in with Google
**Id:** c0f33d2e-9252-4765-99f1-9210db20ee78
**Repo:** tcgreplay
**Depends on:** none
**Execution:** agent

Creates the hosted sign-in service's foundation: its settings, an in-memory store with explicit lifetimes, and a Google sign-in that sends a browser to Google's account chooser and turns Google's answer into a signed-in account (Google ID and email). The same sign-in is used later by both the connector and the board, and always leaves the browser with a board session. A fake Google for tests is included, so every later task can test real sign-in flows without the internet.

*Technical detail:* [context.md#task-sign-users-in-with-google](./context.md#task-sign-users-in-with-google)

**Acceptance criteria**:
- [x] A browser sent to sign in reaches Google's account chooser with a one-time state, and coming back with that state and a valid Google answer signs the account in.
- [x] A callback whose state does not match the browser's sign-in in progress, or whose Google ID token has the wrong issuer, audience, nonce or has expired, is refused with a clear page.
- [x] Every Google sign-in address is built from the configured public address, never from the incoming request.
- [x] Nothing about a sign-in is written to disk; a new instance of the service knows none of the previous instance's sign-ins.
- [x] Tests can sign in named test accounts against an in-process fake Google.

#### - [x] Task: Serve the connector sign-in endpoints
**Id:** 820b8120-9917-437a-9198-5c10e02d0b4a
**Repo:** tcgreplay
**Depends on:**
- c0f33d2e-9252-4765-99f1-9210db20ee78 — Sign users in with Google
**Execution:** agent

Makes the playmat the sign-in service claude.ai talks to when the connector is added: it publishes where to sign in, lets claude.ai register itself, sends the user through Google sign-in, and issues and refreshes access tokens. It also provides the token check that protects the connector endpoint, so an anonymous request gets the response that tells claude.ai to start sign-in. Tokens live in memory, so a restart ends them and claude.ai is told to register and sign in again.

*Technical detail:* [context.md#task-serve-the-connector-sign-in-endpoints](./context.md#task-serve-the-connector-sign-in-endpoints)

**Acceptance criteria**:
- [x] An anonymous request to the connector is refused with a pointer to the sign-in metadata, and the metadata names the connector address and this server as its sign-in service.
- [x] A client can register with Claude's callback address or a loopback address, and is refused with any other address.
- [x] Sign-in requires PKCE; a code works once, only for its own client, address and verifier, and only within its lifetime.
- [x] Access tokens expire after an hour and refresh tokens renew them until a restart; a used refresh token cannot be used again.
- [x] After a restart, an old token is refused and refreshing is answered with the standard signal to register again.
- [x] A standard MCP OAuth client completes registration, Google sign-in and token exchange end to end against a fake Google.

#### - [x] Task: Require sign-in for the hosted connector
**Id:** b7458f66-1161-4f23-b4f5-5cb8c6264fae
**Repo:** tcgreplay
**Depends on:**
- 820b8120-9917-437a-9198-5c10e02d0b4a — Serve the connector sign-in endpoints
**Execution:** agent

Switches hosted mode on to the sign-in service: new settings for the Google credentials, the sign-in routes and metadata mounted beside the connector and the board, the connector endpoint protected by the token check, and a health check that needs no sign-in. Hosted mode refuses to start without Google credentials. The existing hosted tests now run signed in, and the App Platform definition declares the new secret settings and uses the new health check. The desktop modes are unchanged.

*Technical detail:* [context.md#task-require-sign-in-for-the-hosted-connector](./context.md#task-require-sign-in-for-the-hosted-connector)

**Acceptance criteria**:
- [x] Hosted mode reads the Google client ID from a flag or environment value and the secret from the environment, and stops at start-up with a clear message when either is missing.
- [x] In hosted mode an anonymous connector request is refused and logged, and no tool runs.
- [x] A client signed in through the hosted handler sees the same tools as the desktop and can import a game; two different accounts can each sign in.
- [x] The health check answers without a sign-in, and the App Platform definition uses it and declares the Google credentials as secrets.
- [x] The desktop modes start and serve as before with no sign-in.

#### - [x] Task: Create the Google sign-in client and deployment secrets
**Id:** 39b356cd-82b5-46d2-81c6-69a1688b0acd
**Repo:** tcgreplay
**Depends on:**
- b7458f66-1161-4f23-b4f5-5cb8c6264fae — Require sign-in for the hosted connector
**Execution:** human — needs the owner's Google Cloud console, DigitalOcean App Platform settings and Cloudflare account

The owner creates a Google OAuth client for the playmat's domain, publishes its consent screen so any Google account can sign in, and puts the client ID and secret into the App Platform app's settings. The owner also makes sure Cloudflare lets claude.ai's servers reach the sign-in endpoints.

*Technical detail:* [context.md#task-create-the-google-sign-in-client-and-deployment-secrets](./context.md#task-create-the-google-sign-in-client-and-deployment-secrets)

**Acceptance criteria**:
- [x] A Google OAuth web client exists with the playmat's callback address (and the local one) authorised, and its consent screen is published for any Google account.
- [ ] The deployed app has the Google client ID and secret as settings and starts after a redeploy.
- [ ] The deployed sign-in metadata and token endpoints are reachable from outside without a Cloudflare challenge.

### Milestone 2: The board needs a Google sign-in, and the user can sign out

**What changes**: Opening a board link while signed out takes the user through Google sign-in and lands them on that board; a browser that has just connected claude.ai is already signed in. Without a sign-in the board gets no live updates and its actions are refused. The board header shows the signed-in account with a Sign out button; after signing out, or after a restart, reloading the board asks for sign-in again. The desktop board is unchanged and never asks.

**Validation point**: The hosted tests show a signed-out board page redirected to sign-in and back, signed-out live connections and actions refused, and sign-out closing the live connection; on the deployed app, a board link opened on a phone while signed out goes through Google and shows the board, and Sign out works.

#### - [x] Task: Guard the board with Google sign-in
**Id:** 0e5ee83a-730b-4c2c-8041-ad398c912be6
**Repo:** tcgreplay
**Depends on:**
- b7458f66-1161-4f23-b4f5-5cb8c6264fae — Require sign-in for the hosted connector
**Execution:** agent

Puts the hosted board behind a Google sign-in held in a session cookie. A signed-out visitor asking for a board page goes through Google and lands back on that page; a signed-out live connection or card lookup is refused. The board can ask who is signed in, and signing out ends the session and closes its live connections at once. The desktop board is unchanged.

*Technical detail:* [context.md#task-guard-the-board-with-google-sign-in](./context.md#task-guard-the-board-with-google-sign-in)

**Acceptance criteria**:
- [x] Opening a board link while signed out goes through Google sign-in and lands on the page that was asked for.
- [x] Without a sign-in the board gets no live updates, its actions never reach the game, and card lookups are refused.
- [x] A signed-in browser keeps working across reloads until the server restarts; after a restart it is sent to sign in again.
- [x] Signing out ends the session, closes its open live connections, and the next page load asks for sign-in.
- [x] Signing in for the connector in a browser also signs that browser in to the board.
- [x] The desktop board serves and accepts actions with no sign-in, as before.

#### - [x] Task: Show the account and Sign out on the board
**Id:** 61eb625d-5d97-45b1-84a6-353ccd4914ef
**Repo:** tcgreplay
**Depends on:**
- 0e5ee83a-730b-4c2c-8041-ad398c912be6 — Guard the board with Google sign-in
**Execution:** agent

Updates the board's web page so a hosted board shows who is signed in and offers a Sign out button in its header. When the live connection is refused because the sign-in has ended (a sign-out in another tab or a restart), the page sends the user through sign-in and back to the same board instead of retrying for ever. The desktop board looks and behaves exactly as before.

*Technical detail:* [context.md#task-show-the-account-and-sign-out-on-the-board](./context.md#task-show-the-account-and-sign-out-on-the-board)

**Acceptance criteria**:
- [x] On the hosted board the header shows the signed-in account's email and a Sign out button that signs the user out.
- [x] After the server restarts, an open board sends the user to sign in and returns to the board afterwards.
- [x] On the desktop the header has no account or Sign out button, and reconnecting works as before.

### Milestone 3: Limits keep one account from overloading the server

**What changes**: The operator can cap how many requests one account makes a minute and how many accounts are signed in at once, as settings with sensible defaults. An account over the request cap gets a "too many requests" message naming the limit — through Claude in the conversation and as a message on the board — while other accounts carry on. Once the user cap is reached, a new account trying to sign in sees a "server busy" page. The design document describes sign-in, sign-out, the limits, the settings and the one-time Google setup.

**Validation point**: The hosted tests show the request limit refusing one account with the named limit while another proceeds, and the user cap refusing a new account with "server busy"; on the deployed app, temporarily low limits produce both messages and restoring the settings with a restart removes them, with no rebuild.

#### - [x] Task: Limit requests per account
**Id:** 8f26f522-2b88-4870-b8ae-002de65cb2ae
**Repo:** tcgreplay
**Depends on:**
- 0e5ee83a-730b-4c2c-8041-ad398c912be6 — Guard the board with Google sign-in
**Execution:** agent

Adds an operator setting for how many requests one account may make a minute, counting connector requests, board actions and card lookups. Over the limit, Claude gets a tool error naming the limit to pass on, the board shows the same message, and other accounts are unaffected. The board server gains one optional hook for this, which the desktop leaves unset.

*Technical detail:* [context.md#task-limit-requests-per-account](./context.md#task-limit-requests-per-account)

**Acceptance criteria**:
- [x] An account over the limit gets a "too many requests" message naming the limit, in Claude's tool results and on the board.
- [x] Another account keeps working while one is over its limit, and the limited account recovers as time passes.
- [x] The limit is a setting with a default; setting it to zero turns it off; changing it needs only a restart.
- [x] The desktop board and connector have no limit.

#### - [x] Task: Cap the number of signed-in users
**Id:** 9c46c22b-9b35-4e1a-bf4a-83eb3905c33f
**Repo:** tcgreplay
**Depends on:**
- 0e5ee83a-730b-4c2c-8041-ad398c912be6 — Guard the board with Google sign-in
**Execution:** agent

Adds an operator setting for how many Google accounts may be signed in at once. A new account signing in over the cap, through the connector or the board, sees a "server busy" page; accounts already signed in carry on and can sign in again. A place frees when an account's last sign-in ends, and every place frees on restart.

*Technical detail:* [context.md#task-cap-the-number-of-signed-in-users](./context.md#task-cap-the-number-of-signed-in-users)

**Acceptance criteria**:
- [x] Once the cap is reached, a new account trying to sign in sees a "server busy" message, whichever way it signs in.
- [x] An account already signed in can still sign in again at the cap.
- [x] When an account's last sign-in ends, its place goes to the next account.
- [x] The cap is a setting with a default; setting it to zero turns it off; changing it needs only a restart.

#### - [x] Task: Document sign-in and limits
**Id:** 4836861e-8c3f-4f0a-af94-4a133cc3009f
**Repo:** tcgreplay
**Depends on:**
- 8f26f522-2b88-4870-b8ae-002de65cb2ae — Limit requests per account
- 9c46c22b-9b35-4e1a-bf4a-83eb3905c33f — Cap the number of signed-in users
- 61eb625d-5d97-45b1-84a6-353ccd4914ef — Show the account and Sign out on the board
**Execution:** agent

Describes the hosted sign-in in the project's design document beside the hosted mode: how the connector and the board sign in, signing out, what a restart does, the limits and their messages, every setting, and the one-time Google client and App Platform setup an operator needs.

*Technical detail:* [context.md#task-document-sign-in-and-limits](./context.md#task-document-sign-in-and-limits)

**Acceptance criteria**:
- [x] The design document explains connector and board sign-in, sign-out and the effect of a restart.
- [x] Every new setting is listed with its default, and the Google client setup is described step by step.
- [x] The document states that the desktop never asks for sign-in.

## Open Questions

- **Does claude.ai complete this sign-in flow against the deployed playmat?** Depends on claude.ai's live connector client (documented to support DCR with S256 PKCE, but there are open user reports of it not calling the token endpoint after a code is issued), only observable on the deployed app at the end of Milestone 1. If claude.ai fails to register, never reaches Google, or never exchanges its code, capture what the server logged (each sign-in endpoint request and its response status) and STOP and report it to the user before changing the flow; do not switch to another sign-in arrangement on your own.
- **What does claude.ai do after a restart wipes its sign-in?** The server answers with 401 `invalid_client` on refresh, Anthropic's documented signal to register again; whether claude.ai then prompts the user to sign in or only shows errors can only be seen after a real redeploy. If claude.ai just shows errors and gives the user no way to sign in again short of removing and re-adding the connector, record exactly what it shows and STOP and ask the user: meeting "Restart" may then need sign-ins that survive a restart, which the spec rules out.
- **Do Cloudflare and App Platform pass the sign-in cookies, redirects and Anthropic's server-to-server calls through unchanged?** Depends on the live proxy and bot settings for the owner's domain. If sign-in loops (the cookie is not kept), or claude.ai's calls to `/register` or `/token` are challenged or blocked, first check the Cloudflare bot/WAF settings named in the Google client task; if that does not fix it, STOP and ask the user before changing the hosting arrangement.

## Out of Scope

- **Separate games per account** — everyone signed in still shares the one current game. Tracked in the per-user-games spec (`20261017143118-b05aa6b1-per-user-games`), which builds on the account this plan attaches to every request and on these limits.
- **User profiles or per-user records** — nothing about an account is kept beyond its live sign-ins in memory (spec non-goal). Sign-ins and refusals appear only in the server log.
- **Billing or quotas beyond the two basic limits** (spec non-goal).
- **Sign-ins that survive a restart or redeploy** — ruled out by the spec; every restart signs everyone out.
- **Sign-in providers other than Google**, allowlists of accounts or email domains — the spec requires Google only and any account.
- **Signing the connector out from the board** — the board's Sign out ends the board session only; disconnecting the connector is done in claude.ai.
- **Client ID Metadata Documents and Anthropic-held client credentials** — claude.ai registers itself through Dynamic Client Registration; the other registration methods are not offered.
- **A consent page of the playmat's own** — Google's account chooser is the user's step; registered callback addresses are restricted instead.
- **Idle expiry of sign-ins** — places under the user cap free only on sign-out or restart; idle game clean-up belongs to per-user-games.
- **Any sign-in on the desktop** — Claude Code and Claude Desktop keep running the playmat locally with no login (spec constraint).
- **Packaging the tcg-* skills for claude.ai** — a later spec.

## Changelog


### 2026-10-17 — Task: Sign users in with Google

**What was done**: Added the new `internal/hostauth` package's foundation: its `Config`, an in-memory store with explicit lifetimes (pending logins, board sessions, signed-in accounts, keyed by the hash of each random value), and a Google sign-in that sends the browser to Google's account chooser with state, nonce and PKCE, checks Google's answer at `/auth/google/callback` against the browser's login cookie and the ID token's issuer, audience, expiry and nonce, and always leaves the browser with a board session cookie. Added an unauthenticated `/healthz`, minimal sign-in pages, and a test helper package with an in-process fake Google and a test browser.

**Deviations**: The test hooks (clock and HTTP client) are exported `Config` fields (`Now`, `HTTPClient`) rather than unexported options, because the hosted command's tests in another package build the service too. `hostauthtest` defines its own `Account` type and does not import `hostauth`, so in-package tests can use it without an import cycle. `hostauthtest.Browser` takes both the public address and the test server address and aliases one to the other, so tests can use an https public address (as the existing hosted tests do) served by a local test server. `go mod tidy` also moved `github.com/coder/websocket` to the direct requirements (it was already imported directly).

**Files changed**:
- `internal/hostauth/hostauth.go`
- `internal/hostauth/store.go`
- `internal/hostauth/google.go`
- `internal/hostauth/pages.go`
- `internal/hostauth/google_test.go`
- `internal/hostauth/hostauthtest/google.go`
- `go.mod`

**Discoveries**: The connector continuation is a stub returning 500 until the next task. The go-sdk's `RequireBearerToken` checks token expiry against the real clock, so the verifier added next must translate the injected clock's remaining lifetime into real time.

### 2026-10-17 — Task: Serve the connector sign-in endpoints

**What was done**: Added the connector's OAuth authorization server to `internal/hostauth`: protected-resource metadata (go-sdk `auth.ProtectedResourceMetadataHandler`) and RFC 8414 server metadata built from the public URL, Dynamic Client Registration restricted to Claude's callbacks and loopback, `/authorize` (S256 PKCE required, resource checked, always through Google's account chooser), a one-time 5-minute code continuation after the Google callback, and `/token` (code exchange with PKCE, refresh with rotation, RFC 6749 errors, 401 `invalid_client` for unknown clients so claude.ai registers again after a restart). `RequireToken` wraps the go-sdk bearer middleware with refusal logging, and its verifier puts the Google account on `TokenInfo.UserID`; `AccountFromToken` reads it back. Test helpers `hostauthtest.MCPTransport` (the go-sdk's own OAuth client end to end) and `hostauthtest.Connect` (manual flow) were added.

**Deviations**: A token request with no `grant_type` (including a JSON or query-string body) is answered 400 `invalid_request` before client authentication, rather than whatever the client check would say. The verifier hands the go-sdk the token's remaining lifetime on the real clock, because the library checks expiry with `time.Now`. Helper names differ from the plan: `Connect(t, public, server, g) Tokens` instead of `BearerToken`, and the helpers take the public and server addresses separately.

**Files changed**:
- `internal/hostauth/oauth.go`
- `internal/hostauth/store.go`
- `internal/hostauth/google.go`
- `internal/hostauth/hostauth.go`
- `internal/hostauth/oauth_test.go`
- `internal/hostauth/hostauthtest/client.go`

**Discoveries**: The go-sdk v1.6.0 `AuthorizationCodeHandler` works against `httptest` servers on 127.0.0.1 (its HTTPS checks allow loopback), so the whole connector flow runs in-process. The fake Google stamps ID-token expiry from the real clock while the playmat checks it with the injected clock, so tests must sign in before moving the injected clock more than an hour ahead.

### 2026-10-17 — Task: Require sign-in for the hosted connector

**What was done**: Hosted mode now builds the sign-in service before it listens: new settings `-google-client-id` / `TCGREPLAY_GOOGLE_CLIENT_ID`, the secret from `TCGREPLAY_GOOGLE_CLIENT_SECRET` only, and `-rate-limit` / `TCGREPLAY_RATE_LIMIT` (300) and `-max-users` / `TCGREPLAY_MAX_USERS` (10), with start-up refused when either credential is missing. The hosted handler mounts the sign-in routes and metadata, puts `/mcp` behind the token check, and serves an unauthenticated `/healthz`. `.do/app.yaml` uses `/healthz` for the health check and declares the two Google credentials as secrets plus the two limits; the Makefile's `docker-run` and the Dockerfile's notes pass and name the new settings. Every hosted test now runs signed in through a fake Google.

**Deviations**: `hostedAuth` takes Google's endpoints as a third argument (zero = the real Google) so the hosted tests wire exactly like `runHosted` with a fake Google. `newHosted` already takes the `*hostauth.Auth`, used by the limit task. The tests that run the real binary or `runHosted` cannot sign in against the real Google, so they check readiness on `/healthz` and that an anonymous `/mcp` request gets the 401 naming the public address. The start-up log line now also names Google sign-in and the two limits.

**Files changed**:
- `cmd/tcgreplay/main.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_test.go`
- `.do/app.yaml`
- `Makefile`
- `Dockerfile`

**Discoveries**: Because the owner created the App Platform app in the console rather than from `.do/app.yaml`, the new environment values (and the `/healthz` health check) must also be set in the console; the deployed app will not start after this lands until the Google credentials are set.

### 2026-10-17 — Task: Guard the board with Google sign-in

**What was done**: Put the hosted board behind a Google sign-in held in the `tcgreplay_session` cookie. `RequireSession` sends a signed-out page request through `/auth/login` and Google back to the page asked for, refuses a signed-out `/ws`, `/api/*` or non-GET request with 401 (logged), puts the account on signed-in requests, and cancels a live connection's context when its session ends. Added `/auth/session` (who is signed in), `POST /auth/logout` (Origin or Referer must be the public host; ends the session, closes its live connections, shows `/auth/signed-out`) and the signed-out page. The hosted handler now wraps the board in `RequireSession`; the desktop board is untouched.

**Deviations**: Ran this task before the human task "Create the Google sign-in client and deployment secrets", as the orchestrator directed (it does not depend on it); the human task stays unchecked. A signed-out non-GET/HEAD request to the board is refused with 401 rather than redirected. Also fixed a data race in the hosted tests' subprocess helper (`helperCmd` now writes stderr to a mutex-guarded buffer).

**Files changed**:
- `internal/hostauth/board.go`
- `internal/hostauth/store.go`
- `internal/hostauth/hostauth.go`
- `internal/hostauth/board_test.go`
- `internal/hostauth/hostauthtest/client.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_test.go`

**Discoveries**: Cancelling the websocket request's context is enough to close a live board connection: `serveWS` derives its connection context from `r.Context()`, so no hub change was needed. `go test -race ./cmd/tcgreplay/` reports an existing data race in `TestRelaySurvivesDaemonRestart` (`cmd/tcgreplay/shim_test.go:120-128`), which this spec did not touch; `make test` (no race detector) passes.

### 2026-10-17 — Task: Show the account and Sign out on the board

**What was done**: The board client asks `/auth/session` who is signed in (new `web/src/lib/session.js`). On the hosted playmat the header shows the account's email and a Sign out button (a form POST to `/auth/logout`); when the live connection closes and the session has ended (a sign-out elsewhere or a restart), the page sends the user through sign-in and back to the same page instead of reconnecting for ever. On the desktop the route does not exist, so there is no account or button and reconnecting works as before.

**Deviations**: Added a `signingOut` flag set when the Sign out form is submitted, so the live connection that the sign-out closes does not send the browser straight back through sign-in before the signed-out page loads. No automated tests: the repo has no front-end tests, as the plan expected; the behaviour goes into the manual test plan. Verified by a clean `npm run build` (in the worktree) and `make test`.

**Files changed**:
- `web/src/lib/session.js`
- `web/src/lib/store.svelte.js`
- `web/src/lib/ws.js`
- `web/src/main.js`
- `web/src/App.svelte`

**Discoveries**: Ending a board session closes its websocket from the server at once, which races the browser's own navigation after the sign-out POST; any client logic that reacts to a closed connection has to know a sign-out is in progress.

### 2026-10-17 — Task: Limit requests per account

**What was done**: Added a per-account token bucket (`TCGREPLAY_RATE_LIMIT` / `-rate-limit` requests a minute, default 300, 0 = off) that counts every MCP request (go-sdk receiving middleware, notifications excluded), every board action (new optional `webui.Server.Limit` hook, called first in `dispatch`) and every `/api/*` lookup (in `RequireSession`). Over the limit a tool call returns a tool error naming the limit, other MCP methods return the same message as an error, a board action returns it as its error toast, and a card lookup gets 429; other accounts are unaffected. Refusals are logged at most once per 10 seconds per account. Hosted mode wires the middleware and the hook in `newHosted`; the desktop sets neither.

**Deviations**: None in behaviour. Idle buckets (untouched for a minute, so full again) are dropped when the store sweeps on writes, which keeps memory bounded by active accounts.

**Files changed**:
- `internal/hostauth/limit.go`
- `internal/hostauth/store.go`
- `internal/hostauth/board.go`
- `internal/hostauth/limit_test.go`
- `internal/webui/server.go`
- `internal/webui/actions.go`
- `internal/webui/webui_test.go`
- `cmd/tcgreplay/hosted.go`
- `cmd/tcgreplay/hosted_test.go`

**Discoveries**: The MCP `initialize` and `tools/list` requests count against the limit too, so a very low limit (1-2) can refuse the first tool call after connecting; the default of 300 a minute leaves plenty of room.

### 2026-10-17 — Task: Cap the number of signed-in users

**What was done**: Added the cap on signed-in accounts (`TCGREPLAY_MAX_USERS` / `-max-users`, default 10, 0 = off). It is applied once, when the Google callback creates the browser's session, so it covers both the board and the connector flow. A new account over the cap sees a 503 "tcgreplay is busy" page and the refusal is logged; an account already signed in always gets back in. An account holds its place while it has any live sign-in: a board session, a connector refresh token, or a connector code not yet exchanged. The place frees when the last one ends, and every place frees on restart.

**Deviations**: The cap check lives inside the store's session creation, under the same lock as the count, instead of a separate `admit` step before `signIn`, so two new accounts signing in at once cannot both take the last place. Changed from the plan's counting: an unexchanged authorization code now holds a live sign-in until it is exchanged or expires. Refresh rotation and code exchange hand the count straight to the new refresh token rather than releasing and re-taking it. Without both changes, an account could sign out of the board between getting a code and exchanging it (or during a refresh) and push the server one account over the cap. The test sub-agent found both cases; a regression test covers them.

**Files changed**:
- `internal/hostauth/store.go`
- `internal/hostauth/google.go`
- `internal/hostauth/limit.go`
- `internal/hostauth/pages.go`
- `internal/hostauth/oauth.go`
- `internal/hostauth/cap_test.go`
- `cmd/tcgreplay/hosted_cap_test.go`

**Discoveries**: Each connector sign-in also leaves a board session in that browser, and sessions have no idle expiry, so an account that connected claude.ai keeps its place until it signs out of the board in that browser (and its connector is disconnected) or the server restarts. This is the intended "no idle expiry" behaviour, but it means the cap fills by distinct accounts ever signed in since the last deploy.

### 2026-10-17 — Task: Document sign-in and limits

**What was done**: The design document's hosted-mode description now covers sign-in. A new "Hosted sign-in" section describes connector sign-in and board sign-in, Sign out (board only), what a restart does, the request limit and the user cap with their messages, and what is logged. It also has a table of every new setting with its default and the one-time operator setup: the Google consent screen and Web application client, the App Platform values and health check, the Cloudflare paths and re-adding the connector. It states that the desktop modes never ask for sign-in. The hosted-mode paragraph drops "(trial)" and "no login", and names the `/healthz` health check and the credentials needed for `make docker-run`. The command's package comment already says hosted mode requires Google sign-in (written with the connector task), and the `hostauth` package comment matches the final behaviour.

**Deviations**: None.

**Files changed**:
- `TCG Review Playmat Design and Build Plan.md`

**Discoveries**: None.

### 2026-10-18 — Task: Create the Google sign-in client and deployment secrets

**What was done** (by the owner, from the setup steps the run handed back):
- **Google Cloud:** created a new project `tcgreplay`, rather than an existing one whose consent screen belongs to another app. In Google Auth Platform it is External and **In production**, with only the `openid` and `userinfo.email` scopes. The home page is https://tcgreplay.demo.gs, the privacy policy link is https://github.com/nicholasjackson/tcgreplay, and the authorised domains are `demo.gs` and `github.com`. The Web application client `tcgreplay` has no JavaScript origins and the redirect URIs `https://tcgreplay.demo.gs/auth/google/callback` and `http://localhost:8080/auth/google/callback`.
- **App Platform console:** the owner set `TCGREPLAY_GOOGLE_CLIENT_ID` and `TCGREPLAY_GOOGLE_CLIENT_SECRET`, encrypted, at run time. `TCGREPLAY_PUBLIC_URL` is the live domain.
- **Cloudflare** (demo.gs, free plan): Bot Fight Mode was already off and there are no custom, WAF or rate-limit rules, so nothing needed changing.

**Deviations**: Done after the Milestone 2 and 3 agent tasks rather than before them, as the orchestrator directed. The App Platform health check is still `/`; the owner switches it to `/healthz` once this branch deploys, because the build running now has no `/healthz`. Two criteria stay unticked until this branch is merged and deployed, and they are in the test plan:
- "starts after a redeploy"
- "reachable from outside without a Cloudflare challenge"

**Files changed**:
- None (external setup only).

**Discoveries**: Google's consent screen in production needs a home page and a privacy policy link on authorised domains, even for the non-sensitive `openid` and `email` scopes.
