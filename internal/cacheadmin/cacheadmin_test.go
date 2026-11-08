package cacheadmin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/cardset"
	"github.com/nicholasjackson/tcgreplay/internal/catalog"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
	"github.com/nicholasjackson/tcgreplay/internal/spaces"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// fakeCatalog names me01 by MEG, me01 or ME01, answers checks with the
// result or error it holds, and reports a fixed status.
type fakeCatalog struct {
	mu     sync.Mutex
	checks int
	result catalog.CheckResult
	err    error
	status catalog.Status
}

func (f *fakeCatalog) SetID(name string) (string, string, error) {
	switch name {
	case "MEG", "me01", "ME01":
		return "me01", "Mega Evolution", nil
	}
	return "", "", fmt.Errorf("%q: %w", name, cards.ErrUnknownSet)
}

func (f *fakeCatalog) Check(ctx context.Context) (catalog.CheckResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks++
	if _, ok := ctx.Deadline(); !ok {
		return catalog.CheckResult{}, errors.New("check without a deadline")
	}
	return f.result, f.err
}

func (f *fakeCatalog) Status() catalog.Status { return f.status }

// fakeImages records the scopes it is cleared with, reports what it
// holds, and can hold a clear until released.
type fakeImages struct {
	mu      sync.Mutex
	scopes  []cards.Scope
	reports []cards.TierReport
	hold    chan struct{}
}

func (f *fakeImages) Clear(_ context.Context, s cards.Scope) []cards.TierReport {
	f.mu.Lock()
	f.scopes = append(f.scopes, s)
	hold, reports := f.hold, f.reports
	f.mu.Unlock()
	if hold != nil {
		<-hold
	}
	if reports == nil {
		reports = []cards.TierReport{{Tier: "images", Removed: 4}, {Tier: "cdn", Err: cards.ErrCDNNotConfigured}}
	}
	return reports
}

func (f *fakeImages) cleared() []cards.Scope {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.scopes)
}

// logLines collects log lines.
type logLines struct {
	mu    sync.Mutex
	lines []string
}

func (l *logLines) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *logLines) all() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

