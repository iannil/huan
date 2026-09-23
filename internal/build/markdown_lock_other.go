//go:build !linux && !darwin

package build

import "os"

// Unsupported platforms retain the memory tier and may read existing records.
func acquireMarkdownLease(string) (*os.File, error) { return nil, nil }
