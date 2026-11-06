package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/site"
)

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	resp := rec.Result()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestPlaymatAddressesAreUnavailable(t *testing.T) {
	h := handler()
	for _, path := range []string{"/board", "/board/", "/board/abc", "/mcp", "/ws", "/api/card", "/api/"} {
		if code, _ := get(t, h, path); code != http.StatusServiceUnavailable {
			t.Errorf("GET %s = %d, want 503", path, code)
		}
	}
}

func TestSiteServedAtRoot(t *testing.T) {
	h := handler()
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
