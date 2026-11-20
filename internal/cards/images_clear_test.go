package cards

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// clearRels are the images the clear tests copy: two of me01, one of sv1
// and borrowed Energy art under swsh12.5.
var clearRels = []string{"en/me/me01/001", "en/me/me01/002", "en/sv/sv1/1", "en/swsh/swsh12.5/157"}

const cdnToken = "dop_v1_testtoken"

// clearFixture is an image fixture whose copies of clearRels are made,
// with a stand-in DigitalOcean CDN API as its purge client.
func newClearFixture(t *testing.T) (*imageFixture, *spacestest.CDN) {
	t.Helper()
	f := newImageFixture(t)
	cdn := spacestest.NewCDN(t, "test-bucket.lon1.digitaloceanspaces.com", cdnToken)
	purger := spaces.NewCDN(spaces.Config{Endpoint: "https://lon1.digitaloceanspaces.com", Bucket: "test-bucket"}, cdnToken)
	purger.API = cdn.URL
	f.ic.CDN = purger
	f.run(t)
	for _, r := range clearRels {
		f.ic.Image(f.host.URL + "/" + r)
	}
	f.settle(t, 10*time.Second)
	for _, r := range clearRels {
		if _, _, v := f.ic.Image(f.host.URL + "/" + r); v == "" {
			t.Fatalf("%s not copied", r)
		}
	}
	return f, cdn
}

func (f *imageFixture) srcOf(r string) string { return f.host.URL + "/" + r }

// imageKeys returns the image objects left for r.
func (f *imageFixture) imageKeys(r string) []string { return f.srv.Keys(ImagePrefix + r + "/") }

func tierOf(t *testing.T, rs []TierReport, tier string) TierReport {
	t.Helper()
	for _, r := range rs {
		if r.Tier == tier {
			return r
		}
	}
	t.Fatalf("no %s report in %v", tier, rs)
	return TierReport{}
}

func TestImageClearOfOneSet(t *testing.T) {
	f, cdn := newClearFixture(t)
	rs := f.ic.Clear(context.Background(), Scope{Set: "me01"})
	if r := tierOf(t, rs, "images"); r.Removed != 4 || r.Err != nil {
		t.Errorf("images report = %+v, want 4 removed", r)
	}
	if r := tierOf(t, rs, "cdn"); r.Removed != 1 || r.Err != nil {
		t.Errorf("cdn report = %+v, want 1 path purged", r)
	}
	for _, r := range clearRels[:2] {
		if keys := f.imageKeys(r); len(keys) != 0 {
			t.Errorf("%s still has %v", r, keys)
		}
		if img, fb, v := f.ic.Image(f.srcOf(r)); img != f.srcOf(r) || fb != "" || v != "" {
			t.Errorf("cleared %s answered (%q, %q, %q), want the source", r, img, fb, v)
		}
	}
	for _, r := range clearRels[2:] {
		if keys := f.imageKeys(r); len(keys) != 2 {
			t.Errorf("%s left %v, want both files", r, keys)
		}
		if img, _, v := f.ic.Image(f.srcOf(r)); img != f.srv.Config().PublicBase()+"/images/"+r || v == "" {
			t.Errorf("other set's %s answered %q (%q), want its CDN copy", r, img, v)
		}
	}
	if got := cdn.Purges(); !reflect.DeepEqual(got, [][]string{{"images/en/me/me01/*"}}) {
		t.Errorf("purges = %v", got)
	}
	if n := f.log.count("card images: cleared set me01: 4 objects, CDN purged"); n != 1 {
		t.Errorf("log = %q", f.log.lines)
	}
}

