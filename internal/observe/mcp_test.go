package observe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const testPlayer = "p_6cef43a09af39824"

// toolCall is a tools/call request for tool carrying traceparent (none
// when empty) in its HTTP headers.
func toolCall(tool, traceparent string) *mcp.CallToolRequest {
	h := http.Header{}
	if traceparent != "" {
		h.Set("traceparent", traceparent)
	}
	return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Name: tool}, Extra: &mcp.RequestExtra{Header: h}}
}

// staleContext is a context holding an ended span, standing in for the
// initialize request's context the MCP SDK hands every tool call.
func staleContext(t *testing.T, tel *Telemetry) (context.Context, trace.SpanContext) {
	t.Helper()
	ctx, span := tel.tracerProvider.Tracer("test").Start(context.Background(), "POST /mcp")
	span.End()
	return ctx, span.SpanContext()
}

// spanNamed returns the only exported span named name, failing otherwise.
func spanNamed(t *testing.T, exp *tracetest.InMemoryExporter, name string) tracetest.SpanStub {
	t.Helper()
	var found []tracetest.SpanStub
	for _, s := range exp.GetSpans() {
		if s.Name == name {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d spans named %q, want 1", len(found), name)
	}
	return found[0]
}

// TestMCPMiddlewareSpanFromRequestHeaders: each tools/call is a span
// "tools/call <tool>" in the trace its HTTP request's headers carry,
// a child of the span they name (not of the stale span in the session's
// context), naming the tool, the assistant and the player; the tool runs
// with that span in its context. Two calls with two requests' headers
// land in two traces.
func TestMCPMiddlewareSpanFromRequestHeaders(t *testing.T) {
	tel, exp := traced(t)
	stale, staleSC := staleContext(t, tel)
	var inner []trace.SpanContext
	next := func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		inner = append(inner, trace.SpanContextFromContext(ctx))
		return &mcp.CallToolResult{}, nil
	}
	h := tel.MCPMiddleware(func(mcp.Request) (string, string) { return testPlayer, "Claude" })(next)
	if _, err := h(stale, "tools/call", toolCall("get_state", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")); err != nil {
		t.Fatal(err)
	}
	if _, err := h(stale, "tools/call", toolCall("draw", "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01")); err != nil {
		t.Fatal(err)
	}

	for i, c := range []struct{ name, tool, traceID, parent string }{
		{"tools/call get_state", "get_state", "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7"},
		{"tools/call draw", "draw", "0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331"},
	} {
		s := spanNamed(t, exp, c.name)
		if s.SpanContext.TraceID().String() != c.traceID || s.Parent.SpanID().String() != c.parent {
			t.Errorf("%s in trace %s parent %s, want trace %s parent %s", c.name, s.SpanContext.TraceID(), s.Parent.SpanID(), c.traceID, c.parent)
		}
		if s.SpanContext.TraceID() == staleSC.TraceID() {
			t.Errorf("%s is in the stale session context's trace", c.name)
		}
		if s.SpanKind != trace.SpanKindInternal {
			t.Errorf("%s kind %v, want internal", c.name, s.SpanKind)
		}
		a := attrMap(s.Attributes)
		if a["tcgreplay.tool"] != c.tool || a["tcgreplay.assistant"] != "Claude" || a["tcgreplay.player"] != testPlayer {
			t.Errorf("%s attributes %v", c.name, a)
		}
		if s.Status.Code != codes.Unset {
			t.Errorf("%s status %v, want unset for a call that succeeded", c.name, s.Status)
		}
		if inner[i].SpanID() != s.SpanContext.SpanID() {
			t.Errorf("%s: the tool ran with span %s in its context, want %s", c.name, inner[i].SpanID(), s.SpanContext.SpanID())
		}
	}
}

// TestMCPMiddlewareCollapsesAssistant: known assistant names pass as they
// are; any other name, or none, is "other".
func TestMCPMiddlewareCollapsesAssistant(t *testing.T) {
	for _, c := range []struct{ assistant, want string }{
		{"Claude", "Claude"}, {"ChatGPT", "ChatGPT"}, {"Codex", "Codex"}, {"Gemini", "Gemini"},
		{"cursor-vscode", "other"}, {"claude", "other"}, {"", "other"},
	} {
		tel, exp := traced(t)
		next := func(context.Context, string, mcp.Request) (mcp.Result, error) { return &mcp.CallToolResult{}, nil }
		h := tel.MCPMiddleware(func(mcp.Request) (string, string) { return testPlayer, c.assistant })(next)
		h(context.Background(), "tools/call", toolCall("ping", ""))
		if got := attrMap(spanNamed(t, exp, "tools/call ping").Attributes)["tcgreplay.assistant"]; got != c.want {
			t.Errorf("assistant %q sent as %q, want %q", c.assistant, got, c.want)
		}
	}
}

// TestMCPMiddlewareMarksFailures: a call that returns an error result or
// an error marks its span failed with no message of its own (never the
// error's text); a message an inner layer already set (the recorder's
// redacted one) is kept.
func TestMCPMiddlewareMarksFailures(t *testing.T) {
	for _, c := range []struct {
		name     string
		next     mcp.MethodHandler
		wantCode codes.Code
		wantDesc string
	}{
		{"succeeds", func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return &mcp.CallToolResult{}, nil
		}, codes.Unset, ""},
		{"error result", func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "PASTEDSECRET is not a card"}}}, nil
		}, codes.Error, ""},
		{"error", func(context.Context, string, mcp.Request) (mcp.Result, error) {
			return nil, errors.New("PASTEDSECRET broke")
		}, codes.Error, ""},
		{"already failed", func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
			trace.SpanFromContext(ctx).SetStatus(codes.Error, "line 1: not a battle log")
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "PASTEDSECRET"}}}, nil
		}, codes.Error, "line 1: not a battle log"},
	} {
		tel, exp := traced(t)
		h := tel.MCPMiddleware(func(mcp.Request) (string, string) { return testPlayer, "Claude" })(c.next)
		h(context.Background(), "tools/call", toolCall("import_log", ""))
		s := spanNamed(t, exp, "tools/call import_log")
		if s.Status.Code != c.wantCode || s.Status.Description != c.wantDesc {
			t.Errorf("%s: status %v %q, want %v %q", c.name, s.Status.Code, s.Status.Description, c.wantCode, c.wantDesc)
		}
	}
}

