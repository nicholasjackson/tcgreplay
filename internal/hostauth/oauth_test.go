package hostauth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth/hostauthtest"
)

// testVerifier is the PKCE verifier the hand-driven connector sign-ins use.
const testVerifier = "oauth-test-verifier-0123456789-0123456789-0123456789"

// fakeClock is an injected clock a test moves forward by hand.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// swapHandler serves whichever handler was set last, so a test can restart
// the sign-in service behind the same address.
type swapHandler struct {
	mu sync.RWMutex
	h  http.Handler
}

func (s *swapHandler) set(h http.Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.h = h
}

func (s *swapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	h := s.h
	s.mu.RUnlock()
	h.ServeHTTP(w, r)
}

// connector is a harness that also serves an MCP endpoint guarded by
// RequireToken, runs on an injected clock, and can be restarted.
type connector struct {
	*harness
	clock *fakeClock
	sw    *swapHandler
	srv   *mcp.Server
}

// whoami is what the test MCP tool answers: the account its call acts for.
type whoami struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// newWhoamiServer returns an MCP server with one tool, whoami, that reports
// the signed-in account the call's token is for.
func newWhoamiServer() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "hostauth-test", Version: "1"}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "whoami", Description: "report the signed-in account"},
		func(ctx context.Context, req *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, whoami, error) {
			acct, ok := AccountFromToken(req.Extra.TokenInfo)
			if !ok {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "no account"}}}, whoami{}, nil
			}
			return nil, whoami{ID: acct.ID, Email: acct.Email}, nil
		})
	return srv
}

// newConnector serves a sign-in service and MCP endpoint at a test
// server's own address, with a fake Google and an injected clock.
func newConnector(t *testing.T) *connector {
	t.Helper()
	g := hostauthtest.NewGoogle(t)
	sw := &swapHandler{h: http.NotFoundHandler()}
	ts := httptest.NewUnstartedServer(sw)
	ts.Start()
	t.Cleanup(ts.Close)
	pub, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{t: time.Now()}
	cfg := Config{
		Public:         pub,
		GoogleClientID: g.ClientID,
		GoogleSecret:   g.ClientSecret,
		Google:         g.Endpoint(),
		Now:            clock.Now,
	}
	c := &connector{harness: &harness{ts: ts, g: g, cfg: cfg}, clock: clock, sw: sw, srv: newWhoamiServer()}
	c.restart(t)
	return c
}

// restart builds a new sign-in service from the same config, as a
// redeploy does, and serves it at the same address.
func (c *connector) restart(t *testing.T) {
	t.Helper()
	a, err := New(c.cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mux := http.NewServeMux()
	a.Routes(mux)
	mux.Handle("/mcp", a.RequireToken(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return c.srv }, nil)))
	mux.HandleFunc("GET /start", func(w http.ResponseWriter, r *http.Request) {
		a.startGoogle(w, r, &pendingLogin{kind: loginBoard, next: boardNext})
	})
	mux.HandleFunc("GET /board", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "OK")
	})
	c.a = a
	c.sw.set(mux)
}

// readJSON reads resp's body as a JSON object and closes it.
func readJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("body %q is not a JSON object: %v", b, err)
	}
	return m
}

// register posts client metadata to /register and returns the response and
// its JSON body.
func (c *connector) register(t *testing.T, meta any) (*http.Response, map[string]any) {
	t.Helper()
	body, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(c.ts.URL+"/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp, readJSON(t, resp)
}

// registerClient registers a client with the given auth method and
// redirects and returns its ID and secret.
func (c *connector) registerClient(t *testing.T, method string, redirects ...string) (id, secret string) {
	t.Helper()
	resp, m := c.register(t, map[string]any{"client_name": "test", "redirect_uris": redirects, "token_endpoint_auth_method": method})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: status %d %v, want 201", resp.StatusCode, m)
	}
	id, _ = m["client_id"].(string)
	secret, _ = m["client_secret"].(string)
	if id == "" {
		t.Fatalf("register: no client_id in %v", m)
	}
	return id, secret
}

// authorizeURL is a valid authorization request for clientID, with mod
// applied to its parameters.
func (c *connector) authorizeURL(clientID, redirect string, mod func(url.Values)) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirect},
		"state":                 {"st"},
		"code_challenge":        {s256(testVerifier)},
		"code_challenge_method": {"S256"},
		"resource":              {c.ts.URL + "/mcp"},
	}
	if mod != nil {
		mod(q)
	}
	return c.ts.URL + "/authorize?" + q.Encode()
}

