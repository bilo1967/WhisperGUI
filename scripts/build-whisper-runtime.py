#!/usr/bin/env python3
"""Build the pinned whisper.cpp CLI and test its relocated runtime package."""

import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import signal
import subprocess
import sys
import threading
import time
import urllib.request
import zipfile


ROOT = Path.cwd()
SOURCE = ROOT / 'whisper-src'
BUILD = ROOT / 'whisper-build'
PACKAGE = ROOT / 'runtime-package'
LOGS = ROOT / 'runtime-logs'
DIST = ROOT / 'dist'
REQUIRED_CLI_FLAGS = ('--dtw', '--no-flash-attn', '--vad', '--vad-model', '--prompt',
                      '--carry-initial-prompt', '--temperature', '--no-gpu',
                      '--output-json-full', '--output-txt', '--output-srt', '--output-vtt')
# Published SHA256 values from the upstream Hugging Face model file pages.
SMOKE_MODELS = (
    ('ggml-tiny.en.bin', 'https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en.bin',
     '921e4cf8686fdd993dcd081a5da5b6c365bfde1162e72b08d75ac75289920b1f'),
    ('ggml-silero-v6.2.0.bin', 'https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v6.2.0.bin',
     '2aa269b785eeb53a82983a20501ddf7c1d9c48e33ab63a41391ac6c9f7fb6987'))


def run(args, name, *, cwd=ROOT, env=None, timeout=600):
    """Stream diagnostics to disk/console, retaining output even on timeout."""
    print('+', ' '.join(map(str, args)), flush=True)
    lines = []
    with (LOGS / (name + '.log')).open('w', encoding='utf-8', buffering=1) as log:
        process = subprocess.Popen(list(map(str, args)), cwd=cwd, env=env,
                                   stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                   text=True, encoding='utf-8', errors='replace',
                                   start_new_session=os.name != 'nt')
        def copy_output():
            for line in process.stdout:
                log.write(line)
                print(line, end='', flush=True)
                lines.append(line)
        reader = threading.Thread(target=copy_output, daemon=True)
        reader.start()
        try:
            process.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            if os.name == 'nt':
                subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                               timeout=30, check=False)
            else:
                os.killpg(process.pid, signal.SIGKILL)
            process.wait()
            raise
        finally:
            reader.join()
            process.stdout.close()
    if process.returncode:
        raise subprocess.CalledProcessError(process.returncode, args)
    return ''.join(lines)


def download(url, destination, minimum_size=100_000):
    """Download required inputs with retries before starting an expensive build."""
    for attempt in range(3):
        try:
            with urllib.request.urlopen(url, timeout=120) as response:
                with destination.open('wb') as output:
                    shutil.copyfileobj(response, output)
            if destination.stat().st_size < minimum_size:
                raise RuntimeError(f'Download is unexpectedly small: {url}')
            return
        except (OSError, RuntimeError):
            if attempt == 2:
                raise
            time.sleep(3)


def require_files(paths):
    """Report all missing or empty packaging inputs in one early failure."""
    missing = [str(path) for path in paths if not path.is_file() or path.stat().st_size == 0]
    if missing:
        raise RuntimeError('Missing or empty required files:\n' + '\n'.join(missing))


def prepare_windows_dependencies(cuda):
    """Resolve and stage external DLLs and licenses before CMake compilation."""
    vswhere = Path(os.environ['ProgramFiles(x86)']) / 'Microsoft Visual Studio/Installer/vswhere.exe'
    require_files([vswhere])
    installation = run([vswhere, '-latest', '-products', '*', '-property', 'installationPath'],
                       'vswhere').strip()
    if not installation:
        raise RuntimeError('Visual Studio installation not found')
    vs = Path(installation)
    redists = list((vs / 'VC/Redist/MSVC').glob('14.*'))
    redists.sort(key=lambda p: tuple(int(n) for n in p.name.split('.')), reverse=True)
    if not redists:
        raise RuntimeError('MSVC redistributable directory not found')
    dependencies = []
    for family in ('CRT', 'OpenMP'):
        folders = list((redists[0] / 'x64').glob(f'Microsoft.VC*.{family}'))
        if len(folders) != 1:
            raise RuntimeError(f'Expected one MSVC {family} directory, found {folders}')
        dlls = list(folders[0].glob('*.dll'))
        if not dlls:
            raise RuntimeError(f'MSVC {family} directory contains no DLLs: {folders[0]}')
        dependencies.extend(dlls)
    required_names = {'vcruntime140.dll', 'vcruntime140_1.dll', 'msvcp140.dll', 'vcomp140.dll'}
    if not required_names.issubset({dll.name.lower() for dll in dependencies}):
        raise RuntimeError('MSVC redistributables are incomplete: CRT/OpenMP DLLs are missing')
    if cuda:
        cuda_root = Path(os.environ['CUDA_PATH'])
        dependencies.extend(cuda_root / 'bin' / filename for filename in
                            ('cudart64_12.dll', 'cublas64_12.dll', 'cublasLt64_12.dll'))
    require_files(dependencies)
    for dll in dependencies:
        shutil.copy2(dll, PACKAGE / 'bin' / dll.name)
    if cuda:
        prepare_cuda_license(cuda_root)


