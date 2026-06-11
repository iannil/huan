#!/bin/bash
# diff-summary.sh - Generates a structured summary of huan vs Hugo differences.
set -euo pipefail

HUGO_DIR="/tmp/hugo-baseline"
HUAN_DIR="/tmp/huan-output"

# Refresh huan output
rm -rf /Users/rong.zhu/Code/zhurongshuo/docs "$HUAN_DIR"
/Users/rong.zhu/Code/huan/huan build -s /Users/rong.zhu/Code/zhurongshuo > /dev/null 2>&1
cp -r /Users/rong.zhu/Code/zhurongshuo/docs "$HUAN_DIR"

cd "$HUGO_DIR" && find . -type f | sort > /tmp/h-files.txt
cd "$HUAN_DIR" && find . -type f | sort > /tmp/n-files.txt

ONLY_HUGO=$(comm -23 /tmp/h-files.txt /tmp/n-files.txt)
ONLY_HUAN=$(comm -13 /tmp/h-files.txt /tmp/n-files.txt)
COMMON=$(comm -12 /tmp/h-files.txt /tmp/n-files.txt)

echo "=== File counts ==="
echo "Hugo total: $(wc -l < /tmp/h-files.txt)"
echo "huan total: $(wc -l < /tmp/n-files.txt)"
echo "Common:     $(echo "$COMMON" | grep -c . || true)"
echo "Only Hugo:  $(echo "$ONLY_HUGO" | grep -c . || true)"
echo "Only huan:  $(echo "$ONLY_HUAN" | grep -c . || true)"

echo ""
echo "=== Only in Hugo (by directory) ==="
echo "$ONLY_HUGO" | awk -F/ '{print $2}' | sort | uniq -c | sort -rn | head -10

echo ""
echo "=== Only in huan (by directory) ==="
echo "$ONLY_HUAN" | awk -F/ '{print $2}' | sort | uniq -c | sort -rn | head -10

echo ""
echo "=== Content diff (common files) ==="
DIFF=0
IDENTICAL=0
while IFS= read -r f; do
    [ -z "$f" ] && continue
    if cmp -s "$HUGO_DIR/$f" "$HUAN_DIR/$f"; then
        IDENTICAL=$((IDENTICAL + 1))
    else
        DIFF=$((DIFF + 1))
    fi
done <<< "$COMMON"
echo "Identical: $IDENTICAL"
echo "Different: $DIFF"

echo ""
echo "=== Sample diff (first 3 differing files, 5 lines each) ==="
SHOWN=0
while IFS= read -r f; do
    [ -z "$f" ] && continue
    if ! cmp -s "$HUGO_DIR/$f" "$HUAN_DIR/$f"; then
        echo ""
        echo "--- $f ---"
        diff "$HUGO_DIR/$f" "$HUAN_DIR/$f" | head -5
        SHOWN=$((SHOWN + 1))
        if [ "$SHOWN" -ge 3 ]; then
            break
        fi
    fi
done <<< "$COMMON"
