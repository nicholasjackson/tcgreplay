package usergames

import (
	"context"
	"errors"
	"html/template"
	"log"
	"net/http"

	"github.com/nicholasjackson/tcgreplay/internal/hostauth"
)

// Board returns the hosted board's front door, to mount behind
// hostauth.RequireSession. It sends each signed-in browser to its own
// account's board server, so a tab only ever reads and receives its
// owner's game:
//   - a page load (GET / or /index.html) counts as activity, and a
//     ?board= handle that is not the signed-in account's own gets the
//     "not your game" page and nothing else;
//   - the live connection (/ws) is not activity, and ends when the
//     account's entry is removed;
//   - everything else (assets, card lookups) is not activity.
func (r *Registry) Board() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		acct, ok := hostauth.AccountFrom(req.Context())
		if !ok {
			// RequireSession has already refused this; be safe anyway.
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "sign in required", http.StatusUnauthorized)
			return
		}
		if pageLoad(req) {
			if h := req.URL.Query().Get("board"); h != "" && h != Handle(acct.ID) {
				log.Printf("usergames: %s opened another account's board link", acct.Email)
				notYoursPage(w, acct)
				return
			}
			e, err := r.Use(acct)
			if err != nil {
				refuse(w, err)
				return
			}
			e.board.ServeHTTP(w, req)
			return
		}
		e := r.Get(acct)
		if req.URL.Path == "/ws" {
			ctx, cancel := untilDone(req.Context(), e.ctx)
			defer cancel()
			req = req.WithContext(ctx)
		}
		e.board.ServeHTTP(w, req)
	})
}

// BoardLimit returns the Limit hook for a per-account board server: it
// runs next (the sign-in service's request limit) and, if that passes,
// counts the board action as its account's activity.
func (r *Registry) BoardLimit(next func(ctx context.Context) error) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if next != nil {
			if err := next(ctx); err != nil {
				return err
			}
		}
		acct, ok := hostauth.AccountFrom(ctx)
		if !ok {
			return nil
		}
		_, err := r.Use(acct)
		return err
	}
}

// pageLoad reports whether req loads the board page itself.
func pageLoad(req *http.Request) bool {
	return (req.Method == http.MethodGet || req.Method == http.MethodHead) &&
		(req.URL.Path == "/" || req.URL.Path == "/index.html")
}

// untilDone returns ctx, cancelled also when done is.
func untilDone(ctx, done context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-done.Done():
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// refuse answers a page load the registry turned away.
func refuse(w http.ResponseWriter, err error) {
	w.Header().Set("Cache-Control", "no-store")
	status := http.StatusInternalServerError
	if errors.Is(err, hostauth.ErrBusy) {
		status = http.StatusServiceUnavailable
	}
	http.Error(w, err.Error(), status)
}

// notYours is the page shown for another account's board link: no board
// app and nothing of the game.
var notYours = template.Must(template.New("notyours").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Not your game · tcgreplay</title>
<style>
body { font: 16px/1.5 system-ui, sans-serif; margin: 0; padding: 3rem 1rem; background: #f6f6f4; color: #222; }
main { max-width: 32rem; margin: 0 auto; }
a { color: #1a5fb4; }
button { font: inherit; padding: 0.4rem 1rem; }
@media (prefers-color-scheme: dark) { body { background: #1c1c1c; color: #e6e6e6; } a { color: #8ab4f8; } }
</style>
</head>
<body>
<main>
<h1>Not your game</h1>
<p>This board belongs to another account. You are signed in as {{.}}.</p>
<p><a href="/">Open your own board</a></p>
<form method="post" action="/auth/logout"><button type="submit">Sign out</button></form>
</main>
</body>
</html>
`))

// notYoursPage shows the "not your game" page with status 403.
func notYoursPage(w http.ResponseWriter, acct hostauth.Account) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	notYours.Execute(w, acct.Email)
}
