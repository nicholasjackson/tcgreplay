// Package spacestest provides an in-process stand-in for DigitalOcean
// Spaces, so tests in any package can exercise the shared store with no
// network and no real bucket.
package spacestest

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
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
	etag   string // hex MD5 of body, as S3 reports a single-part upload
}

// newObject stores body with its ETag.
func newObject(body []byte, h http.Header) object {
	sum := md5.Sum(body)
	return object{body: body, header: h, etag: hex.EncodeToString(sum[:])}
}

// Server is a stand-in for the Spaces S3 API: GET and PUT of whole
// objects in one bucket, path-style, listing the bucket (ListObjectsV2,
// counted as method "LIST" with the prefix as key, each object with its
// ETag, the hex MD5 of its bytes) and deleting objects in batches
// (DeleteObjects, counted as method "POST" with key "?delete"). It checks each
// request's signature with the real verifier, counts requests per method
// and key, and can be made to fail, refuse credentials, hang or become
// unreachable. Like Spaces and its CDN, it serves an object put with
// public-read access to an unsigned GET, and refuses an unsigned GET of a
// private or missing object with 403 AccessDenied.
type Server struct {
	srv *httptest.Server

	mu      sync.Mutex
	objects map[string]object
	counts  map[string]int // "METHOD key" -> requests
	fail    bool
	deny    bool
	hang    chan struct{}
	closed  bool
	page    int // listing page size; 0 means 1000

	denyDelete map[string]bool // keys DeleteObjects refuses one by one
	noETag     map[string]bool // keys listed without an ETag
	badMD5     bool            // check Content-MD5 against altered bytes
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

	list := r.Method == http.MethodGet && key == "" && r.URL.Query().Get("list-type") == "2"
	del := r.Method == http.MethodPost && key == "" && r.URL.Query().Has("delete")
	// An unsigned read is an anonymous one, as a browser makes through the
	// CDN: credentials do not apply to it.
	anonymous := r.Header.Get("Authorization") == "" && r.Method == http.MethodGet && key != ""

	s.mu.Lock()
	switch {
	case list:
		s.counts["LIST "+r.URL.Query().Get("prefix")]++
	case del:
		s.counts["POST ?delete"]++
	default:
		s.counts[r.Method+" "+key]++
	}
	fail, deny, hang := s.fail, s.deny && !anonymous, s.hang
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
	if anonymous {
		s.serveAnonymous(w, bucket, key)
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
	if list {
		s.serveList(w, r)
		return
	}
	if del {
		s.serveDelete(w, r, payload)
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
		s.objects[key] = newObject(payload, h)
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	default:
		writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "the method is not allowed")
	}
}

// serveAnonymous answers an unsigned GET: the object when it was put
// public-read, otherwise 403 AccessDenied, missing or not.
func (s *Server) serveAnonymous(w http.ResponseWriter, bucket, key string) {
	s.mu.Lock()
	o, ok := s.objects[key]
	s.mu.Unlock()
	if bucket != Bucket || !ok || o.header.Get("X-Amz-Acl") != "public-read" {
		writeError(w, http.StatusForbidden, "AccessDenied", "access denied")
		return
	}
	for k, vs := range o.header {
		w.Header()[k] = vs
	}
	w.Write(o.body)
}

// listResult is the ListObjectsV2 answer.
type listResult struct {
	XMLName               xml.Name `xml:"ListBucketResult"`
	Name                  string   `xml:"Name"`
	Prefix                string   `xml:"Prefix"`
	KeyCount              int      `xml:"KeyCount"`
	MaxKeys               int      `xml:"MaxKeys"`
	IsTruncated           bool     `xml:"IsTruncated"`
	Contents              []listObject
	NextContinuationToken string `xml:"NextContinuationToken,omitempty"`
}

type listObject struct {
	XMLName xml.Name `xml:"Contents"`
	Key     string   `xml:"Key"`
	ETag    string   `xml:"ETag,omitempty"`
	Size    int      `xml:"Size"`
}

