package design

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// markShapes are the drawing elements of internal/design/mark.svg.
func markShapes(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("mark.svg")
	if err != nil {
		t.Fatal(err)
	}
	shapes := regexp.MustCompile(`<(?:rect|path)\b[^>]*/>`).FindAllString(string(b), -1)
	if len(shapes) != 4 {
		t.Fatalf("mark.svg has %d shapes, want the mark's 4 (square, two cards, arrow)", len(shapes))
	}
	return shapes
}

// TestIconsComeFromTheMark: the listing icon's source and the board's
// favicon draw the mark itself, and every rendered logo and icon is a
// square PNG of its expected size.
func TestIconsComeFromTheMark(t *testing.T) {
	root := filepath.Join("..", "..")
	shapes := markShapes(t)

	listing, err := os.ReadFile(filepath.Join(root, "plugin", "assets", "icon.svg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range shapes {
		if !strings.Contains(string(listing), s) {
			t.Errorf("plugin/assets/icon.svg lacks the mark's %s", s)
		}
	}

	index, err := os.ReadFile(filepath.Join(root, "web", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	favicon := regexp.MustCompile(`<link rel="icon" href="([^"]*)"`).FindStringSubmatch(string(index))
	if favicon == nil {
		t.Fatal("web/index.html has no favicon link")
	}
	// The favicon's line holds its link and nothing after it: anything left
	// of an older icon would be stray text on the page.
	if !regexp.MustCompile(`(?m)^\s*<link rel="icon" href="[^"]*" />$`).Match(index) {
		t.Error("web/index.html's favicon line has something after its <link>: leftover markup shows as text on the page")
	}
	for _, s := range shapes {
		// The favicon is a data URI: single quotes, # written %23.
		want := strings.NewReplacer(`"`, "'", "#", "%23").Replace(s)
		if !strings.Contains(favicon[1], want) {
			t.Errorf("the board's favicon lacks the mark's %s", s)
		}
	}

	for file, px := range map[string]int{
		"site/public/logo.png":   512,
		"site/public/icon.png":   256,
		"docs/logo.png":          512,
		"docs/icon.png":          256,
		"plugin/assets/icon.png": 512,
	} {
		f, err := os.Open(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			t.Error(err)
			continue
		}
		cfg, format, err := image.DecodeConfig(f)
		f.Close()
		if err != nil || format != "png" || cfg.Width != px || cfg.Height != px {
			t.Errorf("%s: %s %dx%d (%v), want a %dx%d PNG rendered from the mark", file, format, cfg.Width, cfg.Height, err, px, px)
		}
	}
}

// TestMarkHasNoGameImagery: the mark's source names no Pokémon trademark,
// as the listing icon's own test requires of its source.
func TestMarkHasNoGameImagery(t *testing.T) {
	b, err := os.ReadFile("mark.svg")
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(b))
	for _, w := range []string{"pokémon", "pokemon", "poké", "poke ball", "pokeball", "pikachu", "nintendo"} {
		if strings.Contains(lower, w) {
			t.Errorf("mark.svg names %q", w)
		}
	}
}
