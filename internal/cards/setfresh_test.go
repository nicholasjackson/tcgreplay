package cards

import (
	"context"
	"testing"
	"time"
)

// Set bodies for the set-freshness tests, beside the shared ones.
const (
	// tSetListNew is tSetList with a set released since.
	tSetListNew = `[{"id":"sv01"},{"id":"sv02"},{"id":"new"}]`
	tSetNew     = `{"id":"new","name":"New Set","releaseDate":"2026-10-01","serie":{"id":"sv"},"abbreviation":{"official":"NEW"},"cards":[{"id":"new-001","localId":"001","name":"Newmon","image":"https://assets.tcgdex.net/en/sv/new/001"},{"id":"new-002","localId":"002","name":"Brandnew","image":"https://assets.tcgdex.net/en/sv/new/002"}]}`
	// tSetSV01Late is tSetSV01 with card 200, added to the set since.
	tSetSV01Late = `{"id":"sv01","name":"Scarlet & Violet","releaseDate":"2023-03-31","serie":{"id":"sv"},"abbreviation":{"official":"SVI"},"cards":[{"id":"sv01-001","localId":"001","name":"Pineco","image":"https://assets.tcgdex.net/en/sv/sv01/001"},{"id":"sv01-081","localId":"081","name":"Miraidon ex"},{"id":"sv01-200","localId":"200","name":"Latemon","image":"https://assets.tcgdex.net/en/sv/sv01/200"}]}`
	// tSetSV02Newmon is tSetSV02 with an older print of Newmon.
	tSetSV02Newmon = `{"id":"sv02","name":"Paldea Evolved","releaseDate":"2023-06-09","serie":{"id":"sv"},"abbreviation":{"official":"PAL"},"cards":[{"id":"sv02-001","localId":"001","name":"Pineco","image":"https://assets.tcgdex.net/en/sv/sv02/001"},{"id":"sv02-050","localId":"050","name":"Newmon","image":"https://assets.tcgdex.net/en/sv/sv02/050"}]}`
)

const day = 24 * time.Hour

// seedSetData caches tSetList fetched listAge before testEpoch, and
// tSetSV01 and tSetSV02 fetched setAge before it.
func seedSetData(t *testing.T, dir string, listAge, setAge time.Duration) {
	t.Helper()
	seedEnvelope(t, dir, "sets", tSetList, testEpoch.Add(-listAge))
	seedEnvelope(t, dir, "sets/sv01", tSetSV01, testEpoch.Add(-setAge))
	seedEnvelope(t, dir, "sets/sv02", tSetSV02, testEpoch.Add(-setAge))
}

// TestUnknownSetResolvesFirstTime: a deck naming a set the cached set list
// lacks resolves on the first import, whether the cached list is past its
// refresh age or not.
func TestUnknownSetResolvesFirstTime(t *testing.T) {
	for _, tc := range []struct {
		name    string
		listAge time.Duration
	}{
		{"stale list", 31 * day},
		{"young list", 2 * day},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeTCGdex(t)
			serveAll(f)
			f.put("/sets", tSetListNew)
			f.put("/sets/new", tSetNew)
			dir := t.TempDir()
			seedSetData(t, dir, tc.listAge, 10*day)
			c, _ := newTestClientDir(t, dir, f)
			t.Cleanup(c.waitBackground)

			id, err := c.Resolve(context.Background(), "NEW", "1")
			if err != nil {
				t.Fatalf("first Resolve(NEW, 1): %v", err)
			}
			if id != "new-001" {
				t.Errorf("NEW 1 resolved to %s, want new-001", id)
			}
		})
	}
}

// TestUnknownSetYoungListOneRequest: with the set list younger than 30
// days, resolving a new set's code asks for the set list once.
func TestUnknownSetYoungListOneRequest(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	f.put("/sets", tSetListNew)
	f.put("/sets/new", tSetNew)
	dir := t.TempDir()
	seedSetData(t, dir, 2*day, 10*day)
	c, _ := newTestClientDir(t, dir, f)
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	if id, err := c.Resolve(ctx, "NEW", "2"); err != nil || id != "new-002" {
		t.Fatalf("Resolve(NEW, 2) = %q, %v; want new-002", id, err)
	}
	c.waitBackground()
	if n := f.hits("/sets"); n != 1 {
		t.Errorf("%d set list requests, want 1", n)
	}
	// The new set is now in the catalog: another of its cards needs no request.
	before := f.totalHits()
	if id, err := c.Resolve(ctx, "NEW", "1"); err != nil || id != "new-001" {
		t.Fatalf("Resolve(NEW, 1) = %q, %v; want new-001", id, err)
	}
	if n := f.totalHits(); n != before {
		t.Errorf("resolving another card of the new set made %d requests, want 0", n-before)
	}
}

