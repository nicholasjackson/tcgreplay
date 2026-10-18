package hostauth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

// boardNext is the board page every test sign-in asks to return to.
const boardNext = "/board?x=1"

// harness is a sign-in service served by a test server, with a fake Google
// behind it and a test-only route that starts a board sign-in.
type harness struct {
	ts  *httptest.Server
	a   *Auth
	g   *hostauthtest.FakeGoogle
	cfg Config
}

// newHarness serves an Auth reached at the test server's own address.
// mod, if set, may change the config before the Auth is built.
func newHarness(t *testing.T, mod func(*Config)) *harness {
	t.Helper()
	g := hostauthtest.NewGoogle(t)
	mux := http.NewServeMux()
	ts := httptest.NewUnstartedServer(mux)
	ts.Start()
	t.Cleanup(ts.Close)
	pub, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{
		Public:         pub,
		GoogleClientID: g.ClientID,
		GoogleSecret:   g.ClientSecret,
		Google:         g.Endpoint(),
	}
	if mod != nil {
		mod(&cfg)
	}
	a, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a.Routes(mux)
	mux.HandleFunc("GET /start", func(w http.ResponseWriter, r *http.Request) {
		a.startGoogle(w, r, &pendingLogin{kind: loginBoard, next: boardNext})
	})
	mux.HandleFunc("GET /board", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "OK")
	})
	return &harness{ts: ts, a: a, g: g, cfg: cfg}
}

// browser returns a fresh browser for the harness's playmat.
func (h *harness) browser(t *testing.T) *http.Client {
	return hostauthtest.Browser(t, h.ts.URL, h.ts.URL)
}

