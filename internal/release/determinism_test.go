//go:build integration

package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/iannil/huan/internal/observability"
)

// TestDeterminism_SameCommitSameBinarySHA verifies that two consecutive
// builds of the same target produce byte-identical binaries. This is the
// Q7 / ADR 0004 §7 invariant: CGO_ENABLED=0 + -trimpath + -ldflags=-s -w
// (no wall-clock injection) guarantees reproducibility.
//
// If this test fails, the most likely cause is a stray -ldflags injection
// embedding build time, or someone removed -trimpath from GoBuildBuilder.Build.
func TestDeterminism_SameCommitSameBinarySHA(t *testing.T) {
	root := projectRoot(t)
	target := HostTarget()

	logger := observability.NewLoggerWithWriter("determinism", io.Discard)
	builder := &GoBuildBuilder{SourceDir: root, Logger: logger}

	dir1 := t.TempDir()
	dir2 := t.TempDir()
	bin1 := filepath.Join(dir1, BinaryName(target))
	bin2 := filepath.Join(dir2, BinaryName(target))

	if err := builder.Build(context.Background(), target, bin1); err != nil {
		t.Fatalf("build 1: %v", err)
	}
	if err := builder.Build(context.Background(), target, bin2); err != nil {
		t.Fatalf("build 2: %v", err)
	}

	sha1, err := sha256File(bin1)
	if err != nil {
		t.Fatalf("sha1: %v", err)
	}
	sha2, err := sha256File(bin2)
	if err != nil {
		t.Fatalf("sha2: %v", err)
	}
	if sha1 != sha2 {
		t.Errorf("determinism broken: build 1 sha = %s, build 2 sha = %s", sha1, sha2)
		t.Errorf("binary 1 size = %d, binary 2 size = %d", fileSize(bin1), fileSize(bin2))
	}
}

// TestDeterminism_ArchiveReproducible verifies the tar.gz archive produced
// for the same target is byte-identical across two CreateTarGZ calls. This
// guards the zero-time header invariant in archive.go.
func TestDeterminism_ArchiveReproducible(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "huan")
	if err := os.WriteFile(binPath, []byte("deterministic-content"), 0o755); err != nil {
		t.Fatal(err)
	}
	members := []ArchiveMember{
		{Name: "huan", Path: binPath, Mode: 0o755},
	}

	out1 := filepath.Join(dir, "a1.tar.gz")
	out2 := filepath.Join(dir, "a2.tar.gz")
	if err := CreateTarGZ(out1, members); err != nil {
		t.Fatalf("CreateTarGZ #1: %v", err)
	}
	if err := CreateTarGZ(out2, members); err != nil {
		t.Fatalf("CreateTarGZ #2: %v", err)
	}

	sha1, _ := sha256File(out1)
	sha2, _ := sha256File(out2)
	if sha1 != sha2 {
		t.Errorf("archive not deterministic: %s vs %s", sha1, sha2)
	}
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return info.Size()
}
