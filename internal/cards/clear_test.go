package cards

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// Bodies for the client clear tests: two sets, me01 (decklist code MEG)
// and sv1 (SVI), and a card in each.
const (
	cSetList  = `[{"id":"me01"},{"id":"sv1"}]`
	cSetME01  = `{"id":"me01","name":"Mega Evolution","releaseDate":"2025-09-26","serie":{"id":"me"},"abbreviation":{"official":"MEG"},"cards":[{"id":"me01-001","localId":"001","name":"Bulbasaur","image":"https://assets.tcgdex.net/en/me/me01/001"}]}`
	cSetME01b = `{"id":"me01","name":"Mega Evolution","releaseDate":"2025-09-26","serie":{"id":"me"},"abbreviation":{"official":"MEG"},"cards":[{"id":"me01-001","localId":"001","name":"Bulbasaur","image":"https://assets.tcgdex.net/en/me/me01/001"},{"id":"me01-009","localId":"009","name":"Latebloom","image":"https://assets.tcgdex.net/en/me/me01/009"}]}`
	cSetSV1   = `{"id":"sv1","name":"Scarlet & Violet","releaseDate":"2023-03-31","serie":{"id":"sv"},"abbreviation":{"official":"SVI"},"cards":[{"id":"sv1-001","localId":"001","name":"Pineco","image":"https://assets.tcgdex.net/en/sv/sv1/001"}]}`
	// cCardME01 is me01-001 before the card source changed it, cCardME01b
	// after.
	cCardME01  = `{"id":"me01-001","localId":"001","name":"Bulbasaur","category":"Pokemon","hp":70,"image":"https://assets.tcgdex.net/en/me/me01/001","set":{"id":"me01","name":"Mega Evolution"}}`
	cCardME01b = `{"id":"me01-001","localId":"001","name":"Bulbasaur","category":"Pokemon","hp":80,"image":"https://assets.tcgdex.net/en/me/me01/001","set":{"id":"me01","name":"Mega Evolution"}}`
	cCardME09  = `{"id":"me01-009","localId":"009","name":"Latebloom","category":"Pokemon","hp":60,"image":"https://assets.tcgdex.net/en/me/me01/009","set":{"id":"me01","name":"Mega Evolution"}}`
	cCardSV1   = `{"id":"sv1-001","localId":"001","name":"Pineco","category":"Pokemon","hp":50,"image":"https://assets.tcgdex.net/en/sv/sv1/001","set":{"id":"sv1","name":"Scarlet & Violet"}}`
)

// serveClearSets serves the clear tests' sets and cards.
func serveClearSets(f *fakeTCGdex) {
	f.put("/sets", cSetList)
	f.put("/sets/me01", cSetME01)
	f.put("/sets/sv1", cSetSV1)
	f.put("/cards/me01-001", cCardME01)
	f.put("/cards/sv1-001", cCardSV1)
}

// mustCard looks up id and fails the test unless it has hp.
func mustCard(t *testing.T, c *Client, id string, hp int) {
	t.Helper()
	card, err := c.Card(context.Background(), id)
	if err != nil {
		t.Fatalf("Card(%s): %v", id, err)
	}
	if card.HP != hp {
		t.Fatalf("Card(%s) HP = %d, want %d", id, card.HP, hp)
	}
}

