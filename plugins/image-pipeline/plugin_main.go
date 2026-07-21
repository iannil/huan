package main

import "github.com/iannil/huan-plugin-image-pipeline/plugin"

// InitPlugin is the exported symbol for .so plugin loading.
func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
	parsedCfg, err := ParseConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &ImagePipelinePlugin{cfg: parsedCfg}, nil
}