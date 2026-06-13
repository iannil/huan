// Package version exposes the huan version string, embedded from the VERSION
// file at build time.
package version

import (
	"fmt"
	"runtime/debug"
	"strings"

	_ "embed"
)

//go:embed VERSION
var versionString string

func init() {
	// VERSION file may contain a trailing newline (common when edited by
	// text editors); strip it so versionString is the bare semver.
	versionString = strings.TrimSpace(versionString)
}

// String returns the huan version (e.g. "0.1.0").
func String() string {
	return versionString
}

// VCSInfo carries VCS (git) provenance extracted from debug.ReadBuildInfo.
// All fields are zero-valued if the binary was built outside a git working
// tree (e.g. via `go install` from a tarball) or with -buildvcs=false.
type VCSInfo struct {
	SHA         string // short SHA (first 7 chars), empty if unavailable
	Dirty       bool   // true if working tree had uncommitted changes
	CommitTime  string // ISO 8601 / RFC 3339 of the commit, empty if unavailable
	Available   bool   // false when no VCS info was embedded at all
}

// VCS returns VCS provenance from Go's embedded build info (Go 1.18+ auto-
// embeds VCS settings when building from a git working tree; no -ldflags
// injection needed). When the binary was not built from a git checkout,
// Available is false and all other fields are zero.
//
// The SHA is shortened to 7 chars to match `git log --oneline` style; this
// is enough for "which commit produced this binary" identification while
// keeping `huan version` output compact.
func VCS() VCSInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return VCSInfo{}
	}
	return parseVCSSettings(info.Settings)
}

// parseVCSSettings is the pure-function core of VCS, extracted so unit tests
// can drive the parsing logic without needing to influence debug.ReadBuildInfo.
func parseVCSSettings(settings []debug.BuildSetting) VCSInfo {
	out := VCSInfo{}
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			out.Available = true
			if len(s.Value) >= 7 {
				out.SHA = s.Value[:7]
			} else {
				out.SHA = s.Value
			}
		case "vcs.modified":
			out.Available = true
			out.Dirty = s.Value == "true"
		case "vcs.time":
			out.Available = true
			out.CommitTime = s.Value
		}
	}
	return out
}

// StringWithVCS returns the version suffixed with short SHA (and -dirty
// marker) when VCS info is available. Falls back to plain version when not.
// Format examples:
//
//	"0.1.0"                       (no VCS info)
//	"0.1.0 (87b2836)"             (clean commit)
//	"0.1.0 (87b2836-dirty)"       (uncommitted changes at build time)
func StringWithVCS() string {
	return formatVersionWithVCS(versionString, VCS())
}

// formatVersionWithVCS is the pure-function core of StringWithVCS so tests
// can drive the formatting directly without depending on debug.ReadBuildInfo.
func formatVersionWithVCS(v string, info VCSInfo) string {
	if !info.Available || info.SHA == "" {
		return v
	}
	suffix := info.SHA
	if info.Dirty {
		suffix += "-dirty"
	}
	return fmt.Sprintf("%s (%s)", v, suffix)
}
