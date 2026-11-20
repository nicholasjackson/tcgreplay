package cards

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"testing"
	"time"
)

// md5Hex is the ETag the bucket gives a single-part upload of b.
func md5Hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}

// wantVersionOf is the version of an image whose files hold low and high:
// the first 12 hex characters of SHA-256("<md5 low>\n<md5 high>").
func wantVersionOf(low, high []byte) string {
	sum := sha256.Sum256([]byte(md5Hex(low) + "\n" + md5Hex(high)))
	return hex.EncodeToString(sum[:])[:12]
}

// hostVersion is the version of card n's image as the host serves it at
// host version v.
func hostVersion(n int, v string) string {
	return wantVersionOf(imageBody("/"+rel(n)+"/low.webp", v), imageBody("/"+rel(n)+"/high.webp", v))
}

// versionOfCard is the version Image answers for card n.
func (f *imageFixture) versionOfCard(t *testing.T, n int) string {
	t.Helper()
	start := time.Now()
	_, _, v := f.ic.Image(f.src(n))
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("Image took %v, want under 50ms", d)
	}
	return v
}

// forget deletes card n's copy from the bucket and from the cache's
// memory, as if it had never been copied.
func (f *imageFixture) forget(t *testing.T, n int) {
	t.Helper()
	var keys []string
	for _, file := range ImageFiles {
		keys = append(keys, "images/"+rel(n)+"/"+file)
	}
	if err := f.srv.Client(t).Delete(context.Background(), keys); err != nil {
		t.Fatal(err)
	}
	f.ic.mu.Lock()
	delete(f.ic.known, rel(n))
	delete(f.ic.partial, rel(n))
	f.ic.mu.Unlock()
}

var versionForm = regexp.MustCompile(`^[0-9a-f]{12}$`)

func TestImageVersionFormula(t *testing.T) {
	// The first 12 hex characters of SHA-256("abc\ndef"), from sha256sum.
	if v := imageVersion("abc", "def"); v != "d53d6b91af7c" || !versionForm.MatchString(v) {
		t.Errorf("imageVersion = %q", v)
	}
	if imageVersion("abc", "def") == imageVersion("def", "abc") {
		t.Error("the files' order does not count")
	}
}

func TestImageCopyIsAnsweredWithItsContentVersion(t *testing.T) {
	f := newImageFixture(t)
	f.run(t)
	if img, fb, v := f.ic.Image(f.src(1)); img != f.src(1) || fb != "" || v != "" {
		t.Errorf("uncopied image = (%q, %q, %q), want the source with no version", img, fb, v)
	}
	f.settle(t, 10*time.Second)
	img, fb, v := f.ic.Image(f.src(1))
	if img != f.srv.Config().PublicBase()+"/images/"+rel(1) || fb != f.src(1) {
		t.Errorf("copied image = (%q, %q), want the CDN address with the source fallback", img, fb)
	}
	if want := hostVersion(1, ""); v != want {
		t.Errorf("version = %q, want %q", v, want)
	}
	// The version is what the bucket's listing reports.
	low, high := f.srv.ETag("images/"+rel(1)+"/low.webp"), f.srv.ETag("images/"+rel(1)+"/high.webp")
	if v != imageVersion(low, high) {
		t.Errorf("version %q does not match the bucket's ETags", v)
	}
	if _, _, v := f.ic.Image("https://elsewhere.example/x"); v != "" {
		t.Errorf("address outside the source has version %q", v)
	}
}

func TestImageVersionFollowsTheBytes(t *testing.T) {
	f := newImageFixture(t)
	f.run(t)
	f.wantSource(t, f.src(1))
	f.settle(t, 10*time.Second)
	first := f.versionOfCard(t, 1)

	// The same bytes copied again: the same version.
	f.forget(t, 1)
	f.wantSource(t, f.src(1))
	f.settle(t, 10*time.Second)
	if v := f.versionOfCard(t, 1); v != first {
		t.Errorf("version after copying the same bytes = %q, want %q", v, first)
	}

	// Different bytes: a different version.
	f.forget(t, 1)
	f.host.setVersion("v2 ")
	f.wantSource(t, f.src(1))
	f.settle(t, 10*time.Second)
	v := f.versionOfCard(t, 1)
	if v == first || v != hostVersion(1, "v2 ") {
		t.Errorf("version after new bytes = %q, want %q (was %q)", v, hostVersion(1, "v2 "), first)
	}
	f.wantCopied(t, 1, "v2 ")
}

