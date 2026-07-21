### Task 3: 实现 Config 解析

**Files:**
- Create: `plugins/image-pipeline/options.go` — Config + ParseConfig + 默认值

- [ ] **Step 1: 编写测试**

`plugins/image-pipeline/options_test.go`：

```go
package main

import (
    "testing"
)

func TestParseConfig_Defaults(t *testing.T) {
    cfg, err := ParseConfig(map[string]any{})
    if err != nil {
        t.Fatalf("ParseConfig empty: %v", err)
    }
    if len(cfg.Formats) != 1 || cfg.Formats[0] != "webp" {
        t.Errorf("default Formats = %v, want [webp]", cfg.Formats)
    }
    if cfg.Quality != 80 {
        t.Errorf("default Quality = %d, want 80", cfg.Quality)
    }
    if !cfg.InjectSrcset {
        t.Error("default InjectSrcset should be true")
    }
    if !cfg.InjectPicture {
        t.Error("default InjectPicture should be true")
    }
    if !cfg.InjectLazyLoading {
        t.Error("default InjectLazyLoading should be true")
    }
    if !cfg.SkipLarger {
        t.Error("default SkipLarger should be true")
    }
}

func TestParseConfig_Override(t *testing.T) {
    cfg, err := ParseConfig(map[string]any{
        "quality": 90,
        "formats": []any{"webp", "avif"},
        "sizes":   []any{480, 768, 1200},
        "inject_srcset": false,
        "max_dimension": 2048,
    })
    if err != nil {
        t.Fatalf("ParseConfig: %v", err)
    }
    if cfg.Quality != 90 {
        t.Errorf("Quality = %d, want 90", cfg.Quality)
    }
    if len(cfg.Formats) != 2 || cfg.Formats[1] != "avif" {
        t.Errorf("Formats = %v, want [webp avif]", cfg.Formats)
    }
    if len(cfg.Sizes) != 3 || cfg.Sizes[1] != 768 {
        t.Errorf("Sizes = %v, want [480 768 1200]", cfg.Sizes)
    }
    if cfg.InjectSrcset {
        t.Error("InjectSrcset should be false")
    }
    if cfg.MaxDimension != 2048 {
        t.Errorf("MaxDimension = %d, want 2048", cfg.MaxDimension)
    }
}

func TestParseConfig_InvalidFormats(t *testing.T) {
    _, err := ParseConfig(map[string]any{
        "formats": []any{"gif"},
    })
    if err == nil {
        t.Error("expected error for invalid format 'gif'")
    }
}

func TestParseConfig_InvalidQuality(t *testing.T) {
    _, err := ParseConfig(map[string]any{
        "quality": 150,
    })
    if err == nil {
        t.Error("expected error for quality > 100")
    }
}
```

- [ ] **Step 2: 实现 Config 和 ParseConfig**

`plugins/image-pipeline/options.go`：

```go
package main

import (
    "fmt"
    "gopkg.in/yaml.v3"
)

// Config is the typed image pipeline plugin configuration.
type Config struct {
    Formats           []string `yaml:"formats"`
    Quality           int      `yaml:"quality"`
    Sizes             []int    `yaml:"sizes"`
    InjectSrcset      bool     `yaml:"inject_srcset"`
    InjectPicture     bool     `yaml:"inject_picture"`
    InjectLazyLoading bool     `yaml:"inject_lazy_loading"`
    MaxDimension      int      `yaml:"max_dimension"`
    SkipLarger        bool     `yaml:"skip_larger"`
}

// defaults sets sensible defaults for unset fields.
func (c *Config) defaults() {
    if c.Formats == nil {
        c.Formats = []string{"webp"}
    }
    if c.Quality == 0 {
        c.Quality = 80
    }
    if c.Sizes == nil {
        c.Sizes = nil
    }
    c.InjectSrcset = true
    c.InjectPicture = true
    c.InjectLazyLoading = true
    c.SkipLarger = true
}

// validate returns an error if config is invalid.
func (c Config) validate() error {
    validFormats := map[string]bool{"webp": true, "avif": true}
    for _, f := range c.Formats {
        if !validFormats[f] {
            return fmt.Errorf("image_pipeline: unsupported format %q (supported: webp, avif)", f)
        }
    }
    if c.Quality < 1 || c.Quality > 100 {
        return fmt.Errorf("image_pipeline: quality must be 1-100, got %d", c.Quality)
    }
    for _, s := range c.Sizes {
        if s < 16 {
            return fmt.Errorf("image_pipeline: size %d too small (min 16px)", s)
        }
    }
    if c.MaxDimension < 0 {
        return fmt.Errorf("image_pipeline: max_dimension must be >= 0, got %d", c.MaxDimension)
    }
    return nil
}

// ParseConfig decodes the raw config map into Config with defaults + validation.
func ParseConfig(raw map[string]any) (Config, error) {
    data, err := yaml.Marshal(raw)
    if err != nil {
        return Config{}, fmt.Errorf("image_pipeline: re-encode config: %w", err)
    }
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return Config{}, fmt.Errorf("image_pipeline: decode config: %w", err)
    }
    cfg.defaults()
    if err := cfg.validate(); err != nil {
        return Config{}, err
    }
    return cfg, nil
}
```

- [ ] **Step 3: 运行测试**

```bash
cd plugins/image-pipeline && go test -v -run "TestParseConfig" .
```

- [ ] **Step 4: 提交**

```bash
git add plugins/image-pipeline/options.go plugins/image-pipeline/options_test.go
git commit -m "feat(image-pipeline): add config parsing with defaults and validation"
```

---

