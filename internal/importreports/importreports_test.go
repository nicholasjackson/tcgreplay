package importreports

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
)

// memStore is an in-memory Store that records whether each object was
// written public.
type memStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	public  map[string]bool
}

func newMem() *memStore {
	return &memStore{objects: map[string][]byte{}, public: map[string]bool{}}
}

func (m *memStore) Put(_ context.Context, key string, body []byte, o spaces.PutOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = body
	m.public[key] = o.Public
	return nil
}

func (m *memStore) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objects[key]
	if !ok {
		return nil, spaces.ErrNotFound
	}
	return b, nil
}

func (m *memStore) List(_ context.Context, prefix, _ string) (spaces.ListPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keys []string
	for k := range m.objects {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return spaces.ListPage{Keys: keys}, nil
}

func (m *memStore) Delete(_ context.Context, keys []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.objects, k)
	}
	return nil
}

const failedLog = `Setup
NicJackson chose tails for the opening coin flip.
R_B777 won the coin toss.
R_B777 decided to go first.
NicJackson drew 7 cards for the opening hand.
- 7 drawn cards.
   • Riolu, Lunatone, Solrock, Ultra Ball, Iono, Basic Fighting Energy, Basic Fighting Energy
R_B777 drew 7 cards for the opening hand.
- 7 drawn cards.

Turn # 1 - R_B777's Turn
R_B777 drew a card.
NicJackson played a card.
`

func TestSaveHidesNamesAndStoresPrivately(t *testing.T) {
	store := newMem()
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	s := &Service{Store: store, Now: func() time.Time { return now }}
	r, err := s.Save(context.Background(), Report{
		Player: "p_0123456789abcdef", Assistant: "Claude",
		Error: `no a card in NicJackson's hand`, Log: failedLog, Deck: "4 Riolu",
	}, "NicJackson")
	if err != nil {
		t.Fatal(err)
	}
	if !idRe.MatchString(r.ID) || !strings.HasPrefix(r.ID, "ir-20261125-") {
		t.Fatalf("id %q", r.ID)
	}
	k := Prefix + "2026-11-25/" + r.ID + ".json"
	body, ok := store.objects[k]
	if !ok {
		t.Fatalf("no object at %s; have %v", k, store.objects)
	}
	if store.public[k] {
		t.Fatal("report written public")
	}
	for _, name := range []string{"NicJackson", "R_B777"} {
		if strings.Contains(string(body), name) {
			t.Errorf("stored report still names %s:\n%s", name, body)
		}
	}
	var got Report
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Names != 2 || got.Error != "no a card in Player 1's hand" || !strings.Contains(got.Log, "Player 2 won the coin toss.") ||
		!strings.Contains(got.Log, "Turn # 1 - Player 2's Turn") || got.Deck != "4 Riolu" || got.Player != "p_0123456789abcdef" {
		t.Fatalf("stored report: %+v", got)
	}

	// Read back by ID; a malformed or unknown ID is not found.
	b, err := s.Get(context.Background(), r.ID)
	if err != nil || string(b) != string(body) {
		t.Fatalf("get: %v", err)
	}
	for _, id := range []string{"../secret", "ir-20261125-00000000", ""} {
		if _, err := s.Get(context.Background(), id); !errors.Is(err, ErrNotFound) {
			t.Errorf("get %q: %v, want not found", id, err)
		}
	}
	list, err := s.List(context.Background(), 10)
	if err != nil || len(list) != 1 || list[0].ID != r.ID || list[0].Error != "no a card in Player 1's hand" {
		t.Fatalf("list: %+v, %v", list, err)
	}
}

func TestSaveRefusesEmptyLog(t *testing.T) {
	s := &Service{Store: newMem()}
	if _, err := s.Save(context.Background(), Report{Log: "  "}, ""); err == nil {
		t.Fatal("saved a report with no log")
	}
}

func TestHideWholeWordsOnly(t *testing.T) {
	got := Hide("Ann played Annihilape. Ann's turn. Bo, Ann.", []string{"Ann", "Bo"})
	want := "Player 1 played Annihilape. Player 1's turn. Player 2, Player 1."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// A name inside a longer one is not split, and placeholders are not
	// matched by a later name.
	if got := Hide("Sam Smith and Sam", []string{"Sam", "Sam Smith"}); got != "Player 2 and Player 1" {
		t.Fatalf("nested names: %q", got)
	}
	if got := Hide("Player 1 and P", []string{"P"}); got != "Player 1 and Player 1" {
		t.Fatalf("short name: %q", got)
	}
}

func TestSweepDeletesOldReports(t *testing.T) {
	store := newMem()
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	s := &Service{Store: store, Now: func() time.Time { return now }}
	for _, k := range []string{
		Prefix + "2026-11-10/ir-20261110-aaaaaaaa.json", // ended 15 days ago: deleted
		Prefix + "2026-11-11/ir-20261111-bbbbbbbb.json", // ended 13 days 10 hours ago... kept
		Prefix + "2026-11-25/ir-20261125-cccccccc.json",
	} {
		store.objects[k] = []byte("{}")
	}
	n, err := s.Sweep(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("sweep deleted %d, %v; want 1", n, err)
	}
	if _, ok := store.objects[Prefix+"2026-11-10/ir-20261110-aaaaaaaa.json"]; ok {
		t.Fatal("old report kept")
	}
	if len(store.objects) != 2 {
		t.Fatalf("objects left: %v", store.objects)
	}
}
