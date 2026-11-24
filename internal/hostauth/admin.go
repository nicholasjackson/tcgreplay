package hostauth

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/telemetry"
)

// IsAdmin reports whether acct may use the admin pages: it is a Google
// account, its email is one of Config.Admins, in any case, and Google
// marked it verified. A password account is never an admin, whatever its
// username.
func (a *Auth) IsAdmin(acct Account) bool {
	if acct.Password() {
		return false
	}
	return acct.EmailVerified && a.admins[strings.ToLower(strings.TrimSpace(acct.Email))]
}

// AdminCount is how many admins are named, for the start-up log.
func (a *Auth) AdminCount() int { return len(a.admins) }

// PasswordAccountCount is how many password accounts are configured, for
// the start-up log.
func (a *Auth) PasswordAccountCount() int { return a.cfg.PasswordAccounts.Len() }

// RequireAdmin guards the admin pages. A signed-out request for a page is
// sent through Google sign-in and back to the page; any other signed-out
// request is refused with 401. A signed-in account that is not an admin
// (see IsAdmin), including a sign-in from before the verified flag was
// kept, gets 403 and a page saying so. A request that changes something
// (anything but GET or HEAD) must come from the playmat's own pages,
// judged by the same Origin check sign-out uses, or it gets 403. An admin
// request carries its account (see AccountFrom).
func (a *Auth) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, s := a.sessionFor(r)
		read := r.Method == http.MethodGet || r.Method == http.MethodHead
		if s == nil {
			w.Header().Set("Cache-Control", "no-store")
			if !read || subresource(r) {
				log.Printf("hostauth: refused %s %s: not signed in", r.Method, r.URL.Path)
				http.Error(w, "sign in required", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/auth/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		}
		if !a.IsAdmin(s.account) {
			a.cfg.Recorder.RecordContext(r.Context(), telemetry.Event{Name: telemetry.AdminRefused, Email: s.account.PlayerKey(), Reason: "not admin"})
			msg := "This page is for tcgreplay admins. You are signed in as " + s.account.Email + "."
			if !s.account.EmailVerified && !s.account.Password() {
				msg += " If you are an admin, sign out and sign in again."
			}
			renderPage(w, http.StatusForbidden, pageData{Title: "Not an admin", Message: msg, Link: "/board", LinkText: "Back to the board"})
			return
		}
		if !read && !a.sameSite(r) {
			a.cfg.Recorder.RecordContext(r.Context(), telemetry.Event{Name: telemetry.AdminRefused, Email: s.account.PlayerKey(), Reason: "not sent from the playmat"})
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "admin changes must come from the playmat", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(withAccount(r.Context(), s.account)))
	})
}
