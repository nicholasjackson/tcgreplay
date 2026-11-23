package observe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/nicholasjackson/tcgreplay/internal/observe/observetest"
)

// promRow is one row of a canned instant vector.
type promRow struct {
	labels map[string]string
	value  string
}

// promReq is one query the fake Prometheus API was sent.
type promReq struct {
	path, kind, query, auth string
	at                      int64
}

// promFake is a fake Grafana Cloud Prometheus query API. It answers each
// query from canned vectors keyed by what the query reads and its time,
// with an empty vector for anything not canned, and keeps every request.
type promFake struct {
	srv *httptest.Server

	mu       sync.Mutex
	readBack []promRow
	answers  map[string][]promRow // "<kind>@<unix time>"
	fail     map[string]int       // kind -> HTTP status; "*" fails every query
	reqs     []promReq
}

func newPromFake(t *testing.T, useTLS bool) *promFake {
	t.Helper()
	p := &promFake{answers: map[string][]promRow{}, fail: map[string]int{}}
	if useTLS {
		p.srv = httptest.NewTLSServer(http.HandlerFunc(p.serve))
	} else {
		p.srv = httptest.NewServer(http.HandlerFunc(p.serve))
	}
	t.Cleanup(p.srv.Close)
	return p
}

// promKind says what a query reads, by the metric it names: the exported
// daily table, the "any counter at all" check, or one of the six counts.
func promKind(q string) string {
	switch {
	case strings.Contains(q, "tcgreplay_usage_daily"):
		return "readback"
	case strings.Contains(q, "__name__"):
		return "present"
	case strings.Contains(q, "tcgreplay_player_days_total"):
		return "players"
	case strings.Contains(q, "tcgreplay_sign_ins_total"):
		return "sign_ins"
	case strings.Contains(q, "tcgreplay_tool_calls_total"):
		return "tool_calls"
	case strings.Contains(q, "tcgreplay_imports_total"):
		return "imports"
	case strings.Contains(q, "tcgreplay_new_games_total"):
		return "new_games"
	case strings.Contains(q, "tcgreplay_errors_total"):
		return "errors"
	}
	return "unknown"
}

func (p *promFake) serve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("query")
	at, _ := strconv.ParseInt(r.URL.Query().Get("time"), 10, 64)
	kind := promKind(q)
	p.mu.Lock()
	p.reqs = append(p.reqs, promReq{path: r.URL.Path, kind: kind, query: q, auth: r.Header.Get("Authorization"), at: at})
	status := p.fail[kind]
	if s := p.fail["*"]; s != 0 {
		status = s
	}
	rows := p.answers[kind+"@"+strconv.FormatInt(at, 10)]
	if kind == "readback" {
		rows = p.readBack
	}
	p.mu.Unlock()
	if status != 0 {
		http.Error(w, "nope", status)
		return
	}
	result := []map[string]any{}
	for _, row := range rows {
		result = append(result, map[string]any{"metric": row.labels, "value": []any{at, row.value}})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": result}})
}

func (p *promFake) answer(kind string, at time.Time, rows ...promRow) {
	p.mu.Lock()
	p.answers[kind+"@"+strconv.FormatInt(at.Unix(), 10)] = rows
	p.mu.Unlock()
}

func (p *promFake) setFail(kind string, status int) {
	p.mu.Lock()
	p.fail[kind] = status
	p.mu.Unlock()
}

func (p *promFake) setReadBack(rows ...promRow) {
	p.mu.Lock()
	p.readBack = rows
	p.mu.Unlock()
}

func (p *promFake) requests() []promReq {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]promReq(nil), p.reqs...)
}

func (p *promFake) reset() {
	p.mu.Lock()
	p.reqs = nil
	p.mu.Unlock()
}

func row(value string, kv ...string) promRow {
	l := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		l[kv[i]] = kv[i+1]
	}
	return promRow{labels: l, value: value}
}

// utc is midnight UTC at the start of the given day.
func utc(day string) time.Time {
	d, err := time.Parse("2006-01-02", day)
	if err != nil {
		panic(err)
	}
	return d
}

