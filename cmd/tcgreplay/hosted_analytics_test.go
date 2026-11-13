package main

import (
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/webanalytics"
	"github.com/nicholasjackson/tcgreplay/internal/webui"
)

// analyticsToken is a well-formed Cloudflare site token for the tests.
const analyticsToken = "0123456789abcdef0123456789abcdef"

// sitePaths are the website pages the analytics tests load.
var sitePaths = []string{"/", "/docs/connect", "/privacy", "/terms"}

// withPrivacy sets what the privacy-page check answers for the test: the
// Go tests embed whatever website happens to be built.
func withPrivacy(t *testing.T, describes bool) {
	t.Helper()
	prev := privacyDescribesCounting
	privacyDescribesCounting = func() bool { return describes }
	t.Cleanup(func() { privacyDescribesCounting = prev })
}

// analyticsLog captures the standard logger for the rest of the test.
func analyticsLog(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return buf
}

// startCounting starts hosted mode with the token set to tok.
func startCounting(t *testing.T, tok string) *hostedServer {
	t.Helper()
	return startHostedWith(t, testPublic, func(c *config) { c.webAnalyticsToken = tok })
}

// asPublic sends the request for the public host (testPublic's), as the
// proxy in front of the live playmat does.
func asPublic(r *http.Request) {
	r.Host = "playmat.example"
	r.Header.Set("Sec-Fetch-Mode", "navigate")
}

// TestHostedAnalyticsCountsPublicPages: with a token and a privacy page
// that describes counting, every website page and the signed-in board page
// carry exactly one beacon for the public host, and none for another host.
func TestHostedAnalyticsCountsPublicPages(t *testing.T) {
	withPrivacy(t, true)
	logs := analyticsLog(t)
	h := startCounting(t, analyticsToken)
	snippet := webanalytics.Snippet(analyticsToken)

	if out := logs.String(); !strings.Contains(out, "visitor analytics: on for playmat.example") {
		t.Errorf("start-up log does not say counting is on:\n%s", out)
	}
	for _, path := range sitePaths {
		code, page := siteResponse(t, path)
		resp, body := getWith(t, h.ts.URL+path, nil, asPublic)
		if resp.StatusCode != code {
			t.Errorf("GET %s: %d, want the website's %d", path, resp.StatusCode, code)
		}
		if code == http.StatusOK {
			// A built page carries one beacon, and is otherwise the website's.
			if n := strings.Count(body, snippet); n != 1 {
				t.Errorf("GET %s for the public host: %d beacons, want 1:\n%s", path, n, body)
			}
			if strings.Replace(body, snippet, "", 1) != page {
				t.Errorf("GET %s: page differs from the website's beyond the beacon", path)
			}
		} else if body != page {
			// The website's 404 (an unbuilt site has only the home page).
			t.Errorf("GET %s: %d %q, want the website's own %q", path, resp.StatusCode, body, page)
		}
		if c := resp.Header.Values("Set-Cookie"); len(c) != 0 {
			t.Errorf("GET %s set cookies %q", path, c)
		}

		// Any other host (the App Platform address, a local run) is not counted.
		resp, body = getWith(t, h.ts.URL+path, nil, func(r *http.Request) { r.Header.Set("Sec-Fetch-Mode", "navigate") })
		if resp.StatusCode != code || body != page {
			t.Errorf("GET %s for another host: %d, changed from the website's page", path, resp.StatusCode)
		}
	}
	if code, page := siteResponse(t, "/"); code != http.StatusOK || page == "" {
		t.Fatalf("the website's home page: %d", code)
	}

	// An unknown path is the website's 404, never counted.
	_, want404 := siteResponse(t, "/no-such-page")
	if resp, body := getWith(t, h.ts.URL+"/no-such-page", nil, asPublic); resp.StatusCode != http.StatusNotFound || body != want404 {
		t.Errorf("GET /no-such-page: %d, want the website's own 404", resp.StatusCode)
	}

	// Signed out, /board goes straight to sign-in, with no beacon.
	resp, body := getWith(t, h.ts.URL+"/board", nil, asPublic)
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "/auth/login") {
		t.Errorf("GET /board signed out: %d to %q, want sign-in", resp.StatusCode, resp.Header.Get("Location"))
	}
	if strings.Contains(body, "cloudflareinsights") {
		t.Errorf("signed-out /board redirect carries the beacon: %q", body)
	}

	// Signed in, the board page carries one beacon per load.
	cookie := h.cookie(t)
	for range 2 {
		resp, body = getWith(t, h.ts.URL+"/board", cookie, asPublic)
		if resp.StatusCode != http.StatusOK || !strings.Contains(body, "<title>tcgreplay</title>") {
			t.Fatalf("GET /board signed in: %d %q", resp.StatusCode, body)
		}
		if n := strings.Count(body, snippet); n != 1 {
			t.Errorf("GET /board signed in: %d beacons, want 1", n)
		}
		if c := resp.Header.Values("Set-Cookie"); len(c) != 0 {
			t.Errorf("GET /board set cookies %q", c)
		}
	}
	if _, body := getWith(t, h.ts.URL+"/board", cookie, func(r *http.Request) { r.Header.Set("Sec-Fetch-Mode", "navigate") }); strings.Contains(body, "cloudflareinsights") {
		t.Error("GET /board for another host carries the beacon")
	}

	// Another account's board link gets "Not your game", never counted.
	if resp, body := getWith(t, h.ts.URL+"/board?board=someone-else", cookie, asPublic); resp.StatusCode != http.StatusForbidden || strings.Contains(body, "cloudflareinsights") {
		t.Errorf("GET /board?board=someone-else: %d, beacon %v; want 403 with no beacon", resp.StatusCode, strings.Contains(body, "cloudflareinsights"))
	}

	// Nothing else the playmat serves is ever counted, signed in or out.
	for _, tc := range []struct {
		path   string
		cookie *http.Cookie
	}{
		{"/assets/app.js", cookie},
		{"/api/card?def=x", cookie},
		{"/ws", nil},
		{"/mcp", nil},
		{"/.well-known/oauth-protected-resource/mcp", nil},
		{"/.well-known/oauth-authorization-server", nil},
		{"/auth/login", nil},
		{"/healthz", nil},
		{"/admin", nil},
		{"/admin", cookie}, // the signed-in account is not an admin
	} {
		resp, body := getWith(t, h.ts.URL+tc.path, tc.cookie, asPublic)
		if strings.Contains(body, "cloudflareinsights") {
			t.Errorf("GET %s (signed in %v): %d carries the beacon", tc.path, tc.cookie != nil, resp.StatusCode)
		}
	}
	if resp, body := sendWith(t, http.MethodHead, h.ts.URL+"/", nil, asPublic); resp.StatusCode != http.StatusOK || body != "" {
		t.Errorf("HEAD /: %d %q", resp.StatusCode, body)
	}

	if out := logs.String(); strings.Contains(out, analyticsToken) {
		t.Errorf("log shows the token:\n%s", out)
	}
}

