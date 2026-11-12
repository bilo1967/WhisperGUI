package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type event struct {
	Type    string   `json:"type"`
	Message string   `json:"message,omitempty"`
	Line    string   `json:"line,omitempty"`
	State   string   `json:"state,omitempty"`
	Outputs []string `json:"outputs,omitempty"`
	Archive string   `json:"archive,omitempty"`
}
type work struct {
	mu        sync.Mutex
	state     string
	cancel    context.CancelFunc
	listeners map[chan event]struct{}
	archive   string
	history   []event
}
type jobRequest struct {
	File                 string `json:"file"`
	Model                string `json:"model"`
	Language             string `json:"language"`
	Translate            bool   `json:"translate"`
	WordTimestamps       bool   `json:"word_timestamps"`
	TextGrid             bool   `json:"textgrid"`
	ExperimentalFeatures bool   `json:"experimental_features"`
	CPU                  bool   `json:"cpu"`
	GlossaryEnabled      bool   `json:"glossary_enabled"`
	GlossaryHint         string `json:"glossary_hint"`
	CarryInitialPrompt   bool   `json:"carry_initial_prompt"`
}

func (w *work) emit(e event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e.State != "" {
		w.state = e.State
	}
	w.history = append(w.history, e)
	if len(w.history) > 4000 {
		w.history = w.history[len(w.history)-4000:]
	}
	for ch := range w.listeners {
		select {
		case ch <- e:
		default:
		}
	}
}
func (w *work) snapshot() (string, []event) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state, append([]event(nil), w.history...)
}

