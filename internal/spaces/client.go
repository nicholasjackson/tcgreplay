package spaces

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
	resp, err := c.send(ctx, method, u, body, h)
	if err != nil {
		return nil, fmt.Errorf("spaces: %s %s: %w", method, key, err)
	}
	return resp, nil
}

// send signs and sends one request to u.
func (c *Client) send(ctx context.Context, method string, u *url.URL, body []byte, h http.Header) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return nil, err
	}
	for k, vs := range h {
		req.Header[k] = vs
	}
	sign(req, payloadHash(body), c.cfg.AccessKey, c.cfg.SecretKey, c.cfg.Region, c.now())
	return c.cfg.HTTP.Do(req)
}

// ListPage is one page of keys under a prefix (S3 ListObjectsV2).
type ListPage struct {
	Keys []string // full object keys, in key order
	Next string   // continuation token; "" on the last page
	// ETags holds each listed key's ETag, unquoted. For an object written
	// by a single-part PUT without customer-supplied encryption (every
	// object this package writes) it is the hex MD5 of the object's bytes,
	// so it changes exactly when the bytes do. A key the listing gave no
	// ETag for is absent.
	ETags map[string]string
}

// List returns the page of keys under prefix that follows token ("" for
// the first page). Spaces decides the page size (up to 1,000 keys); the
// caller bounds each page with its context. Any answer but success is an
// *Error.
func (c *Client) List(ctx context.Context, prefix, token string) (ListPage, error) {
	u, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return ListPage{}, fmt.Errorf("spaces: list %s: %w", prefix, err)
	}
	u.Path = "/" + c.cfg.Bucket
	u.RawPath = canonicalURI(u.Path)
	q := url.Values{"list-type": {"2"}}
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	if token != "" {
		q.Set("continuation-token", token)
	}
	u.RawQuery = q.Encode()
	resp, err := c.send(ctx, http.MethodGet, u, nil, nil)
	if err != nil {
		return ListPage{}, fmt.Errorf("spaces: list %s: %w", prefix, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ListPage{}, responseError(resp)
	}
	var x struct {
		Contents []struct {
			Key  string `xml:"Key"`
			ETag string `xml:"ETag"`
		} `xml:"Contents"`
		IsTruncated           bool   `xml:"IsTruncated"`
		NextContinuationToken string `xml:"NextContinuationToken"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, maxObject)).Decode(&x); err != nil {
		return ListPage{}, fmt.Errorf("spaces: list %s: %w", prefix, err)
	}
	page := ListPage{Keys: make([]string, 0, len(x.Contents)), ETags: make(map[string]string, len(x.Contents))}
	for _, o := range x.Contents {
		page.Keys = append(page.Keys, o.Key)
		if tag := strings.Trim(o.ETag, `"`); tag != "" {
			page.ETags[o.Key] = tag
		}
	}
	if x.IsTruncated {
		page.Next = x.NextContinuationToken
	}
	return page, nil
}

// deleteBatch is how many keys one DeleteObjects request removes at most,
// S3's limit.
const deleteBatch = 1000

// DeleteError names the keys Delete left in place, with S3's error code
// for each: the code Spaces refused that key with one by one (for example
// AccessDenied), or, when a whole request failed, the request's code for
// every key it carried and every key after it, which were never sent
// ("RequestFailed" when Spaces could not be reached). Err is that request
// failure (an *Error, or the transport error), nil when Spaces only
// refused keys one by one. The other keys were deleted.
type DeleteError struct {
	Failed map[string]string // key -> S3 error code
	Err    error
}

func (e *DeleteError) Error() string {
	first := ""
	for k := range e.Failed {
		if first == "" || k < first {
			first = k
		}
	}
	msg := fmt.Sprintf("spaces: %d keys not deleted (first: %s %s)", len(e.Failed), first, e.Failed[first])
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

// Unwrap returns the request failure, so errors.As finds its *Error.
func (e *DeleteError) Unwrap() error { return e.Err }

// Delete removes keys, deleteBatch keys per S3 DeleteObjects request. A
// key with no object is not an error. Any failure is a *DeleteError naming
// every key not deleted: keys Spaces refuses one by one are collected
// across every request; a request that fails as a whole (an *Error, or a
// transport error, in Err) stops the call, and its keys and every later
// key are named with its code. The caller bounds the whole call with its
// context.
func (c *Client) Delete(ctx context.Context, keys []string) error {
	failed := map[string]string{}
	for len(keys) > 0 {
		n := min(len(keys), deleteBatch)
		f, err := c.deleteBatch(ctx, keys[:n])
		if err != nil {
			code := "RequestFailed"
			var e *Error
			if errors.As(err, &e) {
				code = e.Code
			}
			for _, k := range keys {
				failed[k] = code
			}
			return &DeleteError{Failed: failed, Err: err}
		}
		for k, code := range f {
			failed[k] = code
		}
		keys = keys[n:]
	}
	if len(failed) > 0 {
		return &DeleteError{Failed: failed}
	}
	return nil
}

// deleteBatch sends one quiet DeleteObjects request (POST
// /<bucket>?delete) for keys, and returns the keys it refused.
func (c *Client) deleteBatch(ctx context.Context, keys []string) (map[string]string, error) {
	type object struct {
		Key string `xml:"Key"`
	}
	req := struct {
		XMLName xml.Name `xml:"Delete"`
		Quiet   bool     `xml:"Quiet"`
		Objects []object `xml:"Object"`
	}{Quiet: true}
	for _, k := range keys {
		req.Objects = append(req.Objects, object{Key: k})
	}
	body, err := xml.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("spaces: delete %d keys: %w", len(keys), err)
	}
	u, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("spaces: delete %d keys: %w", len(keys), err)
	}
	u.Path = "/" + c.cfg.Bucket
	u.RawPath = canonicalURI(u.Path)
	u.RawQuery = "delete="
	sum := md5.Sum(body)
	h := http.Header{}
	h.Set("Content-Type", "application/xml")
	h.Set("Content-Md5", base64.StdEncoding.EncodeToString(sum[:]))
	resp, err := c.send(ctx, http.MethodPost, u, body, h)
	if err != nil {
		return nil, fmt.Errorf("spaces: delete %d keys: %w", len(keys), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp)
	}
	var x struct {
		Errors []struct {
			Key  string `xml:"Key"`
			Code string `xml:"Code"`
		} `xml:"Error"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, maxObject)).Decode(&x); err != nil && err != io.EOF {
		return nil, fmt.Errorf("spaces: delete %d keys: %w", len(keys), err)
	}
	var failed map[string]string
	for _, e := range x.Errors {
		if failed == nil {
			failed = map[string]string{}
		}
		failed[e.Key] = e.Code
	}
	return failed, nil
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
