package cloudflare

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/iannil/huan/internal/deploy"
)

// PagesDeployer orchestrates the 5-endpoint Cloudflare Pages direct-upload
// protocol per ADR 0002 §7.
type PagesDeployer struct {
	client *Client
	logger *deploy.Logger
}

// NewPagesDeployer returns a deployer using the given client. logger carries
// the trace_id used to correlate all log lines for this deploy invocation.
func NewPagesDeployer(client *Client, logger *deploy.Logger) *PagesDeployer {
	return &PagesDeployer{client: client, logger: logger}
}

// DeploymentResult models the response from POST .../deployments.
type DeploymentResult struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Aliases   []string `json:"aliases"`
	Env       string `json:"environment"`
	Stage     string `json:"latest_stage"`
	Status    string `json:"latest_stage_status"`
}

// CommitMeta attaches git metadata to a deployment. Empty values are allowed;
// Cloudflare accepts deployments without commit metadata.
type CommitMeta struct {
	SHA     string
	Message string
	Dirty   bool
}

// DeployPagesOptions captures deploy-time parameters for the Pages protocol.
type DeployPagesOptions struct {
	Project  string
	Branch   string
	Commit   *CommitMeta
	Assets   []Asset
	HTTPParallel int  // hard-capped at 3 internally per ADR 0002 §14.3
}

// DeployPages runs the 5-endpoint Pages direct-upload protocol and returns a
// deploy.Report describing the outcome.
//
// The 5 endpoints (see ADR 0002 §7 for protocol details):
//  1. GET  /accounts/{id}/pages/projects/{project}/upload-token → JWT
//  2. POST /pages/assets/check-missing {hashes:[...]} → missing hashes
//  3. POST /pages/assets/upload [{key,value,metadata,base64}] (batched ≤2000)
//  4. POST /pages/assets/upsert-hashes {hashes:[...]} → ack
//  5. POST /accounts/{id}/pages/projects/{project}/deployments multipart
//
// Per ADR 0002 §9, individual file failures are collected into Report.Failures
// rather than aborting the deploy.
func (p *PagesDeployer) DeployPages(ctx context.Context, opts DeployPagesOptions) (*deploy.Report, error) {
	start := time.Now()
	traceID := p.logger.TraceID()
	report := &deploy.Report{
		TraceID: traceID,
		Target:  "pages",
	}

	if opts.Project == "" {
		report.Failures = append(report.Failures, deploy.FileFailure{Path: "", Stage: "init", Error: "project name required"})
		return report, fmt.Errorf("pages: project name required")
	}
	if opts.Branch == "" {
		return report, fmt.Errorf("pages: branch required")
	}

	// Validate that every Asset.Path has a leading slash (CF API requirement).
	// BuildManifest enforces this for huan-built manifests, but programmatic
	// callers can construct Asset slices directly — fail-fast here rather
	// than getting a confusing CF 4xx mid-deploy.
	for _, a := range opts.Assets {
		if !strings.HasPrefix(a.Path, "/") {
			report.Failures = append(report.Failures, deploy.FileFailure{
				Path:  a.Path,
				Stage: "validate",
				Error: "manifest path missing leading slash (CF requires leading slash on deployment keys)",
			})
			return report, fmt.Errorf("pages: asset path %q missing leading slash", a.Path)
		}
	}

	p.logger.Log("pages-deploy", deploy.EventFunctionStart, map[string]any{
		"project":      opts.Project,
		"branch":       opts.Branch,
		"asset_count":  len(opts.Assets),
	})

	// Step 1: get JWT (cached via Client.UploadToken).
	jwtAuth, err := p.client.JWTViaProject(ctx, opts.Project)
	if err != nil {
		report.Failures = append(report.Failures, deploy.FileFailure{Path: "", Stage: "upload-token", Error: err.Error()})
		return report, fmt.Errorf("upload-token: %w", err)
	}

	report.Attempted = len(opts.Assets)

	// Step 2: check-missing to find which hashes need uploading.
	allHashes := uniqueHashes(opts.Assets)
	missing, err := p.checkMissingWithRefresh(ctx, opts.Project, jwtAuth, allHashes)
	if err != nil {
		report.Failures = append(report.Failures, deploy.FileFailure{Path: "", Stage: "check-missing", Error: err.Error()})
		return report, fmt.Errorf("check-missing: %w", err)
	}
	missingSet := toSet(missing)

	// Split assets into (skip / upload) based on missing set.
	var toUpload []Asset
	for _, a := range opts.Assets {
		if missingSet[a.Hash] {
			toUpload = append(toUpload, a)
		} else {
			report.Skipped++
		}
	}

	p.logger.Log("pages-deploy", deploy.EventPoint, map[string]any{
		"stage":          "check-missing",
		"total_hashes":   len(allHashes),
		"missing_hashes": len(missing),
		"to_upload":      len(toUpload),
		"skipped":        report.Skipped,
	})

	// Step 3: upload missing assets in batches of MaxFilesPerBatch.
	// Per ADR §9 "collection-not-interruption": per-batch failures are
	// collected into report.Failures rather than aborting mid-batch. But if
	// any batch fails, we abort the whole deploy: a deployment manifest
	// referencing hashes not on CF would be rejected by Cloudflare anyway.
	if err := p.uploadAssets(ctx, opts.Project, jwtAuth, toUpload, report); err != nil {
		report.DurationMs = time.Since(start).Milliseconds()
		return report, fmt.Errorf("upload: %w", err)
	}

	// Step 4: upsert-hashes with ALL hashes for this deployment (not just new).
	if err := p.upsertHashesWithRefresh(ctx, opts.Project, jwtAuth, allHashes); err != nil {
		report.Failures = append(report.Failures, deploy.FileFailure{Path: "", Stage: "upsert-hashes", Error: err.Error()})
		// Without upsert, the deployment will fail; abort here.
		report.DurationMs = time.Since(start).Milliseconds()
		return report, fmt.Errorf("upsert-hashes: %w", err)
	}

	// Step 5: create deployment.
	deployment, err := p.createDeployment(ctx, opts.Project, opts.Branch, opts.Commit, opts.Assets)
	if err != nil {
		report.Failures = append(report.Failures, deploy.FileFailure{Path: "", Stage: "deployment", Error: err.Error()})
		report.DurationMs = time.Since(start).Milliseconds()
		return report, fmt.Errorf("create-deployment: %w", err)
	}

	// Finalize report.
	// Succeeded = attempted - skipped - failed (everything that went through
	// the pipeline without error). report.Skipped already counts assets that
	// check-missing found already on remote; report.Failed already counts
	// assets that exhausted retries during upload.
	report.Succeeded = report.Attempted - report.Skipped - report.Failed

	report.DurationMs = time.Since(start).Milliseconds()
	p.logger.Log("pages-deploy", deploy.EventFunctionEnd, map[string]any{
		"deployment_id": deployment.ID,
		"url":           deployment.URL,
		"attempted":     report.Attempted,
		"succeeded":     report.Succeeded,
		"skipped":       report.Skipped,
		"failed":        report.Failed,
		"duration_ms":   report.DurationMs,
	})
	return report, nil
}