// code signs g's current account in for clientID through a fresh browser
// and returns the code the playmat sends back to redirect.
func (c *connector) code(t *testing.T, clientID, redirect string) string {
	t.Helper()
	loc, err := hostauthtest.FollowSignIn(c.browser(t), c.authorizeURL(clientID, redirect, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := loc.Scheme + "://" + loc.Host + loc.Path; got != redirect {
		t.Fatalf("authorize returned to %s, want %s", got, redirect)
	}
	q := loc.Query()
	if q.Get("code") == "" || q.Get("state") != "st" {
		t.Fatalf("authorize returned %s, want a code and state st", loc)
	}
	return q.Get("code")
}

// token posts form to /token, with Basic credentials if user is set, and
// returns the response and its JSON body.
func (c *connector) token(t *testing.T, form url.Values, user, pass string) (*http.Response, map[string]any) {
	t.Helper()
	req, err := http.NewRequest("POST", c.ts.URL+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if user != "" {
		req.SetBasicAuth(url.QueryEscape(user), url.QueryEscape(pass))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp, readJSON(t, resp)
}

// exchange is the form that exchanges code for a public client's tokens.
func exchange(clientID, code, redirect, verifier string) url.Values {
	return url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirect},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}
}

// refreshForm is the form that refreshes a public client's tokens.
func refreshForm(clientID, refresh string) url.Values {
	return url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {clientID}}
}

// checkOAuthError checks resp is a JSON OAuth error with status and code.
func checkOAuthError(t *testing.T, resp *http.Response, m map[string]any, status int, code string) {
	t.Helper()
	if resp.StatusCode != status || m["error"] != code {
		t.Errorf("got %d %v, want %d error %s", resp.StatusCode, m, status, code)
	}
	if _, ok := m["access_token"]; ok {
		t.Errorf("error response carries an access token: %v", m)
	}
}

// initializeBody is an MCP initialize request.
const initializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

// callMCP sends an MCP initialize request with the bearer token (none if
// empty) and returns the response, body closed.
func (c *connector) callMCP(t *testing.T, token string, mod func(*http.Request)) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", c.ts.URL+"/mcp", strings.NewReader(initializeBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if mod != nil {
		mod(req)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp
}

// wantChallenge is the WWW-Authenticate header a refused MCP request gets.
func (c *connector) wantChallenge() string {
	return `Bearer resource_metadata="` + c.ts.URL + `/.well-known/oauth-protected-resource/mcp", scope="playmat"`
}

// checkMCPAccepted checks the access token gets through to the MCP server.
func (c *connector) checkMCPAccepted(t *testing.T, token string) {
	t.Helper()
	if resp := c.callMCP(t, token, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("MCP request with the token: status %d, want 200", resp.StatusCode)
	}
}

// checkMCPRefused checks the access token is refused with the sign-in
// challenge.
func (c *connector) checkMCPRefused(t *testing.T, token string) {
	t.Helper()
	resp := c.callMCP(t, token, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("MCP request with the token: status %d, want 401", resp.StatusCode)
	}
	if got := resp.Header.Get("WWW-Authenticate"); got != c.wantChallenge() {
		t.Errorf("WWW-Authenticate = %q, want %q", got, c.wantChallenge())
	}
}

