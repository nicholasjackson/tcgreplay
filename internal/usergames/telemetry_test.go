package usergames

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// callRequest is a tools/call request for tool with args, signed in as
// acct (no token when acct has no ID), on no session.
func callRequest(acct hostauth.Account, tool string, args string) *mcp.CallToolRequest {
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: tool, Arguments: json.RawMessage(args)}}
	if acct.ID != "" {
		req.Extra = tokenExtra(acct.ID, acct.Email)
	}
	return req
}

// textResult is a tool result holding text, an error result when isErr.
func textResult(text string, isErr bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: isErr}
}

// openNamedGame opens a game for acct in r with the test deck on both
// sides and the players named p1 and p2.
func openNamedGame(t *testing.T, r *Registry, acct hostauth.Account, p1, p2 string) {
	t.Helper()
	req := game.NewGameRequest{ID: "named", DeckP1: testDeck, DeckP2: testDeck, Names: map[state.Seat]string{state.P1: p1, state.P2: p2}}
	if _, _, err := use(t, r, acct).M.New(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}

// TestMCPMiddlewareRecords: every tool call writes tool_call with the tool
// and the caller's ID; import_log and new_game also write import or
// new_game when they succeed, and nothing more when they fail; a failure,
// as an error result or a Go error, writes error naming the tool with the
// message redacted against the call's arguments and the caller's game. The
// result passes through unchanged.
func TestMCPMiddlewareRecords(t *testing.T) {
	goErr := fmt.Errorf("card %q not found", "Ultra Ball")
	tests := []struct {
		name, tool, args string
		res              *mcp.CallToolResult
		err              error
		wantEvent        string // the record after tool_call ("" for none)
		wantMessage      string // the error record's message
	}{
		{name: "import ok", tool: "import_log", args: `{"log":"x"}`, res: textResult("imported", false), wantEvent: telemetry.Import},
		{name: "new game ok", tool: "new_game", args: `{"deck_p1":"x"}`, res: textResult("started", false), wantEvent: telemetry.NewGame},
		{name: "other tool ok", tool: "get_state", res: textResult("{}", false)},
		{
			name: "error result names game cards and players", tool: "attach", args: `{"iid":"p1-1"}`,
			res:       textResult("cannot attach Lucario to Ash Ketchum's Riolu: no room", true),
			wantEvent: telemetry.Error, wantMessage: "cannot attach … to …'s …: no room",
		},
		{
			name: "error result quotes an argument", tool: "annotate", args: `{"note":"Misty plays Starmie"}`,
			res:       textResult("annotate: note Misty plays Starmie is too long", true),
			wantEvent: telemetry.Error, wantMessage: "annotate: note … is too long",
		},
		{
			name: "failed import writes error not import", tool: "import_log", args: `{"log":"Gary Oak drew a card."}`,
			res:       textResult(`line 1: "Gary Oak drew a card." before Setup`, true),
			wantEvent: telemetry.Error, wantMessage: `line 1: "…" before Setup`,
		},
		{
			name: "go error", tool: "new_game", args: `{}`, err: goErr,
			wantEvent: telemetry.Error, wantMessage: `card "…" not found`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRegistry(t, t.TempDir(), newClock().Now)
			rec, buf := withRecorder(t, r)
			openNamedGame(t, r, alice, "Ash Ketchum", "Gary Oak")
			buf.buf.Reset() // drop the player_day from opening the game

			var gotReq mcp.Request
			next := func(_ context.Context, method string, req mcp.Request) (mcp.Result, error) {
				gotReq = req
				if tt.err != nil {
					return nil, tt.err
				}
				return tt.res, nil
			}
			req := callRequest(alice, tt.tool, tt.args)
			res, err := r.MCPMiddleware()(next)(context.Background(), "tools/call", req)

			if gotReq != req {
				t.Error("the tools did not receive the request")
			}
			if err != tt.err {
				t.Errorf("error %v, want %v unchanged", err, tt.err)
			}
			if tt.err == nil && res != mcp.Result(tt.res) {
				t.Errorf("result %v, want %v unchanged", res, tt.res)
			}

			recs := records(t, buf)
			want := 1
			if tt.wantEvent != "" {
				want = 2
			}
			if len(recs) != want {
				t.Fatalf("%d records, want %d: %v", len(recs), want, recs)
			}
			id := rec.ID(alice.Email)
			if c := recs[0]; c["event"] != telemetry.ToolCall || c["tool"] != tt.tool || c["player"] != id || c["level"] != "info" || c["time"] == "" {
				t.Errorf("first record %v, want tool_call for %s by %s", c, tt.tool, id)
			}
			if _, ok := recs[0]["assistant"]; ok {
				t.Errorf("tool_call on no session names assistant %q", recs[0]["assistant"])
			}
			if tt.wantEvent == "" {
				return
			}
			e := recs[1]
			if e["event"] != tt.wantEvent || e["player"] != id || e["time"] == "" {
				t.Errorf("second record %v, want %s by %s", e, tt.wantEvent, id)
			}
			if tt.wantEvent == telemetry.Error {
				if e["tool"] != tt.tool || e["level"] != "error" || e["message"] != tt.wantMessage {
					t.Errorf("error record %v, want tool %s, level error, message %q", e, tt.tool, tt.wantMessage)
				}
			}
			noEmail(t, buf)
		})
	}
}

