# Building whisper.cpp runtimes

The **Build whisper.cpp runtimes** workflow creates our own runtime packages from
whisper.cpp **v1.9.6**, fixed at commit
`96ec4d9823e9d0211944172da35f9f29134af990`. It does not follow upstream `master`
or search upstream Actions for the latest successful run.

## Running and downloading

1. Open **Actions → Build whisper.cpp runtimes → Run workflow** on `main`.
2. Leave **Keep all verified packages in a dedicated runtime release** enabled
   to preserve the packages beyond Actions artifact expiry.
3. After all five builds succeed, download from **Releases**. The release is named
   `whisper.cpp 1.9.6 runtimes (build N)`, with tag
   `whisper-runtime-v1.9.6-build-N`. It is marked as a prerelease because the
   packages still need GPU and end-user platform validation.

Each build also uploads its package to the run's **Artifacts**, retained for
90 days. Diagnostics are retained for 14 days, including when a build fails.
Changes to the workflow or build script automatically trigger builds on `main`;
these runs do not publish releases. Manual runs publish only if requested and
only after **all five targets** succeed. A failed target does not prevent other
targets from completing and uploading their verified artifacts.

| Package target | Runner | Backend and compatibility |
| --- | --- | --- |
| `windows-x64-cuda` | Windows Server 2022 | Automatically selected CPU variants plus CUDA 12.4.1; also runs without a GPU |
| `macos-x64-metal` | macOS 15 Intel | Intel CPU and embedded Metal; macOS 13+ |
| `macos-arm64-metal` | macOS 15 ARM64 | Apple Silicon CPU and embedded Metal; macOS 13+ |
| `linux-x64-cpu` | Ubuntu 22.04 x64 | CPU, AVX2-capable x86-64 processor |
| `linux-arm64-cpu` | Ubuntu 22.04 ARM64 | CPU, native ARM64 build |

The Windows package includes matching whisper/ggml DLLs, CPU backend DLLs, the
MSVC CRT/OpenMP redistributable DLLs, the CUDA runtime and cuBLAS libraries.
The same package runs on CPU when no compatible GPU is available; a separate
Windows CPU build is unnecessary. GPU acceleration requires a compatible NVIDIA
driver on the destination machine; installing the CUDA toolkit there is
unnecessary.
CUDA GPU architecture selection follows the pinned upstream CMake defaults,
including PTX where upstream uses it. CUDA 12.4 does not include native Blackwell
kernels; this is not a validated package for RTX 50-series GPUs.

CUDA 12.4 requires an older MSVC compiler than the default on current runners.
The workflow installs MSVC 14.38 and selects it explicitly, rather than disabling
NVIDIA's compiler compatibility check.

Unix builds link whisper/ggml statically. macOS packages require only system
frameworks/libraries and include the Metal shader library in the executable.
Linux packages still depend on the system glibc/libstdc++ runtime; they are built
on Ubuntu 22.04 and are not intended for older distributions such as Ubuntu 20.04
or CentOS 6. OpenMP is disabled for Unix packages to avoid a separate libomp/libgomp
dependency; ggml's own CPU threading remains available.

## Package contents and checks

ZIP files contain `bin/whisper-cli` (or `.exe`), supporting Windows DLLs,
license notices and `BUILD-INFO.json`. They contain **no models, FFmpeg or
WhisperGUI executable**. Keep the complete `bin/` contents together.
For manual use, copy them into WhisperGUI's `bin/` directory for the matching
platform, replacing the previous whisper.cpp runtime as a complete set.

The script tests the staged executable outside the build directory, removes
library-path overrides, and on Windows removes the compiler and CUDA toolkit
from the test process's `PATH`. It verifies the version, required CLI options,
and two actual CPU transcriptions of upstream `samples/jfk.wav` with `tiny.en`:

- DTW word timestamps with full JSON.
- Silero VAD 6.2.0.

Both runs check TXT, SRT, VTT and JSON output, including a basic recognition
check. The DTW run also checks that full JSON contains tokens. The test models
are downloaded from Hugging Face for CI only and are excluded from the archives.
Unix library dependencies and macOS binary architecture are checked as well.

CPU fallback is tested in CUDA and Metal packages too. **GPU inference is not
tested on the hosted runners**; local GPU validation is still required.

Each ZIP has a SHA256 sidecar; permanent runtime releases include a combined
`SHA256SUMS.txt`. Build metadata records the source commit, CMake version/options,
runner, workflow URL and validation performed. Compiler configuration and full
test logs are available in the diagnostic artifacts.

## Updating

Update `WHISPER_VERSION`, `WHISPER_COMMIT` and the workflow run name together.
Use the exact upstream release commit, not a moving branch. For CUDA updates,
also review the selected MSVC toolset and packaged NVIDIA DLLs/licenses.
Run all targets and validate the relevant GPUs before using the new packages
in an application release.

This workflow is independent of WhisperGUI's application packaging. The existing
Windows and macOS packaging workflows continue to use their current runtime
sources until explicitly switched over. The older macOS-only
`build_whisper_cli_for_mac.yml` workflow is retained for now; use this new
workflow for the complete 1.9.6 runtime set.