// TestSetIDNamesASetByCodeOrID: a decklist code, an id and an id in
// another case all name the same set; a name matching nothing is refused,
// after one refresh of the set list.
func TestSetIDNamesASetByCodeOrID(t *testing.T) {
	f := newFakeTCGdex(t)
	serveClearSets(f)
	c, _ := newTestClient(t, f)
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	for _, name := range []string{"MEG", "meg", "me01", "ME01", " me01 "} {
		id, setName, err := c.SetID(ctx, name)
		if err != nil {
			t.Errorf("SetID(%q): %v", name, err)
			continue
		}
		if id != "me01" || setName != "Mega Evolution" {
			t.Errorf("SetID(%q) = %q, %q; want me01, Mega Evolution", name, id, setName)
		}
	}
	if n := f.hits("/sets"); n != 1 {
		t.Fatalf("set list requests after the build = %d, want 1", n)
	}

	for range 2 {
		if _, _, err := c.SetID(ctx, "XYZ"); !errors.Is(err, ErrUnknownSet) {
			t.Errorf("SetID(XYZ) = %v, want ErrUnknownSet", err)
		}
	}
	if n := f.hits("/sets"); n != 2 {
		t.Errorf("set list requests after two unknown names = %d, want 2 (the build and one refresh)", n)
	}

	before := f.totalHits()
	for _, name := range []string{"", "  "} {
		if _, _, err := c.SetID(ctx, name); !errors.Is(err, ErrUnknownSet) {
			t.Errorf("SetID(%q) = %v, want ErrUnknownSet", name, err)
		}
	}
	if n := f.totalHits() - before; n != 0 {
		t.Errorf("empty names made %d requests, want 0", n)
	}
}

// TestClearEverythingRefetchesCardsAndSets: after a clear of everything
// through a tiered store, a card lookup asks the card source once and gets
// its current data, deck codes resolve through freshly fetched set data,
// and neither tier holds any entry.
func TestClearEverythingRefetchesCardsAndSets(t *testing.T) {
	f := newFakeTCGdex(t)
	serveClearSets(f)
	srv := spacestest.New(t)
	c, _ := newTieredClient(t, f, srv)
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	mustCard(t, c, "me01-001", 70)
	mustCard(t, c, "sv1-001", 50)
	if id, err := c.Resolve(ctx, "MEG", "1"); err != nil || id != "me01-001" {
		t.Fatalf("Resolve(MEG, 1) = %q, %v", id, err)
	}
	if got := len(srv.Keys("tcgdex/")); got != 5 {
		t.Fatalf("shared objects before the clear = %d, want 5", got)
	}
	f.put("/cards/me01-001", cCardME01b)
	cardHits, listHits, setHits := f.hits("/cards/me01-001"), f.hits("/sets"), f.hits("/sets/me01")

	rs := c.Clear(ctx, Scope{})
	for tier, want := range map[string]int{"shared": 5, "disk": 5, "memory": 2} {
		if r := report(t, rs, tier); r.Removed != want || r.Err != nil {
			t.Errorf("%s report = %+v, want %d removed", tier, r, want)
		}
	}
	if got := srv.Keys("tcgdex/"); len(got) != 0 {
		t.Errorf("shared objects after the clear = %v, want none", got)
	}
	for _, p := range []string{"cards", "sets", "sets.json"} {
		if _, err := os.Stat(filepath.Join(c.CacheDir, p)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still in the cache directory (%v)", p, err)
		}
	}

	mustCard(t, c, "me01-001", 80)
	if n := f.hits("/cards/me01-001") - cardHits; n != 1 {
		t.Errorf("card-source requests for the cleared card = %d, want 1", n)
	}
	if id, err := c.Resolve(ctx, "MEG", "1"); err != nil || id != "me01-001" {
		t.Fatalf("Resolve(MEG, 1) after the clear = %q, %v", id, err)
	}
	if n := f.hits("/sets") - listHits; n != 1 {
		t.Errorf("set list requests after the clear = %d, want 1", n)
	}
	if n := f.hits("/sets/me01") - setHits; n != 1 {
		t.Errorf("me01 requests after the clear = %d, want 1", n)
	}
}

