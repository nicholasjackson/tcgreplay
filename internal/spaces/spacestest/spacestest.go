// Package spacestest provides an in-process stand-in for DigitalOcean
// Spaces, so tests in any package can exercise the shared store with no
// network and no real bucket.
package spacestest

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
)

// The bucket and credentials a Server accepts.
const (
	Bucket    = "test-bucket"
	AccessKey = "TESTKEY"
	SecretKey = "testsecret"
	Region    = "us-east-1"
)

type object struct {
	body   []byte
	header http.Header
}

// Server is a stand-in for the Spaces S3 API: GET and PUT of whole
// objects in one bucket, path-style. It checks each request's signature
// with the real verifier, counts requests per method and key, and can be
// made to fail, refuse credentials, hang or become unreachable.
type Server struct {
	srv *httptest.Server

	mu      sync.Mutex
	objects map[string]object
	counts  map[string]int // "METHOD key" -> requests
	fail    bool
	deny    bool
	hang    chan struct{}
	closed  bool
}

// New starts a Server, closed when the test ends.
func New(t testing.TB) *Server {
	s := &Server{objects: map[string]object{}, counts: map[string]int{}}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Config is a client configuration pointing at the server, with the
// credentials it accepts.
func (s *Server) Config() spaces.Config {
	return spaces.Config{
		Endpoint:  s.srv.URL,
		Region:    Region,
		Bucket:    Bucket,
		AccessKey: AccessKey,
		SecretKey: SecretKey,
	}
}

// Client returns a spaces client for the server, failing the test if it
// cannot be built.
func (s *Server) Client(t testing.TB) *spaces.Client {
	t.Helper()
	c, err := spaces.New(s.Config())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "IncompleteBody", err.Error())
		return
	}

	s.mu.Lock()
	s.counts[r.Method+" "+key]++
	fail, deny, hang := s.fail, s.deny, s.hang
	s.mu.Unlock()

	if hang != nil {
		select {
		case <-hang:
		case <-r.Context().Done():
			return
		}
	}
	switch {
	case fail:
		writeError(w, http.StatusInternalServerError, "InternalError", "we encountered an internal error")
		return
	case deny:
		writeError(w, http.StatusForbidden, "InvalidAccessKeyId", "the access key id does not exist")
		return
	}
	if err := spaces.VerifyRequest(r, payload, AccessKey, SecretKey, Region); err != nil {
		e := err.(*spaces.Error)
		writeError(w, e.Status, e.Code, e.Message)
		return
	}
	if bucket != Bucket {
		writeError(w, http.StatusNotFound, "NoSuchBucket", "the specified bucket does not exist")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		o, ok := s.objects[key]
		s.mu.Unlock()
		if !ok {
			writeError(w, http.StatusNotFound, "NoSuchKey", "the specified key does not exist")
			return
		}
		for k, vs := range o.header {
			w.Header()[k] = vs
		}
		w.Write(o.body)
	case http.MethodPut:
		h := http.Header{}
		for _, k := range []string{"Content-Type", "Cache-Control", "X-Amz-Acl"} {
			if v := r.Header.Get(k); v != "" {
				h.Set(k, v)
			}
		}
		s.mu.Lock()
		s.objects[key] = object{body: payload, header: h}
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "the method is not allowed")
	}
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	var b strings.Builder
	xml.EscapeText(&b, []byte(msg))
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message></Error>`, code, b.String())
}

// Object returns the object stored under key.
func (s *Server) Object(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.objects[key]
	return o.body, ok
}

// Header returns the headers the object under key was written with:
// Content-Type, Cache-Control and X-Amz-Acl, where set.
func (s *Server) Header(key string) http.Header {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.objects[key].header.Clone()
}

// SetObject stores body under key, as if it had been put earlier.
func (s *Server) SetObject(key string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = object{body: append([]byte(nil), body...), header: http.Header{}}
}

// Requests is how many requests with method the server has had for key.
func (s *Server) Requests(method, key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counts[method+" "+key]
}

// Total is how many requests the server has had.
func (s *Server) Total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.counts {
		n += c
	}
	return n
}

// Reset forgets the request counts; objects are kept.
func (s *Server) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counts = map[string]int{}
}

// SetFail makes every request fail with 500 InternalError.
func (s *Server) SetFail(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = fail
}

// SetDeny makes every request be refused with 403 InvalidAccessKeyId, as
// Spaces answers wrong credentials.
func (s *Server) SetDeny(deny bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deny = deny
}

// Hang makes requests block until release is called or the client gives
// up.
func (s *Server) Hang() (release func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hang == nil {
		s.hang = make(chan struct{})
	}
	ch := s.hang
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.hang == ch {
			close(ch)
			s.hang = nil
		}
	}
}

// Close shuts the server down; later requests fail to connect. It is
// safe to call more than once.
func (s *Server) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	if s.hang != nil {
		close(s.hang)
		s.hang = nil
	}
	s.mu.Unlock()
	s.srv.Close()
}
