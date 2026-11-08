package cards

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
)

const (
	// ImagePrefix is where card image copies live in the bucket: an image
	// whose source address is <Source>/<path> is copied to
	// images/<path>/low.webp and images/<path>/high.webp. It is reserved
	// for card images; tcgdex/ holds the private card data.
	ImagePrefix = "images/"
	// ImageCacheControl is how long browsers and the CDN may keep a copy.
	// The source's art at an address never changes; a week matches the
	// CDN's longest edge lifetime.
	ImageCacheControl = "public, max-age=604800"

	imageQueueSize    = 256              // queued copies; more are dropped
	imageWorkers      = 2                // copies at a time
	imageFetchTimeout = 10 * time.Second // per file from the source
	maxImageFile      = 16 << 20         // bytes read per file from the source
)

// ImageFiles are the files of one image the board shows: the card on the
// mat loads low.webp, the zoom and the preview high.webp. An image counts
// as copied only once both are in the bucket.
var ImageFiles = []string{"low.webp", "high.webp"}

// ImageCache decides where the hosted board loads each card image from,
// and keeps the public copies in the shared store that make the CDN answer
// possible.
//
// Each copy has a content version, a short hash of its two files' ETags
// (the MD5 of their bytes): the copier takes it from the bytes it uploads,
// and the start-up listing from the ETags the bucket reports, so a copy
// keeps its version across restarts and gets a new one exactly when its
// bytes change. The board puts it in the copy's address, so browsers and
// the CDN, which keep a copy for a week, fetch changed art again.
//
// Run first lists the copies already in the bucket, so copies made before
// a restart are served from the CDN and never uploaded again. An object
// already in the bucket is never written over. Image
// answers from memory alone: the CDN address of an image with a
// copy, with the source address as fallback, otherwise the source address,
// queuing the image to be copied. Run copies queued images in the
// background, byte for byte, as public objects under ImagePrefix. Only
// addresses under Source are ever copied. Copies are never overwritten;
// art that appears later has a new address and gets its own copy. Only an
// admin's Clear deletes copies, after which the next view copies the
// source's current art again.
//
// Shared store failures follow one rule: each request is
// bounded by SharedTimeout, and a failure skips copies for
// SharedRetryAfter, logged once when the outage starts and once when it
// ends. A copy skipped meanwhile, or dropped from a full queue, is queued
// again the next time a board asks for the image. An image the source
// fails to serve is not fetched again for RetryAfter.
type ImageCache struct {
	Spaces *spaces.Client
	// Public is the bucket's public base address (spaces.Config.PublicBase).
	Public string
	// Source is the prefix of the addresses that may be copied, with no
	// trailing slash.
	Source string
	// HTTP fetches images from Source.
	HTTP *http.Client
	// Now is the clock; tests inject one.
	Now func() time.Time
	// CDN purges cleared copies from the bucket's CDN (a *spaces.CDN).
	// Nil means clears do not purge.
	CDN Purger

	timeout time.Duration        // per shared store request
	logf    func(string, ...any) // log output; tests capture it
	brk     breaker
	// attempt serialises shared store requests across workers, so an
	// outage costs one attempt per retry window, not one per worker.
	attempt sync.Mutex

	queue chan string   // relative image paths to copy
	ready chan struct{} // closed when copying may start

	mu      sync.Mutex
	idle    *sync.Cond                   // signalled when an image leaves pending
	known   map[string]string            // relative paths with both files copied -> content version
	partial map[string]map[string]string // files found of an image not known whole -> their ETags ("" when unknown)
	listed  bool                         // the start-up listing completed
	pending map[string]bool              // queued or being copied
	failed  map[string]time.Time         // when the source last failed to serve one
	copying map[string]bool              // being copied by a worker now
	// clearing holds the scopes a Clear is working on: their images are
	// answered from the source and never copied until it ends.
	clearing []Scope
	// unsure holds the scopes of clears that could not finish: the bucket
	// may still hold copies of their images, so before copying one the
	// cache asks the bucket rather than trusting the start-up listing, and
	// never writes over what is there.
	unsure []Scope
	cdn    int // answers with a CDN address
	source int // answers with the source address
	copied int // images copied
}