// getAsEvil fetches path from the playmat as if addressed to host evil.example,
// and returns the response and its body.
func (c *connector) getAsEvil(t *testing.T, path string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest("GET", c.ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "evil.example"
	req.Header.Set("X-Forwarded-Host", "evil.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Forwarded", "host=evil.example;proto=https")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(b)
}

// TestConnectorAnonymousRefused shows an MCP request without a live access
// token is refused with 401 and a pointer to the protected resource
// metadata, that the metadata names the connector address and this server
// as its sign-in service, and that the authorization server metadata
// describes the endpoints, PKCE S256 and nothing it does not support. All
// of it is built from the configured public address, not the request's
// host.
func TestConnectorAnonymousRefused(t *testing.T) {
	c := newConnector(t)

	for name, token := range map[string]string{"no token": "", "unknown token": "not-a-token"} {
		t.Run(name, func(t *testing.T) { c.checkMCPRefused(t, token) })
	}
	// A request addressed to another host is pointed at the public address.
	resp := c.callMCP(t, "", func(r *http.Request) {
		r.Host = "evil.example"
		r.Header.Set("X-Forwarded-Host", "evil.example")
	})
	if got := resp.Header.Get("WWW-Authenticate"); resp.StatusCode != http.StatusUnauthorized || got != c.wantChallenge() {
		t.Errorf("request for host evil.example: %d %q, want 401 %q", resp.StatusCode, got, c.wantChallenge())
	}

	for _, path := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource"} {
		t.Run(path, func(t *testing.T) {
			resp, body := c.getAsEvil(t, path)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status %d, want 200", resp.StatusCode)
			}
			if strings.Contains(body, "evil.example") {
				t.Errorf("metadata names the request's host: %s", body)
			}
			var m struct {
				Resource             string   `json:"resource"`
				AuthorizationServers []string `json:"authorization_servers"`
				ScopesSupported      []string `json:"scopes_supported"`
			}
			if err := json.Unmarshal([]byte(body), &m); err != nil {
				t.Fatal(err)
			}
			if m.Resource != c.ts.URL+"/mcp" {
				t.Errorf("resource = %q, want %q", m.Resource, c.ts.URL+"/mcp")
			}
			if len(m.AuthorizationServers) != 1 || m.AuthorizationServers[0] != c.ts.URL {
				t.Errorf("authorization_servers = %q, want [%s]", m.AuthorizationServers, c.ts.URL)
			}
			if len(m.ScopesSupported) != 1 || m.ScopesSupported[0] != "playmat" {
				t.Errorf("scopes_supported = %q, want [playmat]", m.ScopesSupported)
			}
		})
	}

	for _, path := range []string{"/.well-known/oauth-authorization-server", "/.well-known/openid-configuration"} {
		t.Run(path, func(t *testing.T) {
			resp, body := c.getAsEvil(t, path)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status %d, want 200", resp.StatusCode)
			}
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
				t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
			}
			if strings.Contains(body, "evil.example") {
				t.Errorf("metadata names the request's host: %s", body)
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(body), &m); err != nil {
				t.Fatal(err)
			}
			for k, want := range map[string]string{
				"issuer":                 c.ts.URL,
				"authorization_endpoint": c.ts.URL + "/authorize",
				"token_endpoint":         c.ts.URL + "/token",
				"registration_endpoint":  c.ts.URL + "/register",
			} {
				if m[k] != want {
					t.Errorf("%s = %v, want %s", k, m[k], want)
				}
			}
			lists := map[string][]string{
				"code_challenge_methods_supported":      {"S256"},
				"response_types_supported":              {"code"},
				"grant_types_supported":                 {"authorization_code", "refresh_token"},
				"token_endpoint_auth_methods_supported": {"none", "client_secret_post", "client_secret_basic"},
				"scopes_supported":                      {"playmat"},
			}
			for k, want := range lists {
				got, _ := json.Marshal(m[k])
				w, _ := json.Marshal(want)
				if string(got) != string(w) {
					t.Errorf("%s = %s, want %s", k, got, w)
				}
			}
			for _, k := range []string{"jwks_uri", "client_id_metadata_document_supported"} {
				if _, ok := m[k]; ok {
					t.Errorf("metadata has %s: %v", k, m[k])
				}
			}
			if strings.Contains(body, "offline_access") {
				t.Errorf("metadata offers offline_access: %s", body)
			}
		})
	}
}

// TestRegisterRedirects shows a client may register Claude's callback
// addresses or a loopback address on any port, and is refused with
// invalid_redirect_uri for any other address or none; a public client gets
// no secret.
func TestRegisterRedirects(t *testing.T) {
	c := newConnector(t)
	for _, tt := range []struct {
		redirects []string
		ok        bool
	}{
		{[]string{"https://claude.ai/api/mcp/auth_callback"}, true},
		{[]string{"https://claude.com/api/mcp/auth_callback"}, true},
		{[]string{"https://claude.ai/api/mcp/auth_callback", "https://claude.com/api/mcp/auth_callback"}, true},
		{[]string{"http://localhost:1234/cb"}, true},
		{[]string{"http://127.0.0.1:5555/callback"}, true},
		{[]string{"http://[::1]:9/x"}, true},
		{[]string{"http://127.0.0.1/callback"}, true},
		{[]string{"https://evil.example/cb"}, false},
		{[]string{"http://evil.example/cb"}, false},
		{[]string{"https://claude.ai/other"}, false},
		{[]string{"https://claude.ai/api/mcp/auth_callback?x=1"}, false},
		{[]string{"http://claude.ai/api/mcp/auth_callback"}, false},
		{[]string{"https://localhost/cb"}, false},
		{[]string{"http://localhost.evil.example/cb"}, false},
		{[]string{"http://127.0.0.1:5555/cb#frag"}, false},
		{[]string{"http://user@127.0.0.1/cb"}, false},
		{[]string{"javascript:alert(1)"}, false},
		{[]string{"https://claude.ai/api/mcp/auth_callback", "https://evil.example/cb"}, false},
		{[]string{}, false},
		{nil, false},
	} {
		name := strings.Join(tt.redirects, " ")
		if name == "" {
			name = "none"
		}
		t.Run(name, func(t *testing.T) {
			meta := map[string]any{"client_name": "test"}
			if tt.redirects != nil {
				meta["redirect_uris"] = tt.redirects
			}
			resp, m := c.register(t, meta)
			if !tt.ok {
				if resp.StatusCode != http.StatusBadRequest || m["error"] != "invalid_redirect_uri" {
					t.Errorf("got %d %v, want 400 invalid_redirect_uri", resp.StatusCode, m)
				}
				if _, ok := m["client_id"]; ok {
					t.Errorf("refused registration has a client_id")
				}
				return
			}
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("got %d %v, want 201", resp.StatusCode, m)
			}
			id, _ := m["client_id"].(string)
			if id == "" || c.a.st.client(id) == nil {
				t.Errorf("client_id %q is not registered", id)
			}
			if m["token_endpoint_auth_method"] != "none" {
				t.Errorf("token_endpoint_auth_method = %v, want none", m["token_endpoint_auth_method"])
			}
			if _, ok := m["client_secret"]; ok {
				t.Errorf("public client got a secret")
			}
			if got := resp.Header.Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
		})
	}

	t.Run("not JSON", func(t *testing.T) {
		resp, err := http.Post(c.ts.URL+"/register", "application/json", strings.NewReader("redirect_uris=x"))
		if err != nil {
			t.Fatal(err)
		}
		if m := readJSON(t, resp); resp.StatusCode != http.StatusBadRequest || m["error"] != "invalid_client_metadata" {
			t.Errorf("got %d %v, want 400 invalid_client_metadata", resp.StatusCode, m)
		}
	})
	t.Run("unsupported auth method", func(t *testing.T) {
		resp, m := c.register(t, map[string]any{"redirect_uris": []string{hostauthtest.LoopbackCallback}, "token_endpoint_auth_method": "private_key_jwt"})
		if resp.StatusCode != http.StatusBadRequest || m["error"] != "invalid_client_metadata" {
			t.Errorf("got %d %v, want 400 invalid_client_metadata", resp.StatusCode, m)
		}
	})
}

