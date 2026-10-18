package hostauth

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

// capA and capB are the two accounts the user cap tests sign in: A takes
// the only place, B is the newcomer.
var (
	capA = hostauthtest.DefaultAccount
	capB = hostauthtest.Account{ID: "sub-2", Email: "two@example.com"}
)

// fakePlaces is an in-test Places: it hands out up to max places (0 for
// no cap) to distinct accounts, refuses a newcomer beyond that with
// BusyError(max) while an account already holding one always gets in,
// frees a place on Release, and records every call.
type fakePlaces struct {
	mu       sync.Mutex
	max      int
	held     map[string]bool
	admits   []string
	releases []string
	// err, if set, is what Admit returns for every account instead.
	err error
}

func newFakePlaces(max int) *fakePlaces {
	return &fakePlaces{max: max, held: map[string]bool{}}
}

func (p *fakePlaces) Admit(acct Account) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.admits = append(p.admits, acct.ID)
	if p.err != nil {
		return p.err
	}
	if !p.held[acct.ID] && p.max > 0 && len(p.held) >= p.max {
		return BusyError(p.max)
	}
	p.held[acct.ID] = true
	return nil
}

func (p *fakePlaces) Release(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.releases = append(p.releases, id)
	delete(p.held, id)
}

// holds reports whether the account holds a place.
func (p *fakePlaces) holds(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.held[id]
}

// released returns the ids Release was called with, in order.
func (p *fakePlaces) released() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.releases)
}

// admitted returns the ids Admit was called with, in order.
func (p *fakePlaces) admitted() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.admits)
}

// capped is the board harness with its sign-in service capped by a fake
// Places.
type capped struct {
	*board
	places *fakePlaces
}

// newCapped is the board harness with its sign-in service's places kept
// by a fakePlaces of max places.
func newCapped(t *testing.T, max int) *capped {
	t.Helper()
	c := &capped{board: newBoard(t)}
	c.restartPlaces(t, max)
	return c
}

// restartPlaces restarts the sign-in service with fresh places, as a
// redeploy does in hosted mode, where the places live in memory too.
func (c *capped) restartPlaces(t *testing.T, max int) {
	t.Helper()
	c.places = newFakePlaces(max)
	c.cfg.Places = c.places
	c.restart(t)
}

// checkReleased fails unless Release has been called exactly with want.
func (c *capped) checkReleased(t *testing.T, when string, want ...string) {
	t.Helper()
	if got := c.places.released(); !slices.Equal(got, want) {
		t.Errorf("%s: Release calls %q, want %q", when, got, want)
	}
}

// captureHostLog redirects the standard logger for the rest of the test.
func captureHostLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &logs
}

// boardAs signs g's current account in to the board through /auth/login
// in a fresh browser and returns the final response, its body and the
// browser.
func (b *board) boardAs(t *testing.T) (*http.Response, string, *http.Client) {
	t.Helper()
	br := b.browser(t)
	resp, body := get(t, br, b.ts.URL+"/auth/login?next="+url.QueryEscape("/"))
	return resp, body, br
}

// boardOK signs acct (g's current account) in to the board through
// /auth/login and checks it lands on the board as that account; it
// returns the session cookie value.
func (b *board) boardOK(t *testing.T, acct hostauthtest.Account) string {
	t.Helper()
	resp, body, br := b.boardAs(t)
	if resp.StatusCode != http.StatusOK || body != boardText(acct) {
		t.Fatalf("board sign-in as %s: %d %q, want the board page", acct.Email, resp.StatusCode, body)
	}
	id := b.sessionCookieIn(br)
	if id == "" {
		t.Fatalf("board sign-in as %s set no session cookie", acct.Email)
	}
	return id
}

// connectorAs runs g's current account through the connector's
// authorization request for clientID in a fresh browser, following it
// through Google, and returns the final response, its body and the
// browser. A sign-in that gets a code stops at the client's loopback
// callback with a 302.
func (b *board) connectorAs(t *testing.T, clientID string) (*http.Response, string, *http.Client) {
	t.Helper()
	br := b.browser(t)
	resp, body := get(t, br, b.authorizeURL(clientID, hostauthtest.LoopbackCallback, nil))
	return resp, body, br
}

