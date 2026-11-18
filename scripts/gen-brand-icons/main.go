// Command gen-brand-icons composes the macOS iconset PNGs and the runtime
// Dock icon from the brand PNGs in web/admin/public/brand, adding the
// macOS-standard transparent margin so huan matches neighboring app icons
// in the Dock (Apple icon grid: content 824 / canvas 1024). Stdlib only —
// plugin sub-modules replace the root module and must not inherit any new
// tool dependency.
//
// Usage (from the repo root, usually via gen-brand-icons.sh):
//
//	go run ./scripts/gen-brand-icons <iconset-dir> <assets-dir>
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
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
	content := int(float64(canvas) * contentRatio)
	scaled := areaScale(src, content, content)
	dst := image.NewRGBA(image.Rect(0, 0, canvas, canvas))
	off := (canvas - content) / 2
	draw.Draw(dst, image.Rect(off, off, off+content, off+content), scaled, scaled.Bounds().Min, draw.Src)
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, dst)
}

// areaScale downscales src to w×h by averaging the source rectangle that
// maps to each destination pixel (box filter, fractional edge coverage).
// RGBA() returns alpha-premultiplied values and SetRGBA expects them, so
// the average composites correctly onto the transparent canvas.
func areaScale(src image.Image, w, h int) *image.RGBA {
	sb := src.Bounds()
	sx0, sy0 := float64(sb.Min.X), float64(sb.Min.Y)
	sxf, syf := float64(sb.Dx())/float64(w), float64(sb.Dy())/float64(h)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for dy := 0; dy < h; dy++ {
		y0 := sy0 + float64(dy)*syf
		y1 := sy0 + float64(dy+1)*syf
		for dx := 0; dx < w; dx++ {
			x0 := sx0 + float64(dx)*sxf
			x1 := sx0 + float64(dx+1)*sxf
			var rs, gs, bs, as, area float64
			for sy := int(y0); float64(sy) < y1; sy++ {
				hf := overlap(y0, y1, float64(sy), float64(sy+1))
				for sx := int(x0); float64(sx) < x1; sx++ {
					wf := overlap(x0, x1, float64(sx), float64(sx+1))
					r, g, b, a := src.At(sx, sy).RGBA()
					wgt := hf * wf
					rs += wgt * float64(r>>8)
					gs += wgt * float64(g>>8)
					bs += wgt * float64(b>>8)
					as += wgt * float64(a>>8)
					area += wgt
				}
			}
			if area > 0 {
				dst.SetRGBA(dx, dy, color.RGBA{
					R: uint8(rs/area + 0.5),
					G: uint8(gs/area + 0.5),
					B: uint8(bs/area + 0.5),
					A: uint8(as/area + 0.5),
				})
			}
		}
	}
	return dst
}

// overlap returns the overlap length of the half-open ranges [a0,a1) and
// [b0,b1).
func overlap(a0, a1, b0, b1 float64) float64 {
	lo := math.Max(a0, b0)
	hi := math.Min(a1, b1)
	if hi <= lo {
		return 0
	}
	return hi - lo
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
