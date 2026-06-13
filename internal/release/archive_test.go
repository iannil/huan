package release

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestCreateTarGZ_FlatAndDeterministic(t *testing.T) {
	dir := t.TempDir()
	// Two members with distinct content.
	binPath := filepath.Join(dir, "huan")
	if err := os.WriteFile(binPath, []byte("binary-content"), 0o755); err != nil {
		t.Fatal(err)
	}
	licPath := filepath.Join(dir, "LICENSE")
	if err := os.WriteFile(licPath, []byte("MIT..."), 0o644); err != nil {
		t.Fatal(err)
	}
	members := []ArchiveMember{
		{Name: "huan", Path: binPath, Mode: 0o755},
		{Name: "LICENSE", Path: licPath, Mode: 0o644},
	}

	out1 := filepath.Join(dir, "out1.tar.gz")
	out2 := filepath.Join(dir, "out2.tar.gz")
	if err := CreateTarGZ(out1, members); err != nil {
		t.Fatalf("CreateTarGZ #1: %v", err)
	}
	if err := CreateTarGZ(out2, members); err != nil {
		t.Fatalf("CreateTarGZ #2: %v", err)
	}
	b1, _ := os.ReadFile(out1)
	b2, _ := os.ReadFile(out2)
	if !bytes.Equal(b1, b2) {
		t.Errorf("tar.gz not deterministic: %d vs %d bytes", len(b1), len(b2))
	}

	// Verify contents by extracting names.
	names, err := listTarGZ(out1)
	if err != nil {
		t.Fatalf("listTarGZ: %v", err)
	}
	sort.Strings(names)
	want := []string{"LICENSE", "huan"}
	if len(names) != 2 || names[0] != want[0] || names[1] != want[1] {
		t.Errorf("tar contents = %v, want %v", names, want)
	}
}

func listTarGZ(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		names = append(names, h.Name)
	}
	return names, nil
}

func TestCreateZip_FlatAndDeterministic(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "huan.exe")
	if err := os.WriteFile(binPath, []byte("exe-content"), 0o755); err != nil {
		t.Fatal(err)
	}
	licPath := filepath.Join(dir, "LICENSE")
	if err := os.WriteFile(licPath, []byte("MIT..."), 0o644); err != nil {
		t.Fatal(err)
	}
	members := []ArchiveMember{
		{Name: "huan.exe", Path: binPath, Mode: 0o755},
		{Name: "LICENSE", Path: licPath, Mode: 0o644},
	}

	out1 := filepath.Join(dir, "out1.zip")
	out2 := filepath.Join(dir, "out2.zip")
	if err := CreateZip(out1, members); err != nil {
		t.Fatalf("CreateZip #1: %v", err)
	}
	if err := CreateZip(out2, members); err != nil {
		t.Fatalf("CreateZip #2: %v", err)
	}
	b1, _ := os.ReadFile(out1)
	b2, _ := os.ReadFile(out2)
	if !bytes.Equal(b1, b2) {
		t.Errorf("zip not deterministic: %d vs %d bytes", len(b1), len(b2))
	}

	zr, err := zip.OpenReader(out1)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	want := []string{"LICENSE", "huan.exe"}
	if len(names) != 2 || names[0] != want[0] || names[1] != want[1] {
		t.Errorf("zip contents = %v, want %v", names, want)
	}
}

func TestCreateArchive_DispatchesByOS(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "huan")
	if err := os.WriteFile(srcPath, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	members := []ArchiveMember{{Name: "huan", Path: srcPath, Mode: 0o755}}

	// windows → zip
	winOut := filepath.Join(dir, "win.zip")
	if err := CreateArchive(winOut, Target{OS: "windows", Arch: "amd64"}, members); err != nil {
		t.Fatalf("CreateArchive windows: %v", err)
	}
	if _, err := zip.OpenReader(winOut); err != nil {
		t.Errorf("expected zip for windows target: %v", err)
	}

	// darwin → tar.gz
	darwinOut := filepath.Join(dir, "darwin.tar.gz")
	if err := CreateArchive(darwinOut, Target{OS: "darwin", Arch: "arm64"}, members); err != nil {
		t.Fatalf("CreateArchive darwin: %v", err)
	}
	if _, err := listTarGZ(darwinOut); err != nil {
		t.Errorf("expected tar.gz for darwin target: %v", err)
	}
}
