package hostauth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

// limitAccount and otherAccount are the accounts the limit tests count for.
var (
	limitAccount = Account{ID: "sub-1", Email: "one@example.com"}
	otherAccount = Account{ID: "sub-2", Email: "two@example.com"}
)

// newLimited returns a sign-in service limited to limit requests a minute
// per account, on an injected clock.
func newLimited(t *testing.T, limit int) (*Auth, *fakeClock) {
	t.Helper()
	clock := &fakeClock{t: time.Now()}
	a, err := New(Config{
		Public:         &url.URL{Scheme: "https", Host: "playmat.example"},
		GoogleClientID: "client-id",
		GoogleSecret:   "client-secret",
		RateLimit:      limit,
		Now:            clock.Now,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, clock
}

// spend makes n requests for acct, failing the test if any is refused.
func spend(t *testing.T, a *Auth, acct Account, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := a.allow(acct); err != nil {
			t.Fatalf("request %d of %d for %s refused: %v", i+1, n, acct.Email, err)
		}
	}
}

// checkLimited fails unless err is the over-the-limit error naming the
// limit and telling the user to wait want.
func checkLimited(t *testing.T, err error, limit int, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("request over the limit allowed")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("error %v does not wrap ErrRateLimited", err)
	}
	msg := err.Error()
	for _, s := range []string{"too many requests", "limited to " + strconv.Itoa(limit) + " requests a minute", "try again in " + want} {
		if !strings.Contains(msg, s) {
			t.Errorf("message %q does not contain %q", msg, s)
		}
	}
}

// TestLimitRefusesAndRecovers shows an account gets its limit's worth of
// requests, is refused the next with a message naming the limit and a
// sensible wait, gets one more request back after 60/limit seconds, and
// its whole allowance back after a minute.
func TestLimitRefusesAndRecovers(t *testing.T) {
	a, clock := newLimited(t, 5)
	spend(t, a, limitAccount, 5)
	checkLimited(t, a.allow(limitAccount), 5, "12s")

	// Half way to the next request, the wait is half as long.
	clock.Advance(6 * time.Second)
	checkLimited(t, a.allow(limitAccount), 5, "6s")

	// 60/5 = 12 seconds after running out, one request is back.
	clock.Advance(6 * time.Second)
	spend(t, a, limitAccount, 1)
	checkLimited(t, a.allow(limitAccount), 5, "12s")

	// A minute on, the whole allowance is back, and no more.
	clock.Advance(time.Minute)
	spend(t, a, limitAccount, 5)
	checkLimited(t, a.allow(limitAccount), 5, "12s")
}

// TestLimitWaitForHighLimit shows the wait in the message for a high limit
// rounds up to whole seconds rather than reading "0s".
func TestLimitWaitForHighLimit(t *testing.T) {
	a, clock := newLimited(t, 300)
	spend(t, a, limitAccount, 300)
	checkLimited(t, a.allow(limitAccount), 300, "1s")
	clock.Advance(200 * time.Millisecond)
	spend(t, a, limitAccount, 1)
}

// TestLimitAccountsIndependent shows one account over its limit does not
// stop another, which gets its own full allowance.
func TestLimitAccountsIndependent(t *testing.T) {
	a, _ := newLimited(t, 3)
	spend(t, a, limitAccount, 3)
	checkLimited(t, a.allow(limitAccount), 3, "20s")
	spend(t, a, otherAccount, 3)
	checkLimited(t, a.allow(otherAccount), 3, "20s")
}