func TestImageClearOfEverything(t *testing.T) {
	f, cdn := newClearFixture(t)
	f.srv.SetObject("images/stray.txt", []byte("x"))
	rs := f.ic.Clear(context.Background(), Scope{})
	if r := tierOf(t, rs, "images"); r.Removed != 9 || r.Err != nil {
		t.Errorf("images report = %+v, want 9 removed", r)
	}
	if keys := f.srv.Keys(ImagePrefix); len(keys) != 0 {
		t.Errorf("images left: %v", keys)
	}
	if got := cdn.Purges(); !reflect.DeepEqual(got, [][]string{{"images/*"}}) {
		t.Errorf("purges = %v, want [[images/*]]", got)
	}
	for _, r := range clearRels {
		if img, _, _ := f.ic.Image(f.srcOf(r)); img != f.srcOf(r) {
			t.Errorf("%s answered %q, want the source", r, img)
		}
	}
}

func TestImageClearHoldsOffCopiesThenCopiesTheCurrentArt(t *testing.T) {
	f, _ := newClearFixture(t)
	r := clearRels[0]
	_, _, before := f.ic.Image(f.srcOf(r))
	f.host.setVersion("v2 ")
	puts := f.srv.Requests("PUT", "images/"+r+"/low.webp")

	release := f.srv.Hang() // the clear's listing waits
	done := make(chan []TierReport)
	go func() { done <- f.ic.Clear(context.Background(), Scope{Set: "me01"}) }()
	waitFor(t, 5*time.Second, "the clear to start", func() bool {
		f.ic.mu.Lock()
		defer f.ic.mu.Unlock()
		return len(f.ic.clearing) == 1
	})
	for range 3 {
		img, fb, v := f.ic.Image(f.srcOf(r))
		if img != f.srcOf(r) || fb != "" || v != "" {
			t.Errorf("during the clear: (%q, %q, %q), want the source", img, fb, v)
		}
	}
	f.ic.mu.Lock()
	queued := f.ic.pending[r]
	f.ic.mu.Unlock()
	if queued {
		t.Error("an image being cleared was queued for copying")
	}
	release()
	rs := <-done
	if err := tierOf(t, rs, "images").Err; err != nil {
		t.Fatal(err)
	}
	if n := f.srv.Requests("PUT", "images/"+r+"/low.webp"); n != puts {
		t.Errorf("PUTs during the clear = %d, want %d", n, puts)
	}

	// The next view copies the source's current art again, at a new version.
	f.wantSource(t, f.srcOf(r))
	f.settle(t, 10*time.Second)
	img, _, after := f.ic.Image(f.srcOf(r))
	if img != f.srv.Config().PublicBase()+"/images/"+r {
		t.Errorf("after the clear, answered %q, want the new copy", img)
	}
	if after == "" || after == before {
		t.Errorf("version after re-copying new art = %q, was %q", after, before)
	}
	if b, _ := f.srv.Object("images/" + r + "/high.webp"); string(b) != string(imageBody("/"+r+"/high.webp", "v2 ")) {
		t.Errorf("re-copied bytes = %q", b)
	}
}

