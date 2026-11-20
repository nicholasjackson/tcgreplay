package usergames

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
	"github.com/nicholasjackson/tcgreplay/internal/mcpserver"
	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// MCPMiddleware records every MCP tool call that reaches the tools: a
// tool_call record naming the tool and the assistant (from the session's
// client name, as the playmat names it), import or new_game when those
// tools succeed, and error, with the message redacted, when a call fails.
// The account comes from each request's token and the assistant from each
// request's session, never from anything cached at session creation. It
// must run inside the rate limit, so a refused request is recorded once,
// as limited, by the limit. It never changes the result.
func (r *Registry) MCPMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			call, ok := req.(*mcp.CallToolRequest)
			if method != "tools/call" || !ok || r.cfg.Recorder == nil {
				return next(ctx, method, req)
			}
			var acct hostauth.Account
			if extra := req.GetExtra(); extra != nil {
				acct, _ = hostauth.AccountFromToken(extra.TokenInfo)
			}
			var assistant string
			if call.Session != nil {
				if p := call.Session.InitializeParams(); p != nil && p.ClientInfo != nil {
					assistant = mcpserver.AssistantName(p.ClientInfo.Name)
				}
			}
			var tool string
			var args json.RawMessage
			if call.Params != nil {
				tool, args = call.Params.Name, call.Params.Arguments
			}

			res, err := next(ctx, method, req)

			rec := r.cfg.Recorder
			rec.Record(telemetry.Event{Name: telemetry.ToolCall, Email: acct.Email, Tool: tool, Assistant: assistant})
			failed, msg := err != nil, ""
			if failed {
				msg = err.Error()
			} else if cr, ok := res.(*mcp.CallToolResult); ok && cr != nil && cr.IsError {
				failed, msg = true, resultText(cr)
			}
			switch {
			case failed:
				sensitive := append(telemetry.SensitiveFromJSON(args), r.gameNames(acct)...)
				rec.Record(telemetry.Event{Name: telemetry.Error, Email: acct.Email, Tool: tool, Message: telemetry.Redact(msg, sensitive)})
			case tool == "import_log":
				rec.Record(telemetry.Event{Name: telemetry.Import, Email: acct.Email, Assistant: assistant})
			case tool == "new_game":
				rec.Record(telemetry.Event{Name: telemetry.NewGame, Email: acct.Email, Assistant: assistant})
			}
			return res, err
		}
	}
}

// resultText is a tool result's first text.
func resultText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// BoardError is the board's error hook (webui.Server.OnError): it records
// a failed board action as error, naming the action, with the message
// redacted against the account's current game. Limit refusals, already
// recorded as limited, are not recorded again.
func (r *Registry) BoardError(ctx context.Context, action string, err error) {
	if r.cfg.Recorder == nil || err == nil || errors.Is(err, hostauth.ErrRateLimited) || errors.Is(err, hostauth.ErrBusy) {
		return
	}
	acct, _ := hostauth.AccountFrom(ctx)
	r.cfg.Recorder.Record(telemetry.Event{Name: telemetry.Error, Email: acct.Email, Action: action, Message: telemetry.Redact(err.Error(), r.gameNames(acct))})
}

// gameNames returns the player text in acct's current game, for redaction:
// its players' names, the name of every card in it, and, for an imported
// game, every line of its battle log and decklists. It does not count as
// activity, and gives nil when the account has no game open.
func (r *Registry) gameNames(acct hostauth.Account) []string {
	if acct.ID == "" {
		return nil
	}
	r.mu.Lock()
	e := r.entries[acct.ID]
	r.mu.Unlock()
	if e == nil {
		return nil
	}
	g, err := e.M.Current()
	if err != nil {
		return nil
	}
	return gameText(g)
}

// gameText is gameNames for one game.
func gameText(g *game.Game) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	st := g.State()
	for _, side := range st.Sides {
		if side != nil {
			add(side.Name)
		}
	}
	db := g.DB()
	for _, c := range st.Cards {
		add(c.Name)
		if c.Def != "" && db != nil {
			if card := db.Cached(c.Def); card != nil {
				add(card.Name)
			}
		}
	}
	if rec, ok := g.Record(); ok {
		raw, _ := json.Marshal([]string{rec.Me, rec.Log, rec.Deck, rec.OppDeck})
		for _, s := range telemetry.SensitiveFromJSON(raw) {
			add(s)
		}
	}
	return out
}