// TestLimitZeroIsOff shows a limit of 0 never refuses, through allow, the
// board hook or the MCP middleware.
func TestLimitZeroIsOff(t *testing.T) {
	a, _ := newLimited(t, 0)
	ctx := withAccount(context.Background(), limitAccount)
	mw := a.MCPMiddleware()(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		return &mcp.CallToolResult{}, nil
	})
	req := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "ping"}, Extra: &mcp.RequestExtra{TokenInfo: tokenFor(limitAccount)}}
	for i := 0; i < 10000; i++ {
		if err := a.allow(limitAccount); err != nil {
			t.Fatalf("request %d refused with no limit: %v", i+1, err)
		}
		if err := a.BoardLimit(ctx); err != nil {
			t.Fatalf("board action %d refused with no limit: %v", i+1, err)
		}
		res, err := mw(context.Background(), "tools/call", req)
		if err != nil || res.(*mcp.CallToolResult).IsError {
			t.Fatalf("tool call %d refused with no limit: %v %v", i+1, res, err)
		}
	}
}

// tokenFor is the token information the MCP library hands a request
// signed in as acct, as RequireToken builds it.
func tokenFor(acct Account) *auth.TokenInfo {
	return &auth.TokenInfo{Scopes: []string{scope}, Expiration: time.Now().Add(time.Hour), UserID: acct.ID, Extra: map[string]any{"email": acct.Email}}
}

// nextHandler is a stand-in MCP method handler that counts the requests
// that reach it.
type nextHandler struct{ calls int }

func (n *nextHandler) handle(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
	n.calls++
	if method == "tools/call" {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ran"}}}, nil
	}
	return &mcp.ListToolsResult{}, nil
}

// TestMCPMiddleware shows the middleware counts tool calls and other
// requests for the token's account: over the limit a tool call gets a tool
// error naming the limit and any other request fails with it, neither
// reaching the server. Another account still gets through.
func TestMCPMiddleware(t *testing.T) {
	a, _ := newLimited(t, 2)
	next := &nextHandler{}
	h := a.MCPMiddleware()(next.handle)
	call := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "ping"}, Extra: &mcp.RequestExtra{TokenInfo: tokenFor(limitAccount)}}
	list := &mcp.ListToolsRequest{Params: &mcp.ListToolsParams{}, Extra: &mcp.RequestExtra{TokenInfo: tokenFor(limitAccount)}}

	if _, err := h(context.Background(), "tools/list", list); err != nil {
		t.Fatalf("first request: %v", err)
	}
	res, err := h(context.Background(), "tools/call", call)
	if err != nil || res.(*mcp.CallToolResult).IsError {
		t.Fatalf("second request: %v %v", res, err)
	}
	if next.calls != 2 {
		t.Fatalf("%d requests reached the server, want 2", next.calls)
	}

	res, err = h(context.Background(), "tools/call", call)
	if err != nil {
		t.Fatalf("tool call over the limit: error %v, want a tool error result", err)
	}
	tr, ok := res.(*mcp.CallToolResult)
	if !ok || !tr.IsError || len(tr.Content) != 1 {
		t.Fatalf("tool call over the limit: %#v, want one tool error", res)
	}
	text, _ := tr.Content[0].(*mcp.TextContent)
	if text == nil || !strings.Contains(text.Text, "too many requests") || !strings.Contains(text.Text, "2 requests a minute") {
		t.Errorf("tool error %#v does not name the limit", tr.Content[0])
	}

	if res, err := h(context.Background(), "tools/list", list); !errors.Is(err, ErrRateLimited) || !strings.Contains(err.Error(), "2 requests a minute") {
		t.Errorf("list over the limit: %v %v, want the limit error", res, err)
	}
	if next.calls != 2 {
		t.Errorf("%d requests reached the server, want the refused ones stopped", next.calls)
	}

	other := &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: "ping"}, Extra: &mcp.RequestExtra{TokenInfo: tokenFor(otherAccount)}}
	if res, err := h(context.Background(), "tools/call", other); err != nil || res.(*mcp.CallToolResult).IsError {
		t.Errorf("other account's tool call: %v %v, want it run", res, err)
	}
}

