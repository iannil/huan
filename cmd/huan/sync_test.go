package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGalleryDir_GeneratesNewMarkdown(t *testing.T) {
	tmp := t.TempDir()
	galleryDir := filepath.Join(tmp, "static", "images", "gallery")
	contentDir := filepath.Join(tmp, "content", "gallery")
	if err := os.MkdirAll(galleryDir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"photo1.jpg", "photo2.png"} {
		if err := os.WriteFile(filepath.Join(galleryDir, name), []byte("fake"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	total, generated, skipped, err := syncGalleryDir(galleryDir, contentDir)
	if err != nil {
		t.Fatalf("syncGalleryDir: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if generated != 2 {
		t.Errorf("generated = %d, want 2", generated)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}

	for _, stem := range []string{"photo1", "photo2"} {
		md := filepath.Join(contentDir, stem+".md")
		data, err := os.ReadFile(md)
		if err != nil {
			t.Fatalf("read %s: %v", md, err)
		}
		s := string(data)
		if !strings.Contains(s, "title: ") {
			t.Errorf("missing title in %s: %s", md, s)
		}
		if !strings.Contains(s, "type: gallery") {
			t.Errorf("missing type:gallery in %s: %s", md, s)
		}
	}
}

func TestSyncGalleryDir_SkipsExistingMarkdown(t *testing.T) {
	tmp := t.TempDir()
	galleryDir := filepath.Join(tmp, "static", "images", "gallery")
	contentDir := filepath.Join(tmp, "content", "gallery")
	if err := os.MkdirAll(galleryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(galleryDir, "exists.jpg"), []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pre-create md
	existing := "---\ntitle: custom\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(contentDir, "exists.md"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	total, generated, skipped, err := syncGalleryDir(galleryDir, contentDir)
	if err != nil {
		t.Fatalf("syncGalleryDir: %v", err)
	}
	if total != 1 || generated != 0 || skipped != 1 {
		t.Errorf("counts = (%d,%d,%d), want (1,0,1)", total, generated, skipped)
	}

	data, err := os.ReadFile(filepath.Join(contentDir, "exists.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != existing {
		t.Errorf("existing md was overwritten: got %s", data)
	}
}

func TestSyncGalleryDir_IgnoresNonImageFiles(t *testing.T) {
	tmp := t.TempDir()
	galleryDir := filepath.Join(tmp, "gallery")
	contentDir := filepath.Join(tmp, "content")
	if err := os.MkdirAll(galleryDir, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{".gitkeep", "readme.txt", "notes.md"} {
		if err := os.WriteFile(filepath.Join(galleryDir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	total, _, _, err := syncGalleryDir(galleryDir, contentDir)
	if err != nil {
		t.Fatalf("syncGalleryDir: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0 (non-image files ignored)", total)
	}
}

func TestSyncGalleryDir_HandlesNestedImages(t *testing.T) {
	tmp := t.TempDir()
	galleryDir := filepath.Join(tmp, "gallery")
	contentDir := filepath.Join(tmp, "content")
	nested := filepath.Join(galleryDir, "subdir")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "deep.jpg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	total, generated, _, err := syncGalleryDir(galleryDir, contentDir)
	if err != nil {
		t.Fatalf("syncGalleryDir: %v", err)
	}
	if total != 1 || generated != 1 {
		t.Errorf("counts = (%d,%d), want (1,1)", total, generated)
	}

	data, err := os.ReadFile(filepath.Join(contentDir, "deep.md"))
	if err != nil {
		t.Fatalf("read md: %v", err)
	}
	if !strings.Contains(string(data), "/images/gallery/subdir/deep.jpg") {
		t.Errorf("nested image path not in md: %s", data)
	}
}
