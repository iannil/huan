// Package deploy defines the Deployer capability interface — the first
// concrete capability in huan's unified plugin system (see
// docs/adr/0003-unified-plugin-system.md). Plugins that publish build output
// to a remote target (Cloudflare Pages, future netlify/s3/etc.) implement
// Deployer.
//
// This package owns only the capability contract + shared types (Options,
// Report). Each deployer implementation lives in its own subpackage
// (e.g. internal/deploy/cloudflare).
package deploy

import (
	"context"

	"github.com/iannil/huan/internal/plugin"
)

// Deployer is the capability interface for plugins that publish build output.
// It embeds plugin.Plugin (so deployers are also discoverable as base plugins)
// and adds Deploy.
//
// A plugin implementing Deployer registers under a unique Name (e.g.
// "cloudflare") and is queried via:
//
//	deployers := plugin.Find[*deploy.Deployer](registry)
type Deployer interface {
	plugin.Plugin

	// Deploy publishes the build output to the remote target. Implementations
	// should:
	//   - Honor ctx for cancellation.
	//   - Return a Report describing attempted/succeeded/failed/skipped counts
	//     even on partial failure (collection-not-interruption per ADR 0002 §9).
	//   - Return a non-nil error only when the deploy cannot proceed at all
	//     (e.g. invalid config, missing prerequisite resource). Individual file
	//     failures go into Report.Failures, not the error return.
	Deploy(ctx context.Context, opts Options) (*Report, error)
}

// Options carries invocation-time deploy parameters. Capability-specific
// knobs live in sub-structs (Pages, future R2/Worker) so the top-level stays
// stable as new deploy targets are added.
type Options struct {
	// SourceDir is the project root containing huan.yaml.
	SourceDir string

	// OutputDir is the build output directory (typically publishDir or --destination).
	OutputDir string

	// Targets filters which sub-targets to run. For Cloudflare PR1 only "pages"
	// is supported; "r2" and "worker" arrive in later PRs.
	Targets []string

	// DryRun computes manifests and would-be-uploaded file lists but performs
	// no network calls. The Report still reflects the would-be attempted counts.
	DryRun bool

	// Concurrency caps CPU-bound work (file hashing, base64 encoding). Defaults
	// to min(GOMAXPROCS, 8) when zero (see ADR 0002 §14.3). HTTP POST parallelism
	// is a separate concern capped at 3 by the deployer, not governed by this.
	Concurrency int

	// Pages carries Cloudflare-Pages-specific options. Nil when Targets does
	// not include "pages".
	Pages *PagesOptions
}

// PagesOptions carries Cloudflare Pages invocation-time parameters. Most
// field-level config (project name, branch) comes from yaml; these are
// CLI overrides.
type PagesOptions struct {
	// Branch overrides the yaml pages.branch. Useful for triggering preview
	// deployments via --branch=preview.
	Branch string

	// CommitSHA and CommitMessage attach git metadata to the deployment. When
	// empty, the deployer may fall back to inferring from git; if still empty,
	// Cloudflare accepts the deployment without commit metadata.
	CommitSHA    string
	CommitMessage string
}

// Report summarizes a deploy invocation. It is JSON-marshaled to stdout on
// completion (and to stderr as a structured log event) for CI consumption.
type Report struct {
	// TraceID correlates all log events for this deploy invocation. Generated
	// at deploy start, propagated to all span logs.
	TraceID string `json:"trace_id"`

	// Target labels the deploy target (e.g. "pages").
	Target string `json:"target"`

	// Attempted counts the files we tried to upload (after dedup).
	Attempted int `json:"attempted"`

	// Succeeded counts files uploaded (or already present via dedup).
	Succeeded int `json:"succeeded"`

	// Failed counts files whose upload exhausted retries.
	Failed int `json:"failed"`

	// Skipped counts files skipped before upload (e.g. identical hash already
	// on remote).
	Skipped int `json:"skipped"`

	// DurationMs is the wall-clock time of the deploy invocation.
	DurationMs int64 `json:"duration_ms"`

	// Failures lists files that ultimately failed, with the stage that failed
	// and the error. Empty when Failed=0.
	Failures []FileFailure `json:"failures,omitempty"`
}

// FileFailure describes a single file that failed during deploy.
type FileFailure struct {
	// Path is the deployment manifest path (leading slash, e.g. "/index.html").
	Path string `json:"path"`

	// Stage identifies which protocol step failed:
	//   "hash"            — manifest build (file read or hash compute)
	//   "check-missing"   — POST /pages/assets/check-missing
	//   "upload"          — POST /pages/assets/upload
	//   "upsert-hashes"   — POST /pages/assets/upsert-hashes
	//   "deployment"      — POST .../deployments
	Stage string `json:"stage"`

	// Error is the final error message after all retries exhausted.
	Error string `json:"error"`
}
