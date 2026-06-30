package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestURLToFilePath verifies the URL → file system path mapping. URLs end
// in `/` and map to `index.html` inside that directory.
func TestURLToFilePath(t *testing.T) {
	cases := []struct {
		name       string
		url        string
		publishDir string
		want       string
	}{
		{"root", "/", "/tmp/out", "/tmp/out/index.html"},
		{"section", "/posts/", "/tmp/out", "/tmp/out/posts/index.html"},
		{"deep", "/posts/2026/foo/", "/tmp/out", "/tmp/out/posts/2026/foo/index.html"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := URLToFilePath(tc.url, tc.publishDir)
			if got != tc.want {
				t.Errorf("URLToFilePath(%q, %q) = %q, want %q",
					tc.url, tc.publishDir, got, tc.want)
			}
		})
	}
}

// TestPathToFilePath verifies the path-based mapping (no auto-index.html).
func TestPathToFilePath(t *testing.T) {
	got := PathToFilePath("/css/main.css", "/tmp/out")
	want := "/tmp/out/css/main.css"
	if got != want {
		t.Errorf("PathToFilePath = %q, want %q", got, want)
	}
}

// TestWriter_WriteCreatesFileWithDirectories verifies the round-trip:
// writing to a URL with nested dirs creates all intermediate dirs and
// the final index.html.
func TestWriter_WriteCreatesFileWithDirectories(t *testing.T) {
	tmp := t.TempDir()
	w := NewWriter(tmp)

	if err := w.Write("/posts/2026/hello/index.html", "<html>hello</html>"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "posts", "2026", "hello", "index.html"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Errorf("content mismatch: %s", string(data))
	}
}

// TestWriter_WriteBytesPathForRawFiles verifies WriteBytesPath for non-HTML
// files (e.g., images, fonts) — no index.html wrapping.
func TestWriter_WriteBytesPathForRawFiles(t *testing.T) {
	tmp := t.TempDir()
	w := NewWriter(tmp)

	if err := w.WriteBytesPath("/css/main.css", []byte("body{}")); err != nil {
		t.Fatalf("WriteBytesPath: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmp, "css", "main.css"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "body{}" {
		t.Errorf("got %q, want body{}", string(data))
	}
}

// TestWriter_CopyStaticMirrorsDirectory verifies static dir is mirrored
// recursively into publishDir.
func TestWriter_CopyStaticMirrorsDirectory(t *testing.T) {
	tmp := t.TempDir()
	staticDir := filepath.Join(tmp, "static")
	// Source: <tmp>/static/{css/main.css,js/app.js}
	mustWrite(t, filepath.Join(staticDir, "css", "main.css"), "body{}")
	mustWrite(t, filepath.Join(staticDir, "js", "app.js"), "console.log(1)")

	publishDir := filepath.Join(tmp, "out")
	w := NewWriter(publishDir)
	if err := w.CopyStatic(staticDir); err != nil {
		t.Fatalf("CopyStatic: %v", err)
	}

	for _, rel := range []string{"css/main.css", "js/app.js"} {
		if _, err := os.Stat(filepath.Join(publishDir, rel)); err != nil {
			t.Errorf("expected %s copied: %v", rel, err)
		}
	}
}

// TestWriter_StatsCountsFilesAndBytes verifies Stats returns cumulative
// counts after multiple writes.
func TestWriter_StatsCountsFilesAndBytes(t *testing.T) {
	tmp := t.TempDir()
	w := NewWriter(tmp)

	_ = w.Write("/a/index.html", "aaaa")   // 4 bytes
	_ = w.Write("/b/index.html", "bbbbbb") // 6 bytes

	files, bytes := w.Stats()
	if files != 2 {
		t.Errorf("files = %d, want 2", files)
	}
	if bytes != 10 {
		t.Errorf("bytes = %d, want 10", bytes)
	}
}

// TestWriter_CanonifyOptionApplies verifies SetCanonify wires Canonify into
// Write — URLs in the HTML content get rewritten using the configured baseURL.
func TestWriter_CanonifyOptionApplies(t *testing.T) {
	tmp := t.TempDir()
	w := NewWriter(tmp)
	w.SetCanonify(CanonifyOptions{BaseURL: "https://example.com/"})

	if err := w.Write("/posts/foo/index.html", `<a href="/bar/">x</a>`); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmp, "posts", "foo", "index.html"))
	if !strings.Contains(string(data), `https://example.com/bar/`) {
		t.Errorf("canonify not applied: %s", string(data))
	}
}

// TestCleanPublishDirRemovesExisting verifies CleanPublishDir wipes the dir.
func TestCleanPublishDirRemovesExisting(t *testing.T) {
	tmp := t.TempDir()
	// Pre-populate with stale files.
	mustWrite(t, filepath.Join(tmp, "stale.html"), "old")

	if err := CleanPublishDir(tmp); err != nil {
		t.Fatalf("CleanPublishDir: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmp, "stale.html")); !os.IsNotExist(err) {
		t.Errorf("stale.html not removed")
	}
}

// mustWrite is a test helper that creates parent dirs + writes a file,
// fataling on any error. Mirrors the pattern in serve/server_test.go.
func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