// begin starts a board sign-in in c and returns the Google address the
// playmat sent it to, without following it.
func (h *harness) begin(t *testing.T, c *http.Client) *url.URL {
	t.Helper()
	follow := c.CheckRedirect
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer func() { c.CheckRedirect = follow }()
	resp, err := c.Get(h.ts.URL + "/start")
	if err != nil {
		t.Fatalf("GET /start: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("GET /start: status %d, want 302", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("bad Location %q: %v", resp.Header.Get("Location"), err)
	}
	return loc
}

// get fetches u in c, following redirects as a browser would, and returns
// the final response with its body read.
func get(t *testing.T, c *http.Client, u string) (*http.Response, string) {
	t.Helper()
	resp, err := c.Get(u)
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(b)
}

// sessionCookieIn returns the board session cookie value c holds for the
// playmat, or "".
func (h *harness) sessionCookieIn(c *http.Client) string {
	u, _ := url.Parse(h.ts.URL)
	for _, ck := range c.Jar.Cookies(u) {
		if ck.Name == sessionCookie {
			return ck.Value
		}
	}
	return ""
}

// signInOK runs a whole board sign-in in c and checks it ends on the board
// page; it returns the session cookie value.
func (h *harness) signInOK(t *testing.T, c *http.Client) string {
	t.Helper()
	resp, body := get(t, c, h.begin(t, c).String())
	if resp.StatusCode != http.StatusOK || body != "OK" {
		t.Fatalf("sign-in ended with status %d body %q, want the board page", resp.StatusCode, body)
	}
	if got := resp.Request.URL.RequestURI(); got != boardNext {
		t.Fatalf("sign-in ended at %s, want %s", got, boardNext)
	}
	id := h.sessionCookieIn(c)
	if id == "" {
		t.Fatal("sign-in set no session cookie")
	}
	return id
}

// checkRefused checks resp is a sign-in problem page and c holds no
// session.
func (h *harness) checkRefused(t *testing.T, c *http.Client, resp *http.Response, body string) {
	t.Helper()
	if resp.StatusCode < 400 {
		t.Errorf("status %d, want a refusal", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type %q, want an HTML page", ct)
	}
	if !strings.Contains(body, "Sign-in problem") {
		t.Errorf("body does not explain the problem: %q", body)
	}
	if id := h.sessionCookieIn(c); id != "" {
		t.Errorf("refused sign-in set a session cookie")
	}
}

// TestGoogleSignInSurvivesSecondStart shows a sign-in still completes when
// the same browser starts another before coming back from Google, as a
// second tab or a signed-out request for a page does.
func TestGoogleSignInSurvivesSecondStart(t *testing.T) {
	h := newHarness(t, nil)
	c := h.browser(t)
	first := h.begin(t, c)
	h.begin(t, c)

	resp, body := get(t, c, first.String())
	if resp.StatusCode != http.StatusOK || body != "OK" || resp.Request.URL.RequestURI() != boardNext {
		t.Fatalf("first sign-in ended at %s with status %d body %q, want the board page %s", resp.Request.URL, resp.StatusCode, body, boardNext)
	}
}

// TestGoogleSignIn shows a browser sent to sign in reaches Google's account
// chooser with a one-time state, nonce and PKCE challenge, that coming back
// with a valid Google answer signs the account in and returns to the
// board page asked for, and that the same state cannot be used again.
func TestGoogleSignIn(t *testing.T) {
	h := newHarness(t, nil)
	c := h.browser(t)
	loc := h.begin(t, c)

	if got, want := loc.Scheme+"://"+loc.Host+loc.Path, h.g.URL+"/authorize"; got != want {
		t.Fatalf("sent to %s, want Google's %s", got, want)
	}
	q := loc.Query()
	for _, p := range []string{"state", "nonce", "code_challenge"} {
		if q.Get(p) == "" {
			t.Errorf("Google address has no %s", p)
		}
	}
	checks := map[string]string{
		"code_challenge_method": "S256",
		"prompt":                "select_account",
		"redirect_uri":          h.ts.URL + "/auth/google/callback",
		"client_id":             h.g.ClientID,
		"response_type":         "code",
	}
	for p, want := range checks {
		if got := q.Get(p); got != want {
			t.Errorf("%s = %q, want %q", p, got, want)
		}
	}
	scopes := strings.Fields(q.Get("scope"))
	for _, want := range []string{"openid", "email"} {
		found := false
		for _, s := range scopes {
			found = found || s == want
		}
		if !found {
			t.Errorf("scope %q lacks %q", q.Get("scope"), want)
		}
	}
	state := q.Get("state")

	resp, body := get(t, c, loc.String())
	if resp.StatusCode != http.StatusOK || body != "OK" || resp.Request.URL.RequestURI() != boardNext {
		t.Fatalf("sign-in ended at %s with status %d body %q, want the board page %s", resp.Request.URL, resp.StatusCode, body, boardNext)
	}
	id := h.sessionCookieIn(c)
	if id == "" {
		t.Fatal("sign-in set no session cookie")
	}
	s := h.a.st.session(id)
	if s == nil {
		t.Fatal("session cookie names no session")
	}
	if s.account != (Account(hostauthtest.DefaultAccount)) {
		t.Errorf("session is for %+v, want %+v", s.account, hostauthtest.DefaultAccount)
	}
	if got := h.g.Prompts(); len(got) != 1 || got[0] != "select_account" {
		t.Errorf("Google saw prompts %q, want one select_account", got)
	}

	// The same state again, with its login cookie, is refused.
	c2 := h.browser(t)
	req, _ := http.NewRequest("GET", h.ts.URL+"/auth/google/callback?code=x&state="+url.QueryEscape(state), nil)
	req.AddCookie(&http.Cookie{Name: loginCookieName(state), Value: state})
	resp2, err := c2.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	h.checkRefused(t, c2, resp2, string(b))
}

// TestGoogleCallbackRefused shows a callback is refused with a clear page,
// and no session, when its state is not this browser's sign-in in progress
// or Google's ID token has the wrong issuer, audience or nonce, or has
// expired, or Google reports the sign-in was not completed.
func TestGoogleCallbackRefused(t *testing.T) {
	// callback runs the case and returns the browser and the response the
	// callback ended with.
	type callback func(t *testing.T, h *harness) (*http.Client, *http.Response, string)

	followIn := func(t *testing.T, h *harness) (*http.Client, *http.Response, string) {
		c := h.browser(t)
		resp, body := get(t, c, h.begin(t, c).String())
		return c, resp, body
	}
	claims := func(f func(map[string]any)) func(*harness) {
		return func(h *harness) { h.g.Claims = f }
	}

	tests := []struct {
		name  string
		cfg   func(*Config)
		setup func(*harness)
		run   callback
	}{
		{
			name: "no login cookie",
			run: func(t *testing.T, h *harness) (*http.Client, *http.Response, string) {
				loc := h.begin(t, h.browser(t))
				other := h.browser(t)
				resp, body := get(t, other, loc.String())
				return other, resp, body
			},
		},
		{
			name: "login cookie is another sign-in's",
			run: func(t *testing.T, h *harness) (*http.Client, *http.Response, string) {
				// This browser started its own sign-in, but comes back
				// with another browser's.
				c := h.browser(t)
				h.begin(t, c)
				theirs := h.begin(t, h.browser(t))
				resp, body := get(t, c, theirs.String())
				return c, resp, body
			},
		},
		{
			name: "no state",
			run: func(t *testing.T, h *harness) (*http.Client, *http.Response, string) {
				c := h.browser(t)
				h.begin(t, c)
				resp, body := get(t, c, h.ts.URL+"/auth/google/callback?code=x")
				return c, resp, body
			},
		},
		{
			name: "state already used",
			run: func(t *testing.T, h *harness) (*http.Client, *http.Response, string) {
				c := h.browser(t)
				state := h.begin(t, c).Query().Get("state")
				u := h.ts.URL + "/auth/google/callback?error=access_denied&state=" + url.QueryEscape(state)
				get(t, c, u) // uses the state up
				other := h.browser(t)
				req, _ := http.NewRequest("GET", h.ts.URL+"/auth/google/callback?code=x&state="+url.QueryEscape(state), nil)
				req.AddCookie(&http.Cookie{Name: loginCookieName(state), Value: state})
				resp, err := other.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				b, _ := io.ReadAll(resp.Body)
				return other, resp, string(b)
			},
		},
		{
			name: "Google reports access denied",
			run: func(t *testing.T, h *harness) (*http.Client, *http.Response, string) {
				c := h.browser(t)
				state := h.begin(t, c).Query().Get("state")
				resp, body := get(t, c, h.ts.URL+"/auth/google/callback?error=access_denied&state="+url.QueryEscape(state))
				return c, resp, body
			},
		},
		{
			name:  "wrong issuer",
			setup: claims(func(m map[string]any) { m["iss"] = "https://evil.example" }),
			run:   followIn,
		},
		{
			name:  "wrong audience",
			setup: claims(func(m map[string]any) { m["aud"] = "other-client.apps.googleusercontent.com" }),
			run:   followIn,
		},
		{
			name:  "audience array without this client",
			setup: claims(func(m map[string]any) { m["aud"] = []any{"a", "b"} }),
			run:   followIn,
		},
		{
			name:  "wrong nonce",
			setup: claims(func(m map[string]any) { m["nonce"] = "not-the-nonce" }),
			run:   followIn,
		},
		{
			name:  "no expiry",
			setup: claims(func(m map[string]any) { delete(m, "exp") }),
			run:   followIn,
		},
		{
			name: "expired",
			// The fake's token lasts an hour of real time; this clock is
			// two hours ahead.
			cfg: func(c *Config) { c.Now = func() time.Time { return time.Now().Add(2 * time.Hour) } },
			run: followIn,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.cfg)
			if tt.setup != nil {
				tt.setup(h)
			}
			c, resp, body := tt.run(t, h)
			h.checkRefused(t, c, resp, body)
			if n := len(h.a.st.sessions); n != 0 {
				t.Errorf("store holds %d sessions, want none", n)
			}
		})
	}
}

