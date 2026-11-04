package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/spaces"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
	"github.com/nicholasjackson/tcgreplay/internal/state"
	"github.com/nicholasjackson/tcgreplay/internal/webui"
)

// syncLog is a log destination safe to read while background goroutines
// write to it.
type syncLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *syncLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *syncLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// captureSyncLog redirects the standard logger to a syncLog for the rest
// of the test.
func captureSyncLog(t *testing.T) *syncLog {
	t.Helper()
	l := &syncLog{}
	prev := log.Writer()
	log.SetOutput(l)
	t.Cleanup(func() { log.SetOutput(prev) })
	return l
}

var spacesEnv = []string{spaces.EnvEndpoint, spaces.EnvBucket, spaces.EnvKey, spaces.EnvSecret}

// clearSpacesEnv unsets the shared-store settings for the rest of the
// test.
func clearSpacesEnv(t *testing.T) {
	t.Helper()
	for _, k := range spacesEnv {
		t.Setenv(k, "")
	}
}

// setSpacesEnv sets the shared-store settings to cfg for the rest of the
// test.
func setSpacesEnv(t *testing.T, cfg spaces.Config) {
	t.Helper()
	t.Setenv(spaces.EnvEndpoint, cfg.Endpoint)
	t.Setenv(spaces.EnvBucket, cfg.Bucket)
	t.Setenv(spaces.EnvKey, cfg.AccessKey)
	t.Setenv(spaces.EnvSecret, cfg.SecretKey)
}

// tinySource is a card source with one set, "tst", and one card, "x".
// While held, requests for the set's card list wait.
type tinySource struct {
	*httptest.Server
	mu   sync.Mutex
	gate chan struct{}
}

func newTinySource(t *testing.T) *tinySource {
	t.Helper()
	s := &tinySource{}
	bodies := map[string]string{
		"/sets":     `[{"id":"tst"}]`,
		"/sets/tst": `{"id":"tst","name":"Test","releaseDate":"2024-01-01","serie":{"id":"sv"},"abbreviation":{"official":"TST"},"cards":[{"id":"x","localId":"1","name":"Xmon"}]}`,
		"/cards/x":  `{"id":"x","localId":"1","name":"Xmon","category":"Pokemon","hp":60}`,
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		gate := s.gate
		s.mu.Unlock()
		if gate != nil && r.URL.Path == "/sets/tst" {
			select {
			case <-gate:
			case <-r.Context().Done():
				return
			}
		}
		body, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	t.Cleanup(s.release) // runs first: held requests end before Close waits on them
	return s
}

func (s *tinySource) hold() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gate = make(chan struct{})
}

func (s *tinySource) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gate != nil {
		close(s.gate)
		s.gate = nil
	}
}

// onlineConfig is a hosted config that is online against src.
func onlineConfig(t *testing.T, src *tinySource) config {
	t.Helper()
	return config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", cardBase: src.URL}
}

func cachedOnDisk(dir, rel string) bool {
	_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil
}

