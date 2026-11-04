package cards

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// The spec's acceptance criteria at the card database. A restart is a
// second client on an empty cache directory against the same shared
// store.

// restartClient is a client on a fresh cache directory, with a tiered
// store over srv whose shared timeout is short and whose log lines go to
// logs. It returns the tiered store too, for tests that tune it.
func restartClient(t *testing.T, f *fakeTCGdex, srv *spacestest.Server, logs *logBuffer) (*Client, *tieredStore, *fakeClock) {
	t.Helper()
	c, clock := newTestClient(t, f)
	ts := NewTieredStore(NewDiskStore(c.CacheDir), NewSpacesStore(srv.Client(t))).(*tieredStore)
	ts.timeout = 200 * time.Millisecond
	if logs != nil {
		ts.logf = logs.logf
	}
	c.Store = ts
	return c, ts, clock
}

// importAnswers is what an import asks the card database: deck codes,
// the cards they name and a card known only by name.
type importAnswers struct {
	IDs    []string
	Cards  []Card
	ByName string
}

func runImport(t *testing.T, c *Client) importAnswers {
	t.Helper()
	ctx := context.Background()
	var a importAnswers
	for _, code := range [][2]string{{"SVI", "1"}, {"SVI", "081"}, {"PAL", "1"}} {
		id, err := c.Resolve(ctx, code[0], code[1])
		if err != nil {
			t.Fatalf("Resolve %s %s: %v", code[0], code[1], err)
		}
		a.IDs = append(a.IDs, id)
	}
	for _, id := range a.IDs {
		card, err := c.Card(ctx, id)
		if err != nil {
			t.Fatalf("Card %s: %v", id, err)
		}
		a.Cards = append(a.Cards, *card)
	}
	id, err := c.ByName(ctx, "Miraidon ex")
	if err != nil {
		t.Fatalf("ByName: %v", err)
	}
	a.ByName = id
	c.waitBackground()
	return a
}

// cardDataPaths are the card-source paths an import of runImport needs.
var cardDataPaths = []string{"/sets", "/sets/sv01", "/sets/sv02", "/cards/sv01-001", "/cards/sv01-081", "/cards/sv02-001"}

func hitsOf(f *fakeTCGdex) map[string]int {
	m := map[string]int{}
	for _, p := range cardDataPaths {
		m[p] = f.hits(p)
	}
	return m
}

// TestWarmRestartMakesNoCardSourceRequests: after a restart with an empty
// local cache, the same import asks the card source nothing and gets the
// same answers.
func TestWarmRestartMakesNoCardSourceRequests(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	srv := spacestest.New(t)

	a, _, _ := restartClient(t, f, srv, nil)
	first := runImport(t, a)
	before := hitsOf(f)
	for _, p := range cardDataPaths {
		if before[p] != 1 {
			t.Errorf("first import fetched %s %d times, want 1", p, before[p])
		}
	}

	b, _, _ := restartClient(t, f, srv, nil)
	second := runImport(t, b)
	after := hitsOf(f)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("card-source requests after the restart: before %v, after %v", before, after)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("answers differ after the restart:\nfirst  %+v\nsecond %+v", first, second)
	}
	for _, key := range []string{"sets", "sets/sv01", "sets/sv02", "cards/sv01-001", "cards/sv01-081", "cards/sv02-001"} {
		if _, err := NewDiskStore(b.CacheDir).Get(context.Background(), key); err != nil {
			t.Errorf("restarted client's cache directory lacks %s: %v", key, err)
		}
	}
}

// TestRestartFillsLocalAndSharesNewCards: with an empty local cache, a
// card only in the shared store comes without the card source and is then
// local; a card in neither is fetched once and is then in both.
func TestRestartFillsLocalAndSharesNewCards(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	srv := spacestest.New(t)
	fetched := testEpoch.Add(-2 * time.Hour)
	seedSpaces(t, srv, "cards/sv01-001", tCard001, fetched)
	c, _, _ := restartClient(t, f, srv, nil)
	ctx := context.Background()

	if _, err := c.Card(ctx, "sv01-001"); err != nil {
		t.Fatal(err)
	}
	if n := f.hits("/cards/sv01-001"); n != 0 {
		t.Errorf("card-source requests for a shared card = %d, want 0", n)
	}
	requireEnvelope(t, c.CacheDir, "cards/sv01-001", tCard001, fetched)

	if _, err := c.Card(ctx, "sv02-001"); err != nil {
		t.Fatal(err)
	}
	if n := f.hits("/cards/sv02-001"); n != 1 {
		t.Errorf("card-source requests for a new card = %d, want 1", n)
	}
	requireEnvelope(t, c.CacheDir, "cards/sv02-001", tCard201, testEpoch)
	e, err := NewSpacesStore(srv.Client(t)).Get(ctx, "cards/sv02-001")
	if err != nil {
		t.Fatalf("new card not in the shared store: %v", err)
	}
	if string(e.Data) != tCard201 || !e.Fetched.Equal(testEpoch) {
		t.Errorf("shared copy = %.40s @ %v", e.Data, e.Fetched)
	}
}

