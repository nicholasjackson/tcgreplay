// Package site serves the public website: static pages built by Astro from
// site/ into dist/app and embedded in the binary, the same way package
// webui embeds the board. When the site has not been built, a placeholder
// page is served instead, so the binary builds and tests without Node.
//
// The site is plain files. It needs no sign-in, never sets a cookie, sends
// pages with no-cache so a deploy shows at once, and caches Astro's hashed
// build assets under /_astro/ for a year.
package site

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

// dist holds the built website under dist/app (see site/astro.config.mjs)
// and a placeholder page used when the website has not been built.
//
//go:embed all:dist
var dist embed.FS

// fsys is the tree holding dist/app and dist/placeholder; tests swap it.
var fsys fs.FS = dist

// Cache-Control values: hashed build assets never change under the same
// name; everything else is revalidated on every load.
const (
	cacheImmutable = "public, max-age=31536000, immutable"
	cacheNone      = "no-cache"
)

// types names the content types the site uses, so they don't depend on the
// host's MIME tables (the distroless image has none).
var types = map[string]string{
	".html":        "text/html; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".json":        "application/json",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".jpeg":        "image/jpeg",
	".webp":        "image/webp",
	".avif":        "image/avif",
	".ico":         "image/x-icon",
	".txt":         "text/plain; charset=utf-8",
	".xml":         "application/xml",
	".webmanifest": "application/manifest+json",
}

// assets returns the built website, or the placeholder if it is missing.
func assets() fs.FS {
	if app, err := fs.Sub(fsys, "dist/app"); err == nil {
		if _, err := fs.Stat(app, "index.html"); err == nil {
			return app
		}
	}
	ph, _ := fs.Sub(fsys, "dist/placeholder")
	return ph
}

// Built reports whether the website was built into the binary, rather than
// only the placeholder.
func Built() bool {
	_, err := fs.Stat(fsys, "dist/app/index.html")
	return err == nil
}

// Handler serves the embedded website: pages, build assets and the site's
// 404 page. It never sets cookies and needs no sign-in.
func Handler() http.Handler {
	root := assets()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name, ok := resolve(root, r.URL.Path)
		if !ok {
			notFound(w, r, root)
			return
		}
		body, err := fs.ReadFile(root, name)
		if err != nil {
			notFound(w, r, root)
			return
		}
		h := w.Header()
		if strings.HasPrefix(name, "_astro/") {
			h.Set("Cache-Control", cacheImmutable)
		} else {
			h.Set("Cache-Control", cacheNone)
		}
		if ct := contentType(name); ct != "" {
			h.Set("Content-Type", ct)
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
	})
}

// resolve maps a request path to a file in root: "/" is index.html, and
// "/x" is the file x, else x/index.html (Astro's directory format), else
// x.html.
func resolve(root fs.FS, urlPath string) (string, bool) {
	name := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
	if name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) {
		return "", false
	}
	for _, c := range []string{name, name + "/index.html", name + ".html"} {
		if info, err := fs.Stat(root, c); err == nil && info.Mode().IsRegular() {
			return c, true
		}
	}
	return "", false
}

// notFound sends the site's own 404 page with status 404, or plain text
// when the site has none (the placeholder).
func notFound(w http.ResponseWriter, r *http.Request, root fs.FS) {
	body, err := fs.ReadFile(root, "404.html")
	if err != nil {
		w.Header().Set("Cache-Control", cacheNone)
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Cache-Control", cacheNone)
	h.Set("Content-Type", types[".html"])
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		w.Write(body)
	}
}

// contentType names a file's content type from its extension, or "" to let
// http.ServeContent sniff it.
func contentType(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if ct, ok := types[ext]; ok {
		return ct
	}
	return mime.TypeByExtension(ext)
}
