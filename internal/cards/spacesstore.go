package cards

import (
	"context"
	"errors"
	"fmt"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
)

// spacesPrefix is where card data lives in the bucket. It is reserved for
// card data: other uses of the bucket keep to prefixes of their own.
const spacesPrefix = "tcgdex/"

// NewSpacesStore returns a store keeping entries as objects in a Spaces
// bucket, under tcgdex/ at the disk store's relative paths
// (tcgdex/sets.json, tcgdex/sets/<id>.json, tcgdex/cards/<id>.json), in
// the same envelope the disk store writes. Objects are private. It holds
// no timeout or retry logic: the tiered store bounds its calls.
func NewSpacesStore(c *spaces.Client) Store { return spacesStore{c: c} }

type spacesStore struct{ c *spaces.Client }

// objectKey is the object an entry is kept in.
func objectKey(key string) string { return spacesPrefix + relPath(key) }

// Get implements Store. A missing object, or one that is not an envelope,
// is ErrNotCached, so it is fetched again and overwritten. Any other
// failure is returned as it is.
func (s spacesStore) Get(ctx context.Context, key string) (Entry, error) {
	b, err := s.c.Get(ctx, objectKey(key))
	if errors.Is(err, spaces.ErrNotFound) {
		return Entry{}, fmt.Errorf("%s: %w", key, ErrNotCached)
	}
	if err != nil {
		return Entry{}, fmt.Errorf("%s: %w", key, err)
	}
	e, ok := decodeEnvelope(b)
	if !ok {
		return Entry{}, fmt.Errorf("%s: shared copy is not an envelope: %w", key, ErrNotCached)
	}
	return e, nil
}

// Put implements Store.
func (s spacesStore) Put(ctx context.Context, key string, e Entry) error {
	b, err := encodeEnvelope(key, e)
	if err != nil {
		return err
	}
	if err := s.c.Put(ctx, objectKey(key), b, spaces.PutOptions{ContentType: "application/json"}); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return nil
}
