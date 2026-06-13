package release

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/iannil/huan/internal/observability"
)

// Release runs the full release pipeline:
//  1. Validate inputs (version semver, LICENSE present, targets non-empty).
//  2. For each target: build → archive → checksum (continue on per-target
//     failure, collect into Report.Failures per ADR 0004 §15 / Q15 A1).
//  3. Write checksums.txt + manifest.json atomically.
//  4. If !opts.DryRun: move artifacts to opts.OutDir (overwriting expected
//     files, leaving any extra files the operator may have added — Q15 B1).
//  5. Always: clean up the temp work dir.
//
// ctx cancellation (Ctrl-C / signal.NotifyContext) propagates to all `go
// build` subprocesses via the Builder (Q15 C1).
//
// On success, returns a populated Report. On validation failure, returns
// (nil, error) before touching the filesystem. On per-target failure,
// returns (report, nil) — the report's Failures slice is the truth.
func Release(ctx context.Context, opts Options, builder Builder, logger *observability.Logger) (*Report, error) {
	start := time.Now()
	traceID := logger.TraceID()

	// --- Validate inputs ---
	spanValidate := "validate"
	logger.LogFunctionStart(spanValidate, map[string]any{
		"version": opts.Version,
		"targets": len(opts.Targets),
		"dry_run": opts.DryRun,
	})

	if err := ValidateVersion(opts.Version); err != nil {
		err = fmt.Errorf("version: %w", err)
		logger.LogError(spanValidate, err, nil)
		return nil, err
	}
	if len(opts.Targets) == 0 {
		err := fmt.Errorf("no targets specified")
		logger.LogError(spanValidate, err, nil)
		return nil, err
	}
	licensePath := filepath.Join(opts.SourceDir, "LICENSE")
	if _, err := os.Stat(licensePath); err != nil {
		err = fmt.Errorf("LICENSE not found at %s — public binary release requires a license (add one before running huan release)", licensePath)
		logger.LogError(spanValidate, err, nil)
		return nil, err
	}
	logger.LogFunctionEnd(spanValidate, time.Since(start), nil)

	// --- Prepare temp work dir (used both for dry-run and real release). ---
	// The temp dir holds intermediate artifacts. On dry-run, it's deleted
	// at the end and OutDir is never touched. On real release, archives +
	// checksums + manifest are moved to OutDir.
	tmpRoot, err := os.MkdirTemp("", "huan-release-*")
	if err != nil {
		err = fmt.Errorf("mkdir temp: %w", err)
		logger.LogError(spanMain, err, nil)
		return nil, err
	}
	defer os.RemoveAll(tmpRoot)
	logger.Log(spanMain, observability.EventPoint, map[string]any{
		"tmp_root": tmpRoot,
	})

	// Final output dir: temp staging for dry-run, opts.OutDir for real.
	var finalDir string
	if opts.DryRun {
		finalDir = filepath.Join(tmpRoot, "stage")
	} else {
		finalDir = opts.OutDir
	}
	if err := os.MkdirAll(finalDir, 0o755); err != nil {
		err = fmt.Errorf("mkdir out %s: %w", finalDir, err)
		logger.LogError(spanMain, err, nil)
		return nil, err
	}

	report := &Report{
		TraceID:   traceID,
		Version:   opts.Version,
		GoVersion: runtime.Version(),
		OutDir:    finalDir,
		Targets:   targetsToStrings(opts.Targets),
		DryRun:    opts.DryRun,
		BuildTime: start.UTC().Format(time.RFC3339Nano),
	}
	// Source provenance: shell out to git rather than rely on the operator's
	// debug.ReadBuildInfo(). The operator may have been built via `go run`
	// (Q14 canonical invocation) which doesn't always embed VCS info, leaving
	// the manifest without provenance. Shelling out gives us the source
	// directory's actual git state, which is what the manifest should
	// describe (not the operator's build state).
	if sha, dirty, ok := gitInfo(opts.SourceDir); ok {
		report.GitSHA = sha
		report.GitDirty = dirty
	}

	// --- Per-target pipeline: compile → archive → checksum. ---
	for _, target := range opts.Targets {
		artifact, failure := buildTarget(ctx, opts, target, builder, logger, tmpRoot, finalDir)
		if failure != nil {
			report.Failures = append(report.Failures, *failure)
			continue
		}
		report.Artifacts = append(report.Artifacts, *artifact)
	}

	// --- Always write checksums + manifest, even if some targets failed. ---
	// A partial release (3/5 targets succeeded) is still useful: the operator
	// can ship those 3 platforms immediately and investigate the 2 failures
	// separately. The checksums + manifest describe what's actually in the
	// release, not what was requested.
	if len(report.Artifacts) > 0 {
		checksumsPath, err := WriteChecksumsFile(finalDir, opts.Version, report.Artifacts)
		if err != nil {
			logger.LogError(spanMain, err, nil)
			report.Failures = append(report.Failures, Failure{
				Target: Target{}, Phase: "checksum", Error: err.Error(),
			})
		} else {
			logger.Log(spanMain, observability.EventPoint, map[string]any{
				"checksums_path": checksumsPath,
			})
		}
	}

	manifestPath, err := WriteManifest(finalDir, opts.Version, report)
	if err != nil {
		logger.LogError(spanMain, err, nil)
		report.Failures = append(report.Failures, Failure{
			Target: Target{}, Phase: "manifest", Error: err.Error(),
		})
	} else {
		logger.Log(spanMain, observability.EventPoint, map[string]any{
			"manifest_path": manifestPath,
		})
	}

	report.DurationMs = time.Since(start).Milliseconds()
	logger.LogFunctionEnd(spanMain, time.Since(start), map[string]any{
		"artifacts": len(report.Artifacts),
		"failures":  len(report.Failures),
	})

	return report, nil
}

