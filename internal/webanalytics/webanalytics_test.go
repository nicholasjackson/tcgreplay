package webanalytics

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

const token = "0123456789abcdef0123456789abcdef"

// wantSnippet is Cloudflare's manual tag for token, written out by hand.
const wantSnippet = `<script defer src="https://static.cloudflareinsights.com/beacon.min.js" data-cf-beacon='{"token":"0123456789abcdef0123456789abcdef","spa":false}'></script>`

func TestSnippetIsCloudflaresManualTag(t *testing.T) {
	if got := Snippet(token); got != wantSnippet {
		t.Fatalf("Snippet = %s\nwant      %s", got, wantSnippet)
	}
}

func TestSetupDecides(t *testing.T) {
	for _, tc := range []struct {
		name   string
		s      Settings
		on     bool
		status string
	}{
		{"on", Settings{Token: token, Host: "tcgreplay.app", PrivacyDescribes: true}, true,
			"visitor analytics: on for tcgreplay.app"},
		{"token trimmed, host normalised", Settings{Token: "  " + token + "\n", Host: "TCGReplay.App:443", PrivacyDescribes: true}, true,
			"visitor analytics: on for tcgreplay.app"},
		{"no token", Settings{Host: "tcgreplay.app", PrivacyDescribes: true}, false,
			"visitor analytics: off (no TCGREPLAY_WEB_ANALYTICS_TOKEN)"},
		{"blank token", Settings{Token: "   ", Host: "tcgreplay.app", PrivacyDescribes: true}, false,
			"visitor analytics: off (no TCGREPLAY_WEB_ANALYTICS_TOKEN)"},
		{"bad token", Settings{Token: "not a token!", Host: "tcgreplay.app", PrivacyDescribes: true}, false,
			"warning: visitor analytics: off (TCGREPLAY_WEB_ANALYTICS_TOKEN is not a Cloudflare site token: 16 to 64 letters and digits)"},
		{"no host", Settings{Token: token, PrivacyDescribes: true}, false,
			"visitor analytics: off (no public host)"},
		{"privacy page silent", Settings{Token: token, Host: "tcgreplay.app"}, false,
			"visitor analytics: off (the privacy page does not describe visitor counting yet)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wrap, status := Setup(tc.s)
			if status != tc.status {
				t.Errorf("status = %q, want %q", status, tc.status)
			}
			if strings.Contains(status, token) || (strings.TrimSpace(tc.s.Token) != "" && strings.Contains(status, strings.TrimSpace(tc.s.Token))) {
				t.Errorf("status %q shows the token", status)
			}
			_, body := serve(t, wrap(htmlPage), http.MethodGet, "tcgreplay.app", "/")
			if got := strings.Count(body, "cloudflareinsights"); tc.on && got != 1 || !tc.on && got != 0 {
				t.Errorf("page carries %d beacons, want on=%v:\n%s", got, tc.on, body)
			}
		})
	}
}

func TestSetupRefusesMalformedTokens(t *testing.T) {
	for _, bad := range []string{
		`0123456789abcdef"`,
		`0123456789abcdef<`,
		`0123456789abcdef'`,
		`01234567 89abcdef`,
		`0123456789abcde`, // 15
		strings.Repeat("a", 65),
		`0123456789abcdefé`,
		`0123456789abcdef-_`,
	} {
		wrap, status := Setup(Settings{Token: bad, Host: "tcgreplay.app", PrivacyDescribes: true})
		if !strings.HasPrefix(status, "warning: visitor analytics: off") {
			t.Errorf("token %q: status %q, want a warning that counting is off", bad, status)
		}
		if strings.Contains(status, bad) {
			t.Errorf("token %q: status shows it: %q", bad, status)
		}
		if _, body := serve(t, wrap(htmlPage), http.MethodGet, "tcgreplay.app", "/"); body != page {
			t.Errorf("token %q: page changed: %q", bad, body)
		}
	}
	for _, good := range []string{strings.Repeat("a", 16), strings.Repeat("Z9", 32)} {
		if _, status := Setup(Settings{Token: good, Host: "h", PrivacyDescribes: true}); status != "visitor analytics: on for h" {
			t.Errorf("token %q refused: %q", good, status)
		}
	}
}

const page = "<!doctype html><html><body><h1>hi</h1></body></html>"

var htmlPage = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(page)))
	w.Write([]byte(page))
})

func serve(t *testing.T, h http.Handler, method, host, path string) (*http.Response, string) {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	r.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	resp := rec.Result()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestInjectAddsOneSnippetBeforeTheLastBodyTag(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"body tag", page, "<!doctype html><html><body><h1>hi</h1>" + wantSnippet + "</body></html>"},
		{"upper case", "<HTML><BODY>x</BODY></HTML>", "<HTML><BODY>x" + wantSnippet + "</BODY></HTML>"},
		{"no body tag", "<p>bare", "<p>bare" + wantSnippet},
		{"two body tags", "<body>a</body><pre></body></pre></body>", "<body>a</body><pre></body></pre>" + wantSnippet + "</body>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := Inject(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.Header().Set("Content-Length", strconv.Itoa(len(tc.in)))
				w.Write([]byte(tc.in[:len(tc.in)/2]))
				w.Write([]byte(tc.in[len(tc.in)/2:]))
			}), wantSnippet, "tcgreplay.app")
			resp, body := serve(t, h, http.MethodGet, "tcgreplay.app", "/")
			if resp.StatusCode != http.StatusOK || body != tc.want {
				t.Fatalf("got %d %q\nwant 200 %q", resp.StatusCode, body, tc.want)
			}
			if cl := resp.Header.Get("Content-Length"); cl != strconv.Itoa(len(tc.want)) {
				t.Errorf("Content-Length = %q, want %d", cl, len(tc.want))
			}
		})
	}
}

