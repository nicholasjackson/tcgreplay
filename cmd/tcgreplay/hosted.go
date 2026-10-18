package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/webui"
)

// mcpSessionTimeout closes MCP sessions that have been idle this long, so
// abandoned sessions do not pile up, while still covering a long review
// with idle gaps.
const mcpSessionTimeout = 4 * time.Hour

// hostedListen resolves the listen address: the flag if set, else the port
// the platform assigns in $PORT, else :8080.
func hostedListen(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return ":8080"
}

// hostedPublicURL parses and validates the public address the playmat is
// reached at. It must be an http or https URL with a host and no path,
// query or fragment; a trailing slash is dropped. With no address it
// falls back to http://localhost:<port of listen> and logs a warning.
func hostedPublicURL(raw, listen string) (*url.URL, error) {
	if raw == "" {
		port := "8080"
		if _, p, err := net.SplitHostPort(listen); err == nil && p != "" {
			port = p
		}
		u := &url.URL{Scheme: "http", Host: "localhost:" + port}
		log.Printf("warning: no public URL set (-public-url or TCGREPLAY_PUBLIC_URL); using %s", u)
		return u, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("public URL %q: %v", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("public URL %q: scheme must be http or https", raw)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("public URL %q: no host", raw)
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, fmt.Errorf("public URL %q: must be a scheme and host only, with no path, query or fragment", raw)
	}
	return &url.URL{Scheme: u.Scheme, Host: u.Host}, nil
}

// envInt reads a whole-number setting from the environment, falling back
// to def when it is unset or not a number (which is logged).
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		log.Printf("warning: %s=%q is not a whole number; using %d", key, v, def)
		return def
	}
	return n
}

// hostedAuth builds the sign-in service from the settings: the Google
// client ID (flag or TCGREPLAY_GOOGLE_CLIENT_ID), the secret (only from
// TCGREPLAY_GOOGLE_CLIENT_SECRET, so it never shows in a process listing),
// the limits and the public URL. Hosted mode must not run open, so missing
// credentials are an error. google is Google's endpoints; the zero value
// means the real Google.
func hostedAuth(c config, public *url.URL, google oauth2.Endpoint) (*hostauth.Auth, error) {
	secret := os.Getenv("TCGREPLAY_GOOGLE_CLIENT_SECRET")
	if c.googleClientID == "" || secret == "" {
		return nil, errors.New("hosted mode needs Google sign-in: set TCGREPLAY_GOOGLE_CLIENT_ID and TCGREPLAY_GOOGLE_CLIENT_SECRET")
	}
	return hostauth.New(hostauth.Config{
		Public:         public,
		GoogleClientID: c.googleClientID,
		GoogleSecret:   secret,
		RateLimit:      c.rateLimit,
		MaxUsers:       c.maxUsers,
		Google:         google,
	})
}

// hostedHandler serves MCP over Streamable HTTP at /mcp, every session
// sharing srv, and the playmat on every other path, with a's sign-in in
// front: its own routes (metadata, client registration, authorize, token,
// Google's callback, /healthz) are open, /mcp needs an access token and
// the board needs a session.
func hostedHandler(srv *mcp.Server, ui *webui.Server, a *hostauth.Auth) http.Handler {
	mux := http.NewServeMux()
	a.Routes(mux)
	mux.Handle("/mcp", a.RequireToken(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{
		SessionTimeout: mcpSessionTimeout,
		// Hosted mode is public; the request's Host is the proxy's business.
		DisableLocalhostProtection: true,
	})))
	mux.Handle("/", a.RequireSession(ui.Handler()))
	return mux
}

// newHosted builds the game manager, MCP server and board for hosted mode,
// both servers taking their address from the one public URL: board links
// point at its root and the board accepts live connections from it. Every
// MCP request and board action counts against its account's request
// limit.
func newHosted(c config, public *url.URL, a *hostauth.Auth) (*game.Manager, *mcp.Server, *webui.Server) {
	m, srv := newServer(c, public.String()+"/")
	srv.AddReceivingMiddleware(a.MCPMiddleware())
	ui := webui.New(m, state.Seat(c.seat).Other())
	ui.Public = public
	ui.Limit = a.BoardLimit
	return m, srv, ui
}

// runHosted serves MCP and the playmat over one plain-HTTP listener until
// ctx is done, for running in a container behind an HTTPS proxy.
func runHosted(ctx context.Context, c config) error {
	listen := hostedListen(c.listen)
	public, err := hostedPublicURL(c.publicURL, listen)
	if err != nil {
		return err
	}
	a, err := hostedAuth(c, public, oauth2.Endpoint{})
	if err != nil {
		return err
	}
	_, srv, ui := newHosted(c, public, a)
	ui.StartHub(ctx)

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	log.Printf("tcgreplay %s hosted: listening on %s, public %s, data %s, cache %s, claude plays %s, Google sign-in, %d requests a minute per account, %d users", version, ln.Addr(), public, c.dataDir, c.cacheDir, c.seat, c.rateLimit, c.maxUsers)
	// No WriteTimeout: MCP event streams and the board's websocket are long-lived.
	hs := &http.Server{Handler: hostedHandler(srv, ui, a), ReadHeaderTimeout: 10 * time.Second}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()
	if err := hs.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	// Let in-flight requests finish before returning.
	<-stopped
	log.Printf("tcgreplay hosted: stopped")
	return nil
}
