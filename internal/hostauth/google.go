package hostauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const (
	googleCallbackPath = "/auth/google/callback"
	// loginCookie ties a sign-in in progress to the browser that started it.
	loginCookie = "tcgreplay_login"
	// sessionCookie holds the board session.
	sessionCookie = "tcgreplay_session"
	// googleTimeout bounds the code exchange with Google.
	googleTimeout = 10 * time.Second
)

// googleIssuers are the issuers Google's ID tokens carry.
var googleIssuers = []string{"https://accounts.google.com", "accounts.google.com"}

// startGoogle records p as a sign-in in progress, ties it to this browser
// with the login cookie, and sends the browser to Google's account
// chooser.
func (a *Auth) startGoogle(w http.ResponseWriter, r *http.Request, p *pendingLogin) {
	p.nonce = newToken()
	p.verifier = oauth2.GenerateVerifier()
	state := a.st.addPending(a.now(), p)
	http.SetCookie(w, &http.Cookie{
		Name:     loginCookie,
		Value:    state,
		Path:     googleCallbackPath,
		MaxAge:   int(pendingLifetime / time.Second),
		HttpOnly: true,
		Secure:   a.secure(),
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, a.google.AuthCodeURL(state,
		oauth2.S256ChallengeOption(p.verifier),
		oauth2.SetAuthURLParam("nonce", p.nonce),
		oauth2.SetAuthURLParam("prompt", "select_account"),
	), http.StatusFound)
}

// serveGoogleCallback takes Google's answer for both sign-in flows: it
// checks the answer belongs to this browser's sign-in in progress, reads
// the account from Google's ID token, signs the browser in to the board
// and continues the flow that started the sign-in.
func (a *Auth) serveGoogleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")
	c, err := r.Cookie(loginCookie)
	if state == "" || err != nil || c.Value != state {
		log.Printf("hostauth: refused Google callback: state does not match this browser's sign-in")
		errorPage(w, http.StatusBadRequest, "This sign-in link has expired or was opened in a different browser. Please start signing in again.")
		return
	}
	// The login cookie has done its job either way.
	http.SetCookie(w, &http.Cookie{Name: loginCookie, Path: googleCallbackPath, MaxAge: -1, HttpOnly: true, Secure: a.secure(), SameSite: http.SameSiteLaxMode})
	p := a.st.takePending(a.now(), state)
	if p == nil {
		log.Printf("hostauth: refused Google callback: no sign-in in progress for its state")
		errorPage(w, http.StatusBadRequest, "This sign-in has expired or was already used. Please start signing in again.")
		return
	}
	if e := q.Get("error"); e != "" {
		log.Printf("hostauth: Google sign-in not completed: %s", e)
		errorPage(w, http.StatusUnauthorized, "Google sign-in was not completed. Please try again.")
		return
	}
	acct, err := a.exchangeGoogle(r.Context(), q.Get("code"), p)
	if err != nil {
		log.Printf("hostauth: refused Google sign-in: %v", err)
		errorPage(w, http.StatusUnauthorized, "Google sign-in could not be confirmed. Please try again.")
		return
	}
	if err := a.signIn(w, acct); err != nil {
		log.Printf("hostauth: refused sign-in for %s: %v", acct.Email, err)
		busyPage(w, a.cfg.MaxUsers)
		return
	}
	switch p.kind {
	case loginConnector:
		a.continueConnector(w, r, p, acct)
	default:
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, p.next, http.StatusFound)
	}
}

// exchangeGoogle trades Google's code for tokens and returns the account
// its ID token names, after checking the token is meant for this sign-in.
func (a *Auth) exchangeGoogle(ctx context.Context, code string, p *pendingLogin) (Account, error) {
	if code == "" {
		return Account{}, errors.New("no code")
	}
	ctx, cancel := context.WithTimeout(ctx, googleTimeout)
	defer cancel()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, a.client)
	tok, err := a.google.Exchange(ctx, code, oauth2.VerifierOption(p.verifier))
	if err != nil {
		return Account{}, fmt.Errorf("code exchange: %v", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return Account{}, errors.New("no ID token")
	}
	return a.checkIDToken(raw, p.nonce)
}

// idClaims are the ID token claims the sign-in reads.
type idClaims struct {
	Iss   string          `json:"iss"`
	Aud   json.RawMessage `json:"aud"`
	Exp   float64         `json:"exp"`
	Nonce string          `json:"nonce"`
	Sub   string          `json:"sub"`
	Email string          `json:"email"`
}

// checkIDToken decodes Google's ID token and checks its issuer, audience,
// expiry and nonce. The signature is not checked: the token came straight
// from Google's token endpoint over TLS (OpenID Connect Core §3.1.3.7).
func (a *Auth) checkIDToken(raw, nonce string) (Account, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Account{}, errors.New("malformed ID token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return Account{}, fmt.Errorf("malformed ID token: %v", err)
	}
	var c idClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return Account{}, fmt.Errorf("malformed ID token: %v", err)
	}
	okIss := false
	for _, iss := range googleIssuers {
		okIss = okIss || c.Iss == iss
	}
	if !okIss {
		return Account{}, fmt.Errorf("ID token issuer %q is not Google", c.Iss)
	}
	if !audienceHas(c.Aud, a.cfg.GoogleClientID) {
		return Account{}, errors.New("ID token is for another client")
	}
	if c.Exp == 0 || !a.now().Before(time.Unix(int64(c.Exp), 0)) {
		return Account{}, errors.New("ID token has expired")
	}
	if c.Nonce != nonce {
		return Account{}, errors.New("ID token nonce does not match")
	}
	if c.Sub == "" {
		return Account{}, errors.New("ID token names no account")
	}
	return Account{ID: c.Sub, Email: c.Email}, nil
}

// audienceHas reports whether an aud claim (a string or an array of
// strings) names id.
func audienceHas(aud json.RawMessage, id string) bool {
	var one string
	if json.Unmarshal(aud, &one) == nil {
		return one == id
	}
	var many []string
	if json.Unmarshal(aud, &many) == nil {
		for _, a := range many {
			if a == id {
				return true
			}
		}
	}
	return false
}

// signIn starts a board session for acct in this browser. Every Google
// sign-in does, whichever flow started it, so a browser that connected
// claude.ai can open a board link without signing in again. It is also
// where the user cap is applied, once for both flows: a new account over
// the cap gets ErrBusy.
func (a *Auth) signIn(w http.ResponseWriter, acct Account) error {
	id, err := a.st.addSession(a.now(), acct, a.cfg.MaxUsers)
	if err != nil {
		return fmt.Errorf("%w: it already has the most signed-in users it serves at once (%d); try again later", ErrBusy, a.cfg.MaxUsers)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secure(),
		SameSite: http.SameSiteLaxMode,
	})
	log.Printf("hostauth: signed in %s", acct.Email)
	return nil
}

// localPath reports whether next is a path on this site that a sign-in may
// return to: it starts with a single "/" and names no other host.
func localPath(next string) bool {
	return strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") && !strings.HasPrefix(next, "/\\")
}
