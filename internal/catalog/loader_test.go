package catalog

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cardset"
)

// mon is a built set holding one Pokémon (<id>-001, with an attack, so the
// set passes the publish check) and one Trainer (<id>-002, with an effect).
func mon(id, abbr, name string, hp int, effect string) *cardset.Set {
	return builtSet(header{id: id, serie: "sv", release: "2024-01-01", abbr: abbr, name: name + " Set"},
		pr{local: "001", name: name, art: true, attacks: []string{"Tackle"}, hp: hp},
		pr{local: "002", name: name + " Ball", art: true, cat: "Trainer", effect: effect})
}

// logBuf captures a loader's log lines; Check may log from several
// goroutines.
type logBuf struct {
	mu    sync.Mutex
	lines []string
}

func (b *logBuf) logf(format string, args ...any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, fmt.Sprintf(format, args...))
}

// count is how many lines contain substr.
func (b *logBuf) count(substr string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, l := range b.lines {
		if strings.Contains(l, substr) {
			n++
		}
	}
	return n
}

func (b *logBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Join(b.lines, "\n")
}

// server serves a published directory over http, counting requests per
// path, and answers 500 to everything while fail is set.
type server struct {
	*httptest.Server
	fail atomic.Bool
	mu   sync.Mutex
	hits map[string]int
}

func serve(t *testing.T, dir string) *server {
	t.Helper()
	s := &server{hits: map[string]int{}}
	files := http.FileServer(http.Dir(dir))
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits[strings.TrimPrefix(r.URL.Path, "/")]++
		s.mu.Unlock()
		if s.fail.Load() {
			http.Error(w, "storage down", http.StatusInternalServerError)
			return
		}
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

// requests is a copy of the per-path request counts.
func (s *server) requests() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int, len(s.hits))
	for k, v := range s.hits {
		out[k] = v
	}
	return out
}

func (s *server) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits = map[string]int{}
}

// env is a card-set directory published to a directory served over http.
type env struct {
	t   *testing.T
	src string // the card-set directory (cardsets/)
	pub string // the published layout
	srv *server
}

func newEnv(t *testing.T, sets ...*cardset.Set) *env {
	t.Helper()
	e := &env{t: t, src: t.TempDir(), pub: t.TempDir()}
	e.publish(sets...)
	e.srv = serve(t, e.pub)
	return e
}

// publish writes the sets into the card-set directory and publishes it.
func (e *env) publish(sets ...*cardset.Set) {
	e.t.Helper()
	for _, s := range sets {
		if err := cardset.Write(filepath.Join(e.src, "sets", s.Set.ID+".json"), s); err != nil {
			e.t.Fatal(err)
		}
	}
	_, err := cardset.Publish(context.Background(), cardset.PublishOptions{
		Dir: e.src, Prefix: "", Target: cardset.DirTarget{Dir: e.pub}, Logf: func(string, ...any) {},
	})
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) manifestBytes() []byte {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.pub, cardset.ManifestKey))
	if err != nil {
		e.t.Fatal(err)
	}
	return b
}

func (e *env) manifest() cardset.Manifest {
	e.t.Helper()
	m, err := cardset.DecodeManifest(e.manifestBytes())
	if err != nil {
		e.t.Fatal(err)
	}
	return m
}

// version is the version a loader reports for the published manifest:
// its sha256.
func (e *env) version() string {
	e.t.Helper()
	h := sha256.Sum256(e.manifestBytes())
	return hex.EncodeToString(h[:])
}

// objectPath is the published object path of set id.
func (e *env) objectPath(id string) string {
	e.t.Helper()
	p := e.manifest().Sets[id].Path
	if p == "" {
		e.t.Fatalf("set %s not in the published manifest", id)
	}
	return p
}

func writeFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func gz(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// started is a loader started against base with cacheDir, waiting at most
// a second for its first check.
func started(t *testing.T, base, cacheDir string, offline bool, logs *logBuf) *Loader {
	t.Helper()
	l := &Loader{Base: base, CacheDir: cacheDir, Offline: offline, Logf: logs.logf}
	l.Start(context.Background(), time.Second)
	return l
}

// wantHP checks the loader's current snapshot serves card id with hp.
func wantHP(t *testing.T, l *Loader, id string, hp int) {
	t.Helper()
	c, err := l.Current().Card(context.Background(), id)
	if err != nil {
		t.Fatalf("card %s: %v", id, err)
	}
	if c.HP != hp {
		t.Errorf("card %s HP %d, want %d", id, c.HP, hp)
	}
}

// wantEmpty checks every lookup in the loader's catalogue finds nothing.
func wantEmpty(t *testing.T, l *Loader) {
	t.Helper()
	ctx := context.Background()
	cur := l.Current()
	if _, err := cur.Card(ctx, "sva-001"); !errors.Is(err, cards.ErrNotFound) {
		t.Errorf("Card(sva-001) = %v, want not found", err)
	}
	if c := l.Snapshot().Cached("sva-001"); c != nil {
		t.Errorf("Cached(sva-001) = %+v", c)
	}
	if _, err := cur.Resolve(ctx, "SVA", "1"); !errors.Is(err, cards.ErrNotFound) {
		t.Errorf("Resolve(SVA, 1) = %v, want not found", err)
	}
	if _, err := cur.ByName(ctx, "Pikachu"); !errors.Is(err, cards.ErrNotFound) {
		t.Errorf("ByName(Pikachu) = %v, want not found", err)
	}
	if _, _, err := l.SetID("sva"); !errors.Is(err, cards.ErrUnknownSet) {
		t.Errorf("SetID(sva) = %v, want unknown set", err)
	}
	st := l.Status()
	if st.Sets != 0 || st.Prints != 0 || st.Version != "" {
		t.Errorf("Status = %+v, want no sets, prints or version", st)
	}
}

// A publish is picked up at once when asked (Check), with no restart,
// from an https-style base or a local directory. (Criterion 1.)
func TestLoaderCheckPicksUpPublish(t *testing.T) {
	for _, viaDir := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "directory"}[viaDir], func(t *testing.T) {
			e := newEnv(t, mon("sva", "SVA", "Pikachu", 60, "Search your deck."))
			base := e.srv.URL
			if viaDir {
				base = e.pub
			}
			logs := &logBuf{}
			l := started(t, base, t.TempDir(), false, logs)
			v1 := e.version()
			wantHP(t, l, "sva-001", 60)
			if got := l.Current().Version(); got != v1 {
				t.Errorf("version %q, want the published manifest's sha256 %q", got, v1)
			}
			if logs.count("card sets: loaded 1 sets (version "+v1[:12]+") from "+base) != 1 {
				t.Errorf("no start line naming the base:\n%s", logs)
			}

			e.publish(mon("sva", "SVA", "Pikachu", 90, "Search your deck for a card."))
			wantHP(t, l, "sva-001", 60) // nothing changes until a check
			res, err := l.Check(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			v2 := e.version()
			if v2 == v1 || res.Version != v2 || !slices.Equal(res.Changed, []string{"sva"}) || len(res.Removed) != 0 {
				t.Errorf("Check = %+v, want version %s changing sva", res, v2)
			}
			wantHP(t, l, "sva-001", 90)
			if c := l.Current().Cached("sva-002"); c == nil || c.Effect != "Search your deck for a card." {
				t.Errorf("trainer text not updated: %+v", c)
			}
			st := l.Status()
			if st.Version != v2 || st.Sets != 1 || st.Prints != 2 || st.LastError != "" || st.LastChange.IsZero() || st.Base != base {
				t.Errorf("Status = %+v", st)
			}
			if logs.count("card sets: picked up version "+v2[:12]+": changed sva, removed none") != 1 {
				t.Errorf("no pick-up line:\n%s", logs)
			}

			// Nothing new: an up-to-date check changes nothing.
			res, err = l.Check(context.Background())
			if err != nil || res.Version != v2 || len(res.Changed)+len(res.Removed) != 0 {
				t.Errorf("second Check = %+v, %v", res, err)
			}
			if got := res.String(); got != "already up to date (version "+v2[:12]+")" {
				t.Errorf("String = %q", got)
			}
		})
	}
}

