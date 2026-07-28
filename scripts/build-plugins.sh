#!/usr/bin/env bash
#
# build-plugins.sh — compile every plugin under plugins/*/ into the release
# plugin directory as a Go plugin (.so).
#
# The output file name is derived from each plugin's Name() (the huan.yaml key),
# converting underscores to hyphens per the loader convention:
#   Name()=="qwen3_translate"  ->  qwen3-translate.so
#   Name()=="seo_injector"     ->  seo-injector.so
# This matches internal/plugin.SoFileName so `huan` resolves published plugins
# by config key (see $HUAN_HOME / <project>/plugins lookup in loader.go).
#
# Usage:
#   scripts/build-plugins.sh [OUT_DIR]
# OUT_DIR defaults to release/plugins (per project release conventions).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${1:-$ROOT/release/plugins}"

mkdir -p "$OUT_DIR"

built=0
for dir in "$ROOT"/plugins/*/; do
  [ -f "$dir/go.mod" ] || continue
  name="$(dir="$dir" grep -hoE 'Name\(\) string \{ return "[^"]+"' "$dir"*.go 2>/dev/null | head -1 | sed -E 's/.*return "([^"]+)".*/\1/')"
  if [ -z "$name" ]; then
    echo "skip: $(basename "$dir") (no Name() found)" >&2
    continue
  fi
  so="${name//_/-}.so"
  echo "building $(basename "$dir") -> $so"
  ( cd "$dir" && go build -buildmode=plugin -o "$OUT_DIR/$so" . )
  built=$((built + 1))
done

echo "built $built plugin(s) into $OUT_DIR"