// TestMissingNumberResolvesFirstTime: a deck naming a card that a recently
// cached set lacks, but the card source now has, resolves on the first
// import, and the refreshed set serves later lookups.
func TestMissingNumberResolvesFirstTime(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	f.put("/sets/sv01", tSetSV01Late)
	dir := t.TempDir()
	seedSetData(t, dir, 2*day, 10*day)
	c, _ := newTestClientDir(t, dir, f)
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	id, err := c.Resolve(ctx, "SVI", "200")
	if err != nil {
		t.Fatalf("first Resolve(SVI, 200): %v", err)
	}
	if id != "sv01-200" {
		t.Errorf("SVI 200 resolved to %s, want sv01-200", id)
	}
	if n := f.hits("/sets/sv01"); n != 1 {
		t.Errorf("%d requests for sv01, want 1", n)
	}
	if n := f.hits("/sets"); n != 0 {
		t.Errorf("%d set list requests, want 0", n)
	}
	if id, err := c.Resolve(ctx, "SVI", "200"); err != nil || id != "sv01-200" {
		t.Fatalf("second Resolve(SVI, 200) = %q, %v", id, err)
	}
	if id, err := c.ByName(ctx, "Latemon"); err != nil || id != "sv01-200" {
		t.Errorf("ByName(Latemon) = %q, %v; want sv01-200", id, err)
	}
	c.waitBackground()
	if n := f.totalHits(); n != 1 {
		t.Errorf("%d requests in all, want 1", n)
	}
}

// TestUnknownCodeMissOncePerRetry: looking up the same unknown set code
// twice within 5 minutes asks for the set list once; after 5 minutes the
// miss may ask again.
func TestUnknownCodeMissOncePerRetry(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	dir := t.TempDir()
	seedSetData(t, dir, 2*day, 10*day)
	c, clock := newTestClientDir(t, dir, f)
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	for range 2 {
		if _, err := c.Resolve(ctx, "ZZZ", "1"); err == nil {
			t.Fatal("unknown set code resolved")
		}
	}
	c.waitBackground()
	if n := f.hits("/sets"); n != 1 {
		t.Errorf("%d set list requests within 5 minutes, want 1", n)
	}
	clock.Advance(4 * time.Minute)
	c.Resolve(ctx, "ZZZ", "1")
	c.waitBackground()
	if n := f.hits("/sets"); n != 1 {
		t.Errorf("%d set list requests after 4 minutes, want 1", n)
	}
	clock.Advance(2 * time.Minute)
	if _, err := c.Resolve(ctx, "ZZZ", "1"); err == nil {
		t.Fatal("unknown set code resolved")
	}
	c.waitBackground()
	if n := f.hits("/sets"); n != 2 {
		t.Errorf("%d set list requests after 6 minutes, want 2", n)
	}
}

// TestMissingNumberMissOncePerRetry: looking up the same number missing
// from a set twice within 5 minutes asks for that set once; after 5
// minutes the miss may ask again.
func TestMissingNumberMissOncePerRetry(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	dir := t.TempDir()
	seedSetData(t, dir, 2*day, 10*day)
	c, clock := newTestClientDir(t, dir, f)
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	for range 2 {
		if _, err := c.Resolve(ctx, "SVI", "999"); err == nil {
			t.Fatal("missing number resolved")
		}
	}
	c.waitBackground()
	if n := f.hits("/sets/sv01"); n != 1 {
		t.Errorf("%d requests for sv01 within 5 minutes, want 1", n)
	}
	clock.Advance(6 * time.Minute)
	if _, err := c.Resolve(ctx, "SVI", "999"); err == nil {
		t.Fatal("missing number resolved")
	}
	c.waitBackground()
	if n := f.hits("/sets/sv01"); n != 2 {
		t.Errorf("%d requests for sv01 after 6 minutes, want 2", n)
	}
	if n := f.hits("/sets"); n != 0 {
		t.Errorf("%d set list requests, want 0", n)
	}
}

// TestYoungSetDataNoRequests: with the set list younger than 30 days and
// the sets younger than 90 days, looking up their cards by code or name
// makes no request to the card source.
func TestYoungSetDataNoRequests(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	dir := t.TempDir()
	seedSetData(t, dir, 10*day, 50*day)
	c, _ := newTestClientDir(t, dir, f)
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	if id, err := c.Resolve(ctx, "SVI", "1"); err != nil || id != "sv01-001" {
		t.Fatalf("Resolve(SVI, 1) = %q, %v", id, err)
	}
	if id, err := c.Resolve(ctx, "PAL", "001"); err != nil || id != "sv02-001" {
		t.Fatalf("Resolve(PAL, 001) = %q, %v", id, err)
	}
	if id, err := c.ByName(ctx, "Miraidon ex"); err != nil || id != "sv01-081" {
		t.Fatalf("ByName(Miraidon ex) = %q, %v", id, err)
	}
	c.waitBackground()
	if n := f.totalHits(); n != 0 {
		t.Errorf("%d requests to the card source, want 0", n)
	}
}

