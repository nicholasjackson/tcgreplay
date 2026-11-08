package catalog

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cardset"
)

// DefaultInterval is how often a running playmat checks for a publish.
const DefaultInterval = 5 * time.Minute

const (
	// maxManifest bounds how much of a manifest is read.
	maxManifest = 4 << 20
	// maxSet bounds a set object, compressed and uncompressed.
	maxSet = 64 << 20
	// downloads is how many set objects a check fetches at a time.
	downloads = 8
	// localDir is the local copy's directory within the cache directory.
	// The loader reads and writes nothing else there, so the files the
	// on-demand card cache left (sets.json, sets/, cards/, set-index*.json)
	// are never read.
	localDir = "cardsets"
)

// Loader holds the playmat's current catalogue snapshot and keeps it up to
// date with the published card sets. It fetches the manifest anonymously
// from Base (an https address, or a local directory), downloads and
// verifies only the sets whose hash changed, and swaps in a new snapshot
// in one step. It keeps a content-addressed copy of what it loaded under
// <CacheDir>/cardsets/, with the manifest written last, and loads that
// copy at start, so a playmat that cannot reach Base (or runs Offline)
// plays from it.
type Loader struct {
	Base     string        // published-sets address: https://…/cardsets/ or a directory
	CacheDir string        // the local copy lives in <CacheDir>/cardsets/; "" keeps none
	Offline  bool          // never fetch; use the local copy alone
	Interval time.Duration // between checks in Run; DefaultInterval when 0
	HTTP     *http.Client  // a client with a 30 second timeout when nil
	Logf     func(string, ...any)

	cur     atomic.Pointer[Snapshot]
	checkMu sync.Mutex // serialises checks

	mu     sync.Mutex // guards the fields below
	status Status
	down   bool // the last check failed; logged once until one succeeds
}

// Status is the loader's state, for the admin page.
type Status struct {
	Version    string    // sha256 of the manifest the current snapshot came from; "" when empty
	Sets       int       // sets in the current snapshot
	Prints     int       // cards in the current snapshot
	LastCheck  time.Time // when the last check finished; zero before one
	LastChange time.Time // when the snapshot last changed
	LastError  string    // the last check's error; "" when it succeeded
	Base       string
	Offline    bool
}

// CheckResult is what a check picked up.
type CheckResult struct {
	Version string   // the current snapshot's version after the check
	Changed []string // set ids added or replaced, sorted
	Removed []string // set ids dropped, sorted
}

func (r CheckResult) String() string {
	if len(r.Changed) == 0 && len(r.Removed) == 0 {
		return "already up to date (version " + short(r.Version) + ")"
	}
	return fmt.Sprintf("picked up version %s: changed %s, removed %s", short(r.Version), list(r.Changed), list(r.Removed))
}

func short(v string) string {
	if len(v) > 12 {
		return v[:12]
	}
	if v == "" {
		return "none"
	}
	return v
}

// maxListed bounds how many set ids a log line names.
const maxListed = 10

func list(ids []string) string {
	switch {
	case len(ids) == 0:
		return "none"
	case len(ids) > maxListed:
		return fmt.Sprintf("%s and %d more", strings.Join(ids[:maxListed], ", "), len(ids)-maxListed)
	}
	return strings.Join(ids, ", ")
}

