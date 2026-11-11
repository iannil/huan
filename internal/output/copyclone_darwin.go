//go:build darwin

package output

import "golang.org/x/sys/unix"

// cloneFile copies src to a not-yet-existing dst using APFS copy-on-write.
// It is a no-op optimization: callers must still fall back to a byte copy
// when this returns an error (unsupported FS, cross-device, existing dst).
func cloneFile(src, dst string) error {
	return unix.Clonefile(src, dst, 0)
}
