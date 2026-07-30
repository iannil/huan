#!/usr/bin/env bash
#
# release.sh — One-command release build for huan + plugins.
#
# Compiles the huan binary and all plugins for the current platform,
# outputs them to release/v{version}/{os}-{arch}/ with checksums and
# provenance manifest.
#
# Usage:
#   scripts/release.sh
#   scripts/release.sh --out-dir /tmp/test-release
#   scripts/release.sh --skip-build
#   scripts/release.sh --skip-checksums
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# ---- Flags ----
OUT_DIR=""
SKIP_BUILD=false
SKIP_CHECKSUMS=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out-dir) OUT_DIR="$2"; shift 2 ;;
    --skip-build) SKIP_BUILD=true; shift ;;
    --skip-checksums) SKIP_CHECKSUMS=true; shift ;;
    --help|-h)
      echo "Usage: $0 [--out-dir DIR] [--skip-build] [--skip-checksums]"
      exit 0 ;;
    *) echo "unknown flag: $1"; exit 1 ;;
  esac
done

# ---- Step 1: Version detection ----
VERSION_FILE="$ROOT/internal/version/VERSION"
if [ ! -f "$VERSION_FILE" ]; then
  echo "error: VERSION file not found at $VERSION_FILE" >&2
  exit 1
fi
VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"

# Validate semver: must match major.minor.patch with optional prerelease/build
if ! echo "$VERSION" | grep -qE '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(\.[0-9a-zA-Z-]+)*)?(\+[0-9a-zA-Z-]+(\.[0-9a-zA-Z-]+)*)?$'; then
  echo "error: invalid semver in VERSION: '$VERSION'" >&2
  exit 1
fi
if [[ "$VERSION" =~ ^[vV] ]]; then
  echo "error: leading v not allowed in VERSION: '$VERSION'" >&2
  exit 1
fi

# ---- Step 2: Platform detection ----
GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"
PLATFORM="${GOOS}-${GOARCH}"

# ---- Step 3: Output directory ----
if [ -z "$OUT_DIR" ]; then
  OUT_DIR="$ROOT/release/v$VERSION"
fi
BIN_DIR="$OUT_DIR/$PLATFORM"
PLUGIN_DIR="$BIN_DIR/plugins"

# ---- Step 4: Build huan binary ----
if [ "$SKIP_BUILD" = false ]; then
  mkdir -p "$BIN_DIR"

  echo "==> building huan v$VERSION for $PLATFORM"
  CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o "$BIN_DIR/huan" \
    "$ROOT/cmd/huan"

  echo "     binary: $(wc -c < "$BIN_DIR/huan" | tr -d ' ') bytes"
else
  echo "==> skipping build (--skip-build)"
fi

# ---- Step 5: Build plugins ----
if [ "$SKIP_BUILD" = false ]; then
  mkdir -p "$PLUGIN_DIR"

  FAILURES=()
  for dir in "$ROOT"/plugins/*/; do
    name="$(dir="$dir" grep -hoE 'Name\(\) string \{ return "[^"]+"' "$dir"*.go 2>/dev/null | head -1 | sed -E 's/.*return "([^"]+)".*/\1/')"
    if [ -z "$name" ]; then
      echo "  skip: $(basename "$dir") (no Name() found)"
      continue
    fi
    so="${name//_/-}.so"
    echo "  plugin: $(basename "$dir") -> $so"
    if ! ( cd "$dir" && go build -buildmode=plugin -o "$PLUGIN_DIR/$so" . ); then
      echo "  error: $(basename "$dir") build failed, skipping" >&2
      FAILURES+=("$(basename "$dir")")
    fi
  done

  if [ ${#FAILURES[@]} -gt 0 ]; then
    echo "  plugin failures: ${FAILURES[*]}" >&2
  fi
fi

# ---- Step 6: Generate checksums ----
if [ "$SKIP_CHECKSUMS" = false ]; then
  # Determine available sha256 command
  if command -v sha256sum &>/dev/null; then
    SHASUM_CMD="sha256sum"
  elif command -v shasum &>/dev/null; then
    SHASUM_CMD="shasum -a 256"
  else
    echo "error: no sha256sum or shasum found" >&2
    exit 1
  fi

  CHECKSUMS_FILE="$OUT_DIR/huan_$VERSION-checksums.txt"
  CHECKSUMS_TMP="$(mktemp)"

  # Collect all files to checksum (relative to OUT_DIR)
  FILES_TO_CHECKSUM=()
  if [ -f "$BIN_DIR/huan" ]; then
    FILES_TO_CHECKSUM+=("$PLATFORM/huan")
  fi
  if [ -d "$PLUGIN_DIR" ]; then
    for so in "$PLUGIN_DIR"/*.so; do
      [ -f "$so" ] && FILES_TO_CHECKSUM+=("$PLATFORM/plugins/$(basename "$so")")
    done
  fi

  for f in "${FILES_TO_CHECKSUM[@]}"; do
    $SHASUM_CMD "$OUT_DIR/$f" | sed "s|$OUT_DIR/||" >> "$CHECKSUMS_TMP"
  done

  mv "$CHECKSUMS_TMP" "$CHECKSUMS_FILE"
  echo "  checksums: $(wc -l < "$CHECKSUMS_FILE" | tr -d ' ') files"
fi

# ---- Step 7: Generate manifest ----
if [ "$SKIP_CHECKSUMS" = false ]; then
  MANIFEST_FILE="$OUT_DIR/huan_$VERSION-manifest.json"
  MANIFEST_TMP="$(mktemp)"

  # Gather metadata
  GO_VERSION="$(go version | sed 's/.*go\([0-9.]*\).*/\1/')"
  GIT_SHA="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
  GIT_DIRTY=false
  if [ -n "$(git -C "$ROOT" status --porcelain 2>/dev/null)" ]; then
    GIT_DIRTY=true
  fi
  BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  # Build file list
  FILE_LIST="["
  SEP=""
  for f in "${FILES_TO_CHECKSUM[@]}"; do
    SHA=$(grep "  $f$" "$CHECKSUMS_FILE" 2>/dev/null | awk '{print $1}')
    SIZE=$(stat -f%z "$OUT_DIR/$f" 2>/dev/null || stat -c%s "$OUT_DIR/$f" 2>/dev/null || echo 0)
    FILE_LIST+="$SEP{\"path\":\"$f\",\"sha256\":\"$SHA\",\"size\":$SIZE}"
    SEP=","
  done
  FILE_LIST+="]"

  cat > "$MANIFEST_TMP" <<EOF
{
  "version": "$VERSION",
  "go_version": "$GO_VERSION",
  "git_sha": "$GIT_SHA",
  "git_dirty": $GIT_DIRTY,
  "build_time": "$BUILD_TIME",
  "host_platform": "$PLATFORM",
  "files": $FILE_LIST
}
EOF

  mv "$MANIFEST_TMP" "$MANIFEST_FILE"
  echo "  manifest: $(wc -c < "$MANIFEST_FILE" | tr -d ' ') bytes"
fi

# ---- Step 8: Summary ----
echo ""
echo "release complete: v$VERSION ($PLATFORM)"
echo "  out_dir: $OUT_DIR"
echo "  binary:  $BIN_DIR/huan"
echo "  plugins: $(ls "$PLUGIN_DIR"/*.so 2>/dev/null | wc -l | tr -d ' ') files"
echo "  checksums: $CHECKSUMS_FILE"
echo "  manifest:  $MANIFEST_FILE"
if [ ${#FAILURES[@]} -gt 0 ]; then
  echo "  plugin failures: ${FAILURES[*]}"
fi