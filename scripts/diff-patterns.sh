#!/bin/bash
# diff-patterns.sh - Sample many files and categorize the differences.
set -euo pipefail

HUGO_DIR="/tmp/hugo-baseline"
HUAN_DIR="/tmp/huan-output"

# Get common files
cd "$HUGO_DIR" && find . -type f | sort > /tmp/h-files.txt
cd "$HUAN_DIR" && find . -type f | sort > /tmp/n-files.txt
COMMON=$(comm -12 /tmp/h-files.txt /tmp/n-files.txt)

# Sample 30 random files, categorize each diff line
echo "=== Sampling diff patterns ==="
declare -A PATTERNS
TOTAL=0
SAMPLED=0

while IFS= read -r f; do
    [ -z "$f" ] && continue
    if cmp -s "$HUGO_DIR/$f" "$HUAN_DIR/$f"; then
        continue
    fi
    SAMPLED=$((SAMPLED + 1))
    if [ "$SAMPLED" -gt 100 ]; then break; fi

    # Get diff lines (only changes), normalize values to find patterns
    while IFS= read -r line; do
        # Skip pure whitespace changes
        case "$line" in
            "---"|"+++"|"") continue;;
        esac
        # Capture first 80 chars to detect pattern
        prefix=$(echo "$line" | head -c 80)
        PATTERNS["$prefix"]=$((PATTERNS["$prefix"] + 1))
        TOTAL=$((TOTAL + 1))
    done < <(diff "$HUGO_DIR/$f" "$HUAN_DIR/$f" 2>/dev/null | head -20 || true)
done <<< "$COMMON"

echo ""
echo "Total diff lines sampled: $TOTAL"
echo "Top patterns:"
for pat in "${!PATTERNS[@]}"; do
    echo "${PATTERNS[$pat]} | $pat"
done | sort -rn | head -20
