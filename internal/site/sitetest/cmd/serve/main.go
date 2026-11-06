// Command serve serves the built website through site.Handler, the way the
// hosted playmat serves it, for the site's browser suite (site/e2e). The
// playmat's own addresses (/board, /mcp, /ws, /api/) answer 503, as if the
// playmat were unreachable. It is test support, run by Playwright; build
// the site first (make site), since the site is embedded.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/site"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7791", "loopback address to serve the website on")
	flag.Parse()
	if err := run(*addr); err != nil {
		log.Fatal(err)
	}
}

// handler is the website with the playmat's own addresses unavailable.
func handler() http.Handler {
	unavailable := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "the playmat is not part of the website test server", http.StatusServiceUnavailable)
	})
	mux := http.NewServeMux()
	mux.Handle("/", site.Handler())
	for _, p := range []string{"/board", "/board/", "/mcp", "/ws", "/api/"} {
		mux.Handle(p, unavailable)
	}
	return mux
}

func run(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%s is not a loopback address", addr)
	}
	if !site.Built() {
		return errors.New("the website is not built into this binary: run make site first")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	log.Printf("site serve: serving the website on http://%s/", addr)
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
