package spaces

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// maxObject bounds how much of an object Get reads.
const maxObject = 32 << 20

// ErrNotFound means the bucket holds no object under the key.
var ErrNotFound = errors.New("spaces: no such key")

// PutOptions are the headers a Put writes with the object.
type PutOptions struct {
	ContentType  string
	CacheControl string
	// Public makes the object readable by anyone (x-amz-acl: public-read).
	// Objects are private otherwise.
	Public bool
}

// Client reads and writes whole objects in one Spaces bucket.
type Client struct {
	cfg Config
	now func() time.Time
}

// New returns a client for cfg, which must name an http or https
// endpoint, a bucket and both credentials. Without an HTTP client in cfg,
// one with a 30 second outer bound is used; callers bound each call with
// its context.
func New(cfg Config) (*Client, error) {
	n, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	if n.HTTP == nil {
		n.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{cfg: n, now: time.Now}, nil
}

// Bucket is the bucket the client reads and writes.
func (c *Client) Bucket() string { return c.cfg.Bucket }

// Endpoint is the endpoint URL the client talks to.
func (c *Client) Endpoint() string { return c.cfg.Endpoint }

// objectURL is the path-style URL of key: <endpoint>/<bucket>/<key>, with
// the path encoded on the wire exactly as it is signed.
func (c *Client) objectURL(key string) (*url.URL, error) {
	u, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	u.Path = "/" + c.cfg.Bucket + "/" + key
	u.RawPath = canonicalURI(u.Path)
	return u, nil
}

// Get returns the object under key, or ErrNotFound when there is none. Any
// other answer from Spaces is an *Error.
func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	resp, err := c.do(ctx, http.MethodGet, key, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	default:
		return nil, responseError(resp)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxObject+1))
	if err != nil {
		return nil, fmt.Errorf("spaces: get %s: %w", key, err)
	}
	if len(b) > maxObject {
		return nil, fmt.Errorf("spaces: get %s: object is larger than %d bytes", key, maxObject)
	}
	return b, nil
}

// Put writes body under key, replacing any object there. Any answer from
// Spaces but success is an *Error.
func (c *Client) Put(ctx context.Context, key string, body []byte, o PutOptions) error {
	h := http.Header{}
	if o.ContentType != "" {
		h.Set("Content-Type", o.ContentType)
	}
	if o.CacheControl != "" {
		h.Set("Cache-Control", o.CacheControl)
	}
	if o.Public {
		h.Set("X-Amz-Acl", "public-read")
	}
	resp, err := c.do(ctx, http.MethodPut, key, body, h)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return responseError(resp)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

// do sends one signed request for key.
func (c *Client) do(ctx context.Context, method, key string, body []byte, h http.Header) (*http.Response, error) {
	u, err := c.objectURL(key)
	if err != nil {
		return nil, fmt.Errorf("spaces: %s %s: %w", method, key, err)
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return nil, fmt.Errorf("spaces: %s %s: %w", method, key, err)
	}
	for k, vs := range h {
		req.Header[k] = vs
	}
	sign(req, payloadHash(body), c.cfg.AccessKey, c.cfg.SecretKey, c.cfg.Region, c.now())
	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spaces: %s %s: %w", method, key, err)
	}
	return resp, nil
}

// responseError reads an S3 error response into an *Error. A body that is
// not S3's XML leaves the code as the status text.
func responseError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var x struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	e := &Error{Status: resp.StatusCode}
	if xml.Unmarshal(b, &x) == nil && x.Code != "" {
		e.Code, e.Message = x.Code, x.Message
	} else {
		e.Code, e.Message = http.StatusText(resp.StatusCode), resp.Status
	}
	return e
}