func (a *application) runJob(req jobRequest) {
	defer func() { a.job.mu.Lock(); a.job.cancel = nil; a.job.mu.Unlock() }()
	a.job.mu.Lock()
	ctx := a.jobContext
	a.job.mu.Unlock()
	a.job.emit(event{Type: "state", State: "converting", Message: "Converting audio with FFmpeg..."})
	tmp, err := os.MkdirTemp("", "whispergui-*")
	if err != nil {
		a.jobError(err)
		return
	}
	defer os.RemoveAll(tmp)
	wav := filepath.Join(tmp, "audio.wav")
	if err = runProcess(ctx, a.paths.FFmpeg, []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-i", req.File, "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav}, nil); err != nil {
		a.jobError(err)
		return
	}
	model := filepath.Join(a.paths.ModelsDir, req.Model)
	config := a.snapshotConfig()
	outputDir := config.Paths.Output
	if outputDir == "" {
		outputDir = filepath.Dir(req.File)
	} else if !filepath.IsAbs(outputDir) {
		outputDir = filepath.Join(a.paths.ExecutableDir, outputDir)
	}
	if err = os.MkdirAll(outputDir, 0755); err != nil {
		a.jobError(err)
		return
	}
	base := strings.TrimSuffix(filepath.Base(req.File), filepath.Ext(req.File))
	prefix := filepath.Join(outputDir, base)
	// Avoid overwriting existing results: use a unique stem when needed.
	for n := 0; ; n++ {
		candidate := prefix
		if n > 0 {
			candidate = filepath.Join(outputDir, fmt.Sprintf("%s_%s_%d", base, time.Now().Format("20060102_150405"), n))
		}
		available := true
		for _, ext := range []string{".json", ".txt", ".srt", ".vtt", ".words.json", ".words.csv", ".timestamps.txt", ".TextGrid", ".whisper.zip"} {
			if _, err := os.Stat(candidate + ext); err == nil || !os.IsNotExist(err) {
				available = false
				break
			}
		}
		if available {
			prefix = candidate
			break
		}
	}
	args := []string{"-m", model, "-f", wav, "-l", req.Language, "-of", prefix, "-oj", "-pp"}
	if req.Translate {
		args = append(args, "-tr")
	}
	if req.WordTimestamps || req.TextGrid {
		preset, ok := dtwPreset(req.Model)
		if !ok {
			a.jobError(fmt.Errorf("word-level timestamps (DTW) not supported for model %q; disable the option or select a compatible model", req.Model))
			return
		}
		args = append(args, "--dtw", preset, "--no-flash-attn", "-ojf")
	}
	if config.Whisper.VADEnabled {
		vadModel := filepath.Join(a.paths.ModelsDir, "ggml-silero-v6.2.0.bin")
		if info, statErr := os.Stat(vadModel); statErr != nil || info.IsDir() {
			a.jobError(fmt.Errorf("Silero VAD model is missing; install it using Manage models, or disable Silence detection (VAD) in Settings"))
			return
		}
		args = append(args, "--vad", "--vad-model", vadModel)
	}
	if req.GlossaryEnabled && strings.TrimSpace(req.GlossaryHint) != "" {
		args = append(args, "--prompt", strings.TrimSpace(req.GlossaryHint))
		if req.CarryInitialPrompt {
			args = append(args, "--carry-initial-prompt")
		}
	}
	if config.Whisper.Temperature > 0 {
		args = append(args, "--temperature", strconv.FormatFloat(config.Whisper.Temperature, 'f', 2, 64))
	}
	if req.CPU {
		args = append(args, "-ng")
	}
	if config.Whisper.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(config.Whisper.Threads))
	}
	args = append(args, "-otxt", "-osrt", "-ovtt")
	a.job.emit(event{Type: "state", State: "transcribing", Message: "Transcribing..."})
	if err = runProcess(ctx, a.paths.Whisper, args, func(line string) {
		if strings.TrimSpace(line) != "" {
			a.job.emit(event{Type: "line", Line: line})
		}
	}); err != nil {
		a.jobError(err)
		return
	}
	outputs := []string{}
	for _, f := range []string{"srt", "vtt", "txt", "json"} {
		p := prefix + "." + f
		if _, e := os.Stat(p); e == nil {
			outputs = append(outputs, p)
		}
	}
	{
		src := prefix + ".json"
		if err = makeTimestamped(src, prefix+".timestamps.txt"); err != nil {
			a.jobError(err)
			return
		}
		outputs = append(outputs, prefix+".timestamps.txt")
		if req.WordTimestamps || req.TextGrid {
			if err = makeWordOutputs(src, prefix+".words.json", prefix+".words.csv"); err != nil {
				a.jobError(err)
				return
			}
			outputs = append(outputs, prefix+".words.json", prefix+".words.csv")
			if req.TextGrid {
				if err = makeTextGrid(prefix+".words.json", wav, prefix+".TextGrid"); err != nil {
					a.jobError(err)
					return
				}
				outputs = append(outputs, prefix+".TextGrid")
			}
		}
	}
	archivePath := ""
	if config.Output.PackageZIP {
		a.job.emit(event{Type: "line", Line: "Packaging results as ZIP..."})
		archivePath = prefix + ".whisper.zip"
		warnings, zipErr := packageResults(ctx, archivePath, outputs)
		if zipErr != nil {
			a.jobError(fmt.Errorf("ZIP packaging failed; original outputs were retained: %w", zipErr))
			return
		}
		for _, warning := range warnings {
			a.job.emit(event{Type: "line", Line: "Warning: " + warning})
		}
		outputs = []string{archivePath}
	}
	a.job.mu.Lock()
	a.job.archive = archivePath
	a.job.mu.Unlock()
	a.job.emit(event{Type: "done", State: "done", Message: "Transcription completed.", Outputs: outputs, Archive: archivePath})
}
func (a *application) jobError(err error) {
	if errors.Is(err, context.Canceled) {
		a.job.emit(event{Type: "state", State: "stopped", Message: "Operation stopped."})
	} else {
		a.job.emit(event{Type: "error", State: "error", Message: err.Error()})
	}
}
func runProcess(ctx context.Context, bin string, args []string, onLine func(string)) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = filepath.Dir(bin)
	prepareProcess(cmd)
	if onLine == nil {
		out, err := cmd.CombinedOutput()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("%s: %w: %s", filepath.Base(bin), err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	r, writer := io.Pipe()
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Start(); err != nil {
		r.Close()
		writer.Close()
		return err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		scan := bufio.NewScanner(r)
		scan.Buffer(make([]byte, 4096), 1024*1024)
		for scan.Scan() {
			onLine(scan.Text())
		}
	}()
	err := cmd.Wait()
	writer.Close()
	<-done
	r.Close()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(bin), err)
	}
	return nil
}
func formatTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	s := ms / 1000
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, (s/60)%60, s%60)
}
func makeTimestamped(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	var v struct {
		Transcription []struct {
			Timestamps struct {
				From string `json:"from"`
				To   string `json:"to"`
			} `json:"timestamps"`
			Offsets struct {
				From int64 `json:"from"`
				To   int64 `json:"to"`
			} `json:"offsets"`
			Text string `json:"text"`
		} `json:"transcription"`
	}
	if err = json.Unmarshal(b, &v); err != nil {
		return err
	}
	if len(v.Transcription) == 0 {
		return fmt.Errorf("JSON contains no transcription segments")
	}
	var sb strings.Builder
	for _, t := range v.Transcription {
		start, end := formatTime(t.Offsets.From), formatTime(t.Offsets.To)
		if t.Timestamps.From != "" {
			start = strings.Split(t.Timestamps.From, ",")[0]
		}
		if t.Timestamps.To != "" {
			end = strings.Split(t.Timestamps.To, ",")[0]
		}
		fmt.Fprintf(&sb, "[%s - %s] %s\n", start, end, strings.TrimSpace(t.Text))
	}
	return os.WriteFile(dst, []byte(sb.String()), 0644)
}

// dtwPreset maps standard GGML filenames to their whisper.cpp DTW presets.
// Quantized variants use the same architecture/preset as the base model.
func dtwPreset(filename string) (string, bool) {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(filename), ".bin"))
	name = strings.TrimPrefix(name, "ggml-")
	// Strip common GGML quantization suffixes, e.g. small-q5_0.
	for _, suffix := range []string{"-q2_k", "-q3_k", "-q4_0", "-q4_1", "-q4_k", "-q5_0", "-q5_1", "-q5_k", "-q6_k", "-q8_0", "-q8_1"} {
		name = strings.TrimSuffix(name, suffix)
	}
	switch name {
	case "tiny", "tiny.en", "base", "base.en", "small", "small.en", "medium", "medium.en":
		return name, true
	case "large", "large-v3":
		return "large.v3", true
	case "large-v3-turbo":
		return "large.v3.turbo", true
	case "large-v1":
		return "large.v1", true
	case "large-v2":
		return "large.v2", true
	default:
		return "", false
	}
}
