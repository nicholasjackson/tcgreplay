package usergames

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tokenExtra is the request extra a signed-in tool call carries.
func tokenExtra(userID, email string) *mcp.RequestExtra {
	return &mcp.RequestExtra{TokenInfo: &auth.TokenInfo{UserID: userID, Extra: map[string]any{"email": email}}}
}

func TestResolveMCPFindsTheCallersGame(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := newRegistry(t, t.TempDir(), clock.Now)

	// Create the entry first without activity, so the call's own activity
	// shows as a later LastActive.
	a := r.Get(alice)
	clock.Advance(7 * time.Minute)

	m, link, err := r.ResolveMCP(context.Background(), tokenExtra(alice.ID, alice.Email))
	if err != nil {
		t.Fatalf("ResolveMCP: %v", err)
	}
	if m != a.M {
		t.Error("ResolveMCP returned a manager other than the account's own")
	}
	if want := "https://playmat.example.com/?board=" + Handle(alice.ID); link != want {
		t.Errorf("link = %q, want %q", link, want)
	}
	if got := r.LastActive(a); !got.Equal(clock.t) {
		t.Errorf("after a tool call, LastActive = %v, want %v", got, clock.t)
	}

	mb, linkB, err := r.ResolveMCP(context.Background(), tokenExtra(bob.ID, bob.Email))
	if err != nil {
		t.Fatalf("ResolveMCP(bob): %v", err)
	}
	if mb == m {
		t.Error("two accounts resolved to the same manager")
	}
	if linkB == link {
		t.Errorf("two accounts resolved to the same board link %q", link)
	}
	if mb != r.Get(bob).M {
		t.Error("bob's call did not resolve to bob's own manager")
	}
}

func TestResolveMCPRequiresSignIn(t *testing.T) {
	r := newRegistry(t, t.TempDir(), nil)
	for name, extra := range map[string]*mcp.RequestExtra{
		"nil extra":     nil,
		"no token":      {},
		"empty user id": {TokenInfo: &auth.TokenInfo{Extra: map[string]any{"email": "a@example.com"}}},
	} {
		m, link, err := r.ResolveMCP(context.Background(), extra)
		if err == nil {
			t.Errorf("%s: no error", name)
		}
		if m != nil || link != "" {
			t.Errorf("%s: got manager %v and link %q with the error", name, m, link)
		}
	}
}

// TestResolveMCPQuietIsNotActivity: the wait's quiet lookup finds the
// caller's own manager and board link without moving its last activity or
// taking a place, while ResolveMCP still counts as activity.
func TestResolveMCPQuietIsNotActivity(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)}
	r := newRegistry(t, t.TempDir(), clock.Now)

	a := r.Get(alice)
	created := r.LastActive(a)
	clock.Advance(7 * time.Minute)
	m, link, err := r.ResolveMCPQuiet(context.Background(), tokenExtra(alice.ID, alice.Email))
	if err != nil {
		t.Fatalf("ResolveMCPQuiet: %v", err)
	}
	if m != a.M {
		t.Error("ResolveMCPQuiet returned a manager other than the account's own")
	}
	if want := "https://playmat.example.com/?board=" + Handle(alice.ID); link != want {
		t.Errorf("link = %q, want %q", link, want)
	}
	if got := r.LastActive(a); !got.Equal(created) {
		t.Errorf("after a quiet lookup, LastActive = %v, want it unchanged at %v", got, created)
	}
	r.mu.Lock()
	holds := a.holds
	r.mu.Unlock()
	if holds {
		t.Error("a quiet lookup took a place")
	}

	// Any other tool call is activity, as before.
	clock.Advance(time.Minute)
	if _, _, err := r.ResolveMCP(context.Background(), tokenExtra(alice.ID, alice.Email)); err != nil {
		t.Fatal(err)
	}
	if got := r.LastActive(a); !got.Equal(clock.t) {
		t.Errorf("after a tool call, LastActive = %v, want %v", got, clock.t)
	}

	if _, _, err := r.ResolveMCPQuiet(context.Background(), nil); err == nil {
		t.Error("quiet lookup without sign-in: no error")
	}
}
