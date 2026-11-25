package usergames

import (
	"context"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
	"github.com/nicholasjackson/tcgreplay/internal/importreports"
	"github.com/nicholasjackson/tcgreplay/internal/mcpserver"
	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// errSignIn refuses a tool call that carries no signed-in account. The
// HTTP layer already refuses anonymous requests, so this is defensive.
var errSignIn = errors.New("sign in required")

// ResolveMCP is the MCP tool server's Resolve hook: it finds the caller's
// account from the request's token and returns that account's game
// manager and board link. A tool call counts as the account's activity.
func (r *Registry) ResolveMCP(ctx context.Context, extra *mcp.RequestExtra) (*game.Manager, string, error) {
	if extra == nil {
		return nil, "", errSignIn
	}
	acct, ok := hostauth.AccountFromToken(extra.TokenInfo)
	if !ok {
		return nil, "", errSignIn
	}
	e, err := r.Use(ctx, acct)
	if err != nil {
		return nil, "", err
	}
	return e.M, e.Link(r.cfg.Public), nil
}

// ResolveMCPQuiet is the MCP tool server's ResolveQuiet hook, used by
// wait_for_turn: it finds the caller's game manager and board link like
// ResolveMCP, but neither records activity nor takes a place. Waiting is
// not activity, so an abandoned game still expires.
func (r *Registry) ResolveMCPQuiet(_ context.Context, extra *mcp.RequestExtra) (*game.Manager, string, error) {
	if extra == nil {
		return nil, "", errSignIn
	}
	acct, ok := hostauth.AccountFromToken(extra.TokenInfo)
	if !ok {
		return nil, "", errSignIn
	}
	e := r.Get(acct)
	return e.M, e.Link(r.cfg.Public), nil
}

// AccountID is the MCP tool server's Account hook: the Google account ID
// (sub) of the account a request acts for, read from the request's token
// on every call. It touches no entry, so asking who is signed in takes no
// place and is not activity.
func (r *Registry) AccountID(_ context.Context, extra *mcp.RequestExtra) (string, error) {
	if extra == nil {
		return "", errSignIn
	}
	acct, ok := hostauth.AccountFromToken(extra.TokenInfo)
	if !ok || acct.ID == "" {
		return "", errSignIn
	}
	return acct.ID, nil
}

// Reports is the service keeping failed imports, or nil when none are
// kept (the MCP server's ReportImport hook is set only when it is not).
func (r *Registry) Reports() *importreports.Service { return r.cfg.Reports }

// ReportImport is the MCP tool server's ReportImport hook: it saves the
// battle log that failed to import, which the player agreed to send, under
// their player ID with the players' names replaced, and records
// import_reported with the report's ID, so the report can be found from
// the player's error records. It is not activity and takes no place: the
// failed import left no game.
func (r *Registry) ReportImport(ctx context.Context, extra *mcp.RequestExtra, in mcpserver.ImportReport) (string, error) {
	if extra == nil {
		return "", errSignIn
	}
	acct, ok := hostauth.AccountFromToken(extra.TokenInfo)
	if !ok {
		return "", errSignIn
	}
	if r.cfg.Reports == nil {
		return "", errors.New("import reports are not kept on this playmat")
	}
	rep, err := r.cfg.Reports.Save(ctx, importreports.Report{
		Player: r.cfg.Recorder.ID(acct.PlayerKey()), Assistant: in.Assistant,
		Error: in.Error, Log: in.Log, Deck: in.Deck, OppDeck: in.OppDeck,
	}, in.Me)
	if err != nil {
		return "", err
	}
	r.cfg.Recorder.RecordContext(ctx, telemetry.Event{Name: telemetry.ImportReported, Email: acct.PlayerKey(), Assistant: in.Assistant, Report: rep.ID})
	return rep.ID, nil
}
