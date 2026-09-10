package main

// InitPlugin is the exported symbol for .so plugin loading.
// The loader calls this with the plugin's config map (from huan.yaml).
func InitPlugin(cfg map[string]any) (interface{}, error) {
	parsed, err := ParseConfig(cfg)
	if err != nil {
		return nil, err
	}
	return New(parsed), nil
}

// main is required for the plugin package but does not execute.
func main() {}
