package cloudflare

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFixture creates a file under dir with the given content.
func writeFixture(t *testing.T, dir, relPath string, content []byte) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

func TestBuildManifest_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	assets, err := BuildManifest(dir)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	if len(assets) != 0 {
		t.Errorf("got %d assets, want 0", len(assets))
	}
}

func TestBuildManifest_SingleFile(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "index.html", []byte("<html></html>"))

	assets, err := BuildManifest(dir)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("got %d assets, want 1", len(assets))
	}
	a := assets[0]
	if a.Path != "/index.html" {
		t.Errorf("Path = %q, want /index.html", a.Path)
	}
	if a.Size != int64(len("<html></html>")) {
		t.Errorf("Size = %d, want %d", a.Size, len("<html></html>"))
	}
	wantHash := Hash([]byte("<html></html>"), "html")
	if a.Hash != wantHash {
		t.Errorf("Hash = %q, want %q", a.Hash, wantHash)
	}
	if !strings.HasPrefix(a.ContentType, "text/html") {
		t.Errorf("ContentType = %q, want text/html prefix", a.ContentType)
	}
	if string(a.Content) != "<html></html>" {
		t.Errorf("Content = %q", string(a.Content))
	}
}

func TestBuildManifest_NestedDirs(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "blog/2024/post.html", []byte("post"))
	writeFixture(t, dir, "assets/css/main.css", []byte("css"))

	assets, err := BuildManifest(dir)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	paths := make(map[string]bool)
	for _, a := range assets {
		paths[a.Path] = true
	}
	if !paths["/blog/2024/post.html"] {
		t.Errorf("missing nested path; got %v", paths)
	}
	if !paths["/assets/css/main.css"] {
		t.Errorf("missing nested css path; got %v", paths)
	}
}

func TestBuildManifest_LeadingSlashEnforced(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "index.html", []byte("x"))

	assets, err := BuildManifest(dir)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	for _, a := range assets {
		if !strings.HasPrefix(a.Path, "/") {
			t.Errorf("Path %q missing leading slash", a.Path)
		}
	}
}

func TestBuildManifest_ForwardSlashOnAllOS(t *testing.T) {
	// On Windows the path would be "\dir\file.html" without ToSlash. Verify
	// we always emit forward slashes for CF manifest keys.
	if runtime.GOOS == "windows" {
		t.Skip("test verifies posix-style on non-Windows; Windows path tested implicitly")
	}
	dir := t.TempDir()
	writeFixture(t, dir, "a/b/c.html", []byte("deep"))

	assets, err := BuildManifest(dir)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	if len(assets) != 1 || assets[0].Path != "/a/b/c.html" {
		t.Errorf("got %+v, want /a/b/c.html", assets)
	}
}

func TestBuildManifest_SkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "real.html", []byte("real"))
	// Create a symlink to real.html
	if err := os.Symlink(filepath.Join(dir, "real.html"), filepath.Join(dir, "link.html")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	assets, err := BuildManifest(dir)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	paths := make(map[string]bool)
	for _, a := range assets {
		paths[a.Path] = true
	}
	if !paths["/real.html"] {
		t.Errorf("real.html should be present: %v", paths)
	}
	if paths["/link.html"] {
		t.Errorf("symlink link.html should be skipped: %v", paths)
	}
}

func TestBuildManifest_FileSizeLimit(t *testing.T) {
	dir := t.TempDir()
	// Create a file just over MaxFileSize.
	if err := os.WriteFile(filepath.Join(dir, "big.html"), make([]byte, MaxFileSize+1), 0o644); err != nil {
		t.Fatalf("write big.html: %v", err)
	}

	_, err := BuildManifest(dir)
	if err == nil {
		t.Fatal("BuildManifest: want error for oversized file, got nil")
	}
	var me *ManifestError
	if !errors.As(err, &me) {
		t.Errorf("err = %T, want *ManifestError", err)
	}
	if me.Limit != "MaxFileSize" {
		t.Errorf("Limit = %q, want MaxFileSize", me.Limit)
	}
}

func TestBuildManifest_NotADirectory(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "notadir.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := BuildManifest(filePath)
	if err == nil {
		t.Fatal("want error for non-directory")
	}
}

func TestBuildManifest_NonExistentDir(t *testing.T) {
	_, err := BuildManifest("/definitely/does/not/exist/xyz")
	if err == nil {
		t.Fatal("want error for non-existent dir")
	}
}

func TestBuildManifest_ExtensionlessFile(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "Makefile", []byte("all:"))

	assets, err := BuildManifest(dir)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("got %d assets, want 1", len(assets))
	}
	wantHash := Hash([]byte("all:"), "")
	if assets[0].Hash != wantHash {
		t.Errorf("Hash = %q, want %q", assets[0].Hash, wantHash)
	}
}

func TestBatch_Empty(t *testing.T) {
	if got := Batch(nil); got != nil {
		t.Errorf("Batch(nil) = %v, want nil", got)
	}
}