// TestStaleListServedThenRefreshed: with the set list older than 30 days,
// a known code resolves at once from the stale list, without waiting on
// the background refresh, which asks for the list once and loads the set
// it newly names.
func TestStaleListServedThenRefreshed(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	f.put("/sets", tSetListNew)
	f.put("/sets/new", tSetNew)
	dir := t.TempDir()
	seedSetData(t, dir, 31*day, 10*day)
	c, _ := newTestClientDir(t, dir, f)
	c.Timeout = 10 * time.Second // the held set request must not time out
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	f.hold() // the refresh stalls loading the new set
	within(t, 2*time.Second, "Resolve with a stale set list", func() {
		id, err := c.Resolve(ctx, "SVI", "1")
		if err != nil || id != "sv01-001" {
			t.Errorf("Resolve(SVI, 1) = %q, %v", id, err)
		}
	})
	waitFor(t, 5*time.Second, "the background request for the new set", func() bool {
		return f.hits("/sets/new") > 0
	})
	f.release()
	c.waitBackground()
	if n := f.hits("/sets"); n != 1 {
		t.Errorf("%d set list requests, want 1", n)
	}
	if n := f.hits("/sets/new"); n != 1 {
		t.Errorf("%d requests for the new set, want 1", n)
	}
	// The sets fetched 10 days ago were not due, so were not asked for.
	if n := f.hits("/sets/sv01") + f.hits("/sets/sv02"); n != 0 {
		t.Errorf("%d requests for young sets, want 0", n)
	}
	// The refreshed catalog knows the new set's code with no further request.
	before := f.totalHits()
	if id, err := c.Resolve(ctx, "NEW", "1"); err != nil || id != "new-001" {
		t.Fatalf("Resolve(NEW, 1) after the refresh = %q, %v", id, err)
	}
	c.waitBackground()
	if n := f.totalHits(); n != before {
		t.Errorf("%d requests after the refresh, want 0", n-before)
	}
}

// TestStaleSetRefreshedInBackground: a set older than 90 days is served,
// then refreshed in the background with one request for it alone, and
// the refreshed card list serves later lookups.
func TestStaleSetRefreshedInBackground(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	f.put("/sets/sv01", tSetSV01Late)
	dir := t.TempDir()
	seedEnvelope(t, dir, "sets", tSetList, testEpoch.Add(-10*day))
	seedEnvelope(t, dir, "sets/sv01", tSetSV01, testEpoch.Add(-91*day))
	seedEnvelope(t, dir, "sets/sv02", tSetSV02, testEpoch.Add(-50*day))
	c, _ := newTestClientDir(t, dir, f)
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	if id, err := c.Resolve(ctx, "SVI", "81"); err != nil || id != "sv01-081" {
		t.Fatalf("Resolve(SVI, 81) = %q, %v", id, err)
	}
	c.waitBackground()
	if n := f.hits("/sets/sv01"); n != 1 {
		t.Errorf("%d requests for sv01, want 1", n)
	}
	if n := f.hits("/sets/sv02"); n != 0 {
		t.Errorf("%d requests for sv02 (50 days old), want 0", n)
	}
	if n := f.hits("/sets"); n != 0 {
		t.Errorf("%d set list requests (10 days old), want 0", n)
	}
	requireEnvelope(t, dir, "sets/sv01", tSetSV01Late, testEpoch)
	// Card 200 is in the refreshed list: no miss, no further request.
	if id, err := c.Resolve(ctx, "SVI", "200"); err != nil || id != "sv01-200" {
		t.Fatalf("Resolve(SVI, 200) after the refresh = %q, %v", id, err)
	}
	c.waitBackground()
	if n := f.totalHits(); n != 1 {
		t.Errorf("%d requests in all, want 1", n)
	}
}

