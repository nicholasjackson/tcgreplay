// Command serve serves the built website through site.Handler, the way the
// hosted playmat serves it, for the site's browser suite (site/e2e). The
// playmat's own addresses (/board, /mcp, /ws, /api/) answer 503, as if the
// playmat were unreachable. It is test support, run by Playwright; build
// the site first (make site), since the site is embedded.
//
// With -analytics-token, the website is wrapped with visitor counting the
// way hosted mode wraps it (package webanalytics), gated on the same
// privacy-page check, for this server's own loopback host, so the
// browser suite can check the pages with Cloudflare's beacon in place
// (the suite stubs or blocks Cloudflare, so nothing is sent). If counting
// would be off, for example because the built privacy page does not
// describe it, the server stops with the reason instead of serving pages
// with no beacon. Without the flag it serves the website as it is.
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

	"github.com/nicholasjackson/tcgreplay/internal/site"
	"github.com/nicholasjackson/tcgreplay/internal/webanalytics"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7791", "loopback address to serve the website on")
	token := flag.String("analytics-token", "", "with a Cloudflare Web Analytics site token, add the beacon to the website's pages as hosted mode does (tests stub Cloudflare)")
	flag.Parse()
	if err := run(*addr, *token); err != nil {
		log.Fatal(err)
	}
}

// handler is the website, wrapped by counted, with the playmat's own
// addresses unavailable.
func handler(counted func(http.Handler) http.Handler) http.Handler {
	unavailable := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "the playmat is not part of the website test server", http.StatusServiceUnavailable)
	})
	mux := http.NewServeMux()
	mux.Handle("/", counted(site.Handler()))
	for _, p := range []string{"/board", "/board/", "/mcp", "/ws", "/api/"} {
		mux.Handle(p, unavailable)
	}
	return mux
}

// analytics returns the wrapper for the website: the identity without a
// token, else hosted mode's visitor counting for host, or an error naming
// why counting would be off.
func analytics(token, host string) (func(http.Handler) http.Handler, error) {
	if token == "" {
		return func(h http.Handler) http.Handler { return h }, nil
	}
	wrap, status := webanalytics.Setup(webanalytics.Settings{Token: token, Host: host, PrivacyDescribes: site.DescribesVisitorCounting()})
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
	if !site.Built() {
		return errors.New("the website is not built into this binary: run make site first")
	}
	counted, err := analytics(token, host)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: handler(counted), ReadHeaderTimeout: 10 * time.Second}
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
