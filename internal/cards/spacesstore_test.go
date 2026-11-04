package cards

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// TestSpacesStoreRoundTrip puts entries and gets back the same body and
// exactly the same fetch time, down to the nanosecond.
func TestSpacesStoreRoundTrip(t *testing.T) {
	srv := spacestest.New(t)
	s := NewSpacesStore(srv.Client(t))
	ctx := context.Background()
	fetched := time.Date(2026, 11, 4, 10, 0, 0, 123456789, time.FixedZone("CET", 3600))
	for _, tc := range []struct{ key, body string }{
		{"sets", `[{"id":"sv03.5"}]`},
		{"sets/sv03.5", `{"id":"sv03.5","cards":[{"id":"sv03.5-001","localId":"001","name":"Bulbasaur"}]}`},
		{"cards/sv03.5-001", `{"id":"sv03.5-001",  "name":"Bulbasaur"}`},
	} {
		if err := s.Put(ctx, tc.key, Entry{Data: []byte(tc.body), Fetched: fetched}); err != nil {
			t.Fatalf("put %s: %v", tc.key, err)
		}
		e, err := s.Get(ctx, tc.key)
		if err != nil {
			t.Fatalf("get %s: %v", tc.key, err)
		}
		if string(e.Data) != tc.body {
			t.Errorf("%s: body %s, want %s", tc.key, e.Data, tc.body)
		}
		if !e.Fetched.Equal(fetched) {
			t.Errorf("%s: fetched %v, want %v", tc.key, e.Fetched, fetched)
		}
	}
}

// TestSpacesStoreObjectsMatchDiskFiles checks each entry lands at
// tcgdex/<disk path> holding exactly the bytes the disk store writes.
func TestSpacesStoreObjectsMatchDiskFiles(t *testing.T) {
	srv := spacestest.New(t)
	s := NewSpacesStore(srv.Client(t))
	dir := t.TempDir()
	disk := NewDiskStore(dir)
	ctx := context.Background()
	e := Entry{Data: []byte(`{"id":"x"}`), Fetched: time.Date(2026, 11, 4, 9, 30, 0, 5, time.UTC)}
	for key, want := range map[string]struct{ object, file string }{
		"sets":             {"tcgdex/sets.json", "sets.json"},
		"sets/sv03.5":      {"tcgdex/sets/sv03.5.json", "sets/sv03.5.json"},
		"cards/sv03.5-001": {"tcgdex/cards/sv03.5-001.json", "cards/sv03.5-001.json"},
	} {
		if err := s.Put(ctx, key, e); err != nil {
			t.Fatal(err)
		}
		if err := disk.Put(ctx, key, e); err != nil {
			t.Fatal(err)
		}
		obj, ok := srv.Object(want.object)
		if !ok {
			t.Errorf("%s: no object %s", key, want.object)
			continue
		}
		file, err := os.ReadFile(filepath.Join(dir, want.file))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(obj, file) {
			t.Errorf("%s: object %q differs from disk file %q", key, obj, file)
		}
		h := srv.Header(want.object)
		if ct := h.Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: Content-Type %q, want application/json", key, ct)
		}
		if acl := h.Get("X-Amz-Acl"); acl != "" {
			t.Errorf("%s: X-Amz-Acl %q, want private", key, acl)
		}
	}
	const wantBody = `{"fetched":"2026-11-04T09:30:00.000000005Z","data":{"id":"x"}}` + "\n"
	if obj, _ := srv.Object("tcgdex/sets.json"); string(obj) != wantBody {
		t.Errorf("object = %q, want %q", obj, wantBody)
	}
}

func TestSpacesStoreMissingAndMalformedAreNotCached(t *testing.T) {
	srv := spacestest.New(t)
	s := NewSpacesStore(srv.Client(t))
	ctx := context.Background()
	if _, err := s.Get(ctx, "cards/none-1"); !errors.Is(err, ErrNotCached) {
		t.Errorf("missing object: %v, want ErrNotCached", err)
	}
	for name, body := range map[string]string{
		"bare body":     `{"id":"tst-1","name":"Riolu"}`,
		"no data":       `{"fetched":"2026-11-04T09:30:00Z"}`,
		"not json":      `garbage`,
		"bare set list": `[{"id":"tst"}]`,
	} {
		srv.SetObject("tcgdex/cards/tst-1.json", []byte(body))
		if _, err := s.Get(ctx, "cards/tst-1"); !errors.Is(err, ErrNotCached) {
			t.Errorf("%s: %v, want ErrNotCached", name, err)
		}
	}
}

func TestSpacesStoreFailuresAreNotMisses(t *testing.T) {
	srv := spacestest.New(t)
	s := NewSpacesStore(srv.Client(t))
	ctx := context.Background()
	e := Entry{Data: []byte(`{}`), Fetched: testEpoch}

	srv.SetDeny(true)
	if _, err := s.Get(ctx, "sets"); err == nil || errors.Is(err, ErrNotCached) {
		t.Errorf("refused get: %v, want an error that is not ErrNotCached", err)
	}
	if err := s.Put(ctx, "sets", e); err == nil {
		t.Error("refused put succeeded")
	}
	srv.SetDeny(false)

	srv.SetFail(true)
	if _, err := s.Get(ctx, "sets"); err == nil || errors.Is(err, ErrNotCached) {
		t.Errorf("failed get: %v, want an error that is not ErrNotCached", err)
	}
	srv.SetFail(false)

	srv.Close()
	if _, err := s.Get(ctx, "sets"); err == nil || errors.Is(err, ErrNotCached) {
		t.Errorf("unreachable get: %v, want an error that is not ErrNotCached", err)
	}
}

func TestSpacesStoreRefusesNonJSONBody(t *testing.T) {
	srv := spacestest.New(t)
	s := NewSpacesStore(srv.Client(t))
	if err := s.Put(context.Background(), "sets", Entry{Data: []byte("not json"), Fetched: testEpoch}); err == nil {
		t.Error("put of a non-JSON body succeeded")
	}
	if srv.Total() != 0 {
		t.Errorf("requests = %d, want 0", srv.Total())
	}
}
