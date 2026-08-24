package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/BIJJUDAMA/runora/credentials"
)

// Version is the current semantic release version of Runora.
const Version = "v2.1.1"

// AppDataDir returns the fixed directory where runora stores all its data.
// Each supported platform places data in its conventional location.
func AppDataDir() (string, error) {
	var base string
	var err error

	switch runtime.GOOS {
	case "windows":
		base, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("could not determine config directory: %w", err)
		}
	case "darwin":
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("could not determine user home directory: %w", homeErr)
		}
		base = filepath.Join(home, "Library", "Application Support")
	case "linux":
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			base = xdg
		} else {
			home, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return "", fmt.Errorf("could not determine user home directory: %w", homeErr)
			}
			base = filepath.Join(home, ".config")
		}
	default:
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("could not determine user home directory: %w", homeErr)
		}
		base = filepath.Join(home, ".config")
	}

	oldPath := filepath.Join(base, "llmgr")
	newPath := filepath.Join(base, "runora")

	// Migration check: if new path does not exist, but old path exists, rename it.
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		if _, err := os.Stat(oldPath); err == nil {
			_ = os.Rename(oldPath, newPath)
		}
	}

	return newPath, nil
}

type Paths struct {
	Models           string   `json:"models"`
	ModelDirectories []string `json:"model_directories,omitempty"`
	LlamaCPP         string   `json:"llama_cpp"`
	OnnxRuntime      string   `json:"onnxruntime"`
	Profiles         string   `json:"profiles"`
	Cache            string   `json:"cache"`
	Benchmarks       string   `json:"benchmarks"`
	Downloads        string   `json:"downloads"`
}

// AllModelDirectories returns a deduplicated list of all configured model directory paths,
// starting with the primary Models directory followed by any secondary ModelDirectories.
func (p *Paths) AllModelDirectories() []string {
	var dirs []string
	seen := make(map[string]bool)

	addDir := func(d string) {
		trimmed := strings.TrimSpace(d)
		if trimmed == "" {
			return
		}
		clean := filepath.Clean(trimmed)
		if !seen[clean] {
			seen[clean] = true
			dirs = append(dirs, clean)
		}
	}

	addDir(p.Models)
	for _, d := range p.ModelDirectories {
		addDir(d)
	}

	return dirs
}

type ModelSource struct {
	Type         string    `json:"type"`
	Name         string    `json:"name"`
	Enabled      bool      `json:"enabled"`
	CustomPath   string    `json:"custom_path,omitempty"`
	DetectedPath string    `json:"detected_path,omitempty"`
	Detected     bool      `json:"detected"`
	LastScanTime time.Time `json:"last_scan_time,omitempty"`
	ModelCount   int       `json:"model_count"`
}

type Config struct {
	Paths               Paths             `json:"paths"`
	Favorites           []string          `json:"favorites"`
	RecentLaunches      []string          `json:"recent_launches"`
	LastSelectedModel   string            `json:"last_selected_model"`
	Theme               string            `json:"theme"`
	ModelProfiles       map[string]string `json:"model_profiles"`
	ModelTasks          map[string]string `json:"model_tasks"`
	ModelSources        []ModelSource     `json:"model_sources,omitempty"`
	HFToken             string            `json:"-"`
	GitHubToken         string            `json:"-"`
	HuggingFaceToken    string            `json:"-"`
	OnboardingCompleted bool              `json:"onboarding_completed"`

	// configPath is the resolved path to config.json on disk.
	// It is not serialised to JSON.
	configPath string
}

const configFileName = "config.json"

