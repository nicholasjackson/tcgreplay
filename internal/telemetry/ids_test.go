package telemetry

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

// testKey is a 37-character key; the golden IDs below were worked out
// from it independently of this package (Python's hmac and hashlib).
const testKey = "0123456789abcdef0123456789abcdef-test"

func mustIDs(t *testing.T, key string) *IDs {
	t.Helper()
	ids, err := NewIDs(key)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

// TestIDGolden pins the ID format: p_ and 16 hex characters of the keyed,
// domain-separated HMAC.
func TestIDGolden(t *testing.T) {
	ids := mustIDs(t, testKey)
	for email, want := range map[string]string{
		"ash@example.com":   "p_94f4d9148dc444be",
		"misty@example.com": "p_e356af17f70f9e00",
	} {
		if got := ids.ID(email); got != want {
			t.Errorf("ID(%q) = %q, want %q", email, got, want)
		}
	}
}

// TestIDStable gives the same email the same ID whatever its case or
// surrounding spaces, and from another IDs built with the same key.
func TestIDStable(t *testing.T) {
	a, b := mustIDs(t, testKey), mustIDs(t, testKey)
	want := "p_94f4d9148dc444be"
	for _, email := range []string{"ash@example.com", "Ash@Example.COM", "  ash@example.com\n", "ASH@EXAMPLE.COM "} {
		if got := a.ID(email); got != want {
			t.Errorf("ID(%q) = %q, want %q", email, got, want)
		}
		if got := b.ID(email); got != want {
			t.Errorf("second IDs: ID(%q) = %q, want %q", email, got, want)
		}
	}
}

// TestIDDiffers gives other emails and other keys other IDs.
func TestIDDiffers(t *testing.T) {
	ids := mustIDs(t, testKey)
	if ids.ID("ash@example.com") == ids.ID("misty@example.com") {
		t.Error("two emails share an ID")
	}
	other := mustIDs(t, testKey+"-rotated")
	if got := other.ID("ash@example.com"); got == "p_94f4d9148dc444be" {
		t.Errorf("another key gave the same ID %q", got)
	}
}

// TestIDNotUnkeyed checks that hashing the email without the key, in any
// common way, never gives its ID.
func TestIDNotUnkeyed(t *testing.T) {
	ids := mustIDs(t, testKey)
	email := "ash@example.com"
	id := ids.ID(email)
	hexOf := func(b []byte) string { return hex.EncodeToString(b) }
	var guesses []string
	for _, input := range []string{email, strings.ToUpper(email), "tcgreplay player\x00" + email} {
		sum := sha256.Sum256([]byte(input))
		guesses = append(guesses, hexOf(sum[:]), base64.StdEncoding.EncodeToString(sum[:]), base64.RawURLEncoding.EncodeToString(sum[:]))
		for _, key := range [][]byte{nil, []byte(testKey)} {
			m := hmac.New(sha256.New, key)
			m.Write([]byte(input))
			guesses = append(guesses, hexOf(m.Sum(nil)))
		}
	}
	for _, g := range guesses {
		if strings.Contains(g, strings.TrimPrefix(id, "p_")) || "p_"+g[:16] == id {
			t.Errorf("unkeyed hash %q matches the ID %q", g, id)
		}
	}
}

// TestNewIDsShortKey refuses a key under 32 characters and accepts one of
// exactly 32.
func TestNewIDsShortKey(t *testing.T) {
	for _, key := range []string{"", "short", strings.Repeat("k", 31)} {
		if _, err := NewIDs(key); err == nil {
			t.Errorf("NewIDs(%d characters) accepted", len(key))
		}
	}
	if _, err := NewIDs(strings.Repeat("k", 32)); err != nil {
		t.Errorf("NewIDs(32 characters): %v", err)
	}
}

// TestIDEmpty gives an empty email, or nil IDs, no ID.
func TestIDEmpty(t *testing.T) {
	ids := mustIDs(t, testKey)
	for _, email := range []string{"", "   "} {
		if got := ids.ID(email); got != "" {
			t.Errorf("ID(%q) = %q, want empty", email, got)
		}
	}
	var none *IDs
	if got := none.ID("ash@example.com"); got != "" {
		t.Errorf("nil IDs gave %q", got)
	}
}
