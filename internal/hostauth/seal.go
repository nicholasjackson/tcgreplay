package hostauth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
)

// minSigningKey is the shortest signing key accepted.
const minSigningKey = 32

// sealer seals the sign-in state the playmat hands out (client IDs, tokens
// and the board cookie) so it can be checked again without having been
// stored: each value is encrypted and authenticated with a key shared by
// every instance started with the same signing key, so it survives a
// restart. Without a signing key the key is made up at start, and a
// restart signs everyone out.
type sealer struct {
	aead cipher.AEAD
}

// newSealer returns a sealer keyed by secret, or by a random key when
// secret is empty.
func newSealer(secret string) (*sealer, error) {
	var k [32]byte
	switch {
	case secret == "":
		rand.Read(k[:])
	case len(secret) < minSigningKey:
		return nil, errors.New("hostauth: the signing key must be at least 32 characters")
	default:
		k = sha256.Sum256([]byte("tcgreplay sign-in\x00" + secret))
	}
	block, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &sealer{aead: aead}, nil
}

// seal returns v sealed as a value of kind. A value sealed as one kind
// never opens as another.
func (s *sealer) seal(kind string, v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	nonce := make([]byte, s.aead.NonceSize())
	rand.Read(nonce)
	return base64.RawURLEncoding.EncodeToString(s.aead.Seal(nonce, nonce, b, []byte(kind)))
}

// open reads a value of kind sealed by seal into v, reporting whether it
// was one.
func (s *sealer) open(kind, sealed string, v any) bool {
	b, err := base64.RawURLEncoding.DecodeString(sealed)
	n := s.aead.NonceSize()
	if err != nil || len(b) < n {
		return false
	}
	plain, err := s.aead.Open(nil, b[:n], b[n:], []byte(kind))
	if err != nil {
		return false
	}
	return json.Unmarshal(plain, v) == nil
}

// Kinds of sealed value.
const (
	sealClient  = "client"
	sealAccess  = "access"
	sealRefresh = "refresh"
	sealSession = "session"
)

// clientClaims is a registered client, sealed into its client ID.
type clientClaims struct {
	Redirects []string `json:"r"`
	Method    string   `json:"m"`
	SecretKey string   `json:"s,omitempty"`
	Created   int64    `json:"c"`
}

// grantClaims is a sign-in sealed into a token or the board cookie: who it
// is for, until when, and for a token which client holds it.
type grantClaims struct {
	// ID names a refresh token or board session, so the instance can tell
	// one already used or signed out.
	ID string `json:"i,omitempty"`
	// Client is clientKey of the client a token was issued to.
	Client string `json:"c,omitempty"`
	Sub    string `json:"u"`
	Email  string `json:"e"`
	Exp    int64  `json:"x"`
}

func (g grantClaims) account() Account { return Account{ID: g.Sub, Email: g.Email} }

// clientKey is a short stand-in for a client ID inside its tokens.
func clientKey(id string) string { return key(id)[:32] }
