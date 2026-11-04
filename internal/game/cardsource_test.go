package game

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cards/cardstest"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// deadSource is a card source that never answers usefully: it drops every
// connection without a response, or answers too slowly. It counts the
// requests it gets per path.
type deadSource struct {
	*httptest.Server
	mu   sync.Mutex
	hits map[string]int
}

func newDeadSource(t *testing.T, slow bool) *deadSource {
	t.Helper()
	d := &deadSource{hits: map[string]int{}}
	d.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.hits[r.URL.Path]++
		d.mu.Unlock()
		if slow {
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
			}
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			conn.Close() // unreachable: no response at all
		}
	}))
	t.Cleanup(d.Close)
	return d
}

func (d *deadSource) count(path string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.hits[path]
}

// seedCardCache writes a cache holding the test pool's set data and cards,
// fetched at fetched: set "tst" (code TST) lists tst-1..tst-8 and tst-9
// (Ghostmon), whose card is not cached; set "sve" (code SVE) holds the
// Fighting Energy. Riolu and Lucario are cached with artwork, the rest
// without.
func seedCardCache(t *testing.T, dir string, fetched time.Time) {
	t.Helper()
	ctx := context.Background()
	s := cards.NewDiskStore(dir)
	put := func(key string, body []byte) {
		t.Helper()
		if err := s.Put(ctx, key, cards.Entry{Data: body, Fetched: fetched}); err != nil {
			t.Fatal(err)
		}
	}
	type setCard struct {
		ID      string `json:"id"`
		LocalID string `json:"localId"`
		Name    string `json:"name"`
		Image   string `json:"image,omitempty"`
	}
	type set struct {
		ID           string            `json:"id"`
		ReleaseDate  string            `json:"releaseDate"`
		Serie        map[string]string `json:"serie"`
		Abbreviation map[string]string `json:"abbreviation"`
		Cards        []setCard         `json:"cards"`
	}
	pool := cardstest.New()
	tst := set{ID: "tst", ReleaseDate: "2024-01-01", Serie: map[string]string{"id": "sv"}, Abbreviation: map[string]string{"official": "TST"}}
	for _, id := range []string{"tst-1", "tst-2", "tst-3", "tst-4", "tst-5", "tst-6", "tst-7", "tst-8"} {
		tst.Cards = append(tst.Cards, setCard{ID: id, LocalID: strings.TrimPrefix(id, "tst-"), Name: pool[id].Name})
	}
	tst.Cards = append(tst.Cards, setCard{ID: "tst-9", LocalID: "9", Name: "Ghostmon"})
	sve := set{ID: "sve", ReleaseDate: "2023-03-31", Serie: map[string]string{"id": "sv"}, Abbreviation: map[string]string{"official": "SVE"},
		Cards: []setCard{{ID: "sve-006", LocalID: "006", Name: pool["sve-006"].Name}}}
	put("sets", []byte(`[{"id":"tst"},{"id":"sve"}]`))
	for _, st := range []set{tst, sve} {
		b, err := json.Marshal(st)
		if err != nil {
			t.Fatal(err)
		}
		put("sets/"+st.ID, b)
	}
	for id, c := range pool {
		cc := *c
		if id == "tst-1" || id == "tst-2" {
			cc.Image = "https://assets.tcgdex.net/en/tst/" + id
		}
		b, err := json.Marshal(cc)
		if err != nil {
			t.Fatal(err)
		}
		put("cards/"+id, b)
	}
}

// sourceDownGame builds a game through the manager with a real card
// database over a cache fetched at fetched and a dead card source.
func sourceDownGame(t *testing.T, fetched time.Time, slow bool, deckP1 string) (*Game, []string, *cards.Client, *deadSource) {
	t.Helper()
	dir := t.TempDir()
	seedCardCache(t, dir, fetched)
	src := newDeadSource(t, slow)
	client := cards.NewClient(dir)
	client.Base = src.URL
	client.Timeout = 200 * time.Millisecond
	m := NewManager(t.TempDir(), client)
	m.Rand = func() *rand.Rand { return rand.New(rand.NewPCG(3, 99)) }
	g, res, err := m.New(context.Background(), NewGameRequest{ID: "down", DeckP1: deckP1, DeckP2: testDeck})
	if err != nil {
		t.Fatal(err)
	}
	return g, res.Warnings, client, src
}

