package cloudflare

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iannil/huan/internal/deploy"
	"github.com/iannil/huan/internal/observability"
)

// DefaultWorkerCompatibilityDate is used when WorkerConfig.CompatibilityDate
// is empty. CF requires this field; we default to the same date wrangler uses.
const DefaultWorkerCompatibilityDate = "2024-01-01"

// WorkerMIMEType is the Content-Type CF Workers API expects for ES module
// scripts. Not the standard application/javascript — CF distinguishes
// service-worker (legacy) from module (current) via this MIME type.
const WorkerMIMEType = "application/javascript+module"

// WorkerDeployer uploads a Worker script via the CF Workers modules API.
type WorkerDeployer struct {
	client *Client
	logger *observability.Logger
}

// NewWorkerDeployer returns a deployer using the given client.
func NewWorkerDeployer(client *Client, logger *observability.Logger) *WorkerDeployer {
	return &WorkerDeployer{client: client, logger: logger}
}

// WorkerDeployResult models the response from PUT .../workers/scripts/{name}.
//
// Audit M3: ModifiedOn is a string (not time.Time) so we always log what CF
// actually returned, even if their format shifts (sub-microsecond precision,
// non-RFC3339 variants). Go's default time.Time JSON unmarshal requires
// strict RFC3339 — a parse failure leaves a zero time which is misleading.
// Strings preserve the raw value; callers can time.Parse on the string if
// needed.
type WorkerDeployResult struct {
	// ScriptName (echoes request).
	ScriptName string `json:"-"`

	// ModifiedOn is the raw timestamp string from CF (typically RFC3339).
	ModifiedOn string `json:"modified_on"`

	// UsageModel (default "bundled").
	UsageModel string `json:"usage_model"`

	// Handler is the entrypoint (typically "default" for ES modules).
	Handler string `json:"handler"`
}

// DeployWorkerOptions captures deploy-time parameters.
type DeployWorkerOptions struct {
	// SourceDir is the project root for resolving relative Script paths.
	SourceDir string

	// DryRun skips the actual PUT; returns success if local file reads OK.
	DryRun bool
}

// Deploy uploads the Worker script + metadata. The metadata JSON includes
// main_module (script basename), compatibility_date, bindings, and routes.
//
// Algorithm per ADR 0002 §8:
//  1. Read Script file relative to opts.SourceDir.
//  2. Build metadata JSON.
//  3. PUT multipart/form-data to /accounts/{id}/workers/scripts/{name}:
//     - part "metadata": application/json
//     - part "<basename>": application/javascript+module with filename set
func (d *WorkerDeployer) Deploy(ctx context.Context, cfg WorkerConfig, opts DeployWorkerOptions) (*deploy.Report, error) {
	start := time.Now()
	traceID := d.logger.TraceID()
	report := &deploy.Report{
		TraceID:   traceID,
		Target:    "worker",
		Attempted: 1,
	}

	if err := cfg.validate(); err != nil {
		report.Failures = append(report.Failures, deploy.FileFailure{
			Stage: "validate",
			Error: err.Error(),
		})
		return report, fmt.Errorf("worker validate: %w", err)
	}

	compatDate := cfg.CompatibilityDate
	if compatDate == "" {
		compatDate = DefaultWorkerCompatibilityDate
	}

	scriptPath := cfg.Script
	if !filepath.IsAbs(scriptPath) && opts.SourceDir != "" {
		scriptPath = filepath.Join(opts.SourceDir, scriptPath)
	}
	scriptContent, err := os.ReadFile(filepath.Clean(scriptPath))
	if err != nil {
		report.Failures = append(report.Failures, deploy.FileFailure{
			Path:  scriptPath,
			Stage: "read-script",
			Error: err.Error(),
		})
		return report, fmt.Errorf("read script %q: %w", scriptPath, err)
	}

	mainModule := filepath.Base(scriptPath)
	metadata := buildWorkerMetadata(mainModule, compatDate, cfg.Bindings, cfg.Routes)
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return report, fmt.Errorf("marshal metadata: %w", err)
	}

	d.logger.Log("worker-deploy", observability.EventFunctionStart, map[string]any{
		"script":             scriptPath,
		"script_name":        cfg.Name,
		"main_module":        mainModule,
		"compatibility_date": compatDate,
		"bindings_count":     len(cfg.Bindings),
		"routes_count":       len(cfg.Routes),
		"script_size":        len(scriptContent),
	})

	if opts.DryRun {
		report.Succeeded = 0 // dry-run: nothing actually uploaded
		report.Skipped = 1
		report.DurationMs = time.Since(start).Milliseconds()
		d.logger.Log("worker-deploy", observability.EventFunctionEnd, map[string]any{
			"dry_run":     true,
			"metadata":    string(metadataJSON),
			"duration_ms": report.DurationMs,
		})
		return report, nil
	}

	fields := map[string]formField{
		"metadata": {
			contentType: "application/json",
			value:       metadataJSON,
		},
		mainModule: {
			contentType: WorkerMIMEType,
			filename:    mainModule,
			value:       scriptContent,
		},
	}
	path := fmt.Sprintf("/accounts/%s/workers/scripts/%s", d.client.accountID, cfg.Name)

	var result WorkerDeployResult
	if err := d.client.PutForm(ctx, path, d.client.APITokenAuth(), fields, &result); err != nil {
		report.Failures = append(report.Failures, deploy.FileFailure{
			Path:  scriptPath,
			Stage: "upload",
			Error: err.Error(),
		})
		report.DurationMs = time.Since(start).Milliseconds()
		return report, fmt.Errorf("worker upload: %w", err)
	}
	result.ScriptName = cfg.Name

	report.Succeeded = 1
	report.DurationMs = time.Since(start).Milliseconds()
	d.logger.Log("worker-deploy", observability.EventFunctionEnd, map[string]any{
		"script_name": result.ScriptName,
		"modified_on": result.ModifiedOn,
		"usage_model": result.UsageModel,
		"duration_ms": report.DurationMs,
	})
	return report, nil
}

