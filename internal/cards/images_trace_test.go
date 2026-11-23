package cards

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
)

// spanRecorder is an HTTP transport noting, for each request, the span
// context its request context carried.
type spanRecorder struct {
	mu   sync.Mutex
	seen map[string]trace.SpanContext // "METHOD path" -> span context
}

func (s *spanRecorder) RoundTrip(r *http.Request) (*http.Response, error) {
	s.mu.Lock()
	if s.seen == nil {
		s.seen = map[string]trace.SpanContext{}
	}
	s.seen[r.Method+" "+r.URL.Path] = trace.SpanContextFromContext(r.Context())
	s.mu.Unlock()
	return http.DefaultTransport.RoundTrip(r)
}

func (s *spanRecorder) get(key string) (trace.SpanContext, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sc, ok := s.seen[key]
	return sc, ok
}

// TestImageCopyRunsUnderTheRequestSpan: a card request that queues an
// image copy hands its span on, so the copy's source fetches and store
// writes run under it, while the request returns at once without the
// copy. A copy queued without a request carries no span.
func TestImageCopyRunsUnderTheRequestSpan(t *testing.T) {
	f := newImageFixture(t)
	rec := &spanRecorder{}
	cfg := f.srv.Config()
	cfg.HTTP = &http.Client{Transport: rec, Timeout: 5 * time.Second}
	store, err := spaces.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ic := NewImageCache(store, cfg.PublicBase())
	ic.Source = f.host.URL
	ic.Now = f.clock.Now
	ic.logf = f.log.logf
	ic.HTTP = &http.Client{Transport: rec, Timeout: 5 * time.Second}
	f.ic = ic
	f.run(t)
	f.waitReady(t, 2*time.Second)

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x4b, 0xf9, 0x2f, 0x35, 0x77, 0xb3, 0x4d, 0xa6, 0xa3, 0xce, 0x92, 0x9d, 0x0e, 0x0e, 0x47, 0x36},
		SpanID:     trace.SpanID{0x00, 0xf0, 0x67, 0xaa, 0x0b, 0xa9, 0x02, 0xb7},
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	release := f.host.hold()
	start := time.Now()
	if img, _, _ := ic.ImageContext(ctx, f.src(1)); img != f.src(1) {
		t.Fatalf("first answer %q, want the source", img)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("ImageContext waited %v for the copy", d)
	}
	release()
	ic.Image(f.src(2)) // no request: no span
	f.settle(t, 5*time.Second)

	for _, key := range []string{
		"GET /en/sv/sv1/1/low.webp", "GET /en/sv/sv1/1/high.webp",
		"PUT /" + cfg.Bucket + "/images/en/sv/sv1/1/low.webp", "PUT /" + cfg.Bucket + "/images/en/sv/sv1/1/high.webp",
	} {
		got, ok := rec.get(key)
		if !ok {
			t.Errorf("no request %s", key)
			continue
		}
		if got.TraceID() != sc.TraceID() || got.SpanID() != sc.SpanID() {
			t.Errorf("%s ran under span %s/%s, want the card request's %s/%s", key, got.TraceID(), got.SpanID(), sc.TraceID(), sc.SpanID())
		}
	}
	for _, key := range []string{"GET /en/sv/sv1/2/low.webp", "PUT /" + cfg.Bucket + "/images/en/sv/sv1/2/high.webp"} {
		got, ok := rec.get(key)
		if !ok {
			t.Errorf("no request %s", key)
		} else if got.IsValid() {
			t.Errorf("%s, queued without a request, ran under span %s", key, got.TraceID())
		}
	}
	ic.mu.Lock()
	left := len(ic.spanCtx)
	ic.mu.Unlock()
	if left != 0 {
		t.Errorf("%d span contexts kept after the copies finished", left)
	}
}