// AtomicWriteFile writes data to a temporary file in the same directory as filename,
// calls Sync() to flush to disk, closes the file, and performs an atomic rename.
func AtomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("could not create directory %s: %w", dir, err)
	}

	tmpFile, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(filename)+"-*")
	if err != nil {
		return fmt.Errorf("could not create temp file in %s: %w", dir, err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("could not write to temp file %s: %w", tmpName, err)
	}

	_ = tmpFile.Chmod(perm)

	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("could not sync temp file %s: %w", tmpName, err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("could not close temp file %s: %w", tmpName, err)
	}

	if err := os.Rename(tmpName, filename); err != nil {
		if runtime.GOOS == "windows" {
			var rerr error
			for attempt := 0; attempt < 10; attempt++ {
				_ = os.Remove(filename)
				rerr = os.Rename(tmpName, filename)
				if rerr == nil {
					return nil
				}
				time.Sleep(time.Duration(10*(attempt+1)) * time.Millisecond)
			}
			return fmt.Errorf("could not atomic rename %s to %s: %w", tmpName, filename, rerr)
		}
		return fmt.Errorf("could not atomic rename %s to %s: %w", tmpName, filename, err)
	}

	return nil
}

// Load reads the configuration from the platform app data directory,
// creating it with defaults if it does not exist.
func Load() (*Config, error) {
	dir, err := AppDataDir()
	if err != nil {
		return nil, err
	}
	return LoadFromDir(dir)
}

