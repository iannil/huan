#!/usr/bin/env python3
"""Measure the I/O floor of producing a site's output tree.

Creates N files with the same size distribution as a reference build output
and reports wall time. Use it to decide whether a build-time target is
physically reachable before optimizing CPU work.

Usage:
    python3 scripts/bench-io-floor.py --reference /tmp/huan-bench/r5
    python3 scripts/bench-io-floor.py --reference DIR --root /tmp/probe
"""

import argparse
import os
import random
import shutil
import sys
import time


def collect(reference):
    sizes = []
    for dirpath, _, filenames in os.walk(reference):
        for name in filenames:
            try:
                sizes.append(os.path.getsize(os.path.join(dirpath, name)))
            except OSError:
                pass
    return sizes


def run(sizes, root, rounds):
    if os.path.exists(root):
        shutil.rmtree(root)
    payload = b"x" * (max(sizes) if sizes else 1)
    results = []
    for _ in range(rounds):
        if os.path.exists(root):
            shutil.rmtree(root)
        os.makedirs(root)
        start = time.time()
        for i, size in enumerate(sizes):
            d = os.path.join(root, "d%d" % (i % 900))
            os.makedirs(d, exist_ok=True)
            with open(os.path.join(d, "f%d" % i), "wb") as f:
                f.write(payload[:size])
        elapsed = time.time() - start
        results.append(elapsed)
        print(f"round {len(results)}: {elapsed:.3f}s for {len(sizes)} files")
    shutil.rmtree(root, ignore_errors=True)
    results.sort()
    return results[len(results) // 2]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--reference", required=True,
                    help="reference output tree (e.g. a previous build)")
    ap.add_argument("--root", default="/tmp/huan-io-floor")
    ap.add_argument("--rounds", type=int, default=3)
    args = ap.parse_args()

    sizes = collect(args.reference)
    if not sizes:
        print("no files found in reference", file=sys.stderr)
        return 1
    total = sum(sizes)
    print(f"reference: {len(sizes)} files, {total / 1e6:.1f} MB")
    median = run(sizes, args.root, args.rounds)
    print(f"median write-only floor: {median:.3f}s "
          f"({median / len(sizes) * 1000:.3f} ms/file)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
