package cards

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestOneRequestForSameCard: simultaneous lookups of one uncached card
// make exactly one request to the card source, and all get the card.
func TestOneRequestForSameCard(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	f.setDelay(100 * time.Millisecond)
	c, _ := newTestClient(t, f)
	var wg sync.WaitGroup
	errs := make([]error, 10)
	names := make([]string, 10)
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			card, err := c.Card(context.Background(), "sv01-081")
			errs[i] = err
			if card != nil {
				names[i] = card.Name
			}
		}()
	}
	wg.Wait()
	for i := range 10 {
		if errs[i] != nil || names[i] != "Miraidon ex" {
			t.Errorf("lookup %d: %q, %v", i, names[i], errs[i])
		}
	}
	if n := f.hits("/cards/sv01-081"); n != 1 {
		t.Errorf("%d requests for one card, want 1", n)
	}
}

// TestWaiterGivesUp: a lookup whose own context ends returns early, and
// does not cancel the shared request for the others.
func TestWaiterGivesUp(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	f.setDelay(150 * time.Millisecond)
	c, _ := newTestClient(t, f)

	quick, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var wg sync.WaitGroup
	var quickErr, slowErr error
	var slowCard *Card
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, quickErr = c.Card(quick, "sv01-081")
	}()
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond) // join the request already in flight
		slowCard, slowErr = c.Card(context.Background(), "sv01-081")
	}()
	wg.Wait()
	if !errors.Is(quickErr, context.DeadlineExceeded) {
		t.Errorf("impatient lookup: %v, want its own deadline", quickErr)
	}
	if slowErr != nil || slowCard == nil || slowCard.Name != "Miraidon ex" {
		t.Errorf("patient lookup: %+v, %v", slowCard, slowErr)
	}
	if n := f.hits("/cards/sv01-081"); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
	// The impatient caller giving up is not a failure of the card.
	if c.recentlyFailed("sv01-081") {
		t.Error("card remembered as failed")
	}
}

// TestFailedCardRemembered: a card the source reports unknown, cannot
// return because it is down, or answers too slowly for, is requested at
// most once in 5 minutes; lookups in between return at once with no card.
// After 5 minutes the source is asked again.
func TestFailedCardRemembered(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(f *fakeTCGdex)
	}{
		{"unknown", func(f *fakeTCGdex) {}},
		{"erroring", func(f *fakeTCGdex) { f.put("/cards/x-1", `{"id":"x-1"}`); f.setFail(true) }},
		{"slow", func(f *fakeTCGdex) { f.put("/cards/x-1", `{"id":"x-1"}`); f.setDelay(time.Second) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeTCGdex(t)
			tc.setup(f)
			c, clock := newTestClient(t, f)
			ctx := context.Background()
			if _, err := c.Card(ctx, "x-1"); err == nil {
				t.Fatal("first lookup found the card")
			}
			if n := f.hits("/cards/x-1"); n != 1 {
				t.Fatalf("%d requests after the first lookup, want 1", n)
			}
			clock.Advance(4 * time.Minute)
			for range 5 {
				start := time.Now()
				card, err := c.Card(ctx, "x-1")
				if card != nil || !errors.Is(err, ErrNotFound) {
					t.Errorf("repeat lookup: %+v, %v", card, err)
				}
				if d := time.Since(start); d > time.Second {
					t.Errorf("repeat lookup took %v", d)
				}
			}
			if n := f.hits("/cards/x-1"); n != 1 {
				t.Errorf("%d requests within 5 minutes, want 1", n)
			}
			clock.Advance(2 * time.Minute) // 6 minutes after the failure
			c.Card(ctx, "x-1")
			if n := f.hits("/cards/x-1"); n != 2 {
				t.Errorf("%d requests after 5 minutes, want 2", n)
			}
		})
	}
}

// TestFailedCardDown: with the source unreachable, a card is attempted
// once (no request lands), and lookups in between return at once.
func TestFailedCardDown(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	f.close()
	c, clock := newTestClient(t, f)
	ctx := context.Background()
	if _, err := c.Card(ctx, "sv01-081"); err == nil {
		t.Fatal("card found with the source down")
	}
	if !c.recentlyFailed("sv01-081") {
		t.Fatal("unreachable card not remembered as failed")
	}
	start := time.Now()
	if _, err := c.Card(ctx, "sv01-081"); !errors.Is(err, ErrNotFound) {
		t.Errorf("repeat lookup: %v", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("repeat lookup took %v", d)
	}
	clock.Advance(6 * time.Minute)
	if c.recentlyFailed("sv01-081") {
		t.Error("still remembered as failed after 5 minutes")
	}
}

// TestFailureClearedOnSuccess: once a failed card comes back, it is no
// longer remembered as failed.
func TestFailureClearedOnSuccess(t *testing.T) {
	f := newFakeTCGdex(t)
	c, clock := newTestClient(t, f)
	ctx := context.Background()
	if _, err := c.Card(ctx, "sv01-081"); err == nil {
		t.Fatal("unknown card found")
	}
	f.put("/cards/sv01-081", tCard081)
	clock.Advance(6 * time.Minute)
	if _, err := c.Card(ctx, "sv01-081"); err != nil {
		t.Fatal(err)
	}
	if c.recentlyFailed("sv01-081") {
		t.Error("card still remembered as failed")
	}
}

// TestOfflineNoRequests: offline, an uncached card is not found and the
// card source is never contacted, whether or not the card recently failed.
func TestOfflineNoRequests(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	c, _ := newTestClient(t, f)
	c.Offline = true
	ctx := context.Background()
	for range 3 {
		if _, err := c.Card(ctx, "sv01-081"); !errors.Is(err, ErrNotFound) {
			t.Errorf("offline lookup: %v", err)
		}
	}
	if _, err := c.Resolve(ctx, "SVI", "81"); err == nil {
		t.Error("offline resolve with nothing cached succeeded")
	}
	if _, err := c.ByName(ctx, "Pineco"); err == nil {
		t.Error("offline by-name with nothing cached succeeded")
	}
	if n := f.totalHits(); n != 0 {
		t.Errorf("offline client made %d requests", n)
	}
}

// TestSetFetchShared: simultaneous loads of one set make one request.
func TestSetFetchShared(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	f.setDelay(100 * time.Millisecond)
	c, _ := newTestClient(t, f)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.set(context.Background(), "sv01"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := f.hits("/sets/sv01"); n != 1 {
		t.Errorf("%d requests for one set, want 1", n)
	}
}
