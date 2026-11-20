package cards

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// fakeImageHost stands in for TCGdex's asset host: it serves any path
// ending in low.webp or high.webp with a body distinct to that path,
// counts requests per path, and can be made to fail or hang.
type fakeImageHost struct {
	*httptest.Server

	mu      sync.Mutex
	count   map[string]int
	version string                // part of every body, so new art can be told apart
	status  func(path string) int // non-zero: answer path with this status
	block   chan struct{}         // non-nil: hold requests until closed or the client gives up
}

func newFakeImageHost(t *testing.T) *fakeImageHost {
	t.Helper()
	h := &fakeImageHost{count: map[string]int{}}
	h.Server = httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(h.Close)
	return h
}

func (h *fakeImageHost) serve(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.count[r.URL.Path]++
	status, block, version := 0, h.block, h.version
	if h.status != nil {
		status = h.status(r.URL.Path)
	}
	h.mu.Unlock()

	if block != nil {
		select {
		case <-block:
		case <-r.Context().Done():
			return
		}
	}
	if status != 0 {
		http.Error(w, http.StatusText(status), status)
		return
	}
	if !strings.HasSuffix(r.URL.Path, "/low.webp") && !strings.HasSuffix(r.URL.Path, "/high.webp") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Write(imageBody(r.URL.Path, version))
}

// imageBody is what the host serves for path at version.
func imageBody(path, version string) []byte {
	return []byte("RIFF....WEBP " + version + path)
}

// hits is how many requests path has had.
func (h *fakeImageHost) hits(path string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.count[path]
}

// totalHits is how many requests the host has had.
func (h *fakeImageHost) totalHits() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, c := range h.count {
		n += c
	}
	return n
}

func (h *fakeImageHost) setStatus(f func(path string) int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.status = f
}

func (h *fakeImageHost) setVersion(v string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.version = v
}

// hold makes requests hang; the returned func releases them.
func (h *fakeImageHost) hold() (release func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan struct{})
	h.block = ch
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			close(ch)
			if h.block == ch {
				h.block = nil
			}
		})
	}
}

type imageFixture struct {
	srv   *spacestest.Server
	host  *fakeImageHost
	ic    *ImageCache
	clock *fakeClock
	log   *logBuffer
}

// newImageFixture builds an image cache over a stand-in Spaces and a fake
// image host, with a fake clock, a 200ms shared timeout and captured logs.
// It does not start Run.
func newImageFixture(t *testing.T) *imageFixture {
	t.Helper()
	f := &imageFixture{srv: spacestest.New(t), host: newFakeImageHost(t), clock: &fakeClock{t: testEpoch}, log: &logBuffer{}}
	f.ic = f.newCache(t)
	return f
}

// newCache builds an image cache over the fixture's Spaces, image host,
// clock and log, as a process starting up would.
func (f *imageFixture) newCache(t *testing.T) *ImageCache {
	t.Helper()
	ic := NewImageCache(f.srv.Client(t), f.srv.Config().PublicBase())
	ic.Source = f.host.URL
	ic.Now = f.clock.Now
	ic.logf = f.log.logf
	ic.timeout = 200 * time.Millisecond
	return ic
}

// start starts the cache's Run; stop cancels it and waits for it to end.
// It is also called when the test ends.
func (f *imageFixture) start(t *testing.T) (stop func()) {
	t.Helper()
	ic := f.ic
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		ic.Run(ctx)
		close(done)
	}()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
	t.Cleanup(stop)
	return stop
}

// run starts the cache's Run, stopped when the test ends.
func (f *imageFixture) run(t *testing.T) {
	t.Helper()
	f.start(t)
}

// waitReady waits for Run's start-up listing to end, failing the test if
// that takes longer than d.
func (f *imageFixture) waitReady(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case <-f.ic.ready:
	case <-time.After(d):
		t.Fatalf("start-up listing still running after %v", d)
	}
}

// lists is how many listing pages of the image prefix the store has had.
func (f *imageFixture) lists() int { return f.srv.Requests("LIST", ImagePrefix) }

// puts is how many PUTs the store has had for card n's image files.
func (f *imageFixture) puts(n int) int {
	total := 0
	for _, file := range ImageFiles {
		total += f.srv.Requests("PUT", "images/"+rel(n)+"/"+file)
	}
	return total
}

// settle waits for queued copies to finish, failing the test if that
// takes longer than d.
func (f *imageFixture) settle(t *testing.T, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		f.ic.settle()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("copies still queued after %v", d)
	}
}

// src is the source address of card n's image.
func (f *imageFixture) src(n int) string {
	return fmt.Sprintf("%s/en/sv/sv1/%d", f.host.URL, n)
}

// rel is card n's image path below the source.
func rel(n int) string { return fmt.Sprintf("en/sv/sv1/%d", n) }

// image calls Image, failing the test if it takes longer than 50ms.
func (f *imageFixture) image(t *testing.T, src string) (string, string) {
	t.Helper()
	start := time.Now()
	img, fb, _ := f.ic.Image(src)
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("Image(%q) took %v, want under 50ms", src, d)
	}
	return img, fb
}

// wantSource checks that Image answers src itself with no fallback.
func (f *imageFixture) wantSource(t *testing.T, src string) {
	t.Helper()
	if img, fb := f.image(t, src); img != src || fb != "" {
		t.Errorf("Image(%q) = (%q, %q), want (%q, \"\")", src, img, fb, src)
	}
}

// wantCDN checks that Image answers card n's CDN address with the source
// as fallback.
func (f *imageFixture) wantCDN(t *testing.T, n int) {
	t.Helper()
	want := f.srv.Config().PublicBase() + "/images/" + rel(n)
	if img, fb := f.image(t, f.src(n)); img != want || fb != f.src(n) {
		t.Errorf("Image(%q) = (%q, %q), want (%q, %q)", f.src(n), img, fb, want, f.src(n))
	}
}

