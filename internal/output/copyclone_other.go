//go:build !darwin

package output

import "errors"

// cloneFile is unsupported on this platform; callers fall back to a byte copy.
func cloneFile(src, dst string) error {
	return errors.New("clonefile unsupported")
}
