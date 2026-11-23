package observe

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// sentLabels is every metric the playmat sends, as Grafana stores it, with
// the labels it carries. Hand-maintained from what the code sends.
var sentLabels = map[string][]string{
	"tcgreplay_player_days_total": {},
	"tcgreplay_sign_ins_total":    {"provider"},
	"tcgreplay_tool_calls_total":  {"assistant", "tool"},
	"tcgreplay_imports_total":     {"assistant"},
	"tcgreplay_new_games_total":   {"assistant"},
	"tcgreplay_errors_total":      {"kind"},
	"tcgreplay_limited_total":     {"reason"},
	"tcgreplay_usage_daily":       {"day", "count", "provider", "assistant"},
}

// dailyCounts is every count the daily table holds.
var dailyCounts = []string{"players", "sign_ins", "tool_calls", "imports", "new_games", "errors"}

// dashPanel is a panel of the dashboard and the queries it charts.
type dashPanel struct {
	title, kind string
	exprs       []string
}

// dashboardPanels reads the repository's dashboard and returns every panel
// (nested ones included) that has a query.
func dashboardPanels(t *testing.T) []dashPanel {
	t.Helper()
	b, err := os.ReadFile("../../docs/grafana/tcgreplay-usage.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("dashboard is not JSON: %v", err)
	}
	var out []dashPanel
	var exprs func(v any) []string
	exprs = func(v any) []string {
		var got []string
		switch v := v.(type) {
		case map[string]any:
			if e, ok := v["expr"].(string); ok {
				got = append(got, e)
			}
			for k, c := range v {
				if k != "panels" {
					got = append(got, exprs(c)...)
				}
			}
		case []any:
			for _, c := range v {
				got = append(got, exprs(c)...)
			}
		}
		return got
	}
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			if ps, ok := v["panels"].([]any); ok {
				for _, p := range ps {
					pm, ok := p.(map[string]any)
					if !ok {
						continue
					}
					if es := exprs(pm); len(es) > 0 {
						title, _ := pm["title"].(string)
						kind, _ := pm["type"].(string)
						out = append(out, dashPanel{title: title, kind: kind, exprs: es})
					}
					walk(pm)
				}
			}
		case []any:
			for _, c := range v {
				walk(c)
			}
		}
	}
	walk(doc)
	if len(out) == 0 {
		t.Fatal("dashboard has no queries")
	}
	return out
}

var (
	metricName = regexp.MustCompile(`\btcgreplay_[a-z_]+\b`)
	selector   = regexp.MustCompile(`\{([^}]*)\}`)
	matcher    = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*(=~|!~|!=|=)\s*"([^"]*)"\s*$`)
	grouping   = regexp.MustCompile(`\b(?:by|without)\s*\(([^)]*)\)`)
)

// exprLabels returns the labels expr selects on or groups by, and the
// values it matches each selected label against.
func exprLabels(t *testing.T, expr string) (labels []string, values map[string][]string) {
	t.Helper()
	values = map[string][]string{}
	for _, m := range selector.FindAllStringSubmatch(expr, -1) {
		for _, part := range strings.Split(m[1], ",") {
			if strings.TrimSpace(part) == "" {
				continue
			}
			mm := matcher.FindStringSubmatch(part)
			if mm == nil {
				t.Errorf("query %q: cannot read matcher %q", expr, part)
				continue
			}
			labels = append(labels, mm[1])
			values[mm[1]] = append(values[mm[1]], mm[3])
		}
	}
	for _, m := range grouping.FindAllStringSubmatch(expr, -1) {
		for _, l := range strings.Split(m[1], ",") {
			if l = strings.TrimSpace(l); l != "" {
				labels = append(labels, l)
			}
		}
	}
	return labels, values
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// TestDashboardQueriesNameOnlyWhatIsSent: every query in the dashboard
// names one metric the playmat sends, and selects on or groups by only
// labels that metric carries, with count values from the daily table.
func TestDashboardQueriesNameOnlyWhatIsSent(t *testing.T) {
	for _, p := range dashboardPanels(t) {
		for _, expr := range p.exprs {
			names := map[string]bool{}
			for _, n := range metricName.FindAllString(expr, -1) {
				names[n] = true
			}
			if len(names) != 1 {
				t.Errorf("panel %q query %q names %d metrics, want exactly one", p.title, expr, len(names))
				continue
			}
			var name string
			for n := range names {
				name = n
			}
			carried, ok := sentLabels[name]
			if !ok {
				t.Errorf("panel %q query %q names %s, which the playmat does not send", p.title, expr, name)
				continue
			}
			labels, values := exprLabels(t, expr)
			for _, l := range labels {
				if l == "__name__" {
					t.Errorf("panel %q query %q selects by __name__; name the metric", p.title, expr)
				} else if !contains(carried, l) {
					t.Errorf("panel %q query %q uses label %q, which %s does not carry", p.title, expr, l, name)
				}
			}
			for _, v := range values["count"] {
				if !contains(dailyCounts, v) {
					t.Errorf("panel %q query %q selects count %q, not one of %v", p.title, expr, v, dailyCounts)
				}
			}
		}
	}
}

// TestDashboardChartsEachCountPerDay: for each count of the daily table a
// panel charts the daily gauge for that count, grouped by day.
func TestDashboardChartsEachCountPerDay(t *testing.T) {
	panels := dashboardPanels(t)
	for _, count := range dailyCounts {
		var found []string
		for _, p := range panels {
			for _, expr := range p.exprs {
				if !strings.Contains(expr, "tcgreplay_usage_daily") {
					continue
				}
				labels, values := exprLabels(t, expr)
				if contains(values["count"], count) && contains(labels, "day") {
					found = append(found, p.title)
				}
			}
		}
		if len(found) != 1 {
			t.Errorf("count %s is charted per day by %d panels %v, want one", count, len(found), found)
		}
	}
}

// TestDashboardChartsTheRawCounters: the counters behind each daily count
// are charted too, for the days before the table holds them.
func TestDashboardChartsTheRawCounters(t *testing.T) {
	var all []string
	for _, p := range dashboardPanels(t) {
		all = append(all, p.exprs...)
	}
	joined := strings.Join(all, "\n")
	for _, m := range []string{"tcgreplay_player_days_total", "tcgreplay_sign_ins_total", "tcgreplay_tool_calls_total",
		"tcgreplay_imports_total", "tcgreplay_new_games_total", "tcgreplay_errors_total"} {
		if !strings.Contains(joined, m) {
			t.Errorf("no dashboard query charts %s", m)
		}
	}
}
