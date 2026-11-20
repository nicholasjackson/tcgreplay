package hostauth

import (
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// TestTokensCarryVerifiedEmail: access and refresh tokens issued for an
// account Google verified open as verified, also on a new instance with
// the same key (a restart), and tokens for an unverified account open as
// unverified.
func TestTokensCarryVerifiedEmail(t *testing.T) {
	seal, err := newSealer(testSigningKey)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, verified := range []bool{true, false} {
		acct := Account{ID: "sub-r", Email: "reviewer@example.com", EmailVerified: verified}
		s := newStore(seal, time.Now)
		access, refresh := s.issueTokens(now, "c", acct)
		if got := s.accessToken(access); got == nil || got.account != acct {
			t.Errorf("verified %v: access token opens as %+v, want %+v", verified, got, acct)
		}
		if got, ok := s.takeRefresh("c", refresh); !ok || got != acct {
			t.Errorf("verified %v: refresh token gives %+v %v, want %+v", verified, got, ok, acct)
		}

		// After a restart neither token is in memory: the sealed claims alone.
		access, refresh = s.issueTokens(now, "c", acct)
		restarted := newStore(seal, time.Now)
		if got := restarted.accessToken(access); got == nil || got.account != acct {
			t.Errorf("verified %v: access token after a restart opens as %+v, want %+v", verified, got, acct)
		}
		if got, ok := restarted.takeRefresh("c", refresh); !ok || got != acct {
			t.Errorf("verified %v: refresh token after a restart gives %+v %v, want %+v", verified, got, ok, acct)
		}
	}
}

// TestTokenWithoutVerifiedClaimIsUnverified: a token sealed before the
// verified flag was kept (no "v" claim) still works, as unverified.
func TestTokenWithoutVerifiedClaimIsUnverified(t *testing.T) {
	seal, err := newSealer(testSigningKey)
	if err != nil {
		t.Fatal(err)
	}
	s := newStore(seal, time.Now)
	old := seal.seal(sealAccess, struct {
		Client string `json:"c"`
		Sub    string `json:"u"`
		Email  string `json:"e"`
		Exp    int64  `json:"x"`
	}{clientKey("c"), "sub-r", "reviewer@example.com", time.Now().Add(time.Hour).Unix()})
	got := s.accessToken(old)
	if got == nil {
		t.Fatal("a token without the verified claim no longer opens")
	}
	if want := (Account{ID: "sub-r", Email: "reviewer@example.com"}); got.account != want {
		t.Errorf("old token opens as %+v, want %+v (unverified)", got.account, want)
	}
}

// TestAccountFromTokenVerified: the account an MCP request acts for
// carries the verified flag the token information holds, and is
// unverified when it holds none.
func TestAccountFromTokenVerified(t *testing.T) {
	for _, tt := range []struct {
		name  string
		extra map[string]any
		want  bool
	}{
		{"verified", map[string]any{"email": "r@example.com", "verified": true}, true},
		{"unverified", map[string]any{"email": "r@example.com", "verified": false}, false},
		{"no flag", map[string]any{"email": "r@example.com"}, false},
	} {
		acct, ok := AccountFromToken(&auth.TokenInfo{UserID: "sub-r", Extra: tt.extra})
		if !ok || acct.ID != "sub-r" || acct.Email != "r@example.com" || acct.EmailVerified != tt.want {
			t.Errorf("%s: AccountFromToken = %+v %v, want sub-r r@example.com verified %v", tt.name, acct, ok, tt.want)
		}
	}
}
