package design

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The stray-value check: every colour and every px or rem font size in the
// board (web/src) and the website (site/src) must come from the tokens, as
// a var(--name) reference. StrayValues finds the ones that do not, so a
// test can fail on them with their place. Relative sizes (em, %, and
// calc() over the card width) are layout, not design values, and pass.

// Stray is one design value written outside the tokens.
type Stray struct {
	File    string // slash path relative to the scanned root
	Line    int
	Literal string
}

func (s Stray) String() string { return fmt.Sprintf("%s:%d: %s", s.File, s.Line, s.Literal) }

var (
	comment    = regexp.MustCompile(`(?s)/\*.*?\*/`)
	styleBlock = regexp.MustCompile(`(?is)<style\b[^>]*>(.*?)</style>`)
	styleAttr  = regexp.MustCompile(`(?i)\bstyle="([^"]*)"`)
	styleDir   = regexp.MustCompile(`\bstyle:([-\w]+)="([^"]*)"`)
	decl       = regexp.MustCompile(`([-\w]+)\s*:\s*([^;{}]+)`)
	hexColour  = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	colourFn   = regexp.MustCompile(`\b(?:rgba?|hsla?|hwb|lab|lch|oklab|oklch|color)\(`)
	sizeLit    = regexp.MustCompile(`(?:^|[^/\w.-])(\d*\.?\d+(?:px|rem))\b`)
	varCall    = regexp.MustCompile(`var\(\s*--[-\w]+\s*(?:,[^()]*(?:\([^()]*\)[^()]*)*)?\)`)
	wordRe     = regexp.MustCompile(`[A-Za-z]+`)
)

// colourProps are the properties whose named-colour keywords count as
// colours (elsewhere a word such as "white" in white-space is no colour).
var colourProps = map[string]bool{
	"color": true, "background": true, "background-color": true, "background-image": true,
	"border": true, "border-color": true, "border-top": true, "border-right": true, "border-bottom": true, "border-left": true,
	"border-top-color": true, "border-right-color": true, "border-bottom-color": true, "border-left-color": true,
	"outline": true, "outline-color": true, "box-shadow": true, "text-shadow": true, "fill": true, "stroke": true,
	"caret-color": true, "accent-color": true, "text-decoration": true, "text-decoration-color": true,
	"column-rule": true, "column-rule-color": true, "mask": true, "mask-image": true, "-webkit-mask-image": true,
	"filter": true, "scrollbar-color": true,
}

// namedColours are CSS's named colours (the keywords transparent and
// currentColor are not design values and are left out).
var namedColours = map[string]bool{}

func init() {
	for _, c := range strings.Fields(`aliceblue antiquewhite aqua aquamarine azure beige bisque black blanchedalmond blue
	blueviolet brown burlywood cadetblue chartreuse chocolate coral cornflowerblue cornsilk crimson cyan darkblue darkcyan
	darkgoldenrod darkgray darkgreen darkgrey darkkhaki darkmagenta darkolivegreen darkorange darkorchid darkred darksalmon
	darkseagreen darkslateblue darkslategray darkslategrey darkturquoise darkviolet deeppink deepskyblue dimgray dimgrey
	dodgerblue firebrick floralwhite forestgreen fuchsia gainsboro ghostwhite gold goldenrod gray green greenyellow grey
	honeydew hotpink indianred indigo ivory khaki lavender lavenderblush lawngreen lemonchiffon lightblue lightcoral
	lightcyan lightgoldenrodyellow lightgray lightgreen lightgrey lightpink lightsalmon lightseagreen lightskyblue
	lightslategray lightslategrey lightsteelblue lightyellow lime limegreen linen magenta maroon mediumaquamarine
	mediumblue mediumorchid mediumpurple mediumseagreen mediumslateblue mediumspringgreen mediumturquoise mediumvioletred
	midnightblue mintcream mistyrose moccasin navajowhite navy oldlace olive olivedrab orange orangered orchid
	palegoldenrod palegreen paleturquoise palevioletred papayawhip peachpuff peru pink plum powderblue purple rebeccapurple
	red rosybrown royalblue saddlebrown salmon sandybrown seagreen seashell sienna silver skyblue slateblue slategray
	slategrey snow springgreen steelblue tan teal thistle tomato turquoise violet wheat white whitesmoke yellow yellowgreen`) {
		namedColours[c] = true
	}
}