// serveList answers ListObjectsV2: the keys under prefix in key order,
// after the continuation token (the last key of the previous page, opaque
// to clients), at most max-keys or the page size at a time.
func (s *Server) serveList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	prefix, token := q.Get("prefix"), q.Get("continuation-token")
	s.mu.Lock()
	size := s.page
	var keys []string
	sizes := map[string]int{}
	etags := map[string]string{}
	for k, o := range s.objects {
		if strings.HasPrefix(k, prefix) && k > token {
			keys = append(keys, k)
			sizes[k] = len(o.body)
			if !s.noETag[k] {
				etags[k] = `"` + o.etag + `"`
			}
		}
	}
	s.mu.Unlock()
	if size <= 0 {
		size = 1000
	}
	if n, err := strconv.Atoi(q.Get("max-keys")); err == nil && n > 0 && n < size {
		size = n
	}
	sort.Strings(keys)
	res := listResult{Name: Bucket, Prefix: prefix, MaxKeys: size}
	if len(keys) > size {
		keys = keys[:size]
		res.IsTruncated = true
		res.NextContinuationToken = keys[len(keys)-1]
	}
	res.KeyCount = len(keys)
	for _, k := range keys {
		res.Contents = append(res.Contents, listObject{Key: k, ETag: etags[k], Size: sizes[k]})
	}
	w.Header().Set("Content-Type", "application/xml")
	io.WriteString(w, xml.Header)
	xml.NewEncoder(w).Encode(res)
}

// serveDelete answers DeleteObjects: it checks Content-MD5 against the
// body, as S3 requires, removes every listed key that exists (a missing
// key succeeds) except those DenyDelete names, which are reported as
// AccessDenied, and answers the errors only (quiet mode).
func (s *Server) serveDelete(w http.ResponseWriter, r *http.Request, payload []byte) {
	s.mu.Lock()
	bad := s.badMD5
	s.mu.Unlock()
	digest := payload
	if bad {
		digest = append(append([]byte(nil), payload...), '!')
	}
	sum := md5.Sum(digest)
	if r.Header.Get("Content-Md5") != base64.StdEncoding.EncodeToString(sum[:]) {
		writeError(w, http.StatusBadRequest, "InvalidDigest", "the Content-MD5 you specified did not match what was received")
		return
	}
	var req struct {
		Objects []struct {
			Key string `xml:"Key"`
		} `xml:"Object"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		writeError(w, http.StatusBadRequest, "MalformedXML", err.Error())
		return
	}
	if len(req.Objects) > 1000 {
		writeError(w, http.StatusBadRequest, "MalformedXML", "more than 1000 keys")
		return
	}
	type deleteError struct {
		XMLName xml.Name `xml:"Error"`
		Key     string   `xml:"Key"`
		Code    string   `xml:"Code"`
		Message string   `xml:"Message"`
	}
	res := struct {
		XMLName xml.Name `xml:"DeleteResult"`
		Errors  []deleteError
	}{}
	s.mu.Lock()
	for _, o := range req.Objects {
		if s.denyDelete[o.Key] {
			res.Errors = append(res.Errors, deleteError{Key: o.Key, Code: "AccessDenied", Message: "access denied"})
			continue
		}
		delete(s.objects, o.Key)
	}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/xml")
	io.WriteString(w, xml.Header)
	xml.NewEncoder(w).Encode(res)
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
	s.objects[key] = newObject(append([]byte(nil), body...), http.Header{})
}

// Keys returns the keys of the objects stored under prefix, sorted.
func (s *Server) Keys(prefix string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []string
	for k := range s.objects {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// ETag is the ETag listings report for the object under key: the hex MD5
// of its bytes, unquoted.
func (s *Server) ETag(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.objects[key].etag
}

// DenyDelete makes DeleteObjects refuse keys one by one with AccessDenied,
// leaving them in place, while it deletes the others. With no keys it
// clears the list.
func (s *Server) DenyDelete(keys ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.denyDelete = map[string]bool{}
	for _, k := range keys {
		s.denyDelete[k] = true
	}
}

// OmitETag makes listings leave out the ETag of keys. With no keys it
// clears the list.
func (s *Server) OmitETag(keys ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.noETag = map[string]bool{}
	for _, k := range keys {
		s.noETag[k] = true
	}
}

// CorruptMD5 makes DeleteObjects check Content-MD5 against altered bytes,
// so every delete is refused with 400 InvalidDigest.
func (s *Server) CorruptMD5(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.badMD5 = on
}

// Public reports whether the object under key was put public-read, so
// anyone may read it unsigned.
func (s *Server) Public(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.objects[key]
	return ok && o.header.Get("X-Amz-Acl") == "public-read"
}

// PageSize makes listings answer at most n keys a page (n <= 0 restores
// the default of 1000).
func (s *Server) PageSize(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.page = n
}

// Requests is how many requests with method the server has had for key.
// Listings count as method "LIST" with the prefix as key.
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
