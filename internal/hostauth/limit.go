package hostauth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrRateLimited is the error an account over its request limit gets; its
// message names the limit.
var ErrRateLimited = errors.New("too many requests")

// ErrBusy is the refusal of a new account's sign-in once the cap on
// signed-in accounts is reached.
var ErrBusy = errors.New("the server is busy")

// busyError is the refusal at a cap of max places; it matches ErrBusy.
type busyError struct{ max int }

func (e busyError) Error() string {
	return fmt.Sprintf("%v: it already has the most signed-in users it serves at once (%d); try again later", ErrBusy, e.max)
}

func (e busyError) Is(target error) bool { return target == ErrBusy }

// BusyError returns the refusal for a server at its cap of max places: it
// wraps ErrBusy and its message names the cap, so the wording lives in one
// place.
func BusyError(max int) error { return busyError{max} }

// limitLogEvery is how often one account's refusals are logged.
const limitLogEvery = 10 * time.Second

// bucket is one account's request allowance: it holds up to the per-minute
// limit and refills at that rate.
type bucket struct {
	tokens float64
	last   time.Time
	logged time.Time
}

// take spends one request from the account's bucket. When it is empty it
// returns how long until the next request is allowed, and whether this
// refusal should be logged.
func (s *store) take(now time.Time, id string, limit int) (ok bool, wait time.Duration, logIt bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := float64(limit)
	b := s.buckets[id]
	if b == nil {
		b = &bucket{tokens: n, last: now}
		s.buckets[id] = b
	}
	b.tokens = math.Min(n, b.tokens+now.Sub(b.last).Seconds()*n/60)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0, false
	}
	wait = time.Duration((1 - b.tokens) * 60 / n * float64(time.Second))
	if now.Sub(b.logged) >= limitLogEvery {
		b.logged = now
		logIt = true
	}
	return false, wait, logIt
}

// allow counts one request for acct against the request limit.
func (a *Auth) allow(acct Account) error {
	if a.cfg.RateLimit <= 0 {
		return nil
	}
	ok, wait, logIt := a.st.take(a.now(), acct.ID, a.cfg.RateLimit)
	if ok {
		return nil
	}
	err := fmt.Errorf("%w: this account is limited to %d requests a minute; try again in %ds",
		ErrRateLimited, a.cfg.RateLimit, int(math.Ceil(wait.Seconds())))
	if logIt {
		log.Printf("hostauth: rate limited %s: %v", acct.Email, err)
	}
	return err
}

// MCPMiddleware counts every MCP request against its account's limit. Over
// the limit, a tool call returns a tool error naming the limit, so Claude
// can tell the user; any other request fails with the same message.
// Notifications are not counted.
func (a *Auth) MCPMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if strings.HasPrefix(method, "notifications/") {
				return next(ctx, method, req)
			}
			var acct Account
			ok := false
			if extra := req.GetExtra(); extra != nil {
				acct, ok = AccountFromToken(extra.TokenInfo)
			}
			if !ok {
				// The HTTP layer has already refused anonymous requests.
				return next(ctx, method, req)
			}
			if err := a.allow(acct); err != nil {
				if method == "tools/call" {
					return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, nil
				}
				return nil, err
			}
			return next(ctx, method, req)
		}
	}
}

// BoardLimit counts a board action against its account's limit; it is the
// board server's Limit hook. A request with no account is not counted.
func (a *Auth) BoardLimit(ctx context.Context) error {
	acct, ok := AccountFrom(ctx)
	if !ok {
		return nil
	}
	return a.allow(acct)
}
