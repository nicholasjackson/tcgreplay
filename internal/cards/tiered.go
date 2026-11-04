package cards

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

const (
	// SharedTimeout bounds each request to the shared store.
	SharedTimeout = 3 * time.Second
	// SharedRetryAfter is how long the shared store is skipped after a
	// request to it fails.
	SharedRetryAfter = time.Minute
)

// NewTieredStore returns a store that puts local in front of shared.
//
// A read is served from local. On a local miss it is served from shared,
// and the entry is copied to local with its Fetched time unchanged, so an
// entry's age survives the copy and a restart never makes stale data look
// fresh. A write goes to local and then to shared. Like every store it
// knows nothing about expiry.
//
// Shared failures never reach the caller: each shared request is bounded
// by SharedTimeout, and a failure (anything but success or a miss) is
// logged and makes the store skip shared for SharedRetryAfter, after which
// the next request tries it again. An outage is logged once when it starts
// and once when it ends. A write skipped meanwhile is not queued: the
// entry reaches shared the next time it is fetched.
//
// Callers that must not reach the network read the local tier alone,
// through Local.
func NewTieredStore(local, shared Store) Store {
	return &tieredStore{
		local:   local,
		shared:  shared,
		timeout: SharedTimeout,
		breaker: breaker{
			retry:   SharedRetryAfter,
			now:     time.Now,
			logf:    log.Printf,
			downMsg: "card cache: shared store unavailable (%v); using the local cache and the card source, retrying in %s",
			upMsg:   "card cache: shared store reachable again",
		},
	}
}

type tieredStore struct {
	local, shared Store
	timeout       time.Duration
	breaker       // skips shared after a failure; logs each outage once
}

// Local returns the local tier.
func (t *tieredStore) Local() Store { return t.local }

// outcome records how a shared request ended. A caller that gave up (its
// own context ended) says nothing about the shared store, so it neither
// trips nor clears the breaker.
func (t *tieredStore) outcome(ctx context.Context, op, key string, err error) {
	switch {
	case err == nil || errors.Is(err, ErrNotCached):
		t.ok()
	case ctx.Err() != nil:
	default:
		t.failed(fmt.Errorf("%s %s: %w", op, key, err))
	}
}

// Get implements Store.
func (t *tieredStore) Get(ctx context.Context, key string) (Entry, error) {
	if e, err := t.local.Get(ctx, key); err == nil {
		return e, nil
	}
	// Any local failure counts as a miss: shared or the card source can
	// still answer.
	if t.skip() {
		return Entry{}, fmt.Errorf("%s: %w", key, ErrNotCached)
	}
	sctx, cancel := context.WithTimeout(ctx, t.timeout)
	e, err := t.shared.Get(sctx, key)
	cancel()
	t.outcome(ctx, "get", key, err)
	if err != nil {
		return Entry{}, fmt.Errorf("%s: %w", key, ErrNotCached)
	}
	t.local.Put(ctx, key, e) // a local write failure only costs another shared read
	return e, nil
}

// Put implements Store. Only a local failure is returned.
func (t *tieredStore) Put(ctx context.Context, key string, e Entry) error {
	lerr := t.local.Put(ctx, key, e)
	if !t.skip() {
		sctx, cancel := context.WithTimeout(ctx, t.timeout)
		err := t.shared.Put(sctx, key, e)
		cancel()
		t.outcome(ctx, "put", key, err)
	}
	return lerr
}