// TestHostedAnalyticsOff: with no token, a malformed token or a privacy
// page that does not describe counting, every page is exactly what the
// website or the board serves, and the start-up log names the reason
// without the token.
func TestHostedAnalyticsOff(t *testing.T) {
	for _, tc := range []struct {
		name, token string
		privacy     bool
		status      string
	}{
		{"no token", "", true, "visitor analytics: off (no TCGREPLAY_WEB_ANALYTICS_TOKEN)"},
		{"malformed token", "bad token", true, "warning: visitor analytics: off (TCGREPLAY_WEB_ANALYTICS_TOKEN is not a Cloudflare site token: 16 to 64 letters and digits)"},
		{"privacy page silent", analyticsToken, false, "visitor analytics: off (the privacy page does not describe visitor counting yet)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withPrivacy(t, tc.privacy)
			logs := analyticsLog(t)
			h := startCounting(t, tc.token)
			for _, path := range append(sitePaths, "/no-such-page") {
				code, page := siteResponse(t, path)
				resp, body := getWith(t, h.ts.URL+path, nil, asPublic)
				if resp.StatusCode != code || body != page {
					t.Errorf("GET %s: %d, not byte for byte the website's page", path, resp.StatusCode)
				}
			}
			cookie := h.cookie(t)
			resp, body := getWith(t, h.ts.URL+"/board", cookie, asPublic)
			if resp.StatusCode != http.StatusOK || strings.Contains(body, "cloudflareinsights") {
				t.Errorf("GET /board signed in: %d, beacon %v", resp.StatusCode, strings.Contains(body, "cloudflareinsights"))
			}
			out := logs.String()
			if !strings.Contains(out, tc.status) {
				t.Errorf("start-up log does not say %q:\n%s", tc.status, out)
			}
			if tc.token != "" && strings.Contains(out, tc.token) {
				t.Errorf("log shows the token:\n%s", out)
			}
		})
	}
}

// TestDesktopBoardNeverCounts: the desktop app's board never carries the
// beacon, whatever the environment says.
func TestDesktopBoardNeverCounts(t *testing.T) {
	t.Setenv("TCGREPLAY_WEB_ANALYTICS_TOKEN", analyticsToken)
	withPrivacy(t, true)
	m, _ := newServer(config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true}, "", nil)
	ts := httptest.NewServer(webui.New(m, state.P1).Handler())
	defer ts.Close()
	resp, body := getWith(t, ts.URL+"/", nil, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "<title>tcgreplay</title>") {
		t.Fatalf("GET / on the desktop board: %d %q", resp.StatusCode, body)
	}
	if strings.Contains(body, "cloudflareinsights") {
		t.Error("the desktop board carries the beacon")
	}
}