// TestStaleStaysStaleAfterRestart: a shared entry past its refresh age is
// served at once after a restart, copied to disk with its old fetch time,
// and refreshed from the card source once in the background; the fresh
// copy lands in both tiers.
func TestStaleStaysStaleAfterRestart(t *testing.T) {
	t.Run("card", func(t *testing.T) {
		f := newFakeTCGdex(t)
		srv := spacestest.New(t)
		old := testEpoch.Add(-25 * time.Hour)
		seedSpaces(t, srv, "cards/sv01-081", tCard081, old) // no image
		withImage := strings.Replace(tCard081, `"hp":220`, `"hp":220,"image":"https://assets.tcgdex.net/en/sv/sv01/081"`, 1)
		f.put("/cards/sv01-081", withImage)
		c, _, _ := restartClient(t, f, srv, nil)
		ctx := context.Background()

		card, err := c.Card(ctx, "sv01-081")
		if err != nil {
			t.Fatal(err)
		}
		if card.Image != "" {
			t.Errorf("first answer has image %q, want the stale imageless copy", card.Image)
		}
		c.waitBackground()
		if n := f.hits("/cards/sv01-081"); n != 1 {
			t.Errorf("card-source refreshes = %d, want 1", n)
		}
		requireEnvelope(t, c.CacheDir, "cards/sv01-081", withImage, testEpoch)
		e, err := NewSpacesStore(srv.Client(t)).Get(ctx, "cards/sv01-081")
		if err != nil || string(e.Data) != withImage || !e.Fetched.Equal(testEpoch) {
			t.Errorf("shared copy after refresh = %.40s @ %v (%v)", e.Data, e.Fetched, err)
		}
		if got, _ := c.Card(ctx, "sv01-081"); got == nil || got.Image == "" {
			t.Errorf("card after refresh = %+v, want the image", got)
		}
	})
	t.Run("set", func(t *testing.T) {
		f := newFakeTCGdex(t)
		serveAll(f)
		srv := spacestest.New(t)
		seedSpaces(t, srv, "sets", tSetList, testEpoch)
		seedSpaces(t, srv, "sets/sv01", tSetSV01, testEpoch.Add(-91*24*time.Hour))
		seedSpaces(t, srv, "sets/sv02", tSetSV02, testEpoch)
		c, _, _ := restartClient(t, f, srv, nil)
		ctx := context.Background()

		if id, err := c.Resolve(ctx, "SVI", "1"); err != nil || id != "sv01-001" {
			t.Fatalf("Resolve = %q, %v", id, err)
		}
		c.waitBackground()
		if n := f.hits("/sets/sv01"); n != 1 {
			t.Errorf("refreshes of the stale set = %d, want 1", n)
		}
		if n := f.hits("/sets") + f.hits("/sets/sv02"); n != 0 {
			t.Errorf("fresh entries were fetched %d times, want 0", n)
		}
		requireEnvelope(t, c.CacheDir, "sets/sv01", tSetSV01, testEpoch)
		if e, err := NewSpacesStore(srv.Client(t)).Get(ctx, "sets/sv01"); err != nil || !e.Fetched.Equal(testEpoch) {
			t.Errorf("shared copy of the set after refresh: %v @ %v", err, e.Fetched)
		}
	})
}

// TestSharedOutageImportsFromCardSource: with the shared store
// unreachable, refusing its credentials, failing or hanging, an import
// succeeds from the card source with no error and the failure is logged.
func TestSharedOutageImportsFromCardSource(t *testing.T) {
	want := func(t *testing.T) importAnswers {
		f := newFakeTCGdex(t)
		serveAll(f)
		c, _ := newTestClient(t, f)
		return runImport(t, c)
	}(t)
	for name, mode := range outageModes {
		t.Run(name, func(t *testing.T) {
			f := newFakeTCGdex(t)
			serveAll(f)
			srv := spacestest.New(t)
			logs := &logBuffer{}
			c, _, _ := restartClient(t, f, srv, logs)
			if heal := mode(srv); heal != nil {
				defer heal()
			}
			got := runImport(t, c)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("answers during the outage:\n%+v\nwant\n%+v", got, want)
			}
			if n := logs.count("shared store unavailable"); n != 1 {
				t.Errorf("outage logged %d times, want 1: %q", n, logs.lines)
			}
		})
	}
}

// TestOfflineRestartSkipsSharedStore: offline, with the shared store
// attached and the entries on disk, an import works and the shared store
// receives no request.
func TestOfflineRestartSkipsSharedStore(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	srv := spacestest.New(t)
	online, _, _ := restartClient(t, f, srv, nil)
	runImport(t, online)

	srv.Reset()
	c, _ := newTestClientDir(t, online.CacheDir, f)
	c.Store = NewTieredStore(NewDiskStore(c.CacheDir), NewSpacesStore(srv.Client(t)))
	c.Offline = true
	runImport(t, c)
	if n := srv.Total(); n != 0 {
		t.Errorf("shared-store requests offline = %d, want 0", n)
	}
}
