package release

import (
	"fmt"
	"regexp"
	"strings"
)

// semverPattern matches canonical semver per semver.org with optional
// prerelease and build metadata. The leading "v" is explicitly NOT allowed
// (we canonicalize to bare version strings).
//
//	0.1.0
//	1.2.3-rc1
//	1.0.0+build.5
//	0.1.0-alpha.1+exp.sha.5114f85
var semverPattern = regexp.MustCompile(
	`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)` + // major.minor.patch
		`(?:-(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*)?` + // prerelease
		`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`, // build metadata
)

// ValidateVersion returns an error if v is not a canonical semver string.
// Empty strings, strings with leading "v", strings with whitespace, and
// non-semver like "latest" or "dev" are all rejected.
//
// Examples:
//
//	"0.1.0"        → nil
//	"0.1.0-rc1"    → nil
//	"0.1.0+b.5"    → nil
//	""             → error "empty version"
//	"v0.1.0"       → error "leading v not allowed"
//	"0.1"          → error "not semver"
//	"0.1.0\n"      → error "contains whitespace"
//	"latest"       → error "not semver"
func ValidateVersion(v string) error {
	if v == "" {
		return fmt.Errorf("empty version")
	}
	if strings.ContainsAny(v, " \t\n\r") {
		return fmt.Errorf("version %q contains whitespace", v)
	}
	if strings.HasPrefix(v, "v") || strings.HasPrefix(v, "V") {
		return fmt.Errorf("leading v not allowed: %q (use %q)", v, v[1:])
	}
	if !semverPattern.MatchString(v) {
		return fmt.Errorf("not semver: %q", v)
	}
	return nil
}
