package hostauth

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

// TestBothEntryPointsShowTheSigninPage shows an assistant starting to
// connect, a signed-out browser asking to sign in to the board, and a
// signed-out browser opening a board page are all sent to the TCG Replay
// sign-in page, not to Google.
func TestBothEntryPointsShowTheSigninPage(t *testing.T) {
	c := newConnector(t)
	id, _ := c.registerClient(t, "none", hostauthtest.LoopbackCallback)
	mux := http.NewServeMux()
	c.a.Routes(mux)
	mux.Handle("GET /board", c.a.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	c.sw.set(mux)

	for name, start := range map[string]string{
		"connecting":           c.authorizeURL(id, hostauthtest.LoopbackCallback, nil),
		"board sign-in":        c.ts.URL + "/auth/login?next=/board",
		"opening a board page": c.ts.URL + "/board?board=x",
	} {
		t.Run(name, func(t *testing.T) {
			b := c.browser(t)
			resp, body := getPage(t, b, start)
			if resp.StatusCode != http.StatusOK || resp.Request.URL.Path != signinPath {
				t.Fatalf("ended at %s with %d, want the sign-in page", resp.Request.URL, resp.StatusCode)
			}
			if !strings.Contains(body, `data-provider="google"`) || !strings.Contains(body, "Sign in with Google") {
				t.Errorf("sign-in page has no Google button: %s", body)
			}
			if got := c.g.Prompts(); len(got) != 0 {
				t.Errorf("Google was asked %q before the button was chosen", got)
			}
		})
	}
}

// TestSigninPageWithoutPasswordAccounts shows that with no password
// accounts the page offers only Google: no form and no username or
// password field, and never a sign-up, password change or recovery
// control.
func TestSigninPageWithoutPasswordAccounts(t *testing.T) {
	h := newHarness(t, nil)
	_, body := getPage(t, h.browser(t), h.ts.URL+"/start")
	for _, bad := range []string{"<form", "<input", `type="password"`, `name="username"`} {
		if strings.Contains(body, bad) {
			t.Errorf("sign-in page without password accounts contains %q", bad)
		}
	}
	lower := strings.ToLower(body)
	for _, bad := range []string{"sign up", "sign-up", "register", "create account", "forgot", "reset", "recover", "change password"} {
		if strings.Contains(lower, bad) {
			t.Errorf("sign-in page offers %q", bad)
		}
	}
	if n := len(regexp.MustCompile(`data-provider="`).FindAllString(body, -1)); n != 1 {
		t.Errorf("sign-in page has %d provider buttons, want 1 (Google)", n)
	}
}

// TestSigninRoutesRefuseOtherSignIns shows the sign-in page and the Google
// start show the "start signing in again" page, and send nobody to
// Google, when the sign-in is missing, unknown, expired, or belongs to
// another browser.
func TestSigninRoutesRefuseOtherSignIns(t *testing.T) {
	for _, path := range []string{signinPath, googleStartPath} {
		t.Run(path, func(t *testing.T) {
			h := newHarness(t, nil)
			mine := h.browser(t)
			myState := h.toSignin(t, mine, h.ts.URL+"/start").Query().Get("state")
			other := h.browser(t)
			theirState := h.toSignin(t, other, h.ts.URL+"/start").Query().Get("state")

			cases := map[string]struct {
				c     *http.Client
				state string
				fake  bool // send a login cookie for state by hand
			}{
				"no state":                    {c: mine, state: ""},
				"unknown state with a cookie": {c: h.browser(t), state: "NOSUCHSTATE", fake: true},
				"no login cookie":             {c: h.browser(t), state: myState},
				"another browser's state":     {c: mine, state: theirState},
			}
			for name, tc := range cases {
				u := h.ts.URL + path + "?state=" + url.QueryEscape(tc.state)
				req, _ := http.NewRequest("GET", u, nil)
				if tc.fake {
					req.AddCookie(&http.Cookie{Name: loginCookieName(tc.state), Value: tc.state})
				}
				follow := tc.c.CheckRedirect
				tc.c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
				resp, err := tc.c.Do(req)
				tc.c.CheckRedirect = follow
				if err != nil {
					t.Fatal(err)
				}
				buf, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(buf), "start signing in again") {
					t.Errorf("%s: status %d, want 400 with the start-again page: %s", name, resp.StatusCode, buf)
				}
			}
			if got := h.g.Prompts(); len(got) != 0 {
				t.Errorf("refused sign-ins reached Google: %q", got)
			}
		})
	}
}

