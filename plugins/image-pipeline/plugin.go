package main

import (
	"fmt"

	pkgimage "github.com/iannil/huan/pkg/image"
)

// ImagePipelinePlugin processes images during build: compress, convert
// formats, generate multi-size variants, and inject srcset/picture in HTML.
type ImagePipelinePlugin struct {
	cfg Config
}

// Compile-time proof that ImagePipelinePlugin satisfies the shared host
// contract. If Process's signature ever diverges from pkg/image (e.g. a config
// struct is added), this fails at plugin build time — catching the mismatch
// before it becomes a silent runtime "no image processor" like the 2026-07-28
// deploy/translate bugs.
var _ pkgimage.ImageProcessor = (*ImagePipelinePlugin)(nil)

// Name returns the plugin identifier.
func (p *ImagePipelinePlugin) Name() string { return "image_pipeline" }

// Process runs the full image pipeline: scan -> process -> inject.
func (p *ImagePipelinePlugin) Process(outputDir, sourceDir string) error {
	// 1. Scan output directory for images
	assets, err := Scan(outputDir)
	if err != nil {
		return fmt.Errorf("image_pipeline: scan: %w", err)
	}
	if len(assets) == 0 {
		return nil // no images to process
	}

	// 2. Process images (compress, convert, resize)
	results, err := Process(assets, p.cfg, outputDir)
	if err != nil {
		return fmt.Errorf("image_pipeline: process: %w", err)
	}

	// 3. Inject srcset/picture into HTML files
	if p.cfg.InjectSrcset || p.cfg.InjectPicture {
		if err := InjectHTMLFiles(outputDir, results, p.cfg); err != nil {
			return fmt.Errorf("image_pipeline: html inject: %w", err)
		}
	}

	return nil
}