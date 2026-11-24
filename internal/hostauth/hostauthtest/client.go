package hostauthtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// LoopbackCallback is the redirect address the test MCP clients register,
// as a native client such as Claude Code does. Nothing listens there: the
// test browser stops at the redirect and reads the code from it.
const LoopbackCallback = "http://127.0.0.1/callback"

// Server returns an HTTP client for server-to-server calls to the playmat
// served at server and reached at public.
func Server(public, server string) *http.Client {
	pu, _ := url.Parse(public)
	su, _ := url.Parse(server)
	return &http.Client{Transport: &alias{from: pu.Host, to: su.Host}}
}

// MCPTransport returns a Streamable HTTP client transport for the playmat's
// MCP endpoint that signs in the way an MCP client such as claude.ai does,
// through the MCP library's own OAuth client: it discovers the metadata,
// registers itself, sends a test browser through sign-in with g's current
// account, and exchanges the code for tokens, refreshing as needed.
func MCPTransport(t testing.TB, public, server string, g *FakeGoogle) mcp.Transport {
	t.Helper()
	return mcpTransportWith(t, public, server, ChooseGoogle)
}

// PasswordMCPTransport is MCPTransport signing in with a username and
// password on the sign-in page instead of Google.
func PasswordMCPTransport(t testing.TB, public, server, username, password string) mcp.Transport {
	t.Helper()
	return mcpTransportWith(t, public, server, ChoosePassword(username, password))
}

// mcpTransportWith is MCPTransport choosing the way in with choose.
func mcpTransportWith(t testing.TB, public, server string, choose Chooser) mcp.Transport {
	t.Helper()
	browser := Browser(t, public, server)
	h, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{ClientName: "hostauthtest", RedirectURIs: []string{LoopbackCallback}},
		},
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			loc, err := FollowSignInWith(browser, args.URL, choose)
			if err != nil {
				return nil, err
			}
			q := loc.Query()
			if e := q.Get("error"); e != "" {
				return nil, fmt.Errorf("authorization refused: %s", e)
			}
			return &auth.AuthorizationResult{Code: q.Get("code"), State: q.Get("state")}, nil
		},
		Client: Server(public, server),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &mcp.StreamableClientTransport{
		Endpoint:     strings.TrimSuffix(public, "/") + "/mcp",
		HTTPClient:   Server(public, server),
		OAuthHandler: h,
	}
}

// googleButton finds the sign-in page's Google button. The page renders it
// as <a ... data-provider="google" href="...">, attributes in that order.
var googleButton = regexp.MustCompile(`data-provider="google" href="([^"]+)"`)

// ChooseGoogle continues a browser that has landed on the playmat's
// sign-in page by following its "Sign in with Google" button, as a person
// would click it. Any other response is returned as it is. It closes the
// page's body when it follows the button.
func ChooseGoogle(browser *http.Client, resp *http.Response) (*http.Response, error) {
	if resp.StatusCode != http.StatusOK || resp.Request.URL.Path != "/auth/signin" {
		return resp, nil
	}
	var b bytes.Buffer
	b.ReadFrom(resp.Body)
	resp.Body.Close()
	m := googleButton.FindStringSubmatch(b.String())
	if m == nil {
		return nil, fmt.Errorf("sign-in page at %s has no Google button: %s", resp.Request.URL, b.String())
	}
	href, err := resp.Request.URL.Parse(strings.ReplaceAll(m[1], "&amp;", "&"))
	if err != nil {
		return nil, err
	}
	return browser.Get(href.String())
}

// stateField finds the sign-in page's password form's state.
var stateField = regexp.MustCompile(`name="state" value="([^"]+)"`)

// Chooser continues a browser that has landed on the playmat's sign-in
// page, as a person choosing a way in would; any other response passes
// through unchanged.
type Chooser func(browser *http.Client, resp *http.Response) (*http.Response, error)

// ChoosePassword returns a Chooser that fills in the sign-in page's
// username and password form and submits it.
func ChoosePassword(username, password string) Chooser {
	return func(browser *http.Client, resp *http.Response) (*http.Response, error) {
		if resp.StatusCode != http.StatusOK || resp.Request.URL.Path != "/auth/signin" {
			return resp, nil
		}
		var b bytes.Buffer
		b.ReadFrom(resp.Body)
		resp.Body.Close()
		m := stateField.FindStringSubmatch(b.String())
		if m == nil {
			return nil, fmt.Errorf("sign-in page at %s has no username and password form: %s", resp.Request.URL, b.String())
		}
		action, _ := resp.Request.URL.Parse("/auth/password")
		return browser.PostForm(action.String(), url.Values{"state": {m[1]}, "username": {username}, "password": {password}})
	}
}

// FollowSignIn opens a sign-in address in browser, following it through
// the playmat, its sign-in page's Google button and Google, and returns
// where it finally sends the browser off the test servers (an MCP
// client's callback). It fails if the browser stops on a page instead.
func FollowSignIn(browser *http.Client, start string) (*url.URL, error) {
	return FollowSignInWith(browser, start, ChooseGoogle)
}

