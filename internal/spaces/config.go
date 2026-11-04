package spaces

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// The environment variables FromEnv reads.
const (
	EnvEndpoint = "TCGREPLAY_SPACES_ENDPOINT"
	EnvBucket   = "TCGREPLAY_SPACES_BUCKET"
	EnvKey      = "TCGREPLAY_SPACES_KEY"
	EnvSecret   = "TCGREPLAY_SPACES_SECRET"
)

// Config is where a Client finds its bucket and how it signs requests.
type Config struct {
	// Endpoint is the Spaces endpoint URL, for example
	// https://ams3.digitaloceanspaces.com.
	Endpoint string
	// Region is the signing region. When empty it is derived from
	// Endpoint: the first host label of a *.digitaloceanspaces.com
	// endpoint, otherwise us-east-1.
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	// HTTP is the client requests go through. Optional.
	HTTP *http.Client
}

// String names the bucket for logs: <endpoint>/<bucket>. It never includes
// the credentials.
func (c Config) String() string {
	return strings.TrimSuffix(c.Endpoint, "/") + "/" + c.Bucket
}

// PublicBase is where the bucket's public-read objects are served, with
// no trailing slash: https://<bucket>.<region>.cdn.digitaloceanspaces.com,
// the bucket's CDN, for an endpoint https://<region>.digitaloceanspaces.com,
// and otherwise <endpoint>/<bucket> (path style). The CDN serves an object
// only once it is enabled on the bucket.
func (c Config) PublicBase() string {
	endpoint := strings.TrimRight(c.Endpoint, "/")
	if u, err := url.Parse(endpoint); err == nil && u.Port() == "" {
		if region, ok := strings.CutSuffix(u.Hostname(), ".digitaloceanspaces.com"); ok && region != "" && !strings.Contains(region, ".") {
			return "https://" + c.Bucket + "." + region + ".cdn.digitaloceanspaces.com"
		}
	}
	return endpoint + "/" + c.Bucket
}

// FromEnv reads the four TCGREPLAY_SPACES_* settings through getenv.
// Values are trimmed and an empty value counts as unset. With none set it
// returns set=false and no error. With some but not all set, or an invalid
// endpoint, it returns set=true and an error naming what is missing or
// invalid; the error names settings only and never echoes a value, so it
// never contains the key or the secret.
func FromEnv(getenv func(string) string) (cfg Config, set bool, err error) {
	vals := map[string]string{}
	var missing []error
	for _, name := range []string{EnvEndpoint, EnvBucket, EnvKey, EnvSecret} {
		v := strings.TrimSpace(getenv(name))
		if v == "" {
			missing = append(missing, fmt.Errorf("%s is not set", name))
			continue
		}
		vals[name] = v
	}
	if len(vals) == 0 {
		return Config{}, false, nil
	}
	if len(missing) > 0 {
		return Config{}, true, errors.Join(missing...)
	}
	cfg = Config{
		Endpoint:  vals[EnvEndpoint],
		Bucket:    vals[EnvBucket],
		AccessKey: vals[EnvKey],
		SecretKey: vals[EnvSecret],
	}
	cfg, err = cfg.normalized()
	return cfg, true, err
}

// normalized checks c and returns it with the endpoint's trailing slash
// dropped and the region filled in.
func (c Config) normalized() (Config, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		// The value is not echoed: a mis-set variable could hold a
		// credential, and an endpoint can carry user info.
		return Config{}, fmt.Errorf("%s is not an http or https URL with a host and no path", EnvEndpoint)
	}
	switch {
	case c.Bucket == "":
		return Config{}, fmt.Errorf("%s is not set", EnvBucket)
	case strings.Contains(c.Bucket, "/"):
		return Config{}, fmt.Errorf("%s contains a slash", EnvBucket)
	case c.AccessKey == "":
		return Config{}, fmt.Errorf("%s is not set", EnvKey)
	case c.SecretKey == "":
		return Config{}, fmt.Errorf("%s is not set", EnvSecret)
	}
	c.Endpoint = u.Scheme + "://" + u.Host
	if c.Region == "" {
		c.Region = "us-east-1"
		host := u.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if label, _, ok := strings.Cut(host, "."); ok && strings.HasSuffix(host, ".digitaloceanspaces.com") {
			c.Region = label
		}
	}
	return c, nil
}
