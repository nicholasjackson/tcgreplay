// Command tcgreplay is a local virtual Pokémon TCG playmat driven by an AI
// assistant (Claude, Codex, Gemini) over MCP (stdio).
//
// The Claude desktop app may launch several copies, and restarts them at
// will, so by default this process is only a thin stdio shim: it relays MCP
// to a long-running daemon (starting one if needed) that owns the current
// game (in memory) and the playmat UI. Every client therefore sees the same
// game, and the UI survives app restarts. Games last only as long as the
// process. Run with -no-daemon to serve everything in-process.
//
// -hosted serves MCP over HTTP at /mcp and the playmat on one address, for
// running in a container behind an HTTPS proxy (see -listen, -public-url).
// Hosted mode requires Google sign-in for both, keeps one game per
// signed-in account and drops games left idle (-idle-timeout); the desktop
// modes never ask for sign-in and keep one game.
//
// tcgreplay sets builds card sets into the project ahead of time, for the
// maintainer: sets import <set> fetches and merges a set, sets check checks
// the built sets, and sets publish publishes them to the bucket the
// playmat loads card data from (see internal/cardset). It never starts the
// playmat.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/catalog"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/mcpserver"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/webui"
)

var version = "dev"

type config struct {
	// cacheDir holds the local copy of published card sets; offline uses
	// that copy alone.
	dataDir, cacheDir, seat, uiAddr string
	offline                         bool
	// cardsetsURL is where card sets are published (an https address or a
	// directory); "" means none, so only the local copy is used.
	// cardsetsInterval is how often a running playmat checks it.
	cardsetsURL      string
	cardsetsInterval time.Duration
	// The rest are for hosted mode only.
	publicURL, listen string
	// googleClientID is the Google OAuth client users sign in with (its
	// secret is read from the environment only).
	googleClientID string
	// rateLimit is requests a minute per account; maxUsers is places:
	// signed-in accounts served at once. 0 turns either off.
	rateLimit, maxUsers int
	// idleTimeout is how long an account's game may go without activity
	// before it is dropped; 0 never drops. idleErr is an invalid
	// TCGREPLAY_IDLE_TIMEOUT, reported when hosted mode starts.
	idleTimeout time.Duration
	idleErr     error
	// timing is the turn wait's settings, given to every game manager;
	// timingErr is an invalid TCGREPLAY_WAIT_CAP, TCGREPLAY_CHECK_IN or
	// TCGREPLAY_LOCK_RELEASE whose flag was not given.
	timing    game.Timing
	timingErr error
	// now, if set, is the per-account registry's clock (tests).
	now func() time.Time
	// imageBase, if set, is the card image source's prefix (tests).
	imageBase string
	// cdnAPI, if set, is the DigitalOcean API base the CDN purge client
	// calls (tests).
	cdnAPI string
}

// args renders the config as flags for spawning the daemon. Unset wait
// timings are left out, so the daemon uses its defaults.
func (c config) args() []string {
	args := []string{"-data", c.dataDir, "-cache", c.cacheDir, "-seat", c.seat, "-ui-addr", c.uiAddr, fmt.Sprintf("-offline=%t", c.offline), "-cardsets-url", c.cardsetsURL}
	if c.cardsetsInterval != 0 {
		args = append(args, "-cardsets-interval", c.cardsetsInterval.String())
	}
	for _, t := range []struct {
		flag string
		d    time.Duration
	}{{"-wait-cap", c.timing.WaitCap}, {"-check-in", c.timing.CheckIn}, {"-lock-release", c.timing.LockRelease}} {
		if t.d != 0 {
			args = append(args, t.flag, t.d.String())
		}
	}
	return args
}

// maxWaitCap keeps every wait_for_turn call well inside the one-minute
// limit of clients and proxies.
const maxWaitCap = 50 * time.Second

// validTiming checks the turn wait's settings: the cap must be above 0 and
// at most 50s, and the check-in and lock-release times must not be
// negative (0 means the default).
func validTiming(c config) error {
	if c.timingErr != nil {
		return c.timingErr
	}
	if c.timing.WaitCap <= 0 || c.timing.WaitCap > maxWaitCap {
		return fmt.Errorf("-wait-cap %s: must be above 0 and at most %s", c.timing.WaitCap, maxWaitCap)
	}
	if c.timing.CheckIn < 0 {
		return fmt.Errorf("-check-in %s: must not be negative", c.timing.CheckIn)
	}
	if c.timing.LockRelease < 0 {
		return fmt.Errorf("-lock-release %s: must not be negative", c.timing.LockRelease)
	}
	return nil
}

// timingText describes the turn wait's settings for the start-up log.
func timingText(t game.Timing) string {
	return fmt.Sprintf("turn waits up to %s a call, check-in after %s, lock release after %s", t.WaitCap, t.CheckIn, t.LockRelease)
}

// envWait reads one of the turn wait's duration settings from the
// environment, like envDuration.
func envWait(key string, def time.Duration) (time.Duration, error) {
	d, err := envDuration(key, def)
	if err != nil {
		return def, fmt.Errorf("%s: invalid duration %q (use e.g. 30s or 2m)", key, os.Getenv(key))
	}
	return d, nil
}

