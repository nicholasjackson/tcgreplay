package hostauth

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

const (
	// signinPath shows the sign-in page for a sign-in in progress.
	signinPath = "/auth/signin"
	// googleStartPath starts Google for a sign-in in progress.
	googleStartPath = "/auth/google"
	// passwordPath takes the sign-in page's username and password form.
	passwordPath = "/auth/password"
	// maxPasswordForm bounds the password form's body.
	maxPasswordForm = 8 << 10

	msgBadCredentials = "That username and password don't match. Please try again."
	msgThrottled      = "Too many sign-in attempts. Please try again later."
	// loginCookiePath scopes the per-sign-in login cookie to every route a
	// sign-in passes through: the page, each provider and Google's callback.
	loginCookiePath = "/auth/"
)

// buttonProvider is a sign-in provider the page offers as a button. Adding
// a provider is one more entry here plus its own start and callback
// routes; the page and the shared completion do not change.
type buttonProvider struct {
	ID, Label, StartPath string
}

// buttonProviders are the providers the sign-in page offers, in order.
var buttonProviders = []buttonProvider{
	{ID: "google", Label: "Sign in with Google", StartPath: googleStartPath},
}

// signinRoutes registers the sign-in page and the provider start routes.
func (a *Auth) signinRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+signinPath, a.serveSignin)
	mux.HandleFunc("GET "+googleStartPath, a.serveGoogleStart)
	mux.HandleFunc("POST "+passwordPath, a.servePassword)
}

// startSignIn records p as a sign-in in progress, ties it to this browser
// with the login cookie, and sends the browser to the sign-in page, where
// the person chooses how to sign in. Both ways a sign-in starts (an
// assistant connecting, a signed-out browser opening the board) come here.
func (a *Auth) startSignIn(w http.ResponseWriter, r *http.Request, p *pendingLogin) {
	state := a.st.addPending(a.now(), p)
	http.SetCookie(w, &http.Cookie{
		Name:     loginCookieName(state),
		Value:    state,
		Path:     loginCookiePath,
		MaxAge:   int(pendingLifetime / time.Second),
		HttpOnly: true,
		Secure:   a.secure(),
		SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, signinPath+"?state="+url.QueryEscape(state), http.StatusFound)
}

// clearLoginCookie removes the login cookie for state, at its current path
// and at the callback-only path sign-ins started before the page used.
func (a *Auth) clearLoginCookie(w http.ResponseWriter, state string) {
	for _, path := range []string{loginCookiePath, googleCallbackPath} {
		http.SetCookie(w, &http.Cookie{Name: loginCookieName(state), Path: path, MaxAge: -1, HttpOnly: true, Secure: a.secure(), SameSite: http.SameSiteLaxMode})
	}
}

// ownsState reports whether the request's browser started the sign-in
// named by state: its login cookie for that state carries it.
func ownsState(r *http.Request, state string) bool {
	if state == "" {
		return false
	}
	c, err := r.Cookie(loginCookieName(state))
	return err == nil && c.Value == state
}

// pendingFor is the guard every sign-in route opens with: the state must
// be present, belong to this browser, and name a live sign-in in progress.
// Otherwise it shows the "start signing in again" page and returns nil.
// The sign-in in progress is not used up.
func (a *Auth) pendingFor(w http.ResponseWriter, r *http.Request, state string) *pendingLogin {
	if !ownsState(r, state) {
		log.Printf("hostauth: refused %s: state does not match this browser's sign-in", r.URL.Path)
		errorPage(w, http.StatusBadRequest, "This sign-in has expired or was opened in a different browser. Please start signing in again.")
		return nil
	}
	p := a.st.peekPending(a.now(), state)
	if p == nil {
		log.Printf("hostauth: refused %s: no sign-in in progress for its state", r.URL.Path)
		errorPage(w, http.StatusBadRequest, "This sign-in has expired or was already used. Please start signing in again.")
		return nil
	}
	return p
}

// serveSignin shows the sign-in page for this browser's sign-in in
// progress.
func (a *Auth) serveSignin(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if a.pendingFor(w, r, state) == nil {
		return
	}
	renderSignin(w, http.StatusOK, signinData{State: state, Password: a.cfg.PasswordAccounts.Len() > 0})
}

// servePassword takes the sign-in page's username and password form for
// this browser's sign-in in progress. A throttled attempt is refused
// before the password is checked; a wrong username and a wrong password
// get the same page; neither uses up the sign-in, so the person can try
// again. A configured account finishes the sign-in as Google's does.
// Nothing about the attempt (username or password) is logged or recorded.
func (a *Auth) servePassword(w http.ResponseWriter, r *http.Request) {
	accounts := a.cfg.PasswordAccounts
	if accounts.Len() == 0 {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPasswordForm)
	if err := r.ParseForm(); err != nil {
		errorPage(w, http.StatusBadRequest, "This sign-in could not be read. Please start signing in again.")
		return
	}
	state := r.PostForm.Get("state")
	if a.pendingFor(w, r, state) == nil {
		return
	}
	user, pass := r.PostForm.Get("username"), r.PostForm.Get("password")
	now := a.now()
	client := a.clientAddr(r)
	again := signinData{State: state, Password: true, Username: user}
	if ok, _ := a.loginAllowed(now, user, client); !ok {
		log.Printf("hostauth: refused password sign-in: too many failed attempts")
		a.cfg.Recorder.RecordContext(r.Context(), telemetry.Event{Name: telemetry.SignInRefused, Reason: "throttled", Provider: ProviderPassword})
		again.Error = msgThrottled
		renderSignin(w, http.StatusTooManyRequests, again)
		return
	}
	acct, ok := accounts.Check(user, pass)
	if !ok {
		a.loginFailed(now, user, client)
		log.Printf("hostauth: refused password sign-in: username and password do not match")
		a.cfg.Recorder.RecordContext(r.Context(), telemetry.Event{Name: telemetry.SignInRefused, Reason: "credentials", Provider: ProviderPassword})
		again.Error = msgBadCredentials
		renderSignin(w, http.StatusUnauthorized, again)
		return
	}
	p := a.st.takePending(now, state)
	if p == nil {
		errorPage(w, http.StatusBadRequest, "This sign-in has expired or was already used. Please start signing in again.")
		return
	}
	a.clearLoginCookie(w, state)
	a.finishSignIn(w, r, p, acct)
}

// serveGoogleStart sends this browser's sign-in in progress to Google.
func (a *Auth) serveGoogleStart(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	p := a.pendingFor(w, r, state)
	if p == nil {
		return
	}
	a.startGoogle(w, r, state, p)
}

// finishSignIn ends every provider's sign-in once it knows the account: it
// applies the user cap and starts the board session (signIn), then
// continues the flow that started the sign-in, returning a code to the
// assistant or the browser to the board page it asked for.
func (a *Auth) finishSignIn(w http.ResponseWriter, r *http.Request, p *pendingLogin, acct Account) {
	if err := a.signIn(r.Context(), w, acct); err != nil {
		a.cfg.Recorder.RecordContext(r.Context(), telemetry.Event{Name: telemetry.SignInRefused, Email: acct.PlayerKey(), Reason: err.Error()})
		var b busyError
		if errors.As(err, &b) {
			busyPage(w, b.max)
			return
		}
		errorPage(w, http.StatusServiceUnavailable, "Sign-in could not be completed right now. Please try again later.")
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
