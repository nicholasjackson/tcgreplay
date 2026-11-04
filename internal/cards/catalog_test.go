package cards

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// within runs fn and fails the test if it has not returned after d.
func within(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s did not return within %v", what, d)
	}
}

// waitFor polls cond until it holds, failing the test after d.
func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestCatalogBuildDoesNotBlockLookups starts a cold catalog build that is
// stuck fetching set data, and looks up cards already cached meanwhile:
// both Card and Cached answer at once, and the build finishes once the
// card source answers.
func TestCatalogBuildDoesNotBlockLookups(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	dir := t.TempDir()
	seedEnvelope(t, dir, "cards/sv01-081", tCard081, testEpoch)
	seedEnvelope(t, dir, "cards/sv02-001", tCard201, testEpoch)
	c, _ := newTestClientDir(t, dir, f)
	c.Timeout = 10 * time.Second // held set requests must not time out
	ctx := context.Background()

	f.hold()
	type result struct {
		id  string
		err error
	}
	resolved := make(chan result, 1)
	go func() {
		id, err := c.Resolve(ctx, "SVI", "81")
		resolved <- result{id, err}
	}()
	// The build is under way once it is waiting on a set.
	waitFor(t, 5*time.Second, "a set request", func() bool {
		return f.hits("/sets/sv01")+f.hits("/sets/sv02") > 0
	})

	within(t, time.Second, "Card during a cold build", func() {
		card, err := c.Card(ctx, "sv01-081")
		if err != nil {
			t.Errorf("Card: %v", err)
			return
		}
		if card.Name != "Miraidon ex" {
			t.Errorf("Card named %s, want Miraidon ex", card.Name)
		}
	})
	within(t, time.Second, "Cached during a cold build", func() {
		if cc := c.Cached("sv02-001"); cc == nil || cc.Name != "Pineco" {
			t.Errorf("Cached(sv02-001) = %+v", cc)
		}
	})
	select {
	case r := <-resolved:
		t.Fatalf("Resolve returned while the set requests were held: %+v", r)
	default:
	}

	f.release()
	select {
	case r := <-resolved:
		if r.err != nil {
			t.Fatalf("Resolve: %v", r.err)
		}
		if r.id != "sv01-081" {
			t.Errorf("SVI 81 resolved to %s, want sv01-081", r.id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Resolve did not finish after the set requests were released")
	}
}

// TestCatalogBuiltOnce resolves deck codes from several imports at once
// on empty caches: the set list and each set are fetched only once.
func TestCatalogBuiltOnce(t *testing.T) {
	f := newFakeTCGdex(t)
	serveAll(f)
	c, _ := newTestClient(t, f)
	c.Timeout = 10 * time.Second
	ctx := context.Background()

	cases := []struct{ code, num, want string }{
		{"SVI", "81", "sv01-081"},
		{"PAL", "1", "sv02-001"},
		{"svi", "001", "sv01-001"},
		{"SVI", "1", "sv01-001"},
		{"pal", "001", "sv02-001"},
		{"SVI", "081", "sv01-081"},
	}
	f.hold()
	var started, wg sync.WaitGroup
	errs := make([]error, len(cases))
	got := make([]string, len(cases))
	for i, tc := range cases {
		started.Add(1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			started.Done()
			got[i], errs[i] = c.Resolve(ctx, tc.code, tc.num)
		}()
	}
	started.Wait()
	waitFor(t, 5*time.Second, "a set request", func() bool {
		return f.hits("/sets/sv01")+f.hits("/sets/sv02") > 0
	})
	time.Sleep(50 * time.Millisecond) // let every import reach the build
	f.release()
	wg.Wait()

	for i, tc := range cases {
		if errs[i] != nil {
			t.Errorf("%s %s: %v", tc.code, tc.num, errs[i])
			continue
		}
		if got[i] != tc.want {
			t.Errorf("%s %s resolved to %s, want %s", tc.code, tc.num, got[i], tc.want)
		}
	}
	for _, p := range []string{"/sets", "/sets/sv01", "/sets/sv02"} {
		if n := f.hits(p); n != 1 {
			t.Errorf("%s requested %d times, want 1", p, n)
		}
	}
}

// TestCatalogSharedCodeKeepsMainSet resolves a code two sets share (the
// 30th Celebration and its Classic Collection are both 30C, and here both
// list the same TCG Online code) to the main set, whose id is shorter,
// whichever order the set list gives them in.
func TestCatalogSharedCodeKeepsMainSet(t *testing.T) {
	const (
		set30th  = `{"id":"30th","releaseDate":"2026-09-01","serie":{"id":"me"},"abbreviation":{"official":"30C"},"tcgOnline":"TC30","cards":[{"id":"30th-001","localId":"001","name":"Pikachu","image":"a"}]}`
		set30thC = `{"id":"30th-c","releaseDate":"2026-09-01","serie":{"id":"me"},"abbreviation":{"official":"30C"},"tcgOnline":"TC30","cards":[{"id":"30th-c-001","localId":"001","name":"Charizard","image":"b"}]}`
	)
	for _, list := range []string{
		`[{"id":"30th"},{"id":"30th-c"}]`,
		`[{"id":"30th-c"},{"id":"30th"}]`,
	} {
		f := newFakeTCGdex(t)
		f.put("/sets", list)
		f.put("/sets/30th", set30th)
		f.put("/sets/30th-c", set30thC)
		c, _ := newTestClient(t, f)
		for _, code := range []string{"30C", "30c", "TC30"} {
			id, err := c.Resolve(context.Background(), code, "1")
			if err != nil {
				t.Fatalf("list %s, %s 1: %v", list, code, err)
			}
			if id != "30th-001" {
				t.Errorf("list %s: %s 1 resolved to %s, want 30th-001", list, code, id)
			}
		}
	}
}

// TestCatalogPrintPreference picks prints by name as before: prints with
// art first, then the newest release, then the lowest number (lettered
// prints after numbered ones), and Pokémon TCG Pocket prints never.
func TestCatalogPrintPreference(t *testing.T) {
	f := newFakeTCGdex(t)
	f.put("/sets", `[{"id":"sva"},{"id":"svb"},{"id":"tcgp1"}]`)
	f.put("/sets/sva", `{"id":"sva","releaseDate":"2023-01-01","serie":{"id":"sv"},"abbreviation":{"official":"SVA"},"cards":[
		{"id":"sva-010","localId":"010","name":"Pikachu","image":"a"}]}`)
	f.put("/sets/svb", `{"id":"svb","releaseDate":"2024-01-01","serie":{"id":"sv"},"abbreviation":{"official":"SVB"},"cards":[
		{"id":"svb-050","localId":"050","name":"Pikachu"},
		{"id":"svb-TG05","localId":"TG05","name":"Pikachu","image":"b"},
		{"id":"svb-020","localId":"020","name":"Pikachu","image":"b"},
		{"id":"svb-005","localId":"005","name":"Pikachu","image":"b"},
		{"id":"svb-030","localId":"030","name":"Fire Energy","image":"b"}]}`)
	f.put("/sets/tcgp1", `{"id":"tcgp1","releaseDate":"2025-01-01","serie":{"id":"tcgp"},"abbreviation":{"official":"TP1"},"cards":[
		{"id":"tcgp1-001","localId":"001","name":"Pikachu","image":"c"}]}`)
	c, _ := newTestClient(t, f)
	ctx := context.Background()

	ids, err := c.prints(ctx, "pikachu")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"svb-005", "svb-020", "svb-TG05", "sva-010", "svb-050"}
	if !slices.Equal(ids, want) {
		t.Errorf("Pikachu prints %v, want %v", ids, want)
	}
	for name, want := range map[string]string{
		"Pikachu":           "svb-005",
		"Basic Fire Energy": "svb-030",
	} {
		id, err := c.ByName(ctx, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if id != want {
			t.Errorf("%s resolved to %s, want %s", name, id, want)
		}
	}
	// The Pocket set still resolves by code: only by-name skips it.
	id, err := c.Resolve(ctx, "TP1", "1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "tcgp1-001" {
		t.Errorf("TP1 1 resolved to %s, want tcgp1-001", id)
	}
}

// TestCatalogNoSetIndexFile builds the catalog without writing the
// retired set index file, and ignores an old one left on disk with wrong
// mappings, online and offline: codes resolve by the cached sets.
func TestCatalogNoSetIndexFile(t *testing.T) {
	ctx := context.Background()
	f := newFakeTCGdex(t)
	serveAll(f)

	c, _ := newTestClient(t, f)
	if _, err := c.Resolve(ctx, "SVI", "81"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ByName(ctx, "Pineco"); err != nil {
		t.Fatal(err)
	}
	filepath.WalkDir(c.CacheDir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasPrefix(d.Name(), "set-index") {
			t.Errorf("set index file written: %s", p)
		}
		return nil
	})

	const stale = `{"SVI":"sv02","PAL":"sv01","XYZ":"sv01"}`
	for _, offline := range []bool{false, true} {
		dir := t.TempDir()
		indexFile := filepath.Join(dir, "set-index-v2.json")
		if err := os.WriteFile(indexFile, []byte(stale), 0o644); err != nil {
			t.Fatal(err)
		}
		if offline {
			seedEnvelope(t, dir, "sets", tSetList, testEpoch)
			seedEnvelope(t, dir, "sets/sv01", tSetSV01, testEpoch)
			seedEnvelope(t, dir, "sets/sv02", tSetSV02, testEpoch)
		}
		c, _ := newTestClientDir(t, dir, f)
		c.Offline = offline
		for _, tc := range []struct{ code, num, want string }{
			{"SVI", "1", "sv01-001"},
			{"PAL", "1", "sv02-001"},
		} {
			id, err := c.Resolve(ctx, tc.code, tc.num)
			if err != nil {
				t.Fatalf("offline=%v %s %s: %v", offline, tc.code, tc.num, err)
			}
			if id != tc.want {
				t.Errorf("offline=%v: %s %s resolved to %s, want %s", offline, tc.code, tc.num, id, tc.want)
			}
		}
		// A code only the old file knows is not resolved through it.
		if id, err := c.Resolve(ctx, "XYZ", "1"); err == nil {
			t.Errorf("offline=%v: XYZ 1 resolved to %s through the old index file", offline, id)
		}
		b, err := os.ReadFile(indexFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != stale {
			t.Errorf("offline=%v: old index file rewritten: %s", offline, b)
		}
	}
}
