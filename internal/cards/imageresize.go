package cards

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg" // decode JPEG sources (malie.io)
	_ "image/png"  // decode PNG sources

	// webp encodes the copies, and registers its decoder for WebP sources.
	"github.com/gen2brain/webp"
	"golang.org/x/image/draw"
)

// imageWidths are the widths, in pixels, of the files of a resized copy,
// matching TCGdex's own: low.webp for the card on the mat and in the hand,
// high.webp for the zoom and the preview.
var imageWidths = map[string]int{"low.webp": 245, "high.webp": 600}

// imageQuality and imageMethod are the WebP quality and effort of a
// resized copy: at quality 75 a 600-pixel card is about 60 KB, the size of
// TCGdex's own high.webp; effort 6 is slowest and smallest, which a copy
// made once in the background can afford.
const (
	imageQuality = 75
	imageMethod  = 6
)

// resizeImage decodes a full-size card image (JPEG, PNG or WebP) and
// returns it as WebP at the width of file, one of ImageFiles, keeping its
// aspect ratio and never enlarging it.
func resizeImage(src []byte, file string) ([]byte, error) {
	w, ok := imageWidths[file]
	if !ok {
		return nil, fmt.Errorf("resize: unknown image file %q", file)
	}
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("resize: decode: %w", err)
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil, fmt.Errorf("resize: empty image")
	}
	if b.Dx() > w {
		h := max(1, b.Dy()*w/b.Dx())
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
		img = dst
	}
	var out bytes.Buffer
	if err := webp.Encode(&out, img, webp.Options{Quality: imageQuality, Method: imageMethod}); err != nil {
		return nil, fmt.Errorf("resize: encode: %w", err)
	}
	return out.Bytes(), nil
}
