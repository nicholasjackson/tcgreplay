package hostauth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

const (
	// scope is the one scope the playmat grants: full use of the playmat.
	scope = "playmat"
	// codeLifetime is how long an authorization code may be exchanged.
	codeLifetime = 5 * time.Minute
	// accessLifetime is how long an access token lasts; refresh tokens
	// last until the process ends.
	accessLifetime = time.Hour
	// maxClients bounds registered clients; the oldest is forgotten first.
	maxClients = 1000
	// maxBody bounds registration and token request bodies.
	maxBody = 64 << 10

	resourceMetadataPath = "/.well-known/oauth-protected-resource"
)

// claudeCallbacks are the redirect addresses claude.ai registers.
var claudeCallbacks = []string{
	"https://claude.ai/api/mcp/auth_callback",
	"https://claude.com/api/mcp/auth_callback",
}

// resource is the protected resource's identifier: the MCP endpoint.
func (a *Auth) resource() string { return a.public + "/mcp" }

// oauthRoutes registers the connector's authorization server.
func (a *Auth) oauthRoutes(mux *http.ServeMux) {
	prm := auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               a.resource(),
		AuthorizationServers:   []string{a.public},
		ScopesSupported:        []string{scope},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "tcgreplay playmat",
	})
	mux.Handle(resourceMetadataPath, prm)
	mux.Handle(resourceMetadataPath+"/mcp", prm)
	mux.HandleFunc("/.well-known/oauth-authorization-server", a.serveServerMetadata)
	mux.HandleFunc("/.well-known/openid-configuration", a.serveServerMetadata)
	mux.HandleFunc("POST /register", a.serveRegister)
	mux.HandleFunc("OPTIONS /register", serveCORSPreflight)
	mux.HandleFunc("GET /authorize", a.serveAuthorize)
	mux.HandleFunc("POST /token", a.serveToken)
	mux.HandleFunc("OPTIONS /token", serveCORSPreflight)
}

