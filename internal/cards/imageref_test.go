package cards

import (
	"reflect"
	"testing"
)

func TestImageIsFile(t *testing.T) {
	for addr, want := range map[string]bool{
		"https://cdn.malie.io/file/malie-io/tcgl/cards/jpg/en/me6/me6_en_001_std.jpg": true,
		"https://example.com/a/b.PNG":                       true,
		"https://example.com/a/b.jpeg?x=1":                  true,
		"https://example.com/a/b.webp":                      true,
		"https://assets.tcgdex.net/en/me/me06/001":          false,
		"https://assets.tcgdex.net/en/swsh/swsh12.5/157":    false,
		"https://assets.tcgdex.net/en/me/me06/001/low.webp": true,
		"": false,
	} {
		if got := ImageIsFile(addr); got != want {
			t.Errorf("ImageIsFile(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestImageList(t *testing.T) {
	both := Card{Image: "https://a.test/x", Images: []ImageRef{{URL: "https://a.test/x"}, {URL: "https://b.test/y.jpg", File: true}}}
	if got := both.ImageList(); !reflect.DeepEqual(got, both.Images) {
		t.Errorf("Images set: %+v", got)
	}
	only := Card{Image: "https://cdn.malie.io/x.jpg"}
	if got := only.ImageList(); !reflect.DeepEqual(got, []ImageRef{{URL: "https://cdn.malie.io/x.jpg", File: true}}) {
		t.Errorf("Image only: %+v", got)
	}
	base := Card{Image: "https://assets.tcgdex.net/en/sv/sv1/1"}
	if got := base.ImageList(); !reflect.DeepEqual(got, []ImageRef{{URL: "https://assets.tcgdex.net/en/sv/sv1/1"}}) {
		t.Errorf("base path only: %+v", got)
	}
	if got := (&Card{}).ImageList(); got != nil {
		t.Errorf("no art: %+v", got)
	}
}

// Borrowed Basic Energy art fills the image list as well as Image.
func TestEnergyArtFillsImageList(t *testing.T) {
	c := Card{Name: "Basic Fighting Energy", Category: "Energy", EnergyType: "Normal"}
	EnergyArt(&c)
	want := []ImageRef{{URL: "https://assets.tcgdex.net/en/swsh/swsh12.5/157"}}
	if c.Image != want[0].URL || !reflect.DeepEqual(c.Images, want) {
		t.Errorf("Image %q, Images %+v", c.Image, c.Images)
	}
}