func TestBatch_SingleBatchUnderLimit(t *testing.T) {
	assets := make([]Asset, 100)
	batches := Batch(assets)
	if len(batches) != 1 {
		t.Errorf("got %d batches, want 1", len(batches))
	}
}

func TestBatch_MultipleBatches(t *testing.T) {
	// All Size=0, so count is the only constraint.
	assets := make([]Asset, MaxFilesPerBatch*2+5)
	batches := Batch(assets)
	if len(batches) != 3 {
		t.Errorf("got %d batches, want 3", len(batches))
	}
	// Verify batch sizes.
	if len(batches[0]) != MaxFilesPerBatch {
		t.Errorf("batch 0 size = %d, want %d", len(batches[0]), MaxFilesPerBatch)
	}
	if len(batches[1]) != MaxFilesPerBatch {
		t.Errorf("batch 1 size = %d, want %d", len(batches[1]), MaxFilesPerBatch)
	}
	if len(batches[2]) != 5 {
		t.Errorf("batch 2 size = %d, want 5", len(batches[2]))
	}
}

func TestBatch_ExactMultiple(t *testing.T) {
	assets := make([]Asset, MaxFilesPerBatch*2)
	batches := Batch(assets)
	if len(batches) != 2 {
		t.Errorf("got %d batches, want 2", len(batches))
	}
}

// TestBatch_SplitsBySizeWhenSizeConstraintHitsFirst verifies the new
// MaxBatchSize (40 MiB) constraint from audit C3.
//
// With 1 MiB files: MaxBatchSize / 1MiB = 40. Iter 40: currentSize=39MiB,
// 39+1=40 not > 40, append (40 files). Iter 41: 40+1=41 > 40, flush.
// So batch 0 = 40 files (exactly at limit); batch 1 = remainder.
func TestBatch_SplitsBySizeWhenSizeConstraintHitsFirst(t *testing.T) {
	const fileSize = 1024 * 1024 // 1 MiB
	const fileCount = 60         // 60 MiB total -> 2 batches (40 + 20)
	assets := make([]Asset, fileCount)
	for i := range assets {
		assets[i].Size = fileSize
	}
	batches := Batch(assets)
	if len(batches) != 2 {
		t.Fatalf("got %d batches, want 2 (size constraint)", len(batches))
	}
	if len(batches[0]) != 40 {
		t.Errorf("batch 0 file count = %d, want 40 (exactly at size limit)", len(batches[0]))
	}
	var size0 int64
	for _, a := range batches[0] {
		size0 += a.Size
	}
	if size0 != 40*fileSize {
		t.Errorf("batch 0 size = %d, want %d", size0, 40*fileSize)
	}
	if len(batches[1]) != 20 {
		t.Errorf("batch 1 file count = %d, want 20", len(batches[1]))
	}
}

// TestBatch_SingleFileOverSizeLimit handled at BuildManifest, not Batch.
// Batch assumes BuildManifest already rejected oversized files.
func TestBatch_FilesExactly40MiB_FitsInOneBucket(t *testing.T) {
	// 40 files * 1 MiB = 40 MiB exactly = MaxBatchSize. Should fit.
	const fileSize = 1024 * 1024
	assets := make([]Asset, 40)
	for i := range assets {
		assets[i].Size = fileSize
	}
	batches := Batch(assets)
	if len(batches) != 1 {
		t.Errorf("got %d batches, want 1 (exactly at size limit)", len(batches))
	}
}

func TestBatch_Over40MiBWith41Files_SplitsIntoTwo(t *testing.T) {
	const fileSize = 1024 * 1024 // 1 MiB
	assets := make([]Asset, 41)
	for i := range assets {
		assets[i].Size = fileSize
	}
	batches := Batch(assets)
	if len(batches) != 2 {
		t.Errorf("got %d batches, want 2 (size limit exceeded)", len(batches))
	}
}

func TestGuessContentType_KnownTypes(t *testing.T) {
	// mime.TypeByExtension varies by OS / system registry. We only assert
	// the type-family prefix (e.g. "text/" or "image/"), not the exact MIME,
	// since macOS registers .js as "text/javascript" while Linux typically
	// registers "application/javascript".
	cases := map[string]string{
		"index.html": "text/",
		"style.css":  "text/",
		"logo.png":   "image/",
	}
	for name, wantPrefix := range cases {
		got := guessContentType(name)
		if !strings.HasPrefix(got, wantPrefix) {
			t.Errorf("guessContentType(%q) = %q, want prefix %q", name, got, wantPrefix)
		}
	}
	// .js is non-empty regardless of OS registry.
	if got := guessContentType("script.js"); got == "" {
		t.Errorf("guessContentType(script.js) empty; want some javascript MIME")
	}
}

func TestGuessContentType_UnknownExtension(t *testing.T) {
	got := guessContentType("data.unknownext")
	if got != "" {
		t.Errorf("guessContentType(unknownext) = %q, want empty", got)
	}
}
