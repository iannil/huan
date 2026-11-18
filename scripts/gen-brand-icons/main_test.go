package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// opaqueSquare returns a fully opaque src of the given size (alpha 255).
func opaqueSquare(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 0xaa, G: 0x22, B: 0x22, A: 0xff})
		}
	}
	return img
}

func alphaAt(img *image.RGBA, x, y int) uint8 {
	return img.Pix[img.PixOffset(x, y)+3]
}

// TestSquircleMask_CornersRound pins the macOS icon shape: a fully opaque
// square masked with the Apple body radius keeps full alpha on the straight
// edges and center, loses the corners, and anti-aliases the arc.
func TestSquircleMask_CornersRound(t *testing.T) {
	const size = 164 // 2x the 82px grid reference for sub-pixel headroom
	img := opaqueSquare(size, size)
	radius := float64(size) * 185.4 / 824.0 // Apple body radius on the 824 grid
	squircleMask(img, radius)

	if a := alphaAt(img, 0, 0); a > 10 {
		t.Fatalf("top-left corner must be transparent, got alpha %d", a)
	}
	if a := alphaAt(img, size-1, size-1); a > 10 {
		t.Fatalf("bottom-right corner must be transparent, got alpha %d", a)
	}
	if a := alphaAt(img, size/2, 0); a != 255 {
		t.Fatalf("top edge midpoint is on the straight side, want alpha 255, got %d", a)
	}
	if a := alphaAt(img, size/2, size/2); a != 255 {
		t.Fatalf("center must stay opaque, got alpha %d", a)
	}
	// A point on the corner arc (45° toward the corner from its center) is
	// half covered — the mask must anti-alias, not clip hard.
	c := radius * (1 - 1/math.Sqrt2)
	a := alphaAt(img, int(c), int(c))
	if a <= 10 || a >= 245 {
		t.Fatalf("arc point (%.1f,%.1f) must be partially covered, got alpha %d", c, c, a)
	}
}

// TestCompose_CornerAlphaInOutput runs the full composition end-to-end and
// reads the written PNG back: the bundled artwork must carry the rounded
// corners (this is the defect fix — previously a sharp square reached the
// Dock).
func TestCompose_CornerAlphaInOutput(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "brand.png")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, opaqueSquare(256, 256)); err != nil {
		t.Fatal(err)
	}
	f.Close()

	const canvas = 64
	out := filepath.Join(dir, "out.png")
	if err := compose(src, canvas, out); err != nil {
		t.Fatal(err)
	}
	g, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	decoded, err := png.Decode(g)
	if err != nil {
		t.Fatal(err)
	}
	alphaAtDecoded := func(x, y int) uint8 {
		_, _, _, a := decoded.At(x, y).RGBA()
		return uint8(a >> 8)
	}
	var content = int(math.Round(float64(canvas) * contentRatio))
	off := (canvas - content) / 2
	if a := alphaAtDecoded(off, off); a > 10 {
		t.Fatalf("composed artwork corner must be transparent, got alpha %d", a)
	}
	if a := alphaAtDecoded(canvas/2, canvas/2); a != 255 {
		t.Fatalf("composed artwork center must be opaque, got alpha %d", a)
	}
}
