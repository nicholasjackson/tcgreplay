package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/cardset"
	"github.com/nicholasjackson/tcgreplay/internal/catalog"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
	"github.com/nicholasjackson/tcgreplay/internal/spaces"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// tcgdexAssets is TCGdex's image host, where every fixture print's image
// lives, and the image cache's default source.
const tcgdexAssets = "https://assets.tcgdex.net"

// fixturePrint is a print for the published fixtures: a Basic Grass
// Pokémon with one attack, unless cat names another category.
type fixturePrint struct {
	local, name string
	cat, energy string // "Trainer" or "Energy", and the Energy's kind; "" for a Pokémon
	hp          int
	attack      string // the Pokémon's attack
	effect      string // the attack's effect, or the card's own for a Trainer or special Energy
}

// fixtureSet is a built set whose prints each have a TCGdex image at
// https://assets.tcgdex.net/en/<serie>/<id>/<local>, so it passes the
// publish check.
func fixtureSet(id, serie, abbr, name, release string, prints ...fixturePrint) *cardset.Set {
	s := &cardset.Set{
		Format: cardset.Format,
		Set: cardset.SetHeader{ID: id, Serie: serie, ReleaseDate: release, Abbreviation: abbr,
			Text: map[string]cardset.SetText{"en": {Name: name}}},
	}
	for _, p := range prints {
		cp := cardset.Print{ID: id + "-" + p.local, LocalID: p.local, Category: "Pokemon",
			Images: []cardset.Image{{Source: cardset.SourceTCGdex, URL: tcgdexAssets + "/en/" + serie + "/" + id + "/" + p.local}}}
		text := cardset.PrintText{Name: p.name}
		if p.cat == "" {
			cp.HP, cp.Stage, cp.Types = p.hp, "Basic", []string{"Grass"}
			cp.Attacks = []cardset.AttackCore{{Cost: []string{"Grass"}, Damage: "10"}}
			text.Attacks = []cardset.MoveText{{Name: p.attack, Effect: p.effect}}
		} else {
			cp.Category, cp.EnergyType = p.cat, p.energy
			if p.cat == "Trainer" {
				cp.TrainerType = "Item"
			}
			text.Effect = p.effect
		}
		cp.Text = map[string]cardset.PrintText{"en": text}
		s.Prints = append(s.Prints, cp)
	}
	return s
}

// adminSets are the published sets most cmd tests play with: me01 (MEG)
// with Bulbasaur, sv1 (SVI) with Pineco and sve (SVE) with Grass Energy,
// the cards adminDeck lists.
func adminSets() []*cardset.Set {
	return []*cardset.Set{
		fixtureSet("me01", "me", "MEG", "Mega Evolution", "2025-09-26",
			fixturePrint{local: "001", name: "Bulbasaur", hp: 70, attack: "Vine Whip", effect: "Heal 10 damage from this Pokémon."}),
		fixtureSet("sv1", "sv", "SVI", "Scarlet & Violet", "2023-03-31",
			fixturePrint{local: "001", name: "Pineco", hp: 70, attack: "Tackle"}),
		fixtureSet("sve", "sv", "SVE", "Scarlet & Violet Energies", "2023-03-31",
			fixturePrint{local: "005", name: "Grass Energy", cat: "Energy", energy: "Normal"}),
	}
}

// bulbasaurText is card_text for me01-001 as adminSets publish it.
const bulbasaurText = "Bulbasaur (me01-001)\nBasic Pokémon. HP 70. Type Grass.\nAttack [Grass] Vine Whip 10. Heal 10 damage from this Pokémon.\nRetreat 0"

// cardsetHost is a card-set directory published to a directory, which it
// serves over http as the bucket's CDN serves the published layout,
// counting requests per path.
type cardsetHost struct {
	*httptest.Server
	t   *testing.T
	src string // the card-set directory (cardsets/)
	dir string // the published layout

	mu   sync.Mutex
	hits map[string]int
}

