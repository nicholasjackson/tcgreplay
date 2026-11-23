package observe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Grafana Cloud's free tier keeps metrics for 14 days, but the daily usage
// counts must be readable for at least 28. The usage history keeps them
// inside Grafana: it reads back the daily table it exported before, adds
// each completed UTC day computed from the raw counters, drops days older
// than six weeks, and exports the table again as one gauge. Grafana is
// the only store; the playmat keeps nothing on disk, and a restart loses
// nothing.
const (
	metricUsageDaily = "tcgreplay.usage_daily"
	historyKeep      = 42 // days
	historyRaw       = 13 // completed days the raw counters still cover
	historyFirst     = 30 * time.Second
	historyEvery     = time.Hour
	historyTimeout   = 10 * time.Second
)

// historyQueries computes each daily count from the raw counters over the
// day ending at the query time. A count split by provider or assistant
// gives one row per value.
var historyQueries = []struct {
	count, by, promql string
}{
	{"players", "", `round(sum(increase(tcgreplay_player_days_total[1d])))`},
	{"sign_ins", "provider", `round(sum by (provider) (increase(tcgreplay_sign_ins_total[1d])))`},
	{"tool_calls", "assistant", `round(sum by (assistant) (increase(tcgreplay_tool_calls_total[1d])))`},
	{"imports", "assistant", `round(sum by (assistant) (increase(tcgreplay_imports_total[1d])))`},
	{"new_games", "assistant", `round(sum by (assistant) (increase(tcgreplay_new_games_total[1d])))`},
	{"errors", "", `round(sum(increase(tcgreplay_errors_total[1d])))`},
}

// historyReadBack reads the table the playmat exported, by its largest
// value per day and count, so two instances during a deploy agree.
const historyReadBack = `max by (day, count, provider, assistant) (last_over_time(tcgreplay_usage_daily[14d]))`

// historyPresent tells whether the playmat exported any counter in the day
// ending at the query time; a day with none is not computed.
const historyPresent = `count(last_over_time({__name__=~"tcgreplay_(player_days|sign_ins|tool_calls|imports|new_games|errors|limited)_total"}[1d]))`

// dayKey is one row of the daily table.
type dayKey struct {
	Day, Count, Provider, Assistant string
}

// history is the carried daily table and the client that reads it back.
type history struct {
	endpoint, user, token string
	client                *http.Client
	now                   func() time.Time
	health                *health

	mu    sync.Mutex
	table map[dayKey]float64
}

func newHistory(endpoint, user, token string, base http.RoundTripper, now func() time.Time, h *health) *history {
	if base == nil {
		base = http.DefaultTransport
	}
	return &history{
		endpoint: endpoint, user: user, token: token, now: now, health: h,
		client: &http.Client{Timeout: historyTimeout, Transport: statusTransport{signal: "history", health: h, next: base}},
		table:  map[dayKey]float64{},
	}
}

// sample is one row of a Prometheus instant vector.
type sample struct {
	labels map[string]string
	value  float64
}

