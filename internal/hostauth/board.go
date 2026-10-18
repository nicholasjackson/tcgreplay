package hostauth

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// boardRoutes registers the board's sign-in routes.
func (a *Auth) boardRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", a.serveLogin)
	mux.HandleFunc("GET /auth/session", a.serveSession)
	mux.HandleFunc("POST /auth/logout", a.serveLogout)
	mux.HandleFunc("GET /auth/signed-out", serveSignedOut)
}

// serveLogin starts a board sign-in that returns to the local path next.
func (a *Auth) serveLogin(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Query().Get("next")
	if !localPath(next) {
		next = "/"
	}
	a.startGoogle(w, r, &pendingLogin{kind: loginBoard, next: next})
}

// sessionFor returns the board session the request's cookie names, or nil.
func (a *Auth) sessionFor(r *http.Request) (string, *session) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return "", nil
	}
	return c.Value, a.st.session(c.Value)
}

// liveRequest reports whether a board request is a live connection or a
// data request, which are refused rather than sent to sign in.
func liveRequest(r *http.Request) bool {
	return r.URL.Path == "/ws" || strings.HasPrefix(r.URL.Path, "/api/")
}

// RequireSession guards the board. A signed-out request for a page is sent
// through Google sign-in and back to the page; a signed-out live
// connection or data request is refused with 401. A signed-in request
// carries its account (see AccountFrom), and a live connection ends as
// soon as its session does. Data requests count against the account's
// request limit and get 429 over it.
func (a *Auth) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, s := a.sessionFor(r)
		if s == nil {
			if liveRequest(r) || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
				log.Printf("hostauth: refused %s %s: not signed in", r.Method, r.URL.Path)
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, "sign in required", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			http.Redirect(w, r, "/auth/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if err := a.allow(s.account); err != nil {
				http.Error(w, err.Error(), http.StatusTooManyRequests)
				return
			}
		}
		ctx := withAccount(r.Context(), s.account)
		if r.URL.Path == "/ws" {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			defer cancel()
			go func() {
				select {
				case <-s.done:
					cancel()
				case <-ctx.Done():
				}
			}()
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// sessionInfo is what the board reads about its sign-in.
type sessionInfo struct {
	Email string `json:"email"`
}

// serveSession tells the board who is signed in, or 401.
func (a *Auth) serveSession(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	_, s := a.sessionFor(r)
	if s == nil {
		writeJSON(w, http.StatusUnauthorized, oauthError{Error: "signed_out", Description: "not signed in"})
		return
	}
	writeJSON(w, http.StatusOK, sessionInfo{Email: s.account.Email})
}

// sameSite reports whether a request was sent by a page on the public
// address, judged by its Origin, or Referer when there is no Origin.
func (a *Auth) sameSite(r *http.Request) bool {
	from := r.Header.Get("Origin")
	if from == "" || from == "null" {
		from = r.Header.Get("Referer")
	}
	u, err := url.Parse(from)
	return err == nil && from != "" && u.Host == a.cfg.Public.Host
}

// serveLogout ends this browser's board session, closing its live
// connections, and shows the signed-out page. It signs out of the board
// only: the connector's tokens are disconnected in claude.ai.
func (a *Auth) serveLogout(w http.ResponseWriter, r *http.Request) {
	if !a.sameSite(r) {
		http.Error(w, "sign out must come from the playmat", http.StatusForbidden)
		return
	}
	if id, s := a.sessionFor(r); s != nil {
		a.st.endSession(id)
		log.Printf("hostauth: signed out %s", s.account.Email)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.secure(), SameSite: http.SameSiteLaxMode})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/auth/signed-out", http.StatusSeeOther)
}

func serveSignedOut(w http.ResponseWriter, r *http.Request) {
	renderPage(w, http.StatusOK, pageData{
		Title:    "Signed out",
		Message:  "You have signed out of tcgreplay.",
		Link:     "/auth/login?next=/",
		LinkText: "Sign in again",
	})
}
