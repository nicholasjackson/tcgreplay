package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// readOnlyTools never change a game: they read the board, its log, its
// record, card text or a guide, or wait for the player. search_deck and
// look_top only add to the hidden-information audit. Clients such as
// ChatGPT ask the user before any tool not marked read-only, so these are
// marked to spare the user a prompt for every look at the board.
var readOnlyTools = map[string]bool{
	"ping":          true,
	"get_record":    true,
	"get_state":     true,
	"get_log":       true,
	"get_warnings":  true,
	"card_text":     true,
	"wait_for_turn": true,
	"search_deck":   true,
	"look_top":      true,
	"list_branches": true,
	"get_profile":   true,
	"guide":         true,
	"get_account":   true,
}

// replacingTools replace the account's current game, losing anything not
// kept in a record. Every other write can be undone or rewound on the
// playmat, so it is not destructive.
var replacingTools = map[string]bool{
	"new_game":          true,
	"import_log":        true,
	"set_opponent_deck": true,
}

// annotations are the MCP behaviour hints for the tool called name. No tool
// reaches beyond the playmat, so none is open-world. The read-only flag of
// a write cannot be sent as false through these (see explicitLabels).
func annotations(name string) *mcp.ToolAnnotations {
	closed := false
	destructive := replacingTools[name]
	if readOnlyTools[name] {
		return &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: &destructive, OpenWorldHint: &closed}
	}
	return &mcp.ToolAnnotations{DestructiveHint: &destructive, OpenWorldHint: &closed}
}

// oauth2 is the security scheme every hosted tool declares in its _meta:
// each call needs the player's sign-in, as OpenAI asks tools to say.
var oauth2 = []map[string]any{{"type": "oauth2"}}

// tool is the definition of the tool called name: its title, description
// and behaviour hints, and in hosted mode (a resolver is set) the sign-in
// it needs. Every tool is defined through here.
func (s *Server) tool(name, title, desc string) *mcp.Tool {
	t := &mcp.Tool{Name: name, Title: title, Description: desc, Annotations: annotations(name)}
	if s.Resolve != nil {
		t.Meta = mcp.Meta{"securitySchemes": oauth2}
	}
	return t
}

// explicitLabels is a receiving middleware that makes every tool in a
// tools/list result state readOnlyHint, destructiveHint and openWorldHint
// as an explicit true or false, as the tool declared them (see tool and
// annotations). A tool that declared no annotations, or left a pointer
// hint unset, is sent as it is, without the missing label, so the
// metadata test catches a tool registered without them.
// go-sdk v1.6.0 declares ToolAnnotations.ReadOnlyHint as a bool with
// omitempty (mcp/protocol.go), so a write's readOnlyHint:false never
// reaches the wire through the SDK's own type; OpenAI's review asks for
// all three on every tool. Every other method passes through. The server
// installs it itself, so the desktop, hosted and moved servers all send
// the same labels. Delete it once the SDK can express a false read-only
// flag.
func explicitLabels() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if method != "tools/list" || err != nil {
				return res, err
			}
			if lr, ok := res.(*mcp.ListToolsResult); ok && lr != nil {
				return labelledTools{lr}, nil
			}
			return res, err
		}
	}
}

// labelledTools is a tools/list result whose JSON encoding writes all
// three behaviour labels on every tool (see explicitLabels). Embedding the
// SDK's result keeps it an mcp.Result.
type labelledTools struct{ *mcp.ListToolsResult }

// MarshalJSON encodes the result as the SDK does, then sets each tool's
// declared readOnlyHint, destructiveHint and openWorldHint explicitly.
func (l labelledTools) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(l.ListToolsResult)
	if err != nil {
		return nil, err
	}
	var res map[string]json.RawMessage
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, err
	}
	tools := make([]json.RawMessage, 0, len(l.Tools))
	for _, t := range l.Tools {
		tb, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		a := t.Annotations
		if a == nil {
			tools = append(tools, tb)
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(tb, &fields); err != nil {
			return nil, err
		}
		labels := map[string]any{}
		if raw, ok := fields["annotations"]; ok {
			if err := json.Unmarshal(raw, &labels); err != nil {
				return nil, err
			}
		}
		labels["readOnlyHint"] = a.ReadOnlyHint
		if a.DestructiveHint != nil {
			labels["destructiveHint"] = *a.DestructiveHint
		}
		if a.OpenWorldHint != nil {
			labels["openWorldHint"] = *a.OpenWorldHint
		}
		if fields["annotations"], err = json.Marshal(labels); err != nil {
			return nil, err
		}
		if tb, err = json.Marshal(fields); err != nil {
			return nil, err
		}
		tools = append(tools, tb)
	}
	if res["tools"], err = json.Marshal(tools); err != nil {
		return nil, err
	}
	return json.Marshal(res)
}
