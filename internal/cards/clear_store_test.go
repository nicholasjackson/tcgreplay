package cards

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// clearKeys are the entries the store clear tests seed.
var clearKeys = []string{
	"sets", "sets/me01", "sets/me01.5", "sets/sv1",
	"cards/me01-001", "cards/me01-002", "cards/me01.5-001", "cards/sv1-001",
}

func seedStore(t *testing.T, s Store, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if err := s.Put(context.Background(), k, Entry{Data: []byte(`{"id":"x"}`), Fetched: testEpoch}); err != nil {
			t.Fatal(err)
		}
	}
}

// cachedKeys returns the keys of clearKeys s still holds.
func cachedKeys(t *testing.T, s Store) []string {
	t.Helper()
	var have []string
	for _, k := range clearKeys {
		_, err := s.Get(context.Background(), k)
		switch {
		case err == nil:
			have = append(have, k)
		case !errors.Is(err, ErrNotCached):
			t.Fatalf("Get(%s): %v", k, err)
		}
	}
	return have
}

func report(t *testing.T, rs []TierReport, tier string) TierReport {
	t.Helper()
	for _, r := range rs {
		if r.Tier == tier {
			return r
		}
	}
	t.Fatalf("no %q report in %v", tier, rs)
	return TierReport{}
}

func TestDiskStoreClearsOneSet(t *testing.T) {
	dir := t.TempDir()
	s := NewDiskStore(dir)
	seedStore(t, s, clearKeys...)
	rs := s.(clearer).Clear(context.Background(), Scope{Set: "me01"})
	if r := report(t, rs, "disk"); r.Removed != 3 || r.Err != nil {
		t.Errorf("report = %+v, want 3 removed", r)
	}
	want := []string{"sets", "sets/me01.5", "sets/sv1", "cards/me01.5-001", "cards/sv1-001"}
	if got := cachedKeys(t, s); !slices.Equal(got, want) {
		t.Errorf("left = %v, want %v", got, want)
	}
}

func TestDiskStoreClearsEverythingItOwnsAndNothingElse(t *testing.T) {
	dir := t.TempDir()
	s := NewDiskStore(dir)
	seedStore(t, s, clearKeys...)
	for _, f := range []string{"set-index-v2.json", "unrelated.txt", "games/one.json"} {
		p := filepath.Join(dir, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rs := s.(clearer).Clear(context.Background(), Scope{})
	if r := report(t, rs, "disk"); r.Removed != len(clearKeys)+1 || r.Err != nil {
		t.Errorf("report = %+v, want %d removed", r, len(clearKeys)+1)
	}
	if got := cachedKeys(t, s); len(got) != 0 {
		t.Errorf("left = %v, want nothing", got)
	}
	for _, f := range []string{"sets.json", "sets", "cards", "set-index-v2.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still there (%v)", f, err)
		}
	}
	for _, f := range []string{"unrelated.txt", "games/one.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s was removed: %v", f, err)
		}
	}
}

func TestDiskStoreClearOfAnEmptyDirectory(t *testing.T) {
	s := NewDiskStore(filepath.Join(t.TempDir(), "never-made"))
	for _, sc := range []Scope{{}, {Set: "me01"}} {
		if r := report(t, s.(clearer).Clear(context.Background(), sc), "disk"); r.Removed != 0 || r.Err != nil {
			t.Errorf("%v: report = %+v, want nothing removed and no error", sc, r)
		}
	}
}

func TestSpacesStoreClearsOneSetAndEverything(t *testing.T) {
	f := newTiered(t)
	shared := f.store.shared
	seedStore(t, shared, clearKeys...)
	f.srv.SetObject("images/en/me/me01/001/low.webp", []byte("x"))

	rs := shared.(clearer).Clear(context.Background(), Scope{Set: "me01"})
	if r := report(t, rs, "shared"); r.Removed != 3 || r.Err != nil {
		t.Errorf("set report = %+v, want 3 removed", r)
	}
	want := []string{"tcgdex/cards/me01.5-001.json", "tcgdex/cards/sv1-001.json", "tcgdex/sets.json", "tcgdex/sets/me01.5.json", "tcgdex/sets/sv1.json"}
	if got := f.srv.Keys("tcgdex/"); !slices.Equal(got, want) {
		t.Errorf("left = %v, want %v", got, want)
	}

	rs = shared.(clearer).Clear(context.Background(), Scope{})
	if r := report(t, rs, "shared"); r.Removed != 5 || r.Err != nil {
		t.Errorf("all report = %+v, want 5 removed", r)
	}
	if got := f.srv.Keys("tcgdex/"); len(got) != 0 {
		t.Errorf("left = %v, want no tcgdex/ objects", got)
	}
	if got := f.srv.Keys("images/"); len(got) != 1 {
		t.Errorf("images = %v, want the image kept", got)
	}
}

