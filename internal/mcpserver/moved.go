package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MovedText is the one message an assistant still connected at an address
// the playmat has moved from gets: that the playmat has moved, and to add
// it again at mcpURL, its new connection address. It names no assistant,
// as every assistant shows it.
func MovedText(mcpURL string) string {
	return "The tcgreplay playmat has moved to a new address. This connection no longer works: remove it, then add the playmat again at " + mcpURL + " and sign in."
}

// Moved is a receiving middleware for a playmat MCP server reached at an
// address the playmat has moved from (see cmd/tcgreplay's moved-host
// guard). The server still lists its usual tools and guide prompts, so an
// assistant that kept the old tool list reaches the message whichever
// tool it calls, but:
//
//   - initialize answers with MovedText as the instructions;
//   - every tool call answers with MovedText as an error result, without
//     running the tool;
//   - every prompt request answers with MovedText as the prompt.
//
// Every other method passes through. As no tool handler ever runs, the
// server it wraps needs no game manager or resolver.
func Moved(mcpURL string) mcp.Middleware {
	text := MovedText(mcpURL)
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case "initialize":
				res, err := next(ctx, method, req)
				if ir, ok := res.(*mcp.InitializeResult); ok && err == nil {
					ir.Instructions = text
				}
				return res, err
			case "tools/call":
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
			case "prompts/get":
				return &mcp.GetPromptResult{
					Description: "The playmat has moved",
					Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}},
				}, nil
			}
			return next(ctx, method, req)
		}
	}
}
