package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
	"github.com/nicholasjackson/tcgreplay/internal/observe"
	"github.com/nicholasjackson/tcgreplay/internal/observe/observetest"
	"github.com/nicholasjackson/tcgreplay/internal/spaces"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// flushedSpans sends everything h's telemetry holds and returns the spans
// r received.
func flushedSpans(t *testing.T, h *hostedServer, r *observetest.Receiver) []observetest.Span {
	t.Helper()
	if err := h.tel.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
	return r.Spans(t)
}

// serverSpan returns the first server span named name.
func serverSpan(spans []observetest.Span, name string) *observetest.Span {
	for i, s := range spans {
		if s.Kind == "server" && s.Name == name {
			return &spans[i]
		}
	}
	return nil
}

// TestHostedTracesEveryRequest: each kind of request to the hosted playmat
// (website, board page, card data, live connection, sign-in steps, MCP,
// admin) is a server span named by its route, and the websocket and MCP
// streams still work through the wrapper (the session would fail
// otherwise).
func TestHostedTracesEveryRequest(t *testing.T) {
	captureLog(t)
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = sendingTo(r) })
	ws := telemetrySession(t, h)
	cookie := h.cookie(t)
	getWith(t, h.ts.URL+"/privacy", nil, nil)
	getWith(t, h.ts.URL+"/api/card?src=nowhere", cookie, nil)
	getWith(t, h.ts.URL+"/admin", cookie, nil)
	getWith(t, h.ts.URL+"/board?board=x", cookie, nil)

	// The live connection's span ends when it closes.
	ws.Close(websocket.StatusNormalClosure, "")
	var spans []observetest.Span
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if spans = flushedSpans(t, h, r); serverSpan(spans, "GET /ws") != nil || time.Now().After(deadline) {
			break
		}
	}
	for _, name := range []string{
		"GET /",                     // the website
		"GET /board",                // the board page
		"GET /api/",                 // card data
		"GET /ws",                   // the live connection
		"GET /auth/login",           // board sign-in
		"GET /auth/google/callback", // back from Google
		"GET /authorize",            // the connector's sign-in
		"POST /token",               // the connector's token
		"POST /register",            // the connector's registration
		"POST /mcp",                 // tool calls
		"GET /admin",                // admin
	} {
		s := serverSpan(spans, name)
		if s == nil {
			t.Errorf("no server span %q", name)
			continue
		}
		if want := strings.TrimPrefix(strings.TrimPrefix(name, "GET "), "POST "); s.Attrs["http.route"] != want {
			t.Errorf("span %q has http.route %q, want %q", name, s.Attrs["http.route"], want)
		}
	}
	for _, s := range spans {
		if s.Kind == "server" && !strings.Contains(s.Name, " /") {
			t.Errorf("server span %q is not named by a route", s.Name)
		}
	}
}

// TestHostedSignInTraceHoldsTheGoogleCall: the token exchange with Google
// is a client span inside the sign-in callback's trace.
func TestHostedSignInTraceHoldsTheGoogleCall(t *testing.T) {
	captureLog(t)
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = sendingTo(r) })
	h.cookie(t)

	spans := flushedSpans(t, h, r)
	cb := serverSpan(spans, "GET /auth/google/callback")
	if cb == nil {
		t.Fatal("no sign-in callback span")
	}
	tokenURL, _ := url.Parse(h.g.Endpoint().TokenURL)
	var found bool
	for _, s := range spans {
		if s.Kind == "client" && s.Name == "POST "+tokenURL.Host {
			found = true
			if s.TraceID != cb.TraceID || s.Parent != cb.SpanID {
				t.Errorf("Google call span in trace %s parent %s, want under the callback %s/%s", s.TraceID, s.Parent, cb.TraceID, cb.SpanID)
			}
			if s.Attrs["url.full"] != h.g.Endpoint().TokenURL {
				t.Errorf("Google call url.full = %q", s.Attrs["url.full"])
			}
		}
	}
	if !found {
		t.Errorf("no client span for the token exchange with %s in %v", tokenURL.Host, spans)
	}
}