def prepare_cuda_license(cuda_root):
    """Use the installed EULA or NVIDIA's archived EULA for the exact toolkit."""
    candidates = [cuda_root / 'EULA.txt', cuda_root / 'doc/EULA.txt']
    for license_file in candidates:
        if license_file.is_file() and license_file.stat().st_size > 0:
            shutil.copy2(license_file, PACKAGE / 'LICENSE-CUDA.txt')
            return
    version = os.environ['CUDA_VERSION']
    if not re.fullmatch(r'\d+\.\d+\.\d+', version):
        raise RuntimeError(f'Invalid CUDA version for license download: {version}')
    url = f'https://docs.nvidia.com/cuda/archive/{version}/eula/index.html'
    destination = PACKAGE / 'LICENSE-CUDA.html'
    download(url, destination, minimum_size=1000)
    text = destination.read_text(encoding='utf-8').lower()
    if not all(marker in text for marker in ('nvidia', 'cuda', 'license agreement', 'supplement')):
        destination.unlink()
        raise RuntimeError(f'Unexpected CUDA license document: {url}')
    (PACKAGE / 'LICENSE-CUDA-SOURCE.txt').write_text(url + '\n', encoding='utf-8')


def copy_windows_dependencies(cuda):
    """Stage generated backends; external dependencies were checked before build."""
    bindir = PACKAGE / 'bin'
    for dll in (BUILD / 'bin' / 'Release').glob('*.dll'):
        shutil.copy2(dll, bindir / dll.name)
    required = ['whisper.dll', 'ggml.dll', 'ggml-base.dll', 'ggml-cpu-x64.dll']
    if cuda:
        required.append('ggml-cuda.dll')
    elif (bindir / 'ggml-cuda.dll').exists():
        raise RuntimeError('CPU package unexpectedly contains a CUDA backend')
    require_files([bindir / name for name in required])


def prepare_smoke_inputs():
    """Fetch test data before compilation; reuse it when verifying the package."""
    require_files([SOURCE / 'samples/jfk.wav'])
    testdir = ROOT / 'smoke-test'
    testdir.mkdir(exist_ok=True)
    for filename, url, expected in SMOKE_MODELS:
        destination = testdir / filename
        if not destination.is_file() or sha256(destination) != expected:
            download(url, destination)
        if sha256(destination) != expected:
            destination.unlink()
            raise RuntimeError(f'Smoke-test model checksum does not match: {filename}')
    shutil.copy2(SOURCE / 'samples/jfk.wav', testdir / 'jfk.wav')
    return testdir


def sha256(path):
    """Hash model inputs and published archives without loading them into memory."""
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def preflight(target, stage_only=False):
    """Check packaging inputs, source options and tools before the long build."""
    require_files([SOURCE / 'LICENSE', SOURCE / 'examples/cli/cli.cpp'])
    cli_source = (SOURCE / 'examples/cli/cli.cpp').read_text(encoding='utf-8')
    for flag in REQUIRED_CLI_FLAGS:
        if f'"{flag}"' not in cli_source:
            raise RuntimeError(f'Source does not support required CLI option: {flag}')
    if target.startswith('windows-'):
        env = {key.upper(): value for key, value in os.environ.items()}
        if not env.get('SYSTEMROOT'):
            raise RuntimeError('SYSTEMROOT is missing')
        tools = () if stage_only else ('cmake', 'ninja', 'cl', 'nvcc')
        if target.endswith('-cuda') and not os.environ.get('CUDA_VERSION'):
            raise RuntimeError('CUDA_VERSION is missing')
        prepare_windows_dependencies(target.endswith('-cuda'))
    else:
        tools = ('cmake', 'c++', 'lipo', 'otool') if target.startswith('macos-') else ('cmake', 'c++', 'ldd', 'file')
    missing = [tool for tool in tools if not shutil.which(tool)]
    if missing:
        raise RuntimeError('Required tools are missing from PATH: ' + ', '.join(missing))
    prepare_smoke_inputs()