// LoadFromDir reads configuration rooted at dir, recovering from corrupted configs automatically.
func LoadFromDir(dir string) (*Config, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("could not create app data directory: %w", err)
	}

	configPath := filepath.Join(dir, configFileName)

	var cfg *Config
	defaults := defaultConfig(dir)
	var needsResaveAfterMigration bool

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg = defaultConfig(dir)
		if err := cfg.Save(); err != nil {
			return nil, err
		}
	} else {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, err
		}
		cfg = defaultConfig(dir)
		if err := json.Unmarshal(data, cfg); err != nil {
			// Corrupted config detected: backup corrupted config before generating defaults
			timestamp := time.Now().Format("20060102150405")
			corruptedPath := filepath.Join(dir, fmt.Sprintf("config.corrupted.%s.json", timestamp))
			_ = os.WriteFile(corruptedPath, data, 0600)

			cfg = defaultConfig(dir)
			_ = cfg.Save()
		} else {
			// Check for legacy plaintext tokens in raw json and migrate to OS keyring
			var rawMap map[string]interface{}
			if err := json.Unmarshal(data, &rawMap); err == nil {
				if gh, ok := rawMap["github_token"].(string); ok && strings.TrimSpace(gh) != "" {
					_ = credentials.Set(credentials.ProviderGitHub, gh)
					needsResaveAfterMigration = true
				}
				if hf, ok := rawMap["huggingface_token"].(string); ok && strings.TrimSpace(hf) != "" {
					_ = credentials.Set(credentials.ProviderHuggingFace, hf)
					needsResaveAfterMigration = true
				} else if hfOld, ok := rawMap["hf_token"].(string); ok && strings.TrimSpace(hfOld) != "" {
					_ = credentials.Set(credentials.ProviderHuggingFace, hfOld)
					needsResaveAfterMigration = true
				}
			}
		}
	}

	cfg.configPath = configPath

	// Backfill any missing or empty paths/settings from defaults
	if cfg.Paths.Models == "" {
		cfg.Paths.Models = defaults.Paths.Models
	}
	if cfg.Paths.LlamaCPP == "" {
		cfg.Paths.LlamaCPP = defaults.Paths.LlamaCPP
	}
	if cfg.Paths.OnnxRuntime == "" {
		cfg.Paths.OnnxRuntime = defaults.Paths.OnnxRuntime
	}
	if cfg.Paths.Profiles == "" {
		cfg.Paths.Profiles = defaults.Paths.Profiles
	}
	if cfg.Paths.Cache == "" {
		cfg.Paths.Cache = defaults.Paths.Cache
	}
	if cfg.Paths.Benchmarks == "" {
		cfg.Paths.Benchmarks = defaults.Paths.Benchmarks
	}
	if cfg.Paths.Downloads == "" {
		cfg.Paths.Downloads = defaults.Paths.Downloads
	}
	if cfg.Paths.ModelDirectories == nil {
		cfg.Paths.ModelDirectories = []string{}
	}
	if cfg.Theme == "" {
		cfg.Theme = "forest"
	}

	if cfg.ModelProfiles == nil {
		cfg.ModelProfiles = make(map[string]string)
	}
	if cfg.ModelTasks == nil {
		cfg.ModelTasks = make(map[string]string)
	}
	if cfg.Favorites == nil {
		cfg.Favorites = []string{}
	}
	if cfg.RecentLaunches == nil {
		cfg.RecentLaunches = []string{}
	}

	// Load active API tokens from OS Keyring (fallback to environment variables)
	ghToken, _ := credentials.Get(credentials.ProviderGitHub)
	if ghToken != "" {
		cfg.GitHubToken = ghToken
	} else if defaults.GitHubToken != "" {
		cfg.GitHubToken = defaults.GitHubToken
	}

	hfToken, _ := credentials.Get(credentials.ProviderHuggingFace)
	if hfToken != "" {
		cfg.HuggingFaceToken = hfToken
		cfg.HFToken = hfToken
	} else if defaults.HuggingFaceToken != "" {
		cfg.HuggingFaceToken = defaults.HuggingFaceToken
		cfg.HFToken = defaults.HuggingFaceToken
	}

	if needsResaveAfterMigration {
		_ = cfg.Save()
	}

	if err := cfg.CreateDirectories(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// DefaultConfig returns a Config with default values rooted at the platform
// app data directory. Exported so it can be used in tests and tooling.
func DefaultConfig() *Config {
	dir, err := AppDataDir()
	if err != nil {
		// Fallback: callers that cannot resolve the platform dir get an
		// empty-rooted config. Tests should use defaultConfig(dir) directly.
		dir = ""
	}
	return defaultConfig(dir)
}

// defaultConfig returns a Config with all paths rooted at dir.
func defaultConfig(dir string) *Config {
	ghToken, _ := credentials.Get(credentials.ProviderGitHub)
	if ghToken == "" {
		ghToken = os.Getenv("GITHUB_TOKEN")
		if ghToken == "" {
			ghToken = os.Getenv("GH_TOKEN")
		}
	}

	hfToken, _ := credentials.Get(credentials.ProviderHuggingFace)
	if hfToken == "" {
		hfToken = os.Getenv("HF_TOKEN")
		if hfToken == "" {
			hfToken = os.Getenv("HUGGING_FACE_HUB_TOKEN")
		}
	}

	return &Config{
		Paths: Paths{
			Models:           filepath.Join(dir, "models"),
			ModelDirectories: []string{},
			LlamaCPP:         filepath.Join(dir, "llama.cpp"),
			OnnxRuntime:      filepath.Join(dir, "onnxruntime"),
			Profiles:         filepath.Join(dir, "profiles"),
			Cache:            filepath.Join(dir, "cache"),
			Benchmarks:       filepath.Join(dir, "benchmarks"),
			Downloads:        filepath.Join(dir, "downloads"),
		},
		Favorites:           []string{},
		RecentLaunches:      []string{},
		LastSelectedModel:   "",
		Theme:               "forest",
		ModelProfiles:       make(map[string]string),
		ModelTasks:          make(map[string]string),
		HFToken:             hfToken,
		GitHubToken:         ghToken,
		HuggingFaceToken:    hfToken,
		OnboardingCompleted: false,
		configPath:          filepath.Join(dir, configFileName),
	}
}

// Save writes the current configuration to disk atomically.
func (c *Config) Save() error {
	if c.configPath == "" {
		dir, err := AppDataDir()
		if err != nil {
			return err
		}
		c.configPath = filepath.Join(dir, configFileName)
	}

	// Persist tokens securely to OS keyring
	if c.GitHubToken != "" {
		_ = credentials.Set(credentials.ProviderGitHub, c.GitHubToken)
	}
	if c.HuggingFaceToken != "" {
		_ = credentials.Set(credentials.ProviderHuggingFace, c.HuggingFaceToken)
	} else if c.HFToken != "" {
		_ = credentials.Set(credentials.ProviderHuggingFace, c.HFToken)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWriteFile(c.configPath, data, 0600)
}

func (c *Config) CreateDirectories() error {
	dirs := append([]string{
		c.Paths.Models,
		c.Paths.LlamaCPP,
		c.Paths.OnnxRuntime,
		c.Paths.Profiles,
		c.Paths.Cache,
		c.Paths.Benchmarks,
		c.Paths.Downloads,
	}, c.Paths.ModelDirectories...)
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// ToggleFavorite adds or removes a model path from Favorites.
func (c *Config) ToggleFavorite(modelPath string) {
	for i, f := range c.Favorites {
		if f == modelPath {
			c.Favorites = append(c.Favorites[:i], c.Favorites[i+1:]...)
			return
		}
	}
	c.Favorites = append(c.Favorites, modelPath)
}

// IsFavorite returns true if the model path is in Favorites.
func (c *Config) IsFavorite(modelPath string) bool {
	for _, f := range c.Favorites {
		if f == modelPath {
			return true
		}
	}
	return false
}

// RecordLaunch prepends the model path to RecentLaunches, capped at 5 unique items.
func (c *Config) RecordLaunch(modelPath string) {
	for i, r := range c.RecentLaunches {
		if r == modelPath {
			c.RecentLaunches = append(c.RecentLaunches[:i], c.RecentLaunches[i+1:]...)
			break
		}
	}
	c.RecentLaunches = append([]string{modelPath}, c.RecentLaunches...)
	if len(c.RecentLaunches) > 5 {
		c.RecentLaunches = c.RecentLaunches[:5]
	}
}

// DetectDefaultModelSources detects standard default directories for Ollama and LM Studio.
func DetectDefaultModelSources() []ModelSource {
	var sources []ModelSource

	home, _ := os.UserHomeDir()

	// 1. Ollama
	var ollamaPaths []string
	if env := os.Getenv("OLLAMA_MODELS"); env != "" {
		ollamaPaths = append(ollamaPaths, filepath.Clean(env))
	}
	if home != "" {
		ollamaPaths = append(ollamaPaths, filepath.Join(home, ".ollama", "models"))
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			ollamaPaths = append(ollamaPaths, filepath.Join(local, "Ollama", "models"))
		}
		if prof := os.Getenv("USERPROFILE"); prof != "" {
			ollamaPaths = append(ollamaPaths, filepath.Join(prof, ".ollama", "models"))
		}
	} else {
		ollamaPaths = append(ollamaPaths, "/usr/share/ollama/.ollama/models")
		ollamaPaths = append(ollamaPaths, "/var/lib/ollama/.ollama/models")
	}

	ollamaDetectedPath := ""
	for _, p := range ollamaPaths {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			ollamaDetectedPath = p
			break
		}
	}
	sources = append(sources, ModelSource{
		Type:         "ollama",
		Name:         "Ollama",
		Enabled:      ollamaDetectedPath != "",
		Detected:     ollamaDetectedPath != "",
		DetectedPath: ollamaDetectedPath,
	})

	// 2. LM Studio
	var lmPaths []string
	if env := os.Getenv("LM_STUDIO_MODELS"); env != "" {
		lmPaths = append(lmPaths, filepath.Clean(env))
	}
	if home != "" {
		lmPaths = append(lmPaths, filepath.Join(home, ".cache", "lm-studio", "models"))
		lmPaths = append(lmPaths, filepath.Join(home, ".lmstudio", "models"))
	}
	if runtime.GOOS == "windows" {
		if app := os.Getenv("APPDATA"); app != "" {
			lmPaths = append(lmPaths, filepath.Join(app, "LM Studio", "models"))
		}
		if prof := os.Getenv("USERPROFILE"); prof != "" {
			lmPaths = append(lmPaths, filepath.Join(prof, ".cache", "lm-studio", "models"))
			lmPaths = append(lmPaths, filepath.Join(prof, ".lmstudio", "models"))
		}
	}

	lmDetectedPath := ""
	for _, p := range lmPaths {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			lmDetectedPath = p
			break
		}
	}
	sources = append(sources, ModelSource{
		Type:         "lmstudio",
		Name:         "LM Studio",
		Enabled:      lmDetectedPath != "",
		Detected:     lmDetectedPath != "",
		DetectedPath: lmDetectedPath,
	})

	return sources
}
