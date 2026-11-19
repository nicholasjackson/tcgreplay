// Package appspec reads settings from the DigitalOcean App Platform
// definition (.do/app.yaml) without a YAML parser: the file's env entries
// are a flat list, and a line scan is enough for the few values the
// project reads from it (the public address, and the tests' checks of
// which settings are secrets).
package appspec

import (
	"fmt"
	"net/url"
	"strings"
)

// EnvFields returns the fields of the env entry for key in the App
// Platform definition text (value, scope, type, ...), or nil if it
// declares none. Values are returned as written, quotes included.
func EnvFields(text, key string) map[string]string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "- key: "+key {
			continue
		}
		out := map[string]string{}
		indent := len(l) - len(strings.TrimLeft(l, " ")) + 2
		for _, next := range lines[i+1:] {
			trimmed := strings.TrimSpace(next)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			if len(next)-len(strings.TrimLeft(next, " ")) != indent || strings.HasPrefix(trimmed, "-") {
				break
			}
			k, v, _ := strings.Cut(trimmed, ":")
			out[k] = strings.TrimSpace(v)
		}
		return out
	}
	return nil
}

// unquote strips one pair of matching YAML quotes.
func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// PublicURL returns TCGREPLAY_PUBLIC_URL from the App Platform definition
// text: the one public address of the hosted playmat. It must be a bare
// https origin (https://host, no path, query or credentials).
func PublicURL(text string) (*url.URL, error) {
	f := EnvFields(text, "TCGREPLAY_PUBLIC_URL")
	v := unquote(f["value"])
	if v == "" {
		return nil, fmt.Errorf("app spec: no TCGREPLAY_PUBLIC_URL value")
	}
	u, err := url.Parse(v)
	if err != nil {
		return nil, fmt.Errorf("app spec: TCGREPLAY_PUBLIC_URL %q: %v", v, err)
	}
	if u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("app spec: TCGREPLAY_PUBLIC_URL %q must be a bare https origin such as https://example.com", v)
	}
	u.Path = ""
	return u, nil
}
