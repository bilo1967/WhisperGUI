package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ConfigVersion int `yaml:"config_version" json:"config_version"`
	Paths         struct {
		Models string `yaml:"models" json:"models"`
		Output string `yaml:"output" json:"output"`
	} `yaml:"paths" json:"paths"`
	Runtime struct {
		Backend    string `yaml:"backend" json:"backend"`
		AutoUpdate bool   `yaml:"auto_update" json:"auto_update"`
	} `yaml:"runtime" json:"runtime"`
	UI struct {
		Theme                string `yaml:"theme" json:"theme"`
		ExperimentalFeatures bool   `yaml:"experimental_features" json:"experimental_features"`
	} `yaml:"ui" json:"ui"`
	Languages []LanguageOption `yaml:"languages" json:"languages"`
	Whisper   struct {
		Model           string  `yaml:"model" json:"model"`
		Language        string  `yaml:"language" json:"language"`
		Translate       bool    `yaml:"translate" json:"translate"`
		GlossaryEnabled bool    `yaml:"glossary_enabled" json:"glossary_enabled"`
		GlossaryHint    string  `yaml:"glossary_hint,omitempty" json:"-"` // Legacy setting, discarded when loading
		VADEnabled      bool    `yaml:"vad_enabled" json:"vad_enabled"`
		Temperature     float64 `yaml:"temperature" json:"temperature"`
		Threads         int     `yaml:"threads" json:"threads"`
	} `yaml:"whisper" json:"whisper"`
	Output struct {
		Formats        []string `yaml:"formats" json:"formats"`
		TimestampedTXT bool     `yaml:"timestamped_txt" json:"timestamped_txt"`
		PackageZIP     bool     `yaml:"package_zip" json:"package_zip"`
	} `yaml:"output" json:"output"`
	Server struct {
		Port                   int  `yaml:"port" json:"port"`
		OpenBrowser            bool `yaml:"open_browser" json:"open_browser"`
		DisconnectGraceSeconds int  `yaml:"disconnect_grace_seconds" json:"disconnect_grace_seconds"`
	} `yaml:"server" json:"server"`
}

type LanguageOption struct {
	Code  string `yaml:"code" json:"code"`
	Label string `yaml:"label" json:"label"`
}

type Paths struct {
	Mode           string `json:"mode"`
	ExecutableDir  string `json:"executable_dir"`
	ConfigFile     string `json:"config_file"`
	BinDir         string `json:"bin_dir"`
	ModelsDir      string `json:"models_dir"`
	FFmpeg         string `json:"ffmpeg"`
	Whisper        string `json:"whisper"`
	ModelsWritable bool   `json:"models_writable_by_policy"`
}

type installSpec struct {
	Mode string `yaml:"mode"`
}

func defaultConfig() Config {
	var c Config
	c.ConfigVersion = 1
	c.Languages = []LanguageOption{{"auto", "Automatic"}, {"it", "Italian"}, {"en", "English"}, {"fr", "French"}, {"de", "German"}, {"es", "Spanish"}, {"pt", "Portuguese"}, {"ru", "Russian"}, {"pl", "Polish"}, {"uk", "Ukrainian"}, {"ar", "Arabic"}, {"ja", "Japanese"}, {"zh", "Chinese"}}
	c.Runtime.Backend = "auto"
	c.UI.Theme = "light"
	c.Whisper.Model = "base"
	c.Whisper.Language = "auto"
	c.Output.Formats = []string{"txt", "srt", "vtt", "json"}
	c.Output.TimestampedTXT = true
	c.Output.PackageZIP = true
	c.Server.OpenBrowser = true
	c.Server.DisconnectGraceSeconds = 5
	return c
}

