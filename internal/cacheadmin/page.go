package cacheadmin

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/nicholasjackson/tcgreplay/internal/cards"
)

// page is the admin page, in the style of the sign-in pages: no script,
// light and dark, and a refresh every 2 seconds only while a clear runs.
var page = template.Must(template.New("admin").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
{{if .Running}}<meta http-equiv="refresh" content="2">{{end}}
<title>Card sets and images · tcgreplay</title>
<style>
body { font: 16px/1.5 system-ui, sans-serif; margin: 0; padding: 3rem 1rem; background: #f6f6f4; color: #222; }
main { max-width: 32rem; margin: 0 auto; }
a { color: #1a5fb4; }
form { margin: 1rem 0; }
input { font: inherit; padding: .3rem .5rem; }
button { font: inherit; padding: .3rem .8rem; }
dl { display: grid; grid-template-columns: max-content 1fr; gap: .2rem 1rem; }
dt { color: #666; }
dd { margin: 0; overflow-wrap: anywhere; }
.msg { padding: .5rem .8rem; border-left: 4px solid #c01c28; background: #fbe9eb; }
.failed { color: #c01c28; }
@media (prefers-color-scheme: dark) { body { background: #1c1c1c; color: #e6e6e6; } a { color: #8ab4f8; } dt { color: #aaa; } .msg { background: #3a1d20; } .failed { color: #f28b82; } }
</style>
</head>
<body>
<main>
<h1>Card sets and images</h1>
<p>Signed in as {{.Email}}.</p>
{{if .Message}}<p class="msg" role="alert">{{.Message}}</p>{{end}}
<h2>Card sets</h2>
{{with .Catalog}}<dl>
<dt>Version</dt><dd>{{.Version}}</dd>
<dt>Sets</dt><dd>{{.Sets}} ({{.Prints}} cards)</dd>
<dt>Published at</dt><dd>{{.Base}}{{if .Offline}} (offline: local copy only){{end}}</dd>
<dt>Last check</dt><dd>{{.LastCheck}}</dd>
<dt>Last change</dt><dd>{{.LastChange}}</dd>
{{if .LastError}}<dt>Last error</dt><dd class="failed">{{.LastError}}</dd>{{end}}
</dl>{{else}}<p>No card catalogue.</p>{{end}}
<p>The playmat checks for published card sets every few minutes. Games already open keep the cards they started with; new and reopened games use the latest.</p>
<form method="post" action="/admin/check"><button>Check for published card sets now</button></form>
{{with .Check}}<p>Last check now by {{.By}} at {{.At}}: {{if .Failed}}<span class="failed">{{.Text}}</span>{{else}}{{.Text}}{{end}}</p>{{end}}
{{if .Images}}<h2>Clear images: everything</h2>
<p>Forgets every card image copy, so each is copied again from its source.</p>
<form method="post" action="/admin/clear"><input type="hidden" name="all" value="1"><button>Clear all images</button></form>
<h2>Clear images for one set</h2>
<form method="post" action="/admin/clear"><label>Set code or id <input name="set" placeholder="MEG or me01" required></label> <button>Clear images for set</button></form>
<h2>{{if .Running}}Clearing now{{else}}Last clear{{end}}</h2>
{{with .Job}}{{if $.Running}}<p>Clearing images for {{.Label}} for {{.By}}, started {{$.Started}}…</p>{{else}}<p>Images for {{.Label}} by {{.By}} at {{$.Started}}, took {{$.Took}}{{if .Failed}}, <span class="failed">with failures</span>{{end}}.</p>
<ul>{{range $.Tiers}}<li{{if .Failed}} class="failed"{{end}}>{{.Text}}</li>{{end}}</ul>{{end}}{{else}}<p>No clear since the playmat started.</p>{{end}}{{else}}<h2>Images</h2>
<p>Images are not copied: the shared store is off.</p>{{end}}
<p><a href="/board">Back to the board</a></p>
</main>
</body>
</html>
`))

// catalogLine is the loaded catalogue as the page shows it.
type catalogLine struct {
	Version, Base, LastCheck, LastChange, LastError string
	Sets, Prints                                    int
	Offline                                         bool
}

// checkLine is the last admin check as the page shows it.
type checkLine struct {
	By, At, Text string
	Failed       bool
}

type tierLine struct {
	Text   string
	Failed bool
}

type pageData struct {
	Email, Message string
	Catalog        *catalogLine
	Check          *checkLine
	Images         bool
	Job            *Job
	Running        bool
	Started, Took  string
	Tiers          []tierLine
}

// tierNames are how each tier is named on the page.
var tierNames = map[string]string{
	"images": "image copies removed",
}

// tierText describes one tier's report for the page.
func tierText(r cards.TierReport) tierLine {
	if r.Tier == "cdn" {
		switch {
		case errors.Is(r.Err, cards.ErrCDNNotConfigured):
			return tierLine{Text: "CDN not purged (no TCGREPLAY_SPACES_CDN_TOKEN)"}
		case r.Err != nil:
			return tierLine{Text: "CDN purge failed: " + r.Err.Error(), Failed: true}
		case r.Removed == 0:
			return tierLine{Text: "CDN: nothing to purge"}
		}
		return tierLine{Text: fmt.Sprintf("CDN purged (%d paths)", r.Removed)}
	}
	name := tierNames[r.Tier]
	if name == "" {
		name = r.Tier + " entries removed"
	}
	if r.Err != nil {
		return tierLine{Text: fmt.Sprintf("%d %s, then failed: %v", r.Removed, name, r.Err), Failed: true}
	}
	return tierLine{Text: fmt.Sprintf("%d %s", r.Removed, name)}
}

// render writes the page with status, for the admin email, the current or
// last job and msg (shown as an alert when not empty).
func (s *Service) render(w http.ResponseWriter, status int, email, msg string) {
	d := pageData{Email: email, Message: msg, Images: s.Images != nil}
	if s.Catalog != nil {
		st := s.Catalog.Status()
		c := &catalogLine{Version: short(st.Version), Base: st.Base, Sets: st.Sets, Prints: st.Prints, Offline: st.Offline,
			LastCheck: when(st.LastCheck), LastChange: when(st.LastChange), LastError: st.LastError}
		d.Catalog = c
	}
	if c, ok := s.LastCheck(); ok {
		l := &checkLine{By: c.By, At: when(c.At), Text: c.Result.String()}
		if c.Err != nil {
			l.Text, l.Failed = c.Err.Error(), true
		}
		d.Check = l
	}
	if j, ok := s.Status(); ok {
		d.Job, d.Running = &j, j.Running
		d.Started = when(j.Started)
		if !j.Running {
			d.Took = j.Ended.Sub(j.Started).Round(time.Millisecond).String()
		}
		for _, r := range j.Reports {
			d.Tiers = append(d.Tiers, tierText(r))
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	page.Execute(w, d)
}

// when formats a time for the page; "never" for none.
func when(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

// short is a catalogue version as the page shows it: its first 12 hex
// digits, or "none loaded".
func short(v string) string {
	switch {
	case v == "":
		return "none loaded"
	case len(v) > 12:
		return v[:12]
	}
	return v
}
