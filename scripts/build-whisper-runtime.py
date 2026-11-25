#!/usr/bin/env python3
"""Build the pinned whisper.cpp CLI and test its relocated runtime package."""

import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import time
import urllib.request
import zipfile


ROOT = Path.cwd()
SOURCE = ROOT / 'whisper-src'
BUILD = ROOT / 'whisper-build'
PACKAGE = ROOT / 'runtime-package'
LOGS = ROOT / 'runtime-logs'
DIST = ROOT / 'dist'


def run(args, name, *, cwd=ROOT, env=None):
    """Keep complete command output in diagnostics and propagate failures."""
    print('+', ' '.join(map(str, args)), flush=True)
    result = subprocess.run(list(map(str, args)), cwd=cwd, env=env,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                            text=True, encoding='utf-8', errors='replace', timeout=5400)
    (LOGS / (name + '.log')).write_text(result.stdout, encoding='utf-8')
    print(result.stdout, flush=True)
    result.check_returncode()
    return result.stdout


def download(url, destination):
    """Download smoke-test models with retries; never include them in packages."""
    for attempt in range(3):
        try:
            with urllib.request.urlopen(url, timeout=120) as response:
                with destination.open('wb') as output:
                    shutil.copyfileobj(response, output)
            if destination.stat().st_size < 100_000:
                raise RuntimeError('Downloaded model is unexpectedly small')
            return
        except (OSError, RuntimeError):
            if attempt == 2:
                raise
            time.sleep(3)


