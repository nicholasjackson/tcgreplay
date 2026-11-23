// Package observe sends the hosted playmat's records, log lines, request
// traces and usage counters to the maintainer's Grafana Cloud stack over
// OTLP/HTTP. It is the only package that imports the OpenTelemetry SDK,
// and only hosted mode builds it: the desktop, daemon, shim and in-process
// modes never call Start, so they hold no exporter and make no connection.
//
// Everything here is optional and never in the player's way. With the
// TCGREPLAY_GRAFANA_* settings unset or incomplete, Start hands back no-op
// providers and an honest status. With them set, data goes to bounded
// batch queues that drop rather than block, exporters give up after five
// seconds without retrying, and a health watch writes one plain line to
// standard error when sending has failed for more than five minutes. The
// token is read only from the environment and never formatted anywhere.
package observe

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The hosted settings this package reads. All are optional.
const (
	envOTLPURL     = "TCGREPLAY_GRAFANA_OTLP_URL"
	envInstanceID  = "TCGREPLAY_GRAFANA_INSTANCE_ID"
	envToken       = "TCGREPLAY_GRAFANA_TOKEN"
	envMetricsURL  = "TCGREPLAY_GRAFANA_METRICS_URL"
	envMetricsUser = "TCGREPLAY_GRAFANA_METRICS_USER"
)

// Settings configure Start. The first five come from the environment
// (SettingsFromEnv); the rest are filled in by hosted wiring.
type Settings struct {
	// OTLPURL is the stack's OTLP endpoint as its OpenTelemetry tile shows
	// it (https://otlp-gateway-….grafana.net/otlp); the signal paths
	// /v1/traces, /v1/metrics and /v1/logs are appended to it.
	OTLPURL string
	// InstanceID is the stack's instance ID, the Basic-auth user for OTLP.
	InstanceID string
	// Token is the access policy token. It is a secret: nothing in this
	// package ever formats it.
	Token string
	// MetricsURL and MetricsUser are the stack's Prometheus query endpoint
	// and its metrics instance ID, for reading back the usage history.
	MetricsURL, MetricsUser string

	// Version is the playmat's version, sent as service.version.
	Version string
	// Now is the clock the health watch reads; nil means time.Now.
	Now func() time.Time
	// Stderr receives the health watch's failure line; nil means discard.
	// Production passes os.Stderr, never the std log package's writer, so
	// the line is shown live but never exported.
	Stderr io.Writer
	// HTTPClient, when set, supplies the transport the exporters send
	// through (tests pass a client that trusts their TLS fake receiver).
	// Production leaves it nil.
	HTTPClient *http.Client
}

// SettingsFromEnv reads the TCGREPLAY_GRAFANA_* settings through getenv
// (os.Getenv in production), trimming spaces.
func SettingsFromEnv(getenv func(string) string) Settings {
	get := func(k string) string { return strings.TrimSpace(getenv(k)) }
	return Settings{
		OTLPURL:     get(envOTLPURL),
		InstanceID:  get(envInstanceID),
		Token:       get(envToken),
		MetricsURL:  get(envMetricsURL),
		MetricsUser: get(envMetricsUser),
	}
}

// exportState is what the settings decide about export.
type exportState struct {
	on       bool
	endpoint string // the OTLP base URL, without a trailing slash
	host     string
	problems []string // why export is off when some settings are set
}

// historyState is what the settings decide about the usage history.
type historyState struct {
	on       bool
	endpoint string // the Prometheus query base URL, without a trailing slash
	host     string
	why      string // why it is off
	warn     bool   // off because a setting is set but wrong
}

// httpsBase reports whether raw is an https URL with a host, and returns
// it without a trailing slash, query or fragment.
func httpsBase(raw string) (base, host string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", false
	}
	return strings.TrimRight(u.String(), "/"), u.Host, true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (s Settings) export() exportState {
	var st exportState
	if s.OTLPURL == "" && s.InstanceID == "" && s.Token == "" {
		if s.MetricsURL != "" || s.MetricsUser != "" {
			st.problems = append(st.problems, envOTLPURL+", "+envInstanceID+" and "+envToken+" are not set")
		}
		return st
	}
	switch base, host, ok := httpsBase(s.OTLPURL); {
	case s.OTLPURL == "":
		st.problems = append(st.problems, envOTLPURL+" is not set")
	case !ok:
		st.problems = append(st.problems, envOTLPURL+" is not an https URL")
	default:
		st.endpoint, st.host = base, host
	}
	switch {
	case s.InstanceID == "":
		st.problems = append(st.problems, envInstanceID+" is not set")
	case !allDigits(s.InstanceID):
		st.problems = append(st.problems, envInstanceID+" is not a number")
	}
	switch {
	case s.Token == "":
		st.problems = append(st.problems, envToken+" is not set")
	case strings.ContainsAny(s.Token, " \t\r\n"):
		st.problems = append(st.problems, envToken+" contains spaces")
	}
	st.on = len(st.problems) == 0
	return st
}

func (s Settings) history(export exportState) historyState {
	var st historyState
	if s.MetricsURL == "" && s.MetricsUser == "" {
		st.why = "no " + envMetricsURL + " or " + envMetricsUser
		return st
	}
	var problems []string
	switch base, host, ok := httpsBase(s.MetricsURL); {
	case s.MetricsURL == "":
		problems = append(problems, envMetricsURL+" is not set")
	case !ok:
		problems = append(problems, envMetricsURL+" is not an https URL")
	default:
		st.endpoint, st.host = base, host
	}
	switch {
	case s.MetricsUser == "":
		problems = append(problems, envMetricsUser+" is not set")
	case !allDigits(s.MetricsUser):
		problems = append(problems, envMetricsUser+" is not a number")
	}
	switch {
	case len(problems) > 0:
		st.why, st.warn = strings.Join(problems, "; "), true
	case !export.on:
		st.why = "telemetry export is off"
	default:
		st.on = true
	}
	return st
}

// Status returns the start-up text for export and for the usage history,
// such as "telemetry export: on (Grafana Cloud otlp-gateway-….grafana.net)"
// or "warning: telemetry export: off (TCGREPLAY_GRAFANA_INSTANCE_ID is not
// a number)". Settings that are set but wrong give a "warning:" text, never
// an error: telemetry is optional and must never stop play. Neither text
// ever contains the token.
func (s Settings) Status() (export, history string) {
	ex := s.export()
	switch {
	case ex.on:
		export = "telemetry export: on (Grafana Cloud " + ex.host + ")"
	case len(ex.problems) > 0:
		export = "warning: telemetry export: off (incomplete: " + strings.Join(ex.problems, "; ") + ")"
	default:
		export = "telemetry export: off (no TCGREPLAY_GRAFANA_* settings)"
	}
	h := s.history(ex)
	switch {
	case h.on:
		history = "usage history: on (" + h.host + ")"
	case h.warn:
		history = "warning: usage history: off (" + h.why + ")"
	default:
		history = "usage history: off (" + h.why + ")"
	}
	return export, history
}