// NewImageCache returns an image cache keeping copies in the bucket c
// writes to, served from public, copying from TCGdex's asset host.
func NewImageCache(c *spaces.Client, public string) *ImageCache {
	ic := &ImageCache{
		Spaces:  c,
		Public:  strings.TrimRight(public, "/"),
		Source:  "https://assets.tcgdex.net",
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		Now:     time.Now,
		timeout: SharedTimeout,
		logf:    log.Printf,
		queue:   make(chan string, imageQueueSize),
		ready:   make(chan struct{}),
		known:   map[string]string{},
		partial: map[string]map[string]string{},
		pending: map[string]bool{},
		copying: map[string]bool{},
		failed:  map[string]time.Time{},
	}
	ic.idle = sync.NewCond(&ic.mu)
	ic.brk = breaker{
		retry:   SharedRetryAfter,
		now:     func() time.Time { return ic.Now() },
		logf:    func(format string, args ...any) { ic.logf(format, args...) },
		downMsg: "card images: shared store unavailable (%v); using the card source, retrying in %s",
		upMsg:   "card images: shared store reachable again",
	}
	return ic
}

// imageRel is the path of src below Source, when src is a clean address
// under it: no query, no fragment, and no empty, "." or ".." segment.
func (ic *ImageCache) imageRel(src string) (string, bool) {
	rest, ok := strings.CutPrefix(src, ic.Source+"/")
	if !ok || strings.ContainsAny(rest, "?#") {
		return "", false
	}
	for _, seg := range strings.Split(rest, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", false
		}
	}
	return rest, true
}

// imageVersion is a copy's content version: the first 12 hex characters
// of the SHA-256 of its low.webp ETag, a newline and its high.webp ETag.
func imageVersion(lowETag, highETag string) string {
	sum := sha256.Sum256([]byte(lowETag + "\n" + highETag))
	return hex.EncodeToString(sum[:])[:12]
}

// versionOf is the content version of an image whose files have etags,
// or "" unless every one of ImageFiles has a known ETag.
func versionOf(etags map[string]string) string {
	for _, f := range ImageFiles {
		if etags[f] == "" {
			return ""
		}
	}
	return imageVersion(etags[ImageFiles[0]], etags[ImageFiles[1]])
}

// Image returns where the board should load src from, the fallback when
// that differs from src, and the copy's content version when it is a copy
// ("" otherwise). It never blocks or touches the network; an image without
// a copy is queued for copying.
func (ic *ImageCache) Image(src string) (image, fallback, version string) {
	if src == "" {
		return "", "", ""
	}
	rel, ok := ic.imageRel(src)
	if !ok {
		return src, "", ""
	}
	ic.mu.Lock()
	defer ic.mu.Unlock()
	if inScopes(ic.clearing, rel) {
		ic.source++ // being cleared: the source, and no copy until it ends
		return src, "", ""
	}
	if v, ok := ic.known[rel]; ok {
		ic.cdn++
		return ic.Public + "/" + ImagePrefix + rel, src, v
	}
	ic.source++
	if ic.pending[rel] {
		return src, "", ""
	}
	if t, ok := ic.failed[rel]; ok && ic.Now().Sub(t) < RetryAfter {
		return src, "", ""
	}
	ic.pending[rel] = true
	select {
	case ic.queue <- rel:
	default:
		// Full: the next view queues it again.
		delete(ic.pending, rel)
	}
	return src, "", ""
}

// imageReportEvery is how often Run logs the image counts.
var imageReportEvery = time.Hour

// Run lists the copies already in the bucket, then copies queued images,
// and logs the counts every imageReportEvery, until ctx is done. Copying
// waits for the listing, so an image the listing finds is not uploaded
// again.
func (ic *ImageCache) Run(ctx context.Context) {
	go func() {
		ic.loadKnown(ctx)
		close(ic.ready)
	}()
	for range imageWorkers {
		go ic.work(ctx)
	}
	t := time.NewTicker(imageReportEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ic.report()
		}
	}
}

// report logs, and resets, how many answers since the last report pointed
// at the shared store's copies and how many at the card source, and how
// many images were copied: the running measure of how much of the board's
// art comes from the CDN. Nothing is logged when nothing happened.
func (ic *ImageCache) report() {
	ic.mu.Lock()
	cdn, source, copied := ic.cdn, ic.source, ic.copied
	ic.cdn, ic.source, ic.copied = 0, 0, 0
	ic.mu.Unlock()
	if cdn+source+copied == 0 {
		return
	}
	shared := 0.0
	if cdn+source > 0 {
		shared = 100 * float64(cdn) / float64(cdn+source)
	}
	ic.logf("card images: last hour %d from the shared store, %d from the card source (%.0f%% shared), %d copied", cdn, source, shared, copied)
}

