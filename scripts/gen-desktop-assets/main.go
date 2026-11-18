// Command gen-desktop-assets writes the huan desktop tray icon from the
// brand geometry (web/admin/public/brand/huan-primary.svg): three cream
// parallelograms on a red square, reduced to a black template glyph that
// macOS tints adaptively in the menu bar. Run once from the repo root:
//
//	go run ./scripts/gen-desktop-assets
package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// glyph holds the three brand parallelograms in huan-primary.svg coordinates
// (viewBox 0 0 1000 1000).
var glyph = [3][4][2]float64{
	{{240, 295}, {570, 205}, {760, 315}, {430, 405}},
	{{240, 470}, {570, 380}, {760, 490}, {430, 580}},
	{{240, 645}, {570, 555}, {760, 665}, {430, 755}},
}

func main() {
	outDir := filepath.Join("internal", "desktop", "assets")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}
	if err := writeTray(filepath.Join(outDir, "tray.png")); err != nil {
		panic(err)
	}
}

// writeTray renders the glyph into an 18x18 black+alpha template PNG,
// supersampled 8x with box downsampling for smooth edges. The glyph fills
// ~78% of the canvas, the macOS menu-bar convention (Template images stay
// black+alpha; macOS recolors for light/dark menus).
func writeTray(path string) error {
	const (
		size      = 18.0
		ss        = 8
		fillRatio = 0.78
	)
	minX, minY := glyph[0][0][0], glyph[0][0][1]
	maxX, maxY := minX, minY
	for _, poly := range glyph {
		for _, p := range poly {
			if p[0] < minX {
				minX = p[0]
			}
			if p[0] > maxX {
				maxX = p[0]
			}
			if p[1] < minY {
				minY = p[1]
			}
			if p[1] > maxY {
				maxY = p[1]
			}
		}
	}
	gw, gh := maxX-minX, maxY-minY
	scale := size * fillRatio / gw
	if size*fillRatio/gh < scale {
		scale = size * fillRatio / gh
	}
	ox := (size - gw*scale) / 2
	oy := (size - gh*scale) / 2

	// contains reports whether the canvas point (x, y) maps into any brand
	// parallelogram (convex quads: consistent cross-product sign test).
	contains := func(x, y float64) bool {
		bx := (x-ox)/scale + minX
		by := (y-oy)/scale + minY
		for _, poly := range glyph {
			sign := 0.0
			inside := true
			for i := 0; i < 4; i++ {
				a := poly[i]
				b := poly[(i+1)%4]
				cross := (b[0]-a[0])*(by-a[1]) - (b[1]-a[1])*(bx-a[0])
				if sign == 0 {
					sign = cross
				} else if (cross > 0) != (sign > 0) {
					inside = false
					break
				}
			}
			if inside {
				return true
			}
		}
		return false
	}

	img := image.NewNRGBA(image.Rect(0, 0, int(size), int(size)))
	for y := 0; y < int(size); y++ {
		for x := 0; x < int(size); x++ {
			count := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/float64(ss)
					py := float64(y) + (float64(sy)+0.5)/float64(ss)
					if contains(px, py) {
						count++
					}
				}
			}
			if count > 0 {
				a := uint8(count * 255 / (ss * ss))
				img.SetNRGBA(x, y, color.NRGBA{R: 0, G: 0, B: 0, A: a})
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
