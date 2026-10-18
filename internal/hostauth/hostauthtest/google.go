// Package hostauthtest is test support for the hosted playmat's sign-in:
// an in-process fake Google and helpers that sign a test client in through
// the real sign-in routes. Production code never imports it.
package hostauthtest

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// Account is a Google account the fake signs in.
type Account struct {
	ID, Email string
}

// DefaultAccount is the account the fake signs in until SetAccount.
var DefaultAccount = Account{ID: "sub-1", Email: "one@example.com"}

// FakeGoogle is an in-process stand-in for Google's OAuth endpoints. Its
// authorization endpoint signs the current account in at once, without a
// page; its token endpoint checks the client, redirect and PKCE verifier
// and answers with an ID token (unsigned, as the playmat does not check
// signatures).
type FakeGoogle struct {
	*httptest.Server
	// ClientID and ClientSecret are the client the fake accepts.
	ClientID, ClientSecret string
	// Claims, if set, may change an ID token's claims before it is issued,
	// for tests of bad tokens.
	Claims func(claims map[string]any)

	mu      sync.Mutex
	account Account
	codes   map[string]fakeCode
	// Prompts records the prompt parameter of every authorization request.
	prompts []string
}

type fakeCode struct {
	clientID, redirect, nonce, challenge string
	account                              Account
}

// NewGoogle starts a fake Google for the rest of the test.
func NewGoogle(t testing.TB) *FakeGoogle {
	t.Helper()
	g := &FakeGoogle{
		ClientID:     "test-client.apps.googleusercontent.com",
		ClientSecret: "test-client-secret",
		account:      DefaultAccount,
		codes:        map[string]fakeCode{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /authorize", g.authorize)
	mux.HandleFunc("POST /token", g.token)
	g.Server = httptest.NewServer(mux)
	t.Cleanup(g.Close)
	return g
}

// Endpoint is the fake's OAuth endpoints, for hostauth.Config.Google.
func (g *FakeGoogle) Endpoint() oauth2.Endpoint {
	return oauth2.Endpoint{AuthURL: g.URL + "/authorize", TokenURL: g.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}
}

// SetAccount switches the account the next sign-in signs in.
func (g *FakeGoogle) SetAccount(a Account) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.account = a
}

// Prompts returns the prompt parameter of every authorization request so
// far, in order.
func (g *FakeGoogle) Prompts() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.prompts...)
}

func (g *FakeGoogle) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("client_id") != g.ClientID {
		http.Error(w, "unknown client", http.StatusBadRequest)
		return
	}
	if q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil || redirect.Scheme == "" {
		http.Error(w, "bad redirect_uri", http.StatusBadRequest)
		return
	}
	code := rand.Text()
	g.mu.Lock()
	g.prompts = append(g.prompts, q.Get("prompt"))
	g.codes[code] = fakeCode{
		clientID:  g.ClientID,
		redirect:  redirect.String(),
		nonce:     q.Get("nonce"),
		challenge: q.Get("code_challenge"),
		account:   g.account,
	}
	g.mu.Unlock()
	v := redirect.Query()
	v.Set("code", code)
	v.Set("state", q.Get("state"))
	redirect.RawQuery = v.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (g *FakeGoogle) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		tokenError(w, "invalid_request")
		return
	}
	f := r.PostForm
	if f.Get("client_id") != g.ClientID || f.Get("client_secret") != g.ClientSecret {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client"})
		return
	}
	g.mu.Lock()
	c, ok := g.codes[f.Get("code")]
	delete(g.codes, f.Get("code"))
	g.mu.Unlock()
	if f.Get("grant_type") != "authorization_code" || !ok || c.redirect != f.Get("redirect_uri") || challenge(f.Get("code_verifier")) != c.challenge {
		tokenError(w, "invalid_grant")
		return
	}
	now := time.Now()
	claims := map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            g.ClientID,
		"sub":            c.account.ID,
		"email":          c.account.Email,
		"email_verified": true,
		"nonce":          c.nonce,
		"iat":            now.Unix(),
		"exp":            now.Add(time.Hour).Unix(),
	}
	if g.Claims != nil {
		g.Claims(claims)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token": "fake-google-access-token",
		"token_type":   "Bearer",
		"expires_in":   3600,
		"id_token":     unsignedJWT(claims),
	})
}

func tokenError(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func unsignedJWT(claims map[string]any) string {
	head, _ := json.Marshal(map[string]string{"alg": "none", "typ": "JWT"})
	body, _ := json.Marshal(claims)
	enc := base64.RawURLEncoding.EncodeToString
	return enc(head) + "." + enc(body) + ".c2ln"
}

// Browser returns an HTTP client that acts like a user's browser against
// the playmat served at server and reached at public (the two are often
// the same). Requests to public's host go to server; it keeps cookies; and
// it follows redirects to public and to other test servers (loopback with
// a port, such as the fake Google), stopping at any other address — an MCP
// client's loopback callback, claude.ai — and returning that redirect.
func Browser(t testing.TB, public, server string) *http.Client {
	t.Helper()
	pu, err := url.Parse(public)
	if err != nil {
		t.Fatal(err)
	}
	su, err := url.Parse(server)
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar:       jar,
		Transport: &alias{from: pu.Host, to: su.Host},
		Timeout:   10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			if req.URL.Host == pu.Host || isTestServer(req.URL) {
				return nil
			}
			return http.ErrUseLastResponse
		},
	}
}

// isTestServer reports whether u is a loopback address with an explicit
// port, as httptest servers are.
func isTestServer(u *url.URL) bool {
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || port == "" {
		return false
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// alias sends requests for one host to another over plain HTTP, so a test
// can use a public https address served by a local test server.
type alias struct{ from, to string }

func (a *alias) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == a.from && a.from != a.to {
		r = r.Clone(r.Context())
		r.URL.Scheme = "http"
		r.URL.Host = a.to
		r.Host = a.from
	}
	return http.DefaultTransport.RoundTrip(r)
}