def copy_windows_dependencies(cuda):
    """Stage generated DLLs and redistributable MSVC/CUDA runtime libraries."""
    bindir = PACKAGE / 'bin'
    for dll in (BUILD / 'bin' / 'Release').glob('*.dll'):
        shutil.copy2(dll, bindir / dll.name)
    for filename in ('whisper.dll', 'ggml.dll', 'ggml-base.dll', 'ggml-cpu-x64.dll'):
        if not (bindir / filename).is_file():
            raise RuntimeError(f'Missing required DLL: {filename}')
    vswhere = Path(os.environ['ProgramFiles(x86)']) / 'Microsoft Visual Studio/Installer/vswhere.exe'
    vs = Path(subprocess.check_output([str(vswhere), '-latest', '-products', '*',
                                      '-property', 'installationPath'], text=True).strip())
    redists = list((vs / 'VC/Redist/MSVC').glob('14.*'))
    redists.sort(key=lambda p: tuple(int(n) for n in p.name.split('.')), reverse=True)
    if not redists:
        raise RuntimeError('MSVC redistributable directory not found')
    for family in ('CRT', 'OpenMP'):
        folders = list((redists[0] / 'x64').glob(f'Microsoft.VC*.{family}'))
        if len(folders) != 1:
            raise RuntimeError(f'Expected one MSVC {family} directory, found {folders}')
        for dll in folders[0].glob('*.dll'):
            shutil.copy2(dll, bindir / dll.name)
    if cuda:
        cuda_root = Path(os.environ['CUDA_PATH'])
        for filename in ('cudart64_12.dll', 'cublas64_12.dll', 'cublasLt64_12.dll'):
            shutil.copy2(cuda_root / 'bin' / filename, bindir / filename)
        if not (bindir / 'ggml-cuda.dll').is_file():
            raise RuntimeError('CUDA backend was not built')
        license_file = cuda_root / 'EULA.txt'
        if not license_file.is_file():
            raise RuntimeError('CUDA redistribution license is missing')
        shutil.copy2(license_file, PACKAGE / 'LICENSE-CUDA.txt')
    elif (bindir / 'ggml-cuda.dll').exists():
        raise RuntimeError('CPU package unexpectedly contains a CUDA backend')


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
    for key in ('LD_LIBRARY_PATH', 'DYLD_LIBRARY_PATH', 'GGML_BACKEND_PATH'):
        env.pop(key, None)
    if target.startswith('windows-'):
        system = Path(env['SystemRoot'])
        env['PATH'] = os.pathsep.join(map(str, (system / 'System32', system)))
    reported = run([cli, '--version'], 'version', cwd=cli.parent, env=env)
    if not re.search(r'\b' + re.escape(version) + r'\b', reported):
        raise RuntimeError('Unexpected whisper.cpp version')
    help_text = run([cli, '--help'], 'help', cwd=cli.parent, env=env)
    required = ('--dtw', '--no-flash-attn', '--vad', '--vad-model', '--prompt',
                '--carry-initial-prompt', '--temperature', '--no-gpu',
                '--output-json-full', '--output-txt', '--output-srt', '--output-vtt')
    for flag in required:
        if flag not in help_text:
            raise RuntimeError(f'Required CLI option is missing: {flag}')
    testdir = ROOT / 'smoke-test'
    testdir.mkdir()
    model = testdir / 'ggml-tiny.en.bin'
    vad = testdir / 'ggml-silero-v6.2.0.bin'
    download('https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en.bin', model)
    download('https://huggingface.co/ggml-org/whisper-vad/resolve/main/ggml-silero-v6.2.0.bin', vad)
    shutil.copy2(SOURCE / 'samples/jfk.wav', testdir / 'jfk.wav')
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
    actual_commit = run(['git', '-C', SOURCE, 'rev-parse', 'HEAD'], 'source').strip()
    if actual_commit != commit:
        raise RuntimeError('Source commit does not match pinned commit')
    cmake_version = run(['cmake', '--version'], 'cmake-version')
    options = ['-DCMAKE_BUILD_TYPE=Release', '-DGGML_NATIVE=OFF',
               '-DWHISPER_BUILD_IS_DEV=OFF', '-DWHISPER_BUILD_TESTS=OFF',
               '-DWHISPER_BUILD_SERVER=OFF', '-DWHISPER_BUILD_EXAMPLES=ON',
               '-DWHISPER_SDL2=OFF', '-DWHISPER_CURL=OFF',
               '-DGGML_CUDA=' + ('ON' if cuda else 'OFF')]
    if windows:
        options += ['-G', 'Visual Studio 17 2022', '-A', 'x64',
                    '-DBUILD_SHARED_LIBS=ON', '-DGGML_BACKEND_DL=ON',
                    '-DGGML_CPU_ALL_VARIANTS=ON']
        if cuda:
            options += ['-T', 'version=14.38',
                        '-DCMAKE_CUDA_RUNTIME_LIBRARY=Shared']
    else:
        options += ['-DBUILD_SHARED_LIBS=OFF', '-DGGML_BACKEND_DL=OFF',
                    '-DGGML_OPENMP=OFF', '-DGGML_METAL=' + ('ON' if macos else 'OFF')]
        if macos:
            options += ['-DGGML_METAL_EMBED_LIBRARY=ON', '-DCMAKE_OSX_DEPLOYMENT_TARGET=13.0',
                        '-DCMAKE_OSX_ARCHITECTURES=' + expected_machine]
    run(['cmake', '-S', SOURCE, '-B', BUILD, *options], 'configure')
    # Build ALL on Windows: dynamically loaded CPU/CUDA backends are not linked
    # dependencies of the whisper-cli target, but must be shipped with it.
    command = ['cmake', '--build', BUILD, '--config', 'Release', '--parallel', '3']
    if not windows:
        command += ['--target', 'whisper-cli']
    run(command, 'build')
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
                    'See LICENSE-CUDA.txt. NVIDIA display driver is not included.']
    (PACKAGE / 'THIRD-PARTY-NOTICES.txt').write_text('\n'.join(notices) + '\n', encoding='utf-8')
    verify_dependencies(cli, target)
    smoke_test(cli, version, target)
    metadata = {'version': version, 'commit': commit, 'target': target,
                'cuda_toolkit': os.environ['CUDA_VERSION'] if cuda else None,
                'cmake': cmake_version.strip(), 'cmake_options': options,
                'runner': platform.platform(), 'run_url':
                f"{os.environ.get('GITHUB_SERVER_URL', 'https://github.com')}/{os.environ.get('GITHUB_REPOSITORY', '')}/actions/runs/{os.environ.get('GITHUB_RUN_ID', '')}",
                'validation': 'CPU inference: tiny.en, DTW, Silero VAD 6.2.0, TXT/SRT/VTT/JSON',
                'gpu_inference_tested': False}
    (PACKAGE / 'BUILD-INFO.json').write_text(json.dumps(metadata, indent=2) + '\n', encoding='utf-8')
    archive = DIST / f'whisper-{version}-{target}.zip'
    with zipfile.ZipFile(archive, 'w', compression=zipfile.ZIP_DEFLATED) as output:
        for path in sorted(PACKAGE.rglob('*')):
            if path.is_file():
                output.write(path, path.relative_to(PACKAGE))
    with archive.open('rb') as stream:
        digest = hashlib.file_digest(stream, 'sha256').hexdigest()
    archive.with_suffix('.zip.sha256').write_text(f'{digest}  {archive.name}\n', encoding='utf-8')
    summary = os.environ.get('GITHUB_STEP_SUMMARY')
    if summary:
        with open(summary, 'a', encoding='utf-8') as output:
            output.write(f'### {target}\n\nVerified `{archive.name}`; SHA256 `{digest}`.\n\n'
                         'Packaged CLI passed CPU transcription, DTW and VAD checks. '
                         'GPU execution still needs testing on a machine with a supported GPU.\n')


if __name__ == '__main__':
    main()
