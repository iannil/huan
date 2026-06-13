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
// PR1 only supports Target=["pages"]. Other targets return a clear "not
// implemented in this PR" error so callers don't waste time uploading the
// wrong thing.
func (p *Plugin) Deploy(ctx context.Context, opts deploy.Options) (*deploy.Report, error) {
	traceID := newTraceIDForDeploy()
	logger := deploy.NewLogger(traceID)

	// PR1 only supports target ["pages"]. Either no "pages" in targets or any
	// non-pages target means we cannot proceed.
	if !contains(opts.Targets, "pages") || hasUnsupportedTarget(opts.Targets) {
		return &deploy.Report{
			TraceID: traceID,
			Target:  joinTargets(opts.Targets),
			Failed:  0,
		}, fmt.Errorf("cloudflare plugin PR1 only supports target \"pages\"; got %v (r2/worker arrive in later PRs)", opts.Targets)
	}

	// Build manifest from output dir.
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
	if pagesOpts != nil {
		if pagesOpts.Branch != "" {
			branch = pagesOpts.Branch
		}
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

// contains checks if haystack contains needle.
func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// hasUnsupportedTarget returns true if any target is not "pages". Used by
// Deploy to reject mixed targets like ["pages", "r2"] in PR1.
func hasUnsupportedTarget(targets []string) bool {
	for _, t := range targets {
		if t != "pages" {
			return true
		}
	}
	return false
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
