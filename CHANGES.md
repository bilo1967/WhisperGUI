# Changes

Release history for WhisperGUI. The [README](README.md) describes the **current** application rather than historical revisions.

## 0.6.27
- Styled **Show in folder** as a solid green button.
- Persisted and restored the selected transcription language.

## 0.6.26
- Replaced the archive symbol with the supplied ZIP SVG.
- Updated the archive action styling for light and dark themes.

## 0.6.25
- Simplified archive actions to **Show in folder** and **Delete**.
- Experimental features can be used immediately in the current session without saving preferences.
- Missing-model status circles start the corresponding model download; installed-model checks remain inert.
- Added **Open models folder** in Model Manager.

## 0.6.24
- Improved the Windows **Show** command so it does not activate an unrelated Explorer window.
- Persisted and restored the selected ASR model when available.

## 0.6.23
- Added Windows window-activation handling for archive actions, with safeguards against changing focus after the user switches windows.

## 0.6.22
- Used Windows ShellExecuteW with the foreground browser window as the owner for archive actions.

## 0.6.21
- Added model-availability circles in Model Manager: green checks for installed models and neutral empty circles for missing ones.

## 0.6.20
- Added optional packaging of transcription output files into a verified ZIP archive, enabled by default.
- Added an archive action bar below the log, with safeguards before removing individual outputs.

## 0.6.19
- Made the temperature slider compact and clarified its description; temperature zero omits the explicit CLI option.

## 0.6.18
- Made glossary text and the carry-through choice session-only, ignoring empty hints.
- Added the experimental-features switch for TextGrid.
- Added a temperature slider and simplified CPU/VAD descriptions.

## 0.6.17
- Added optional Praat TextGrid export with Segments and Words tiers based on whisper.cpp DTW timestamps.
- Moved CPU-only to Advanced Settings.
- Added **Copy log** and **Copy Diagnostics**.

## 0.6.16
- Replaced Select file and Transcribe button icons with the supplied SVGs.

## 0.6.15
- Highlighted Select file, added a Transcribe icon, separated ASR and additional models, and reset inactive Model Manager progress on reopening.

## 0.6.14
- Added Silero VAD 6.2.0 download and optional silence detection.

## 0.6.13
- Moved Exit to the far right and differentiated its appearance.

## 0.6.12
- Added an explicit Exit button with confirmation and active-operation handling.

## 0.6.11
- Introduced a glossary/context hint; its original persistence behavior was subsequently replaced.

## 0.6.10
- Added simplified per-word JSON and CSV output from the full DTW result.

## 0.6.9
- Added DTW word-timestamp integration, disabled Flash Attention when DTW is active, expanded language defaults and added English tooltips.

## 0.6.8
- Added optional DTW token timestamps and full JSON output.

## 0.6.7
- Replaced HTTP heartbeat/pagehide detection with WebSocket client-presence tracking.

## 0.6.6
- Improved disconnect logging and local-data privacy messaging; added the project TODO/ISSUES/DONE tracker.

## 0.6.5
- Added single-instance enforcement, improved process/window lifecycle handling, and added Clear log.

## 0.6.4
- Tested the Windows native-dialog foreground-window attachment approach.

## 0.6.1
- Standardized the interface in English, added whisper.cpp attribution and saved theme preferences.

## Earlier prototypes (v3–v5 and 0.6.0)
- Established the Go HTTP backend, embedded web frontend, native file selection, FFmpeg conversion and whisper.cpp integration.
- Added incremental transcription logs, stop handling, standard transcript/subtitle formats, diagnostics and model management.
- Introduced portable, per-user and system-wide configuration modes, a light/dark theme, runtime version display and the early browser-lifecycle mechanism.