// serve serves the service's handler as the admin admin@example.com would
// reach it behind the admin guard.
func serve(t *testing.T, s *Service) *httptest.Server {
	t.Helper()
	h := s.Handler()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := hostauth.WithAccount(r.Context(), hostauth.Account{ID: "sub-1", Email: "admin@example.com", EmailVerified: true})
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func getPage(t *testing.T, ts *httptest.Server) (int, string) {
	t.Helper()
	resp, err := http.Get(ts.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	return resp.StatusCode, string(b)
}

func post(t *testing.T, ts *httptest.Server, form url.Values) (int, string, string) {
	t.Helper()
	return postTo(t, ts, "/admin/clear", form)
}

func postTo(t *testing.T, ts *httptest.Server, path string, form url.Values) (int, string, string) {
	t.Helper()
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.PostForm(ts.URL+path, form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Location"), string(b)
}

func newService(c Catalog, i Images) (*Service, *logLines) {
	l := &logLines{}
	s := &Service{Catalog: c, Logf: l.logf}
	if i != nil {
		s.Images = i
	}
	return s, l
}

func TestPageShowsTheCatalogueAndTheActions(t *testing.T) {
	c := &fakeCatalog{status: catalog.Status{
		Version: "0123456789abcdef0123", Sets: 206, Prints: 21439, Base: "https://cdn.example/cardsets/",
		LastCheck: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC), LastChange: time.Date(2026, 11, 8, 11, 0, 0, 0, time.UTC), LastError: "GET manifest: 503",
	}}
	s, _ := newService(c, &fakeImages{})
	status, body := getPage(t, serve(t, s))
	for _, want := range []string{
		"Signed in as admin@example.com",
		"0123456789ab", "206 (21439 cards)", "https://cdn.example/cardsets/", "2026-11-08 12:00:00 UTC", "2026-11-08 11:00:00 UTC", "GET manifest: 503",
		`action="/admin/check"`, "Check for published card sets now",
		"Clear images: everything", `name="all" value="1"`, "Clear images for one set", `name="set"`, "MEG or me01",
		"No clear since the playmat started", `<a href="/board">Back to the board</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if status != http.StatusOK || strings.Contains(body, `http-equiv="refresh"`) {
		t.Errorf("status %d, or refreshing with no clear running", status)
	}
}

// The page no longer offers to clear card definitions: only images.
func TestPageOffersNoCardDataClear(t *testing.T) {
	s, _ := newService(&fakeCatalog{}, &fakeImages{})
	_, body := getPage(t, serve(t, s))
	lower := strings.ToLower(body)
	for _, gone := range []string{"card cache", "card data", "card entries", "clear everything", "cached card"} {
		if strings.Contains(lower, gone) {
			t.Errorf("page still offers %q:\n%s", gone, body)
		}
	}
	if n := strings.Count(body, `action="/admin/clear"`); n != 2 {
		t.Errorf("page has %d clear forms, want the two image forms", n)
	}
}

func TestClearEverythingClearsImagesAndShowsEachTier(t *testing.T) {
	i := &fakeImages{}
	s, logs := newService(&fakeCatalog{}, i)
	ts := serve(t, s)
	status, loc, _ := post(t, ts, url.Values{"all": {"1"}})
	if status != http.StatusSeeOther || loc != "/admin" {
		t.Fatalf("POST all = %d to %q, want 303 to /admin", status, loc)
	}
	s.wait()
	if got := i.cleared(); !slices.Equal(got, []cards.Scope{{}}) {
		t.Errorf("images cleared %v", got)
	}
	_, body := getPage(t, ts)
	for _, want := range []string{"Last clear", "Images for everything by admin@example.com", "4 image copies removed", "CDN not purged (no TCGREPLAY_SPACES_CDN_TOKEN)"} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "with failures") || strings.Contains(body, `http-equiv="refresh"`) {
		t.Error("finished clear shown as failed or still refreshing")
	}
	log := logs.all()
	if !strings.Contains(log, "cache admin: admin@example.com started clearing images for everything") || !strings.Contains(log, "cache admin: cleared images for everything for admin@example.com in ") {
		t.Errorf("log = %q", log)
	}
}

func TestClearOneSetByCodeOrID(t *testing.T) {
	for _, name := range []string{"MEG", "me01", "ME01", " MEG "} {
		i := &fakeImages{}
		s, logs := newService(&fakeCatalog{}, i)
		ts := serve(t, s)
		if status, _, body := post(t, ts, url.Values{"set": {name}}); status != http.StatusSeeOther {
			t.Fatalf("POST set=%q = %d %s", name, status, body)
		}
		s.wait()
		if got := i.cleared(); !slices.Equal(got, []cards.Scope{{Set: "me01"}}) {
			t.Errorf("set=%q cleared images %v, want me01", name, got)
		}
		if _, body := getPage(t, ts); !strings.Contains(body, "Images for Mega Evolution (me01) by admin@example.com") {
			t.Errorf("page does not name the set")
		}
		if !strings.Contains(logs.all(), "started clearing images for Mega Evolution (me01)") {
			t.Errorf("log = %q", logs.all())
		}
	}
}

func TestUnknownOrEmptySetIsRefusedAndNothingCleared(t *testing.T) {
	i := &fakeImages{}
	s, _ := newService(&fakeCatalog{}, i)
	ts := serve(t, s)
	for _, form := range []url.Values{{"set": {"XYZ"}}, {"set": {"  "}}, {}, {"all": {"0"}}} {
		status, _, body := post(t, ts, form)
		if status != http.StatusBadRequest {
			t.Errorf("%v = %d, want 400", form, status)
		}
		want := "Name a set by its code"
		if form.Get("set") == "XYZ" {
			want = `No set is named &#34;XYZ&#34;. Nothing was cleared.`
		}
		if !strings.Contains(body, want) {
			t.Errorf("%v: page lacks %q:\n%s", form, want, body)
		}
	}
	s.wait()
	if len(i.cleared()) != 0 {
		t.Errorf("cleared images %v, want nothing", i.cleared())
	}
}

// Without the shared store there are no image copies: the page says so
// and offers no clear, and a clear is refused.
func TestNoImageClearWithoutTheSharedStore(t *testing.T) {
	s, _ := newService(&fakeCatalog{}, nil)
	ts := serve(t, s)
	_, body := getPage(t, ts)
	if !strings.Contains(body, "Images are not copied: the shared store is off.") || strings.Contains(body, `action="/admin/clear"`) {
		t.Errorf("page without the shared store:\n%s", body)
	}
	if !strings.Contains(body, "Check for published card sets now") {
		t.Error("page lacks the check")
	}
	if status, _, body := post(t, ts, url.Values{"all": {"1"}}); status != http.StatusConflict || !strings.Contains(body, "no image copies to clear") {
		t.Errorf("clear without images = %d:\n%s", status, body)
	}
}

func TestOneClearAtATime(t *testing.T) {
	i := &fakeImages{hold: make(chan struct{})}
	s, _ := newService(&fakeCatalog{}, i)
	ts := serve(t, s)
	if status, _, _ := post(t, ts, url.Values{"all": {"1"}}); status != http.StatusSeeOther {
		t.Fatal(status)
	}
	_, body := getPage(t, ts)
	if !strings.Contains(body, `<meta http-equiv="refresh" content="2">`) || !strings.Contains(body, "Clearing images for everything for admin@example.com") {
		t.Errorf("running page:\n%s", body)
	}
	status, _, body := post(t, ts, url.Values{"set": {"MEG"}})
	if status != http.StatusConflict || !strings.Contains(body, "A clear is already running") {
		t.Errorf("second clear = %d", status)
	}
	if _, err := s.Start(context.Background(), "x", "", true); !errors.Is(err, ErrBusy) {
		t.Errorf("Start while running = %v, want ErrBusy", err)
	}
	close(i.hold)
	s.wait()
	if _, body := getPage(t, ts); strings.Contains(body, `http-equiv="refresh"`) {
		t.Error("page still refreshes after the clear ended")
	}
	if len(i.cleared()) != 1 {
		t.Errorf("cleared %v, want one clear", i.cleared())
	}
	// Once it has ended another may start.
	if status, _, _ := post(t, ts, url.Values{"set": {"me01"}}); status != http.StatusSeeOther {
		t.Errorf("clear after the first ended = %d", status)
	}
	s.wait()
}

func TestAFailedTierIsShownAndClearingAgainIsAllowed(t *testing.T) {
	i := &fakeImages{reports: []cards.TierReport{{Tier: "images", Removed: 3, Err: errors.New("spaces: 403 AccessDenied: no")}, {Tier: "cdn", Removed: 1}}}
	s, logs := newService(&fakeCatalog{}, i)
	ts := serve(t, s)
	post(t, ts, url.Values{"all": {"1"}})
	s.wait()
	_, body := getPage(t, ts)
	for _, want := range []string{"with failures", "3 image copies removed, then failed: spaces: 403 AccessDenied: no", "CDN purged (1 paths)"} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q:\n%s", want, body)
		}
	}
	if !strings.Contains(logs.all(), "spaces: 403 AccessDenied") {
		t.Errorf("log = %q", logs.all())
	}
	i.mu.Lock()
	i.reports = nil
	i.mu.Unlock()
	if status, _, _ := post(t, ts, url.Values{"all": {"1"}}); status != http.StatusSeeOther {
		t.Fatalf("clearing again = %d", status)
	}
	s.wait()
	if _, body := getPage(t, ts); strings.Contains(body, "with failures") {
		t.Error("second clear still shown as failed")
	}
}

