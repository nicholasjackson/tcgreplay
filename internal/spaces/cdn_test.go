package spaces_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

const (
	testOrigin = "test-bucket.lon1.digitaloceanspaces.com"
	testToken  = "dop_v1_secrettoken"
)

// cdnFor returns a purge client for the fake, with token.
func cdnFor(fake *spacestest.CDN, token string) *spaces.CDN {
	c := spaces.NewCDN(spaces.Config{Endpoint: "https://lon1.digitaloceanspaces.com", Bucket: "test-bucket"}, token)
	c.API = fake.URL
	return c
}

func TestNewCDNDerivesTheBucketOrigin(t *testing.T) {
	c := spaces.NewCDN(spaces.Config{Endpoint: "https://lon1.digitaloceanspaces.com/", Bucket: "b"}, "tok")
	if c.Origin != "b.lon1.digitaloceanspaces.com" {
		t.Errorf("origin = %q, want b.lon1.digitaloceanspaces.com", c.Origin)
	}
	if c.API != "https://api.digitalocean.com" || c.Token != "tok" || c.HTTP == nil {
		t.Errorf("client = %+v", c)
	}
	c = spaces.NewCDN(spaces.Config{Endpoint: "http://127.0.0.1:9000", Bucket: "b"}, "tok")
	if c.Origin != "b.127.0.0.1:9000" {
		t.Errorf("non-DigitalOcean origin = %q, want b.127.0.0.1:9000", c.Origin)
	}
}

func TestPurgeSendsExactlyThePathsToTheBucketsEndpoint(t *testing.T) {
	fake := spacestest.NewCDN(t, testOrigin, testToken)
	paths := []string{"images/en/me/me01/*", "images/en/sv/sv1/1/low.webp"}
	if err := cdnFor(fake, testToken).Purge(context.Background(), paths); err != nil {
		t.Fatal(err)
	}
	if got := fake.Purges(); !reflect.DeepEqual(got, [][]string{paths}) {
		t.Errorf("purges = %v, want %v", got, [][]string{paths})
	}
	if err := cdnFor(fake, testToken).Purge(context.Background(), nil); err != nil {
		t.Errorf("purge of nothing: %v", err)
	}
	if n := len(fake.Purges()); n != 1 {
		t.Errorf("purges = %d, want 1 (none for no paths)", n)
	}
}

func TestPurgeOriginMatchIgnoresCase(t *testing.T) {
	fake := spacestest.NewCDN(t, strings.ToUpper(testOrigin), testToken)
	if err := cdnFor(fake, testToken).Purge(context.Background(), []string{"images/*"}); err != nil {
		t.Fatal(err)
	}
	if len(fake.Purges()) != 1 {
		t.Errorf("purges = %v", fake.Purges())
	}
}

func TestPurgeLongListsGoInBatches(t *testing.T) {
	fake := spacestest.NewCDN(t, testOrigin, testToken)
	var paths []string
	for i := range 120 {
		paths = append(paths, fmt.Sprintf("images/en/sv/sv1/%d/*", i))
	}
	if err := cdnFor(fake, testToken).Purge(context.Background(), paths); err != nil {
		t.Fatal(err)
	}
	got := fake.Purges()
	if len(got) != 3 || len(got[0]) != 50 || len(got[2]) != 20 {
		t.Fatalf("purge batches = %d (%v)", len(got), got)
	}
	var all []string
	for _, p := range got {
		all = append(all, p...)
	}
	if !reflect.DeepEqual(all, paths) {
		t.Errorf("purged paths differ from those asked for")
	}
}

func TestPurgeWithoutAnEndpointForTheBucket(t *testing.T) {
	fake := spacestest.NewCDN(t, testOrigin, testToken)
	fake.SetOrigin("other.lon1.digitaloceanspaces.com")
	err := cdnFor(fake, testToken).Purge(context.Background(), []string{"images/*"})
	if !errors.Is(err, spaces.ErrNoCDN) {
		t.Errorf("err = %v, want ErrNoCDN", err)
	}
	if len(fake.Purges()) != 0 {
		t.Errorf("purges = %v, want none", fake.Purges())
	}
}

func TestPurgeFollowsEndpointPages(t *testing.T) {
	var purged []string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("page") == "1":
			fmt.Fprintf(w, `{"endpoints":[{"id":"x","origin":"x.lon1.digitaloceanspaces.com"}],"links":{"pages":{"next":"%s/v2/cdn/endpoints?page=2"}}}`, "http://"+r.Host)
		case r.Method == http.MethodGet:
			fmt.Fprintf(w, `{"endpoints":[{"id":"mine","origin":"%s"}],"links":{}}`, testOrigin)
		case r.Method == http.MethodDelete:
			purged = append(purged, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer api.Close()
	c := spaces.NewCDN(spaces.Config{Endpoint: "https://lon1.digitaloceanspaces.com", Bucket: "test-bucket"}, testToken)
	c.API = api.URL
	if err := c.Purge(context.Background(), []string{"images/*"}); err != nil {
		t.Fatal(err)
	}
	if len(purged) != 1 || purged[0] != "/v2/cdn/endpoints/mine/cache" {
		t.Errorf("purged = %v, want the second page's endpoint", purged)
	}
}

func TestPurgeFailuresAreReportedWithoutTheToken(t *testing.T) {
	fake := spacestest.NewCDN(t, testOrigin, testToken)

	err := cdnFor(fake, "dop_v1_wrongtoken").Purge(context.Background(), []string{"images/*"})
	var ce *spaces.CDNError
	if !errors.As(err, &ce) || ce.Status != http.StatusUnauthorized || ce.ID != "unauthorized" {
		t.Errorf("wrong token: err = %v, want a 401 *CDNError", err)
	}
	if err != nil && strings.Contains(err.Error(), "wrongtoken") {
		t.Errorf("error %q contains the token", err)
	}

	fake.SetFail(true)
	err = cdnFor(fake, testToken).Purge(context.Background(), []string{"images/*"})
	if !errors.As(err, &ce) || ce.Status != http.StatusInternalServerError {
		t.Errorf("failing API: err = %v, want a 500 *CDNError", err)
	}
	if err != nil && strings.Contains(err.Error(), testToken) {
		t.Errorf("error %q contains the token", err)
	}
	fake.SetFail(false)

	fake.Close()
	err = cdnFor(fake, testToken).Purge(context.Background(), []string{"images/*"})
	if err == nil || errors.As(err, &ce) || strings.Contains(err.Error(), testToken) {
		t.Errorf("unreachable API: err = %v, want a transport error without the token", err)
	}
	if len(fake.Purges()) != 0 {
		t.Errorf("purges = %v, want none", fake.Purges())
	}
}

func TestCDNFakeRefusesRequestsWithoutTheToken(t *testing.T) {
	fake := spacestest.NewCDN(t, testOrigin, testToken)
	for _, req := range []*http.Request{
		httptestRequest(t, http.MethodGet, fake.URL+"/v2/cdn/endpoints", ""),
		httptestRequest(t, http.MethodDelete, fake.URL+"/v2/cdn/endpoints/cdn-1/cache", `{"files":["images/*"]}`),
	} {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without a token = %d, want 401", req.Method, req.URL.Path, resp.StatusCode)
		}
	}
	if len(fake.Purges()) != 0 {
		t.Errorf("purges = %v, want none", fake.Purges())
	}
}

func httptestRequest(t *testing.T, method, url, body string) *http.Request {
	t.Helper()
	r, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
