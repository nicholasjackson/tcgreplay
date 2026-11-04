package cards

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// catalog is an immutable snapshot of the card source's set data and the
// indexes derived from it. It is built outside Client.mu and swapped in
// whole; nothing in it is changed after it is published.
type catalog struct {
	listFetched time.Time
	setIDs      []string            // the set list, in the card source's order
	sets        map[string]setEntry // set id -> its card list; sets that failed to load are absent
	setIndex    map[string]string   // upper set code -> set id
	byName      map[string][]string // nameKey -> card ids, preferred print first
	staleAt     time.Time           // when the set list or the oldest set passes its refresh age
}

// setEntry is one set's card list with the time it was fetched.
type setEntry struct {
	d       *setDetail
	raw     []byte
	fetched time.Time
}

// setEntry returns a set's card list, if the catalog holds it. A nil
// catalog holds nothing.
func (cat *catalog) setEntry(id string) (setEntry, bool) {
	if cat == nil {
		return setEntry{}, false
	}
	e, ok := cat.sets[id]
	return e, ok
}

// ensureCatalog returns the current catalog, building it on first use.
// Simultaneous first uses share one build, and the build never holds
// c.mu, so card lookups carry on while it fetches.
func (c *Client) ensureCatalog(ctx context.Context) (*catalog, error) {
	c.mu.Lock()
	cat := c.cat
	c.mu.Unlock()
	if cat != nil {
		c.maybeSweep(cat)
		return cat, nil
	}
	// Each request the build makes is bounded on its own, so the build as a
	// whole is not.
	v, err := c.flight.do(ctx, "catalog", 0, func(fctx context.Context) (any, error) {
		c.mu.Lock()
		cat := c.cat
		c.mu.Unlock()
		if cat != nil {
			return cat, nil
		}
		cat, err := c.buildCatalog(fctx)
		if err != nil {
			return nil, err
		}
		c.publish(cat)
		return cat, nil
	})
	if err != nil {
		return nil, err
	}
	cat = v.(*catalog)
	c.maybeSweep(cat)
	return cat, nil
}

// publish swaps cat in as the current catalog.
func (c *Client) publish(cat *catalog) {
	c.mu.Lock()
	c.cat = cat
	c.mu.Unlock()
}

// buildCatalog loads the set list and every set, from the store or else
// the card source, and derives the indexes.
func (c *Client) buildCatalog(ctx context.Context) (*catalog, error) {
	var list []setSummary
	le, err := c.loadEntry(ctx, "sets", &list)
	if err != nil {
		if c.Offline {
			return nil, fmt.Errorf("set list not cached and offline: %w", ErrNotFound)
		}
		return nil, err
	}
	ids := setIDs(list)
	sets, err := c.loadSets(ctx, ids, nil, false)
	if len(sets) == 0 && err != nil {
		return nil, err
	}
	return newCatalog(le.Fetched, ids, sets), nil
}

func setIDs(list []setSummary) []string {
	ids := make([]string, 0, len(list))
	for _, s := range list {
		ids = append(ids, s.ID)
	}
	return ids
}

// loadSets loads the sets in ids that have is missing, eight at a time,
// and returns them together with those in have that ids still lists. With
// force they are fetched from the card source whatever the store holds.
// A set that cannot be loaded is left out; the first such error that is
// not ErrNotFound is returned alongside.
func (c *Client) loadSets(ctx context.Context, ids []string, have map[string]setEntry, force bool) (map[string]setEntry, error) {
	out := make(map[string]setEntry, len(ids))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	var firstErr error
	for _, id := range ids {
		if e, ok := have[id]; ok {
			out[id] = e
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var d setDetail
			load := c.loadEntry
			if force {
				load = c.fetchEntry
			}
			e, err := load(ctx, "sets/"+id, &d)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil && !errors.Is(err, ErrNotFound) {
					firstErr = err
				}
				return // offline or gone: its cards are simply not offered
			}
			out[id] = setEntry{d: &d, raw: e.Data, fetched: e.Fetched}
		}(id)
	}
	wg.Wait()
	return out, firstErr
}

// newCatalog assembles a catalog and computes its indexes.
func newCatalog(listFetched time.Time, ids []string, sets map[string]setEntry) *catalog {
	setIndex, byName := indexes(sets)
	return withStaleAt(&catalog{listFetched: listFetched, setIDs: ids, sets: sets, setIndex: setIndex, byName: byName})
}

