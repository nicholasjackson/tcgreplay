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
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/game"
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

// hostedHandler serves MCP over Streamable HTTP at /mcp, every session
// sharing srv, and the playmat on every other path.
func hostedHandler(srv *mcp.Server, ui *webui.Server) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{
		SessionTimeout: mcpSessionTimeout,
		// Hosted mode is public; the request's Host is the proxy's business.
		DisableLocalhostProtection: true,
	}))
	mux.Handle("/", ui.Handler())
	return mux
}

// newHosted builds the game manager, MCP server and board for hosted mode,
// both servers taking their address from the one public URL: board links
// point at its root and the board accepts live connections from it.
func newHosted(c config, public *url.URL) (*game.Manager, *mcp.Server, *webui.Server) {
	m, srv := newServer(c, public.String()+"/")
	ui := webui.New(m, state.Seat(c.seat).Other())
	ui.Public = public
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
	m, srv, ui := newHosted(c, public)
	if err := m.Resume(); err != nil {
		log.Printf("could not reopen the last game: %v", err)
	}
	ui.StartHub(ctx)

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	log.Printf("tcgreplay %s hosted: listening on %s, public %s, data %s, cache %s, claude plays %s", version, ln.Addr(), public, c.dataDir, c.cacheDir, c.seat)
	// No WriteTimeout: MCP event streams and the board's websocket are long-lived.
	hs := &http.Server{Handler: hostedHandler(srv, ui), ReadHeaderTimeout: 10 * time.Second}
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
