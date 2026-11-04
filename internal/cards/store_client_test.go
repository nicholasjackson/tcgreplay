package cards

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Bodies shaped like TCGdex's, shared by the client tests.
const (
	tSetList = `[{"id":"sv01"},{"id":"sv02"}]`
	tSetSV01 = `{"id":"sv01","name":"Scarlet & Violet","releaseDate":"2023-03-31","serie":{"id":"sv"},"abbreviation":{"official":"SVI"},"cards":[{"id":"sv01-001","localId":"001","name":"Pineco","image":"https://assets.tcgdex.net/en/sv/sv01/001"},{"id":"sv01-081","localId":"081","name":"Miraidon ex"}]}`
	tSetSV02 = `{"id":"sv02","name":"Paldea Evolved","releaseDate":"2023-06-09","serie":{"id":"sv"},"abbreviation":{"official":"PAL"},"cards":[{"id":"sv02-001","localId":"001","name":"Pineco","image":"https://assets.tcgdex.net/en/sv/sv02/001"}]}`
	tCard001 = `{"id":"sv01-001","localId":"001","name":"Pineco","category":"Pokemon","hp":70,"image":"https://assets.tcgdex.net/en/sv/sv01/001","set":{"id":"sv01","name":"Scarlet & Violet"}}`
	tCard081 = `{"id":"sv01-081","localId":"081","name":"Miraidon ex","category":"Pokemon","hp":220,"set":{"id":"sv01","name":"Scarlet & Violet"}}`
	tCard201 = `{"id":"sv02-001","localId":"001","name":"Pineco","category":"Pokemon","hp":70,"image":"https://assets.tcgdex.net/en/sv/sv02/001","set":{"id":"sv02","name":"Paldea Evolved"}}`
)

// serveAll puts the shared bodies on the fake.
func serveAll(f *fakeTCGdex) {
	f.put("/sets", tSetList)
	f.put("/sets/sv01", tSetSV01)
	f.put("/sets/sv02", tSetSV02)
	f.put("/cards/sv01-001", tCard001)
	f.put("/cards/sv01-081", tCard081)
	f.put("/cards/sv02-001", tCard201)
}

// requireEnvelope checks the file for key is an envelope holding body,
// fetched at want.
func requireEnvelope(t *testing.T, dir, key, body string, want time.Time) {
	t.Helper()
	raw, err := os.ReadFile(diskStore{dir: dir}.path(key))
	if err != nil {
		t.Fatalf("%s: %v", key, err)
	}
	if !bytes.HasPrefix(raw, []byte(`{"fetched":`)) {
		t.Errorf("%s is not an envelope: %.60s", key, raw)
	}
	e, err := NewDiskStore(dir).Get(context.Background(), key)
	if err != nil {
		t.Fatalf("%s: %v", key, err)
	}
	if string(e.Data) != body {
		t.Errorf("%s body %.60s, want %.60s", key, e.Data, body)
	}
	if !e.Fetched.Equal(want) {
		t.Errorf("%s fetched %v, want %v", key, e.Fetched, want)
	}
}

// TestFetchedEntriesStored saves every card, set and set list fetched from
// the card source with the time it was fetched, and keeps a card's fetch
// time in memory too.
func TestFetchedEntriesStored(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	c, clock := newTestClient(t, f)
	ctx := context.Background()

	card, err := c.Card(ctx, "sv01-081")
	if err != nil {
		t.Fatal(err)
	}
	if card.Name != "Miraidon ex" || card.HP != 220 {
		t.Errorf("card %+v", card)
	}
	requireEnvelope(t, c.CacheDir, "cards/sv01-081", tCard081, testEpoch)
	c.mu.Lock()
	memFetched := c.mem["sv01-081"].fetched
	c.mu.Unlock()
	if !memFetched.Equal(testEpoch) {
		t.Errorf("in-memory fetch time %v, want %v", memFetched, testEpoch)
	}

	clock.Advance(time.Hour)
	id, err := c.Resolve(ctx, "SVI", "81")
	if err != nil {
		t.Fatal(err)
	}
	if id != "sv01-081" {
		t.Errorf("SVI 81 resolved to %s", id)
	}
	at := testEpoch.Add(time.Hour)
	requireEnvelope(t, c.CacheDir, "sets", tSetList, at)
	requireEnvelope(t, c.CacheDir, "sets/sv01", tSetSV01, at)
	requireEnvelope(t, c.CacheDir, "sets/sv02", tSetSV02, at)

	// By name, the newest print with art wins, as before.
	id, err = c.ByName(ctx, "Pineco")
	if err != nil {
		t.Fatal(err)
	}
	if id != "sv02-001" {
		t.Errorf("Pineco resolved to %s, want sv02-001", id)
	}
	// A second lookup of a cached card is served from memory.
	before := f.hits("/cards/sv01-081")
	if _, err := c.Card(ctx, "sv01-081"); err != nil {
		t.Fatal(err)
	}
	if got := f.hits("/cards/sv01-081"); got != before {
		t.Errorf("cached card refetched: %d requests, want %d", got, before)
	}
	// An unknown card is still ErrNotFound and is not stored.
	if _, err := c.Card(ctx, "sv01-999"); err == nil {
		t.Error("unknown card found")
	}
	if _, err := os.Stat(diskStore{dir: c.CacheDir}.path("cards/sv01-999")); err == nil {
		t.Error("unknown card cached")
	}
}