// historyClock is 2026-11-23 12:00 UTC: today is 2026-11-23, the raw
// counters cover the completed days 2026-11-10 to 2026-11-22, and six
// weeks back keeps 2026-10-12 onwards.
var historyNow = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

// base64("654321:glc_SECRETTOKENvalue123"), worked out by hand.
const historyAuth = "Basic NjU0MzIxOmdsY19TRUNSRVRUT0tFTnZhbHVlMTIz"

func testHistory(p *promFake, c *clock, out *syncBuf) *history {
	return newHistory(p.srv.URL+"/api/prom", "654321", testToken, p.srv.Client().Transport, c.now, newHealth(c.now, out))
}

func (h *history) snapshot() map[dayKey]float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[dayKey]float64{}
	for k, v := range h.table {
		out[k] = v
	}
	return out
}

func diffTable(t *testing.T, got, want map[dayKey]float64) {
	t.Helper()
	for k, v := range want {
		g, ok := got[k]
		if !ok {
			t.Errorf("table lacks %+v (want %v)", k, v)
		} else if g != v {
			t.Errorf("table %+v = %v, want %v", k, g, v)
		}
	}
	for k, v := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected table row %+v = %v", k, v)
		}
	}
}

// TestHistoryRefreshMergesComputesAndDrops: the table read back is merged
// with what the playmat holds by the larger value, each completed day with
// raw data that the table lacks is computed at the end of that day (0 for
// an unsplit count with no increase, one row per provider or assistant),
// days with no raw data are skipped, days past six weeks are dropped, and
// a second run gives the same table without recomputing any day.
func TestHistoryRefreshMergesComputesAndDrops(t *testing.T) {
	p := newPromFake(t, false)
	c := &clock{t: historyNow}
	var out syncBuf
	h := testHistory(p, c, &out)
	h.table[dayKey{Day: "2026-11-01", Count: "players"}] = 9
	h.table[dayKey{Day: "2026-11-02", Count: "players"}] = 3

	p.setReadBack(
		row("5", "day", "2026-11-01", "count", "players"),
		row("4", "day", "2026-11-02", "count", "players"),
		row("12", "day", "2026-11-21", "count", "players"),
		row("3", "day", "2026-11-21", "count", "sign_ins", "provider", "google"),
		row("2", "day", "2026-10-11", "count", "players"), // 43 days back
		row("1", "day", "2026-10-12", "count", "errors"),  // 42 days back
		row("99", "count", "players"),                     // no day: ignored
	)
	end22, end20, end19 := utc("2026-11-23"), utc("2026-11-21"), utc("2026-11-20")
	p.answer("present", end22, row("4"))
	p.answer("players", end22, row("6"))
	p.answer("sign_ins", end22, row("4", "provider", "google"), row("1", "provider", "password"))
	p.answer("tool_calls", end22, row("30", "assistant", "Claude"), row("2", "assistant", "other"))
	p.answer("imports", end22, row("3", "assistant", "Claude"))
	// new_games and errors on 2026-11-22: empty vectors.
	p.answer("present", end20, row("1"))
	p.answer("players", end20, row("1"))
	p.answer("present", end19, row("0")) // counters exported, all zero: skipped

	want := map[dayKey]float64{
		{Day: "2026-11-01", Count: "players"}:                         9,
		{Day: "2026-11-02", Count: "players"}:                         4,
		{Day: "2026-11-21", Count: "players"}:                         12,
		{Day: "2026-11-21", Count: "sign_ins", Provider: "google"}:    3,
		{Day: "2026-10-12", Count: "errors"}:                          1,
		{Day: "2026-11-22", Count: "players"}:                         6,
		{Day: "2026-11-22", Count: "sign_ins", Provider: "google"}:    4,
		{Day: "2026-11-22", Count: "sign_ins", Provider: "password"}:  1,
		{Day: "2026-11-22", Count: "tool_calls", Assistant: "Claude"}: 30,
		{Day: "2026-11-22", Count: "tool_calls", Assistant: "other"}:  2,
		{Day: "2026-11-22", Count: "imports", Assistant: "Claude"}:    3,
		{Day: "2026-11-22", Count: "errors"}:                          0,
		{Day: "2026-11-20", Count: "players"}:                         1,
		{Day: "2026-11-20", Count: "errors"}:                          0,
	}

	if err := h.refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	diffTable(t, h.snapshot(), want)

	reqs := p.requests()
	if len(reqs) != 25 {
		t.Errorf("first refresh sent %d queries, want 25 (1 read-back, 12 day checks, 6 counts for each of 2 days)", len(reqs))
	}
	presentAt := map[int64]bool{}
	countsAt := map[int64][]string{}
	for _, r := range reqs {
		if r.path != "/api/prom/api/v1/query" {
			t.Errorf("query sent to %s, want /api/prom/api/v1/query", r.path)
		}
		if r.auth != historyAuth {
			t.Errorf("%s query Authorization = %q, want %q", r.kind, r.auth, historyAuth)
		}
		switch r.kind {
		case "readback":
			if r.at != historyNow.Unix() {
				t.Errorf("read-back at %d, want now (%d)", r.at, historyNow.Unix())
			}
		case "present":
			presentAt[r.at] = true
		case "unknown":
			t.Errorf("unexpected query %q", r.query)
		default:
			countsAt[r.at] = append(countsAt[r.at], r.kind)
			if !strings.Contains(r.query, "[1d]") {
				t.Errorf("%s query %q does not cover one day", r.kind, r.query)
			}
			switch r.kind {
			case "sign_ins":
				if !strings.Contains(r.query, "provider") {
					t.Errorf("sign-ins query %q is not split by provider", r.query)
				}
			case "tool_calls", "imports", "new_games":
				if !strings.Contains(r.query, "assistant") {
					t.Errorf("%s query %q is not split by assistant", r.kind, r.query)
				}
			}
		}
	}
	for d := utc("2026-11-10"); !d.After(utc("2026-11-22")); d = d.AddDate(0, 0, 1) {
		end := d.AddDate(0, 0, 1).Unix()
		if wantCheck := d.Format("2006-01-02") != "2026-11-21"; presentAt[end] != wantCheck {
			t.Errorf("day %s checked for raw data = %v, want %v", d.Format("2006-01-02"), presentAt[end], wantCheck)
		}
	}
	if presentAt[utc("2026-11-10").Unix()] || presentAt[utc("2026-11-24").Unix()] {
		t.Error("checked a day outside the 13 completed days the raw counters cover")
	}
	allCounts := []string{"errors", "imports", "new_games", "players", "sign_ins", "tool_calls"}
	for _, end := range []time.Time{end22, end20} {
		got := countsAt[end.Unix()]
		sort.Strings(got)
		if strings.Join(got, ",") != strings.Join(allCounts, ",") {
			t.Errorf("counts queried at %s = %v, want %v", end.Format(time.RFC3339), got, allCounts)
		}
	}
	if len(countsAt) != 2 {
		t.Errorf("counts queried at %d times, want only the ends of 2026-11-20 and 2026-11-22", len(countsAt))
	}

	// Again: same table, and only the days still without data are checked.
	p.reset()
	if err := h.refresh(context.Background()); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	diffTable(t, h.snapshot(), want)
	reqs = p.requests()
	if len(reqs) != 11 {
		t.Errorf("second refresh sent %d queries, want 11 (1 read-back, 10 day checks)", len(reqs))
	}
	for _, r := range reqs {
		if r.kind != "readback" && r.kind != "present" {
			t.Errorf("second refresh recomputed a day: %s query at %d", r.kind, r.at)
		}
		if r.kind == "present" && (r.at == end22.Unix() || r.at == end20.Unix() || r.at == utc("2026-11-22").Unix()) {
			t.Errorf("second refresh checked a day the table already holds (end %d)", r.at)
		}
	}
}

