#!/usr/bin/env python3
"""Alternate two huan binaries on a read-only site; retain logs and output hashes."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import statistics
import subprocess
import tempfile
import time


def load_known_instability(paths):
    """Read exact relative paths from earlier baseline observations, never globs."""
    known = set()
    for path in paths:
        artifact = json.loads(path.read_text())
        report = artifact.get('build', artifact)
        entries = report.get('baseline_unstable')
        if not isinstance(entries, list):
            raise ValueError(f'{path}: expected baseline_unstable array')
        for entry in entries:
            if (not isinstance(entry, str) or not entry or
                    Path(entry).is_absolute() or '..' in Path(entry).parts or
                    any(char in entry for char in '*?[]\\') or
                    str(Path(entry)) != entry):
                raise ValueError(f'{path}: invalid exact output path {entry!r}')
            known.add(entry)
    return known


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--baseline', type=Path, required=True)
    parser.add_argument('--candidate', type=Path, required=True)
    parser.add_argument('--baseline-plugins', type=Path, required=True)
    parser.add_argument('--candidate-plugins', type=Path, required=True)
    parser.add_argument('--rounds', type=int, default=3, help='measured rounds after one warmup')
    parser.add_argument('--known-instability', type=Path, action='append', default=[],
                        help='prior results JSON with baseline_unstable (optionally under build); exact paths only, repeatable')
    args = parser.parse_args()
    if args.rounds < 1:
        parser.error('--rounds must be positive')
    try:
        known = load_known_instability(args.known_instability)
    except (OSError, ValueError, AttributeError) as exc:
        parser.error(str(exc))
    root = Path(tempfile.mkdtemp(prefix='huan-build-bench-'))
    print(f'Artifacts: {root}', flush=True)
    env = dict(os.environ, HUAN_HOME=str(root / 'empty-home'))
    records, manifests = [], {}
    for iteration in range(args.rounds + 1):
        for name, binary, plugins in [('baseline', args.baseline, args.baseline_plugins),
                                      ('candidate', args.candidate, args.candidate_plugins)]:
            output = root / name
            start = time.perf_counter()
            result = subprocess.run([str(binary.resolve()), 'build', '-s', str(args.source.resolve()),
                                     '-d', str(output), '--plugins', str(plugins.resolve()), '--timings'],
                                    env=env, capture_output=True, text=True)
            elapsed = time.perf_counter() - start
            log = result.stdout + result.stderr
            (root / f'{name}-{iteration}.log').write_text(log)
            if result.returncode or re.search(r'WARN|warning:|error:', log):
                raise RuntimeError(f'{name} build has errors/warnings; see {root}/{name}-{iteration}.log')
            record = dict(variant=name, round=iteration, warmup=iteration == 0, seconds=elapsed)
            records.append(record)
            print(json.dumps(record), flush=True)
            manifests[name, iteration] = {
                str(p.relative_to(output)): hashlib.sha256(p.read_bytes()).hexdigest()
                for p in output.rglob('*') if p.is_file()
            }
            # Outputs are overwritten next round; retain every observed hash.
            (root / f'{name}-{iteration}-manifest.json').write_text(
                json.dumps(manifests[name, iteration], indent=2, sort_keys=True))
    base = manifests['baseline', 0]
    def different(a, b):
        return {p for p in a.keys() | b.keys() if a.get(p) != b.get(p)}
    unstable = set().union(*(different(base, m) for (v, _), m in manifests.items() if v == 'baseline'))
    differences = set().union(*(different(base, m) for (v, _), m in manifests.items() if v == 'candidate'))
    membership_changes = set().union(*(base.keys() ^ m.keys() for m in manifests.values()))
    # An allowed path is not proof that every candidate value is legitimate.
    # Surface values not observed in this run's baseline for separate review.
    unobserved_values = {}
    for path in sorted(known | unstable):
        baseline_values = {m[path] for (v, _), m in manifests.items() if v == 'baseline' and path in m}
        candidate_values = {m[path] for (v, _), m in manifests.items() if v == 'candidate' and path in m}
        if candidate_values - baseline_values:
            unobserved_values[path] = sorted(candidate_values - baseline_values)
    summary = dict(records=records, files=len(base), baseline_unstable=sorted(unstable),
                   known_baseline_unstable=sorted(known),
                   known_instability_artifacts=[str(p.resolve()) for p in args.known_instability],
                   candidate_differences=sorted(differences),
                   missing_or_extra_files=sorted(membership_changes),
                   unobserved_candidate_values=unobserved_values,
                   unexpected_differences=sorted((differences-unstable-known) | membership_changes),
                   median_seconds={v: statistics.median(r['seconds'] for r in records
                                                       if r['variant'] == v and not r['warmup'])
                                   for v in ('baseline', 'candidate')})
    (root / 'results.json').write_text(json.dumps(summary, indent=2))
    print(json.dumps(summary, indent=2))
    if summary['unexpected_differences']:
        raise SystemExit('Unexpected output differences; review artifacts.')


if __name__ == '__main__':
    main()