func TestHostedCardDBNoSettingsIsLocalOnly(t *testing.T) {
	logs := captureSyncLog(t)
	clearSpacesEnv(t)
	src := newTinySource(t)
	c := onlineConfig(t, src)

	db, shared, _ := hostedCardDB(c)
	if shared {
		t.Error("shared store on with no settings")
	}
	if _, ok := db.Store.(interface{ Local() cards.Store }); ok {
		t.Error("card database has a tiered store with no settings")
	}
	if _, err := db.Card(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if !cachedOnDisk(c.cacheDir, "cards/x.json") {
		t.Error("card not cached in the local cache directory")
	}
	out := logs.String()
	if !strings.Contains(out, "local only") {
		t.Errorf("log %q does not say the cache is local only", out)
	}
	if strings.Contains(out, "warning") || strings.Contains(out, "error") {
		t.Errorf("log %q has a warning or error", out)
	}
}

func TestHostedCardDBSharedStoreOn(t *testing.T) {
	logs := captureSyncLog(t)
	srv := spacestest.New(t)
	setSpacesEnv(t, srv.Config())
	src := newTinySource(t)
	c := onlineConfig(t, src)

	db, shared, _ := hostedCardDB(c)
	if !shared {
		t.Fatal("shared store off with all four settings")
	}
	if _, err := db.Card(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if _, ok := srv.Object("tcgdex/cards/x.json"); !ok {
		t.Error("card not saved in the shared store")
	}
	if !cachedOnDisk(c.cacheDir, "cards/x.json") {
		t.Error("card not cached in the local cache directory")
	}
	out := logs.String()
	if !strings.Contains(out, "backed by shared store") || !strings.Contains(out, spacestest.Bucket) {
		t.Errorf("log %q does not name the shared store", out)
	}
	if strings.Contains(out, spacestest.SecretKey) || strings.Contains(out, spacestest.AccessKey) {
		t.Errorf("log %q contains a credential", out)
	}
}

func TestHostedCardDBPartialSettingsWarnAndRunLocal(t *testing.T) {
	const secret = "very-secret-value"
	cases := map[string]func(t *testing.T){}
	for _, missing := range spacesEnv {
		cases["no "+missing] = func(t *testing.T) {
			t.Setenv(spaces.EnvEndpoint, "https://ams3.digitaloceanspaces.com")
			t.Setenv(spaces.EnvBucket, "bucket")
			t.Setenv(spaces.EnvKey, "KEYID")
			t.Setenv(spaces.EnvSecret, secret)
			t.Setenv(missing, "")
		}
	}
	cases["bad endpoint"] = func(t *testing.T) {
		t.Setenv(spaces.EnvEndpoint, "ams3.digitaloceanspaces.com")
		t.Setenv(spaces.EnvBucket, "bucket")
		t.Setenv(spaces.EnvKey, "KEYID")
		t.Setenv(spaces.EnvSecret, secret)
	}
	for name, setEnv := range cases {
		t.Run(name, func(t *testing.T) {
			logs := captureSyncLog(t)
			setEnv(t)
			src := newTinySource(t)
			c := onlineConfig(t, src)

			db, shared, _ := hostedCardDB(c)
			if shared {
				t.Error("shared store on with partial settings")
			}
			if _, err := db.Card(context.Background(), "x"); err != nil {
				t.Fatalf("card not served: %v", err)
			}
			if !cachedOnDisk(c.cacheDir, "cards/x.json") {
				t.Error("card not cached locally")
			}
			out := logs.String()
			if !strings.Contains(out, "warning") {
				t.Errorf("log %q has no warning", out)
			}
			if strings.Contains(name, "no ") && !strings.Contains(out, strings.TrimPrefix(name, "no ")) {
				t.Errorf("log %q does not name the missing setting", out)
			}
			if name == "bad endpoint" && !strings.Contains(out, spaces.EnvEndpoint) {
				t.Errorf("log %q does not name the endpoint setting", out)
			}
			if strings.Contains(out, secret) || strings.Contains(out, "KEYID") {
				t.Errorf("log %q contains a credential", out)
			}
			if strings.Count(strings.TrimSpace(out), "\n") != 0 {
				t.Errorf("warning spans several lines: %q", out)
			}
		})
	}
}

func TestHostedCardDBWrongCredentialsStillServes(t *testing.T) {
	logs := captureSyncLog(t)
	srv := spacestest.New(t)
	setSpacesEnv(t, srv.Config())
	srv.SetDeny(true)
	src := newTinySource(t)
	c := onlineConfig(t, src)

	db, shared, _ := hostedCardDB(c)
	if !shared {
		t.Fatal("shared store off")
	}
	card, err := db.Card(context.Background(), "x")
	if err != nil || card.Name != "Xmon" {
		t.Fatalf("Card = %+v, %v", card, err)
	}
	if out := logs.String(); !strings.Contains(out, "shared store unavailable") || !strings.Contains(out, "InvalidAccessKeyId") {
		t.Errorf("log %q does not record the shared-store failure", out)
	}
}

func TestHostedCardDBOfflineIgnoresSharedStore(t *testing.T) {
	logs := captureSyncLog(t)
	srv := spacestest.New(t)
	setSpacesEnv(t, srv.Config())
	src := newTinySource(t)
	c := onlineConfig(t, src)
	c.offline = true
	if err := cards.NewDiskStore(c.cacheDir).Put(context.Background(), "cards/x", cards.Entry{
		Data: []byte(`{"id":"x","name":"Xmon","category":"Pokemon"}`), Fetched: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	db, shared, _ := hostedCardDB(c)
	if shared {
		t.Error("shared store on in offline mode")
	}
	ctx := context.Background()
	if _, err := db.Card(ctx, "x"); err != nil {
		t.Errorf("Card: %v", err)
	}
	db.Card(ctx, "missing")
	db.Resolve(ctx, "TST", "1")
	if db.Cached("x") == nil {
		t.Error("Cached(x) = nil")
	}
	if n := srv.Total(); n != 0 {
		t.Errorf("shared-store requests offline = %d, want 0", n)
	}
	if out := logs.String(); !strings.Contains(out, "not used") {
		t.Errorf("log %q does not say the shared store is not used", out)
	}
}

// TestDesktopNeverUsesSharedStore: the desktop, daemon and dev builds
// build their card database with newCardDB, which never reads the
// shared-store settings.
func TestDesktopNeverUsesSharedStore(t *testing.T) {
	logs := captureSyncLog(t)
	srv := spacestest.New(t)
	setSpacesEnv(t, srv.Config())
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2"}

	m, _ := newServer(c, "")
	db, ok := m.DB.(*cards.Client)
	if !ok {
		t.Fatalf("desktop card database is %T", m.DB)
	}
	if _, tiered := db.Store.(interface{ Local() cards.Store }); tiered {
		t.Error("desktop card database has a tiered store")
	}
	if _, tiered := newCardDB(c).Store.(interface{ Local() cards.Store }); tiered {
		t.Error("newCardDB built a tiered store")
	}
	if out := logs.String(); strings.Contains(out, "card cache") {
		t.Errorf("desktop logged about the card cache: %q", out)
	}
	if srv.Total() != 0 {
		t.Errorf("shared-store requests = %d, want 0", srv.Total())
	}
}

// TestRunHostedWarmsCatalogInBackground: with the shared store on, hosted
// mode answers its health check while the catalog warm is still waiting
// on the card source, then logs the warm when it finishes and leaves the
// catalog in the shared store.
func TestRunHostedWarmsCatalogInBackground(t *testing.T) {
	logs := captureSyncLog(t)
	srv := spacestest.New(t)
	src := newTinySource(t)
	src.hold()
	base := "http://127.0.0.1:" + freePort(t)
	c := hostedConfig(t, strings.TrimPrefix(base, "http://"), "https://play.example.com")
	c.offline = false
	c.cardBase = src.URL
	setSpacesEnv(t, srv.Config())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runHosted(ctx, c) }()

	waitHealthy(t, base, 5*time.Second, done, logs)
	if strings.Contains(logs.String(), "catalog warm") {
		t.Fatalf("warm finished while the card source was held: %q", logs.String())
	}
	src.release()

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), "catalog warmed") {
		if time.Now().After(deadline) {
			t.Fatalf("no catalog warm logged: %q", logs.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, key := range []string{"tcgdex/sets.json", "tcgdex/sets/tst.json"} {
		if _, ok := srv.Object(key); !ok {
			t.Errorf("shared store lacks %s after the warm", key)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runHosted: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runHosted did not return after cancel")
	}
	if out := logs.String(); strings.Contains(out, "warning") {
		t.Errorf("log has a warning: %q", out)
	}
}

// imageHost is a card image source serving any low.webp or high.webp and
// counting requests.
type imageHost struct {
	*httptest.Server
	mu   sync.Mutex
	hits int
}

func newImageHost(t *testing.T) *imageHost {
	t.Helper()
	h := &imageHost{}
	h.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.hits++
		h.mu.Unlock()
		if !strings.HasSuffix(r.URL.Path, "/low.webp") && !strings.HasSuffix(r.URL.Path, "/high.webp") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		w.Write([]byte("RIFF....WEBP " + r.URL.Path))
	}))
	t.Cleanup(h.Close)
	return h
}

func (h *imageHost) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hits
}

// artSource is a card source with one card, "y", whose image is under
// images.
func artSource(t *testing.T, images string) *httptest.Server {
	t.Helper()
	card := `{"id":"y","localId":"1","name":"Ymon","category":"Pokemon","hp":60,"image":"` + images + `/en/sv/sv1/1"}`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cards/y" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(card))
	}))
	t.Cleanup(s.Close)
	return s
}

