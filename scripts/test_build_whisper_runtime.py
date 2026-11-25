"""Regression checks for logging and recovering expensive runtime builds."""

import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
import zipfile


SPEC = importlib.util.spec_from_file_location(
    'runtime_builder', Path(__file__).with_name('build-whisper-runtime.py'))
BUILDER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(BUILDER)


class RuntimeBuildTests(unittest.TestCase):
    """Test failure paths without compiling CUDA or downloading any models."""

    def test_failed_command_retains_log(self):
        with tempfile.TemporaryDirectory() as folder, patch.object(BUILDER, 'LOGS', Path(folder)):
            with self.assertRaises(subprocess.CalledProcessError):
                BUILDER.run([sys.executable, '-c', 'print("compiler diagnostic"); exit(7)'],
                            'failure', timeout=10)
            self.assertIn('compiler diagnostic', (Path(folder) / 'failure.log').read_text())

    def test_timeout_retains_partial_log(self):
        with tempfile.TemporaryDirectory() as folder, patch.object(BUILDER, 'LOGS', Path(folder)):
            with self.assertRaises(subprocess.TimeoutExpired):
                BUILDER.run([sys.executable, '-c',
                             'import time; print("started compiling", flush=True); time.sleep(60)'],
                            'timeout', timeout=1)
            self.assertIn('started compiling', (Path(folder) / 'timeout.log').read_text())

    def test_windows_environment_is_case_insensitive(self):
        class ReachedVersionCheck(Exception):
            pass

        def inspect(args, name, *, cwd, env):
            self.assertEqual(name, 'version')
            self.assertEqual(env['PATH'], os.pathsep.join(('C:/Windows/System32', 'C:/Windows')))
            self.assertNotIn('GGML_BACKEND_PATH', env)
            self.assertNotIn('Path', env)
            raise ReachedVersionCheck()

        for key in ('SystemRoot', 'SYSTEMROOT', 'systemroot'):
            with self.subTest(key=key), patch.dict(BUILDER.os.environ,
                    {key: 'C:/Windows', 'Path': 'compiler;cuda', 'GGML_BACKEND_PATH': 'build'},
                    clear=True), patch.object(BUILDER, 'run', inspect):
                with self.assertRaises(ReachedVersionCheck):
                    BUILDER.smoke_test(Path('candidate/bin/whisper-cli.exe'), '1.9.6', 'windows-x64-cuda')

    def test_candidate_recovery_checks_metadata_and_validation(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            package, dist = root / 'candidate', root / 'dist'
            (package / 'bin').mkdir(parents=True)
            dist.mkdir()
            (package / 'bin/whisper-cli.exe').write_bytes(b'fixture')
            metadata = {'version': '1.9.6', 'commit': 'pinned', 'target': 'windows-x64-cuda',
                        'validation': 'pending'}
            info = package / 'BUILD-INFO.json'
            info.write_text(json.dumps(metadata))
            with patch.object(BUILDER, 'PACKAGE', package), patch.object(BUILDER, 'DIST', dist), \
                 patch.object(BUILDER, 'verify_dependencies'), patch.object(BUILDER, 'smoke_test') as smoke, \
                 patch.dict(BUILDER.os.environ, {'GITHUB_STEP_SUMMARY': ''}):
                with self.assertRaisesRegex(RuntimeError, 'commit does not match'):
                    BUILDER.verify_package('1.9.6', 'different-source', 'windows-x64-cuda')
                smoke.assert_not_called()
                smoke.side_effect = RuntimeError('failed validation')
                with self.assertRaisesRegex(RuntimeError, 'failed validation'):
                    BUILDER.verify_package('1.9.6', 'pinned', 'windows-x64-cuda')
                self.assertEqual(list(dist.iterdir()), [])
                self.assertEqual(json.loads(info.read_text())['validation'], 'pending')
                smoke.side_effect = None
                BUILDER.verify_package('1.9.6', 'pinned', 'windows-x64-cuda')
            archive = dist / 'whisper-1.9.6-windows-x64-cuda.zip'
            with zipfile.ZipFile(archive) as output:
                saved = json.loads(output.read('BUILD-INFO.json'))
                self.assertNotEqual(saved['validation'], 'pending')
            self.assertTrue(archive.with_suffix('.zip.sha256').is_file())


if __name__ == '__main__':
    unittest.main()