func TestJobOutlivesTheRequestAndIsBounded(t *testing.T) {
	s, _ := newService(&fakeCatalog{}, &slowImages{})
	s.Timeout = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := s.Start(ctx, "admin@example.com", "", true); err != nil {
		t.Fatal(err)
	}
	cancel() // the request ends at once
	s.wait()
	j, _ := s.Status()
	if j.Running || len(j.Reports) != 1 || !errors.Is(j.Reports[0].Err, context.DeadlineExceeded) {
		t.Errorf("job = %+v, want it ended by its own timeout, not the request", j)
	}
}

// slowImages clears until its context ends, then reports why.
type slowImages struct{}

func (slowImages) Clear(ctx context.Context, _ cards.Scope) []cards.TierReport {
	<-ctx.Done()
	return []cards.TierReport{{Tier: "images", Err: ctx.Err()}}
}

// An admin's check runs the catalogue's check at once, bounded, and the
// page shows what it picked up; a failed check is shown and logged.
func TestCheckNow(t *testing.T) {
	c := &fakeCatalog{result: catalog.CheckResult{Version: "fedcba9876543210", Changed: []string{"me06"}}}
	s, logs := newService(c, &fakeImages{})
	ts := serve(t, s)
	status, loc, _ := postTo(t, ts, "/admin/check", nil)
	if status != http.StatusSeeOther || loc != "/admin" {
		t.Fatalf("POST /admin/check = %d to %q", status, loc)
	}
	if c.checks != 1 {
		t.Errorf("catalogue checked %d times, want 1", c.checks)
	}
	_, body := getPage(t, ts)
	if !strings.Contains(body, "Last check now by admin@example.com") || !strings.Contains(body, "picked up version fedcba987654: changed me06, removed none") {
		t.Errorf("page lacks the check outcome:\n%s", body)
	}
	if !strings.Contains(logs.all(), "cache admin: admin@example.com checked published card sets: picked up version fedcba987654") {
		t.Errorf("log = %q", logs.all())
	}

	c.result, c.err = catalog.CheckResult{}, errors.New("GET manifest.json: 503 Service Unavailable")
	status, _, body = postTo(t, ts, "/admin/check", nil)
	if status != http.StatusBadGateway || !strings.Contains(body, "The check for published card sets failed: GET manifest.json: 503 Service Unavailable") {
		t.Errorf("failed check = %d:\n%s", status, body)
	}
	if !strings.Contains(logs.all(), "checked published card sets: GET manifest.json: 503") {
		t.Errorf("log = %q", logs.all())
	}
}

