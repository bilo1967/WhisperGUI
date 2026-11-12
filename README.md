# WhisperGUI

WhisperGUI is a lightweight, cross-platform graphical interface for local audio and video transcription using [whisper.cpp](https://github.com/ggml-org/whisper.cpp) and FFmpeg.

**Current version: 0.6.27** · [Change history](CHANGES.md) · [Open issues and planned work](TODO+ISSUES+DONE.txt)

## Features

- Runs locally using a Go HTTP backend and an embedded browser interface. Audio files are processed locally; model downloads connect to Hugging Face.
- Transcribes audio and video, with selectable Whisper model and language, automatic language detection, and optional translation into English.
- Native file selection, light/dark themes, compact progress log, stop control, and diagnostics.
- Model Manager for downloading, updating, and deleting ASR models and the optional Silero VAD model; download progress and cancellation are supported.
- Optional silence detection with Silero VAD.
- Optional word-level timestamps using whisper.cpp DTW. The full JSON output and simplified word-level JSON/CSV are generated when enabled.
- Optional Praat TextGrid export with `Segments` and `Words` tiers, accessible when experimental features are enabled. Word boundaries are estimates and should be checked for phonetic research.
- Optional context/glossary hint, with independent carry-through option, for the current session; adjustable initial decoding temperature.
- Results in TXT, timestamped TXT, SRT, VTT and JSON, plus word-level outputs and TextGrid when selected.
- **Package results as ZIP** is enabled by default: output files are archived next to the source file, then removed only after ZIP verification. The result bar offers **Show in folder** and **Delete** (with confirmation). Disable packaging in Advanced Settings to keep separate files.
- Remembers selected transcription model, language, theme, and saved advanced preferences. CPU-only and context-hint text are session settings.
- A WebSocket tracks connected browser tabs and allows the local server to shut down after its configurable grace period when the last client disconnects and no active operation prevents shutdown.

## Building

Requires **Go 1.25 or later** and the Go modules specified in `go.mod`.

On Windows, build a console-enabled executable for debugging:

```powershell
go mod tidy
go build -o WhisperGUI-debug.exe .
```

To build without a console window:

```powershell
go build -ldflags="-H=windowsgui" -o WhisperGUI.exe .
```

On macOS or Linux:

```sh
go mod tidy
go build -o WhisperGUI .
```

The Go build embeds the contents of `web/`. The Windows executable icon and version metadata are defined in `winres/`; generating Windows resources with `go-winres` is a separate build step.

## Runtime requirements

Place the matching **`whisper-cli`** and **FFmpeg** executables in a `bin/` directory next to WhisperGUI. On Windows, include the supporting whisper.cpp DLLs from the **same official build**. For the CPU-only package, the tested CUDA-capable `whisper-cli.exe` can run without the NVIDIA/CUDA DLLs; for CUDA acceleration, include those DLLs.

Use binaries for the target operating system and CPU architecture. On Linux, a compatible native file-dialog utility (such as Zenity) is also required. The runtime bundles are not included in this source repository.

## Configuration and model storage

WhisperGUI creates its configuration when run. It supports per-user, system-wide and portable installations; a `portable.flag` beside the executable selects portable mode. In portable mode, models can be stored alongside the application. The **Info** and **Diagnostics** panels show the actual paths used on the current machine.

For shared/system-wide installations, managing models may require administrator privileges. Models can also be installed through **Manage models**.

The selected model and language are restored at startup. Advanced Settings include VAD, ZIP packaging, temperature and experimental features. Session-only choices such as **CPU only** and the glossary text are not retained between launches.

## Output files

Results are written beside the source recording. By default, WhisperGUI packages them as `<basename>.whisper.zip`, including the formats applicable to that transcription. If ZIP packaging is disabled, the individual output files remain in the same directory.

DTW word timestamps are approximate; the TextGrid export converts those values into Praat intervals and does not perform independent forced alignment.

## Project status and licenses

WhisperGUI is under active development. The source code is distributed under the [MIT License](LICENSE). whisper.cpp, FFmpeg and any bundled runtime components retain their own licenses; check the notices and obligations associated with the binaries selected for distribution.

See [CHANGES.md](CHANGES.md) for the version history.
