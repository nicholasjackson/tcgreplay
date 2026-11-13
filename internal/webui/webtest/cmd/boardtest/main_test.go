package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/webanalytics"
)

const token = "0123456789abcdef0123456789abcdef"

// page stands in for the board page.
var page = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, "<!doctype html><title>tcgreplay</title><body><div id=app></div></body>")
})

func serve(h http.Handler) string {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "127.0.0.1:7789"
	h.ServeHTTP(rec, r)
	b, _ := io.ReadAll(rec.Result().Body)
	return string(b)
}

// Without -analytics-token the board is served as it is; with it, the
// page carries hosted mode's beacon for the server's own host.
func TestAnalytics(t *testing.T) {
	off, err := analytics("", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if body := serve(off(page)); strings.Contains(body, "cloudflareinsights") {
		t.Errorf("without a token the board carries the beacon: %q", body)
	}
	on, err := analytics(token, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if body := serve(on(page)); strings.Count(body, webanalytics.Snippet(token)) != 1 {
		t.Errorf("with a token, want one beacon: %q", body)
	}
	if _, err := analytics("not a token", "127.0.0.1"); err == nil || strings.Contains(err.Error(), "not a token") {
		t.Errorf("bad token: err %v, want a refusal without the token", err)
	}
}