def verify_dependencies(cli, target):
    """Reject non-system dynamic dependencies in Unix packages."""
    if target.startswith('macos-'):
        archs = run(['lipo', '-archs', cli], 'architecture').strip().split()
        expected = 'arm64' if 'arm64' in target else 'x86_64'
        if archs != [expected]:
            raise RuntimeError(f'Incorrect binary architectures: {archs}')
        libraries = run(['otool', '-L', cli], 'dependencies')
        for line in libraries.splitlines()[1:]:
            dependency = line.strip().split(' (', 1)[0]
            if not dependency.startswith(('/usr/lib/', '/System/Library/')):
                raise RuntimeError(f'Non-system macOS dependency: {dependency}')
    elif target.startswith('linux-'):
        libraries = run(['ldd', cli], 'dependencies')
        allowed = {'libstdc++.so.6', 'libm.so.6', 'libgcc_s.so.1', 'libc.so.6',
                   'libpthread.so.0', 'libdl.so.2', 'librt.so.1'}
        for line in libraries.splitlines():
            if 'not found' in line:
                raise RuntimeError(f'Missing Linux dependency: {line}')
            if '=>' in line and line.split()[0] not in allowed:
                raise RuntimeError(f'Unexpected Linux dependency: {line}')
        run(['file', cli], 'architecture')


def smoke_test(cli, version, target):
    """Exercise staged binaries without build directories or toolkit on PATH."""
    env = os.environ.copy()
    # os.environ is case-insensitive on Windows, but its plain dict copy is not.
    # Python normally exposes Windows keys in uppercase (including SYSTEMROOT).
    if target.startswith('windows-'):
        env = {key.upper(): value for key, value in env.items()}
    for key in ('LD_LIBRARY_PATH', 'DYLD_LIBRARY_PATH', 'GGML_BACKEND_PATH'):
        env.pop(key, None)
    if target.startswith('windows-'):
        system = Path(env['SYSTEMROOT'])
        env['PATH'] = os.pathsep.join(map(str, (system / 'System32', system)))
    reported = run([cli, '--version'], 'version', cwd=cli.parent, env=env)
    if not re.search(r'\b' + re.escape(version) + r'\b', reported):
        raise RuntimeError('Unexpected whisper.cpp version')
    help_text = run([cli, '--help'], 'help', cwd=cli.parent, env=env)
    for flag in REQUIRED_CLI_FLAGS:
        if flag not in help_text:
            raise RuntimeError(f'Required CLI option is missing: {flag}')
    testdir = prepare_smoke_inputs()
    model = testdir / 'ggml-tiny.en.bin'
    vad = testdir / 'ggml-silero-v6.2.0.bin'
    # CPU inference also checks the fallback in CUDA/Metal packages. DTW and VAD
    # run separately, as these are independently selectable WhisperGUI features.
    for mode, options in (('dtw', ['--dtw', 'tiny.en', '--no-flash-attn', '-ojf']),
                          ('vad', ['--vad', '--vad-model', vad, '-oj'])):
        prefix = testdir / mode
        run([cli, '-m', model, '-f', testdir / 'jfk.wav', '-l', 'en', '-ng', '-t', '2',
             '-of', prefix, '-otxt', '-osrt', '-ovtt', '-pp', '--prompt', 'United States.',
             '--carry-initial-prompt', '--temperature', '0', *options],
            'smoke-' + mode, cwd=testdir, env=env)
        for suffix in ('.txt', '.srt', '.vtt', '.json'):
            output = prefix.with_suffix(suffix)
            if not output.is_file() or output.stat().st_size == 0:
                raise RuntimeError(f'Missing smoke-test output: {output}')
            shutil.copy2(output, LOGS / output.name)
        data = json.loads(prefix.with_suffix('.json').read_text(encoding='utf-8'))
        segments = data.get('transcription', [])
        text = ' '.join(s.get('text', '') for s in segments).lower()
        if 'country' not in text:
            raise RuntimeError(f'Smoke-test transcription looks wrong: {text}')
        if mode == 'dtw':
            tokens = [t for s in segments for t in s.get('tokens', [])]
            if not any(isinstance(t.get('t_dtw'), (int, float)) and t['t_dtw'] >= 0
                       for t in tokens):
                raise RuntimeError('Full JSON contains no DTW token timestamps')