// TestLegacyCacheOffline opens a cache the previous release wrote (bare
// files, a persisted set index), mixed with entries in the new envelope,
// offline: every card, set and by-name lookup is served as before, and the
// card source is never contacted.
func TestLegacyCacheOffline(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	dir := t.TempDir()
	old := testEpoch.Add(-400 * 24 * time.Hour)
	seedLegacy(t, dir, "sets", tSetList, old)
	seedLegacy(t, dir, "sets/sv01", tSetSV01, old)
	seedEnvelope(t, dir, "sets/sv02", tSetSV02, old)
	seedLegacy(t, dir, "cards/sv01-001", tCard001, old)
	seedEnvelope(t, dir, "cards/sv01-081", tCard081, old)
	seedLegacy(t, dir, "cards/sv02-001", tCard201, old)
	if err := os.WriteFile(filepath.Join(dir, "set-index-v2.json"), []byte(`{"SVI":"sv01","PAL":"sv02"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _ := newTestClientDir(t, dir, f)
	c.Offline = true
	ctx := context.Background()

	for id, name := range map[string]string{"sv01-001": "Pineco", "sv01-081": "Miraidon ex", "sv02-001": "Pineco"} {
		card, err := c.Card(ctx, id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if card.Name != name {
			t.Errorf("%s named %s, want %s", id, card.Name, name)
		}
	}
	if cc := c.Cached("sv02-001"); cc == nil || cc.Name != "Pineco" {
		t.Errorf("Cached(sv02-001) = %+v", cc)
	}
	// Cached on a fresh client reads the legacy file straight from the store.
	c2, _ := newTestClientDir(t, dir, f)
	c2.Offline = true
	if cc := c2.Cached("sv01-001"); cc == nil || cc.Image == "" {
		t.Errorf("Cached(sv01-001) on a fresh client = %+v", cc)
	}
	for _, tc := range []struct{ code, num, want string }{
		{"SVI", "1", "sv01-001"},
		{"svi", "081", "sv01-081"},
		{"PAL", "1", "sv02-001"},
	} {
		id, err := c.Resolve(ctx, tc.code, tc.num)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.code, tc.num, err)
		}
		if id != tc.want {
			t.Errorf("%s %s resolved to %s, want %s", tc.code, tc.num, id, tc.want)
		}
	}
	for name, want := range map[string]string{"Pineco": "sv02-001", "Miraidon ex": "sv01-081"} {
		id, err := c.ByName(ctx, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if id != want {
			t.Errorf("%s resolved to %s, want %s", name, id, want)
		}
	}
	// A card the cache lacks is not found, offline.
	if _, err := c.Card(ctx, "sv01-002"); err == nil {
		t.Error("uncached card found offline")
	}
	if n := f.totalHits(); n != 0 {
		t.Errorf("offline client made %d requests to the card source", n)
	}
	// Nothing was rewritten: legacy files keep their old modification time.
	fi, err := os.Stat(filepath.Join(dir, "cards", "sv01-001.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !fi.ModTime().Equal(old) {
		t.Errorf("legacy card rewritten: mtime %v", fi.ModTime())
	}
}

// TestNewClientDefaults constructs the client exactly as the app does
// (NewClient, then Offline) and gets a working disk store, a 5-second
// source timeout and the real clock with no further settings. A client
// without a store or clock falls back to the same defaults.
func TestNewClientDefaults(t *testing.T) {
	dir := t.TempDir()
	c := NewClient(dir)
	c.Offline = false
	if c.Store == nil || c.Now == nil {
		t.Fatal("NewClient left the store or clock unset")
	}
	if c.Timeout != 5*time.Second {
		t.Errorf("timeout %v, want 5s", c.Timeout)
	}
	f := newFakeTCGdex(t)
	serveAll(f)
	c.Base = f.URL
	start := time.Now()
	if _, err := c.Card(context.Background(), "sv01-001"); err != nil {
		t.Fatal(err)
	}
	e, err := NewDiskStore(dir).Get(context.Background(), "cards/sv01-001")
	if err != nil {
		t.Fatal(err)
	}
	if e.Fetched.Before(start.Add(-time.Second)) || e.Fetched.After(time.Now().Add(time.Second)) {
		t.Errorf("fetched %v, not the real clock's time", e.Fetched)
	}

	bare := &Client{Base: f.URL, CacheDir: t.TempDir()}
	card, err := bare.Card(context.Background(), "sv01-081")
	if err != nil {
		t.Fatal(err)
	}
	if card.Name != "Miraidon ex" || bare.Cached("sv01-081") == nil {
		t.Errorf("bare client card %+v", card)
	}
	if _, err := os.Stat(filepath.Join(bare.CacheDir, "cards", "sv01-081.json")); err != nil {
		t.Errorf("bare client did not cache under CacheDir: %v", err)
	}
}
