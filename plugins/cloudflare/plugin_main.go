package main

import (
	"github.com/iannil/huan-plugin-cloudflare/plugin"
)

// InitPlugin is the exported symbol that the .so plugin loader looks up.
// It receives a raw config map, parses it, and returns a Plugin instance.
func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
	parsedCfg, err := ParseConfig(cfg)
	if err != nil {
		return nil, err
	}
	return New(parsedCfg), nil
}
