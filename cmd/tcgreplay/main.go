// Command tcgreplay is a local virtual Pokémon TCG playmat driven by an AI
// assistant (Claude, Codex, Gemini) over MCP (stdio).
//
// "tcgreplay install -client claude|codex|gemini" installs the bundled
// skills and registers the server with that assistant.
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
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/mcpserver"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/webui"
)

var version = "dev"

type config struct {
	dataDir, cacheDir, seat, uiAddr string
	offline                         bool
	// publicURL and listen are for hosted mode only.
	publicURL, listen string
}

// args renders the config as flags for spawning the daemon.
func (c config) args() []string {
	return []string{"-data", c.dataDir, "-cache", c.cacheDir, "-seat", c.seat, "-ui-addr", c.uiAddr, fmt.Sprintf("-offline=%t", c.offline)}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "install" {
		if err := runInstall(os.Args[2:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	var c config
	flag.StringVar(&c.dataDir, "data", game.DefaultDataDir(), "directory for the playmat's log and daemon files")
	flag.StringVar(&c.cacheDir, "cache", cards.DefaultCacheDir(), "directory for the TCGdex cache")
	flag.StringVar(&c.seat, "seat", "p2", "seat Claude plays")
	flag.BoolVar(&c.offline, "offline", false, "use only cached card data")
	flag.StringVar(&c.uiAddr, "ui-addr", "127.0.0.1:7777", "address for the playmat UI; empty disables it")
	noDaemon := flag.Bool("no-daemon", false, "serve MCP over stdio in-process instead of relaying to the daemon")
	daemon := flag.Bool("daemon", false, "run the shared daemon (normally started by the shim)")
	replace := flag.Bool("replace", false, "with -daemon, stop a daemon already running on the data dir instead of exiting")
	logStderr := flag.Bool("log-stderr", false, "log to stderr instead of the data dir's log file (for running the daemon in a terminal)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	hosted := flag.Bool("hosted", false, "serve MCP over HTTP at /mcp and the playmat on one address, for a container")
	flag.StringVar(&c.listen, "listen", "", "with -hosted, the address to listen on (default :$PORT, else :8080)")
	flag.StringVar(&c.publicURL, "public-url", os.Getenv("TCGREPLAY_PUBLIC_URL"), "with -hosted, the public address the playmat is reached at (default $TCGREPLAY_PUBLIC_URL)")
	flag.Parse()

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

// newServer builds the game manager and MCP server. boardURL is the board
// link added to game-opening results in hosted mode; the desktop passes "".
func newServer(c config, boardURL string) (*game.Manager, *mcp.Server) {
	db := cards.NewClient(c.cacheDir)
	db.Offline = c.offline
	m := game.NewManager(c.dataDir, db)
	claude := state.Seat(c.seat)
	return m, (&mcpserver.Server{M: m, ClaudeSeat: claude, Version: version, BoardURL: boardURL}).MCP()
}

func runInProcess(ctx context.Context, c config) error {
	m, srv := newServer(c, "")
	log.Printf("tcgreplay %s in-process: data %s, cache %s, claude plays %s", version, c.dataDir, c.cacheDir, c.seat)
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
