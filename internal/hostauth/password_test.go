package hostauth

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// hashFor returns a bcrypt hash of password at the lowest cost the
// accounts accept.
func hashFor(t *testing.T, password string) string {
	t.Helper()
	return costHash(t, password, minBcryptCost)
}

// costHash returns a bcrypt hash of password at cost.
func costHash(t *testing.T, password string, cost int) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		t.Fatal(err)
	}
	return string(h)
}

// TestParsePasswordAccounts shows several accounts, in any of the list's
// separators, each check against their own password, with plain and
// email-shaped usernames compared without regard to case.
func TestParsePasswordAccounts(t *testing.T) {
	alice, bob, rev := hashFor(t, "alice-pw"), hashFor(t, "bob-pw"), hashFor(t, "rev-pw")
	for name, raw := range map[string]string{
		"commas":   "alice:" + alice + ",bob:" + bob + ",Reviewer@Example.com:" + rev,
		"newlines": "alice:" + alice + "\nbob:" + bob + "\r\nReviewer@Example.com:" + rev + "\n",
		"spaces":   " alice:" + alice + "  bob:" + bob + "\tReviewer@Example.com:" + rev,
	} {
		t.Run(name, func(t *testing.T) {
			p, err := ParsePasswordAccounts(raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if p.Len() != 3 {
				t.Fatalf("Len = %d, want 3", p.Len())
			}
			for _, c := range []struct{ user, pw, wantID string }{
				{"alice", "alice-pw", "password:alice"},
				{"bob", "bob-pw", "password:bob"},
				{"reviewer@example.com", "rev-pw", "password:reviewer@example.com"},
				{"  REVIEWER@example.COM ", "rev-pw", "password:reviewer@example.com"},
			} {
				acct, ok := p.Check(c.user, c.pw)
				if !ok {
					t.Errorf("Check(%q, right password) refused", c.user)
					continue
				}
				if acct.ID != c.wantID || acct.Provider != ProviderPassword || acct.EmailVerified || !acct.Password() {
					t.Errorf("Check(%q) = %+v, want ID %s, provider password, unverified", c.user, acct, c.wantID)
				}
			}
			for _, c := range []struct{ user, pw string }{{"alice", "bob-pw"}, {"bob", "alice-pw"}, {"alice", ""}} {
				if _, ok := p.Check(c.user, c.pw); ok {
					t.Errorf("Check(%q, %q) accepted another account's or no password", c.user, c.pw)
				}
			}
		})
	}
	if p, err := ParsePasswordAccounts("  \n , "); err != nil || p.Len() != 0 {
		t.Errorf("an empty setting = %v accounts, %v; want none and no error", p.Len(), err)
	}
}

// TestParsePasswordAccountsRefuses shows a plain-text password, a
// malformed entry, an invalid username, a low cost and a repeated
// username are refused, with errors that never contain the entry's
// username or value.
func TestParsePasswordAccountsRefuses(t *testing.T) {
	good := hashFor(t, "secret-pw")
	low, _ := bcrypt.GenerateFromPassword([]byte("x"), bcrypt.MinCost)
	cases := map[string]struct{ raw, want string }{
		"plain text":         {"alice:hunter2", "entry 1"},
		"no colon":           {"zelda", "entry 1"},
		"empty username":     {":" + good, "entry 1"},
		"long username":      {strings.Repeat("q", 255) + ":" + good, "entry 1"},
		"username with !":    {"carol!:" + good, "entry 1"},
		"plain text second":  {"dave:" + good + ",erin:opensesame", "entry 2"},
		"low cost":           {"frank:" + string(low), "entry 1"},
		"repeated username":  {"gina:" + good + ",GINA:" + good, "entry 2"},
		"htpasswd md5 entry": {"henry:$apr1$abcdefgh$0123456789abcdefghijkl", "entry 1"},
		"mixed costs":        {"judy:" + good + ",kim:" + costHash(t, "x", minBcryptCost+1), "entry 2"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParsePasswordAccounts(c.raw)
			if err == nil {
				t.Fatal("accepted")
			}
			msg := err.Error()
			if !strings.Contains(msg, c.want) {
				t.Errorf("error %q does not name %s", msg, c.want)
			}
			for _, secret := range []string{"hunter2", "opensesame", "alice", "zelda", "carol", "dave", "erin", "frank", "gina", "henry", "judy", "kim", "$2a$", "$apr1$", strings.Repeat("q", 20)} {
				if strings.Contains(strings.ToLower(msg), strings.ToLower(secret)) {
					t.Errorf("error %q contains %q", msg, secret)
				}
			}
		})
	}
	// A space in a username splits the entry, so the first half has no
	// hash: refused all the same.
	if _, err := ParsePasswordAccounts("ian smith:" + good); err == nil {
		t.Error("a username with a space was accepted")
	}
}