// publishCardsets publishes sets and serves the layout.
func publishCardsets(t *testing.T, sets ...*cardset.Set) *cardsetHost {
	t.Helper()
	h := &cardsetHost{t: t, src: t.TempDir(), dir: t.TempDir(), hits: map[string]int{}}
	h.publish(sets...)
	files := http.FileServer(http.Dir(h.dir))
	h.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.hits[strings.TrimPrefix(r.URL.Path, "/")]++
		h.mu.Unlock()
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(h.Close)
	return h
}

// publish adds or replaces sets in the card-set directory and publishes
// it again.
func (h *cardsetHost) publish(sets ...*cardset.Set) {
	h.t.Helper()
	for _, s := range sets {
		if err := cardset.Write(filepath.Join(h.src, "sets", s.Set.ID+".json"), s); err != nil {
			h.t.Fatal(err)
		}
	}
	if _, err := cardset.Publish(context.Background(), cardset.PublishOptions{
		Dir: h.src, Prefix: "", Target: cardset.DirTarget{Dir: h.dir}, Logf: func(string, ...any) {},
	}); err != nil {
		h.t.Fatal(err)
	}
}

// requests is a copy of the per-path request counts.
func (h *cardsetHost) requests() map[string]int {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string]int, len(h.hits))
	for k, v := range h.hits {
		out[k] = v
	}
	return out
}

func (h *cardsetHost) manifest() []byte {
	h.t.Helper()
	b, err := os.ReadFile(filepath.Join(h.dir, cardset.ManifestKey))
	if err != nil {
		h.t.Fatal(err)
	}
	return b
}

// version is the catalogue version a playmat reports for the published
// manifest: its sha256.
func (h *cardsetHost) version() string {
	h.t.Helper()
	sum := sha256.Sum256(h.manifest())
	return hex.EncodeToString(sum[:])
}

// objects lists the set objects the published manifest names.
func (h *cardsetHost) objects() []string {
	h.t.Helper()
	m, err := cardset.DecodeManifest(h.manifest())
	if err != nil {
		h.t.Fatal(err)
	}
	var out []string
	for _, s := range m.Sets {
		out = append(out, s.Path)
	}
	slices.Sort(out)
	return out
}

// publishedCatalog publishes sets to a directory and returns a catalogue
// loaded from it (a directory, so no request is made), with an empty cache.
func publishedCatalog(t *testing.T, sets ...*cardset.Set) *catalog.Loader {
	t.Helper()
	h := publishCardsets(t, sets...)
	l := &catalog.Loader{Base: h.dir, CacheDir: t.TempDir(), Logf: func(string, ...any) {}}
	l.Start(context.Background(), 5*time.Second)
	if l.Current().Cached(sets[0].Prints[0].ID) == nil {
		t.Fatalf("published catalogue lacks %s", sets[0].Prints[0].ID)
	}
	return l
}

// reroute is an http transport that sends requests for the host keep as
// they are and every other request to the server at to, keeping the path,
// and records the address each rerouted request was for. It keeps a test
// off the network: an image cache copying from TCGdex's image host
// reaches a local stand-in, and a request a catalogue should never make
// lands on a tripwire.
type reroute struct {
	keep string // host:port left alone; "" reroutes everything
	to   *url.URL

	mu   sync.Mutex
	sent []string
}

func newReroute(t *testing.T, keep, to string) *reroute {
	t.Helper()
	u, err := url.Parse(to)
	if err != nil {
		t.Fatal(err)
	}
	if keep != "" {
		k, err := url.Parse(keep)
		if err != nil {
			t.Fatal(err)
		}
		keep = k.Host
	}
	return &reroute{keep: keep, to: u}
}

