package observe

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	lognoop "go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// Export bounds: what keeps a slow or failing Grafana out of the player's
// way and memory flat.
const (
	exportTimeout  = 5 * time.Second
	queueSize      = 2048
	metricInterval = 60 * time.Second
	shutdownWithin = 2 * time.Second
)

// Telemetry is hosted mode's handle on export: the providers the playmat's
// adapters take their tracer, logger and meter from, and the health watch.
// With export off its providers are no-ops, so every adapter costs next to
// nothing. Providers are passed explicitly, never set as OTel globals, so
// parallel hosted tests cannot see each other's data and no desktop code
// path can ever pick one up.
type Telemetry struct {
	enabled bool

	tracerProvider trace.TracerProvider
	loggerProvider otellog.LoggerProvider
	meterProvider  metric.MeterProvider

	// The SDK providers, nil when export is off.
	sdkTracer *sdktrace.TracerProvider
	sdkLogger *sdklog.LoggerProvider
	sdkMeter  *sdkmetric.MeterProvider

	health  *health
	tools   *toolSet
	usage   *usage   // nil when export is off
	history *history // nil when the usage history is off
}

var quietGlobals sync.Once

// Start builds export from s. It never fails: missing or bad settings give
// no-op providers, and Settings.Status says why. The caller runs Run and
// calls Shutdown when the playmat stops.
func Start(ctx context.Context, s Settings) *Telemetry {
	t := &Telemetry{
		tracerProvider: tracenoop.NewTracerProvider(),
		loggerProvider: lognoop.NewLoggerProvider(),
		meterProvider:  metricnoop.NewMeterProvider(),
		health:         newHealth(s.Now, s.Stderr),
		tools:          &toolSet{},
	}
	ex := s.export()
	if !ex.on {
		return t
	}
	// Export failures are reported by the health watch, once every five
	// minutes; the SDK's own error output would repeat them on every batch.
	quietGlobals.Do(func() {
		otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) {}))
		otel.SetLogger(logr.Discard())
	})

	headers := map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(s.InstanceID+":"+s.Token)),
	}
	client := func(signal string) *http.Client {
		base := http.DefaultTransport
		if s.HTTPClient != nil && s.HTTPClient.Transport != nil {
			base = s.HTTPClient.Transport
		}
		return &http.Client{Timeout: exportTimeout, Transport: statusTransport{signal: signal, health: t.health, next: base}}
	}

	spanExp, err1 := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(ex.endpoint+"/v1/traces"),
		otlptracehttp.WithHeaders(headers),
		otlptracehttp.WithTimeout(exportTimeout),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}),
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
		otlptracehttp.WithHTTPClient(client("traces")),
	)
	logExp, err2 := otlploghttp.New(ctx,
		otlploghttp.WithEndpointURL(ex.endpoint+"/v1/logs"),
		otlploghttp.WithHeaders(headers),
		otlploghttp.WithTimeout(exportTimeout),
		otlploghttp.WithRetry(otlploghttp.RetryConfig{Enabled: false}),
		otlploghttp.WithCompression(otlploghttp.GzipCompression),
		otlploghttp.WithHTTPClient(client("logs")),
	)
	metricExp, err3 := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(ex.endpoint+"/v1/metrics"),
		otlpmetrichttp.WithHeaders(headers),
		otlpmetrichttp.WithTimeout(exportTimeout),
		otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{Enabled: false}),
		otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression),
		otlpmetrichttp.WithHTTPClient(client("metrics")),
	)
	if err := errors.Join(err1, err2, err3); err != nil {
		// Not expected with validated settings; export simply stays off.
		t.health.note("start", err)
		return t
	}

	res := resource.NewSchemaless(
		attribute.String("service.name", "tcgreplay"),
		attribute.String("service.version", s.Version),
		attribute.String("service.instance.id", instanceID()),
		attribute.String("deployment.environment.name", "production"),
	)
	t.sdkTracer = sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		// Always on, not parent-based: Cloudflare's sampled flag must never
		// drop a playmat span, while its trace ID is still adopted.
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(healthSpanExporter{SpanExporter: filterExporter{spanExp}, h: t.health},
			sdktrace.WithMaxQueueSize(queueSize),
			sdktrace.WithExportTimeout(exportTimeout)),
	)
	t.sdkLogger = sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(healthLogExporter{Exporter: logExp, h: t.health},
			sdklog.WithMaxQueueSize(queueSize),
			sdklog.WithExportTimeout(exportTimeout))),
	)
	t.sdkMeter = sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(healthMetricExporter{Exporter: metricExp, h: t.health},
			sdkmetric.WithInterval(metricInterval),
			sdkmetric.WithTimeout(exportTimeout))),
	)
	t.tracerProvider, t.loggerProvider, t.meterProvider = t.sdkTracer, t.sdkLogger, t.sdkMeter
	t.usage = newUsage(t.meterProvider.Meter(scope), t.tools.closed)
	if hs := s.history(ex); hs.on {
		var base http.RoundTripper
		if s.HTTPClient != nil {
			base = s.HTTPClient.Transport
		}
		t.history = newHistory(hs.endpoint, s.MetricsUser, s.Token, base, t.health.now, t.health)
		if err := t.history.register(t.meterProvider.Meter(scope)); err != nil {
			t.history = nil
		}
	}
	t.enabled = true
	return t
}

// instanceID names this start of the playmat, so two instances during a
// deploy are told apart.
func instanceID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Enabled reports whether export is on.
func (t *Telemetry) Enabled() bool { return t != nil && t.enabled }

// Run runs the health watch and the usage history until ctx is done. It
// returns at once when export is off.
func (t *Telemetry) Run(ctx context.Context) {
	if !t.Enabled() {
		return
	}
	if t.history != nil {
		go t.history.run(ctx)
	}
	t.health.run(ctx)
}

// HistoryOn reports whether the usage history is kept.
func (t *Telemetry) HistoryOn() bool { return t.Enabled() && t.history != nil }

// ForceFlush sends everything queued now (tests use it to read what was
// sent without waiting for a batch).
func (t *Telemetry) ForceFlush(ctx context.Context) error {
	if !t.Enabled() {
		return nil
	}
	return errors.Join(t.sdkTracer.ForceFlush(ctx), t.sdkLogger.ForceFlush(ctx), t.sdkMeter.ForceFlush(ctx))
}

// Shutdown flushes what is queued and stops the providers, giving up after
// two seconds in all whatever the receiver does.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if !t.Enabled() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, shutdownWithin)
	defer cancel()
	errs := make(chan error, 3)
	for _, stop := range []func(context.Context) error{t.sdkTracer.Shutdown, t.sdkLogger.Shutdown, t.sdkMeter.Shutdown} {
		go func() { errs <- stop(ctx) }()
	}
	var err error
	for range 3 {
		select {
		case e := <-errs:
			err = errors.Join(err, e)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}
