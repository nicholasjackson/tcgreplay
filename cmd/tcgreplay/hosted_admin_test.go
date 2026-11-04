package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
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

// adminSource is a card source with sets me01 (MEG), sv1 (SVI) and sve
// (SVE), a card of each, and their images; it counts requests per path.
type adminSource struct {
	*httptest.Server
	mu     sync.Mutex
	bodies map[string]string
	hits   map[string]int
}

func newAdminSource(t *testing.T) *adminSource {
	t.Helper()
	s := &adminSource{hits: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits[r.URL.Path]++
		body, ok := s.bodies[r.URL.Path]
		s.mu.Unlock()
		switch {
		case ok:
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, body)
		case strings.HasSuffix(r.URL.Path, ".webp"):
			w.Header().Set("Content-Type", "image/webp")
			io.WriteString(w, "RIFF....WEBP "+r.URL.Path)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	img := s.URL
	s.bodies = map[string]string{
		"/sets":           `[{"id":"me01"},{"id":"sv1"},{"id":"sve"}]`,
		"/sets/me01":      `{"id":"me01","name":"Mega Evolution","releaseDate":"2025-09-26","serie":{"id":"me"},"abbreviation":{"official":"MEG"},"cards":[{"id":"me01-001","localId":"001","name":"Bulbasaur","image":"` + img + `/en/me/me01/001"}]}`,
		"/sets/sv1":       `{"id":"sv1","name":"Scarlet & Violet","releaseDate":"2023-03-31","serie":{"id":"sv"},"abbreviation":{"official":"SVI"},"cards":[{"id":"sv1-001","localId":"001","name":"Pineco","image":"` + img + `/en/sv/sv1/001"}]}`,
		"/sets/sve":       `{"id":"sve","name":"Scarlet & Violet Energies","releaseDate":"2023-03-31","serie":{"id":"sv"},"abbreviation":{"official":"SVE"},"cards":[{"id":"sve-005","localId":"005","name":"Grass Energy"}]}`,
		"/cards/me01-001": `{"id":"me01-001","localId":"001","name":"Bulbasaur","category":"Pokemon","stage":"Basic","hp":70,"types":["Grass"],"image":"` + img + `/en/me/me01/001","set":{"id":"me01","name":"Mega Evolution"}}`,
		"/cards/sv1-001":  `{"id":"sv1-001","localId":"001","name":"Pineco","category":"Pokemon","stage":"Basic","hp":70,"types":["Grass"],"image":"` + img + `/en/sv/sv1/001","set":{"id":"sv1","name":"Scarlet & Violet"}}`,
		"/cards/sve-005":  `{"id":"sve-005","localId":"005","name":"Grass Energy","category":"Energy","energyType":"Normal","set":{"id":"sve","name":"Scarlet & Violet Energies"}}`,
	}
	return s
}

func (s *adminSource) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

// adminHosted is hosted mode with the admin page, wired as runHosted
// wires it, over the stand-in Spaces, CDN API and card source.
type adminHosted struct {
	*hostedServer
	src   *adminSource
	srv   *spacestest.Server // nil without the shared store
	cdn   *spacestest.CDN    // nil without a token
	admin *cacheadmin.Service
	db    *cards.Client
}

// startHostedAdmin starts hosted mode with one@example.com (the fake
// Google's default account) as the admin, the shared store when shared,
// and a CDN token when token.
func startHostedAdmin(t *testing.T, shared, token bool) *adminHosted {
	t.Helper()
	g := hostauthtest.NewGoogle(t)
	t.Setenv("TCGREPLAY_GOOGLE_CLIENT_SECRET", g.ClientSecret)
	t.Setenv("TCGREPLAY_ADMINS", "One@Example.com")
	src := newAdminSource(t)
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", googleClientID: g.ClientID, cardBase: src.URL, imageBase: src.URL}
	h := &adminHosted{src: src}
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
	db, _, images := hostedCardDB(c)
	reg, mcpSrv, a, err := newHosted(c, db, images, u, g.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if images != nil {
		go images.Run(ctx)
	}
	h.admin = hostedAdmin(db, images)
	go h.admin.Run(ctx)
	h.db = db
	ts := httptest.NewServer(hostedHandler(mcpSrv, reg, a, h.admin))
	t.Cleanup(ts.Close)
	h.hostedServer = &hostedServer{ts: ts, g: g, reg: reg, public: u.String()}
	return h
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

// gameCards returns the default account's current game's cards.
func (h *adminHosted) gameCards(t *testing.T) map[state.IID]*state.Card {
	t.Helper()
	g, err := h.manager(hostauthtest.DefaultAccount).Current()
	if err != nil {
		t.Fatal(err)
	}
	return g.State().Cards
}

func TestHostedAdminClearsEverything(t *testing.T) {
	logs := captureSyncLog(t)
	h := startHostedAdmin(t, true, true)
	cookie := h.cookie(t)
	resp, body := getWith(t, h.ts.URL+"/admin", cookie, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Signed in as one@example.com") {
		t.Fatalf("admin page = %d %q", resp.StatusCode, body)
	}

	// A game open on the board, its cards fetched into every tier.
	cs := h.connect(t)
	callTool(t, cs, "new_game", map[string]any{"deck_p1": adminDeck, "deck_p2": adminDeck, "id": "live-1"})
	callTool(t, cs, "set_mode", map[string]any{"mode": "review"})
	if len(h.srv.Keys("tcgdex/")) == 0 {
		t.Fatal("the game put no card data in the shared store")
	}
	before := h.src.count("/cards/me01-001")

	page := h.clearAndWait(t, cookie, url.Values{"all": {"1"}})
	if keys := h.srv.Keys("tcgdex/"); len(keys) != 0 {
		t.Errorf("card data left: %v", keys)
	}
	if keys := h.srv.Keys(cards.ImagePrefix); len(keys) != 0 {
		t.Errorf("image copies left: %v", keys)
	}
	if got := h.cdn.Purges(); len(got) != 1 || !slices.Equal(got[0], []string{"images/*"}) {
		t.Errorf("purges = %v, want [[images/*]]", got)
	}
	for _, want := range []string{"everything by one@example.com", "card entries removed from the shared store", "image copies removed", "CDN purged (1 paths)"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}

	// The open game still shows and plays.
	for iid, c := range h.gameCards(t) {
		if c.Def == "" || h.db.Cached(c.Def) == nil {
			t.Errorf("%s (%s) lost its card", iid, c.Name)
		}
	}
	hand := claudeState(t, cs).Sides[state.P2].Hand
	if len(hand) == 0 {
		t.Fatal("no cards in hand")
	}
	callTool(t, cs, "move", map[string]any{"seat": "p2", "iid": string(hand[0].IID), "to": "discard"})
	if text := callTool(t, cs, "card_text", map[string]any{"card": "me01-001"}); !strings.Contains(text, "Bulbasaur") {
		t.Errorf("card text after the clear = %q", text)
	}

	// The next import shows its cards, fetched from the card source again.
	callTool(t, cs, "new_game", map[string]any{"deck_p1": adminDeck, "deck_p2": adminDeck, "id": "live-2"})
	for iid, c := range h.gameCards(t) {
		if c.Def == "" || h.db.Cached(c.Def) == nil {
			t.Errorf("new game's %s (%s) has no card", iid, c.Name)
		}
	}
	if n := h.src.count("/cards/me01-001"); n <= before {
		t.Errorf("card source requests for me01-001 = %d, was %d: not fetched again", n, before)
	}
	out := logs.String()
	for _, want := range []string{"cache admin: one@example.com started clearing everything", "cache admin: cleared everything for one@example.com", "card images: CDN purge on clear: on"} {
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
			ctx := context.Background()
			for _, id := range []string{"me01-001", "sv1-001"} {
				if _, err := h.db.Card(ctx, id); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := h.db.Resolve(ctx, "MEG", "1"); err != nil {
				t.Fatal(err)
			}
			page := h.clearAndWait(t, cookie, url.Values{"set": {name}})
			want := []string{
				"images/en/sv/sv1/001/high.webp", "images/en/sv/sv1/001/low.webp",
				"tcgdex/cards/sv1-001.json", "tcgdex/sets.json", "tcgdex/sets/sv1.json", "tcgdex/sets/sve.json",
			}
			if got := h.srv.Keys(""); !slices.Equal(got, want) {
				t.Errorf("bucket = %v, want %v", got, want)
			}
			if got := h.cdn.Purges(); len(got) != 1 || !slices.Equal(got[0], []string{"images/en/me/me01/*"}) {
				t.Errorf("purges = %v", got)
			}
			if !strings.Contains(page, "Mega Evolution (me01) by one@example.com") {
				t.Errorf("page:\n%s", page)
			}
		})
	}
}

func TestHostedAdminRefusesEveryoneElse(t *testing.T) {
	captureSyncLog(t)
	h := startHostedAdmin(t, true, true)
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
	check("non-admin")
	if _, ok := h.admin.Status(); ok {
		t.Error("a clear was started")
	}
}

func TestHostedAdminWithoutTheSharedStore(t *testing.T) {
	logs := captureSyncLog(t)
	h := startHostedAdmin(t, false, true)
	if !strings.Contains(logs.String(), "card cache: TCGREPLAY_SPACES_CDN_TOKEN ignored without the shared store") {
		t.Errorf("log %q does not say the token is ignored", logs.String())
	}
	cookie := h.cookie(t)
	if _, err := h.db.Card(context.Background(), "me01-001"); err != nil {
		t.Fatal(err)
	}
	page := h.clearAndWait(t, cookie, url.Values{"all": {"1"}})
	if !strings.Contains(page, "card entries removed from local disk") || !strings.Contains(page, "1 card entries marked for refetch in memory") {
		t.Errorf("page:\n%s", page)
	}
	if tiers := page[strings.Index(page, "<ul>"):]; strings.Contains(tiers, "image copies") || strings.Contains(tiers, "CDN") {
		t.Errorf("page shows image or CDN tiers without the shared store:\n%s", page)
	}
	if _, err := h.db.Store.Get(context.Background(), "cards/me01-001"); err == nil {
		t.Error("the card is still on disk")
	}
	if strings.Contains(logs.String(), "warning") {
		t.Errorf("log has a warning: %q", logs.String())
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
	m, _ := newServer(config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true}, "")
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
