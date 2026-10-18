DONE summary for 20261017123240-21493a95-google-login (kept for Step 6 of plan-epic)

Approach: new internal/hostauth package; playmat is its own small OAuth 2.1 authorization server for claude.ai, delegating login to Google (OIDC via golang.org/x/oauth2). Serves RFC 9728 resource metadata, RFC 8414 server metadata, DCR /register, /authorize, /token, S256 PKCE required. /mcp protected by go-sdk auth.RequireBearerToken; TokenInfo.UserID = Google account ID (SDK binds sessions to it). Board (/, /ws, /api/card) behind Google sign-in session cookie: signed-out page -> Google and back; signed-out ws/card -> 401; sign-out closes that session's ws; Google callback always sets board cookie. All sign-in state in memory, opaque random tokens; restart signs everyone out. Access tokens 1h; refresh until restart, rotated. After restart refresh gets 401 invalid_client (claude.ai re-registers). Limits: per-account request budget (MCP, board actions, card lookups) -> tool error / board message / 429; signed-in account cap -> "server busy" at callback. Hosted only: webui.Server gets optional Limit hook (nil on desktop); board shows sign-in UI only when /auth/session exists. Rejected: Google directly as AS (no DCR), third-party OAuth lib, JWT, HTTP-only 429, CIMD, reusing board cookie at /authorize. Based on claude.com/docs/connectors/building/authentication.

Milestones:
M1 Claude in claude.ai signs in with Google before using the playmat:
- Sign users in with Google
- Serve the connector sign-in endpoints
- Require sign-in for the hosted connector
- HUMAN TASK: Create the Google sign-in client and deployment secrets (Google OAuth web client, redirect https://<domain>/auth/google/callback + localhost; consent screen "In production"; two secrets in App Platform; doctl apps update; remove old authless connector from claude.ai; Cloudflare must not challenge 160.79.104.0/21 on sign-in paths)
M2 Board needs Google sign-in, user can sign out:
- Guard the board with Google sign-in
- Show the account and Sign out on the board (Svelte)
M3 Limits keep one account from overloading:
- Limit requests per account
- Cap the number of signed-in users
- Document sign-in and limits

Out of scope: per-account games; profiles/per-user records; billing/quotas beyond two limits; sign-ins surviving restart; other providers, allowlists; signing connector out from board; CIMD & Anthropic-held client creds; own consent page; idle expiry of sign-ins; desktop login; tcg-* skills packaging.

Drafting assumptions: own AS with Google behind; DCR only, no CIMD; opaque in-memory tokens; Google ID token checked for iss/aud/exp/nonce without signature check (direct from token endpoint over TLS, OIDC Core 3.1.3.7); redirect URIs limited to claude.ai and claude.com auth_callback + loopback any port; every connector sign-in goes through Google account chooser, no own consent page; board sign-out ends board session only, connector tokens not revoked; hosted refuses to start without Google creds, no bypass; user cap counts accounts with any live sign-in, frees only when last sign-in ends or restart, no idle expiry; defaults 300 req/min/account and 10 accounts, 0 disables; secret env only; limit counts MCP, board actions, card lookups not page/static; sign-ins (with email) and refused requests logged; one scope `playmat`, refresh tokens always issued; -rate-limit/-max-users flags added in connector task, wired in M3; health check moves to unauthenticated /healthz.

Project-wide rules:
- Relies on tcgreplay -hosted, one plain-HTTP router with /mcp and board.
- Relies on -public-url / TCGREPLAY_PUBLIC_URL as only address source; issuer, metadata, /mcp resource, /auth/google/callback built from it.
- Relies on optional-field hosted switch; adds webui.Server.Limit; hostauth built only in runHosted.
- Relies on everything in memory, lost on restart.
- Relies on secrets from App Platform, one instance.
- Relies on make test; fake Google in new internal/hostauth/hostauthtest.
- Relies on no changelog.
- DIFFERS from web-mcp-docker: .do/app.yaml health check / -> /healthz.
- Decides settings: TCGREPLAY_GOOGLE_CLIENT_ID (-google-client-id), TCGREPLAY_GOOGLE_CLIENT_SECRET (env only), TCGREPLAY_RATE_LIMIT (-rate-limit, 300), TCGREPLAY_MAX_USERS (-max-users, 10).
- Decides signature hostedHandler(srv, ui, a *hostauth.Auth); web-mcp-docker hosted tests updated to sign in via hostauthtest.
- Decides account access for later specs: board hostauth.AccountFrom(ctx); MCP hostauth.AccountFromToken(req.Extra.TokenInfo); account ID = Google sub.
- Overlap: web-mcp-docker first; memory-store-record also edits cmd/tcgreplay/hosted.go, main.go.

Manual checks:
- Success metric: owner connects claude.ai on phone and laptop with one Google sign-in each.
- Success metric: a week of prod logs shows zero unsigned requests reaching game actions (also automated).
- Success metric: re-sign-in after deploy under a minute.
- Adding connector opens Google sign-in; tools listed; real import succeeds.
- Two personal Google accounts each sign in (consent screen published).
- Board link on phone while signed out goes through Google and lands on board.
- A day of connector use + board reloads without deploy never asks for sign-in.
- After redeploy, both claude.ai and board ask for sign-in then work; record what claude.ai shows.
- Sign out on board, reload asks for sign-in.
- Limits set low on deployed app: "too many requests" reaches user via Claude and board; second account sees "server busy".
- Changing creds/limits in App Platform + restart takes effect without rebuild.
- Claude Code & Desktop still use local playmat without sign-in.
- Cookies & redirects work through Cloudflare & App Platform (end of M1).

Open for implementation (STOP and ask): does claude.ai complete the flow (reports of never calling /token); after restart does claude.ai re-prompt sign-in or just error (may need persistent sign-ins, spec rules out -> user); Cloudflare bot/WAF blocking Anthropic calls or cookies.

Discoveries:
- claude.ai connector OAuth requirements: anonymous -> 401 with WWW-Authenticate resource_metadata; metadata resource must equal MCP URL exactly, only first authorization_servers used; DCR public client, refresh rotated; dead refresh -> invalid_grant, 401 invalid_client forces re-registration; token endpoint form-encoded, /register JSON; 10s budget (30s refresh); callbacks claude.ai/claude.com /api/mcp/auth_callback, Claude Code loopback any port; Anthropic calls from 160.79.104.0/21. Tags: auth, oauth, mcp, claude-ai, connector.
- go-sdk v1.6.0 auth: resource-server half only (RequireBearerToken, ProtectedResourceMetadataHandler, oauthex), no AS; Streamable sessions bound to TokenInfo.UserID; req.GetExtra().TokenInfo reaches middleware; auth.AuthorizationCodeHandler usable as test OAuth client. Tags: go-sdk, mcp, auth.
