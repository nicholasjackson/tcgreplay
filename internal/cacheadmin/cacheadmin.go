// Package cacheadmin is the hosted playmat's card cache admin page: a page
// from which an admin clears the card cache, everything or one set, and
// the clear job behind it. It is served only in hosted mode, and only
// behind hostauth.RequireAdmin.
package cacheadmin

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// Cards is the card database a clear empties (a *cards.Client).
type Cards interface {
	// SetID names a set by its TCGdex id or decklist code; an unknown
	// name is cards.ErrUnknownSet.
	SetID(ctx context.Context, name string) (id, setName string, err error)
	Clear(ctx context.Context, s cards.Scope) []cards.TierReport
}

// Images is the card image cache a clear empties (a *cards.ImageCache).
type Images interface {
	Clear(ctx context.Context, s cards.Scope) []cards.TierReport
}

// ErrBusy means a clear is already running.
var ErrBusy = errors.New("a clear is already running")

// DefaultTimeout bounds a whole clear job.
const DefaultTimeout = 10 * time.Minute

// Job is one clear: what it covers, who started it and when, and, once it
// has ended, what each tier removed.
type Job struct {
	Scope   cards.Scope
	SetName string // the set's display name; empty for everything
	By      string // the admin's email
	Started time.Time
	Ended   time.Time
	Reports []cards.TierReport
	Running bool
}

// Label names the job's scope for people: "everything" or
// "<set name> (<set id>)".
func (j Job) Label() string {
	if j.Scope.All() {
		return "everything"
	}
	if j.SetName == "" || j.SetName == j.Scope.Set {
		return j.Scope.Set
	}
	return fmt.Sprintf("%s (%s)", j.SetName, j.Scope.Set)
}

// Failed reports whether any tier of an ended job failed. A CDN that is
// not configured is not a failure.
func (j Job) Failed() bool {
	for _, r := range j.Reports {
		if r.Err != nil && !errors.Is(r.Err, cards.ErrCDNNotConfigured) {
			return true
		}
	}
	return false
}

// Service runs one clear job at a time, in the background, so a long clear
// is never cut off by a request time limit, and keeps the current or last
// job for the page. Job state is in memory only.
type Service struct {
	Cards Cards
	// Images is nil without the shared store: then there are no image
	// copies to clear.
	Images Images
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Timeout bounds a whole job; zero means DefaultTimeout.
	Timeout time.Duration
	// Logf logs each job's start and end; nil means log.Printf.
	Logf func(format string, args ...any)

	mu  sync.Mutex
	job *Job
	ctx context.Context // the server's lifetime, set by Run
	wg  sync.WaitGroup  // jobs running
}

func (s *Service) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

func (s *Service) logf(format string, args ...any) {
	if s.Logf == nil {
		log.Printf(format, args...)
		return
	}
	s.Logf(format, args...)
}

// Run gives jobs the server's lifetime: a job runs on ctx, not on the
// request that started it, and ends when ctx does. It blocks until ctx is
// done.
func (s *Service) Run(ctx context.Context) {
	s.mu.Lock()
	s.ctx = ctx
	s.mu.Unlock()
	<-ctx.Done()
}

// Start starts clearing everything (all) or the set name names, for the
// admin by, and returns the job as it starts. A set name is resolved
// first, so an unknown or empty one (cards.ErrUnknownSet) starts nothing.
// While a job runs, Start returns ErrBusy.
func (s *Service) Start(ctx context.Context, by, name string, all bool) (Job, error) {
	if s.busy() {
		return Job{}, ErrBusy
	}
	j := Job{By: by}
	if !all {
		if strings.TrimSpace(name) == "" {
			return Job{}, cards.ErrUnknownSet
		}
		id, setName, err := s.Cards.SetID(ctx, name)
		if err != nil {
			return Job{}, err
		}
		j.Scope, j.SetName = cards.Scope{Set: id}, setName
	}

	s.mu.Lock()
	if s.job != nil && s.job.Running {
		s.mu.Unlock()
		return Job{}, ErrBusy
	}
	j.Started, j.Running = s.now(), true
	job := j
	s.job = &job
	base := s.ctx
	if base == nil {
		base = context.Background()
	}
	s.wg.Add(1)
	s.mu.Unlock()

	timeout := s.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	s.logf("cache admin: %s started clearing %s", by, j.Label())
	go func() {
		defer s.wg.Done()
		jctx, cancel := context.WithTimeout(base, timeout)
		defer cancel()
		reports := s.Cards.Clear(jctx, j.Scope)
		if s.Images != nil {
			reports = append(reports, s.Images.Clear(jctx, j.Scope)...)
		}
		s.mu.Lock()
		job.Reports, job.Ended, job.Running = reports, s.now(), false
		done := job
		s.mu.Unlock()
		parts := make([]string, len(reports))
		for i, r := range reports {
			parts[i] = r.String()
		}
		s.logf("cache admin: cleared %s for %s in %s: %s", done.Label(), done.By, done.Ended.Sub(done.Started).Round(time.Millisecond), strings.Join(parts, "; "))
	}()
	return j, nil
}

// busy reports whether a job is running.
func (s *Service) busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.job != nil && s.job.Running
}

// Status returns the running job, or the last one, and whether there has
// been one since start.
func (s *Service) Status() (Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == nil {
		return Job{}, false
	}
	j := *s.job
	j.Reports = append([]cards.TierReport(nil), j.Reports...)
	return j, true
}

// wait waits for every job started so far to end. It is for tests.
func (s *Service) wait() { s.wg.Wait() }
