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
	edited := bytes.Replace(source, []byte(`"#e3b341"`), []byte(`"#123456"`), 1)
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

// TestCSSThemes: every token is on :root with its default-theme value, and
// the other theme overrides only the tokens that vary by theme.
func TestCSSThemes(t *testing.T) {
	tok, err := Parse([]byte(`{"color":{"themes":["dark","light"],"default":"dark","switch":"media","tokens":[
		{"name":"ink","value":{"dark":"#eee","light":"#111"}},
		{"name":"accent","value":"#f80"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	css := string(tok.CSS())
	root, light, ok := strings.Cut(css, "@media (prefers-color-scheme: light)")
	if !ok {
		t.Fatalf("no light block:\n%s", css)
	}
	for _, want := range []string{"--ink: #eee;", "--accent: #f80;", "color-scheme: dark;"} {
		if !strings.Contains(root, want) {
			t.Errorf(":root lacks %q:\n%s", want, root)
		}
	}
	if !strings.Contains(light, ":root:not([data-theme='dark'])") || !strings.Contains(light, "--ink: #111;") {
		t.Errorf("light block does not override ink:\n%s", light)
	}
	if strings.Contains(light, "--accent") {
		t.Errorf("light block repeats a token that does not vary by theme:\n%s", light)
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