func main() {
	// sets is the one subcommand, dispatched before the playmat's flags.
	if len(os.Args) > 1 && os.Args[1] == "sets" {
		os.Exit(runSets(os.Args[2:]))
	}
	var c config
	flag.StringVar(&c.dataDir, "data", game.DefaultDataDir(), "directory for the playmat's log and daemon files")
	flag.StringVar(&c.cacheDir, "cache", catalog.DefaultCacheDir(), "directory for the local copy of published card sets")
	flag.StringVar(&c.seat, "seat", "p2", "seat Claude plays")
	flag.BoolVar(&c.offline, "offline", false, "use only the local copy of published card sets")
	flag.StringVar(&c.cardsetsURL, "cardsets-url", envOr("TCGREPLAY_CARDSETS_URL", defaultCardsetsURL), "where card sets are published: an https address or a directory (default $TCGREPLAY_CARDSETS_URL, else the project's CDN)")
	interval, intervalErr := envWait("TCGREPLAY_CARDSETS_INTERVAL", catalog.DefaultInterval)
	flag.DurationVar(&c.cardsetsInterval, "cardsets-interval", interval, "how often a running playmat checks for published card sets, from 10s to 10m (default $TCGREPLAY_CARDSETS_INTERVAL, else 5m)")
	flag.StringVar(&c.uiAddr, "ui-addr", "127.0.0.1:7777", "address for the playmat UI; empty disables it")
	noDaemon := flag.Bool("no-daemon", false, "serve MCP over stdio in-process instead of relaying to the daemon")
	daemon := flag.Bool("daemon", false, "run the shared daemon (normally started by the shim)")
	replace := flag.Bool("replace", false, "with -daemon, stop a daemon already running on the data dir instead of exiting")
	logStderr := flag.Bool("log-stderr", false, "log to stderr instead of the data dir's log file (for running the daemon in a terminal)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	hosted := flag.Bool("hosted", false, "serve MCP over HTTP at /mcp and the playmat on one address, for a container")
	flag.StringVar(&c.listen, "listen", "", "with -hosted, the address to listen on (default :$PORT, else :8080)")
	flag.StringVar(&c.publicURL, "public-url", os.Getenv("TCGREPLAY_PUBLIC_URL"), "with -hosted, the public address the playmat is reached at (default $TCGREPLAY_PUBLIC_URL)")
	flag.StringVar(&c.googleClientID, "google-client-id", os.Getenv("TCGREPLAY_GOOGLE_CLIENT_ID"), "with -hosted, the Google OAuth client users sign in with (default $TCGREPLAY_GOOGLE_CLIENT_ID; the secret is read from $TCGREPLAY_GOOGLE_CLIENT_SECRET)")
	flag.IntVar(&c.rateLimit, "rate-limit", envInt("TCGREPLAY_RATE_LIMIT", 300), "with -hosted, requests a minute one account may make; 0 for no limit (default $TCGREPLAY_RATE_LIMIT, else 300)")
	flag.IntVar(&c.maxUsers, "max-users", envInt("TCGREPLAY_MAX_USERS", 10), "with -hosted, signed-in Google accounts served at once (a place is freed when an account's game goes idle or its last sign-in ends); 0 for no cap (default $TCGREPLAY_MAX_USERS, else 10)")
	idle, idleErr := envDuration("TCGREPLAY_IDLE_TIMEOUT", 60*time.Minute)
	flag.DurationVar(&c.idleTimeout, "idle-timeout", idle, "with -hosted, how long an account's game may go without activity before it is dropped, e.g. 2m or 1h; 0 never drops (default $TCGREPLAY_IDLE_TIMEOUT, else 60m)")
	waitCap, capErr := envWait("TCGREPLAY_WAIT_CAP", 15*time.Second)
	flag.DurationVar(&c.timing.WaitCap, "wait-cap", waitCap, "longest a single wait_for_turn call waits before reporting it is still waiting; above 0 and at most 50s (default $TCGREPLAY_WAIT_CAP, else 15s)")
	checkIn, checkErr := envWait("TCGREPLAY_CHECK_IN", 5*time.Minute)
	flag.DurationVar(&c.timing.CheckIn, "check-in", checkIn, "how long the player may leave the board untouched on their turn before the assistant asks \"Still playing?\" (default $TCGREPLAY_CHECK_IN, else 5m)")
	lockRelease, lockErr := envWait("TCGREPLAY_LOCK_RELEASE", 2*time.Minute)
	flag.DurationVar(&c.timing.LockRelease, "lock-release", lockRelease, "how long the assistant may go without a move before the board unlocks by itself (default $TCGREPLAY_LOCK_RELEASE, else 2m)")
	flag.Parse()
	// sets (dispatched above) is the only subcommand: refuse any other
	// leftover word, so a removed one (such as install) fails instead of
	// starting the shim.
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unknown command %q\n", flag.Arg(0))
		flag.Usage()
		os.Exit(2)
	}
	c.idleErr = idleErr
	// A flag overrides the environment, even an invalid value there.
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "idle-timeout":
			c.idleErr = nil
		case "wait-cap":
			capErr = nil
		case "check-in":
			checkErr = nil
		case "lock-release":
			lockErr = nil
		case "cardsets-interval":
			intervalErr = nil
		}
	})
	if err := validCardsetsInterval(c.cardsetsInterval, intervalErr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	c.timingErr = errors.Join(capErr, checkErr, lockErr)
	if err := validTiming(c); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if c.timing.CheckIn == 0 {
		c.timing.CheckIn = 5 * time.Minute
	}
	if c.timing.LockRelease == 0 {
		c.timing.LockRelease = 2 * time.Minute
	}

	if *showVersion {
		fmt.Println(version)
		return
	}
	// stdout carries the MCP protocol, so logs go to stderr and a file.
	// Hosted mode logs only to stderr, which the container platform collects.
	log.SetOutput(os.Stderr)
	if err := os.MkdirAll(c.dataDir, 0o755); err == nil && !*logStderr && !*hosted {
		if f, err := os.OpenFile(filepath.Join(c.dataDir, "tcgreplay.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			log.SetOutput(f)
			defer f.Close()
		}
	}
	if !state.Seat(c.seat).Valid() {
		log.Fatalf("bad seat %q", c.seat)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	switch {
	case *hosted:
		err = runHosted(ctx, c)
	case *daemon:
		err = runDaemon(ctx, c, *replace)
	case *noDaemon:
		err = runInProcess(ctx, c)
	default:
		err = runShim(ctx, c)
	}
	if err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}

// newServer builds the game manager and MCP server over the card source
// src (nil: no card data). boardURL is the board link added to
// game-opening results in hosted mode; the desktop passes "".
func newServer(c config, boardURL string, src game.CardSource) (*game.Manager, *mcp.Server) {
	m := game.NewManager(c.dataDir, src)
	m.Timing = c.timing
	claude := state.Seat(c.seat)
	return m, (&mcpserver.Server{M: m, ClaudeSeat: claude, Version: version, BoardURL: boardURL}).MCP()
}

// defaultCardsetsURL is where the project publishes card sets: the
// manifest and set files in the playmat's Spaces bucket, through its CDN.
const defaultCardsetsURL = "https://tcgreplay.ams3.cdn.digitaloceanspaces.com/cardsets/"

// startupWait bounds how long a playmat waits at start for the published
// card sets before serving, so an early game does not pin an empty
// catalogue and an outage does not stop the playmat starting.
const startupWait = 30 * time.Second

// Bounds of -cardsets-interval: a publish must reach a running playmat
// within 10 minutes.
const (
	minCardsetsInterval = 10 * time.Second
	maxCardsetsInterval = 10 * time.Minute
)

// validCardsetsInterval checks the check interval: from 10s to 10m. envErr
// is an invalid TCGREPLAY_CARDSETS_INTERVAL whose flag was not given.
func validCardsetsInterval(d time.Duration, envErr error) error {
	if envErr != nil {
		return envErr
	}
	if d < minCardsetsInterval || d > maxCardsetsInterval {
		return fmt.Errorf("-cardsets-interval %s: must be from %s to %s", d, minCardsetsInterval, maxCardsetsInterval)
	}
	return nil
}

// envOr is the environment variable key, or def when it is unset.
func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// newCatalog builds the card catalogue loader for the config: published
// sets at cardsetsURL, the local copy under the cache directory, checked
// every cardsetsInterval. The caller starts and runs it.
func newCatalog(c config) *catalog.Loader {
	return &catalog.Loader{Base: c.cardsetsURL, CacheDir: c.cacheDir, Offline: c.offline, Interval: c.cardsetsInterval}
}

// startCatalog loads the catalogue, waiting up to wait for the published
// sets (0: the local copy only, checking in the background), and keeps it
// up to date in the background until ctx ends.
func startCatalog(ctx context.Context, c config, wait time.Duration) *catalog.Loader {
	l := newCatalog(c)
	l.Start(ctx, wait)
	go l.Run(ctx)
	return l
}

func runInProcess(ctx context.Context, c config) error {
	m, srv := newServer(c, "", startCatalog(ctx, c, startupWait))
	log.Printf("tcgreplay %s in-process: data %s, card sets %s (local copy in %s), claude plays %s, %s", version, c.dataDir, c.cardsetsURL, c.cacheDir, c.seat, timingText(c.timing))
	if c.uiAddr != "" {
		startUI(ctx, m, state.Seat(c.seat).Other(), c.uiAddr)
	}
	return srv.Run(ctx, &mcp.StdioTransport{})
}

// startUI serves the playmat in the background. A failure to listen (e.g.
// another instance holds the port) is logged, not fatal: MCP still works.
func startUI(ctx context.Context, m *game.Manager, human state.Seat, addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("playmat UI disabled: %v", err)
		return
	}
	log.Printf("playmat UI at http://%s/ (human plays %s)", ln.Addr(), human)
	ui := webui.New(m, human)
	go func() {
		if err := ui.Run(ctx, ln); err != nil {
			log.Printf("playmat UI stopped: %v", err)
		}
	}()
}
