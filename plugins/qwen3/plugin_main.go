package main

// InitPlugin is the symbol exported for .so plugin loading.
// The loader calls this with the plugin's config map (from yaml).
// The _project_root key is set by the loader to the project root directory.
func InitPlugin(cfg map[string]any) (interface{}, error) {
	parsedCfg, err := ParseConfig(cfg)
	if err != nil {
		return nil, err
	}
	projectRoot := ""
	if v, ok := cfg["_project_root"].(string); ok {
		projectRoot = v
	}
	return New(parsedCfg, projectRoot)
}