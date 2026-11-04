package spacestest_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

// anonymousGet reads key the way a browser does: unsigned.
func anonymousGet(t *testing.T, s *spacestest.Server, key string) (int, []byte, http.Header) {
	t.Helper()
	resp, err := http.Get(s.Config().PublicBase() + "/" + key)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, resp.Header
}

func TestPublicObjectIsServedUnsigned(t *testing.T) {
	s := spacestest.New(t)
	c := s.Client(t)
	ctx := context.Background()
	body := []byte("webp bytes")
	if err := c.Put(ctx, "images/en/sv/sv1/1/low.webp", body, spaces.PutOptions{ContentType: "image/webp", CacheControl: "public, max-age=604800", Public: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(ctx, "tcgdex/sets.json", []byte("{}"), spaces.PutOptions{ContentType: "application/json"}); err != nil {
		t.Fatal(err)
	}
	if !s.Public("images/en/sv/sv1/1/low.webp") || s.Public("tcgdex/sets.json") || s.Public("missing") {
		t.Error("Public does not report each object's access")
	}

	status, got, h := anonymousGet(t, s, "images/en/sv/sv1/1/low.webp")
	if status != http.StatusOK || !bytes.Equal(got, body) {
		t.Fatalf("public object: %d %q, want 200 %q", status, got, body)
	}
	if h.Get("Content-Type") != "image/webp" || h.Get("Cache-Control") != "public, max-age=604800" {
		t.Errorf("public object headers = %v", h)
	}
	for _, key := range []string{"tcgdex/sets.json", "images/missing/low.webp"} {
		if status, _, _ := anonymousGet(t, s, key); status != http.StatusForbidden {
			t.Errorf("unsigned GET %s = %d, want 403", key, status)
		}
	}
	if n := s.Requests(http.MethodGet, "tcgdex/sets.json"); n != 1 {
		t.Errorf("unsigned GETs are not counted: %d", n)
	}
}

func TestUnsignedReadsIgnoreDenyButNotFail(t *testing.T) {
	s := spacestest.New(t)
	if err := s.Client(t).Put(context.Background(), "images/a/low.webp", []byte("a"), spaces.PutOptions{Public: true}); err != nil {
		t.Fatal(err)
	}
	s.SetDeny(true)
	if status, _, _ := anonymousGet(t, s, "images/a/low.webp"); status != http.StatusOK {
		t.Errorf("with deny, unsigned public GET = %d, want 200", status)
	}
	s.SetDeny(false)
	s.SetFail(true)
	if status, _, _ := anonymousGet(t, s, "images/a/low.webp"); status != http.StatusInternalServerError {
		t.Errorf("with fail, unsigned public GET = %d, want 500", status)
	}
}
