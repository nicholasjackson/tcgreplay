package spaces_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

func TestPutThenGetRoundTripsBytesAndHeaders(t *testing.T) {
	s := spacestest.New(t)
	c := s.Client(t)
	ctx := context.Background()
	body := []byte(`{"fetched":"2026-11-04T12:00:00Z","data":{"id":"sv03.5-001"}}`)

	err := c.Put(ctx, "tcgdex/cards/sv03.5-001.json", body, spaces.PutOptions{
		ContentType: "application/json", CacheControl: "max-age=60", Public: true,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := c.Get(ctx, "tcgdex/cards/sv03.5-001.json")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("Get = %s, want %s", got, body)
	}
	h := s.Header("tcgdex/cards/sv03.5-001.json")
	for k, want := range map[string]string{"Content-Type": "application/json", "Cache-Control": "max-age=60", "X-Amz-Acl": "public-read"} {
		if got := h.Get(k); got != want {
			t.Errorf("stored %s = %q, want %q", k, got, want)
		}
	}
	if n := s.Requests(http.MethodPut, "tcgdex/cards/sv03.5-001.json"); n != 1 {
		t.Errorf("PUT requests = %d, want 1", n)
	}
	if n := s.Requests(http.MethodGet, "tcgdex/cards/sv03.5-001.json"); n != 1 {
		t.Errorf("GET requests = %d, want 1", n)
	}
	if n := s.Total(); n != 2 {
		t.Errorf("Total = %d, want 2", n)
	}
}

func TestPrivatePutHasNoACL(t *testing.T) {
	s := spacestest.New(t)
	if err := s.Client(t).Put(context.Background(), "k.json", []byte(`{}`), spaces.PutOptions{ContentType: "application/json"}); err != nil {
		t.Fatal(err)
	}
	if acl := s.Header("k.json").Get("X-Amz-Acl"); acl != "" {
		t.Errorf("X-Amz-Acl = %q, want none", acl)
	}
}

func TestAwkwardKeysRoundTrip(t *testing.T) {
	s := spacestest.New(t)
	c := s.Client(t)
	ctx := context.Background()
	for _, key := range []string{"a/b/c.json", "tcgdex/cards/swsh12.5-GG01.json", "with space/and+plus=eq.json", "unicodé/ñ.json"} {
		body := []byte(key)
		if err := c.Put(ctx, key, body, spaces.PutOptions{}); err != nil {
			t.Errorf("Put %q: %v", key, err)
			continue
		}
		if b, ok := s.Object(key); !ok || !bytes.Equal(b, body) {
			t.Errorf("stand-in holds %q under %q (present %v)", b, key, ok)
		}
		if got, err := c.Get(ctx, key); err != nil || !bytes.Equal(got, body) {
			t.Errorf("Get %q = %q, %v", key, got, err)
		}
	}
}

func TestGetMissingKeyIsNotFound(t *testing.T) {
	s := spacestest.New(t)
	_, err := s.Client(t).Get(context.Background(), "tcgdex/sets.json")
	if !errors.Is(err, spaces.ErrNotFound) {
		t.Fatalf("Get = %v, want ErrNotFound", err)
	}
	var e *spaces.Error
	if errors.As(err, &e) {
		t.Errorf("not found is also an *Error: %v", e)
	}
}

func TestGetSeededObject(t *testing.T) {
	s := spacestest.New(t)
	s.SetObject("tcgdex/sets.json", []byte(`[]`))
	got, err := s.Client(t).Get(context.Background(), "tcgdex/sets.json")
	if err != nil || string(got) != "[]" {
		t.Fatalf("Get = %q, %v", got, err)
	}
}

// wantError checks err is an *Error with status and code, and not "not
// found".
func wantError(t *testing.T, err error, status int, code string) {
	t.Helper()
	if errors.Is(err, spaces.ErrNotFound) {
		t.Fatalf("got not found, want %d %s", status, code)
	}
	var e *spaces.Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want *spaces.Error", err)
	}
	if e.Status != status || (code != "" && e.Code != code) {
		t.Errorf("got %d %s, want %d %s", e.Status, e.Code, status, code)
	}
}

