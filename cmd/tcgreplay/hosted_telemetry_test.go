package main

import (
	"context"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
	"github.com/nicholasjackson/tcgreplay/internal/observe"
	"github.com/nicholasjackson/tcgreplay/internal/observe/observetest"
)

// grafanaToken stands in for the Grafana Cloud access policy token.
const grafanaToken = "glc_testGRAFANAtoken0123456789"

// sendingTo returns telemetry settings that send to r.
func sendingTo(r *observetest.Receiver) *observe.Settings {
	return &observe.Settings{OTLPURL: r.URL, InstanceID: "123456", Token: grafanaToken, HTTPClient: r.Client()}
}

// telemetrySession plays one player's session on h: a board sign-in, an
// assistant connection, an import, a new game, a malformed import and a
// failing board action. Every step must succeed as the player sees it.
// It returns the board's live connection, still open.
func telemetrySession(t *testing.T, h *hostedServer) *websocket.Conn {
	t.Helper()
	h.g.SetAccount(userA)
	cookie := h.cookie(t)
	toks := hostauthtest.Connect(t, h.public, h.ts.URL, h.g)
	cs := h.connectAs(t, "claude-ai", toks.AccessToken)
	deckText, logText := readTestdata(t, testDeckPath), readTestdata(t, testLogA)
	callObject(t, cs, "import_log", map[string]any{"log": logText, "deck": deckText, "id": "tel-imp"})
	callObject(t, cs, "new_game", map[string]any{"deck_p1": deckText, "deck_p2": deckText, "id": "tel-new"})
	if out, isErr := toolResult(t, cs, "import_log", map[string]any{"log": "stray line\n" + logText, "deck": deckText, "id": "tel-bad"}); !isErr {
		t.Fatalf("malformed import accepted: %s", out)
	}
	c, _ := boardSnapshot(t, h, cookie)
	boardFail(t, c, 1, "move", map[string]any{"iid": "p1-1", "to": "nowhere"})
	return c
}

// stdPrefix matches the std log package's date and time prefix.
var stdPrefix = regexp.MustCompile(`(?m)^\d{4}/\d\d/\d\d \d\d:\d\d:\d\d `)

// TestHostedRecordsAndLogLinesReachGrafana: every record written to
// standard output during a session arrives as a log record whose body is
// the same line, and the playmat's plain log lines (warnings included)
// arrive too, each marked with its source.
func TestHostedRecordsAndLogLinesReachGrafana(t *testing.T) {
	logs := captureLog(t)
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = sendingTo(r) })
	telemetrySession(t, h)
	log.Printf("warning: card images: a test warning")
	if err := h.tel.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	got := map[string]map[string]string{} // body -> attributes
	sev := map[string]string{}
	for _, l := range r.Logs(t) {
		got[l.Body] = l.Attrs
		sev[l.Body] = l.Severity
	}
	recs := strings.Split(strings.TrimSpace(h.events.String()), "\n")
	if len(recs) < 8 {
		t.Fatalf("only %d records written; the session did not run:\n%s", len(recs), h.events)
	}
	for _, rec := range recs {
		a, ok := got[rec]
		if !ok {
			t.Errorf("record not sent with an identical body: %s", rec)
			continue
		}
		if a["log.source"] != "record" {
			t.Errorf("record %s sent with log.source %q", rec, a["log.source"])
		}
		if strings.Contains(rec, `"level":"error"`) != (sev[rec] == "ERROR") {
			t.Errorf("record %s sent at severity %s", rec, sev[rec])
		}
	}
	var lines int
	for _, l := range strings.Split(strings.TrimSpace(stdPrefix.ReplaceAllString(logs.String(), "")), "\n") {
		// The test catalogue loads before serveHosted starts telemetry;
		// in runHosted it loads after, and TestMainHostedSendsStartupLines
		// checks card set lines are sent.
		if l == "" || strings.HasPrefix(l, "card sets: ") {
			continue
		}
		lines++
		if a, ok := got[l]; !ok || a["log.source"] != "log" {
			t.Errorf("log line not sent (or not as a log line): %q", l)
		}
	}
	if lines == 0 {
		t.Fatal("no log lines written; the check proved nothing")
	}
	if sev["warning: card images: a test warning"] != "WARN" {
		t.Errorf("warning line sent at severity %q, want WARN", sev["warning: card images: a test warning"])
	}
	wantSignIn := false
	for _, rec := range recs {
		if strings.Contains(rec, `"event":"sign_in"`) {
			wantSignIn = true
			if !strings.Contains(rec, `"provider":"google"`) || got[rec]["provider"] != "google" {
				t.Errorf("sign-in record %s (attributes %v) does not name provider google", rec, got[rec])
			}
		}
	}
	if !wantSignIn {
		t.Error("no sign_in record written")
	}
}

