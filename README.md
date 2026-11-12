## v0.6.27 — Display and language preference

- Show in folder uses a solid Bootstrap-style green background and white text.
- Transcription language selection is saved immediately and restored on startup, as for the selected model.

## v0.6.26 — Archive bar visuals

- Use the provided Bootstrap ZIP file SVG beside the output archive name.
- Make Show in folder green in light and dark modes.

## v0.6.25 — Windows Show focus and selected model

- Windows Show uses Explorer /n,/select and activates only a newly opened Explorer window. It no longer raises an arbitrary existing Explorer window. If Windows reuses an existing window, selection still works but focus may remain in the browser.
- The selected ASR model is saved when changed and restored at startup when still installed; otherwise the available fallback remains selected.

## v0.6.23 — Windows archive window focus

- Open and Show: after launching Explorer or the archive handler, locate the new window and request foreground activation.
- This addresses the limitation of v0.6.22, where supplying the owner HWND to ShellExecuteW did not itself activate the target window.
- Foreground activation is restricted by Windows and must be tested on Windows 10/11 with the installed ZIP handler.

## v0.6.22 — Windows result window activation

The ZIP result Open and Show buttons use the Windows shell API with the active browser window supplied as owner (like the native file picker), rather than launching an unowned Explorer process. This is intended to prevent resulting windows from opening behind the browser. Verify on Windows.

## v0.6.21 — Model Manager download indicators

- A green circular check identifies downloaded models.
- An empty neutral circle identifies models that have not been downloaded.
- Both indicators use the same size and appear to the left of the model name.

## v0.6.20 — Package transcription results as ZIP

Advanced Settings includes **Package results as ZIP**, enabled by default and saved as `output.package_zip`. New installations use this default; existing configuration files without the setting also default to ZIP packaging. When enabled, WhisperGUI collects all output formats (including word timestamps and Praat TextGrid when selected) into `<basename>.whisper.zip` alongside the source recording. The archive is verified before removing the individual results. If ZIP creation fails, the original output files remain available. When disabled, the individual files remain as before.

A compact bar below the transcription log shows the latest completed ZIP with **Open**, **Show** (in folder) and **Delete**, which requires confirmation and never deletes the original media. The actions are limited to the archive of the most recent completed transcription in the current server session. The bar is hidden when a new job starts or when the archive is deleted. When packaging succeeds but some individual files cannot be removed, the log reports a warning.

## v0.6.19 — Compact temperature control

Temperature slider is now inline with its label and displayed value; the explanatory text is shorter. At temperature 0 (default), WhisperGUI does not pass `--temperature` to whisper-cli, preserving the engine default.

## v0.6.18 — Context, experimental TextGrid and temperature

Advanced Settings now separates the optional context hint from the "carry throughout" choice. Both the hint text and carry choice are session-only: the hint is sent with each job request and never saved to configuration. Empty/whitespace-only hints are ignored. Previous configuration hints are discarded on load and omitted from future saves.

Experimental features can be enabled in Advanced Settings, revealing the optional Praat TextGrid checkbox on the main screen. Switching experimental features off hides and clears this selection. TextGrid still uses whisper.cpp DTW word timestamps; its boundaries are approximate, not independently realigned. Temperature (0.0–1.0, default 0.0) controls initial sampling temperature and leaves whisper.cpp temperature fallback unchanged. CPU-only remains session-only.

## v0.6.17 — Praat TextGrid export

Optional **Praat TextGrid** checkbox on the main screen (off by default). Enabling it automatically selects **Word-level timestamps** (DTW). Two IntervalTiers — `Segments` and `Words` — cover the full duration of the FFmpeg-converted recording, including empty intervals for pauses. The resulting `<basename>.TextGrid` is placed alongside the other outputs. The JSON and CSV word timestamps remain unchanged; TextGrid intervals extending beyond the audio duration and overlapping DTW intervals are clamped to meet Praat’s non-overlapping interval requirement. TextGrid boundaries are approximate and should be checked before phonetic measurements.

**CPU only** moved into Advanced Settings (session-level, not persisted). **Copy log** and **Copy** for Diagnostics added.

WhisperGUI v0.6.10: When Word-level timestamps (DTW) is enabled, also produces .words.json (per-segment word offsets/durations in milliseconds) and .words.csv (start/end/duration in seconds). Original full token JSON remains unchanged.

WhisperGUI v0.6.9: DTW now passes --no-flash-attn; default languages include Polish and Ukrainian; interface controls have English tooltips.

WhisperGUI v0.6.9: Word-level timestamps checkbox enables whisper-cli --dtw with a supported model preset and --output-json-full. Standard SRT/VTT/TXT timing remains segment-based. Non-standard models such as tinydiarize may not have a supported DTW preset.

