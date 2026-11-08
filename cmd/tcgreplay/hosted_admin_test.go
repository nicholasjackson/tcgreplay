package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cacheadmin"
	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
	"github.com/nicholasjackson/tcgreplay/internal/spaces"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/webui"
)

// adminDeck is a deck the admin source resolves: two Pokémon of two sets
// and Basic Energy.
const adminDeck = `Pokémon: 8
4 Bulbasaur MEG 1
4 Pineco SVI 1

Energy: 52
52 Basic {G} Energy SVE 5

Total Cards: 60`

// adminHosted is hosted mode with the admin page, wired as runHosted
// wires it, over the stand-in Spaces and CDN API and card sets published
// to a local server.
type adminHosted struct {
	*hostedServer
	sets  *cardsetHost       // the published layout
	srv   *spacestest.Server // nil without the shared store
	cdn   *spacestest.CDN    // nil without a token
	admin *cacheadmin.Service
}

// startHostedAdmin starts hosted mode with one@example.com (the fake
// Google's default account) as the admin, its catalogue loaded from
// adminSets published to a local server, the shared store when shared,
// and a CDN token when token. The image cache's requests for TCGdex's
// image host go to a local stand-in.
func startHostedAdmin(t *testing.T, shared, token bool) *adminHosted {
	t.Helper()
	g := hostauthtest.NewGoogle(t)
	t.Setenv("TCGREPLAY_GOOGLE_CLIENT_SECRET", g.ClientSecret)
	t.Setenv("TCGREPLAY_ADMINS", "One@Example.com")
	h := &adminHosted{sets: publishCardsets(t, adminSets()...)}
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", googleClientID: g.ClientID, cardsetsURL: h.sets.URL}
	if shared {
		h.srv = spacestest.New(t)
		setSpacesEnv(t, h.srv.Config())
		// Copies already in the bucket: one image of each Pokémon's set.
		for _, rel := range []string{"en/me/me01/001", "en/sv/sv1/001"} {
			for _, f := range cards.ImageFiles {
				h.srv.SetObject(cards.ImagePrefix+rel+"/"+f, []byte("old "+rel+f))
			}
		}
	} else {
		clearSpacesEnv(t)
	}
	t.Setenv("TCGREPLAY_SPACES_CDN_TOKEN", "")
	if token {
		const tok = "dop_v1_admin_test"
		t.Setenv("TCGREPLAY_SPACES_CDN_TOKEN", tok)
		origin := "nowhere"
		if shared {
			origin = spaces.NewCDN(h.srv.Config(), tok).Origin
		}
		h.cdn = spacestest.NewCDN(t, origin, tok)
		c.cdnAPI = h.cdn.URL
	}
	u, err := hostedPublicURL(testPublic, ":8080")
	if err != nil {
		t.Fatal(err)
	}
	images := hostedImages(c)
	loader := newCatalog(c)
	reg, mcpSrv, a, err := newHosted(c, loader, images, u, g.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	loader.Start(ctx, 5*time.Second)
	if loader.Current().Cached("me01-001") == nil {
		t.Fatal("the published sets did not load")
	}
	if images != nil {
		images.HTTP = newReroute(t, "", newImageHost(t).URL).client()
		go images.Run(ctx)
	}
	h.admin = hostedAdmin(loader, images)
	go h.admin.Run(ctx)
	ts := httptest.NewServer(hostedHandler(mcpSrv, reg, a, h.admin))
	t.Cleanup(ts.Close)
	h.hostedServer = &hostedServer{ts: ts, g: g, reg: reg, public: u.String(), cat: loader}
	return h
}

// imageKeysOnly checks the bucket holds image copies alone: card data
// never goes to the shared store.
func (h *adminHosted) imageKeysOnly(t *testing.T) {
	t.Helper()
	for _, k := range h.srv.Keys("") {
		if !strings.HasPrefix(k, cards.ImagePrefix) {
			t.Errorf("bucket holds %s, which is not an image copy", k)
		}
	}
}

// check posts the check for published card sets as the browser holding
// cookie, and returns the response.
func (h *adminHosted) check(t *testing.T, cookie *http.Cookie) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, h.ts.URL+"/admin/check", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	fromPlaymat(req)
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// fromPlaymat sets the Origin a page on the playmat sends.
func fromPlaymat(r *http.Request) {
	r.Header.Set("Origin", testPublic)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
}

// clear posts the clear form as the browser holding cookie, and returns
// the response.
func (h *adminHosted) clear(t *testing.T, cookie *http.Cookie, form url.Values) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, h.ts.URL+"/admin/clear", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	fromPlaymat(req)
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// clearAndWait clears as the admin and waits for the page to stop showing
// the clear as running, then returns the page.
func (h *adminHosted) clearAndWait(t *testing.T, cookie *http.Cookie, form url.Values) string {
	t.Helper()
	if resp, body := h.clear(t, cookie, form); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/admin" {
		t.Fatalf("clear %v = %d %q", form, resp.StatusCode, body)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, body := getWith(t, h.ts.URL+"/admin", cookie, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("admin page = %d %q", resp.StatusCode, body)
		}
		if !strings.Contains(body, "Clearing now") {
			return body
		}
		if time.Now().After(deadline) {
			t.Fatal("clear still running after 10s")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestHostedAdminClearsEverything(t *testing.T) {
	logs := captureSyncLog(t)
	h := startHostedAdmin(t, true, true)
	cookie := h.cookie(t)
	resp, body := getWith(t, h.ts.URL+"/admin", cookie, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Signed in as one@example.com") {
		t.Fatalf("admin page = %d %q", resp.StatusCode, body)
	}

	// A game open on the board.
	cs := h.connect(t)
	callTool(t, cs, "new_game", map[string]any{"deck_p1": adminDeck, "deck_p2": adminDeck, "id": "live-1"})
	callTool(t, cs, "set_mode", map[string]any{"mode": "review"})
	h.imageKeysOnly(t)

	page := h.clearAndWait(t, cookie, url.Values{"all": {"1"}})
	if keys := h.srv.Keys(""); len(keys) != 0 {
		t.Errorf("bucket left with %v", keys)
	}
	if got := h.cdn.Purges(); len(got) != 1 || !slices.Equal(got[0], []string{"images/*"}) {
		t.Errorf("purges = %v, want [[images/*]]", got)
	}
	for _, want := range []string{"Images for everything by one@example.com", "image copies removed", "CDN purged (1 paths)"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}

	// The open game still shows and plays.
	g, err := h.manager(hostauthtest.DefaultAccount).Current()
	if err != nil {
		t.Fatal(err)
	}
	for iid, c := range g.State().Cards {
		if c.Def == "" || g.DB().Cached(c.Def) == nil {
			t.Errorf("%s (%s) lost its card", iid, c.Name)
		}
	}
	hand := claudeState(t, cs).Sides[state.P2].Hand
	if len(hand) == 0 {
		t.Fatal("no cards in hand")
	}
	callTool(t, cs, "move", map[string]any{"seat": "p2", "iid": string(hand[0].IID), "to": "discard"})
	if text := callTool(t, cs, "card_text", map[string]any{"card": "me01-001"}); text != bulbasaurText {
		t.Errorf("card text after the clear = %q, want %q", text, bulbasaurText)
	}

	// The next game shows its cards.
	callTool(t, cs, "new_game", map[string]any{"deck_p1": adminDeck, "deck_p2": adminDeck, "id": "live-2"})
	g, err = h.manager(hostauthtest.DefaultAccount).Current()
	if err != nil {
		t.Fatal(err)
	}
	for iid, c := range g.State().Cards {
		if c.Def == "" || g.DB().Cached(c.Def) == nil {
			t.Errorf("new game's %s (%s) has no card", iid, c.Name)
		}
	}
	h.imageKeysOnly(t)
	out := logs.String()
	for _, want := range []string{"cache admin: one@example.com started clearing images for everything", "cache admin: cleared images for everything for one@example.com", "card images: CDN purge on clear: on"} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q", want)
		}
	}
	if strings.Contains(out, "dop_v1_admin_test") {
		t.Error("the log shows the CDN token")
	}
}

func TestHostedAdminClearsOneSetByCodeAndByID(t *testing.T) {
	captureSyncLog(t)
	for _, name := range []string{"MEG", "me01"} {
		t.Run(name, func(t *testing.T) {
			h := startHostedAdmin(t, true, true)
			cookie := h.cookie(t)
			page := h.clearAndWait(t, cookie, url.Values{"set": {name}})
			// Only MEG's image copies go.
			want := []string{"images/en/sv/sv1/001/high.webp", "images/en/sv/sv1/001/low.webp"}
			if got := h.srv.Keys(""); !slices.Equal(got, want) {
				t.Errorf("bucket = %v, want %v", got, want)
			}
			if got := h.cdn.Purges(); len(got) != 1 || !slices.Equal(got[0], []string{"images/en/me/me01/*"}) {
				t.Errorf("purges = %v", got)
			}
			if !strings.Contains(page, "Images for Mega Evolution (me01) by one@example.com") {
				t.Errorf("page:\n%s", page)
			}
		})
	}
}

func TestHostedAdminRefusesEveryoneElse(t *testing.T) {
	captureSyncLog(t)
	h := startHostedAdmin(t, true, true)
	before := h.sets.version()
	// A publish only an admin's check (or the interval) would pick up.
	h.sets.publish(fixtureSet("sv2", "sv", "PAL", "Paldea Evolved", "2023-06-09",
		fixturePrint{local: "013", name: "Sprigatito", hp: 70, attack: "Scratch"}))
	keys := h.srv.Keys("")
	check := func(what string) {
		t.Helper()
		if got := h.srv.Keys(""); !slices.Equal(got, keys) {
			t.Errorf("%s: bucket changed to %v", what, got)
		}
		if n := h.srv.Requests("POST", "?delete"); n != 0 {
			t.Errorf("%s: %d delete requests", what, n)
		}
		if len(h.cdn.Purges()) != 0 {
			t.Errorf("%s: purged %v", what, h.cdn.Purges())
		}
	}

	// Signed out.
	resp, _ := getWith(t, h.ts.URL+"/admin", nil, func(r *http.Request) { r.Header.Set("Sec-Fetch-Mode", "navigate") })
	if resp.StatusCode != http.StatusFound || !strings.HasPrefix(resp.Header.Get("Location"), "/auth/login?next=") {
		t.Errorf("signed-out page = %d to %q, want sign-in", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp, _ := h.clear(t, nil, url.Values{"all": {"1"}}); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("signed-out clear = %d, want 401", resp.StatusCode)
	}
	check("signed out")

	// Signed in, not an admin.
	h.g.SetAccount(hostauthtest.Account{ID: "sub-2", Email: "two@example.com"})
	other := h.cookie(t)
	if resp, body := getWith(t, h.ts.URL+"/admin", other, nil); resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "Not an admin") {
		t.Errorf("non-admin page = %d", resp.StatusCode)
	}
	if resp, _ := h.clear(t, other, url.Values{"all": {"1"}}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("non-admin clear = %d, want 403", resp.StatusCode)
	}
	// Nor may they ask for a check for published card sets.
	for name, cookie := range map[string]*http.Cookie{"signed out": nil, "non-admin": other} {
		if resp, _ := h.check(t, cookie); resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s check = %d, want refused", name, resp.StatusCode)
		}
	}
	if _, ok := h.admin.LastCheck(); ok {
		t.Error("a check was run")
	}
	if got := h.cat.Current().Version(); got != before {
		t.Errorf("catalogue version %s after refused checks, want %s", got, before)
	}
	check("non-admin")
	if _, ok := h.admin.Status(); ok {
		t.Error("a clear was started")
	}
}

