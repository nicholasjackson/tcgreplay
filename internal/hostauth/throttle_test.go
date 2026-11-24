package hostauth

import (
	"net/http/httptest"
	"testing"
	"time"
)

// TestSlowAllowance shows an allowance of 10 over 15 minutes refuses the
// 11th spend inside the window, says when the next is allowed, and allows
// spending again as time passes (one more every 90 seconds).
func TestSlowAllowance(t *testing.T) {
	s := newStore(nil, nil)
	now := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	for i := 1; i <= 10; i++ {
		if ok, _, _ := s.take(now, "login-user:alice", 10, 15*time.Minute); !ok {
			t.Fatalf("spend %d of 10 refused", i)
		}
	}
	ok, wait, _ := s.take(now.Add(time.Minute), "login-user:alice", 10, 15*time.Minute)
	if ok {
		t.Fatal("the 11th spend inside 15 minutes was allowed")
	}
	if wait <= 0 || wait > 90*time.Second {
		t.Errorf("wait %v, want up to 90s", wait)
	}
	if ok, _, _ := s.take(now.Add(time.Minute+90*time.Second), "login-user:alice", 10, 15*time.Minute); !ok {
		t.Error("no spend allowed 90 seconds later")
	}
	if ok, _, _ := s.take(now.Add(time.Minute+91*time.Second), "login-user:alice", 10, 15*time.Minute); ok {
		t.Error("a second spend allowed straight after the refilled one")
	}
	// After a whole window untouched it is full again.
	later := now.Add(time.Hour)
	for i := 1; i <= 10; i++ {
		if ok, _, _ := s.take(later, "login-user:alice", 10, 15*time.Minute); !ok {
			t.Fatalf("an hour later, spend %d of 10 refused", i)
		}
	}
}

// TestPeekNeverSpends shows checking an allowance does not use it up, and
// reports a used-up one as refused with its wait.
func TestPeekNeverSpends(t *testing.T) {
	s := newStore(nil, nil)
	now := time.Now()
	for range 100 {
		if ok, _ := s.peek(now, "login-client:192.0.2.1", 3, 15*time.Minute); !ok {
			t.Fatal("peek refused a fresh allowance")
		}
	}
	for i := range 3 {
		if ok, _, _ := s.take(now, "login-client:192.0.2.1", 3, 15*time.Minute); !ok {
			t.Fatalf("spend %d of 3 refused after peeking", i+1)
		}
	}
	if ok, wait := s.peek(now, "login-client:192.0.2.1", 3, 15*time.Minute); ok || wait <= 0 {
		t.Errorf("peek of a used-up allowance = %v, %v; want refused with a wait", ok, wait)
	}
}

// TestAllowancesAreIndependent shows allowances under different keys do
// not share, and the request limit's keys stay apart from the sign-in
// ones.
func TestAllowancesAreIndependent(t *testing.T) {
	s := newStore(nil, nil)
	now := time.Now()
	for range 2 {
		s.take(now, "login-user:alice", 2, 15*time.Minute)
	}
	if ok, _, _ := s.take(now, "login-user:alice", 2, 15*time.Minute); ok {
		t.Fatal("alice's used-up allowance allowed another")
	}
	for _, k := range []string{"login-user:bob", "login-client:alice", "acct:alice"} {
		if ok, _, _ := s.take(now, k, 2, 15*time.Minute); !ok {
			t.Errorf("%s was refused because alice's allowance is used up", k)
		}
	}
	uk, ck := loginKeys(" Alice@Example.com ", "192.0.2.7")
	if uk != "login-user:alice@example.com" || ck != "login-client:192.0.2.7" {
		t.Errorf("loginKeys = %q, %q", uk, ck)
	}
}

// TestSweepKeepsSlowAllowances shows a 15-minute allowance survives the
// sweep past one minute, and is forgotten once its whole window passes;
// a per-minute one is forgotten after a minute, as before.
func TestSweepKeepsSlowAllowances(t *testing.T) {
	s := newStore(nil, nil)
	now := time.Now()
	s.take(now, "login-user:alice", 10, 15*time.Minute)
	s.take(now, "acct:alice", 60, time.Minute)
	s.mu.Lock()
	s.sweep(now.Add(2 * time.Minute))
	_, slow := s.buckets["login-user:alice"]
	_, fast := s.buckets["acct:alice"]
	s.mu.Unlock()
	if !slow {
		t.Error("the 15-minute allowance was forgotten after 2 minutes")
	}
	if fast {
		t.Error("the per-minute allowance was kept after 2 minutes")
	}
	s.mu.Lock()
	s.sweep(now.Add(16 * time.Minute))
	_, slow = s.buckets["login-user:alice"]
	s.mu.Unlock()
	if slow {
		t.Error("the 15-minute allowance was kept after its window")
	}
}

// TestLoginThrottleHelpers shows the sign-in throttle checks both the
// username's and the client's allowance without spending, and a failure
// spends from both.
func TestLoginThrottleHelpers(t *testing.T) {
	h := newHarness(t, nil)
	now := time.Now()
	for range loginUserCap {
		if ok, _ := h.a.loginAllowed(now, "alice", "192.0.2.1"); !ok {
			t.Fatal("refused before the username's allowance was used up")
		}
		h.a.loginFailed(now, "alice", "192.0.2.1")
	}
	if ok, wait := h.a.loginAllowed(now, "ALICE", "192.0.2.9"); ok || wait <= 0 {
		t.Error("alice's used-up allowance did not refuse her from another client")
	}
	if ok, _ := h.a.loginAllowed(now, "bob", "192.0.2.9"); !ok {
		t.Error("bob was refused from a fresh client")
	}
	// The client 192.0.2.1 has spent 10 of its 30.
	for i := range loginClientCap - loginUserCap {
		if ok, _ := h.a.loginAllowed(now, "user"+string(rune('a'+i)), "192.0.2.1"); !ok {
			t.Fatalf("client refused at failure %d", loginUserCap+i+1)
		}
		h.a.loginFailed(now, "user"+string(rune('a'+i)), "192.0.2.1")
	}
	if ok, _ := h.a.loginAllowed(now, "fresh-user", "192.0.2.1"); ok {
		t.Error("a client with 30 failures across usernames was not refused")
	}
}

// TestClientAddr shows the client address comes from the edge's headers,
// in order, and falls back to the connection's peer, IPv6 included.
func TestClientAddr(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		remote  string
		want    string
	}{
		{"peer only", nil, "198.51.100.4:5555", "198.51.100.4"},
		{"do-connecting-ip first", map[string]string{"do-connecting-ip": "203.0.113.9", "CF-Connecting-IP": "203.0.113.1"}, "10.0.0.1:1", "203.0.113.9"},
		{"CF-Connecting-IP next", map[string]string{"CF-Connecting-IP": " 203.0.113.1 "}, "10.0.0.1:1", "203.0.113.1"},
		{"bad header ignored", map[string]string{"do-connecting-ip": "not-an-ip"}, "10.0.0.2:1", "10.0.0.2"},
		{"IPv6 header", map[string]string{"do-connecting-ip": "2001:db8::1"}, "10.0.0.1:1", "2001:db8::1"},
		{"IPv6 peer", nil, "[2001:db8::7]:443", "2001:db8::7"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/auth/password", nil)
		r.RemoteAddr = c.remote
		for k, v := range c.headers {
			r.Header.Set(k, v)
		}
		if got := clientAddr(r); got != c.want {
			t.Errorf("%s: clientAddr = %q, want %q", c.name, got, c.want)
		}
	}
}
