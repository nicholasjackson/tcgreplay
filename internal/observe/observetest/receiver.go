// Package observetest is a fake Grafana Cloud OTLP receiver for tests: an
// HTTPS server that keeps every request it is sent, decompressed, or that
// hangs or refuses on demand. Nothing in it reaches a real Grafana.
package observetest

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	collogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/proto"
)

// Request is one request the receiver was sent.
type Request struct {
	Path   string
	Header http.Header
	Body   []byte // decompressed
}

// Receiver is a fake OTLP/HTTP endpoint. Its URL is the base that
// TCGREPLAY_GRAFANA_OTLP_URL would hold; signals arrive at URL+"/v1/…".
type Receiver struct {
	URL string
	srv *httptest.Server

	mu     sync.Mutex
	reqs   []Request
	status int  // the status to answer with; 0 means 200
	hang   bool // block every request until the test ends
	done   chan struct{}
}

// Option sets how the receiver answers.
type Option func(*Receiver)

// Status makes the receiver answer every request with code.
func Status(code int) Option { return func(r *Receiver) { r.status = code } }

// Hang makes the receiver hold every request open until the test ends.
func Hang() Option { return func(r *Receiver) { r.hang = true } }

// New starts a receiver, closed when t ends.
func New(t testing.TB, opts ...Option) *Receiver {
	t.Helper()
	r := &Receiver{done: make(chan struct{})}
	for _, o := range opts {
		o(r)
	}
	r.srv = httptest.NewTLSServer(http.HandlerFunc(r.serve))
	r.URL = r.srv.URL + "/otlp"
	t.Cleanup(func() {
		close(r.done)
		r.srv.Close()
	})
	return r
}

// Client returns an HTTP client that trusts the receiver's certificate,
// for observe.Settings.HTTPClient.
func (r *Receiver) Client() *http.Client { return r.srv.Client() }

// CertFile writes the receiver's certificate to a PEM file and returns its
// path, for a subprocess to trust through SSL_CERT_FILE.
func (r *Receiver) CertFile(t testing.TB) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "receiver.pem")
	b := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: r.srv.Certificate().Raw})
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func (r *Receiver) serve(w http.ResponseWriter, req *http.Request) {
	var body io.Reader = req.Body
	if req.Header.Get("Content-Encoding") == "gzip" {
		if zr, err := gzip.NewReader(req.Body); err == nil {
			body = zr
		}
	}
	b, _ := io.ReadAll(body)
	r.mu.Lock()
	r.reqs = append(r.reqs, Request{Path: req.URL.Path, Header: req.Header.Clone(), Body: b})
	status, hang := r.status, r.hang
	r.mu.Unlock()
	if hang {
		select {
		case <-r.done:
		case <-req.Context().Done():
		}
		return
	}
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
}

// Requests returns every request received so far.
func (r *Receiver) Requests() []Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Request(nil), r.reqs...)
}

// Paths returns the path of every request received so far.
func (r *Receiver) Paths() []string {
	var out []string
	for _, q := range r.Requests() {
		out = append(out, q.Path)
	}
	return out
}

// Contains reports whether any received body holds b.
func (r *Receiver) Contains(b []byte) bool {
	for _, q := range r.Requests() {
		if bytes.Contains(q.Body, b) {
			return true
		}
	}
	return false
}

// Unreachable returns an https URL nothing listens on.
func Unreachable(t testing.TB) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return "https://" + addr + "/otlp"
}

// LogRecord is one OTLP log record the receiver was sent, decoded.
type LogRecord struct {
	Body     string
	Severity string
	Attrs    map[string]string
	TraceID  string // hex, empty when none
	SpanID   string
	Resource map[string]string
}

// Logs decodes every log record received so far, failing t on a body that
// is not an OTLP logs request.
func (r *Receiver) Logs(t testing.TB) []LogRecord {
	t.Helper()
	var out []LogRecord
	for _, q := range r.Requests() {
		if q.Path != "/otlp/v1/logs" {
			continue
		}
		var req collogs.ExportLogsServiceRequest
		if err := proto.Unmarshal(q.Body, &req); err != nil {
			t.Fatalf("logs request: %v", err)
		}
		for _, rl := range req.ResourceLogs {
			res := attrs(rl.GetResource().GetAttributes())
			for _, sl := range rl.ScopeLogs {
				for _, lr := range sl.LogRecords {
					out = append(out, LogRecord{
						Body:     lr.GetBody().GetStringValue(),
						Severity: lr.GetSeverityText(),
						Attrs:    attrs(lr.GetAttributes()),
						TraceID:  hexID(lr.GetTraceId()),
						SpanID:   hexID(lr.GetSpanId()),
						Resource: res,
					})
				}
			}
		}
	}
	return out
}

// Span is one OTLP span the receiver was sent, decoded.
type Span struct {
	Name                    string
	TraceID, SpanID, Parent string // hex; Parent is empty for a root
	Kind                    string // "server", "client", "internal", …
	Attrs                   map[string]string
	StatusError             bool
	StatusMessage           string
	Links                   []string // "traceid/spanid" of each link
}

// Spans decodes every span received so far, failing t on a body that is
// not an OTLP traces request.
func (r *Receiver) Spans(t testing.TB) []Span {
	t.Helper()
	var out []Span
	for _, q := range r.Requests() {
		if q.Path != "/otlp/v1/traces" {
			continue
		}
		var req coltrace.ExportTraceServiceRequest
		if err := proto.Unmarshal(q.Body, &req); err != nil {
			t.Fatalf("traces request: %v", err)
		}
		for _, rs := range req.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, sp := range ss.Spans {
					s := Span{
						Name:          sp.GetName(),
						TraceID:       hexID(sp.GetTraceId()),
						SpanID:        hexID(sp.GetSpanId()),
						Parent:        hexID(sp.GetParentSpanId()),
						Kind:          strings.ToLower(strings.TrimPrefix(sp.GetKind().String(), "SPAN_KIND_")),
						Attrs:         attrs(sp.GetAttributes()),
						StatusError:   sp.GetStatus().GetCode() == 2,
						StatusMessage: sp.GetStatus().GetMessage(),
					}
					for _, l := range sp.GetLinks() {
						s.Links = append(s.Links, hexID(l.GetTraceId())+"/"+hexID(l.GetSpanId()))
					}
					out = append(out, s)
				}
			}
		}
	}
	return out
}

func attrs(kvs []*commonpb.KeyValue) map[string]string {
	m := map[string]string{}
	for _, kv := range kvs {
		switch v := kv.GetValue().GetValue().(type) {
		case *commonpb.AnyValue_StringValue:
			m[kv.GetKey()] = v.StringValue
		case *commonpb.AnyValue_IntValue:
			m[kv.GetKey()] = fmt.Sprint(v.IntValue)
		case *commonpb.AnyValue_BoolValue:
			m[kv.GetKey()] = fmt.Sprint(v.BoolValue)
		case *commonpb.AnyValue_DoubleValue:
			m[kv.GetKey()] = fmt.Sprint(v.DoubleValue)
		default:
			m[kv.GetKey()] = fmt.Sprint(kv.GetValue())
		}
	}
	return m
}

func hexID(b []byte) string {
	if len(b) == 0 || bytes.Equal(b, make([]byte, len(b))) {
		return ""
	}
	return hex.EncodeToString(b)
}
