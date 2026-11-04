package cards

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Scope is what a cache clear covers: everything when Set is empty,
// otherwise the one set whose TCGdex id is Set.
//
// TCGdex card ids are "<set id>-<local id>" (me01-001, sv03.5-012), so a
// set's data is the key "sets/<set id>" plus every "cards/<set id>-…" key;
// the "-" keeps sv03 from matching sv03.5. TCGdex image paths are
// "<lang>/<serie>/<set id>/<local id>" (en/me/me01/001), so a set's images
// are those whose third segment is the set id. Every layer that clears
// asks the scope, so the rule is the same in memory, on disk and in the
// shared store.
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

// Key reports whether a store key ("sets", "sets/<id>" or "cards/<id>") is
// in scope.
func (s Scope) Key(key string) bool {
	if s.All() {
		return true
	}
	return key == "sets/"+s.Set || strings.HasPrefix(key, "cards/"+s.Set+"-")
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

// TierReport is what clearing one cache tier removed, and why it could not
// finish when it could not. Tier is "memory", "disk", "shared", "images"
// or "cdn".
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

// clearer is a store that can forget the entries in a scope. It is
// optional, found by type assertion like Local, so Store does not change.
type clearer interface {
	Clear(ctx context.Context, s Scope) []TierReport
}

// clearTimeout bounds each shared store request a clear makes (a listing
// page, or a delete of up to 1,000 keys). Clears are admin actions, so
// they wait longer than lookups do (SharedTimeout).
const clearTimeout = 30 * time.Second
