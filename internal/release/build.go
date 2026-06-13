package release

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iannil/huan/internal/observability"
)

// Builder abstracts the "compile huan for one target" step so unit tests
// can inject a mock without invoking the real `go build`. Production
// callers use GoBuildBuilder; tests use MockBuilder.
type Builder interface {
	Build(ctx context.Context, target Target, outPath string) error
}

// GoBuildBuilder invokes `go build` with cross-compile flags per ADR 0004 §7.
// Flags applied to every target:
//
//	CGO_ENABLED=0          # static binary, cross-compile friendly
//	-trimpath              # strip local paths (reproducible + privacy)
//	-ldflags="-s -w"       # strip symbol table + DWARF (~30% size reduction)
//	GOOS=<target.OS>
//	GOARCH=<target.Arch>
//
// The resulting binary is byte-identical across runs given the same source
// + Go version (verifiable via the determinism integration test).
type GoBuildBuilder struct {
	SourceDir string // project root containing cmd/huan
	Logger    *observability.Logger
}

// Build compiles huan for target into outPath. The source directory must
// contain cmd/huan (the main package). outPath's parent directory must
// already exist.
func (b *GoBuildBuilder) Build(ctx context.Context, target Target, outPath string) error {
	spanID := "build-" + target.OS + "-" + target.Arch
	b.Logger.LogFunctionStart(spanID, map[string]any{
		"target":   target.String(),
		"out_path": outPath,
	})

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		err = fmt.Errorf("mkdir %s: %w", filepath.Dir(outPath), err)
		b.Logger.LogError(spanID, err, nil)
		return err
	}

	args := []string{
		"build",
		"-trimpath",
		"-ldflags=-s -w",
		"-o", outPath,
		"./cmd/huan",
	}
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = b.SourceDir
	// CGO_ENABLED=0 is critical for cross-compile (no system C toolchain
	// needed). GOOS/GOARCH drive the cross-compile target.
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+target.OS,
		"GOARCH="+target.Arch,
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		err = fmt.Errorf("go build %s: %w; stderr: %s", target, err, strings.TrimSpace(stderr.String()))
		b.Logger.LogError(spanID, err, nil)
		return err
	}

	b.Logger.LogFunctionEnd(spanID, 0, map[string]any{
		"target": target.String(),
	})
	return nil
}

// MockBuilder is the test double for Builder. It writes a small deterministic
// "binary" file for each requested target, optionally returning a per-target
// failure. The build log lets tests assert which targets were attempted
// and in what order.
type MockBuilder struct {
	FailTargets map[Target]error // targets that should fail
	BuildLog    []Target         // record of Build calls, in order
	Content     []byte           // bytes to write as the "binary" (default = mock target name)
}

// Build writes a deterministic mock binary to outPath, or returns the
// configured failure for this target.
func (m *MockBuilder) Build(_ context.Context, target Target, outPath string) error {
	m.BuildLog = append(m.BuildLog, target)
	if err, ok := m.FailTargets[target]; ok {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(outPath), err)
	}
	content := m.Content
	if content == nil {
		content = []byte("mock binary for " + target.String())
	}
	if err := os.WriteFile(outPath, content, 0o755); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}
	return nil
}