// TestHostedAdminCheckTracesTheCardSetFetch: an admin's check for card
// sets shows the fetch from the published card sets as a client span
// inside the check request's trace.
func TestHostedAdminCheckTracesTheCardSetFetch(t *testing.T) {
	captureSyncLog(t)
	host := publishCardsets(t, adminSets()...)
	t.Setenv("TCGREPLAY_ADMINS", "One@Example.com")
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) {
		c.offline, c.cardsetsURL = false, host.URL
		c.telemetry = sendingTo(r)
	})
	cookie := h.cookie(t)
	resp, body := sendWith(t, http.MethodPost, h.ts.URL+"/admin/check", cookie, fromPlaymat)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("check = %d %q", resp.StatusCode, body)
	}

	spans := flushedSpans(t, h, r)
	check := serverSpan(spans, "POST /admin/")
	if check == nil {
		t.Fatalf("no admin check span in %v", spans)
	}
	hostURL, _ := url.Parse(host.URL)
	var fetches int
	for _, s := range spans {
		if s.Kind == "client" && s.Name == "GET "+hostURL.Host {
			fetches++
			if s.TraceID != check.TraceID {
				t.Errorf("card set fetch %s in trace %s, want the check's %s", s.Attrs["url.full"], s.TraceID, check.TraceID)
			}
		}
	}
	if fetches == 0 {
		t.Errorf("no client span for the card set fetch from %s", hostURL.Host)
	}
}

// TestHostedJoinsIncomingTrace: a request carrying Cloudflare's W3C trace
// context, sampled or not, produces its span in that trace, parented on
// the incoming span.
func TestHostedJoinsIncomingTrace(t *testing.T) {
	captureLog(t)
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = sendingTo(r) })
	for _, flags := range []string{"00", "01"} {
		getWith(t, h.ts.URL+"/privacy", nil, func(req *http.Request) {
			req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e47"+flags+"-00f067aa0ba902b7-"+flags)
		})
	}
	spans := flushedSpans(t, h, r)
	for _, flags := range []string{"00", "01"} {
		var ok bool
		for _, s := range spans {
			if s.Kind == "server" && s.TraceID == "4bf92f3577b34da6a3ce929d0e0e47"+flags && s.Parent == "00f067aa0ba902b7" {
				ok = true
			}
		}
		if !ok {
			t.Errorf("no server span joined the incoming trace with flags %s", flags)
		}
	}
}

// TestHostedSpansCarryNothingIdentifying: no span carries a client
// address, user agent, query string, Google authorization code, OAuth
// state or board handle, and no per-route HTTP metrics are sent.
func TestHostedSpansCarryNothingIdentifying(t *testing.T) {
	captureLog(t)
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = sendingTo(r) })
	telemetrySession(t, h)
	cookie := h.cookie(t)
	getWith(t, h.ts.URL+"/board?board=HANDLEMARKER", cookie, func(req *http.Request) { req.Header.Set("User-Agent", "AGENTMARKER/1.0") })

	spans := flushedSpans(t, h, r)
	if len(spans) == 0 {
		t.Fatal("no spans sent")
	}
	for _, s := range spans {
		for _, k := range []string{"client.address", "network.peer.address", "network.peer.port", "user_agent.original", "url.query", "http.target"} {
			if v, ok := s.Attrs[k]; ok {
				t.Errorf("span %q carries %s = %q", s.Name, k, v)
			}
		}
		if strings.Contains(s.Attrs["url.full"], "?") {
			t.Errorf("span %q url.full keeps its query: %q", s.Name, s.Attrs["url.full"])
		}
	}
	for _, marker := range []string{"HANDLEMARKER", "AGENTMARKER", "Go-http-client", "code=", "state=", "board=", h.g.ClientSecret, cookie.Value} {
		if r.Contains([]byte(marker)) {
			t.Errorf("something sent carries %q", marker)
		}
	}
	for _, metric := range []string{"http.server", "http.client"} {
		if r.Contains([]byte(metric)) {
			t.Errorf("HTTP metrics (%s) were sent", metric)
		}
	}
}

