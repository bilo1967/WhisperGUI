package main

import (
	"net/http"
	"time"
)

// Explicit shutdown is authorized by the session token, just like other API calls.
// Respond before stopping HTTP so the browser can display its final message.
func (a *application) shutdownRequest(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if a.closing.Load() {
		jsonReply(w, map[string]string{"status": "closing"})
		return
	}
	jsonReply(w, map[string]string{"status": "closing"})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() { time.Sleep(200 * time.Millisecond); a.shutdown() }()
}

func (a *application) activityStatus(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	a.job.mu.Lock()
	job := a.job.cancel != nil
	a.job.mu.Unlock()
	a.modelTask.mu.Lock()
	model := a.modelTask.busy
	a.modelTask.mu.Unlock()
	jsonReply(w, map[string]bool{"transcription": job, "model_download": model})
}
