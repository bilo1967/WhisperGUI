package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (a *application) listModels(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Non autorizzato", 403)
		return
	}
	names := []string{}
	var modelBytes int64
	var modelCount int
	entries, _ := os.ReadDir(a.paths.ModelsDir)
	for _, f := range entries {
		if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".bin") {
			continue
		}
		if info, err := f.Info(); err == nil && info.Mode().IsRegular() {
			modelBytes += info.Size()
			modelCount++
		}
		if !strings.HasPrefix(strings.ToLower(f.Name()), "ggml-silero-") {
			names = append(names, f.Name())
		}
	}
	sort.Strings(names)
	translationSupported := make(map[string]bool, len(names))
	for _, name := range names {
		translationSupported[name] = modelSupportsTranslation(name)
	}
	jsonReply(w, map[string]any{"models": names, "translation_supported": translationSupported, "model_bytes": modelBytes, "model_count": modelCount})
}
func jsonReply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func (a *application) startJob(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Non autorizzato", 403)
		return
	}
	var req jobRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}
	if len([]rune(req.GlossaryHint)) > 4000 {
		http.Error(w, "Context hint is too long", http.StatusBadRequest)
		return
	}
	if req.TextGrid && !req.ExperimentalFeatures {
		http.Error(w, "Enable experimental features to export Praat TextGrid", http.StatusBadRequest)
		return
	}
	if req.Language == "" {
		req.Language = "auto"
	}
	if req.Model == "" || req.Model != filepath.Base(req.Model) || !strings.HasSuffix(strings.ToLower(req.Model), ".bin") {
		http.Error(w, "Invalid model", 400)
		return
	}
	if req.Translate && !modelSupportsTranslation(req.Model) {
		http.Error(w, "The selected model does not support translation to English", http.StatusBadRequest)
		return
	}
	a.selectedMu.Lock()
	selected := a.selectedFile
	a.selectedMu.Unlock()
	if req.File != selected || selected == "" {
		http.Error(w, "File not selected through the file dialog", 403)
		return
	}
	info, err := os.Stat(req.File)
	if err != nil || info.IsDir() {
		http.Error(w, "File unavailable", 400)
		return
	}
	model := filepath.Join(a.paths.ModelsDir, req.Model)
	if i, e := os.Stat(model); e != nil || i.IsDir() {
		http.Error(w, "Model unavailable", 400)
		return
	}
	for _, p := range []string{a.paths.FFmpeg, a.paths.Whisper} {
		if _, e := os.Stat(p); e != nil {
			http.Error(w, "Binario mancante: "+p, 400)
			return
		}
	}
	a.job.mu.Lock()
	if a.job.cancel != nil {
		a.job.mu.Unlock()
		http.Error(w, "Transcription already in progress", 409)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.jobContext = ctx
	a.job.cancel = cancel
	a.job.history = nil
	a.job.archive = ""
	a.job.state = "starting"
	a.job.mu.Unlock()
	go a.runJob(req)
	jsonReply(w, map[string]string{"status": "started"})
}
// clearLog clears the event backlog without stopping any running job.
func (a *application) clearLog(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Non autorizzato", http.StatusForbidden)
		return
	}
	a.job.mu.Lock()
	a.job.history = nil
	a.job.mu.Unlock()
	jsonReply(w, map[string]string{"status": "ok"})
}

func (a *application) stopJob(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Non autorizzato", 403)
		return
	}
	a.job.mu.Lock()
	cancel := a.job.cancel
	a.job.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	jsonReply(w, map[string]string{"status": "ok"})
}
func (a *application) events(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Non autorizzato", 403)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ch := make(chan event, 128)
	a.job.mu.Lock()
	history := append([]event(nil), a.job.history...)
	a.job.listeners[ch] = struct{}{}
	a.job.mu.Unlock()
	defer func() { a.job.mu.Lock(); delete(a.job.listeners, ch); a.job.mu.Unlock() }()
	send := func(e event) bool {
		b, _ := json.Marshal(e)
		_, err := w.Write(append(append([]byte("data: "), b...), []byte("\n\n")...))
		if err == nil {
			flusher.Flush()
		}
		return err == nil
	}
	for _, e := range history {
		if !send(e) {
			return
		}
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			if !send(e) {
				return
			}
		case <-ticker.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		}
	}
}