// TestHostedImageClientsTraced: hosted mode's card image cache sends its
// image store writes, its fetches from the image source and its CDN
// purges through tracing clients, so each is a client span.
func TestHostedImageClientsTraced(t *testing.T) {
	captureSyncLog(t)
	srv := spacestest.New(t)
	setSpacesEnv(t, srv.Config())
	const tok = "dop_v1_trace_test"
	t.Setenv("TCGREPLAY_SPACES_CDN_TOKEN", tok)
	cdn := spacestest.NewCDN(t, spaces.NewCDN(srv.Config(), tok).Origin, tok)
	host := newImageHost(t)
	cat := publishedCatalog(t, adminSets()...)
	r := observetest.New(t)
	tel := observe.Start(context.Background(), *sendingTo(r))
	t.Cleanup(func() { tel.Shutdown(context.Background()) })
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", cdnAPI: cdn.URL}

	images := hostedImages(c, tel)
	if images == nil {
		t.Fatal("image cache off with all four settings")
	}
	// The image source client hostedImages built is traced...
	resp, err := images.HTTP.Get(host.URL + "/en/sv/sv1/001/low.webp")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	// ...and for the copy below its requests for TCGdex go to the
	// stand-in, through a tracing client as hosted mode's would.
	images.HTTP = tel.Client(newReroute(t, "", host.URL).client())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go images.Run(ctx)
	boardCard(t, cat, images)
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, ok := srv.Object("images/en/sv/sv1/001/high.webp"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("image not copied to the shared store")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := images.CDN.(*spaces.CDN).Purge(ctx, []string{"images/en/sv/sv1/001/high.webp"}); err != nil {
		t.Fatalf("purge: %v", err)
	}
	tel.ForceFlush(context.Background())

	names := map[string]bool{}
	for _, s := range r.Spans(t) {
		if s.Kind == "client" {
			names[s.Name] = true
		}
	}
	hostOf := func(raw string) string { u, _ := url.Parse(raw); return u.Host }
	for _, want := range []string{
		"GET " + hostOf(host.URL),              // the image source
		"PUT " + hostOf(srv.Config().Endpoint), // the image store
		"DELETE " + hostOf(cdn.URL),            // the CDN purge
	} {
		if !names[want] {
			t.Errorf("no client span %q in %v", want, names)
		}
	}
}

// waitSpans flushes h's telemetry until done holds for the spans r has
// received, or five seconds pass, and returns the spans. A request's
// server span ends just after its response, so it may arrive a moment
// after the client has its answer.
func waitSpans(t *testing.T, h *hostedServer, r *observetest.Receiver, done func([]observetest.Span) bool) []observetest.Span {
	t.Helper()
	var spans []observetest.Span
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if spans = flushedSpans(t, h, r); done(spans) || time.Now().After(deadline) {
			return spans
		}
	}
}

// spansNamed returns the spans named name.
func spansNamed(spans []observetest.Span, name string) []observetest.Span {
	var out []observetest.Span
	for _, s := range spans {
		if s.Name == name {
			out = append(out, s)
		}
	}
	return out
}

// spanByID returns the span whose ID is id.
func spanByID(spans []observetest.Span, id string) *observetest.Span {
	for i, s := range spans {
		if id != "" && s.SpanID == id {
			return &spans[i]
		}
	}
	return nil
}

// recordLogs returns the log records r received for records of event.
func recordLogs(t *testing.T, r *observetest.Receiver, event string) []observetest.LogRecord {
	t.Helper()
	var out []observetest.LogRecord
	for _, l := range r.Logs(t) {
		if l.Attrs["log.source"] == "record" && l.Attrs["event"] == event {
			out = append(out, l)
		}
	}
	return out
}

