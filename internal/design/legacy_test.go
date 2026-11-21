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

// legacyMark opens the usage note of every token kept only for the
// website: a name from the token step that no board style may use.
const legacyMark = "Legacy name"

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

// TestBoardUsesNoLegacyTokens: the board (web/src) styles itself with
// Studio token names only. The legacy names left in tokens.json are the
// website's until its pages are restyled.
func TestBoardUsesNoLegacyTokens(t *testing.T) {
	toks, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	legacy := legacyNames(toks)
	files := map[string]string{}
	web := filepath.Join("..", "..", "web", "src")
	err = filepath.WalkDir(web, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if d.IsDir() || !(strings.HasSuffix(p, ".svelte") || strings.HasSuffix(p, ".css") || strings.HasSuffix(p, ".js")) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(filepath.Join("..", ".."), p)
		files[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 10 {
		t.Fatalf("found only %d board sources under web/src: the check would pass by finding nothing", len(files))
	}
	for _, h := range legacyRefs(files, legacy) {
		t.Errorf("%s is a legacy token: use its Studio name (see its usage note in internal/design/tokens.json)", h)
	}
}

// TestLegacyRefsFinder: the finder reports a legacy name with its place
// and lets Studio names pass.
func TestLegacyRefsFinder(t *testing.T) {
	legacy := map[string]bool{"felt": true, "p1": true}
	got := legacyRefs(map[string]string{
		"web/src/a.css":    ".a { color: var(--ink); }\n.b { background: var(--felt); }\n",
		"web/src/B.svelte": "<style>.c { color: var(--p1, var(--seat-p1)); border-color: var(--p1-mat); }</style>\n",
	}, legacy)
	want := []string{"web/src/B.svelte:1: --p1", "web/src/a.css:2: --felt"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("legacyRefs = %v, want %v", got, want)
	}
}