func TestDeniedCredentialsReport403WithCode(t *testing.T) {
	s := spacestest.New(t)
	s.SetDeny(true)
	c := s.Client(t)
	_, err := c.Get(context.Background(), "x.json")
	wantError(t, err, http.StatusForbidden, "InvalidAccessKeyId")
	wantError(t, c.Put(context.Background(), "x.json", []byte("{}"), spaces.PutOptions{}), http.StatusForbidden, "InvalidAccessKeyId")
}

func TestWrongCredentialsAreRefusedBySignatureCheck(t *testing.T) {
	s := spacestest.New(t)
	ctx := context.Background()

	cfg := s.Config()
	cfg.SecretKey = "not-the-secret"
	c, err := spaces.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(ctx, "x.json")
	wantError(t, err, http.StatusForbidden, "SignatureDoesNotMatch")

	cfg = s.Config()
	cfg.AccessKey = "OTHERKEY"
	c, err = spaces.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	wantError(t, c.Put(ctx, "x.json", []byte("{}"), spaces.PutOptions{}), http.StatusForbidden, "InvalidAccessKeyId")
	if _, ok := s.Object("x.json"); ok {
		t.Error("a refused put was stored")
	}
}

// alteringTransport changes each request after the client signed it.
type alteringTransport struct{ alter func(*http.Request) }

func (a alteringTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	a.alter(r)
	return http.DefaultTransport.RoundTrip(r)
}

func TestRequestAlteredAfterSigningIsRefused(t *testing.T) {
	s := spacestest.New(t)
	for name, alter := range map[string]func(*http.Request){
		"content type": func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") },
		"acl added":    func(r *http.Request) { r.Header.Set("X-Amz-Acl", "public-read") },
		"other key":    func(r *http.Request) { r.URL.Path += "x"; r.URL.RawPath = "" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := s.Config()
			cfg.HTTP = &http.Client{Transport: alteringTransport{alter}}
			c, err := spaces.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			err = c.Put(context.Background(), "k.json", []byte(`{}`), spaces.PutOptions{ContentType: "application/json"})
			wantError(t, err, http.StatusForbidden, "SignatureDoesNotMatch")
		})
	}
}

func TestServerErrorReportsStatus(t *testing.T) {
	s := spacestest.New(t)
	s.SetFail(true)
	_, err := s.Client(t).Get(context.Background(), "x.json")
	wantError(t, err, http.StatusInternalServerError, "InternalError")
	s.SetFail(false)
	if _, err := s.Client(t).Get(context.Background(), "x.json"); !errors.Is(err, spaces.ErrNotFound) {
		t.Errorf("after SetFail(false): %v, want ErrNotFound", err)
	}
}

func TestClosedServerIsUnreachable(t *testing.T) {
	s := spacestest.New(t)
	c := s.Client(t)
	s.Close()
	_, err := c.Get(context.Background(), "x.json")
	if err == nil {
		t.Fatal("Get on a closed server succeeded")
	}
	var e *spaces.Error
	if errors.As(err, &e) || errors.Is(err, spaces.ErrNotFound) {
		t.Errorf("unreachable reported as %v", err)
	}
	s.Close() // idempotent
}

func TestHangIsBoundedByTheCallersContext(t *testing.T) {
	s := spacestest.New(t)
	release := s.Hang()
	defer release()
	c := s.Client(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Get(ctx, "x.json")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Get = %v, want a deadline error", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("Get took %v, want about 100ms", d)
	}
	if n := s.Requests(http.MethodGet, "x.json"); n != 1 {
		t.Errorf("requests = %d, want 1", n)
	}

	release()
	if _, err := c.Get(context.Background(), "x.json"); !errors.Is(err, spaces.ErrNotFound) {
		t.Errorf("after release: %v, want ErrNotFound", err)
	}
}

