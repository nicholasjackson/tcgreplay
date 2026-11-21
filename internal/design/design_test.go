package design

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedCurrent: the committed tokens.css is exactly what
// tokens.json generates, so editing the source without regenerating fails.
func TestGeneratedCurrent(t *testing.T) {
	want, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("internal/design/tokens.css is out of date with tokens.json: run go generate ./internal/design")
	}
}

// TestGeneratedCurrentCatchesEdits: a changed source no longer matches the
// committed stylesheet, which is what makes TestGeneratedCurrent fail.
func TestGeneratedCurrentCatchesEdits(t *testing.T) {
	edited := bytes.Replace(source, []byte(`"#38BDF8"`), []byte(`"#123456"`), 1)
	if bytes.Equal(edited, source) {
		t.Fatal("the planted edit did not change the source")
	}
	tok, err := Parse(edited)
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("tokens.css")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(tok.CSS(), committed) {
		t.Fatal("an edited tokens.json still matches the committed tokens.css")
	}
}

// TestCSSThemes: every token is on :root with its default-theme value; the
// other theme overrides only the tokens that vary by theme; each other mat
// repoints the mat-* aliases at its own set; references become var().
func TestCSSThemes(t *testing.T) {
	tok, err := Parse([]byte(`{"color":{"themes":["light","dark"],"default":"light","tokens":[
		{"name":"ink","value":{"light":"#111","dark":"#eee"}},
		{"name":"accent","value":"#f80"},
		{"name":"text","value":"{ink}"},
		{"name":"slate-ink","value":"#ddd"},
		{"name":"felt-ink","value":"#efe"},
		{"name":"mat-ink","value":"{slate-ink}"}]},
		"mats":{"default":"slate","sets":["slate","felt"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	css := string(tok.CSS())
	root, rest, ok := strings.Cut(css, "[data-theme='dark'] {")
	if !ok {
		t.Fatalf("no dark block:\n%s", css)
	}
	dark, felt, ok := strings.Cut(rest, "[data-mat='felt'] {")
	if !ok {
		t.Fatalf("no felt block:\n%s", css)
	}
	for _, want := range []string{"--ink: #111;", "--accent: #f80;", "--text: var(--ink);", "--mat-ink: var(--slate-ink);", "color-scheme: light;"} {
		if !strings.Contains(root, want) {
			t.Errorf(":root lacks %q:\n%s", want, root)
		}
	}
	if !strings.Contains(dark, "--ink: #eee;") || !strings.Contains(dark, "color-scheme: dark;") {
		t.Errorf("dark block does not override ink:\n%s", dark)
	}
	if strings.Contains(dark, "--accent") || strings.Contains(dark, "--text") {
		t.Errorf("dark block repeats a token that does not vary by theme:\n%s", dark)
	}
	if strings.TrimSpace(felt) != "--mat-ink: var(--felt-ink);\n}" {
		t.Errorf("felt block should repoint only the mat alias, got:\n%s", felt)
	}
	if strings.Contains(css, "data-mat='slate'") {
		t.Errorf("the default mat needs no block:\n%s", css)
	}
}

// TestParseRejectsMats: a mat alias needs its counterpart in every mat set,
// a reference must name a token, and a pair must name tokens.
func TestParseRejectsMats(t *testing.T) {
	for name, src := range map[string]string{
		"missing set token": `{"color":{"themes":["light"],"default":"light","tokens":[{"name":"slate-ink","value":"#000"},{"name":"mat-ink","value":"{slate-ink}"}]},"mats":{"default":"slate","sets":["slate","felt"]}}`,
		"unknown reference": `{"color":{"themes":["light"],"default":"light","tokens":[{"name":"text","value":"{nope}"}]}}`,
		"unknown pair":      `{"color":{"themes":["light"],"default":"light","tokens":[{"name":"ink","value":"#000"}]},"pairs":[{"text":"ink","background":"nope","min":4.5}]}`,
		"default mat":       `{"color":{"themes":["light"],"default":"light","tokens":[]},"mats":{"default":"wool","sets":["slate"]}}`,
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: parsed without error", name)
		}
	}
}

// TestParseRejects: a malformed source fails with a reason.
func TestParseRejects(t *testing.T) {
	for name, src := range map[string]string{
		"no default":      `{"color":{"themes":["dark"],"tokens":[]}}`,
		"unknown default": `{"color":{"themes":["dark"],"default":"light","tokens":[]}}`,
		"unknown theme":   `{"color":{"themes":["dark"],"default":"dark","tokens":[{"name":"a","value":{"sepia":"#000"}}]}}`,
		"repeated name":   `{"color":{"themes":["dark"],"default":"dark","tokens":[{"name":"a","value":"#000"},{"name":"a","value":"#111"}]}}`,
		"unnamed token":   `{"color":{"themes":["dark"],"default":"dark","tokens":[{"value":"#000"}]}}`,
		"bad value":       `{"color":{"themes":["dark"],"default":"dark","tokens":[{"name":"a","value":3}]}}`,
		"not json":        `{`,
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: parsed without error", name)
		}
	}
}

// TestBothAppsLoadTheSharedTokens: the board and the website both import the
// one generated stylesheet, and the website's hand copy is gone.
func TestBothAppsLoadTheSharedTokens(t *testing.T) {
	root := filepath.Join("..", "..")
	for file, imp := range map[string]string{
		"web/src/main.js":             "'../../internal/design/tokens.css'",
		"site/src/layouts/Base.astro": "'../../../internal/design/tokens.css'",
	} {
		b, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "import "+imp) {
			t.Errorf("%s does not import %s", file, imp)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "site/src/styles/tokens.css")); err == nil {
		t.Error("site/src/styles/tokens.css, the website's hand copy of the colours, still exists")
	}
}