// loadKnown lists ImagePrefix page by page and marks known, at its
// content version, every image whose files are all there with their ETags,
// remembering which files (and ETags) it found of the others. If the shared store is being skipped or a page fails, it keeps
// what it gathered and copying starts anyway; each image is then checked
// in the bucket before it is copied (see present), so a copy is never
// written over.
func (ic *ImageCache) loadKnown(ctx context.Context) {
	files := map[string]map[string]string{} // relative path -> the ImageFiles listed -> ETag
	ok := ic.list(ctx, ImagePrefix, func(key, etag string) {
		if rel, file, ok := splitImageKey(key); ok {
			if files[rel] == nil {
				files[rel] = map[string]string{}
			}
			files[rel][file] = etag
		}
	})
	n := 0
	ic.mu.Lock()
	for rel, fs := range files {
		if v := versionOf(fs); v != "" {
			ic.known[rel] = v
			n++
		} else {
			// Half a copy, or a whole one whose version cannot be told:
			// served from the source, and never written over.
			ic.partial[rel] = fs
		}
	}
	ic.listed = ok
	ic.mu.Unlock()
	if ok {
		ic.logf("card images: %d copies found in the shared store", n)
	}
}

// splitImageKey splits an object key images/<rel>/<file> whose file is
// one of ImageFiles.
func splitImageKey(key string) (rel, file string, ok bool) {
	rest, _ := strings.CutPrefix(key, ImagePrefix)
	i := strings.LastIndex(rest, "/")
	if i <= 0 || !slices.Contains(ImageFiles, rest[i+1:]) {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

// present returns the files of the image at rel already in the bucket,
// with their ETags ("" when unknown): from the start-up listing when it
// completed, otherwise by listing the image's own prefix. ok is false when
// the bucket could not be asked.
func (ic *ImageCache) present(ctx context.Context, rel string) (files map[string]string, ok bool) {
	files = map[string]string{}
	ic.mu.Lock()
	listed := ic.listed && !inScopes(ic.unsure, rel)
	for f, tag := range ic.partial[rel] {
		files[f] = tag
	}
	ic.mu.Unlock()
	if listed {
		return files, true
	}
	ok = ic.list(ctx, ImagePrefix+rel+"/", func(key, etag string) {
		if r, file, ok := splitImageKey(key); ok && r == rel {
			files[file] = etag
		}
	})
	return files, ok
}

// list calls each with every key under prefix and its ETag ("" when the
// listing gives none), reporting whether the whole listing succeeded.
func (ic *ImageCache) list(ctx context.Context, prefix string, each func(key, etag string)) bool {
	ic.attempt.Lock()
	defer ic.attempt.Unlock()
	token := ""
	for {
		if ic.brk.skip() {
			return false
		}
		lctx, cancel := context.WithTimeout(ctx, ic.timeout)
		page, err := ic.Spaces.List(lctx, prefix, token)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				ic.brk.failed(fmt.Errorf("list %s: %w", prefix, err))
			}
			return false
		}
		ic.brk.ok()
		for _, k := range page.Keys {
			each(k, page.ETags[k])
		}
		if page.Next == "" {
			return true
		}
		token = page.Next
	}
}

// work copies queued images, once copying may start, until ctx is done.
func (ic *ImageCache) work(ctx context.Context) {
	select {
	case <-ic.ready:
	case <-ctx.Done():
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case rel := <-ic.queue:
			ic.handle(ctx, rel)
		}
	}
}

// handle copies one queued image unless it is known by now or the shared
// store is being skipped.
func (ic *ImageCache) handle(ctx context.Context, rel string) {
	defer ic.unpend(rel)
	ic.mu.Lock()
	_, known := ic.known[rel]
	skip := known || inScopes(ic.clearing, rel)
	if !skip {
		ic.copying[rel] = true // a Clear of rel's scope waits for this copy
	}
	ic.mu.Unlock()
	if skip || ic.brk.skip() {
		return
	}
	ic.copyOne(ctx, rel)
}

// unpend takes rel off the pending and copying sets.
func (ic *ImageCache) unpend(rel string) {
	ic.mu.Lock()
	delete(ic.pending, rel)
	delete(ic.copying, rel)
	ic.idle.Broadcast()
	ic.mu.Unlock()
}

