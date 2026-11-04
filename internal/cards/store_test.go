package cards

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDiskStoreRoundTrip puts an entry and gets back the same body and
// exactly the same fetch time, down to the nanosecond.
func TestDiskStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewDiskStore(dir)
	ctx := context.Background()
	fetched := time.Date(2026, 11, 4, 10, 0, 0, 123456789, time.FixedZone("CET", 3600))
	for _, tc := range []struct{ key, body string }{
		{"sets", `[{"id":"sv01"},{"id":"sv02"}]`},
		{"sets/sv01", `{"id":"sv01","cards":[{"id":"sv01-001","localId":"001","name":"Sprigatito"}]}`},
		{"cards/sv01-001", `{"id":"sv01-001",  "name":"Sprigatito", "image":"https://x/y"}`},
	} {
		if err := s.Put(ctx, tc.key, Entry{Data: []byte(tc.body), Fetched: fetched}); err != nil {
			t.Fatalf("put %s: %v", tc.key, err)
		}
		e, err := s.Get(ctx, tc.key)
		if err != nil {
			t.Fatalf("get %s: %v", tc.key, err)
		}
		if string(e.Data) != tc.body {
			t.Errorf("%s body %q, want %q", tc.key, e.Data, tc.body)
		}
		if !e.Fetched.Equal(fetched) {
			t.Errorf("%s fetched %v, want %v", tc.key, e.Fetched, fetched)
		}
	}
	// A second put replaces the first.
	later := fetched.Add(time.Hour)
	if err := s.Put(ctx, "sets", Entry{Data: []byte(`[]`), Fetched: later}); err != nil {
		t.Fatal(err)
	}
	e, err := s.Get(ctx, "sets")
	if err != nil {
		t.Fatal(err)
	}
	if string(e.Data) != `[]` || !e.Fetched.Equal(later) {
		t.Errorf("replaced entry %q at %v", e.Data, e.Fetched)
	}
}

// TestDiskStoreLegacy reads files an earlier release wrote (bare TCGdex
// bodies) with their modification time as the fetch time.
func TestDiskStoreLegacy(t *testing.T) {
	dir := t.TempDir()
	mtime := time.Date(2025, 3, 14, 15, 9, 26, 0, time.UTC)
	files := map[string]string{
		"sets.json":           `[{"id":"sv01"}]`,
		"sets/sv01.json":      `{"id":"sv01","name":"Scarlet & Violet","cards":[]}`,
		"cards/sv01-001.json": `{"id":"sv01-001","name":"Sprigatito"}`,
		// An object holding only one of the envelope's keys is still legacy.
		"cards/odd-1.json": `{"id":"odd-1","data":"x"}`,
	}
	for path, body := range files {
		p := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	s := NewDiskStore(dir)
	for key, path := range map[string]string{
		"sets":           "sets.json",
		"sets/sv01":      "sets/sv01.json",
		"cards/sv01-001": "cards/sv01-001.json",
		"cards/odd-1":    "cards/odd-1.json",
	} {
		e, err := s.Get(context.Background(), key)
		if err != nil {
			t.Fatalf("get %s: %v", key, err)
		}
		if string(e.Data) != files[path] {
			t.Errorf("%s body %q, want %q", key, e.Data, files[path])
		}
		if !e.Fetched.Equal(mtime) {
			t.Errorf("%s fetched %v, want %v", key, e.Fetched, mtime)
		}
	}
}

// TestDiskStoreNotCached reports a missing key as ErrNotCached, and other
// failures as something else.
func TestDiskStoreNotCached(t *testing.T) {
	dir := t.TempDir()
	s := NewDiskStore(dir)
	for _, key := range []string{"sets", "sets/nope", "cards/nope-1"} {
		_, err := s.Get(context.Background(), key)
		if !errors.Is(err, ErrNotCached) {
			t.Errorf("%s: %v, want ErrNotCached", key, err)
		}
	}
	// A directory where the file should be is an error, but not "not cached".
	if err := os.MkdirAll(filepath.Join(dir, "cards", "dir-1.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := s.Get(context.Background(), "cards/dir-1")
	if err == nil || errors.Is(err, ErrNotCached) {
		t.Errorf("unreadable entry: %v, want an error other than ErrNotCached", err)
	}
}

// TestDiskStorePaths writes entries where earlier releases kept their
// files, keeps hostile ids inside the directory, and leaves no temporary
// files behind.
func TestDiskStorePaths(t *testing.T) {
	dir := t.TempDir()
	s := NewDiskStore(dir)
	ctx := context.Background()
	e := Entry{Data: []byte(`{}`), Fetched: time.Now()}
	for key, path := range map[string]string{
		"sets":             "sets.json",
		"sets/sv03.5":      "sets/sv03.5.json",
		"cards/sv03.5-151": "cards/sv03.5-151.json",
		"cards/../escape":  "cards/__escape.json",
		"cards/a/b":        "cards/a_b.json",
		`cards/c\d`:        "cards/c_d.json",
	} {
		if err := s.Put(ctx, key, e); err != nil {
			t.Fatalf("put %s: %v", key, err)
		}
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Errorf("%s: not at %s: %v", key, path, err)
		}
	}
	err := filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(p, ".tmp") {
			t.Errorf("temporary file left behind: %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.json")); err == nil {
		t.Error("an id escaped the cache directory")
	}
}

// TestDiskStoreRejectsNonJSON refuses a body that is not JSON, so a broken
// response never lands in the cache.
func TestDiskStoreRejectsNonJSON(t *testing.T) {
	dir := t.TempDir()
	s := NewDiskStore(dir)
	if err := s.Put(context.Background(), "cards/x", Entry{Data: []byte("<html>"), Fetched: time.Now()}); err == nil {
		t.Fatal("put of a non-JSON body succeeded")
	}
	if _, err := s.Get(context.Background(), "cards/x"); !errors.Is(err, ErrNotCached) {
		t.Errorf("after a refused put: %v, want ErrNotCached", err)
	}
}