// TestHostedOutputUnchangedBySending: the records on standard output and
// the lines on standard error are the same with sending on as with it
// off, so the live log view looks as before.
func TestHostedOutputUnchangedBySending(t *testing.T) {
	run := func(send bool) (records, logLines string) {
		logs := captureLog(t)
		clock := &idleClock{t: time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)}
		h := startHostedWith(t, testPublic, func(c *config) {
			c.now = clock.Now
			if send {
				c.telemetry = sendingTo(observetest.New(t))
			}
		})
		telemetrySession(t, h)
		out := stdPrefix.ReplaceAllString(logs.String(), "")
		return h.events.String(), strings.ReplaceAll(out, h.ts.URL, "TESTSERVER")
	}
	offRecs, offLog := run(false)
	onRecs, onLog := run(true)
	if onRecs != offRecs {
		t.Errorf("records differ with sending on:\n--- off\n%s--- on\n%s", offRecs, onRecs)
	}
	if onLog != offLog {
		t.Errorf("log lines differ with sending on:\n--- off\n%s--- on\n%s", offLog, onLog)
	}
}

// TestHostedPlaysOnWhenGrafanaFails: with the receiver unreachable,
// hanging or refusing, a player signs in, imports, starts a game and acts
// on the board, with no added wait: the exporters wait up to five seconds,
// so any wait on them would show far beyond the allowance.
func TestHostedPlaysOnWhenGrafanaFails(t *testing.T) {
	timed := func(tel *observe.Settings) time.Duration {
		captureLog(t)
		h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = tel })
		if h.tel != nil {
			// Keep an export in flight (stuck, with a hanging receiver)
			// while the player plays, rather than waiting for a batch.
			log.Printf("card sets: a line to export")
			go h.tel.ForceFlush(context.Background())
			time.Sleep(50 * time.Millisecond)
		}
		start := time.Now()
		telemetrySession(t, h)
		return time.Since(start)
	}
	off := timed(nil)
	hang := observetest.New(t, observetest.Hang())
	reject := observetest.New(t, observetest.Status(http.StatusUnauthorized))
	for name, s := range map[string]*observe.Settings{
		"unreachable": {OTLPURL: observetest.Unreachable(t), InstanceID: "123456", Token: grafanaToken},
		"hanging":     sendingTo(hang),
		"rejecting":   sendingTo(reject),
	} {
		t.Run(name, func(t *testing.T) {
			if took := timed(s); took > off+time.Second {
				t.Errorf("session took %v with the receiver %s, %v with sending off", took, name, off)
			}
		})
	}
}

