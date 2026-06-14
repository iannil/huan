package cloudflare

import (
	"context"
	"fmt"

	"github.com/iannil/huan/internal/deploy"
	"github.com/iannil/huan/internal/observability"
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
	// Generate trace ID up front so early-return Reports have non-empty
	// TraceID. Previously newTraceIDForDeploy() returned "" and let Logger
	// auto-generate, which meant pre-logger-failure Reports had no correlation
	// id (audit L1).
	traceID := observability.NewTraceID()
	logger := observability.NewLogger(traceID)

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
func (p *Plugin) deployPages(ctx context.Context, opts deploy.Options, logger *observability.Logger, traceID string) (*deploy.Report, error) {
	if opts.OutputDir == "" {
		return nil, fmt.Errorf("deploy: output dir is required")
	}
	assets, err := BuildManifest(opts.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("build manifest: %w", err)
	}
	logger.Log("manifest", observability.EventFunctionEnd, map[string]any{
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
func (p *Plugin) deployR2(ctx context.Context, opts deploy.Options, logger *observability.Logger, traceID string) (*deploy.Report, error) {
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

// deployWorker uploads one or more Worker scripts via CF Workers modules API.
// Supports both singular `worker:` (legacy) and plural `workers:` yaml forms;
// AllWorkers merges them. Each worker deploys independently — failures are
// collected but don't abort the rest of the batch (collection-not-interruption
// pattern from ADR 0002 §9).
func (p *Plugin) deployWorker(ctx context.Context, opts deploy.Options, logger *observability.Logger, traceID string) (*deploy.Report, error) {
	workers := p.cfg.AllWorkers()
	if len(workers) == 0 {
		return &deploy.Report{
			TraceID: traceID,
			Target:  "worker",
		}, fmt.Errorf("cloudflare plugin: worker target requires worker.* or workers.* config under plugins.cloudflare")
	}

	client := NewClient(p.cfg.AccountID, p.cfg.APIToken, logger)
	deployer := NewWorkerDeployer(client, logger)

	// Deploy each worker; aggregate results.
	type workerResult struct {
		name string
		report *deploy.Report
		err error
	}
	results := make([]workerResult, 0, len(workers))
	for _, w := range workers {
		r, err := deployer.Deploy(ctx, w, DeployWorkerOptions{
			SourceDir: opts.SourceDir,
			DryRun:    opts.DryRun,
		})
		results = append(results, workerResult{name: w.Name, report: r, err: err})
	}

	// Build aggregate report
	aggregate := &deploy.Report{
		TraceID:   traceID,
		Target:    "worker",
		Attempted: len(workers),
	}
	var failures []deploy.FileFailure
	succeeded := 0
	for _, r := range results {
		if r.err != nil {
			failures = append(failures, deploy.FileFailure{
				Path:  r.name,
				Stage: "deploy",
				Error: r.err.Error(),
			})
			continue
		}
		succeeded++
		// Accumulate token/duration stats from individual reports
		if r.report != nil {
			aggregate.Attempted += r.report.Attempted // individual reports have Attempted=1
			aggregate.Succeeded += r.report.Succeeded
			aggregate.Skipped += r.report.Skipped
			aggregate.DurationMs += r.report.DurationMs
			failures = append(failures, r.report.Failures...)
		}
	}
	// Subtract the per-worker Attempted double-count (we added len(workers)
	// initially then added each report's Attempted which is also 1 each)
	aggregate.Attempted = len(workers)
	aggregate.Succeeded = succeeded
	aggregate.Failed = len(failures)
	aggregate.Failures = failures

	if len(failures) > 0 {
		return aggregate, fmt.Errorf("cloudflare plugin: %d/%d workers failed to deploy", len(failures), len(workers))
	}
	return aggregate, nil
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

// dryRunReport builds a Report for Pages dry-run. Semantics per audit L6:
// dry-run does not check remote, so we can't say which files would be skipped
// vs uploaded. All files show up as Attempted; Succeeded/Skipped/Failed all
// zero. The trace_id correlates to the structured log events which carry the
// detailed "would upload" / "would skip" breakdown.
func dryRunReport(traceID string, assets []Asset) *deploy.Report {
	return &deploy.Report{
		TraceID:   traceID,
		Target:    "pages",
		Attempted: len(assets),
		Succeeded: 0,
		Skipped:   0,
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

// newTraceIDForDeploy is deprecated; use observability.NewTraceID() directly.
// Kept as a no-op stub so any remaining callers compile cleanly; will be
// removed once we grep-verify no callers remain.
func newTraceIDForDeploy() string {
	return observability.NewTraceID()
}