func TestImageVersionSurvivesARestart(t *testing.T) {
	f := newImageFixture(t)
	stop := f.start(t)
	f.wantSource(t, f.src(1))
	f.settle(t, 10*time.Second)
	before := f.versionOfCard(t, 1)
	stop()

	f.ic = f.newCache(t)
	f.run(t)
	f.waitReady(t, 5*time.Second)
	if v := f.versionOfCard(t, 1); v != before || v == "" {
		t.Errorf("version after a restart = %q, want %q", v, before)
	}
}

func TestImageHalfCopyIsVersionedFromTheListedFileAndTheUploadedOne(t *testing.T) {
	f := newImageFixture(t)
	seeded := []byte("seeded low bytes")
	f.srv.SetObject("images/"+rel(1)+"/low.webp", seeded)
	f.run(t)
	f.waitReady(t, 5*time.Second)
	f.wantSource(t, f.src(1))
	f.settle(t, 10*time.Second)
	f.wantSeeded(t, "images/"+rel(1)+"/low.webp", seeded)
	want := wantVersionOf(seeded, imageBody("/"+rel(1)+"/high.webp", ""))
	if v := f.versionOfCard(t, 1); v != want {
		t.Errorf("version = %q, want %q", v, want)
	}
}

func TestImageWithoutBothChecksumsCountsAsNotCopied(t *testing.T) {
	f := newImageFixture(t)
	low, high := []byte("low"), []byte("high")
	f.srv.SetObject("images/"+rel(1)+"/low.webp", low)
	f.srv.SetObject("images/"+rel(1)+"/high.webp", high)
	f.srv.OmitETag("images/" + rel(1) + "/high.webp")
	f.run(t)
	f.waitReady(t, 5*time.Second)
	if n := f.log.count("card images: 0 copies found in the shared store"); n != 1 {
		t.Errorf("start-up listing: %q", f.log.lines)
	}
	for range 2 {
		if img, fb, v := f.ic.Image(f.src(1)); img != f.src(1) || fb != "" || v != "" {
			t.Errorf("image = (%q, %q, %q), want the source with no version", img, fb, v)
		}
		f.settle(t, 10*time.Second)
	}
	// Neither file is written over.
	f.wantSeeded(t, "images/"+rel(1)+"/low.webp", low)
	f.wantSeeded(t, "images/"+rel(1)+"/high.webp", high)

	// The same holds when the cache asks the bucket about the image itself
	// (the start-up listing failed).
	g := newImageFixture(t)
	g.srv.SetObject("images/"+rel(2)+"/low.webp", low)
	g.srv.SetObject("images/"+rel(2)+"/high.webp", high)
	g.srv.OmitETag("images/" + rel(2) + "/low.webp")
	g.ic.mu.Lock()
	g.ic.listed = false
	g.ic.mu.Unlock()
	g.ic.copyOne(context.Background(), rel(2))
	if _, _, v := g.ic.Image(g.src(2)); v != "" {
		t.Errorf("version without both checksums = %q, want none", v)
	}
	g.wantSeeded(t, "images/"+rel(2)+"/low.webp", low)
	g.wantSeeded(t, "images/"+rel(2)+"/high.webp", high)
}

func TestImageAnswersWithVersionsNeverWaitOnTheNetwork(t *testing.T) {
	f := newImageFixture(t)
	stop := f.start(t)
	f.wantSource(t, f.src(1))
	f.settle(t, 10*time.Second)
	stop()
	f.ic = f.newCache(t)
	f.run(t)
	f.waitReady(t, 5*time.Second)
	release := f.srv.Hang()
	defer release()
	for range 5 {
		if v := f.versionOfCard(t, 1); v == "" {
			t.Error("copied image lost its version")
		}
		f.versionOfCard(t, 2) // uncopied: queued, still answered at once
	}
}
