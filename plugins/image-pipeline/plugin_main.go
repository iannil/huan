package main

// InitPlugin is the exported symbol for .so plugin loading.
func InitPlugin(cfg map[string]any) (interface{}, error) {
	parsedCfg, err := ParseConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &ImagePipelinePlugin{cfg: parsedCfg}, nil
}

// main is required for go build to succeed but is unused.
// This plugin is built as a .so file and loaded dynamically.
func main() {}