// wantCopied checks that both files of card n's image are in the store as
// public byte-identical copies with the image headers, at version.
func (f *imageFixture) wantCopied(t *testing.T, n int, version string) {
	t.Helper()
	for _, file := range []string{"low.webp", "high.webp"} {
		f.wantCopiedFile(t, n, file, version)
	}
}

// wantCopiedFile checks that one file of card n's image is in the store as
// a public byte-identical copy with the image headers, at version.
func (f *imageFixture) wantCopiedFile(t *testing.T, n int, file, version string) {
	t.Helper()
	key := "images/" + rel(n) + "/" + file
	want := imageBody("/"+rel(n)+"/"+file, version)
	b, ok := f.srv.Object(key)
	if !ok {
		t.Errorf("store has no %s", key)
		return
	}
	if !bytes.Equal(b, want) {
		t.Errorf("%s = %q, want %q", key, b, want)
	}
	if !f.srv.Public(key) {
		t.Errorf("%s is not public", key)
	}
	h := f.srv.Header(key)
	if ct := h.Get("Content-Type"); ct != "image/webp" {
		t.Errorf("%s Content-Type = %q, want image/webp", key, ct)
	}
	if cc := h.Get("Cache-Control"); cc != "public, max-age=604800" {
		t.Errorf("%s Cache-Control = %q, want public, max-age=604800", key, cc)
	}
}

// wantSeeded checks that the store holds exactly body under key, as seeded:
// never written over by the cache.
func (f *imageFixture) wantSeeded(t *testing.T, key string, body []byte) {
	t.Helper()
	if b, ok := f.srv.Object(key); !ok || !bytes.Equal(b, body) {
		t.Errorf("%s = %q (present %v), want it left as seeded %q", key, b, ok, body)
	}
	if n := f.srv.Requests("PUT", key); n != 0 {
		t.Errorf("PUTs of %s = %d, want 0", key, n)
	}
}

// imageLists is how many listings of card n's own image prefix the store
// has had: how often the cache asked the bucket about that image alone.
func (f *imageFixture) imageLists(n int) int {
	return f.srv.Requests("LIST", ImagePrefix+rel(n)+"/")
}

// copiedCount is how many images the cache has counted as copied.
func (f *imageFixture) copiedCount() int {
	f.ic.mu.Lock()
	defer f.ic.mu.Unlock()
	return f.ic.copied
}

func TestImageConstants(t *testing.T) {
	if ImagePrefix != "images/" {
		t.Errorf("ImagePrefix = %q", ImagePrefix)
	}
	if ImageCacheControl != "public, max-age=604800" {
		t.Errorf("ImageCacheControl = %q", ImageCacheControl)
	}
	if strings.Join(ImageFiles, ",") != "low.webp,high.webp" {
		t.Errorf("ImageFiles = %q", ImageFiles)
	}
}

func TestImageFirstRequestAnswersSourceThenCopiesBothSizes(t *testing.T) {
	f := newImageFixture(t)
	f.run(t)

	start := time.Now()
	f.wantSource(t, f.src(1))
	f.settle(t, 60*time.Second)
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("copy took %v, want well within 60s", d)
	}
	f.wantCopied(t, 1, "")
	for _, file := range []string{"low.webp", "high.webp"} {
		if n := f.host.hits("/en/sv/sv1/1/" + file); n != 1 {
			t.Errorf("source hits for %s = %d, want 1", file, n)
		}
		if n := f.srv.Requests("PUT", "images/en/sv/sv1/1/"+file); n != 1 {
			t.Errorf("PUTs of %s = %d, want 1", file, n)
		}
	}
}

func TestImageCopiedAnswersCDNWithSourceFallbackAndNoSourceRequest(t *testing.T) {
	f := newImageFixture(t)
	f.run(t)
	f.image(t, f.src(1))
	f.settle(t, 10*time.Second)

	f.wantCDN(t, 1)

	// The CDN address serves the copies to an anonymous browser.
	public := f.srv.Config().PublicBase() + "/images/" + rel(1)
	for _, file := range []string{"low.webp", "high.webp"} {
		resp, err := http.Get(public + "/" + file)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", file, resp.StatusCode)
		}
		if want := imageBody("/"+rel(1)+"/"+file, ""); !bytes.Equal(b, want) {
			t.Errorf("GET %s = %q, want %q", file, b, want)
		}
	}

	hits, puts := f.host.totalHits(), f.srv.Total()
	for range 3 {
		f.wantCDN(t, 1)
	}
	f.settle(t, 10*time.Second)
	if n := f.host.totalHits(); n != hits {
		t.Errorf("source hits after later requests = %d, want still %d", n, hits)
	}
	if n := f.srv.Total(); n != puts {
		t.Errorf("store requests after later requests = %d, want still %d", n, puts)
	}
}

func TestImageAddressesOutsideSourceAreReturnedUnchangedAndNeverCopied(t *testing.T) {
	f := newImageFixture(t)
	f.run(t)
	base := f.host.URL
	for _, src := range []string{
		"https://example.com/en/sv/sv1/1",
		"https://assets.tcgdex.net/en/sv/sv1/1",
		base,
		base + "/",
		base + "evil/en/sv/sv1/1",
		base + "/en/sv/../sv1/1",
		base + "/en/sv/./sv1/1",
		base + "/en/sv/sv1/1/..",
		base + "/en/sv/sv1/1?x",
		base + "/en/sv/sv1/1?",
		base + "/en/sv/sv1/1#f",
		base + "/en//sv1/1",
		base + "/en/sv/sv1/1/",
		"not a url",
		"/en/sv/sv1/1",
	} {
		for range 2 {
			if img, fb := f.image(t, src); img != src || fb != "" {
				t.Errorf("Image(%q) = (%q, %q), want it unchanged with no fallback", src, img, fb)
			}
		}
	}
	f.waitReady(t, 5*time.Second)
	f.settle(t, 10*time.Second)
	// The start-up listing is the only store request.
	if n := f.lists(); n != 1 {
		t.Errorf("start-up listings = %d, want 1", n)
	}
	if n := f.srv.Total(); n != 1 {
		t.Errorf("store requests = %d, want only the start-up listing", n)
	}
	if n := f.host.totalHits(); n != 0 {
		t.Errorf("source hits = %d, want 0", n)
	}

	if img, fb := f.image(t, ""); img != "" || fb != "" {
		t.Errorf(`Image("") = (%q, %q), want ("", "")`, img, fb)
	}
}