// TestNewSetFollowedByName: while the playmat runs, the set list passes
// 30 days, and the refresh it triggers adds a set: by-name lookups on the
// same client then pick that set's prints, with no restart.
func TestNewSetFollowedByName(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	f.put("/sets", tSetListNew)
	f.put("/sets/sv02", tSetSV02Newmon)
	f.put("/sets/new", tSetNew)
	dir := t.TempDir()
	seedEnvelope(t, dir, "sets", tSetList, testEpoch.Add(-29*day))
	seedEnvelope(t, dir, "sets/sv01", tSetSV01, testEpoch.Add(-10*day))
	seedEnvelope(t, dir, "sets/sv02", tSetSV02Newmon, testEpoch.Add(-10*day))
	c, clock := newTestClientDir(t, dir, f)
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	if id, err := c.ByName(ctx, "Newmon"); err != nil || id != "sv02-050" {
		t.Fatalf("ByName(Newmon) before = %q, %v; want the old print sv02-050", id, err)
	}
	if _, err := c.ByName(ctx, "Brandnew"); err == nil {
		t.Fatal("ByName(Brandnew) found before the new set exists")
	}
	c.waitBackground()
	if n := f.totalHits(); n != 0 {
		t.Fatalf("%d requests with young set data, want 0", n)
	}

	clock.Advance(2 * day) // the set list is now 31 days old
	// This lookup is answered from the catalog it found, and starts the
	// refresh.
	if id, err := c.ByName(ctx, "Newmon"); err != nil || id != "sv02-050" {
		t.Fatalf("ByName(Newmon) at the stale point = %q, %v", id, err)
	}
	c.waitBackground()
	if n := f.hits("/sets"); n != 1 {
		t.Errorf("%d set list requests, want 1", n)
	}

	if id, err := c.ByName(ctx, "Newmon"); err != nil || id != "new-001" {
		t.Errorf("ByName(Newmon) after the refresh = %q, %v; want new-001", id, err)
	}
	if id, err := c.ByName(ctx, "Brandnew"); err != nil || id != "new-002" {
		t.Errorf("ByName(Brandnew) after the refresh = %q, %v; want new-002", id, err)
	}
}

// TestStaleSetDataSourceDown: with the card source down or slower than
// the timeout and all set data stale, deck codes and by-name lookups
// still answer from the stale copies with no error, before and after the
// failed refresh.
func TestStaleSetDataSourceDown(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(f *fakeTCGdex)
	}{
		{"down", func(f *fakeTCGdex) { f.close() }},
		{"slow", func(f *fakeTCGdex) { f.setDelay(time.Second) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeTCGdex(t)
			serveAll(f)
			f.put("/sets", tSetListNew)
			f.put("/sets/new", tSetNew)
			tc.setup(f)
			dir := t.TempDir()
			seedSetData(t, dir, 31*day, 91*day)
			c, _ := newTestClientDir(t, dir, f)
			t.Cleanup(c.waitBackground)
			ctx := context.Background()

			for i := range 2 {
				if id, err := c.Resolve(ctx, "SVI", "81"); err != nil || id != "sv01-081" {
					t.Fatalf("pass %d: Resolve(SVI, 81) = %q, %v", i, id, err)
				}
				if id, err := c.Resolve(ctx, "PAL", "1"); err != nil || id != "sv02-001" {
					t.Fatalf("pass %d: Resolve(PAL, 1) = %q, %v", i, id, err)
				}
				if id, err := c.ByName(ctx, "Pineco"); err != nil || id != "sv02-001" {
					t.Fatalf("pass %d: ByName(Pineco) = %q, %v", i, id, err)
				}
				c.waitBackground()
			}
			// The failed refresh kept the stale copies.
			requireEnvelope(t, dir, "sets", tSetList, testEpoch.Add(-31*day))
			requireEnvelope(t, dir, "sets/sv01", tSetSV01, testEpoch.Add(-91*day))
		})
	}
}

// TestOfflineStaleSetData: offline, stale set data is served, misses are
// answered from it, and the card source is never contacted.
func TestOfflineStaleSetData(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	f.put("/sets", tSetListNew)
	f.put("/sets/new", tSetNew)
	f.put("/sets/sv01", tSetSV01Late)
	dir := t.TempDir()
	seedSetData(t, dir, 31*day, 91*day)
	c, _ := newTestClientDir(t, dir, f)
	c.Offline = true
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	if id, err := c.Resolve(ctx, "SVI", "1"); err != nil || id != "sv01-001" {
		t.Fatalf("Resolve(SVI, 1) = %q, %v", id, err)
	}
	if id, err := c.ByName(ctx, "Pineco"); err != nil || id != "sv02-001" {
		t.Fatalf("ByName(Pineco) = %q, %v", id, err)
	}
	if _, err := c.Resolve(ctx, "NEW", "1"); err == nil {
		t.Error("set absent from the cached list resolved offline")
	}
	if _, err := c.Resolve(ctx, "SVI", "200"); err == nil {
		t.Error("number absent from the cached set resolved offline")
	}
	c.waitBackground()
	if n := f.totalHits(); n != 0 {
		t.Errorf("offline client made %d requests to the card source", n)
	}
}
