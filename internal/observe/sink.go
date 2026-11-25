package observe

import (
	"bytes"
	"context"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"

	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// scope names the instrumentation in everything the playmat sends.
const scope = "github.com/nicholasjackson/tcgreplay"

// recordSink turns each usage or error record the recorder writes into a
// log record: the same JSON line as its body, its fields as attributes,
// and the trace and span of the request it was recorded in. It also
// increments the usage counter the record stands for. It sees only what
// the recorder already wrote (player IDs, redacted messages), so it adds
// nothing about players.
type recordSink struct {
	logger otellog.Logger
	usage  *usage
}

// Sink returns the recorder's sink (for telemetry.Recorder.SetSink), or
// nil when export is off.
func (t *Telemetry) Sink() telemetry.Sink {
	if !t.Enabled() {
		return nil
	}
	return recordSink{logger: t.loggerProvider.Logger(scope), usage: t.usage}
}

func (s recordSink) Emit(ctx context.Context, l telemetry.Line) {
	var rec otellog.Record
	rec.SetTimestamp(l.Time)
	rec.SetObservedTimestamp(l.Time)
	if l.Level == "error" {
		rec.SetSeverity(otellog.SeverityError)
		rec.SetSeverityText("ERROR")
	} else {
		rec.SetSeverity(otellog.SeverityInfo)
		rec.SetSeverityText("INFO")
	}
	rec.SetBody(attribute.StringValue(string(bytes.TrimRight(l.JSON, "\n"))))
	attrs := []attribute.KeyValue{attribute.String("log.source", "record")}
	for _, f := range []struct{ k, v string }{
		{"event", l.Event}, {"player", l.Player}, {"tool", l.Tool}, {"action", l.Action},
		{"assistant", l.Assistant}, {"reason", l.Reason}, {"provider", l.Provider}, {"report", l.Report},
	} {
		if f.v != "" {
			attrs = append(attrs, attribute.String(f.k, f.v))
		}
	}
	rec.AddAttributes(attrs...)
	s.logger.Emit(ctx, rec)
	s.usage.count(ctx, l)
}
