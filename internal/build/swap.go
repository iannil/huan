package build

import (
	"os"
	"sync"
)

// BuildDirSwapper publishes complete builds while deleting the old output in
// the background. Its zero value is ready to use. Call Swap serially and stop
// calling Swap before the final Wait. Do not copy after use.
type BuildDirSwapper struct {
	workers sync.WaitGroup
	cleanup func(string)
}

// Swap waits for the previous cleanup before reusing liveDir+".old", then
// publishes nextDir. A successful swap starts at most one background cleanup.
// Failed swaps retain the same best-effort rollback as SwapBuildDir.
func (s *BuildDirSwapper) Swap(liveDir, nextDir string) error {
	s.Wait()
	return swapBuildDir(liveDir, nextDir, func(oldDir string) {
		cleanup := s.cleanup
		if cleanup == nil {
			cleanup = func(path string) { _ = os.RemoveAll(path) }
		}
		s.workers.Add(1)
		go func() {
			defer s.workers.Done()
			cleanup(oldDir)
		}()
	})
}

// Wait waits for the most recent old-output cleanup to finish.
func (s *BuildDirSwapper) Wait() { s.workers.Wait() }

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
	return swapBuildDir(liveDir, nextDir, func(path string) { _ = os.RemoveAll(path) })
}

func swapBuildDir(liveDir, nextDir string, cleanup func(string)) error {
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
	cleanup(oldDir)
	return nil
}
