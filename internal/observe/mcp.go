package observe

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// The MCP SDK hands every tool call the context of the session's first
// (initialize) request, long ended, so a span taken from it would put the
// call in the wrong trace. Each call does get the headers of the HTTP
// request that carried it, so trace context crosses the MCP boundary the
// way it would a remote hop: CarryContext writes the request's span into
// its headers, and MCPMiddleware reads it back for each call.

// CarryContext goes inside the token check around the MCP handler: it
// writes the current request span's traceparent into the request headers
// (replacing any the client sent), for MCPMiddleware to read. With export
// off it returns h itself.
func (t *Telemetry) CarryContext(h http.Handler) http.Handler {
	if !t.Enabled() {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("traceparent")
		r.Header.Del("tracestate")
		propagation.TraceContext{}.Inject(r.Context(), propagation.HeaderCarrier(r.Header))
		h.ServeHTTP(w, r)
	})
}

// MCPMiddleware must be the outermost MCP middleware (first in the one
// AddReceivingMiddleware call), so the rate limit, the recorder and the
// tools all run inside its span. Each tools/call becomes a span
// "tools/call <tool>", a child of the HTTP request that carried it, naming
// the tool, the assistant (known names only, else "other") and the
// player's pseudonymous ID, which acct gives for the request. A refused or
// failed call marks the span failed; the recorder sets the redacted
// message. Other methods pass through. With export off it does nothing.
func (t *Telemetry) MCPMiddleware(acct func(mcp.Request) (playerID, assistant string)) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		if !t.Enabled() {
			return next
		}
		tracer := t.tracerProvider.Tracer(scope)
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			call, ok := req.(*mcp.CallToolRequest)
			if method != "tools/call" || !ok {
				return next(ctx, method, req)
			}
			if extra := req.GetExtra(); extra != nil && extra.Header != nil {
				// Replaces the stale span context the session's context holds.
				ctx = propagation.TraceContext{}.Extract(ctx, propagation.HeaderCarrier(extra.Header))
			}
			var tool string
			if call.Params != nil {
				tool = call.Params.Name
			}
			player, assistant := acct(req)
			ctx, span := tracer.Start(ctx, "tools/call "+tool, trace.WithSpanKind(trace.SpanKindInternal),
				trace.WithAttributes(
					attribute.String("tcgreplay.tool", tool),
					attribute.String("tcgreplay.assistant", collapse(assistant)),
					attribute.String("tcgreplay.player", player),
				))
			defer span.End()
			res, err := next(ctx, method, req)
			if cr, ok := res.(*mcp.CallToolResult); err != nil || (ok && cr != nil && cr.IsError) {
				markFailed(span)
			}
			return res, err
		}
	}
}

// markFailed marks span failed unless it already is, keeping the redacted
// message the recorder set. It never sets an error's own text, which can
// quote what the player pasted.
func markFailed(span trace.Span) {
	if ro, ok := span.(sdktrace.ReadOnlySpan); ok && ro.Status().Code == codes.Error {
		return
	}
	span.SetStatus(codes.Error, "")
}
