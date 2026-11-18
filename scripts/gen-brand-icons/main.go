// Command gen-brand-icons composes the macOS iconset PNGs and the runtime
// Dock icon from the brand PNGs in web/admin/public/brand, adding the
// macOS-standard transparent margin so huan matches neighboring app icons
// in the Dock (Apple icon grid: content 824 / canvas 1024).
//
// Usage (from the repo root, usually via gen-brand-icons.sh):
//
//	go run ./scripts/gen-brand-icons <iconset-dir> <assets-dir>
package main

import (
	"image"
	"image/png"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

// contentRatio is the Apple icon grid ratio: 824px of artwork on a
// 1024px canvas.
const contentRatio = 824.0 / 1024.0

// entries maps each iconset file to its canvas size and brand source.
var entries = []struct {
	name   string
	canvas int
	source string
}{
	{"icon_16x16.png", 16, "huan-icon-16.png"},
	{"icon_16x16@2x.png", 32, "huan-icon-32.png"},
	{"icon_32x32.png", 32, "huan-icon-32.png"},
	{"icon_32x32@2x.png", 64, "huan-icon-64.png"},
	{"icon_128x128.png", 128, "huan-icon-128.png"},
	{"icon_128x128@2x.png", 256, "huan-icon-256.png"},
	{"icon_256x256.png", 256, "huan-icon-256.png"},
	{"icon_256x256@2x.png", 512, "huan-icon-512.png"},
	{"icon_512x512.png", 512, "huan-icon-512.png"},
	{"icon_512x512@2x.png", 1024, "huan-primary-1024.png"},
}

func main() {
	if len(os.Args) != 3 {
		panic("usage: gen-brand-icons <iconset-dir> <assets-dir>")
	}
	iconsetDir, assetsDir := os.Args[1], os.Args[2]
	brandDir := filepath.Join("web", "admin", "public", "brand")
	for _, e := range entries {
		if err := compose(filepath.Join(brandDir, e.source), e.canvas, filepath.Join(iconsetDir, e.name)); err != nil {
			panic(err)
		}
	}
	// The runtime Dock icon uses the same composition as the 512 bundle icon.
	if err := compose(filepath.Join(brandDir, "huan-icon-512.png"), 512, filepath.Join(assetsDir, "icon.png")); err != nil {
		panic(err)
	}
}

// compose scales src to contentRatio of a square canvas and centers it on a
// transparent background, writing the result as a PNG at out.
func compose(srcPath string, canvas int, out string) error {
	src, err := loadImage(srcPath)
	if err != nil {
		return err
	}
	dst := image.NewRGBA(image.Rect(0, 0, canvas, canvas))
	content := int(float64(canvas) * contentRatio)
	off := (canvas - content) / 2
	rect := image.Rect(off, off, off+content, off+content)
	draw.CatmullRom.Scale(dst, rect, src, src.Bounds(), draw.Over, nil)
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, dst)
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}
