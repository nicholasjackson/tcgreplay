package plugin

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"strings"
	"testing"
)

// listingIcon and listingIconSource are the listing's icon and the vector
// source it is rendered from.
const (
	listingIcon       = "../../plugin/assets/icon.png"
	listingIconSource = "../../plugin/assets/icon.svg"
)

// TestListingIcon: the listing icon is a square PNG at least 512 px across
// and well under OpenAI's 5 MiB limit, with its editable SVG source beside
// it, and the source names no Pokémon trademark.
func TestListingIcon(t *testing.T) {
	b, err := os.ReadFile(listingIcon)
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("decode %s: %v", listingIcon, err)
	}
	if format != "png" {
		t.Errorf("icon is %s, want png", format)
	}
	if cfg.Width != cfg.Height || cfg.Width < 512 {
		t.Errorf("icon is %dx%d, want square and at least 512 px", cfg.Width, cfg.Height)
	}
	if len(b) > 1<<20 {
		t.Errorf("icon is %d bytes, want well under 5 MiB (at most 1 MiB)", len(b))
	}

	src, err := os.ReadFile(listingIconSource)
	if err != nil {
		t.Fatalf("no editable source beside the icon: %v", err)
	}
	if !bytes.Contains(src, []byte("<svg")) {
		t.Errorf("%s is not an SVG", listingIconSource)
	}
	lower := strings.ToLower(string(src))
	for _, mark := range []string{"pokémon", "pokemon", "poké", "poke ball", "pokeball", "pikachu", "nintendo"} {
		if strings.Contains(lower, mark) {
			t.Errorf("icon source mentions %q", mark)
		}
	}
}

var _ = png.Decode // registers the PNG decoder for image.DecodeConfig
