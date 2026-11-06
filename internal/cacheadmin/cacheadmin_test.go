package cacheadmin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
	"github.com/nicholasjackson/tcgreplay/internal/spaces"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// fakeCards names me01 by MEG, me01 or ME01, records the scopes it is
// cleared with, and can hold a clear until released.
type fakeCards struct {
	mu      sync.Mutex
	scopes  []cards.Scope
	reports []cards.TierReport
	hold    chan struct{}
}

func (f *fakeCards) SetID(_ context.Context, name string) (string, string, error) {
	switch name {
	case "MEG", "me01", "ME01":
		return "me01", "Mega Evolution", nil
	}
	return "", "", fmt.Errorf("%q: %w", name, cards.ErrUnknownSet)
}

func (f *fakeCards) Clear(ctx context.Context, s cards.Scope) []cards.TierReport {
	f.mu.Lock()
	f.scopes = append(f.scopes, s)
	hold, reports := f.hold, f.reports
	f.mu.Unlock()
	if hold != nil {
		<-hold
	}
	if reports == nil {
		reports = []cards.TierReport{{Tier: "shared", Removed: 5}, {Tier: "disk", Removed: 5}, {Tier: "memory", Removed: 2}}
	}
	return reports
}

func (f *fakeCards) cleared() []cards.Scope {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.scopes)
}

type fakeImages struct {
	mu     sync.Mutex
	scopes []cards.Scope
}

func (f *fakeImages) Clear(_ context.Context, s cards.Scope) []cards.TierReport {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scopes = append(f.scopes, s)
	return []cards.TierReport{{Tier: "images", Removed: 4}, {Tier: "cdn", Err: cards.ErrCDNNotConfigured}}
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
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.PostForm(ts.URL+"/admin/clear", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Location"), string(b)
}

func newService(c Cards, i Images) (*Service, *logLines) {
	l := &logLines{}
	s := &Service{Cards: c, Logf: l.logf}
	if i != nil {
		s.Images = i
	}
	return s, l
}

func TestPageShowsTheAdminAndBothActions(t *testing.T) {
	s, _ := newService(&fakeCards{}, nil)
	status, body := getPage(t, serve(t, s))
	for _, want := range []string{"Signed in as admin@example.com", `name="all" value="1"`, "Clear everything", `name="set"`, "MEG or me01", "No clear since the playmat started", `<a href="/board">Back to the board</a>`} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if status != http.StatusOK || strings.Contains(body, `http-equiv="refresh"`) {
		t.Errorf("status %d, or refreshing with no clear running", status)
	}
}

func TestClearEverythingClearsCardsAndImagesAndShowsEachTier(t *testing.T) {
	c, i := &fakeCards{}, &fakeImages{}
	s, logs := newService(c, i)
	ts := serve(t, s)
	status, loc, _ := post(t, ts, url.Values{"all": {"1"}})
	if status != http.StatusSeeOther || loc != "/admin" {
		t.Fatalf("POST all = %d to %q, want 303 to /admin", status, loc)
	}
	s.wait()
	if got := c.cleared(); !slices.Equal(got, []cards.Scope{{}}) {
		t.Errorf("cards cleared %v", got)
	}
	if !slices.Equal(i.scopes, []cards.Scope{{}}) {
		t.Errorf("images cleared %v", i.scopes)
	}
	_, body := getPage(t, ts)
	for _, want := range []string{"Last clear", "everything by admin@example.com", "5 card entries removed from the shared store", "5 card entries removed from local disk", "2 card entries marked for refetch in memory", "4 image copies removed", "CDN not purged (no TCGREPLAY_SPACES_CDN_TOKEN)"} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "with failures") || strings.Contains(body, `http-equiv="refresh"`) {
		t.Error("finished clear shown as failed or still refreshing")
	}
	log := logs.all()
	if !strings.Contains(log, "cache admin: admin@example.com started clearing everything") || !strings.Contains(log, "cache admin: cleared everything for admin@example.com in ") {
		t.Errorf("log = %q", log)
	}
}