// TestHistoryKeepsTableWhenQueryAPIFails: with the query API rejecting or
// unreachable, refresh fails, the table and the gauge stay as they were,
// and the failure counts towards the five-minute warning, naming history.
func TestHistoryKeepsTableWhenQueryAPIFails(t *testing.T) {
	reject := newPromFake(t, false)
	reject.setFail("*", http.StatusInternalServerError)
	for _, tc := range []struct {
		name, endpoint string
		base           http.RoundTripper
		kind           string
	}{
		{"rejecting", reject.srv.URL + "/api/prom", reject.srv.Client().Transport, "rejected (HTTP 500)"},
		{"unreachable", observetest.Unreachable(t), nil, "unreachable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &clock{t: historyNow}
			var out syncBuf
			hl := newHealth(c.now, &out)
			h := newHistory(tc.endpoint, "654321", testToken, tc.base, c.now, hl)
			before := map[dayKey]float64{
				{Day: "2026-11-21", Count: "players"}:                         7,
				{Day: "2026-11-21", Count: "tool_calls", Assistant: "Claude"}: 11,
			}
			for k, v := range before {
				h.table[k] = v
			}
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() { mp.Shutdown(context.Background()) })
			if err := h.register(mp.Meter("test")); err != nil {
				t.Fatal(err)
			}

			if err := h.refresh(context.Background()); err == nil {
				t.Fatal("refresh succeeded against a failing query API")
			}
			diffTable(t, h.snapshot(), before)
			got := gaugePoints(t, reader)
			for _, k := range []string{"assistant=Claude,count=tool_calls,day=2026-11-21", "count=players,day=2026-11-21"} {
				if _, ok := got[k]; !ok {
					t.Errorf("gauge no longer reports %s after the failure; reports %v", k, got)
				}
			}

			hl.check(historyNow.Add(4*time.Minute + 59*time.Second))
			if out.String() != "" {
				t.Fatalf("warned before five minutes: %q", out.String())
			}
			hl.check(historyNow.Add(5 * time.Minute))
			want := "warning: telemetry export: sending to Grafana Cloud has failed for 5m (history: " + tc.kind + ")\n"
			if out.String() != want {
				t.Errorf("warning = %q\nwant      %q", out.String(), want)
			}
			if strings.Contains(out.String(), testToken) {
				t.Error("warning carries the token")
			}
		})
	}
}

