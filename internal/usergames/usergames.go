// Package usergames is per-account games for the hosted playmat: one game
// manager and board server per signed-in Google account, an activity
// clock, the idle drop and the server's places. Everything is in memory.
//
// Only hosted mode builds a Registry; the desktop keeps its single game
// manager and board server with no accounts.
package usergames

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/game"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
	"github.com/nicholasjackson/tcgreplay/internal/webui"
)

// Config is what a Registry is built from.
type Config struct {
	// NewManager returns an empty game manager for a new account (sharing
	// the card database and data directory).
	NewManager func() *game.Manager
	// NewBoard returns the board server for one account's manager (seat,
	// public URL, limit hook).
	NewBoard func(*game.Manager) *webui.Server
	// Public is the address the playmat is reached at, for board links.
	Public *url.URL
	// Idle is how long an account's game may go without activity before
	// it is dropped; 0 means never.
	Idle time.Duration
	// MaxUsers is the number of places: accounts that may hold a game at
	// once. 0 means no cap.
	MaxUsers int
	// Now is the clock; nil means time.Now. Tests substitute their own.
	Now func() time.Time
}

// purgeAfter is how long an entry that holds no place is kept without
// activity before it is removed.
const purgeAfter = 24 * time.Hour

// Registry maps each signed-in Google account to its own playmat. It is
// also the server's user cap (hostauth.Places): a place is taken at
// sign-in, or at an account's first action after losing one, and freed
// when its game goes idle, when its last sign-in ends, or on restart.
//
// Lock order: the registry's lock may be held while calling a manager
// (which may run its board's hub hooks), and the sign-in service may call
// Release with its own lock held; the registry never calls back into
// either.
type Registry struct {
	cfg     Config
	mu      sync.Mutex
	entries map[string]*Entry // by hostauth.Account.ID
}

// Entry is one account's playmat: its own game manager and board server.
type Entry struct {
	// M is the account's game manager.
	M *game.Manager
	// UI is the account's board server; its hub watches only M.
	UI *webui.Server
	// Handle is the account's board handle (see Handle).
	Handle string

	acct hostauth.Account
	// last is the last activity and holds whether the entry holds one of
	// the server's places; both are guarded by the registry's mu.
	last  time.Time
	holds bool
	ctx   context.Context
	stop  context.CancelFunc
	board http.Handler
}

// New returns an empty registry for cfg.
func New(cfg Config) *Registry {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Registry{cfg: cfg, entries: map[string]*Entry{}}
}

var _ hostauth.Places = (*Registry)(nil)

// Use returns acct's entry for an action (a tool call, a board move or a
// board page load), creating it the first time, and records the activity.
// A game idle for longer than the idle time is expired first, so the
// action sees the expiry notice. An account without a place takes one;
// when the server is full it is refused with an error wrapping
// hostauth.ErrBusy, and its activity is not recorded.
func (r *Registry) Use(acct hostauth.Account) (*Entry, error) {
	now := r.cfg.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entries[acct.ID]
	if e != nil {
		r.expireIfIdle(e, now)
	}
	if (e == nil || !e.holds) && r.full() {
		return nil, hostauth.BusyError(r.cfg.MaxUsers)
	}
	e = r.entryLocked(acct, now)
	e.last = now
	e.holds = true
	return e, nil
}

// Admit is hostauth.Places' check at every Google sign-in: acct takes a
// place, or is refused with an error wrapping hostauth.ErrBusy when every
// place is held. An account already holding one always gets in. Places
// freed by idle games are counted as free.
func (r *Registry) Admit(acct hostauth.Account) error {
	_, err := r.Use(acct)
	if err != nil {
		log.Printf("usergames: refused %s: %v", acct.Email, err)
	}
	return err
}

// Release is hostauth.Places' notice that an account's last sign-in has
// ended: its entry is removed, freeing its place and closing its boards.
func (r *Registry) Release(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.entries[id]; e != nil {
		log.Printf("usergames: %s signed out; removed", e.acct.Email)
	}
	r.removeLocked(id)
}

// full reports whether every place is held. Idle entries are expired
// first, so their places count as free. The caller holds mu.
func (r *Registry) full() bool {
	if r.cfg.MaxUsers <= 0 {
		return false
	}
	r.sweepLocked(r.cfg.Now())
	held := 0
	for _, e := range r.entries {
		if e.holds {
			held++
		}
	}
	return held >= r.cfg.MaxUsers
}

// Get returns acct's entry without counting as activity, for a board's
// live connection and card lookups, creating it (with no place) the first
// time. It too expires a game idle for longer than the idle time, so an
// open board learns of it.
func (r *Registry) Get(acct hostauth.Account) *Entry {
	now := r.cfg.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entryLocked(acct, now)
	r.expireIfIdle(e, now)
	return e
}

// entryLocked returns acct's entry, creating it (with no place) if absent.
// The caller holds mu.
func (r *Registry) entryLocked(acct hostauth.Account, now time.Time) *Entry {
	e := r.entries[acct.ID]
	if e == nil {
		e = r.newEntry(acct, now)
		r.entries[acct.ID] = e
	}
	if acct.Email != "" {
		e.acct.Email = acct.Email
	}
	return e
}

