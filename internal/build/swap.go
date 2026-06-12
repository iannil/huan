package build

import "os"

// SwapBuildDir atomically (enough) replaces liveDir's contents with nextDir's.
//
// Usage: build the new site into nextDir, then call SwapBuildDir(liveDir, nextDir).
// On success, liveDir contains the new build and the old contents are removed.
// On failure, liveDir is untouched and the caller should clean up nextDir.
//
// Implementation: rename liveDir → liveDir+".old", rename nextDir → liveDir,
// then RemoveAll(liveDir+".old"). Two renames are not atomic together, so a
// request hitting liveDir in the microsecond window between them may 404.
// This is acceptable for a dev server: the LiveReload client retries, and the
// user editing won't notice.
//
// If the second rename fails (extremely unlikely — typically only if nextDir
// disappeared between caller creating it and us renaming it), we attempt to
// restore the original liveDir before returning the error.
func SwapBuildDir(liveDir, nextDir string) error {
	oldDir := liveDir + ".old"
	// Clean any leftover .old from a previous crashed swap.
	_ = os.RemoveAll(oldDir)

	if err := os.Rename(liveDir, oldDir); err != nil {
		return err
	}
	if err := os.Rename(nextDir, liveDir); err != nil {
		// Best-effort rollback.
		_ = os.Rename(oldDir, liveDir)
		return err
	}
	_ = os.RemoveAll(oldDir)
	return nil
}