// checkBusy checks a sign-in ended on the busy page naming the cap, with
// no session cookie in br and no redirect to an MCP client's callback.
func (b *board) checkBusy(t *testing.T, flow string, resp *http.Response, body string, br *http.Client, maxUsers int) {
	t.Helper()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("%s: status %d at %s, want 503", flow, resp.StatusCode, resp.Request.URL)
	}
	if loc := resp.Header.Get("Location"); loc != "" {
		t.Errorf("%s: refused sign-in redirects to %q", flow, loc)
	}
	if resp.Request.URL.Host != b.cfg.Public.Host || resp.Request.URL.Path != googleCallbackPath {
		t.Errorf("%s: ended at %s, want the playmat's Google callback", flow, resp.Request.URL)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("%s: Content-Type %q, want an HTML page", flow, ct)
	}
	for _, want := range []string{"tcgreplay is busy", fmt.Sprintf("as many signed-in users as it can (%d)", maxUsers), "try again later"} {
		if !strings.Contains(body, want) {
			t.Errorf("%s: busy page does not say %q: %q", flow, want, body)
		}
	}
	if strings.Contains(body, "code=") {
		t.Errorf("%s: busy page carries a code: %q", flow, body)
	}
	if id := b.sessionCookieIn(br); id != "" {
		t.Errorf("%s: refused sign-in set a session cookie", flow)
	}
}

// accountCount is how many accounts hold a live sign-in.
func (b *board) accountCount() int {
	b.a.st.mu.Lock()
	defer b.a.st.mu.Unlock()
	return len(b.a.st.accounts)
}

// logout signs the browser holding session id out of the board from the
// playmat.
func (b *board) logout(t *testing.T, id string) {
	t.Helper()
	resp, _ := b.send(t, "POST", "/auth/logout", id, func(r *http.Request) { r.Header.Set("Origin", b.ts.URL) })
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("sign-out: %d, want 303", resp.StatusCode)
	}
}

// TestCapRefusesNewAccountBothFlows shows that with the places provider
// full (a cap of 1, A holding it), B is shown the busy page whichever way
// it signs in: through the board's /auth/login and through a connector's
// /authorize, where its browser is not sent back to the client with a
// code. The provider is asked in both flows, neither sets a session
// cookie, B holds no live sign-in, and each refusal is logged.
func TestCapRefusesNewAccountBothFlows(t *testing.T) {
	logs := captureHostLog(t)
	b := newCapped(t, 1)
	b.boardOK(t, capA)

	b.g.SetAccount(capB)
	logs.Reset()
	resp, body, br := b.boardAs(t)
	b.checkBusy(t, "board", resp, body, br, 1)
	if want := "hostauth: refused sign-in for " + capB.Email; !strings.Contains(logs.String(), want) {
		t.Errorf("board refusal: log %q does not record %q", logs.String(), want)
	}
	if !strings.Contains(logs.String(), "the server is busy") {
		t.Errorf("board refusal: log %q does not say the server is busy", logs.String())
	}

	clientID, _ := b.registerClient(t, "none", hostauthtest.LoopbackCallback)
	logs.Reset()
	resp, body, br = b.connectorAs(t, clientID)
	b.checkBusy(t, "connector", resp, body, br, 1)
	if want := "hostauth: refused sign-in for " + capB.Email; !strings.Contains(logs.String(), want) {
		t.Errorf("connector refusal: log %q does not record %q", logs.String(), want)
	}
	if _, err := hostauthtest.FollowSignIn(b.browser(t), b.authorizeURL(clientID, hostauthtest.LoopbackCallback, nil)); err == nil {
		t.Error("connector sign-in for B over the cap reached the client's callback")
	}

	if got, want := b.places.admitted(), []string{capA.ID, capB.ID, capB.ID, capB.ID}; !slices.Equal(got, want) {
		t.Errorf("Admit calls %q, want %q (once per Google sign-in, both flows)", got, want)
	}
	if b.places.holds(capB.ID) {
		t.Error("refused account holds a place")
	}
	if got := b.liveCount(capB.ID); got != 0 {
		t.Errorf("refused account holds %d live sign-ins, want 0", got)
	}
	if got := b.accountCount(); got != 1 {
		t.Errorf("%d accounts signed in, want 1", got)
	}
	b.checkReleased(t, "after the refusals")
}

