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
	"github.com/nicholasjackson/tcgreplay/internal/mcpserver"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/usergames"
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

// envDuration reads a duration setting (Go syntax, e.g. 2m or 1h; "0"
// for none) from the environment, falling back to def when it is unset.
// An invalid or negative value is an error.
func envDuration(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return def, fmt.Errorf("%s: invalid duration %q (use e.g. 2m, 60m or 1h; 0 to never drop)", key, v)
	}
	return d, nil
}

// hostedAuth builds the sign-in service from the settings: the Google
// client ID (flag or TCGREPLAY_GOOGLE_CLIENT_ID), the secret (only from
// TCGREPLAY_GOOGLE_CLIENT_SECRET, so it never shows in a process listing),
// the signing key that keeps sign-ins across restarts (only from
// TCGREPLAY_SIGNING_KEY, for the same reason), the request limit and the
// public URL. places keeps the user cap (nil for
// none). Hosted mode must not run open, so missing credentials are an
// error. google is Google's endpoints; the zero value means the real
// Google.
func hostedAuth(c config, public *url.URL, google oauth2.Endpoint, places hostauth.Places) (*hostauth.Auth, error) {
	secret := os.Getenv("TCGREPLAY_GOOGLE_CLIENT_SECRET")
	if c.googleClientID == "" || secret == "" {
		return nil, errors.New("hosted mode needs Google sign-in: set TCGREPLAY_GOOGLE_CLIENT_ID and TCGREPLAY_GOOGLE_CLIENT_SECRET")
	}
	return hostauth.New(hostauth.Config{
		Public:         public,
		GoogleClientID: c.googleClientID,
		GoogleSecret:   secret,
		SigningKey:     os.Getenv("TCGREPLAY_SIGNING_KEY"),
		RateLimit:      c.rateLimit,
		Places:         places,
		Google:         google,
	})
}

// signInText describes, for the start-up log, whether sign-ins outlast a
// restart.
func signInText() string {
	if os.Getenv("TCGREPLAY_SIGNING_KEY") == "" {
		return "Google sign-in (ends on restart: no TCGREPLAY_SIGNING_KEY)"
	}
	return "Google sign-in (kept across restarts)"
}

// idleText describes the idle setting for the start-up log.
func idleText(d time.Duration) string {
	if d <= 0 {
		return "idle games never dropped"
	}
	return fmt.Sprintf("idle games dropped after %s", d)
}

// hostedHandler serves MCP over Streamable HTTP at /mcp, every session
// sharing srv, and the playmat on every other path, with a's sign-in in
// front: its own routes (metadata, client registration, authorize, token,
// Google's callback, /healthz) are open, /mcp needs an access token and
// the board needs a session. The board goes through reg's front door to
// the signed-in account's own board.
func hostedHandler(srv *mcp.Server, reg *usergames.Registry, a *hostauth.Auth) http.Handler {
	mux := http.NewServeMux()
	a.Routes(mux)
	mux.Handle("/mcp", a.RequireToken(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{
		SessionTimeout: mcpSessionTimeout,
		// Hosted mode is public; the request's Host is the proxy's business.
		DisableLocalhostProtection: true,
	})))
	mux.Handle("/", a.RequireSession(reg.Board()))
	return mux
}

// newHosted builds the per-account game registry, the sign-in service and
// the MCP server for hosted mode. Each signed-in account gets its own game
// manager (sharing one card database) and its own board server; every
// tool call acts on the caller's game, and its board link names the
// caller's own board on the public URL, from which the board also accepts
// live connections. The registry keeps the user cap, so a place follows
// activity. Every MCP request and board action counts against its
// account's request limit, and tool calls, board actions and board page
// loads count as the account's activity. google is as for hostedAuth.
func newHosted(c config, public *url.URL, google oauth2.Endpoint) (*usergames.Registry, *mcp.Server, *hostauth.Auth, error) {
	db := newCardDB(c)
	human := state.Seat(c.seat).Other()
	var reg *usergames.Registry
	var a *hostauth.Auth // set below, before any board is built
	reg = usergames.New(usergames.Config{
		NewManager: func() *game.Manager { return game.NewManager(c.dataDir, db) },
		NewBoard: func(m *game.Manager) *webui.Server {
			ui := webui.New(m, human)
			ui.Public = public
			ui.Limit = reg.BoardLimit(a.BoardLimit)
			return ui
		},
		Public:   public,
		Idle:     c.idleTimeout,
		MaxUsers: c.maxUsers,
		Now:      c.now,
	})
	a, err := hostedAuth(c, public, google, reg)
	if err != nil {
		return nil, nil, nil, err
	}
	// BoardURL keeps the instructions' board-link line; each result's link
	// comes from Resolve.
	srv := (&mcpserver.Server{ClaudeSeat: state.Seat(c.seat), Version: version, BoardURL: public.String() + "/", Resolve: reg.ResolveMCP}).MCP()
	srv.AddReceivingMiddleware(a.MCPMiddleware())
	return reg, srv, a, nil
}

// runHosted serves MCP and the playmat over one plain-HTTP listener until
// ctx is done, for running in a container behind an HTTPS proxy.
func runHosted(ctx context.Context, c config) error {
	if c.idleErr != nil {
		return fmt.Errorf("hosted mode: %v", c.idleErr)
	}
	if c.idleTimeout < 0 {
		return fmt.Errorf("hosted mode: -idle-timeout %s: must not be negative", c.idleTimeout)
	}
	listen := hostedListen(c.listen)
	public, err := hostedPublicURL(c.publicURL, listen)
	if err != nil {
		return err
	}
	reg, srv, a, err := newHosted(c, public, oauth2.Endpoint{})
	if err != nil {
		return err
	}
	go reg.Run(ctx)

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	log.Printf("tcgreplay %s hosted: listening on %s, public %s, data %s, cache %s, claude plays %s, %s, %d requests a minute per account, %d users, %s", version, ln.Addr(), public, c.dataDir, c.cacheDir, c.seat, signInText(), c.rateLimit, c.maxUsers, idleText(c.idleTimeout))
	// No WriteTimeout: MCP event streams and the board's websocket are long-lived.
	hs := &http.Server{Handler: hostedHandler(srv, reg, a), ReadHeaderTimeout: 10 * time.Second}
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
