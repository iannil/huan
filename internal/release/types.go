// Package release implements the `huan release` command: cross-compile huan
// for a set of target platforms, archive each with LICENSE/README, compute
// sha256 checksums, and emit a JSON manifest into /release/{version}/.
//
// Scope (per ADR 0004 + docs/progress/release-command.md):
//   - Pure local artifact production. No git tag, no push, no remote upload.
//   - Operator huan (the binary running `huan release`) is decoupled from
//     artifact huans (the binaries being packaged). Artifacts are always
//     freshly compiled via `go build` with cross-compile flags.
//   - Five standard targets by default; `--targets` flag for overrides.
//   - Reproducible: -trimpath + CGO_ENABLED=0 + no wall-clock ldflags =>
//     same commit + same Go version produces byte-identical binaries.
//
// Logging uses the cross-cutting internal/observability package (shared
// with internal/deploy). Report/Artifact/Failure types are release-specific
// and stay in this package — their shapes differ from deploy's analogues.
package release

// Target describes a GOOS/GOARCH pair to compile for.
type Target struct {
	OS   string // e.g. "darwin", "linux", "windows"
	Arch string // e.g. "amd64", "arm64"
}

// String returns "os/arch" (e.g. "darwin/arm64") for log lines and errors.
func (t Target) String() string { return t.OS + "/" + t.Arch }

// StandardTargets is the default platform matrix: darwin (amd64+arm64),
// linux (amd64+arm64), windows (amd64). Covers ~99% of Go CLI users per
// Hugo/Caddy precedent. Override with the --targets flag.
var StandardTargets = []Target{
	{OS: "darwin", Arch: "amd64"},
	{OS: "darwin", Arch: "arm64"},
	{OS: "linux", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
	{OS: "windows", Arch: "amd64"},
}

// Options configures a release invocation.
type Options struct {
	Version  string   // canonical semver (no leading v), e.g. "0.1.0"
	OutDir   string   // absolute path to /release/{version}/
	Targets  []Target // platforms to compile for; must be non-empty
	SourceDir string  // project root, used to locate LICENSE/README and main pkg
	DryRun   bool     // when true, build to a temp dir, never touch OutDir
	TraceID  string   // optional; auto-generated when empty
}

// Artifact describes one file in the release output.
type Artifact struct {
	Name   string `json:"name"`   // e.g. "huan_0.1.0_darwin_arm64.tar.gz"
	SHA256 string `json:"sha256"` // hex-encoded sha256 of the file bytes
	Size   int64  `json:"size"`   // file size in bytes
	Binary string `json:"binary"` // binary name inside the archive ("huan" or "huan.exe")
	OS     string `json:"os"`     // target OS (matches Target.OS)
	Arch   string `json:"arch"`   // target arch (matches Target.Arch)
}

// Failure captures a per-target error during the release pipeline.
type Failure struct {
	Target Target `json:"target"` // darwin/arm64 etc.
	Phase  string `json:"phase"`  // "compile" / "archive" / "checksum" / "manifest"
	Error  string `json:"error"`
}

// Report is the machine-readable outcome of a release invocation.
type Report struct {
	TraceID    string     `json:"trace_id"`
	Version    string     `json:"version"`
	GoVersion  string     `json:"go_version"`
	GitSHA     string     `json:"git_sha,omitempty"`
	GitDirty   bool       `json:"git_dirty,omitempty"`
	BuildTime  string     `json:"build_time"`
	OutDir     string     `json:"out_dir"`
	Targets    []string   `json:"targets"`
	Artifacts  []Artifact `json:"artifacts"`
	DurationMs int64      `json:"duration_ms"`
	DryRun     bool       `json:"dry_run"`
	Failures   []Failure  `json:"failures,omitempty"`
}
