package cards

import (
	"context"
	"os"
	"testing"
	"time"
)

const (
	tImageless = `{"id":"sv09-200","localId":"200","name":"Newmon","category":"Pokemon","hp":60,"set":{"id":"sv09","name":"New"}}`
	tWithImage = `{"id":"sv09-200","localId":"200","name":"Newmon","category":"Pokemon","hp":60,"image":"https://assets.tcgdex.net/en/sv/sv09/200","set":{"id":"sv09","name":"New"}}`
	tEnergy    = `{"id":"sve-006","localId":"006","name":"Basic Fighting Energy","category":"Energy","energyType":"Normal"}`
)

// lookups are the two ways a card is read: Card, and Cached (which never
// waits on the network).
var lookups = map[string]func(c *Client, id string) *Card{
	"Card": func(c *Client, id string) *Card {
		card, err := c.Card(context.Background(), id)
		if err != nil {
			return nil
		}
		return card
	},
	"Cached": func(c *Client, id string) *Card { return c.Cached(id) },
}

// TestCardWithArtKept: a card cached with artwork, however long ago, makes
// no request to the card source.
func TestCardWithArtKept(t *testing.T) {
	for name, look := range lookups {
		t.Run(name, func(t *testing.T) {
			f := newFakeTCGdex(t)
			f.put("/cards/sv09-200", tWithImage)
			dir := t.TempDir()
			seedEnvelope(t, dir, "cards/sv09-200", tWithImage, testEpoch.Add(-3*365*24*time.Hour))
			c, _ := newTestClientDir(t, dir, f)
			for range 3 {
				if card := look(c, "sv09-200"); card == nil || card.Image == "" {
					t.Fatalf("card %+v", card)
				}
			}
			c.waitBackground()
			if n := f.totalHits(); n != 0 {
				t.Errorf("%d requests for a card with art", n)
			}
		})
	}
}

// TestImagelessCardYoung: a card cached without artwork less than 24 hours
// ago makes no request.
func TestImagelessCardYoung(t *testing.T) {
	f := newFakeTCGdex(t)
	f.put("/cards/sv09-200", tWithImage)
	dir := t.TempDir()
	seedEnvelope(t, dir, "cards/sv09-200", tImageless, testEpoch.Add(-23*time.Hour))
	c, _ := newTestClientDir(t, dir, f)
	for _, look := range lookups {
		if card := look(c, "sv09-200"); card == nil || card.Image != "" {
			t.Fatalf("card %+v", card)
		}
	}
	c.waitBackground()
	if n := f.totalHits(); n != 0 {
		t.Errorf("%d requests for a card cached 23 hours ago", n)
	}
}

// TestLateArtAppears: a card cached without artwork more than 24 hours ago
// is returned at once from the cache; once the refresh finishes, lookups
// show the artwork the source now has, and the store holds the refreshed
// entry with its new fetch time.
func TestLateArtAppears(t *testing.T) {
	for name, look := range lookups {
		t.Run(name, func(t *testing.T) {
			f := newFakeTCGdex(t)
			f.put("/cards/sv09-200", tWithImage)
			f.setDelay(200 * time.Millisecond)
			dir := t.TempDir()
			seedEnvelope(t, dir, "cards/sv09-200", tImageless, testEpoch.Add(-25*time.Hour))
			c, clock := newTestClientDir(t, dir, f)
			clock.Advance(time.Minute)

			start := time.Now()
			card := look(c, "sv09-200")
			if card == nil || card.Image != "" {
				t.Fatalf("first lookup %+v, want the cached card without art", card)
			}
			if d := time.Since(start); d > 100*time.Millisecond {
				t.Errorf("first lookup waited %v for the refresh", d)
			}
			c.waitBackground()
			card = look(c, "sv09-200")
			if card == nil || card.Image == "" {
				t.Fatalf("after the refresh %+v, want the art", card)
			}
			if n := f.hits("/cards/sv09-200"); n != 1 {
				t.Errorf("%d requests, want 1", n)
			}
			requireEnvelope(t, dir, "cards/sv09-200", tWithImage, testEpoch.Add(time.Minute))
			// The refreshed card has art: no more requests, however late.
			clock.Advance(1000 * time.Hour)
			look(c, "sv09-200")
			c.waitBackground()
			if n := f.hits("/cards/sv09-200"); n != 1 {
				t.Errorf("%d requests after the refresh, want 1", n)
			}
		})
	}
}