// FollowPasswordSignIn is FollowSignIn signing in with a username and
// password on the sign-in page instead of Google.
func FollowPasswordSignIn(browser *http.Client, start, username, password string) (*url.URL, error) {
	return FollowSignInWith(browser, start, ChoosePassword(username, password))
}

// FollowSignInWith is FollowSignIn choosing the way in with choose.
func FollowSignInWith(browser *http.Client, start string, choose Chooser) (*url.URL, error) {
	resp, err := browser.Get(start)
	if err != nil {
		return nil, err
	}
	if resp, err = choose(browser, resp); err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 3 {
		var b bytes.Buffer
		b.ReadFrom(resp.Body)
		return nil, fmt.Errorf("sign-in stopped at %s with %d: %s", resp.Request.URL, resp.StatusCode, b.String())
	}
	return resp.Location()
}

// Tokens are what a client signed in through Connect holds.
type Tokens struct {
	ClientID, AccessToken, RefreshToken string
}

// Connect signs a public client in through the connector endpoints by
// hand — register, authorize through g with a test browser, exchange the
// code — and returns its tokens.
func Connect(t testing.TB, public, server string, g *FakeGoogle) Tokens {
	t.Helper()
	return connectWith(t, public, server, ChooseGoogle)
}

// ConnectPassword is Connect signing in with a username and password on
// the sign-in page instead of Google.
func ConnectPassword(t testing.TB, public, server, username, password string) Tokens {
	t.Helper()
	return connectWith(t, public, server, ChoosePassword(username, password))
}

// connectWith is Connect choosing the way in with choose.
func connectWith(t testing.TB, public, server string, choose Chooser) Tokens {
	t.Helper()
	public = strings.TrimSuffix(public, "/")
	hc := Server(public, server)
	body, _ := json.Marshal(map[string]any{"client_name": "hostauthtest", "redirect_uris": []string{LoopbackCallback}, "token_endpoint_auth_method": "none"})
	resp, err := hc.Post(public+"/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var reg struct {
		ClientID string `json:"client_id"`
	}
	json.NewDecoder(resp.Body).Decode(&reg)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || reg.ClientID == "" {
		t.Fatalf("register: %d", resp.StatusCode)
	}
	verifier := "test-verifier-0123456789-0123456789-0123456789"
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {reg.ClientID},
		"redirect_uri":          {LoopbackCallback},
		"state":                 {"st"},
		"code_challenge":        {challenge(verifier)},
		"code_challenge_method": {"S256"},
		"resource":              {public + "/mcp"},
	}
	loc, err := FollowSignInWith(Browser(t, public, server), public+"/authorize?"+q.Encode(), choose)
	if err != nil {
		t.Fatal(err)
	}
	code := loc.Query().Get("code")
	if code == "" || loc.Query().Get("state") != "st" {
		t.Fatalf("authorize ended at %s, want a code and state st", loc)
	}
	resp, err = hc.PostForm(public+"/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {LoopbackCallback},
		"client_id":     {reg.ClientID},
		"code_verifier": {verifier},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	json.NewDecoder(resp.Body).Decode(&tok)
	if resp.StatusCode != http.StatusOK || tok.AccessToken == "" {
		t.Fatalf("token: %d", resp.StatusCode)
	}
	return Tokens{ClientID: reg.ClientID, AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken}
}

// BoardCookie signs a test browser in to the board through /auth/login,
// the sign-in page and Google with g's current account, and returns the board session
// cookie, for a websocket dial or a plain request.
func BoardCookie(t testing.TB, public, server string, g *FakeGoogle) *http.Cookie {
	t.Helper()
	return boardCookieWith(t, public, server, ChooseGoogle)
}

// PasswordBoardCookie is BoardCookie signing in with a username and
// password on the sign-in page instead of Google.
func PasswordBoardCookie(t testing.TB, public, server, username, password string) *http.Cookie {
	t.Helper()
	return boardCookieWith(t, public, server, ChoosePassword(username, password))
}

// boardCookieWith is BoardCookie choosing the way in with choose.
func boardCookieWith(t testing.TB, public, server string, choose Chooser) *http.Cookie {
	t.Helper()
	public = strings.TrimSuffix(public, "/")
	b := Browser(t, public, server)
	resp, err := b.Get(public + "/auth/login?next=/auth/session")
	if err != nil {
		t.Fatal(err)
	}
	if resp, err = choose(b, resp); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("board sign-in ended at %s with %d", resp.Request.URL, resp.StatusCode)
	}
	u, _ := url.Parse(public + "/")
	for _, c := range b.Jar.Cookies(u) {
		if c.Name == "tcgreplay_session" {
			return c
		}
	}
	t.Fatal("board sign-in set no session cookie")
	return nil
}
