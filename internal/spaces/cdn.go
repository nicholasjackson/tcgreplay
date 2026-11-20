package spaces

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNoCDN means the DigitalOcean account has no CDN endpoint whose origin
// is the bucket, so there is no CDN cache to purge.
var ErrNoCDN = errors.New("spaces: no CDN endpoint for this bucket")

// purgeBatch is how many paths one purge request carries at most.
const purgeBatch = 50

// CDN purges cached paths from the bucket's DigitalOcean CDN through the
// DigitalOcean API (v2, CDN endpoints). It finds the bucket's endpoint by
// its origin, so no endpoint id is configured. It knows nothing about what
// the paths hold.
type CDN struct {
	// Token is a DigitalOcean API token with the cdn:read scope (to list
	// endpoints) and cdn:delete (to purge a cache; not cdn:update).
	// It is sent only as a bearer token and never appears in an error.
	Token string
	// Origin is the bucket's origin host, <bucket>.<region>.digitaloceanspaces.com.
	Origin string
	// API is the DigitalOcean API base address; tests point it at a fake.
	API string
	// HTTP is the client requests go through.
	HTTP *http.Client
}

// CDNError is a failure the DigitalOcean API reported: the HTTP status
// and DigitalOcean's error id and message, when the response carried them.
type CDNError struct {
	Status  int
	ID      string
	Message string
}

func (e *CDNError) Error() string {
	return fmt.Sprintf("spaces: CDN API %d %s: %s", e.Status, e.ID, e.Message)
}

// NewCDN returns a purge client for cfg's bucket using token. The origin
// is the bucket name followed by the endpoint's host, which for an
// endpoint https://<region>.digitaloceanspaces.com is
// <bucket>.<region>.digitaloceanspaces.com.
func NewCDN(cfg Config, token string) *CDN {
	host := strings.TrimSuffix(cfg.Endpoint, "/")
	if u, err := url.Parse(host); err == nil && u.Host != "" {
		host = u.Host
	}
	return &CDN{
		Token:  token,
		Origin: cfg.Bucket + "." + host,
		API:    "https://api.digitalocean.com",
		HTTP:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Purge drops paths (object keys with no leading slash; "*" is a wildcard,
// so "images/*" covers every image) from the bucket's CDN cache, at most
// purgeBatch paths a request. It returns ErrNoCDN, purging nothing, when
// no CDN endpoint has the bucket as its origin.
func (c *CDN) Purge(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	id, err := c.endpointID(ctx)
	if err != nil {
		return err
	}
	for len(paths) > 0 {
		n := min(len(paths), purgeBatch)
		body, err := json.Marshal(map[string][]string{"files": paths[:n]})
		if err != nil {
			return fmt.Errorf("spaces: CDN purge: %w", err)
		}
		resp, err := c.do(ctx, http.MethodDelete, "/v2/cdn/endpoints/"+url.PathEscape(id)+"/cache", body)
		if err != nil {
			return fmt.Errorf("spaces: CDN purge: %w", err)
		}
		resp.Body.Close()
		paths = paths[n:]
	}
	return nil
}

// endpointID pages through the account's CDN endpoints for the one whose
// origin is c.Origin.
func (c *CDN) endpointID(ctx context.Context) (string, error) {
	for page := 1; ; page++ {
		resp, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/v2/cdn/endpoints?per_page=200&page=%d", page), nil)
		if err != nil {
			return "", fmt.Errorf("spaces: CDN endpoints: %w", err)
		}
		var x struct {
			Endpoints []struct {
				ID     string `json:"id"`
				Origin string `json:"origin"`
			} `json:"endpoints"`
			Links struct {
				Pages struct {
					Next string `json:"next"`
				} `json:"pages"`
			} `json:"links"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&x)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("spaces: CDN endpoints: %w", err)
		}
		for _, e := range x.Endpoints {
			if strings.EqualFold(e.Origin, c.Origin) {
				return e.ID, nil
			}
		}
		if x.Links.Pages.Next == "" || len(x.Endpoints) == 0 {
			return "", ErrNoCDN
		}
	}
}

// do sends one API request with the bearer token, returning the response
// on a 2xx status and a *CDNError otherwise.
func (c *CDN) do(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.API, "/")+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, c.scrub(err)
	}
	if resp.StatusCode/100 == 2 {
		return resp, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var x struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	e := &CDNError{Status: resp.StatusCode}
	if json.Unmarshal(b, &x) == nil && x.ID != "" {
		e.ID, e.Message = x.ID, c.scrubText(x.Message)
	} else {
		e.ID, e.Message = http.StatusText(resp.StatusCode), resp.Status
	}
	return nil, e
}

// scrub keeps the token out of a transport error's text.
func (c *CDN) scrub(err error) error {
	if c.Token == "" || !strings.Contains(err.Error(), c.Token) {
		return err
	}
	return errors.New(c.scrubText(err.Error()))
}

func (c *CDN) scrubText(s string) string {
	if c.Token == "" {
		return s
	}
	return strings.ReplaceAll(s, c.Token, "[token]")
}