// TestSigninPageDoesNotUseUpTheSignIn shows the page can be shown again,
// and Google chosen afterwards, without the sign-in in progress being
// used up: only the provider's answer does that.
func TestSigninPageDoesNotUseUpTheSignIn(t *testing.T) {
	h := newHarness(t, nil)
	c := h.browser(t)
	page := h.toSignin(t, c, h.ts.URL+"/start")
	for i := 0; i < 3; i++ {
		if resp, body := getPage(t, c, page.String()); resp.StatusCode != http.StatusOK {
			t.Fatalf("showing the page again (%d): %d %s", i, resp.StatusCode, body)
		}
	}
	resp, body := get(t, c, h.chooseGoogle(t, c, page).String())
	if resp.StatusCode != http.StatusOK || body != "OK" || resp.Request.URL.RequestURI() != boardNext {
		t.Fatalf("sign-in ended at %s with %d %q, want the board page", resp.Request.URL, resp.StatusCode, body)
	}
	// Used up now: the page refuses it.
	if resp, _ := getPage(t, c, page.String()); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("page after the sign-in finished: %d, want 400", resp.StatusCode)
	}
}

// TestCallbackClearsOldLoginCookiePath shows Google's callback also clears
// a login cookie at the callback-only path sign-ins used before the page,
// so one in flight at deploy time does not linger.
func TestCallbackClearsOldLoginCookiePath(t *testing.T) {
	h := newHarness(t, nil)
	c := h.browser(t)
	loc := h.begin(t, c)
	follow := c.CheckRedirect
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Get(loc.String()) // fake Google answers with a redirect to the callback
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	cb, _ := resp.Location()
	resp, err = c.Get(cb.String())
	c.CheckRedirect = follow
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	paths := map[string]bool{}
	for _, ck := range resp.Cookies() {
		if strings.HasPrefix(ck.Name, loginCookie+"_") && ck.MaxAge < 0 {
			paths[ck.Path] = true
		}
	}
	for _, p := range []string{loginCookiePath, googleCallbackPath} {
		if !paths[p] {
			t.Errorf("callback did not clear the login cookie at path %s (cleared: %v)", p, paths)
		}
	}
}

// TestConnectorSignInThroughThePage follows an assistant connecting in one
// browser: it is shown the sign-in page (Google not yet asked), chooses
// Google, and comes back with a code for the assistant's own callback and
// state. The code exchanges, with the request's PKCE verifier, for tokens
// that act for the Google account, and the same browser is now signed in
// to the board as that account.
func TestConnectorSignInThroughThePage(t *testing.T) {
	c := newConnector(t)
	acct := hostauthtest.Account{ID: "sub-7", Email: "seven@example.com"}
	c.g.SetAccount(acct)
	id, _ := c.registerClient(t, "none", hostauthtest.LoopbackCallback)
	b := c.browser(t)

	page := c.toSignin(t, b, c.authorizeURL(id, hostauthtest.LoopbackCallback, nil))
	if got := c.g.Prompts(); len(got) != 0 {
		t.Fatalf("Google was asked %q before the button was chosen", got)
	}
	if resp, _ := getPage(t, b, c.ts.URL+"/auth/session"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("/auth/session before choosing Google: %d, want 401", resp.StatusCode)
	}
	google := c.chooseGoogle(t, b, page)
	if got := google.Scheme + "://" + google.Host + google.Path; got != c.g.URL+"/authorize" {
		t.Fatalf("Google button sent the browser to %s, want Google's /authorize", got)
	}

	loc, err := hostauthtest.FollowSignIn(b, google.String())
	if err != nil {
		t.Fatal(err)
	}
	if got := loc.Scheme + "://" + loc.Host + loc.Path; got != hostauthtest.LoopbackCallback {
		t.Fatalf("sign-in returned to %s, want the assistant's callback %s", got, hostauthtest.LoopbackCallback)
	}
	code := loc.Query().Get("code")
	if code == "" || loc.Query().Get("state") != "st" {
		t.Fatalf("sign-in returned %s, want a code and the assistant's state st", loc)
	}

	resp, m := c.token(t, exchange(id, code, hostauthtest.LoopbackCallback, testVerifier), "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exchange: %d %v, want tokens", resp.StatusCode, m)
	}
	access, _ := m["access_token"].(string)
	tok := c.a.st.accessToken(access)
	if tok == nil {
		t.Fatalf("access token %q does not open", access)
	}
	if tok.account.ID != acct.ID || tok.account.Email != acct.Email {
		t.Errorf("access token acts for %+v, want %+v", tok.account, acct)
	}
	c.checkMCPAccepted(t, access)

	resp, err = b.Get(c.ts.URL + "/auth/session")
	if err != nil {
		t.Fatal(err)
	}
	info := readJSON(t, resp)
	if resp.StatusCode != http.StatusOK || info["email"] != acct.Email {
		t.Errorf("/auth/session in the connecting browser: %d %v, want 200 for %s", resp.StatusCode, info, acct.Email)
	}
	if s := c.a.st.session(c.sessionCookieIn(b)); s == nil || s.account.ID != acct.ID {
		t.Errorf("the connecting browser's board session is %+v, want one for %s", s, acct.ID)
	}
	if got := c.g.Prompts(); len(got) != 1 || got[0] != "select_account" {
		t.Errorf("Google saw prompts %q, want one select_account", got)
	}
}

