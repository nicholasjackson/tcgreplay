---
tags: [mcp, go-sdk, auth, transport]
---

# go-sdk v1.6.0: Streamable HTTP sessions and auth traps

- **Resource server only.** The SDK provides `auth.RequireBearerToken`, `ProtectedResourceMetadataHandler` and the `oauthex` types, but no authorization server. Serving claude.ai's OAuth needs our own.
- **Sessions are bound to `TokenInfo.UserID`.** `req.GetExtra().TokenInfo` reaches MCP middleware and tool handlers.
- **`getServer` runs once per session**, at creation (`mcp/streamable.go`, around lines 1142-1164). Per-user state chosen there goes stale (for example after an idle drop). Look the user's state up on every request from `RequestExtra.TokenInfo` instead.
- **Localhost protection.** The Streamable handler rejects a request that arrives on a loopback listener with a non-loopback `Host` unless `DisableLocalhostProtection` is set. This bites behind tunnels and proxies.
- **Server `KeepAlive` closes the session** when a ping goes unanswered, so enabling it can drop clients that do not answer pings.
- `auth.AuthorizationCodeHandler` works as an OAuth client in tests.