func TestHostedAdminWithoutTheSharedStore(t *testing.T) {
	logs := captureSyncLog(t)
	h := startHostedAdmin(t, false, true)
	if !strings.Contains(logs.String(), "card images: TCGREPLAY_SPACES_CDN_TOKEN ignored without the shared store") {
		t.Errorf("log %q does not say the token is ignored", logs.String())
	}
	cookie := h.cookie(t)
	// Without the shared store there are no image copies to clear.
	resp, body := getWith(t, h.ts.URL+"/admin", cookie, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Images are not copied: the shared store is off.") || strings.Contains(body, `action="/admin/clear"`) {
		t.Errorf("page = %d:\n%s", resp.StatusCode, body)
	}
	if !strings.Contains(body, h.sets.version()[:12]) || !strings.Contains(body, "3 (3 cards)") {
		t.Errorf("page does not show the loaded card sets:\n%s", body)
	}
	if resp, body := h.clear(t, cookie, url.Values{"all": {"1"}}); resp.StatusCode != http.StatusConflict || !strings.Contains(body, "no image copies to clear") {
		t.Errorf("clear without the shared store = %d", resp.StatusCode)
	}
	if c := h.cat.Current().Cached("me01-001"); c == nil || c.Name != "Bulbasaur" {
		t.Errorf("card data changed: me01-001 = %+v", c)
	}
	if strings.Contains(logs.String(), "warning") {
		t.Errorf("log has a warning: %q", logs.String())
	}
}

