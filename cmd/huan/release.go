package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/iannil/huan/internal/observability"
	"github.com/iannil/huan/internal/release"
	"github.com/iannil/huan/internal/version"
	"github.com/spf13/cobra"
)

func newReleaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Cross-compile huan and package release artifacts",
		Long: `release cross-compiles huan for the standard platform matrix
(darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64),
packs each into a tarball/zip with LICENSE + READMEs, computes sha256
checksums, and emits a JSON manifest into /release/{version}/.

Pure local artifact production: no git tag, no remote upload. To publish,
manually run 'git tag v{version} && git push --tags' after a successful
release.

Operator bootstrap is transparent — invoke via 'go run ./cmd/huan release'
to compile the operator fresh each time.`,
		RunE: runRelease,
	}
	cmd.Flags().String("targets", "all", "comma-separated GOOS/GOARCH list (e.g. darwin/arm64,linux/amd64), or 'all' for the 5 standard targets, or 'current' for the host platform only")
	cmd.Flags().Bool("dry-run", false, "compute and build everything to a temp dir, write nothing to /release/, delete temp at the end")
	cmd.Flags().String("out-dir", "", "override the output directory (default: <sourceDir>/release/<version>/)")
	return cmd
}

func runRelease(cmd *cobra.Command, args []string) error {
	targetsFlag, _ := cmd.Flags().GetString("targets")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	outDirFlag, _ := cmd.Flags().GetString("out-dir")

	targets, err := parseTargetsFlag(targetsFlag)
	if err != nil {
		return fmt.Errorf("--targets: %w", err)
	}

	ver := version.String()
	if err := release.ValidateVersion(ver); err != nil {
		return fmt.Errorf("VERSION file: %w", err)
	}

	outDir := outDirFlag
	if outDir == "" {
		outDir = filepath.Join(sourceDir, "release", ver)
	}

	traceID := observability.NewTraceID()
	logger := observability.NewLogger(traceID)

	// context cancellation propagates to `go build` subprocesses via the
	// Builder (Q15 C1). SIGINT/SIGTERM trigger cancel.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := release.Options{
		Version:   ver,
		OutDir:    outDir,
		Targets:   targets,
		SourceDir: sourceDir,
		DryRun:    dryRun,
		TraceID:   traceID,
	}

	builder := &release.GoBuildBuilder{SourceDir: sourceDir, Logger: logger}

	report, err := release.Release(ctx, opts, builder, logger)
	if report != nil {
		printReleaseReport(report)
	}
	if err != nil {
		return err
	}
	return nil
}

// parseTargetsFlag turns the --targets string into a Target slice.
// Special values:
//
//	"all"     → release.StandardTargets
//	"current" → host GOOS/GOARCH only
//
// Otherwise expects comma-separated "os/arch" pairs.
func parseTargetsFlag(flag string) ([]release.Target, error) {
	switch flag {
	case "all":
		return release.StandardTargets, nil
	case "current":
		return []release.Target{release.HostTarget()}, nil
	case "":
		return nil, fmt.Errorf("empty targets flag")
	}

	out := make([]release.Target, 0, 4)
	for _, p := range strings.Split(flag, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		os, arch, found := strings.Cut(p, "/")
		if !found || os == "" || arch == "" {
			return nil, fmt.Errorf("invalid target %q (expected os/arch like darwin/arm64)", p)
		}
		out = append(out, release.Target{OS: os, Arch: arch})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no targets parsed from %q", flag)
	}
	return out, nil
}

func printReleaseReport(r *release.Report) {
	if r == nil {
		return
	}
	status := "ok"
	if len(r.Failures) > 0 {
		status = "partial"
	}
	if r.DryRun {
		status = "dry-run"
	}
	fmt.Printf("release version=%s trace_id=%s status=%s\n",
		r.Version, r.TraceID, status)
	fmt.Printf("  artifacts=%d failures=%d duration_ms=%d\n",
		len(r.Artifacts), len(r.Failures), r.DurationMs)
	if r.GitSHA != "" {
		dirty := ""
		if r.GitDirty {
			dirty = " (dirty)"
		}
		fmt.Printf("  git_sha=%s%s go=%s\n", r.GitSHA, dirty, r.GoVersion)
	}
	if !r.DryRun {
		fmt.Printf("  out_dir=%s\n", r.OutDir)
	}
	for _, a := range r.Artifacts {
		fmt.Printf("  artifact %s sha256=%s size=%d\n", a.Name, a.SHA256, a.Size)
	}
	for _, f := range r.Failures {
		fmt.Printf("  failure target=%s phase=%s error=%s\n", f.Target, f.Phase, f.Error)
	}
	// Machine-readable JSON manifest goes to stderr for piping if needed.
	enc := json.NewEncoder(os.Stderr)
	_ = enc.Encode(r)
}
