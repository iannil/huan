package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/iannil/huan/internal/build"
	"github.com/iannil/huan/internal/config"
	"github.com/iannil/huan/internal/deploy"
	"github.com/iannil/huan/internal/plugin"
	"github.com/spf13/cobra"
)

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy the site to a remote target",
		Long:  "Deploy runs a deployer plugin (e.g. cloudflare) to publish build output to a remote target. See `huan plugin list` for available plugins.",
	}
	cmd.AddCommand(newDeployCloudflareCmd())
	return cmd
}

func newDeployCloudflareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cloudflare [target]",
		Short: "Deploy via Cloudflare (Pages in PR1; R2 in PR2; Worker in PR3)",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runDeployCloudflare,
	}
	cmd.Flags().Bool("build", false, "build the site before deploying")
	cmd.Flags().Bool("dry-run", false, "compute manifests but do not perform network calls")
	cmd.Flags().String("branch", "", "override yaml pages.branch (use 'preview' for CF Pages preview deployment)")
	cmd.Flags().Int("concurrency", 0, "max CPU-bound goroutines for hashing/base64 (0 = min(GOMAXPROCS, 8))")
	cmd.Flags().String("commit-sha", "", "commit SHA to attach to the deployment (default: git HEAD)")
	cmd.Flags().String("commit-message", "", "commit message to attach (default: git log -1)")
	cmd.Flags().Bool("prune", false, "(r2 only) delete remote objects not present in local sync set")
	return cmd
}

func runDeployCloudflare(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(sourceDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	target := "pages"
	if len(args) > 0 {
		target = args[0]
	}

	// Build first if requested.
	if doBuild, _ := cmd.Flags().GetBool("build"); doBuild {
		outputDir := filepath.Join(sourceDir, cfg.PublishDir)
		if _, err := build.BuildSite(build.Options{
			SourceDir: sourceDir,
			OutputDir: outputDir,
		}); err != nil {
			return fmt.Errorf("build: %w", err)
		}
	}

	registry, err := newPluginRegistry(cfg)
	if err != nil {
		return fmt.Errorf("plugin registry: %w", err)
	}

	p, ok := registry.Get("cloudflare")
	if !ok {
		return fmt.Errorf("cloudflare plugin not configured (add plugins.cloudflare.* to huan.yaml)")
	}
	deployer, ok := p.(deploy.Deployer)
	if !ok {
		return fmt.Errorf("cloudflare plugin does not implement Deployer (internal error)")
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	branchFlag, _ := cmd.Flags().GetString("branch")
	commitSHA, _ := cmd.Flags().GetString("commit-sha")
	commitMessage, _ := cmd.Flags().GetString("commit-message")
	pruneFlag, _ := cmd.Flags().GetBool("prune")

	outputDir := filepath.Join(sourceDir, cfg.PublishDir)

	opts := deploy.Options{
		SourceDir:   sourceDir,
		OutputDir:   outputDir,
		Targets:     []string{target},
		DryRun:      dryRun,
		Concurrency: concurrency,
		Pages: &deploy.PagesOptions{
			Branch:        branchFlag,
			CommitSHA:     commitSHA,
			CommitMessage: commitMessage,
		},
		R2: &deploy.R2Options{
			Prune: pruneFlag,
		},
	}

	report, err := deployer.Deploy(context.Background(), opts)
	if err != nil {
		// Even on error, print the report so the user sees what was attempted.
		if report != nil {
			printDeployReport(report)
		}
		return err
	}
	printDeployReport(report)
	return nil
}

func printDeployReport(r *deploy.Report) {
	if r == nil {
		return
	}
	fmt.Printf("deploy target=%s trace_id=%s\n", r.Target, r.TraceID)
	fmt.Printf("  attempted=%d succeeded=%d skipped=%d failed=%d duration_ms=%d\n",
		r.Attempted, r.Succeeded, r.Skipped, r.Failed, r.DurationMs)
	if len(r.Failures) > 0 {
		fmt.Printf("  failures (%d):\n", len(r.Failures))
		for _, f := range r.Failures {
			fmt.Printf("    %s [%s]: %s\n", f.Path, f.Stage, f.Error)
		}
	}
}

// joinLabels is a small helper for rendering capability labels in CLI output.
func joinLabels(labels []string) string {
	if len(labels) == 0 {
		return "-"
	}
	return strings.Join(labels, ",")
}

// defaultConcurrency returns min(GOMAXPROCS, 8) per ADR 0002 §14.3.
func defaultConcurrency() int {
	n := runtime.GOMAXPROCS(0)
	if n > 8 {
		return 8
	}
	return n
}

// Verify the registry has at least one deployer (used by deploy subcommand).
func assertHasDeployer(registry *plugin.Registry) error {
	deployers := plugin.Find[deploy.Deployer](registry)
	if len(deployers) == 0 {
		return fmt.Errorf("no deployer plugins configured")
	}
	return nil
}