func cors(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func serveCORSPreflight(w http.ResponseWriter, r *http.Request) {
	cors(w)
	w.WriteHeader(http.StatusNoContent)
}

// serverMetadata is the RFC 8414 authorization server metadata the
// playmat publishes. It has no jwks_uri: tokens are opaque.
type serverMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RegistrationEndpoint              string   `json:"registration_endpoint"`
	ScopesSupported                   []string `json:"scopes_supported"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
}

func (a *Auth) serveServerMetadata(w http.ResponseWriter, r *http.Request) {
	cors(w)
	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
		return
	case http.MethodGet, http.MethodHead:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, serverMetadata{
		Issuer:                            a.public,
		AuthorizationEndpoint:             a.public + "/authorize",
		TokenEndpoint:                     a.public + "/token",
		RegistrationEndpoint:              a.public + "/register",
		ScopesSupported:                   []string{scope},
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		TokenEndpointAuthMethodsSupported: []string{"none", "client_secret_post", "client_secret_basic"},
		CodeChallengeMethodsSupported:     []string{"S256"},
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// oauthError is an OAuth error response (RFC 6749 §5.2, RFC 7591 §3.2.2).
type oauthError struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

// allowedRedirect reports whether a client may register redirect address
// u: claude.ai's callbacks, or a loopback address on any port (a native
// client such as Claude Code). Restricting redirects keeps a stranger who
// registers a client from collecting codes issued in a victim's browser.
func allowedRedirect(u string) bool {
	if slices.Contains(claudeCallbacks, u) {
		return true
	}
	p, err := url.Parse(u)
	if err != nil || p.Scheme != "http" || p.Fragment != "" || p.User != nil {
		return false
	}
	return isLoopbackHost(p.Hostname())
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// registration is the response to a client registration (RFC 7591 §3.2.1).
type registration struct {
	oauthex.ClientRegistrationMetadata
	ClientID              string `json:"client_id"`
	ClientSecret          string `json:"client_secret,omitempty"`
	ClientIDIssuedAt      int64  `json:"client_id_issued_at"`
	ClientSecretExpiresAt *int64 `json:"client_secret_expires_at,omitempty"`
}

// serveRegister registers an MCP client (RFC 7591).
func (a *Auth) serveRegister(w http.ResponseWriter, r *http.Request) {
	cors(w)
	w.Header().Set("Cache-Control", "no-store")
	var meta oauthex.ClientRegistrationMetadata
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody)).Decode(&meta); err != nil {
		writeJSON(w, http.StatusBadRequest, oauthError{"invalid_client_metadata", "the body is not client metadata JSON"})
		return
	}
	if len(meta.RedirectURIs) == 0 {
		writeJSON(w, http.StatusBadRequest, oauthError{"invalid_redirect_uri", "at least one redirect_uri is required"})
		return
	}
	for _, u := range meta.RedirectURIs {
		if !allowedRedirect(u) {
			log.Printf("hostauth: refused client registration with redirect %q", u)
			writeJSON(w, http.StatusBadRequest, oauthError{"invalid_redirect_uri", "redirect_uri must be Claude's callback or a loopback address"})
			return
		}
	}
	switch meta.TokenEndpointAuthMethod {
	case "":
		meta.TokenEndpointAuthMethod = "none"
	case "none", "client_secret_post", "client_secret_basic":
	default:
		writeJSON(w, http.StatusBadRequest, oauthError{"invalid_client_metadata", "unsupported token_endpoint_auth_method"})
		return
	}
	meta.GrantTypes = []string{"authorization_code", "refresh_token"}
	meta.ResponseTypes = []string{"code"}
	meta.Scope = scope

	now := a.now()
	c := &client{
		id:         newToken(),
		redirects:  meta.RedirectURIs,
		authMethod: meta.TokenEndpointAuthMethod,
		created:    now,
	}
	res := registration{ClientRegistrationMetadata: meta, ClientID: c.id, ClientIDIssuedAt: now.Unix()}
	if c.authMethod != "none" {
		secret := newToken()
		c.secretKey = key(secret)
		res.ClientSecret = secret
		var never int64
		res.ClientSecretExpiresAt = &never
	}
	a.st.addClient(c)
	log.Printf("hostauth: registered client %q", meta.ClientName)
	writeJSON(w, http.StatusCreated, res)
}

// redirectMatches reports whether redirect is one c registered. A loopback
// redirect matches whatever its port (RFC 8252 §7.3), since a native
// client picks a free port each time.
func (c *client) redirectMatches(redirect string) bool {
	if slices.Contains(c.redirects, redirect) {
		return true
	}
	ru, err := url.Parse(redirect)
	if err != nil || ru.Scheme != "http" || !isLoopbackHost(ru.Hostname()) {
		return false
	}
	for _, reg := range c.redirects {
		gu, err := url.Parse(reg)
		if err != nil || gu.Scheme != "http" || !isLoopbackHost(gu.Hostname()) {
			continue
		}
		if gu.Hostname() == ru.Hostname() && gu.EscapedPath() == ru.EscapedPath() && gu.RawQuery == ru.RawQuery {
			return true
		}
	}
	return false
}

// serveAuthorize starts a connector sign-in (RFC 6749 §4.1.1 with PKCE).
// Problems with the client or its redirect are shown in the browser;
// anything else is sent back to the client's redirect.
func (a *Auth) serveAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c := a.st.client(q.Get("client_id"))
	if c == nil {
		log.Printf("hostauth: refused authorization for an unknown client")
		errorPage(w, http.StatusBadRequest, "This app is not registered with tcgreplay. Remove the connector and add it again.")
		return
	}
	redirect := q.Get("redirect_uri")
	if redirect == "" && len(c.redirects) == 1 {
		redirect = c.redirects[0]
	}
	if !c.redirectMatches(redirect) {
		log.Printf("hostauth: refused authorization with an unregistered redirect %q", redirect)
		errorPage(w, http.StatusBadRequest, "This sign-in request names an address its app did not register.")
		return
	}
	state := q.Get("state")
	if q.Get("response_type") != "code" {
		redirectError(w, r, redirect, state, "unsupported_response_type", "only response_type=code is supported")
		return
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		redirectError(w, r, redirect, state, "invalid_request", "PKCE with code_challenge_method=S256 is required")
		return
	}
	if res := q.Get("resource"); res != "" && res != a.resource() {
		redirectError(w, r, redirect, state, "invalid_target", "unknown resource")
		return
	}
	a.startGoogle(w, r, &pendingLogin{
		kind:        loginConnector,
		clientID:    c.id,
		redirect:    redirect,
		clientState: state,
		challenge:   q.Get("code_challenge"),
	})
}

// withQuery returns redirect with the parameters added to its query.
func withQuery(redirect string, params url.Values) string {
	u, err := url.Parse(redirect)
	if err != nil {
		return redirect
	}
	q := u.Query()
	for k, vs := range params {
		for _, v := range vs {
			if v != "" {
				q.Set(k, v)
			}
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func redirectError(w http.ResponseWriter, r *http.Request, redirect, state, code, desc string) {
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, withQuery(redirect, url.Values{"error": {code}, "error_description": {desc}, "state": {state}}), http.StatusFound)
}

// continueConnector finishes a connector sign-in once Google has signed
// the account in: it issues a one-time code and returns the browser to the
// client.
func (a *Auth) continueConnector(w http.ResponseWriter, r *http.Request, p *pendingLogin, acct Account) {
	code := a.st.addCode(a.now(), &authCode{
		clientID:  p.clientID,
		redirect:  p.redirect,
		challenge: p.challenge,
		account:   acct,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, withQuery(p.redirect, url.Values{"code": {code}, "state": {p.clientState}}), http.StatusFound)
}

// tokenResponse is a successful token response (RFC 6749 §5.1).
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// serveToken exchanges a code or a refresh token for tokens (RFC 6749 §3.2).
func (a *Auth) serveToken(w http.ResponseWriter, r *http.Request) {
	cors(w)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") == "" {
		writeJSON(w, http.StatusBadRequest, oauthError{"invalid_request", "a form-encoded body with a grant_type is required"})
		return
	}
	c := a.authenticateClient(r)
	if c == nil {
		// Also the answer once a restart has forgotten the client: the
		// signal for it to register again.
		w.Header().Set("WWW-Authenticate", `Basic realm="tcgreplay"`)
		writeJSON(w, http.StatusUnauthorized, oauthError{"invalid_client", "unknown client or bad client credentials; register again"})
		return
	}
	now := a.now()
	var acct Account
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		code := a.st.takeCode(r.PostForm.Get("code"))
		if code == nil || now.After(code.expires) || code.clientID != c.id ||
			code.redirect != r.PostForm.Get("redirect_uri") || s256(r.PostForm.Get("code_verifier")) != code.challenge {
			if code != nil {
				a.st.release(code.account.ID)
			}
			writeJSON(w, http.StatusBadRequest, oauthError{"invalid_grant", "the code is unknown, used, expired or not for this client, redirect or verifier"})
			return
		}
		acct = code.account
	case "refresh_token":
		var ok bool
		acct, ok = a.st.takeRefresh(c.id, r.PostForm.Get("refresh_token"))
		if !ok {
			writeJSON(w, http.StatusBadRequest, oauthError{"invalid_grant", "the refresh token is unknown or already used"})
			return
		}
	default:
		writeJSON(w, http.StatusBadRequest, oauthError{"unsupported_grant_type", "only authorization_code and refresh_token are supported"})
		return
	}
	access, refresh := a.st.issueTokens(now, c.id, acct)
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(accessLifetime / time.Second),
		RefreshToken: refresh,
		Scope:        scope,
	})
}

// authenticateClient returns the registered client a token request comes
// from, checking its secret if it has one, or nil.
func (a *Auth) authenticateClient(r *http.Request) *client {
	id, secret, basic := r.BasicAuth()
	if basic {
		// RFC 6749 §2.3.1: the credentials are form-encoded first.
		if v, err := url.QueryUnescape(id); err == nil {
			id = v
		}
		if v, err := url.QueryUnescape(secret); err == nil {
			secret = v
		}
	} else {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	c := a.st.client(id)
	if c == nil {
		return nil
	}
	if c.secretKey != "" && subtle.ConstantTimeCompare([]byte(key(secret)), []byte(c.secretKey)) != 1 {
		return nil
	}
	return c
}

// s256 is the PKCE S256 challenge for a verifier (RFC 7636 §4.2).
func s256(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// RequireToken protects the MCP endpoint: a request without a live access
// token is refused with 401 and a pointer to the protected resource
// metadata, which is how claude.ai learns to sign in. Refusals are logged.
func (a *Auth) RequireToken(next http.Handler) http.Handler {
	guarded := auth.RequireBearerToken(a.verify, &auth.RequireBearerTokenOptions{
		ResourceMetadataURL: a.public + resourceMetadataPath + "/mcp",
		Scopes:              []string{scope},
	})(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f := strings.Fields(r.Header.Get("Authorization")); len(f) != 2 || !strings.EqualFold(f[0], "bearer") {
			log.Printf("hostauth: refused %s %s: no bearer token", r.Method, r.URL.Path)
		}
		guarded.ServeHTTP(w, r)
	})
}

// verify checks an access token for the MCP endpoint.
func (a *Auth) verify(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
	now := a.now()
	t := a.st.accessToken(token)
	if t == nil || !now.Before(t.expires) {
		log.Printf("hostauth: refused %s %s: unknown or expired token", r.Method, r.URL.Path)
		return nil, auth.ErrInvalidToken
	}
	return &auth.TokenInfo{
		Scopes: []string{scope},
		// The library checks expiry against the real clock: give it the
		// token's remaining life.
		Expiration: time.Now().Add(t.expires.Sub(now)),
		UserID:     t.account.ID,
		Extra:      map[string]any{"email": t.account.Email},
	}, nil
}

// AccountFromToken returns the signed-in account an MCP request acts for,
// from the token information the MCP library hands every request
// (RequestExtra.TokenInfo).
func AccountFromToken(ti *auth.TokenInfo) (Account, bool) {
	if ti == nil || ti.UserID == "" {
		return Account{}, false
	}
	email, _ := ti.Extra["email"].(string)
	return Account{ID: ti.UserID, Email: email}, true
}
