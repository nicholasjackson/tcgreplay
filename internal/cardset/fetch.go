package cardset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// ErrNotFound means an upstream source has no such set or card.
var ErrNotFound = errors.New("not found")

// UserAgent is sent with every upstream request.
var UserAgent = "tcgreplay-sets"

// maxBody bounds an upstream response body. The largest malie set file is
// well under a megabyte.
const maxBody = 32 << 20

// httpGetter fetches JSON from an upstream source with no caching: each
// request has its own timeout, and network errors, 429s, 5xx responses and
// bodies that do not decode are retried with a growing pause.
type httpGetter struct {
	Client  *http.Client  // nil means http.DefaultClient
	Timeout time.Duration // per attempt; 0 means 20s
	// Backoff is the pause before each retry; its length is the number of
	// retries. nil means 500ms then 2s (three attempts in all).
	Backoff []time.Duration
}

var defaultBackoff = []time.Duration{500 * time.Millisecond, 2 * time.Second}

// getJSON fetches url and decodes its body into out. A 404 is ErrNotFound
// and is not retried. Errors name the URL.
func (g httpGetter) getJSON(ctx context.Context, url string, out any) error {
	backoff := g.Backoff
	if backoff == nil {
		backoff = defaultBackoff
	}
	var err error
	for attempt := 0; ; attempt++ {
		var retry bool
		retry, err = g.once(ctx, url, out)
		if err == nil || !retry || attempt >= len(backoff) {
			break
		}
		select {
		case <-time.After(backoff[attempt]):
		case <-ctx.Done():
			return fmt.Errorf("GET %s: %w", url, ctx.Err())
		}
	}
	if err != nil && !errors.Is(err, ErrNotFound) && len(backoff) > 0 {
		return fmt.Errorf("GET %s: %w (after %d attempts)", url, err, len(backoff)+1)
	}
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	return nil
}

// once makes one attempt, reporting whether a failure is worth retrying.
func (g httpGetter) once(ctx context.Context, url string, out any) (retry bool, err error) {
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	timeout := g.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")
	client := g.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return false, ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return true, fmt.Errorf("status %s", resp.Status)
	case resp.StatusCode != http.StatusOK:
		return false, fmt.Errorf("status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return true, err
	}
	if len(body) > maxBody {
		return false, fmt.Errorf("body larger than %d bytes", maxBody)
	}
	// An empty or cut-off body is a passing upstream fault: retry it.
	if err := json.Unmarshal(body, out); err != nil {
		return true, fmt.Errorf("decoding body: %w", err)
	}
	return false, nil
}

// flexInt decodes a JSON number, a numeric string or null.
type flexInt int

func (n *flexInt) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var v json.Number
	if err := json.Unmarshal(b, &v); err == nil {
		i, err := strconv.Atoi(v.String())
		if err != nil {
			return err
		}
		*n = flexInt(i)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		return nil
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*n = flexInt(i)
	return nil
}