// TestClearOneSetLeavesOtherSets: a clear of one set refetches its cards
// and leaves another set's cached cards alone.
func TestClearOneSetLeavesOtherSets(t *testing.T) {
	f := newFakeTCGdex(t)
	serveClearSets(f)
	c, _ := newTestClient(t, f)
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	mustCard(t, c, "me01-001", 70)
	mustCard(t, c, "sv1-001", 50)
	f.put("/cards/me01-001", cCardME01b)

	rs := c.Clear(ctx, Scope{Set: "me01"})
	if r := report(t, rs, "memory"); r.Removed != 1 || r.Err != nil {
		t.Errorf("memory report = %+v, want 1 removed", r)
	}
	if r := report(t, rs, "disk"); r.Removed != 1 || r.Err != nil {
		t.Errorf("disk report = %+v, want 1 removed", r)
	}

	mustCard(t, c, "me01-001", 80)
	if n := f.hits("/cards/me01-001"); n != 2 {
		t.Errorf("requests for me01-001 = %d, want 2 (before and after the clear)", n)
	}
	mustCard(t, c, "sv1-001", 50)
	if n := f.hits("/cards/sv1-001"); n != 1 {
		t.Errorf("requests for sv1-001 = %d, want 1 (before the clear only)", n)
	}
}

// TestCachedServesClearedCopyThenFreshOne: after a clear, the cached-only
// lookup open games make returns the old copy at once, and the fresh copy
// once the background warm it starts has finished.
func TestCachedServesClearedCopyThenFreshOne(t *testing.T) {
	f := newFakeTCGdex(t)
	serveClearSets(f)
	c, _ := newTestClient(t, f)
	c.Timeout = 10 * time.Second
	t.Cleanup(c.waitBackground)

	mustCard(t, c, "me01-001", 70)
	f.put("/cards/me01-001", cCardME01b)
	c.Clear(context.Background(), Scope{})
	f.setDelay(time.Second) // the warm is slow; Cached must not wait for it

	within(t, 500*time.Millisecond, "Cached of a cleared card", func() {
		if cc := c.Cached("me01-001"); cc == nil || cc.HP != 70 {
			t.Errorf("Cached after the clear = %+v, want the old copy (HP 70)", cc)
		}
	})
	c.waitBackground()
	f.setDelay(0)
	if n := f.hits("/cards/me01-001"); n != 2 {
		t.Errorf("requests for me01-001 = %d, want 2 (the first lookup and the warm)", n)
	}
	if cc := c.Cached("me01-001"); cc == nil || cc.HP != 80 {
		t.Errorf("Cached after the warm = %+v, want the fresh copy (HP 80)", cc)
	}
	mustCard(t, c, "me01-001", 80)
	if n := f.hits("/cards/me01-001"); n != 2 {
		t.Errorf("Card after the warm made %d more requests, want 0", n-2)
	}
}

// TestClearedCardSurvivesSourceOutage: with the card source down after a
// clear, or the client offline, Card returns the old copy without error.
func TestClearedCardSurvivesSourceOutage(t *testing.T) {
	for _, tc := range []struct {
		name string
		down func(*fakeTCGdex, *Client)
	}{
		{"unreachable", func(f *fakeTCGdex, _ *Client) { f.close() }},
		{"failing", func(f *fakeTCGdex, _ *Client) { f.setFail(true) }},
		{"offline", func(_ *fakeTCGdex, c *Client) { c.Offline = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeTCGdex(t)
			serveClearSets(f)
			c, _ := newTestClient(t, f)
			t.Cleanup(c.waitBackground)

			mustCard(t, c, "me01-001", 70)
			c.Clear(context.Background(), Scope{})
			tc.down(f, c)
			mustCard(t, c, "me01-001", 70)
		})
	}
}