func TestImageCopiesAreNeverRewrittenAndLateArtGetsItsOwnCopy(t *testing.T) {
	f := newImageFixture(t)
	f.run(t)
	f.image(t, f.src(1))
	f.settle(t, 10*time.Second)
	f.wantCopied(t, 1, "")

	// The source changes its bytes; the copy is neither refetched nor
	// rewritten.
	f.host.setVersion("v2 ")
	for range 3 {
		f.wantCDN(t, 1)
	}

	// Art appearing later for another card gets its own copy.
	f.wantSource(t, f.src(2))
	f.settle(t, 10*time.Second)
	f.wantCopied(t, 2, "v2 ")
	f.wantCDN(t, 2)
	f.wantCDN(t, 1)
	f.settle(t, 10*time.Second)

	f.wantCopied(t, 1, "")
	for _, file := range []string{"low.webp", "high.webp"} {
		key := "images/" + rel(1) + "/" + file
		if n := f.srv.Requests("PUT", key); n != 1 {
			t.Errorf("PUTs of %s = %d, want 1", key, n)
		}
		if n := f.srv.Requests("DELETE", key); n != 0 {
			t.Errorf("DELETEs of %s = %d, want 0", key, n)
		}
		if n := f.host.hits("/" + rel(1) + "/" + file); n != 1 {
			t.Errorf("source hits for %s = %d, want 1", rel(1)+"/"+file, n)
		}
	}
}

func TestImageAbsorbsSharedOutages(t *testing.T) {
	const (
		down = "card images: shared store unavailable"
		up   = "card images: shared store reachable again"
	)
	for name, mode := range outageModes {
		for _, atStart := range []bool{false, true} {
			sub := name + "/after start-up"
			if atStart {
				sub = name + "/at start-up"
			}
			t.Run(sub, func(t *testing.T) {
				f := newImageFixture(t)
				var heal func()
				if atStart {
					// The outage is under way when Run starts: the start-up
					// listing is its first attempt.
					heal = mode(f.srv)
					f.run(t)
				} else {
					// The listing succeeds, then the outage starts.
					f.run(t)
					f.waitReady(t, 5*time.Second)
					heal = mode(f.srv)
				}
				if heal == nil {
					// Unreachable for good: recovery is a reachable store.
					heal = func() {
						srv := spacestest.New(t)
						f.ic.Spaces = srv.Client(t)
						f.ic.Public = srv.Config().PublicBase()
						f.srv = srv
					}
				} else {
					defer heal()
				}
				// The closed server counts nothing, so copy attempts are told
				// by source fetches: a copy fetches both files before its PUTs.
				counted := name != "unreachable"
				attempts := func() int { return f.host.totalHits() / 2 }

				// The outage's first attempt trips the breaker: the listing
				// when the outage was there at start-up, otherwise the first
				// copy's first PUT.
				wantAttempts, wantStore := 1, 2 // copies; listing + PUTs
				if atStart {
					f.waitReady(t, 5*time.Second)
					wantAttempts, wantStore = 0, 1
				}
				f.wantSource(t, f.src(1))
				f.wantSource(t, f.src(1)) // while the copy is under way
				f.settle(t, 5*time.Second)
				f.wantSource(t, f.src(1))
				if n := attempts(); n != wantAttempts {
					t.Errorf("copy attempts = %d, want %d", n, wantAttempts)
				}
				if counted {
					if n := f.lists(); n != 1 {
						t.Errorf("start-up listings = %d, want 1", n)
					}
					if n := f.puts(1); n != wantAttempts {
						t.Errorf("PUTs = %d, want %d", n, wantAttempts)
					}
					if n := f.srv.Total(); n != wantStore {
						t.Errorf("store requests = %d, want %d", n, wantStore)
					}
				}
				if n := f.log.count(down); n != 1 {
					t.Errorf("unavailable logged %d times, want 1: %q", n, f.log.lines)
				}

				// Inside the window: nothing is tried.
				f.wantSource(t, f.src(2))
				f.settle(t, 5*time.Second)
				f.clock.Advance(SharedRetryAfter - time.Second)
				f.wantSource(t, f.src(3))
				f.settle(t, 5*time.Second)
				if n := attempts(); n != wantAttempts {
					t.Errorf("copy attempts inside the window = %d, want still %d", n, wantAttempts)
				}
				if counted && f.srv.Total() != wantStore {
					t.Errorf("store requests inside the window = %d, want still %d", f.srv.Total(), wantStore)
				}

				// After the window: one probe, which fails without logging
				// again. When the start-up listing completed, the probe is a
				// copy's PUT. When it failed, the cache must first ask the
				// bucket whether the image is already there, so the probe is
				// the listing of the image's own prefix; with the bucket
				// unanswered nothing is fetched or put.
				f.clock.Advance(time.Second)
				f.wantSource(t, f.src(3))
				f.settle(t, 5*time.Second)
				f.wantSource(t, f.src(3))
				wantProbeLists, wantProbePuts, wantProbeAttempts := 0, 1, 1
				if atStart {
					wantProbeLists, wantProbePuts, wantProbeAttempts = 1, 0, 0
				}
				if n := attempts(); n != wantAttempts+wantProbeAttempts {
					t.Errorf("copy attempts after the window = %d, want %d", n, wantAttempts+wantProbeAttempts)
				}
				if counted {
					if n := f.imageLists(3); n != wantProbeLists {
						t.Errorf("listings of card 3's prefix after the window = %d, want %d", n, wantProbeLists)
					}
					if n := f.puts(3); n != wantProbePuts {
						t.Errorf("PUTs of card 3 after the window = %d, want %d", n, wantProbePuts)
					}
					if n := f.srv.Total(); n != wantStore+1 {
						t.Errorf("store requests after the window = %d, want %d", n, wantStore+1)
					}
				}
				if n := f.log.count(down); n != 1 {
					t.Errorf("unavailable logged %d times, want 1: %q", n, f.log.lines)
				}
				if n := f.log.count(up); n != 0 {
					t.Errorf("reachable logged %d times during the outage, want 0", n)
				}

				// Recovery: the next probe copies, logged once. With the
				// start-up listing failed, the copy first asks the bucket
				// about the image, finds nothing and puts both files.
				heal()
				lists1, puts1 := f.imageLists(1), f.puts(1) // f.srv may be new
				f.clock.Advance(SharedRetryAfter)
				f.wantSource(t, f.src(1))
				f.settle(t, 5*time.Second)
				f.wantCDN(t, 1)
				f.wantCopied(t, 1, "")
				wantImageLists := 0
				if atStart {
					wantImageLists = 1
				}
				if n := f.imageLists(1) - lists1; n != wantImageLists {
					t.Errorf("listings of card 1's prefix on recovery = %d, want %d", n, wantImageLists)
				}
				if n := f.puts(1) - puts1; n != 2 {
					t.Errorf("PUTs of card 1 on recovery = %d, want 2", n)
				}
				f.wantSource(t, f.src(2))
				f.settle(t, 5*time.Second)
				f.wantCDN(t, 2)
				if n := f.log.count(down); n != 1 {
					t.Errorf("unavailable logged %d times, want 1: %q", n, f.log.lines)
				}
				if n := f.log.count(up); n != 1 {
					t.Errorf("reachable logged %d times, want 1: %q", n, f.log.lines)
				}
			})
		}
	}
}

