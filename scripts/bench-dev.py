#!/usr/bin/env python3
"""Measure dev save-to-HTTP latency on a temporary copy; never edit the source site.

Requires two built huan executables and their matching plugins. The selected
loopback port must be free. Uses the default debounce and checks fresh HTML.
"""
import argparse
import json
import os
import pathlib
import queue
import re
import shutil
import signal
import statistics
import subprocess
import tempfile
import threading
import time
import urllib.request


parser = argparse.ArgumentParser(description=__doc__)
for name in ['source', 'baseline', 'candidate', 'baseline-plugins', 'candidate-plugins']:
    parser.add_argument('--' + name, type=pathlib.Path, required=True)
parser.add_argument('--rounds', type=int, default=3, help='measured edits per session, after one warmup')
parser.add_argument('--port', type=int, default=18573)
args = parser.parse_args()
if args.rounds < 1 or not 1 <= args.port <= 65535:
    parser.error('positive rounds and a valid port are required')
root = pathlib.Path(tempfile.mkdtemp(prefix='huan-dev-bench-'))
print(f'Artifacts: {root}', flush=True)
site = root / 'dev-site'
shutil.copytree(args.source.resolve(), site, ignore=shutil.ignore_patterns('.git'))
binaries = {'baseline': args.baseline.resolve(), 'optimized': args.candidate.resolve()}
plugins = {'baseline': args.baseline_plugins.resolve(), 'optimized': args.candidate_plugins.resolve()}
probe = site / 'content/posts/perf-probe.md'
if probe.exists():
    raise RuntimeError('probe path already exists in source; choose another fixture')
probe.parent.mkdir(parents=True, exist_ok=True)
records = []
for (session, variant) in enumerate(['baseline', 'optimized', 'baseline', 'optimized']):
    initial = 'HUANPERFPROBEINITIAL'
    prefix = '---\ntitle: Performance probe\ndate: 2026-09-23\nslug: perf-probe\n---\n'
    probe.write_text(prefix + initial)
    lines = queue.Queue()
    log = []
    env = dict(os.environ, HUAN_HOME=str(root / 'empty-home'), HUAN_ADMIN_TOKEN='benchmark-local-token')
    proc = subprocess.Popen([str(binaries[variant]), 'dev', '-s', str(site), '--plugins', str(plugins[variant]), '--port', str(args.port), '--timings'], env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, bufsize=1)

    def read():
        for line in proc.stdout:
            log.append(line)
            lines.put(line)
        lines.put(None)
    reader = threading.Thread(target=read, daemon=True)
    reader.start()

    def until(marker):
        deadline = time.monotonic() + 90
        while time.monotonic() < deadline:
            line = lines.get(timeout=max(0.01, deadline - time.monotonic()))
            if line is None:
                raise RuntimeError(f'{variant} exited {proc.poll()}; inspect retained log')
            if marker in line:
                return line
        raise RuntimeError('timeout')
    try:
        until('Serving at:')
        text = ''.join(log)
        if re.search('plugin load warning|theme activate|WARN', text):
            raise RuntimeError('build warnings')
        output = pathlib.Path(re.search('Output:\\s+(\\S+)', text)[1])
        html = next((p for p in output.rglob('index.html') if 'perf-probe' in str(p) and initial in p.read_text()))
        url = f'http://127.0.0.1:{args.port}/' + str(html.parent.relative_to(output)) + '/'
        for n in range(args.rounds + 1):
            marker = f'HUANPERFPROBE{variant.upper()}{n}'
            start = time.perf_counter()
            probe.write_text(prefix + marker)
            line = until('[watch] rebuild complete in')
            with urllib.request.urlopen(url, timeout=10) as response:
                assert marker in response.read().decode()
            records.append(dict(variant=variant, session=session, round=n, warmup=n == 0, save_to_http_seconds=time.perf_counter() - start, rebuild=line.strip()))
            print(json.dumps(records[-1]), flush=True)
    finally:
        if proc.poll() is None:
            proc.send_signal(signal.SIGINT)
            try:
                proc.wait(timeout=30)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait()
        reader.join(timeout=10)
        (root / f'{variant}-dev-{session}.log').write_text(''.join(log))
summary = dict(records=records, source=str(args.source.resolve()), port=args.port, median_seconds={v: statistics.median((r['save_to_http_seconds'] for r in records if r['variant'] == v and (not r['warmup']))) for v in ['baseline', 'optimized']})
(root / 'dev-results.json').write_text(json.dumps(summary, indent=2))
print(json.dumps(summary, indent=2))
