package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// accountInfo is get_account's result: the signed-in account's ID only.
type accountInfo struct {
	ID string `json:"id"`
}

// accountDescription is get_account's description.
const accountDescription = "Use this when you need to know which TCG Replay account is signed in, for example to tell connected accounts apart. It returns the account's ID, which never changes for that account. Do not use this to read a game or the player's details; it returns only an opaque account ID."

// registerAccount adds get_account, OpenAI's profile tool: it answers who
// is signed in with the account ID s.Account gives for the request (the
// Google account ID in hosted mode). Like guide, it is registered with
// mcp.AddTool rather than add, so asking needs no game, never takes a
// place, is not activity and is never refused as busy. It is offered only
// when s.Account is set.
func (s *Server) registerAccount(srv *mcp.Server) {
	t := s.tool("get_account", "Who is signed in", accountDescription)
	t.Meta = mcp.Meta{"openai/profile": true, "securitySchemes": oauth2}
	// OpenAI's profile tool takes no input and returns only a non-blank id.
	t.InputSchema = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	t.OutputSchema = map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"id": map[string]any{"type": "string", "minLength": 1, "pattern": `\S`}},
		"required":             []string{"id"},
		"additionalProperties": false,
	}
	mcp.AddTool(srv, t, func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, accountInfo, error) {
		var extra *mcp.RequestExtra
		if req != nil {
			extra = req.Extra
		}
		id, err := s.Account(ctx, extra)
		if err != nil {
			return nil, accountInfo{}, err
		}
		return nil, accountInfo{ID: id}, nil
	})
}