func (l *Loader) logf(format string, args ...any) {
	if l.Logf != nil {
		l.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

// Current is the current snapshot; never nil, and Empty before anything
// is loaded.
func (l *Loader) Current() *Snapshot {
	if s := l.cur.Load(); s != nil {
		return s
	}
	return Empty
}

// Snapshot is Current, as the card database a new game pins.
func (l *Loader) Snapshot() cards.DB { return l.Current() }

// SetID names a set through the current snapshot (see Snapshot.SetID).
func (l *Loader) SetID(name string) (id, setName string, err error) { return l.Current().SetID(name) }

// Status reports the loader's state.
func (l *Loader) Status() Status {
	cur := l.Current()
	l.mu.Lock()
	st := l.status
	l.mu.Unlock()
	st.Version, st.Sets, st.Prints = cur.Version(), len(cur.sets), cur.Prints()
	st.Base, st.Offline = l.Base, l.Offline
	return st
}

// Start loads the local copy, then, unless offline, checks the published
// sets, bounded by wait, so the playmat serves the newest catalogue it can
// get without waiting long on an outage. It logs what it loaded.
func (l *Loader) Start(ctx context.Context, wait time.Duration) {
	from := "local copy"
	if s, err := l.loadLocal(); err != nil {
		l.logf("card sets: local copy not used: %v", err)
	} else if s != nil {
		l.swap(s)
	}
	if !l.Offline {
		cctx, cancel := context.WithTimeout(ctx, wait)
		res, err := l.Check(cctx)
		cancel()
		if err == nil && (len(res.Changed) > 0 || len(res.Removed) > 0) {
			from = l.Base
		}
	}
	cur := l.Current()
	if len(cur.sets) == 0 {
		l.logf("card sets: none loaded; cards will show as unknown")
		return
	}
	l.logf("card sets: loaded %d sets (version %s) from %s", len(cur.sets), short(cur.Version()), from)
}

// Run checks the published sets every Interval until ctx ends. Offline,
// it returns at once.
func (l *Loader) Run(ctx context.Context) {
	if l.Offline {
		return
	}
	every := l.Interval
	if every <= 0 {
		every = DefaultInterval
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			l.Check(ctx) // errors are logged and kept in Status
		}
	}
}

// Check fetches the published manifest and, when it changed, downloads and
// verifies the changed sets, writes them to the local copy, and swaps in
// the new snapshot. Any failure before the swap leaves the current
// snapshot in place and is returned (and logged once per outage).
// Overlapping checks run one after another.
func (l *Loader) Check(ctx context.Context) (CheckResult, error) {
	l.checkMu.Lock()
	defer l.checkMu.Unlock()
	if l.Offline {
		return CheckResult{Version: l.Current().Version()}, errors.New("card sets: offline; using the local copy only")
	}
	res, err := l.check(ctx)
	l.mu.Lock()
	l.status.LastCheck = time.Now()
	wasDown := l.down
	if err != nil {
		l.status.LastError = err.Error()
		l.down = true
	} else {
		l.status.LastError = ""
		l.down = false
		if len(res.Changed) > 0 || len(res.Removed) > 0 {
			l.status.LastChange = l.status.LastCheck
		}
	}
	l.mu.Unlock()
	switch {
	case err != nil && !wasDown:
		l.logf("card sets: check failed (%v); keeping version %s", err, short(l.Current().Version()))
	case err == nil && wasDown:
		l.logf("card sets: published sets reachable again")
	}
	if err == nil && (len(res.Changed) > 0 || len(res.Removed) > 0) {
		l.logf("card sets: %s", res)
	}
	return res, err
}

// fetched is one set downloaded (or read from the local copy) and
// converted.
type fetched struct {
	id   string
	gz   []byte // the object's bytes, to write to the local copy
	data *setData
}

func (l *Loader) check(ctx context.Context) (CheckResult, error) {
	raw, err := l.fetch(ctx, cardset.ManifestKey, maxManifest)
	if err != nil {
		return CheckResult{}, err
	}
	m, err := cardset.DecodeManifest(raw)
	if err != nil {
		return CheckResult{}, err
	}
	version := sum(raw)
	cur := l.Current()
	if version == cur.Version() {
		return CheckResult{Version: version}, nil
	}

	var need []string
	keep := map[string]bool{}
	for _, id := range slices.Sorted(maps.Keys(m.Sets)) {
		e := m.Sets[id]
		old := cur.sets[id]
		if old != nil && old.hash == e.SHA256 {
			keep[id] = true
			continue
		}
		if e.Format != cardset.Format {
			l.logf("card sets: set %s is in format %d, which this program does not read; skipped", id, e.Format)
			if old != nil {
				keep[id] = true // keeps the version it has
			}
			continue
		}
		need = append(need, id)
	}

	got, err := l.download(ctx, m, need)
	if err != nil {
		return CheckResult{}, err
	}
	changed := make(map[string]*setData, len(got))
	for _, f := range got {
		if err := l.writeLocal(cardset.SetKey(f.id, f.data.hash), f.gz); err != nil {
			l.logf("card sets: local copy of set %s not written: %v", f.id, err)
		}
		changed[f.id] = f.data
	}
	var removed []string
	for id := range cur.sets {
		if !keep[id] && changed[id] == nil {
			removed = append(removed, id)
		}
	}
	slices.Sort(removed)
	next := Next(cur, version, changed, removed)
	if err := l.writeLocalManifest(next); err != nil {
		l.logf("card sets: local copy's manifest not written: %v", err)
	}
	l.swap(next)
	l.pruneLocal(next)
	return CheckResult{Version: version, Changed: slices.Sorted(maps.Keys(changed)), Removed: removed}, nil
}

// download fetches, verifies and converts the sets ids names, downloads
// at a time, preferring a verified object already in the local copy. Any
// failure fails the whole download.
func (l *Loader) download(ctx context.Context, m cardset.Manifest, ids []string) ([]fetched, error) {
	out := make([]fetched, len(ids))
	errs := make([]error, len(ids))
	sem := make(chan struct{}, downloads)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			e := m.Sets[id]
			if gz, err := l.readLocal(cardset.SetKey(id, e.SHA256)); err == nil {
				if d, err := openSet(gz, e.SHA256); err == nil {
					out[i] = fetched{id: id, gz: gz, data: d}
					return
				}
			}
			gz, err := l.fetch(ctx, e.Path, maxSet)
			if err != nil {
				errs[i] = fmt.Errorf("set %s: %w", id, err)
				return
			}
			d, err := openSet(gz, e.SHA256)
			if err != nil {
				errs[i] = fmt.Errorf("set %s: %w", id, err)
				return
			}
			out[i] = fetched{id: id, gz: gz, data: d}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

// openSet decompresses a set object, checks it against the sha256 the
// manifest gives for the built file, and converts it.
func openSet(gz []byte, want string) (*setData, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(zr, maxSet+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxSet {
		return nil, fmt.Errorf("larger than %d bytes uncompressed", maxSet)
	}
	if got := sum(raw); got != want {
		return nil, fmt.Errorf("sha256 %s does not match the manifest's %s", short(got), short(want))
	}
	s, err := cardset.Decode(raw)
	if err != nil {
		return nil, err
	}
	if s.Format != cardset.Format {
		return nil, fmt.Errorf("format %d is not one this program reads", s.Format)
	}
	return convertSet(s, want)
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// isURL reports whether the base is an http or https address rather than
// a directory.
func (l *Loader) isURL() bool {
	return strings.HasPrefix(l.Base, "https://") || strings.HasPrefix(l.Base, "http://")
}

// fetch reads rel (a path relative to the manifest's directory) from the
// base, at most limit bytes.
func (l *Loader) fetch(ctx context.Context, rel string, limit int64) ([]byte, error) {
	if l.Base == "" {
		return nil, errors.New("no published card sets address")
	}
	if !l.isURL() {
		b, err := os.ReadFile(filepath.Join(l.Base, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		if int64(len(b)) > limit {
			return nil, fmt.Errorf("%s: larger than %d bytes", rel, limit)
		}
		return b, nil
	}
	u := strings.TrimSuffix(l.Base, "/") + "/" + rel
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c := l.HTTP
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", u, err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("GET %s: larger than %d bytes", u, limit)
	}
	return b, nil
}

// local is the path of key in the local copy, or "" when there is none.
func (l *Loader) local(key string) string {
	if l.CacheDir == "" {
		return ""
	}
	return filepath.Join(l.CacheDir, localDir, filepath.FromSlash(key))
}

func (l *Loader) readLocal(key string) ([]byte, error) {
	p := l.local(key)
	if p == "" {
		return nil, fs.ErrNotExist
	}
	return os.ReadFile(p)
}

// writeLocal writes key to the local copy through a temporary file, so an
// interrupted write never leaves half an object under its name.
func (l *Loader) writeLocal(key string, b []byte) error {
	p := l.local(key)
	if p == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), "."+filepath.Base(p)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(b)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// localManifest is the manifest of the sets s holds, in the local copy's
// own layout.
func localManifest(s *Snapshot) cardset.Manifest {
	m := cardset.Manifest{Format: cardset.ManifestFormat, Sets: map[string]cardset.ManifestSet{}}
	for id, d := range s.sets {
		m.Sets[id] = cardset.ManifestSet{Path: cardset.SetKey(id, d.hash), SHA256: d.hash, Prints: len(d.cards), Format: cardset.Format}
	}
	return m
}

// writeLocalManifest writes the local copy's manifest, after every object
// it names, then the version the snapshot came from.
func (l *Loader) writeLocalManifest(s *Snapshot) error {
	if l.CacheDir == "" {
		return nil
	}
	b, err := cardset.EncodeManifest(localManifest(s))
	if err != nil {
		return err
	}
	if err := l.writeLocal(cardset.ManifestKey, b); err != nil {
		return err
	}
	return l.writeLocal(versionKey, []byte(s.Version()+" "+sum(b)+"\n"))
}

// versionKey holds the version of the snapshot the local copy was written
// from, with the sha256 of the local manifest it goes with, so a restart
// reports the same version and its first check finds nothing new. A
// version written for another manifest (an interrupted write) is ignored.
const versionKey = "version"

// loadLocal loads the local copy: every set its manifest names, each
// verified against its hash. A copy with a set missing or damaged is not
// used at all, so a snapshot is never partial. No copy gives nil.
func (l *Loader) loadLocal() (*Snapshot, error) {
	raw, err := l.readLocal(cardset.ManifestKey)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m, err := cardset.DecodeManifest(raw)
	if err != nil {
		return nil, err
	}
	var sets []*setData
	for _, id := range slices.Sorted(maps.Keys(m.Sets)) {
		e := m.Sets[id]
		gz, err := l.readLocal(e.Path)
		if err != nil {
			return nil, fmt.Errorf("set %s: %w", id, err)
		}
		d, err := openSet(gz, e.SHA256)
		if err != nil {
			return nil, fmt.Errorf("set %s: %w", id, err)
		}
		sets = append(sets, d)
	}
	version := sum(raw)
	if v, err := l.readLocal(versionKey); err == nil {
		if ver, of, ok := strings.Cut(strings.TrimSpace(string(v)), " "); ok && of == version && ver != "" {
			version = ver
		}
	}
	return NewSnapshot(version, sets), nil
}

// pruneLocal removes set objects the local copy's manifest no longer
// names, after a successful swap.
func (l *Loader) pruneLocal(s *Snapshot) {
	if l.CacheDir == "" {
		return
	}
	named := map[string]bool{}
	for id, d := range s.sets {
		named[cardset.SetKey(id, d.hash)] = true
	}
	root := filepath.Join(l.CacheDir, localDir)
	filepath.WalkDir(filepath.Join(root, "sets"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		if key := filepath.ToSlash(rel); !named[key] && path.Ext(key) == ".gz" {
			os.Remove(p)
		}
		return nil
	})
}

// swap makes s the current snapshot.
func (l *Loader) swap(s *Snapshot) {
	l.cur.Store(s)
}