// copyOne fetches each file of the image at rel that the bucket lacks
// from the source, then puts each to the shared store as a public copy,
// and marks the image known, at its content version, once all are in. The
// version comes from the MD5 of the bytes it puts (the ETag the bucket
// gives a single-part upload) and the listed ETags of files already there.
// A file already in the bucket is never written again, so an image with a
// file whose ETag is unknown stays uncopied.
func (ic *ImageCache) copyOne(ctx context.Context, rel string) {
	have, ok := ic.present(ctx, rel)
	if !ok {
		return
	}
	var missing []string
	for _, f := range ImageFiles {
		if _, ok := have[f]; !ok {
			missing = append(missing, f)
		}
	}
	bodies := make([][]byte, len(missing))
	for i, f := range missing {
		b, err := ic.fetch(ctx, ic.Source+"/"+rel+"/"+f)
		if err != nil {
			if ctx.Err() == nil {
				ic.mu.Lock()
				ic.failed[rel] = ic.Now()
				ic.mu.Unlock()
			}
			return
		}
		bodies[i] = b
	}
	for i, f := range missing {
		if !ic.put(ctx, ImagePrefix+rel+"/"+f, bodies[i]) {
			return
		}
		sum := md5.Sum(bodies[i])
		have[f] = hex.EncodeToString(sum[:])
		// Remember the file is in, so a copy that fails part way never
		// writes it again.
		ic.mu.Lock()
		if ic.partial[rel] == nil {
			ic.partial[rel] = map[string]string{}
		}
		ic.partial[rel][f] = have[f]
		ic.mu.Unlock()
	}
	v := versionOf(have)
	if v == "" {
		return
	}
	ic.mu.Lock()
	ic.known[rel] = v
	delete(ic.partial, rel)
	if len(missing) > 0 {
		ic.copied++
	}
	delete(ic.failed, rel)
	ic.mu.Unlock()
}

// put writes one public image object, unless the shared store is being
// skipped, and reports whether it was written. Workers take turns, so a
// failure is seen by the next worker before it tries.
func (ic *ImageCache) put(ctx context.Context, key string, body []byte) bool {
	ic.attempt.Lock()
	defer ic.attempt.Unlock()
	if ic.brk.skip() {
		return false
	}
	pctx, cancel := context.WithTimeout(ctx, ic.timeout)
	err := ic.Spaces.Put(pctx, key, body, spaces.PutOptions{ContentType: "image/webp", CacheControl: ImageCacheControl, Public: true})
	cancel()
	if err != nil {
		// A Run that is ending says nothing about the shared store.
		if ctx.Err() == nil {
			ic.brk.failed(fmt.Errorf("put %s: %w", key, err))
		}
		return false
	}
	ic.brk.ok()
	return true
}

// fetch reads one image file from the source: a 200 with a body.
func (ic *ImageCache) fetch(ctx context.Context, url string) ([]byte, error) {
	fctx, cancel := context.WithTimeout(ctx, imageFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(fctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := ic.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxImageFile+1))
	switch {
	case err != nil:
		return nil, err
	case len(b) == 0:
		return nil, errors.New(url + ": empty body")
	case len(b) > maxImageFile:
		return nil, fmt.Errorf("%s: larger than %d bytes", url, maxImageFile)
	}
	return b, nil
}

// settle waits until no image is queued or being copied. It is for tests,
// and needs Run.
func (ic *ImageCache) settle() {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	for len(ic.pending) > 0 {
		ic.idle.Wait()
	}
}

// Purger purges paths from a CDN's cache; *spaces.CDN is one.
type Purger interface {
	Purge(ctx context.Context, paths []string) error
}

// ErrCDNNotConfigured is the "cdn" tier's error when the cache has no
// purge client: the copies were deleted but the CDN was not purged.
var ErrCDNNotConfigured = errors.New("CDN purge not configured")

// inScopes reports whether image path rel is in any of scopes.
func inScopes(scopes []Scope, rel string) bool {
	for _, s := range scopes {
		if s.Image(rel) {
			return true
		}
	}
	return false
}

