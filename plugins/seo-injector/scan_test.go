package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCollectHTMLFiles_Recursive proves the output scan finds HTML at every
// depth — root-level (e.g. index.html), one level deep, and two+ levels deep.
// The old filepath.Glob("**/*.html") matched only files exactly one directory
// deep, silently skipping root and nested pages.
func TestCollectHTMLFiles_Recursive(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("<html></html>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("index.html")             // root level
	write("posts/index.html")       // one level deep
	write("posts/a/b/deep.html")    // three levels deep
	write("assets/style.css")       // non-HTML, must be excluded

	got, err := collectHTMLFiles(root)
	if err != nil {
		t.Fatalf("collectHTMLFiles: %v", err)
	}

	want := map[string]bool{
		filepath.Join(root, "index.html"):          true,
		filepath.Join(root, "posts/index.html"):     true,
		filepath.Join(root, "posts/a/b/deep.html"):  true,
	}
	if len(got) != len(want) {
		t.Fatalf("found %d HTML files, want %d: %v", len(got), len(want), got)
	}
	for _, f := range got {
		if !want[f] {
			t.Errorf("unexpected file (css should be excluded): %s", f)
		}
	}
}
