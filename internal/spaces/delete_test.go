package spaces_test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/spaces"
	"github.com/nicholasjackson/tcgreplay/internal/spaces/spacestest"
)

func TestDeleteRemovesExactlyTheKeysGiven(t *testing.T) {
	s := spacestest.New(t)
	for _, k := range []string{"tcgdex/a.json", "tcgdex/b.json", "tcgdex/c.json", "tcgdex/d.json", "images/x/low.webp"} {
		s.SetObject(k, []byte(k))
	}
	if err := s.Client(t).Delete(context.Background(), []string{"tcgdex/a.json", "tcgdex/c.json", "images/x/low.webp"}); err != nil {
		t.Fatal(err)
	}
	if got, want := s.Keys(""), []string{"tcgdex/b.json", "tcgdex/d.json"}; !slices.Equal(got, want) {
		t.Errorf("left = %v, want %v", got, want)
	}
	if n := s.Requests("POST", "?delete"); n != 1 {
		t.Errorf("delete requests = %d, want 1", n)
	}
}

func TestDeleteSplitsIntoBatchesOfAThousand(t *testing.T) {
	s := spacestest.New(t)
	var keys []string
	for i := range 2500 {
		k := fmt.Sprintf("tcgdex/cards/sv1-%04d.json", i)
		keys = append(keys, k)
		s.SetObject(k, []byte("{}"))
	}
	s.SetObject("tcgdex/sets.json", []byte("[]"))
	if err := s.Client(t).Delete(context.Background(), keys); err != nil {
		t.Fatal(err)
	}
	if n := s.Requests("POST", "?delete"); n != 3 {
		t.Errorf("delete requests = %d, want 3", n)
	}
	if got := s.Keys(""); !slices.Equal(got, []string{"tcgdex/sets.json"}) {
		t.Errorf("left = %d keys (%v), want only tcgdex/sets.json", len(got), got[:min(len(got), 3)])
	}
}

func TestDeleteMissingKeyAndNoKeysAreNotErrors(t *testing.T) {
	s := spacestest.New(t)
	s.SetObject("tcgdex/a.json", []byte("{}"))
	c := s.Client(t)
	if err := c.Delete(context.Background(), []string{"tcgdex/missing.json", "tcgdex/a.json"}); err != nil {
		t.Errorf("delete with a missing key: %v", err)
	}
	if len(s.Keys("")) != 0 {
		t.Errorf("left = %v, want nothing", s.Keys(""))
	}
	if err := c.Delete(context.Background(), nil); err != nil {
		t.Errorf("delete of no keys: %v", err)
	}
	if n := s.Requests("POST", "?delete"); n != 1 {
		t.Errorf("delete requests = %d, want 1 (none for no keys)", n)
	}
}

func TestDeleteAwkwardKeys(t *testing.T) {
	s := spacestest.New(t)
	keys := []string{"with space/and+plus=eq.json", "amp&lt<gt>.json", "unicodé/é.json"}
	for _, k := range keys {
		s.SetObject(k, []byte("x"))
	}
	if err := s.Client(t).Delete(context.Background(), keys); err != nil {
		t.Fatal(err)
	}
	if left := s.Keys(""); len(left) != 0 {
		t.Errorf("left = %v", left)
	}
}

// wantNotDeleted checks err is a *DeleteError naming exactly keys, each
// with code.
func wantNotDeleted(t *testing.T, err error, code string, keys ...string) {
	t.Helper()
	var de *spaces.DeleteError
	if !errors.As(err, &de) {
		t.Fatalf("err = %v, want *spaces.DeleteError", err)
	}
	if len(de.Failed) != len(keys) {
		t.Errorf("failed = %v, want %v", de.Failed, keys)
	}
	for _, k := range keys {
		if de.Failed[k] != code {
			t.Errorf("failed[%s] = %q, want %q (all: %v)", k, de.Failed[k], code, de.Failed)
		}
	}
}

func TestDeleteFailuresAreReported(t *testing.T) {
	s := spacestest.New(t)
	c := s.Client(t)
	s.SetObject("a", []byte("x"))
	s.SetObject("b", []byte("x"))
	ab := []string{"a", "b"}

	s.SetDeny(true)
	err := c.Delete(context.Background(), ab)
	wantError(t, err, http.StatusForbidden, "InvalidAccessKeyId")
	wantNotDeleted(t, err, "InvalidAccessKeyId", "a", "b")
	if !strings.Contains(err.Error(), "2 keys not deleted (first: a InvalidAccessKeyId)") {
		t.Errorf("refused credentials: error = %q, want the keys named", err)
	}
	s.SetDeny(false)

	s.SetFail(true)
	err = c.Delete(context.Background(), ab)
	wantError(t, err, http.StatusInternalServerError, "InternalError")
	wantNotDeleted(t, err, "InternalError", "a", "b")
	s.SetFail(false)

	s.CorruptMD5(true)
	err = c.Delete(context.Background(), ab)
	wantError(t, err, http.StatusBadRequest, "InvalidDigest")
	wantNotDeleted(t, err, "InvalidDigest", "a", "b")
	s.CorruptMD5(false)
	if got := s.Keys(""); !slices.Equal(got, ab) {
		t.Fatalf("refused deletes removed objects: left %v", got)
	}

	s.Close()
	err = c.Delete(context.Background(), []string{"a"})
	var e *spaces.Error
	if err == nil || errors.As(err, &e) || !strings.Contains(err.Error(), "spaces: delete 1 keys") {
		t.Errorf("closed server: err = %v, want a transport error naming the delete", err)
	}
	wantNotDeleted(t, err, "RequestFailed", "a")
}