// TestMCPMiddlewareSkipsNotifications shows notifications are not counted:
// they pass while the account is over its limit, and many of them leave
// the allowance untouched.
func TestMCPMiddlewareSkipsNotifications(t *testing.T) {
	a, _ := newLimited(t, 2)
	next := &nextHandler{}
	h := a.MCPMiddleware()(next.handle)
	note := &mcp.InitializedRequest{Params: &mcp.InitializedParams{}, Extra: &mcp.RequestExtra{TokenInfo: tokenFor(limitAccount)}}
	for i := 0; i < 50; i++ {
		if _, err := h(context.Background(), "notifications/initialized", note); err != nil {
			t.Fatalf("notification %d: %v", i+1, err)
		}
	}
	spend(t, a, limitAccount, 2)
	checkLimited(t, a.allow(limitAccount), 2, "30s")
	if _, err := h(context.Background(), "notifications/initialized", note); err != nil {
		t.Errorf("notification over the limit: %v, want it passed", err)
	}
	if next.calls != 51 {
		t.Errorf("%d notifications reached the server, want 51", next.calls)
	}
}

// TestMCPMiddlewareWithoutToken shows a request with no account (no extra,
// or no token) is passed through uncounted.
func TestMCPMiddlewareWithoutToken(t *testing.T) {
	a, _ := newLimited(t, 1)
	next := &nextHandler{}
	h := a.MCPMiddleware()(next.handle)
	for _, req := range []*mcp.CallToolRequest{
		{Params: &mcp.CallToolParamsRaw{Name: "ping"}},
		{Params: &mcp.CallToolParamsRaw{Name: "ping"}, Extra: &mcp.RequestExtra{}},
	} {
		for i := 0; i < 5; i++ {
			if res, err := h(context.Background(), "tools/call", req); err != nil || res.(*mcp.CallToolResult).IsError {
				t.Fatalf("request without a token: %v %v, want it run", res, err)
			}
		}
	}
	if next.calls != 10 {
		t.Errorf("%d requests reached the server, want 10", next.calls)
	}
}

// TestBoardLimit shows the board hook counts against the request's
// account and lets a request with no account through.
func TestBoardLimit(t *testing.T) {
	a, _ := newLimited(t, 1)
	for i := 0; i < 5; i++ {
		if err := a.BoardLimit(context.Background()); err != nil {
			t.Fatalf("board action with no account: %v, want nil", err)
		}
	}
	ctx := withAccount(context.Background(), limitAccount)
	if err := a.BoardLimit(ctx); err != nil {
		t.Fatalf("first board action: %v", err)
	}
	checkLimited(t, a.BoardLimit(ctx), 1, "60s")
	if err := a.BoardLimit(withAccount(context.Background(), otherAccount)); err != nil {
		t.Errorf("other account's board action: %v", err)
	}
}

// TestRequireSessionLimitsData shows a signed-in data request over the
// account's limit gets 429 with the message naming the limit.
func TestRequireSessionLimitsData(t *testing.T) {
	b := newBoard(t)
	b.cfg.RateLimit = 2
	b.restart(t)
	_, cookie := b.signInBoard(t, "/")
	for i := 0; i < 2; i++ {
		if resp, body := b.send(t, "GET", "/api/card?def=x", cookie, nil); resp.StatusCode != http.StatusOK || body != "data" {
			t.Fatalf("data request %d: %d %q, want 200 data", i+1, resp.StatusCode, body)
		}
	}
	resp, body := b.send(t, "GET", "/api/card?def=x", cookie, nil)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("data request over the limit: %d %q, want 429", resp.StatusCode, body)
	}
	if !strings.Contains(body, "too many requests") || !strings.Contains(body, "2 requests a minute") {
		t.Errorf("429 body %q does not name the limit", body)
	}

	// Another account's data requests still get through.
	two := hostauthtest.Account{ID: "sub-2", Email: "two@example.com"}
	b.g.SetAccount(two)
	br := b.browser(t)
	if resp, body := get(t, br, b.ts.URL+"/"); body != boardText(two) {
		t.Fatalf("second account's sign-in: %d %q", resp.StatusCode, body)
	}
	other := b.sessionCookieIn(br)
	if resp, body := b.send(t, "GET", "/api/card?def=x", other, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("other account's data request: %d %q, want 200", resp.StatusCode, body)
	}
}
