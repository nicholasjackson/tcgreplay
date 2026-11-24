// Package design owns every design value the board (web/) and the website
// (site/) use: colours, typefaces, type sizes, spacing, radii, shadows and
// sizes. tokens.json is the one source, mirroring the TCG Replay Studio
// design system. Generate turns it into tokens.css, a stylesheet of CSS
// custom properties that both apps import; the file is generated,
// committed, and checked by this package's tests, so neither front-end
// build needs Go. To change a value, edit tokens.json and run
// go generate ./internal/design.
//
// Interface themes switch on the document root's data-theme attribute
// (the default theme's values sit on :root). Mats switch on data-mat: each
// mat-* token is an alias of the default mat's set (mat-surface is
// {slate-surface}), and each other mat repoints the aliases at its own set.
package design

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

//go:embed tokens.json
var source []byte

// TokensCSS is tokens.css, the generated stylesheet of design tokens. The
// hosted app's server-rendered pages (sign-in, problem, busy, signed out)
// inline it so they share the board's and website's colours and type.
//
//go:embed tokens.css
var TokensCSS string

// MarkSVG is mark.svg, the TCG Replay mark, for the hosted app's
// server-rendered pages to inline.
//
//go:embed mark.svg
var MarkSVG string

// Tokens is the token source, tokens.json.
type Tokens struct {
	Color   Colors `json:"color"`
	Mats    Mats   `json:"mats"`
	Type    Type   `json:"type"`
	Spacing Family `json:"spacing"`
	Radius  Family `json:"radius"`
	Shadow  Family `json:"shadow"`
	Size    Family `json:"size"`
	Pairs   []Pair `json:"pairs"`
}

// Colors are the colour tokens and the interface themes they vary by.
type Colors struct {
	Themes  []string `json:"themes"`
	Default string   `json:"default"`
	Tokens  []Token  `json:"tokens"`
}

// Mats names the mat sets. Each mat-* alias points at the default set.
type Mats struct {
	Default string   `json:"default"`
	Sets    []string `json:"sets"`
}

// Token is one named value: a string, the same in every theme, or an
// object with a value per theme. A value written {name} refers to another
// token.
type Token struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
	Usage string          `json:"usage,omitempty"`
}

// Family is a list of tokens, each written as --<name>.
type Family struct {
	Tokens []Token `json:"tokens"`
}

// Type holds the typefaces (as --font-<name>) and the type styles (as
// --fs-<name>, with --lh-, --fw- and --ls-<name> when given).
type Type struct {
	Families map[string]string `json:"families"`
	Styles   []TypeStyle       `json:"styles"`
}

// TypeStyle is one named type style.
type TypeStyle struct {
	Name          string `json:"name"`
	FontSize      string `json:"fontSize"`
	LineHeight    string `json:"lineHeight,omitempty"`
	FontWeight    string `json:"fontWeight,omitempty"`
	LetterSpacing string `json:"letterSpacing,omitempty"`
	Usage         string `json:"usage,omitempty"`
}

// Pair is a foreground and background colour the design puts together,
// and the contrast it must reach: 4.5 for text, 3 for large text, focus
// rings and markers.
type Pair struct {
	Text       string  `json:"text"`
	Background string  `json:"background"`
	Min        float64 `json:"min"`
}

var ref = regexp.MustCompile(`^\{([-\w]+)\}$`)

// Load parses the embedded token source.
func Load() (Tokens, error) {
	return Parse(source)
}

