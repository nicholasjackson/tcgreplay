// Command tcgreplay is a local virtual Pokémon TCG playmat driven by Claude
// over MCP (stdio).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/mcpserver"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

var version = "dev"

func main() {
	dataDir := flag.String("data", game.DefaultDataDir(), "directory for game files")
	cacheDir := flag.String("cache", cards.DefaultCacheDir(), "directory for the TCGdex cache")
	seat := flag.String("seat", "p2", "seat Claude plays")
	offline := flag.Bool("offline", false, "use only cached card data")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	// stdout carries the MCP protocol, so logs go to stderr and a file.
	log.SetOutput(os.Stderr)
	if err := os.MkdirAll(*dataDir, 0o755); err == nil {
		if f, err := os.OpenFile(filepath.Join(*dataDir, "tcgreplay.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			log.SetOutput(f)
			defer f.Close()
		}
	}

	claude := state.Seat(*seat)
	if !claude.Valid() {
		log.Fatalf("bad seat %q", *seat)
	}
	db := cards.NewClient(*cacheDir)
	db.Offline = *offline
	m := game.NewManager(*dataDir, db)
	srv := (&mcpserver.Server{M: m, ClaudeSeat: claude, Version: version}).MCP()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("tcgreplay %s starting: data %s, cache %s, claude plays %s", version, *dataDir, *cacheDir, claude)
	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
