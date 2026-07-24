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
//
// Registration model: image_pipeline is a .so-only plugin — it is not
// compiled-in. This function creates a temporary Loader to load it, then
// registers it into the provided registry so that it can be discovered via
// plugin.Find[image.ImageProcessor].
func runImagePipeline(cfg *config.Config, sourceDir, outputDir string, registry *plugin.Registry) error {
	raw, ok := cfg.Plugins["image_pipeline"]
	if !ok {
		return nil // not configured
	}

	// Check if already registered (e.g. by LifecycleManager)
	processors := plugin.Find[image.ImageProcessor](registry)
	if len(processors) > 0 {
		for _, p := range processors {
			if err := p.Process(outputDir, sourceDir); err != nil {
				return fmt.Errorf("image_pipeline: process: %w", err)
			}
		}
		return nil
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

	// Register into the provided registry so it's discoverable
	_ = registry.Register(p)

	// Execute the image processing pipeline
	return processor.Process(outputDir, sourceDir)
}