package observe

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TestBoardSpanIsALinkedRoot: each board action is a new root span
// "board <verb>", in a trace of its own, linked to the live connection's
// span, naming the action and the player; the action runs with it in its
// context.
func TestBoardSpanIsALinkedRoot(t *testing.T) {
	tel, exp := traced(t)
	wsCtx, ws := tel.tracerProvider.Tracer("test").Start(context.Background(), "GET /ws", trace.WithSpanKind(trace.SpanKindServer))
	ctx1, end1 := tel.BoardSpan(wsCtx, "move", testPlayer)
	end1(nil)
	ctx2, end2 := tel.BoardSpan(wsCtx, "attach", testPlayer)
	end2(nil)
	ws.End()

	wsSC := ws.SpanContext()
	var traces []trace.TraceID
	for _, c := range []struct {
		verb string
		ctx  context.Context
	}{{"move", ctx1}, {"attach", ctx2}} {
		s := spanNamed(t, exp, "board "+c.verb)
		if s.Parent.IsValid() {
			t.Errorf("board %s has parent %s, want a root", c.verb, s.Parent.SpanID())
		}
		if s.SpanContext.TraceID() == wsSC.TraceID() {
			t.Errorf("board %s is in the live connection's trace", c.verb)
		}
		if len(s.Links) != 1 || s.Links[0].SpanContext.TraceID() != wsSC.TraceID() || s.Links[0].SpanContext.SpanID() != wsSC.SpanID() {
			t.Errorf("board %s links %v, want one to the live connection's span %s/%s", c.verb, s.Links, wsSC.TraceID(), wsSC.SpanID())
		}
		if s.SpanKind != trace.SpanKindInternal {
			t.Errorf("board %s kind %v, want internal", c.verb, s.SpanKind)
		}
		if a := attrMap(s.Attributes); a["tcgreplay.board.verb"] != c.verb || a["tcgreplay.player"] != testPlayer {
			t.Errorf("board %s attributes %v", c.verb, a)
		}
		if s.Status.Code != codes.Unset {
			t.Errorf("board %s status %v, want unset", c.verb, s.Status)
		}
		if trace.SpanContextFromContext(c.ctx).SpanID() != s.SpanContext.SpanID() {
			t.Errorf("board %s: the action's context does not hold its span", c.verb)
		}
		traces = append(traces, s.SpanContext.TraceID())
	}
	if traces[0] == traces[1] {
		t.Error("two board actions share a trace")
	}
}

// TestBoardSpanMarksFailures: a failed action marks its span failed,
// keeping the redacted message the recorder set and never adding the
// error's own text; without a live span in the context there is no link;
// with export off nothing changes.
func TestBoardSpanMarksFailures(t *testing.T) {
	tel, exp := traced(t)
	ctx, end := tel.BoardSpan(context.Background(), "draw", testPlayer)
	end(errors.New("PASTEDSECRET broke"))
	ctx, end = tel.BoardSpan(context.Background(), "move", testPlayer)
	trace.SpanFromContext(ctx).SetStatus(codes.Error, "no card p1-99 in hand")
	end(errors.New("PASTEDSECRET broke"))

	draw := spanNamed(t, exp, "board draw")
	if draw.Status.Code != codes.Error || draw.Status.Description != "" {
		t.Errorf("failed draw status %v %q, want error with no message", draw.Status.Code, draw.Status.Description)
	}
	if len(draw.Links) != 0 {
		t.Errorf("board span with no live connection has links %v", draw.Links)
	}
	move := spanNamed(t, exp, "board move")
	if move.Status.Code != codes.Error || move.Status.Description != "no card p1-99 in hand" {
		t.Errorf("failed move status %v %q, want the recorder's message kept", move.Status.Code, move.Status.Description)
	}

	off := Start(context.Background(), Settings{})
	base := context.WithValue(context.Background(), traceKeyForTest{}, "base")
	got, offEnd := off.BoardSpan(base, "move", testPlayer)
	offEnd(errors.New("x"))
	if got != base || trace.SpanContextFromContext(got).IsValid() {
		t.Error("BoardSpan with export off changed the context")
	}
}

type traceKeyForTest struct{}