// TestStaleCardServedWhenSourceFails: with the source down, failing or
// slower than the timeout, a stale card is still served with no error,
// and the source is asked again no sooner than 5 minutes later.
func TestStaleCardServedWhenSourceFails(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(f *fakeTCGdex)
	}{
		{"down", func(f *fakeTCGdex) { f.close() }},
		{"failing", func(f *fakeTCGdex) { f.setFail(true) }},
		{"slow", func(f *fakeTCGdex) { f.setDelay(time.Second) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeTCGdex(t)
			f.put("/cards/sv09-200", tWithImage)
			tc.setup(f)
			dir := t.TempDir()
			seedEnvelope(t, dir, "cards/sv09-200", tImageless, testEpoch.Add(-48*time.Hour))
			c, clock := newTestClientDir(t, dir, f)
			ctx := context.Background()
			for range 3 {
				card, err := c.Card(ctx, "sv09-200")
				if err != nil || card == nil || card.Name != "Newmon" {
					t.Fatalf("stale card: %+v, %v", card, err)
				}
				if c.Cached("sv09-200") == nil {
					t.Fatal("Cached lost the stale card")
				}
				c.waitBackground()
			}
			if n := f.hits("/cards/sv09-200"); n > 1 {
				t.Errorf("%d requests within 5 minutes, want at most 1", n)
			}
			c.mu.Lock()
			attempted := c.tracker.bg["cards/sv09-200"]
			c.mu.Unlock()
			if !attempted.Equal(testEpoch) {
				t.Errorf("refresh attempted at %v, want %v", attempted, testEpoch)
			}
			clock.Advance(4 * time.Minute)
			c.Card(ctx, "sv09-200")
			c.waitBackground()
			c.mu.Lock()
			again := c.tracker.bg["cards/sv09-200"]
			c.mu.Unlock()
			if !again.Equal(testEpoch) {
				t.Errorf("refresh retried after 4 minutes, at %v", again)
			}
			clock.Advance(2 * time.Minute)
			c.Card(ctx, "sv09-200")
			c.waitBackground()
			c.mu.Lock()
			again = c.tracker.bg["cards/sv09-200"]
			c.mu.Unlock()
			if !again.Equal(testEpoch.Add(6 * time.Minute)) {
				t.Errorf("refresh not retried after 6 minutes: last at %v", again)
			}
			// The stale copy is still what is stored.
			e, err := NewDiskStore(dir).Get(ctx, "cards/sv09-200")
			if err != nil || string(e.Data) != tImageless {
				t.Errorf("stored entry changed: %s, %v", e.Data, err)
			}
		})
	}
}

// TestStaleCardSlowSourceCounted: against a source slower than the
// timeout, the refresh is one request per 5 minutes.
func TestStaleCardSlowSourceCounted(t *testing.T) {
	f := newFakeTCGdex(t)
	f.put("/cards/sv09-200", tWithImage)
	f.setDelay(time.Second)
	dir := t.TempDir()
	seedEnvelope(t, dir, "cards/sv09-200", tImageless, testEpoch.Add(-48*time.Hour))
	c, clock := newTestClientDir(t, dir, f)
	for range 5 {
		c.Cached("sv09-200")
		c.waitBackground()
	}
	if n := f.hits("/cards/sv09-200"); n != 1 {
		t.Errorf("%d requests within 5 minutes, want 1", n)
	}
	clock.Advance(6 * time.Minute)
	c.Cached("sv09-200")
	c.waitBackground()
	if n := f.hits("/cards/sv09-200"); n != 2 {
		t.Errorf("%d requests after 6 minutes, want 2", n)
	}
}

