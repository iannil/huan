package main

import (
	"path/filepath"
	"testing"
)

func TestDevMarkdownOnlyChanges(t *testing.T) {
	source := t.TempDir()
	for _, tt := range []struct {
		name  string
		paths []string
		want  bool
	}{
		{"manual", nil, false},
		{"empty", []string{}, false},
		{"body and sidecar", []string{filepath.Join(source, "content/posts/a.md"), filepath.Join(source, "content/posts/a.en.md")}, true},
		{"deleted markdown", []string{filepath.Join(source, "content/removed.md")}, true},
		{"config", []string{filepath.Join(source, "huan.yaml")}, false},
		{"mixed", []string{filepath.Join(source, "content/a.md"), filepath.Join(source, "static/a.css")}, false},
		{"directory", []string{filepath.Join(source, "content/posts")}, false},
		{"outside", []string{filepath.Join(source, "../content/a.md")}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := devMarkdownOnlyChanges(source, tt.paths); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}