const spanMain = "release"

// buildTarget runs compile + archive for one target. On success, the
// resulting archive is at <finalDir>/<ArchiveName> (real) or
// <tmpRoot>/stage/<ArchiveName> (dry-run) and the populated Artifact is
// returned. On failure, the populated Failure is returned and partial state
// under tmpRoot is cleaned up by the deferred RemoveAll in Release.
func buildTarget(
	ctx context.Context,
	opts Options,
	target Target,
	builder Builder,
	logger *observability.Logger,
	tmpRoot, finalDir string,
) (*Artifact, *Failure) {
	spanID := "target-" + target.OS + "-" + target.Arch
	logger.LogFunctionStart(spanID, map[string]any{"target": target.String()})

	// Compile into a per-target subdir of tmpRoot to avoid collisions
	// between concurrent target builds (also gives clean isolation for
	// debugging when something goes wrong).
	buildDir := filepath.Join(tmpRoot, target.OS+"-"+target.Arch)
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		err = fmt.Errorf("mkdir %s: %w", buildDir, err)
		logger.LogError(spanID, err, nil)
		return nil, &Failure{Target: target, Phase: "compile", Error: err.Error()}
	}
	binaryPath := filepath.Join(buildDir, BinaryName(target))
	if err := builder.Build(ctx, target, binaryPath); err != nil {
		return nil, &Failure{Target: target, Phase: "compile", Error: err.Error()}
	}

	// Build archive members: binary + LICENSE + READMEs at the archive root.
	members, err := archiveMembers(opts.SourceDir, binaryPath, target)
	if err != nil {
		logger.LogError(spanID, err, nil)
		return nil, &Failure{Target: target, Phase: "archive", Error: err.Error()}
	}
	archiveName := ArchiveName(target, opts.Version)
	archiveTmpPath := filepath.Join(tmpRoot, archiveName)
	if err := CreateArchive(archiveTmpPath, target, members); err != nil {
		logger.LogError(spanID, err, nil)
		return nil, &Failure{Target: target, Phase: "archive", Error: err.Error()}
	}

	sha, err := SHA256File(archiveTmpPath)
	if err != nil {
		logger.LogError(spanID, err, nil)
		return nil, &Failure{Target: target, Phase: "checksum", Error: err.Error()}
	}
	info, err := os.Stat(archiveTmpPath)
	if err != nil {
		logger.LogError(spanID, err, nil)
		return nil, &Failure{Target: target, Phase: "checksum", Error: err.Error()}
	}

	// Move archive into final dir. Per Q15 B1: overwrite expected archive
	// files but leave any other files in finalDir alone (operator may have
	// added .sig / RELEASE_NOTES.md / etc).
	finalArchivePath := filepath.Join(finalDir, archiveName)
	if err := os.Rename(archiveTmpPath, finalArchivePath); err != nil {
		// Rename can fail across filesystems; fall back to copy + remove.
		if copyErr := copyFile(archiveTmpPath, finalArchivePath); copyErr != nil {
			err = fmt.Errorf("move archive %s → %s: %w (rename err: %v)", archiveTmpPath, finalArchivePath, copyErr, err)
			logger.LogError(spanID, err, nil)
			return nil, &Failure{Target: target, Phase: "archive", Error: err.Error()}
		}
	}

	logger.LogFunctionEnd(spanID, time.Since(time.Now()), map[string]any{
		"target":  target.String(),
		"sha256":  sha,
		"size":    info.Size(),
		"path":    finalArchivePath,
	})

	return &Artifact{
		Name:   archiveName,
		SHA256: sha,
		Size:   info.Size(),
		Binary: BinaryName(target),
		OS:     target.OS,
		Arch:   target.Arch,
	}, nil
}

