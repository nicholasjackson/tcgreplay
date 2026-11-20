package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/mcpserver"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// movedHosts is the set of old host names the hosted playmat has moved
// from (-moved-from), lower-cased, with no port.
type movedHosts map[string]bool

// parseMovedHosts parses -moved-from: a comma-separated list of bare host
// names, blanks skipped, case ignored. An entry with a scheme, path, port,
// user or space is an error, as is the public address's own host (its
// requests would redirect to themselves).
func parseMovedHosts(raw string, public *url.URL) (movedHosts, error) {
	hosts := movedHosts{}
	for _, entry := range strings.Split(raw, ",") {
		h := strings.ToLower(strings.TrimSpace(entry))
		if h == "" {
			continue
		}
		if strings.ContainsAny(h, "/:@ \t") {
			return nil, fmt.Errorf("moved-from host %q: must be a host name only, with no scheme, path or port", strings.TrimSpace(entry))
		}
		if h == strings.ToLower(public.Hostname()) {
			return nil, fmt.Errorf("moved-from host %q: is the public address's host (%s)", strings.TrimSpace(entry), public)
		}
		hosts[h] = true
	}
	return hosts, nil
}

// movedText describes the moved hosts for the start-up log.
func movedText(hosts movedHosts) string {
	if len(hosts) == 0 {
		return "no moved hosts"
	}
	names := make([]string, 0, len(hosts))
	for h := range hosts {
		names = append(names, h)
	}
	sort.Strings(names)
	return "moved from " + strings.Join(names, ", ")
}

// movedMCP is the MCP endpoint served at /mcp on a moved host: the
// playmat's own MCP server definition with no sign-in, no game and the
// moved answer in front (mcpserver.Moved), so it lists the real tools and
// guides and every call says the playmat has moved to public.
func movedMCP(c config, public *url.URL) http.Handler {
	srv := (&mcpserver.Server{ClaudeSeat: state.Seat(c.seat), Version: version, BoardURL: public.String() + "/board"}).MCP()
	srv.AddReceivingMiddleware(mcpserver.Moved(public.String() + "/mcp"))
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{
		SessionTimeout: mcpSessionTimeout,
		// Hosted mode is public; the request's Host is the proxy's business.
		DisableLocalhostProtection: true,
	})
}

// movedGuard sits in front of the hosted router. A request for a moved
// host never reaches the router: /mcp goes to moved (no sign-in, so a
// stale token is ignored and no sign-in starts there), and every other
// path redirects permanently (301 for GET and HEAD, 308 otherwise) to the
// same path and query on the public address, so the old host serves no
// page, board or sign-in. The request's host only picks between these and
// next; every address sent comes from public. With no moved hosts it is
// next itself.
func movedGuard(hosts movedHosts, public *url.URL, moved, next http.Handler) http.Handler {
	if len(hosts) == 0 {
		return next
	}
	base := public.String()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hosts[requestHost(r)] {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/mcp" {
			moved.ServeHTTP(w, r)
			return
		}
		target := base + r.URL.EscapedPath()
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		code := http.StatusPermanentRedirect
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			code = http.StatusMovedPermanently
		}
		http.Redirect(w, r, target, code)
	})
}

// requestHost is r's host name, lower-cased, without its port.
func requestHost(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}