// recordField is the field k of the record l carries as its body.
func recordField(t *testing.T, l observetest.LogRecord, k string) string {
	t.Helper()
	var rec map[string]string
	if err := json.Unmarshal([]byte(l.Body), &rec); err != nil {
		t.Fatalf("record body %q: %v", l.Body, err)
	}
	return rec[k]
}

// TestHostedToolCallsTracedPerRequest: each tool call is a span "tools/call
// <tool>" inside the trace of the POST /mcp request that carried it, a
// child of that request's server span, naming the tool, the assistant
// (known names as they are, any other "other") and the player's ID. Two
// calls on one session land in two traces, and each call's tool_call
// record carries its span.
func TestHostedToolCallsTracedPerRequest(t *testing.T) {
	captureLog(t)
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = sendingTo(r) })
	h.g.SetAccount(userA)
	toks := hostauthtest.Connect(t, h.public, h.ts.URL, h.g)
	cs := h.connectAs(t, "claude-ai", toks.AccessToken)
	callTool(t, cs, "ping", nil)
	callTool(t, cs, "ping", nil)
	callTool(t, h.connectAs(t, "Some New Assistant", toks.AccessToken), "ping", nil)

	parentOf := func(spans []observetest.Span, s observetest.Span) *observetest.Span {
		p := spanByID(spans, s.Parent)
		if p == nil || p.Kind != "server" || p.Name != "POST /mcp" || p.TraceID != s.TraceID {
			return nil
		}
		return p
	}
	spans := waitSpans(t, h, r, func(spans []observetest.Span) bool {
		calls := spansNamed(spans, "tools/call ping")
		for _, s := range calls {
			if parentOf(spans, s) == nil {
				return false
			}
		}
		return len(calls) == 3
	})
	calls := spansNamed(spans, "tools/call ping")
	if len(calls) != 3 {
		t.Fatalf("%d tools/call ping spans, want 3: %v", len(calls), spans)
	}
	traces := map[string]bool{}
	assistants := map[string]int{}
	ids := map[string]string{} // span ID -> trace ID
	for _, s := range calls {
		if parentOf(spans, s) == nil {
			t.Errorf("tool span %s (trace %s, parent %s) is not a child of a POST /mcp server span in its trace", s.SpanID, s.TraceID, s.Parent)
		}
		if s.Kind != "internal" {
			t.Errorf("tool span kind %q, want internal", s.Kind)
		}
		if s.Attrs["tcgreplay.tool"] != "ping" || s.Attrs["tcgreplay.player"] != aID {
			t.Errorf("tool span attributes %v, want tool ping, player %s", s.Attrs, aID)
		}
		if s.StatusError {
			t.Errorf("tool span for a ping that succeeded is marked failed")
		}
		traces[s.TraceID] = true
		assistants[s.Attrs["tcgreplay.assistant"]]++
		ids[s.SpanID] = s.TraceID
	}
	if len(traces) != 3 {
		t.Errorf("3 tool calls in %d traces, want 3 (one per request)", len(traces))
	}
	if assistants["Claude"] != 2 || assistants["other"] != 1 {
		t.Errorf("tool spans' assistants %v, want Claude twice and other once", assistants)
	}
	recs := recordLogs(t, r, "tool_call")
	if len(recs) != 3 {
		t.Fatalf("%d tool_call records sent, want 3", len(recs))
	}
	for _, l := range recs {
		if tr, ok := ids[l.SpanID]; !ok || tr != l.TraceID {
			t.Errorf("tool_call record carries %s/%s, want one of the tool spans", l.TraceID, l.SpanID)
		}
	}
}