// TestGoogleAudienceArray shows an ID token whose aud is an array naming
// the playmat's client among others is accepted.
func TestGoogleAudienceArray(t *testing.T) {
	h := newHarness(t, nil)
	h.g.Claims = func(m map[string]any) { m["aud"] = []any{"other-client", h.g.ClientID} }
	h.signInOK(t, h.browser(t))
}

// TestGoogleAddressesFromPublic shows every Google sign-in address is built
// from the configured public address and Google endpoint, never from the
// incoming request's host.
func TestGoogleAddressesFromPublic(t *testing.T) {
	h := newHarness(t, nil)
	req := httptest.NewRequest("GET", "http://evil.example/start", nil)
	req.Host = "evil.example"
	rec := httptest.NewRecorder()
	h.a.startGoogle(rec, req, &pendingLogin{kind: loginBoard, next: boardNext})
	if rec.Code != http.StatusFound {
		t.Fatalf("status %d, want 302", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	g, _ := url.Parse(h.g.URL)
	if loc.Host != g.Host || loc.Path != "/authorize" {
		t.Errorf("sent to %s, want the configured Google endpoint %s/authorize", loc, h.g.URL)
	}
	if got, want := loc.Query().Get("redirect_uri"), h.ts.URL+"/auth/google/callback"; got != want {
		t.Errorf("redirect_uri = %q, want %q", got, want)
	}
	if strings.Contains(rec.Header().Get("Location"), "evil.example") {
		t.Errorf("Google address names the request's host: %s", rec.Header().Get("Location"))
	}
}

// TestCookieAttributes shows the login and session cookies are HttpOnly
// and SameSite=Lax, are marked Secure exactly when the playmat is reached
// over https, and that the session cookie covers the whole site.
func TestCookieAttributes(t *testing.T) {
	for _, tt := range []struct {
		public string
		secure bool
	}{
		{"https://playmat.example", true},
		{"http://localhost:8080", false},
	} {
		t.Run(tt.public, func(t *testing.T) {
			pub, _ := url.Parse(tt.public)
			a, err := New(Config{Public: pub, GoogleClientID: "id", GoogleSecret: "secret"})
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			a.startGoogle(rec, httptest.NewRequest("GET", "/start", nil), &pendingLogin{kind: loginBoard, next: "/"})
			loc, _ := url.Parse(rec.Header().Get("Location"))
			if got, want := loc.Query().Get("redirect_uri"), tt.public+"/auth/google/callback"; got != want {
				t.Errorf("redirect_uri = %q, want %q", got, want)
			}
			if loc.Host != "accounts.google.com" {
				t.Errorf("default Google endpoint host = %q, want accounts.google.com", loc.Host)
			}
			rec2 := httptest.NewRecorder()
			a.signIn(rec2, Account{ID: "sub-1", Email: "one@example.com"})

			cookies := append(rec.Result().Cookies(), rec2.Result().Cookies()...)
			want := map[string]string{loginCookie: "/auth/google/callback", sessionCookie: "/"}
			for _, ck := range cookies {
				name := ck.Name
				if strings.HasPrefix(name, loginCookie+"_") {
					name = loginCookie
				}
				path, ok := want[name]
				if !ok {
					continue
				}
				delete(want, name)
				if ck.Secure != tt.secure {
					t.Errorf("%s Secure = %v, want %v", ck.Name, ck.Secure, tt.secure)
				}
				if !ck.HttpOnly {
					t.Errorf("%s is not HttpOnly", ck.Name)
				}
				if ck.SameSite != http.SameSiteLaxMode {
					t.Errorf("%s SameSite = %v, want Lax", ck.Name, ck.SameSite)
				}
				if ck.Path != path {
					t.Errorf("%s Path = %q, want %q", ck.Name, ck.Path, path)
				}
			}
			for name := range want {
				t.Errorf("no %s cookie set", name)
			}
		})
	}
}

// TestSignInsAreMemoryOnly shows a sign-in writes nothing to disk and that
// a new instance of the service knows none of the previous instance's
// sessions or sign-ins in progress.
func TestSignInsAreMemoryOnly(t *testing.T) {
	dirs := map[string]string{"cwd": t.TempDir()}
	for _, env := range []string{"HOME", "TMPDIR", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"} {
		d := t.TempDir()
		dirs[env] = d
		t.Setenv(env, d)
	}
	t.Chdir(dirs["cwd"])

	h := newHarness(t, nil)
	id := h.signInOK(t, h.browser(t))
	c := h.browser(t)
	state := h.begin(t, c).Query().Get("state")

	for name, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("sign-in wrote to %s (%s): %v", name, d, entries)
		}
	}

	a2, err := New(h.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if a2.st.session(id) != nil {
		t.Error("new instance knows the previous instance's session")
	}
	req := httptest.NewRequest("GET", "/auth/google/callback?code=x&state="+url.QueryEscape(state), nil)
	req.AddCookie(&http.Cookie{Name: loginCookieName(state), Value: state})
	rec := httptest.NewRecorder()
	a2.serveGoogleCallback(rec, req)
	if rec.Code < 400 {
		t.Errorf("new instance accepted the previous instance's sign-in: status %d", rec.Code)
	}
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == sessionCookie && ck.MaxAge >= 0 {
			t.Error("new instance set a session cookie for the previous instance's sign-in")
		}
	}
	// The first instance still knows its own session.
	if h.a.st.session(id) == nil {
		t.Error("first instance lost its session")
	}
}

