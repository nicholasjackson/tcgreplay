package spacestest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// CDNEndpointID is the id of the bucket's endpoint a CDN fake lists.
const CDNEndpointID = "cdn-1"

// CDN is a stand-in for the DigitalOcean API's CDN endpoints: it lists the
// bucket's endpoint (id CDNEndpointID, with the origin it was given) and a
// decoy with another origin, and records every cache purge of the
// bucket's endpoint. A request without the bearer token it was given is
// refused with 401. Point spaces.CDN.API at URL.
type CDN struct {
	*httptest.Server
	token string

	mu     sync.Mutex
	origin string
	fail   bool
	purges [][]string
}

// NewCDN starts a CDN fake for origin accepting token, closed when the
// test ends.
func NewCDN(t testing.TB, origin, token string) *CDN {
	c := &CDN{token: token, origin: origin}
	c.Server = httptest.NewServer(http.HandlerFunc(c.serve))
	t.Cleanup(c.Server.Close)
	return c
}

func (c *CDN) serve(w http.ResponseWriter, r *http.Request) {
	reply := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v)
	}
	if r.Header.Get("Authorization") != "Bearer "+c.token {
		reply(http.StatusUnauthorized, map[string]string{"id": "unauthorized", "message": "Unable to authenticate you."})
		return
	}
	c.mu.Lock()
	fail, origin := c.fail, c.origin
	c.mu.Unlock()
	if fail {
		reply(http.StatusInternalServerError, map[string]string{"id": "server_error", "message": "Server was unable to give you a response."})
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v2/cdn/endpoints":
		reply(http.StatusOK, map[string]any{
			"endpoints": []map[string]string{
				{"id": "cdn-decoy", "origin": "other-bucket.nyc3.digitaloceanspaces.com", "endpoint": "other-bucket.nyc3.cdn.digitaloceanspaces.com"},
				{"id": CDNEndpointID, "origin": origin, "endpoint": strings.Replace(origin, ".digitaloceanspaces.com", ".cdn.digitaloceanspaces.com", 1)},
			},
			"links": map[string]any{},
			"meta":  map[string]int{"total": 2},
		})
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v2/cdn/endpoints/") && strings.HasSuffix(r.URL.Path, "/cache"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/cdn/endpoints/"), "/cache")
		if id != CDNEndpointID {
			reply(http.StatusNotFound, map[string]string{"id": "not_found", "message": "The resource you were accessing could not be found."})
			return
		}
		var body struct {
			Files []string `json:"files"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Files) == 0 {
			reply(http.StatusUnprocessableEntity, map[string]string{"id": "unprocessable_entity", "message": "files is required"})
			return
		}
		c.mu.Lock()
		c.purges = append(c.purges, body.Files)
		c.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		reply(http.StatusNotFound, map[string]string{"id": "not_found", "message": "The resource you were accessing could not be found."})
	}
}

// Purges returns the files of every purge so far, in order.
func (c *CDN) Purges() [][]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]string, len(c.purges))
	for i, p := range c.purges {
		out[i] = append([]string(nil), p...)
	}
	return out
}

// SetFail makes every request with the right token fail with 500.
func (c *CDN) SetFail(fail bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fail = fail
}

// SetOrigin changes the origin the bucket's endpoint lists, so a client
// looking for another origin finds no endpoint.
func (c *CDN) SetOrigin(origin string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.origin = origin
}