func resolvePaths() (Paths, error) {
	exe, err := os.Executable()
	if err != nil {
		return Paths{}, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return Paths{}, err
	}
	dir := filepath.Dir(exe)
	p := Paths{ExecutableDir: dir, BinDir: filepath.Join(dir, "bin")}
	p.FFmpeg = filepath.Join(p.BinDir, binaryName("ffmpeg"))
	p.Whisper = filepath.Join(p.BinDir, binaryName("whisper-cli"))
	_, flagErr := os.Stat(filepath.Join(dir, "portable.flag"))
	switch {
	case flagErr == nil:
		p.Mode = "portable"
	case os.IsNotExist(flagErr):
		data, err := os.ReadFile(filepath.Join(dir, "install.yaml"))
		if err == nil {
			var spec installSpec
			if err = yaml.Unmarshal(data, &spec); err != nil {
				return p, fmt.Errorf("install.yaml: %w", err)
			}
			switch spec.Mode {
			case "system", "user":
				p.Mode = spec.Mode
			default:
				return p, fmt.Errorf("install.yaml: mode deve essere system oppure user")
			}
		} else if os.IsNotExist(err) {
			p.Mode = "user"
		} else {
			return p, err
		}
	default:
		return p, flagErr
	}
	if p.Mode == "portable" {
		p.ConfigFile = filepath.Join(dir, "config.yaml")
		p.ModelsDir = filepath.Join(dir, "models")
	} else {
		cfgHome, err := os.UserConfigDir()
		if err != nil {
			return p, err
		}
		p.ConfigFile = filepath.Join(cfgHome, "WhisperGUI", "config.yaml")
		if p.Mode == "system" {
			switch runtime.GOOS {
			case "windows":
				base := os.Getenv("PROGRAMDATA")
				if base == "" {
					return p, fmt.Errorf("PROGRAMDATA non definita")
				}
				p.ModelsDir = filepath.Join(base, "WhisperGUI", "models")
			case "darwin":
				p.ModelsDir = "/Library/Application Support/WhisperGUI/models"
			default:
				p.ModelsDir = "/var/lib/whispergui/models"
			}
		} else {
			cache, err := os.UserCacheDir()
			if err != nil {
				return p, err
			}
			if runtime.GOOS == "windows" {
				p.ModelsDir = filepath.Join(cache, "WhisperGUI", "models")
			} else {
				home, err := os.UserHomeDir()
				if err != nil {
					return p, err
				}
				p.ModelsDir = filepath.Join(home, ".local", "share", "whispergui", "models")
			}
		}
	}
	p.ModelsWritable = p.Mode != "system"
	return p, nil
}
func binaryName(s string) string {
	if runtime.GOOS == "windows" {
		return s + ".exe"
	}
	return s
}

func loadConfig(p Paths) (Config, Paths, error) {
	c := defaultConfig()
	b, err := os.ReadFile(p.ConfigFile)
	if os.IsNotExist(err) {
		if err := saveConfig(p.ConfigFile, c); err != nil {
			return c, p, err
		}
	} else if err != nil {
		return c, p, err
	} else if err := strictYAML(b, &c); err != nil {
		return c, p, fmt.Errorf("config.yaml: %w", err)
	}
	// Preserve the new default for configurations written by older versions.
	if len(b) > 0 {
		var raw struct {
			Output map[string]any `yaml:"output"`
		}
		if err := yaml.Unmarshal(b, &raw); err == nil {
			if _, present := raw.Output["package_zip"]; !present {
				c.Output.PackageZIP = true
			}
		}
	}
	if c.Server.DisconnectGraceSeconds < 1 || c.Server.DisconnectGraceSeconds > 120 {
		return c, p, fmt.Errorf("server.disconnect_grace_seconds must be between 1 and 120")
	}
	// Never retain a transcription-specific prompt from previous versions.
	c.Whisper.GlossaryHint = ""
	if c.Whisper.Temperature < 0 || c.Whisper.Temperature > 1 {
		return c, p, fmt.Errorf("whisper.temperature must be between 0 and 1")
	}
	if c.UI.Theme == "" {
		c.UI.Theme = "light"
	}
	if c.UI.Theme != "light" && c.UI.Theme != "dark" {
		return c, p, fmt.Errorf("invalid ui.theme: expected light or dark")
	}
	if c.ConfigVersion != 1 {
		return c, p, fmt.Errorf("config_version non supportata: %d", c.ConfigVersion)
	}
	if len(c.Languages) == 0 {
		c.Languages = defaultConfig().Languages
	}
	for _, lang := range c.Languages {
		if lang.Code == "" || lang.Label == "" || len(lang.Code) > 12 {
			return c, p, fmt.Errorf("lingua non valida nel file YAML")
		}
	}
	if c.Server.Port < 0 || c.Server.Port > 65535 {
		return c, p, fmt.Errorf("porta HTTP non valida")
	}
	if strings.TrimSpace(c.Paths.Models) != "" {
		if filepath.IsAbs(c.Paths.Models) {
			p.ModelsDir = filepath.Clean(c.Paths.Models)
		} else {
			p.ModelsDir = filepath.Join(p.ExecutableDir, c.Paths.Models)
		}
	}
	// Non scriviamo modelli in una cartella condivisa: il provisioning e' responsabilita' dell'installer/amministratore.
	return c, p, nil
}
func saveConfig(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// A prompt is session-specific and must not be written to disk.
	c.Whisper.GlossaryHint = ""
	b, err := yaml.Marshal(&c)
	if err == nil {
		b = append([]byte("# Windows: per i percorsi usa C:/Cartella/file oppure 'C:\\Cartella\\file' (apici singoli).\n# Le stringhe YAML con doppi apici interpretano i backslash come sequenze di escape.\n"), b...)
	}
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func strictYAML(b []byte, dst any) error {
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	return d.Decode(dst)
}