// TestBoardSignInThroughThePageKeepsTheAccount shows a signed-out browser
// opening a board page is shown the sign-in page, and choosing Google
// returns it to the page it asked for, signed in as the Google account.
func TestBoardSignInThroughThePageKeepsTheAccount(t *testing.T) {
	acct := hostauthtest.Account{ID: "sub-8", Email: "eight@example.com"}
	b := newBoard(t)
	b.g.SetAccount(acct)
	br := b.browser(t)
	const path = "/game/live-1?x=1"

	page := b.toSignin(t, br, b.ts.URL+"/auth/login?next="+url.QueryEscape(path))
	if got := b.g.Prompts(); len(got) != 0 {
		t.Fatalf("Google was asked %q before the button was chosen", got)
	}
	resp, body := get(t, br, b.chooseGoogle(t, br, page).String())
	if resp.StatusCode != http.StatusOK || resp.Request.URL.RequestURI() != path || body != boardText(acct) {
		t.Fatalf("board sign-in ended at %s with %d %q, want %s showing %s", resp.Request.URL, resp.StatusCode, body, path, boardText(acct))
	}
	if s := b.a.st.session(b.sessionCookieIn(br)); s == nil || s.account.ID != acct.ID || s.account.Email != acct.Email {
		t.Errorf("board session is %+v, want one for %+v", s, acct)
	}
}

