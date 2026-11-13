package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// removeManagedModels only removes catalogued model files, and does not
// recursively remove a potentially user-configured directory.
func removeManagedModels(dir string) (int, error) {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("models path is not a regular directory")
	}
	removed := 0
	for _, entry := range entries() {
		for _, name := range []string{entry.Filename, entry.Filename + ".meta.json"} {
			path := filepath.Join(dir, name)
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return removed, err
			}
			if !info.Mode().IsRegular() {
				return removed, fmt.Errorf("refusing to remove non-regular file %q", path)
			}
			if err := os.Remove(path); err != nil {
				return removed, err
			}
			removed++
		}
	}
	// Only prune an empty models directory; never delete unrelated files.
	_ = os.Remove(dir)
	return removed, nil
}

func removeApplicationConfig(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to remove non-regular config file")
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	// Remove the configuration directory only if it is now empty.
	_ = os.Remove(filepath.Dir(path))
	return nil
}

func (a *application) removeApplicationData(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Models       bool   `json:"models"`
		Config       bool   `json:"config"`
		Confirmation string `json:"confirmation"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Confirmation != "REMOVE" || (!req.Models && !req.Config) {
		http.Error(w, "Explicit confirmation and a selection are required", http.StatusBadRequest)
		return
	}
	if a.workInProgress() {
		http.Error(w, "Stop transcription and model downloads before removing data", http.StatusConflict)
		return
	}
	if req.Models {
		// Shared installations require administrator rights for model changes.
		if !a.modelWritable() {
			http.Error(w, "Administrator privileges are required to remove system models", http.StatusForbidden)
			return
		}
		if strings.TrimSpace(a.paths.ModelsDir) == "" || filepath.Clean(a.paths.ModelsDir) == filepath.Clean(a.paths.ExecutableDir) {
			http.Error(w, "Unsafe model directory", http.StatusBadRequest)
			return
		}
		if _, err := removeManagedModels(a.paths.ModelsDir); err != nil {
			http.Error(w, "Could not remove models: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if req.Config {
		if err := removeApplicationConfig(a.paths.ConfigFile); err != nil {
			http.Error(w, "Could not remove configuration: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	jsonReply(w, map[string]string{"status": "removed"})
	// Respond before shutting down the server; no settings are saved afterward.
	time.AfterFunc(300*time.Millisecond, a.shutdown)
}
