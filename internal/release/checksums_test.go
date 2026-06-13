package release

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSHA256File_KnownContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	content := []byte("hello world\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	// sha256("hello world\n") = a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447
	got, err := SHA256File(path)
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}
	want := "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
	if got != want {
		t.Errorf("SHA256File = %q, want %q", got, want)
	}
}

func TestSHA256File_Missing(t *testing.T) {
	_, err := SHA256File("/nonexistent/path/xyz")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestChecksumsLine_Format(t *testing.T) {
	got := ChecksumsLine("foo.tar.gz", "abc123")
	want := "abc123  foo.tar.gz\n"
	if got != want {
		t.Errorf("ChecksumsLine = %q, want %q", got, want)
	}
}

func TestWriteChecksumsFile_SortedAndAtomic(t *testing.T) {
	dir := t.TempDir()
	artifacts := []Artifact{
		{Name: "z-last.tar.gz", SHA256: "zzz"},
		{Name: "a-first.tar.gz", SHA256: "aaa"},
		{Name: "m-middle.tar.gz", SHA256: "mmm"},
	}
	path, err := WriteChecksumsFile(dir, "0.1.0", artifacts)
	if err != nil {
		t.Fatalf("WriteChecksumsFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "aaa  a-first.tar.gz\nmmm  m-middle.tar.gz\nzzz  z-last.tar.gz\n"
	if !bytes.Equal(data, []byte(want)) {
		t.Errorf("checksums file:\n got: %q\nwant: %q", string(data), want)
	}
}

func TestWriteChecksumsFile_EmptyList(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteChecksumsFile(dir, "0.1.0", nil)
	if err != nil {
		t.Fatalf("WriteChecksumsFile: %v", err)
	}
	data, _ := os.ReadFile(path)
	if len(data) != 0 {
		t.Errorf("empty artifact list produced non-empty file: %q", string(data))
	}
}

func TestAtomicWrite_TempFileCleanedUp(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "final.txt")
	if err := atomicWrite(final, []byte("payload"), 0o644); err != nil {
		t.Fatalf("atomicWrite: %v", err)
	}
	// After successful write, no .tmp-* files should remain in dir.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if len(e.Name()) >= 5 && e.Name()[:5] == ".tmp-" {
			t.Errorf("temp file leaked: %s", e.Name())
		}
	}
	data, _ := os.ReadFile(final)
	if string(data) != "payload" {
		t.Errorf("final content = %q, want 'payload'", string(data))
	}
}
