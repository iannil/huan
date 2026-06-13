package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchetypeKind(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"posts/my-post.md", "posts"},
		{"books/v1/ch01.md", "books"},
		{"gallery/img.jpg", "gallery"},
		{"my-post.md", ""},
		{"index.md", ""},
		{"deeply/nested/path.md", "deeply"},
		{"", ""},
	}
	for _, c := range cases {
		if got := archetypeKind(c.in); got != c.want {
			t.Errorf("archetypeKind(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRenderArchetype_KindSpecific(t *testing.T) {
	tmp := t.TempDir()
	oldSource := sourceDir
	sourceDir = tmp
	defer func() { sourceDir = oldSource }()

	archetypeDir := filepath.Join(tmp, "archetypes")
	if err := os.MkdirAll(archetypeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	posts := "---\ntitle: {{ .Name }}\ndate: {{ .Date }}\nkind: post\n---\n"
	if err := os.WriteFile(filepath.Join(archetypeDir, "posts.md"), []byte(posts), 0o644); err != nil {
		t.Fatal(err)
	}
	def := "---\ntitle: default\n---\n"
	if err := os.WriteFile(filepath.Join(archetypeDir, "default.md"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := renderArchetype("posts/hello-world.md")
	if err != nil {
		t.Fatalf("renderArchetype: %v", err)
	}
	if !strings.Contains(got, "kind: post") {
		t.Errorf("expected posts.md archetype, got: %s", got)
	}
	if !strings.Contains(got, "title: hello-world") {
		t.Errorf("expected .Name substituted, got: %s", got)
	}
}

func TestRenderArchetype_FallbackToDefault(t *testing.T) {
	tmp := t.TempDir()
	oldSource := sourceDir
	sourceDir = tmp
	defer func() { sourceDir = oldSource }()

	archetypeDir := filepath.Join(tmp, "archetypes")
	if err := os.MkdirAll(archetypeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	def := "---\ntitle: {{ .Name }}\ndate: {{ .Date }}\n---\n"
	if err := os.WriteFile(filepath.Join(archetypeDir, "default.md"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := renderArchetype("posts/hello.md")
	if err != nil {
		t.Fatalf("renderArchetype: %v", err)
	}
	if !strings.Contains(got, "title: hello") {
		t.Errorf("expected default archetype, got: %s", got)
	}
}

func TestRenderArchetype_BuiltinDefault(t *testing.T) {
	tmp := t.TempDir()
	oldSource := sourceDir
	sourceDir = tmp
	defer func() { sourceDir = oldSource }()

	got, err := renderArchetype("posts/hello-world.md")
	if err != nil {
		t.Fatalf("renderArchetype: %v", err)
	}
	if !strings.Contains(got, "title: ") {
		t.Errorf("expected built-in default with title, got: %s", got)
	}
	if !strings.Contains(got, "draft: true") {
		t.Errorf("expected built-in default with draft:true, got: %s", got)
	}
	if !strings.Contains(got, `"Hello World"`) {
		t.Errorf("expected title-cased name, got: %s", got)
	}
}

func TestRenderArchetype_NoKind(t *testing.T) {
	tmp := t.TempDir()
	oldSource := sourceDir
	sourceDir = tmp
	defer func() { sourceDir = oldSource }()

	archetypeDir := filepath.Join(tmp, "archetypes")
	if err := os.MkdirAll(archetypeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	def := "---\ntitle: default\n---\n"
	if err := os.WriteFile(filepath.Join(archetypeDir, "default.md"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := renderArchetype("hello.md")
	if err != nil {
		t.Fatalf("renderArchetype: %v", err)
	}
	if !strings.Contains(got, "title: default") {
		t.Errorf("expected default archetype, got: %s", got)
	}
}
