// Command boardtest serves the built board over the fake card pool, from
// the fixed position the browser tests start from. POST /test/reset starts
// that position afresh. It is test support, run by Playwright; build the
// board first (make web), since the board is embedded.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/webui"
	"github.com/nicholasjackson/tcgreplay/internal/webui/webtest"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7790", "loopback address to serve the board on")
	flag.Parse()
	if err := run(*addr); err != nil {
		log.Fatal(err)
	}
}

func run(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%s is not a loopback address", addr)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	dir, err := os.MkdirTemp("", "boardtest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	m := webtest.NewManager(dir)
	if err := webtest.Reset(ctx, m); err != nil {
		return err
	}
	s := webui.New(m, state.P1)
	s.StartHub(ctx)

	mux := http.NewServeMux()
	mux.Handle("/", s.Handler())
	mux.HandleFunc("POST /test/reset", func(w http.ResponseWriter, r *http.Request) {
		if err := webtest.Reset(r.Context(), m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	log.Printf("boardtest: serving the board on http://%s/", addr)
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