// TestConfidentialClient shows a client registered with a secret-based
// method gets a secret and can exchange its code authenticating with Basic
// credentials or with the secret in the form, and that a wrong or missing
// secret, or an unknown client, is refused with 401 invalid_client.
func TestConfidentialClient(t *testing.T) {
	for _, method := range []string{"client_secret_basic", "client_secret_post"} {
		t.Run(method, func(t *testing.T) {
			c := newConnector(t)
			id, secret := c.registerClient(t, method, hostauthtest.LoopbackCallback)
			if secret == "" {
				t.Fatal("confidential client got no secret")
			}

			// auth sends form with the credentials as method says.
			auth := func(form url.Values, secret string) (*http.Response, map[string]any) {
				if method == "client_secret_basic" {
					return c.token(t, form, id, secret)
				}
				form.Set("client_id", id)
				form.Set("client_secret", secret)
				return c.token(t, form, "", "")
			}
			form := func(code string) url.Values {
				f := exchange("", code, hostauthtest.LoopbackCallback, testVerifier)
				f.Del("client_id")
				return f
			}

			// A wrong secret, or none, is refused and leaves the code unused.
			code := c.code(t, id, hostauthtest.LoopbackCallback)
			resp, m := auth(form(code), secret+"x")
			checkOAuthError(t, resp, m, http.StatusUnauthorized, "invalid_client")
			if resp.Header.Get("WWW-Authenticate") == "" {
				t.Error("401 invalid_client has no WWW-Authenticate")
			}
			if method == "client_secret_basic" {
				resp, m = c.token(t, form(code), id, "")
			} else {
				f := form(code)
				f.Set("client_id", id)
				resp, m = c.token(t, f, "", "")
			}
			checkOAuthError(t, resp, m, http.StatusUnauthorized, "invalid_client")

			resp, m = auth(form(code), secret)
			if resp.StatusCode != http.StatusOK || m["access_token"] == nil {
				t.Fatalf("exchange with the right secret: %d %v, want tokens", resp.StatusCode, m)
			}
			c.checkMCPAccepted(t, m["access_token"].(string))

			// Refreshing needs the secret too.
			refresh := m["refresh_token"].(string)
			resp, m = auth(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}}, "nope")
			checkOAuthError(t, resp, m, http.StatusUnauthorized, "invalid_client")
			resp, m = auth(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}}, secret)
			if resp.StatusCode != http.StatusOK || m["access_token"] == nil {
				t.Errorf("refresh with the right secret: %d %v, want tokens", resp.StatusCode, m)
			}
		})
	}

	t.Run("unknown client", func(t *testing.T) {
		c := newConnector(t)
		resp, m := c.token(t, exchange("no-such-client", "x", hostauthtest.LoopbackCallback, testVerifier), "", "")
		checkOAuthError(t, resp, m, http.StatusUnauthorized, "invalid_client")
		resp, m = c.token(t, url.Values{"grant_type": {"authorization_code"}}, "no-such-client", "secret")
		checkOAuthError(t, resp, m, http.StatusUnauthorized, "invalid_client")
	})
}

