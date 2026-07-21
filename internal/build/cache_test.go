package build

import (
	"testing"
)

func TestNewPipelineCache_Empty(t *testing.T) {
	c := NewPipelineCache()
	if c == nil {
		t.Fatal("NewPipelineCache returned nil")
	}
	if c.Templates != nil {
		t.Error("Templates should be nil for fresh cache")
	}
	if c.BuiltAt.IsZero() {
		t.Error("BuiltAt should be set")
	}
}

func TestHasTemplateChanges_LayoutsFile(t *testing.T) {
	changed := []string{"/site/layouts/_default/single.html"}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("layouts/ change should trigger full build")
	}
}

func TestHasTemplateChanges_ContentFile(t *testing.T) {
	changed := []string{"/site/content/posts/hello.md"}
	if HasTemplateChanges(changed, "/site") {
		t.Error("content/ change should NOT trigger full build")
	}
}

func TestHasTemplateChanges_HuanYaml(t *testing.T) {
	changed := []string{"/site/huan.yaml"}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("huan.yaml change should trigger full build")
	}
}

func TestHasTemplateChanges_I18nFile(t *testing.T) {
	changed := []string{"/site/i18n/en.yaml"}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("i18n/ change should trigger full build")
	}
}

func TestHasTemplateChanges_ThemesFile(t *testing.T) {
	changed := []string{"/site/themes/zozo/layouts/baseof.html"}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("themes/ change should trigger full build")
	}
}

func TestHasTemplateChanges_MixedFiles(t *testing.T) {
	// One content, one template → should trigger (template change wins)
	changed := []string{
		"/site/content/posts/hello.md",
		"/site/layouts/_default/list.html",
	}
	if !HasTemplateChanges(changed, "/site") {
		t.Error("mixed files with one template change should trigger full build")
	}
}

func TestHasTemplateChanges_EmptyList(t *testing.T) {
	if HasTemplateChanges([]string{}, "/site") {
		t.Error("empty change list should not trigger full build")
	}
}

func TestHasTemplateChanges_OutsideSourceDir(t *testing.T) {
	// File outside sourceDir (Rel fails or gives long path) → not a template change
	changed := []string{"/other/path/file.html"}
	if HasTemplateChanges(changed, "/site") {
		t.Error("file outside sourceDir should not trigger full build")
	}
}