// TestHostedLimitedCallTraced: a tool call the request limit refuses still
// has its span, marked failed, and the limited record carries that span's
// trace and span IDs.
func TestHostedLimitedCallTraced(t *testing.T) {
	captureLog(t)
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) {
		c.rateLimit = 1
		c.telemetry = sendingTo(r)
	})
	if out, isErr := toolResult(t, h.connect(t), "ping", nil); !isErr || !strings.Contains(out, "too many requests") {
		t.Fatalf("ping over a limit of 1: %q, want refused", out)
	}
	spans := waitSpans(t, h, r, func(spans []observetest.Span) bool { return len(spansNamed(spans, "tools/call ping")) == 1 })
	calls := spansNamed(spans, "tools/call ping")
	if len(calls) != 1 {
		t.Fatalf("%d spans for the refused ping, want 1", len(calls))
	}
	s := calls[0]
	if s.Attrs["tcgreplay.tool"] != "ping" || s.Attrs["tcgreplay.player"] != oneID || s.Attrs["tcgreplay.assistant"] != "other" {
		t.Errorf("refused call's span attributes %v", s.Attrs)
	}
	if !s.StatusError {
		t.Error("refused call's span is not marked failed")
	}
	var found bool
	for _, l := range recordLogs(t, r, "limited") {
		if l.Attrs["reason"] != "rate" {
			continue
		}
		found = true
		if l.TraceID != s.TraceID || l.SpanID != s.SpanID {
			t.Errorf("limited record carries %s/%s, want the refused call's span %s/%s", l.TraceID, l.SpanID, s.TraceID, s.SpanID)
		}
	}
	if !found {
		t.Error("no limited record (reason rate) sent")
	}
}

// TestHostedFailedCallsMarkTheirSpans: the error record of a malformed
// import carries the trace and span IDs of that tool call's span, which is
// marked failed with the record's redacted message; likewise a failed
// board action's error record and its board span.
func TestHostedFailedCallsMarkTheirSpans(t *testing.T) {
	captureLog(t)
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = sendingTo(r) })
	ws := telemetrySession(t, h)
	ws.Close(websocket.StatusNormalClosure, "")

	var errs []observetest.LogRecord
	spans := waitSpans(t, h, r, func(spans []observetest.Span) bool {
		errs = recordLogs(t, r, "error")
		for _, l := range errs {
			if spanByID(spans, l.SpanID) == nil {
				return false
			}
		}
		return len(errs) == 2
	})
	if len(errs) != 2 {
		t.Fatalf("%d error records sent, want 2 (the import and the move)", len(errs))
	}
	for _, c := range []struct{ field, value, span string }{
		{"tool", "import_log", "tools/call import_log"},
		{"action", "move", "board move"},
	} {
		var l *observetest.LogRecord
		for i := range errs {
			if errs[i].Attrs[c.field] == c.value {
				l = &errs[i]
			}
		}
		if l == nil {
			t.Errorf("no error record with %s %s", c.field, c.value)
			continue
		}
		msg := recordField(t, *l, "message")
		if msg == "" {
			t.Errorf("error record for %s has no message: %s", c.value, l.Body)
		}
		s := spanByID(spans, l.SpanID)
		if s == nil {
			t.Errorf("error record for %s carries span %s, which was not sent", c.value, l.SpanID)
			continue
		}
		if s.Name != c.span || s.TraceID != l.TraceID {
			t.Errorf("error record for %s carries span %q in trace %s (record trace %s), want %q", c.value, s.Name, s.TraceID, l.TraceID, c.span)
		}
		if !s.StatusError || s.StatusMessage != msg {
			t.Errorf("span %q status error=%v %q, want failed with the record's message %q", s.Name, s.StatusError, s.StatusMessage, msg)
		}
	}
}