func TestClearOneSetByCodeOrID(t *testing.T) {
	for _, name := range []string{"MEG", "me01", "ME01", " MEG "} {
		c := &fakeCards{}
		i := &fakeImages{}
		s, logs := newService(c, i)
		ts := serve(t, s)
		if status, _, body := post(t, ts, url.Values{"set": {name}}); status != http.StatusSeeOther {
			t.Fatalf("POST set=%q = %d %s", name, status, body)
		}
		s.wait()
		if got := c.cleared(); !slices.Equal(got, []cards.Scope{{Set: "me01"}}) || !slices.Equal(i.scopes, []cards.Scope{{Set: "me01"}}) {
			t.Errorf("set=%q cleared cards %v images %v, want me01", name, got, i.scopes)
		}
		if _, body := getPage(t, ts); !strings.Contains(body, "Mega Evolution (me01) by admin@example.com") {
			t.Errorf("page does not name the set")
		}
		if !strings.Contains(logs.all(), "started clearing Mega Evolution (me01)") {
			t.Errorf("log = %q", logs.all())
		}
	}
}

func TestUnknownOrEmptySetIsRefusedAndNothingCleared(t *testing.T) {
	c, i := &fakeCards{}, &fakeImages{}
	s, _ := newService(c, i)
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
	if len(c.cleared()) != 0 || len(i.scopes) != 0 {
		t.Errorf("cleared cards %v images %v, want nothing", c.cleared(), i.scopes)
	}
}

func TestOneClearAtATime(t *testing.T) {
	c := &fakeCards{hold: make(chan struct{})}
	s, _ := newService(c, nil)
	ts := serve(t, s)
	if status, _, _ := post(t, ts, url.Values{"all": {"1"}}); status != http.StatusSeeOther {
		t.Fatal(status)
	}
	_, body := getPage(t, ts)
	if !strings.Contains(body, `<meta http-equiv="refresh" content="2">`) || !strings.Contains(body, "Clearing everything for admin@example.com") {
		t.Errorf("running page:\n%s", body)
	}
	status, _, body := post(t, ts, url.Values{"set": {"MEG"}})
	if status != http.StatusConflict || !strings.Contains(body, "A clear is already running") {
		t.Errorf("second clear = %d", status)
	}
	if _, err := s.Start(context.Background(), "x", "", true); !errors.Is(err, ErrBusy) {
		t.Errorf("Start while running = %v, want ErrBusy", err)
	}
	close(c.hold)
	s.wait()
	if _, body := getPage(t, ts); strings.Contains(body, `http-equiv="refresh"`) {
		t.Error("page still refreshes after the clear ended")
	}
	if len(c.cleared()) != 1 {
		t.Errorf("cleared %v, want one clear", c.cleared())
	}
	// Once it has ended another may start.
	if status, _, _ := post(t, ts, url.Values{"set": {"me01"}}); status != http.StatusSeeOther {
		t.Errorf("clear after the first ended = %d", status)
	}
	s.wait()
}

func TestAFailedTierIsShownAndClearingAgainIsAllowed(t *testing.T) {
	c := &fakeCards{reports: []cards.TierReport{{Tier: "shared", Err: errors.New("spaces: 403 AccessDenied: no")}, {Tier: "disk", Removed: 3}, {Tier: "memory", Removed: 1}}}
	s, logs := newService(c, nil)
	ts := serve(t, s)
	post(t, ts, url.Values{"all": {"1"}})
	s.wait()
	_, body := getPage(t, ts)
	for _, want := range []string{"with failures", "0 card entries removed from the shared store, then failed: spaces: 403 AccessDenied: no", "3 card entries removed from local disk"} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q:\n%s", want, body)
		}
	}
	if !strings.Contains(logs.all(), "error: spaces: 403 AccessDenied") {
		t.Errorf("log = %q", logs.all())
	}
	c.mu.Lock()
	c.reports = nil
	c.mu.Unlock()
	if status, _, _ := post(t, ts, url.Values{"all": {"1"}}); status != http.StatusSeeOther {
		t.Fatalf("clearing again = %d", status)
	}
	s.wait()
	if _, body := getPage(t, ts); strings.Contains(body, "with failures") {
		t.Error("second clear still shown as failed")
	}
}

