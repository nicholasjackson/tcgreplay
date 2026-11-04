package cards

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry is one card source response as a cache tier holds it.
type Entry struct {
	// Data is the body exactly as the card source sent it.
	Data []byte
	// Fetched is when the card source returned it. A tier that copies an
	// entry from another keeps Fetched as it is, so its age survives the
	// copy.
	Fetched time.Time
}

// ErrNotCached means a store holds no entry for a key.
var ErrNotCached = errors.New("not cached")

// Store keeps card source responses by key. A key names one response:
// "sets" (the set list), "sets/<set id>" or "cards/<card id>", with raw
// ids; each store makes them safe for its own medium. A store knows
// nothing about expiry: freshness is decided by the Client above it.
type Store interface {
	// Get returns the entry for key, or ErrNotCached when there is none.
	Get(ctx context.Context, key string) (Entry, error)
	// Put stores e under key, replacing any entry already there.
	Put(ctx context.Context, key string, e Entry) error
}

// NewDiskStore returns a store keeping entries as files under dir, at the
// paths earlier releases used: sets.json, sets/<id>.json and
// cards/<id>.json.
func NewDiskStore(dir string) Store { return diskStore{dir: dir} }

type diskStore struct{ dir string }

// relPath maps a key to its slash-separated path relative to a store's
// root: "sets" is sets.json and "<kind>/<id>" is <kind>/<safe id>.json.
// The disk store and the Spaces store share it, so both tiers name an
// entry alike.
func relPath(key string) string {
	kind, id, ok := strings.Cut(key, "/")
	if !ok {
		return safeName(key) + ".json"
	}
	return safeName(kind) + "/" + safeName(id) + ".json"
}

// path maps a key to its file under the store's directory.
func (s diskStore) path(key string) string {
	return filepath.Join(s.dir, filepath.FromSlash(relPath(key)))
}

// envelope is the format of entries written by this release, on disk and
// in the shared store alike.
type envelope struct {
	Fetched *time.Time      `json:"fetched"`
	Data    json.RawMessage `json:"data"`
}

// encodeEnvelope encodes e as an envelope. It is written by hand rather
// than marshalled, so the body is kept byte for byte.
func encodeEnvelope(key string, e Entry) ([]byte, error) {
	if !json.Valid(e.Data) {
		return nil, fmt.Errorf("%s: entry body is not JSON", key)
	}
	ts, err := e.Fetched.UTC().MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	var buf bytes.Buffer
	buf.WriteString(`{"fetched":`)
	buf.Write(ts)
	buf.WriteString(`,"data":`)
	buf.Write(e.Data)
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}

// decodeEnvelope decodes an envelope, reporting false when b is not one
// (both fields must be present).
func decodeEnvelope(b []byte) (Entry, bool) {
	if t := bytes.TrimLeft(b, " \t\r\n"); len(t) == 0 || t[0] != '{' {
		return Entry{}, false
	}
	var env envelope
	if json.Unmarshal(b, &env) != nil || env.Fetched == nil || env.Data == nil {
		return Entry{}, false
	}
	return Entry{Data: env.Data, Fetched: *env.Fetched}, true
}

// Get implements Store. A file without the envelope was written by an
// earlier release: its whole content is the body and its modification
// time stands in for the fetch time.
func (s diskStore) Get(_ context.Context, key string) (Entry, error) {
	p := s.path(key)
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return Entry{}, fmt.Errorf("%s: %w", key, ErrNotCached)
	}
	if err != nil {
		return Entry{}, err
	}
	if e, ok := decodeEnvelope(b); ok {
		return e, nil
	}
	fi, err := os.Stat(p)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Data: b, Fetched: fi.ModTime()}, nil
}

// Put implements Store. The file is written to a temporary name and
// renamed into place, so a crash never leaves a half-written entry.
func (s diskStore) Put(_ context.Context, key string, e Entry) error {
	b, err := encodeEnvelope(key, e)
	if err != nil {
		return err
	}

	p := s.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), filepath.Base(p)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(b)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmp, 0o644)
	}
	if werr == nil {
		werr = os.Rename(tmp, p)
	}
	if werr != nil {
		os.Remove(tmp)
	}
	return werr
}