// TestAuthorizeRefused shows an authorization request from an unknown
// client or for an unregistered address is refused with a page in the
// browser and no redirect, and that one without PKCE S256, with another
// response type or for another resource is sent back to the client with
// the OAuth error and its state; none of them reaches Google.
func TestAuthorizeRefused(t *testing.T) {
	const cb = hostauthtest.LoopbackCallback
	for _, tt := range []struct {
		name  string
		mod   func(url.Values)
		error string // "" means a page in the browser
	}{
		{name: "unknown client", mod: func(q url.Values) { q.Set("client_id", "no-such-client") }},
		{name: "no client", mod: func(q url.Values) { q.Del("client_id") }},
		{name: "unregistered redirect", mod: func(q url.Values) { q.Set("redirect_uri", "https://evil.example/callback") }},
		{name: "loopback redirect on another path", mod: func(q url.Values) { q.Set("redirect_uri", "http://127.0.0.1/other") }},
		{name: "loopback redirect on another host", mod: func(q url.Values) { q.Set("redirect_uri", "http://localhost/callback") }},
		{name: "no code challenge", mod: func(q url.Values) { q.Del("code_challenge") }, error: "invalid_request"},
		{name: "plain challenge method", mod: func(q url.Values) { q.Set("code_challenge_method", "plain") }, error: "invalid_request"},
		{name: "no challenge method", mod: func(q url.Values) { q.Del("code_challenge_method") }, error: "invalid_request"},
		{name: "other resource", mod: func(q url.Values) { q.Set("resource", "https://evil.example/mcp") }, error: "invalid_target"},
		{name: "token response type", mod: func(q url.Values) { q.Set("response_type", "token") }, error: "unsupported_response_type"},
		{name: "no response type", mod: func(q url.Values) { q.Del("response_type") }, error: "unsupported_response_type"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newConnector(t)
			id, _ := c.registerClient(t, "none", cb)
			u := c.authorizeURL(id, cb, tt.mod)
			b := c.browser(t)
			resp, body := get(t, b, u)
			if tt.error == "" {
				c.checkRefused(t, b, resp, body)
				if resp.StatusCode != http.StatusBadRequest {
					t.Errorf("status %d, want 400", resp.StatusCode)
				}
				if resp.Request.URL.String() != u || resp.Header.Get("Location") != "" {
					t.Errorf("refusal redirected the browser: ended at %s, Location %q", resp.Request.URL, resp.Header.Get("Location"))
				}
			} else {
				if resp.StatusCode != http.StatusFound {
					t.Fatalf("status %d, want a 302 back to the client", resp.StatusCode)
				}
				loc, err := resp.Location()
				if err != nil {
					t.Fatal(err)
				}
				if got := loc.Scheme + "://" + loc.Host + loc.Path; got != cb {
					t.Errorf("sent to %s, want the client's %s", got, cb)
				}
				q := loc.Query()
				if q.Get("error") != tt.error || q.Get("state") != "st" || q.Get("code") != "" {
					t.Errorf("sent back %s, want error=%s state=st and no code", loc, tt.error)
				}
			}
			if p := c.g.Prompts(); len(p) != 0 {
				t.Errorf("refused request reached Google: %q", p)
			}
		})
	}
}

// TestAuthorizeLoopbackPort shows a client registered with a portless
// loopback callback can sign in with a callback on any port, and that the
// code goes to, and is exchanged for, that exact address.
func TestAuthorizeLoopbackPort(t *testing.T) {
	c := newConnector(t)
	got := make(chan url.Values, 1)
	cbSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.URL.Query()
		io.WriteString(w, "done")
	}))
	t.Cleanup(cbSrv.Close)
	redirect := cbSrv.URL + "/callback"

	id, _ := c.registerClient(t, "none", hostauthtest.LoopbackCallback)
	resp, body := get(t, c.browser(t), c.authorizeURL(id, redirect, nil))
	if resp.StatusCode != http.StatusOK || body != "done" || resp.Request.URL.Path != "/callback" {
		t.Fatalf("sign-in ended at %s with %d %q, want the client's callback", resp.Request.URL, resp.StatusCode, body)
	}
	q := <-got
	if q.Get("code") == "" || q.Get("state") != "st" {
		t.Fatalf("callback got %v, want a code and state st", q)
	}
	r, m := c.token(t, exchange(id, q.Get("code"), redirect, testVerifier), "", "")
	if r.StatusCode != http.StatusOK || m["access_token"] == nil {
		t.Errorf("exchange for the port-specific callback: %d %v, want tokens", r.StatusCode, m)
	}
}

