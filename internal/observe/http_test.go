package observe

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// traced returns a Telemetry whose spans go, through the privacy filter,
// to an in-memory exporter, synchronously.
func traced(t *testing.T) (*Telemetry, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()), sdktrace.WithSyncer(filterExporter{exp}))
	t.Cleanup(func() { tp.Shutdown(context.Background()) })
	return &Telemetry{enabled: true, tracerProvider: tp, health: newHealth(nil, nil)}, exp
}

func attrMap(kvs []attribute.KeyValue) map[string]string {
	m := map[string]string{}
	for _, kv := range kvs {
		m[string(kv.Key)] = kv.Value.Emit()
	}
	return m
}

func testMux() *http.ServeMux {
	mux := http.NewServeMux()
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "ok") })
	mux.Handle("GET /board", ok)
	mux.Handle("/mcp", ok)
	mux.Handle("/ws", ok)
	mux.Handle("/api/", ok)
	mux.Handle("/", ok)
	return mux
}

func TestHandlerNamesSpansByRoute(t *testing.T) {
	tel, exp := traced(t)
	srv := httptest.NewServer(tel.Handler(testMux()))
	defer srv.Close()
	for _, c := range []struct{ method, path, name, route string }{
		{"GET", "/board?board=SECRETHANDLE", "GET /board", "/board"},
		{"POST", "/mcp", "POST /mcp", "/mcp"},
		{"GET", "/ws", "GET /ws", "/ws"},
		{"GET", "/api/card?src=x", "GET /api/", "/api/"},
		{"GET", "/privacy", "GET /", "/"},
	} {
		exp.Reset()
		req, _ := http.NewRequest(c.method, srv.URL+c.path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		spans := exp.GetSpans()
		if len(spans) != 1 {
			t.Fatalf("%s %s: %d spans", c.method, c.path, len(spans))
		}
		s := spans[0]
		a := attrMap(s.Attributes)
		if s.Name != c.name || a["http.route"] != c.route || s.SpanKind != trace.SpanKindServer {
			t.Errorf("%s %s: span %q kind %v route %q, want %q route %q server", c.method, c.path, s.Name, s.SpanKind, a["http.route"], c.name, c.route)
		}
		if a["http.response.status_code"] != "200" || a["http.request.method"] != c.method {
			t.Errorf("%s %s: attributes %v", c.method, c.path, a)
		}
	}
}

func TestHandlerJoinsIncomingTrace(t *testing.T) {
	tel, exp := traced(t)
	srv := httptest.NewServer(tel.Handler(testMux()))
	defer srv.Close()
	for _, flags := range []string{"00", "01"} {
		exp.Reset()
		req, _ := http.NewRequest("GET", srv.URL+"/board", nil)
		req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-"+flags)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		spans := exp.GetSpans()
		if len(spans) != 1 {
			t.Fatalf("flags %s: %d spans exported, want the server span even when unsampled", flags, len(spans))
		}
		s := spans[0]
		if s.SpanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" || s.Parent.SpanID().String() != "00f067aa0ba902b7" {
			t.Errorf("flags %s: span in trace %s parent %s, want the incoming trace and span", flags, s.SpanContext.TraceID(), s.Parent.SpanID())
		}
		if len(s.Links) != 0 {
			t.Errorf("flags %s: incoming context linked rather than adopted", flags)
		}
	}
}

