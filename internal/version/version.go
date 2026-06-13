// Package version exposes the huan version string, embedded from the VERSION
// file at build time.
package version

import _ "embed"

//go:embed VERSION
var versionString string

// String returns the huan version (e.g. "0.1.0").
func String() string {
	return versionString
}