// TestAuthorizeAlwaysAsksGoogle shows a connector sign-in sends the browser
// to Google's account chooser even when the browser is already signed in to
// the board.
func TestAuthorizeAlwaysAsksGoogle(t *testing.T) {
	c := newConnector(t)
	b := c.browser(t)
	c.signInOK(t, b)

	id, _ := c.registerClient(t, "none", hostauthtest.LoopbackCallback)
	follow := b.CheckRedirect
	b.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, _ := get(t, b, c.authorizeURL(id, hostauthtest.LoopbackCallback, nil))
	b.CheckRedirect = follow
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status %d, want a 302 to Google", resp.StatusCode)
	}
	loc, err := resp.Location()
	if err != nil {
		t.Fatal(err)
	}
	if got := loc.Scheme + "://" + loc.Host + loc.Path; got != c.g.URL+"/authorize" {
		t.Errorf("sent to %s, want Google's %s/authorize", got, c.g.URL)
	}
	if got := loc.Query().Get("prompt"); got != "select_account" {
		t.Errorf("prompt = %q, want select_account", got)
	}

	if _, err := hostauthtest.FollowSignIn(b, c.authorizeURL(id, hostauthtest.LoopbackCallback, nil)); err != nil {
		t.Fatal(err)
	}
	if p := c.g.Prompts(); len(p) != 2 || p[0] != "select_account" || p[1] != "select_account" {
		t.Errorf("Google saw prompts %q, want select_account for the board and the connector sign-in", p)
	}
}

// TestCodeExchange shows a code is exchanged only once, only by the client
// it was issued to, only with the address and PKCE verifier of its
// request, and only within its five-minute lifetime.
func TestCodeExchange(t *testing.T) {
	const cb = hostauthtest.LoopbackCallback
	for _, tt := range []struct {
		name string
		// run uses code, issued to client id, and returns the final
		// exchange's response.
		run func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any)
		ok  bool
	}{
		{
			name: "once",
			run: func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any) {
				return c.token(t, exchange(id, code, cb, testVerifier), "", "")
			},
			ok: true,
		},
		{
			name: "within its lifetime",
			run: func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any) {
				c.clock.Advance(4 * time.Minute)
				return c.token(t, exchange(id, code, cb, testVerifier), "", "")
			},
			ok: true,
		},
		{
			name: "twice",
			run: func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any) {
				if r, m := c.token(t, exchange(id, code, cb, testVerifier), "", ""); r.StatusCode != http.StatusOK {
					t.Fatalf("first exchange: %d %v", r.StatusCode, m)
				}
				return c.token(t, exchange(id, code, cb, testVerifier), "", "")
			},
		},
		{
			name: "by another client",
			run: func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any) {
				other, _ := c.registerClient(t, "none", cb)
				return c.token(t, exchange(other, code, cb, testVerifier), "", "")
			},
		},
		{
			name: "after another client tried it",
			run: func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any) {
				other, _ := c.registerClient(t, "none", cb)
				c.token(t, exchange(other, code, cb, testVerifier), "", "")
				return c.token(t, exchange(id, code, cb, testVerifier), "", "")
			},
		},
		{
			name: "with another redirect",
			run: func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any) {
				return c.token(t, exchange(id, code, "http://127.0.0.1:9/callback", testVerifier), "", "")
			},
		},
		{
			name: "without a redirect",
			run: func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any) {
				return c.token(t, exchange(id, code, "", testVerifier), "", "")
			},
		},
		{
			name: "with the wrong verifier",
			run: func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any) {
				return c.token(t, exchange(id, code, cb, testVerifier+"x"), "", "")
			},
		},
		{
			name: "with the challenge as verifier",
			run: func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any) {
				return c.token(t, exchange(id, code, cb, s256(testVerifier)), "", "")
			},
		},
		{
			name: "without a verifier",
			run: func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any) {
				return c.token(t, exchange(id, code, cb, ""), "", "")
			},
		},
		{
			name: "after it expired",
			run: func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any) {
				c.clock.Advance(6 * time.Minute)
				return c.token(t, exchange(id, code, cb, testVerifier), "", "")
			},
		},
		{
			name: "unknown",
			run: func(t *testing.T, c *connector, id, code string) (*http.Response, map[string]any) {
				return c.token(t, exchange(id, code+"x", cb, testVerifier), "", "")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newConnector(t)
			id, _ := c.registerClient(t, "none", cb)
			code := c.code(t, id, cb)
			resp, m := tt.run(t, c, id, code)
			if tt.ok {
				if resp.StatusCode != http.StatusOK || m["access_token"] == nil {
					t.Errorf("got %d %v, want tokens", resp.StatusCode, m)
				}
				return
			}
			checkOAuthError(t, resp, m, http.StatusBadRequest, "invalid_grant")
		})
	}
}

