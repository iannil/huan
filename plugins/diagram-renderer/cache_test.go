package main

import (
	"path/filepath"
	"testing"
)

func TestCacheRoundTrip(t *testing.T) {
	c := NewCache(t.TempDir())
	k := c.Key("mermaid", "graph TD\nA-->B\n")
	if _, ok := c.Get(k); ok {
		t.Fatalf("expected miss on empty cache")
	}
	if err := c.Put(k, "<svg>ok</svg>"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok := c.Get(k)
	if !ok || got != "<svg>ok</svg>" {
		t.Errorf("Get = %q,%v", got, ok)
	}
}

func TestCacheKeyStableAndDistinct(t *testing.T) {
	c := NewCache(t.TempDir())
	k1 := c.Key("mermaid", "A")
	k2 := c.Key("mermaid", "A")
	k3 := c.Key("d2", "A")
	if k1 != k2 {
		t.Errorf("key not stable: %s != %s", k1, k2)
	}
	if k1 == k3 {
		t.Errorf("key not lang-sensitive")
	}
	if filepath.Ext(k1) != "" {
		t.Errorf("key must be bare hex, got %q", k1)
	}
}
