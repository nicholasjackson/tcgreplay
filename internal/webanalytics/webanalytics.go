// Package webanalytics is the playmat's one Cloudflare Web Analytics
// integration: the beacon tag, the token check, the decision whether
// visitor counting is on, and an HTTP wrapper that adds the tag to a page.
//
// Only hosted mode uses it, on the website and the board page, and the
// website and board test servers use it when a test asks for it. Nothing
// else does, so local runs, desktop mode and the default test servers never
// count. Counting is cookieless: the tag carries only the site token and
// turns Cloudflare's single-page-app tracking off, so a board session is
// one page view however much the player moves around it.
package webanalytics

import (
	"bytes"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// BeaconURL is the address of Cloudflare's beacon script.
const BeaconURL = "https://static.cloudflareinsights.com/beacon.min.js"

// tokenPattern is what a Cloudflare site token looks like. Checking it also
// guarantees nothing written into the page can break out of the attribute.
var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9]{16,64}$`)

// validToken reports whether s can be a Cloudflare site token: 16 to 64
// ASCII letters and digits.
func validToken(s string) bool { return tokenPattern.MatchString(s) }

// Snippet is Cloudflare's manual beacon tag for token, with single-page-app
// tracking off ("spa": false, which only the manual tag allows). Cloudflare
// places it before the closing body tag. token must have passed validToken.
func Snippet(token string) string {
	return `<script defer src="` + BeaconURL + `" data-cf-beacon='{"token":"` + token + `","spa":false}'></script>`
}

// Settings are what decide whether counting is on.
type Settings struct {
	Token            string // the Cloudflare site token; "" = off
	Host             string // the public host the tag is added for (from the public URL)
	PrivacyDescribes bool   // the embedded privacy page describes visitor counting
}

// identity is the wrapper used when counting is off.
func identity(h http.Handler) http.Handler { return h }

// Setup returns the wrapper to apply to counted pages (the identity when
// counting is off) and a one-line status for the start-up log, such as
// "visitor analytics: on for tcgreplay.app" or
// "visitor analytics: off (no TCGREPLAY_WEB_ANALYTICS_TOKEN)". The status
// never contains the token.
func Setup(s Settings) (wrap func(http.Handler) http.Handler, status string) {
	tok := strings.TrimSpace(s.Token)
	host := normalHost(s.Host)
	switch {
	case tok == "":
		return identity, "visitor analytics: off (no TCGREPLAY_WEB_ANALYTICS_TOKEN)"
	case !validToken(tok):
		return identity, "warning: visitor analytics: off (TCGREPLAY_WEB_ANALYTICS_TOKEN is not a Cloudflare site token: 16 to 64 letters and digits)"
	case host == "":
		return identity, "visitor analytics: off (no public host)"
	case !s.PrivacyDescribes:
		return identity, "visitor analytics: off (the privacy page does not describe visitor counting yet)"
	}
	snippet := Snippet(tok)
	return func(next http.Handler) http.Handler { return Inject(next, snippet, host) },
		"visitor analytics: on for " + host
}

// normalHost lower-cases a host and drops any port.
func normalHost(h string) string {
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	return strings.ToLower(strings.TrimSpace(h))
}

// Inject adds snippet before the last closing body tag of a successful GET
// HTML page that next serves for host (compared ignoring case and port), or
// at the end of the page when it has no body tag, and fixes the length.
// Everything else passes through byte for byte: other hosts, other methods
// (HEAD included), any status but 200 (redirects, errors, range responses),
// anything not HTML, and a page that is flushed while it is written.
func Inject(next http.Handler, snippet, host string) http.Handler {
	host = normalHost(host)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || normalHost(r.Host) != host {
			next.ServeHTTP(w, r)
			return
		}
		iw := &injector{w: w, snippet: snippet}
		next.ServeHTTP(iw, r)
		iw.finish()
	})
}

// injector states.
const (
	undecided = iota
	capturing // a 200 HTML page, buffered until the handler returns
	streaming // anything else, written straight through
)

// injector is the response writer Inject hands to the page handler.
type injector struct {
	w       http.ResponseWriter
	snippet string
	state   int
	buf     bytes.Buffer
}

func (iw *injector) Header() http.Header { return iw.w.Header() }

func (iw *injector) WriteHeader(code int) {
	if iw.state != undecided {
		if iw.state == streaming {
			iw.w.WriteHeader(code) // let net/http report the superfluous call
		}
		return
	}
	if code == http.StatusOK && isPlainHTML(iw.w.Header()) {
		iw.state = capturing
		return
	}
	iw.state = streaming
	iw.w.WriteHeader(code)
}

func (iw *injector) Write(p []byte) (int, error) {
	if iw.state == undecided {
		iw.WriteHeader(http.StatusOK)
	}
	if iw.state == capturing {
		return iw.buf.Write(p)
	}
	return iw.w.Write(p)
}

// Flush stops capturing: what is buffered goes out unchanged and the rest
// streams, so a page that streams is never altered.
func (iw *injector) Flush() {
	if iw.state == undecided {
		iw.WriteHeader(http.StatusOK)
	}
	if iw.state == capturing {
		iw.state = streaming
		iw.w.WriteHeader(http.StatusOK)
		iw.w.Write(iw.buf.Bytes())
		iw.buf.Reset()
	}
	if f, ok := iw.w.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (iw *injector) Unwrap() http.ResponseWriter { return iw.w }

// finish writes a captured page with the snippet added.
func (iw *injector) finish() {
	if iw.state != capturing {
		return
	}
	body := insertSnippet(iw.buf.Bytes(), iw.snippet)
	h := iw.w.Header()
	h.Del("Content-Length")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	// The page is no longer the file a range would be cut from.
	h.Del("Accept-Ranges")
	iw.w.WriteHeader(http.StatusOK)
	iw.w.Write(body)
}

// isPlainHTML reports whether h describes an HTML body with no content
// encoding, the only kind the snippet can be added to.
func isPlainHTML(h http.Header) bool {
	if ce := h.Get("Content-Encoding"); ce != "" && !strings.EqualFold(ce, "identity") {
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(h.Get("Content-Type"))), "text/html")
}

// insertSnippet puts snippet before the last closing body tag (any case),
// or at the end when there is none.
func insertSnippet(body []byte, snippet string) []byte {
	i := bytes.LastIndex(bytes.ToLower(body), []byte("</body>"))
	if i < 0 {
		i = len(body)
	}
	out := make([]byte, 0, len(body)+len(snippet))
	out = append(out, body[:i]...)
	out = append(out, snippet...)
	return append(out, body[i:]...)
}
