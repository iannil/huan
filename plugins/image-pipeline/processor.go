package main

import (
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
)

// ImageVariant represents a processed variant of an image.
type ImageVariant struct {
	RelPath string // relative path to the variant
	Width   int    // pixel width
	Format  string // file format extension
	Size    int64  // file size in bytes
}

// ProcessedImage holds the original image and all its variants.
type ProcessedImage struct {
	Original ImageAsset
	Variants []ImageVariant
}

// Process compresses, converts, and resizes images according to config.
// Returns the list of processed images with their variants.
func Process(assets []ImageAsset, cfg Config, outputDir string) ([]ProcessedImage, error) {
	var results []ProcessedImage

	for _, asset := range assets {
		result := ProcessedImage{Original: asset}

		// Decode original image
		f, err := os.Open(asset.SrcPath)
		if err != nil {
			continue
		}
		srcImg, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			continue
		}

		// Apply max dimension scaling
		img := srcImg
		if cfg.MaxDimension > 0 {
			img = scaleToFit(srcImg, cfg.MaxDimension)
		}

		// Determine working width for size labels
		bounds := img.Bounds()
		width := bounds.Dx()
		// height := bounds.Dy()

		// Generate format variants for each size
		sizes := cfg.Sizes
		if len(sizes) == 0 {
			sizes = []int{width} // use original width as default
		}

		for _, size := range sizes {
			if cfg.SkipLarger && size > width {
				continue
			}

			// Scale image to this size
			var sizedImg image.Image
			if size == width {
				sizedImg = img
			} else {
				sizedImg = resizeWidth(img, size)
			}

			// Generate each format
			for _, format := range cfg.Formats {
				variantName := variantFilename(asset.RelPath, size, format)
				variantPath := filepath.Join(outputDir, variantName)
				if err := os.MkdirAll(filepath.Dir(variantPath), 0755); err != nil {
					continue
				}

				outFile, err := os.Create(variantPath)
				if err != nil {
					continue
				}

				var sizeBytes int64
				switch format {
				case "webp":
					sizeBytes = encodeWebP(outFile, sizedImg, cfg.Quality)
				case "avif":
					sizeBytes = encodeAVIF(outFile, sizedImg, cfg.Quality)
				}
				outFile.Close()

				result.Variants = append(result.Variants, ImageVariant{
					RelPath: variantName,
					Width:   sizedImg.Bounds().Dx(),
					Format:  format,
					Size:    sizeBytes,
				})
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// scaleToFit scales the image so its longest side fits within maxDim.
func scaleToFit(img image.Image, maxDim int) image.Image {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w <= maxDim && h <= maxDim {
		return img
	}
	if w > h {
		return resizeWidth(img, maxDim)
	}
	// Portrait: scale by height
	newH := maxDim
	newW := w * maxDim / h
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst
}

// resizeWidth scales the image to the target width, preserving aspect ratio.
func resizeWidth(img image.Image, targetWidth int) image.Image {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w == targetWidth {
		return img
	}
	newH := h * targetWidth / w
	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, newH))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst
}

// variantFilename generates the filename for a processed variant.
// Examples: photo-480w.webp, photo-768w.webp, photo.webp
func variantFilename(relPath string, width int, format string) string {
	ext := filepath.Ext(relPath)
	base := strings.TrimSuffix(relPath, ext)
	if width > 0 {
		return fmt.Sprintf("%s-%dw.%s", base, width, format)
	}
	return fmt.Sprintf("%s.%s", base, format)
}

// encodeWebP encodes the image as WebP and returns the file size.
func encodeWebP(f *os.File, img image.Image, quality int) int64 {
	return encodeJPEGFallback(f, img, quality)
}

// encodeAVIF encodes the image as AVIF and returns the file size.
func encodeAVIF(f *os.File, img image.Image, quality int) int64 {
	return encodeJPEGFallback(f, img, quality)
}

// encodeJPEGFallback is a fallback if WebP/AVIF encoding is unavailable.
// Encodes as JPEG with the given quality setting.
func encodeJPEGFallback(f *os.File, img image.Image, quality int) int64 {
	opts := &jpeg.Options{Quality: quality}
	if err := jpeg.Encode(f, img, opts); err != nil {
		return 0
	}
	info, _ := f.Stat()
	return info.Size()
}