## Version 0.6.1

- Interface labels and messages in English.
- whisper.cpp GitHub link beside its version in Information and in the log.
- The UI theme is now saved under `ui.theme` (`light` or `dark`) in `config.yaml`; existing configuration files are supported without editing.

# WhisperGUI v4 - prototipo di trascrizione

Backend Go, frontend HTML embedded, dialog nativo `zenity`, FFmpeg e whisper.cpp esterni.

## Windows: avvio rapido

1. Estrai il sorgente in una directory.
2. `go mod tidy` (necessaria connessione Internet la prima volta).
3. `go build -o WhisperGUI-debug.exe .` per avere la console, oppure `go build -ldflags="-H=windowsgui" -o WhisperGUI.exe .` per nasconderla.
4. Crea `bin` accanto all'eseguibile e copia `ffmpeg.exe`, `whisper-cli.exe` e **tutte le DLL della stessa build** whisper.cpp.
5. Copia un modello GGML (`ggml-base.bin`, per esempio) nella directory indicata dalla diagnostica, oppure imposta `paths.models` nel `config.yaml` già generato.
6. Avvia, scegli un file e il modello, poi premi *Trascrivi*.

La selezione nativa su Windows può apparire dietro al browser nella build windowsgui: problema noto e non ancora risolto. La GUI funziona comunque se si porta in primo piano la finestra di selezione (Alt+Tab).

## Configurazione e directory

Mantiene le regole della v2: `install.yaml` con `mode: system` o `mode: user`; `portable.flag` per il funzionamento portabile. In system-wide i modelli sono in `%PROGRAMDATA%\\WhisperGUI\\models`, e l'installer dovrà gestirli con permessi riservati agli amministratori. Per il test immediato è pratico creare `portable.flag` e una cartella `models` accanto all'eseguibile. `paths.output` vuoto salva i risultati accanto al file sorgente.

## Funzioni v3

- Scelta di file via dialog Go, senza upload HTTP.
- Ricerca dei modelli `*.bin` nella directory configurata, senza downloader.
- Conversione del file in WAV 16 kHz, mono, PCM 16 bit tramite FFmpeg.
- Trascrizione whisper-cli con lingua, traduzione in inglese e selezione opzionale di sola CPU.
- Log incrementale nel browser tramite stream HTTP; TXT, SRT, VTT, JSON e TXT con timestamp.
- Interruzione di FFmpeg/whisper-cli e pulizia della directory temporanea.
- Diagnostica ripiegata; interfaccia compatta.

Nota: il pulsante Interrompi utilizza la cancellazione di `exec.CommandContext`; su Windows termina il processo principale. La gestione dei processi figli e l'eventuale ripulitura degli output parziali richiederanno un affinamento. Non è ancora prevista la gestione/download dei modelli né il salvataggio delle preferenze cambiate nella pagina. Il prototipo accetta un job alla volta.

## Linux / macOS

Compila con `go build .` dopo `go mod tidy`. Servono i corrispondenti binari `ffmpeg`, `whisper-cli` nella directory `bin` e un dialog disponibile (`zenity`/equivalente su Linux). Il test end-to-end su questi sistemi resta da effettuare.

## Verifiche

La sintassi Go e il packaging sono stati controllati; la build Go completa non è stata eseguibile nell'ambiente di preparazione poiché le dipendenze Go non sono scaricabili. Eseguire `go mod tidy` e `go build` su una macchina con rete.

## Modifiche v4

- Configurazione YAML generata con commento su percorsi Windows: usa `C:/percorso/...` oppure apici singoli (`'C:\\percorso\\...'`). I vecchi YAML esistenti non vengono riscritti: inserisci manualmente il commento se necessario.
- Log ad altezza fissa, messaggi di stato e percorsi dei risultati integrati nel log.
- Nome file in grassetto e percorso sulla stessa riga della selezione.
- Tema chiaro/scuro con scelta conservata nel browser.
- Tutti gli output automatici: SRT, VTT, TXT, JSON e TXT con timestamp; nessuna checkbox.

## Novita v5

- Browser: segnale di chiusura con `pagehide` e heartbeat; quando l’ultima pagina smette di comunicare il server termina dopo circa 10 secondi. La scheda sospesa puo essere considerata chiusa da questo meccanismo.
- Pulsante Info per riepilogo percorsi.
- Lingue configurabili in `config.yaml`, voce `languages` (lista `code` / `label`). La lista predefinita sostituisce olandese con russo e include arabo.
- Gestisci modelli: elenco da Hugging Face, download/aggiornamento/eliminazione, una operazione per volta, progresso e annullamento. Modelli scaricati direttamente da `ggerganov/whisper.cpp`; aggiornamento riconosciuto tramite dimensioni e ETag registrato quando disponibili.
- In installazione `system` i modelli possono essere modificati soltanto avviando WhisperGUI elevato come amministratore; Inno Setup dovra configurare i permessi della cartella condivisa.
- La selezione file con dialog nativo resta invariata e potrebbe apparire dietro al browser.
- I download parziali non vengono mantenuti dopo un annullamento. Per modelli gia installati, l'aggiornamento conserva il vecchio file fino al completamento del nuovo download.