// A running loader picks up a publish on its next interval check.
// (Criterion 1.)
func TestLoaderRunPicksUpPublishOnInterval(t *testing.T) {
	e := newEnv(t, mon("sva", "SVA", "Pikachu", 60, "Search your deck."))
	logs := &logBuf{}
	l := &Loader{Base: e.srv.URL, CacheDir: t.TempDir(), Interval: 20 * time.Millisecond, Logf: logs.logf}
	l.Start(context.Background(), time.Second)
	wantHP(t, l, "sva-001", 60)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		l.Run(ctx)
	}()
	defer func() {
		cancel()
		<-done
	}()

	e.publish(mon("sva", "SVA", "Pikachu", 90, "Search your deck."))
	want := e.version()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if c := l.Current().Cached("sva-001"); c != nil && c.HP == 90 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("publish not picked up by Run; log:\n%s", logs)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := l.Current().Version(); got != want {
		t.Errorf("version %q, want %q", got, want)
	}
}

// Offline, Run returns at once and Check refuses, keeping the catalogue.
func TestLoaderOfflineNeverFetches(t *testing.T) {
	e := newEnv(t, mon("sva", "SVA", "Pikachu", 60, "Search your deck."))
	l := started(t, e.srv.URL, t.TempDir(), true, &logBuf{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		l.Run(context.Background())
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return offline")
	}
	if _, err := l.Check(context.Background()); err == nil {
		t.Error("offline Check succeeded")
	}
	if n := len(e.srv.requests()); n != 0 {
		t.Errorf("offline loader made requests: %v", e.srv.requests())
	}
	wantEmpty(t, l)
}

// A check downloads only the sets whose published version changed, and
// the new snapshot shares the unchanged sets with the old one. A restart
// from the local copy fetches only the manifest. (Criterion 2.)
func TestLoaderDownloadsOnlyChangedSets(t *testing.T) {
	e := newEnv(t,
		mon("sva", "SVA", "Pikachu", 60, "A."),
		mon("svb", "SVB", "Eevee", 50, "B."),
		mon("svc", "SVC", "Mew", 70, "C."))
	cache := t.TempDir()
	l := started(t, e.srv.URL, cache, false, &logBuf{})
	first := e.srv.requests()
	if len(first) != 4 || first[cardset.ManifestKey] != 1 || first[e.objectPath("sva")] != 1 || first[e.objectPath("svb")] != 1 || first[e.objectPath("svc")] != 1 {
		t.Errorf("first load requested %v, want the manifest and each set once", first)
	}
	prev := l.Current()

	e.srv.reset()
	e.publish(mon("svb", "SVB", "Eevee", 80, "B."))
	res, err := l.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Changed, []string{"svb"}) || len(res.Removed) != 0 {
		t.Errorf("Check = %+v, want svb changed", res)
	}
	want := map[string]int{cardset.ManifestKey: 1, e.objectPath("svb"): 1}
	if got := e.srv.requests(); !maps.Equal(got, want) {
		t.Errorf("check requested %v, want %v", got, want)
	}

	cur := l.Current()
	for _, id := range []string{"sva", "svc"} {
		if cur.sets[id] != prev.sets[id] {
			t.Errorf("unchanged set %s not shared with the previous snapshot", id)
		}
	}
	if cur.sets["svb"] == prev.sets["svb"] {
		t.Error("changed set svb is the old one")
	}
	wantHP(t, l, "svb-001", 80)
	if c := prev.Cached("svb-001"); c == nil || c.HP != 50 {
		t.Errorf("previous snapshot changed: %+v", c)
	}

	// A restart from the same cache directory serves the local copy at the
	// same version, so its first check fetches the manifest alone.
	e.srv.reset()
	again := started(t, e.srv.URL, cache, false, &logBuf{})
	if got := e.srv.requests(); !maps.Equal(got, map[string]int{cardset.ManifestKey: 1}) {
		t.Errorf("restart requested %v, want the manifest alone", got)
	}
	if again.Current().Version() != cur.Version() {
		t.Errorf("restart version %q, want %q", again.Current().Version(), cur.Version())
	}
	wantHP(t, again, "svb-001", 80)
}

