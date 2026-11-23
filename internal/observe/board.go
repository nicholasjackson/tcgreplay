package observe

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// BoardSpan is the board server's trace hook for one board action: a new
// root span "board <verb>" naming the action and the player's pseudonymous
// ID, linked to the live connection's span in ctx. A board connection can
// stay open for hours, so each action gets a short trace of its own rather
// than being a child of the connection's. The returned function ends the
// span, marking it failed when the action failed; the recorder sets the
// redacted message.
func (t *Telemetry) BoardSpan(ctx context.Context, verb, player string) (context.Context, func(error)) {
	if !t.Enabled() {
		return ctx, func(error) {}
	}
	opts := []trace.SpanStartOption{
		trace.WithNewRoot(),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String("tcgreplay.board.verb", verb), attribute.String("tcgreplay.player", player)),
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		opts = append(opts, trace.WithLinks(trace.Link{SpanContext: sc}))
	}
	ctx, span := t.tracerProvider.Tracer(scope).Start(ctx, "board "+verb, opts...)
	return ctx, func(err error) {
		if err != nil {
			markFailed(span)
		}
		span.End()
	}
}