// archiveMembers returns the list of files to pack into the target's archive.
// Always: binary (named per BinaryName), LICENSE.
// Optional but typically present: README.md, README.zh-CN.md.
// Missing READMEs are skipped silently (LICENSE is mandatory and was
// validated earlier in Release).
func archiveMembers(sourceDir, binaryPath string, target Target) ([]ArchiveMember, error) {
	members := []ArchiveMember{
		{Name: BinaryName(target), Path: binaryPath, Mode: 0o755},
	}

	// Walk candidate aux files; missing ones are skipped (only LICENSE is
	// mandatory, validated upstream).
	candidates := []struct {
		name string
		mode os.FileMode
	}{
		{"LICENSE", 0o644},
		{"README.md", 0o644},
		{"README.zh-CN.md", 0o644},
	}
	for _, c := range candidates {
		p := filepath.Join(sourceDir, c.name)
		if _, err := os.Stat(p); err == nil {
			members = append(members, ArchiveMember{Name: c.name, Path: p, Mode: c.mode})
		}
	}
	return members, nil
}

// copyFile copies src to dst, preserving the source mode. Used as a fallback
// when os.Rename fails (e.g. cross-filesystem move).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src %s: %w", src, err)
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat src %s: %w", src, err)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("open dst %s: %w", dst, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s → %s: %w", src, dst, err)
	}
	return nil
}

// targetsToStrings converts a Target slice to "os/arch" strings for the
// Report.Targets field.
func targetsToStrings(targets []Target) []string {
	out := make([]string, len(targets))
	for i, t := range targets {
		out[i] = t.String()
	}
	return out
}

// gitInfo returns the source directory's git provenance (short SHA + dirty
// flag). Returns ok=false (no error) when sourceDir is not a git repo or git
// is not installed — manifest provenance is best-effort, not a release
// requirement. The release should still succeed for source dirs that aren't
// under git (e.g. extracted tarballs).
//
// We shell out to `git rev-parse` and `git status` instead of reading
// debug.ReadBuildInfo() because the latter reflects the *operator binary's*
// build context, which may differ from the source directory's actual git
// state. `go run` in particular doesn't reliably embed VCS settings.
func gitInfo(sourceDir string) (sha string, dirty bool, ok bool) {
	revCmd := exec.Command("git", "rev-parse", "--short=7", "HEAD")
	revCmd.Dir = sourceDir
	revOut, err := revCmd.Output()
	if err != nil {
		return "", false, false
	}
	sha = strings.TrimSpace(string(revOut))
	if sha == "" {
		return "", false, false
	}

	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = sourceDir
	statusOut, err := statusCmd.Output()
	if err != nil {
		// Got SHA but couldn't check dirty — return SHA, assume clean.
		return sha, false, true
	}
	dirty = len(strings.TrimSpace(string(statusOut))) > 0
	return sha, dirty, true
}

// debug.ReadBuildInfo stays referenced via this var so the import doesn't
// bit-rot. The operator's own build info isn't used directly by release
// (we shell out to git for provenance per Q14), but keeping the import
// means future refactors that want to surface Go module info still can.
var _ = debug.ReadBuildInfo
