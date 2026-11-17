// Command gen-desktop-assets writes placeholder tray and app icons for the
// huan desktop shell. Run once: go run ./scripts/gen-desktop-assets
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

func circle(m *image.NRGBA, cx, cy, r int, c color.Color) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				m.Set(x, y, c)
			}
		}
	}
}

func main() {
	outDir := filepath.Join("internal", "desktop", "assets")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}

	// 18x18 template tray icon: solid disc, black+alpha (macOS renders it adaptively).
	tray := image.NewNRGBA(image.Rect(0, 0, 18, 18))
	draw.Draw(tray, tray.Bounds(), image.Transparent, image.Point{}, draw.Src)
	circle(tray, 9, 9, 6, color.NRGBA{R: 0, G: 0, B: 0, A: 255})
	f, err := os.Create(filepath.Join(outDir, "tray.png"))
	if err != nil {
		panic(err)
	}
	if err := png.Encode(f, tray); err != nil {
		panic(err)
	}
	f.Close()

	// 512x512 app icon: dark rounded square with a light disc.
	const size = 512
	app := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.Draw(app, app.Bounds(), image.Transparent, image.Point{}, draw.Src)
	dark := color.NRGBA{R: 30, G: 41, B: 59, A: 255}
	for y := 64; y < size-64; y++ {
		for x := 64; x < size-64; x++ {
			app.Set(x, y, dark)
		}
	}
	circle(app, size/2, size/2, 110, color.NRGBA{R: 148, G: 163, B: 184, A: 255})
	f2, err := os.Create(filepath.Join(outDir, "icon.png"))
	if err != nil {
		panic(err)
	}
	if err := png.Encode(f2, app); err != nil {
		panic(err)
	}
	f2.Close()
}