// Clear forgets, deletes and purges the copies of the images in s, and
// reports the "images" tier (objects deleted) and the "cdn" tier (paths
// purged, or why not).
//
// From its start, images in s are answered with their source address and
// are not copied; Clear first waits for any copy of one already under way.
// It then lists ImagePrefix, deletes the objects in s in batches, and
// purges their paths from the CDN when a purge client is set: "images/*"
// for everything, else images/<lang>/<serie>/<set>/* for each directory
// the set's copies were in. When it ends, the next view of each image
// copies the source's current art again, at a new version if the bytes
// changed. Like the card store clears, it calls the bucket directly with
// clearTimeout bounds, whether or not copies are being skipped after a
// failure, and reports failures rather than skipping. If the listing or
// the delete fails, copies may remain, so the cache asks the bucket before
// copying an image of s again and never writes over one.
func (ic *ImageCache) Clear(ctx context.Context, s Scope) []TierReport {
	ic.mu.Lock()
	ic.clearing = append(ic.clearing, s)
	for {
		busy := false
		for rel := range ic.copying {
			busy = busy || s.Image(rel)
		}
		if !busy {
			break
		}
		ic.idle.Wait()
	}
	for rel := range ic.known {
		if s.Image(rel) {
			delete(ic.known, rel)
		}
	}
	for rel := range ic.partial {
		if s.Image(rel) {
			delete(ic.partial, rel)
		}
	}
	for rel := range ic.failed {
		if s.Image(rel) {
			delete(ic.failed, rel)
		}
	}
	ic.mu.Unlock()
	defer func() {
		ic.mu.Lock()
		for i, c := range ic.clearing {
			if c == s {
				ic.clearing = append(ic.clearing[:i:i], ic.clearing[i+1:]...)
				break
			}
		}
		ic.mu.Unlock()
	}()

	images := TierReport{Tier: "images"}
	var keys, paths []string
	dirs := map[string]bool{}
	all, err := listAll(ctx, ic.Spaces, ImagePrefix)
	if err != nil {
		images.Err = err
	}
	for _, key := range all {
		rest := strings.TrimPrefix(key, ImagePrefix)
		i := strings.LastIndex(rest, "/")
		if !s.All() && (i <= 0 || !s.Image(rest[:i])) {
			continue // everything under images/ goes in a full clear
		}
		keys = append(keys, key)
		if s.All() {
			continue
		}
		if segs := strings.Split(rest[:i], "/"); len(segs) >= 3 {
			dir := ImagePrefix + strings.Join(segs[:3], "/") + "/*"
			if !dirs[dir] {
				dirs[dir] = true
				paths = append(paths, dir)
			}
		}
	}
	if s.All() {
		paths = []string{ImagePrefix + "*"}
	}
	if images.Err == nil && len(keys) > 0 {
		dctx, cancel := context.WithTimeout(ctx, clearTimeout*time.Duration(1+len(keys)/1000))
		images.Removed = len(keys)
		if err := ic.Spaces.Delete(dctx, keys); err != nil {
			var de *spaces.DeleteError
			if errors.As(err, &de) {
				images.Removed -= len(de.Failed)
			} else {
				images.Removed = 0
			}
			images.Err = err
		}
		cancel()
	}

	cdn := TierReport{Tier: "cdn"}
	switch {
	case ic.CDN == nil:
		cdn.Err = ErrCDNNotConfigured
	case len(paths) == 0 && images.Err != nil:
		cdn.Err = fmt.Errorf("not purged: the copies could not be listed: %w", images.Err)
	case len(paths) > 0:
		pctx, cancel := context.WithTimeout(ctx, clearTimeout)
		if err := ic.CDN.Purge(pctx, paths); err != nil {
			cdn.Err = err
		} else {
			cdn.Removed = len(paths)
		}
		cancel()
	}

	ic.mu.Lock()
	if images.Err != nil {
		ic.unsure = append(ic.unsure, s)
	} else {
		kept := ic.unsure[:0:0]
		for _, u := range ic.unsure {
			if !s.All() && u != s {
				kept = append(kept, u)
			}
		}
		ic.unsure = kept
	}
	ic.mu.Unlock()

	purge := "purged"
	switch {
	case errors.Is(cdn.Err, ErrCDNNotConfigured):
		purge = "not purged (no TCGREPLAY_SPACES_CDN_TOKEN)"
	case cdn.Err != nil:
		purge = "error: " + cdn.Err.Error()
	case len(paths) == 0:
		purge = "nothing to purge"
	}
	if images.Err != nil {
		ic.logf("card images: cleared %s: %d objects, error: %v; CDN %s", s, images.Removed, images.Err, purge)
	} else {
		ic.logf("card images: cleared %s: %d objects, CDN %s", s, images.Removed, purge)
	}
	return []TierReport{images, cdn}
}
