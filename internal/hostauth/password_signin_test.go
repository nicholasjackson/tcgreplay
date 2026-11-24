package hostauth

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// alicePassword is the configured password of the test account alice.
const alicePassword = "hunter2-correct-horse"

// passwordNote is the note under the username and password form.
const passwordNote = "This form is for reviewer and invited accounts. Players sign in with Google."

// aliceConnector is a connector with one password account, alice.
func aliceConnector(t *testing.T) *connector {
	t.Helper()
	return passwordConnector(t, "alice:"+hashFor(t, alicePassword))
}

// formState opens the sign-in page start leads to in b and returns the
// state its username and password form carries.
func (h *harness) formState(t *testing.T, b *http.Client, start string) string {
	t.Helper()
	page := h.toSignin(t, b, start)
	resp, body := getPage(t, b, page.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sign-in page: %d %s", resp.StatusCode, body)
	}
	m := regexp.MustCompile(`name="state" value="([^"]+)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("sign-in page has no username and password form: %s", body)
	}
	if m[1] != page.Query().Get("state") {
		t.Fatalf("form carries state %q, want the page's %q", m[1], page.Query().Get("state"))
	}
	return m[1]
}

// postPassword submits the username and password form for state from b,
// without following a redirect, and returns the response and its body.
func (h *harness) postPassword(t *testing.T, b *http.Client, state, user, pass string) (*http.Response, string) {
	t.Helper()
	follow := b.CheckRedirect
	b.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer func() { b.CheckRedirect = follow }()
	resp, err := b.PostForm(h.ts.URL+passwordPath, url.Values{"state": {state}, "username": {user}, "password": {pass}})
	if err != nil {
		t.Fatalf("POST %s: %v", passwordPath, err)
	}
	defer resp.Body.Close()
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(buf)
}

// wantPasswordPage fails unless resp is the sign-in page again with
// status and msg, its form refilled with user, and nobody signed in.
func (h *harness) wantPasswordPage(t *testing.T, b *http.Client, resp *http.Response, body string, status int, msg, user string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Errorf("status %d, want %d: %s", resp.StatusCode, status, body)
	}
	if !strings.Contains(html.UnescapeString(body), `<p class="error" role="alert">`+msg+`</p>`) {
		t.Errorf("page lacks the message %q: %s", msg, body)
	}
	if !strings.Contains(body, `name="username" type="text" autocomplete="username" autocapitalize="none" spellcheck="false" required value="`+user+`"`) {
		t.Errorf("username field not refilled with %q: %s", user, body)
	}
	if strings.Contains(body, alicePassword) {
		t.Error("page shows the password")
	}
	if id := h.sessionCookieIn(b); id != "" {
		t.Errorf("browser holds a board session cookie %q after a refused sign-in", id)
	}
	for _, ck := range resp.Cookies() {
		if ck.Name == sessionCookie {
			t.Errorf("refused sign-in set a session cookie %+v", ck)
		}
	}
}

// TestPasswordFormShownOnlyWithAccounts shows the sign-in page offers the
// username and password form, after Google and with the reviewer and
// invited accounts note, when accounts are configured; without them it
// shows neither and the form's route does not exist.
func TestPasswordFormShownOnlyWithAccounts(t *testing.T) {
	t.Run("with accounts", func(t *testing.T) {
		c := aliceConnector(t)
		b := c.browser(t)
		page := c.toSignin(t, b, c.ts.URL+"/start")
		resp, body := getPage(t, b, page.String())
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("sign-in page: %d %s", resp.StatusCode, body)
		}
		state := regexp.QuoteMeta(page.Query().Get("state"))
		form := regexp.MustCompile(`(?s)<div class="divider">or</div>\s*` +
			`<form class="password" method="POST" action="/auth/password">\s*` +
			`<input type="hidden" name="state" value="` + state + `">.*?` +
			`<input id="username" name="username" type="text" autocomplete="username"[^>]* value="">.*?` +
			`<input id="password" name="password" type="password" autocomplete="current-password" required>.*?` +
			`<button type="submit">Sign in</button>\s*</form>\s*` +
			`<p class="note">` + regexp.QuoteMeta(passwordNote) + `</p>`)
		if !form.MatchString(body) {
			t.Errorf("sign-in page lacks the username and password form with its note: %s", body)
		}
		if g, f := strings.Index(body, `data-provider="google"`), strings.Index(body, `<form class="password"`); g < 0 || f < g {
			t.Errorf("Google button at %d, form at %d: want Google first", g, f)
		}
		if got := c.g.Prompts(); len(got) != 0 {
			t.Errorf("showing the form asked Google %q", got)
		}
	})
	t.Run("without accounts", func(t *testing.T) {
		c := newConnector(t)
		b := c.browser(t)
		page := c.toSignin(t, b, c.ts.URL+"/start")
		_, body := getPage(t, b, page.String())
		for _, bad := range []string{`<form`, `name="password"`, `name="username"`, `class="divider"`, passwordNote} {
			if strings.Contains(body, bad) {
				t.Errorf("sign-in page without password accounts contains %q", bad)
			}
		}
		resp, _ := c.postPassword(t, b, page.Query().Get("state"), "alice", alicePassword)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("POST %s without accounts: %d, want 404", passwordPath, resp.StatusCode)
		}
		if id := c.sessionCookieIn(b); id != "" {
			t.Error("a post without accounts signed the browser in")
		}
	})
}

// TestPasswordConnectorSignIn follows an assistant connecting in one
// browser with alice's username and password: the code comes back to the
// assistant's callback with its state, exchanges for tokens that act for
// password:alice (provider password), and the same browser is signed in
// to the board as alice.
func TestPasswordConnectorSignIn(t *testing.T) {
	c := aliceConnector(t)
	id, _ := c.registerClient(t, "none", hostauthtest.LoopbackCallback)
	b := c.browser(t)

	// The username is compared without regard to case or spaces.
	loc, err := hostauthtest.FollowPasswordSignIn(b, c.authorizeURL(id, hostauthtest.LoopbackCallback, nil), " Alice ", alicePassword)
	if err != nil {
		t.Fatal(err)
	}
	if got := loc.Scheme + "://" + loc.Host + loc.Path; got != hostauthtest.LoopbackCallback {
		t.Fatalf("sign-in returned to %s, want the assistant's callback", got)
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
	acct, ok := c.tokenAccount(t, access)
	want := Account{ID: "password:alice", Email: "alice", Provider: ProviderPassword}
	if !ok || acct != want {
		t.Errorf("access token acts for %+v (ok %v), want %+v", acct, ok, want)
	}
	c.checkMCPAccepted(t, access)

	resp, err = b.Get(c.ts.URL + "/auth/session")
	if err != nil {
		t.Fatal(err)
	}
	info := readJSON(t, resp)
	if resp.StatusCode != http.StatusOK || info["email"] != "alice" {
		t.Errorf("/auth/session in the connecting browser: %d %v, want 200 for alice", resp.StatusCode, info)
	}
	if s := c.a.st.session(c.sessionCookieIn(b)); s == nil || s.account.ID != want.ID {
		t.Errorf("the connecting browser's board session is %+v, want one for %s", s, want.ID)
	}
	if got := c.g.Prompts(); len(got) != 0 {
		t.Errorf("a password sign-in asked Google %q", got)
	}
}

// TestPasswordMCPClientToolsRunAsTheAccount shows the MCP library's own
// OAuth client, signing in with a username and password, calls a tool
// that acts for the password account.
func TestPasswordMCPClientToolsRunAsTheAccount(t *testing.T) {
	c := aliceConnector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "password-test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, hostauthtest.PasswordMCPTransport(t, c.ts.URL, c.ts.URL, "alice", alicePassword), nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cs.Close()
	if got, want := callWhoami(t, ctx, cs), (whoami{ID: "password:alice", Email: "alice"}); got != want {
		t.Errorf("tool ran as %+v, want %+v", got, want)
	}
	tok := hostauthtest.ConnectPassword(t, c.ts.URL, c.ts.URL, "alice", alicePassword)
	if acct, ok := c.tokenAccount(t, tok.AccessToken); !ok || acct.ID != "password:alice" || acct.Provider != ProviderPassword {
		t.Errorf("ConnectPassword's token acts for %+v (ok %v), want password:alice", acct, ok)
	}
}

// TestPasswordBoardSignInReturnsToThePage shows a board sign-in with a
// username and password returns the browser to the board page it asked
// for, signed in as the password account.
func TestPasswordBoardSignInReturnsToThePage(t *testing.T) {
	c := aliceConnector(t)
	b := c.browser(t)
	const next = "/board?board=x"
	state := c.formState(t, b, c.ts.URL+"/auth/login?next="+url.QueryEscape(next))
	resp, body := c.postPassword(t, b, state, "alice", alicePassword)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != next {
		t.Fatalf("sign-in answered %d to %q, want 302 to %s: %s", resp.StatusCode, resp.Header.Get("Location"), next, body)
	}
	resp, body = getPage(t, b, c.ts.URL+next)
	if resp.StatusCode != http.StatusOK || body != "OK" {
		t.Errorf("board page after sign-in: %d %q, want the board", resp.StatusCode, body)
	}
	s := c.a.st.session(c.sessionCookieIn(b))
	if s == nil || s.account.ID != "password:alice" || s.account.Provider != ProviderPassword {
		t.Errorf("board session is %+v, want one for password:alice", s)
	}
	for _, name := range c.loginCookiesIn(b) {
		if name == loginCookieName(state) {
			t.Errorf("the login cookie for %s was not cleared after the sign-in", state)
		}
	}

	cookie := hostauthtest.PasswordBoardCookie(t, c.ts.URL, c.ts.URL, "alice", alicePassword)
	if s := c.a.st.session(cookie.Value); s == nil || s.account.ID != "password:alice" {
		t.Errorf("PasswordBoardCookie's session is %+v, want one for password:alice", s)
	}
}

// loginCookiesIn returns the names of the login cookies b holds for the
// sign-in routes.
func (h *harness) loginCookiesIn(b *http.Client) []string {
	u, _ := url.Parse(h.ts.URL + loginCookiePath)
	var names []string
	for _, ck := range b.Jar.Cookies(u) {
		if strings.HasPrefix(ck.Name, loginCookie+"_") {
			names = append(names, ck.Name)
		}
	}
	return names
}

// TestPasswordFailuresLookAlike shows an unknown username and a wrong
// password get the same status and page, apart from the refilled
// username, sign nobody in, and leave the sign-in usable: the right
// password then finishes it on the same page.
func TestPasswordFailuresLookAlike(t *testing.T) {
	c := aliceConnector(t)
	b := c.browser(t)
	state := c.formState(t, b, c.ts.URL+"/start")

	unknownResp, unknown := c.postPassword(t, b, state, "mallory", alicePassword)
	c.wantPasswordPage(t, b, unknownResp, unknown, http.StatusUnauthorized, msgBadCredentials, "mallory")
	wrongResp, wrong := c.postPassword(t, b, state, "alice", "not-the-password")
	c.wantPasswordPage(t, b, wrongResp, wrong, http.StatusUnauthorized, msgBadCredentials, "alice")
	if strings.Contains(wrong, "not-the-password") {
		t.Error("the page shows the attempted password")
	}

	if unknownResp.StatusCode != wrongResp.StatusCode {
		t.Errorf("unknown username %d, wrong password %d: want the same", unknownResp.StatusCode, wrongResp.StatusCode)
	}
	a := strings.Replace(unknown, `value="mallory"`, `value=""`, 1)
	w := strings.Replace(wrong, `value="alice"`, `value=""`, 1)
	if a != w {
		t.Errorf("unknown username and wrong password pages differ:\n%s\n---\n%s", a, w)
	}
	if !strings.Contains(unknown, `name="state" value="`+state+`"`) {
		t.Error("the failure page's form does not carry the same sign-in")
	}

	resp, body := c.postPassword(t, b, state, "alice", alicePassword)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != boardNext {
		t.Fatalf("retry with the right password: %d to %q, want 302 to %s: %s", resp.StatusCode, resp.Header.Get("Location"), boardNext, body)
	}
	if s := c.a.st.session(c.sessionCookieIn(b)); s == nil || s.account.ID != "password:alice" {
		t.Errorf("board session after the retry is %+v, want one for password:alice", s)
	}
	// Used up now.
	resp, body = c.postPassword(t, b, state, "alice", alicePassword)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "start signing in again") {
		t.Errorf("posting a finished sign-in again: %d, want 400 with the start-again page: %s", resp.StatusCode, body)
	}
}

// TestPasswordThrottlePerUsername shows that after 10 failures for one
// username within 15 minutes the right password is refused too, with the
// try-again-later page, until the allowance refills (one every 90
// seconds); other usernames are not held up.
func TestPasswordThrottlePerUsername(t *testing.T) {
	c := passwordConnector(t, "alice:"+hashFor(t, alicePassword)+",bob:"+hashFor(t, "bob-pw"))
	_, buf := recordTo(t, c.a)
	b := c.browser(t)
	state := c.formState(t, b, c.ts.URL+"/start")

	for i := 1; i <= loginUserCap; i++ {
		resp, body := c.postPassword(t, b, state, "alice", "wrong")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("failure %d: %d, want 401: %s", i, resp.StatusCode, body)
		}
	}
	// The username is throttled in any case.
	resp, body := c.postPassword(t, b, state, "ALICE", alicePassword)
	c.wantPasswordPage(t, b, resp, body, http.StatusTooManyRequests, msgThrottled, "ALICE")
	if !strings.Contains(buf.String(), `"event":"sign_in_refused","provider":"password","reason":"throttled"`) {
		t.Errorf("no throttled refusal recorded:\n%s", buf)
	}

	// bob, from another browser, is not held up by alice's failures.
	other := c.browser(t)
	resp, body = c.postPassword(t, other, c.formState(t, other, c.ts.URL+"/start"), "bob", "bob-pw")
	if resp.StatusCode != http.StatusFound {
		t.Errorf("bob's sign-in while alice is throttled: %d, want 302: %s", resp.StatusCode, body)
	}

	// Still refused a minute on; two minutes on one more try is allowed.
	c.clock.Advance(time.Minute)
	resp, body = c.postPassword(t, b, state, "alice", alicePassword)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("right password a minute after the throttle: %d, want 429: %s", resp.StatusCode, body)
	}
	c.clock.Advance(time.Minute)
	resp, body = c.postPassword(t, b, state, "alice", alicePassword)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != boardNext {
		t.Errorf("right password after the allowance refilled: %d to %q, want 302 to %s: %s", resp.StatusCode, resp.Header.Get("Location"), boardNext, body)
	}
}

// TestPasswordThrottlePerClient shows 30 failures across 30 usernames from
// one client refuse that client's next attempt, for a username it has not
// tried and for a configured account with its right password.
func TestPasswordThrottlePerClient(t *testing.T) {
	c := aliceConnector(t)
	b := c.browser(t)
	state := c.formState(t, b, c.ts.URL+"/start")
	for i := 0; i < loginClientCap; i++ {
		user := "user" + string(rune('a'+i/10)) + string(rune('0'+i%10))
		resp, body := c.postPassword(t, b, state, user, "wrong")
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("failure %d (%s): %d, want 401: %s", i+1, user, resp.StatusCode, body)
		}
	}
	resp, body := c.postPassword(t, b, state, "fresh-user", "anything")
	c.wantPasswordPage(t, b, resp, body, http.StatusTooManyRequests, msgThrottled, "fresh-user")
	resp, body = c.postPassword(t, b, state, "alice", alicePassword)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("alice's right password from the throttled client: %d, want 429: %s", resp.StatusCode, body)
	}
}

// TestPasswordPostFromElsewhereRefused shows the form's post is refused
// with the start-again page, signing nobody in, when it comes from
// another site (no login cookie: it is SameSite=Lax and scoped to the
// sign-in routes) or from a browser that did not start the sign-in; the
// browser that did can still finish it.
func TestPasswordPostFromElsewhereRefused(t *testing.T) {
	c := aliceConnector(t)
	b := c.browser(t)

	// The login cookie the sign-in sets is what ties it to b.
	b.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := b.Get(c.ts.URL + "/start")
	b.CheckRedirect = nil
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc, _ := resp.Location()
	state := loc.Query().Get("state")
	var login *http.Cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == loginCookieName(state) {
			login = ck
		}
	}
	if login == nil || login.Value != state || login.Path != "/auth/" || login.SameSite != http.SameSiteLaxMode || !login.HttpOnly {
		t.Fatalf("login cookie %+v, want %s=%s, Path /auth/, SameSite=Lax, HttpOnly", login, loginCookieName(state), state)
	}

	checkRefused := func(t *testing.T, resp *http.Response, body string) {
		t.Helper()
		if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "start signing in again") {
			t.Errorf("status %d, want 400 with the start-again page: %s", resp.StatusCode, body)
		}
		for _, ck := range resp.Cookies() {
			if ck.Name == sessionCookie {
				t.Errorf("refused post set a session cookie %+v", ck)
			}
		}
	}

	t.Run("another site", func(t *testing.T) {
		req, _ := http.NewRequest("POST", c.ts.URL+passwordPath, strings.NewReader(url.Values{"state": {state}, "username": {"alice"}, "password": {alicePassword}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "https://evil.example")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		buf, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		checkRefused(t, resp, string(buf))
	})
	t.Run("a browser that did not start it", func(t *testing.T) {
		other := c.browser(t)
		c.formState(t, other, c.ts.URL+"/start") // it holds a login cookie of its own
		resp, body := c.postPassword(t, other, state, "alice", alicePassword)
		checkRefused(t, resp, body)
		if id := c.sessionCookieIn(other); id != "" {
			t.Error("the other browser was signed in")
		}
	})
	if n := len(c.a.st.sessions); n != 0 {
		t.Errorf("store holds %d sessions after the refused posts, want none", n)
	}

	resp, body := c.postPassword(t, b, state, "alice", alicePassword)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != boardNext {
		t.Errorf("the starting browser's post: %d to %q, want 302 to %s: %s", resp.StatusCode, resp.Header.Get("Location"), boardNext, body)
	}
}

// TestPasswordSignInKeepsSecretsOut shows that after a successful and
// failed password sign-ins, including a password typed into the username
// field, neither the logs nor the records contain a password, an
// attempted password or the attempted username; and that sign-in records
// carry the provider: password for alice, google for a Google sign-in.
func TestPasswordSignInKeepsSecretsOut(t *testing.T) {
	logs := captureHostLog(t)
	c := aliceConnector(t)
	rec, buf := recordTo(t, c.a)
	const (
		attempted = "attempted-pw-7c1e"
		typedUser = "hunter2-typed-as-username"
		unknown   = "mallory-unknown"
	)

	b := c.browser(t)
	state := c.formState(t, b, c.ts.URL+"/start")
	for _, try := range [][2]string{{"alice", attempted}, {typedUser, ""}, {unknown, attempted}} {
		if resp, body := c.postPassword(t, b, state, try[0], try[1]); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %q: %d, want 401: %s", try[0], resp.StatusCode, body)
		}
	}
	if resp, body := c.postPassword(t, b, state, "alice", alicePassword); resp.StatusCode != http.StatusFound {
		t.Fatalf("right password: %d, want 302: %s", resp.StatusCode, body)
	}
	c.signInOK(t, c.browser(t)) // a Google sign-in, for its record

	for name, out := range map[string]string{"logs": logs.String(), "records": buf.String()} {
		for _, secret := range []string{alicePassword, attempted, typedUser, unknown} {
			if strings.Contains(out, secret) {
				t.Errorf("%s contain %q:\n%s", name, secret, out)
			}
		}
	}
	if !strings.Contains(logs.String(), "hostauth: refused password sign-in: username and password do not match") {
		t.Errorf("logs lack the refused password sign-in line:\n%s", logs)
	}
	if n := strings.Count(buf.String(), `"event":"sign_in_refused","provider":"password","reason":"credentials"`); n != 3 {
		t.Errorf("%d credentials refusals recorded, want 3 with no player:\n%s", n, buf)
	}
	wantRecord(t, buf, telemetry.SignIn, rec.ID("password:alice"), `,"provider":"password"`)
	wantRecord(t, buf, telemetry.SignIn, rec.ID(hostauthtest.DefaultAccount.Email), `,"provider":"google"`)
	noEmails(t, buf)
}
