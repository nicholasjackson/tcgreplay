package observe

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// The usage counters, derived from the records. Grafana Cloud stores each
// as tcgreplay_<name>_total (dots become underscores, a counter gains
// _total, and a {…} unit adds no suffix). Every label is a closed set, so
// the free tier's series allowance is safe: no player ID or other
// per-player label is ever attached.
const (
	metricPlayerDays = "tcgreplay.player_days"
	metricSignIns    = "tcgreplay.sign_ins"
	metricToolCalls  = "tcgreplay.tool_calls"
	metricImports    = "tcgreplay.imports"
	metricNewGames   = "tcgreplay.new_games"
	metricErrors     = "tcgreplay.errors"
	metricLimited    = "tcgreplay.limited"
)

// usage holds the counters.
type usage struct {
	playerDays, signIns, toolCalls, imports, newGames, errors, limited metric.Int64Counter
	tools                                                              func(string) string
}

func newUsage(m metric.Meter, tools func(string) string) *usage {
	c := func(name, unit, desc string) metric.Int64Counter {
		ctr, _ := m.Int64Counter(name, metric.WithUnit(unit), metric.WithDescription(desc))
		return ctr
	}
	return &usage{
		playerDays: c(metricPlayerDays, "{player}", "Players' first activity in a UTC day (player_day records)."),
		signIns:    c(metricSignIns, "{sign_in}", "Completed sign-ins, by provider."),
		toolCalls:  c(metricToolCalls, "{call}", "MCP tool calls past the request limit, by assistant and tool."),
		imports:    c(metricImports, "{import}", "Successful import_log calls, by assistant."),
		newGames:   c(metricNewGames, "{game}", "Successful new_game calls, by assistant."),
		errors:     c(metricErrors, "{error}", "Failed tool calls and board actions, by kind."),
		limited:    c(metricLimited, "{request}", "Requests stopped by the request limit or the user cap, by reason."),
		tools:      tools,
	}
}

// count increments the counter a record stands for; records that are not
// usage (game expiry, refusals, admin actions) count nothing.
func (u *usage) count(ctx context.Context, l telemetry.Line) {
	one := func(c metric.Int64Counter, kv ...attribute.KeyValue) {
		c.Add(ctx, 1, metric.WithAttributes(kv...))
	}
	switch l.Event {
	case telemetry.PlayerDay:
		one(u.playerDays)
	case telemetry.SignIn:
		one(u.signIns, attribute.String("provider", provider(l.Provider)))
	case telemetry.ToolCall:
		one(u.toolCalls, attribute.String("assistant", collapse(l.Assistant)), attribute.String("tool", u.tools(l.Tool)))
	case telemetry.Import:
		one(u.imports, attribute.String("assistant", collapse(l.Assistant)))
	case telemetry.NewGame:
		one(u.newGames, attribute.String("assistant", collapse(l.Assistant)))
	case telemetry.Error:
		kind := "board"
		if l.Tool != "" {
			kind = "tool"
		}
		one(u.errors, attribute.String("kind", kind))
	case telemetry.Limited:
		one(u.limited, attribute.String("reason", oneOf(l.Reason, "rate", "places")))
	}
}

// toolSet is the registered MCP tool names, set by hosted wiring.
type toolSet struct {
	mu    sync.RWMutex
	names map[string]bool
}

// closed is name if it is a registered tool, else "other". Before any are
// set, every name is "other".
func (s *toolSet) closed(name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.names[name] {
		return name
	}
	return "other"
}

// SetTools tells the usage counters the registered MCP tool names, so a
// call to any other name counts as "other".
func (t *Telemetry) SetTools(names []string) {
	if t == nil {
		return
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	t.tools.mu.Lock()
	t.tools.names = m
	t.tools.mu.Unlock()
}