// TestRealPiecesClearOneSet runs the page over the real catalogue (a
// loader over a published directory) and the real image cache over the
// stand-in Spaces and CDN API: clearing MEG's images leaves the other
// set's copies and everything else in the bucket.
func TestRealPiecesClearOneSet(t *testing.T) {
	dir := t.TempDir()
	for _, s := range []*cardset.Set{
		builtSet("me01", "me", "MEG", "Mega Evolution", "Bulbasaur"),
		builtSet("sv1", "sv", "SVI", "Scarlet & Violet", "Pineco"),
	} {
		if err := cardset.Write(filepath.Join(dir, "sets", s.Set.ID+".json"), s); err != nil {
			t.Fatal(err)
		}
	}
	published := t.TempDir()
	if _, err := cardset.Publish(context.Background(), cardset.PublishOptions{Dir: dir, Target: cardset.DirTarget{Dir: published}, Logf: t.Logf}); err != nil {
		t.Fatal(err)
	}
	loader := &catalog.Loader{Base: published, CacheDir: t.TempDir(), Logf: t.Logf}
	loader.Start(context.Background(), 10*time.Second)

	srv := spacestest.New(t)
	sc := srv.Client(t)
	for _, rel := range []string{"en/me/me01/001", "en/sv/sv1/001"} {
		for _, f := range cards.ImageFiles {
			srv.SetObject(cards.ImagePrefix+rel+"/"+f, []byte(rel+f))
		}
	}
	srv.SetObject("cardsets/manifest.json", []byte("{}"))
	cdn := spacestest.NewCDN(t, "test-bucket.lon1.digitaloceanspaces.com", "tok")
	images := cards.NewImageCache(sc, srv.Config().PublicBase())
	purger := spaces.NewCDN(spaces.Config{Endpoint: "https://lon1.digitaloceanspaces.com", Bucket: "test-bucket"}, "tok")
	purger.API = cdn.URL
	images.CDN = purger

	s, _ := newService(loader, images)
	ts := serve(t, s)
	if status, _, body := post(t, ts, url.Values{"set": {"MEG"}}); status != http.StatusSeeOther {
		t.Fatalf("POST = %d %s", status, body)
	}
	s.wait()
	want := []string{"cardsets/manifest.json", "images/en/sv/sv1/001/high.webp", "images/en/sv/sv1/001/low.webp"}
	if got := srv.Keys(""); !slices.Equal(got, want) {
		t.Errorf("bucket after clearing MEG = %v, want %v", got, want)
	}
	if got := cdn.Purges(); len(got) != 1 || !slices.Equal(got[0], []string{"images/en/me/me01/*"}) {
		t.Errorf("purges = %v", got)
	}
	_, body := getPage(t, ts)
	for _, want := range []string{"Mega Evolution (me01)", "2 image copies removed", "CDN purged (1 paths)", "2 (2 cards)"} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q:\n%s", want, body)
		}
	}
}

