package cards

import (
	"sync"
	"time"
)

// breaker is the shared store's failure rule: after a failed request the
// store is skipped for retry, after which the next request tries it again.
// An outage is logged once when it starts (downMsg, given the error and the
// retry window) and once when it ends (upMsg). A failed probe re-arms the
// window without logging again.
type breaker struct {
	retry   time.Duration
	now     func() time.Time
	logf    func(string, ...any)
	downMsg string // format with the error (%v) and the retry window (%s)
	upMsg   string

	mu    sync.Mutex
	down  bool      // a request failed and none has succeeded since
	until time.Time // while down, the store is skipped until then
}

// skip reports whether the store is being skipped after a failure.
func (b *breaker) skip() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.down && b.now().Before(b.until)
}

// failed marks the store down for the retry window, logging only the first
// failure of an outage.
func (b *breaker) failed(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.down {
		b.logf(b.downMsg, err, b.retry)
	}
	b.down = true
	b.until = b.now().Add(b.retry)
}

// ok marks the store up, logging the recovery once.
func (b *breaker) ok() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.down {
		b.logf(b.upMsg)
		b.down = false
	}
}
