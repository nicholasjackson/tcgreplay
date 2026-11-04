package hostauth

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

// adminBoard is a board harness whose service also guards a stand-in admin
// page with RequireAdmin: /admin answers "admin for <email>" and counts
// the requests that reach it.
type adminBoard struct {
	*board
	reached int
}

// newAdminBoard serves the board and the guarded admin page with admins
// as the admin list.
func newAdminBoard(t *testing.T, admins ...string) *adminBoard {
	t.Helper()
	b := &adminBoard{board: &board{connector: newConnector(t), wsOpen: make(chan struct{}, 4), wsClosed: make(chan struct{}, 4)}}
	b.cfg.Admins = admins
	a, err := New(b.cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	admin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.reached++
		acct, _ := AccountFrom(r.Context())
		fmt.Fprintf(w, "admin for %s", acct.Email)
	})
	board := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acct, _ := AccountFrom(r.Context())
		fmt.Fprintf(w, "board for %s %s", acct.ID, acct.Email)
	})
	mux := http.NewServeMux()
	a.Routes(mux)
	mux.Handle("/admin", a.RequireAdmin(admin))
	mux.Handle("/admin/", a.RequireAdmin(admin))
	mux.Handle("/", a.RequireSession(board))
	b.a = a
	b.sw.set(mux)
	return b
}

// fromPlaymat sets the Origin a page on the playmat sends.
func (b *adminBoard) fromPlaymat(r *http.Request) { r.Header.Set("Origin", b.ts.URL) }

func TestAdminPassesTheGuardInAnyCase(t *testing.T) {
	b := newAdminBoard(t, " ONE@Example.com ")
	_, cookie := b.signInBoard(t, "/")
	resp, body := b.send(t, "GET", "/admin", cookie, nil)
	if resp.StatusCode != http.StatusOK || body != "admin for one@example.com" {
		t.Errorf("admin GET = %d %q", resp.StatusCode, body)
	}
	resp, body = b.send(t, "POST", "/admin/clear", cookie, b.fromPlaymat)
	if resp.StatusCode != http.StatusOK || body != "admin for one@example.com" {
		t.Errorf("admin POST = %d %q", resp.StatusCode, body)
	}
	if !b.a.IsAdmin(Account{Email: "One@EXAMPLE.com", EmailVerified: true}) {
		t.Error("IsAdmin is case-sensitive")
	}
	if b.a.AdminCount() != 1 {
		t.Errorf("AdminCount = %d", b.a.AdminCount())
	}
}

func TestAdminSignedOut(t *testing.T) {
	b := newAdminBoard(t, "one@example.com")
	resp, _ := b.send(t, "GET", "/admin", "", func(r *http.Request) { r.Header.Set("Sec-Fetch-Mode", "navigate") })
	checkSentToSignIn(t, resp, "/admin")
	for _, tc := range []struct {
		method string
		mod    func(*http.Request)
	}{
		{"POST", b.fromPlaymat},
		{"GET", func(r *http.Request) { r.Header.Set("Sec-Fetch-Mode", "cors") }},
	} {
		resp, body := b.send(t, tc.method, "/admin/clear", "", tc.mod)
		if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(body, "sign in required") {
			t.Errorf("signed-out %s = %d %q, want 401", tc.method, resp.StatusCode, body)
		}
	}
	if b.reached != 0 {
		t.Errorf("admin page reached %d times", b.reached)
	}

	// Through Google sign-in, the browser comes back to the admin page.
	br := b.browser(t)
	resp, body := get(t, br, b.ts.URL+"/admin")
	if resp.StatusCode != http.StatusOK || body != "admin for one@example.com" || resp.Request.URL.Path != "/admin" {
		t.Errorf("signing in from /admin ended at %s with %d %q", resp.Request.URL, resp.StatusCode, body)
	}
}

