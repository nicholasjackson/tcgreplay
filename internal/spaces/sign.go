package spaces

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	algorithm   = "AWS4-HMAC-SHA256"
	service     = "s3"
	amzDateForm = "20060102T150405Z"
	dateForm    = "20060102"
)

// emptyHash is the SHA-256 of an empty payload.
var emptyHash = payloadHash(nil)

// payloadHash returns the hex SHA-256 of body, as X-Amz-Content-Sha256
// carries it.
func payloadHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// sign signs req with AWS Signature Version 4 for S3: it sets X-Amz-Date
// and X-Amz-Content-Sha256 and then Authorization. hash is the hex SHA-256
// of the body req will send. The signed headers are host and every
// x-amz-*, content-type, content-md5, cache-control and range header the
// request carries.
func sign(req *http.Request, hash, accessKey, secretKey, region string, now time.Time) {
	amzDate := now.UTC().Format(amzDateForm)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", hash)

	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	var names []string
	for k := range req.Header {
		if lk := strings.ToLower(k); signable(lk) {
			names = append(names, lk)
		}
	}
	names = append(names, "host")
	sort.Strings(names)

	sig, scope := signature(req.Method, req.URL, host, req.Header, names, hash, amzDate, region, secretKey)
	req.Header.Set("Authorization", fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, accessKey, scope, strings.Join(names, ";"), sig))
}

// signable reports whether a lower-cased header name is one sign signs.
func signable(name string) bool {
	switch name {
	case "content-type", "content-md5", "cache-control", "range":
		return true
	}
	return strings.HasPrefix(name, "x-amz-")
}

// signature computes the request signature and its credential scope from
// the parts of a request that are signed. names are the signed header
// names, lower-cased and sorted; "host" is taken from host, the others
// from h.
func signature(method string, u *url.URL, host string, h http.Header, names []string, hash, amzDate, region, secretKey string) (sig, scope string) {
	var canon strings.Builder
	canon.WriteString(method + "\n")
	canon.WriteString(canonicalURI(u.Path) + "\n")
	canon.WriteString(canonicalQuery(u.RawQuery) + "\n")
	for _, n := range names {
		v := host
		if n != "host" {
			v = strings.Join(h.Values(n), ",")
		}
		canon.WriteString(n + ":" + strings.Join(strings.Fields(v), " ") + "\n")
	}
	canon.WriteString("\n")
	canon.WriteString(strings.Join(names, ";") + "\n")
	canon.WriteString(hash)

	date := amzDate
	if len(date) >= len(dateForm) {
		date = date[:len(dateForm)]
	}
	scope = date + "/" + region + "/" + service + "/aws4_request"
	sum := sha256.Sum256([]byte(canon.String()))
	toSign := algorithm + "\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])

	key := hmacSHA256([]byte("AWS4"+secretKey), date)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, service)
	key = hmacSHA256(key, "aws4_request")
	return hex.EncodeToString(hmacSHA256(key, toSign)), scope
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

// canonicalURI encodes each segment of a decoded path once, keeping "/",
// as S3 signing requires.
func canonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	segs := strings.Split(path, "/")
	for i, s := range segs {
		segs[i] = uriEncode(s)
	}
	return strings.Join(segs, "/")
}

// canonicalQuery sorts and encodes a raw query string.
func canonicalQuery(raw string) string {
	if raw == "" {
		return ""
	}
	q, _ := url.ParseQuery(raw)
	var pairs []string
	for k, vs := range q {
		for _, v := range vs {
			pairs = append(pairs, uriEncode(k)+"="+uriEncode(v))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

// uriEncode percent-encodes every byte of s except the RFC 3986 unreserved
// characters A-Z, a-z, 0-9, '-', '.', '_' and '~'.
func uriEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// VerifyRequest checks a request received by a server against the
// signature its Authorization header carries, as S3 would. It is for test
// doubles. payload is the body the server read. A request signed with
// another access key gets an *Error coded InvalidAccessKeyId; one whose
// signature or payload hash does not match, or that carries a signable
// header it did not sign, gets SignatureDoesNotMatch.
func VerifyRequest(r *http.Request, payload []byte, accessKey, secretKey, region string) error {
	auth := r.Header.Get("Authorization")
	rest, ok := strings.CutPrefix(auth, algorithm+" ")
	if !ok {
		return &Error{Status: http.StatusForbidden, Code: "AccessDenied", Message: "request is not signed with " + algorithm}
	}
	fields := map[string]string{}
	for _, part := range strings.Split(rest, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		fields[k] = v
	}
	key, _, _ := strings.Cut(fields["Credential"], "/")
	if key != accessKey {
		return &Error{Status: http.StatusForbidden, Code: "InvalidAccessKeyId", Message: "the access key id does not exist"}
	}
	mismatch := &Error{Status: http.StatusForbidden, Code: "SignatureDoesNotMatch", Message: "the request signature does not match"}
	hash := r.Header.Get("X-Amz-Content-Sha256")
	if hash != payloadHash(payload) {
		return mismatch
	}
	names := strings.Split(fields["SignedHeaders"], ";")
	signed := map[string]bool{}
	for _, n := range names {
		signed[n] = true
	}
	// As S3 does, refuse x-amz-* (and the other signable) headers that
	// were added without being signed.
	for k := range r.Header {
		if lk := strings.ToLower(k); signable(lk) && !signed[lk] {
			return mismatch
		}
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	want, _ := signature(r.Method, r.URL, host, r.Header, names, hash, r.Header.Get("X-Amz-Date"), region, secretKey)
	if !hmac.Equal([]byte(want), []byte(fields["Signature"])) {
		return mismatch
	}
	return nil
}
