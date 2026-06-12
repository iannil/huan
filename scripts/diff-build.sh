#!/bin/bash
# diff-build.sh: Compare huan output against a Hugo baseline.
#
# Usage:
#   ./scripts/diff-build.sh
#
# Builds both Hugo and huan outputs, then diffs them recursively. Reports:
#   - Files only in Hugo (missing from huan)
#   - Files only in huan (extra)
#   - Files differing in content (with allowed-difference filtering)
#
# Allowed differences (filtered out):
#   - <meta name="generator" content="Hugo ..."> vs huan's version
#   - Empty content variations in whitespace-only files

set -euo pipefail

PROJECT_DIR="/Users/rong.zhu/Code/zhurongshuo"
HUAN_BIN="/Users/rong.zhu/Code/huan/huan"
HUAN_REPO_DIR="/Users/rong.zhu/Code/huan"
HUGO_DIR="/tmp/hugo-baseline"
HUAN_DIR="/tmp/huan-output"
DIFF_DIR="/tmp/huan-diff"

echo "=== Step 1: Build Hugo baseline ==="
rm -rf "$HUGO_DIR"
hugo --destination "$HUGO_DIR" -s "$PROJECT_DIR" --quiet
echo "Hugo output: $(find "$HUGO_DIR" -type f | wc -l | tr -d ' ') files"

echo ""
echo "=== Step 2: Build huan output ==="
rm -rf "$HUAN_DIR" "$PROJECT_DIR/docs"
"$HUAN_BIN" build -s "$PROJECT_DIR" > /dev/null
cp -r "$PROJECT_DIR/docs" "$HUAN_DIR"
echo "huan output: $(find "$HUAN_DIR" -type f | wc -l | tr -d ' ') files"

echo ""
echo "=== Step 3: File list diff ==="
HUGO_FILES=$(cd "$HUGO_DIR" && find . -type f | sort)
HUAN_FILES=$(cd "$HUAN_DIR" && find . -type f | sort)

ONLY_HUGO=$(comm -23 <(echo "$HUGO_FILES") <(echo "$HUAN_FILES"))
ONLY_HUAN=$(comm -13 <(echo "$HUGO_FILES") <(echo "$HUAN_FILES"))

HUGO_ONLY_COUNT=$(echo "$ONLY_HUGO" | grep -c . || true)
HUAN_ONLY_COUNT=$(echo "$ONLY_HUAN" | grep -c . || true)

echo "Files only in Hugo: $HUGO_ONLY_COUNT"
echo "Files only in huan: $HUAN_ONLY_COUNT"

if [ "$HUGO_ONLY_COUNT" -gt 0 ]; then
    echo ""
    echo "--- Files only in Hugo (top 20 by extension) ---"
    echo "$ONLY_HUGO" | sed 's/.*\.//' | sort | uniq -c | sort -rn | head -20
fi

echo ""
echo "=== Step 4: Content diff (sample 20 files) ==="
mkdir -p "$DIFF_DIR"

# Get files that exist in both
COMMON=$(comm -12 <(echo "$HUGO_FILES") <(echo "$HUAN_FILES"))

DIFF_COUNT=0
IDENTICAL_COUNT=0
ALLOWED_DIFF_COUNT=0
SAMPLE_SHOWN=0

while IFS= read -r f; do
    [ -z "$f" ] && continue

    hugo_file="$HUGO_DIR/$f"
    huan_file="$HUAN_DIR/$f"

    if cmp -s "$hugo_file" "$huan_file"; then
        IDENTICAL_COUNT=$((IDENTICAL_COUNT + 1))
        continue
    fi

    # Check if differences are only "allowed" (generator meta, etc.)
    diff_out=$(diff "$hugo_file" "$huan_file" 2>&1 || true)

    # Filter out allowed differences
    filtered=$(echo "$diff_out" | grep -v 'name="generator"' | \
              grep -v 'content="Hugo' | \
              grep -E '^[<>]' || true)

    if [ -z "$filtered" ]; then
        ALLOWED_DIFF_COUNT=$((ALLOWED_DIFF_COUNT + 1))
        continue
    fi

    DIFF_COUNT=$((DIFF_COUNT + 1))

    if [ "$SAMPLE_SHOWN" -lt 5 ]; then
        echo ""
        echo "--- Diff in $f ---"
        echo "$filtered" | head -10
        SAMPLE_SHOWN=$((SAMPLE_SHOWN + 1))
    fi
done <<< "$COMMON"

echo ""
echo "=== Summary ==="
echo "Identical files:           $IDENTICAL_COUNT"
echo "Files with allowed diffs:  $ALLOWED_DIFF_COUNT"
echo "Files with real diffs:     $DIFF_COUNT"
echo "Files only in Hugo:        $HUGO_ONLY_COUNT"
echo "Files only in huan:        $HUAN_ONLY_COUNT"

echo ""
echo "=== Step 5: Three-dimension equivalence check ==="

# Build equiv-check binary if missing or stale
EQUIV_BIN="/tmp/equiv-check"
needs_build=0
if [ ! -x "$EQUIV_BIN" ]; then
    needs_build=1
elif [ -n "$(find "$HUAN_REPO_DIR/cmd/equiv-check" "$HUAN_REPO_DIR/internal/equiv" -name '*.go' -newer "$EQUIV_BIN" 2>/dev/null)" ]; then
    needs_build=1
fi

if [ "$needs_build" -eq 1 ]; then
    echo "Building equiv-check..."
    (cd "$HUAN_REPO_DIR" && go build -o "$EQUIV_BIN" ./cmd/equiv-check/) || {
        echo "FAILED to build equiv-check; skipping 3-dim check"
        # Don't fail the script — byte-diff summary above is still useful
        exit 0
    }
fi

# Run all three modes; normalized/seo/ai failures exit 1
"$EQUIV_BIN" -a "$HUAN_DIR" -b "$HUGO_DIR" --mode all || {
    echo "Three-dimension equivalence check FAILED"
    exit 1
}
echo "Three-dimension equivalence check PASSED"
