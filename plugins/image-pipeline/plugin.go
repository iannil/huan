package main

import (
	"fmt"
)

// ImagePipelinePlugin processes images during build: compress, convert
// formats, generate multi-size variants, and inject srcset/picture in HTML.
type ImagePipelinePlugin struct {
	cfg Config
}

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