// withStaleAt sets when cat first holds something past its refresh age.
func withStaleAt(cat *catalog) *catalog {
	cat.staleAt = cat.listFetched.Add(SetListMaxAge)
	for _, e := range cat.sets {
		if t := e.fetched.Add(SetMaxAge); t.Before(cat.staleAt) {
			cat.staleAt = t
		}
	}
	return cat
}

// indexes derives the set-code index and the by-name index from the sets.
// It is pure, so the same sets always give the same indexes.
func indexes(sets map[string]setEntry) (map[string]string, map[string][]string) {
	setIndex := map[string]string{}
	type pick struct {
		id, release string
		art         bool
		num         int
	}
	better := func(a, b pick) bool { // is a preferred to b?
		if a.art != b.art {
			return a.art
		}
		if a.release != b.release {
			return a.release > b.release
		}
		return a.num < b.num
	}
	all := map[string][]pick{}
	for _, e := range sets {
		d := e.d
		for _, code := range []string{d.Abbreviation.Official, d.TCGOnline} {
			if code == "" {
				continue
			}
			// Sets sharing a code (30th and 30th-c, cel25 and cel25cc) keep
			// the main set, whose id is shorter, so the pick does not
			// depend on load order.
			k := strings.ToUpper(code)
			if old, ok := setIndex[k]; !ok || len(d.ID) < len(old) || (len(d.ID) == len(old) && d.ID < old) {
				setIndex[k] = d.ID
			}
		}
		if d.Serie.ID == "tcgp" {
			continue // Pokémon TCG Pocket, a different game
		}
		for _, card := range d.Cards {
			num, err := strconv.Atoi(card.LocalID)
			if err != nil {
				num = 1 << 30 // lettered prints (TG05) after numbered ones
			}
			k := nameKey(card.Name)
			all[k] = append(all[k], pick{card.ID, d.ReleaseDate, card.Image != "", num})
		}
	}
	byName := make(map[string][]string, len(all))
	for k, ps := range all {
		slices.SortFunc(ps, func(a, b pick) int {
			switch {
			case better(a, b):
				return -1
			case better(b, a):
				return 1
			}
			return strings.Compare(a.id, b.id)
		})
		ids := make([]string, len(ps))
		for i, p := range ps {
			ids[i] = p.id
		}
		byName[k] = ids
	}
	return setIndex, byName
}

// listUpdate is a freshly fetched set list.
type listUpdate struct {
	ids     []string
	fetched time.Time
}

// update merges freshly fetched set data into the current catalog and
// publishes the result (nil, publishing nothing, before the first build).
// An entry older than the one the catalog holds is
// ignored, so updates racing each other keep the newest data. The indexes
// are recomputed only when a body or the list of sets changed. Updates
// are serialised by c.catMu, which is held for this CPU work only, never
// across I/O.
func (c *Client) update(list *listUpdate, fresh map[string]setEntry) *catalog {
	c.catMu.Lock()
	defer c.catMu.Unlock()
	c.mu.Lock()
	cur := c.cat
	c.mu.Unlock()
	if cur == nil {
		return nil // nothing built yet: the first build loads fresh data
	}
	ids, listFetched := cur.setIDs, cur.listFetched
	changed := false
	if list != nil && list.fetched.After(listFetched) {
		changed = !slices.Equal(ids, list.ids)
		ids, listFetched = list.ids, list.fetched
	}
	listed := make(map[string]bool, len(ids))
	sets := make(map[string]setEntry, len(ids))
	for _, id := range ids {
		listed[id] = true
		if e, ok := cur.sets[id]; ok {
			sets[id] = e
		}
	}
	if len(sets) != len(cur.sets) {
		changed = true // a set left the list
	}
	for id, e := range fresh {
		if !listed[id] {
			continue
		}
		old, ok := sets[id]
		if ok && !e.fetched.After(old.fetched) {
			continue
		}
		if !ok || !bytes.Equal(old.raw, e.raw) {
			changed = true
		}
		sets[id] = e
	}
	var cat *catalog
	if changed {
		cat = newCatalog(listFetched, ids, sets)
	} else {
		cat = withStaleAt(&catalog{listFetched: listFetched, setIDs: ids, sets: sets, setIndex: cur.setIndex, byName: cur.byName})
	}
	c.publish(cat)
	return cat
}

