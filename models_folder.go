package main

import (
	"net/http"
	"os"
)

// The browser never supplies a path. Only the configured models directory is opened.
func (a *application) openModelsFolder(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	info, err := os.Stat(a.paths.ModelsDir)
	if err != nil || !info.IsDir() {
		http.Error(w, "Models directory is not available", http.StatusNotFound)
		return
	}
	if err := openResultFile(a.paths.ModelsDir, false); err != nil {
		http.Error(w, "Unable to open models folder: "+err.Error(), http.StatusInternalServerError)
		return
	}
	jsonReply(w, map[string]string{"status": "ok"})
}
