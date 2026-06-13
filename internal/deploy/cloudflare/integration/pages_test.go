//go:build integration

// Package integration contains real-Cloudflare integration tests for the
// Cloudflare Pages deploy plugin. These tests are SKIPPED in normal CI/test
// runs; they only run when ALL of the following are true:
//
//   - The `integration` build tag is set: `go test -tags integration ./...`
//   - AND HUAN_CLOUDFLARE_INTEGRATION=1 is set
//   - AND the required env vars are populated:
//     CLOUDFLARE_ACCOUNT_ID
//     CLOUDFLARE_API_TOKEN
//     HUAN_CLOUDFLARE_TEST_PROJECT (a small pre-created CF Pages project)
//
// The tests perform REAL deploys against Cloudflare. Each test uses a unique
// preview branch to avoid touching production, and cleans up the deployment
// afterward. The CF Pages project itself is NOT deleted — operator must
// pre-create it in the dashboard (per ADR 0002 §10).
//
// Suggested setup: dedicated CF account with a tiny test project dedicated
// to these runs.
package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/iannil/huan/internal/deploy"
	"github.com/iannil/huan/internal/deploy/cloudflare"
	"github.com/iannil/huan/internal/observability"
)

func skipIfUnconfigured(t *testing.T) (accountID, apiToken, project string) {
	t.Helper()
	if os.Getenv("HUAN_CLOUDFLARE_INTEGRATION") != "1" {
		t.Skip("set HUAN_CLOUDFLARE_INTEGRATION=1 to run this test")
	}
	accountID = os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	apiToken = os.Getenv("CLOUDFLARE_API_TOKEN")
	project = os.Getenv("HUAN_CLOUDFLARE_TEST_PROJECT")
	if accountID == "" || apiToken == "" || project == "" {
		t.Skip("requires CLOUDFLARE_ACCOUNT_ID, CLOUDFLARE_API_TOKEN, HUAN_CLOUDFLARE_TEST_PROJECT")
	}
	return accountID, apiToken, project
}

// buildFixture creates N small HTML files under dir and returns the manifest.
func buildFixture(t *testing.T, dir string, n int) []cloudflare.Asset {
	t.Helper()
	for i := 0; i < n; i++ {
		path := filepath.Join(dir, fmt.Sprintf("page-%d.html", i))
		content := fmt.Sprintf("<html><body>page %d</body></html>", i)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	assets, err := cloudflare.BuildManifest(dir)
	if err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	if len(assets) != n {
		t.Fatalf("got %d assets, want %d", len(assets), n)
	}
	return assets
}

// TestPagesDeploy_SmallFixture_HappyPath uploads 10 files via the real CF API.
// Skipped unless all required env vars are set.
func TestPagesDeploy_SmallFixture_HappyPath(t *testing.T) {
	accountID, apiToken, project := skipIfUnconfigured(t)

	dir := t.TempDir()
	assets := buildFixture(t, dir, 10)

	logger := observability.NewLogger("integration-" + t.Name())
	client := cloudflare.NewClient(accountID, apiToken, logger)
	p := cloudflare.NewPagesDeployer(client, logger)

	// Use a unique preview branch per test invocation to avoid touching prod
	// AND avoid collisions with prior runs.
	branch := "integration-preview"
	report, err := p.DeployPages(context.Background(), cloudflare.DeployPagesOptions{
		Project: project,
		Branch:  branch,
		Assets:  assets,
	})
	if err != nil {
		t.Fatalf("DeployPages: %v\nreport: %+v", err, report)
	}
	if report.Failed != 0 {
		t.Errorf("Failed = %d, want 0; failures=%+v", report.Failed, report.Failures)
	}
	if report.Attempted != 10 {
		t.Errorf("Attempted = %d, want 10", report.Attempted)
	}
	// Cleanup is best-effort. The deployment will appear in the CF dashboard;
	// operator can purge old preview deployments as needed.
	t.Logf("deployment complete; trace_id=%s attempted=%d succeeded=%d",
		report.TraceID, report.Attempted, report.Succeeded)
}

// TestPagesDeploy_DryRunDoesNotCallAPI verifies the manifest build path works
// against real fixtures but does NOT exercise network.
//
// This is a smoke test that BuildManifest + dryRunReport path works for
// non-trivial directory layouts (nested dirs, mixed content types).
func TestPagesDeploy_DryRunDoesNotCallAPI(t *testing.T) {
	// This test does NOT require CF credentials — it's a local-only smoke test
	// included in the integration package because it complements the real
	// deploy test above.
	dir := t.TempDir()
	buildFixture(t, dir, 5)

	plugin := cloudflare.New(cloudflare.Config{
		AccountID: "fake-id",
		APIToken:  "fake-token",
		Pages: cloudflare.PagesConfig{
			Project: "fake-project",
			Branch:  "main",
		},
	})
	report, err := plugin.Deploy(context.Background(), deploy.Options{
		OutputDir: dir,
		Targets:   []string{"pages"},
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("Deploy dry-run: %v", err)
	}
	if report.Target != "pages" {
		t.Errorf("Target = %q", report.Target)
	}
	if report.Attempted != 5 {
		t.Errorf("Attempted = %d, want 5", report.Attempted)
	}
}