// workerMetadata is the JSON shape CF Workers modules API expects in the
// "metadata" multipart part.
type workerMetadata struct {
	MainModule        string              `json:"main_module"`
	CompatibilityDate string              `json:"compatibility_date"`
	Bindings          []workerBindingJSON `json:"bindings,omitempty"`
	Routes            []workerRouteJSON   `json:"routes,omitempty"`
}

// workerBindingJSON is the JSON shape CF Workers modules API expects for one
// binding entry. All optional fields are omitempty, so switch-case in
// buildWorkerMetadata only needs to set the field(s) relevant to that type;
// zero values for unrelated fields are automatically omitted from the JSON.
//
// Audit H6 investigation: previously worried that "default case sets all
// fields" would pollute the JSON. Empirically verified that omitempty
// handles this correctly — a switch case that only sets Bucket for
// r2_bucket leaves NamespaceID/ID/Text as zero strings, which omitempty
// skips. No bug, just adding this comment to lock the contract.
type workerBindingJSON struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Bucket      string `json:"bucket,omitempty"`
	NamespaceID string `json:"namespace_id,omitempty"`
	ID          string `json:"id,omitempty"`
	Text        string `json:"text,omitempty"` // for vars / secret_text
}

type workerRouteJSON struct {
	Pattern string `json:"pattern"`
	// ZoneName is what CF Workers modules API v4 expects. wrangler's
	// metadata JSON uses zone_name (not zone); we follow that convention.
	// Audit H5: previously sent both zone and zone_name with the same
	// value, which could be rejected by CF as ambiguous. Now sends only
	// zone_name.
	ZoneName string `json:"zone_name,omitempty"`
}

// buildWorkerMetadata assembles the metadata JSON from typed config.
// Routes: if Zone is set, both zone and zone_name are populated for max compat.
// Vars bindings: Type="vars" → text field holds the value.
func buildWorkerMetadata(mainModule, compatDate string, bindings []WorkerBinding, routes []WorkerRoute) workerMetadata {
	md := workerMetadata{
		MainModule:        mainModule,
		CompatibilityDate: compatDate,
	}
	for _, b := range bindings {
		bj := workerBindingJSON{
			Type: b.Type,
			Name: b.Name,
		}
		switch strings.ToLower(b.Type) {
		case "r2_bucket":
			bj.Bucket = b.Bucket
		case "kv_namespace":
			bj.NamespaceID = b.NamespaceID
		case "d1":
			bj.ID = b.ID
		case "vars", "secret_text":
			bj.Text = b.Value
		default:
			// Pass through Bucket/NamespaceID/ID/Value so unknown binding types
			// can still carry the user-declared resource identifier.
			bj.Bucket = b.Bucket
			bj.NamespaceID = b.NamespaceID
			bj.ID = b.ID
			bj.Text = b.Value
		}
		md.Bindings = append(md.Bindings, bj)
	}
	for _, r := range routes {
		rj := workerRouteJSON{
			Pattern: r.Pattern,
		}
		if r.Zone != "" {
			rj.ZoneName = r.Zone
		}
		md.Routes = append(md.Routes, rj)
	}
	return md
}
