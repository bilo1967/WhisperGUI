package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (a *application) snapshotConfig() Config {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	return a.config
}

func (a *application) updateTheme(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Theme string `json:"theme"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Theme != "light" && req.Theme != "dark" {
		http.Error(w, "Invalid theme", http.StatusBadRequest)
		return
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	updated := a.config
	updated.UI.Theme = req.Theme
	if err := saveConfig(a.paths.ConfigFile, updated); err != nil {
		http.Error(w, "Unable to save configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}
	a.config = updated
	jsonReply(w, map[string]string{"theme": req.Theme})
}

// updateGlossary saves advanced preferences; transcription hints remain session-only.
func (a *application) updateGlossary(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Enabled              bool    `json:"enabled"`
		VADEnabled           bool    `json:"vad_enabled"`
		ExperimentalFeatures bool    `json:"experimental_features"`
		Temperature          float64 `json:"temperature"`
		PackageZIP           bool    `json:"package_zip"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Temperature < 0 || req.Temperature > 1 {
		http.Error(w, "Temperature must be between 0 and 1", http.StatusBadRequest)
		return
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	updated := a.config
	updated.Whisper.GlossaryEnabled = req.Enabled
	updated.Whisper.GlossaryHint = ""
	updated.Whisper.Temperature = req.Temperature
	updated.Output.PackageZIP = req.PackageZIP
	updated.UI.ExperimentalFeatures = req.ExperimentalFeatures
	updated.Whisper.VADEnabled = req.VADEnabled
	if err := saveConfig(a.paths.ConfigFile, updated); err != nil {
		http.Error(w, "Unable to save configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}
	a.config = updated
	jsonReply(w, map[string]any{"enabled": req.Enabled, "vad_enabled": req.VADEnabled, "experimental_features": req.ExperimentalFeatures, "temperature": req.Temperature, "package_zip": req.PackageZIP})
}

// updateModel persists the most recently selected ASR model.
func (a *application) updateModel(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Model == "" || req.Model != filepath.Base(req.Model) || !strings.HasSuffix(strings.ToLower(req.Model), ".bin") || strings.HasPrefix(strings.ToLower(req.Model), "ggml-silero-") {
		http.Error(w, "Invalid ASR model", http.StatusBadRequest)
		return
	}
	info, err := os.Stat(filepath.Join(a.paths.ModelsDir, req.Model))
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "Model not installed", http.StatusBadRequest)
		return
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	updated := a.config
	updated.Whisper.Model = req.Model
	if err := saveConfig(a.paths.ConfigFile, updated); err != nil {
		http.Error(w, "Unable to save model: "+err.Error(), http.StatusInternalServerError)
		return
	}
	a.config = updated
	jsonReply(w, map[string]string{"model": req.Model})
}

// updateLanguage remembers the most recently selected transcription language.
func (a *application) updateLanguage(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Language string `json:"language"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	a.configMu.Lock()
	defer a.configMu.Unlock()
	valid := false
	for _, lang := range a.config.Languages {
		if req.Language == lang.Code {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "Invalid language", http.StatusBadRequest)
		return
	}
	updated := a.config
	updated.Whisper.Language = req.Language
	if err := saveConfig(a.paths.ConfigFile, updated); err != nil {
		http.Error(w, "Unable to save language: "+err.Error(), http.StatusInternalServerError)
		return
	}
	a.config = updated
	jsonReply(w, map[string]string{"language": req.Language})
}
