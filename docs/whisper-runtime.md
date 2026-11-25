# Building whisper.cpp runtimes

The **Build whisper.cpp runtimes** workflow creates our own runtime packages from
whisper.cpp **v1.9.6**, fixed at commit
`96ec4d9823e9d0211944172da35f9f29134af990`. It does not follow upstream `master`
or search upstream Actions for the latest successful run.

## Normal use: build all five platforms

This is a reusable build workflow, not a one-time recovery operation.

1. Open **Actions → Build all 5 whisper.cpp runtimes → Run workflow** on `main`.
2. Leave **Keep all verified packages in a dedicated runtime release** enabled.
3. Click **Run workflow**. No run IDs or recovery settings are needed.

Every run compiles and verifies all five targets listed below. Only manual
launches start builds; changes to the build scripts do not automatically start
another expensive Windows compilation.

After all five targets succeed, **Releases** contains all five ZIPs and
`SHA256SUMS.txt`. The prerelease is named
`whisper.cpp 1.9.6 runtimes (build N)`, tagged `whisper-runtime-v1.9.6-build-N`.
GPU and end-user validation is still needed before application distribution.
Rerunning the publication job updates an existing release for that run.

Each successful target also uploads its verified ZIP and SHA256 sidecar to
**Artifacts**, retained for 90 days. If a target fails, the other four continue
and their successful artifacts remain downloadable; the complete release is
published only when all five pass. Diagnostics are retained for 14 days.
Unverified candidates are saved separately before verification, and partial
Windows binaries are preserved when a build/staging step fails.

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

Before CMake configuration or compilation, the script verifies the upstream
license/sample and required CLI options, resolves the MSVC CRT/OpenMP and CUDA
DLLs, stages their redistributables, and prepares the CUDA license. It uses a
local `EULA.txt` when available; otherwise it downloads the official
[NVIDIA EULA for CUDA 12.4.1](https://docs.nvidia.com/cuda/archive/12.4.1/eula/index.html)
as `LICENSE-CUDA.html`, recording its URL in `LICENSE-CUDA-SOURCE.txt`.
Selective toolkit installation does not have to provide `CUDA_PATH/EULA.txt`.
Required build/verification tools are checked as well. Smoke-test models are
downloaded and checked against their published SHA256 values before compilation,
then reused during verification. Missing DLLs, unavailable downloads, invalid
license documents or corrupt models therefore fail before the long build.
Checks that require a compiled executable still run after staging.

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

## Maintenance recovery (optional)

**Recover a saved Windows runtime (maintenance)** is a separate workflow for
exceptional recovery. It does not build the five-platform set or publish a full
release. For normal builds, use **Build all 5 whisper.cpp runtimes** above.

Supply `source_run` with the earlier run ID and choose `compiled-binaries`
for `whisper-partial-windows-x64-cuda`, or `staged-candidate` for
`whisper-candidate-1.9.6-windows-x64-cuda`. The workflow validates the original
source/CUDA pins and repackages/reverifies Windows without compilation. Its
verified Windows ZIP is available in that run's Artifacts. Incomplete binaries
fail verification rather than triggering a new build.

When a Unix candidate is verified after artifact transfer, the builder restores
its executable permission before running it and before creating the final ZIP.

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