// TestTokenResponse shows a token response carries a Bearer access token
// lasting an hour, a refresh token and the playmat scope, and is not to be
// cached; and that the token endpoint reads only a form body and knows only
// the code and refresh grants.
func TestTokenResponse(t *testing.T) {
	const cb = hostauthtest.LoopbackCallback
	c := newConnector(t)
	id, _ := c.registerClient(t, "none", cb)
	code := c.code(t, id, cb)

	// The same request as JSON, or in the query string, is refused and
	// leaves the code unused.
	body, _ := json.Marshal(map[string]string{"grant_type": "authorization_code", "code": code, "redirect_uri": cb, "client_id": id, "code_verifier": testVerifier})
	resp, err := http.Post(c.ts.URL+"/token", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if m := readJSON(t, resp); resp.StatusCode < 400 || m["access_token"] != nil {
		t.Errorf("JSON token request: %d %v, want a refusal", resp.StatusCode, m)
	}
	resp, err = http.Post(c.ts.URL+"/token?"+exchange(id, code, cb, testVerifier).Encode(), "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	if m := readJSON(t, resp); resp.StatusCode < 400 || m["access_token"] != nil {
		t.Errorf("query-string token request: %d %v, want a refusal", resp.StatusCode, m)
	}

	resp, m := c.token(t, exchange(id, code, cb, testVerifier), "", "")
	checkTokens := func(what string, resp *http.Response, m map[string]any) {
		t.Helper()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d %v, want 200", what, resp.StatusCode, m)
		}
		if s, _ := m["access_token"].(string); s == "" {
			t.Errorf("%s: no access_token in %v", what, m)
		}
		if s, _ := m["refresh_token"].(string); s == "" {
			t.Errorf("%s: no refresh_token in %v", what, m)
		}
		if m["token_type"] != "Bearer" || m["expires_in"] != float64(3600) || m["scope"] != "playmat" {
			t.Errorf("%s: response %v, want token_type Bearer, expires_in 3600, scope playmat", what, m)
		}
		if got := resp.Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", what, got)
		}
		if got := resp.Header.Get("Pragma"); got != "no-cache" {
			t.Errorf("%s: Pragma = %q, want no-cache", what, got)
		}
		if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("%s: Content-Type = %q, want JSON", what, got)
		}
	}
	checkTokens("code exchange", resp, m)
	resp, m2 := c.token(t, refreshForm(id, m["refresh_token"].(string)), "", "")
	checkTokens("refresh", resp, m2)

	for _, grant := range []string{"password", "client_credentials", "implicit"} {
		resp, m := c.token(t, url.Values{"grant_type": {grant}, "client_id": {id}}, "", "")
		checkOAuthError(t, resp, m, http.StatusBadRequest, "unsupported_grant_type")
	}
	// A request with no grant_type at all is malformed (RFC 6749 §5.2).
	resp, m = c.token(t, url.Values{"client_id": {id}}, "", "")
	checkOAuthError(t, resp, m, http.StatusBadRequest, "invalid_request")
}

// TestAccessTokenLifetime shows an access token is accepted for an hour by
// the injected clock and refused after it, that a refresh token renews it
// with a new pair however long after, and that a used refresh token, or one
// presented by another client, is refused.
func TestAccessTokenLifetime(t *testing.T) {
	c := newConnector(t)
	tok := hostauthtest.Connect(t, c.ts.URL, c.ts.URL, c.g)
	// Signed in now: the fake Google's ID tokens follow the real clock.
	other := hostauthtest.Connect(t, c.ts.URL, c.ts.URL, c.g)

	c.checkMCPAccepted(t, tok.AccessToken)
	c.clock.Advance(59 * time.Minute)
	c.checkMCPAccepted(t, tok.AccessToken)
	c.clock.Advance(2 * time.Minute)
	c.checkMCPRefused(t, tok.AccessToken)

	resp, m := c.token(t, refreshForm(tok.ClientID, tok.RefreshToken), "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: %d %v, want 200", resp.StatusCode, m)
	}
	access, refresh := m["access_token"].(string), m["refresh_token"].(string)
	if access == tok.AccessToken || refresh == tok.RefreshToken {
		t.Errorf("refresh returned the old tokens")
	}
	c.checkMCPAccepted(t, access)
	c.checkMCPRefused(t, tok.AccessToken)

	// The used refresh token is spent.
	resp, m = c.token(t, refreshForm(tok.ClientID, tok.RefreshToken), "", "")
	checkOAuthError(t, resp, m, http.StatusBadRequest, "invalid_grant")

	// Another client cannot use this client's refresh token, and trying
	// does not spend it.
	resp, m = c.token(t, refreshForm(other.ClientID, refresh), "", "")
	checkOAuthError(t, resp, m, http.StatusBadRequest, "invalid_grant")

	// A refresh token lasts until a restart, however long that is.
	c.clock.Advance(30 * 24 * time.Hour)
	c.checkMCPRefused(t, access)
	resp, m = c.token(t, refreshForm(tok.ClientID, refresh), "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh a month later: %d %v, want 200", resp.StatusCode, m)
	}
	c.checkMCPAccepted(t, m["access_token"].(string))
}