// TestHistoryFailedDayIsTriedAgain: a count query failing part-way through
// a day leaves that day out (the rows read back stay) and the next refresh
// computes it, clearing the failure so no warning follows.
func TestHistoryFailedDayIsTriedAgain(t *testing.T) {
	p := newPromFake(t, false)
	c := &clock{t: historyNow}
	var out syncBuf
	h := testHistory(p, c, &out)
	p.setReadBack(row("2", "day", "2026-11-15", "count", "players"))
	end22 := utc("2026-11-23")
	p.answer("present", end22, row("3"))
	p.answer("players", end22, row("5"))
	p.answer("imports", end22, row("2", "assistant", "Gemini"))
	p.setFail("imports", http.StatusServiceUnavailable)

	if err := h.refresh(context.Background()); err == nil {
		t.Fatal("refresh succeeded with the imports query failing")
	}
	diffTable(t, h.snapshot(), map[dayKey]float64{{Day: "2026-11-15", Count: "players"}: 2})

	p.setFail("imports", 0)
	c.set(historyNow.Add(time.Minute))
	if err := h.refresh(context.Background()); err != nil {
		t.Fatalf("refresh after recovery: %v", err)
	}
	diffTable(t, h.snapshot(), map[dayKey]float64{
		{Day: "2026-11-15", Count: "players"}:                      2,
		{Day: "2026-11-22", Count: "players"}:                      5,
		{Day: "2026-11-22", Count: "imports", Assistant: "Gemini"}: 2,
		{Day: "2026-11-22", Count: "errors"}:                       0,
	})
	h.health.check(historyNow.Add(10 * time.Minute))
	if out.String() != "" {
		t.Errorf("warned after the history recovered: %q", out.String())
	}
}

