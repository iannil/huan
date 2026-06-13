package version

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestString_NonEmpty(t *testing.T) {
	if v := String(); v == "" {
		t.Fatal("version string is empty; check internal/version/VERSION")
	}
}

func TestParseVCSSettings_Empty(t *testing.T) {
	got := parseVCSSettings(nil)
	if got.Available {
		t.Errorf("Available = true on empty settings, want false")
	}
	if got.SHA != "" || got.Dirty || got.CommitTime != "" {
		t.Errorf("zero VCSInfo expected, got %+v", got)
	}
}

func TestParseVCSSettings_AllFields(t *testing.T) {
	settings := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "87b2836abcdef1234567890"},
		{Key: "vcs.modified", Value: "false"},
		{Key: "vcs.time", Value: "2026-06-13T17:42:00Z"},
		{Key: "GOOS", Value: "darwin"},
		{Key: "GOARCH", Value: "arm64"},
	}
	got := parseVCSSettings(settings)
	if !got.Available {
		t.Fatal("Available = false, want true")
	}
	if got.SHA != "87b2836" {
		t.Errorf("SHA = %q, want 87b2836", got.SHA)
	}
	if got.Dirty {
		t.Errorf("Dirty = true, want false")
	}
	if got.CommitTime != "2026-06-13T17:42:00Z" {
		t.Errorf("CommitTime = %q", got.CommitTime)
	}
}

func TestParseVCSSettings_DirtyTrue(t *testing.T) {
	settings := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "abcdef1234567890"},
		{Key: "vcs.modified", Value: "true"},
	}
	got := parseVCSSettings(settings)
	if !got.Dirty {
		t.Error("Dirty = false, want true")
	}
	if got.SHA != "abcdef1" {
		t.Errorf("SHA = %q, want abcdef1", got.SHA)
	}
}

func TestParseVCSSettings_ShortSHA(t *testing.T) {
	// Revisions shorter than 7 chars should be returned as-is (no panic).
	settings := []debug.BuildSetting{
		{Key: "vcs.revision", Value: "abc"},
	}
	got := parseVCSSettings(settings)
	if got.SHA != "abc" {
		t.Errorf("SHA = %q, want abc", got.SHA)
	}
}

func TestParseVCSSettings_OnlyModifiedStillAvailable(t *testing.T) {
	// Per implementation: Available is set true by any vcs.* field, not just
	// revision. If only vcs.modified is present, the binary was built from a
	// git working tree but the revision somehow wasn't recorded.
	settings := []debug.BuildSetting{
		{Key: "vcs.modified", Value: "false"},
	}
	got := parseVCSSettings(settings)
	if !got.Available {
		t.Error("Available = false with vcs.modified present, want true")
	}
}

func TestStringWithVCS_NoVCSInfo(t *testing.T) {
	// When VCS() returns no info, output is plain version. Hard to drive
	// directly without mocking debug.ReadBuildInfo, but the fallback path
	// is exercised whenever SHA is empty: StringWithVCS must not append
	// " ()". This test documents the contract by checking that no parens
	// appear when SHA is empty.
	got := StringWithVCS()
	if strings.Contains(got, "()") {
		t.Errorf("StringWithVCS produced %q with empty parens", got)
	}
}

func TestStringWithVCS_FormatsSHA(t *testing.T) {
	// Verify the formatter directly with a SHA-bearing VCSInfo by patching
	// via the formatter helpers. Since StringWithVCS reads the real
	// debug.ReadBuildInfo, we exercise the same code path through
	// formatVersionWithVCS.
	got := formatVersionWithVCS("0.1.0", VCSInfo{
		Available: true,
		SHA:       "87b2836",
		Dirty:     false,
	})
	if got != "0.1.0 (87b2836)" {
		t.Errorf("got %q, want 0.1.0 (87b2836)", got)
	}
}

func TestStringWithVCS_DirtySuffix(t *testing.T) {
	got := formatVersionWithVCS("0.1.0", VCSInfo{
		Available: true,
		SHA:       "87b2836",
		Dirty:     true,
	})
	if got != "0.1.0 (87b2836-dirty)" {
		t.Errorf("got %q, want 0.1.0 (87b2836-dirty)", got)
	}
}
