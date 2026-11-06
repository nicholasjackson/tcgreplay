package site

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// builtSite is a website as Astro builds it (directory format).
var builtSite = fstest.MapFS{
	"dist/placeholder/index.html":      {Data: []byte("placeholder page")},
	"dist/app/index.html":              {Data: []byte("<!doctype html><title>home</title>")},
	"dist/app/404.html":                {Data: []byte("<!doctype html><title>Page not found</title>")},
	"dist/app/privacy/index.html":      {Data: []byte("<!doctype html><title>privacy</title>")},
	"dist/app/docs/connect/index.html": {Data: []byte("<!doctype html><title>connect</title>")},
	"dist/app/flat.html":               {Data: []byte("<!doctype html><title>flat</title>")},
	"dist/app/_astro/site.AbC123.css":  {Data: []byte("body{color:red}")},
	"dist/app/_astro/copy.XyZ789.js":   {Data: []byte("console.log(1)")},
	"dist/app/logo.svg":                {Data: []byte("<svg xmlns='http://www.w3.org/2000/svg'/>")},
	"dist/app/logo.png":                {Data: []byte("\x89PNG\r\n\x1a\n")},
	"dist/app/example.txt":             {Data: []byte("Setup\n")},
}

// withFS swaps the embedded tree for fs for the length of the test.
func withFS(t *testing.T, fs fstest.MapFS) {
	t.Helper()
	old := fsys
	fsys = fs
	t.Cleanup(func() { fsys = old })
}

func get(t *testing.T, h http.Handler, method, path string) (*http.Response, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	resp := rec.Result()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

func TestPlaceholderWhenNotBuilt(t *testing.T) {
	withFS(t, fstest.MapFS{"dist/placeholder/index.html": {Data: []byte("placeholder page")}})
	if Built() {
		t.Fatal("Built() = true with only the placeholder")
	}
	resp, body := get(t, Handler(), http.MethodGet, "/")
	if resp.StatusCode != http.StatusOK || body != "placeholder page" {
		t.Fatalf("GET / = %d %q, want 200 placeholder", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	resp, _ = get(t, Handler(), http.MethodGet, "/privacy")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /privacy on placeholder = %d, want 404", resp.StatusCode)
	}
}

// The real embedded tree: a fresh checkout has only the placeholder, so the
// binary builds and serves it without Node.
func TestEmbeddedTreeServes(t *testing.T) {
	resp, body := get(t, Handler(), http.MethodGet, "/")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "tcgreplay") {
		t.Fatalf("GET / = %d %q", resp.StatusCode, body)
	}
	if !Built() && !strings.Contains(body, "make site") {
		t.Errorf("unbuilt site should serve the placeholder, got %q", body)
	}
}

func TestBuiltSiteResolvesPages(t *testing.T) {
	withFS(t, builtSite)
	if !Built() {
		t.Fatal("Built() = false with dist/app/index.html present")
	}
	h := Handler()
	for _, tc := range []struct{ path, title string }{
		{"/", "home"},
		{"/index.html", "home"},
		{"/privacy", "privacy"},
		{"/privacy/", "privacy"},
		{"/docs/connect", "connect"},
		{"/flat", "flat"},
	} {
		resp, body := get(t, h, http.MethodGet, tc.path)
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, "<title>"+tc.title+"</title>") {
			t.Errorf("GET %s = %d %q, want 200 %s", tc.path, resp.StatusCode, body, tc.title)
		}
	}
}

func TestContentTypes(t *testing.T) {
	withFS(t, builtSite)
	h := Handler()
	for path, want := range map[string]string{
		"/privacy":                "text/html; charset=utf-8",
		"/_astro/site.AbC123.css": "text/css; charset=utf-8",
		"/_astro/copy.XyZ789.js":  "text/javascript; charset=utf-8",
		"/logo.svg":               "image/svg+xml",
		"/logo.png":               "image/png",
		"/example.txt":            "text/plain; charset=utf-8",
	} {
		resp, _ := get(t, h, http.MethodGet, path)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d", path, resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Type"); got != want {
			t.Errorf("GET %s Content-Type = %q, want %q", path, got, want)
		}
	}
}

func TestCacheHeaders(t *testing.T) {
	withFS(t, builtSite)
	h := Handler()
	for path, want := range map[string]string{
		"/":                       "no-cache",
		"/docs/connect":           "no-cache",
		"/logo.png":               "no-cache",
		"/_astro/site.AbC123.css": "public, max-age=31536000, immutable",
		"/_astro/copy.XyZ789.js":  "public, max-age=31536000, immutable",
		"/_astro/missing.js":      "no-cache",
		"/nope":                   "no-cache",
	} {
		resp, _ := get(t, h, http.MethodGet, path)
		if got := resp.Header.Get("Cache-Control"); got != want {
			t.Errorf("GET %s Cache-Control = %q, want %q", path, got, want)
		}
	}
}

func TestUnknownPathGetsSite404(t *testing.T) {
	withFS(t, builtSite)
	h := Handler()
	for _, path := range []string{"/nope", "/docs/nope", "/_astro/missing.js", "/../etc/passwd", "/docs"} {
		resp, body := get(t, h, http.MethodGet, path)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
		if !strings.Contains(body, "Page not found") {
			t.Errorf("GET %s body = %q, want the site's 404 page", path, body)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("GET %s Content-Type = %q", path, ct)
		}
	}
	resp, body := get(t, h, http.MethodHead, "/nope")
	if resp.StatusCode != http.StatusNotFound || body != "" {
		t.Errorf("HEAD /nope = %d %q, want 404 with no body", resp.StatusCode, body)
	}
}

func TestNoCookies(t *testing.T) {
	withFS(t, builtSite)
	h := Handler()
	for _, path := range []string{"/", "/privacy", "/_astro/site.AbC123.css", "/nope"} {
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
			resp, _ := get(t, h, method, path)
			if c := resp.Header.Values("Set-Cookie"); len(c) != 0 {
				t.Errorf("%s %s set cookies %q", method, path, c)
			}
		}
	}
}

func TestMethods(t *testing.T) {
	withFS(t, builtSite)
	h := Handler()
	resp, body := get(t, h, http.MethodHead, "/privacy")
	if resp.StatusCode != http.StatusOK || body != "" {
		t.Errorf("HEAD /privacy = %d %q, want 200 with no body", resp.StatusCode, body)
	}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		resp, _ := get(t, h, method, "/")
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s / = %d, want 405", method, resp.StatusCode)
		}
		if got := resp.Header.Get("Allow"); got != "GET, HEAD" {
			t.Errorf("%s / Allow = %q", method, got)
		}
	}
}
