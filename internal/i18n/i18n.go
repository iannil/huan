// Package i18n loads translation files and resolves keys to translated strings.
package i18n

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Bundle holds translations for a single language.
type Bundle struct {
	strings map[string]string
}

// New creates an empty bundle.
func New() *Bundle {
	return &Bundle{strings: map[string]string{}}
}

// LoadFile loads a YAML translation file (Hugo format: key: {other: "value"}).
func (b *Bundle) LoadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw map[string]map[string]string
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key, m := range raw {
		if v, ok := m["other"]; ok {
			b.strings[key] = v
		}
	}
	return nil
}

// LoadDir loads all translation files from a directory (and subdirs).
// Each .yaml file is loaded into the bundle (later files override earlier).
func (b *Bundle) LoadDir(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		return b.LoadFile(path)
	})
}

// Translate returns the translation for the given key.
// If the key is not found, returns the key itself.
// Hugo's i18n supports arguments via fmt.Sprintf, but Hugo themes typically don't use them.
func (b *Bundle) Translate(key string, args ...interface{}) string {
	if v, ok := b.strings[key]; ok {
		return v
	}
	return key
}