// gaugePoints collects the daily gauge as "k=v,..." (sorted) -> value.
func gaugePoints(t *testing.T, r *sdkmetric.ManualReader) map[string]float64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := r.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	out := map[string]float64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "tcgreplay.usage_daily" {
				continue
			}
			if m.Unit != "{count}" {
				t.Errorf("gauge unit = %q, want {count}", m.Unit)
			}
			g, ok := m.Data.(metricdata.Gauge[float64])
			if !ok {
				t.Fatalf("tcgreplay.usage_daily is %T, want a float64 gauge", m.Data)
			}
			for _, dp := range g.DataPoints {
				var kv []string
				for _, a := range dp.Attributes.ToSlice() {
					kv = append(kv, string(a.Key)+"="+a.Value.AsString())
				}
				sort.Strings(kv)
				k := strings.Join(kv, ",")
				if _, dup := out[k]; dup {
					t.Errorf("two points for %s", k)
				}
				out[k] = dp.Value
			}
		}
	}
	return out
}

// TestHistoryGaugeReportsEveryRow: the daily gauge reports one point per
// table row, labelled by day and count, plus provider or assistant only
// for the split counts.
func TestHistoryGaugeReportsEveryRow(t *testing.T) {
	c := &clock{t: historyNow}
	h := newHistory("https://prom.example/api/prom", "654321", testToken, nil, c.now, newHealth(c.now, nil))
	for k, v := range map[dayKey]float64{
		{Day: "2026-11-22", Count: "players"}:                         6,
		{Day: "2026-11-22", Count: "sign_ins", Provider: "google"}:    4,
		{Day: "2026-11-22", Count: "tool_calls", Assistant: "Claude"}: 30,
		{Day: "2026-11-22", Count: "imports", Assistant: "Claude"}:    3,
		{Day: "2026-11-22", Count: "new_games", Assistant: "Codex"}:   1,
		{Day: "2026-11-22", Count: "errors"}:                          0,
		{Day: "2026-11-21", Count: "players"}:                         12,
	} {
		h.table[k] = v
	}
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { mp.Shutdown(context.Background()) })
	if err := h.register(mp.Meter(scope)); err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{
		"count=players,day=2026-11-22":                     6,
		"count=sign_ins,day=2026-11-22,provider=google":    4,
		"assistant=Claude,count=tool_calls,day=2026-11-22": 30,
		"assistant=Claude,count=imports,day=2026-11-22":    3,
		"assistant=Codex,count=new_games,day=2026-11-22":   1,
		"count=errors,day=2026-11-22":                      0,
		"count=players,day=2026-11-21":                     12,
	}
	got := gaugePoints(t, reader)
	for k, v := range want {
		if g, ok := got[k]; !ok || g != v {
			t.Errorf("gauge point {%s} = %v (present %v), want %v", k, g, ok, v)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected gauge point {%s}", k)
		}
	}
}

// TestStartKeepsHistoryOnlyWithMetricsSettings: the usage history is on
// only with export on and both metrics settings valid; export itself does
// not depend on them.
func TestStartKeepsHistoryOnlyWithMetricsSettings(t *testing.T) {
	r := observetest.New(t)
	for _, tc := range []struct {
		name             string
		metricsURL, user string
		export           bool
		enabled, history bool
	}{
		{"export only", "", "", true, true, false},
		{"export and metrics", "https://prom.example/api/prom", "654321", true, true, true},
		{"metrics user not a number", "https://prom.example/api/prom", "stack", true, true, false},
		{"metrics without export", "https://prom.example/api/prom", "654321", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Settings{MetricsURL: tc.metricsURL, MetricsUser: tc.user, HTTPClient: r.Client()}
			if tc.export {
				s = on(r.URL, r.Client())
				s.MetricsURL, s.MetricsUser = tc.metricsURL, tc.user
			}
			tel := Start(context.Background(), s)
			t.Cleanup(func() { tel.Shutdown(context.Background()) })
			if tel.Enabled() != tc.enabled {
				t.Errorf("Enabled() = %v, want %v", tel.Enabled(), tc.enabled)
			}
			if tel.HistoryOn() != tc.history {
				t.Errorf("HistoryOn() = %v, want %v", tel.HistoryOn(), tc.history)
			}
		})
	}
}

