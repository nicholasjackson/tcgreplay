// Package hostauth is sign-in for the hosted playmat: an OAuth
// authorization server for MCP clients (claude.ai's custom connectors)
// that hands the actual login to Google, and board sessions held in a
// cookie. Everything it knows lives in process memory, so a restart or
// redeploy signs everyone out. Every address it hands out is built from
// the configured public URL, never from the incoming request.
//
// The desktop never builds an Auth: only hosted mode signs users in.
package hostauth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

// Config is what the sign-in service is built from.
type Config struct {
	// Public is the address the playmat is reached at (the -public-url
	// value): scheme and host only.
	Public *url.URL
	// GoogleClientID and GoogleSecret are the Google OAuth web client the
	// playmat signs users in with.
	GoogleClientID, GoogleSecret string
	// RateLimit is the number of requests one account may make a minute;
	// 0 means no limit.
	RateLimit int
	// MaxUsers is the number of distinct accounts that may be signed in at
	// once; 0 means no cap.
	MaxUsers int
	// Google is Google's OAuth endpoints; the zero value means the real
	// Google. Tests substitute a fake.
	Google oauth2.Endpoint
	// Now is the clock; nil means time.Now. Tests substitute their own.
	Now func() time.Time
	// HTTPClient makes the calls to Google; nil means http.DefaultClient.
	HTTPClient *http.Client
}

// Account is the signed-in Google account a request acts for.
type Account struct {
	// ID is Google's stable subject identifier ("sub") for the account.
	ID string
	// Email is the account's email address, for display and logs.
	Email string
}

// Auth is the hosted playmat's sign-in service.
type Auth struct {
	cfg    Config
	public string // cfg.Public as a string, no trailing slash
	google *oauth2.Config
	now    func() time.Time
	client *http.Client
	st     *store
}

// New returns a sign-in service for cfg. It fails when the public address
// or the Google credentials are missing.
func New(cfg Config) (*Auth, error) {
	if cfg.Public == nil || cfg.Public.Scheme == "" || cfg.Public.Host == "" {
		return nil, errors.New("hostauth: a public URL with a scheme and host is required")
	}
	if cfg.GoogleClientID == "" || cfg.GoogleSecret == "" {
		return nil, errors.New("hostauth: a Google client ID and secret are required")
	}
	if cfg.Google.AuthURL == "" {
		cfg.Google = endpoints.Google
	}
	a := &Auth{
		cfg:    cfg,
		public: (&url.URL{Scheme: cfg.Public.Scheme, Host: cfg.Public.Host}).String(),
		now:    cfg.Now,
		client: cfg.HTTPClient,
		st:     newStore(),
	}
	if a.now == nil {
		a.now = time.Now
	}
	if a.client == nil {
		a.client = http.DefaultClient
	}
	a.google = &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleSecret,
		Endpoint:     cfg.Google,
		RedirectURL:  a.public + googleCallbackPath,
		Scopes:       []string{"openid", "email"},
	}
	return a, nil
}

// Routes registers the sign-in service's own routes on mux. None of them
// needs a sign-in.
func (a *Auth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+googleCallbackPath, a.serveGoogleCallback)
	mux.HandleFunc("GET /healthz", serveHealth)
	a.oauthRoutes(mux)
	a.boardRoutes(mux)
}

// secure reports whether cookies must be marked Secure: the playmat is
// reached over https.
func (a *Auth) secure() bool { return a.cfg.Public.Scheme == "https" }

func serveHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte("ok\n"))
}

type accountKey struct{}

// withAccount returns ctx carrying the signed-in account.
func withAccount(ctx context.Context, acct Account) context.Context {
	return context.WithValue(ctx, accountKey{}, acct)
}

// AccountFrom returns the signed-in account a board request acts for, if
// the request passed the board's session check.
func AccountFrom(ctx context.Context) (Account, bool) {
	acct, ok := ctx.Value(accountKey{}).(Account)
	return acct, ok
}