// expireIfIdle drops e's game and frees its place if it has gone without
// activity for longer than the idle time. It reports whether it did. The
// caller holds mu, which keeps an action from starting a new game between
// the check and the drop.
func (r *Registry) expireIfIdle(e *Entry, now time.Time) bool {
	if r.cfg.Idle <= 0 || !e.holds || now.Sub(e.last) <= r.cfg.Idle {
		return false
	}
	e.holds = false
	if _, err := e.M.Current(); err == nil {
		e.M.Expire(r.notice())
		log.Printf("usergames: game for %s expired after %s idle", e.acct.Email, humanDuration(r.cfg.Idle))
	}
	return true
}

// notice is what an expired game's account is told.
func (r *Registry) notice() string {
	return fmt.Sprintf("Your game expired after %s without activity and was cleared. Import its saved game record again (import_log with the record) to carry on.", humanDuration(r.cfg.Idle))
}

// humanDuration renders an idle time for people: seconds under a minute,
// minutes under two hours, else hours.
func humanDuration(d time.Duration) string {
	plural := func(n int64, unit string) string {
		if n == 1 {
			return fmt.Sprintf("1 %s", unit)
		}
		return fmt.Sprintf("%d %ss", n, unit)
	}
	switch {
	case d < time.Minute:
		return plural(int64(d/time.Second), "second")
	case d < 2*time.Hour:
		return plural(int64(d/time.Minute), "minute")
	default:
		return plural(int64(d/time.Hour), "hour")
	}
}

// Run sweeps for idle games until ctx is done: often enough that a game
// expires, and its place is freed, close to its idle time even if nobody
// touches it.
func (r *Registry) Run(ctx context.Context) {
	t := time.NewTicker(r.sweepEvery())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.Sweep()
		}
	}
}

// sweepEvery is the idle sweep's interval: a quarter of the idle time,
// between 1 and 30 seconds (30 seconds when games never expire, for the
// purge).
func (r *Registry) sweepEvery() time.Duration {
	if r.cfg.Idle <= 0 {
		return 30 * time.Second
	}
	return min(max(r.cfg.Idle/4, time.Second), 30*time.Second)
}

// Sweep runs one idle sweep now: it expires every game idle for longer
// than the idle time, freeing its place, and removes entries that have
// held no place and seen no activity for a day, closing their boards.
func (r *Registry) Sweep() {
	now := r.cfg.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked(now)
}

// sweepLocked is Sweep with mu held.
func (r *Registry) sweepLocked(now time.Time) {
	changed := false
	for id, e := range r.entries {
		if r.expireIfIdle(e, now) {
			changed = true
		}
		if !e.holds && now.Sub(e.last) > purgeAfter {
			r.removeLocked(id)
			log.Printf("usergames: removed idle account %s", e.acct.Email)
			changed = true
		}
	}
	if changed {
		held, games := 0, 0
		for _, e := range r.entries {
			if e.holds {
				held++
			}
			if _, err := e.M.Current(); err == nil {
				games++
			}
		}
		log.Printf("usergames: %d accounts, %d holding places, %d games", len(r.entries), held, games)
	}
}

// removeLocked forgets the entry for id and ends its lifetime, which stops
// its board's hub and closes its open live connections. The caller holds
// mu.
func (r *Registry) removeLocked(id string) {
	e := r.entries[id]
	if e == nil {
		return
	}
	delete(r.entries, id)
	e.stop()
}

// newEntry builds an account's manager and board server and starts the
// board's hub for the entry's lifetime.
func (r *Registry) newEntry(acct hostauth.Account, now time.Time) *Entry {
	m := r.cfg.NewManager()
	ui := r.cfg.NewBoard(m)
	ctx, stop := context.WithCancel(context.Background())
	ui.StartHub(ctx)
	return &Entry{
		M:      m,
		UI:     ui,
		Handle: Handle(acct.ID),
		acct:   acct,
		last:   now,
		ctx:    ctx,
		stop:   stop,
		board:  ui.Handler(),
	}
}

// LastActive returns when the entry was last used for an action.
func (r *Registry) LastActive(e *Entry) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return e.last
}

// Handle returns the stable board handle for a Google account id: a
// domain-separated SHA-256 of the id, truncated and base64url-encoded. It
// is the same across restarts, differs between accounts and does not
// reveal the id.
func Handle(accountID string) string {
	sum := sha256.Sum256([]byte("tcgreplay board\x00" + accountID))
	return base64.RawURLEncoding.EncodeToString(sum[:12])
}

// Link returns the entry's board link on the public address:
// <public>/?board=<handle>.
func (e *Entry) Link(public *url.URL) string {
	if public == nil {
		return ""
	}
	u := url.URL{Scheme: public.Scheme, Host: public.Host, Path: "/", RawQuery: url.Values{"board": {e.Handle}}.Encode()}
	return u.String()
}
