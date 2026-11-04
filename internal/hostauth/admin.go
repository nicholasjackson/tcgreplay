package hostauth

import (
	"log"
	"net/http"
	"net/url"
	"strings"
)

// IsAdmin reports whether acct may use the admin pages: its email is one
// of Config.Admins, in any case, and Google marked it verified.
func (a *Auth) IsAdmin(acct Account) bool {
	return acct.EmailVerified && a.admins[strings.ToLower(strings.TrimSpace(acct.Email))]
}

// AdminCount is how many admins are named, for the start-up log.
func (a *Auth) AdminCount() int { return len(a.admins) }

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
			log.Printf("hostauth: refused admin %s %s for %s", r.Method, r.URL.Path, s.account.Email)
			msg := "This page is for tcgreplay admins. You are signed in as " + s.account.Email + "."
			if !s.account.EmailVerified {
				msg += " If you are an admin, sign out and sign in again."
			}
			renderPage(w, http.StatusForbidden, pageData{Title: "Not an admin", Message: msg, Link: "/", LinkText: "Back to the board"})
			return
		}
		if !read && !a.sameSite(r) {
			log.Printf("hostauth: refused admin %s %s for %s: not sent from the playmat", r.Method, r.URL.Path, s.account.Email)
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "admin changes must come from the playmat", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(withAccount(r.Context(), s.account)))
	})
}
