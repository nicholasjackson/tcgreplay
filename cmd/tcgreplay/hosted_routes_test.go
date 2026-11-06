package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/site"
)

// siteResponse is what the website itself answers for path, to compare
// the hosted router's answer against (the placeholder when unbuilt).
func siteResponse(t *testing.T, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	site.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	body, _ := io.ReadAll(rec.Result().Body)
	return rec.Code, string(body)
}

// TestHostedRoutes: the hosted playmat's path table. The website is public
// on every path the playmat doesn't otherwise use; the board page is at
// /board behind sign-in, with its files, live connection and card lookups
// still at the root behind sign-in; MCP, sign-in, health and admin are as
// before.
func TestHostedRoutes(t *testing.T) {
	h := startHosted(t, testPublic)
	navigate := func(r *http.Request) { r.Header.Set("Sec-Fetch-Mode", "navigate") }

	// Signed out, the website is served with no redirect and no cookie.
	_, home := siteResponse(t, "/")
	for _, path := range []string{"/", "/docs/connect", "/privacy"} {
		wantCode, wantBody := siteResponse(t, path)
		resp, body := getWith(t, h.ts.URL+path, nil, navigate)
		if resp.StatusCode != wantCode || body != wantBody {
			t.Errorf("GET %s signed out: %d %q, want the website's %d %q", path, resp.StatusCode, body, wantCode, wantBody)
		}
		if loc := resp.Header.Get("Location"); loc != "" {
			t.Errorf("GET %s signed out redirected to %q", path, loc)
		}
		if c := resp.Header.Values("Set-Cookie"); len(c) != 0 {
			t.Errorf("GET %s signed out set cookies %q", path, c)
		}
	}
	if resp, body := getWith(t, h.ts.URL+"/", nil, navigate); resp.StatusCode != http.StatusOK || body != home {
		t.Fatalf("GET / signed out: %d %q, want the website's home", resp.StatusCode, body)
	}

	// An unknown path is the website's 404.
	wantCode, want404 := siteResponse(t, "/no-such-page")
	if wantCode != http.StatusNotFound {
		t.Fatalf("site answers /no-such-page with %d, want 404", wantCode)
	}
	if resp, body := getWith(t, h.ts.URL+"/no-such-page", nil, navigate); resp.StatusCode != http.StatusNotFound || body != want404 {
		t.Errorf("GET /no-such-page: %d %q, want the website's 404 %q", resp.StatusCode, body, want404)
	}

	// Signed out, the board page sends the browser to sign in and back to
	// /board, keeping its query.
	resp, _ := getWith(t, h.ts.URL+"/board?board=x", nil, navigate)
	if want := "/auth/login?next=%2Fboard%3Fboard%3Dx"; resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != want {
		t.Errorf("GET /board?board=x signed out: %d to %q, want 302 to %s", resp.StatusCode, resp.Header.Get("Location"), want)
	}

	// The board's files, live connection and card lookups still need a
	// session, and none of them is the website.
	for _, path := range []string{"/assets/app.js", "/ws", "/api/card?def=x"} {
		resp, body := getWith(t, h.ts.URL+path, nil, func(r *http.Request) { r.Header.Set("Sec-Fetch-Mode", "cors") })
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("GET %s signed out: %d %q, want 401", path, resp.StatusCode, body)
		}
	}
	if resp, _ := getWith(t, h.ts.URL+"/assets/app.js", nil, navigate); resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "/auth/login?next=%2Fassets") {
		t.Errorf("GET /assets/app.js signed out as a page: %d to %q, want sign-in", resp.StatusCode, resp.Header.Get("Location"))
	}

	// Signed in, /board is the account's board; / is still the website.
	cookie := h.cookie(t)
	resp, board := getWith(t, h.ts.URL+"/board", cookie, navigate)
	if resp.StatusCode != http.StatusOK || !strings.Contains(board, "<title>tcgreplay</title>") || board == home {
		t.Fatalf("GET /board signed in: %d %q, want the board page", resp.StatusCode, board)
	}
	if resp, body := getWith(t, h.ts.URL+"/", cookie, navigate); resp.StatusCode != http.StatusOK || body != home {
		t.Errorf("GET / signed in: %d %q, want the website's home", resp.StatusCode, body)
	}
	if resp, body := getWith(t, h.ts.URL+"/api/card", cookie, nil); resp.StatusCode != http.StatusNotFound || !strings.Contains(body, "unknown card") {
		t.Errorf("GET /api/card signed in: %d %q, want the board's own 404", resp.StatusCode, body)
	}

	// /board/ redirects to /board, keeping the query.
	for path, want := range map[string]string{"/board/": "/board", "/board/?board=x": "/board?board=x"} {
		resp, _ := getWith(t, h.ts.URL+path, cookie, navigate)
		if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != want {
			t.Errorf("GET %s: %d to %q, want 301 to %s", path, resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}

	// MCP, health and admin are unchanged.
	if resp, _ := getWith(t, h.ts.URL+"/mcp", nil, nil); resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") == "" {
		t.Errorf("GET /mcp without a token: %d (WWW-Authenticate %q), want 401 with a challenge", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
	if resp, body := getWith(t, h.ts.URL+"/healthz", nil, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz: %d %q, want 200", resp.StatusCode, body)
	}
	if resp, _ := getWith(t, h.ts.URL+"/admin", nil, navigate); resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/auth/login?next=%2Fadmin" {
		t.Errorf("GET /admin signed out: %d to %q, want sign-in returning to /admin", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp, _ := getWith(t, h.ts.URL+"/.well-known/oauth-protected-resource", nil, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("GET OAuth metadata: %d, want 200", resp.StatusCode)
	}
}