func TestJobOutlivesTheRequestAndIsBounded(t *testing.T) {
	c := &slowCards{}
	s, _ := newService(c, nil)
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

// slowCards clears until its context ends, then reports why.
type slowCards struct{ fakeCards }

func (s *slowCards) Clear(ctx context.Context, _ cards.Scope) []cards.TierReport {
	<-ctx.Done()
	return []cards.TierReport{{Tier: "shared", Err: ctx.Err()}}
}

// TestRealPiecesClearOneSet runs the page over the real card database (a
// tiered store over a temp directory and the stand-in Spaces, against a
// stand-in TCGdex) and the real image cache with the stand-in CDN API.
func TestRealPiecesClearOneSet(t *testing.T) {
	src := newTCGdex(t)
	srv := spacestest.New(t)
	sc := srv.Client(t)
	dir := t.TempDir()
	db := cards.NewClient(dir)
	db.Base = src.URL
	db.Store = cards.NewTieredStore(cards.NewDiskStore(dir), cards.NewSpacesStore(sc))
	ctx := context.Background()
	for _, id := range []string{"me01-001", "sv1-001"} {
		if _, err := db.Card(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{"en/me/me01/001", "en/sv/sv1/1"} {
		for _, f := range cards.ImageFiles {
			srv.SetObject(cards.ImagePrefix+rel+"/"+f, []byte(rel+f))
		}
	}
	cdn := spacestest.NewCDN(t, "test-bucket.lon1.digitaloceanspaces.com", "tok")
	images := cards.NewImageCache(sc, srv.Config().PublicBase())
	purger := spaces.NewCDN(spaces.Config{Endpoint: "https://lon1.digitaloceanspaces.com", Bucket: "test-bucket"}, "tok")
	purger.API = cdn.URL
	images.CDN = purger

	s, _ := newService(db, images)
	ts := serve(t, s)
	if status, _, body := post(t, ts, url.Values{"set": {"MEG"}}); status != http.StatusSeeOther {
		t.Fatalf("POST = %d %s", status, body)
	}
	s.wait()
	want := []string{
		"images/en/sv/sv1/1/high.webp", "images/en/sv/sv1/1/low.webp",
		"tcgdex/cards/sv1-001.json", "tcgdex/sets.json", "tcgdex/sets/sv1.json",
	}
	if got := srv.Keys(""); !slices.Equal(got, want) {
		t.Errorf("bucket after clearing MEG = %v, want %v", got, want)
	}
	if got := cdn.Purges(); len(got) != 1 || !slices.Equal(got[0], []string{"images/en/me/me01/*"}) {
		t.Errorf("purges = %v", got)
	}
	_, body := getPage(t, ts)
	for _, want := range []string{"Mega Evolution (me01)", "2 card entries removed from the shared store", "2 image copies removed", "CDN purged (1 paths)"} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q:\n%s", want, body)
		}
	}
}

// newTCGdex is a stand-in TCGdex with sets me01 (code MEG) and sv1 (SVI),
// one card each.
func newTCGdex(t *testing.T) *httptest.Server {
	t.Helper()
	bodies := map[string]string{
		"/sets":           `[{"id":"me01"},{"id":"sv1"}]`,
		"/sets/me01":      `{"id":"me01","name":"Mega Evolution","releaseDate":"2025-09-26","serie":{"id":"me"},"abbreviation":{"official":"MEG"},"cards":[{"id":"me01-001","localId":"001","name":"Bulbasaur"}]}`,
		"/sets/sv1":       `{"id":"sv1","name":"Scarlet & Violet","releaseDate":"2023-03-31","serie":{"id":"sv"},"abbreviation":{"official":"SVI"},"cards":[{"id":"sv1-001","localId":"001","name":"Pineco"}]}`,
		"/cards/me01-001": `{"id":"me01-001","localId":"001","name":"Bulbasaur","category":"Pokemon","hp":70,"image":"https://assets.tcgdex.net/en/me/me01/001","set":{"id":"me01","name":"Mega Evolution"}}`,
		"/cards/sv1-001":  `{"id":"sv1-001","localId":"001","name":"Pineco","category":"Pokemon","hp":70,"image":"https://assets.tcgdex.net/en/sv/sv1/1","set":{"id":"sv1","name":"Scarlet & Violet"}}`,
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := bodies[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, b)
	}))
	t.Cleanup(ts.Close)
	return ts
}