def main():
    """Configure one native target and publish only after package verification."""
    target = os.environ['RUNTIME_TARGET']
    version = os.environ['WHISPER_VERSION']
    commit = os.environ['WHISPER_COMMIT']
    windows = target.startswith('windows-')
    macos = target.startswith('macos-')
    cuda = target.endswith('-cuda')
    expected_machine = 'arm64' if 'arm64' in target else 'x86_64'
    actual_machine = platform.machine().lower().replace('amd64', 'x86_64').replace('aarch64', 'arm64')
    if actual_machine != expected_machine:
        raise RuntimeError(f'Runner architecture {actual_machine} does not match {target}')
    for folder in (LOGS, DIST, PACKAGE / 'bin'):
        folder.mkdir(parents=True, exist_ok=True)
    if '--verify-only' in sys.argv:
        verify_package(version, commit, target)
        return
    actual_commit = run(['git', '-C', SOURCE, 'rev-parse', 'HEAD'], 'source').strip()
    if actual_commit != commit:
        raise RuntimeError('Source commit does not match pinned commit')
    stage_only = '--stage-only' in sys.argv
    if stage_only:
        if not windows or not os.environ.get('RECOVERED_BUILD_RUN') or not os.environ.get('RECOVERED_BUILD_HEAD'):
            raise RuntimeError('Staging recovered binaries requires a validated Windows source run')
        require_files([BUILD / 'bin/Release/whisper-cli.exe'])
    print('Checking packaging dependencies, licenses and smoke-test inputs before compilation.', flush=True)
    preflight(target, stage_only=stage_only)
    cmake_version = None if stage_only else run(['cmake', '--version'], 'cmake-version').strip()
    options = ['-DCMAKE_BUILD_TYPE=Release', '-DGGML_NATIVE=OFF',
               '-DWHISPER_BUILD_IS_DEV=OFF', '-DWHISPER_BUILD_TESTS=OFF',
               '-DWHISPER_BUILD_SERVER=OFF', '-DWHISPER_BUILD_EXAMPLES=ON',
               '-DWHISPER_SDL2=OFF', '-DWHISPER_CURL=OFF',
               '-DGGML_CUDA=' + ('ON' if cuda else 'OFF')]
    if windows:
        # Match upstream's CUDA release workflow. vcvarsall selects MSVC 14.38
        # before this script starts; Ninja does not accept Visual Studio -A/-T.
        options += ['-G', 'Ninja Multi-Config',
                    '-DBUILD_SHARED_LIBS=ON', '-DGGML_BACKEND_DL=ON',
                    '-DGGML_CPU_ALL_VARIANTS=ON']
        if cuda:
            options += ['-DCMAKE_CUDA_RUNTIME_LIBRARY=Shared']
        if shutil.which('ccache'):
            options += ['-DCMAKE_C_COMPILER_LAUNCHER=ccache',
                        '-DCMAKE_CXX_COMPILER_LAUNCHER=ccache',
                        '-DCMAKE_CUDA_COMPILER_LAUNCHER=ccache']
    else:
        options += ['-DBUILD_SHARED_LIBS=OFF', '-DGGML_BACKEND_DL=OFF',
                    '-DGGML_OPENMP=OFF', '-DGGML_METAL=' + ('ON' if macos else 'OFF')]
        if macos:
            options += ['-DGGML_METAL_EMBED_LIBRARY=ON', '-DCMAKE_OSX_DEPLOYMENT_TARGET=13.0',
                        '-DCMAKE_OSX_ARCHITECTURES=' + expected_machine]
    # Build ALL on Windows: dynamically loaded CPU/CUDA backends are not linked
    # dependencies of the whisper-cli target, but must be shipped with it.
    jobs = str(os.cpu_count() or 3) if windows else '3'
    command = ['cmake', '--build', BUILD, '--config', 'Release', '--parallel', jobs]
    if not windows:
        command += ['--target', 'whisper-cli']
    if not stage_only:
        run(['cmake', '-S', SOURCE, '-B', BUILD, *options], 'configure')
        run(command, 'build', timeout=12600)
    filename = 'whisper-cli.exe' if windows else 'whisper-cli'
    original = BUILD / 'bin' / ('Release' if windows else '') / filename
    cli = PACKAGE / 'bin' / filename
    shutil.copy2(original, cli)
    if windows:
        copy_windows_dependencies(cuda)
    else:
        cli.chmod(0o755)
    shutil.copy2(SOURCE / 'LICENSE', PACKAGE / 'LICENSE-whisper.cpp.txt')
    notices = ['whisper.cpp and ggml: MIT (LICENSE-whisper.cpp.txt).',
               f'Source: https://github.com/ggml-org/whisper.cpp/tree/{commit}',
               'This package contains no FFmpeg and no recognition models.']
    if windows:
        notices += ['Includes Microsoft Visual C++ runtime redistributables.',
                    'https://visualstudio.microsoft.com/license-terms/']
    if cuda:
        notices += ['Includes NVIDIA CUDA runtime and cuBLAS redistributables.',
                    'See LICENSE-CUDA.txt or LICENSE-CUDA.html. NVIDIA display driver is not included.']
    (PACKAGE / 'THIRD-PARTY-NOTICES.txt').write_text('\n'.join(notices) + '\n', encoding='utf-8')
    metadata = {'version': version, 'commit': commit, 'target': target,
                'cuda_toolkit': os.environ['CUDA_VERSION'] if cuda else None,
                'cmake': cmake_version, 'cmake_options': None if stage_only else options,
                'runner': platform.platform(), 'run_url':
                f"{os.environ.get('GITHUB_SERVER_URL', 'https://github.com')}/{os.environ.get('GITHUB_REPOSITORY', '')}/actions/runs/{os.environ.get('GITHUB_RUN_ID', '')}",
                'validation': 'pending',
                'gpu_inference_tested': False}
    if stage_only:
        metadata['recovered_build_run'] = os.environ['RECOVERED_BUILD_RUN']
        metadata['recovered_build_head'] = os.environ['RECOVERED_BUILD_HEAD']
    (PACKAGE / 'BUILD-INFO.json').write_text(json.dumps(metadata, indent=2) + '\n', encoding='utf-8')
    if '--build-only' not in sys.argv and not stage_only:
        verify_package(version, commit, target)


