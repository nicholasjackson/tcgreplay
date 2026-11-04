package spaces

import (
	"strings"
	"testing"
)

const (
	testKeyValue    = "DO00SECRETKEYID"
	testSecretValue = "s3cr3t/VALUE+never-logged"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func fullEnv() map[string]string {
	return map[string]string{
		EnvEndpoint: "https://ams3.digitaloceanspaces.com",
		EnvBucket:   "playmat-cache",
		EnvKey:      testKeyValue,
		EnvSecret:   testSecretValue,
	}
}

func TestFromEnvNoneSet(t *testing.T) {
	cfg, set, err := FromEnv(env(nil))
	if set || err != nil {
		t.Fatalf("FromEnv = %+v, %v, %v; want not set, no error", cfg, set, err)
	}
}

func TestFromEnvWhitespaceOnlyIsUnset(t *testing.T) {
	_, set, err := FromEnv(env(map[string]string{EnvEndpoint: "  ", EnvBucket: "\t", EnvKey: "", EnvSecret: " \n"}))
	if set || err != nil {
		t.Fatalf("set=%v err=%v; want not set, no error", set, err)
	}
}

func TestFromEnvComplete(t *testing.T) {
	cfg, set, err := FromEnv(env(fullEnv()))
	if !set || err != nil {
		t.Fatalf("set=%v err=%v; want set, no error", set, err)
	}
	want := Config{
		Endpoint:  "https://ams3.digitaloceanspaces.com",
		Region:    "ams3",
		Bucket:    "playmat-cache",
		AccessKey: testKeyValue,
		SecretKey: testSecretValue,
	}
	if cfg != want {
		t.Errorf("cfg = %+v, want %+v", cfg, want)
	}
	if got := cfg.String(); got != "https://ams3.digitaloceanspaces.com/playmat-cache" {
		t.Errorf("String() = %q", got)
	}
}

func TestFromEnvRegion(t *testing.T) {
	for endpoint, want := range map[string]string{
		"https://ams3.digitaloceanspaces.com":     "ams3",
		"https://nyc3.digitaloceanspaces.com/":    "nyc3",
		"https://fra1.digitaloceanspaces.com:443": "fra1",
		"http://127.0.0.1:9000":                   "us-east-1",
		"https://minio.example.com":               "us-east-1",
	} {
		m := fullEnv()
		m[EnvEndpoint] = "  " + endpoint + " "
		cfg, _, err := FromEnv(env(m))
		if err != nil {
			t.Errorf("%s: %v", endpoint, err)
			continue
		}
		if cfg.Region != want {
			t.Errorf("%s: region %q, want %q", endpoint, cfg.Region, want)
		}
		if strings.HasSuffix(cfg.Endpoint, "/") {
			t.Errorf("%s: endpoint kept its trailing slash: %q", endpoint, cfg.Endpoint)
		}
	}
}

func TestFromEnvPartial(t *testing.T) {
	for _, missing := range []string{EnvEndpoint, EnvBucket, EnvKey, EnvSecret} {
		t.Run(missing, func(t *testing.T) {
			m := fullEnv()
			delete(m, missing)
			_, set, err := FromEnv(env(m))
			if !set || err == nil {
				t.Fatalf("set=%v err=%v; want set and an error", set, err)
			}
			assertNamesWithoutSecrets(t, err, missing)
		})
	}
	t.Run("only the key", func(t *testing.T) {
		_, set, err := FromEnv(env(map[string]string{EnvKey: testKeyValue}))
		if !set || err == nil {
			t.Fatalf("set=%v err=%v; want set and an error", set, err)
		}
		for _, name := range []string{EnvEndpoint, EnvBucket, EnvSecret} {
			assertNamesWithoutSecrets(t, err, name)
		}
	})
}

func TestFromEnvBadEndpoint(t *testing.T) {
	for _, endpoint := range []string{
		"ams3.digitaloceanspaces.com",
		"ftp://x",
		"https://x/path",
		"https://x?q=1",
		"https://x#frag",
		"https://user:pass@x",
		"https://",
		"://bad",
	} {
		t.Run(endpoint, func(t *testing.T) {
			m := fullEnv()
			m[EnvEndpoint] = endpoint
			_, set, err := FromEnv(env(m))
			if !set || err == nil {
				t.Fatalf("set=%v err=%v; want set and an error", set, err)
			}
			assertNamesWithoutSecrets(t, err, EnvEndpoint)
		})
	}
}

// TestFromEnvNeverEchoesValues puts the credentials where they do not
// belong (a mis-set variable, user info in the endpoint) and checks the
// error still names only the setting.
func TestFromEnvNeverEchoesValues(t *testing.T) {
	for name, set := range map[string]func(m map[string]string){
		"secret as endpoint": func(m map[string]string) { m[EnvEndpoint] = testSecretValue },
		"key as endpoint":    func(m map[string]string) { m[EnvEndpoint] = testKeyValue },
		"user info in endpoint": func(m map[string]string) {
			m[EnvEndpoint] = "https://" + testKeyValue + ":" + testSecretValue + "@ams3.digitaloceanspaces.com"
		},
		"secret as bucket": func(m map[string]string) { m[EnvBucket] = testSecretValue },
	} {
		t.Run(name, func(t *testing.T) {
			m := fullEnv()
			set(m)
			_, _, err := FromEnv(env(m))
			if err == nil {
				t.Fatal("FromEnv accepted a credential in the wrong setting")
			}
			if strings.Contains(err.Error(), testKeyValue) || strings.Contains(err.Error(), testSecretValue) {
				t.Errorf("error %q contains a credential", err)
			}
		})
	}
}

func assertNamesWithoutSecrets(t *testing.T, err error, name string) {
	t.Helper()
	msg := err.Error()
	if !strings.Contains(msg, name) {
		t.Errorf("error %q does not name %s", msg, name)
	}
	if strings.Contains(msg, testKeyValue) || strings.Contains(msg, testSecretValue) {
		t.Errorf("error %q contains a credential", msg)
	}
}