// TestCapLetsSignedInAccountBackIn shows that at the cap, the account
// already holding a place can sign in again in other browsers, through
// the board and through a connector all the way to tokens, and that none
// of it releases its place.
func TestCapLetsSignedInAccountBackIn(t *testing.T) {
	b := newCapped(t, 1)
	b.boardOK(t, capA)

	b.boardOK(t, capA)
	clientID, _ := b.registerClient(t, "none", hostauthtest.LoopbackCallback)
	code := b.code(t, clientID, hostauthtest.LoopbackCallback)
	if resp, m := b.token(t, exchange(clientID, code, hostauthtest.LoopbackCallback, testVerifier), "", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("A's code exchange at the cap: %d %v", resp.StatusCode, m)
	}
	if tok := hostauthtest.Connect(t, b.ts.URL, b.ts.URL, b.g); tok.AccessToken == "" || tok.RefreshToken == "" {
		t.Errorf("A's second connector sign-in at the cap got tokens %+v", tok)
	}

	if got := b.accountCount(); got != 1 {
		t.Errorf("%d accounts signed in after A's repeat sign-ins, want 1", got)
	}
	// Two board sessions, two connector browsers' sessions and two
	// refresh tokens.
	if got := b.liveCount(capA.ID); got != 6 {
		t.Errorf("A holds %d live sign-ins, want 6", got)
	}
	if !b.places.holds(capA.ID) {
		t.Error("A holds no place after its repeat sign-ins")
	}
	b.checkReleased(t, "after A's repeat sign-ins")
}

// TestCapPlaceFreedAtLastSignOut shows a place is released only when the
// account's last sign-in ends: with A signed in to the board in two
// browsers, Release is not called at the first sign-out, so B stays busy,
// and is called once, for A, at the second, after which B gets in and A
// is the one kept out, in both flows.
func TestCapPlaceFreedAtLastSignOut(t *testing.T) {
	b := newCapped(t, 1)
	one := b.boardOK(t, capA)
	two := b.boardOK(t, capA)

	b.g.SetAccount(capB)
	resp, body, br := b.boardAs(t)
	b.checkBusy(t, "B before A signs out", resp, body, br, 1)

	b.logout(t, one)
	b.checkReleased(t, "after A's first sign-out")
	resp, body, br = b.boardAs(t)
	b.checkBusy(t, "B after A's first sign-out", resp, body, br, 1)

	b.logout(t, two)
	b.checkReleased(t, "after A's last sign-out", capA.ID)
	if got := b.accountCount(); got != 0 {
		t.Fatalf("%d accounts signed in after A's last sign-out, want 0", got)
	}
	b.boardOK(t, capB)

	b.g.SetAccount(capA)
	resp, body, br = b.boardAs(t)
	b.checkBusy(t, "A after B took the place", resp, body, br, 1)
	clientID, _ := b.registerClient(t, "none", hostauthtest.LoopbackCallback)
	resp, body, br = b.connectorAs(t, clientID)
	b.checkBusy(t, "A's connector after B took the place", resp, body, br, 1)
}

// TestCapRefreshTokenHoldsPlace shows a connector's refresh token holds a
// live sign-in: after A signs a connector in and then signs out of the
// board, Release is not called and B is still busy, and a refresh, which
// rotates the token, does not release either. A restart, with fresh
// places, lets B in.
func TestCapRefreshTokenHoldsPlace(t *testing.T) {
	b := newCapped(t, 1)
	clientID, _ := b.registerClient(t, "none", hostauthtest.LoopbackCallback)
	resp, _, br := b.connectorAs(t, clientID)
	loc, err := resp.Location()
	if resp.StatusCode != http.StatusFound || err != nil || loc.Query().Get("code") == "" {
		t.Fatalf("A's connector sign-in: %d to %v, want a code", resp.StatusCode, loc)
	}
	tr, m := b.token(t, exchange(clientID, loc.Query().Get("code"), hostauthtest.LoopbackCallback, testVerifier), "", "")
	if tr.StatusCode != http.StatusOK {
		t.Fatalf("A's code exchange: %d %v", tr.StatusCode, m)
	}
	refresh, _ := m["refresh_token"].(string)

	b.logout(t, b.sessionCookieIn(br))
	if got := b.liveCount(capA.ID); got != 1 {
		t.Fatalf("A holds %d live sign-ins after signing out of the board, want 1 (the refresh token)", got)
	}
	b.checkReleased(t, "with A's refresh token live")
	b.g.SetAccount(capB)
	resp, body, bb := b.boardAs(t)
	b.checkBusy(t, "B with A's refresh token live", resp, body, bb, 1)

	tr, m = b.token(t, refreshForm(clientID, refresh), "", "")
	if tr.StatusCode != http.StatusOK {
		t.Fatalf("A's refresh at the cap: %d %v", tr.StatusCode, m)
	}
	if got := b.liveCount(capA.ID); got != 1 {
		t.Errorf("A holds %d live sign-ins after a refresh, want 1", got)
	}
	b.checkReleased(t, "after A's refresh")
	resp, body, bb = b.boardAs(t)
	b.checkBusy(t, "B after A's refresh", resp, body, bb, 1)

	b.restartPlaces(t, 1)
	b.boardOK(t, capB)
}

