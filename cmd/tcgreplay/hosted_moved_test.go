package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/mcpserver"
)

// oldHost is the host name the moved-host tests configure as moved.
const oldHost = "old.example"

// withHost sends every request with its Host header set to host, as a
// proxy forwarding a request for that name would.
type withHost struct {
	host  string
	token string
}

func (h withHost) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Host = h.host
	if h.token != "" {
		r.Header.Set("Authorization", "Bearer "+h.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// startMoved serves hosted mode at testPublic with oldHost moved.
func startMoved(t *testing.T) *hostedServer {
	t.Helper()
	return startHostedWith(t, testPublic, func(c *config) { c.movedFrom = oldHost })
}

// connectAs opens an MCP session at h's /mcp with the given Host header
// and bearer token (none if empty), with no sign-in.
func connectAs(t *testing.T, h *hostedServer, host, token string) (*mcp.ClientSession, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tr := &mcp.StreamableClientTransport{Endpoint: h.ts.URL + "/mcp", HTTPClient: &http.Client{Transport: withHost{host: host, token: token}}}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "old-connector"}, nil).Connect(ctx, tr, nil)
	if err == nil {
		t.Cleanup(func() { cs.Close() })
	}
	return cs, err
}

// TestMovedHostMCP: an assistant at /mcp on a moved host connects with
// no sign-in, even with a stale token, and every tool call answers with
// the moved message naming the public address's /mcp. Host matching
// ignores case and port.
func TestMovedHostMCP(t *testing.T) {
	h := startMoved(t)
	want := mcpserver.MovedText(testPublic + "/mcp")
	for _, tc := range []struct{ name, host, token string }{
		{"no token", oldHost, ""},
		{"stale token", oldHost, "junk"},
		{"case and port", "OLD.Example:443", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs, err := connectAs(t, h, tc.host, tc.token)
			if err != nil {
				t.Fatalf("connect at the moved host: %v", err)
			}
			if got := cs.InitializeResult().Instructions; got != want {
				t.Errorf("instructions %q, want the moved message", got)
			}
			tools, err := cs.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(tools.Tools) == 0 {
				t.Fatal("no tools listed at the moved host")
			}
			for _, name := range []string{"get_state", "new_game", "guide"} {
				res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				text := ""
				for _, c := range res.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						text += tc.Text
					}
				}
				if !res.IsError || text != want || !strings.Contains(text, "https://playmat.example/mcp") {
					t.Errorf("%s at the moved host: IsError %v, %q; want the moved error naming %s/mcp", name, res.IsError, text, testPublic)
				}
			}
		})
	}

	// A raw initialize with a stale token is answered, not refused.
	resp, body := sendMCP(t, h.ts.URL+"/mcp", oldHost, "junk")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("WWW-Authenticate") != "" {
		t.Fatalf("initialize at the moved host with a stale token: %d %q (WWW-Authenticate %q), want 200 and no challenge", resp.StatusCode, body, resp.Header.Get("WWW-Authenticate"))
	}
	if !strings.Contains(body, "https://playmat.example/mcp") {
		t.Errorf("initialize at the moved host: %q does not name the new address", body)
	}
}

