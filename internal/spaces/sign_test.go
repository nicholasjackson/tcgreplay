package spaces

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	exampleKey    = "AKIAIOSFODNN7EXAMPLE"
	exampleSecret = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
)

// TestSignMatchesAWSGetObjectExample signs the GET Object worked example
// from AWS's "Signature Calculations for the Authorization Header:
// Transferring Payload in a Single Chunk" and checks the Authorization
// header is exactly the one AWS documents.
func TestSignMatchesAWSGetObjectExample(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-9")
	sign(req, emptyHash, exampleKey, exampleSecret, "us-east-1", time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))

	const want = "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, " +
		"SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization =\n%s\nwant\n%s", got, want)
	}
	if got := req.Header.Get("X-Amz-Date"); got != "20130524T000000Z" {
		t.Errorf("X-Amz-Date = %q", got)
	}
	if got := req.Header.Get("X-Amz-Content-Sha256"); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("X-Amz-Content-Sha256 = %q", got)
	}
}

func TestCanonicalURIEncodesEachSegmentOnce(t *testing.T) {
	for path, want := range map[string]string{
		"":                          "/",
		"/bucket/tcgdex/sets.json":  "/bucket/tcgdex/sets.json",
		"/bucket/a b/c~d_e-f.g":     "/bucket/a%20b/c~d_e-f.g",
		"/bucket/é/x+y=z":           "/bucket/%C3%A9/x%2By%3Dz",
		"/bucket/already%20encoded": "/bucket/already%2520encoded",
	} {
		if got := canonicalURI(path); got != want {
			t.Errorf("canonicalURI(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestCanonicalQuerySortsAndEncodes(t *testing.T) {
	if got, want := canonicalQuery("prefix=a b&list-type=2&delimiter=%2F"), "delimiter=%2F&list-type=2&prefix=a%20b"; got != want {
		t.Errorf("canonicalQuery = %q, want %q", got, want)
	}
}

// signedRequest returns a server-side view of a PUT signed by sign.
func signedRequest(t *testing.T, body []byte, key, secret string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:9000/bucket/tcgdex/cards/a%20b.json", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "max-age=60")
	sign(req, payloadHash(body), key, secret, "us-east-1", time.Now())
	return req
}

func TestVerifyRequestAcceptsWhatSignProduced(t *testing.T) {
	body := []byte(`{"fetched":"2026-11-04T12:00:00Z","data":{}}`)
	req := signedRequest(t, body, "TESTKEY", "testsecret")
	if err := VerifyRequest(req, body, "TESTKEY", "testsecret", "us-east-1"); err != nil {
		t.Fatalf("VerifyRequest: %v", err)
	}
}

func TestVerifyRequestRefusesAlteredRequests(t *testing.T) {
	body := []byte(`{"a":1}`)
	tests := []struct {
		name     string
		alter    func(r *http.Request) []byte
		key      string
		wantCode string
	}{
		{"changed header", func(r *http.Request) []byte { r.Header.Set("Content-Type", "text/plain"); return body }, "TESTKEY", "SignatureDoesNotMatch"},
		{"added acl", func(r *http.Request) []byte { r.Header.Set("X-Amz-Acl", "public-read"); return body }, "TESTKEY", "SignatureDoesNotMatch"},
		{"changed body", func(r *http.Request) []byte { return []byte(`{"a":2}`) }, "TESTKEY", "SignatureDoesNotMatch"},
		{"changed path", func(r *http.Request) []byte { r.URL.Path = "/bucket/other.json"; return body }, "TESTKEY", "SignatureDoesNotMatch"},
		{"wrong secret", nil, "TESTKEY", "SignatureDoesNotMatch"},
		{"wrong key", func(r *http.Request) []byte { return body }, "OTHERKEY", "InvalidAccessKeyId"},
		{"unsigned", func(r *http.Request) []byte { r.Header.Del("Authorization"); return body }, "TESTKEY", "AccessDenied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := "testsecret"
			if tt.alter == nil {
				secret = "othersecret"
			}
			req := signedRequest(t, body, "TESTKEY", secret)
			payload := body
			if tt.alter != nil {
				payload = tt.alter(req)
			}
			err := VerifyRequest(req, payload, tt.key, "testsecret", "us-east-1")
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("VerifyRequest = %v, want *Error", err)
			}
			if e.Status != http.StatusForbidden || e.Code != tt.wantCode {
				t.Errorf("got %d %s, want 403 %s", e.Status, e.Code, tt.wantCode)
			}
		})
	}
}