func TestResetForgetsCountsOnly(t *testing.T) {
	s := spacestest.New(t)
	c := s.Client(t)
	if err := c.Put(context.Background(), "k", []byte("1"), spaces.PutOptions{}); err != nil {
		t.Fatal(err)
	}
	s.Reset()
	if s.Total() != 0 {
		t.Errorf("Total after Reset = %d", s.Total())
	}
	if _, ok := s.Object("k"); !ok {
		t.Error("Reset dropped an object")
	}
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	if _, err := spaces.New(spaces.Config{Endpoint: "ftp://x", Bucket: "b", AccessKey: "k", SecretKey: "s"}); err == nil {
		t.Error("New accepted an ftp endpoint")
	}
	if _, err := spaces.New(spaces.Config{Endpoint: "https://x", AccessKey: "k", SecretKey: "s"}); err == nil {
		t.Error("New accepted a config with no bucket")
	}
}

// listAll follows a listing of prefix to its last page.
func listAll(t *testing.T, c *spaces.Client, prefix string) (keys []string, pages int) {
	t.Helper()
	token := ""
	for {
		page, err := c.List(context.Background(), prefix, token)
		if err != nil {
			t.Fatalf("List(%q, %q): %v", prefix, token, err)
		}
		pages++
		keys = append(keys, page.Keys...)
		if page.Next == "" {
			return keys, pages
		}
		if pages > 10 {
			t.Fatal("listing does not end")
		}
		token = page.Next
	}
}

func TestListReturnsKeysUnderPrefixAcrossPages(t *testing.T) {
	s := spacestest.New(t)
	for _, k := range []string{
		"images/en/sv/sv1/2/low.webp", "images/en/sv/sv1/1/low.webp", "images/en/sv/sv1/1/high.webp",
		"images/en/swsh/swsh12.5/157/low.webp", "images/en/swsh/swsh12.5/157/high.webp",
		"tcgdex/sets.json", "tcgdex/cards/sv1-1.json", "imagesque.json",
	} {
		s.SetObject(k, []byte(k))
	}
	s.PageSize(2)
	keys, pages := listAll(t, s.Client(t), "images/")
	want := []string{
		"images/en/sv/sv1/1/high.webp", "images/en/sv/sv1/1/low.webp", "images/en/sv/sv1/2/low.webp",
		"images/en/swsh/swsh12.5/157/high.webp", "images/en/swsh/swsh12.5/157/low.webp",
	}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("keys = %v, want %v", keys, want)
	}
	if pages != 3 {
		t.Errorf("pages = %d, want 3", pages)
	}
	if n := s.Requests("LIST", "images/"); n != 3 {
		t.Errorf("LIST requests = %d, want 3", n)
	}
}

func TestListOnePageAndEmptyPrefix(t *testing.T) {
	s := spacestest.New(t)
	s.SetObject("tcgdex/sets.json", []byte("{}"))
	c := s.Client(t)
	page, err := c.List(context.Background(), "images/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Keys) != 0 || page.Next != "" {
		t.Errorf("empty prefix listing = %+v, want no keys and no token", page)
	}
	page, err = c.List(context.Background(), "tcgdex/", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Keys) != 1 || page.Keys[0] != "tcgdex/sets.json" || page.Next != "" {
		t.Errorf("listing = %+v, want [tcgdex/sets.json] on one page", page)
	}
}

func TestListAwkwardPrefixIsSigned(t *testing.T) {
	s := spacestest.New(t)
	s.SetObject("with space/and+plus=eq.json", []byte("x"))
	s.SetObject("with spacer.json", []byte("x"))
	keys, _ := listAll(t, s.Client(t), "with space/")
	if len(keys) != 1 || keys[0] != "with space/and+plus=eq.json" {
		t.Errorf("keys = %v", keys)
	}
}

func TestListErrorsAreTyped(t *testing.T) {
	s := spacestest.New(t)
	c := s.Client(t)
	s.SetDeny(true)
	_, err := c.List(context.Background(), "images/", "")
	wantError(t, err, http.StatusForbidden, "InvalidAccessKeyId")
	s.SetDeny(false)
	s.SetFail(true)
	_, err = c.List(context.Background(), "images/", "")
	wantError(t, err, http.StatusInternalServerError, "InternalError")
	s.SetFail(false)
	s.Close()
	_, err = c.List(context.Background(), "images/", "")
	var e *spaces.Error
	if err == nil || errors.As(err, &e) || !strings.Contains(err.Error(), "spaces: list images/") {
		t.Errorf("closed server: err = %v, want a transport error naming the listing", err)
	}
}
