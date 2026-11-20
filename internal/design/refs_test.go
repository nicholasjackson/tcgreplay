package design

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	// varRef matches a var(--name) reference. A name built from a Svelte
	// expression, such as var(--{seat}), does not match and is not checked.
	varRef = regexp.MustCompile(`var\(\s*--([A-Za-z0-9_-]+)`)
	// propDef matches a custom property assignment: `--name:` in a rule or
	// a style attribute, or Svelte's `style:--name=` directive.
	propDef = regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])--([A-Za-z0-9_-]+)\s*[:=]`)
)

// definedProps returns every custom property name assigned in src.
func definedProps(src string, into map[string]bool) {
	for _, m := range propDef.FindAllStringSubmatch(src, -1) {
		into[m[1]] = true
	}
}

// unresolvedRefs returns, as "file:line: --name", every var(--name) in
// files (keyed by display path) whose name is not in defined.
func unresolvedRefs(files map[string]string, defined map[string]bool) []string {
	var miss []string
	for file, src := range files {
		for i, line := range strings.Split(src, "\n") {
			for _, m := range varRef.FindAllStringSubmatch(line, -1) {
				if !defined[m[1]] {
					miss = append(miss, fmt.Sprintf("%s:%d: --%s", file, i+1, m[1]))
				}
			}
		}
	}
	slices.Sort(miss)
	return miss
}

// TestBoardVarsResolve: every var(--name) in the board's styles names a
// shared token from tokens.css or a property the board itself assigns
// (layout locals such as --cw, --ec, --fs), so a mistyped token name cannot
// silently drop a colour or size from a view the screenshots miss.
func TestBoardVarsResolve(t *testing.T) {
	root := filepath.Join("..", "..")
	defined := map[string]bool{}
	tokens, err := os.ReadFile("tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	definedProps(string(tokens), defined)

	web := filepath.Join(root, "web", "src")
	err = filepath.WalkDir(web, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch filepath.Ext(p) {
		case ".css", ".svelte", ".js":
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			definedProps(string(b), defined)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	styled := []string{"web/src/app.css", "web/src/App.svelte"}
	components, err := filepath.Glob(filepath.Join(web, "components", "*.svelte"))
	if err != nil {
		t.Fatal(err)
	}
	if len(components) == 0 {
		t.Fatal("found no board components under web/src/components")
	}
	for _, c := range components {
		rel, err := filepath.Rel(root, c)
		if err != nil {
			t.Fatal(err)
		}
		styled = append(styled, filepath.ToSlash(rel))
	}
	files := map[string]string{}
	refs := 0
	for _, f := range styled {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			t.Fatal(err)
		}
		files[f] = string(b)
		refs += len(varRef.FindAllString(string(b), -1))
	}
	if refs == 0 {
		t.Fatal("found no var(--name) references in the board's styles: the scan is looking in the wrong place")
	}
	for _, m := range unresolvedRefs(files, defined) {
		t.Errorf("%s is neither a token in internal/design/tokens.css nor a property assigned in web/src", m)
	}
}

// TestUnresolvedRefsResolver: the resolver reports a planted unknown name
// with its file and line, and accepts names defined as a token, a rule's
// local property, a style directive and a fallback-bearing reference.
func TestUnresolvedRefsResolver(t *testing.T) {
	defined := map[string]bool{}
	definedProps(":root {\n  --warn: #ffb300;\n  --fs-12: 12px;\n}\n", defined)
	definedProps("<b style:--seat=\"x\" style=\"--y: 2px\"></b>\n.t { --ec: var(--warn); }\n", defined)
	for _, name := range []string{"warn", "fs-12", "seat", "y", "ec"} {
		if !defined[name] {
			t.Errorf("--%s was not recognised as defined", name)
		}
	}
	if defined["fs"] {
		t.Error("--fs-12: was taken as a definition of --fs")
	}
	files := map[string]string{
		"web/src/components/Good.svelte": ".a { color: var(--warn); font-size: var( --fs-12 ); }\n.b { background: var(--ec, var(--seat)); top: var(--y); }\n",
		"web/src/components/Bad.svelte":  ".a { color: var(--warn); }\n.b { color: var(--wran); }\n.c { border-color: var(--{seat}); }\n",
	}
	got := unresolvedRefs(files, defined)
	if want := []string{"web/src/components/Bad.svelte:2: --wran"}; !slices.Equal(got, want) {
		t.Errorf("unresolved %v, want %v", got, want)
	}
}

// TestSiteVarsResolve: every var(--name) in the website's styles (its
// stylesheet, layouts, components and pages) is a token in tokens.css or a
// property the website assigns itself, so a mistyped token name cannot
// silently drop a colour from a page.
func TestSiteVarsResolve(t *testing.T) {
	root := filepath.Join("..", "..")
	defined := map[string]bool{}
	tokens, err := os.ReadFile("tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	definedProps(string(tokens), defined)

	site := filepath.Join(root, "site", "src")
	files := map[string]string{}
	refs := 0
	err = filepath.WalkDir(site, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch filepath.Ext(p) {
		case ".css", ".astro", ".ts":
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			definedProps(string(b), defined)
			if filepath.Ext(p) != ".ts" {
				rel, err := filepath.Rel(root, p)
				if err != nil {
					return err
				}
				files[filepath.ToSlash(rel)] = string(b)
				refs += len(varRef.FindAllString(string(b), -1))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if refs == 0 {
		t.Fatal("found no var(--name) references in the website's styles: the scan is looking in the wrong place")
	}
	for _, m := range unresolvedRefs(files, defined) {
		t.Errorf("%s is neither a token in internal/design/tokens.css nor a property assigned in site/src", m)
	}
}
