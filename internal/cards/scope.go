package cards

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
)

// Scope is what an image clear covers: everything when Set is empty,
// otherwise the one set whose id is Set. TCGdex image paths are
// "<lang>/<serie>/<set id>/<local id>" (en/me/me01/001), so a set's images
// are those whose third segment is the set id.
type Scope struct{ Set string }

// All reports whether the scope is everything.
func (s Scope) All() bool { return s.Set == "" }

// String names the scope for logs and pages.
func (s Scope) String() string {
	if s.All() {
		return "everything"
	}
	return "set " + s.Set
}

// Image reports whether an image path below the image source
// (<lang>/<serie>/<set>/<number>) is in scope.
func (s Scope) Image(rel string) bool {
	if s.All() {
		return true
	}
	segs := strings.Split(rel, "/")
	return len(segs) >= 3 && segs[2] == s.Set
}

// TierReport is what clearing one tier removed, and why it could not
// finish when it could not. Tier is "images" or "cdn".
type TierReport struct {
	Tier    string
	Removed int
	Err     error
}

func (r TierReport) String() string {
	if r.Err != nil {
		return fmt.Sprintf("%s %d removed, error: %v", r.Tier, r.Removed, r.Err)
	}
	return fmt.Sprintf("%s %d removed", r.Tier, r.Removed)
}

const (
	// SharedTimeout bounds each image request to the shared store.
	SharedTimeout = 3 * time.Second
	// SharedRetryAfter is how long the shared store is skipped after a
	// request to it fails.
	SharedRetryAfter = time.Minute
	// RetryAfter is how long an image the card source failed to serve is
	// not fetched again.
	RetryAfter = 5 * time.Minute
)

// clearTimeout bounds each shared store request a clear makes (a listing
// page, or a delete of up to 1,000 keys). Clears are admin actions, so
// they wait longer than lookups do (SharedTimeout).
const clearTimeout = 30 * time.Second

// listAll lists every key under prefix, each page bounded by clearTimeout.
func listAll(ctx context.Context, c *spaces.Client, prefix string) ([]string, error) {
	var keys []string
	token := ""
	for {
		lctx, cancel := context.WithTimeout(ctx, clearTimeout)
		page, err := c.List(lctx, prefix, token)
		cancel()
		if err != nil {
			return keys, err
		}
		keys = append(keys, page.Keys...)
		if page.Next == "" {
			return keys, nil
		}
		token = page.Next
	}
}