func TestImageClearWaitsForACopyUnderWay(t *testing.T) {
	f, _ := newClearFixture(t)
	r := "en/me/me01/003"
	release := f.host.hold() // the copy's fetch waits
	f.ic.Image(f.srcOf(r))
	waitFor(t, 5*time.Second, "the copy to start", func() bool {
		f.ic.mu.Lock()
		defer f.ic.mu.Unlock()
		return f.ic.copying[r]
	})
	done := make(chan []TierReport)
	go func() { done <- f.ic.Clear(context.Background(), Scope{Set: "me01"}) }()
	select {
	case <-done:
		t.Fatal("the clear did not wait for the copy under way")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	<-done
	if keys := f.imageKeys(r); len(keys) != 0 {
		t.Errorf("a copy made just before the clear survived it: %v", keys)
	}
	if img, _, _ := f.ic.Image(f.srcOf(r)); img != f.srcOf(r) {
		t.Errorf("answered %q, want the source", img)
	}
}

func TestImageClearWithoutAPurgeClient(t *testing.T) {
	f, cdn := newClearFixture(t)
	f.ic.CDN = nil
	rs := f.ic.Clear(context.Background(), Scope{Set: "sv1"})
	if r := tierOf(t, rs, "images"); r.Removed != 2 || r.Err != nil {
		t.Errorf("images report = %+v", r)
	}
	if r := tierOf(t, rs, "cdn"); !errors.Is(r.Err, ErrCDNNotConfigured) {
		t.Errorf("cdn report = %+v, want ErrCDNNotConfigured", r)
	}
	if len(cdn.Purges()) != 0 {
		t.Errorf("purges = %v", cdn.Purges())
	}
	if n := f.log.count("CDN not purged (no TCGREPLAY_SPACES_CDN_TOKEN)"); n != 1 {
		t.Errorf("log = %q", f.log.lines)
	}
}

func TestImageClearReportsAFailedPurge(t *testing.T) {
	f, cdn := newClearFixture(t)
	cdn.SetFail(true)
	rs := f.ic.Clear(context.Background(), Scope{Set: "me01"})
	if r := tierOf(t, rs, "cdn"); r.Err == nil {
		t.Errorf("cdn report = %+v, want an error", r)
	}
	if r := tierOf(t, rs, "images"); r.Removed != 4 || r.Err != nil {
		t.Errorf("images report = %+v, want the copies deleted", r)
	}
	if keys := f.imageKeys(clearRels[0]); len(keys) != 0 {
		t.Errorf("left %v", keys)
	}
}

func TestImageClearWithDeletesRefused(t *testing.T) {
	f, _ := newClearFixture(t)
	r := clearRels[0]
	f.srv.DenyDelete("images/"+r+"/low.webp", "images/"+r+"/high.webp")
	rs := f.ic.Clear(context.Background(), Scope{Set: "me01"})
	if rep := tierOf(t, rs, "images"); rep.Err == nil || rep.Removed != 2 {
		t.Errorf("images report = %+v, want an error and 2 removed", rep)
	}
	// The copies left behind are no longer pointed at.
	if img, _, v := f.ic.Image(f.srcOf(r)); img != f.srcOf(r) || v != "" {
		t.Errorf("answered (%q, %q), want the source", img, v)
	}
	f.settle(t, 10*time.Second)
	// The cache asks the bucket about the image before copying it again,
	// and never writes over the objects the clear could not delete.
	if n := f.srv.Requests("LIST", ImagePrefix+r+"/"); n == 0 {
		t.Error("the image was not looked up in the bucket before copying")
	}
	for _, file := range ImageFiles {
		if n := f.srv.Requests("PUT", "images/"+r+"/"+file); n != 1 {
			t.Errorf("PUTs of %s = %d, want only the original copy", file, n)
		}
	}

	// A clear that succeeds afterwards removes them, and copying resumes.
	f.srv.DenyDelete()
	rs = f.ic.Clear(context.Background(), Scope{Set: "me01"})
	if rep := tierOf(t, rs, "images"); rep.Err != nil {
		t.Fatal(rep.Err)
	}
	if keys := f.imageKeys(r); len(keys) != 0 {
		t.Errorf("left %v", keys)
	}
	f.ic.mu.Lock()
	unsure := slices.Clone(f.ic.unsure)
	f.ic.mu.Unlock()
	if len(unsure) != 0 {
		t.Errorf("unsure scopes after a good clear: %v", unsure)
	}
	f.wantSource(t, f.srcOf(r))
	f.settle(t, 10*time.Second)
	if _, _, v := f.ic.Image(f.srcOf(r)); v == "" {
		t.Error("not copied again after a good clear")
	}
}

func TestImageClearReachesTheBucketWhileCopiesAreSkipped(t *testing.T) {
	f, _ := newClearFixture(t)
	f.ic.brk.failed(errors.New("test outage"))
	if !f.ic.brk.skip() {
		t.Fatal("breaker not tripped")
	}
	rs := f.ic.Clear(context.Background(), Scope{Set: "me01"})
	if r := tierOf(t, rs, "images"); r.Removed != 4 || r.Err != nil {
		t.Errorf("images report = %+v", r)
	}
}