// TestCapCodeHoldsPlace shows a connector code not yet exchanged holds a
// live sign-in: at a cap of 1, A gets a code and signs out of the board
// before its client exchanges it; Release is not called and B is still
// busy, and the exchange does not release either. A code left to expire
// ends A's last sign-in, so Release is called and B gets in.
func TestCapCodeHoldsPlace(t *testing.T) {
	b := newCapped(t, 1)
	id, _ := b.registerClient(t, "none", hostauthtest.LoopbackCallback)

	resp, _, br := b.connectorAs(t, id)
	loc, err := resp.Location()
	if resp.StatusCode != http.StatusFound || err != nil || loc.Query().Get("code") == "" {
		t.Fatalf("A's authorize: %d %v, want a redirect with a code", resp.StatusCode, err)
	}
	code := loc.Query().Get("code")
	b.logout(t, b.sessionCookieIn(br))
	b.checkReleased(t, "while A's code is pending")

	b.g.SetAccount(capB)
	resp, body, brB := b.boardAs(t)
	b.checkBusy(t, "B while A's code is pending", resp, body, brB, 1)

	tr, m := b.token(t, exchange(id, code, hostauthtest.LoopbackCallback, testVerifier), "", "")
	if tr.StatusCode != http.StatusOK {
		t.Fatalf("A's exchange: %d %v", tr.StatusCode, m)
	}
	if n := b.accountCount(); n != 1 {
		t.Fatalf("%d accounts signed in after A's exchange, want 1", n)
	}
	b.checkReleased(t, "after A's exchange")
	resp, body, brB = b.boardAs(t)
	b.checkBusy(t, "B after A's exchange", resp, body, brB, 1)

	// A fresh service: A gets a code it never exchanges, then signs out.
	b.restartPlaces(t, 1)
	b.g.SetAccount(capA)
	id, _ = b.registerClient(t, "none", hostauthtest.LoopbackCallback)
	_, _, br = b.connectorAs(t, id)
	b.logout(t, b.sessionCookieIn(br))
	b.checkReleased(t, "with A's unexchanged code live")
	b.clock.Advance(codeLifetime + time.Minute)
	b.g.SetAccount(capB)
	b.boardOK(t, capB)
	b.checkReleased(t, "after A's code expired", capA.ID)
}

// TestStoreReleaseAtLastSignIn shows the store tells onRelease when, and
// only when, an account's live sign-ins (board sessions, pending codes and
// refresh tokens) reach zero, once per time: not while a second session,
// a code or a refresh token remains, and not for another account. It no
// longer refuses anyone itself: 50 accounts get sessions.
func TestStoreReleaseAtLastSignIn(t *testing.T) {
	s := newStore()
	var released []string
	s.onRelease = func(id string) { released = append(released, id) }
	check := func(when string, want ...string) {
		t.Helper()
		if !slices.Equal(released, want) {
			t.Errorf("%s: released %q, want %q", when, released, want)
		}
	}
	now := time.Now()
	one := Account{ID: "sub-1", Email: "1@example.com"}
	two := Account{ID: "sub-2", Email: "2@example.com"}

	first := s.addSession(now, one)
	second := s.addSession(now, one)
	other := s.addSession(now, two)
	s.addCode(now, &authCode{clientID: "c", account: one})
	s.endSession(first)
	s.endSession(second)
	check("with sub-1's code live")
	s.mu.Lock()
	s.sweep(now.Add(codeLifetime + time.Second))
	s.mu.Unlock()
	check("after sub-1's code expired", one.ID)

	// A code taken for exchange passes its hold to the tokens issued.
	code := s.addCode(now, &authCode{clientID: "c", account: two})
	s.endSession(other)
	c := s.takeCode(code)
	if c == nil {
		t.Fatal("takeCode found no code")
	}
	_, refresh := s.issueTokens(now, "c", two)
	check("with sub-2's refresh token live", one.ID)
	acct, ok := s.takeRefresh("c", refresh)
	if !ok || acct.ID != two.ID {
		t.Fatalf("takeRefresh: %v %v", acct, ok)
	}
	s.issueTokens(now, "c", two)
	check("after sub-2's refresh", one.ID)

	// A code that fails its exchange gives its hold back.
	three := Account{ID: "sub-3"}
	code = s.addCode(now, &authCode{clientID: "c", account: three})
	s.takeCode(code)
	s.release(three.ID)
	check("after sub-3's failed exchange", one.ID, three.ID)

	for i := 0; i < 50; i++ {
		if id := s.addSession(now, Account{ID: fmt.Sprintf("many-%d", i)}); id == "" {
			t.Fatalf("account %d got no session", i+1)
		}
	}
	if got := len(s.accounts); got != 51 {
		t.Errorf("%d accounts hold live sign-ins, want 51", got)
	}
}

