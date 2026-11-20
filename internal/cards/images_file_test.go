package cards

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gen2brain/webp"
)

// testJPEG is a w x h JPEG, a gradient so it is not trivially compressed.
func testJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x + y), 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// webpSize decodes a WebP's dimensions, failing the test if b is not one.
func webpSize(t *testing.T, b []byte) (int, int) {
	t.Helper()
	cfg, err := webp.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("not a WebP: %v", err)
	}
	return cfg.Width, cfg.Height
}

func TestResizeImage(t *testing.T) {
	src := testJPEG(t, 734, 1024) // malie.io's card JPEG size
	for file, want := range map[string][2]int{"low.webp": {245, 341}, "high.webp": {600, 837}} {
		out, err := resizeImage(src, file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if w, h := webpSize(t, out); w != want[0] || h != want[1] {
			t.Errorf("%s is %dx%d, want %dx%d", file, w, h, want[0], want[1])
		}
		if len(out) >= len(src) {
			t.Errorf("%s is %d bytes, not smaller than the %d-byte source", file, len(out), len(src))
		}
	}
	// A source narrower than the width is converted, never enlarged.
	out, err := resizeImage(testJPEG(t, 200, 280), "high.webp")
	if err != nil {
		t.Fatal(err)
	}
	if w, h := webpSize(t, out); w != 200 || h != 280 {
		t.Errorf("small source became %dx%d, want 200x280", w, h)
	}
	if _, err := resizeImage([]byte("not an image"), "low.webp"); err == nil {
		t.Error("resizing a non-image succeeded")
	}
}

// TestImageCacheFileSource: an image file under a file source is copied
// on first use, fetched once and resized to WebP low.webp and high.webp
// under the source's key, then answered with its CDN address at a content
// version; a restarted cache finds the copy without fetching again. Other
// addresses under the host, and files that are not images, are not copied.
func TestImageCacheFileSource(t *testing.T) {
	jpg := testJPEG(t, 734, 1024)
	var fetches atomic.Int32
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tcgl/cards/jpg/en/me6/me6_en_094_std.jpg" {
			http.NotFound(w, r)
			return
		}
		fetches.Add(1)
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(jpg)
	}))
	t.Cleanup(host.Close)
	src := host.URL + "/tcgl/cards/jpg/en/me6/me6_en_094_std.jpg"
	const relPath = "malie/jpg/en/me6/me6_en_094_std"

	f := newImageFixture(t)
	withMalie := func(ic *ImageCache) *ImageCache {
		ic.FileSources = []FileSource{{Prefix: host.URL + "/tcgl/cards", Key: "malie"}}
		return ic
	}
	withMalie(f.ic)
	f.run(t)
	f.waitReady(t, time.Second)

	for _, other := range []string{host.URL + "/tcgl/cards/jpg/en/me6/notes.txt", host.URL + "/elsewhere/x.jpg"} {
		f.wantSource(t, other)
	}
	f.wantSource(t, src) // first use: the source while the copy is made
	f.settle(t, 5*time.Second)
	if n := fetches.Load(); n != 1 {
		t.Fatalf("source fetched %d times, want 1", n)
	}
	want := f.srv.Config().PublicBase() + "/" + ImagePrefix + relPath
	img, fb, ver := f.ic.Image(src)
	if img != want || fb != src || ver == "" {
		t.Fatalf("Image = %q, %q, %q; want %q, %q and a version", img, fb, ver, want, src)
	}
	for file, width := range imageWidths {
		key := ImagePrefix + relPath + "/" + file
		body, ok := f.srv.Object(key)
		if !ok {
			t.Fatalf("no copy at %s", key)
		}
		if w, _ := webpSize(t, body); w != width {
			t.Errorf("%s is %d wide, want %d", file, w, width)
		}
		if ct := f.srv.Header(key).Get("Content-Type"); ct != "image/webp" {
			t.Errorf("%s content type %q, want image/webp", file, ct)
		}
		if !f.srv.Public(key) {
			t.Errorf("%s is not public", file)
		}
	}
	if keys := f.srv.Keys(ImagePrefix); len(keys) != 2 {
		t.Errorf("bucket holds %v, want only the two files of the copy", keys)
	}

	// A restart finds the copy in the bucket and answers with it at the
	// same version, without fetching the source again.
	f.ic = withMalie(f.newCache(t))
	f.run(t)
	f.waitReady(t, time.Second)
	if img2, _, ver2 := f.ic.Image(src); img2 != want || ver2 != ver {
		t.Errorf("after restart Image = %q at %q, want %q at %q", img2, ver2, want, ver)
	}
	if n := fetches.Load(); n != 1 {
		t.Errorf("source fetched %d times after restart, want 1", n)
	}
}

// TestImageCacheFileSourceUnreadable: a file source answer that is not an
// image is not copied, and is not fetched again before RetryAfter.
func TestImageCacheFileSourceUnreadable(t *testing.T) {
	var fetches atomic.Int32
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches.Add(1)
		w.Write([]byte("<html>not an image</html>"))
	}))
	t.Cleanup(host.Close)
	src := host.URL + "/tcgl/cards/jpg/en/me6/me6_en_001_std.jpg"
	f := newImageFixture(t)
	f.ic.FileSources = []FileSource{{Prefix: host.URL + "/tcgl/cards", Key: "malie"}}
	f.run(t)
	f.waitReady(t, time.Second)
	f.wantSource(t, src)
	f.settle(t, 5*time.Second)
	f.wantSource(t, src)
	f.settle(t, 5*time.Second)
	if n := fetches.Load(); n != 1 {
		t.Errorf("source fetched %d times, want 1 (no retry before RetryAfter)", n)
	}
	if keys := f.srv.Keys(ImagePrefix); len(keys) != 0 {
		t.Errorf("bucket holds %v, want nothing", keys)
	}
}
