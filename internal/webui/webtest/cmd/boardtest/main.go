// Command boardtest serves the built board over the fake card pool, from
// the fixed position the browser tests start from. POST /test/reset starts
// that position afresh. It is test support, run by Playwright; build the
// board first (make web), since the board is embedded.
//
// With -analytics-token, the board is wrapped with visitor counting the
// way hosted mode wraps the /board page (package webanalytics), for this
// server's own loopback host, so the browser suite can check the board
// with Cloudflare's beacon in place (the suite stubs or blocks Cloudflare,
// so nothing is sent). Without the flag the board is served as it is.
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
	"strings"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/webanalytics"
	"github.com/nicholasjackson/tcgreplay/internal/webui"
	"github.com/nicholasjackson/tcgreplay/internal/webui/webtest"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7790", "loopback address to serve the board on")
	token := flag.String("analytics-token", "", "with a Cloudflare Web Analytics site token, add the beacon to the board page as hosted mode does (tests stub Cloudflare)")
	flag.Parse()
	if err := run(*addr, *token); err != nil {
		log.Fatal(err)
	}
}

// analytics returns the wrapper for the board: the identity without a
// token, else hosted mode's visitor counting for host, or an error naming
// why counting would be off. The board test server has no website, so
// the privacy-page precondition is the hosted playmat's business, not the
// board's: it is taken as met here.
func analytics(token, host string) (func(http.Handler) http.Handler, error) {
	if token == "" {
		return func(h http.Handler) http.Handler { return h }, nil
	}
	wrap, status := webanalytics.Setup(webanalytics.Settings{Token: token, Host: host, PrivacyDescribes: true})
	if !strings.HasPrefix(status, "visitor analytics: on") {
		return nil, errors.New(status)
	}
	log.Print(status)
	return wrap, nil
}

func run(addr, token string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%s is not a loopback address", addr)
	}
	counted, err := analytics(token, host)
	if err != nil {
		return err
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
	mux.Handle("/", counted(s.Handler()))
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
