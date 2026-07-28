package main

// InitPlugin is the exported symbol for .so plugin loading.
// The loader calls this with the plugin's config map (from huan.yaml).
func InitPlugin(cfg map[string]any) (interface{}, error) {
	return &ZhurongshuoTheme{}, nil
}

// main is required for the plugin package but does not execute.
func main() {}