// TestMainHostedSendsStartupLines runs the binary's main in hosted mode
// with the Grafana settings pointing at a fake receiver: the start-up line
// says sending is on and never shows the token, and the start-up line,
// card set lines and warnings (including those logged while the settings
// are read, before telemetry starts) reach the receiver at WARN, flushed
// when the playmat stops.
func TestMainHostedSendsStartupLines(t *testing.T) {
	r := observetest.New(t)
	port := freePort(t)
	// No public URL and a bad rate limit, for two warnings.
	cmd, stderr, _ := helperCmd(t, "PORT="+port, "TCGREPLAY_PUBLIC_URL=", "TCGREPLAY_RATE_LIMIT=lots",
		"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret",
		"SSL_CERT_FILE="+r.CertFile(t),
		"TCGREPLAY_GRAFANA_OTLP_URL="+r.URL, "TCGREPLAY_GRAFANA_INSTANCE_ID=123456", "TCGREPLAY_GRAFANA_TOKEN="+grafanaToken)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() { cmd.Process.Kill() })
	waitHealthy(t, "http://127.0.0.1:"+port, 10*time.Second, exited, stderr)
	cmd.Process.Signal(syscall.SIGTERM)
	<-exited

	out := stderr.String()
	host := strings.TrimSuffix(strings.TrimPrefix(r.URL, "https://"), "/otlp")
	if !strings.Contains(out, "player IDs on, telemetry export: on (Grafana Cloud "+host+")") {
		t.Errorf("start-up line does not say sending is on:\n%s", out)
	}
	if strings.Contains(out, grafanaToken) {
		t.Errorf("the token was printed:\n%s", out)
	}
	var bodies []string
	sev := map[string]string{}
	for _, l := range r.Logs(t) {
		bodies = append(bodies, l.Body)
		sev[l.Body] = l.Severity
	}
	for _, w := range []string{
		`warning: TCGREPLAY_RATE_LIMIT="lots" is not a whole number; using 300`,
		"warning: no public URL set (-public-url or TCGREPLAY_PUBLIC_URL); using http://localhost:" + port,
	} {
		if !strings.Contains(out, w) {
			t.Errorf("stderr lacks %q:\n%s", w, out)
		}
		if sev[w] != "WARN" {
			t.Errorf("warning %q not sent at WARN (severity %q); sent %q", w, sev[w], bodies)
		}
	}
	if !slices.ContainsFunc(bodies, func(b string) bool {
		return strings.HasPrefix(b, "tcgreplay ") && strings.Contains(b, " hosted: listening on ")
	}) {
		t.Errorf("start-up line not sent; sent %q", bodies)
	}
	if !slices.ContainsFunc(bodies, func(b string) bool { return strings.HasPrefix(b, "card sets: ") }) {
		t.Errorf("card set line not sent; sent %q", bodies)
	}
	if r.Contains([]byte(grafanaToken)) {
		t.Error("the token was sent in a body")
	}
}

// TestMainHostedSaysWhetherSending: the start-up line says sending is off
// without settings and off (incomplete, with a warning line naming the
// setting) with some, never showing the token.
func TestMainHostedSaysWhetherSending(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want []string
	}{
		{"unset", nil, []string{"player IDs on, telemetry export: off (no TCGREPLAY_GRAFANA_* settings)"}},
		{"incomplete", []string{"TCGREPLAY_GRAFANA_OTLP_URL=https://otlp.example/otlp", "TCGREPLAY_GRAFANA_INSTANCE_ID=stack", "TCGREPLAY_GRAFANA_TOKEN=" + grafanaToken},
			[]string{"warning: telemetry export: off (incomplete: TCGREPLAY_GRAFANA_INSTANCE_ID is not a number)",
				"player IDs on, telemetry export: off (incomplete: TCGREPLAY_GRAFANA_INSTANCE_ID is not a number)"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			port := freePort(t)
			env := append([]string{"PORT=" + port, "TCGREPLAY_PUBLIC_URL=https://play.example.com",
				"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret",
				"TCGREPLAY_GRAFANA_OTLP_URL=", "TCGREPLAY_GRAFANA_INSTANCE_ID=", "TCGREPLAY_GRAFANA_TOKEN=",
				"TCGREPLAY_GRAFANA_METRICS_URL=", "TCGREPLAY_GRAFANA_METRICS_USER="}, tc.env...)
			cmd, stderr, _ := helperCmd(t, env...)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			t.Cleanup(func() { cmd.Process.Kill() })
			waitHealthy(t, "http://127.0.0.1:"+port, 10*time.Second, exited, stderr)
			cmd.Process.Signal(syscall.SIGTERM)
			<-exited
			out := stderr.String()
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("stderr lacks %q:\n%s", w, out)
				}
			}
			if strings.Contains(out, grafanaToken) {
				t.Errorf("the token was printed:\n%s", out)
			}
		})
	}
}

// TestRunbookListsProvider: the runbook's record catalogue lists the
// provider field and names it on sign_in.
func TestRunbookListsProvider(t *testing.T) {
	doc := readTestdata(t, "../../docs/observability.md")
	for _, want := range []string{"| `provider` | the sign-in provider (`google` or `password`), on `sign_in`", "| `sign_in` | `provider` (`google` or `password`) |", "reason `credentials`", "`throttled`"} {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/observability.md lacks %q", want)
		}
	}
}