func TestFilterKeepsOnlyAllowedAttributes(t *testing.T) {
	tel, exp := traced(t)
	tracer := tel.tracerProvider.Tracer("test")
	_, span := tracer.Start(context.Background(), "GET /auth/google/callback", trace.WithAttributes(
		attribute.String("client.address", "203.0.113.9"),
		attribute.String("network.peer.address", "203.0.113.9"),
		attribute.Int("network.peer.port", 51234),
		attribute.String("user_agent.original", "Mozilla/5.0 TESTAGENT"),
		attribute.String("url.query", "code=GOOGLECODE&state=OAUTHSTATE"),
		attribute.String("url.full", "https://oauth2.googleapis.com/token?code=GOOGLECODE&state=OAUTHSTATE"),
		attribute.String("url.path", "/auth/google/callback"),
		attribute.String("http.request.method", "GET"),
		attribute.Int("http.response.status_code", 302),
		attribute.String("tcgreplay.player", "p_6cef43a09af39824"),
	))
	span.AddEvent("exception", trace.WithAttributes(attribute.String("exception.message", "Get https://x/?code=GOOGLECODE")))
	span.AddEvent("tcgreplay.note")
	span.SetStatus(codes.Error, `Get "https://oauth2.googleapis.com/token?code=GOOGLECODE&state=OAUTHSTATE": timeout`)
	span.End()

	s := exp.GetSpans()[0]
	a := attrMap(s.Attributes)
	want := map[string]string{
		"url.full":                  "https://oauth2.googleapis.com/token",
		"url.path":                  "/auth/google/callback",
		"http.request.method":       "GET",
		"http.response.status_code": "302",
		"tcgreplay.player":          "p_6cef43a09af39824",
	}
	if len(a) != len(want) {
		t.Errorf("attributes %v, want exactly %v", a, want)
	}
	for k, v := range want {
		if a[k] != v {
			t.Errorf("attribute %s = %q, want %q", k, a[k], v)
		}
	}
	if len(s.Events) != 1 || s.Events[0].Name != "tcgreplay.note" {
		t.Errorf("events %v, want only tcgreplay.note", s.Events)
	}
	if s.Status.Description != `Get "https://oauth2.googleapis.com/token": timeout` {
		t.Errorf("status description %q, want its query cut", s.Status.Description)
	}
	all := s.Name + s.Status.Description
	for _, kv := range s.Attributes {
		all += kv.Value.Emit()
	}
	for _, e := range s.Events {
		all += e.Name
		for _, kv := range e.Attributes {
			all += kv.Value.Emit()
		}
	}
	for _, secret := range []string{"203.0.113.9", "TESTAGENT", "GOOGLECODE", "OAUTHSTATE"} {
		if strings.Contains(all, secret) {
			t.Errorf("exported span carries %q", secret)
		}
	}
}

func TestHandlerKeepsWebsocketAndStreaming(t *testing.T) {
	tel, _ := traced(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		typ, b, err := c.Read(r.Context())
		if err == nil {
			c.Write(r.Context(), typ, b)
		}
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	srv := httptest.NewServer(tel.Handler(mux))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatalf("websocket through the wrapper: %v", err)
	}
	c.Write(ctx, websocket.MessageText, []byte("echo"))
	if _, b, err := c.Read(ctx); err != nil || string(b) != "echo" {
		t.Errorf("websocket echo = %q, %v", b, err)
	}
	c.CloseNow()

	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/mcp", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || line != "data: first\n" {
		t.Errorf("streamed event = %q, %v; the flush did not get through", line, err)
	}
}

func TestClientSpansAreChildrenAndSendNoContext(t *testing.T) {
	tel, exp := traced(t)
	var gotHeader http.Header
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gotHeader = r.Header.Clone() }))
	defer up.Close()
	client := tel.Client(nil)
	if client.Timeout != 30*time.Second {
		t.Errorf("Client(nil) timeout = %v, want 30s", client.Timeout)
	}
	ctx, parent := tel.tracerProvider.Tracer("test").Start(context.Background(), "GET /api/")
	req, _ := http.NewRequestWithContext(ctx, "PUT", up.URL+"/images/x.webp?X-Amz-Signature=SIG", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	parent.End()

	var cs sdktrace.ReadOnlySpan
	for _, s := range exp.GetSpans().Snapshots() {
		if s.SpanKind() == trace.SpanKindClient {
			cs = s
		}
	}
	if cs == nil {
		t.Fatal("no client span")
	}
	host := strings.TrimPrefix(up.URL, "http://")
	if cs.Name() != "PUT "+host || cs.Parent().SpanID() != parent.SpanContext().SpanID() || cs.SpanContext().TraceID() != parent.SpanContext().TraceID() {
		t.Errorf("client span %q parent %s, want PUT %s under the request span", cs.Name(), cs.Parent().SpanID(), host)
	}
	if a := attrMap(cs.Attributes()); a["url.full"] != up.URL+"/images/x.webp" || a["server.address"] != "127.0.0.1" {
		t.Errorf("client span attributes %v", a)
	}
	if gotHeader.Get("traceparent") != "" {
		t.Errorf("trace context sent to the outside service: %q", gotHeader.Get("traceparent"))
	}

	off := Start(context.Background(), Settings{})
	base := &http.Client{Timeout: time.Second}
	if off.Client(base) != base || off.Client(nil) != nil {
		t.Error("Client with export off does not hand back its base")
	}
	h := http.NewServeMux()
	if off.Handler(h) != h {
		t.Error("Handler with export off is not the handler itself")
	}
}