// TestMCPMiddlewareRedactsGameText: a failing call's message is redacted
// against the caller's own game (its card and player names), and no name
// from it survives in the records.
func TestMCPMiddlewareRedactsGameText(t *testing.T) {
	r := newRegistry(t, t.TempDir(), newClock().Now)
	_, buf := withRecorder(t, r)
	openNamedGame(t, r, alice, "Ash Ketchum", "Gary Oak")
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return textResult("Gary Oak cannot play Ultra Ball onto Lucario or Riolu", true), nil
	}
	if _, err := r.MCPMiddleware()(next)(context.Background(), "tools/call", callRequest(alice, "play_trainer", `{"iid":"p2-9"}`)); err != nil {
		t.Fatal(err)
	}
	errs := only(records(t, buf), telemetry.Error)
	if len(errs) != 1 {
		t.Fatalf("error records %v, want 1", errs)
	}
	for _, name := range []string{"Gary", "Oak", "Ultra Ball", "Lucario", "Riolu"} {
		if strings.Contains(buf.String(), name) {
			t.Errorf("records contain %q:\n%s", name, buf)
		}
	}
	if want := "… cannot play … onto … or …"; errs[0]["message"] != want {
		t.Errorf("message %q, want %q", errs[0]["message"], want)
	}
}

// TestMCPMiddlewareOtherMethods: a method other than tools/call, and any
// call when the registry has no recorder, passes through and writes
// nothing.
func TestMCPMiddlewareOtherMethods(t *testing.T) {
	r := newRegistry(t, t.TempDir(), newClock().Now)
	_, buf := withRecorder(t, r)
	want := &mcp.ListToolsResult{}
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) { return want, nil }
	req := &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}, Extra: tokenExtra(alice.ID, alice.Email)}
	res, err := r.MCPMiddleware()(next)(context.Background(), "tools/list", req)
	if err != nil || res != mcp.Result(want) {
		t.Errorf("tools/list gave %v, %v; want the result unchanged", res, err)
	}
	if out := buf.String(); out != "" {
		t.Errorf("tools/list wrote records:\n%s", out)
	}

	off := newRegistry(t, t.TempDir(), nil)
	callNext := func(context.Context, string, mcp.Request) (mcp.Result, error) { return textResult("x", true), nil }
	if _, err := off.MCPMiddleware()(callNext)(context.Background(), "tools/call", callRequest(alice, "ping", `{}`)); err != nil {
		t.Errorf("tools/call without a recorder: %v", err)
	}
}