// TestHostedBoardActionsTraced: each board action is its own root span
// "board <verb>", in a trace of its own, linked to the live connection's
// GET /ws span and naming the action and the player.
func TestHostedBoardActionsTraced(t *testing.T) {
	captureLog(t)
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = sendingTo(r) })
	c := telemetrySession(t, h)
	boardFail(t, c, 2, "move", map[string]any{"iid": "p1-1", "to": "elsewhere"})
	// The live connection's span ends when it closes.
	c.Close(websocket.StatusNormalClosure, "")
	spans := waitSpans(t, h, r, func(spans []observetest.Span) bool {
		return serverSpan(spans, "GET /ws") != nil && len(spansNamed(spans, "board move")) == 2
	})
	ws := serverSpan(spans, "GET /ws")
	if ws == nil {
		t.Fatal("no GET /ws span")
	}
	moves := spansNamed(spans, "board move")
	if len(moves) != 2 {
		t.Fatalf("%d board move spans, want 2", len(moves))
	}
	link := ws.TraceID + "/" + ws.SpanID
	for _, s := range moves {
		if s.Parent != "" {
			t.Errorf("board span has parent %s, want a root", s.Parent)
		}
		if s.TraceID == ws.TraceID {
			t.Error("board span is in the live connection's trace")
		}
		if len(s.Links) != 1 || s.Links[0] != link {
			t.Errorf("board span links %v, want [%s]", s.Links, link)
		}
		if s.Kind != "internal" || s.Attrs["tcgreplay.board.verb"] != "move" || s.Attrs["tcgreplay.player"] != aID {
			t.Errorf("board span kind %q attributes %v, want internal, verb move, player %s", s.Kind, s.Attrs, aID)
		}
	}
	if moves[0].TraceID == moves[1].TraceID {
		t.Error("two board actions share a trace")
	}
}

// TestHostedRecordsCarryTheirRequestTrace: sign-in, sign-in refusal at the
// user cap (and its limited record), board refusal and sign-out records
// carry the trace and span of the request that caused them; the idle
// game's game_expired and the idle account's account_removed records
// carry none and are sent as written.
func TestHostedRecordsCarryTheirRequestTrace(t *testing.T) {
	captureLog(t)
	clock := &idleClock{t: time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)}
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) {
		c.maxUsers = 1
		c.idleTimeout = 2 * time.Minute
		c.now = clock.Now
		c.telemetry = sendingTo(r)
	})
	csA, cookieA := h.signInAs(t, userA)
	importFor(t, csA, testLogA, "game-a", "")
	getWith(t, h.ts.URL+"/board?board=someone-else", cookieA, nil) // another account's board link
	h.g.SetAccount(userB)
	h.checkBoardBusy(t, "B", 1)
	h.logoutBoard(t, cookieA)
	clock.Advance(2*time.Minute + time.Second)
	toolResult(t, csA, "get_state", nil) // finds the game expired
	clock.Advance(25 * time.Hour)
	h.reg.Sweep() // removes the idle account

	want := []struct{ event, player, span string }{
		{"sign_in", aID, "GET /auth/google/callback"},
		{"sign_in_refused", bID, "GET /auth/google/callback"},
		{"limited", bID, "GET /auth/google/callback"},
		{"board_refused", aID, "GET /board"},
		{"sign_out", aID, "POST /auth/logout"},
	}
	spans := waitSpans(t, h, r, func(spans []observetest.Span) bool {
		for _, w := range want {
			for _, l := range recordLogs(t, r, w.event) {
				if spanByID(spans, l.SpanID) == nil {
					return false
				}
			}
		}
		return true
	})
	for _, w := range want {
		recs := recordLogs(t, r, w.event)
		if len(recs) == 0 {
			t.Errorf("no %s record sent", w.event)
		}
		for _, l := range recs {
			if l.Attrs["player"] != w.player {
				t.Errorf("%s record for %s, want %s", w.event, l.Attrs["player"], w.player)
			}
			s := spanByID(spans, l.SpanID)
			if s == nil {
				t.Errorf("%s record carries span %q, which is not a span sent", w.event, l.SpanID)
				continue
			}
			if s.Kind != "server" || s.Name != w.span || s.TraceID != l.TraceID {
				t.Errorf("%s record carries %s/%s, the %s span %q; want the %q server span", w.event, l.TraceID, l.SpanID, s.Kind, s.Name, w.span)
			}
		}
	}

	written := h.events.String()
	for _, c := range []struct{ event, reason string }{{"game_expired", ""}, {"account_removed", "idle"}} {
		var n int
		for _, l := range recordLogs(t, r, c.event) {
			if c.reason != "" && l.Attrs["reason"] != c.reason {
				continue
			}
			n++
			if l.TraceID != "" || l.SpanID != "" {
				t.Errorf("background %s record carries trace %q span %q, want none", c.event, l.TraceID, l.SpanID)
			}
			if !strings.Contains(written, l.Body+"\n") {
				t.Errorf("background %s record sent as %s, not as written", c.event, l.Body)
			}
		}
		if n == 0 {
			t.Errorf("no %s record sent", c.event)
		}
	}
}