// TestHostedCountsUsage: a hosted session's usage reaches Grafana as
// counters whose tool label is the registered tool's name, with no player
// ID anywhere in the metrics.
func TestHostedCountsUsage(t *testing.T) {
	captureLog(t)
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = sendingTo(r) })
	telemetrySession(t, h)
	if err := h.tel.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
	var metrics []byte
	for _, q := range r.Requests() {
		if q.Path == "/otlp/v1/metrics" {
			metrics = append(metrics, q.Body...)
		}
	}
	for _, want := range []string{"tcgreplay.player_days", "tcgreplay.sign_ins", "tcgreplay.tool_calls", "tcgreplay.imports",
		"tcgreplay.new_games", "tcgreplay.errors", "google", "import_log", "new_game", "Claude"} {
		if !strings.Contains(string(metrics), want) {
			t.Errorf("metrics sent lack %q", want)
		}
	}
	if strings.Contains(string(metrics), aID) || strings.Contains(string(metrics), userA.Email) {
		t.Error("metrics sent carry the player's ID or email")
	}
}

// TestToolNamesListsTheRegisteredTools: the counters' closed tool set is
// the tools the hosted MCP server serves.
func TestToolNamesListsTheRegisteredTools(t *testing.T) {
	g := hostauthtest.NewGoogle(t)
	t.Setenv("TCGREPLAY_GOOGLE_CLIENT_SECRET", g.ClientSecret)
	c := config{dataDir: t.TempDir(), cacheDir: t.TempDir(), seat: "p2", offline: true, googleClientID: g.ClientID, events: &syncBuffer{}}
	u, _ := hostedPublicURL(testPublic, ":8080")
	_, srv, _, err := newHosted(c, newCatalog(c), nil, u, g.Endpoint(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names, err := toolNames(srv)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"import_log", "new_game", "get_state", "ping"} {
		if !slices.Contains(names, want) {
			t.Errorf("tool names %v lack %s", names, want)
		}
	}
}

// TestRunbookReadsFromGrafana: the observability runbook reads records,
// usage and traces in Grafana Cloud, no longer tells the maintainer to
// read records in Insights, keeps platform metrics and alerts in App
// Platform, and covers setup, edge traces, export status and retention.
func TestRunbookReadsFromGrafana(t *testing.T) {
	doc := readTestdata(t, "../../docs/observability.md")
	for _, want := range []string{
		"## Setting up Grafana Cloud", "## Reading daily usage in Grafana", "## Finding a player's errors",
		"## Traces", "## Turning on edge traces", "## Export status", "## Retention",
		// setup: the token's scopes, every setting, the dashboard, the start-up line
		"`metrics:read`, `metrics:write`, `logs:write` and `traces:write`",
		"TCGREPLAY_GRAFANA_OTLP_URL", "TCGREPLAY_GRAFANA_INSTANCE_ID", "TCGREPLAY_GRAFANA_TOKEN",
		"TCGREPLAY_GRAFANA_METRICS_URL", "TCGREPLAY_GRAFANA_METRICS_USER",
		"docs/grafana/tcgreplay-usage.json", "telemetry export: on (Grafana Cloud", "usage history: on",
		// reading: daily usage, a player's errors via the admin lookup, the trace
		`tcgreplay_usage_daily{count="players"}`, `tcgreplay_usage_daily{count="tool_calls"}`,
		"Find a player's ID", `| player="p_3f9a1c0b7d2e4a68" | event="error"`, "trace_id",
		// edge traces
		"Forward to origin", "/v1/traces", "**Status:**",
		// platform metrics and alerts stay in DigitalOcean
		"Platform metrics and alerts: DigitalOcean",
		// export failure line
		"warning: telemetry export: sending to Grafana Cloud has failed for",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/observability.md lacks %q", want)
		}
	}
	for _, gone := range []string{"Insights > Logs**", "## Reading daily usage in Insights", "search `\"player\":"} {
		if strings.Contains(doc, gone) {
			t.Errorf("docs/observability.md still tells the maintainer to read records in Insights (%q)", gone)
		}
	}
	i := strings.Index(doc, "## Retention")
	if i < 0 {
		t.Fatal("no Retention section")
	}
	retention := doc[i:]
	for _, want := range []string{"**14 days**", "**six weeks**", "**7 days**", "Request traces", "checked on 2026-11-23", "grafana.com/pricing"} {
		if !strings.Contains(retention, want) {
			t.Errorf("Retention section lacks %q", want)
		}
	}
}