// TestMCPMiddlewarePassesOtherMethods: requests other than tools/call get
// no span, and with export off the middleware adds none either.
func TestMCPMiddlewarePassesOtherMethods(t *testing.T) {
	tel, exp := traced(t)
	var called int
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) { called++; return nil, nil }
	acct := func(mcp.Request) (string, string) { return testPlayer, "Claude" }
	tel.MCPMiddleware(acct)(next)(context.Background(), "tools/list", &mcp.ListToolsRequest{})
	if called != 1 || len(exp.GetSpans()) != 0 {
		t.Errorf("tools/list: next called %d times, %d spans; want 1 and none", called, len(exp.GetSpans()))
	}

	off := Start(context.Background(), Settings{})
	var sc trace.SpanContext
	offNext := func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		sc = trace.SpanContextFromContext(ctx)
		return &mcp.CallToolResult{}, nil
	}
	if _, err := off.MCPMiddleware(acct)(offNext)(context.Background(), "tools/call", toolCall("ping", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")); err != nil {
		t.Fatal(err)
	}
	if sc.IsValid() {
		t.Errorf("with export off the tool ran in span context %v, want none", sc)
	}
}

// TestCarryContextWritesTheRequestSpan: inside the tracing wrapper, the
// MCP handler sees a traceparent naming the request's own server span,
// replacing the one the client sent; with export off it is the handler
// itself.
func TestCarryContextWritesTheRequestSpan(t *testing.T) {
	tel, exp := traced(t)
	var got string
	mux := http.NewServeMux()
	mux.Handle("/mcp", tel.CarryContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("traceparent")
	})))
	srv := httptest.NewServer(tel.Handler(mux))
	defer srv.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/mcp", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	s := spanNamed(t, exp, "POST /mcp")
	want := "00-" + s.SpanContext.TraceID().String() + "-" + s.SpanContext.SpanID().String() + "-01"
	if got != want {
		t.Errorf("MCP handler saw traceparent %q, want the request span's %q", got, want)
	}

	off := Start(context.Background(), Settings{})
	if off.CarryContext(mux) != http.Handler(mux) {
		t.Error("CarryContext with export off is not the handler itself")
	}
}
