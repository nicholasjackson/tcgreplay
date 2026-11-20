// Package telemetry holds what the hosted playmat records about its use:
// the pseudonymous player ID that stands in for an email address, the
// recorder that writes one JSON record per line, and the redaction that
// keeps a player's own text out of error messages. It depends on no other
// playmat package, and a nil *Recorder records nothing, so the desktop,
// daemon and dev builds, which build none, are unchanged.
package telemetry

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// MinKey is the shortest player-ID key accepted, the same as the sign-in
// signing key's minimum.
const MinKey = 32

// IDs turns email addresses into player IDs: "p_" and the first 16 hex
// characters of a keyed, domain-separated HMAC-SHA256 of the trimmed,
// lower-cased address. The same key always gives the same ID, so an ID
// survives restarts and redeploys; without the key a list of known emails
// cannot be hashed to match it, and another key gives every player
// another ID.
type IDs struct {
	key []byte
}

// NewIDs returns the IDs keyed by secret, or an error when secret is
// shorter than MinKey characters.
func NewIDs(secret string) (*IDs, error) {
	if len(secret) < MinKey {
		return nil, errors.New("telemetry: the player-ID key must be at least 32 characters")
	}
	k := sha256.Sum256([]byte("tcgreplay player-id\x00" + secret))
	return &IDs{key: k[:]}, nil
}

// ID returns the player ID of email, or "" for an empty email (or nil
// IDs).
func (s *IDs) ID(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if s == nil || email == "" {
		return ""
	}
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte("tcgreplay player\x00" + email))
	return "p_" + hex.EncodeToString(m.Sum(nil))[:16]
}
