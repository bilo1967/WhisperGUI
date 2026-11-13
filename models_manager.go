package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type catalogEntry struct{ ID, Label, Filename, Repository string }

func modelSupportsTranslation(filename string) bool {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(filename), ".bin"))
	// Whisper Turbo is trained for transcription, not translation.
	// English-only models cannot translate non-English input.
	return !strings.Contains(name, "turbo") && !strings.HasSuffix(name, ".en") && !strings.Contains(name, ".en-")
}

var officialModels = []string{"tiny", "tiny.en", "base", "base.en", "small", "small.en", "medium", "medium.en", "large-v1", "large-v2", "large-v3", "large-v3-turbo", "large-v3-turbo-q5_0"}

func entries() []catalogEntry {
	out := make([]catalogEntry, 0, len(officialModels)+1)
	for _, id := range officialModels {
		label := id
		if strings.HasSuffix(id, ".en") {
			label += " (English only)"
		} else {
			label += " (multilingual)"
		}
		out = append(out, catalogEntry{id, label, "ggml-" + id + ".bin", "ggerganov/whisper.cpp"})
	}
	out = append(out, catalogEntry{ID: "silero-vad-v6.2.0", Label: "Silero VAD 6.2.0 (silence detection)", Filename: "ggml-silero-v6.2.0.bin", Repository: "ggml-org/whisper-vad"})
	return out
}

type modelMeta struct {
	ETag string `json:"etag"`
	Size int64  `json:"size"`
}
type modelView struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Filename   string `json:"filename"`
	LocalSize  int64  `json:"local_size"`
	RemoteSize int64  `json:"remote_size"`
	Status     string `json:"status"`
	CanTranslate bool `json:"translation_supported"`
}
type modelTask struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	busy     bool
	message  string
	received int64
	total    int64
	percent  float64
}

var hfClient = &http.Client{Timeout: 20 * time.Second}