// TestHostedAdminCheckPicksUpPublish: an admin's check picks up a new
// publish at once, with no restart: the page shows the new version and
// what changed, a new game resolves the new set, and the game open before
// keeps the cards it started with.
func TestHostedAdminCheckPicksUpPublish(t *testing.T) {
	logs := captureSyncLog(t)
	h := startHostedAdmin(t, false, false)
	cookie := h.cookie(t)
	v1 := h.sets.version()
	if _, body := getWith(t, h.ts.URL+"/admin", cookie, nil); !strings.Contains(body, v1[:12]) {
		t.Errorf("page does not show version %s:\n%s", v1[:12], body)
	}
	cs := h.connect(t)
	callTool(t, cs, "new_game", map[string]any{"deck_p1": adminDeck, "deck_p2": adminDeck, "id": "before"})
	old, err := h.manager(hostauthtest.DefaultAccount).Current()
	if err != nil {
		t.Fatal(err)
	}

	h.sets.publish(fixtureSet("sv2", "sv", "PAL", "Paldea Evolved", "2023-06-09",
		fixturePrint{local: "013", name: "Sprigatito", hp: 70, attack: "Scratch", effect: "Flip a coin."}))
	v2 := h.sets.version()
	if h.cat.Current().Cached("sv2-013") != nil {
		t.Fatal("publish picked up before the check")
	}
	if resp, body := h.check(t, cookie); resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/admin" {
		t.Fatalf("check = %d %q", resp.StatusCode, body)
	}
	_, page := getWith(t, h.ts.URL+"/admin", cookie, nil)
	for _, want := range []string{v2[:12], "4 (4 cards)", "Last check now by one@example.com", "picked up version " + v2[:12] + ": changed sv2, removed none"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}
	if !strings.Contains(logs.String(), "cache admin: one@example.com checked published card sets: picked up version "+v2[:12]) {
		t.Errorf("log does not record the check: %q", logs.String())
	}

	const deck = `Pokémon: 4
4 Sprigatito PAL 13

Energy: 56
56 Basic {G} Energy SVE 5

Total Cards: 60`
	callTool(t, cs, "new_game", map[string]any{"deck_p1": deck, "deck_p2": adminDeck, "id": "after"})
	g, err := h.manager(hostauthtest.DefaultAccount).Current()
	if err != nil {
		t.Fatal(err)
	}
	sprigatitos := 0
	for iid, c := range g.State().Cards {
		if c.Def == "" {
			t.Errorf("%s (%s) did not resolve", iid, c.Name)
		}
		if c.Def == "sv2-013" {
			sprigatitos++
		}
	}
	if sprigatitos != 4 {
		t.Errorf("%d cards resolved to sv2-013, want 4", sprigatitos)
	}
	if text := callTool(t, cs, "card_text", map[string]any{"card": "sv2-013"}); !strings.Contains(text, "Sprigatito (sv2-013)") || !strings.Contains(text, "Scratch 10. Flip a coin.") {
		t.Errorf("card_text sv2-013 = %q", text)
	}
	if old.DB().Cached("sv2-013") != nil {
		t.Error("the game open before the check sees the new set")
	}
}

