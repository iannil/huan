package main

// ImagePipelinePlugin processes images during build: compress, convert
// formats, generate multi-size variants, and inject srcset/picture in HTML.
type ImagePipelinePlugin struct {
	cfg Config
}

// Name returns the plugin identifier.
func (p *ImagePipelinePlugin) Name() string { return "image_pipeline" }

// Process runs the full image pipeline: scan -> process -> inject.
func (p *ImagePipelinePlugin) Process(outputDir, sourceDir string) error {
	// TODO: implement in subsequent tasks
	return nil
}