// Parse parses and checks a token source.
func Parse(b []byte) (Tokens, error) {
	var t Tokens
	if err := json.Unmarshal(b, &t); err != nil {
		return t, fmt.Errorf("design: tokens.json: %w", err)
	}
	if len(t.Color.Themes) == 0 || t.Color.Default == "" {
		return t, fmt.Errorf("design: tokens.json: color needs themes and a default")
	}
	if !slices.Contains(t.Color.Themes, t.Color.Default) {
		return t, fmt.Errorf("design: tokens.json: default theme %q is not in themes", t.Color.Default)
	}
	if len(t.Mats.Sets) > 0 && !slices.Contains(t.Mats.Sets, t.Mats.Default) {
		return t, fmt.Errorf("design: tokens.json: default mat %q is not in mats", t.Mats.Default)
	}
	names := map[string]bool{}
	for _, tok := range t.all() {
		if tok.Name == "" || names[tok.Name] {
			return t, fmt.Errorf("design: tokens.json: token %q is unnamed or repeated", tok.Name)
		}
		names[tok.Name] = true
	}
	for _, tok := range t.all() {
		vals, err := tok.Values(t.Color.Themes)
		if err != nil {
			return t, err
		}
		for _, v := range vals {
			if m := ref.FindStringSubmatch(v); m != nil && !names[m[1]] {
				return t, fmt.Errorf("design: token %q refers to unknown token %q", tok.Name, m[1])
			}
		}
		if mat, rest, ok := t.matAlias(tok); ok {
			for _, set := range t.Mats.Sets {
				if !names[set+"-"+rest] {
					return t, fmt.Errorf("design: %s is a %s mat alias but the %s mat has no %s-%s", tok.Name, mat, set, set, rest)
				}
			}
		}
	}
	for _, p := range t.Pairs {
		if !names[p.Text] || !names[p.Background] || p.Min <= 0 {
			return t, fmt.Errorf("design: pair %s on %s names an unknown token or has no minimum", p.Text, p.Background)
		}
	}
	return t, nil
}

// all lists every token that is written as --<name>.
func (t Tokens) all() []Token {
	var out []Token
	out = append(out, t.Color.Tokens...)
	for _, f := range []Family{t.Spacing, t.Radius, t.Shadow, t.Size} {
		out = append(out, f.Tokens...)
	}
	return out
}

// matAlias reports whether tok is a mat-* alias of the default mat's set
// (mat-surface = {slate-surface}), giving the default mat and the part of
// the name every set shares ("surface").
func (t Tokens) matAlias(tok Token) (mat, rest string, ok bool) {
	if !strings.HasPrefix(tok.Name, "mat-") || t.Mats.Default == "" {
		return "", "", false
	}
	var v string
	if json.Unmarshal(tok.Value, &v) != nil {
		return "", "", false
	}
	m := ref.FindStringSubmatch(v)
	if m == nil || m[1] != t.Mats.Default+"-"+strings.TrimPrefix(tok.Name, "mat-") {
		return "", "", false
	}
	return t.Mats.Default, strings.TrimPrefix(tok.Name, "mat-"), true
}

// Values gives the token's value in each theme. A single string applies to
// every theme; an object must name only known themes.
func (tok Token) Values(themes []string) (map[string]string, error) {
	var one string
	if err := json.Unmarshal(tok.Value, &one); err == nil {
		out := map[string]string{}
		for _, th := range themes {
			out[th] = one
		}
		return out, nil
	}
	var per map[string]string
	if err := json.Unmarshal(tok.Value, &per); err != nil {
		return nil, fmt.Errorf("design: token %q: value must be a string or an object of themes", tok.Name)
	}
	for th := range per {
		if !slices.Contains(themes, th) {
			return nil, fmt.Errorf("design: token %q: unknown theme %q", tok.Name, th)
		}
	}
	return per, nil
}

// themed reports whether the token's value differs by theme (an object).
func (tok Token) themed() bool {
	return strings.HasPrefix(strings.TrimSpace(string(tok.Value)), "{")
}

// css writes a token value as CSS: a {name} reference becomes var(--name).
func css(v string) string {
	if m := ref.FindStringSubmatch(v); m != nil {
		return "var(--" + m[1] + ")"
	}
	return v
}

