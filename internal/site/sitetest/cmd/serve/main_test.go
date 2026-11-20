package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/site"
	"github.com/nicholasjackson/tcgreplay/internal/webanalytics"
)

// identity is the website with no counting, as the server runs by default.
func identity(h http.Handler) http.Handler { return h }

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	resp := rec.Result()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestPlaymatAddressesAreUnavailable(t *testing.T) {
	h := handler(identity)
	for _, path := range []string{"/board", "/board/", "/board/abc", "/mcp", "/ws", "/api/card", "/api/"} {
		if code, _ := get(t, h, path); code != http.StatusServiceUnavailable {
			t.Errorf("GET %s = %d, want 503", path, code)
		}
	}
}

func TestSiteServedAtRoot(t *testing.T) {
	h := handler(identity)
	for _, path := range []string{"/", "/no-such-page"} {
		gotCode, gotBody := get(t, h, path)
		wantCode, wantBody := get(t, site.Handler(), path)
		if gotCode != wantCode || gotBody != wantBody {
			t.Errorf("GET %s = %d (%d bytes), want the site handler's %d (%d bytes)", path, gotCode, len(gotBody), wantCode, len(wantBody))
		}
	}
	if code, _ := get(t, h, "/"); code != http.StatusOK {
		t.Errorf("GET / = %d, want 200", code)
	}
}

const token = "0123456789abcdef0123456789abcdef"

// getFrom fetches path from h as a browser on host would.
func getFrom(t *testing.T, h http.Handler, host, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Host = host
	h.ServeHTTP(rec, r)
	body, _ := io.ReadAll(rec.Result().Body)
	return rec.Code, string(body)
}

// With -analytics-token the website carries hosted mode's beacon for the
// server's own host; without it, it does not.
func TestAnalyticsToken(t *testing.T) {
	if !site.Built() {
		t.Skip("the website is not built (make site)")
	}
	off, err := analytics("", "127.0.0.1")
	if err != nil {
		t.Fatalf("no token: %v", err)
	}
	if _, body := getFrom(t, handler(off), "127.0.0.1:7792", "/"); strings.Contains(body, "cloudflareinsights") {
		t.Error("without a token, / carries the beacon")
	}

	if !site.DescribesVisitorCounting() {
		// The server refuses to count without the privacy page's section.
		if _, err := analytics(token, "127.0.0.1"); err == nil || !strings.Contains(err.Error(), "privacy page") {
			t.Fatalf("privacy page without the section: err %v, want the reason", err)
		}
		return
	}
	on, err := analytics(token, "127.0.0.1")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	h := handler(on)
	snippet := webanalytics.Snippet(token)
	for _, path := range []string{"/", "/privacy"} {
		if code, body := getFrom(t, h, "127.0.0.1:7792", path); code != http.StatusOK || strings.Count(body, snippet) != 1 {
			t.Errorf("GET %s with a token: %d, %d beacons, want 200 with 1", path, code, strings.Count(body, snippet))
		}
	}
	if code, body := getFrom(t, h, "127.0.0.1:7792", "/no-such-page"); code != http.StatusNotFound || strings.Contains(body, "cloudflareinsights") {
		t.Errorf("GET /no-such-page with a token: %d, beacon %v", code, strings.Contains(body, "cloudflareinsights"))
	}
	if code, _ := getFrom(t, h, "127.0.0.1:7792", "/board"); code != http.StatusServiceUnavailable {
		t.Errorf("GET /board with a token: %d, want 503", code)
	}
}

// A malformed token stops the server with the reason, never the token.
func TestAnalyticsBadTokenRefused(t *testing.T) {
	_, err := analytics("not a token", "127.0.0.1")
	if err == nil || strings.Contains(err.Error(), "not a token") {
		t.Fatalf("bad token: err %v, want a refusal without the token", err)
	}
}