def verify_package(version, commit, target):
    """Verify an existing candidate, including one recovered from an earlier run."""
    metadata_path = PACKAGE / 'BUILD-INFO.json'
    metadata = json.loads(metadata_path.read_text(encoding='utf-8'))
    for key, expected in (('version', version), ('commit', commit), ('target', target)):
        if metadata.get(key) != expected:
            raise RuntimeError(f'Candidate {key} does not match: {metadata.get(key)} != {expected}')
    filename = 'whisper-cli.exe' if target.startswith('windows-') else 'whisper-cli'
    cli = PACKAGE / 'bin' / filename
    require_files([cli])
    if not target.startswith('windows-'):
        # Artifact upload/download preserves content but resets Unix files to 0644.
        cli.chmod(0o755)
    verify_dependencies(cli, target)
    smoke_test(cli, version, target)
    metadata['validation'] = 'CPU inference: tiny.en, DTW, Silero VAD 6.2.0, TXT/SRT/VTT/JSON'
    metadata['verification_run_url'] = (
        f"{os.environ.get('GITHUB_SERVER_URL', 'https://github.com')}/"
        f"{os.environ.get('GITHUB_REPOSITORY', '')}/actions/runs/{os.environ.get('GITHUB_RUN_ID', '')}")
    metadata_path.write_text(json.dumps(metadata, indent=2) + '\n', encoding='utf-8')
    archive = DIST / f'whisper-{version}-{target}.zip'
    with zipfile.ZipFile(archive, 'w', compression=zipfile.ZIP_DEFLATED) as output:
        for path in sorted(PACKAGE.rglob('*')):
            if path.is_file():
                output.write(path, path.relative_to(PACKAGE))
    digest = sha256(archive)
    archive.with_suffix('.zip.sha256').write_text(f'{digest}  {archive.name}\n', encoding='utf-8')
    summary = os.environ.get('GITHUB_STEP_SUMMARY')
    if summary:
        with open(summary, 'a', encoding='utf-8') as output:
            output.write(f'### {target}\n\nVerified `{archive.name}`; SHA256 `{digest}`.\n\n'
                         'Packaged CLI passed CPU transcription, DTW and VAD checks. '
                         'GPU execution still needs testing on a machine with a supported GPU.\n')


if __name__ == '__main__':
    main()