// A loader that loaded once serves the same definitions from its local
// copy when started again with no network. (Criterion 3.)
func TestLoaderServesLocalCopyWithoutNetwork(t *testing.T) {
	e := newEnv(t, mon("sva", "SVA", "Pikachu", 60, "Search your deck."), mon("svb", "SVB", "Eevee", 50, "B."))
	cache := t.TempDir()
	first := started(t, e.srv.URL, cache, false, &logBuf{})
	version := first.Current().Version()
	if version != e.version() {
		t.Fatalf("first load version %q, want %q", version, e.version())
	}
	ref, err := first.Current().Card(context.Background(), "sva-001")
	if err != nil {
		t.Fatal(err)
	}

	closed := serve(t, e.pub)
	closedURL := closed.URL
	closed.Close()

	for name, tc := range map[string]struct {
		base    string
		offline bool
	}{
		"offline":           {e.srv.URL, true},
		"server stopped":    {closedURL, false},
		"directory missing": {filepath.Join(t.TempDir(), "nope"), false},
	} {
		t.Run(name, func(t *testing.T) {
			logs := &logBuf{}
			l := started(t, tc.base, cache, tc.offline, logs)
			cur := l.Current()
			if cur.Version() != version {
				t.Errorf("version %q, want %q", cur.Version(), version)
			}
			c, err := cur.Card(context.Background(), "sva-001")
			if err != nil {
				t.Fatal(err)
			}
			if c.Name != ref.Name || c.HP != 60 || len(c.Attacks) != 1 || c.Attacks[0].Name != "Tackle" {
				t.Errorf("card from the local copy %+v, want %+v", c, ref)
			}
			if c := cur.Cached("sva-002"); c == nil || c.Effect != "Search your deck." {
				t.Errorf("trainer from the local copy: %+v", c)
			}
			if st := l.Status(); st.Sets != 2 || st.Prints != 4 {
				t.Errorf("Status = %+v", st)
			}
			if logs.count("card sets: loaded 2 sets (version "+version[:12]+") from local copy") != 1 {
				t.Errorf("no local-copy start line:\n%s", logs)
			}
			if !tc.offline && l.Status().LastError == "" {
				t.Error("failed first check not reported")
			}
		})
	}
}

