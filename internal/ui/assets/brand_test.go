package assets

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func decode(t *testing.T, name string, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decoding %s: %v", name, err)
	}
	return img
}

// opaque collects the pixels the icon actually paints.
func opaque(img image.Image) []image.Point {
	b := img.Bounds()
	var pts []image.Point
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 16<<8 {
				pts = append(pts, image.Pt(x, y))
			}
		}
	}
	return pts
}

// The tray icon is a template image: macOS tints it to match a light or dark
// menu bar, which needs one colour and an alpha channel. A full-colour mark
// such as logo.png would be flattened into a solid block instead.
func TestTrayIconIsSingleColorTemplate(t *testing.T) {
	img := decode(t, "tray.png", TrayPNG())
	pts := opaque(img)
	if len(pts) == 0 {
		t.Fatal("the tray icon paints nothing")
	}
	for _, p := range pts {
		r, g, b, _ := img.At(p.X, p.Y).RGBA()
		if r > 40<<8 || g > 40<<8 || b > 40<<8 {
			t.Fatalf("the tray icon is not a single dark colour: pixel %v is rgb(%d,%d,%d)",
				p, r>>8, g>>8, b>>8)
		}
	}
}

// The tray icon carries the same mark as brand.svg and logo.png: a chat
// bubble with the arrow inside it. The arrow is a stroke of its own, so a
// row across the middle crosses the bubble's two sides plus the arrow —
// at least three separate runs of paint. An outline-only bubble, which the
// icon used to be, would only ever show two.
func TestTrayIconKeepsBrandArrow(t *testing.T) {
	img := decode(t, "tray.png", TrayPNG())
	b := img.Bounds()
	painted := func(x, y int) bool {
		_, _, _, a := img.At(x, y).RGBA()
		return a > 16<<8
	}
	best := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		runs, in := 0, false
		for x := b.Min.X; x < b.Max.X; x++ {
			switch {
			case painted(x, y) && !in:
				in = true
				runs++
			case !painted(x, y):
				in = false
			}
		}
		best = max(best, runs)
	}
	if best < 3 {
		t.Fatalf("no row crosses bubble plus arrow: most runs on a row is %d, want >= 3", best)
	}
}

// The tray icon is square and at least 2x, so the menu bar has a sharp mark
// on a Retina display.
func TestTrayIconIsRetinaSquare(t *testing.T) {
	b := decode(t, "tray.png", TrayPNG()).Bounds()
	if b.Dx() != b.Dy() {
		t.Fatalf("the tray icon is %dx%d, want a square", b.Dx(), b.Dy())
	}
	if b.Dx() < 64 {
		t.Fatalf("the tray icon is %dpx, want at least 64 for a 2x menu bar", b.Dx())
	}
}

// The app icon stays the full-colour mark: it is drawn as-is in the dock and
// the window, never tinted, so flattening it to one colour would be a loss.
func TestLogoKeepsItsColors(t *testing.T) {
	img := decode(t, "logo.png", LogoPNG())
	buckets := map[[3]uint32]bool{}
	for _, p := range opaque(img) {
		r, g, b, _ := img.At(p.X, p.Y).RGBA()
		buckets[[3]uint32{r >> 14, g >> 14, b >> 14}] = true
	}
	if len(buckets) < 2 {
		t.Fatalf("logo.png has %d colour buckets, want a multi-colour mark", len(buckets))
	}
}