// builtSet is a one-print built set that passes the set check.
func builtSet(id, serie, code, name, card string) *cardset.Set {
	return &cardset.Set{
		Format: cardset.Format,
		Set:    cardset.SetHeader{ID: id, Serie: serie, Abbreviation: code, Text: map[string]cardset.SetText{"en": {Name: name}}},
		Prints: []cardset.Print{{
			ID: id + "-001", LocalID: "001", Category: "Trainer", TrainerType: "Item",
			Images: []cardset.Image{{Source: "tcgdex", URL: "https://assets.tcgdex.net/en/" + serie + "/" + id + "/001"}},
			Text:   map[string]cardset.PrintText{"en": {Name: card, Effect: "Draw a card."}},
		}},
	}
}

// With the real catalogue loader, a set published since the last check is
// picked up at once when an admin asks for a check.
func TestRealCheckPicksUpAPublish(t *testing.T) {
	dir, published := t.TempDir(), t.TempDir()
	publish := func(s *cardset.Set) {
		t.Helper()
		if err := cardset.Write(filepath.Join(dir, "sets", s.Set.ID+".json"), s); err != nil {
			t.Fatal(err)
		}
		if _, err := cardset.Publish(context.Background(), cardset.PublishOptions{Dir: dir, Target: cardset.DirTarget{Dir: published}, Logf: t.Logf}); err != nil {
			t.Fatal(err)
		}
	}
	publish(builtSet("me01", "me", "MEG", "Mega Evolution", "Bulbasaur"))
	loader := &catalog.Loader{Base: published, CacheDir: t.TempDir(), Logf: t.Logf}
	loader.Start(context.Background(), 10*time.Second)
	before := loader.Status().Version

	publish(builtSet("sv1", "sv", "SVI", "Scarlet & Violet", "Pineco"))
	s, _ := newService(loader, nil)
	ts := serve(t, s)
	if status, _, body := postTo(t, ts, "/admin/check", nil); status != http.StatusSeeOther {
		t.Fatalf("check = %d %s", status, body)
	}
	st := loader.Status()
	if st.Version == before || st.Sets != 2 || loader.Current().Cached("sv1-001") == nil {
		t.Errorf("after the check: %+v; want the new set picked up", st)
	}
	_, body := getPage(t, ts)
	for _, want := range []string{"2 (2 cards)", "changed sv1, removed none", st.Version[:12]} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q:\n%s", want, body)
		}
	}
}
