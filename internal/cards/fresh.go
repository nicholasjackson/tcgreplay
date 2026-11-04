package cards

import (
	"context"
	"encoding/json"
	"time"
)

// Refresh ages: an entry older than its kind's age is stale. A stale entry
// is still served at once, and refreshed in the background.
const (
	// SetListMaxAge is the refresh age of the set list.
	SetListMaxAge = 30 * 24 * time.Hour
	// SetMaxAge is the refresh age of a set's card list.
	SetMaxAge = 90 * 24 * time.Hour
	// ImagelessCardMaxAge is the refresh age of a card cached without
	// artwork. A card with artwork never goes stale.
	ImagelessCardMaxAge = 24 * time.Hour
)

// RetryAfter is how long the card database waits before asking the card
// source again for a card it failed to return, or before retrying a
// background refresh of the same entry.
const RetryAfter = 5 * time.Minute

// refreshTracker remembers recent attempts on the card source, so a down
// source or an unknown card costs one request per RetryAfter rather than
// one per lookup. It is in memory only, and guarded by Client.mu.
type refreshTracker struct {
	// failed holds when each card id last failed to come back from the
	// card source (unknown, error or timeout).
	failed map[string]time.Time
	// bg holds when each key last had a background refresh started
	// ("sweep" for a sweep of the catalog).
	bg map[string]time.Time
	// miss holds when each lookup miss last refreshed data before
	// answering: "code:<SET CODE>" or "num:<set id>:<number>".
	miss map[string]time.Time
}

// recent reports whether key was marked in m less than RetryAfter before
// now.
func recent(m map[string]time.Time, key string, now time.Time) bool {
	t, ok := m[key]
	return ok && now.Sub(t) < RetryAfter
}

// mark records an attempt on key at now.
func mark(m *map[string]time.Time, key string, now time.Time) {
	if *m == nil {
		*m = map[string]time.Time{}
	}
	(*m)[key] = now
}

// recentlyFailed reports whether the card source failed to return card id
// within the last RetryAfter.
func (c *Client) recentlyFailed(id string) bool {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	return recent(c.tracker.failed, id, now)
}

// noteCard records the outcome of fetching card id from the card source.
func (c *Client) noteCard(id string, err error) {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		mark(&c.tracker.failed, id, now)
	} else {
		delete(c.tracker.failed, id)
	}
}

// cardStale reports whether a cached card is due a refresh: it has no
// artwork (as served, so Basic Energy with borrowed art counts as having
// it) and was fetched more than ImagelessCardMaxAge before now.
func cardStale(e cardEntry, now time.Time) bool {
	return e.card.Image == "" && now.Sub(e.fetched) > ImagelessCardMaxAge
}

// maybeRefresh starts a background refresh of key, unless the client is
// offline or a refresh of key started less than RetryAfter ago. On
// success the new entry is stored and passed to apply; on failure
// everything is left as it was, so the stale copy keeps being served.
func (c *Client) maybeRefresh(key string, apply func(Entry)) {
	if c.Offline {
		return
	}
	now := c.now()
	c.mu.Lock()
	if recent(c.tracker.bg, key, now) {
		c.mu.Unlock()
		return
	}
	mark(&c.tracker.bg, key, now)
	c.mu.Unlock()
	c.bgwg.Add(1)
	go func() {
		defer c.bgwg.Done()
		if e, err := c.shared(context.Background(), key, false); err == nil {
			apply(e)
		}
	}()
}

// refreshCardIfStale starts a background refresh of a stale card; the
// refreshed card replaces it in memory.
func (c *Client) refreshCardIfStale(id string, e cardEntry) {
	if !cardStale(e, c.now()) {
		return
	}
	c.maybeRefresh("cards/"+id, func(ne Entry) {
		var card Card
		if json.Unmarshal(ne.Data, &card) != nil {
			return
		}
		energyArt(&card)
		c.remember(id, &card, ne.Fetched)
	})
}

// waitBackground waits for every background refresh started so far.
func (c *Client) waitBackground() { c.bgwg.Wait() }
