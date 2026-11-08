package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cardset"
	"github.com/nicholasjackson/tcgreplay/internal/catalog"
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

// TestHostedImagesPartialSettingsWarn: partial or invalid shared-store
// settings are a one-line warning naming the setting, never a credential,
// and boards load images from their sources.
func TestHostedImagesPartialSettingsWarn(t *testing.T) {
	const secret = "very-secret-value"
	full := func(t *testing.T, endpoint string) {
		t.Setenv(spaces.EnvEndpoint, endpoint)
		t.Setenv(spaces.EnvBucket, "bucket")
		t.Setenv(spaces.EnvKey, "KEYID")
		t.Setenv(spaces.EnvSecret, secret)
	}
	cases := map[string]func(t *testing.T){}
	for _, missing := range spacesEnv {
		cases["no "+missing] = func(t *testing.T) {
			full(t, "https://ams3.digitaloceanspaces.com")
			t.Setenv(missing, "")
		}
	}
	cases["bad endpoint"] = func(t *testing.T) { full(t, "ams3.digitaloceanspaces.com") }
	cases["bucket host endpoint"] = func(t *testing.T) { full(t, "https://bucket.ams3.digitaloceanspaces.com") }
	cases["CDN endpoint"] = func(t *testing.T) { full(t, "https://ams3.cdn.digitaloceanspaces.com") }
	for name, setEnv := range cases {
		t.Run(name, func(t *testing.T) {
			logs := captureSyncLog(t)
			setEnv(t)
			t.Setenv("TCGREPLAY_SPACES_CDN_TOKEN", "")
			c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2"}

			if images := hostedImages(c); images != nil {
				t.Error("image cache built with partial settings")
			}
			out := logs.String()
			if !strings.Contains(out, "warning: card images: ") || !strings.Contains(out, "; loading images from their sources") {
				t.Errorf("log %q has no image warning", out)
			}
			if strings.HasPrefix(name, "no ") && !strings.Contains(out, strings.TrimPrefix(name, "no ")) {
				t.Errorf("log %q does not name the missing setting", out)
			}
			if strings.Contains(name, "endpoint") && !strings.Contains(out, spaces.EnvEndpoint) {
				t.Errorf("log %q does not name the endpoint setting", out)
			}
			if strings.Contains(name, "host") || strings.Contains(name, "CDN") {
				if !strings.Contains(out, "https://<region>.digitaloceanspaces.com") {
					t.Errorf("log %q does not give the region form", out)
				}
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

// TestDesktopNeverUsesSharedStore: the desktop, daemon and dev builds
// load their cards through the catalogue newServer is given, never the
// shared-store settings, and build no image cache.
func TestDesktopNeverUsesSharedStore(t *testing.T) {
	logs := captureSyncLog(t)
	srv := spacestest.New(t)
	setSpacesEnv(t, srv.Config())
	host := publishCardsets(t, adminSets()...)
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", cardsetsURL: host.dir}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m, _ := newServer(c, "", startCatalog(ctx, c, 5*time.Second))
	if _, ok := m.CardDB().(*catalog.Snapshot); !ok {
		t.Fatalf("desktop card database is %T, want a catalogue snapshot", m.CardDB())
	}
	if card := m.CardDB().Cached("me01-001"); card == nil || card.Name != "Bulbasaur" {
		t.Errorf("desktop catalogue has me01-001 = %+v", card)
	}
	if out := logs.String(); strings.Contains(out, "card images") || strings.Contains(out, "shared store") {
		t.Errorf("desktop logged about the shared store: %q", out)
	}
	if srv.Total() != 0 {
		t.Errorf("shared-store requests = %d, want 0", srv.Total())
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

// pinecoImage is the TCGdex image of sv1-001 as adminSets publish it.
const pinecoImage = tcgdexAssets + "/en/sv/sv1/001"

// boardCard asks a hosted board, over a manager pinning its cards from
// src and loading images through images, for sv1-001 and returns the first
// entry of its image list (empty when it has none).
func boardCard(t *testing.T, src game.CardSource, images *cards.ImageCache) map[string]any {
	t.Helper()
	m := game.NewManager(t.TempDir(), src)
	ui := hostedBoard(m, state.P1, nil, nil, images)
	ts := httptest.NewServer(ui.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/card?def=sv1-001")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/card: %d", resp.StatusCode)
	}
	var got struct {
		Images []map[string]any `json:"images"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Images) == 0 {
		return map[string]any{}
	}
	return got.Images[0]
}

// TestHostedImagesCopiedAndServedFromSharedStore: with the shared store
// on, a published card's TCGdex image is answered from its source first,
// copied to the bucket, then answered from the bucket's public address
// with the source as fallback. The image cache's requests for TCGdex's
// image host go to a local stand-in.
func TestHostedImagesCopiedAndServedFromSharedStore(t *testing.T) {
	logs := captureSyncLog(t)
	srv := spacestest.New(t)
	setSpacesEnv(t, srv.Config())
	t.Setenv("TCGREPLAY_SPACES_CDN_TOKEN", "")
	host := newImageHost(t)
	cat := publishedCatalog(t, adminSets()...)
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2"}

	images := hostedImages(c)
	if images == nil {
		t.Fatal("image cache off with all four settings")
	}
	if images.Source != tcgdexAssets {
		t.Fatalf("image source %q, want %s", images.Source, tcgdexAssets)
	}
	route := newReroute(t, "", host.URL)
	images.HTTP = route.client()
	public := srv.Config().PublicBase()
	out := logs.String()
	if !strings.Contains(out, "card images: copied from the card source to the shared store ") || !strings.Contains(out, spacestest.Bucket) ||
		!strings.Contains(out, "served from "+public+"/images/") {
		t.Errorf("log %q does not name the shared store and the public image address", out)
	}
	if strings.Contains(out, spacestest.SecretKey) || strings.Contains(out, spacestest.AccessKey) {
		t.Errorf("log %q contains a credential", out)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go images.Run(ctx)

	got := boardCard(t, cat, images)
	if got["image"] != pinecoImage {
		t.Errorf("first answer image = %v, want the card source %s", got["image"], pinecoImage)
	}
	if _, ok := got["image_fallback"]; ok {
		t.Errorf("first answer has a fallback: %v", got)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		_, low := srv.Object("images/en/sv/sv1/001/low.webp")
		_, high := srv.Object("images/en/sv/sv1/001/high.webp")
		if low && high && srv.Public("images/en/sv/sv1/001/high.webp") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("image not copied to the shared store")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if b, _ := srv.Object("images/en/sv/sv1/001/low.webp"); string(b) != "RIFF....WEBP /en/sv/sv1/001/low.webp" {
		t.Errorf("copy = %q, want the stand-in's bytes", b)
	}
	// The copy is marked known just after its last put lands.
	for {
		got = boardCard(t, cat, images)
		if got["image"] != pinecoImage || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if want := public + "/images/en/sv/sv1/001"; got["image"] != want || got["image_fallback"] != pinecoImage {
		t.Errorf("later answer = (%v, %v), want (%s, %s)", got["image"], got["image_fallback"], want, pinecoImage)
	}
	for _, u := range route.rerouted() {
		if !strings.HasPrefix(u, pinecoImage+"/") {
			t.Errorf("image cache fetched %s, want only sv1-001's files", u)
		}
	}
	if host.count() == 0 {
		t.Error("the image stand-in was never asked")
	}
	if out := logs.String(); strings.Contains(out, "warning") {
		t.Errorf("log has a warning: %q", out)
	}
}

// TestHostedImagesOffWithoutTheSharedStore: with no settings, partial
// settings or offline there is no image cache, boards answer the card's
// own image address, and nothing reaches the shared store.
func TestHostedImagesOffWithoutTheSharedStore(t *testing.T) {
	cases := map[string]struct {
		env      func(t *testing.T, srv *spacestest.Server)
		offline  bool
		warnings int
		want     string // the image cache's log line
	}{
		"no settings": {env: func(t *testing.T, _ *spacestest.Server) { clearSpacesEnv(t) },
			want: "card images: not copied, loaded from their sources (no TCGREPLAY_SPACES_* settings)"},
		"partial settings": {env: func(t *testing.T, srv *spacestest.Server) {
			setSpacesEnv(t, srv.Config())
			t.Setenv("TCGREPLAY_SPACES_SECRET", "")
		}, warnings: 1, want: "warning: card images: TCGREPLAY_SPACES_SECRET is not set; loading images from their sources"},
		"offline": {env: func(t *testing.T, srv *spacestest.Server) { setSpacesEnv(t, srv.Config()) }, offline: true,
			want: "card images: offline, shared store "},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			logs := captureSyncLog(t)
			srv := spacestest.New(t)
			tc.env(t, srv)
			t.Setenv("TCGREPLAY_SPACES_CDN_TOKEN", "")
			cat := publishedCatalog(t, adminSets()...)
			c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: tc.offline}

			images := hostedImages(c)
			if images != nil {
				t.Fatal("image cache built without the shared store")
			}
			got := boardCard(t, cat, images)
			if got["image"] != pinecoImage {
				t.Errorf("image = %v, want the card's own %s", got["image"], pinecoImage)
			}
			if _, ok := got["image_fallback"]; ok {
				t.Errorf("answer has a fallback: %v", got)
			}
			if n := srv.Total(); n != 0 {
				t.Errorf("shared store requests = %d, want 0", n)
			}
			out := logs.String()
			if n := strings.Count(out, "warning"); n != tc.warnings {
				t.Errorf("log has %d warnings, want %d: %q", n, tc.warnings, out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("log %q lacks %q", out, tc.want)
			}
		})
	}
}

// TestDesktopBoardHasNoImageSource: the desktop board is built without an
// image source whatever the shared-store settings say, so its card
// answers are the card's own image address.
func TestDesktopBoardHasNoImageSource(t *testing.T) {
	srv := spacestest.New(t)
	setSpacesEnv(t, srv.Config())
	m := game.NewManager(t.TempDir(), publishedCatalog(t, adminSets()...))
	ui := webui.New(m, state.P1) // as startUI builds it
	if ui.Images != nil {
		t.Fatal("desktop board has an image source")
	}
	ts := httptest.NewServer(ui.Handler())
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/card?def=sv1-001")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Images []map[string]any `json:"images"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if len(got.Images) != 1 || got.Images[0]["image"] != pinecoImage {
		t.Fatalf("desktop answer = %v", got)
	}
	if _, ok := got.Images[0]["image_fallback"]; ok {
		t.Errorf("desktop answer has a fallback: %v", got)
	}
	if srv.Total() != 0 {
		t.Errorf("desktop reached the shared store (%d)", srv.Total())
	}
}

// TestRunHostedAnswersWhileImageListingHangs: hosted mode loads the
// published card sets before it serves, then answers its health check
// while the image cache's start-up listing is still waiting on the shared
// store.
func TestRunHostedAnswersWhileImageListingHangs(t *testing.T) {
	logs := captureSyncLog(t)
	srv := spacestest.New(t)
	release := srv.Hang()
	defer release()
	host := publishCardsets(t, adminSets()...)
	base := "http://127.0.0.1:" + freePort(t)
	c := hostedConfig(t, strings.TrimPrefix(base, "http://"), "https://play.example.com")
	c.offline = false
	c.cardsetsURL = host.URL
	setSpacesEnv(t, srv.Config())
	t.Setenv("TCGREPLAY_SPACES_CDN_TOKEN", "")

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
	out := logs.String()
	for _, want := range []string{
		"card sets: loaded 3 sets (version " + host.version()[:12] + ") from " + host.URL,
		"card sets " + host.URL + " (local copy in " + c.cacheDir + ")",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}
	if host.requests()[cardset.ManifestKey] == 0 {
		t.Error("the published manifest was never fetched")
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
