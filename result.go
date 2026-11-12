package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Result actions only operate on the archive created by the latest completed job.
// No client-provided filesystem path is accepted.
func (a *application) resultAction(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", 400)
		return
	}
	if req.Action != "show" && req.Action != "delete" {
		http.Error(w, "Invalid action", 400)
		return
	}
	a.job.mu.Lock()
	path := a.job.archive
	if path == "" || a.job.state != "done" {
		a.job.mu.Unlock()
		http.Error(w, "No completed archive available", 404)
		return
	}
	// Keep the result locked while the action is performed, to prevent a race
	// with the start of a new transcription.
	if req.Action == "delete" {
		err := os.Remove(path)
		if err == nil {
			a.job.archive = ""
		}
		a.job.mu.Unlock()
		if err != nil {
			http.Error(w, "Unable to delete archive: "+err.Error(), 500)
			return
		}
		a.job.emit(event{Type: "archive-deleted", Message: "Archive deleted."})
		jsonReply(w, map[string]string{"status": "deleted"})
		return
	}
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		a.job.mu.Unlock()
		http.Error(w, "Archive is no longer available", 404)
		return
	}
	err := openResultFile(path, req.Action == "show")
	a.job.mu.Unlock()
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to open archive: %v", err), 500)
		return
	}
	jsonReply(w, map[string]string{"status": "ok"})
}

func openResultFile(path string, show bool) error {
	switch runtime.GOOS {
	case "windows":
		return openWindowsResult(path, show)
	case "darwin":
		if show {
			return exec.Command("open", "-R", path).Start()
		}
		return exec.Command("open", path).Start()
	default:
		if show {
			return exec.Command("xdg-open", filepath.Dir(path)).Start()
		}
		return exec.Command("xdg-open", path).Start()
	}
}