// wantNotAdmin checks resp is the "Not an admin" page.
func wantNotAdmin(t *testing.T, resp *http.Response, body, email string) {
	t.Helper()
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "Not an admin") || !strings.Contains(body, email) {
		t.Errorf("got %d %q, want 403 Not an admin for %s", resp.StatusCode, body, email)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestAdminRefusesOtherAccounts(t *testing.T) {
	b := newAdminBoard(t, "admin@example.com")
	two := hostauthtest.Account{ID: "sub-2", Email: "two@example.com"}
	b.g.SetAccount(two)
	_, cookie := func() (*http.Client, string) {
		br := b.browser(t)
		get(t, br, b.ts.URL+"/")
		return br, b.sessionCookieIn(br)
	}()
	resp, body := b.send(t, "GET", "/admin", cookie, nil)
	wantNotAdmin(t, resp, body, "two@example.com")
	resp, body = b.send(t, "POST", "/admin/clear", cookie, b.fromPlaymat)
	wantNotAdmin(t, resp, body, "two@example.com")
	if b.reached != 0 {
		t.Errorf("admin page reached %d times", b.reached)
	}
	// The board still works for them.
	if resp, body := b.send(t, "GET", "/", cookie, nil); resp.StatusCode != http.StatusOK || body != boardText(two) {
		t.Errorf("board = %d %q", resp.StatusCode, body)
	}
}

func TestAdminNeedsAVerifiedEmail(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		admin bool
	}{
		{"false", false, false},
		{"string false", "false", false},
		{"absent", nil, false},
		{"string true", "true", true},
		{"true", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newAdminBoard(t, "one@example.com")
			b.g.Claims = func(m map[string]any) {
				if tc.value == nil {
					delete(m, "email_verified")
				} else {
					m["email_verified"] = tc.value
				}
			}
			_, cookie := b.signInBoard(t, "/")
			resp, body := b.send(t, "GET", "/admin", cookie, nil)
			if tc.admin {
				if resp.StatusCode != http.StatusOK {
					t.Errorf("verified admin = %d %q", resp.StatusCode, body)
				}
				return
			}
			wantNotAdmin(t, resp, body, "one@example.com")
			if !strings.Contains(body, "sign out and sign in again") {
				t.Errorf("unverified refusal does not say to sign in again: %q", body)
			}
		})
	}
}

func TestAdminSignInFromBeforeTheVerifiedFlag(t *testing.T) {
	b := newAdminBoard(t, "one@example.com")
	acct := hostauthtest.DefaultAccount
	// A cookie sealed as releases before this one sealed it: no "v".
	old := b.a.st.seal.seal(sealSession, grantClaims{ID: "old-session", Sub: acct.ID, Email: acct.Email, Exp: time.Now().Add(time.Hour).Unix()})
	resp, body := b.send(t, "GET", "/admin", old, nil)
	wantNotAdmin(t, resp, body, acct.Email)
	if resp, body := b.send(t, "GET", "/", old, nil); resp.StatusCode != http.StatusOK || body != boardText(acct) {
		t.Errorf("old sign-in on the board = %d %q, want the board", resp.StatusCode, body)
	}
	if b.reached != 0 {
		t.Errorf("admin page reached %d times", b.reached)
	}
}

func TestAdminChangesMustComeFromThePlaymat(t *testing.T) {
	b := newAdminBoard(t, "one@example.com")
	_, cookie := b.signInBoard(t, "/")
	for name, mod := range map[string]func(*http.Request){
		"foreign origin":  func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") },
		"foreign referer": func(r *http.Request) { r.Header.Set("Referer", "https://evil.example/page") },
		"no origin":       nil,
		"null origin":     func(r *http.Request) { r.Header.Set("Origin", "null") },
	} {
		resp, body := b.send(t, "POST", "/admin/clear", cookie, mod)
		if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "must come from the playmat") {
			t.Errorf("%s: %d %q, want 403", name, resp.StatusCode, body)
		}
	}
	if b.reached != 0 {
		t.Errorf("admin page reached %d times", b.reached)
	}
	// A page read needs no origin.
	if resp, _ := b.send(t, "GET", "/admin", cookie, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("GET without origin = %d", resp.StatusCode)
	}
}

func TestNoAdminsMeansNobody(t *testing.T) {
	for _, admins := range [][]string{nil, {"", "  "}} {
		b := newAdminBoard(t, admins...)
		_, cookie := b.signInBoard(t, "/")
		resp, body := b.send(t, "GET", "/admin", cookie, nil)
		wantNotAdmin(t, resp, body, "one@example.com")
		if b.a.AdminCount() != 0 {
			t.Errorf("AdminCount = %d", b.a.AdminCount())
		}
	}
}
