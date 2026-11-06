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
<title>Card cache · tcgreplay</title>
<style>
body { font: 16px/1.5 system-ui, sans-serif; margin: 0; padding: 3rem 1rem; background: #f6f6f4; color: #222; }
main { max-width: 32rem; margin: 0 auto; }
a { color: #1a5fb4; }
form { margin: 1rem 0; }
input { font: inherit; padding: .3rem .5rem; }
button { font: inherit; padding: .3rem .8rem; }
.msg { padding: .5rem .8rem; border-left: 4px solid #c01c28; background: #fbe9eb; }
.failed { color: #c01c28; }
@media (prefers-color-scheme: dark) { body { background: #1c1c1c; color: #e6e6e6; } a { color: #8ab4f8; } .msg { background: #3a1d20; } .failed { color: #f28b82; } }
</style>
</head>
<body>
<main>
<h1>Card cache</h1>
<p>Signed in as {{.Email}}.</p>
{{if .Message}}<p class="msg" role="alert">{{.Message}}</p>{{end}}
<h2>Clear everything</h2>
<p>Forgets all cached card data and image copies. Open games keep their cards while they are fetched again.</p>
<form method="post" action="/admin/clear"><input type="hidden" name="all" value="1"><button>Clear everything</button></form>
<h2>Clear one set</h2>
<form method="post" action="/admin/clear"><label>Set code or id <input name="set" placeholder="MEG or me01" required></label> <button>Clear set</button></form>
<h2>{{if .Running}}Clearing now{{else}}Last clear{{end}}</h2>
{{with .Job}}{{if $.Running}}<p>Clearing {{.Label}} for {{.By}}, started {{$.Started}}…</p>{{else}}<p>{{.Label}} by {{.By}} at {{$.Started}}, took {{$.Took}}{{if .Failed}}, <span class="failed">with failures</span>{{end}}.</p>
<ul>{{range $.Tiers}}<li{{if .Failed}} class="failed"{{end}}>{{.Text}}</li>{{end}}</ul>{{end}}{{else}}<p>No clear since the playmat started.</p>{{end}}
<p><a href="/board">Back to the board</a></p>
</main>
</body>
</html>
`))

type tierLine struct {
	Text   string
	Failed bool
}

type pageData struct {
	Email, Message string
	Job            *Job
	Running        bool
	Started, Took  string
	Tiers          []tierLine
}

// tierNames are how each tier is named on the page.
var tierNames = map[string]string{
	"memory": "card entries marked for refetch in memory",
	"disk":   "card entries removed from local disk",
	"shared": "card entries removed from the shared store",
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
	d := pageData{Email: email, Message: msg}
	if j, ok := s.Status(); ok {
		d.Job, d.Running = &j, j.Running
		d.Started = j.Started.UTC().Format("2006-01-02 15:04:05 UTC")
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