// A corrupt download, an unknown manifest format or unreachable storage
// leaves the current catalogue (and the local copy) unchanged and is
// reported: Check errors, Status has the error, repeated failures log
// once, and one line says when it works again. (Criterion 4.)
func TestLoaderFailureKeepsCatalogue(t *testing.T) {
	type breakage struct {
		breakIt func(t *testing.T, e *env, l *Loader) // after the change is published
		fix     func(t *testing.T, e *env, l *Loader)
	}
	var savedObject, savedManifest []byte
	var objPath string
	cases := map[string]breakage{
		"corrupt object": {
			func(t *testing.T, e *env, l *Loader) {
				objPath = filepath.Join(e.pub, e.objectPath("sva"))
				savedObject = readFile(t, objPath)
				b := slices.Clone(savedObject)
				for i := len(b) / 2; i < len(b); i++ {
					b[i] ^= 0xff
				}
				writeFile(t, objPath, b)
			},
			func(t *testing.T, e *env, l *Loader) { writeFile(t, objPath, savedObject) },
		},
		"object hash mismatch": {
			func(t *testing.T, e *env, l *Loader) {
				objPath = filepath.Join(e.pub, e.objectPath("sva"))
				savedObject = readFile(t, objPath)
				other, err := cardset.Marshal(mon("sva", "SVA", "Pikachu", 999, "Forged."))
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, objPath, gz(t, other))
			},
			func(t *testing.T, e *env, l *Loader) { writeFile(t, objPath, savedObject) },
		},
		"manifest format 2": {
			func(t *testing.T, e *env, l *Loader) {
				savedManifest = e.manifestBytes()
				b := strings.Replace(string(savedManifest), `"format": 1`, `"format": 2`, 1)
				if b == string(savedManifest) {
					t.Fatal("manifest has no top-level format to change")
				}
				writeFile(t, filepath.Join(e.pub, cardset.ManifestKey), []byte(b))
			},
			func(t *testing.T, e *env, l *Loader) {
				writeFile(t, filepath.Join(e.pub, cardset.ManifestKey), savedManifest)
			},
		},
		"server error": {
			func(t *testing.T, e *env, l *Loader) { e.srv.fail.Store(true) },
			func(t *testing.T, e *env, l *Loader) { e.srv.fail.Store(false) },
		},
		"server stopped": {
			func(t *testing.T, e *env, l *Loader) { e.srv.Close() },
			func(t *testing.T, e *env, l *Loader) { l.Base = serve(t, e.pub).URL },
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, mon("sva", "SVA", "Pikachu", 60, "Search your deck."))
			cache := t.TempDir()
			logs := &logBuf{}
			l := started(t, e.srv.URL, cache, false, logs)
			v1 := l.Current().Version()
			wantHP(t, l, "sva-001", 60)

			e.publish(mon("sva", "SVA", "Pikachu", 90, "Search your deck."))
			v2 := e.version()
			tc.breakIt(t, e, l)

			for i := range 3 {
				res, err := l.Check(context.Background())
				if err == nil {
					t.Fatalf("check %d succeeded: %+v", i+1, res)
				}
				if got := l.Current().Version(); got != v1 {
					t.Errorf("check %d: version %q, want %q", i+1, got, v1)
				}
				wantHP(t, l, "sva-001", 60)
				st := l.Status()
				if st.LastError == "" || st.LastCheck.IsZero() || st.Version != v1 {
					t.Errorf("check %d: Status = %+v", i+1, st)
				}
			}
			if n := logs.count("card sets: check failed"); n != 1 {
				t.Errorf("failure logged %d times, want once:\n%s", n, logs)
			}
			if logs.count("keeping version "+v1[:12]) != 1 {
				t.Errorf("failure line does not name the kept version:\n%s", logs)
			}

			// The local copy is untouched too.
			if got := started(t, e.srv.URL, cache, true, &logBuf{}).Current(); got.Version() != v1 || got.Cached("sva-001") == nil || got.Cached("sva-001").HP != 60 {
				t.Errorf("local copy changed by a failed check: version %q", got.Version())
			}

			tc.fix(t, e, l)
			res, err := l.Check(context.Background())
			if err != nil {
				t.Fatalf("check after the fix: %v", err)
			}
			if res.Version != v2 || !slices.Equal(res.Changed, []string{"sva"}) {
				t.Errorf("check after the fix = %+v, want version %s", res, v2)
			}
			wantHP(t, l, "sva-001", 90)
			if st := l.Status(); st.LastError != "" {
				t.Errorf("LastError after recovery %q", st.LastError)
			}
			if _, err := l.Check(context.Background()); err != nil {
				t.Fatal(err)
			}
			if n := logs.count("card sets: published sets reachable again"); n != 1 {
				t.Errorf("recovery logged %d times, want once:\n%s", n, logs)
			}
			if n := logs.count("card sets: check failed"); n != 1 {
				t.Errorf("failure logged %d times after recovery, want once:\n%s", n, logs)
			}
		})
	}
}

// writeLegacy writes the files the on-demand card cache left in the cache
// directory, with a conflicting definition of sva-001.
func writeLegacy(t *testing.T, cache string) {
	t.Helper()
	card := `{"id":"sva-001","localId":"001","name":"Raichu","category":"Pokemon","hp":999,"set":{"id":"sva","name":"Legacy Set"}}`
	writeFile(t, filepath.Join(cache, "cards", "sva-001.json"), []byte(card))
	writeFile(t, filepath.Join(cache, "sets.json"), []byte(`[{"id":"sva","name":"Legacy Set","abbreviation":"SVA"}]`))
	writeFile(t, filepath.Join(cache, "sets", "sva.json"), []byte(`{"id":"sva","name":"Legacy Set","cards":[{"id":"sva-001","localId":"001","name":"Raichu"}]}`))
	writeFile(t, filepath.Join(cache, "set-index.json"), []byte(`{"SVA":"sva"}`))
}

