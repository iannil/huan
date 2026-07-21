package main

import (
	"fmt"
	"path/filepath"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/image"
	"github.com/iannil/huan/internal/plugin"
)

// runImagePipeline loads and executes the image_pipeline plugin if configured.
// Returns nil if image_pipeline is not in cfg.Plugins.
// Returns an error if the plugin is configured but fails to load or execute.
func runImagePipeline(cfg *config.Config, sourceDir, outputDir string) error {
	raw, ok := cfg.Plugins["image_pipeline"]
	if !ok {
		return nil // not configured
	}

	// Load the .so plugin from <sourceDir>/plugins/image_pipeline.so
	pluginDir := filepath.Join(sourceDir, "plugins")
	loader := plugin.NewLoader(pluginDir)
	soPath := filepath.Join(pluginDir, "image_pipeline.so")

	p, err := loader.LoadPlugin(soPath, raw)
	if err != nil {
		return fmt.Errorf("image_pipeline: load plugin: %w", err)
	}

	// Type-assert to ImageProcessor capability interface
	processor, ok := p.(image.ImageProcessor)
	if !ok {
		return fmt.Errorf("image_pipeline: plugin does not implement ImageProcessor")
	}

	// Execute the image processing pipeline
	return processor.Process(outputDir, sourceDir)
}