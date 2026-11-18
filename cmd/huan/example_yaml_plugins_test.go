package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/plugin"
	"gopkg.in/yaml.v3"
)

// bundledPluginNames derives the Name() of every plugin shipped under
// plugins/, using the same detection rule as scripts/build-plugins.sh
// (first `Name() string { return "..." }` match per plugin directory).
// Deriving at test time keeps the example-yaml contract in sync with the
// actual plugin set instead of hardcoding a list that can drift.
func bundledPluginNames(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..", "plugins")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read plugins dir: %v", err)
	}
	re := regexp.MustCompile(`Name\(\) string \{ return "([^"]+)"`)
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}
		for _, file := range matches {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			if m := re.FindSubmatch(src); m != nil {
				names = append(names, string(m[1]))
				break
			}
		}
	}
	if len(names) == 0 {
		t.Fatal("no bundled plugin Name() found under plugins/")
	}
	return names
}

// TestExampleYAMLDeclaresOnlyBundledPlugins guards the example config's
// contract: every plugin declared in huan.example.yaml must resolve to an
// huan-shipped .so, so running `huan` against the example file produces no
// plugin config warnings. A site-specific plugin (e.g. zhurongshuo) belongs
// in that site's own huan.yaml, not here.
func TestExampleYAMLDeclaresOnlyBundledPlugins(t *testing.T) {
	t.Setenv("HUAN_HOME", t.TempDir()) // isolate from the real ~/.huan

	// Stage resolvable (dummy) .so files for exactly the bundled plugins;
	// a declared plugin outside this set cannot resolve and must warn.
	pluginDir := t.TempDir()
	for _, name := range bundledPluginNames(t) {
		so := strings.ReplaceAll(name, "_", "-") + ".so" // loader naming convention
		if err := os.WriteFile(filepath.Join(pluginDir, so), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	src, err := os.ReadFile(filepath.Join("..", "..", "huan.example.yaml"))
	if err != nil {
		t.Fatalf("read huan.example.yaml: %v", err)
	}
	var doc struct {
		Plugins map[string]config.PluginConfig `yaml:"plugins"`
	}
	if err := yaml.Unmarshal(src, &doc); err != nil {
		t.Fatalf("parse huan.example.yaml: %v", err)
	}
	if len(doc.Plugins) == 0 {
		t.Fatal("huan.example.yaml declares no plugins")
	}

	reg, err := plugin.NewRegistryFromConfig(
		&config.Config{Plugins: doc.Plugins},
		t.TempDir(), // sourceDir (unused: pluginDir override below)
		pluginDir,
	)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	if len(reg.ConfigWarnings) > 0 {
		t.Errorf("huan.example.yaml produces plugin config warnings (every declared plugin must be bundled with huan):\n  - %s",
			strings.Join(reg.ConfigWarnings, "\n  - "))
	}
}
