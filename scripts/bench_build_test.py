"""Regression checks for retained benchmark evidence and exact allowances."""
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('bench-build.py')
spec = importlib.util.spec_from_file_location('bench_build', SCRIPT)
bench = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bench)


class BenchmarkArtifactsTest(unittest.TestCase):
    def test_known_instability_reads_nested_exact_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            artifact = Path(directory) / 'known.json'
            artifact.write_text(json.dumps({'build': {'baseline_unstable': ['posts/a/index.html']}}))
            self.assertEqual(bench.load_known_instability([artifact]), {'posts/a/index.html'})

    def test_known_instability_rejects_patterns_and_parent_paths(self):
        for path in ['../outside', '/absolute', 'posts/*/index.html']:
            with self.subTest(path=path), tempfile.TemporaryDirectory() as directory:
                artifact = Path(directory) / 'known.json'
                artifact.write_text(json.dumps({'baseline_unstable': [path]}))
                with self.assertRaises(ValueError):
                    bench.load_known_instability([artifact])

    def test_round_manifests_and_exact_allowance(self):
        for mode in ['allowed', 'unexpected', 'missing']:
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                for name in ['baseline', 'candidate']:
                    binary = root / name
                    binary.write_text('#!' + sys.executable + '\n' +
                        'from pathlib import Path\nimport sys\n' +
                        'out=Path(sys.argv[sys.argv.index("-d")+1]);out.mkdir(exist_ok=True)\n' +
                        f'candidate={name == "candidate"!r}\n' +
                        f'mode={mode!r}\n' +
                        'if not (candidate and mode=="missing"):\n' +
                        ' (out/"collision.html").write_text("alternate" if candidate else "original")\n' +
                        '(out/"stable.html").write_text("changed" if candidate and mode=="unexpected" else "stable")\n')
                    binary.chmod(0o700)
                known = root / 'known.json'
                known.write_text(json.dumps({'baseline_unstable': ['collision.html']}))
                result = subprocess.run([sys.executable, str(SCRIPT), '--source', str(root),
                    '--baseline', str(root/'baseline'), '--candidate', str(root/'candidate'),
                    '--baseline-plugins', str(root), '--candidate-plugins', str(root),
                    '--rounds', '1', '--known-instability', str(known)], capture_output=True, text=True,
                    env={**__import__('os').environ, 'TMPDIR': str(root)})
                artifacts = next(root.glob('huan-build-bench-*'))
                report = json.loads((artifacts/'results.json').read_text())
                self.assertEqual(len(list(artifacts.glob('*-manifest.json'))), 4)
                self.assertEqual(report['known_baseline_unstable'], ['collision.html'])
                if mode == 'allowed':
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(report['unexpected_differences'], [])
                elif mode == 'unexpected':
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(report['unexpected_differences'], ['stable.html'])
                else:
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(report['missing_or_extra_files'], ['collision.html'])


if __name__ == '__main__':
    unittest.main()
