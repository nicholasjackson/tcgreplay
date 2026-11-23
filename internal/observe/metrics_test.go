package observe

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	lognoop "go.opentelemetry.io/otel/log/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// counted returns a Telemetry whose counters a manual reader collects,
// knowing the given tool names.
func counted(t *testing.T, tools ...string) (*Telemetry, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { mp.Shutdown(context.Background()) })
	tel := &Telemetry{enabled: true, loggerProvider: lognoop.NewLoggerProvider(), meterProvider: mp, tools: &toolSet{}, health: newHealth(nil, nil)}
	tel.usage = newUsage(mp.Meter(scope), tel.tools.closed)
	tel.SetTools(tools)
	return tel, reader
}

// points collects every counter point as "metric{k=v,...}" -> value.
func points(t *testing.T, r *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := r.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("%s is %T, want a cumulative int64 sum", m.Name, m.Data)
			}
			if !sum.IsMonotonic || sum.Temporality != metricdata.CumulativeTemporality {
				t.Errorf("%s is not a cumulative counter", m.Name)
			}
			for _, dp := range sum.DataPoints {
				var kv []string
				for _, a := range dp.Attributes.ToSlice() {
					kv = append(kv, string(a.Key)+"="+a.Value.AsString())
				}
				sort.Strings(kv)
				out[m.Name+"{"+strings.Join(kv, ",")+"}"] += dp.Value
			}
		}
	}
	return out
}

func line(event string, f func(*telemetry.Line)) telemetry.Line {
	l := telemetry.Line{Time: time.Now(), Level: "info", Event: event, Player: "p_6cef43a09af39824", JSON: []byte("{}\n")}
	if f != nil {
		f(&l)
	}
	return l
}

func TestUsageRecordsIncrementTheirCounter(t *testing.T) {
	tel, reader := counted(t, "import_log", "new_game", "ping")
	sink := tel.Sink()
	ctx := context.Background()
	for _, l := range []telemetry.Line{
		line(telemetry.PlayerDay, nil),
		line(telemetry.PlayerDay, nil),
		line(telemetry.SignIn, func(l *telemetry.Line) { l.Provider = "google" }),
		line(telemetry.SignIn, nil), // recorded before providers: Google
		line(telemetry.SignIn, func(l *telemetry.Line) { l.Provider = "password" }),
		line(telemetry.SignIn, func(l *telemetry.Line) { l.Provider = "github" }),
		line(telemetry.ToolCall, func(l *telemetry.Line) { l.Tool, l.Assistant = "import_log", "Claude" }),
		line(telemetry.ToolCall, func(l *telemetry.Line) { l.Tool, l.Assistant = "ping", "my-own-bot" }),
		line(telemetry.ToolCall, func(l *telemetry.Line) { l.Tool, l.Assistant = "drop_tables", "ChatGPT" }),
		line(telemetry.Import, func(l *telemetry.Line) { l.Assistant = "Gemini" }),
		line(telemetry.NewGame, func(l *telemetry.Line) { l.Assistant = "Codex" }),
		line(telemetry.NewGame, func(l *telemetry.Line) { l.Assistant = "Whatever" }),
		line(telemetry.Error, func(l *telemetry.Line) { l.Level, l.Tool, l.Message = "error", "import_log", "bad log" }),
		line(telemetry.Error, func(l *telemetry.Line) { l.Level, l.Action, l.Message = "error", "move", "no such card" }),
		line(telemetry.Limited, func(l *telemetry.Line) { l.Reason = "rate" }),
		line(telemetry.Limited, func(l *telemetry.Line) { l.Reason = "places" }),
		// Not usage: nothing counts these.
		line(telemetry.GameExpired, func(l *telemetry.Line) { l.Idle = "60 minutes" }),
		line(telemetry.AccountRemoved, func(l *telemetry.Line) { l.Reason = "idle" }),
		line(telemetry.SignOut, nil),
		line(telemetry.SignInRefused, func(l *telemetry.Line) { l.Reason = "the server is busy" }),
		line(telemetry.BoardRefused, nil),
		line(telemetry.AdminRefused, func(l *telemetry.Line) { l.Reason = "not admin" }),
		line(telemetry.AdminAction, func(l *telemetry.Line) { l.Action = "check" }),
	} {
		sink.Emit(ctx, l)
	}
	want := map[string]int64{
		"tcgreplay.player_days{}":                                2,
		"tcgreplay.sign_ins{provider=google}":                    2,
		"tcgreplay.sign_ins{provider=password}":                  1,
		"tcgreplay.sign_ins{provider=github}":                    1,
		"tcgreplay.tool_calls{assistant=Claude,tool=import_log}": 1,
		"tcgreplay.tool_calls{assistant=other,tool=ping}":        1,
		"tcgreplay.tool_calls{assistant=ChatGPT,tool=other}":     1,
		"tcgreplay.imports{assistant=Gemini}":                    1,
		"tcgreplay.new_games{assistant=Codex}":                   1,
		"tcgreplay.new_games{assistant=other}":                   1,
		"tcgreplay.errors{kind=tool}":                            1,
		"tcgreplay.errors{kind=board}":                           1,
		"tcgreplay.limited{reason=rate}":                         1,
		"tcgreplay.limited{reason=places}":                       1,
	}
	got := points(t, reader)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %d, want %d", k, got[k], v)
		}
	}
	for k, v := range got {
		if _, ok := want[k]; !ok && v != 0 {
			t.Errorf("unexpected point %s = %d", k, v)
		}
		if strings.Contains(k, "player") && !strings.HasPrefix(k, "tcgreplay.player_days{") || strings.Contains(k, "p_") {
			t.Errorf("point %s carries a player label", k)
		}
	}
}

func TestToolsBeforeSetCountAsOther(t *testing.T) {
	tel, reader := counted(t)
	tel.Sink().Emit(context.Background(), line(telemetry.ToolCall, func(l *telemetry.Line) { l.Tool, l.Assistant = "import_log", "Claude" }))
	if got := points(t, reader)["tcgreplay.tool_calls{assistant=Claude,tool=other}"]; got != 1 {
		t.Errorf("tool call with no tool names known counted %d under tool=other, want 1", got)
	}
}

func TestStartBuildsCountersOnlyWhenOn(t *testing.T) {
	if off := Start(context.Background(), Settings{}); off.usage != nil || off.Sink() != nil {
		t.Error("export off builds counters or a sink")
	}
	var nilTel *Telemetry
	nilTel.SetTools([]string{"ping"}) // no panic
}