func (r *reroute) RoundTrip(req *http.Request) (*http.Response, error) {
	if r.keep == "" || req.URL.Host != r.keep {
		r.mu.Lock()
		r.sent = append(r.sent, req.URL.String())
		r.mu.Unlock()
		req = req.Clone(req.Context())
		req.URL.Scheme, req.URL.Host, req.Host = r.to.Scheme, r.to.Host, ""
	}
	return http.DefaultTransport.RoundTrip(req)
}

func (r *reroute) client() *http.Client { return &http.Client{Transport: r, Timeout: 10 * time.Second} }

// rerouted lists the addresses of the requests sent elsewhere.
func (r *reroute) rerouted() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.sent)
}

// counter is a test server counting every request, answering 404: a
// stand-in for TCGdex (the card source the playmat no longer uses) that
// must never be asked for anything.
type counter struct {
	*httptest.Server
	mu sync.Mutex
	n  int
}

func newCounter(t *testing.T) *counter {
	t.Helper()
	c := &counter{}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.n++
		c.mu.Unlock()
		http.NotFound(w, r)
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *counter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// serveHosted wires hosted mode as runHosted does, over the started
// catalogue cat (newHosted, and the admin service over cat), for the
// public address with the fake Google g, and serves it on a test server.
func serveHosted(t *testing.T, public string, c config, g *hostauthtest.FakeGoogle, cat *catalog.Loader) *hostedServer {
	t.Helper()
	u, err := hostedPublicURL(public, ":8080")
	if err != nil {
		t.Fatal(err)
	}
	reg, srv, a, err := newHosted(c, cat, nil, u, g.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(hostedHandler(srv, reg, a, hostedAdmin(cat, nil)))
	t.Cleanup(ts.Close)
	return &hostedServer{ts: ts, g: g, reg: reg, public: u.String(), cat: cat}
}

// hostedOn starts hosted mode on the card sets published at base (an
// address or a directory) with an empty cache, its catalogue built from
// the config by newCatalog and fetching through client when set.
func hostedOn(t *testing.T, base string, client *http.Client) *hostedServer {
	t.Helper()
	g := hostauthtest.NewGoogle(t)
	t.Setenv("TCGREPLAY_GOOGLE_CLIENT_SECRET", g.ClientSecret)
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", googleClientID: g.ClientID, cardsetsURL: base}
	cat := newCatalog(c)
	cat.HTTP = client
	cat.Start(context.Background(), 5*time.Second)
	return serveHosted(t, testPublic, c, g, cat)
}

// desktopOn builds the desktop server as runInProcess does, its
// catalogue started from c, and returns its manager and an MCP session on
// it.
func desktopOn(t *testing.T, c config) (*game.Manager, *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	m, srv := newServer(c, "", startCatalog(ctx, c, 5*time.Second))
	st, ct := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "desktop-test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return m, cs
}

// wantGameFromSets checks every card of g resolves, through the game's own
// snapshot, to exactly the definition the built sets give it.
func wantGameFromSets(t *testing.T, g *game.Game, sets []*cardset.Set) {
	t.Helper()
	byID := map[string]*cardset.Set{}
	for _, s := range sets {
		for _, p := range s.Prints {
			byID[p.ID] = s
		}
	}
	n := 0
	for iid, c := range g.State().Cards {
		if c.Def == "" {
			t.Errorf("%s (%s) did not resolve", iid, c.Name)
			continue
		}
		s := byID[c.Def]
		if s == nil {
			t.Errorf("%s resolved to %s, which no published set holds", iid, c.Def)
			continue
		}
		want, err := s.Card(c.Def, "en")
		if err != nil {
			t.Fatal(err)
		}
		got := g.DB().Cached(c.Def)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: game's definition\n%+v\nwant the published\n%+v", c.Def, got, want)
		}
		n++
	}
	if n != 120 {
		t.Errorf("%d cards checked, want both 60-card decks", n)
	}
}

// playSomeMoves switches the open game to review and plays a few moves
// for Claude's seat: a draw and a card from hand to the discard pile.
func playSomeMoves(t *testing.T, cs *mcp.ClientSession) {
	t.Helper()
	callTool(t, cs, "set_mode", map[string]any{"mode": "review"})
	callTool(t, cs, "draw", map[string]any{"seat": "p2"})
	hand := claudeState(t, cs).Sides[state.P2].Hand
	if len(hand) == 0 {
		t.Fatal("no cards in hand")
	}
	callTool(t, cs, "move", map[string]any{"seat": "p2", "iid": string(hand[0].IID), "to": "discard"})
	if got := claudeState(t, cs).Sides[state.P2].Discard; !hasCard(got, hand[0].IID) {
		t.Errorf("%s not in the discard pile: %v", hand[0].IID, got)
	}
}

// A hosted playmat pointed at a published layout plays a whole game from
// it: the game's definitions are exactly the published sets', card_text
// quotes the published text, and the only card-data requests are for the
// published manifest and set objects; nothing else is asked, TCGdex least
// of all.
func TestHostedPlaysFromPublishedSets(t *testing.T) {
	sets := adminSets()
	host := publishCardsets(t, sets...)
	tcgdex := newCounter(t)
	route := newReroute(t, host.URL, tcgdex.URL)
	h := hostedOn(t, host.URL, route.client())
	cs := h.connect(t)

	callTool(t, cs, "new_game", map[string]any{"deck_p1": adminDeck, "deck_p2": adminDeck, "id": "published-1"})
	playSomeMoves(t, cs)
	if got := callTool(t, cs, "card_text", map[string]any{"card": "me01-001"}); got != bulbasaurText {
		t.Errorf("card_text me01-001 = %q, want the published %q", got, bulbasaurText)
	}
	hand := claudeState(t, cs).Sides[state.P2].Hand
	if len(hand) == 0 {
		t.Fatal("no cards in hand")
	}
	if got := callTool(t, cs, "card_text", map[string]any{"card": string(hand[0].IID)}); !strings.Contains(got, "("+hand[0].Def+")") {
		t.Errorf("card_text %s = %q, want %s's text", hand[0].IID, got, hand[0].Def)
	}
	g, err := h.manager(hostauthtest.DefaultAccount).Current()
	if err != nil {
		t.Fatal(err)
	}
	wantGameFromSets(t, g, sets)

	if n := tcgdex.count(); n != 0 {
		t.Errorf("the TCGdex stand-in got %d requests, want none", n)
	}
	if got := route.rerouted(); len(got) != 0 {
		t.Errorf("the catalogue asked other hosts: %v", got)
	}
	want := map[string]int{cardset.ManifestKey: 1}
	for _, p := range host.objects() {
		want[p] = 1
	}
	if got := host.requests(); !reflect.DeepEqual(got, want) {
		t.Errorf("published layout requests %v, want the manifest and each set object once: %v", got, want)
	}
}

// With no reachable published address and an empty cache there are no
// definitions: card_text reports the card unknown, and the game still
// runs, on desktop and hosted.
func TestNoCardDataWithoutPublishedSets(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	unreachable := closed.URL
	closed.Close()
	for name, open := range map[string]func(t *testing.T) (*mcp.ClientSession, *game.Manager){
		"hosted": func(t *testing.T) (*mcp.ClientSession, *game.Manager) {
			h := hostedOn(t, unreachable, nil)
			return h.connect(t), h.manager(hostauthtest.DefaultAccount)
		},
		"desktop": func(t *testing.T) (*mcp.ClientSession, *game.Manager) {
			m, cs := desktopOn(t, config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", cardsetsURL: unreachable})
			return cs, m
		},
		"desktop, no address": func(t *testing.T) (*mcp.ClientSession, *game.Manager) {
			m, cs := desktopOn(t, config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2"})
			return cs, m
		},
	} {
		t.Run(name, func(t *testing.T) {
			captureSyncLog(t)
			cs, m := open(t)
			if text, isErr := toolResult(t, cs, "card_text", map[string]any{"card": "me01-001"}); !isErr || !strings.Contains(text, "not found") {
				t.Errorf("card_text with no catalogue = %q (error %v), want not found", text, isErr)
			}
			callTool(t, cs, "new_game", map[string]any{"deck_p1": adminDeck, "deck_p2": adminDeck, "id": "unknown-1"})
			playSomeMoves(t, cs)
			g, err := m.Current()
			if err != nil {
				t.Fatal(err)
			}
			// The Pokémon have no definition; Basic Energy is known by
			// name alone, without card data.
			var pokemon state.IID
			for iid, c := range g.State().Cards {
				if strings.Contains(c.Name, "Energy") {
					continue
				}
				if c.Def != "" {
					t.Errorf("%s (%s) resolved to %s with no catalogue", iid, c.Name, c.Def)
				}
				pokemon = iid
			}
			if pokemon == "" {
				t.Fatal("no Pokémon in the game")
			}
			if text, isErr := toolResult(t, cs, "card_text", map[string]any{"card": string(pokemon)}); !isErr || !strings.Contains(text, "unresolved") {
				t.Errorf("card_text %s = %q (error %v), want it unresolved", pokemon, text, isErr)
			}
			if text, isErr := toolResult(t, cs, "card_text", map[string]any{"card": "me01-001"}); !isErr || !strings.Contains(text, "not found") {
				t.Errorf("card_text in the game = %q (error %v), want not found", text, isErr)
			}
		})
	}
}

// A desktop playmat that loaded the published sets once plays with full
// card text from its cache directory when started again offline or with
// the published address gone.
func TestDesktopPlaysOfflineAfterFirstUse(t *testing.T) {
	captureSyncLog(t)
	sets := adminSets()
	host := publishCardsets(t, sets...)
	cache := t.TempDir()
	first := config{dataDir: t.TempDir(), cacheDir: cache, seat: "p2", cardsetsURL: host.URL}
	m, _ := desktopOn(t, first)
	if c := m.CardDB().Cached("me01-001"); c == nil || c.Name != "Bulbasaur" {
		t.Fatalf("first run has me01-001 = %+v", c)
	}
	host.Close()
	before := host.requests()

	for name, c := range map[string]config{
		"offline":        {dataDir: t.TempDir(), cacheDir: cache, seat: "p2", cardsetsURL: host.URL, offline: true},
		"server stopped": {dataDir: t.TempDir(), cacheDir: cache, seat: "p2", cardsetsURL: host.URL},
	} {
		t.Run(name, func(t *testing.T) {
			m, cs := desktopOn(t, c)
			callTool(t, cs, "new_game", map[string]any{"deck_p1": adminDeck, "deck_p2": adminDeck, "id": "offline-1"})
			playSomeMoves(t, cs)
			if got := callTool(t, cs, "card_text", map[string]any{"card": "me01-001"}); got != bulbasaurText {
				t.Errorf("card_text me01-001 = %q, want %q", got, bulbasaurText)
			}
			g, err := m.Current()
			if err != nil {
				t.Fatal(err)
			}
			wantGameFromSets(t, g, sets)
		})
	}
	if got := host.requests(); !reflect.DeepEqual(got, before) {
		t.Errorf("the stopped server was asked for something: %v, was %v", got, before)
	}
}

// Every mode builds its catalogue from the config: the desktop
// (newServer over startCatalog), the daemon (runDaemon) and hosted mode
// load the published sets at -cardsets-url into the -cache directory.
func TestEveryModeBuildsItsCatalogueFromConfig(t *testing.T) {
	captureSyncLog(t)
	host := publishCardsets(t, adminSets()...)
	localCopy := func(t *testing.T, cache string) {
		t.Helper()
		if _, err := os.Stat(filepath.Join(cache, "cardsets", cardset.ManifestKey)); err != nil {
			t.Errorf("no local copy in the -cache directory: %v", err)
		}
	}

	t.Run("desktop", func(t *testing.T) {
		c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", cardsetsURL: host.dir}
		m, cs := desktopOn(t, c)
		if got := callTool(t, cs, "card_text", map[string]any{"card": "me01-001"}); got != bulbasaurText {
			t.Errorf("card_text = %q", got)
		}
		if _, ok := m.CardDB().(*catalog.Snapshot); !ok {
			t.Errorf("desktop card database is %T, want a catalogue snapshot", m.CardDB())
		}
		localCopy(t, c.cacheDir)
	})

	t.Run("daemon", func(t *testing.T) {
		c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", cardsetsURL: host.dir}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- runDaemon(ctx, c, false) }()
		defer func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("runDaemon: %v", err)
			}
		}()
		var conn net.Conn
		deadline := time.Now().Add(5 * time.Second)
		for {
			var err error
			if conn, err = net.Dial("unix", socketPath(c)); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("daemon socket: %v", err)
			}
			time.Sleep(10 * time.Millisecond)
		}
		cs, err := mcp.NewClient(&mcp.Implementation{Name: "daemon-test"}, nil).Connect(ctx, &mcp.IOTransport{Reader: conn, Writer: conn}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cs.Close()
		// The daemon checks for published sets in the background.
		for {
			text, isErr := toolResult(t, cs, "card_text", map[string]any{"card": "me01-001"})
			if !isErr {
				if text != bulbasaurText {
					t.Errorf("card_text = %q", text)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("daemon never loaded the published sets: %s", text)
			}
			time.Sleep(10 * time.Millisecond)
		}
		localCopy(t, c.cacheDir)
	})

	t.Run("hosted", func(t *testing.T) {
		h := hostedOn(t, host.URL, nil)
		db := h.manager(hostauthtest.DefaultAccount).CardDB()
		if c := db.Cached("me01-001"); c == nil || c.Name != "Bulbasaur" {
			t.Errorf("hosted account's catalogue has me01-001 = %+v", c)
		}
		if st := h.cat.Status(); st.Base != host.URL || st.Sets != 3 || st.Version != host.version() {
			t.Errorf("hosted catalogue status %+v, want 3 sets from %s at version %s", st, host.URL, host.version())
		}
	})
}

// The check interval is from 10s to 10m: a flag or variable outside that,
// or a variable that is no duration, stops the binary with status 2 and a
// message; a flag overrides an invalid variable.
func TestMainRejectsBadCardsetsInterval(t *testing.T) {
	for _, tt := range []struct {
		name, env string
		args      []string
		want      string
	}{
		{"below 10s", "", []string{"-cardsets-interval", "9s"}, "-cardsets-interval 9s: must be from 10s to 10m0s"},
		{"above 10m", "", []string{"-cardsets-interval", "11m"}, "-cardsets-interval 11m0s: must be from 10s to 10m0s"},
		{"zero", "", []string{"-cardsets-interval", "0"}, "-cardsets-interval 0s: must be from 10s to 10m0s"},
		{"env out of range", "5s", nil, "-cardsets-interval 5s: must be from 10s to 10m0s"},
		{"invalid env", "soon", nil, `TCGREPLAY_CARDSETS_INTERVAL: invalid duration "soon"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TCGREPLAY_CARDSETS_INTERVAL", tt.env)
			code, stdout, stderr := runMain(t, append([]string{"-offline"}, append(tt.args, "-version")...)...)
			if code != 2 || !strings.Contains(stderr, tt.want) {
				t.Fatalf("exit %d, stderr %q; want 2 and %q", code, stderr, tt.want)
			}
			if stdout != "" {
				t.Errorf("printed %q before refusing", stdout)
			}
		})
	}
	for _, tt := range []struct {
		name, env string
		args      []string
	}{
		{"default", "", nil},
		{"10s", "", []string{"-cardsets-interval", "10s"}},
		{"10m", "", []string{"-cardsets-interval", "10m"}},
		{"env", "2m", nil},
		{"flag over invalid env", "soon", []string{"-cardsets-interval", "30s"}},
	} {
		t.Run("accepts "+tt.name, func(t *testing.T) {
			t.Setenv("TCGREPLAY_CARDSETS_INTERVAL", tt.env)
			if code, stdout, stderr := runMain(t, append([]string{"-offline"}, append(tt.args, "-version")...)...); code != 0 || stdout == "" {
				t.Fatalf("exit %d, stdout %q, stderr %q; want the version", code, stdout, stderr)
			}
		})
	}
}

// The daemon the shim spawns gets the published address and the check
// interval as flags; an unset interval is left to its default.
func TestConfigArgsCarryCardsets(t *testing.T) {
	c := config{dataDir: "/d", cacheDir: "/c", seat: "p2", uiAddr: "127.0.0.1:7777",
		cardsetsURL: "https://cdn.example/cardsets/", cardsetsInterval: 30 * time.Second}
	args := c.args()
	for _, pair := range [][2]string{{"-cardsets-url", "https://cdn.example/cardsets/"}, {"-cardsets-interval", "30s"}, {"-cache", "/c"}, {"-offline=false", ""}} {
		i := slices.Index(args, pair[0])
		if i < 0 || (pair[1] != "" && (i+1 >= len(args) || args[i+1] != pair[1])) {
			t.Errorf("args %q do not pass %s %s", args, pair[0], pair[1])
		}
	}
	c.cardsetsInterval = 0
	c.offline = true
	args = c.args()
	if slices.Contains(args, "-cardsets-interval") {
		t.Errorf("args with no interval set pass one: %q", args)
	}
	if !slices.Contains(args, "-offline=true") {
		t.Errorf("args %q do not pass -offline=true", args)
	}
	if i := slices.Index(args, "-cardsets-url"); i < 0 || i+1 >= len(args) || args[i+1] != "https://cdn.example/cardsets/" {
		t.Errorf("args %q do not pass the published address", args)
	}
}

// sets publish refuses an endpoint that names the bucket's own host or a
// CDN address, exiting 2 with the region form, before any request. A proxy
// stand-in catches any request that would leave the machine.
func TestSetsPublishRefusesBucketOrCDNEndpoint(t *testing.T) {
	dir := builtCardsets(t)
	proxy := newCounter(t)
	for _, k := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		t.Setenv(k, proxy.URL)
	}
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	for name, endpoint := range map[string]string{
		"bucket host": "https://test-bucket.ams3.digitaloceanspaces.com",
		"CDN":         "https://ams3.cdn.digitaloceanspaces.com",
	} {
		t.Run(name, func(t *testing.T) {
			setPublishEnv(t, spaces.Config{Endpoint: endpoint, Bucket: "test-bucket", AccessKey: "KEYID", SecretKey: "very-secret-value"}, "")
			code, stdout, stderr := runMain(t, "sets", "publish", "-dir", dir)
			if code != 2 {
				t.Errorf("exit %d, want 2; stderr:\n%s", code, stderr)
			}
			for _, want := range []string{spaces.EnvEndpoint, "https://<region>.digitaloceanspaces.com"} {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr lacks %q:\n%s", want, stderr)
				}
			}
			if strings.Contains(stderr, "very-secret-value") || strings.Contains(stderr, "KEYID") {
				t.Errorf("stderr shows a credential:\n%s", stderr)
			}
			if strings.Contains(stdout, "uploaded") {
				t.Errorf("published anyway:\n%s", stdout)
			}
		})
	}
	if n := proxy.count(); n != 0 {
		t.Errorf("%d requests left for the network", n)
	}
}