Per inserire lingue personalizzate nello YAML esistente:

```yaml
languages:
  - code: auto
    label: Automatica
  - code: it
    label: Italiano
  - code: ru
    label: Russo
  - code: ar
    label: Arabo
```

## Versioni (v0.6.0)

All’avvio il frontend visualizza le versioni WhisperGUI, whisper.cpp e FFmpeg nel log e nel pannello Info. Le versioni dei tool sono rilevate con `whisper-cli --version` e `ffmpeg -version` (prima riga). Nessun controllo aggiornamenti automatico.

## UI language (v0.6.1)

The web interface, status messages and default language names are in English. The whisper.cpp version links to https://github.com/ggml-org/whisper.cpp in the Info dialog and includes its URL in the startup log. Custom labels already stored in config.yaml are retained, by design.

## 0.6.4 - Windows file dialog focus experiment

On Windows, the file picker now uses Zenity's `Attach` option with the
currently foreground window handle (`GetForegroundWindow`). This should
associate the picker with the browser window instead of the Go console.
No change to the dialog implementation on macOS or Linux.

Test with:

```powershell
go build -ldflags="-H=windowsgui" -o WhisperGUI.exe .
```

This is a Windows-only focus experiment: the foreground window may differ
if another application takes focus between the click and the API call.

### v0.6.5: lifecycle and window handling

- Only one instance per user session on Windows (named mutex) and per user on macOS/Linux (file lock). A second invocation exits; it does not start another browser.
- Background tabs do not trigger shutdown after 10 seconds. The heartbeat idle timeout is now 10 minutes, and active jobs/model downloads prevent timeout shutdown. Closing the browser may consequently leave WhisperGUI running for up to 10 minutes; use Task Manager or Ctrl+C to exit sooner.
- On Windows, browser launch uses ShellExecuteW rather than rundll32; this may avoid flickering transient windows.
- Clear log button in the log header.
- The experimental Zenity Attach behavior from v0.6.4 is unchanged: if browser focus/modal behavior recurs, return to v0.6.3 for dialog selection until a safer fix is implemented.

### v0.6.6: log messages and project tracking

- A single friendly log message replaces repetitive event-stream errors when the local server disconnects. A reconnection is reported if the event stream returns.
- Startup log explains that audio and transcripts remain local; model downloads may contact Hugging Face.
- Added `TODO+ISSUES+DONE.txt` as a maintained project tracker.

## Client lifecycle (0.6.7)

The browser maintains one WebSocket per tab to `/api/client`, with a session token.
Once all client connections close, WhisperGUI shuts down after the configured
`server.disconnect_grace_seconds` (default 5), unless a transcription or model
download is in progress. Browser background tabs do not rely on JavaScript
timers; the server uses protocol-level ping/pong. A first connection must be
established within 90 seconds. Reconnecting clients cancel pending shutdown.

```yaml
server:
  disconnect_grace_seconds: 5
```

The WebSocket dependency is `github.com/gorilla/websocket` v1.5.3. Run
`go mod tidy` before building.

## Silero VAD (0.6.14)

Manage models offers Silero VAD 6.2.0 (about 885 KB). In Settings, enable Silence detection (VAD). When enabled, the transcription command receives `--vad --vad-model <models-dir>/ggml-silero-v6.2.0.bin`. VAD is disabled by default and uses whisper-cli defaults for sensitivity. The VAD model is not listed as a transcription model. Preferences are persisted as `whisper.vad_enabled` in config.yaml.

## Interface improvements (0.6.15)

The Select file and Transcribe buttons now use compact icons, and Select file uses a primary visual style. Manage models separates speech recognition models from additional models (currently Silero VAD). When the dialog is reopened without an active download, the progress indicator starts at 0% with Ready. Active downloads retain their live progress.

## Interface icons (0.6.16)

The Select file and Transcribe buttons use the provided inline SVG icons. No external image assets are required.

### 0.6.25

- The output archive bar now offers Show in folder and Delete (no Open action).
- Enabling experimental features applies immediately for the current session; Save settings persists the choice for future launches.
- Clicking an empty model status circle downloads the missing model (subject to existing permissions); a green check remains noninteractive.
- The model manager provides an Open models folder shortcut.
