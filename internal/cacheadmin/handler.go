package cacheadmin

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
)

// Handler serves the admin page (GET /admin), the image clear (POST
// /admin/clear, a form with all=1 or set=<code or id>) and the check for
// published card sets (POST /admin/check). It must be served behind
// hostauth.RequireAdmin, which supplies the admin's account and refuses
// everyone else and cross-site posts.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		s.render(w, http.StatusOK, email(r), "")
	})
	mux.HandleFunc("POST /admin/clear", s.serveClear)
	mux.HandleFunc("POST /admin/check", s.serveCheck)
	return mux
}

// serveCheck checks for published card sets at once and sends the admin
// back to the page, which shows the outcome, or shows the page with why
// the check failed.
func (s *Service) serveCheck(w http.ResponseWriter, r *http.Request) {
	if _, err := s.CheckNow(r.Context(), email(r)); err != nil {
		s.render(w, http.StatusBadGateway, email(r), "The check for published card sets failed: "+err.Error()+". The loaded card sets are unchanged.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// email is the signed-in admin's email.
func email(r *http.Request) string {
	acct, _ := hostauth.AccountFrom(r.Context())
	return acct.Email
}

// serveClear starts a clear and sends the admin back to the page, or shows
// the page with why it did not start.
func (s *Service) serveClear(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render(w, http.StatusBadRequest, email(r), "That request could not be read.")
		return
	}
	all := r.PostForm.Get("all") == "1"
	set := strings.TrimSpace(r.PostForm.Get("set"))
	if !all && set == "" {
		s.render(w, http.StatusBadRequest, email(r), "Name a set by its code (such as MEG) or id (such as me01).")
		return
	}
	_, err := s.Start(r.Context(), email(r), set, all)
	switch {
	case err == nil:
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	case errors.Is(err, cards.ErrUnknownSet):
		s.render(w, http.StatusBadRequest, email(r), fmt.Sprintf("No set is named %q. Nothing was cleared.", set))
	case errors.Is(err, ErrBusy):
		s.render(w, http.StatusConflict, email(r), "A clear is already running. Wait for it to finish, then try again.")
	case errors.Is(err, ErrNoImages):
		s.render(w, http.StatusConflict, email(r), "There are no image copies to clear: the shared store is off.")
	default:
		s.render(w, http.StatusBadGateway, email(r), "Could not start the clear: "+err.Error()+". Nothing was cleared.")
	}
}