// checkMissingWithRefresh calls /pages/assets/check-missing. On JWT-expired
// error, refreshes the JWT once and retries. Returns the missing-hash list.
//
// CF response shape: {"result": ["hash1", ...], "success": true, ...}
// The decodeEnvelope step unwraps .result and decodes it into out.
func (p *PagesDeployer) checkMissingWithRefresh(ctx context.Context, project, jwtAuth string, hashes []string) ([]string, error) {
	body := map[string]any{"hashes": hashes}
	var out []string
	if err := p.callWithJWTRefresh(ctx, project, jwtAuth, "/pages/assets/check-missing", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// upsertHashesWithRefresh calls /pages/assets/upsert-hashes. Empty body
// response from CF (just success:true).
func (p *PagesDeployer) upsertHashesWithRefresh(ctx context.Context, project, jwtAuth string, hashes []string) error {
	body := map[string]any{"hashes": hashes}
	return p.callWithJWTRefresh(ctx, project, jwtAuth, "/pages/assets/upsert-hashes", body, nil)
}

// callWithJWTRefresh wraps PostJSON with a single JWT-refresh-and-retry on
// jwt-expired errors.
func (p *PagesDeployer) callWithJWTRefresh(ctx context.Context, project, jwtAuth, path string, body any, out any) error {
	err := p.client.PostJSON(ctx, path, jwtAuth, body, out)
	if err == nil {
		return nil
	}
	if !isJWTExpired(err) {
		return err
	}
	p.logger.Log("jwt-refresh", deploy.EventPoint, map[string]any{
		"endpoint": path,
		"reason":   "jwt expired",
	})
	p.client.InvalidateJWT(project)
	newAuth, err := p.client.JWTViaProject(ctx, project)
	if err != nil {
		return fmt.Errorf("refresh jwt: %w", err)
	}
	return p.client.PostJSON(ctx, path, newAuth, body, out)
}

// uploadAssets uploads the missing assets in batches. Uses HTTPParallel (capped
// to 3) workers for concurrent batches. Per-file failures are collected into
// report.Failures (collection-not-interruption per ADR §9).
//
// Concurrency: uses a Limiter with HTTP cap = 3 (HTTPMaxParallel). On any 5xx
// response from upload, the Limiter is Degrade()'d to HTTPDegradedParallel=1
// for the remainder of the deploy (per ADR §9 gateway-5xx auto-degrade).
func (p *PagesDeployer) uploadAssets(ctx context.Context, project, jwtAuth string, assets []Asset, report *deploy.Report) error {
	batches := Batch(assets)
	if len(batches) == 0 {
		return nil
	}
	limiter := NewLimiter()
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for i, batch := range batches {
		wg.Add(1)
		go func(idx int, b []Asset) {
			defer wg.Done()
			if err := limiter.Acquire(ctx); err != nil {
				return // ctx cancelled during acquire
			}
			defer limiter.Release()

			if err := p.uploadBatchWithRefresh(ctx, project, jwtAuth, b); err != nil {
				// Per ADR §9: any 5xx triggers permanent HTTP concurrency degrade.
				if is5xxError(err) {
					limiter.Degrade()
				}
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				for _, a := range b {
					report.Failed++
					report.Failures = append(report.Failures, deploy.FileFailure{
						Path:  a.Path,
						Stage: "upload",
						Error: err.Error(),
					})
				}
				errMu.Unlock()
				return
			}
			errMu.Lock()
			report.Succeeded += len(b)
			errMu.Unlock()
		}(i, batch)
	}
	wg.Wait()
	return firstErr
}

// is5xxError returns true if err is a cfError with HTTP status in the 5xx range.
// Used to trigger Limiter.Degrade per ADR §9 gateway-5xx auto-degrade.
func is5xxError(err error) bool {
	var cfe *cfError
	if !errors.As(err, &cfe) {
		return false
	}
	return cfe.Status >= 500 && cfe.Status < 600
}

// uploadBatchWithRefresh POSTs one batch to /pages/assets/upload. The body is
// a JSON array of {key, value, metadata, base64} objects.
func (p *PagesDeployer) uploadBatchWithRefresh(ctx context.Context, project, jwtAuth string, batch []Asset) error {
	items := make([]map[string]any, 0, len(batch))
	for _, a := range batch {
		items = append(items, map[string]any{
			"key":   a.Hash,
			"value": base64.StdEncoding.EncodeToString(a.Content),
			"metadata": map[string]any{
				"contentType": a.ContentType,
			},
			"base64": true,
		})
	}
	if err := p.client.PostJSON(ctx, "/pages/assets/upload", jwtAuth, items, nil); err != nil {
		if !isJWTExpired(err) {
			return err
		}
		p.client.InvalidateJWT(project)
		newAuth, err2 := p.client.JWTViaProject(ctx, project)
		if err2 != nil {
			return fmt.Errorf("refresh jwt: %w", err2)
		}
		return p.client.PostJSON(ctx, "/pages/assets/upload", newAuth, items, nil)
	}
	return nil
}

// createDeployment POSTs multipart/form-data to .../deployments.
func (p *PagesDeployer) createDeployment(ctx context.Context, project, branch string, commit *CommitMeta, assets []Asset) (*DeploymentResult, error) {
	manifest := make(map[string]string, len(assets))
	for _, a := range assets {
		manifest[a.Path] = a.Hash
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	fields := map[string]formField{
		"manifest": {contentType: "application/json", value: manifestBytes},
		"branch":   {contentType: "text/plain", value: []byte(branch)},
	}
	if commit != nil {
		if commit.SHA != "" {
			fields["commit_hash"] = formField{contentType: "text/plain", value: []byte(commit.SHA)}
		}
		if commit.Message != "" {
			fields["commit_message"] = formField{contentType: "text/plain", value: []byte(commit.Message)}
		}
		if commit.Dirty {
			fields["commit_dirty"] = formField{contentType: "text/plain", value: []byte("true")}
		}
	}

	path := fmt.Sprintf("/accounts/%s/pages/projects/%s/deployments", p.client.accountID, project)
	var result DeploymentResult
	if err := p.client.PostForm(ctx, path, p.client.APITokenAuth(), fields, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// uniqueHashes returns the deduplicated list of hashes from assets.
func uniqueHashes(assets []Asset) []string {
	seen := make(map[string]struct{}, len(assets))
	out := make([]string, 0, len(assets))
	for _, a := range assets {
		if _, ok := seen[a.Hash]; ok {
			continue
		}
		seen[a.Hash] = struct{}{}
		out = append(out, a.Hash)
	}
	return out
}

func toSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, i := range items {
		out[i] = true
	}
	return out
}
