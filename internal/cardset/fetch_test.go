package cardset

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeUpstream is a stand-in for TCGdex or the malie export. It serves
// bodies by path, counts requests per path, and can fail a number of
// times, fail for good, or be slow.
type fakeUpstream struct {
	*httptest.Server

	mu       sync.Mutex
	bodies   map[string]string
	count    map[string]int
	failures map[string]int // path -> 500s still to serve
	status   int            // if set, every request gets it
	delay    time.Duration
	agents   []string
}

func newFakeUpstream(t *testing.T) *fakeUpstream {
	t.Helper()
	f := &fakeUpstream{bodies: map[string]string{}, count: map[string]int{}, failures: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeUpstream) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.count[r.URL.Path]++
	f.agents = append(f.agents, r.UserAgent())
	body, ok := f.bodies[r.URL.Path]
	status, delay := f.status, f.delay
	failing := f.failures[r.URL.Path] > 0
	if failing {
		f.failures[r.URL.Path]--
	}
	f.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	switch {
	case status != 0:
		http.Error(w, "boom", status)
	case failing:
		http.Error(w, "boom", http.StatusInternalServerError)
	case !ok:
		http.NotFound(w, r)
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}
}

func (f *fakeUpstream) put(path, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies[path] = body
}

func (f *fakeUpstream) hits(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count[path]
}

// fastGetter retries quickly so tests do not wait out real backoffs.
var fastGetter = httpGetter{Timeout: 2 * time.Second, Backoff: []time.Duration{time.Millisecond, time.Millisecond}}

