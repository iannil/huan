package main

import "github.com/iannil/huan-plugin-zhurongshuo/plugin"

// InitPlugin is the exported symbol for .so plugin loading.
// The loader calls this with the plugin's config map (from huan.yaml).
func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
	return &ZhurongshuoTheme{}, nil
}

// main is required for the plugin package but does not execute.
func main() {}
