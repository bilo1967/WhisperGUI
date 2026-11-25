"""Regression checks for logging and recovering expensive runtime builds."""

import importlib.util
from contextlib import contextmanager, ExitStack
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

    @contextmanager
    def windows_build_fixture(self, mode='--build-only'):
        """Model the complete Windows staging flow with real fixture files."""
        with tempfile.TemporaryDirectory() as folder, ExitStack() as stack:
            root = Path(folder)
            for name, child in (('ROOT', ''), ('SOURCE', 'source'), ('BUILD', 'build'),
                                ('PACKAGE', 'package'), ('LOGS', 'logs'), ('DIST', 'dist')):
                stack.enter_context(patch.object(BUILDER, name, root / child))
            files = {
                'source/LICENSE': 'MIT fixture',
                'source/samples/jfk.wav': 'audio fixture',
                'source/examples/cli/cli.cpp': '\n'.join(f'"{flag}"' for flag in BUILDER.REQUIRED_CLI_FLAGS),
                'programs/Microsoft Visual Studio/Installer/vswhere.exe': 'fixture',
                'vs/VC/Redist/MSVC/14.38.1/x64/Microsoft.VC143.CRT/vcruntime140.dll': 'fixture',
                'vs/VC/Redist/MSVC/14.38.1/x64/Microsoft.VC143.CRT/vcruntime140_1.dll': 'fixture',
                'vs/VC/Redist/MSVC/14.38.1/x64/Microsoft.VC143.CRT/msvcp140.dll': 'fixture',
                'vs/VC/Redist/MSVC/14.38.1/x64/Microsoft.VC143.OpenMP/vcomp140.dll': 'fixture',
                'cuda/bin/cudart64_12.dll': 'fixture',
                'cuda/bin/cublas64_12.dll': 'fixture',
                'cuda/bin/cublasLt64_12.dll': 'fixture',
            }
            for relative, content in files.items():
                path = root / relative
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(content, encoding='utf-8')
            generated = ('whisper-cli.exe', 'whisper.dll', 'ggml.dll', 'ggml-base.dll',
                         'ggml-cpu-x64.dll', 'ggml-cuda.dll')
            output = root / 'build/bin/Release'
            output.mkdir(parents=True)
            def generate():
                for name in generated:
                    (output / name).write_bytes(b'compiled fixture')
            if mode == '--stage-only':
                generate()
            events = []
            def command(args, name, **kwargs):
                events.append(name)
                if name == 'source':
                    return 'pinned\n'
                if name == 'vswhere':
                    return str(root / 'vs')
                if name == 'cmake-version':
                    return 'cmake fixture'
                if name in ('configure', 'build'):
                    # Packaging and network inputs must already be present.
                    self.assertTrue((root / 'package/LICENSE-CUDA.html').is_file())
                    self.assertTrue((root / 'package/bin/vcomp140.dll').is_file())
                    self.assertTrue((root / 'smoke-test/ggml-tiny.en.bin').is_file())
                    self.assertTrue((root / 'smoke-test/ggml-silero-v6.2.0.bin').is_file())
                if name == 'build':
                    generate()
                return ''
            def fetch(url, destination, minimum_size=100_000):
                events.append('license' if 'eula' in url else 'model')
                if 'eula' in url:
                    destination.write_text('<html>NVIDIA CUDA license agreement supplement</html>', encoding='utf-8')
                else:
                    destination.write_bytes(b'x' * minimum_size)
            stack.enter_context(patch.dict(BUILDER.os.environ, {
                'RUNTIME_TARGET': 'windows-x64-cuda', 'WHISPER_VERSION': '1.9.6',
                'WHISPER_COMMIT': 'pinned', 'CUDA_VERSION': '12.4.1',
                'CUDA_PATH': str(root / 'cuda'), 'ProgramFiles(x86)': str(root / 'programs'),
                'SYSTEMROOT': str(root / 'Windows'), 'RECOVERED_BUILD_RUN': '123',
                'RECOVERED_BUILD_HEAD': 'original-build-head',
            }))
            stack.enter_context(patch.object(BUILDER.platform, 'machine', return_value='AMD64'))
            stack.enter_context(patch.object(BUILDER.shutil, 'which', return_value='fixture-tool'))
            stack.enter_context(patch.object(BUILDER, 'run', side_effect=command))
            stack.enter_context(patch.object(BUILDER, 'download', side_effect=fetch))
            fixture_hash = BUILDER.hashlib.sha256(b'x' * 100_000).hexdigest()
            stack.enter_context(patch.object(BUILDER, 'SMOKE_MODELS',
                tuple((name, url, fixture_hash) for name, url, digest in BUILDER.SMOKE_MODELS)))
            stack.enter_context(patch.object(sys, 'argv', ['builder', mode]))
            yield root, events

    def test_full_windows_staging_checks_inputs_before_compilation(self):
        with self.windows_build_fixture() as (root, events):
            BUILDER.main()
            self.assertLess(events.index('license'), events.index('configure'))
            self.assertLess(events.index('model'), events.index('configure'))
            self.assertTrue((root / 'package/bin/ggml-cuda.dll').is_file())
            metadata = json.loads((root / 'package/BUILD-INFO.json').read_text())
            self.assertEqual(metadata['validation'], 'pending')
            self.assertEqual(list((root / 'dist').iterdir()), [])

    def test_missing_cuda_dll_fails_before_compilation(self):
        with self.windows_build_fixture() as (root, events):
            (root / 'cuda/bin/cublasLt64_12.dll').unlink()
            with self.assertRaisesRegex(RuntimeError, 'cublasLt64_12.dll'):
                BUILDER.main()
            self.assertNotIn('configure', events)
            self.assertNotIn('build', events)

    def test_empty_msvc_redist_fails_before_compilation(self):
        with self.windows_build_fixture() as (root, events):
            (root / 'vs/VC/Redist/MSVC/14.38.1/x64/Microsoft.VC143.OpenMP/vcomp140.dll').unlink()
            with self.assertRaisesRegex(RuntimeError, 'contains no DLLs'):
                BUILDER.main()
            self.assertNotIn('build', events)

    def test_license_download_failure_prevents_compilation(self):
        with self.windows_build_fixture() as (root, events), \
             patch.object(BUILDER, 'download', side_effect=OSError('license unavailable')):
            with self.assertRaisesRegex(OSError, 'license unavailable'):
                BUILDER.main()
            self.assertNotIn('build', events)
            self.assertFalse((root / 'package/BUILD-INFO.json').exists())

    def test_local_cuda_license_needs_no_network(self):
        with self.windows_build_fixture() as (root, events):
            (root / 'package').mkdir()
            (root / 'cuda/EULA.txt').write_text('installed CUDA agreement', encoding='utf-8')
            BUILDER.prepare_cuda_license(root / 'cuda')
            self.assertEqual((root / 'package/LICENSE-CUDA.txt').read_text(), 'installed CUDA agreement')
            self.assertEqual(events, [])

    def test_cuda_license_fallback_uses_exact_archive_version(self):
        with self.windows_build_fixture() as (root, events):
            (root / 'package').mkdir()
            BUILDER.prepare_cuda_license(root / 'cuda')
            self.assertEqual((root / 'package/LICENSE-CUDA-SOURCE.txt').read_text(),
                             'https://docs.nvidia.com/cuda/archive/12.4.1/eula/index.html\n')

    def test_rejects_unexpected_license_document(self):
        with self.windows_build_fixture() as (root, events):
            (root / 'package').mkdir()
            def wrong_page(url, destination, **kwargs):
                destination.write_text('<html>Not found</html>', encoding='utf-8')
            with patch.object(BUILDER, 'download', side_effect=wrong_page):
                with self.assertRaisesRegex(RuntimeError, 'Unexpected CUDA license'):
                    BUILDER.prepare_cuda_license(root / 'cuda')
            self.assertFalse((root / 'package/LICENSE-CUDA.html').exists())

    def test_missing_test_model_prevents_compilation(self):
        with self.windows_build_fixture() as (root, events):
            fetch = BUILDER.download.side_effect
            def unavailable(url, destination, **kwargs):
                if 'huggingface.co' in url:
                    raise OSError('model unavailable')
                return fetch(url, destination, **kwargs)
            with patch.object(BUILDER, 'download', side_effect=unavailable):
                with self.assertRaisesRegex(OSError, 'model unavailable'):
                    BUILDER.main()
            self.assertNotIn('build', events)

    def test_smoke_inputs_are_reusable(self):
        with self.windows_build_fixture() as (root, events):
            first = BUILDER.prepare_smoke_inputs()
            BUILDER.prepare_smoke_inputs()
            self.assertEqual(events, ['model', 'model'])
            self.assertTrue((first / 'jfk.wav').is_file())

    def test_corrupt_test_model_prevents_compilation(self):
        with self.windows_build_fixture() as (root, events):
            fetch = BUILDER.download.side_effect
            def corrupt(url, destination, **kwargs):
                fetch(url, destination, **kwargs)
                if 'huggingface.co' in url:
                    destination.write_bytes(b'bad model' * 20000)
            with patch.object(BUILDER, 'download', side_effect=corrupt):
                with self.assertRaisesRegex(RuntimeError, 'checksum does not match'):
                    BUILDER.main()
            self.assertNotIn('build', events)
            self.assertFalse((root / 'smoke-test/ggml-tiny.en.bin').exists())

    def test_missing_required_source_option_prevents_compilation(self):
        with self.windows_build_fixture() as (root, events):
            (root / 'source/examples/cli/cli.cpp').write_text('old CLI', encoding='utf-8')
            with self.assertRaisesRegex(RuntimeError, 'required CLI option'):
                BUILDER.main()
            self.assertNotIn('build', events)

    def test_recovery_stages_existing_binaries_without_compiler(self):
        with self.windows_build_fixture('--stage-only') as (root, events), \
             patch.object(BUILDER.shutil, 'which', return_value=None):
            BUILDER.main()
            self.assertNotIn('configure', events)
            self.assertNotIn('build', events)
            self.assertNotIn('cmake-version', events)
            metadata = json.loads((root / 'package/BUILD-INFO.json').read_text())
            self.assertEqual(metadata['recovered_build_run'], '123')
            self.assertEqual(metadata['recovered_build_head'], 'original-build-head')
            self.assertIsNone(metadata['cmake_options'])
            self.assertEqual(metadata['validation'], 'pending')

    def test_incomplete_recovered_binaries_are_rejected(self):
        with self.windows_build_fixture('--stage-only') as (root, events):
            (root / 'build/bin/Release/ggml-cuda.dll').unlink()
            with self.assertRaisesRegex(RuntimeError, 'ggml-cuda.dll'):
                BUILDER.main()
            self.assertFalse((root / 'package/BUILD-INFO.json').exists())
            self.assertNotIn('build', events)

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
            # Compare paths using the host's conventions (backslashes on Windows).
            expected_path = os.pathsep.join(str(Path(entry)) for entry in
                                            ('C:/Windows/System32', 'C:/Windows'))
            self.assertEqual(env['PATH'], expected_path)
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
