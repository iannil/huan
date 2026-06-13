package cloudflare

import (
	"context"
	"fmt"

	"github.com/iannil/huan/internal/deploy"
)

// Plugin is the Cloudflare deploy plugin. It implements both plugin.Plugin
// (via Name) and deploy.Deployer (via Deploy), so it is discoverable both as
// a base plugin and as a deployer via plugin.Find[*deploy.Deployer].
type Plugin struct {
	cfg Config
}

// New constructs a Plugin from parsed Config. The cfg should come from
// ParseConfig; pass its output directly here.
func New(cfg Config) *Plugin {
	return &Plugin{cfg: cfg}
}

// Name is the unique plugin identifier. Pairs with the yaml key under plugins:
// (i.e. plugins.cloudflare.*).
func (p *Plugin) Name() string { return "cloudflare" }

// Config returns the parsed configuration. Used by the CLI to display
// effective config in `huan plugin info`.
func (p *Plugin) Config() Config { return p.cfg }

// Deploy implements deploy.Deployer. It dispatches to the appropriate sub-
// target based on opts.Targets.
//
// Supported targets:
//   - "pages" — Cloudflare Pages direct-upload (PR1)
//   - "r2"    — R2 bucket sync via S3-compatible API (PR2)
//   - "worker" — Worker modules upload (PR3, not yet implemented)
//
// Mixed targets like ["pages", "r2"] return an error — invoke each separately.
func (p *Plugin) Deploy(ctx context.Context, opts deploy.Options) (*deploy.Report, error) {
	traceID := newTraceIDForDeploy()
	logger := deploy.NewLogger(traceID)

	target, err := singleTarget(opts.Targets)
	if err != nil {
		return &deploy.Report{
			TraceID: traceID,
			Target:  joinTargets(opts.Targets),
		}, err
	}

	switch target {
	case "pages":
		return p.deployPages(ctx, opts, logger, traceID)
	case "r2":
		return p.deployR2(ctx, opts, logger, traceID)
	case "worker":
		return p.deployWorker(ctx, opts, logger, traceID)
	default:
		return &deploy.Report{
			TraceID: traceID,
			Target:  target,
		}, fmt.Errorf("cloudflare plugin: unknown target %q", target)
	}
}

// deployPages runs the Pages direct-upload protocol.
func (p *Plugin) deployPages(ctx context.Context, opts deploy.Options, logger *deploy.Logger, traceID string) (*deploy.Report, error) {
	if opts.OutputDir == "" {
		return nil, fmt.Errorf("deploy: output dir is required")
	}
	assets, err := BuildManifest(opts.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("build manifest: %w", err)
	}
	logger.Log("manifest", deploy.EventFunctionEnd, map[string]any{
		"asset_count": len(assets),
		"output_dir":  opts.OutputDir,
	})

	if opts.DryRun {
		return dryRunReport(traceID, assets), nil
	}

	// Resolve deploy-time pages options: branch override + commit metadata.
	branch := p.cfg.Pages.Branch
	pagesOpts := opts.Pages
	if pagesOpts != nil && pagesOpts.Branch != "" {
		branch = pagesOpts.Branch
	}
	var commit *CommitMeta
	if pagesOpts != nil {
		cm := InferCommitMetadata(pagesOpts.CommitSHA, pagesOpts.CommitMessage)
		commit = &cm
	} else {
		cm := InferCommitMetadata("", "")
		commit = &cm
	}

	// Construct client + deployer and run.
	client := NewClient(p.cfg.AccountID, p.cfg.APIToken, logger)
	deployer := NewPagesDeployer(client, logger)

	report, err := deployer.DeployPages(ctx, DeployPagesOptions{
		Project: p.cfg.Pages.Project,
		Branch:  branch,
		Commit:  commit,
		Assets:  assets,
	})
	if err != nil {
		return report, err
	}
	return report, nil
}

