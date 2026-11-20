// Package design owns every design value the board (web/) and the website
// (site/) use: colours, typefaces, type sizes, spacing, radii, shadows and
// sizes. tokens.json is the one source. Generate turns it into tokens.css,
// a stylesheet of CSS custom properties that both apps import; the file is
// generated, committed, and checked by this package's tests, so neither
// front-end build needs Go. To change a value, edit tokens.json and run
// go generate ./internal/design.
package design

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed tokens.json
var source []byte

// Tokens is the token source, tokens.json.
type Tokens struct {
	Color   Colors `json:"color"`
	Type    Type   `json:"type"`
	Spacing Family `json:"spacing"`
	Radius  Family `json:"radius"`
	Shadow  Family `json:"shadow"`
	Size    Family `json:"size"`
}

// Colors are the colour tokens. A token's value is either one string, the
// same in every interface theme, or an object with a value per theme.
type Colors struct {
	Themes  []string     `json:"themes"`
	Default string       `json:"default"`
	Switch  string       `json:"switch"` // "media": other themes follow prefers-color-scheme
	Tokens  []ColorToken `json:"tokens"`
}

// ColorToken is one colour, with a value per theme or one for all.
type ColorToken struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
	Usage string          `json:"usage,omitempty"`
}

// Type holds the typefaces (as --font-<name>) and the type styles (as
// --fs-<name>, with --lh-<name> and --fw-<name> when given).
type Type struct {
	Families map[string]string `json:"families"`
	Styles   []TypeStyle       `json:"styles"`
}

// TypeStyle is one named type size.
type TypeStyle struct {
	Name       string `json:"name"`
	FontSize   string `json:"fontSize"`
	LineHeight string `json:"lineHeight,omitempty"`
	FontWeight string `json:"fontWeight,omitempty"`
}

// Family is a list of plain tokens, each written as --<name>.
type Family struct {
	Tokens []Token `json:"tokens"`
}

// Token is one named value.
type Token struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Usage string `json:"usage,omitempty"`
}

// Load parses the embedded token source.
func Load() (Tokens, error) {
	return Parse(source)
}

// Parse parses a token source.
func Parse(b []byte) (Tokens, error) {
	var t Tokens
	if err := json.Unmarshal(b, &t); err != nil {
		return t, fmt.Errorf("design: tokens.json: %w", err)
	}
	if len(t.Color.Themes) == 0 || t.Color.Default == "" {
		return t, fmt.Errorf("design: tokens.json: color needs themes and a default")
	}
	if !contains(t.Color.Themes, t.Color.Default) {
		return t, fmt.Errorf("design: tokens.json: default theme %q is not in themes", t.Color.Default)
	}
	seen := map[string]bool{}
	for _, c := range t.Color.Tokens {
		if c.Name == "" || seen[c.Name] {
			return t, fmt.Errorf("design: tokens.json: colour %q is unnamed or repeated", c.Name)
		}
		seen[c.Name] = true
		if _, err := c.Values(t.Color.Themes); err != nil {
			return t, err
		}
	}
	return t, nil
}

// Values gives the token's value in each theme. A single string applies to
// every theme; an object must name only known themes and the default.
func (c ColorToken) Values(themes []string) (map[string]string, error) {
	var one string
	if err := json.Unmarshal(c.Value, &one); err == nil {
		out := map[string]string{}
		for _, th := range themes {
			out[th] = one
		}
		return out, nil
	}
	var per map[string]string
	if err := json.Unmarshal(c.Value, &per); err != nil {
		return nil, fmt.Errorf("design: colour %q: value must be a string or an object of themes", c.Name)
	}
	for th := range per {
		if !contains(themes, th) {
			return nil, fmt.Errorf("design: colour %q: unknown theme %q", c.Name, th)
		}
	}
	return per, nil
}

// themed reports whether the token's value differs by theme (an object).
func (c ColorToken) themed() bool {
	return strings.HasPrefix(strings.TrimSpace(string(c.Value)), "{")
}

// Generate writes tokens.css from the embedded token source.
func Generate() ([]byte, error) {
	t, err := Load()
	if err != nil {
		return nil, err
	}
	return t.CSS(), nil
}

// CSS renders the tokens as a stylesheet. Every token is a custom property
// on :root with its default-theme value; each other theme overrides the
// tokens that differ for it.
func (t Tokens) CSS() []byte {
	var b strings.Builder
	b.WriteString("/* Generated from internal/design/tokens.json by go generate ./internal/design.\n   Do not edit: change tokens.json and regenerate. */\n")
	b.WriteString(":root {\n")
	for _, c := range t.Color.Tokens {
		v, _ := c.Values(t.Color.Themes)
		if val, ok := v[t.Color.Default]; ok {
			fmt.Fprintf(&b, "  --%s: %s;\n", c.Name, val)
		}
	}
	families := make([]string, 0, len(t.Type.Families))
	for name := range t.Type.Families {
		families = append(families, name)
	}
	sort.Strings(families)
	for _, name := range families {
		fmt.Fprintf(&b, "  --font-%s: %s;\n", name, t.Type.Families[name])
	}
	for _, s := range t.Type.Styles {
		fmt.Fprintf(&b, "  --fs-%s: %s;\n", s.Name, s.FontSize)
		if s.LineHeight != "" {
			fmt.Fprintf(&b, "  --lh-%s: %s;\n", s.Name, s.LineHeight)
		}
		if s.FontWeight != "" {
			fmt.Fprintf(&b, "  --fw-%s: %s;\n", s.Name, s.FontWeight)
		}
	}
	for _, f := range []Family{t.Spacing, t.Radius, t.Shadow, t.Size} {
		for _, tok := range f.Tokens {
			fmt.Fprintf(&b, "  --%s: %s;\n", tok.Name, tok.Value)
		}
	}
	fmt.Fprintf(&b, "  color-scheme: %s;\n", t.Color.Default)
	b.WriteString("}\n")

	for _, th := range t.Color.Themes {
		if th == t.Color.Default {
			continue
		}
		var body strings.Builder
		for _, c := range t.Color.Tokens {
			if !c.themed() {
				continue
			}
			v, _ := c.Values(t.Color.Themes)
			if val, ok := v[th]; ok {
				fmt.Fprintf(&body, "    --%s: %s;\n", c.Name, val)
			}
		}
		if t.Color.Switch == "media" {
			fmt.Fprintf(&b, "@media (prefers-color-scheme: %s) {\n  :root:not([data-theme='%s']) {\n%s    color-scheme: %s;\n  }\n}\n",
				th, t.Color.Default, body.String(), th)
		} else {
			fmt.Fprintf(&b, "[data-theme='%s'] {\n%s  color-scheme: %s;\n}\n", th, strings.ReplaceAll(body.String(), "    ", "  "), th)
		}
	}
	return []byte(b.String())
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
