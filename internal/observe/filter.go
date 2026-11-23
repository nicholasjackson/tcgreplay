package observe

import (
	"context"
	"regexp"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// allowedAttrs are the span attributes that may leave the process; any
// other attribute a library adds is dropped until someone adds it here on
// purpose. Keys under "tcgreplay." are the playmat's own and always pass.
var allowedAttrs = map[attribute.Key]bool{
	"http.request.method":       true,
	"http.route":                true,
	"http.response.status_code": true,
	"url.scheme":                true,
	"url.path":                  true,
	"url.full":                  true,
	"server.address":            true,
	"server.port":               true,
	"network.protocol.version":  true,
	"error.type":                true,
}

// query matches a URL's query string within free text.
var query = regexp.MustCompile(`\?[^\s"']*`)

// filterExporter is the single guarantee of what a trace may say: it
// passes only allow-listed attributes, strips query strings (Google's
// authorization code, OAuth state, board handles) from URLs and error
// descriptions, and drops every span event a library adds. Client
// addresses and user agents are never on the list.
type filterExporter struct {
	sdktrace.SpanExporter
}

func (f filterExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	out := make([]sdktrace.ReadOnlySpan, len(spans))
	for i, s := range spans {
		out[i] = filterSpan(s)
	}
	return f.SpanExporter.ExportSpans(ctx, out)
}

// filteredSpan is a span with its attributes, events and status replaced.
type filteredSpan struct {
	sdktrace.ReadOnlySpan
	attrs  []attribute.KeyValue
	events []sdktrace.Event
	status sdktrace.Status
}

func (s filteredSpan) Attributes() []attribute.KeyValue { return s.attrs }
func (s filteredSpan) Events() []sdktrace.Event         { return s.events }
func (s filteredSpan) Status() sdktrace.Status          { return s.status }

func filterSpan(s sdktrace.ReadOnlySpan) sdktrace.ReadOnlySpan {
	fs := filteredSpan{ReadOnlySpan: s, status: s.Status()}
	for _, kv := range s.Attributes() {
		switch {
		case kv.Key == "url.full":
			fs.attrs = append(fs.attrs, attribute.String("url.full", stripQuery(kv.Value.AsString())))
		case allowedAttrs[kv.Key] || strings.HasPrefix(string(kv.Key), "tcgreplay."):
			fs.attrs = append(fs.attrs, kv)
		}
	}
	for _, e := range s.Events() {
		if strings.HasPrefix(e.Name, "tcgreplay.") {
			fs.events = append(fs.events, e)
		}
	}
	fs.status.Description = query.ReplaceAllString(fs.status.Description, "")
	return fs
}

// stripQuery drops a URL's query and fragment.
func stripQuery(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		return u[:i]
	}
	return u
}
