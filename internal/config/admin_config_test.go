package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRequireTokenEnabled_DefaultIsTrue(t *testing.T) {
	var c Config
	if err := yaml.Unmarshal([]byte("title: t\n"), &c); err != nil {
		t.Fatal(err)
	}
	if !c.Admin.RequireTokenEnabled() {
		t.Fatal("absent admin section must default to requireToken=true")
	}
}

func TestRequireTokenEnabled_ExplicitValues(t *testing.T) {
	var off Config
	if err := yaml.Unmarshal([]byte("admin:\n  requireToken: false\n"), &off); err != nil {
		t.Fatal(err)
	}
	if off.Admin.RequireTokenEnabled() {
		t.Fatal("requireToken: false must disable")
	}
	var on Config
	if err := yaml.Unmarshal([]byte("admin:\n  requireToken: true\n"), &on); err != nil {
		t.Fatal(err)
	}
	if !on.Admin.RequireTokenEnabled() {
		t.Fatal("requireToken: true must stay enabled")
	}
}

func TestRequireTokenEnabled_LoadFromSiteYaml(t *testing.T) {
	dir := t.TempDir()
	yml := "baseURL: http://x/\ntitle: t\nadmin:\n  requireToken: false\n"
	if err := os.WriteFile(filepath.Join(dir, "huan.yaml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Admin.RequireTokenEnabled() {
		t.Fatal("site yaml admin.requireToken: false must load through config.Load")
	}
}