func modelURL(e catalogEntry) string {
	return "https://huggingface.co/" + e.Repository + "/resolve/main/" + e.Filename
}
func modelInfo(e catalogEntry) (modelMeta, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "HEAD", modelURL(e), nil)
	res, err := hfClient.Do(req)
	if err != nil {
		return modelMeta{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return modelMeta{}, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return modelMeta{ETag: strings.Trim(res.Header.Get("ETag"), "\""), Size: res.ContentLength}, nil
}
func findModel(id string) (catalogEntry, bool) {
	for _, m := range entries() {
		if m.ID == id {
			return m, true
		}
	}
	return catalogEntry{}, false
}
func (a *application) modelWritable() bool {
	if a.paths.Mode != "system" {
		return true
	}
	return isAdmin()
}
func (a *application) modelCatalog(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", 403)
		return
	}
	a.modelTask.mu.Lock()
	busy := a.modelTask.busy
	a.modelTask.mu.Unlock()
	es := entries()
	out := make([]modelView, len(es))
	var wg sync.WaitGroup
	for i, e := range es {
		wg.Add(1)
		go func(i int, e catalogEntry) {
			defer wg.Done()
			v := modelView{ID: e.ID, Label: e.Label, Filename: e.Filename, Status: "absent", CanTranslate: modelSupportsTranslation(e.Filename)}
			path := filepath.Join(a.paths.ModelsDir, e.Filename)
			if f, err := os.Stat(path); err == nil && !f.IsDir() {
				v.LocalSize = f.Size()
				v.Status = "unknown"
			}
			meta, err := modelInfo(e)
			if err == nil {
				v.RemoteSize = meta.Size
				if v.LocalSize > 0 {
					v.Status = "installed"
					if meta.Size > 0 && v.LocalSize != meta.Size {
						v.Status = "update"
					}
					if b, err := os.ReadFile(path + ".meta.json"); err == nil {
						var old modelMeta
						if json.Unmarshal(b, &old) == nil && old.ETag != "" && meta.ETag != "" && old.ETag != meta.ETag {
							v.Status = "update"
						}
					}
				}
			}
			out[i] = v
		}(i, e)
	}
	wg.Wait()
	jsonReply(w, map[string]any{"models": out, "busy": busy, "writable": a.modelWritable()})
}
func decodeModel(w http.ResponseWriter, r *http.Request) (catalogEntry, bool) {
	var input struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil {
		return catalogEntry{}, false
	}
	return findModel(input.ID)
}
func (a *application) beginTask(w http.ResponseWriter, r *http.Request) (catalogEntry, context.Context, bool) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", 403)
		return catalogEntry{}, nil, false
	}
	if !a.modelWritable() {
		http.Error(w, "Administrator privileges required", 403)
		return catalogEntry{}, nil, false
	}
	e, ok := decodeModel(w, r)
	if !ok {
		http.Error(w, "Invalid model", 400)
		return catalogEntry{}, nil, false
	}
	a.modelTask.mu.Lock()
	defer a.modelTask.mu.Unlock()
	if a.modelTask.busy {
		http.Error(w, "Operation already in progress", 409)
		return catalogEntry{}, nil, false
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.modelTask.busy = true
	a.modelTask.cancel = cancel
	a.modelTask.message = "Preparing " + e.Filename
	a.modelTask.received = 0
	a.modelTask.total = 0
	a.modelTask.percent = 0
	return e, ctx, true
}
func (a *application) finishTask(err error) {
	a.modelTask.mu.Lock()
	defer a.modelTask.mu.Unlock()
	a.modelTask.busy = false
	a.modelTask.cancel = nil
	if err != nil {
		a.modelTask.message = "Error: " + err.Error()
	} else {
		a.modelTask.message = "Operation completed"
	}
}
func (a *application) downloadModel(w http.ResponseWriter, r *http.Request) {
	e, ctx, ok := a.beginTask(w, r)
	if !ok {
		return
	}
	a.job.mu.Lock()
	jobBusy := a.job.cancel != nil
	a.job.mu.Unlock()
	if jobBusy {
		a.finishTask(errors.New("transcription in progress"))
		http.Error(w, "Operation unavailable during transcription", 409)
		return
	}
	go func() { err := a.download(ctx, e); a.finishTask(err) }()
	jsonReply(w, map[string]string{"status": "started"})
}
func (a *application) download(ctx context.Context, e catalogEntry) error {
	if err := os.MkdirAll(a.paths.ModelsDir, 0755); err != nil {
		return err
	}
	target := filepath.Join(a.paths.ModelsDir, e.Filename)
	// Exclusive temp file; only a complete verified transfer is installed.
	tmp, err := os.CreateTemp(a.paths.ModelsDir, ".whispergui-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	req, err := http.NewRequestWithContext(ctx, "GET", modelURL(e), nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 0}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("download HTTP %d", res.StatusCode)
	}
	total := res.ContentLength
	a.modelTask.mu.Lock()
	a.modelTask.total = total
	a.modelTask.message = "Downloading " + e.Filename
	a.modelTask.mu.Unlock()
	buf := make([]byte, 256*1024)
	var count int64
	for {
		n, readErr := res.Body.Read(buf)
		if n > 0 {
			written, e := tmp.Write(buf[:n])
			if e != nil {
				return e
			}
			count += int64(written)
			a.modelTask.mu.Lock()
			a.modelTask.received = count
			if total > 0 {
				a.modelTask.percent = 100 * float64(count) / float64(total)
			} else {
				a.modelTask.percent = -1
			}
			a.modelTask.mu.Unlock()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	if total > 0 && count != total {
		return errors.New("dimensione del download non corretta")
	}
	if count == 0 {
		return errors.New("file scaricato vuoto")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Backup of previous model for atomic replacement on Windows (Rename won't replace there).
	backup := target + ".old"
	hadOld := false
	if _, err := os.Stat(target); err == nil {
		_ = os.Remove(backup)
		if err := os.Rename(target, backup); err != nil {
			return err
		}
		hadOld = true
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		if hadOld {
			_ = os.Rename(backup, target)
		}
		return err
	}
	if hadOld {
		_ = os.Remove(backup)
	}
	meta := modelMeta{ETag: strings.Trim(res.Header.Get("ETag"), "\""), Size: count}
	b, _ := json.Marshal(meta)
	_ = os.WriteFile(target+".meta.json", b, 0644)
	return nil
}
func (a *application) deleteModel(w http.ResponseWriter, r *http.Request) {
	e, ctx, ok := a.beginTask(w, r)
	if !ok {
		return
	}
	_ = ctx
	a.job.mu.Lock()
	jobBusy := a.job.cancel != nil
	a.job.mu.Unlock()
	if jobBusy {
		a.finishTask(errors.New("transcription in progress"))
		http.Error(w, "Cannot delete models during transcription", 409)
		return
	}
	err := os.Remove(filepath.Join(a.paths.ModelsDir, e.Filename))
	if err == nil {
		_ = os.Remove(filepath.Join(a.paths.ModelsDir, e.Filename+".meta.json"))
	}
	a.finishTask(err)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	jsonReply(w, map[string]string{"status": "deleted"})
}
func (a *application) cancelModel(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", 403)
		return
	}
	a.modelTask.mu.Lock()
	c := a.modelTask.cancel
	a.modelTask.mu.Unlock()
	if c != nil {
		c()
	}
	jsonReply(w, map[string]string{"status": "ok"})
}
func (a *application) modelProgress(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Forbidden", 403)
		return
	}
	a.modelTask.mu.Lock()
	defer a.modelTask.mu.Unlock()
	jsonReply(w, map[string]any{"busy": a.modelTask.busy, "message": a.modelTask.message, "received": a.modelTask.received, "total": a.modelTask.total, "percent": a.modelTask.percent})
}
func isAdmin() bool {
	if runtime.GOOS != "windows" {
		return os.Geteuid() == 0
	}
	return windowsAdmin()
}
