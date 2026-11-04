package cards

import (
	"context"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// TestWarmLoadsCatalogIntoBothTiers: Warm loads the set list and every
// set's card list once, through the store, so they land in the shared
// store; warming again, or warming a restarted client, asks the card
// source nothing.
func TestWarmLoadsCatalogIntoBothTiers(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	srv := spacestest.New(t)
	c, _ := newTieredClient(t, f, srv)
	ctx := context.Background()

	if err := c.Warm(ctx); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/sets", "/sets/sv01", "/sets/sv02"} {
		if n := f.hits(p); n != 1 {
			t.Errorf("%s fetched %d times, want 1", p, n)
		}
	}
	if n := f.hits("/cards/sv01-001"); n != 0 {
		t.Errorf("Warm fetched a card (%d requests), want only the catalog", n)
	}
	for _, key := range []string{"tcgdex/sets.json", "tcgdex/sets/sv01.json", "tcgdex/sets/sv02.json"} {
		if _, ok := srv.Object(key); !ok {
			t.Errorf("shared store lacks %s", key)
		}
	}

	before := f.totalHits()
	if err := c.Warm(ctx); err != nil {
		t.Fatal(err)
	}
	restarted, _ := newTieredClient(t, f, srv)
	if err := restarted.Warm(ctx); err != nil {
		t.Fatal(err)
	}
	if n := f.totalHits() - before; n != 0 {
		t.Errorf("warming again made %d card-source requests, want 0", n)
	}
	if id, err := restarted.Resolve(ctx, "PAL", "1"); err != nil || id != "sv02-001" {
		t.Errorf("Resolve after a warm restart = %q, %v", id, err)
	}
}

func TestWarmOfflineWithoutCacheFails(t *testing.T) {
	f := newFakeTCGdex(t)
	c, _ := newTestClient(t, f)
	c.Offline = true
	if err := c.Warm(context.Background()); err == nil {
		t.Error("Warm succeeded offline with an empty cache")
	}
}
