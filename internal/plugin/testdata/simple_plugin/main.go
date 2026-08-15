package main

type simplePlugin struct {
	name    string
	version string
}

func (p *simplePlugin) Name() string { return p.name }

func (p *simplePlugin) Version() string { return p.version }

// InitPlugin 是 Loader 查找的导出符号。
// 契约（internal/plugin/loader.go:136）：返回 interface{}（自包含类型，避免
// 跨模块接口身份校验失败），而非 plugin.Plugin——loader 侧做 Plugin 断言。
func InitPlugin(cfg map[string]any) (interface{}, error) {
	name := "simple-test"
	if v, ok := cfg["name"].(string); ok && v != "" {
		name = v
	}
	return &simplePlugin{name: name, version: "1.0.0"}, nil
}