// TestFakeGoogleNamedAccount shows a test can sign in a named account
// against the in-process fake Google.
func TestFakeGoogleNamedAccount(t *testing.T) {
	h := newHarness(t, nil)
	two := hostauthtest.Account{ID: "sub-2", Email: "two@example.com"}
	h.g.SetAccount(two)
	id := h.signInOK(t, h.browser(t))
	s := h.a.st.session(id)
	if s == nil {
		t.Fatal("session cookie names no session")
	}
	if s.account != (Account{ID: "sub-2", Email: "two@example.com"}) {
		t.Errorf("session is for %+v, want %+v", s.account, two)
	}
	if st := h.a.st.accounts["sub-2"]; st == nil || st.live != 1 || st.email != two.Email {
		t.Errorf("account state for sub-2 = %+v, want one live sign-in for %s", st, two.Email)
	}
}

// TestLocalPath shows only a path on this site is accepted as a sign-in's
// return address.
func TestLocalPath(t *testing.T) {
	for _, tt := range []struct {
		next string
		ok   bool
	}{
		{"/", true},
		{"/board?x=1", true},
		{"//evil.example", false},
		{`/\evil`, false},
		{"https://evil.example", false},
		{"", false},
		{"board", false},
	} {
		if got := localPath(tt.next); got != tt.ok {
			t.Errorf("localPath(%q) = %v, want %v", tt.next, got, tt.ok)
		}
	}
}

