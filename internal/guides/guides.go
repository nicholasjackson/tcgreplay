// Package guides holds the playmat's coaching workflows, served to connected
// assistants over MCP. Each guide is a markdown file named after it, with a
// frontmatter holding its name and a one-line "use when" description.
package guides

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed *.md
var files embed.FS

// order is the order players meet the workflows in, and the order List
// returns them in.
var order = []string{"import-game", "opponent-deck", "review-game", "replay-drill", "before-you-move", "numbers-drill"}

// Guide is one coaching workflow.
type Guide struct {
	Name        string // e.g. "review-game"
	Description string // one line: when to use it
	Text        string // full body, no frontmatter
}

var all = mustLoad()

// mustLoad parses every embedded guide. A malformed file, or one missing
// from order, is a build defect, so it panics (the package tests catch it).
func mustLoad() map[string]Guide {
	entries, err := files.ReadDir(".")
	if err != nil {
		panic(err)
	}
	known := map[string]bool{}
	for _, n := range order {
		known[n] = true
	}
	out := map[string]Guide{}
	for _, e := range entries {
		b, err := files.ReadFile(e.Name())
		if err != nil {
			panic(err)
		}
		g, err := parse(string(b))
		if err != nil {
			panic(fmt.Sprintf("guide %s: %v", e.Name(), err))
		}
		if g.Name+".md" != e.Name() || !known[g.Name] {
			panic(fmt.Sprintf("guide %s: name %q is not in the guide order", e.Name(), g.Name))
		}
		out[g.Name] = g
	}
	for _, n := range order {
		if _, ok := out[n]; !ok {
			panic(fmt.Sprintf("guide %s has no file", n))
		}
	}
	return out
}

// parse splits a guide file into its frontmatter fields and body.
func parse(s string) (Guide, error) {
	head, body, ok := strings.Cut(strings.TrimPrefix(s, "---\n"), "\n---\n")
	if !ok || !strings.HasPrefix(s, "---\n") {
		return Guide{}, fmt.Errorf("no frontmatter")
	}
	var g Guide
	for _, line := range strings.Split(head, "\n") {
		k, v, _ := strings.Cut(line, ": ")
		switch k {
		case "name":
			g.Name = v
		case "description":
			g.Description = v
		}
	}
	g.Text = strings.TrimLeft(body, "\n")
	return g, nil
}

// List returns every guide, in the order players meet them.
func List() []Guide {
	out := make([]Guide, len(order))
	for i, n := range order {
		out[i] = all[n]
	}
	return out
}

// Get returns the guide called name, or an error listing the valid names.
func Get(name string) (Guide, error) {
	g, ok := all[name]
	if !ok {
		return Guide{}, fmt.Errorf("unknown guide %q; guides are %s", name, strings.Join(order, ", "))
	}
	return g, nil
}
