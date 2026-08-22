package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

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
	case "linux":
		// TODO: implement XDG_DATA_HOME (~/.local/share/runora) for Linux
		return "", fmt.Errorf("linux app data directory not yet implemented")
	case "darwin":
		// TODO: implement ~/Library/Application Support/runora for macOS
		return "", fmt.Errorf("macOS app data directory not yet implemented")
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
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
	Models      string `json:"models"`
	LlamaCPP    string `json:"llama_cpp"`
	OnnxRuntime string `json:"onnxruntime"`
	Profiles    string `json:"profiles"`
	Cache       string `json:"cache"`
	Benchmarks  string `json:"benchmarks"`
	Downloads   string `json:"downloads"`
}

type Config struct {
	Paths               Paths             `json:"paths"`
	Favorites           []string          `json:"favorites"`
	RecentLaunches      []string          `json:"recent_launches"`
	LastSelectedModel   string            `json:"last_selected_model"`
	Theme               string            `json:"theme"`
	ModelProfiles       map[string]string `json:"model_profiles"`
	ModelTasks          map[string]string `json:"model_tasks"`
	HFToken             string            `json:"hf_token"`
	GitHubToken         string            `json:"github_token"`
	OnboardingCompleted bool              `json:"onboarding_completed"`

	// configPath is the resolved path to config.json on disk.
	// It is not serialised to JSON.
	configPath string
}

const configFileName = "config.json"

// Load reads the configuration from the platform app data directory,
// creating it with defaults if it does not exist.
func Load() (*Config, error) {
	dir, err := AppDataDir()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("could not create app data directory: %w", err)
	}

	configPath := filepath.Join(dir, configFileName)

	var cfg *Config
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
		cfg = &Config{}
		if err := json.Unmarshal(data, cfg); err != nil {
			cfg = defaultConfig(dir)
			_ = cfg.Save()
		}
	}

	cfg.configPath = configPath

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
	return &Config{
		Paths: Paths{
			Models:      filepath.Join(dir, "models"),
			LlamaCPP:    filepath.Join(dir, "llama.cpp"),
			OnnxRuntime: filepath.Join(dir, "onnxruntime"),
			Profiles:    filepath.Join(dir, "profiles"),
			Cache:       filepath.Join(dir, "cache"),
			Benchmarks:  filepath.Join(dir, "benchmarks"),
			Downloads:   filepath.Join(dir, "downloads"),
		},
		Favorites:           []string{},
		RecentLaunches:      []string{},
		LastSelectedModel:   "",
		Theme:               "forest",
		ModelProfiles:       make(map[string]string),
		ModelTasks:          make(map[string]string),
		OnboardingCompleted: false,
		configPath:          filepath.Join(dir, configFileName),
	}
}

// Save writes the current configuration to disk.
func (c *Config) Save() error {
	if c.configPath == "" {
		dir, err := AppDataDir()
		if err != nil {
			return err
		}
		c.configPath = filepath.Join(dir, configFileName)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.configPath, data, 0600)
}

func (c *Config) CreateDirectories() error {
	dirs := []string{
		c.Paths.Models,
		c.Paths.LlamaCPP,
		c.Paths.OnnxRuntime,
		c.Paths.Profiles,
		c.Paths.Cache,
		c.Paths.Benchmarks,
		c.Paths.Downloads,
	}
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