// query runs promql at time at against the stack's Prometheus query API.
// Its errors never carry the token or the response body.
func (h *history) query(ctx context.Context, promql string, at time.Time) ([]sample, error) {
	ctx, cancel := context.WithTimeout(ctx, historyTimeout)
	defer cancel()
	q := url.Values{"query": {promql}, "time": {strconv.FormatInt(at.Unix(), 10)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.endpoint+"/api/v1/query?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(h.user, h.token)
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("query: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string `json:"resultType"`
			Result     []struct {
				Metric map[string]string `json:"metric"`
				Value  [2]any            `json:"value"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&body); err != nil {
		return nil, errors.New("query: unreadable response")
	}
	if body.Status != "success" || body.Data.ResultType != "vector" {
		return nil, errors.New("query: not a vector")
	}
	out := make([]sample, 0, len(body.Data.Result))
	for _, r := range body.Data.Result {
		s, _ := r.Value[1].(string)
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			continue
		}
		out = append(out, sample{labels: r.Metric, value: v})
	}
	return out, nil
}

// refresh reads the table back, adds each completed day the raw counters
// still cover that the table lacks, and drops days past six weeks. A
// failed query leaves the table as it was (a failed day is tried again
// next time) and counts towards the export health warning.
func (h *history) refresh(ctx context.Context) error {
	now := h.now().UTC()
	back, err := h.query(ctx, historyReadBack, now)
	h.health.note("history", err)
	if err != nil {
		return err
	}
	h.mu.Lock()
	for _, s := range back {
		k := dayKey{Day: s.labels["day"], Count: s.labels["count"], Provider: s.labels["provider"], Assistant: s.labels["assistant"]}
		if k.Day == "" || k.Count == "" {
			continue
		}
		if s.value > h.table[k] {
			h.table[k] = s.value
		}
	}
	have := map[string]bool{}
	for k := range h.table {
		have[k.Day] = true
	}
	h.mu.Unlock()

	today := now.Truncate(24 * time.Hour)
	for i := historyRaw; i >= 1; i-- {
		day := today.AddDate(0, 0, -i)
		name := day.Format("2006-01-02")
		if have[name] {
			continue
		}
		rows, err := h.computeDay(ctx, day)
		h.health.note("history", err)
		if err != nil {
			return err
		}
		h.mu.Lock()
		for k, v := range rows {
			h.table[k] = v
		}
		h.mu.Unlock()
	}

	oldest := today.AddDate(0, 0, -historyKeep).Format("2006-01-02")
	h.mu.Lock()
	for k := range h.table {
		if k.Day < oldest {
			delete(h.table, k)
		}
	}
	h.mu.Unlock()
	return nil
}

// computeDay computes day's rows from the raw counters, at the end of the
// day. It gives no rows for a day the playmat exported no counter in, and
// 0 for an unsplit count with no increase.
func (h *history) computeDay(ctx context.Context, day time.Time) (map[dayKey]float64, error) {
	end := day.AddDate(0, 0, 1)
	present, err := h.query(ctx, historyPresent, end)
	if err != nil {
		return nil, err
	}
	if len(present) == 0 || present[0].value == 0 {
		return nil, nil
	}
	name := day.Format("2006-01-02")
	rows := map[dayKey]float64{}
	for _, hq := range historyQueries {
		res, err := h.query(ctx, hq.promql, end)
		if err != nil {
			return nil, err
		}
		if hq.by == "" {
			k := dayKey{Day: name, Count: hq.count}
			rows[k] = 0
			if len(res) > 0 {
				rows[k] = res[0].value
			}
			continue
		}
		for _, s := range res {
			k := dayKey{Day: name, Count: hq.count}
			switch hq.by {
			case "provider":
				k.Provider = s.labels["provider"]
			case "assistant":
				k.Assistant = s.labels["assistant"]
			}
			rows[k] = s.value
		}
	}
	return rows, nil
}

// observe reports every row of the table on the daily gauge.
func (h *history) observe(o metric.Float64Observer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, v := range h.table {
		attrs := []attribute.KeyValue{attribute.String("day", k.Day), attribute.String("count", k.Count)}
		if k.Provider != "" {
			attrs = append(attrs, attribute.String("provider", k.Provider))
		}
		if k.Assistant != "" {
			attrs = append(attrs, attribute.String("assistant", k.Assistant))
		}
		o.Observe(v, metric.WithAttributes(attrs...))
	}
}

// register makes the daily gauge on m, reporting the table.
func (h *history) register(m metric.Meter) error {
	// A {…} unit adds no suffix to the stored name (unit 1 would add
	// _ratio), so Grafana stores it as tcgreplay_usage_daily.
	_, err := m.Float64ObservableGauge(metricUsageDaily,
		metric.WithUnit("{count}"),
		metric.WithDescription("Daily usage totals carried for six weeks, by day and count."),
		metric.WithFloat64Callback(func(_ context.Context, o metric.Float64Observer) error {
			h.observe(o)
			return nil
		}))
	return err
}

// run refreshes 30 seconds after start-up, so the raw counters exist, and
// then every hour, until ctx is done.
func (h *history) run(ctx context.Context) {
	t := time.NewTimer(historyFirst)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.refresh(ctx)
			t.Reset(historyEvery)
		}
	}
}
