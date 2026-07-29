package main

// InitPlugin is the exported symbol the .so plugin loader looks up. It parses
// the raw config map and returns a DiagramRenderer instance.
func InitPlugin(cfg map[string]any) (interface{}, error) {
	parsedCfg, err := ParseConfig(cfg)
	if err != nil {
		return nil, err
	}
	return New(parsedCfg), nil
}

func main() {}