// TestCatalogBuildBegunBeforeClearIsNotKept: a catalog build stuck on set
// data when a clear runs finishes for its caller, but is not published,
// and the next lookup builds afresh.
func TestCatalogBuildBegunBeforeClearIsNotKept(t *testing.T) {
	f := newFakeTCGdex(t)
	serveClearSets(f)
	c, _ := newTestClient(t, f)
	c.Timeout = 10 * time.Second // held set requests must not time out
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	f.hold()
	done := make(chan error, 1)
	go func() {
		_, err := c.Resolve(ctx, "MEG", "1")
		done <- err
	}()
	waitFor(t, 5*time.Second, "a set request", func() bool {
		return f.hits("/sets/me01")+f.hits("/sets/sv1") > 0
	})
	c.Clear(ctx, Scope{})
	f.release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Resolve begun before the clear: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Resolve did not finish after the set requests were released")
	}

	c.mu.Lock()
	cat := c.cat
	c.mu.Unlock()
	if cat != nil {
		t.Errorf("the build begun before the clear was published")
	}
	listHits := f.hits("/sets")
	if id, err := c.Resolve(ctx, "MEG", "1"); err != nil || id != "me01-001" {
		t.Fatalf("Resolve after the clear = %q, %v", id, err)
	}
	if n := f.hits("/sets") - listHits; n != 1 {
		t.Errorf("set list requests by the next build = %d, want 1", n)
	}
}

// TestCardFetchBegunBeforeClearIsNotKept: a card fetch in flight when a
// clear runs answers its caller but leaves nothing in memory.
func TestCardFetchBegunBeforeClearIsNotKept(t *testing.T) {
	f := newFakeTCGdex(t)
	serveClearSets(f)
	c, _ := newTestClient(t, f)
	c.Timeout = 10 * time.Second
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	f.setDelay(time.Second)
	done := make(chan error, 1)
	go func() {
		_, err := c.Card(ctx, "me01-001")
		done <- err
	}()
	waitFor(t, 5*time.Second, "the card request", func() bool {
		return f.hits("/cards/me01-001") > 0
	})
	rs := c.Clear(ctx, Scope{})
	if r := report(t, rs, "memory"); r.Removed != 0 {
		t.Errorf("memory report = %+v, want 0 removed", r)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Card begun before the clear: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Card did not finish")
	}
	f.setDelay(0)

	c.mu.Lock()
	_, ok := c.mem["me01-001"]
	c.mu.Unlock()
	if ok {
		t.Error("a card fetched before the clear was kept in memory")
	}
}

// TestClearForgetsRecentFailuresInScope: a card that recently failed, and
// a number a set recently lacked, are asked for again at once after a
// clear of their set.
func TestClearForgetsRecentFailuresInScope(t *testing.T) {
	f := newFakeTCGdex(t)
	serveClearSets(f)
	c, _ := newTestClient(t, f)
	t.Cleanup(c.waitBackground)
	ctx := context.Background()

	for range 2 {
		if _, err := c.Card(ctx, "me01-009"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Card(me01-009) = %v, want not found", err)
		}
	}
	if n := f.hits("/cards/me01-009"); n != 1 {
		t.Fatalf("requests for the failed card = %d, want 1 (the failure is remembered)", n)
	}
	for range 2 {
		if _, err := c.Resolve(ctx, "MEG", "9"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Resolve(MEG, 9) = %v, want not found", err)
		}
	}
	setHits := f.hits("/sets/me01")

	f.put("/cards/me01-009", cCardME09)
	f.put("/sets/me01", cSetME01b)
	c.Clear(ctx, Scope{Set: "me01"})

	card, err := c.Card(ctx, "me01-009")
	if err != nil || card.Name != "Latebloom" {
		t.Fatalf("Card(me01-009) after the clear = %+v, %v", card, err)
	}
	if n := f.hits("/cards/me01-009"); n != 2 {
		t.Errorf("requests for me01-009 = %d, want 2", n)
	}
	if id, err := c.Resolve(ctx, "MEG", "9"); err != nil || id != "me01-009" {
		t.Errorf("Resolve(MEG, 9) after the clear = %q, %v", id, err)
	}
	if n := f.hits("/sets/me01") - setHits; n != 1 {
		t.Errorf("me01 requests after the clear = %d, want 1", n)
	}
}