// Resolve gives a colour token's final value under an interface theme and a
// mat, following references and repointing mat-* aliases at the mat's set.
func (t Tokens) Resolve(name, theme, mat string) (string, error) {
	byName := map[string]Token{}
	for _, tok := range t.all() {
		byName[tok.Name] = tok
	}
	for range 32 {
		tok, ok := byName[name]
		if !ok {
			return "", fmt.Errorf("design: no token %q", name)
		}
		if _, rest, ok := t.matAlias(tok); ok && mat != "" {
			name = mat + "-" + rest
			continue
		}
		vals, err := tok.Values(t.Color.Themes)
		if err != nil {
			return "", err
		}
		v, ok := vals[theme]
		if !ok {
			v = vals[t.Color.Default]
		}
		if m := ref.FindStringSubmatch(v); m != nil {
			name = m[1]
			continue
		}
		return v, nil
	}
	return "", fmt.Errorf("design: token %q refers in a loop", name)
}

// Generate writes tokens.css from the embedded token source.
func Generate() ([]byte, error) {
	t, err := Load()
	if err != nil {
		return nil, err
	}
	return t.CSS(), nil
}

// CSS renders the tokens as a stylesheet: every token on :root with its
// default-theme value, an override block per other interface theme, and a
// block per other mat repointing the mat-* aliases.
func (t Tokens) CSS() []byte {
	var b strings.Builder
	b.WriteString("/* Generated from internal/design/tokens.json by go generate ./internal/design.\n   Do not edit: change tokens.json and regenerate. */\n")
	b.WriteString(":root {\n")
	for _, tok := range t.Color.Tokens {
		v, _ := tok.Values(t.Color.Themes)
		fmt.Fprintf(&b, "  --%s: %s;\n", tok.Name, css(v[t.Color.Default]))
	}
	families := make([]string, 0, len(t.Type.Families))
	for name := range t.Type.Families {
		families = append(families, name)
	}
	sort.Strings(families)
	for _, name := range families {
		fmt.Fprintf(&b, "  --font-%s: %s;\n", name, css(t.Type.Families[name]))
	}
	for _, s := range t.Type.Styles {
		fmt.Fprintf(&b, "  --fs-%s: %s;\n", s.Name, s.FontSize)
		for _, x := range []struct{ prefix, v string }{{"lh", s.LineHeight}, {"fw", s.FontWeight}, {"ls", s.LetterSpacing}} {
			if x.v != "" {
				fmt.Fprintf(&b, "  --%s-%s: %s;\n", x.prefix, s.Name, x.v)
			}
		}
	}
	for _, f := range []Family{t.Spacing, t.Radius, t.Shadow, t.Size} {
		for _, tok := range f.Tokens {
			v, _ := tok.Values(t.Color.Themes)
			fmt.Fprintf(&b, "  --%s: %s;\n", tok.Name, css(v[t.Color.Default]))
		}
	}
	fmt.Fprintf(&b, "  color-scheme: %s;\n", t.Color.Default)
	b.WriteString("}\n")

	for _, th := range t.Color.Themes {
		if th == t.Color.Default {
			continue
		}
		fmt.Fprintf(&b, "[data-theme='%s'] {\n", th)
		for _, tok := range t.all() {
			if !tok.themed() {
				continue
			}
			v, _ := tok.Values(t.Color.Themes)
			if val, ok := v[th]; ok {
				fmt.Fprintf(&b, "  --%s: %s;\n", tok.Name, css(val))
			}
		}
		fmt.Fprintf(&b, "  color-scheme: %s;\n}\n", th)
	}

	for _, set := range t.Mats.Sets {
		if set == t.Mats.Default {
			continue
		}
		fmt.Fprintf(&b, "[data-mat='%s'] {\n", set)
		for _, tok := range t.Color.Tokens {
			if _, rest, ok := t.matAlias(tok); ok {
				fmt.Fprintf(&b, "  --%s: var(--%s-%s);\n", tok.Name, set, rest)
			}
		}
		b.WriteString("}\n")
	}
	return []byte(b.String())
}
