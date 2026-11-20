---
tags: [mcp, go-sdk, chatgpt, testing]
---

# go-sdk v1.6.0 cannot send `readOnlyHint: false`

In go-sdk v1.6.0, `mcp.ToolAnnotations.ReadOnlyHint` is a plain `bool` tagged `omitempty` (`mcp/protocol.go`, around line 1381). A write tool therefore never sends `readOnlyHint: false`: the key is simply missing from `tools/list`. `DestructiveHint` and `OpenWorldHint` are `*bool` and can be sent as `false`.

OpenAI's plugin review wants all three hints present as explicit booleans on every tool. Setting the struct fields is not enough.

- **Getting explicit values on the wire (built and confirmed, 2026-11-13).** `mcp.Result` has an unexported method, but a type that embeds `*mcp.ListToolsResult` satisfies it. `internal/mcpserver/annotations.go` has a receiving middleware, `explicitLabels`, that wraps only `tools/list` and returns `labelledTools{res}`, whose `MarshalJSON` writes all three hints on every tool from `readOnlyTools` and `replacingTools`. The SDK's JSON-RPC layer encodes results with `encoding/json`, so the wrapper's encoding is what clients receive on both the in-memory and Streamable HTTP transports. `Server.MCP()` installs it itself, so the desktop, hosted and moved servers all get it. The server-side SDK never type-asserts the `tools/list` result after middleware (only the client reads `nextCursorPtr`), so the wrapper is safe.
- **Tests must read raw JSON.** The Go client decodes `readOnlyHint` into the same `bool`, so a missing key and `false` look identical through `cs.ListTools`. Read the raw response with a bare connection instead: call `Connect(ctx)` on the client-side `mcp.Transport` (in-memory or `StreamableClientTransport`), write `jsonrpc.Request`s for `initialize`, `notifications/initialized` and `tools/list`, and read the `*jsonrpc.Response` whose `Result` is raw JSON. No session-update call is needed for Streamable HTTP. The helper is `rawToolListBytes` in `internal/mcpserver/annotations_test.go`.
- Remove the workaround if a later go-sdk makes `ReadOnlyHint` a pointer.

Related: [[go-sdk-sessions-and-auth]], [[publishing-to-chatgpt]].
