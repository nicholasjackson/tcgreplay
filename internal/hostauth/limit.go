package hostauth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
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

// The failed password sign-in allowances: per username attempted and per
// client address, each refilled over loginWindow.
const (
	loginUserCap   = 10
	loginClientCap = 30
	loginWindow    = 15 * time.Minute
)

// bucket is an allowance under one key: it holds up to its capacity and
// refills completely over its window (the request limit: limit requests,
// refilled over a minute).
type bucket struct {
	tokens float64
	last   time.Time
	logged time.Time
	window time.Duration
}

// refill brings b up to date at now for capacity n over window, creating
// it full if it is new. The caller holds mu.
func (s *store) refill(now time.Time, key string, capacity int, window time.Duration) *bucket {
	n := float64(capacity)
	b := s.buckets[key]
	if b == nil {
		b = &bucket{tokens: n, last: now}
		s.buckets[key] = b
	}
	b.window = window
	b.tokens = math.Min(n, b.tokens+now.Sub(b.last).Seconds()*n/window.Seconds())
	b.last = now
	return b
}

// take spends one from the allowance under key, which holds capacity and
// refills over window. When it is empty it returns how long until the next
// one is allowed, and whether this refusal should be logged.
func (s *store) take(now time.Time, key string, capacity int, window time.Duration) (ok bool, wait time.Duration, logIt bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.refill(now, key, capacity, window)
	if b.tokens >= 1 {
		b.tokens--
		return true, 0, false
	}
	wait = time.Duration((1 - b.tokens) * window.Seconds() / float64(capacity) * float64(time.Second))
	if now.Sub(b.logged) >= limitLogEvery {
		b.logged = now
		logIt = true
	}
	return false, wait, logIt
}

// peek reports whether the allowance under key has one to spend, without
// spending it, and if not, how long until it will.
func (s *store) peek(now time.Time, key string, capacity int, window time.Duration) (ok bool, wait time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.refill(now, key, capacity, window)
	if b.tokens >= 1 {
		return true, 0
	}
	return false, time.Duration((1 - b.tokens) * window.Seconds() / float64(capacity) * float64(time.Second))
}

// loginKeys are the allowance keys of a password sign-in attempt: the
// username attempted (whether or not it exists) and the client address.
func loginKeys(user, client string) (userKey, clientKey string) {
	return "login-user:" + normalise(user), "login-client:" + client
}

// loginAllowed reports whether a password sign-in may be checked: neither
// the username's nor the client's failure allowance is used up. It spends
// nothing, so a throttled attempt is refused before any password check.
func (a *Auth) loginAllowed(now time.Time, user, client string) (bool, time.Duration) {
	uk, ck := loginKeys(user, client)
	okU, waitU := a.st.peek(now, uk, loginUserCap, loginWindow)
	okC, waitC := a.st.peek(now, ck, loginClientCap, loginWindow)
	return okU && okC, max(waitU, waitC)
}

// loginFailed spends one failure from the username's and the client's
// allowances.
func (a *Auth) loginFailed(now time.Time, user, client string) {
	uk, ck := loginKeys(user, client)
	a.st.take(now, uk, loginUserCap, loginWindow)
	a.st.take(now, ck, loginClientCap, loginWindow)
}

// clientAddr is the address a request came from, for the per-client
// failure allowance: the address App Platform's edge reports
// (do-connecting-ip, then CF-Connecting-IP), else the connection's peer.
func clientAddr(r *http.Request) string {
	addr, _ := clientAddrFrom(r)
	return addr
}

// clientAddrFrom is clientAddr and where it came from: the header's name,
// or "peer".
func clientAddrFrom(r *http.Request) (addr, source string) {
	for _, h := range []string{"do-connecting-ip", "CF-Connecting-IP"} {
		if ip := net.ParseIP(strings.TrimSpace(r.Header.Get(h))); ip != nil {
			return ip.String(), h
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr, "peer"
	}
	return host, "peer"
}

// clientAddr is clientAddr, logging once where the first password
// attempt's address came from (the source, never the address), so a
// deploy check can see whether the edge's headers arrive.
func (a *Auth) clientAddr(r *http.Request) string {
	addr, source := clientAddrFrom(r)
	a.clientSourceOnce.Do(func() {
		log.Printf("hostauth: password sign-in client addresses come from %s", source)
	})
	return addr
}

// allow counts one request for acct against the request limit; a refusal
// it records carries the trace of the request in ctx.
func (a *Auth) allow(ctx context.Context, acct Account) error {
	if a.cfg.RateLimit <= 0 {
		return nil
	}
	ok, wait, logIt := a.st.take(a.now(), "acct:"+acct.ID, a.cfg.RateLimit, time.Minute)
	if ok {
		return nil
	}
	err := fmt.Errorf("%w: this account is limited to %d requests a minute; try again in %ds",
		ErrRateLimited, a.cfg.RateLimit, int(math.Ceil(wait.Seconds())))
	if logIt {
		a.cfg.Recorder.RecordContext(ctx, telemetry.Event{Name: telemetry.Limited, Email: acct.PlayerKey(), Reason: "rate", Message: err.Error()})
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
			if err := a.allow(ctx, acct); err != nil {
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
	return a.allow(ctx, acct)
}
