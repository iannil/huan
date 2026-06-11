#!/usr/bin/env python3
"""Sample many files and categorize the differences to find common patterns."""
import os
import re
import subprocess
from collections import Counter

HUGO_DIR = "/tmp/hugo-baseline"
HUAN_DIR = "/tmp/huan-output"


def list_files(d):
    out = set()
    for root, _, files in os.walk(d):
        for f in files:
            rel = os.path.relpath(os.path.join(root, f), d)
            out.add(rel)
    return out


def main():
    hugo = list_files(HUGO_DIR)
    huan = list_files(HUAN_DIR)
    common = sorted(hugo & huan)

    # Sample up to 100 differing files
    differing = []
    for f in common:
        h = os.path.join(HUGO_DIR, f)
        n = os.path.join(HUAN_DIR, f)
        if not filecmp_eq(h, n):
            differing.append(f)
        if len(differing) >= 100:
            break

    print(f"Sampled {len(differing)} differing files")

    patterns = Counter()
    by_ext = Counter()
    sample_diffs = {}
    for f in differing:
        ext = os.path.splitext(f)[1] or "noext"
        by_ext[ext] += 1
        # Tokenize each diff line, extract pattern
        diff = run_diff(os.path.join(HUGO_DIR, f), os.path.join(HUAN_DIR, f))
        for line in diff.split("\n"):
            if not line or line.startswith("---") or line.startswith("+++") or line[0] not in "<>":
                continue
            # Strip content values, keep tag structure
            tag = extract_tag_pattern(line[1:])
            if tag:
                patterns[tag] += 1
                if tag not in sample_diffs:
                    sample_diffs[tag] = (f, line[1:80])

    print("\n=== Differences by extension ===")
    for ext, n in by_ext.most_common(10):
        print(f"  {ext or 'none'}: {n}")

    print("\n=== Top diff patterns ===")
    for pat, count in patterns.most_common(15):
        f, sample = sample_diffs.get(pat, ("", ""))
        print(f"  {count:4d} | {pat}")
        print(f"        sample from {f}: {sample}")


def filecmp_eq(a, b):
    with open(a, "rb") as fa, open(b, "rb") as fb:
        return fa.read() == fb.read()


def run_diff(a, b):
    try:
        return subprocess.check_output(["diff", a, b], text=True, errors="replace")
    except subprocess.CalledProcessError as e:
        return e.output


def extract_tag_pattern(line):
    # Extract first HTML tag or significant pattern from a diff line
    m = re.search(r"<(\w+)([^>]*)>", line)
    if m:
        tag = m.group(1)
        attrs = m.group(2)
        # Simplify attribute pattern
        attr_names = re.findall(r"\s([\w-]+)=", attrs)
        return f"<{tag} {' '.join(sorted(attr_names))}>"
    return None


if __name__ == "__main__":
    main()
