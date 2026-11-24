package hostauth

import (
	"fmt"
	"html/template"
	"net/http"

	"github.com/nicholasjackson/tcgreplay/internal/design"
)

// pageStyle is the server-rendered pages' own stylesheet. It uses only the
// shared design tokens (inlined before it), in the light interface: the
// website has no dark theme. The font stack names IBM Plex Sans but loads
// nothing, so the pages request nothing from another site.
const pageStyle = `
body { margin: 0; padding: var(--space-10) var(--space-4); background: var(--bg); color: var(--ink); font-family: var(--font-sans); font-size: var(--fs-body); line-height: 1.5; }
main { max-width: 24rem; margin: 0 auto; background: var(--panel); border: 1px solid var(--line); border-radius: var(--radius-l); padding: var(--space-8) var(--space-6); }
header { display: flex; align-items: center; gap: var(--space-3); margin-bottom: var(--space-6); }
header svg { width: 40px; height: 40px; flex: none; }
header span { font-weight: 600; font-size: var(--fs-card-title); color: var(--primary); }
h1 { font-size: var(--fs-heading); line-height: 1.25; margin: 0 0 var(--space-4); }
p { margin: 0 0 var(--space-4); color: var(--ink-soft); }
a { color: var(--primary); }
.providers { display: grid; gap: var(--space-3); margin: var(--space-6) 0 0; }
.provider { display: flex; align-items: center; justify-content: center; gap: var(--space-3); min-height: var(--touch-target); padding: 0 var(--space-4); border: 1px solid var(--control-line); border-radius: var(--radius-s); background: var(--panel); color: var(--ink); font-weight: 500; text-decoration: none; }
.provider:hover { background: var(--hover); }
.provider:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
.provider svg { width: 18px; height: 18px; flex: none; }
.divider { display: flex; align-items: center; gap: var(--space-3); margin: var(--space-6) 0 var(--space-4); color: var(--muted); font-size: var(--fs-body-s); }
.divider::before, .divider::after { content: ""; flex: 1; border-top: 1px solid var(--line); }
form.password { display: grid; gap: var(--space-2); }
form.password label { font-size: var(--fs-body-s); font-weight: 500; color: var(--ink-soft); }
form.password input { min-height: var(--touch-target); padding: 0 var(--space-3); border: 1px solid var(--control-line); border-radius: var(--radius-s); background: var(--panel); color: var(--ink); font: inherit; }
form.password input:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
form.password button { min-height: var(--touch-target); margin-top: var(--space-2); border: 0; border-radius: var(--radius-s); background: var(--primary); color: var(--on-primary); font: inherit; font-weight: 600; cursor: pointer; }
.note { margin: var(--space-4) 0 0; font-size: var(--fs-body-s); color: var(--muted); }
.error { padding: var(--space-3) var(--space-4); border: 1px solid var(--danger-line); border-radius: var(--radius-s); background: var(--danger-bg); color: var(--danger); }
`

// googleMark is Google's "G", drawn in Google's own colours as its sign-in
// branding asks.
const googleMark = `<svg viewBox="0 0 48 48" aria-hidden="true"><path fill="#EA4335" d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z"/><path fill="#4285F4" d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z"/><path fill="#FBBC05" d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z"/><path fill="#34A853" d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z"/></svg>`

// layout wraps every server-rendered page: the tokens, the page style and
// the header with the TCG Replay mark. Each page defines "body".
const layout = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{template "title" .}} · TCG Replay</title>
<style>{{.Tokens}}</style>
<style>{{.Style}}</style>
</head>
<body>
<main>
<header>{{.Mark}}<span>TCG Replay</span></header>
{{template "body" .}}
</main>
</body>
</html>
`

// page is the template every problem, busy and signed-out page uses.
var page = template.Must(template.Must(template.New("page").Parse(layout)).Parse(`
{{define "title"}}{{.Title}}{{end}}
{{define "body"}}<h1>{{.Title}}</h1>
<p>{{.Message}}</p>
{{if .Link}}<p><a href="{{.Link}}">{{.LinkText}}</a></p>{{end}}{{end}}
`))

// signinPage is the TCG Replay sign-in page: the provider buttons, Google
// first, then any further way in. It offers no sign-up, password change or
// recovery.
var signinPage = template.Must(template.Must(template.New("signin").Parse(layout)).Parse(`
{{define "title"}}Sign in{{end}}
{{define "body"}}<h1>Sign in</h1>
{{if .Error}}<p class="error" role="alert">{{.Error}}</p>{{end}}
<div class="providers">
{{range .Providers}}<a class="provider" data-provider="{{.ID}}" href="{{.StartPath}}?state={{$.State}}">{{if eq .ID "google"}}{{$.GoogleMark}}{{end}}{{.Label}}</a>
{{end}}</div>
{{if .Password}}<div class="divider">or</div>
<form class="password" method="POST" action="/auth/password">
<input type="hidden" name="state" value="{{.State}}">
<label for="username">Username</label>
<input id="username" name="username" type="text" autocomplete="username" autocapitalize="none" spellcheck="false" required value="{{.Username}}">
<label for="password">Password</label>
<input id="password" name="password" type="password" autocomplete="current-password" required>
<button type="submit">Sign in</button>
</form>
<p class="note">This form is for reviewer and invited accounts. Players sign in with Google.</p>{{end}}{{end}}
`))

// chrome is what the layout needs on every page.
type chrome struct {
	Tokens template.CSS
	Style  template.CSS
	Mark   template.HTML
}

var pageChrome = chrome{
	Tokens: template.CSS(design.TokensCSS),
	Style:  template.CSS(pageStyle),
	Mark:   template.HTML(design.MarkSVG),
}

// signinData is what the sign-in page shows for one sign-in in progress.
type signinData struct {
	chrome
	State      string
	Error      string
	Providers  []buttonProvider
	GoogleMark template.HTML
	// Password shows the username and password form (password accounts
	// are configured); Username refills it after a failed attempt.
	Password bool
	Username string
}

// pageHeaders are sent with every server-rendered page: not cached, not
// framed by another site, and allowed to load nothing from anywhere.
func pageHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
}

// renderSignin shows the sign-in page.
func renderSignin(w http.ResponseWriter, status int, d signinData) {
	if d.Providers == nil {
		d.Providers = buttonProviders
	}
	d.chrome = pageChrome
	d.GoogleMark = template.HTML(googleMark)
	pageHeaders(w)
	w.WriteHeader(status)
	signinPage.Execute(w, d)
}

type pageData struct {
	chrome
	Title, Message, Link, LinkText string
}

func renderPage(w http.ResponseWriter, status int, d pageData) {
	d.chrome = pageChrome
	pageHeaders(w)
	w.WriteHeader(status)
	page.Execute(w, d)
}

// errorPage shows a sign-in problem to the user in the browser.
func errorPage(w http.ResponseWriter, status int, msg string) {
	renderPage(w, status, pageData{Title: "Sign-in problem", Message: msg, Link: "/auth/login?next=/board", LinkText: "Sign in again"})
}

// busyPage tells a new account that the server is at its user cap.
func busyPage(w http.ResponseWriter, maxUsers int) {
	renderPage(w, http.StatusServiceUnavailable, pageData{
		Title:   "tcgreplay is busy",
		Message: fmt.Sprintf("tcgreplay is serving as many signed-in users as it can (%d) right now. Please try again later.", maxUsers),
	})
}