// TestHealthz shows the health check answers without a sign-in.
func TestHealthz(t *testing.T) {
	h := newHarness(t, nil)
	resp, body := get(t, &http.Client{}, h.ts.URL+"/healthz")
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(body) != "ok" {
		t.Errorf("GET /healthz = %d %q, want 200 ok", resp.StatusCode, body)
	}
}

// TestNewRefusesIncompleteConfig shows New fails without a public URL or
// Google credentials.
func TestNewRefusesIncompleteConfig(t *testing.T) {
	pub, _ := url.Parse("https://playmat.example")
	for _, tt := range []struct {
		name string
		cfg  Config
	}{
		{"no public URL", Config{GoogleClientID: "id", GoogleSecret: "secret"}},
		{"public URL without host", Config{Public: &url.URL{Scheme: "https"}, GoogleClientID: "id", GoogleSecret: "secret"}},
		{"no client ID", Config{Public: pub, GoogleSecret: "secret"}},
		{"no secret", Config{Public: pub, GoogleClientID: "id"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, err := New(tt.cfg)
			if err == nil || a != nil {
				t.Errorf("New = %v, %v; want an error", a, err)
			}
		})
	}
	if _, err := New(Config{Public: pub, GoogleClientID: "id", GoogleSecret: "secret"}); err != nil {
		t.Errorf("New with a complete config: %v", err)
	}
}