// strayInStyle reports the design values written outside the tokens in a
// piece of CSS (a stylesheet, a <style> block or a style attribute) that
// starts at line first of its file.
func strayInStyle(css string, first int) []Stray {
	// Comments may mention values; blank them, keeping the line count.
	css = comment.ReplaceAllStringFunc(css, func(c string) string {
		return strings.Repeat("\n", strings.Count(c, "\n"))
	})
	var out []Stray
	for _, m := range decl.FindAllStringSubmatchIndex(css, -1) {
		prop := strings.ToLower(css[m[2]:m[3]])
		value := css[m[4]:m[5]]
		line := first + strings.Count(css[:m[4]], "\n")
		// A token reference, fallback included, is the right way to write a value.
		bare := varCall.ReplaceAllString(value, "var()")
		for _, h := range hexColour.FindAllString(bare, -1) {
			out = append(out, Stray{Line: line, Literal: prop + ": " + h})
		}
		if colourFn.MatchString(bare) {
			out = append(out, Stray{Line: line, Literal: prop + ": " + strings.TrimSpace(value)})
		}
		if colourProps[prop] {
			for _, w := range wordRe.FindAllString(bare, -1) {
				if namedColours[strings.ToLower(w)] {
					out = append(out, Stray{Line: line, Literal: prop + ": " + w})
				}
			}
		}
		if prop == "font-size" || prop == "font" || prop == "--fs" || strings.HasPrefix(prop, "--fs-") {
			for _, s := range sizeLit.FindAllStringSubmatch(bare, -1) {
				out = append(out, Stray{Line: line, Literal: prop + ": " + s[1]})
			}
		}
	}
	return out
}

// strayInFile reports the stray values in one source file: all of a .css
// file, and the <style> blocks, style attributes and style: directives of
// a .svelte or .astro file.
func strayInFile(name, src string) []Stray {
	if strings.HasSuffix(name, ".css") {
		return strayInStyle(src, 1)
	}
	var out []Stray
	lineAt := func(i int) int { return 1 + strings.Count(src[:i], "\n") }
	for _, m := range styleBlock.FindAllStringSubmatchIndex(src, -1) {
		out = append(out, strayInStyle(src[m[2]:m[3]], lineAt(m[2]))...)
	}
	for _, m := range styleAttr.FindAllStringSubmatchIndex(src, -1) {
		out = append(out, strayInStyle(src[m[2]:m[3]], lineAt(m[2]))...)
	}
	for _, m := range styleDir.FindAllStringSubmatchIndex(src, -1) {
		out = append(out, strayInStyle(src[m[2]:m[3]]+": "+src[m[4]:m[5]], lineAt(m[2]))...)
	}
	return out
}

// StrayValues walks each dir under root (paths relative to root) and
// reports every colour or px/rem font size in a .css, .svelte or .astro
// file that is not a var(--name) token reference. node_modules and
// dot-directories are skipped.
func StrayValues(root string, dirs ...string) ([]Stray, error) {
	var out []Stray
	for _, dir := range dirs {
		start := filepath.Join(root, dir)
		err := filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if name := d.Name(); name == "node_modules" || (strings.HasPrefix(name, ".") && p != start) {
					return filepath.SkipDir
				}
				return nil
			}
			switch filepath.Ext(p) {
			case ".css", ".svelte", ".astro":
			default:
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			for _, s := range strayInFile(p, string(b)) {
				s.File = filepath.ToSlash(rel)
				out = append(out, s)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