// TestRestartForgetsTokens shows that after the service restarts at the
// same address an old access token is refused with the sign-in challenge,
// refreshing with an old refresh token is answered with 401 invalid_client
// (the signal to register again), the old client is unknown, and a client
// that registers again can sign in.
func TestRestartForgetsTokens(t *testing.T) {
	c := newConnector(t)
	tok := hostauthtest.Connect(t, c.ts.URL, c.ts.URL, c.g)
	c.checkMCPAccepted(t, tok.AccessToken)

	c.restart(t)

	c.checkMCPRefused(t, tok.AccessToken)
	resp, m := c.token(t, refreshForm(tok.ClientID, tok.RefreshToken), "", "")
	checkOAuthError(t, resp, m, http.StatusUnauthorized, "invalid_client")
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("401 invalid_client has no WWW-Authenticate")
	}
	b := c.browser(t)
	r, body := get(t, b, c.authorizeURL(tok.ClientID, hostauthtest.LoopbackCallback, nil))
	c.checkRefused(t, b, r, body)

	again := hostauthtest.Connect(t, c.ts.URL, c.ts.URL, c.g)
	c.checkMCPAccepted(t, again.AccessToken)
}

// callWhoami calls the whoami tool in cs and returns the account it
// reports.
func callWhoami(t *testing.T, ctx context.Context, cs *mcp.ClientSession) whoami {
	t.Helper()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "whoami"})
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if res.IsError {
		t.Fatalf("whoami failed: %+v", res.Content)
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var w whoami
	if err := json.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	return w
}

// TestMCPClientSignsIn shows a standard MCP OAuth client — the MCP
// library's own — discovers the metadata, registers, signs in through
// Google and exchanges the code, then calls a tool that sees the signed-in
// account; and that a second account signing in with its own client sees
// its own account while the first keeps its own.
func TestMCPClientSignsIn(t *testing.T) {
	c := newConnector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	connect := func() *mcp.ClientSession {
		t.Helper()
		client := mcp.NewClient(&mcp.Implementation{Name: "oauth-test", Version: "1"}, nil)
		cs, err := client.Connect(ctx, hostauthtest.MCPTransport(t, c.ts.URL, c.ts.URL, c.g), nil)
		if err != nil {
			t.Fatalf("Connect: %v", err)
		}
		t.Cleanup(func() { cs.Close() })
		return cs
	}

	one := connect()
	want1 := whoami{ID: hostauthtest.DefaultAccount.ID, Email: hostauthtest.DefaultAccount.Email}
	if got := callWhoami(t, ctx, one); got != want1 {
		t.Errorf("first client is %+v, want %+v", got, want1)
	}

	two := hostauthtest.Account{ID: "sub-2", Email: "two@example.com"}
	c.g.SetAccount(two)
	second := connect()
	want2 := whoami{ID: two.ID, Email: two.Email}
	if got := callWhoami(t, ctx, second); got != want2 {
		t.Errorf("second client is %+v, want %+v", got, want2)
	}
	if got := callWhoami(t, ctx, one); got != want1 {
		t.Errorf("first client after the second signed in is %+v, want %+v", got, want1)
	}

	if n := len(c.a.st.clients); n != 2 {
		t.Errorf("%d clients registered, want 2", n)
	}
	for _, id := range []string{want1.ID, want2.ID} {
		if c.a.st.accounts[id] == nil {
			t.Errorf("account %s holds no live sign-in", id)
		}
	}
}

// TestConnectorLiveSignIns shows a connector sign-in counts as a live
// sign-in for its account, alongside the board session the browser gets,
// and that refreshing passes the count on rather than adding to it.
func TestConnectorLiveSignIns(t *testing.T) {
	c := newConnector(t)
	id := hostauthtest.DefaultAccount.ID
	live := func() int {
		c.a.st.mu.Lock()
		defer c.a.st.mu.Unlock()
		if st := c.a.st.accounts[id]; st != nil {
			return st.live
		}
		return 0
	}

	tok := hostauthtest.Connect(t, c.ts.URL, c.ts.URL, c.g)
	if got := live(); got != 2 {
		t.Fatalf("after one connector sign-in %s has %d live sign-ins, want 2 (board session and refresh token)", id, got)
	}
	if st := c.a.st.accounts[id]; st.email != hostauthtest.DefaultAccount.Email {
		t.Errorf("account email = %q, want %q", st.email, hostauthtest.DefaultAccount.Email)
	}

	resp, m := c.token(t, refreshForm(tok.ClientID, tok.RefreshToken), "", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: %d %v", resp.StatusCode, m)
	}
	if got := live(); got != 2 {
		t.Errorf("after a refresh %s has %d live sign-ins, want 2", id, got)
	}
	// A refused refresh changes nothing.
	c.token(t, refreshForm(tok.ClientID, tok.RefreshToken), "", "")
	if got := live(); got != 2 {
		t.Errorf("after a refused refresh %s has %d live sign-ins, want 2", id, got)
	}

	hostauthtest.Connect(t, c.ts.URL, c.ts.URL, c.g)
	if got := live(); got != 4 {
		t.Errorf("after a second connector sign-in %s has %d live sign-ins, want 4", id, got)
	}
}
