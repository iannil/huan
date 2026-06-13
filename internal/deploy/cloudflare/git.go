package cloudflare

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// gitCommandTimeout caps how long we wait for git invocations. Generous enough
// for slow disks, short enough that a misconfigured repo doesn't hang deploy.
const gitCommandTimeout = 5 * time.Second

// InferCommitMetadata resolves commit metadata via three-layer fallback per
// ADR 0002 §14.2:
//
//  1. Explicit flag values (--commit-sha / --commit-message) win if non-empty.
//  2. Otherwise, infer from the current git repo via `git rev-parse HEAD` and
//     `git log -1 --pretty=%s`.
//  3. If git inference fails (not a repo, no commits, git not installed),
//     return CommitMeta with empty values; Cloudflare accepts deployments
//     without commit metadata.
//
// flagSHA / flagMessage are typically passed from CLI flags. The working
// directory for git inference is the process's current working directory —
// callers should chdir to the project root before calling (or pass the project
// root via cwd parameter in future).
func InferCommitMetadata(flagSHA, flagMessage string) CommitMeta {
	cm := CommitMeta{
		SHA:     flagSHA,
		Message: flagMessage,
	}
	if cm.SHA != "" && cm.Message != "" {
		return cm
	}

	// Try git inference for whichever field is empty.
	gitSHA, gitMessage, dirty := gitInfer()
	if cm.SHA == "" {
		cm.SHA = gitSHA
	}
	if cm.Message == "" {
		cm.Message = gitMessage
	}
	cm.Dirty = dirty
	return cm
}

// gitInfer runs git rev-parse / git log / git status in the cwd and returns
// SHA + last commit message + dirty flag. Returns all-empty on any error
// (non-git dir, git not installed, etc.).
func gitInfer() (sha, message string, dirty bool) {
	ctx, cancel := context.WithTimeout(context.Background(), gitCommandTimeout)
	defer cancel()

	sha, err := gitOutput(ctx, "rev-parse", "HEAD")
	if err != nil || sha == "" {
		return "", "", false
	}

	message, _ = gitOutput(ctx, "log", "-1", "--pretty=%s")

	// Dirty check: `git status --porcelain` returns empty if working tree clean.
	out, err := gitOutput(ctx, "status", "--porcelain")
	if err == nil {
		dirty = strings.TrimSpace(out) != ""
	}

	return sha, message, dirty
}

// gitOutput runs git with the given args and returns trimmed stdout. Returns
// error on non-zero exit or context timeout.
func gitOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
