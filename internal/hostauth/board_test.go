package hostauth

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

// board is a connector harness whose service also guards a stand-in board
// with RequireSession: every page answers with the account it acts for,
// /api/ answers "data", and /ws stays open until its request context ends.
type board struct {
	*connector
	// wsOpen gets a value when a guarded /ws request reaches the board;
	// wsClosed gets one when that request's context has ended.
	wsOpen, wsClosed chan struct{}
}

// newBoard serves the sign-in service and the guarded stand-in board at a
// test server's own address, with a fake Google and an injected clock.
func newBoard(t *testing.T) *board {
	t.Helper()
	b := &board{connector: newConnector(t), wsOpen: make(chan struct{}, 4), wsClosed: make(chan struct{}, 4)}
	b.restart(t)
	return b
}

// restart builds a new sign-in service from the same config, as a
// redeploy does, and serves it and the guarded board at the same address.
func (b *board) restart(t *testing.T) {
	t.Helper()
	a, err := New(b.cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ui := http.NewServeMux()
	ui.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		acct, ok := AccountFrom(r.Context())
		if !ok {
			io.WriteString(w, "no account")
			return
		}
		fmt.Fprintf(w, "board for %s %s", acct.ID, acct.Email)
	})
	ui.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "data")
	})
	ui.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		b.wsOpen <- struct{}{}
		<-r.Context().Done()
		b.wsClosed <- struct{}{}
	})
	mux := http.NewServeMux()
	a.Routes(mux)
	mux.Handle("/", a.RequireSession(ui))
	b.a = a
	b.sw.set(mux)
}

// boardText is what the stand-in board page answers for acct.
func boardText(acct hostauthtest.Account) string {
	return "board for " + acct.ID + " " + acct.Email
}

// send makes one request to the playmat with the board cookie (none if
// empty), without following redirects, and returns the response with its
// body read. mod, if set, may change the request first.
func (b *board) send(t *testing.T, method, path, cookie string, mod func(*http.Request)) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, b.ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	if mod != nil {
		mod(req)
	}
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
}

// signInBoard signs a fresh browser in by opening the board page path
// while signed out, checks it lands back on that page signed in, and
// returns the browser and its session cookie value.
func (b *board) signInBoard(t *testing.T, path string) (*http.Client, string) {
	t.Helper()
	br := b.browser(t)
	resp, body := get(t, br, b.ts.URL+path)
	if resp.StatusCode != http.StatusOK || body != boardText(hostauthtest.DefaultAccount) {
		t.Fatalf("opening %s signed out ended with %d %q, want the board page", path, resp.StatusCode, body)
	}
	if got := resp.Request.URL.RequestURI(); got != path {
		t.Fatalf("sign-in ended at %s, want %s", got, path)
	}
	id := b.sessionCookieIn(br)
	if id == "" {
		t.Fatal("sign-in set no session cookie")
	}
	return br, id
}