// TestExpiredSignInIsRefused shows a sign-in in progress lasts ten
// minutes: within that the page, the Google button and Google's callback
// all go through, and after it each shows the "start signing in again"
// page, sends nobody on to Google, and signs nobody in.
func TestExpiredSignInIsRefused(t *testing.T) {
	newClocked := func(t *testing.T) (*harness, *fakeClock) {
		clock := &fakeClock{t: time.Now()}
		return newHarness(t, func(c *Config) { c.Now = clock.Now }), clock
	}
	checkStartAgain := func(t *testing.T, h *harness, c *http.Client, resp *http.Response, body string) {
		t.Helper()
		if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "start signing in again") {
			t.Errorf("ended at %s with %d, want 400 with the start-again page: %s", resp.Request.URL, resp.StatusCode, body)
		}
		h.checkRefused(t, c, resp, body)
	}

	t.Run("within its lifetime", func(t *testing.T) {
		h, clock := newClocked(t)
		c := h.browser(t)
		page := h.toSignin(t, c, h.ts.URL+"/start")
		clock.Advance(pendingLifetime - time.Minute)
		google := h.chooseGoogle(t, c, page)
		clock.Advance(30 * time.Second)
		resp, body := get(t, c, google.String())
		if resp.StatusCode != http.StatusOK || body != "OK" || resp.Request.URL.RequestURI() != boardNext {
			t.Fatalf("sign-in within its lifetime ended at %s with %d %q, want the board page", resp.Request.URL, resp.StatusCode, body)
		}
	})
	t.Run("page", func(t *testing.T) {
		h, clock := newClocked(t)
		c := h.browser(t)
		page := h.toSignin(t, c, h.ts.URL+"/start")
		clock.Advance(pendingLifetime + time.Minute)
		resp, body := getPage(t, c, page.String())
		checkStartAgain(t, h, c, resp, body)
		if strings.Contains(body, `data-provider="`) {
			t.Error("an expired sign-in's page still offers a provider button")
		}
		if got := h.g.Prompts(); len(got) != 0 {
			t.Errorf("an expired sign-in reached Google: %q", got)
		}
	})
	t.Run("Google button", func(t *testing.T) {
		h, clock := newClocked(t)
		c := h.browser(t)
		state := h.toSignin(t, c, h.ts.URL+"/start").Query().Get("state")
		clock.Advance(pendingLifetime + time.Minute)
		resp, body := getPage(t, c, h.ts.URL+googleStartPath+"?state="+url.QueryEscape(state))
		checkStartAgain(t, h, c, resp, body)
		if got := h.g.Prompts(); len(got) != 0 {
			t.Errorf("an expired sign-in reached Google: %q", got)
		}
	})
	t.Run("callback", func(t *testing.T) {
		h, clock := newClocked(t)
		c := h.browser(t)
		google := h.begin(t, c)
		clock.Advance(pendingLifetime + time.Minute)
		resp, body := get(t, c, google.String())
		if resp.Request.URL.Path != googleCallbackPath {
			t.Fatalf("ended at %s, want Google's callback", resp.Request.URL)
		}
		checkStartAgain(t, h, c, resp, body)
		if n := len(h.a.st.sessions); n != 0 {
			t.Errorf("store holds %d sessions, want none", n)
		}
	})
}

// TestUsedSignInIsRefusedByTheButton shows that once a sign-in has
// finished, its Google button cannot start Google again.
func TestUsedSignInIsRefusedByTheButton(t *testing.T) {
	h := newHarness(t, nil)
	c := h.browser(t)
	page := h.toSignin(t, c, h.ts.URL+"/start")
	h.signInOK(t, h.browser(t)) // an unrelated sign-in, so the store is not empty
	resp, body := get(t, c, h.chooseGoogle(t, c, page).String())
	if resp.StatusCode != http.StatusOK || body != "OK" {
		t.Fatalf("sign-in ended with %d %q, want the board page", resp.StatusCode, body)
	}
	prompts := len(h.g.Prompts())
	resp, body = getPage(t, c, h.ts.URL+googleStartPath+"?"+page.RawQuery)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "start signing in again") {
		t.Errorf("Google button after the sign-in finished: %d, want 400 with the start-again page: %s", resp.StatusCode, body)
	}
	if got := len(h.g.Prompts()); got != prompts {
		t.Errorf("a used sign-in reached Google %d more times", got-prompts)
	}
}

// TestSigninPageIsNotCached shows the sign-in page, and the redirect to
// it, tell the browser and any cache in between not to keep them, and
// that the page's Google button names exactly this sign-in.
func TestSigninPageIsNotCached(t *testing.T) {
	h := newHarness(t, nil)
	c := h.browser(t)
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Get(h.ts.URL + "/start")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("redirect to the sign-in page: Cache-Control %q, want no-store", got)
	}
	loc, _ := resp.Location()
	state := loc.Query().Get("state")

	resp, body := getPage(t, c, loc.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sign-in page: %d %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("sign-in page: Cache-Control %q, want no-store", got)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Errorf("sign-in page: Content-Type %q, want text/html", got)
	}
	button := regexp.MustCompile(`<a class="provider" data-provider="google" href="/auth/google\?state=` + regexp.QuoteMeta(state) + `">(?s:.*?)Sign in with Google</a>`)
	if !button.MatchString(body) {
		t.Errorf("sign-in page lacks the Google button for state %s: %s", state, body)
	}
}
