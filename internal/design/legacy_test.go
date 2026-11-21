package design

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// legacyMark opened the usage note of every token kept only for the
// website while its pages still used names from the token step. The
// website is restyled and those tokens are gone; a token marked this way
// must not come back.
const legacyMark = "Legacy name"

// retiredNames are the token-step names the Studio redesign retired, once
// neither the board nor the website used them. Neither app may name them
// again: each has a Studio name (see tokens.json), such as mat-surface for
// felt, card-face for card-text-face, or the Studio card back's brand-navy
// and brand-cyan for card-back-*.
var retiredNames = []string{
	"felt", "strip", "input", "text", "slot-line", "p1", "p2", "on-color",
	"card-back-light", "card-back-band", "card-back-red", "card-back-blue", "card-back-edge",
	"card-text-face", "card-text-face-end", "card-text-ink", "card-text-edge", "card-text-edge-trainer",
	"card-text-face-trainer", "card-text-face-trainer-end", "card-text-hp",
	"energy-special", "on-felt", "link-light", "button-ink", "log-line", "coach-bg", "sweep",
}

// legacyNames returns the tokens whose usage marks them legacy.
func legacyNames(t Tokens) map[string]bool {
	out := map[string]bool{}
	for _, tok := range t.all() {
		if strings.HasPrefix(tok.Usage, legacyMark) {
			out[tok.Name] = true
		}
	}
	return out
}

// legacyRefs returns, as "file:line: --name", every var(--name) in files
// (keyed by display path) that names a legacy token.
func legacyRefs(files map[string]string, legacy map[string]bool) []string {
	var hits []string
	for file, src := range files {
		for i, line := range strings.Split(src, "\n") {
			for _, m := range varRef.FindAllStringSubmatch(line, -1) {
				if legacy[m[1]] {
					hits = append(hits, fmt.Sprintf("%s:%d: --%s", file, i+1, m[1]))
				}
			}
		}
	}
	sort.Strings(hits)
	return hits
}

// sources reads every file under dir (relative to the repo root) with one
// of exts, keyed by its slash path from the root, skipping node_modules.
func sources(t *testing.T, dir string, exts ...string) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..")
	files := map[string]string{}
	err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if d.IsDir() || !hasExt(p, exts) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func hasExt(p string, exts []string) bool {
	for _, e := range exts {
		if strings.HasSuffix(p, e) {
			return true
		}
	}
	return false
}

// TestNoLegacyTokens: tokens.json holds Studio names only. No token is
// marked legacy and no retired name is defined again.
func TestNoLegacyTokens(t *testing.T) {
	toks, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for name := range legacyNames(toks) {
		t.Errorf("tokens.json: --%s is marked %q: the legacy names are retired, give it a Studio name", name, legacyMark)
	}
	retired := map[string]bool{}
	for _, n := range retiredNames {
		retired[n] = true
	}
	for _, tok := range toks.all() {
		if retired[tok.Name] {
			t.Errorf("tokens.json: --%s is a retired token-step name: use its Studio name", tok.Name)
		}
	}
}

// TestAppsUseNoLegacyTokens: the board (web/src) and the website
// (site/src) style themselves with Studio token names only: neither names
// a retired token or one marked legacy.
func TestAppsUseNoLegacyTokens(t *testing.T) {
	toks, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	legacy := legacyNames(toks)
	for _, n := range retiredNames {
		legacy[n] = true
	}
	for _, app := range []struct {
		dir  string
		exts []string
		min  int
	}{
		{"web/src", []string{".svelte", ".css", ".js"}, 10},
		{"site/src", []string{".astro", ".css", ".ts", ".js"}, 10},
	} {
		files := sources(t, app.dir, app.exts...)
		if len(files) < app.min {
			t.Fatalf("found only %d sources under %s: the check would pass by finding nothing", len(files), app.dir)
		}
		for _, h := range legacyRefs(files, legacy) {
			t.Errorf("%s is a retired legacy token: use its Studio name (see internal/design/tokens.json)", h)
		}
	}
}

// TestLegacyRefsFinder: the finder reports a legacy name with its place
// and lets Studio names pass.
func TestLegacyRefsFinder(t *testing.T) {
	legacy := map[string]bool{"felt": true, "p1": true}
	got := legacyRefs(map[string]string{
		"web/src/a.css":       ".a { color: var(--ink); }\n.b { background: var(--felt); }\n",
		"web/src/B.svelte":    "<style>.c { color: var(--p1, var(--seat-p1)); border-color: var(--p1-mat); }</style>\n",
		"site/src/C.astro":    "<style>.d { background: var(--mat-surface); color: var( --felt); }</style>\n",
		"site/src/pages/e.ts": "const ok = 'var(--seat-p1-mat)';\n",
	}, legacy)
	want := []string{"site/src/C.astro:1: --felt", "web/src/B.svelte:1: --p1", "web/src/a.css:2: --felt"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("legacyRefs = %v, want %v", got, want)
	}
}
