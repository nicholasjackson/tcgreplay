package appspec

import (
	"os"
	"strings"
	"testing"
)

// realSpec is the repository's App Platform definition.
func realSpec(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../.do/app.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestPublicURLFromRealSpec: the public address in the real definition is
// https://tcgreplay.app.
func TestPublicURLFromRealSpec(t *testing.T) {
	u, err := PublicURL(realSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	if got := u.String(); got != "https://tcgreplay.app" {
		t.Errorf("PublicURL = %q, want https://tcgreplay.app", got)
	}
}

// TestEnvFields: an env entry's fields are read as written, a secret has
// no value, and an undeclared key gives nil.
func TestEnvFields(t *testing.T) {
	spec := realSpec(t)
	if f := EnvFields(spec, "TCGREPLAY_PUBLIC_URL"); f["value"] != "https://tcgreplay.app" || f["scope"] != "RUN_TIME" {
		t.Errorf("TCGREPLAY_PUBLIC_URL fields %v", f)
	}
	if f := EnvFields(spec, "TCGREPLAY_GOOGLE_CLIENT_SECRET"); f["type"] != "SECRET" || f["value"] != "" {
		t.Errorf("TCGREPLAY_GOOGLE_CLIENT_SECRET fields %v", f)
	}
	if f := EnvFields(spec, "TCGREPLAY_NO_SUCH_KEY"); f != nil {
		t.Errorf("undeclared key gave %v", f)
	}
	quoted := "envs:\n  - key: A\n    value: \"60m\"\n  - key: B\n    value: x\n"
	if f := EnvFields(quoted, "A"); f["value"] != `"60m"` {
		t.Errorf("A value %q, want it as written", f["value"])
	}
}

// TestPublicURLRefusesNonOrigins: anything but a bare https origin is
// refused, naming the setting; a quoted origin is accepted.
func TestPublicURLRefusesNonOrigins(t *testing.T) {
	spec := realSpec(t)
	for _, tt := range []struct{ name, value string }{
		{"http", "http://tcgreplay.app"},
		{"path", "https://tcgreplay.app/play"},
		{"query", "https://tcgreplay.app?x=1"},
		{"credentials", "https://me@tcgreplay.app"},
		{"no host", "https://"},
		{"empty", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			broken := strings.Replace(spec, "value: https://tcgreplay.app", "value: "+tt.value, 1)
			if _, err := PublicURL(broken); err == nil || !strings.Contains(err.Error(), "TCGREPLAY_PUBLIC_URL") {
				t.Errorf("PublicURL with %q: %v, want an error naming TCGREPLAY_PUBLIC_URL", tt.value, err)
			}
		})
	}
	quoted := strings.Replace(spec, "value: https://tcgreplay.app", `value: "https://tcgreplay.app/"`, 1)
	if u, err := PublicURL(quoted); err != nil || u.String() != "https://tcgreplay.app" {
		t.Errorf("quoted origin: %v %v", u, err)
	}
}