// boardCard asks a hosted board built over db and images for card y.
func boardCard(t *testing.T, db *cards.Client, images *cards.ImageCache) map[string]any {
	t.Helper()
	m := game.NewManager(t.TempDir(), db)
	ui := hostedBoard(m, state.P1, nil, nil, images)
	ts := httptest.NewServer(ui.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/card?def=y")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/card: %d", resp.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestHostedImagesCopiedAndServedFromSharedStore(t *testing.T) {
	logs := captureSyncLog(t)
	srv := spacestest.New(t)
	setSpacesEnv(t, srv.Config())
	host := newImageHost(t)
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", cardBase: artSource(t, host.URL).URL, imageBase: host.URL}

	db, shared, images := hostedCardDB(c)
	if !shared || images == nil {
		t.Fatalf("shared = %v, images = %v; want both on", shared, images)
	}
	public := srv.Config().PublicBase()
	if out := logs.String(); !strings.Contains(out, "card images: copied from the card source to the shared store, served from "+public+"/images/") {
		t.Errorf("log %q does not name the public image address", out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go images.Run(ctx)

	src := host.URL + "/en/sv/sv1/1"
	got := boardCard(t, db, images)
	if got["image"] != src {
		t.Errorf("first answer image = %v, want the card source %s", got["image"], src)
	}
	if _, ok := got["image_fallback"]; ok {
		t.Errorf("first answer has a fallback: %v", got)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		_, low := srv.Object("images/en/sv/sv1/1/low.webp")
		_, high := srv.Object("images/en/sv/sv1/1/high.webp")
		if low && high && srv.Public("images/en/sv/sv1/1/high.webp") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("image not copied to the shared store")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The copy is marked known just after its last put lands.
	for {
		got = boardCard(t, db, images)
		if got["image"] != src || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if want := public + "/images/en/sv/sv1/1"; got["image"] != want || got["image_fallback"] != src {
		t.Errorf("later answer = (%v, %v), want (%s, %s)", got["image"], got["image_fallback"], want, src)
	}
	if out := logs.String(); strings.Contains(out, "warning") {
		t.Errorf("log has a warning: %q", out)
	}
}

func TestHostedImagesOffWithoutTheSharedStore(t *testing.T) {
	cases := map[string]struct {
		env      func(t *testing.T, srv *spacestest.Server)
		offline  bool
		warnings int // warnings the shared card cache already logs
	}{
		"no settings": {env: func(t *testing.T, _ *spacestest.Server) { clearSpacesEnv(t) }},
		"partial settings": {env: func(t *testing.T, srv *spacestest.Server) {
			setSpacesEnv(t, srv.Config())
			t.Setenv("TCGREPLAY_SPACES_SECRET", "")
		}, warnings: 1},
		"offline": {env: func(t *testing.T, srv *spacestest.Server) { setSpacesEnv(t, srv.Config()) }, offline: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			logs := captureSyncLog(t)
			srv := spacestest.New(t)
			tc.env(t, srv)
			host := newImageHost(t)
			c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", cardBase: artSource(t, host.URL).URL, imageBase: host.URL, offline: tc.offline}
			if tc.offline {
				// Offline never reaches the card source: have the card cached.
				if err := cards.NewDiskStore(c.cacheDir).Put(context.Background(), "cards/y", cards.Entry{
					Data: []byte(`{"id":"y","name":"Ymon","category":"Pokemon","image":"` + host.URL + `/en/sv/sv1/1"}`), Fetched: time.Now(),
				}); err != nil {
					t.Fatal(err)
				}
			}

			db, _, images := hostedCardDB(c)
			if images != nil {
				t.Fatal("image cache built without the shared store")
			}
			got := boardCard(t, db, images)
			if got["image"] != host.URL+"/en/sv/sv1/1" {
				t.Errorf("image = %v, want the card source", got["image"])
			}
			if _, ok := got["image_fallback"]; ok {
				t.Errorf("answer has a fallback: %v", got)
			}
			if n := host.count(); n != 0 {
				t.Errorf("image source requests = %d, want 0", n)
			}
			if n := srv.Total(); n != 0 {
				t.Errorf("shared store requests = %d, want 0", n)
			}
			out := logs.String()
			if n := strings.Count(out, "warning"); n != tc.warnings {
				t.Errorf("log has %d warnings, want %d: %q", n, tc.warnings, out)
			}
			if strings.Contains(out, "card images") {
				t.Errorf("log mentions card images: %q", out)
			}
		})
	}
}

// TestDesktopBoardHasNoImageSource: the desktop board is built without an
// image source whatever the shared-store settings say, so its card
// answers are as before.
func TestDesktopBoardHasNoImageSource(t *testing.T) {
	srv := spacestest.New(t)
	setSpacesEnv(t, srv.Config())
	host := newImageHost(t)
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2"}
	db := newCardDB(c)
	db.Base = artSource(t, host.URL).URL
	m := game.NewManager(t.TempDir(), db)
	ui := webui.New(m, state.P1) // as startUI builds it
	if ui.Images != nil {
		t.Fatal("desktop board has an image source")
	}
	ts := httptest.NewServer(ui.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/card?def=y")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if _, ok := got["image_fallback"]; ok || got["image"] != host.URL+"/en/sv/sv1/1" {
		t.Errorf("desktop answer = %v", got)
	}
	if srv.Total() != 0 || host.count() != 0 {
		t.Errorf("desktop reached the shared store (%d) or image host (%d)", srv.Total(), host.count())
	}
}

// TestRunHostedAnswersWhileImageListingHangs: hosted mode answers its
// health check while the image cache's start-up listing is still waiting
// on the shared store.
func TestRunHostedAnswersWhileImageListingHangs(t *testing.T) {
	logs := captureSyncLog(t)
	srv := spacestest.New(t)
	release := srv.Hang()
	defer release()
	src := newTinySource(t)
	base := "http://127.0.0.1:" + freePort(t)
	c := hostedConfig(t, strings.TrimPrefix(base, "http://"), "https://play.example.com")
	c.offline = false
	c.cardBase = src.URL
	setSpacesEnv(t, srv.Config())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runHosted(ctx, c) }()

	deadline := time.Now().Add(5 * time.Second)
	for srv.Requests("LIST", cards.ImagePrefix) == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("image listing never started: %q", logs.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	waitHealthy(t, base, 2*time.Second, done, logs)
	if strings.Contains(logs.String(), "copies found") {
		t.Fatal("listing finished while the shared store was held")
	}
	release()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runHosted: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runHosted did not return after cancel")
	}
}