func TestImageSourceFailureWaitsRetryAfterThenCopies(t *testing.T) {
	for _, tc := range []struct {
		name string
		// fail breaks the source and returns how to heal it.
		fail      func(f *imageFixture) (heal func())
		low, high int // source hits per file for the failed attempt
	}{
		{"not found", func(f *imageFixture) func() {
			f.host.setStatus(func(string) int { return http.StatusNotFound })
			return func() { f.host.setStatus(nil) }
		}, 1, 0},
		{"server error", func(f *imageFixture) func() {
			f.host.setStatus(func(string) int { return http.StatusInternalServerError })
			return func() { f.host.setStatus(nil) }
		}, 1, 0},
		{"second size missing", func(f *imageFixture) func() {
			f.host.setStatus(func(p string) int {
				if strings.HasSuffix(p, "/high.webp") {
					return http.StatusNotFound
				}
				return 0
			})
			return func() { f.host.setStatus(nil) }
		}, 1, 1},
		{"slow", func(f *imageFixture) func() {
			f.ic.HTTP = &http.Client{Timeout: 100 * time.Millisecond}
			return f.host.hold()
		}, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newImageFixture(t)
			heal := tc.fail(f)
			defer heal()
			f.run(t)
			low, high := "/"+rel(1)+"/low.webp", "/"+rel(1)+"/high.webp"
			wantHits := func(when string, l, h int) {
				t.Helper()
				if n := f.host.hits(low); n != l {
					t.Errorf("%s: low.webp hits = %d, want %d", when, n, l)
				}
				if n := f.host.hits(high); n != h {
					t.Errorf("%s: high.webp hits = %d, want %d", when, n, h)
				}
			}

			f.wantSource(t, f.src(1))
			f.settle(t, 5*time.Second)
			wantHits("failed attempt", tc.low, tc.high)
			// Copying waits for the start-up listing, so settling has seen
			// it; a copy whose source fails makes no store request.
			if n := f.lists(); n != 1 {
				t.Errorf("start-up listings = %d, want 1", n)
			}
			if n := f.srv.Total() - f.lists(); n != 0 {
				t.Errorf("store requests after a source failure = %d besides the listing, want 0", n)
			}

			f.wantSource(t, f.src(1))
			f.settle(t, 5*time.Second)
			f.clock.Advance(RetryAfter - time.Second)
			f.wantSource(t, f.src(1))
			f.settle(t, 5*time.Second)
			wantHits("inside RetryAfter", tc.low, tc.high)

			heal()
			f.clock.Advance(time.Second)
			f.wantSource(t, f.src(1))
			f.settle(t, 5*time.Second)
			wantHits("after RetryAfter", tc.low+1, tc.high+1)
			f.wantCopied(t, 1, "")
			f.wantCDN(t, 1)
			if n := f.log.count("shared store unavailable"); n != 0 {
				t.Errorf("a source failure was logged as a store outage: %q", f.log.lines)
			}
		})
	}
}

func TestImageFullQueueNeverBlocksAndRequeuesOnTheNextView(t *testing.T) {
	f := newImageFixture(t)
	const n = 300

	// Run is not started, so nothing drains the queue.
	start := time.Now()
	for i := range n {
		if img, fb, _ := f.ic.Image(f.src(i)); img != f.src(i) || fb != "" {
			t.Errorf("Image(%q) = (%q, %q), want the source address", f.src(i), img, fb)
		}
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("%d Image calls on a full queue took %v, want no wait", n, d)
	}
	for i := range n {
		f.wantSource(t, f.src(i))
	}
	f.ic.mu.Lock()
	pending := len(f.ic.pending)
	f.ic.mu.Unlock()
	if pending != imageQueueSize {
		t.Errorf("pending = %d, want the queue size %d", pending, imageQueueSize)
	}
	if hits := f.host.totalHits(); hits != 0 {
		t.Errorf("Image touched the source %d times, want 0", hits)
	}

	// Once copying runs, the queued images are copied, and the dropped ones
	// are queued again by their next view.
	f.run(t)
	f.settle(t, 30*time.Second)
	for i := range n {
		f.ic.Image(f.src(i))
	}
	f.settle(t, 30*time.Second)
	for i := range n {
		f.wantCDN(t, i)
	}
}

