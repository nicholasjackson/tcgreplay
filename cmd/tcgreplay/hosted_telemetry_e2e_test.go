package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/nicholasjackson/tcgreplay/internal/deck"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
	"github.com/nicholasjackson/tcgreplay/internal/observe/observetest"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
	"github.com/nicholasjackson/tcgreplay/internal/usergames"
)

// requestCounter counts the requests that reach the playmat, outside its
// tracing wrapper, so a test can tell whether every one became a span.
type requestCounter struct {
	mu   sync.Mutex
	n    int
	next http.Handler
}

func (c *requestCounter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
	c.next.ServeHTTP(w, r)
}

func (c *requestCounter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// e2eHosted is hosted mode wired as runHosted wires it, with telemetry
// sending to a fake receiver, card sets published locally, the shared
// image store standing in for Spaces and TCGdex's image host for its own.
type e2eHosted struct {
	*hostedServer
	r       *observetest.Receiver
	store   *spacestest.Server
	counter *requestCounter
	sets    *cardsetHost
	images  *imageHost
}

func startE2E(t *testing.T) *e2eHosted {
	t.Helper()
	captureSyncLog(t)
	g := hostauthtest.NewGoogle(t)
	t.Setenv("TCGREPLAY_GOOGLE_CLIENT_SECRET", g.ClientSecret)
	t.Setenv("TCGREPLAY_ADMINS", userA.Email)
	t.Setenv("TCGREPLAY_SPACES_CDN_TOKEN", "")
	store := spacestest.New(t)
	setSpacesEnv(t, store.Config())
	sets := publishCardsets(t, adminSets()...)
	r := observetest.New(t)
	events := &syncBuffer{}
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", googleClientID: g.ClientID, cardsetsURL: sets.URL, events: events, telemetry: sendingTo(r)}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	prev := log.Writer()
	tel, _ := hostedTelemetry(ctx, c)
	t.Cleanup(func() {
		tel.Shutdown(context.Background())
		log.SetOutput(prev)
	})
	u, err := hostedPublicURL(testPublic, ":8080")
	if err != nil {
		t.Fatal(err)
	}
	images := hostedImages(c, tel)
	if images == nil {
		t.Fatal("image cache off with the shared store set")
	}
	// TCGdex's image host is a local stand-in, reached through a tracing
	// client as hosted mode's own would be.
	imgHost := newImageHost(t)
	images.HTTP = tel.Client(newReroute(t, "", imgHost.URL).client())
	loader := newCatalog(c)
	loader.HTTP = tel.Client(loader.HTTP)
	reg, srv, a, err := newHosted(c, loader, images, u, g.Endpoint(), tel)
	if err != nil {
		t.Fatal(err)
	}
	reg.Recorder().SetSink(tel.Sink())
	loader.Start(ctx, 5*time.Second)
	go images.Run(ctx)
	admin := hostedAdmin(loader, images, reg.Recorder())
	go admin.Run(ctx)
	moved, _ := parseMovedHosts("", u)
	counter := &requestCounter{next: hostedServeHandler(c, tel, moved, u, srv, reg, a, admin)}
	ts := httptest.NewServer(counter)
	t.Cleanup(ts.Close)
	return &e2eHosted{
		hostedServer: &hostedServer{ts: ts, g: g, reg: reg, public: u.String(), cat: loader, events: events, tel: tel},
		r:            r, store: store, counter: counter, sets: sets, images: imgHost,
	}
}

// TestHostedSessionFullyTracedAndPrivate drives a full hosted session
// against a fake receiver: a Google sign-in through the connector, then
// an import naming cards and players, a new game, a malformed import, the
// board page, the live connection with one action, a card request for an
// image with no copy yet, and an admin check. Every HTTP request, tool
// call and board action is a span (none missing): every request the
// playmat served is a server span, and every request it made to Google, the
// published card sets, the image store and the image source is a client
// span. The image store write is
// inside the card request's trace; and nothing received identifies the
// player or holds what they pasted.
func TestHostedSessionFullyTracedAndPrivate(t *testing.T) {
	h := startE2E(t)
	issued := googleTokens(h.hostedServer)
	h.g.SetAccount(userA)
	cookie := h.cookie(t)
	toks := hostauthtest.Connect(t, h.public, h.ts.URL, h.g)
	cs := h.connectAs(t, "claude-ai", toks.AccessToken)

	deckText, logText := readTestdata(t, testDeckPath), readTestdata(t, testLogA)
	callObject(t, cs, "import_log", map[string]any{"log": logText, "deck": deckText, "id": "e2e-imp"})
	callObject(t, cs, "new_game", map[string]any{"deck_p1": deckText, "deck_p2": deckText, "id": "e2e-new"})
	const strayLine = "NicJackson played Mega Lucario ex before anything else."
	if out, isErr := toolResult(t, cs, "import_log", map[string]any{"log": strayLine + "\n" + logText, "deck": deckText, "id": "e2e-bad"}); !isErr {
		t.Fatalf("malformed import accepted: %s", out)
	}
	const toolCalls = 3
	const userAgent = "AGENTMARKER-e2e/1.0"
	handle := usergames.Handle(userA.ID)
	ua := func(r *http.Request) { r.Header.Set("User-Agent", userAgent) }
	if resp, _ := getWith(t, h.ts.URL+"/board?board="+handle, cookie, ua); resp.StatusCode != http.StatusOK {
		t.Fatalf("board page: %d", resp.StatusCode)
	}
	ws, _ := boardSnapshot(t, h.hostedServer, cookie)
	boardFail(t, ws, 1, "move", map[string]any{"iid": "p1-1", "to": "nowhere"})
	if resp, body := getWith(t, h.ts.URL+"/api/card?def=sv1-001", cookie, ua); resp.StatusCode != http.StatusOK {
		t.Fatalf("card request: %d %s", resp.StatusCode, body)
	}
	if resp, body := sendWith(t, http.MethodPost, h.ts.URL+"/admin/check", cookie, fromPlaymat); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("admin check: %d %s", resp.StatusCode, body)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, ok := h.store.Object("images/en/sv/sv1/001/high.webp"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("image not copied to the shared store")
		}
		time.Sleep(10 * time.Millisecond)
	}
	ws.Close(websocket.StatusNormalClosure, "")
	cs.Close()

	// Every request reached the playmat is a server span once the long
	// ones (the live connection, the MCP stream) have ended.
	var spans []observetest.Span
	servers := func() int {
		n := 0
		for _, s := range spans {
			if s.Kind == "server" {
				n++
			}
		}
		return n
	}
	for {
		spans = flushedSpans(t, h.hostedServer, h.r)
		if servers() >= h.counter.count() || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got, want := servers(), h.counter.count(); got != want {
		t.Errorf("%d server spans for %d requests the playmat served", got, want)
	}
	tools := 0
	for _, s := range spans {
		if strings.HasPrefix(s.Name, "tools/call ") {
			tools++
		}
	}
	if got := tools; got != toolCalls {
		t.Errorf("%d tools/call spans, want %d", got, toolCalls)
	}
	if got := len(spansNamed(spans, "board move")); got != 1 {
		t.Errorf("%d board move spans, want 1", got)
	}
	card := serverSpan(spans, "GET /api/")
	if card == nil {
		t.Fatal("no span for the card request")
	}
	storeHost := strings.TrimPrefix(h.store.Config().Endpoint, "http://")
	var putInTrace, fetchInTrace int
	for _, s := range spans {
		switch {
		case s.Kind != "client" || s.TraceID != card.TraceID:
		case s.Name == "PUT "+storeHost:
			putInTrace++
		case s.Name == "GET assets.tcgdex.net":
			fetchInTrace++
		}
	}
	if putInTrace == 0 {
		t.Errorf("no image store write inside the card request's trace %s", card.TraceID)
	}
	if fetchInTrace == 0 {
		t.Errorf("no image source fetch inside the card request's trace %s", card.TraceID)
	}
	// Every request the playmat made to an outside service, counted by
	// that service, is a client span.
	hostOf := func(raw string) string { u, _ := url.Parse(raw); return u.Host }
	clientSpans := func(host string) int {
		n := 0
		for _, s := range spans {
			if s.Kind == "client" && strings.HasSuffix(s.Name, " "+host) {
				n++
			}
		}
		return n
	}
	setRequests := 0
	for _, n := range h.sets.requests() {
		setRequests += n
	}
	for _, c := range []struct {
		what, host string
		made       int
	}{
		{"Google token exchanges (one per sign-in)", hostOf(h.g.Endpoint().TokenURL), len(h.g.Prompts())},
		{"card set fetches", hostOf(h.sets.URL), setRequests},
		{"image store requests", hostOf(h.store.Config().Endpoint), h.store.Total()},
		{"image source fetches", "assets.tcgdex.net", h.images.count()},
	} {
		if c.made == 0 {
			t.Errorf("no %s made; the check would prove nothing", c.what)
		}
		if got := clientSpans(c.host); got != c.made {
			t.Errorf("%d client spans to %s for %d %s", got, c.host, c.made, c.what)
		}
	}
	for _, name := range []string{"GET /auth/google/callback", "POST /admin/", "GET /board", "GET /ws", "POST /mcp"} {
		if serverSpan(spans, name) == nil {
			t.Errorf("no server span %q", name)
		}
	}

	// Nothing identifying is received, in any signal.
	tokenHost, _ := url.Parse(h.g.Endpoint().TokenURL)
	secrets := map[string]string{
		"email":                 userA.Email,
		"display name":          testDisplayName,
		"Google access token":   fakeGoogleAccessToken,
		"Google client secret":  h.g.ClientSecret,
		"playmat access token":  toks.AccessToken,
		"playmat refresh token": toks.RefreshToken,
		"board session cookie":  cookie.Value,
		"board handle":          handle,
		"user agent":            userAgent,
		"Go user agent":         "Go-http-client",
		"authorization code":    "code=",
		"OAuth state":           "state=",
		"board query":           "board=",
		"log player":            "NicJackson",
		"log opponent":          "LeDjo23",
		"malformed import line": strayLine,
	}
	ids := issued()
	if len(ids) == 0 {
		t.Fatal("the fake Google issued no ID token")
	}
	for i, tok := range ids {
		secrets["Google ID token "+string(rune('a'+i))] = tok
	}
	for _, l := range strings.Split(logText, "\n") {
		if l = strings.TrimSpace(l); len(l) >= 8 {
			secrets["log line "+l] = l
		}
	}
	list, err := deck.Parse(deckText)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range list.Entries {
		secrets["card "+e.Name] = e.Name
	}
	if len(h.r.Requests()) == 0 {
		t.Fatal("nothing received; the sweep would prove nothing")
	}
	for what, s := range secrets {
		if s == "" {
			t.Errorf("%s is empty; the check would prove nothing", what)
			continue
		}
		if h.r.Contains([]byte(s)) {
			t.Errorf("something sent carries the %s (%q)", what, s)
		}
	}
	for _, s := range spans {
		for _, k := range []string{"client.address", "network.peer.address", "network.peer.port", "user_agent.original", "url.query"} {
			if v, ok := s.Attrs[k]; ok {
				t.Errorf("span %q carries %s = %q", s.Name, k, v)
			}
		}
		if s.Kind == "client" && s.Name == "POST "+tokenHost.Host && strings.Contains(s.Attrs["url.full"], "?") {
			t.Errorf("Google call span keeps a query: %q", s.Attrs["url.full"])
		}
	}
	if !h.r.Contains([]byte(aID)) {
		t.Errorf("the player's ID %s was never sent; players should appear by ID", aID)
	}
}