// refreshSetList fetches the set list afresh, loads any sets it newly
// lists, and publishes the updated catalog. Misses use it to check an
// unknown set code against fresh data.
func (c *Client) refreshSetList(ctx context.Context, old *catalog) (*catalog, error) {
	var list []setSummary
	le, err := c.fetchEntry(ctx, "sets", &list)
	if err != nil {
		return old, err
	}
	c.bgAllowed("sets", c.now()) // the list is fresh: a sweep need not refetch it
	ids := setIDs(list)
	var newIDs []string
	for _, id := range ids {
		if _, ok := old.setEntry(id); !ok {
			newIDs = append(newIDs, id)
		}
	}
	fresh, _ := c.loadSets(ctx, newIDs, nil, false)
	return c.update(&listUpdate{ids, le.Fetched}, fresh), nil
}

// refreshSet fetches one set's card list afresh and publishes the updated
// catalog. Misses use it to check a number absent from a set against
// fresh data. It returns the fetched card list, which a set the set list
// does not name (an alias) needs, since the catalog leaves it out.
func (c *Client) refreshSet(ctx context.Context, id string) (*catalog, *setDetail, error) {
	var d setDetail
	e, err := c.fetchEntry(ctx, "sets/"+id, &d)
	if err != nil {
		return nil, nil, err
	}
	cat := c.update(nil, map[string]setEntry{id: {d: &d, raw: e.Data, fetched: e.Fetched}})
	return cat, &d, nil
}

// maybeSweep starts one background sweep when cat holds set data past its
// refresh age, unless the client is offline, a sweep is running, or one
// started less than RetryAfter ago.
func (c *Client) maybeSweep(cat *catalog) {
	if c.Offline {
		return
	}
	now := c.now()
	if !now.After(cat.staleAt) {
		return
	}
	c.mu.Lock()
	if c.sweeping || recent(c.tracker.bg, "sweep", now) {
		c.mu.Unlock()
		return
	}
	c.sweeping = true
	mark(&c.tracker.bg, "sweep", now)
	c.mu.Unlock()
	c.bgwg.Add(1)
	go func() {
		defer c.bgwg.Done()
		defer func() {
			c.mu.Lock()
			c.sweeping = false
			c.mu.Unlock()
		}()
		c.sweep(cat)
	}()
}

// sweep refreshes the stale set data of cat: the set list if it is past
// SetListMaxAge, then the sets it newly lists, then every set past
// SetMaxAge, each key at most once per RetryAfter. A refresh that fails
// keeps the stale copy.
func (c *Client) sweep(cat *catalog) {
	ctx := context.Background()
	now := c.now()
	ids := cat.setIDs
	var list *listUpdate
	if now.Sub(cat.listFetched) > SetListMaxAge && c.bgAllowed("sets", now) {
		var l []setSummary
		if e, err := c.fetchEntry(ctx, "sets", &l); err == nil {
			ids = setIDs(l)
			list = &listUpdate{ids, e.Fetched}
		}
	}
	var newIDs, stale []string
	for _, id := range ids {
		e, ok := cat.sets[id]
		switch {
		case !ok:
			newIDs = append(newIDs, id)
		case now.Sub(e.fetched) > SetMaxAge && c.bgAllowed("sets/"+id, now):
			stale = append(stale, id)
		}
	}
	fresh, _ := c.loadSets(ctx, newIDs, nil, false)
	refreshed, _ := c.loadSets(ctx, stale, nil, true)
	for id, e := range refreshed {
		fresh[id] = e
	}
	if list != nil || len(fresh) > 0 {
		c.update(list, fresh)
	}
}

// bgAllowed reports whether a background refresh of key may start now,
// and if so records it.
func (c *Client) bgAllowed(key string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if recent(c.tracker.bg, key, now) {
		return false
	}
	mark(&c.tracker.bg, key, now)
	return true
}

// missAllowed reports whether a lookup miss on key may refresh data
// before answering: not offline, and the same miss has not done so in the
// last RetryAfter. If so it records it.
func (c *Client) missAllowed(key string) bool {
	if c.Offline {
		return false
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if recent(c.tracker.miss, key, now) {
		return false
	}
	mark(&c.tracker.miss, key, now)
	return true
}
