package usergames

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
)

// errSignIn refuses a tool call that carries no signed-in account. The
// HTTP layer already refuses anonymous requests, so this is defensive.
var errSignIn = errors.New("sign in required")

// ResolveMCP is the MCP tool server's Resolve hook: it finds the caller's
// account from the request's token and returns that account's game
// manager and board link. A tool call counts as the account's activity.
func (r *Registry) ResolveMCP(_ context.Context, extra *mcp.RequestExtra) (*game.Manager, string, error) {
	if extra == nil {
		return nil, "", errSignIn
	}
	acct, ok := hostauth.AccountFromToken(extra.TokenInfo)
	if !ok {
		return nil, "", errSignIn
	}
	e, err := r.Use(acct)
	if err != nil {
		return nil, "", err
	}
	return e.M, e.Link(r.cfg.Public), nil
}