func TestInjectMatchesHostIgnoringCaseAndPort(t *testing.T) {
	h := Inject(htmlPage, wantSnippet, "tcgreplay.app")
	for _, host := range []string{"tcgreplay.app", "TCGReplay.APP", "tcgreplay.app:8080", "TCGREPLAY.APP:443"} {
		if _, body := serve(t, h, http.MethodGet, host, "/"); strings.Count(body, wantSnippet) != 1 {
			t.Errorf("host %q: want one snippet, got %q", host, body)
		}
	}
	for _, host := range []string{"tcgreplay.demo.gs", "playmat-abc.ondigitalocean.app", "127.0.0.1:7792", "localhost", "evil-tcgreplay.app", "tcgreplay.app.example"} {
		if _, body := serve(t, h, http.MethodGet, host, "/"); body != page {
			t.Errorf("host %q: page changed: %q", host, body)
		}
	}
}

// Every response that is not a successful GET HTML page passes through byte
// for byte, headers included.
func TestInjectPassesEverythingElseThrough(t *testing.T) {
	served := func(name string, body []byte) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
		})
	}
	for _, tc := range []struct {
		name   string
		method string
		header http.Header
		h      http.Handler
	}{
		{"HEAD", http.MethodHead, nil, htmlPage},
		{"POST", http.MethodPost, nil, htmlPage},
		{"redirect", http.MethodGet, nil, http.RedirectHandler("https://accounts.example/signin", http.StatusFound)},
		{"404 html", http.MethodGet, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("<html><body>Page not found</body></html>"))
		})},
		{"503", http.MethodGet, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("<html><body>full</body></html>"))
		})},
		{"206 range", http.MethodGet, http.Header{"Range": {"bytes=0-9"}}, served("index.html", []byte(page))},
		{"css", http.MethodGet, nil, served("site.css", []byte("body{color:red}</body>"))},
		{"json", http.MethodGet, nil, served("cards.json", []byte(`{"a":"</body>"}`))},
		{"gzip html", http.MethodGet, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Content-Encoding", "gzip")
			w.Write([]byte("\x1f\x8bnot really</body>"))
		})},
		{"flushed", http.MethodGet, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html><body>part one "))
			w.(http.Flusher).Flush()
			w.Write([]byte("part two</body></html>"))
		})},
		{"flushed through ResponseController", http.MethodGet, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html><body>part one "))
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Errorf("Flush: %v", err)
			}
			w.Write([]byte("part two</body></html>"))
		})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			do := func(h http.Handler) (*http.Response, string) {
				r := httptest.NewRequest(tc.method, "/", nil)
				r.Host = "tcgreplay.app"
				for k, v := range tc.header {
					r.Header[k] = v
				}
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, r)
				resp := rec.Result()
				b, _ := io.ReadAll(resp.Body)
				return resp, string(b)
			}
			want, wantBody := do(tc.h)
			got, gotBody := do(Inject(tc.h, wantSnippet, "tcgreplay.app"))
			if got.StatusCode != want.StatusCode || gotBody != wantBody {
				t.Errorf("got %d %q, want %d %q", got.StatusCode, gotBody, want.StatusCode, wantBody)
			}
			if strings.Contains(gotBody, "cloudflareinsights") {
				t.Errorf("snippet added: %q", gotBody)
			}
			if g, w := headerString(got.Header), headerString(want.Header); g != w {
				t.Errorf("headers changed:\n got %s\nwant %s", g, w)
			}
		})
	}
}

func headerString(h http.Header) string {
	var b strings.Builder
	h.Write(&b)
	return b.String()
}

// A 200 HTML page served by http.ServeContent (as the website and the board
// page are) gets the snippet and a matching length.
func TestInjectIntoServeContentPage(t *testing.T) {
	h := Inject(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", time.Time{}, strings.NewReader(page))
	}), wantSnippet, "tcgreplay.app")
	ts := httptest.NewServer(h)
	defer ts.Close()
	req, _ := http.NewRequest(http.MethodGet, ts.URL, nil)
	req.Host = "tcgreplay.app"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	want := "<!doctype html><html><body><h1>hi</h1>" + wantSnippet + "</body></html>"
	if string(body) != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
	if resp.ContentLength != int64(len(want)) {
		t.Errorf("Content-Length = %d, want %d", resp.ContentLength, len(want))
	}
	if resp.Header.Get("Accept-Ranges") != "" {
		t.Errorf("Accept-Ranges = %q on an altered page", resp.Header.Get("Accept-Ranges"))
	}
	if len(resp.Cookies()) != 0 {
		t.Errorf("cookies set: %v", resp.Cookies())
	}
}