// TestPlaysOnWithSourceDown: with every cached entry past its refresh age
// and the card source unreachable (or slower than the timeout), a game
// builds from decklists with set codes, with no error and no warnings
// beyond those a fresh cache gives; card text and art come from the stale
// copies; board reads each return within one second.
func TestPlaysOnWithSourceDown(t *testing.T) {
	for _, tc := range []struct {
		name string
		slow bool
	}{{"unreachable", false}, {"slow", true}} {
		t.Run(tc.name, func(t *testing.T) {
			stale := time.Now().Add(-400 * 24 * time.Hour)
			g, warnings, client, _ := sourceDownGame(t, stale, tc.slow, testDeck)
			_, freshWarnings, _, _ := sourceDownGame(t, time.Now(), tc.slow, testDeck)
			for _, w := range warnings {
				if !strings.Contains(w, " mulliganed ") {
					t.Errorf("unexpected warning with a stale cache: %s", w)
				}
			}
			if !slices.Equal(warnings, freshWarnings) {
				t.Errorf("stale cache warnings %q, fresh cache %q", warnings, freshWarnings)
			}
			// Every card resolved from the stale set data.
			for iid, c := range g.State().Cards {
				if c.Def == "" {
					t.Errorf("%s (%s) unresolved", iid, c.Name)
				}
			}

			ctx := context.Background()
			iono, err := client.Card(ctx, "tst-4")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(iono.Text(), "Each player shuffles their hand") {
				t.Errorf("Iono text %q", iono.Text())
			}
			riolu, err := client.Card(ctx, "tst-1")
			if err != nil || riolu.Image != "https://assets.tcgdex.net/en/tst/tst-1" {
				t.Errorf("Riolu art: %+v, %v", riolu, err)
			}
			for range 5 {
				for _, seat := range []state.Seat{state.P1, state.P2} {
					quick(t, "View "+string(seat), func() {
						if _, err := g.View(seat, "test", "test"); err != nil {
							t.Error(err)
						}
					})
				}
			}
		})
	}
}

// TestUncachedCardWithSourceDown: a card the stale cache lacks shows by
// name only, and repeated board reads do not ask the card source for it
// again within 5 minutes.
func TestUncachedCardWithSourceDown(t *testing.T) {
	deck := strings.Replace(testDeck, "4 Riolu TST 1", "3 Riolu TST 1\n1 Ghostmon TST 9", 1)
	stale := time.Now().Add(-400 * 24 * time.Hour)
	g, warnings, client, src := sourceDownGame(t, stale, false, deck)
	for _, w := range warnings {
		if !strings.Contains(w, " mulliganed ") {
			t.Errorf("unexpected warning: %s", w)
		}
	}
	var ghost state.IID
	for iid, c := range g.State().Cards {
		if c.Def == "tst-9" {
			ghost = iid
			if c.Name != "Ghostmon" {
				t.Errorf("uncached card named %q, want Ghostmon", c.Name)
			}
		}
	}
	if ghost == "" {
		t.Fatal("Ghostmon did not resolve from the stale set data")
	}
	if client.Cached("tst-9") != nil {
		t.Fatal("Ghostmon is cached")
	}
	before := src.count("/cards/tst-9")
	if before > 1 {
		t.Errorf("%d requests for Ghostmon while building the game, want at most 1", before)
	}
	for range 10 {
		quick(t, "View", func() { g.View(state.P1, "test", "test") })
		quick(t, "SearchDeck", func() { g.SearchDeck(state.P1, "", "test", "test") })
	}
	// The card is remembered as failed: a direct lookup answers at once.
	_, err := client.Card(context.Background(), "tst-9")
	if !errors.Is(err, cards.ErrNotFound) || !strings.Contains(err.Error(), "recently failed") {
		t.Errorf("Ghostmon lookup: %v, want a remembered failure", err)
	}
	if n := src.count("/cards/tst-9"); n != before {
		t.Errorf("%d requests for Ghostmon after repeated reads, want %d", n, before)
	}
}
