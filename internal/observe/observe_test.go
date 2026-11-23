package observe

import (
	"context"
	"net/http"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/nicholasjackson/tcgreplay/internal/observe/observetest"
)

// on returns settings that send to the receiver at url, trusting client.
func on(url string, client *http.Client) Settings {
	return Settings{OTLPURL: url, InstanceID: "123456", Token: testToken, Version: "v-test", HTTPClient: client}
}

// sendOne hands one span, one log record and one counter increment to t.
func sendOne(tel *Telemetry) {
	ctx := context.Background()
	_, span := tel.tracerProvider.Tracer("test").Start(ctx, "GET /board")
	span.End()
	var rec otellog.Record
	rec.SetBody(attribute.StringValue("hello"))
	tel.loggerProvider.Logger("test").Emit(ctx, rec)
	c, _ := tel.meterProvider.Meter("test").Int64Counter("test.count")
	c.Add(ctx, 1)
}

func TestStartOnSendsEverySignal(t *testing.T) {
	r := observetest.New(t)
	tel := Start(context.Background(), on(r.URL, r.Client()))
	t.Cleanup(func() { tel.Shutdown(context.Background()) })
	if !tel.Enabled() {
		t.Fatal("Enabled() = false with all export settings")
	}
	sendOne(tel)
	if err := tel.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
	paths := r.Paths()
	for _, want := range []string{"/otlp/v1/traces", "/otlp/v1/logs", "/otlp/v1/metrics"} {
		if !slices.Contains(paths, want) {
			t.Errorf("receiver paths %v, missing %s", paths, want)
		}
	}
	// base64("123456:glc_SECRETTOKENvalue123"), worked out by hand.
	const wantAuth = "Basic MTIzNDU2OmdsY19TRUNSRVRUT0tFTnZhbHVlMTIz"
	for _, q := range r.Requests() {
		if got := q.Header.Get("Authorization"); got != wantAuth {
			t.Errorf("%s Authorization = %q, want %q", q.Path, got, wantAuth)
		}
		if got := q.Header.Get("Content-Encoding"); got != "gzip" {
			t.Errorf("%s Content-Encoding = %q, want gzip", q.Path, got)
		}
	}
	for _, want := range []string{"service.name", "tcgreplay", "service.version", "v-test", "service.instance.id", "deployment.environment.name", "production"} {
		if !r.Contains([]byte(want)) {
			t.Errorf("no body carries the resource value %q", want)
		}
	}
}

func TestStartOffSendsNothing(t *testing.T) {
	r := observetest.New(t)
	for name, s := range map[string]Settings{
		"unset":      {},
		"incomplete": {OTLPURL: r.URL, Token: testToken, HTTPClient: r.Client()},
		"bad id":     {OTLPURL: r.URL, InstanceID: "x", Token: testToken, HTTPClient: r.Client()},
	} {
		t.Run(name, func(t *testing.T) {
			tel := Start(context.Background(), s)
			if tel.Enabled() {
				t.Fatal("Enabled() = true")
			}
			sendOne(tel)
			if err := tel.ForceFlush(context.Background()); err != nil {
				t.Errorf("ForceFlush: %v", err)
			}
			if err := tel.Shutdown(context.Background()); err != nil {
				t.Errorf("Shutdown: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			tel.Run(ctx) // returns at once
		})
	}
	if got := r.Requests(); len(got) != 0 {
		t.Fatalf("receiver got %d requests with export off", len(got))
	}
}

// failing returns a receiver URL and client for each way Grafana can fail.
func failing(t *testing.T) map[string]Settings {
	hang := observetest.New(t, observetest.Hang())
	reject := observetest.New(t, observetest.Status(http.StatusUnauthorized))
	return map[string]Settings{
		"unreachable": on(observetest.Unreachable(t), nil),
		"hanging":     on(hang.URL, hang.Client()),
		"rejecting":   on(reject.URL, reject.Client()),
	}
}

func TestHandingOverNeverWaits(t *testing.T) {
	for name, s := range failing(t) {
		t.Run(name, func(t *testing.T) {
			tel := Start(context.Background(), s)
			t.Cleanup(func() { tel.Shutdown(context.Background()) })
			tracer := tel.tracerProvider.Tracer("test")
			logger := tel.loggerProvider.Logger("test")
			ctx := context.Background()
			var slowest time.Duration
			start := time.Now()
			for i := range 5000 {
				t0 := time.Now()
				_, span := tracer.Start(ctx, "GET /board")
				span.End()
				var rec otellog.Record
				rec.SetBody(attribute.IntValue(i))
				logger.Emit(ctx, rec)
				slowest = max(slowest, time.Since(t0))
			}
			total := time.Since(start)
			// Each hand-over is a queue push; the generous ceilings allow
			// for a GC pause on a busy machine but not for a network wait
			// (the exporters wait up to five seconds).
			if slowest > 100*time.Millisecond {
				t.Errorf("slowest span+record hand-over took %v", slowest)
			}
			if total > 2*time.Second {
				t.Errorf("5000 hand-overs took %v", total)
			}
		})
	}
}

func TestQueuedDataStaysBounded(t *testing.T) {
	hang := observetest.New(t, observetest.Hang())
	tel := Start(context.Background(), on(hang.URL, hang.Client()))
	t.Cleanup(func() { tel.Shutdown(context.Background()) })
	tracer := tel.tracerProvider.Tracer("test")
	logger := tel.loggerProvider.Logger("test")
	ctx := context.Background()
	big := strings.Repeat("x", 4096)

	heap := func() uint64 {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc
	}
	before := heap()
	// 40,000 spans and records of 4 KB each would hold over 300 MB if
	// nothing were dropped; two queues of 2,048 plus the batches in flight
	// hold a small fraction of that.
	for range 40000 {
		_, span := tracer.Start(ctx, "GET /board", trace.WithAttributes(attribute.String("tcgreplay.pad", big)))
		span.End()
		var rec otellog.Record
		rec.SetBody(attribute.StringValue(big))
		logger.Emit(ctx, rec)
	}
	grown := int64(heap()) - int64(before)
	if grown > 96<<20 {
		t.Fatalf("heap grew %d MB while the receiver hung; queues are not bounded", grown>>20)
	}
}

func TestShutdownFlushesForAtMostTwoSeconds(t *testing.T) {
	hang := observetest.New(t, observetest.Hang())
	tel := Start(context.Background(), on(hang.URL, hang.Client()))
	sendOne(tel)
	start := time.Now()
	tel.Shutdown(context.Background())
	if took := time.Since(start); took > 2500*time.Millisecond {
		t.Fatalf("Shutdown with a hanging receiver took %v, want at most about 2s", took)
	}
}

func TestShutdownFlushesQueuedData(t *testing.T) {
	r := observetest.New(t)
	tel := Start(context.Background(), on(r.URL, r.Client()))
	sendOne(tel)
	if err := tel.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if !slices.Contains(r.Paths(), "/otlp/v1/traces") || !slices.Contains(r.Paths(), "/otlp/v1/logs") {
		t.Fatalf("Shutdown did not flush the queued span and record: %v", r.Paths())
	}
}
