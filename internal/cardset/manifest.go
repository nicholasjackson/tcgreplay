package cardset

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ManifestFormat is the published manifest's own format version. It is
// bumped only on a breaking change, so a playmat can tell a manifest it
// cannot read from one it can.
const ManifestFormat = 1

// ManifestKey is the manifest's name within the published layout.
const ManifestKey = "manifest.json"

// BucketPrefix is where the published layout sits in the bucket.
const BucketPrefix = "cardsets/"

// Cache lifetimes of published objects. A set object is named by its
// contents and never changes once written, so it is cached for a year; the
// manifest names each set's current object, so it is cached briefly (and
// purged from the CDN after each publish).
const (
	SetCacheControl      = "public, max-age=31536000, immutable"
	ManifestCacheControl = "public, max-age=60"
)

// Manifest is the published list of card sets: the contract between the
// publisher and the playmat's loader. It names each set's current object,
// relative to the manifest's own directory, so the same layout works from
// the CDN, the bucket's origin or a local directory.
type Manifest struct {
	Format int                    `json:"format"`
	Sets   map[string]ManifestSet `json:"sets"`
}

// ManifestSet is one published set.
type ManifestSet struct {
	Path   string `json:"path"`   // "sets/me06/3f9a…c2.json.gz", relative to the manifest
	SHA256 string `json:"sha256"` // hex sha256 of the built set file (uncompressed)
	Prints int    `json:"prints"` // number of prints in the set
	Format int    `json:"format"` // the built set's format (Set.Format)
}

// SetKey is the published object of set id whose built file has the hex
// sha256 sum: sets/<id>/<first 16 hex of sum>.json.gz.
func SetKey(id, sum string) string {
	if len(sum) > 16 {
		sum = sum[:16]
	}
	return "sets/" + id + "/" + sum + ".json.gz"
}

// EncodeManifest returns the manifest's deterministic bytes: two-space
// indent, set ids in sorted order, a trailing newline and no timestamps, so
// a publish with nothing changed produces identical bytes.
func EncodeManifest(m Manifest) ([]byte, error) {
	if m.Sets == nil {
		m.Sets = map[string]ManifestSet{}
	}
	return marshal(m)
}

// DecodeManifest parses a manifest, refusing one whose format this program
// does not know and entries without a path or a full sha256.
func DecodeManifest(b []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return Manifest{}, fmt.Errorf("card-set manifest: %w", err)
	}
	if m.Format != ManifestFormat {
		return Manifest{}, fmt.Errorf("card-set manifest format %d is not supported (this program reads format %d)", m.Format, ManifestFormat)
	}
	if m.Sets == nil {
		m.Sets = map[string]ManifestSet{}
	}
	var errs []error
	for id, s := range m.Sets {
		if s.Path == "" || strings.HasPrefix(s.Path, "/") || strings.Contains(s.Path, "..") {
			errs = append(errs, fmt.Errorf("set %s: bad path %q", id, s.Path))
		}
		if b, err := hex.DecodeString(s.SHA256); err != nil || len(b) != 32 {
			errs = append(errs, fmt.Errorf("set %s: bad sha256 %q", id, s.SHA256))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return Manifest{}, fmt.Errorf("card-set manifest: %w", err)
	}
	return m, nil
}