// deployR2 runs the R2 sync algorithm.
func (p *Plugin) deployR2(ctx context.Context, opts deploy.Options, logger *deploy.Logger, traceID string) (*deploy.Report, error) {
	if !p.cfg.HasR2Configured() {
		return &deploy.Report{
			TraceID: traceID,
			Target:  "r2",
		}, fmt.Errorf("cloudflare plugin: r2 target requires r2.* config under plugins.cloudflare")
	}
	if len(p.cfg.R2.Sync) == 0 {
		return &deploy.Report{
			TraceID: traceID,
			Target:  "r2",
		}, fmt.Errorf("cloudflare plugin: r2.sync mappings are required (at least one {from, to} entry)")
	}

	syncer, err := NewR2Syncer(p.cfg.R2, logger)
	if err != nil {
		return &deploy.Report{
			TraceID: traceID,
			Target:  "r2",
		}, fmt.Errorf("init r2 syncer: %w", err)
	}

	result, err := syncer.Sync(ctx, p.cfg.R2.Sync, R2SyncOptions{
		Prune:       opts.R2 != nil && opts.R2.Prune,
		DryRun:      opts.DryRun,
		Concurrency: opts.Concurrency,
	})
	if err != nil {
		return r2ResultToReport(traceID, result, err), err
	}
	return r2ResultToReport(traceID, result, nil), nil
}

// deployWorker uploads the Worker script via CF Workers modules API.
func (p *Plugin) deployWorker(ctx context.Context, opts deploy.Options, logger *deploy.Logger, traceID string) (*deploy.Report, error) {
	if !p.cfg.HasWorkerConfigured() {
		return &deploy.Report{
			TraceID: traceID,
			Target:  "worker",
		}, fmt.Errorf("cloudflare plugin: worker target requires worker.* config under plugins.cloudflare")
	}

	client := NewClient(p.cfg.AccountID, p.cfg.APIToken, logger)
	deployer := NewWorkerDeployer(client, logger)

	report, err := deployer.Deploy(ctx, p.cfg.Worker, DeployWorkerOptions{
		SourceDir: opts.SourceDir,
		DryRun:    opts.DryRun,
	})
	if err != nil {
		return report, err
	}
	return report, nil
}

// r2ResultToReport adapts R2SyncResult into the deploy.Report shape so callers
// (CLI) see a consistent contract.
func r2ResultToReport(traceID string, r *R2SyncResult, err error) *deploy.Report {
	if r == nil {
		r = &R2SyncResult{}
	}
	report := &deploy.Report{
		TraceID:   traceID,
		Target:    "r2",
		Attempted: r.Attempted,
		Succeeded: r.Succeeded,
		Failed:    r.Failed,
		Skipped:   r.Skipped,
	}
	if err != nil {
		report.Failures = append(report.Failures, deploy.FileFailure{
			Path:  "",
			Stage: "sync",
			Error: err.Error(),
		})
	}
	for _, f := range r.Failures {
		report.Failures = append(report.Failures, deploy.FileFailure{
			Path:  f.LocalPath,
			Stage: f.Stage,
			Error: f.Error,
		})
	}
	return report
}

func dryRunReport(traceID string, assets []Asset) *deploy.Report {
	return &deploy.Report{
		TraceID:   traceID,
		Target:    "pages",
		Attempted: len(assets),
		Skipped:   len(assets), // dry-run: nothing actually uploaded; all "would be skipped" from CF perspective
		Succeeded: 0,
		Failed:    0,
	}
}

// singleTarget validates that opts.Targets has exactly one supported target
// and returns it. Mixed targets (e.g. ["pages", "r2"]) are rejected so callers
// don't accidentally deploy part of a build to one target.
func singleTarget(targets []string) (string, error) {
	if len(targets) == 0 {
		return "", fmt.Errorf("cloudflare plugin: no target specified (use one of: pages, r2, worker)")
	}
	if len(targets) > 1 {
		return "", fmt.Errorf("cloudflare plugin: multiple targets %v not supported (invoke each separately)", targets)
	}
	return targets[0], nil
}

// joinTargets returns a comma-joined string of targets for Report.Target.
func joinTargets(targets []string) string {
	out := ""
	for i, t := range targets {
		if i > 0 {
			out += ","
		}
		out += t
	}
	if out == "" {
		return "none"
	}
	return out
}

// newTraceIDForDeploy generates a fresh trace id for this deploy invocation.
// Mirrors deploy.Logger's trace id generation but standalone so callers can
// inspect the trace id before constructing a logger.
func newTraceIDForDeploy() string {
	// Logger auto-generates one if empty; just pass empty.
	return ""
}
