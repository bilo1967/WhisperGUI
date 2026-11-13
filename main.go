package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ncruces/zenity"
)

const appVersion = "0.6.27"

//go:embed web/*
var webFiles embed.FS

type application struct {
	token               string
	host                string
	dialog              sync.Mutex
	paths               Paths
	config              Config
	configMu            sync.Mutex
	job                 work
	jobContext          context.Context
	selectedFile        string
	selectedMu          sync.Mutex
	modelTask           modelTask
	server              *http.Server
	clientMu            sync.Mutex
	connectedClients    int
	clientEverConnected bool
	lastClientGone      time.Time
	closing             atomic.Bool
	done                chan struct{}
	toolVersions        toolVersionInfo
}

func main() {
	release, err := acquireSingleInstance()
	if err != nil {
		log.Println("WhisperGUI is already running (or instance lock failed):", err)
		return
	}
	defer release()
	paths, err := resolvePaths()
	if err != nil {
		log.Fatal(err)
	}
	cfg, paths, err := loadConfig(paths)
	if err != nil {
		log.Fatal(err)
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Server.Port))
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close()

	a := &application{token: hex.EncodeToString(tokenBytes), host: listener.Addr().String(), paths: paths, config: cfg, done: make(chan struct{})}
	a.job.listeners = make(map[chan event]struct{})
	a.toolVersions = detectToolVersions(paths)
	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServer(http.FS(webRoot)))
	mux.HandleFunc("POST /api/select-file", a.selectFile)
	mux.HandleFunc("GET /api/status", a.status)
	mux.HandleFunc("GET /api/activity", a.activityStatus)
	mux.HandleFunc("POST /api/shutdown", a.shutdownRequest)
	mux.HandleFunc("POST /api/preferences/theme", a.updateTheme)
	mux.HandleFunc("POST /api/preferences/glossary", a.updateGlossary)
	mux.HandleFunc("POST /api/preferences/model", a.updateModel)
	mux.HandleFunc("POST /api/preferences/language", a.updateLanguage)
	mux.HandleFunc("GET /api/models", a.listModels)
	mux.HandleFunc("GET /api/model-catalog", a.modelCatalog)
	mux.HandleFunc("POST /api/models-folder", a.openModelsFolder)
	mux.HandleFunc("POST /api/model-download", a.downloadModel)
	mux.HandleFunc("POST /api/model-delete", a.deleteModel)
	mux.HandleFunc("POST /api/model-cancel", a.cancelModel)
	mux.HandleFunc("GET /api/model-progress", a.modelProgress)
	mux.HandleFunc("GET /api/client", a.clientSocket)
	mux.HandleFunc("POST /api/jobs", a.startJob)
	mux.HandleFunc("POST /api/stop", a.stopJob)
	mux.HandleFunc("POST /api/result", a.resultAction)
	mux.HandleFunc("GET /api/events", a.events)
	mux.HandleFunc("POST /api/log/clear", a.clearLog)
	server := &http.Server{
		Handler:           a.checkHost(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	a.server = server
	go a.monitorClient()
	url := "http://" + a.host + "/#" + a.token
	fmt.Println("WhisperGUI", appVersion)
	fmt.Println("whisper.cpp:", a.toolVersions.Whisper)
	fmt.Println("FFmpeg:", a.toolVersions.FFmpeg)
	fmt.Println("Indirizzo:", url)
	fmt.Println("Per terminare: Ctrl+C")
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Println("Server error:", err)
		}
	}()
	if cfg.Server.OpenBrowser {
		if err := openBrowser(url); err != nil {
			log.Println("Impossibile aprire il browser automaticamente; copia l'indirizzo mostrato sopra:", err)
		}
	}
	// Il listener resta attivo fino alla terminazione dell'eseguibile.
	<-a.shutdownSignal()
}

func (a *application) checkHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != a.host {
			http.Error(w, "Invalid host", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (a *application) selectFile(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Non autorizzato", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !a.dialog.TryLock() {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "File selection dialog already open"})
		return
	}
	defer a.dialog.Unlock()

	path, err := selectAudioFile()
	if errors.Is(err, zenity.ErrCanceled) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "cancelled"})
		return
	}
	if err != nil {
		log.Println("Dialog:", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "File cannot be accessed"})
		return
	}
	a.selectedMu.Lock()
	a.selectedFile = path
	a.selectedMu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok", "path": path, "name": info.Name(), "size": info.Size(),
	})
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		return openBrowserWindows(url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("piattaforma non supportata: %s", runtime.GOOS)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		if err := cmd.Wait(); err != nil && !strings.Contains(err.Error(), "exit status 1") {
			log.Println("Apertura browser:", err)
		}
	}()
	return nil
}

func (a *application) status(w http.ResponseWriter, r *http.Request) {
	if !a.authorized(r) {
		http.Error(w, "Non autorizzato", http.StatusForbidden)
		return
	}
	exists := func(path string) bool { info, err := os.Stat(path); return err == nil && !info.IsDir() }
	modelsInfo, err := os.Stat(a.paths.ModelsDir)
	modelsExists := err == nil && modelsInfo.IsDir()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"mode": a.paths.Mode, "config_file": a.paths.ConfigFile,
		"bin_dir": a.paths.BinDir, "models_dir": a.paths.ModelsDir,
		"models_dir_exists": modelsExists, "models_admin_managed": a.paths.Mode == "system",
		"ffmpeg": a.paths.FFmpeg, "ffmpeg_found": exists(a.paths.FFmpeg),
		"whisper": a.paths.Whisper, "whisper_found": exists(a.paths.Whisper),
		"config":      a.snapshotConfig(),
		"app_version": appVersion, "whisper_version": a.toolVersions.Whisper, "ffmpeg_version": a.toolVersions.FFmpeg,
	})
}
func (a *application) authorized(r *http.Request) bool {
	return (r.Header.Get("Origin") == "" || r.Header.Get("Origin") == "http://"+a.host) &&
		subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Session-Token")), []byte(a.token)) == 1
}

// Windows DLL dependencies should be kept alongside whisper-cli.exe in bin/.
