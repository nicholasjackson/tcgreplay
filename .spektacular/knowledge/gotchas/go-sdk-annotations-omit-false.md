---
tags: [mcp, go-sdk, chatgpt, testing]
---

# go-sdk v1.6.0 cannot send `readOnlyHint: false`

In go-sdk v1.6.0, `mcp.ToolAnnotations.ReadOnlyHint` is a plain `bool` tagged `omitempty` (`mcp/protocol.go`, around line 1381). A write tool therefore never sends `readOnlyHint: false`: the key is simply missing from `tools/list`. `DestructiveHint` and `OpenWorldHint` are `*bool` and can be sent as `false`, but `internal/mcpserver/annotations.go` sets no `DestructiveHint` on read-only tools, so those send no destructive key either.

OpenAI's plugin review wants all three hints present as explicit booleans on every tool. Setting the struct fields is not enough.

- **Getting explicit values on the wire.** `mcp.Result` has an unexported method, but a type that embeds `*mcp.ListToolsResult` satisfies it. A server receiving middleware on `tools/list` can return such a wrapper with its own `MarshalJSON`. The SDK's JSON-RPC layer encodes results with `encoding/json` (`internal/jsonrpc2/messages.go`, `NewResponse`), so the wrapper's encoding is what clients receive, on every transport. This is the approach the chatgpt-submission-prep plan takes; confirm it with a raw-JSON test when it is built.
- **Tests must read raw JSON.** The Go client decodes `readOnlyHint` into the same `bool`, so a missing key and `false` look identical through `cs.ListTools`. A test that checks hints are explicit has to read the raw `tools/list` response, for example by POSTing to an `httptest` Streamable HTTP handler, or by reading a raw connection from `mcp.NewInMemoryTransports()`.
- Remove the workaround if a later go-sdk makes `ReadOnlyHint` a pointer.

Related: [[go-sdk-sessions-and-auth]], [[publishing-to-chatgpt]].