// TestCapSignInError shows signIn asks Places first and passes its
// refusal on: over the cap it wraps ErrBusy, says the server is busy,
// names the cap and tells the user to try again later, and sets no
// cookie or session.
func TestCapSignInError(t *testing.T) {
	places := newFakePlaces(2)
	a, err := New(Config{
		Public:         &url.URL{Scheme: "https", Host: "playmat.example"},
		GoogleClientID: "client-id",
		GoogleSecret:   "client-secret",
		Places:         places,
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if err := a.signIn(httptest.NewRecorder(), Account{ID: fmt.Sprintf("sub-%d", i)}); err != nil {
			t.Fatalf("account %d under the cap: %v", i, err)
		}
	}
	rec := httptest.NewRecorder()
	err = a.signIn(rec, Account{ID: "sub-3", Email: "three@example.com"})
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("third account at a cap of 2: %v, want ErrBusy", err)
	}
	for _, want := range []string{"the server is busy", "(2)", "try again later"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if c := rec.Header().Values("Set-Cookie"); len(c) != 0 {
		t.Errorf("refused sign-in set cookies %v", c)
	}
	if n := len(a.st.sessions); n != 2 {
		t.Errorf("%d sessions after a refusal, want 2", n)
	}
}

// TestBusyError shows BusyError matches ErrBusy and carries the one
// wording, naming the cap.
func TestBusyError(t *testing.T) {
	err := BusyError(7)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("BusyError(7) does not match ErrBusy: %v", err)
	}
	if errors.Is(err, ErrRateLimited) {
		t.Errorf("BusyError(7) matches ErrRateLimited")
	}
	const want = "the server is busy: it already has the most signed-in users it serves at once (7); try again later"
	if err.Error() != want {
		t.Errorf("BusyError(7) = %q, want %q", err, want)
	}
	if !errors.Is(fmt.Errorf("wrapped: %w", err), ErrBusy) {
		t.Error("a wrapped BusyError does not match ErrBusy")
	}
}

// TestCapPlacesOtherError shows a refusal from Places that is not the
// busy error ends the Google sign-in on a 503 error page, not the busy
// page, with no session cookie.
func TestCapPlacesOtherError(t *testing.T) {
	b := newCapped(t, 0)
	b.places.err = errors.New("places unavailable")
	resp, body, br := b.boardAs(t)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503", resp.StatusCode)
	}
	if strings.Contains(body, "tcgreplay is busy") {
		t.Errorf("a non-busy refusal shows the busy page: %q", body)
	}
	if !strings.Contains(body, "Sign-in could not be completed") {
		t.Errorf("error page does not say the sign-in could not be completed: %q", body)
	}
	if id := b.sessionCookieIn(br); id != "" {
		t.Error("refused sign-in set a session cookie")
	}
}

// TestCapNilPlacesIsOff shows no Places means no cap: 50 accounts sign in
// through signIn, and 5 through the board flow over HTTP.
func TestCapNilPlacesIsOff(t *testing.T) {
	a, err := New(Config{
		Public:         &url.URL{Scheme: "https", Host: "playmat.example"},
		GoogleClientID: "client-id",
		GoogleSecret:   "client-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.st.onRelease != nil {
		t.Error("no Places but the store has a release hook")
	}
	for i := 0; i < 50; i++ {
		if err := a.signIn(httptest.NewRecorder(), Account{ID: fmt.Sprintf("sub-%d", i)}); err != nil {
			t.Fatalf("signIn: account %d refused with no cap: %v", i+1, err)
		}
	}

	b := newBoard(t)
	for i := 0; i < 5; i++ {
		acct := hostauthtest.Account{ID: fmt.Sprintf("sub-%d", i), Email: fmt.Sprintf("%d@example.com", i)}
		b.g.SetAccount(acct)
		id := b.boardOK(t, acct)
		b.logout(t, id) // no Places to tell: must not panic
		b.boardOK(t, acct)
	}
	if got := b.accountCount(); got != 5 {
		t.Errorf("%d accounts signed in with no cap, want 5", got)
	}
}