// TestLegacyCardAges: a previous-release card file younger than 24 hours
// makes no request; an older imageless one is refreshed and saved as an
// envelope with its new fetch time.
func TestLegacyCardAges(t *testing.T) {
	f := newFakeTCGdex(t)
	f.put("/cards/sv09-200", tWithImage)
	f.put("/cards/sv09-201", `{"id":"sv09-201","name":"Oldmon","image":"https://x/201"}`)
	dir := t.TempDir()
	seedLegacy(t, dir, "cards/sv09-201", `{"id":"sv09-201","name":"Oldmon"}`, testEpoch.Add(-time.Hour))
	seedLegacy(t, dir, "cards/sv09-200", tImageless, testEpoch.Add(-25*time.Hour))
	c, _ := newTestClientDir(t, dir, f)
	ctx := context.Background()
	if card, err := c.Card(ctx, "sv09-201"); err != nil || card.Name != "Oldmon" {
		t.Fatalf("young legacy card: %+v, %v", card, err)
	}
	if card, err := c.Card(ctx, "sv09-200"); err != nil || card.Image != "" {
		t.Fatalf("old legacy card: %+v, %v", card, err)
	}
	c.waitBackground()
	if n := f.hits("/cards/sv09-201"); n != 0 {
		t.Errorf("%d requests for a legacy card 1 hour old", n)
	}
	if n := f.hits("/cards/sv09-200"); n != 1 {
		t.Errorf("%d requests for a legacy card 25 hours old, want 1", n)
	}
	requireEnvelope(t, dir, "cards/sv09-200", tWithImage, testEpoch)
	if card, _ := c.Card(ctx, "sv09-200"); card == nil || card.Image == "" {
		t.Errorf("refreshed card %+v, want art", card)
	}
}

// TestOfflineStaleCardsUntouched: offline, stale cards are served and the
// card source is never contacted.
func TestOfflineStaleCardsUntouched(t *testing.T) {
	f := newFakeTCGdex(t)
	f.put("/cards/sv09-200", tWithImage)
	dir := t.TempDir()
	seedEnvelope(t, dir, "cards/sv09-200", tImageless, testEpoch.Add(-400*24*time.Hour))
	seedLegacy(t, dir, "cards/sv09-201", `{"id":"sv09-201","name":"Oldmon"}`, testEpoch.Add(-400*24*time.Hour))
	c, _ := newTestClientDir(t, dir, f)
	c.Offline = true
	for _, id := range []string{"sv09-200", "sv09-201"} {
		for _, look := range lookups {
			if look(c, id) == nil {
				t.Errorf("offline stale %s not served", id)
			}
		}
	}
	c.waitBackground()
	if n := f.totalHits(); n != 0 {
		t.Errorf("offline client made %d requests", n)
	}
}

// TestBorrowedEnergyArtKept: a Basic Energy print whose art is borrowed
// counts as having art, so it is not refetched daily.
func TestBorrowedEnergyArtKept(t *testing.T) {
	f := newFakeTCGdex(t)
	f.put("/cards/sve-006", tEnergy)
	dir := t.TempDir()
	seedEnvelope(t, dir, "cards/sve-006", tEnergy, testEpoch.Add(-90*24*time.Hour))
	c, _ := newTestClientDir(t, dir, f)
	card, err := c.Card(context.Background(), "sve-006")
	if err != nil || card.Image == "" {
		t.Fatalf("energy %+v, %v", card, err)
	}
	c.Cached("sve-006")
	c.waitBackground()
	if n := f.totalHits(); n != 0 {
		t.Errorf("%d requests for an Energy with borrowed art", n)
	}
	if _, err := os.Stat(diskStore{dir: dir}.path("cards/sve-006")); err != nil {
		t.Fatal(err)
	}
}