func TestTieredStoreClearsBothTiers(t *testing.T) {
	f := newTiered(t)
	seedStore(t, f.store, clearKeys...)
	rs := f.store.Clear(context.Background(), Scope{Set: "me01"})
	if len(rs) != 2 || rs[0].Tier != "shared" || rs[1].Tier != "disk" {
		t.Fatalf("reports = %v, want shared then disk", rs)
	}
	for _, r := range rs {
		if r.Removed != 3 || r.Err != nil {
			t.Errorf("report = %+v, want 3 removed", r)
		}
	}
	want := []string{"sets", "sets/me01.5", "sets/sv1", "cards/me01.5-001", "cards/sv1-001"}
	if got := cachedKeys(t, f.local); !slices.Equal(got, want) {
		t.Errorf("local left = %v, want %v", got, want)
	}
	if got := cachedKeys(t, f.store.shared); !slices.Equal(got, want) {
		t.Errorf("shared left = %v, want %v", got, want)
	}
	// Nothing comes back down from shared either.
	if got := cachedKeys(t, f.store); !slices.Equal(got, want) {
		t.Errorf("tiered left = %v, want %v", got, want)
	}

	rs = f.store.Clear(context.Background(), Scope{})
	if len(f.srv.Keys("tcgdex/")) != 0 || len(cachedKeys(t, f.store)) != 0 {
		t.Errorf("after clearing everything: shared %v, cached %v", f.srv.Keys("tcgdex/"), cachedKeys(t, f.store))
	}
	if report(t, rs, "shared").Removed != 5 || report(t, rs, "disk").Removed != 5 {
		t.Errorf("reports = %v, want 5 each", rs)
	}
}

func TestTieredClearReportsASharedFailureAndStillClearsLocal(t *testing.T) {
	f := newTiered(t)
	seedStore(t, f.store, clearKeys...)
	f.srv.SetDeny(true)
	rs := f.store.Clear(context.Background(), Scope{})
	if r := report(t, rs, "shared"); r.Err == nil || r.Removed != 0 {
		t.Errorf("shared report = %+v, want an error", r)
	}
	if r := report(t, rs, "disk"); r.Err != nil || r.Removed != len(clearKeys) {
		t.Errorf("disk report = %+v, want everything removed", r)
	}
	if got := cachedKeys(t, f.local); len(got) != 0 {
		t.Errorf("local left = %v", got)
	}
	if f.store.skip() {
		t.Error("a failed clear made lookups skip the shared store")
	}
	f.srv.SetDeny(false)
	if len(f.srv.Keys("tcgdex/")) != len(clearKeys) {
		t.Errorf("shared objects = %v, want all kept", f.srv.Keys("tcgdex/"))
	}

	// Some keys refused one by one: those are reported, the rest go.
	f.srv.DenyDelete("tcgdex/cards/sv1-001.json")
	rs = f.store.Clear(context.Background(), Scope{})
	if r := report(t, rs, "shared"); r.Err == nil || r.Removed != len(clearKeys)-1 {
		t.Errorf("shared report = %+v, want %d removed and an error", r, len(clearKeys)-1)
	}
	if got := f.srv.Keys("tcgdex/"); !slices.Equal(got, []string{"tcgdex/cards/sv1-001.json"}) {
		t.Errorf("left = %v", got)
	}
}

func TestTieredClearReachesSharedWhileLookupsSkipIt(t *testing.T) {
	f := newTiered(t)
	seedStore(t, f.store, clearKeys...)
	// Trip the breaker with a failed lookup.
	f.srv.SetFail(true)
	if _, err := f.store.Get(context.Background(), "cards/missing-1"); !errors.Is(err, ErrNotCached) {
		t.Fatal(err)
	}
	if !f.store.skip() {
		t.Fatal("breaker not tripped")
	}
	f.srv.SetFail(false)
	f.srv.Reset()
	rs := f.store.Clear(context.Background(), Scope{Set: "me01"})
	if r := report(t, rs, "shared"); r.Err != nil || r.Removed != 3 {
		t.Errorf("shared report = %+v, want 3 removed", r)
	}
	if f.srv.Requests("POST", "?delete") != 1 {
		t.Errorf("delete requests = %d, want 1", f.srv.Requests("POST", "?delete"))
	}
	if f.store.skip() {
		t.Error("a successful clear left lookups skipping the shared store")
	}
}

func TestTieredClearUsesItsOwnBoundsNotTheLookupTimeout(t *testing.T) {
	f := newTiered(t)
	seedStore(t, f.store, "cards/me01-001")
	f.store.timeout = time.Millisecond // lookups would give up at once
	rs := f.store.Clear(context.Background(), Scope{})
	if r := report(t, rs, "shared"); r.Err != nil || r.Removed != 1 {
		t.Errorf("shared report = %+v", r)
	}
}

// plainStore is a store that cannot clear.
type plainStore struct{ m map[string]Entry }

func (p plainStore) Get(_ context.Context, key string) (Entry, error) {
	if e, ok := p.m[key]; ok {
		return e, nil
	}
	return Entry{}, ErrNotCached
}

func (p plainStore) Put(_ context.Context, key string, e Entry) error {
	p.m[key] = e
	return nil
}

func TestStoresThatCannotClearAreUnaffected(t *testing.T) {
	local := plainStore{m: map[string]Entry{}}
	f := newTiered(t)
	ts := NewTieredStore(local, f.store.shared).(*tieredStore)
	seedStore(t, ts, "cards/me01-001")
	rs := ts.Clear(context.Background(), Scope{})
	if len(rs) != 1 || rs[0].Tier != "shared" {
		t.Errorf("reports = %v, want the shared tier only", rs)
	}
	if _, ok := local.m["cards/me01-001"]; !ok {
		t.Error("the plain store lost its entry")
	}
	if _, ok := Store(local).(clearer); ok {
		t.Error("plainStore should not be a clearer")
	}
}
