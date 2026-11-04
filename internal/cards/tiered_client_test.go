package cards

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// newTieredClient is newTestClient with a tiered store: its cache
// directory in front of srv.
func newTieredClient(t *testing.T, f *fakeTCGdex, srv *spacestest.Server) (*Client, *fakeClock) {
	t.Helper()
	c, clock := newTestClient(t, f)
	c.Store = NewTieredStore(NewDiskStore(c.CacheDir), NewSpacesStore(srv.Client(t)))
	return c, clock
}

// seedSpaces puts body under key in srv only, as the Spaces store writes
// it, fetched at fetched.
func seedSpaces(t *testing.T, srv *spacestest.Server, key, body string, fetched time.Time) {
	t.Helper()
	b, err := encodeEnvelope(key, Entry{Data: []byte(body), Fetched: fetched})
	if err != nil {
		t.Fatal(err)
	}
	srv.SetObject(objectKey(key), b)
}

// TestCachedNeverReadsSharedStore: the cached-only lookup the game makes
// under its lock reads no further than local disk.
func TestCachedNeverReadsSharedStore(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	srv := spacestest.New(t)
	seedSpaces(t, srv, "cards/sv01-001", tCard001, testEpoch)
	c, _ := newTieredClient(t, f, srv)

	if cc := c.Cached("sv01-001"); cc != nil {
		t.Errorf("Cached of a Spaces-only card = %+v, want nil", cc)
	}
	if n := srv.Total(); n != 0 {
		t.Errorf("Cached made %d shared-store requests, want 0", n)
	}
	if n := f.totalHits(); n != 0 {
		t.Errorf("Cached made %d card-source requests, want 0", n)
	}
}

// TestOnlineCardFromSharedStoreFillsDisk: a card held only in the shared
// store comes back with no card-source request and lands on disk with its
// fetch time, where the cached-only lookup then finds it.
func TestOnlineCardFromSharedStoreFillsDisk(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	srv := spacestest.New(t)
	fetched := testEpoch.Add(-time.Hour)
	seedSpaces(t, srv, "cards/sv01-001", tCard001, fetched)
	c, _ := newTieredClient(t, f, srv)

	card, err := c.Card(context.Background(), "sv01-001")
	if err != nil {
		t.Fatal(err)
	}
	if card.Name != "Pineco" {
		t.Errorf("Card named %q, want Pineco", card.Name)
	}
	if n := f.hits("/cards/sv01-001"); n != 0 {
		t.Errorf("card-source requests = %d, want 0", n)
	}
	requireEnvelope(t, c.CacheDir, "cards/sv01-001", tCard001, fetched)

	// A fresh client on the same directory finds it with the cached-only
	// lookup, without touching the shared store.
	srv.Reset()
	c2, _ := newTestClientDir(t, c.CacheDir, f)
	c2.Store = NewTieredStore(NewDiskStore(c2.CacheDir), NewSpacesStore(srv.Client(t)))
	if cc := c2.Cached("sv01-001"); cc == nil || cc.Name != "Pineco" {
		t.Errorf("Cached after the copy = %+v", cc)
	}
	if n := srv.Total(); n != 0 {
		t.Errorf("shared-store requests = %d, want 0", n)
	}
}

// TestColdCardMissReadsSharedStoreOnce: a card in neither tier costs one
// shared read, one card-source fetch and one shared write; the
// single-flight recheck reads only the local tier.
func TestColdCardMissReadsSharedStoreOnce(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	srv := spacestest.New(t)
	c, _ := newTieredClient(t, f, srv)

	if _, err := c.Card(context.Background(), "sv01-081"); err != nil {
		t.Fatal(err)
	}
	if n := srv.Requests(http.MethodGet, "tcgdex/cards/sv01-081.json"); n != 1 {
		t.Errorf("shared GETs = %d, want 1", n)
	}
	if n := srv.Requests(http.MethodPut, "tcgdex/cards/sv01-081.json"); n != 1 {
		t.Errorf("shared PUTs = %d, want 1", n)
	}
	if n := f.hits("/cards/sv01-081"); n != 1 {
		t.Errorf("card-source requests = %d, want 1", n)
	}
}

// TestOfflineNeverReadsSharedStore: offline, with an empty disk and the
// shared store holding everything, every kind of lookup fails as not
// cached and the shared store receives no request.
func TestOfflineNeverReadsSharedStore(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	srv := spacestest.New(t)
	for key, body := range map[string]string{
		"sets": tSetList, "sets/sv01": tSetSV01, "sets/sv02": tSetSV02,
		"cards/sv01-001": tCard001, "cards/sv01-081": tCard081, "cards/sv02-001": tCard201,
	} {
		seedSpaces(t, srv, key, body, testEpoch)
	}
	c, _ := newTieredClient(t, f, srv)
	c.Offline = true
	ctx := context.Background()

	if _, err := c.Card(ctx, "sv01-001"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Card = %v, want not found offline", err)
	}
	if _, err := c.Resolve(ctx, "SVI", "1"); err == nil {
		t.Error("Resolve succeeded offline with an empty disk")
	}
	if _, err := c.ByName(ctx, "Pineco"); err == nil {
		t.Error("ByName succeeded offline with an empty disk")
	}
	if cc := c.Cached("sv01-081"); cc != nil {
		t.Errorf("Cached = %+v, want nil", cc)
	}
	c.waitBackground()
	if n := srv.Total(); n != 0 {
		t.Errorf("shared-store requests offline = %d, want 0", n)
	}
	if n := f.totalHits(); n != 0 {
		t.Errorf("card-source requests offline = %d, want 0", n)
	}
	if entries, _ := os.ReadDir(c.CacheDir); len(entries) != 0 {
		t.Errorf("offline lookups wrote %d entries to disk", len(entries))
	}
}

// TestOfflineWithDiskStillServesFromLocalTier: offline lookups of entries
// on disk work through a tiered store as they do through a disk store.
func TestOfflineWithDiskStillServesFromLocalTier(t *testing.T) {
	f := newFakeTCGdex(t)
	srv := spacestest.New(t)
	c, _ := newTieredClient(t, f, srv)
	for key, body := range map[string]string{
		"sets": tSetList, "sets/sv01": tSetSV01, "sets/sv02": tSetSV02, "cards/sv01-001": tCard001,
	} {
		seedEnvelope(t, c.CacheDir, key, body, testEpoch)
	}
	c.Offline = true
	ctx := context.Background()
	if id, err := c.Resolve(ctx, "SVI", "1"); err != nil || id != "sv01-001" {
		t.Errorf("Resolve = %q, %v", id, err)
	}
	if card, err := c.Card(ctx, "sv01-001"); err != nil || card.Name != "Pineco" {
		t.Errorf("Card = %+v, %v", card, err)
	}
	if n := srv.Total(); n != 0 {
		t.Errorf("shared-store requests = %d, want 0", n)
	}
}
