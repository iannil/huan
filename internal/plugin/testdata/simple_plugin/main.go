package main

import "github.com/iannil/huan/internal/plugin"

type simplePlugin struct {
	name    string
	version string
}

func (p *simplePlugin) Name() string { return p.name }

// InitPlugin 是 Loader 查找的导出符号
func InitPlugin(cfg map[string]any) (plugin.Plugin, error) {
	name := "simple-test"
	if v, ok := cfg["name"].(string); ok && v != "" {
		name = v
	}
	return &simplePlugin{name: name, version: "1.0.0"}, nil
}
