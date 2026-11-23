package observe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// How long sending must fail before the health watch says so, how often it
// may repeat itself, and how often it looks.
const (
	failAfter   = 5 * time.Minute
	warnEvery   = 5 * time.Minute
	healthEvery = 30 * time.Second
)

// health notes the outcome of every export, per signal ("logs", "traces",
// "metrics", "history"), and writes one plain line to stderr when sending
// has failed continuously for more than five minutes, at most every five
// minutes. The line names the signals and the kind of failure only: never
// the token, the Basic header or the error's own text.
type health struct {
	now    func() time.Time
	stderr io.Writer

	mu       sync.Mutex
	signals  map[string]*signalHealth
	lastWarn time.Time
	status   map[string]int // last HTTP status seen per signal, by the transport
}

type signalHealth struct {
	failingSince time.Time // zero while the last export succeeded
	kind         string
}

func newHealth(now func() time.Time, stderr io.Writer) *health {
	if now == nil {
		now = time.Now
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return &health{now: now, stderr: stderr, signals: map[string]*signalHealth{}, status: map[string]int{}}
}

// note records one export's outcome for signal.
func (h *health) note(signal string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.signals[signal]
	if s == nil {
		s = &signalHealth{}
		h.signals[signal] = s
	}
	if err == nil {
		s.failingSince, s.kind = time.Time{}, ""
		return
	}
	if s.failingSince.IsZero() {
		s.failingSince = h.now()
	}
	s.kind = kind(err, h.status[signal])
}

// sawStatus records the HTTP status of signal's latest response.
func (h *health) sawStatus(signal string, code int) {
	h.mu.Lock()
	h.status[signal] = code
	h.mu.Unlock()
}

// check writes the failure line when it is due at now.
func (h *health) check(now time.Time) {
	h.mu.Lock()
	var oldest time.Time
	byKind := map[string][]string{}
	for name, s := range h.signals {
		if s.failingSince.IsZero() || now.Sub(s.failingSince) < failAfter {
			continue
		}
		if oldest.IsZero() || s.failingSince.Before(oldest) {
			oldest = s.failingSince
		}
		byKind[s.kind] = append(byKind[s.kind], name)
	}
	if len(byKind) == 0 {
		h.lastWarn = time.Time{}
		h.mu.Unlock()
		return
	}
	if !h.lastWarn.IsZero() && now.Sub(h.lastWarn) < warnEvery {
		h.mu.Unlock()
		return
	}
	h.lastWarn = now
	h.mu.Unlock()

	parts := make([]string, 0, len(byKind))
	for k, names := range byKind {
		sort.Strings(names)
		parts = append(parts, strings.Join(names, ", ")+": "+k)
	}
	sort.Strings(parts) // by signal name
	fmt.Fprintf(h.stderr, "warning: telemetry export: sending to Grafana Cloud has failed for %dm (%s)\n",
		int(now.Sub(oldest)/time.Minute), strings.Join(parts, "; "))
}

// run checks every 30 seconds until ctx is done.
func (h *health) run(ctx context.Context) {
	t := time.NewTicker(healthEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.check(h.now())
		}
	}
}

// kind classifies an export failure without ever including its text,
// which can carry the endpoint and the response body.
func kind(err error, status int) string {
	var ne net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	}
	var op *net.OpError
	var dns *net.DNSError
	if errors.As(err, &op) || errors.As(err, &dns) {
		return "unreachable"
	}
	if status >= 300 {
		return fmt.Sprintf("rejected (HTTP %d)", status)
	}
	return "failed"
}

// statusTransport tells the health watch the HTTP status of each export
// response, since the exporters' errors do not always carry it.
type statusTransport struct {
	signal string
	health *health
	next   http.RoundTripper
}

func (t statusTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(r)
	if err == nil {
		t.health.sawStatus(t.signal, resp.StatusCode)
	}
	return resp, err
}

// The exporter wrappers call through and note each outcome.

type healthSpanExporter struct {
	sdktrace.SpanExporter
	h *health
}

func (e healthSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	err := e.SpanExporter.ExportSpans(ctx, spans)
	e.h.note("traces", err)
	return err
}

type healthLogExporter struct {
	sdklog.Exporter
	h *health
}

func (e healthLogExporter) Export(ctx context.Context, records []sdklog.Record) error {
	err := e.Exporter.Export(ctx, records)
	e.h.note("logs", err)
	return err
}

type healthMetricExporter struct {
	sdkmetric.Exporter
	h *health
}

func (e healthMetricExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	err := e.Exporter.Export(ctx, rm)
	e.h.note("metrics", err)
	return err
}