// TestCheckUnknownAndWrongLookAlike shows an unknown username and a wrong
// password fail the same way, each taking at least one bcrypt comparison
// at the configured cost.
func TestCheckUnknownAndWrongLookAlike(t *testing.T) {
	p, err := ParsePasswordAccounts("alice:" + hashFor(t, "alice-pw"))
	if err != nil {
		t.Fatal(err)
	}
	// One comparison at the configured cost, measured directly.
	start := time.Now()
	bcrypt.CompareHashAndPassword(p.dummy, []byte("x"))
	one := time.Since(start)

	for name, c := range map[string]struct{ user, pw string }{
		"unknown username":   {"mallory", "alice-pw"},
		"wrong password":     {"alice", "wrong"},
		"over-long password": {"alice", strings.Repeat("p", 100)},
	} {
		start := time.Now()
		acct, ok := p.Check(c.user, c.pw)
		took := time.Since(start)
		if ok || acct != (Account{}) {
			t.Errorf("%s: Check = %+v, %v; want no account", name, acct, ok)
		}
		if took < one/2 {
			t.Errorf("%s took %v, less than half a bcrypt comparison (%v): it skipped the comparison", name, took, one)
		}
	}

	// The two failures take about the same time: the fastest of a few
	// runs of each is within a factor of two of the other's.
	fastest := func(user, pw string) time.Duration {
		best := time.Hour
		for range 5 {
			start := time.Now()
			p.Check(user, pw)
			best = min(best, time.Since(start))
		}
		return best
	}
	unknown, wrong := fastest("mallory", "alice-pw"), fastest("alice", "wrong")
	if unknown > 2*wrong || wrong > 2*unknown {
		t.Errorf("an unknown username took %v and a wrong password %v: not about the same", unknown, wrong)
	}
}

// TestPasswordAccountIdentityIsItsOwn shows a password account's ID never
// equals a Google account's, even when its username is exactly a Google
// account's email or Google's subject identifier.
func TestPasswordAccountIdentityIsItsOwn(t *testing.T) {
	google := []Account{
		{ID: "108234567890123456789", Email: "reviewer@example.com", EmailVerified: true},
		{ID: "reviewer@example.com", Email: "reviewer@example.com", EmailVerified: true},
		{ID: "openai", Email: "openai@example.com"},
	}
	for _, name := range []string{"reviewer@example.com", "openai", "108234567890123456789"} {
		pa := PasswordAccount(name)
		for _, g := range google {
			if pa.ID == g.ID {
				t.Errorf("password account %q has Google account %+v's ID", name, g)
			}
			if g.Password() {
				t.Errorf("Google account %+v reads as a password account", g)
			}
		}
		if !strings.HasPrefix(pa.ID, "password:") || pa.Email != name || pa.Provider != ProviderPassword {
			t.Errorf("PasswordAccount(%q) = %+v", name, pa)
		}
	}
}

// TestFingerprintRevokes shows a sign-in sealed with an account's
// fingerprint stops being valid when the account is removed or its
// password changes, and stays valid otherwise.
func TestFingerprintRevokes(t *testing.T) {
	before, err := ParsePasswordAccounts("alice:" + hashFor(t, "one") + ",bob:" + hashFor(t, "two"))
	if err != nil {
		t.Fatal(err)
	}
	fpA, okA := before.Fingerprint("alice")
	fpB, okB := before.Fingerprint("BOB")
	if !okA || !okB || len(fpA) != 16 || fpA == fpB {
		t.Fatalf("fingerprints %q %v, %q %v; want two different 16-character values", fpA, okA, fpB, okB)
	}
	if _, ok := before.Fingerprint("carol"); ok {
		t.Error("an unknown username has a fingerprint")
	}
	if !before.Valid("alice", fpA) || before.Valid("alice", fpB) || before.Valid("alice", "") {
		t.Error("Valid does not match alice's own fingerprint exactly")
	}

	// After a restart: alice's password changed and bob was removed.
	after, err := ParsePasswordAccounts("alice:" + hashFor(t, "one-changed") + ",carol:" + hashFor(t, "three"))
	if err != nil {
		t.Fatal(err)
	}
	if after.Valid("alice", fpA) {
		t.Error("alice's sign-in is still valid after her password changed")
	}
	if after.Valid("bob", fpB) {
		t.Error("bob's sign-in is still valid after he was removed")
	}
	var none *PasswordAccounts
	if none.Valid("alice", fpA) || none.Len() != 0 {
		t.Error("no configured accounts still validate a password sign-in")
	}
	if _, ok := none.Check("alice", "one"); ok {
		t.Error("no configured accounts still sign alice in")
	}
}
