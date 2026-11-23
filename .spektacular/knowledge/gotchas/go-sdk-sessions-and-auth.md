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
- **Middleware order across calls is reversed.** `AddReceivingMiddleware` wraps the current handler with each call's list applied in `slices.Backward` (`mcp/shared.go`, `addMiddleware`). Within one call the first middleware is outermost, but a *later* call wraps *outside* everything added before it. To nest B inside A (for example a recorder inside the rate limiter, so refused requests never reach it), pass both in one call: `AddReceivingMiddleware(A, B)`.

- **Tool failures reach middleware as results, not errors.** A typed `mcp.AddTool` handler's returned error is turned into a `*mcp.CallToolResult` with `IsError: true` and the message as its first `TextContent` (`mcp/server.go`, `SetError`), with a nil Go error. A receiving middleware that wants to see failures must inspect `IsError` on the result, not only the error.


- **A tool handler's context is the initialize request's, not the current request's.** The Streamable handler connects each session with the context of the HTTP request that created it (`mcp/streamable.go:488`, `server.Connect(req.Context(), ...)`), and every later call's context descends from it. So anything taken from `ctx` in a receiving middleware or tool (a trace span, request-scoped values) belongs to that first, long-finished request. The current HTTP request's headers do reach every call, as `req.GetExtra().Header` (`streamable.go:1162-1166`). To parent a tool span on the current `POST /mcp`, inject the trace context into `r.Header` in an HTTP wrapper around the Streamable handler and extract it from `extra.Header` in the middleware.
  The playmat does exactly this: `internal/observe/mcp.go` has `CarryContext`, inside `RequireToken`, and `MCPMiddleware`, which is first in the one `AddReceivingMiddleware` call so it runs outermost.
- **There is no accessor for a server's registered tools.** `mcp.Server` has `AddTool` and `RemoveTools` but nothing that lists them. To get the names (for example a closed set of metric labels), connect a client over `mcp.NewInMemoryTransports()` and iterate `ClientSession.Tools`. `toolNames` in `cmd/tcgreplay/hosted.go` does this once at start-up. Receiving middleware sees that session too, with no `TokenInfo`, so keep such middleware a no-op for requests without an account.