// TestStartWithoutHistorySendsEverythingElse: with export on and no
// metrics settings, spans, log records and counters still reach Grafana,
// and no daily gauge is sent.
func TestStartWithoutHistorySendsEverythingElse(t *testing.T) {
	r := observetest.New(t)
	tel := Start(context.Background(), on(r.URL, r.Client()))
	t.Cleanup(func() { tel.Shutdown(context.Background()) })
	sendOne(tel)
	if err := tel.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
	paths := strings.Join(r.Paths(), " ")
	for _, want := range []string{"/otlp/v1/traces", "/otlp/v1/logs", "/otlp/v1/metrics"} {
		if !strings.Contains(paths, want) {
			t.Errorf("receiver paths %s, missing %s", paths, want)
		}
	}
	if !r.Contains([]byte("test.count")) {
		t.Error("the counter was not sent")
	}
	if r.Contains([]byte("tcgreplay.usage_daily")) {
		t.Error("the daily gauge was sent with the usage history off")
	}
}

// trustingBoth returns a client that trusts the fake receiver's and the
// fake query API's certificates.
func trustingBoth(t *testing.T, r *observetest.Receiver, p *promFake) *http.Client {
	t.Helper()
	pool := x509.NewCertPool()
	pem, err := os.ReadFile(r.CertFile(t))
	if err != nil {
		t.Fatal(err)
	}
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatal("receiver certificate not read")
	}
	pool.AddCert(p.srv.Certificate())
	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}
	t.Cleanup(tr.CloseIdleConnections)
	return &http.Client{Transport: tr}
}

// TestStartSendsTheDailyGaugeToGrafana: with export and the usage history
// on, the table computed from the query API reaches the OTLP receiver as
// the daily gauge, labelled by day and count; when the query API then
// fails the gauge keeps sending the table it has, and the rest still flows.
func TestStartSendsTheDailyGaugeToGrafana(t *testing.T) {
	r := observetest.New(t)
	p := newPromFake(t, true)
	p.setReadBack(row("8", "day", "2026-11-01", "count", "players"))
	end22 := utc("2026-11-23")
	p.answer("present", end22, row("2"))
	p.answer("players", end22, row("6"))
	p.answer("sign_ins", end22, row("4", "provider", "google"))

	s := on(r.URL, trustingBoth(t, r, p))
	s.MetricsURL, s.MetricsUser = p.srv.URL+"/api/prom", "654321"
	s.Now = func() time.Time { return historyNow }
	tel := Start(context.Background(), s)
	t.Cleanup(func() { tel.Shutdown(context.Background()) })
	if !tel.HistoryOn() {
		t.Fatal("HistoryOn() = false with export and metrics settings")
	}
	if err := tel.history.refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	for _, q := range p.requests() {
		if q.auth != historyAuth {
			t.Errorf("%s query Authorization = %q, want %q", q.kind, q.auth, historyAuth)
		}
	}
	if err := tel.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
	last := lastMetrics(t, r)
	for _, want := range []string{"tcgreplay.usage_daily", "day", "count", "2026-11-22", "2026-11-01", "players", "sign_ins", "provider", "google"} {
		if !strings.Contains(last, want) {
			t.Errorf("metrics sent lack %q", want)
		}
	}

	p.setFail("*", http.StatusInternalServerError)
	if err := tel.history.refresh(context.Background()); err == nil {
		t.Fatal("refresh succeeded against a failing query API")
	}
	sendOne(tel)
	if err := tel.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush after the failure: %v", err)
	}
	last = lastMetrics(t, r)
	for _, want := range []string{"tcgreplay.usage_daily", "2026-11-22", "2026-11-01", "test.count"} {
		if !strings.Contains(last, want) {
			t.Errorf("metrics sent after the query API failed lack %q", want)
		}
	}
	paths := strings.Join(r.Paths(), " ")
	for _, want := range []string{"/otlp/v1/traces", "/otlp/v1/logs"} {
		if !strings.Contains(paths, want) {
			t.Errorf("receiver paths %s, missing %s after the query API failed", paths, want)
		}
	}
}

// lastMetrics returns the body of the last metrics request r received.
func lastMetrics(t *testing.T, r *observetest.Receiver) string {
	t.Helper()
	var last string
	for _, q := range r.Requests() {
		if q.Path == "/otlp/v1/metrics" {
			last = string(q.Body)
		}
	}
	if last == "" {
		t.Fatal("no metrics request received")
	}
	return last
}
