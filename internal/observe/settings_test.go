package observe

import (
	"strings"
	"testing"
)

const testToken = "glc_SECRETTOKENvalue123"

func TestSettingsFromEnvTrims(t *testing.T) {
	env := map[string]string{
		"TCGREPLAY_GRAFANA_OTLP_URL":     " https://otlp-gateway-prod-eu-west-2.grafana.net/otlp/ ",
		"TCGREPLAY_GRAFANA_INSTANCE_ID":  "123456\n",
		"TCGREPLAY_GRAFANA_TOKEN":        "\t" + testToken,
		"TCGREPLAY_GRAFANA_METRICS_URL":  "https://prometheus-prod-24-prod-eu-west-2.grafana.net/api/prom",
		"TCGREPLAY_GRAFANA_METRICS_USER": "654321",
	}
	s := SettingsFromEnv(func(k string) string { return env[k] })
	want := Settings{
		OTLPURL:     "https://otlp-gateway-prod-eu-west-2.grafana.net/otlp/",
		InstanceID:  "123456",
		Token:       testToken,
		MetricsURL:  "https://prometheus-prod-24-prod-eu-west-2.grafana.net/api/prom",
		MetricsUser: "654321",
	}
	if s.OTLPURL != want.OTLPURL || s.InstanceID != want.InstanceID || s.Token != want.Token ||
		s.MetricsURL != want.MetricsURL || s.MetricsUser != want.MetricsUser {
		t.Fatalf("SettingsFromEnv = %+v, want %+v", s, want)
	}
	if got := s.export().endpoint; got != "https://otlp-gateway-prod-eu-west-2.grafana.net/otlp" {
		t.Errorf("endpoint = %q, want the URL without its trailing slash", got)
	}
}

func TestSettingsStatus(t *testing.T) {
	const otlp = "https://otlp-gateway-prod-eu-west-2.grafana.net/otlp"
	const prom = "https://prometheus-prod-24-prod-eu-west-2.grafana.net/api/prom"
	for _, tc := range []struct {
		name            string
		s               Settings
		export, history string
		on              bool
	}{
		{"all set", Settings{OTLPURL: otlp, InstanceID: "123456", Token: testToken, MetricsURL: prom, MetricsUser: "654321"},
			"telemetry export: on (Grafana Cloud otlp-gateway-prod-eu-west-2.grafana.net)",
			"usage history: on (prometheus-prod-24-prod-eu-west-2.grafana.net)", true},
		{"export only", Settings{OTLPURL: otlp, InstanceID: "123456", Token: testToken},
			"telemetry export: on (Grafana Cloud otlp-gateway-prod-eu-west-2.grafana.net)",
			"usage history: off (no TCGREPLAY_GRAFANA_METRICS_URL or TCGREPLAY_GRAFANA_METRICS_USER)", true},
		{"unset", Settings{},
			"telemetry export: off (no TCGREPLAY_GRAFANA_* settings)",
			"usage history: off (no TCGREPLAY_GRAFANA_METRICS_URL or TCGREPLAY_GRAFANA_METRICS_USER)", false},
		{"token missing", Settings{OTLPURL: otlp, InstanceID: "123456"},
			"warning: telemetry export: off (incomplete: TCGREPLAY_GRAFANA_TOKEN is not set)",
			"usage history: off (no TCGREPLAY_GRAFANA_METRICS_URL or TCGREPLAY_GRAFANA_METRICS_USER)", false},
		{"instance not a number", Settings{OTLPURL: otlp, InstanceID: "stack-1", Token: testToken},
			"warning: telemetry export: off (incomplete: TCGREPLAY_GRAFANA_INSTANCE_ID is not a number)",
			"usage history: off (no TCGREPLAY_GRAFANA_METRICS_URL or TCGREPLAY_GRAFANA_METRICS_USER)", false},
		{"plain http url", Settings{OTLPURL: "http://otlp.example/otlp", InstanceID: "1", Token: testToken},
			"warning: telemetry export: off (incomplete: TCGREPLAY_GRAFANA_OTLP_URL is not an https URL)",
			"usage history: off (no TCGREPLAY_GRAFANA_METRICS_URL or TCGREPLAY_GRAFANA_METRICS_USER)", false},
		{"token with spaces, url missing", Settings{InstanceID: "1", Token: "glc_ab cd"},
			"warning: telemetry export: off (incomplete: TCGREPLAY_GRAFANA_OTLP_URL is not set; TCGREPLAY_GRAFANA_TOKEN contains spaces)",
			"usage history: off (no TCGREPLAY_GRAFANA_METRICS_URL or TCGREPLAY_GRAFANA_METRICS_USER)", false},
		{"only metrics settings", Settings{MetricsURL: prom, MetricsUser: "654321"},
			"warning: telemetry export: off (incomplete: TCGREPLAY_GRAFANA_OTLP_URL, TCGREPLAY_GRAFANA_INSTANCE_ID and TCGREPLAY_GRAFANA_TOKEN are not set)",
			"usage history: off (telemetry export is off)", false},
		{"history user missing", Settings{OTLPURL: otlp, InstanceID: "123456", Token: testToken, MetricsURL: prom},
			"telemetry export: on (Grafana Cloud otlp-gateway-prod-eu-west-2.grafana.net)",
			"warning: usage history: off (TCGREPLAY_GRAFANA_METRICS_USER is not set)", true},
		{"history url not https", Settings{OTLPURL: otlp, InstanceID: "123456", Token: testToken, MetricsURL: "prometheus.example", MetricsUser: "654321"},
			"telemetry export: on (Grafana Cloud otlp-gateway-prod-eu-west-2.grafana.net)",
			"warning: usage history: off (TCGREPLAY_GRAFANA_METRICS_URL is not an https URL)", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			export, history := tc.s.Status()
			if export != tc.export {
				t.Errorf("export status = %q\nwant            %q", export, tc.export)
			}
			if history != tc.history {
				t.Errorf("history status = %q\nwant             %q", history, tc.history)
			}
			if tc.s.Token != "" && (strings.Contains(export, tc.s.Token) || strings.Contains(history, tc.s.Token)) {
				t.Errorf("a status names the token: %q / %q", export, history)
			}
			if got := tc.s.export().on; got != tc.on {
				t.Errorf("export on = %v, want %v", got, tc.on)
			}
		})
	}
}
