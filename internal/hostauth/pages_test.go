package hostauth

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// renderedPages renders each server-rendered page: the sign-in page and
// the problem, busy and signed-out pages.
func renderedPages(t *testing.T) map[string]*httptest.ResponseRecorder {
	t.Helper()
	pages := map[string]*httptest.ResponseRecorder{}
	rec := httptest.NewRecorder()
	renderSignin(rec, http.StatusOK, signinData{State: "STATE"})
	pages["sign-in"] = rec
	rec = httptest.NewRecorder()
	errorPage(rec, http.StatusBadRequest, "Please start signing in again.")
	pages["problem"] = rec
	rec = httptest.NewRecorder()
	busyPage(rec, 3)
	pages["busy"] = rec
	rec = httptest.NewRecorder()
	serveSignedOut(rec, httptest.NewRequest("GET", "/auth/signed-out", nil))
	pages["signed out"] = rec
	return pages
}

// TestPagesUseTheStudioLook shows every server-rendered page carries the
// TCG Replay mark, the shared design tokens in the light interface, and a
// phone-width viewport with a column that fits a phone.
func TestPagesUseTheStudioLook(t *testing.T) {
	for name, rec := range renderedPages(t) {
		body := rec.Body.String()
		for _, want := range []string{
			"<title>TCG Replay</title>", // the mark's own title
			`viewBox="0 0 30 30"`,       // the mark's drawing
			"--primary: #13285C;",       // the tokens, light values on :root
			"background: var(--bg)",     // the page style uses tokens
			`name="viewport" content="width=device-width, initial-scale=1"`,
			"max-width: 24rem",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("%s page lacks %q", name, want)
			}
		}
		if regexp.MustCompile(`<html[^>]*data-theme`).MatchString(body) {
			t.Errorf("%s page sets an interface theme; the pages are light only", name)
		}
	}
}

// TestGoogleButtonComesFirst shows the Google button is the sign-in page's
// first action, labelled "Sign in with Google" with Google's mark.
func TestGoogleButtonComesFirst(t *testing.T) {
	body := renderedPages(t)["sign-in"].Body.String()
	first := regexp.MustCompile(`<(a|button|input|form)\b[^>]*>`).FindString(body)
	if !strings.Contains(first, `data-provider="google"`) {
		t.Fatalf("first action on the sign-in page is %q, want the Google button", first)
	}
	button := regexp.MustCompile(`(?s)<a class="provider" data-provider="google"[^>]*>(.*?)</a>`).FindStringSubmatch(body)
	if button == nil {
		t.Fatal("no Google button")
	}
	if !strings.Contains(button[1], "Sign in with Google") {
		t.Errorf("Google button label %q, want Sign in with Google", button[1])
	}
	for _, colour := range []string{"#EA4335", "#4285F4", "#FBBC05", "#34A853"} {
		if !strings.Contains(button[1], colour) {
			t.Errorf("Google button lacks Google's mark (colour %s)", colour)
		}
	}
}

// TestPagesLoadNothingFromOtherSites shows no page asks for a font,
// script, stylesheet or image from another site, and every page tells
// browsers not to frame it, cache it or load anything from elsewhere.
func TestPagesLoadNothingFromOtherSites(t *testing.T) {
	outside := regexp.MustCompile(`(?i)(?:\bsrc|\bhref|@import|url\()\s*=?\s*\(?["']?(?:https?:)?//`)
	for name, rec := range renderedPages(t) {
		body := rec.Body.String()
		if m := outside.FindString(body); m != "" {
			t.Errorf("%s page loads from another site: %q", name, m)
		}
		for _, tag := range []string{"<script", "<link", "<img", "<iframe"} {
			if strings.Contains(strings.ToLower(body), tag) {
				t.Errorf("%s page has a %s> element", name, tag)
			}
		}
		h := rec.Header()
		if got := h.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s page: Cache-Control %q, want no-store", name, got)
		}
		if got := h.Get("X-Frame-Options"); got != "DENY" {
			t.Errorf("%s page: X-Frame-Options %q, want DENY", name, got)
		}
		csp := h.Get("Content-Security-Policy")
		for _, want := range []string{"default-src 'none'", "frame-ancestors 'none'", "form-action 'self'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%s page: Content-Security-Policy %q lacks %q", name, csp, want)
			}
		}
	}
}