// TestMCPMiddlewareNamesAssistant: over a real session, the tool_call and
// new_game records name the assistant by the playmat's name for the
// session's client ("claude-ai" is Claude).
func TestMCPMiddlewareNamesAssistant(t *testing.T) {
	r := newRegistry(t, t.TempDir(), newClock().Now)
	_, buf := withRecorder(t, r)
	srv := mcp.NewServer(&mcp.Implementation{Name: "playmat"}, nil)
	type noArgs struct{}
	mcp.AddTool(srv, &mcp.Tool{Name: "new_game"}, func(context.Context, *mcp.CallToolRequest, noArgs) (*mcp.CallToolResult, any, error) {
		return textResult("started", false), nil, nil
	})
	srv.AddReceivingMiddleware(r.MCPMiddleware())
	st, ct := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "claude-ai"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	if res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "new_game", Arguments: map[string]any{}}); err != nil || res.IsError {
		t.Fatalf("new_game: %v %+v", err, res)
	}
	recs := records(t, buf)
	calls, games := only(recs, telemetry.ToolCall), only(recs, telemetry.NewGame)
	if len(calls) != 1 || calls[0]["tool"] != "new_game" || calls[0]["assistant"] != "Claude" {
		t.Errorf("tool_call records %v, want one for new_game by Claude", calls)
	}
	if len(games) != 1 || games[0]["assistant"] != "Claude" {
		t.Errorf("new_game records %v, want one by Claude", games)
	}
}

// TestActiveFromUseOnly: Use writes player_day the first time a player
// acts in a UTC day, and again the next day; Get, the quiet MCP lookup and
// a busy refusal never do.
func TestActiveFromUseOnly(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}
	r := placesRegistry(t, 1, 0, clock.Now)
	rec, buf := withRecorder(t, r)
	days := func() []record { return only(records(t, buf), telemetry.PlayerDay) }

	r.Get(alice)
	if _, _, err := r.ResolveMCPQuiet(context.Background(), tokenExtra(alice.ID, alice.Email)); err != nil {
		t.Fatal(err)
	}
	if d := days(); len(d) != 0 {
		t.Fatalf("Get and the quiet lookup wrote player_day: %v", d)
	}

	use(t, r, alice)
	clock.Advance(time.Hour)
	use(t, r, alice)
	if _, _, err := r.ResolveMCP(context.Background(), tokenExtra(alice.ID, alice.Email)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Use(bob); !errors.Is(err, hostauth.ErrBusy) {
		t.Fatalf("Use(bob) at a cap of 1: %v, want busy", err)
	}
	d := days()
	if len(d) != 1 || d[0]["player"] != rec.ID(alice.Email) || d[0]["time"] != "2026-11-12T09:00:00.000Z" {
		t.Fatalf("player_day records %v, want one for alice at 09:00 and none for bob", d)
	}

	clock.Advance(15 * time.Hour) // 2026-11-13 01:00
	use(t, r, alice)
	if d := days(); len(d) != 2 || d[1]["player"] != rec.ID(alice.Email) || d[1]["time"] != "2026-11-13T01:00:00.000Z" {
		t.Errorf("player_day records %v, want a second for alice on the next day", d)
	}
	noEmail(t, buf)
}

// TestBoardErrorRecords: a failed board action writes error naming the
// action, by the signed-in player's ID, redacted against their game; a
// limit refusal, a busy refusal and no error write nothing.
func TestBoardErrorRecords(t *testing.T) {
	r := newRegistry(t, t.TempDir(), newClock().Now)
	rec, buf := withRecorder(t, r)
	openNamedGame(t, r, alice, "Ash Ketchum", "Gary Oak")
	buf.buf.Reset()
	ctx := hostauth.WithAccount(context.Background(), alice)

	r.BoardError(ctx, "move", fmt.Errorf("limited: %w", hostauth.ErrRateLimited))
	r.BoardError(ctx, "move", hostauth.BusyError(1))
	r.BoardError(ctx, "move", nil)
	if out := buf.String(); out != "" {
		t.Fatalf("refusals wrote records:\n%s", out)
	}

	r.BoardError(ctx, "evolve", errors.New("Lucario cannot evolve from Ash Ketchum's Riolu this turn"))
	recs := records(t, buf)
	if len(recs) != 1 {
		t.Fatalf("records %v, want one error", recs)
	}
	want := record{"event": telemetry.Error, "level": "error", "player": rec.ID(alice.Email), "action": "evolve", "message": "… cannot evolve from …'s … this turn"}
	for k, v := range want {
		if recs[0][k] != v {
			t.Errorf("error record %s = %q, want %q (record %v)", k, recs[0][k], v, recs[0])
		}
	}
	if _, ok := recs[0]["tool"]; ok {
		t.Errorf("board error names a tool: %v", recs[0])
	}

	off := newRegistry(t, t.TempDir(), nil)
	off.BoardError(ctx, "move", errors.New("x")) // no recorder: nothing to do, no panic
}
