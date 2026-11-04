package cards

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
)

// ErrUnknownSet means a name given to SetID names no set.
var ErrUnknownSet = fmt.Errorf("no such set: %w", ErrNotFound)

// SetID names a set: by its TCGdex id in any case (me01, ME01), or by a
// set code as decklists print it (MEG), through the same aliases and code
// index Resolve uses. It returns the set's id and display name. A name
// matching no set is refused with ErrUnknownSet, after refreshing the set
// list once per RetryAfter per name, so a set released since the list was
// fetched is found. It may reach the card source, so it is never for use
// under a game lock.
func (c *Client) SetID(ctx context.Context, name string) (id, setName string, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", ErrUnknownSet
	}
	cat, err := c.ensureCatalog(ctx)
	if err != nil {
		return "", "", err
	}
	code := normCode(name)
	find := func(cat *catalog) (string, bool) {
		for _, id := range cat.setIDs {
			if strings.EqualFold(id, name) {
				return id, true
			}
		}
		if id, ok := setAliases[code]; ok {
			return id, true
		}
		id, ok := cat.setIndex[code]
		return id, ok
	}
	id, ok := find(cat)
	if !ok && c.missAllowed("code:"+code) {
		// The cached set list may predate a new set.
		if fresh, err := c.refreshSetList(ctx, cat); err == nil && fresh != nil {
			id, ok = find(fresh)
		}
	}
	if !ok {
		return "", "", fmt.Errorf("%q: %w", name, ErrUnknownSet)
	}
	setName = id
	if e, ok := cat.setEntry(id); ok && e.d.Name != "" {
		setName = e.d.Name
	} else if d, err := c.set(ctx, id); err == nil && d.Name != "" {
		setName = d.Name
	}
	return id, setName, nil
}

// Clear forgets the cached card data in s across every tier the client
// holds, and reports what each tier removed.
//
// The store is cleared first, through its optional clearer (a tiered
// store clears its shared tier, then its local one). Then, under the lock
// and with no I/O, memory is swept: card entries in scope are marked
// cleared rather than dropped, so Cached keeps answering open games with
// the old copy while Card fetches fresh data; the catalog is dropped, so
// the next deck or name lookup rebuilds it from the store and the card
// source; the refresh tracker forgets the cleared keys' throttles, so the
// next lookup asks the card source at once; and the clear generation moves
// on, so a fetch or catalog build begun before the clear is not kept.
func (c *Client) Clear(ctx context.Context, s Scope) []TierReport {
	var reports []TierReport
	if cl, ok := c.store().(clearer); ok {
		reports = append(reports, cl.Clear(ctx, s)...)
	} else {
		reports = append(reports, TierReport{Tier: "disk", Err: errors.New("store cannot be cleared")})
	}

	c.mu.Lock()
	n := 0
	for id, e := range c.mem {
		if s.Key("cards/"+id) && !e.cleared {
			e.cleared = true
			c.mem[id] = e
			n++
		}
	}
	c.cat = nil
	if s.All() {
		c.tracker.failed, c.tracker.bg = nil, nil
	} else {
		for id := range c.tracker.failed {
			if s.Key("cards/" + id) {
				delete(c.tracker.failed, id)
			}
		}
		for key := range c.tracker.bg {
			if s.Key(key) {
				delete(c.tracker.bg, key)
			}
		}
	}
	// Miss throttles are keyed by set code and number, not by store key;
	// they are cheap to rebuild, so every clear forgets them all.
	c.tracker.miss = nil
	c.gen++
	c.mu.Unlock()

	reports = append(reports, TierReport{Tier: "memory", Removed: n})
	log.Printf("card cache: cleared %s: %s", s, reportText(reports))
	return reports
}

// reportText joins tier reports for a log line.
func reportText(rs []TierReport) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = r.String()
	}
	return strings.Join(parts, "; ")
}