// Card files the on-demand approach left in the cache directory are never
// used: with no local copy the card is unknown, and with one the local
// copy's definition wins. (Criterion 5.)
func TestLoaderIgnoresLegacyCache(t *testing.T) {
	t.Run("no local copy", func(t *testing.T) {
		cache := t.TempDir()
		writeLegacy(t, cache)
		logs := &logBuf{}
		l := started(t, "", cache, true, logs)
		wantEmpty(t, l)
		if logs.count("card sets: none loaded") != 1 {
			t.Errorf("no empty-catalogue line:\n%s", logs)
		}
	})
	t.Run("local copy", func(t *testing.T) {
		e := newEnv(t, mon("sva", "SVA", "Pikachu", 60, "Search your deck."))
		cache := t.TempDir()
		writeLegacy(t, cache)
		started(t, e.srv.URL, cache, false, &logBuf{})
		l := started(t, "", cache, true, &logBuf{})
		c, err := l.Current().Card(context.Background(), "sva-001")
		if err != nil {
			t.Fatal(err)
		}
		if c.Name != "Pikachu" || c.HP != 60 || c.Set.Name != "Pikachu Set" {
			t.Errorf("sva-001 = %+v, want the local copy's Pikachu", c)
		}
	})
}

// With no reachable storage and no local copy the catalogue is empty.
// (Criterion 6.)
func TestLoaderEmptyWithoutStorageOrLocalCopy(t *testing.T) {
	closed := serve(t, t.TempDir())
	closedURL := closed.URL
	closed.Close()
	failing := serve(t, t.TempDir())
	failing.fail.Store(true)
	for name, base := range map[string]string{
		"no address":        "",
		"server stopped":    closedURL,
		"server error":      failing.URL,
		"directory missing": filepath.Join(t.TempDir(), "nope"),
		"empty directory":   t.TempDir(),
	} {
		t.Run(name, func(t *testing.T) {
			logs := &logBuf{}
			l := started(t, base, t.TempDir(), false, logs)
			wantEmpty(t, l)
			if l.Status().LastError == "" {
				t.Error("failed check not reported")
			}
			if _, err := l.Check(context.Background()); err == nil {
				t.Error("Check succeeded")
			}
			if logs.count("card sets: none loaded; cards will show as unknown") != 1 {
				t.Errorf("no empty-catalogue line:\n%s", logs)
			}
		})
	}
	t.Run("no cache directory either", func(t *testing.T) {
		l := started(t, closedURL, "", false, &logBuf{})
		wantEmpty(t, l)
	})
}