func TestDeleteRequestFailureNamesTheKeysNotDeleted(t *testing.T) {
	s := spacestest.New(t)
	var keys []string
	for i := range 1500 {
		k := fmt.Sprintf("k%04d", i)
		keys = append(keys, k)
		s.SetObject(k, []byte("x"))
	}
	s.SetFail(true)
	err := s.Client(t).Delete(context.Background(), keys)
	wantNotDeleted(t, err, "InternalError", keys...)
	if n := s.Requests("POST", "?delete"); n != 1 {
		t.Errorf("delete requests = %d, want 1 (the call stops at the failed request)", n)
	}
}

func TestDeleteReportsKeysRefusedOneByOne(t *testing.T) {
	s := spacestest.New(t)
	var keys []string
	for i := range 1500 {
		k := fmt.Sprintf("k%04d", i)
		keys = append(keys, k)
		s.SetObject(k, []byte("x"))
	}
	s.DenyDelete("k0001", "k1200")
	err := s.Client(t).Delete(context.Background(), keys)
	var de *spaces.DeleteError
	if !errors.As(err, &de) {
		t.Fatalf("err = %v, want *spaces.DeleteError", err)
	}
	if len(de.Failed) != 2 || de.Failed["k0001"] != "AccessDenied" || de.Failed["k1200"] != "AccessDenied" {
		t.Errorf("failed = %v, want k0001 and k1200 AccessDenied", de.Failed)
	}
	if !strings.Contains(err.Error(), "2 keys not deleted (first: k0001 AccessDenied)") {
		t.Errorf("error = %q", err)
	}
	if got := s.Keys(""); !slices.Equal(got, []string{"k0001", "k1200"}) {
		t.Errorf("left = %v, want only the refused keys", got)
	}
	if n := s.Requests("POST", "?delete"); n != 2 {
		t.Errorf("delete requests = %d, want 2", n)
	}
}

func md5hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}

func TestListReportsEachObjectsChecksum(t *testing.T) {
	s := spacestest.New(t)
	c := s.Client(t)
	ctx := context.Background()
	if err := c.Put(ctx, "images/en/sv/sv1/1/low.webp", []byte("low bytes"), spaces.PutOptions{Public: true}); err != nil {
		t.Fatal(err)
	}
	s.SetObject("images/en/sv/sv1/1/high.webp", []byte("high bytes"))
	s.PageSize(1)
	tags := map[string]string{}
	token := ""
	for {
		page, err := c.List(ctx, "images/", token)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range page.Keys {
			tags[k] = page.ETags[k]
		}
		if token = page.Next; token == "" {
			break
		}
	}
	want := map[string]string{
		"images/en/sv/sv1/1/low.webp":  md5hex([]byte("low bytes")),
		"images/en/sv/sv1/1/high.webp": md5hex([]byte("high bytes")),
	}
	for k, w := range want {
		if tags[k] != w {
			t.Errorf("ETag of %s = %q, want %q", k, tags[k], w)
		}
	}

	// Rewritten with other bytes, the object lists a new checksum.
	if err := c.Put(ctx, "images/en/sv/sv1/1/low.webp", []byte("new low bytes"), spaces.PutOptions{Public: true}); err != nil {
		t.Fatal(err)
	}
	s.PageSize(0)
	page, err := c.List(ctx, "images/", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := page.ETags["images/en/sv/sv1/1/low.webp"]; got != md5hex([]byte("new low bytes")) || got == want["images/en/sv/sv1/1/low.webp"] {
		t.Errorf("rewritten ETag = %q, want the MD5 of the new bytes", got)
	}
	if len(page.Keys) != 2 || page.Next != "" {
		t.Errorf("listing = %+v, want both keys on one page", page)
	}
}

func TestListLeavesOutAMissingChecksum(t *testing.T) {
	s := spacestest.New(t)
	s.SetObject("images/a/low.webp", []byte("a"))
	s.SetObject("images/a/high.webp", []byte("b"))
	s.OmitETag("images/a/high.webp")
	page, err := s.Client(t).List(context.Background(), "images/", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := page.ETags["images/a/high.webp"]; ok {
		t.Errorf("ETags = %v, want no entry for high.webp", page.ETags)
	}
	if page.ETags["images/a/low.webp"] != md5hex([]byte("a")) {
		t.Errorf("ETags = %v, want low.webp's MD5", page.ETags)
	}
	if len(page.Keys) != 2 {
		t.Errorf("keys = %v, want both", page.Keys)
	}
}
