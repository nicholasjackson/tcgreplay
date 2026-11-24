package hostauth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	// ProviderGoogle is the provider of an account signed in with Google.
	// An account with no provider recorded is a Google account.
	ProviderGoogle = "google"
	// ProviderPassword is the provider of an operator-configured username
	// and password account.
	ProviderPassword = "password"

	// passwordIDPrefix starts every password account's ID. Google account
	// IDs are Google's numeric "sub", so the two never meet.
	passwordIDPrefix = "password:"
	// minBcryptCost is the lowest bcrypt cost a configured hash may have.
	minBcryptCost = 10
	// maxPasswordBytes is the longest password bcrypt can compare.
	maxPasswordBytes = 72
)

// usernameRule is what a password account's username may be, after
// lower-casing: a plain name or an email address, never containing the
// list's separators (",", ":" or whitespace).
var usernameRule = regexp.MustCompile(`^[a-z0-9._+@-]{1,254}$`)

// PasswordAccounts are the username and password accounts the operator
// configured, parsed once at start-up. Its methods are the only way to
// reach the usernames and hashes; nothing it returns or reports contains
// a configured hash or a password.
type PasswordAccounts struct {
	byName map[string]passwordAccount
	// dummy is compared against for an unknown username, at the accounts'
	// one cost, so an unknown username takes as long as a known one.
	dummy []byte
}

type passwordAccount struct {
	hash []byte
	fp   string // first 16 hex characters of SHA-256 of hash
}

// ParsePasswordAccounts parses the configured accounts: entries of
// "username:bcrypt-hash" (as `htpasswd -nBC 12 <username>` prints them),
// separated by commas or whitespace. An empty value means no accounts.
// It refuses a malformed entry, an invalid or repeated username, a hash
// that is not bcrypt (so a plain-text password is refused), a bcrypt
// cost below 10, and entries at different costs: one cost for every
// account is what makes an unknown username's check (against a dummy hash
// at that cost) take as long as a wrong password's. Its errors name the entry by position and the rule
// broken, never the entry's username or value.
func ParsePasswordAccounts(raw string) (*PasswordAccounts, error) {
	p := &PasswordAccounts{byName: map[string]passwordAccount{}}
	entries := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	accountCost := 0
	for i, e := range entries {
		n := i + 1
		name, hash, ok := strings.Cut(e, ":")
		if !ok {
			return nil, fmt.Errorf("entry %d is not username:bcrypt-hash", n)
		}
		name = strings.ToLower(name)
		if !usernameRule.MatchString(name) {
			return nil, fmt.Errorf("entry %d: the username must be 1 to 254 letters, digits or . _ + @ -", n)
		}
		cost, err := bcrypt.Cost([]byte(hash))
		if err != nil {
			return nil, fmt.Errorf("entry %d: the password is not a bcrypt hash (make one with htpasswd -nBC 12 <username>)", n)
		}
		if cost < minBcryptCost {
			return nil, fmt.Errorf("entry %d: the bcrypt cost is below %d", n, minBcryptCost)
		}
		if accountCost != 0 && cost != accountCost {
			return nil, fmt.Errorf("entry %d: every entry must use the same bcrypt cost as entry 1 (htpasswd -nBC 12 makes cost 12)", n)
		}
		accountCost = cost
		if _, dup := p.byName[name]; dup {
			return nil, fmt.Errorf("entry %d repeats an earlier entry's username", n)
		}
		sum := sha256.Sum256([]byte(hash))
		p.byName[name] = passwordAccount{hash: []byte(hash), fp: hex.EncodeToString(sum[:])[:16]}
	}
	if len(p.byName) > 0 {
		dummy, err := bcrypt.GenerateFromPassword([]byte("tcgreplay unknown username"), accountCost)
		if err != nil {
			return nil, fmt.Errorf("preparing password checks: %v", err)
		}
		p.dummy = dummy
	}
	return p, nil
}

// Len is the number of configured accounts. A nil PasswordAccounts has
// none.
func (p *PasswordAccounts) Len() int {
	if p == nil {
		return 0
	}
	return len(p.byName)
}

// normalise is how a username is compared: trimmed and lower-cased.
func normalise(username string) string { return strings.ToLower(strings.TrimSpace(username)) }

// Check reports whether username and password name a configured account,
// and returns that account. It always runs one bcrypt comparison, against
// a dummy hash for an unknown username, and returns one answer, so a
// caller cannot tell an unknown username from a wrong password by its
// result or its timing.
func (p *PasswordAccounts) Check(username, password string) (Account, bool) {
	if p.Len() == 0 {
		return Account{}, false
	}
	name := normalise(username)
	acct, known := p.byName[name]
	hash := p.dummy
	if known {
		hash = acct.hash
	}
	tooLong := len(password) > maxPasswordBytes
	cmp := password
	if tooLong {
		cmp = "" // bcrypt refuses over 72 bytes; compare something of the same shape
	}
	ok := bcrypt.CompareHashAndPassword(hash, []byte(cmp)) == nil
	if !known || tooLong || !ok {
		return Account{}, false
	}
	return PasswordAccount(name), true
}

// PasswordAccount is the signed-in account for a configured username: an
// ID in its own space ("password:<username>"), never equal to any Google
// account's, with the username as its display name.
func PasswordAccount(username string) Account {
	name := normalise(username)
	return Account{ID: passwordIDPrefix + name, Email: name, Provider: ProviderPassword}
}

// Fingerprint is a short hash of the username's configured hash, sealed
// into the account's sign-ins so a changed password ends them.
func (p *PasswordAccounts) Fingerprint(username string) (string, bool) {
	if p.Len() == 0 {
		return "", false
	}
	acct, ok := p.byName[normalise(username)]
	return acct.fp, ok
}

// Valid reports whether a sign-in sealed for username with fingerprint fp
// still stands: the username is still configured, with the same hash.
func (p *PasswordAccounts) Valid(username, fp string) bool {
	want, ok := p.Fingerprint(username)
	return ok && fp != "" && subtle.ConstantTimeCompare([]byte(want), []byte(fp)) == 1
}
