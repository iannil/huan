package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// Cache stores rendered SVGs on disk keyed by a content hash of (lang, source).
type Cache struct {
	dir string
}

// NewCache returns a Cache rooted at dir.
func NewCache(dir string) *Cache { return &Cache{dir: dir} }

// Key returns the hex sha256 of lang + "\n" + source.
func (c *Cache) Key(lang, source string) string {
	h := sha256.Sum256([]byte(lang + "\n" + source))
	return hex.EncodeToString(h[:])
}

// Get returns the cached SVG for key, or ("", false) on miss.
func (c *Cache) Get(key string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(c.dir, key+".svg"))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// Put writes svg to the cache under key, creating the cache dir if needed.
func (c *Cache) Put(key, svg string) error {
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.dir, key+".svg"), []byte(svg), 0644)
}
