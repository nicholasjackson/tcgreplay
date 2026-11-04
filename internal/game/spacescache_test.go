package game

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
	"github.com/nicholasjackson/tcgreplay/internal/state"
)

// liveSource is a card source serving fixed bodies by path and counting
// requests per path.
type liveSource struct {
	*httptest.Server
	mu     sync.Mutex
	bodies map[string][]byte
	hits   map[string]int
}

// newLiveSource serves the test pool as seedCardCache lays it out.
func newLiveSource(t *testing.T) *liveSource {
	t.Helper()
	dir := t.TempDir()
	seedCardCache(t, dir, time.Now())
	store := cards.NewDiskStore(dir)
	s := &liveSource{bodies: map[string][]byte{}, hits: map[string]int{}}
	keys := []string{"sets", "sets/tst", "sets/sve", "cards/sve-006"}
	for i := 1; i <= 8; i++ {
		keys = append(keys, "cards/tst-"+string(rune('0'+i)))
	}
	for _, key := range keys {
		e, err := store.Get(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		s.bodies["/"+key] = e.Data
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits[r.URL.Path]++
		body, ok := s.bodies[r.URL.Path]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *liveSource) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.hits {
		n += c
	}
	return n
}

func (s *liveSource) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits = map[string]int{}
}

// TestGameCardsIdenticalAfterRestartFromSharedStore builds a game's cards
// through the real card database, then again after a restart with an
// empty cache directory against the same shared store: the card list and
// warnings are identical and the card source is not asked again. Cache
// contents never change where a card goes.
func TestGameCardsIdenticalAfterRestartFromSharedStore(t *testing.T) {
	src := newLiveSource(t)
	srv := spacestest.New(t)
	sc := srv.Client(t)
	client := func() *cards.Client {
		dir := t.TempDir()
		c := cards.NewClient(dir)
		c.Base = src.URL
		c.Timeout = time.Second
		c.Store = cards.NewTieredStore(cards.NewDiskStore(dir), cards.NewSpacesStore(sc))
		return c
	}
	lists := map[state.Seat]string{state.P1: testDeck, state.P2: testDeck}
	ctx := context.Background()

	first, warnFirst, err := BuildCards(ctx, client(), lists)
	if err != nil {
		t.Fatal(err)
	}
	if src.total() == 0 {
		t.Fatal("first build made no card-source requests: the test is not exercising the cache")
	}
	for _, c := range first {
		if c.Def == "" {
			t.Fatalf("first build left %s unresolved: %v", c.Name, warnFirst)
		}
	}
	src.reset()

	second, warnSecond, err := BuildCards(ctx, client(), lists)
	if err != nil {
		t.Fatal(err)
	}
	if n := src.total(); n != 0 {
		t.Errorf("card-source requests after the restart = %d, want 0 (%v)", n, src.hits)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("cards differ after the restart:\nfirst  %v\nsecond %v", first, second)
	}
	if strings.Join(warnFirst, "\n") != strings.Join(warnSecond, "\n") {
		t.Errorf("warnings differ: %q vs %q", warnFirst, warnSecond)
	}
}
