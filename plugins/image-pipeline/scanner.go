package main

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

// ImageAsset represents a single image found in the output directory.
type ImageAsset struct {
	SrcPath string // absolute path to the source file
	RelPath string // path relative to output directory
	Width   int    // image width in pixels
	Height  int    // image height in pixels
	Size    int64  // file size in bytes
	Format  string // image format (jpeg, png, gif)
}

// Scan walks the output directory and collects all image files.
// Returns the list of images found, or an error if the directory can't be read.
func Scan(outputDir string) ([]ImageAsset, error) {
	info, err := os.Stat(outputDir)
	if err != nil {
		return nil, fmt.Errorf("image_pipeline: scan %s: %w", outputDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("image_pipeline: %s is not a directory", outputDir)
	}

	var assets []ImageAsset
	err = filepath.Walk(outputDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".gif" {
			return nil
		}
		// Skip already-processed variants (e.g. photo-480w.jpg)
		base := filepath.Base(path)
		if strings.Contains(base, "-") {
			return nil
		}

		relPath, err := filepath.Rel(outputDir, path)
		if err != nil {
			return err
		}

		f, err := os.Open(path)
		if err != nil {
			return nil // skip unreadable files
		}
		defer f.Close()

		cfg, _, err := image.DecodeConfig(f)
		if err != nil {
			return nil // skip if not a valid image
		}

		assets = append(assets, ImageAsset{
			SrcPath: path,
			RelPath: filepath.ToSlash(relPath),
			Width:   cfg.Width,
			Height:  cfg.Height,
			Size:    fi.Size(),
			Format:  ext[1:], // strip leading "."
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return assets, nil
}
