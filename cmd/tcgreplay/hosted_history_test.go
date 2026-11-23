package main

import (
	"context"
	"log"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/observe/observetest"
)

// TestMainHostedSaysUsageHistoryOff runs the binary's main in hosted mode
// with export pointing at a fake receiver and no metrics query settings:
// the start-up line says export is on and the usage history is off, with
// no warning, and the start-up line still reaches the receiver.
func TestMainHostedSaysUsageHistoryOff(t *testing.T) {
	r := observetest.New(t)
	port := freePort(t)
	cmd, stderr, _ := helperCmd(t, "PORT="+port, "TCGREPLAY_PUBLIC_URL=https://play.example.com",
		"TCGREPLAY_GOOGLE_CLIENT_ID=env-client", "TCGREPLAY_GOOGLE_CLIENT_SECRET=env-secret",
		"SSL_CERT_FILE="+r.CertFile(t),
		"TCGREPLAY_GRAFANA_OTLP_URL="+r.URL, "TCGREPLAY_GRAFANA_INSTANCE_ID=123456", "TCGREPLAY_GRAFANA_TOKEN="+grafanaToken,
		"TCGREPLAY_GRAFANA_METRICS_URL=", "TCGREPLAY_GRAFANA_METRICS_USER=")
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
	want := "player IDs on, telemetry export: on (Grafana Cloud " + host + "), " +
		"usage history: off (no TCGREPLAY_GRAFANA_METRICS_URL or TCGREPLAY_GRAFANA_METRICS_USER)\n"
	if !strings.Contains(out, want) {
		t.Errorf("start-up line does not end %q:\n%s", want, out)
	}
	if strings.Contains(out, "warning: usage history") {
		t.Errorf("a warning about the usage history with its settings unset:\n%s", out)
	}
	if strings.Contains(out, grafanaToken) {
		t.Errorf("the token was printed:\n%s", out)
	}
	var bodies []string
	for _, l := range r.Logs(t) {
		bodies = append(bodies, l.Body)
	}
	if !slices.ContainsFunc(bodies, func(b string) bool {
		return strings.Contains(b, " hosted: listening on ") && strings.HasSuffix(b, "usage history: off (no TCGREPLAY_GRAFANA_METRICS_URL or TCGREPLAY_GRAFANA_METRICS_USER)")
	}) {
		t.Errorf("start-up line not sent with the usage history off; sent %q", bodies)
	}
}

// TestHostedTelemetrySaysWhetherHistoryIsOn: hosted mode's start-up text
// names the usage history's state; metrics settings that are set but
// wrong log a warning line of their own, and export stays on regardless.
func TestHostedTelemetrySaysWhetherHistoryIsOn(t *testing.T) {
	r := observetest.New(t)
	for _, tc := range []struct {
		name             string
		metricsURL, user string
		text, warning    string
		history          bool
	}{
		{"unset", "", "", "usage history: off (no TCGREPLAY_GRAFANA_METRICS_URL or TCGREPLAY_GRAFANA_METRICS_USER)", "", false},
		{"user not a number", "https://prometheus.example/api/prom", "stack",
			"usage history: off (TCGREPLAY_GRAFANA_METRICS_USER is not a number)",
			"warning: usage history: off (TCGREPLAY_GRAFANA_METRICS_USER is not a number)", false},
		{"set", "https://prometheus.example/api/prom", "654321", "usage history: on (prometheus.example)", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLog(t)
			s := sendingTo(r)
			s.MetricsURL, s.MetricsUser = tc.metricsURL, tc.user
			prev := log.Writer()
			tel, text := hostedTelemetry(context.Background(), config{telemetry: s})
			t.Cleanup(func() {
				tel.Shutdown(context.Background())
				log.SetOutput(prev)
			})
			host := strings.TrimSuffix(strings.TrimPrefix(r.URL, "https://"), "/otlp")
			if want := "telemetry export: on (Grafana Cloud " + host + "), " + tc.text; text != want {
				t.Errorf("start-up text = %q\nwant           %q", text, want)
			}
			if !tel.Enabled() {
				t.Error("export off")
			}
			if tel.HistoryOn() != tc.history {
				t.Errorf("HistoryOn() = %v, want %v", tel.HistoryOn(), tc.history)
			}
			got := logs.String()
			if tc.warning != "" && !strings.Contains(got, tc.warning+"\n") {
				t.Errorf("no warning line %q; logged:\n%s", tc.warning, got)
			}
			if tc.warning == "" && strings.Contains(got, "warning: usage history") {
				t.Errorf("unexpected usage history warning:\n%s", got)
			}
		})
	}
}

// TestHostedSendsEverythingWithoutHistory: with export on and the usage
// history off, a session's records and counters still reach Grafana, and
// no daily gauge is sent.
func TestHostedSendsEverythingWithoutHistory(t *testing.T) {
	captureLog(t)
	r := observetest.New(t)
	h := startHostedWith(t, testPublic, func(c *config) { c.telemetry = sendingTo(r) })
	if h.tel.HistoryOn() {
		t.Fatal("usage history on without metrics settings")
	}
	telemetrySession(t, h)
	if err := h.tel.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
	var sent []string
	for _, l := range r.Logs(t) {
		sent = append(sent, l.Body)
	}
	recs := strings.Split(strings.TrimSpace(h.events.String()), "\n")
	if len(recs) < 8 {
		t.Fatalf("only %d records written; the session did not run", len(recs))
	}
	for _, rec := range recs {
		if !slices.Contains(sent, rec) {
			t.Errorf("record not sent: %s", rec)
		}
	}
	var metrics []byte
	for _, q := range r.Requests() {
		if q.Path == "/otlp/v1/metrics" {
			metrics = append(metrics, q.Body...)
		}
	}
	for _, want := range []string{"tcgreplay.player_days", "tcgreplay.sign_ins", "tcgreplay.imports", "tcgreplay.new_games"} {
		if !strings.Contains(string(metrics), want) {
			t.Errorf("metrics sent lack %q", want)
		}
	}
	if strings.Contains(string(metrics), "tcgreplay.usage_daily") {
		t.Error("the daily gauge was sent with the usage history off")
	}
}