// checkSentToSignIn checks resp sends a signed-out page request to
// /auth/login with next, and is not cached.
func checkSentToSignIn(t *testing.T, resp *http.Response, next string) {
	t.Helper()
	want := "/auth/login?next=" + url.QueryEscape(next)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != want {
		t.Errorf("got %d to %q, want 302 to %q", resp.StatusCode, resp.Header.Get("Location"), want)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

// liveCount is how many live sign-ins the account holds.
func (b *board) liveCount(id string) int {
	b.a.st.mu.Lock()
	defer b.a.st.mu.Unlock()
	if st := b.a.st.accounts[id]; st != nil {
		return st.live
	}
	return 0
}

// TestBoardSignedOutGoesThroughSignIn shows a signed-out request for a
// board page is sent to /auth/login with the page it asked for, query and
// all, and that a browser following it through Google lands back on that
// page signed in.
func TestBoardSignedOutGoesThroughSignIn(t *testing.T) {
	b := newBoard(t)
	for _, path := range []string{"/", "/?x=1", "/game/live-1?x=1&y=a%20b"} {
		t.Run(path, func(t *testing.T) {
			resp, _ := b.send(t, "GET", path, "", nil)
			checkSentToSignIn(t, resp, path)
			resp, _ = b.send(t, "HEAD", path, "not-a-session", nil)
			checkSentToSignIn(t, resp, path)
			resp, _ = b.send(t, "GET", path, "", func(r *http.Request) { r.Header.Set("Sec-Fetch-Mode", "navigate") })
			checkSentToSignIn(t, resp, path)
			b.signInBoard(t, path)
		})
	}
	resp, _ := b.send(t, "GET", "/", "", nil)
	if got := resp.Header.Get("Location"); got != "/auth/login?next=%2F" {
		t.Errorf("GET / signed out goes to %q, want /auth/login?next=%%2F", got)
	}
}

// TestBoardLoginReturnsOnlyToLocalPaths shows /auth/login returns to the
// local path it is given, and to / for anything that is not one: another
// host, a scheme, a backslash trick, or nothing.
func TestBoardLoginReturnsOnlyToLocalPaths(t *testing.T) {
	b := newBoard(t)
	for next, want := range map[string]string{
		"/?x=1":                "/?x=1",
		"//evil.example":       "/",
		"//evil.example/board": "/",
		"/\\evil.example":      "/",
		"https://evil.example": "/",
		"board":                "/",
		"":                     "/",
	} {
		t.Run(next, func(t *testing.T) {
			br := b.browser(t)
			resp, body := get(t, br, b.ts.URL+"/auth/login?next="+url.QueryEscape(next))
			if resp.Request.URL.Host != b.cfg.Public.Host || resp.Request.URL.RequestURI() != want {
				t.Fatalf("sign-in with next %q ended at %s, want %s on the playmat", next, resp.Request.URL, want)
			}
			if resp.StatusCode != http.StatusOK || body != boardText(hostauthtest.DefaultAccount) {
				t.Fatalf("sign-in with next %q ended with %d %q, want the board page", next, resp.StatusCode, body)
			}
		})
	}
}

// TestBoardSignedOutRefused shows a signed-out live connection, data
// request or non-page request is refused with 401 rather than sent to
// sign in, and the refusal is logged.
func TestBoardSignedOutRefused(t *testing.T) {
	b := newBoard(t)
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	for _, tt := range []struct{ method, path, cookie, mode string }{
		{"GET", "/ws", "", ""},
		{"GET", "/ws", "not-a-session", ""},
		{"GET", "/api/card?def=x", "", ""},
		{"GET", "/api/card?def=x", "not-a-session", ""},
		{"POST", "/", "", ""},
		{"GET", "/favicon.ico", "", "no-cors"},
		{"GET", "/assets/index.js", "not-a-session", "cors"},
	} {
		t.Run(tt.method+" "+tt.path+" "+tt.cookie+" "+tt.mode, func(t *testing.T) {
			logs.Reset()
			resp, body := b.send(t, tt.method, tt.path, tt.cookie, func(r *http.Request) {
				if tt.mode != "" {
					r.Header.Set("Sec-Fetch-Mode", tt.mode)
				}
			})
			if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(body, "sign in required") {
				t.Errorf("got %d %q, want 401 sign in required", resp.StatusCode, body)
			}
			if resp.Header.Get("Location") != "" {
				t.Errorf("refusal redirects to %q", resp.Header.Get("Location"))
			}
			u, _ := url.Parse(tt.path)
			if want := "hostauth: refused " + tt.method + " " + u.Path; !strings.Contains(logs.String(), want) {
				t.Errorf("log %q does not record %q", logs.String(), want)
			}
		})
	}
	select {
	case <-b.wsOpen:
		t.Error("a signed-out /ws request reached the board")
	default:
	}
}

// TestBoardSessionInfo shows /auth/session answers 401 signed out and
// with an unknown cookie, and the signed-in account's email once signed
// in, never cached.
func TestBoardSessionInfo(t *testing.T) {
	b := newBoard(t)
	for _, cookie := range []string{"", "not-a-session"} {
		resp, body := b.send(t, "GET", "/auth/session", cookie, nil)
		if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(body, "signed_out") {
			t.Errorf("/auth/session with cookie %q: %d %q, want 401 signed_out", cookie, resp.StatusCode, body)
		}
	}
	_, id := b.signInBoard(t, "/")
	resp, body := b.send(t, "GET", "/auth/session", id, nil)
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(body) != `{"email":"`+hostauthtest.DefaultAccount.Email+`"}` {
		t.Errorf("/auth/session signed in: %d %q, want 200 with the email", resp.StatusCode, body)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

// TestBoardSessionLastsUntilRestart shows a signed-in browser keeps
// working across reloads, however much later, and after the service restarts its old cookie
// is sent to sign in again, which works.
func TestBoardSessionLastsUntilRestart(t *testing.T) {
	b := newBoard(t)
	br, id := b.signInBoard(t, "/")
	b.clock.Advance(24 * time.Hour)
	for i := 0; i < 3; i++ {
		if resp, body := b.send(t, "GET", "/", id, nil); resp.StatusCode != http.StatusOK || body != boardText(hostauthtest.DefaultAccount) {
			t.Fatalf("reload %d: %d %q, want the board page", i, resp.StatusCode, body)
		}
		if resp, _ := b.send(t, "GET", "/auth/session", id, nil); resp.StatusCode != http.StatusOK {
			t.Fatalf("reload %d: /auth/session %d, want 200", i, resp.StatusCode)
		}
	}
	prompts := len(b.g.Prompts())
	// Back to the fake Google's clock, so its ID tokens are current.
	b.clock.Advance(-24 * time.Hour)

	b.restart(t)

	resp, _ := b.send(t, "GET", "/?x=1", id, nil)
	checkSentToSignIn(t, resp, "/?x=1")
	if resp, _ := b.send(t, "GET", "/auth/session", id, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("/auth/session after restart: %d, want 401", resp.StatusCode)
	}
	resp, body := get(t, br, b.ts.URL+"/?x=1")
	if resp.StatusCode != http.StatusOK || body != boardText(hostauthtest.DefaultAccount) || resp.Request.URL.RequestURI() != "/?x=1" {
		t.Fatalf("signing in again ended at %s with %d %q, want the board page", resp.Request.URL, resp.StatusCode, body)
	}
	if got := len(b.g.Prompts()); got != prompts+1 {
		t.Errorf("signing in again asked Google %d times, want once", got-prompts)
	}
	if again := b.sessionCookieIn(br); again == "" || again == id {
		t.Errorf("signing in again left cookie %q, want a new session", again)
	}
}

// openWS opens a guarded /ws request with the cookie and waits for it to
// reach the board; the returned channel gets the request's error when it
// ends. The request is abandoned when the test ends.
func (b *board) openWS(t *testing.T, cookie string) <-chan error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() {
		req, _ := http.NewRequestWithContext(ctx, "GET", b.ts.URL+"/ws", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		done <- err
	}()
	select {
	case <-b.wsOpen:
	case <-time.After(2 * time.Second):
		t.Fatal("signed-in /ws request did not reach the board")
	}
	return done
}

// TestBoardLogoutNeedsThePlaymat shows sign-out is refused with 403 unless
// the request comes from a page on the public address, by Origin or, with
// no Origin, by Referer; a refused sign-out leaves the session and its
// live connection alone.
func TestBoardLogoutNeedsThePlaymat(t *testing.T) {
	b := newBoard(t)
	_, id := b.signInBoard(t, "/")
	b.openWS(t, id)
	for name, hdr := range map[string]map[string]string{
		"no Origin or Referer":    nil,
		"foreign Origin":          {"Origin": "https://evil.example"},
		"null Origin":             {"Origin": "null"},
		"foreign Referer":         {"Referer": "https://evil.example/"},
		"foreign Origin, Referer": {"Origin": "https://evil.example", "Referer": b.ts.URL + "/"},
		"Origin on another port":  {"Origin": "http://127.0.0.1:1"},
	} {
		t.Run(name, func(t *testing.T) {
			resp, _ := b.send(t, "POST", "/auth/logout", id, func(r *http.Request) {
				for k, v := range hdr {
					r.Header.Set(k, v)
				}
			})
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("sign-out: %d, want 403", resp.StatusCode)
			}
			if resp, _ := b.send(t, "GET", "/auth/session", id, nil); resp.StatusCode != http.StatusOK {
				t.Errorf("session after a refused sign-out: %d, want 200", resp.StatusCode)
			}
		})
	}
	select {
	case <-b.wsClosed:
		t.Error("a refused sign-out closed the live connection")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestBoardLogout shows a sign-out from the playmat (by Origin, or by
// Referer when there is no Origin) ends the session: its open live
// connection ends within 2 seconds, the cookie is expired, the account
// holds one fewer live sign-in, the browser is sent to the signed-out page
// offering to sign in again, and the next page load asks for sign-in.
func TestBoardLogout(t *testing.T) {
	for name, hdr := range map[string]map[string]string{
		"Origin":  {"Origin": "PUBLIC"},
		"Referer": {"Referer": "PUBLIC/game/live-1?x=1"},
	} {
		t.Run(name, func(t *testing.T) {
			b := newBoard(t)
			acct := hostauthtest.DefaultAccount
			_, id := b.signInBoard(t, "/")
			_, other := b.signInBoard(t, "/")
			if got := b.liveCount(acct.ID); got != 2 {
				t.Fatalf("%d live sign-ins before sign-out, want 2", got)
			}
			ws := b.openWS(t, id)

			resp, _ := b.send(t, "POST", "/auth/logout", id, func(r *http.Request) {
				for k, v := range hdr {
					r.Header.Set(k, strings.Replace(v, "PUBLIC", b.ts.URL, 1))
				}
			})
			if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/auth/signed-out" {
				t.Fatalf("sign-out: %d to %q, want 303 to /auth/signed-out", resp.StatusCode, resp.Header.Get("Location"))
			}
			expired := false
			for _, c := range resp.Cookies() {
				expired = expired || (c.Name == sessionCookie && c.MaxAge < 0 && c.Value == "")
			}
			if !expired {
				t.Errorf("sign-out did not expire the session cookie: %v", resp.Header["Set-Cookie"])
			}

			select {
			case <-b.wsClosed:
			case <-time.After(2 * time.Second):
				t.Fatal("live connection still open 2s after sign-out")
			}
			select {
			case <-ws:
			case <-time.After(2 * time.Second):
				t.Fatal("live connection request did not end after sign-out")
			}
			if got := b.liveCount(acct.ID); got != 1 {
				t.Errorf("%d live sign-ins after sign-out, want 1", got)
			}

			resp, _ = b.send(t, "GET", "/", id, nil)
			checkSentToSignIn(t, resp, "/")
			if resp, _ := b.send(t, "GET", "/auth/session", id, nil); resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("/auth/session after sign-out: %d, want 401", resp.StatusCode)
			}
			if resp, body := b.send(t, "GET", "/", other, nil); resp.StatusCode != http.StatusOK || body != boardText(acct) {
				t.Errorf("another browser's session after sign-out: %d %q, want the board page", resp.StatusCode, body)
			}

			resp, body := b.send(t, "GET", "/auth/signed-out", "", nil)
			if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Sign in again") || !strings.Contains(body, `href="/auth/login?next=/"`) {
				t.Errorf("signed-out page: %d %q, want 200 offering to sign in again", resp.StatusCode, body)
			}

			// Signing out again, with the ended session, still lands on the
			// signed-out page and changes nothing.
			resp, _ = b.send(t, "POST", "/auth/logout", id, func(r *http.Request) { r.Header.Set("Origin", b.ts.URL) })
			if resp.StatusCode != http.StatusSeeOther {
				t.Errorf("second sign-out: %d, want 303", resp.StatusCode)
			}
			if got := b.liveCount(acct.ID); got != 1 {
				t.Errorf("%d live sign-ins after a second sign-out, want 1", got)
			}
		})
	}
}

// TestBoardLogoutLastSessionForgetsAccount shows signing out an account's
// only board session leaves it holding no live sign-in.
func TestBoardLogoutLastSessionForgetsAccount(t *testing.T) {
	b := newBoard(t)
	_, id := b.signInBoard(t, "/")
	b.send(t, "POST", "/auth/logout", id, func(r *http.Request) { r.Header.Set("Origin", b.ts.URL) })
	b.a.st.mu.Lock()
	defer b.a.st.mu.Unlock()
	if st := b.a.st.accounts[hostauthtest.DefaultAccount.ID]; st != nil {
		t.Errorf("account still holds %d live sign-ins after its only session signed out", st.live)
	}
}

// TestRequireSessionCarriesAccount shows the guarded board sees the
// account each browser signed in as, and a signed-in data request gets
// through.
func TestRequireSessionCarriesAccount(t *testing.T) {
	b := newBoard(t)
	_, one := b.signInBoard(t, "/")
	two := hostauthtest.Account{ID: "sub-2", Email: "two@example.com"}
	b.g.SetAccount(two)
	br := b.browser(t)
	resp, body := get(t, br, b.ts.URL+"/")
	if resp.StatusCode != http.StatusOK || body != boardText(two) {
		t.Fatalf("second account's board: %d %q, want %q", resp.StatusCode, body, boardText(two))
	}
	if resp, body := b.send(t, "GET", "/", one, nil); body != boardText(hostauthtest.DefaultAccount) {
		t.Errorf("first account's board: %d %q, want %q", resp.StatusCode, body, boardText(hostauthtest.DefaultAccount))
	}
	if resp, body := b.send(t, "GET", "/api/card?def=x", one, nil); resp.StatusCode != http.StatusOK || body != "data" {
		t.Errorf("signed-in data request: %d %q, want 200 data", resp.StatusCode, body)
	}
}

// TestConnectorSignInSignsBoardIn shows a browser that signs in for the
// connector is signed in to the board too: it opens the board without
// another trip to Google.
func TestConnectorSignInSignsBoardIn(t *testing.T) {
	b := newBoard(t)
	clientID, _ := b.registerClient(t, "none", hostauthtest.LoopbackCallback)
	br := b.browser(t)
	loc, err := hostauthtest.FollowSignIn(br, b.authorizeURL(clientID, hostauthtest.LoopbackCallback, nil))
	if err != nil {
		t.Fatal(err)
	}
	if loc.Query().Get("code") == "" {
		t.Fatalf("connector sign-in ended at %s, want a code", loc)
	}
	prompts := len(b.g.Prompts())

	resp, body := get(t, br, b.ts.URL+"/?x=1")
	if resp.StatusCode != http.StatusOK || body != boardText(hostauthtest.DefaultAccount) || resp.Request.URL.RequestURI() != "/?x=1" {
		t.Fatalf("board after connector sign-in: %s %d %q, want the board page", resp.Request.URL, resp.StatusCode, body)
	}
	if got := len(b.g.Prompts()); got != prompts {
		t.Errorf("opening the board asked Google %d more times, want none", got-prompts)
	}
	if resp, _ := b.send(t, "GET", "/auth/session", b.sessionCookieIn(br), nil); resp.StatusCode != http.StatusOK {
		t.Errorf("/auth/session after connector sign-in: %d, want 200", resp.StatusCode)
	}
}