// localSetFiles lists the set objects in the local copy.
func localSetFiles(t *testing.T, cache string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(cache, "cardsets", "sets", "*", "*.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// A local copy whose manifest was never written (an interrupted write)
// gives an empty snapshot, never a partial one.
func TestLoaderInterruptedLocalWriteIsEmpty(t *testing.T) {
	e := newEnv(t, mon("sva", "SVA", "Pikachu", 60, "A."), mon("svb", "SVB", "Eevee", 50, "B."))
	cache := t.TempDir()
	started(t, e.srv.URL, cache, false, &logBuf{})
	if n := len(localSetFiles(t, cache)); n != 2 {
		t.Fatalf("local copy holds %d set files, want 2", n)
	}
	if err := os.Remove(filepath.Join(cache, "cardsets", cardset.ManifestKey)); err != nil {
		t.Fatal(err)
	}
	l := started(t, "", cache, true, &logBuf{})
	wantEmpty(t, l)
	if l.Current().Cached("svb-001") != nil {
		t.Error("svb served from a local copy without a manifest")
	}
}

// A damaged or missing local set file makes the whole local copy unused;
// online, the loader downloads everything again and repairs the copy.
func TestLoaderDamagedLocalSetUnusedAsAWhole(t *testing.T) {
	for name, damage := range map[string]func(t *testing.T, p string){
		"corrupt": func(t *testing.T, p string) {
			b := readFile(t, p)
			for i := len(b) / 2; i < len(b); i++ {
				b[i] ^= 0xff
			}
			writeFile(t, p, b)
		},
		"truncated": func(t *testing.T, p string) { writeFile(t, p, readFile(t, p)[:10]) },
		"missing": func(t *testing.T, p string) {
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, mon("sva", "SVA", "Pikachu", 60, "A."), mon("svb", "SVB", "Eevee", 50, "B."))
			cache := t.TempDir()
			started(t, e.srv.URL, cache, false, &logBuf{})
			damage(t, filepath.Join(cache, "cardsets", filepath.FromSlash(e.objectPath("svb"))))

			logs := &logBuf{}
			l := started(t, "", cache, true, logs)
			wantEmpty(t, l)
			if l.Current().Cached("svb-001") != nil {
				t.Error("partial snapshot: svb served")
			}
			if logs.count("card sets: local copy not used: set svb") != 1 {
				t.Errorf("unused local copy not logged:\n%s", logs)
			}

			// Online, the damaged set is downloaded again and the copy works.
			e.srv.reset()
			online := started(t, e.srv.URL, cache, false, &logBuf{})
			wantHP(t, online, "sva-001", 60)
			wantHP(t, online, "svb-001", 50)
			if got := e.srv.requests(); got[e.objectPath("svb")] != 1 || got[e.objectPath("sva")] != 0 {
				t.Errorf("repair requested %v, want svb's object alone (sva's local copy verifies)", got)
			}
			again := started(t, "", cache, true, &logBuf{})
			wantHP(t, again, "svb-001", 50)
		})
	}
}

// Concurrent checks run one after another: exactly one picks up the
// publish and downloads the changed set once, while lookups keep reading
// a whole snapshot. Meaningful under go test -race.
func TestLoaderConcurrentChecks(t *testing.T) {
	e := newEnv(t, mon("sva", "SVA", "Pikachu", 60, "A."), mon("svb", "SVB", "Eevee", 50, "B."))
	logs := &logBuf{}
	l := started(t, e.srv.URL, t.TempDir(), false, logs)
	e.publish(mon("sva", "SVA", "Pikachu", 90, "A."))
	want := e.version()
	e.srv.reset()
	before := logs.count("card sets: picked up version")

	stop := make(chan struct{})
	var readers sync.WaitGroup
	var bad atomic.Int32
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				cur := l.Current()
				a, b := cur.Cached("sva-001"), cur.Cached("svb-001")
				if a == nil || b == nil || (a.HP != 60 && a.HP != 90) {
					bad.Add(1)
				}
				_ = l.Status()
			}
		})
	}

	const n = 8
	results := make([]CheckResult, n)
	errs := make([]error, n)
	var checks sync.WaitGroup
	for i := range n {
		checks.Go(func() { results[i], errs[i] = l.Check(context.Background()) })
	}
	checks.Wait()
	close(stop)
	readers.Wait()

	if bad.Load() > 0 {
		t.Errorf("%d lookups saw a partial or unknown snapshot", bad.Load())
	}
	changed := 0
	for i := range n {
		if errs[i] != nil {
			t.Errorf("check %d: %v", i, errs[i])
			continue
		}
		if results[i].Version != want {
			t.Errorf("check %d version %q, want %q", i, results[i].Version, want)
		}
		if len(results[i].Changed) > 0 {
			changed++
			if !slices.Equal(results[i].Changed, []string{"sva"}) {
				t.Errorf("check %d changed %v", i, results[i].Changed)
			}
		}
	}
	if changed != 1 {
		t.Errorf("%d checks picked up the publish, want 1", changed)
	}
	if got := e.srv.requests(); !maps.Equal(got, map[string]int{cardset.ManifestKey: n, e.objectPath("sva"): 1}) {
		t.Errorf("requests %v, want the manifest %d times and sva's object once", got, n)
	}
	if c := logs.count("card sets: picked up version") - before; c != 1 {
		t.Errorf("pick-up logged %d times:\n%s", c, logs)
	}
	wantHP(t, l, "sva-001", 90)
}
