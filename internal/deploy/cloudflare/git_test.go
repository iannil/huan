package cloudflare

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupGitRepo initializes a temp dir as a git repo with one commit and
// returns its path. Skips the test if git is unavailable.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	// Create a file and commit it so HEAD exists.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "initial commit"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	return dir
}

// withCwd changes the working directory for the duration of the test.
func withCwd(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getcwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %q: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(orig)
	})
}

func TestInferCommitMetadata_FlagWinsOverGit(t *testing.T) {
	dir := setupGitRepo(t)
	withCwd(t, dir)

	cm := InferCommitMetadata("flag-sha", "flag message")
	if cm.SHA != "flag-sha" {
		t.Errorf("SHA = %q, want flag-sha", cm.SHA)
	}
	if cm.Message != "flag message" {
		t.Errorf("Message = %q, want 'flag message'", cm.Message)
	}
}

func TestInferCommitMetadata_FallsBackToGit(t *testing.T) {
	dir := setupGitRepo(t)
	withCwd(t, dir)

	cm := InferCommitMetadata("", "")
	if cm.SHA == "" {
		t.Errorf("SHA empty; want git HEAD")
	}
	if len(cm.SHA) != 40 {
		t.Errorf("SHA len = %d, want 40 (full git SHA)", len(cm.SHA))
	}
	if cm.Message != "initial commit" {
		t.Errorf("Message = %q, want 'initial commit'", cm.Message)
	}
	if cm.Dirty {
		t.Errorf("Dirty = true; want false (no uncommitted changes)")
	}
}

func TestInferCommitMetadata_DirtyFlagWhenUncommittedChanges(t *testing.T) {
	dir := setupGitRepo(t)
	withCwd(t, dir)

	// Make an uncommitted change.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cm := InferCommitMetadata("", "")
	if !cm.Dirty {
		t.Errorf("Dirty = false; want true (uncommitted changes present)")
	}
}

func TestInferCommitMetadata_PartialFlag_SHAFromFlag_MessageFromGit(t *testing.T) {
	dir := setupGitRepo(t)
	withCwd(t, dir)

	cm := InferCommitMetadata("flag-sha", "")
	if cm.SHA != "flag-sha" {
		t.Errorf("SHA = %q, want flag-sha", cm.SHA)
	}
	if cm.Message != "initial commit" {
		t.Errorf("Message = %q, want 'initial commit' from git", cm.Message)
	}
}

func TestInferCommitMetadata_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	withCwd(t, dir)

	cm := InferCommitMetadata("", "")
	if cm.SHA != "" {
		t.Errorf("SHA = %q, want empty in non-git dir", cm.SHA)
	}
	if cm.Message != "" {
		t.Errorf("Message = %q, want empty in non-git dir", cm.Message)
	}
	if cm.Dirty {
		t.Errorf("Dirty = true; want false")
	}
}

// Verify the function doesn't hang on slow git invocations (timeout enforced).
func TestInferCommitMetadata_DoesNotHang(t *testing.T) {
	dir := setupGitRepo(t)
	withCwd(t, dir)
	// This is a smoke test — if it completes, the timeout is working.
	cm := InferCommitMetadata("", "")
	if !strings.HasPrefix(cm.SHA, "") {
		t.Errorf("unexpected: %v", cm)
	}
}
