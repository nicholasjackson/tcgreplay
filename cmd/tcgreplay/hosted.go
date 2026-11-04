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
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/oauth2"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
	"github.com/nicholasjackson/tcgreplay/internal/mcpserver"
	"github.com/nicholasjackson/tcgreplay/internal/spaces"
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
// loads count as the account's activity. db is the card database every
// account shares (see hostedCardDB); images, when not nil, is where every
// board loads card images from (see hostedCardDB). google is as for
// hostedAuth.
func newHosted(c config, db *cards.Client, images *cards.ImageCache, public *url.URL, google oauth2.Endpoint) (*usergames.Registry, *mcp.Server, *hostauth.Auth, error) {
	human := state.Seat(c.seat).Other()
	var reg *usergames.Registry
	var a *hostauth.Auth // set below, before any board is built
	reg = usergames.New(usergames.Config{
		NewManager: func() *game.Manager {
			m := game.NewManager(c.dataDir, db)
			m.Timing = c.timing
			return m
		},
		NewBoard: func(m *game.Manager) *webui.Server {
			return hostedBoard(m, human, public, reg.BoardLimit(a.BoardLimit), images)
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
	srv := (&mcpserver.Server{ClaudeSeat: state.Seat(c.seat), Version: version, BoardURL: public.String() + "/", Resolve: reg.ResolveMCP, ResolveQuiet: reg.ResolveMCPQuiet}).MCP()
	srv.AddReceivingMiddleware(a.MCPMiddleware())
	return reg, srv, a, nil
}

// hostedBoard builds one account's board server: on the public address,
// with the account's request limit, and loading card images through
// images when it is not nil.
func hostedBoard(m *game.Manager, human state.Seat, public *url.URL, limit func(context.Context) error, images *cards.ImageCache) *webui.Server {
	ui := webui.New(m, human)
	ui.Public = public
	ui.Limit = limit
	// A nil *ImageCache in the interface would not be nil.
	if images != nil {
		ui.Images = images
	}
	return ui
}

// hostedCardDB builds hosted mode's card database. With all four
// TCGREPLAY_SPACES_* settings present and the playmat online, its cache is
// local disk backed by the shared store in DigitalOcean Spaces, so it
// outlasts a deploy, and shared reports true. With none set it caches on
// local disk only, as the desktop does. Partial or invalid settings are a
// warning, not an error: the cache is optional and must never stop play.
// Only hosted mode reads these settings.
//
// With the shared store on, images is the card image cache on the same
// bucket: card images are copied there and boards load them from its
// public (CDN) address. It is nil otherwise, and boards load images from
// the card source. The caller runs it.
func hostedCardDB(c config) (db *cards.Client, shared bool, images *cards.ImageCache) {
	db = newCardDB(c)
	if c.cardBase != "" {
		db.Base = c.cardBase
	}
	cfg, set, err := spaces.FromEnv(os.Getenv)
	switch {
	case !set:
		log.Printf("card cache: local only, in %s (no TCGREPLAY_SPACES_* settings)", c.cacheDir)
		return db, false, nil
	case err != nil:
		log.Printf("warning: card cache: %s; using the local cache only", strings.ReplaceAll(err.Error(), "\n", "; "))
		return db, false, nil
	case c.offline:
		log.Printf("card cache: offline, shared store %s not used", cfg)
		return db, false, nil
	}
	sc, err := spaces.New(cfg)
	if err != nil {
		log.Printf("warning: card cache: %v; using the local cache only", err)
		return db, false, nil
	}
	db.Store = cards.NewTieredStore(db.Store, cards.NewSpacesStore(sc))
	log.Printf("card cache: %s backed by shared store %s", c.cacheDir, cfg)
	images = cards.NewImageCache(sc, cfg.PublicBase())
	if c.imageBase != "" {
		images.Source = c.imageBase
	}
	log.Printf("card images: copied from the card source to the shared store, served from %s/%s", images.Public, cards.ImagePrefix)
	return db, true, images
}

// catalogWarmTimeout bounds the start-up catalog warm.
const catalogWarmTimeout = 2 * time.Minute

// warmCatalog builds the card catalog in the background at start-up, so
// the first import after a deploy does not wait for it, and logs how it
// went.
func warmCatalog(ctx context.Context, db *cards.Client) {
	start := time.Now()
	wctx, cancel := context.WithTimeout(ctx, catalogWarmTimeout)
	defer cancel()
	if err := db.Warm(wctx); err != nil {
		log.Printf("card cache: catalog warm failed after %s: %v", time.Since(start).Round(time.Millisecond), err)
		return
	}
	log.Printf("card cache: catalog warmed in %s", time.Since(start).Round(time.Millisecond))
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
	db, shared, images := hostedCardDB(c)
	reg, srv, a, err := newHosted(c, db, images, public, oauth2.Endpoint{})
	if err != nil {
		return err
	}
	go reg.Run(ctx)
	if images != nil {
		go images.Run(ctx)
	}
	if shared {
		go warmCatalog(ctx, db)
	}

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	log.Printf("tcgreplay %s hosted: listening on %s, public %s, data %s, cache %s, claude plays %s, %s, %d requests a minute per account, %d users, %s, %s", version, ln.Addr(), public, c.dataDir, c.cacheDir, c.seat, signInText(), c.rateLimit, c.maxUsers, idleText(c.idleTimeout), timingText(c.timing))
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
