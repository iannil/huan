package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestRoundtrip(t *testing.T) {
	dir := t.TempDir()
	m := LoadManifest(dir) // empty on first load
	if m == nil {
		t.Fatal("nil manifest")
	}
	m.Entries["demo.zh"] = "hash123"
	if err := SaveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	m2 := LoadManifest(dir)
	if m2.Entries["demo.zh"] != "hash123" {
		t.Fatalf("roundtrip: %+v", m2.Entries)
	}
}

func TestComputeHashStable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	os.WriteFile(p, []byte("hello"), 0o644)
	h1 := ComputeHash([]string{p})
	h2 := ComputeHash([]string{p})
	if h1 == "" || h1 != h2 {
		t.Fatalf("unstable: %q vs %q", h1, h2)
	}
	os.WriteFile(p, []byte("world"), 0o644)
	if ComputeHash([]string{p}) == h1 {
		t.Fatal("content change must change hash")
	}
}