func TestTCGdexFetcher(t *testing.T) {
	f := newFakeUpstream(t)
	f.put("/sets", `[{"id":"me06","name":"Delta Reign","cardCount":{"total":141,"official":103}},{"id":"A1","name":"Genetic Apex","cardCount":{"total":286,"official":226}}]`)
	f.put("/sets/me06", `{"id":"me06","name":"Delta Reign","releaseDate":"2026-11-06","serie":{"id":"me","name":"Mega Evolution"},"abbreviation":{"official":"DLR"},"cards":[{"id":"me06-001","localId":"001","name":"Heracross"}]}`)
	f.put("/cards/me06-001", `{"id":"me06-001","name":"Heracross","hp":130}`)
	td := &TCGdex{Base: f.URL, get: fastGetter}
	ctx := context.Background()

	sets, err := td.Sets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 2 || sets[0].ID != "me06" || sets[0].CardCount.Total != 141 || sets[1].Name != "Genetic Apex" {
		t.Errorf("Sets() = %+v", sets)
	}
	set, err := td.Set(ctx, "me06")
	if err != nil {
		t.Fatal(err)
	}
	if set.Serie.ID != "me" || set.Abbreviation.Official != "DLR" || set.ReleaseDate != "2026-11-06" || len(set.Cards) != 1 || set.Cards[0].LocalID != "001" {
		t.Errorf("Set() = %+v", set)
	}
	body, err := td.Card(ctx, "me06-001")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"id":"me06-001","name":"Heracross","hp":130}` {
		t.Errorf("Card() = %s, want the raw body", body)
	}
	if _, err := td.Set(ctx, "nope"); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "/sets/nope") {
		t.Errorf("missing set: %v, want ErrNotFound naming the URL", err)
	}
	if _, err := td.Card(ctx, "me06-999"); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "/cards/me06-999") {
		t.Errorf("missing card: %v, want ErrNotFound naming the URL", err)
	}
	if f.hits("/cards/me06-999") != 1 {
		t.Errorf("a 404 was retried: %d requests", f.hits("/cards/me06-999"))
	}
	for _, ua := range f.agents {
		if ua != UserAgent {
			t.Errorf("User-Agent %q, want %q", ua, UserAgent)
		}
	}
}

func TestTCGdexCardsFansOut(t *testing.T) {
	f := newFakeUpstream(t)
	for _, id := range []string{"a-1", "a-2", "a-3"} {
		f.put("/cards/"+id, `{"id":"`+id+`"}`)
	}
	td := &TCGdex{Base: f.URL, get: fastGetter}
	bodies, errs := td.Cards(context.Background(), []string{"a-1", "a-2", "a-3", "a-4"}, 2)
	if len(bodies) != 3 || string(bodies["a-2"]) != `{"id":"a-2"}` {
		t.Errorf("bodies = %v", bodies)
	}
	if len(errs) != 1 || !errors.Is(errs["a-4"], ErrNotFound) {
		t.Errorf("errs = %v, want a-4 not found", errs)
	}
}

func TestMalieFetcher(t *testing.T) {
	f := newFakeUpstream(t)
	f.put("/index.json", `{"en-US":{"me6":{"path":"v0.1/me6.en-US.json","name":"<i>Mega Evolution—Delta Reign</i>","num":228,"hash":"x","abbr":"DLR"}},"fr-FR":{"me6":{"path":"v0.1/me6.fr-FR.json","abbr":"DLR"}}}`)
	f.put("/v0.1/me6.en-US.json", `[{"name":"Heracross","card_type":"POKEMON","stage":"BASIC","hp":130,"types":["GRASS"],"retreat":3,"regulation_mark":"J",
		"weakness":{"types":["FIRE"],"operator":"×","amount":2},"resistance":null,
		"collector_number":{"full":"001/103","numeric":1},
		"text":[{"kind":"ATTACK","name":"Spike Draw","text":"Draw 2 cards.","cost":["GRASS"],"damage":{"amount":20}},{"kind":"ATTACK","name":"Rage","cost":["GRASS"],"damage":{"amount":"30","suffix":"+"}}],
		"ext":{"tcgl":{"cardID":"me6_1","longFormID":"Heracross_me6_1_std_Common_NonFoil_None","key":"me6"}},
		"images":{"tcgl":{"jpg":{"front":"https://cdn.malie.io/x/me6_en_001_std.jpg"}}}}]`)
	m := &Malie{Base: f.URL, get: fastGetter}
	ctx := context.Background()
	idx, err := m.Index(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx) != 1 || idx["me6"].Path != "v0.1/me6.en-US.json" || idx["me6"].Abbr != "DLR" || idx["me6"].Num != 228 {
		t.Errorf("Index() = %+v, want the en-US sets only", idx)
	}
	cards, err := m.SetFile(ctx, idx["me6"].Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 {
		t.Fatalf("SetFile() returned %d cards", len(cards))
	}
	c := cards[0]
	if c.Name != "Heracross" || c.CardType != "POKEMON" || c.HP != 130 || c.Retreat != 3 || c.RegulationMark != "J" ||
		c.Number.Numeric != 1 || c.Ext.TCGL.CardID != "me6_1" || !strings.Contains(c.Ext.TCGL.LongFormID, "_std_") ||
		c.Images.TCGL.JPG.Front != "https://cdn.malie.io/x/me6_en_001_std.jpg" || c.Resistance != nil ||
		c.Weakness == nil || c.Weakness.Amount != 2 || c.Weakness.Operator != "×" {
		t.Errorf("card = %+v", c)
	}
	if len(c.Text) != 2 || c.Text[0].Damage.Amount != 20 || c.Text[1].Damage.Amount != 30 || c.Text[1].Damage.Suffix != "+" {
		t.Errorf("text = %+v", c.Text)
	}
	if _, err := m.SetFile(ctx, "v0.1/none.json"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing set file: %v, want ErrNotFound", err)
	}
}

func TestFetchRetries(t *testing.T) {
	ctx := context.Background()
	t.Run("passing failures are retried", func(t *testing.T) {
		f := newFakeUpstream(t)
		f.put("/sets", `[]`)
		f.failures["/sets"] = 2
		td := &TCGdex{Base: f.URL, get: fastGetter}
		if _, err := td.Sets(ctx); err != nil {
			t.Fatalf("Sets() = %v after two 500s, want success on the third try", err)
		}
		if n := f.hits("/sets"); n != 3 {
			t.Errorf("%d requests, want 3", n)
		}
	})
	t.Run("lasting failure gives up", func(t *testing.T) {
		f := newFakeUpstream(t)
		f.status = http.StatusInternalServerError
		td := &TCGdex{Base: f.URL, get: fastGetter}
		_, err := td.Set(ctx, "me06")
		if err == nil || !strings.Contains(err.Error(), "/sets/me06") || !strings.Contains(err.Error(), "3 attempts") {
			t.Errorf("Set() = %v, want an error naming the URL after 3 attempts", err)
		}
		if n := f.hits("/sets/me06"); n != 3 {
			t.Errorf("%d requests, want 3", n)
		}
	})
	t.Run("empty body is retried", func(t *testing.T) {
		f := newFakeUpstream(t)
		f.put("/sets", ``)
		td := &TCGdex{Base: f.URL, get: fastGetter}
		if _, err := td.Sets(ctx); err == nil || !strings.Contains(err.Error(), "/sets") {
			t.Errorf("Sets() = %v, want an error naming the URL", err)
		}
		if n := f.hits("/sets"); n != 3 {
			t.Errorf("%d requests, want 3", n)
		}
	})
	t.Run("slow request times out", func(t *testing.T) {
		f := newFakeUpstream(t)
		f.put("/sets", `[]`)
		f.delay = 2 * time.Second
		td := &TCGdex{Base: f.URL, get: httpGetter{Timeout: 50 * time.Millisecond, Backoff: []time.Duration{time.Millisecond, time.Millisecond}}}
		start := time.Now()
		_, err := td.Sets(ctx)
		if err == nil || !strings.Contains(err.Error(), "/sets") {
			t.Errorf("Sets() = %v, want a timeout naming the URL", err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("took %s: the timeout was not honoured", d)
		}
		if n := f.hits("/sets"); n != 3 {
			t.Errorf("%d requests, want 3", n)
		}
	})
	t.Run("cancelled context stops retrying", func(t *testing.T) {
		f := newFakeUpstream(t)
		f.status = http.StatusBadGateway
		td := &TCGdex{Base: f.URL, get: httpGetter{Timeout: time.Second, Backoff: []time.Duration{time.Hour}}}
		cctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		if _, err := td.Sets(cctx); err == nil {
			t.Error("Sets() succeeded")
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("took %s: the retry pause ignored the context", d)
		}
	})
}

func TestFlexInt(t *testing.T) {
	for in, want := range map[string]flexInt{`12`: 12, `"30"`: 30, `null`: 0, `""`: 0} {
		var n flexInt
		if err := n.UnmarshalJSON([]byte(in)); err != nil || n != want {
			t.Errorf("flexInt(%s) = %d, %v; want %d", in, n, err, want)
		}
	}
	var n flexInt
	if err := n.UnmarshalJSON([]byte(`"x"`)); err == nil {
		t.Error("flexInt accepted a non-number")
	}
}