// TestImageCopiesSurviveARestart copies an image, stops the cache and
// starts a new one on the same bucket, as a deploy does: the new cache
// answers the CDN address from its start-up listing, never touches the
// card source for it, and never rewrites the copy, even after the source's
// bytes change. Art that appears later still gets its own copy.
func TestImageCopiesSurviveARestart(t *testing.T) {
	f := newImageFixture(t)
	stop := f.start(t)
	f.wantSource(t, f.src(1))
	f.settle(t, 10*time.Second)
	f.wantCopied(t, 1, "")
	stop()

	// The source's bytes change while the process restarts.
	f.host.setVersion("v2 ")
	hits := f.host.totalHits()
	f.srv.Reset()
	f.ic = f.newCache(t)
	f.run(t)
	f.waitReady(t, 5*time.Second)
	if n := f.log.count("card images: 1 copies found in the shared store"); n != 1 {
		t.Errorf("start-up listing logged %d times, want 1: %q", n, f.log.lines)
	}

	for range 3 {
		f.wantCDN(t, 1)
	}
	f.settle(t, 10*time.Second)
	if n := f.host.totalHits(); n != hits {
		t.Errorf("source hits after the restart = %d, want still %d", n, hits)
	}
	if n := f.puts(1); n != 0 {
		t.Errorf("PUTs of the copied image after the restart = %d, want 0", n)
	}
	if n := f.srv.Total(); n != 1 {
		t.Errorf("store requests after the restart = %d, want only the start-up listing", n)
	}
	f.wantCopied(t, 1, "")

	// Art appearing later gets its own copy; the old copy is left alone.
	f.wantSource(t, f.src(2))
	f.settle(t, 10*time.Second)
	f.wantCopied(t, 2, "v2 ")
	f.wantCDN(t, 2)
	f.wantCDN(t, 1)
	f.settle(t, 10*time.Second)
	f.wantCopied(t, 1, "")
	if n := f.puts(1); n != 0 {
		t.Errorf("PUTs of the copied image after new art = %d, want 0", n)
	}
	for _, file := range ImageFiles {
		key := "images/" + rel(1) + "/" + file
		if n := f.srv.Requests("DELETE", key); n != 0 {
			t.Errorf("DELETEs of %s = %d, want 0", key, n)
		}
	}
}

// TestImageStartUpListingNeedsBothFiles seeds the bucket with both files
// of one image and only low.webp of another: the first is served from the
// CDN without a copy; of the second, the file already in the bucket is left
// exactly as it is and only the missing one is fetched and put, after which
// it too is served from the CDN.
func TestImageStartUpListingNeedsBothFiles(t *testing.T) {
	f := newImageFixture(t)
	seeded := []byte("RIFF....WEBP seeded")
	for _, file := range ImageFiles {
		f.srv.SetObject("images/"+rel(1)+"/"+file, seeded)
	}
	f.srv.SetObject("images/"+rel(2)+"/low.webp", seeded)
	f.run(t)
	f.waitReady(t, 5*time.Second)
	if n := f.log.count("card images: 1 copies found in the shared store"); n != 1 {
		t.Errorf("start-up listing logged %d times, want 1: %q", n, f.log.lines)
	}

	f.wantCDN(t, 1)
	f.wantSource(t, f.src(2))
	f.settle(t, 10*time.Second)

	for _, file := range ImageFiles {
		if n := f.host.hits("/" + rel(1) + "/" + file); n != 0 {
			t.Errorf("source hits for the listed image's %s = %d, want 0", file, n)
		}
		f.wantSeeded(t, "images/"+rel(1)+"/"+file, seeded)
	}

	// The half copy: low.webp untouched and not fetched, high.webp fetched
	// once and put once.
	f.wantSeeded(t, "images/"+rel(2)+"/low.webp", seeded)
	if n := f.host.hits("/" + rel(2) + "/low.webp"); n != 0 {
		t.Errorf("source hits for the half copy's low.webp = %d, want 0", n)
	}
	if n := f.host.hits("/" + rel(2) + "/high.webp"); n != 1 {
		t.Errorf("source hits for the half copy's high.webp = %d, want 1", n)
	}
	if n := f.srv.Requests("PUT", "images/"+rel(2)+"/high.webp"); n != 1 {
		t.Errorf("PUTs of the half copy's high.webp = %d, want 1", n)
	}
	f.wantCopiedFile(t, 2, "high.webp", "")
	// The completed start-up listing answers whether a file is there; no
	// image is asked about on its own.
	for _, n := range []int{1, 2} {
		if got := f.imageLists(n); got != 0 {
			t.Errorf("listings of card %d's prefix = %d, want 0", n, got)
		}
	}
	if n := f.srv.Total(); n != 2 {
		t.Errorf("store requests = %d, want the start-up listing and one PUT", n)
	}
	f.wantCDN(t, 2)
}

