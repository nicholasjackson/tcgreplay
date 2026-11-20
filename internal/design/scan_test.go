package design

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestNoStrayDesignValues: the board (web/src) and the website (site/src)
// take every colour and every px or rem font size from the tokens. A
// hard-coded one fails here with its file, line and value.
func TestNoStrayDesignValues(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, dir := range []string{"web/src", "site/src"} {
		if _, err := os.Stat(filepath.Join(root, dir)); err != nil {
			t.Fatalf("%s is gone: the scan would pass by finding nothing (%v)", dir, err)
		}
	}
	stray, err := StrayValues(root, "web/src", "site/src")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stray {
		t.Errorf("%s is written outside the design tokens: add it to internal/design/tokens.json, run go generate ./internal/design, and use var(--name)", s)
	}
}

// TestStrayScanner: the scanner reports each kind of hard-coded value with
// its place, and lets token references, relative sizes, keywords and
// non-style text pass.
func TestStrayScanner(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("web/src/app.css", `/* #123456 and 12px in a comment */
body { color: #abc; font: 700 11px/18px system-ui; }
.ok { color: var(--text); font: 700 var(--fs-11)/18px var(--font-ui); background: transparent; }
#outside-services { white-space: nowrap; }
.shade { box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4); }
`)
	write("web/src/components/Thing.svelte", `<script>
  const swatch = '#ff0000';
</script>
<div style="color: #0f0" style:--seat="var(--p1)">#notastyle 14px</div>
<style>
  .a { font-size: 12px; }
  .b { font-size: 0.85em; border: 1px solid white; }
  .c { --fs: 13px; }
  .d { font-size: max(var(--fs-12), 0.625vw); color: var(--x, #fff); }
  .e { font-size: calc(var(--cw) * 0.15); }
</style>
`)
	write("site/src/pages/index.astro", `---
const x = '#999';
---
<p>Plain text with #fff and 2rem.</p>
<style>
  h1 { font-size: 1.6rem; }
</style>
`)
	write("site/src/content/docs/page.md", "color: #fff; font-size: 12px\n")
	write("web/src/node_modules/x/y.css", ".z { color: #fff; }\n")
	write("web/src/lib/thing.js", "el.style.color = '#fff';\n")

	stray, err := StrayValues(root, "web/src", "site/src")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range stray {
		got = append(got, s.String())
	}
	want := []string{
		"web/src/app.css:2: color: #abc",
		"web/src/app.css:2: font: 11px",
		"web/src/app.css:5: box-shadow: 0 2px 6px rgba(0, 0, 0, 0.4)",
		"web/src/components/Thing.svelte:4: color: #0f0",
		"web/src/components/Thing.svelte:6: font-size: 12px",
		"web/src/components/Thing.svelte:7: border: white",
		"web/src/components/Thing.svelte:8: --fs: 13px",
		"site/src/pages/index.astro:6: font-size: 1.6rem",
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("scanner reported:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
