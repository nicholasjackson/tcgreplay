package hostauth

import (
	"fmt"
	"html/template"
	"net/http"
)

// page is the one small template every sign-in page uses.
var page = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · tcgreplay</title>
<style>
body { font: 16px/1.5 system-ui, sans-serif; margin: 0; padding: 3rem 1rem; background: #f6f6f4; color: #222; }
main { max-width: 32rem; margin: 0 auto; }
a { color: #1a5fb4; }
@media (prefers-color-scheme: dark) { body { background: #1c1c1c; color: #e6e6e6; } a { color: #8ab4f8; } }
</style>
</head>
<body>
<main>
<h1>{{.Title}}</h1>
<p>{{.Message}}</p>
{{if .Link}}<p><a href="{{.Link}}">{{.LinkText}}</a></p>{{end}}
</main>
</body>
</html>
`))

type pageData struct {
	Title, Message, Link, LinkText string
}

func renderPage(w http.ResponseWriter, status int, d pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	page.Execute(w, d)
}

// errorPage shows a sign-in problem to the user in the browser.
func errorPage(w http.ResponseWriter, status int, msg string) {
	renderPage(w, status, pageData{Title: "Sign-in problem", Message: msg, Link: "/auth/login?next=/", LinkText: "Sign in again"})
}

// busyPage tells a new account that the server is at its user cap.
func busyPage(w http.ResponseWriter, maxUsers int) {
	renderPage(w, http.StatusServiceUnavailable, pageData{
		Title:   "tcgreplay is busy",
		Message: fmt.Sprintf("tcgreplay is serving as many signed-in users as it can (%d) right now. Please try again later.", maxUsers),
	})
}