// TestImageCopyFailingPartWayNeverRewritesTheFileItPut has a copy's first
// PUT (low.webp) succeed and its second (high.webp) fail, with the start-up
// listing completed. The store is held on the first PUT, made to fail, then
// released: the held PUT was let in before the failure began and succeeds,
// and the next one fails. After the retry window the next view fetches and
// puts only high.webp; low.webp is neither fetched nor put again, and the
// image then answers from the CDN.
func TestImageCopyFailingPartWayNeverRewritesTheFileItPut(t *testing.T) {
	const (
		down = "card images: shared store unavailable"
		up   = "card images: shared store reachable again"
	)
	f := newImageFixture(t)
	// Long enough that the held PUT is still waiting when it is released.
	f.ic.timeout = 30 * time.Second
	f.run(t)
	f.waitReady(t, 5*time.Second) // the listing succeeds

	low, high := "images/"+rel(1)+"/low.webp", "images/"+rel(1)+"/high.webp"
	release := f.srv.Hang()
	defer release()
	f.wantSource(t, f.src(1))
	waitFor(t, 5*time.Second, "the low.webp PUT held", func() bool {
		return f.srv.Requests("PUT", low) == 1
	})
	f.srv.SetFail(true)
	defer f.srv.SetFail(false)
	release()
	f.settle(t, 5*time.Second)

	if n := f.srv.Requests("PUT", low); n != 1 {
		t.Errorf("PUTs of low.webp = %d, want 1", n)
	}
	if n := f.srv.Requests("PUT", high); n != 1 {
		t.Errorf("PUTs of high.webp = %d, want 1 (failed)", n)
	}
	f.wantCopiedFile(t, 1, "low.webp", "")
	if _, ok := f.srv.Object(high); ok {
		t.Errorf("store has %s after its PUT failed", high)
	}
	if n := f.log.count(down); n != 1 {
		t.Errorf("unavailable logged %d times, want 1: %q", n, f.log.lines)
	}
	f.wantSource(t, f.src(1)) // half copied: not served from the CDN
	f.settle(t, 5*time.Second)

	f.srv.SetFail(false)
	f.clock.Advance(SharedRetryAfter)
	lowHits, highHits := f.host.hits("/"+rel(1)+"/low.webp"), f.host.hits("/"+rel(1)+"/high.webp")
	if lowHits != 1 || highHits != 1 {
		t.Errorf("source hits before the retry = low %d, high %d, want 1 each", lowHits, highHits)
	}
	f.wantSource(t, f.src(1))
	f.settle(t, 5*time.Second)

	if n := f.host.hits("/"+rel(1)+"/low.webp") - lowHits; n != 0 {
		t.Errorf("source hits for low.webp on the retry = %d, want 0", n)
	}
	if n := f.host.hits("/"+rel(1)+"/high.webp") - highHits; n != 1 {
		t.Errorf("source hits for high.webp on the retry = %d, want 1", n)
	}
	if n := f.srv.Requests("PUT", low); n != 1 {
		t.Errorf("PUTs of low.webp after the retry = %d, want still 1", n)
	}
	if n := f.srv.Requests("PUT", high); n != 2 {
		t.Errorf("PUTs of high.webp after the retry = %d, want 2 (one failed, one put)", n)
	}
	if n := f.imageLists(1); n != 0 {
		t.Errorf("listings of card 1's prefix = %d, want 0 (the start-up listing completed)", n)
	}
	f.wantCopied(t, 1, "")
	f.wantCDN(t, 1)
	if n := f.copiedCount(); n != 1 {
		t.Errorf("images counted as copied = %d, want 1", n)
	}
	if n := f.log.count(up); n != 1 {
		t.Errorf("reachable logged %d times, want 1: %q", n, f.log.lines)
	}
}

