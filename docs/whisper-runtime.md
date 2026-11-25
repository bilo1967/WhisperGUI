# Building whisper.cpp runtimes

The **Build whisper.cpp runtimes** workflow creates our own runtime packages from
whisper.cpp **v1.9.6**, fixed at commit
`96ec4d9823e9d0211944172da35f9f29134af990`. It does not follow upstream `master`
or search upstream Actions for the latest successful run.

## Running and downloading

1. Open **Actions → Build whisper.cpp runtimes → Run workflow** on `main`.
2. Leave **Keep all verified packages in a dedicated runtime release** enabled
   to preserve the packages beyond Actions artifact expiry.
3. Download verified packages from **Releases**. The release is named
   `whisper.cpp 1.9.6 runtimes (build N)`, with tag
   `whisper-runtime-v1.9.6-build-N`. It is marked as a prerelease because the
   packages still need GPU and end-user platform validation.

Each build also uploads its package to the run's **Artifacts**, retained for
90 days. Diagnostics are retained for 14 days, including when a build fails.
Changes to the workflow or build script automatically trigger builds on `main`
and preserve verified packages in a runtime release. Manual runs publish if
requested. Publication also runs when a platform fails: a partial release states
how many of the five packages passed verification, and preserves those packages.
A failed target does not prevent other targets from completing and uploading.

Compilation and verification are separate steps. Before tests start, each target
uploads a `whisper-candidate-1.9.6-TARGET` artifact containing its staged runtime,
licenses and build metadata. Candidates are **unverified** and retained for
90 days. If Windows tests fail, launch the workflow again and enter that earlier
run's numeric ID in **Optional earlier run ID**. The Windows job downloads its
candidate, skips compiler/CUDA installation and compilation, and reruns the tests.
Candidate metadata must match the fixed version, commit and target. Other targets
build normally. Old runs made before this feature have no candidate to recover.

When Windows compilation itself fails or times out, any files already produced
in its output directory are also uploaded as `whisper-partial-windows-x64-cuda`.
These may be incomplete and cannot be used as a verified runtime or as an input
to the candidate-recovery option. A timeout before linking may leave no binaries.

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

The Windows build follows the upstream v1.9.6
[`release.yml` CUDA job](https://github.com/ggml-org/whisper.cpp/blob/v1.9.6/.github/workflows/release.yml):
Ninja Multi-Config, ccache and parallel compilation using the runner's CPU count.
It retains our explicit MSVC 14.38 selection for CUDA 12.4. The compiler cache is
limited to 2 GiB and is an acceleration mechanism, not permanent binary storage.
The build command has a 210-minute timeout inside a 240-minute job budget; ordinary
checks have a 10-minute timeout. Compiler output streams to the Actions console
and log file as it is produced, including output before a timeout or failure.

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
