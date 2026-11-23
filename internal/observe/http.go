package observe

import (
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Handler wraps the whole hosted handler so every request is a server
// span, named by the route pattern it matched ("GET /board", "POST /mcp")
// and carrying it as http.route. A request carrying a W3C traceparent (as
// Cloudflare forwards it) joins that trace, parented on the incoming span
// whether or not it was sampled; the sampler is always on. otelhttp's own
// HTTP metrics are off: per-route histograms would spend the free tier's
// series allowance. With export off it returns h itself.
func (t *Telemetry) Handler(h http.Handler) http.Handler {
	if !t.Enabled() {
		return h
	}
	return otelhttp.NewHandler(routeNamer(h), "http",
		otelhttp.WithTracerProvider(t.tracerProvider),
		otelhttp.WithMeterProvider(metricnoop.NewMeterProvider()),
		otelhttp.WithPropagators(propagation.TraceContext{}),
		// Not a public endpoint: the incoming context is the parent, not
		// a link.
		otelhttp.WithPublicEndpointFn(func(*http.Request) bool { return false }),
		otelhttp.WithSpanNameFormatter(serverSpanName),
	)
}

// route is the path part of the pattern a ServeMux matched ("/board" for
// "GET /board"), or "" before a pattern is known.
func route(r *http.Request) string {
	p := r.Pattern
	if i := strings.IndexByte(p, ' '); i >= 0 {
		p = p[i+1:]
	}
	if i := strings.IndexByte(p, '/'); i > 0 {
		p = p[i:] // drop a host in the pattern
	}
	return p
}

// serverSpanName names a server span by method and route; before the
// route is known (and for requests no route matched) by method alone.
func serverSpanName(_ string, r *http.Request) string {
	if rt := route(r); rt != "" {
		return r.Method + " " + rt
	}
	return r.Method
}

// routeNamer serves h, then records the route the mux matched on the
// request's server span. The outermost mux sets the pattern on the very
// request it was handed (inner muxes see copies).
func routeNamer(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		if rt := route(r); rt != "" {
			trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("http.route", rt))
		}
	})
}

// Client returns a copy of base whose calls are client spans, children of
// the span in each request's context, named by method and host. A nil
// base means a client with a 30 second timeout. No trace context is sent
// to the outside services: none of them takes part in the trace. With
// export off it returns base itself.
func (t *Telemetry) Client(base *http.Client) *http.Client {
	if !t.Enabled() {
		return base
	}
	c := &http.Client{Timeout: 30 * time.Second}
	if base != nil {
		cp := *base
		c = &cp
	}
	next := c.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	c.Transport = otelhttp.NewTransport(next,
		otelhttp.WithTracerProvider(t.tracerProvider),
		otelhttp.WithMeterProvider(metricnoop.NewMeterProvider()),
		otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator()),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string { return r.Method + " " + r.URL.Host }),
	)
	return c
}