// TestImageFailedStartUpListingAsksTheBucketBeforeCopying makes the start-up
// listing fail, so the cache does not know what the bucket holds. Card 1 has
// both files in the bucket already, card 2 only low.webp, card 3 nothing.
// After the retry window each view asks the bucket about that image alone
// (one listing of its prefix) and puts only what is missing: card 1 is served
// from the CDN with no fetch and no PUT and its bytes untouched, card 2 gets
// only high.webp, and card 3 gets both files. A copy is never written over.
func TestImageFailedStartUpListingAsksTheBucketBeforeCopying(t *testing.T) {
	const (
		down = "card images: shared store unavailable"
		up   = "card images: shared store reachable again"
	)
	for _, name := range []string{"failing", "refused", "hanging"} {
		t.Run(name, func(t *testing.T) {
			f := newImageFixture(t)
			seeded := []byte("RIFF....WEBP seeded")
			for _, file := range ImageFiles {
				f.srv.SetObject("images/"+rel(1)+"/"+file, seeded)
			}
			f.srv.SetObject("images/"+rel(2)+"/low.webp", seeded)

			heal := outageModes[name](f.srv)
			defer heal()
			f.run(t)
			f.waitReady(t, 5*time.Second)
			heal()
			if n := f.lists(); n != 1 {
				t.Errorf("start-up listings = %d, want 1", n)
			}
			if n := f.log.count(down); n != 1 {
				t.Errorf("unavailable logged %d times, want 1: %q", n, f.log.lines)
			}
			if n := f.log.count("copies found in the shared store"); n != 0 {
				t.Errorf("a failed start-up listing was logged as found: %q", f.log.lines)
			}

			// Inside the window the bucket is not asked, so nothing is
			// copied: every image answers its source.
			for n := 1; n <= 3; n++ {
				f.wantSource(t, f.src(n))
			}
			f.settle(t, 5*time.Second)
			if n := f.srv.Total(); n != 1 {
				t.Errorf("store requests inside the window = %d, want only the start-up listing", n)
			}
			if n := f.host.totalHits(); n != 0 {
				t.Errorf("source hits inside the window = %d, want 0", n)
			}

			// After the window each image is asked about once.
			f.clock.Advance(SharedRetryAfter)
			for n := 1; n <= 3; n++ {
				f.wantSource(t, f.src(n))
			}
			f.settle(t, 5*time.Second)
			for n := 1; n <= 3; n++ {
				if got := f.imageLists(n); got != 1 {
					t.Errorf("listings of card %d's prefix = %d, want 1", n, got)
				}
			}
			if n := f.lists(); n != 1 {
				t.Errorf("start-up listings = %d, want still 1 (not retried)", n)
			}
			if n := f.log.count(up); n != 1 {
				t.Errorf("reachable logged %d times, want 1: %q", n, f.log.lines)
			}

			// Card 1, whole in the bucket: no fetch, no PUT, bytes untouched.
			for _, file := range ImageFiles {
				if n := f.host.hits("/" + rel(1) + "/" + file); n != 0 {
					t.Errorf("source hits for card 1's %s = %d, want 0", file, n)
				}
				f.wantSeeded(t, "images/"+rel(1)+"/"+file, seeded)
			}
			// Card 2, half there: only high.webp fetched and put.
			f.wantSeeded(t, "images/"+rel(2)+"/low.webp", seeded)
			if n := f.host.hits("/" + rel(2) + "/low.webp"); n != 0 {
				t.Errorf("source hits for card 2's low.webp = %d, want 0", n)
			}
			if n := f.host.hits("/" + rel(2) + "/high.webp"); n != 1 {
				t.Errorf("source hits for card 2's high.webp = %d, want 1", n)
			}
			if n := f.srv.Requests("PUT", "images/"+rel(2)+"/high.webp"); n != 1 {
				t.Errorf("PUTs of card 2's high.webp = %d, want 1", n)
			}
			f.wantCopiedFile(t, 2, "high.webp", "")
			// Card 3, absent: both files fetched and put.
			for _, file := range ImageFiles {
				if n := f.host.hits("/" + rel(3) + "/" + file); n != 1 {
					t.Errorf("source hits for card 3's %s = %d, want 1", file, n)
				}
			}
			if n := f.puts(3); n != 2 {
				t.Errorf("PUTs of card 3 = %d, want 2", n)
			}
			f.wantCopied(t, 3, "")

			// The start-up listing, three image listings, 1 + 2 PUTs.
			if n := f.srv.Total(); n != 1+3+3 {
				t.Errorf("store requests = %d, want %d", n, 1+3+3)
			}
			if n := f.host.totalHits(); n != 3 {
				t.Errorf("source hits = %d, want 3", n)
			}
			// An image found whole is not counted as copied.
			if n := f.copiedCount(); n != 2 {
				t.Errorf("images counted as copied = %d, want 2 (cards 2 and 3)", n)
			}

			// All three now answer from the CDN, with no further requests.
			total, hits := f.srv.Total(), f.host.totalHits()
			for range 2 {
				for n := 1; n <= 3; n++ {
					f.wantCDN(t, n)
				}
			}
			f.settle(t, 5*time.Second)
			if n := f.srv.Total(); n != total {
				t.Errorf("store requests after CDN answers = %d, want still %d", n, total)
			}
			if n := f.host.totalHits(); n != hits {
				t.Errorf("source hits after CDN answers = %d, want still %d", n, hits)
			}
			f.wantSeeded(t, "images/"+rel(1)+"/low.webp", seeded)
			f.wantSeeded(t, "images/"+rel(1)+"/high.webp", seeded)
			f.wantSeeded(t, "images/"+rel(2)+"/low.webp", seeded)
		})
	}
}

// TestImageOneSharedAttemptWithBothWorkersBusy has both workers reach the
// shared store at once while it is failing: only one PUT is tried in the
// retry window and the outage is logged once; after the window, with the
// store healed, the next views copy and the recovery is logged once.
func TestImageOneSharedAttemptWithBothWorkersBusy(t *testing.T) {
	const (
		down = "card images: shared store unavailable"
		up   = "card images: shared store reachable again"
	)
	for _, name := range []string{"failing", "refused", "hanging"} {
		t.Run(name, func(t *testing.T) {
			f := newImageFixture(t)
			f.run(t)
			f.waitReady(t, 5*time.Second) // the listing succeeds
			heal := outageModes[name](f.srv)
			defer heal()

			// Hold the source so both workers are mid-copy, then let both
			// go on to their PUTs together.
			release := f.host.hold()
			defer release()
			const n = 4
			for i := 1; i <= n; i++ {
				f.wantSource(t, f.src(i))
			}
			waitFor(t, 5*time.Second, "both workers fetching", func() bool {
				return f.host.hits("/"+rel(1)+"/low.webp") == 1 && f.host.hits("/"+rel(2)+"/low.webp") == 1
			})
			release()
			f.settle(t, 5*time.Second)

			puts := 0
			for i := 1; i <= n; i++ {
				puts += f.puts(i)
			}
			if puts != 1 {
				t.Errorf("PUTs in the window = %d, want 1", puts)
			}
			if got := f.srv.Total(); got != 1+1 {
				t.Errorf("store requests = %d, want the listing and one PUT", got)
			}
			if c := f.log.count(down); c != 1 {
				t.Errorf("unavailable logged %d times, want 1: %q", c, f.log.lines)
			}
			for i := 1; i <= n; i++ {
				f.wantSource(t, f.src(i))
			}
			f.settle(t, 5*time.Second)
			if got := f.srv.Total(); got != 1+1 {
				t.Errorf("store requests inside the window = %d, want still 2", got)
			}

			heal()
			f.clock.Advance(SharedRetryAfter)
			for i := 1; i <= n; i++ {
				f.wantSource(t, f.src(i))
			}
			f.settle(t, 5*time.Second)
			for i := 1; i <= n; i++ {
				f.wantCDN(t, i)
				f.wantCopied(t, i, "")
			}
			if c := f.log.count(down); c != 1 {
				t.Errorf("unavailable logged %d times, want 1: %q", c, f.log.lines)
			}
			if c := f.log.count(up); c != 1 {
				t.Errorf("reachable logged %d times, want 1: %q", c, f.log.lines)
			}
		})
	}
}