func TestHostedAdminWithoutTheToken(t *testing.T) {
	logs := captureSyncLog(t)
	h := startHostedAdmin(t, true, false)
	if !strings.Contains(logs.String(), "card images: CDN purge on clear: off (no TCGREPLAY_SPACES_CDN_TOKEN)") {
		t.Errorf("log %q", logs.String())
	}
	page := h.clearAndWait(t, h.cookie(t), url.Values{"set": {"sv1"}})
	if !strings.Contains(page, "CDN not purged (no TCGREPLAY_SPACES_CDN_TOKEN)") || !strings.Contains(page, "2 image copies removed") {
		t.Errorf("page:\n%s", page)
	}
}

func TestAdminText(t *testing.T) {
	if s := adminText(0); s != "no cache admins (TCGREPLAY_ADMINS)" {
		t.Errorf("adminText(0) = %q", s)
	}
	if s := adminText(2); s != "2 cache admins" {
		t.Errorf("adminText(2) = %q", s)
	}
}

// TestRunHostedLogsTheAdminCount: the start-up log names how many cache
// admins there are, never who, and a clean start logs no warning.
func TestRunHostedLogsTheAdminCount(t *testing.T) {
	logs := captureLog(t)
	base := "http://127.0.0.1:" + freePort(t)
	c := hostedConfig(t, strings.TrimPrefix(base, "http://"), "https://play.example.com")
	clearSpacesEnv(t)
	t.Setenv("TCGREPLAY_ADMINS", "one@example.com, two@example.com")
	t.Setenv("TCGREPLAY_SPACES_CDN_TOKEN", "")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runHosted(ctx, c) }()
	waitHealthy(t, base, 5*time.Second, done, logs)
	resp, err := http.Get(base + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	out := logs.String()
	if !strings.Contains(out, ", 2 cache admins") {
		t.Errorf("log %q does not count the admins", out)
	}
	if strings.Contains(out, "one@example.com") || strings.Contains(out, "warning") {
		t.Errorf("log %q names an admin or warns", out)
	}
}

// TestDesktopServesNoAdminPage: the desktop board has no admin page,
// whatever the environment says.
func TestDesktopServesNoAdminPage(t *testing.T) {
	t.Setenv("TCGREPLAY_ADMINS", "one@example.com")
	m, _ := newServer(config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true}, "", nil)
	ts := httptest.NewServer(webui.New(m, state.P1).Handler())
	defer ts.Close()
	for _, path := range []string{"/admin", "/admin/clear"} {
		resp, err := http.PostForm(ts.URL+path, url.Values{"all": {"1"}})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(b), "Card cache") || strings.Contains(string(b), "Clear everything") {
			t.Errorf("desktop %s served the admin page", path)
		}
	}
}