// sendMCP posts an initialize to endpoint with the given Host and bearer
// token, without following redirects.
func sendMCP(t *testing.T, endpoint, host, token string) (*http.Response, string) {
	t.Helper()
	resp, body := sendWithBody(t, http.MethodPost, endpoint, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}`, func(r *http.Request) {
		r.Host = host
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
	})
	return resp, body
}

// sendWithBody is sendWith with a request body.
func sendWithBody(t *testing.T, method, u, body string, mod func(*http.Request)) (*http.Response, string) {
	t.Helper()
	return sendWith(t, method, u, nil, func(r *http.Request) {
		r.Body = http.NoBody
		if body != "" {
			r.Body = io.NopCloser(strings.NewReader(body))
			r.ContentLength = int64(len(body))
		}
		mod(r)
	})
}

// TestMovedHostRedirects: on a moved host every path but /mcp, sign-in
// and its metadata included, redirects permanently to the same path and
// query on the public address: 301 for GET and HEAD, 308 otherwise.
func TestMovedHostRedirects(t *testing.T) {
	h := startMoved(t)
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{http.MethodGet, "/", http.StatusMovedPermanently},
		{http.MethodGet, "/docs/connect/", http.StatusMovedPermanently},
		{http.MethodGet, "/board?x=1", http.StatusMovedPermanently},
		{http.MethodHead, "/docs/", http.StatusMovedPermanently},
		{http.MethodGet, "/authorize?response_type=code&client_id=c&redirect_uri=https%3A%2F%2Fclaude.ai%2Fcb", http.StatusMovedPermanently},
		{http.MethodGet, "/auth/login", http.StatusMovedPermanently},
		{http.MethodGet, "/auth/google/callback?code=x&state=y", http.StatusMovedPermanently},
		{http.MethodGet, "/.well-known/oauth-protected-resource/mcp", http.StatusMovedPermanently},
		{http.MethodGet, "/.well-known/oauth-authorization-server", http.StatusMovedPermanently},
		{http.MethodGet, "/healthz", http.StatusMovedPermanently},
		{http.MethodPost, "/token", http.StatusPermanentRedirect},
		{http.MethodPost, "/register", http.StatusPermanentRedirect},
	} {
		for _, host := range []string{oldHost, "Old.EXAMPLE:8443"} {
			resp, _ := sendWith(t, tc.method, h.ts.URL+tc.path, nil, func(r *http.Request) { r.Host = host })
			want := testPublic + tc.path
			if resp.StatusCode != tc.code || resp.Header.Get("Location") != want {
				t.Errorf("%s %s on %s: %d to %q, want %d to %q", tc.method, tc.path, host, resp.StatusCode, resp.Header.Get("Location"), tc.code, want)
			}
			if c := resp.Header.Values("Set-Cookie"); len(c) != 0 {
				t.Errorf("%s %s on %s set cookies %q", tc.method, tc.path, host, c)
			}
		}
	}
}

// TestUnlistedHostsUnchanged: with a moved host configured, the public
// host and any other host are served by the hosted router as before:
// /mcp without a token is refused with the public address's metadata,
// and pages are served, not redirected.
func TestUnlistedHostsUnchanged(t *testing.T) {
	h := startMoved(t)
	for _, host := range []string{"", "playmat.example", "other.example"} {
		mod := func(r *http.Request) {
			if host != "" {
				r.Host = host
			}
		}
		resp, body := sendMCP(t, h.ts.URL+"/mcp", hostOr(host, h), "")
		checkRefused(t, resp, body, testPublic)
		wantCode, wantBody := siteResponse(t, "/docs/connect")
		if resp, body := getWith(t, h.ts.URL+"/docs/connect", nil, mod); resp.StatusCode != wantCode || body != wantBody || resp.Header.Get("Location") != "" {
			t.Errorf("GET /docs/connect on %q: %d (Location %q), want the website's page", host, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
}

// hostOr is host, or h's own listener address when host is empty.
func hostOr(host string, h *hostedServer) string {
	if host != "" {
		return host
	}
	u, _ := url.Parse(h.ts.URL)
	return u.Host
}

// TestRunHostedMovedFrom: runHosted wires the guard in front of the
// router and names the moved hosts in its start-up log.
func TestRunHostedMovedFrom(t *testing.T) {
	logs := captureLog(t)
	base := "http://127.0.0.1:" + freePort(t)
	c := hostedConfig(t, strings.TrimPrefix(base, "http://"), "https://play.example.com")
	c.movedFrom = "Old.example, older.example,"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runHosted(ctx, c) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	waitHealthy(t, base, 5*time.Second, done, logs)

	resp, _ := sendWith(t, http.MethodGet, base+"/docs/?a=b", nil, func(r *http.Request) { r.Host = "older.example" })
	if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "https://play.example.com/docs/?a=b" {
		t.Errorf("GET /docs/?a=b on a moved host: %d to %q, want 301 to the public address", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, body := sendMCP(t, base+"/mcp", "old.example", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "https://play.example.com/mcp") {
		t.Errorf("initialize on a moved host: %d %q, want the moved answer", resp.StatusCode, body)
	}
	if out := logs.String(); !strings.Contains(out, "moved from old.example, older.example") {
		t.Errorf("start-up log %q does not list the moved hosts", out)
	}
}