// TestImageStartUpListingAcrossPages: copies already in the bucket, over
// several listing pages, answer from the CDN straight after start-up with
// no request to the source, while a half copy is treated as missing.
func TestImageStartUpListingAcrossPages(t *testing.T) {
	f := newImageFixture(t)
	seeded := []byte("RIFF....WEBP seeded")
	for n := 1; n <= 5; n++ {
		for _, file := range ImageFiles {
			f.srv.SetObject("images/"+rel(n)+"/"+file, seeded)
		}
	}
	f.srv.SetObject("images/"+rel(6)+"/high.webp", seeded)
	f.srv.SetObject("tcgdex/sets.json", []byte("{}")) // card data is not listed
	f.srv.PageSize(2)
	f.run(t)
	f.waitReady(t, 5*time.Second)

	// 11 image keys at 2 a page.
	if n := f.lists(); n != 6 {
		t.Errorf("listing pages = %d, want 6", n)
	}
	if n := f.log.count("card images: 5 copies found in the shared store"); n != 1 {
		t.Errorf("start-up count logged %d times, want 1: %q", n, f.log.lines)
	}
	for n := 1; n <= 5; n++ {
		f.wantCDN(t, n)
	}
	f.wantSource(t, f.src(6))
	f.settle(t, 10*time.Second)
	for n := 1; n <= 5; n++ {
		if p := f.puts(n); p != 0 {
			t.Errorf("PUTs of listed card %d = %d, want 0", n, p)
		}
	}
	if hits := f.host.totalHits(); hits != 1 {
		t.Errorf("source hits = %d, want 1 (the half copy's low.webp)", hits)
	}
	f.wantCopiedFile(t, 6, "low.webp", "")
	f.wantCDN(t, 6)
}

// TestImageAskedDuringListingIsNotUploadedAgain: an image a board asks for
// while the start-up listing is still running answers from the source,
// and is not copied once the listing finds it in the bucket.
func TestImageAskedDuringListingIsNotUploadedAgain(t *testing.T) {
	f := newImageFixture(t)
	f.ic.timeout = 10 * time.Second // the held listing must outlast the release
	seeded := []byte("RIFF....WEBP seeded")
	for _, file := range ImageFiles {
		f.srv.SetObject("images/"+rel(1)+"/"+file, seeded)
	}
	release := f.srv.Hang()
	defer release()
	f.run(t)
	deadline := time.Now().Add(5 * time.Second)
	for f.lists() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("start-up listing never reached the store")
		}
		time.Sleep(time.Millisecond)
	}

	f.wantSource(t, f.src(1)) // during the listing: not known yet
	select {
	case <-f.ic.ready:
		t.Fatal("copying started before the listing ended")
	default:
	}
	release()
	f.waitReady(t, 5*time.Second)
	f.settle(t, 10*time.Second)

	if p := f.puts(1); p != 0 {
		t.Errorf("PUTs of an image the listing found = %d, want 0", p)
	}
	if hits := f.host.totalHits(); hits != 0 {
		t.Errorf("source hits = %d, want 0", hits)
	}
	for _, file := range ImageFiles {
		f.wantSeeded(t, "images/"+rel(1)+"/"+file, seeded)
	}
	f.wantCDN(t, 1)
}

// TestImageFailedListingStillAnswers: a start-up listing that fails ends
// the wait for copying, logs the outage once, and leaves Image answering.
func TestImageFailedListingStillAnswers(t *testing.T) {
	f := newImageFixture(t)
	f.srv.SetFail(true)
	f.run(t)
	f.waitReady(t, 5*time.Second)
	f.wantSource(t, f.src(1))
	f.settle(t, 10*time.Second)
	if n := f.log.count("card images: shared store unavailable"); n != 1 {
		t.Errorf("outage logged %d times, want 1: %q", n, f.log.lines)
	}
	if n := f.log.count("copies found"); n != 0 {
		t.Errorf("a failed listing logged a count: %q", f.log.lines)
	}

	// Copying carries on once the store is back.
	f.srv.SetFail(false)
	f.clock.Advance(SharedRetryAfter)
	f.wantSource(t, f.src(1))
	f.settle(t, 10*time.Second)
	f.wantCopied(t, 1, "")
	f.wantCDN(t, 1)
}

// TestImageReportCountsAnswers: the hourly line reports the answers that
// pointed at the shared store and at the source, the share, and the
// copies made, then starts again; an hour with nothing logs nothing.
func TestImageReportCountsAnswers(t *testing.T) {
	f := newImageFixture(t)
	f.run(t)
	f.waitReady(t, 5*time.Second)
	f.wantSource(t, f.src(1))
	f.settle(t, 10*time.Second)
	for range 3 {
		f.wantCDN(t, 1)
	}

	f.ic.report()
	want := "card images: last hour 3 from the shared store, 1 from the card source (75% shared), 1 copied"
	if n := f.log.count(want); n != 1 {
		t.Errorf("report logged %q %d times, want 1: %q", want, n, f.log.lines)
	}
	before := len(f.log.lines)
	f.ic.report()
	if n := f.log.count("last hour"); n != 1 || len(f.log.lines) != before {
		t.Errorf("an idle hour logged: %q", f.log.lines[before:])
	}
}

// TestImageRunReportsOnItsTicker: Run writes the count line on its own
// every imageReportEvery.
func TestImageRunReportsOnItsTicker(t *testing.T) {
	prev := imageReportEvery
	imageReportEvery = 20 * time.Millisecond
	t.Cleanup(func() { imageReportEvery = prev })
	f := newImageFixture(t)
	f.wantSource(t, f.src(1)) // counted before Run starts
	f.run(t)
	deadline := time.Now().Add(5 * time.Second)
	for f.log.count("card images: last hour") == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("no count line from Run: %q", f.log.lines)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