// TestHostedSignOutRemovalCarriesItsTrace: when an account's only live
// sign-in, a board session, ends through POST /auth/logout, its
// account_removed record (reason signed_out) carries the trace and span
// of that request's server span.
func TestHostedSignOutRemovalCarriesItsTrace(t *testing.T) {
	captureLog(t)
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = sendingTo(r) })
	h.g.SetAccount(userA)
	h.logoutBoard(t, h.cookie(t)) // board only: no connector holds a sign-in

	removed := func() []observetest.LogRecord {
		var out []observetest.LogRecord
		for _, l := range recordLogs(t, r, "account_removed") {
			if l.Attrs["reason"] == "signed_out" {
				out = append(out, l)
			}
		}
		return out
	}
	var recs []observetest.LogRecord
	spans := waitSpans(t, h, r, func(spans []observetest.Span) bool {
		recs = removed()
		return len(recs) == 1 && spanByID(spans, recs[0].SpanID) != nil
	})
	if len(recs) != 1 {
		t.Fatalf("%d account_removed (signed_out) records sent, want 1", len(recs))
	}
	l := recs[0]
	if l.Attrs["player"] != aID {
		t.Errorf("account_removed record for %s, want %s", l.Attrs["player"], aID)
	}
	s := spanByID(spans, l.SpanID)
	if s == nil || s.Kind != "server" || s.Name != "POST /auth/logout" || s.TraceID != l.TraceID {
		t.Errorf("account_removed record carries %s/%s (span %+v), want the POST /auth/logout server span", l.TraceID, l.SpanID, s)
	}
}

// TestHostedAdminRecordsCarryTheirRequestTrace: an admin's action record
// and a non-admin's admin_refused record carry the trace and span of
// their requests.
func TestHostedAdminRecordsCarryTheirRequestTrace(t *testing.T) {
	captureLog(t)
	t.Setenv("TCGREPLAY_ADMINS", "One@Example.com")
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = sendingTo(r) })
	admin := h.cookie(t)
	resp, body := sendWithBody(t, http.MethodPost, h.ts.URL+"/admin/player", url.Values{"email": {userA.Email}}.Encode(), func(req *http.Request) {
		fromPlaymat(req)
		req.AddCookie(admin)
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin lookup = %d %q", resp.StatusCode, body)
	}
	h.g.SetAccount(capSecond)
	if resp, _ := getWith(t, h.ts.URL+"/admin", h.cookie(t), nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin /admin: %d, want 403", resp.StatusCode)
	}

	want := []struct{ event, player, span string }{
		{"admin_action", oneID, "POST /admin/"},
		{"admin_refused", twoID, "GET /admin"},
	}
	spans := waitSpans(t, h, r, func(spans []observetest.Span) bool {
		for _, w := range want {
			for _, l := range recordLogs(t, r, w.event) {
				if spanByID(spans, l.SpanID) == nil {
					return false
				}
			}
		}
		return true
	})
	for _, w := range want {
		recs := recordLogs(t, r, w.event)
		if len(recs) != 1 {
			t.Errorf("%d %s records sent, want 1", len(recs), w.event)
			continue
		}
		l := recs[0]
		if l.Attrs["player"] != w.player {
			t.Errorf("%s record for %s, want %s", w.event, l.Attrs["player"], w.player)
		}
		s := spanByID(spans, l.SpanID)
		if s == nil || s.Kind != "server" || s.Name != w.span || s.TraceID != l.TraceID {
			t.Errorf("%s record carries %s/%s (span %+v), want the %q server span", w.event, l.TraceID, l.SpanID, s, w.span)
		}
	}
}
