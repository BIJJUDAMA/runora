package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// loadWithDir loads config rooted at dir, bypassing the platform AppDataDir.
func loadWithDir(dir string) (*Config, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
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
		cfg = defaultConfig(dir)
		_ = json.Unmarshal(data, cfg)
	}

	cfg.configPath = configPath

	if cfg.ModelProfiles == nil {
		cfg.ModelProfiles = make(map[string]string)
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

func TestConfigLoadAndSave(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "runora-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg, err := loadWithDir(tempDir)
	if err != nil {
		t.Fatalf("expected no error loading config, got %v", err)
	}

	// Paths should be absolute and rooted under tempDir
	expectedModels := filepath.Join(tempDir, "models")
	if cfg.Paths.Models != expectedModels {
		t.Errorf("expected paths.models to be %q, got %q", expectedModels, cfg.Paths.Models)
	}

	// All data directories should have been created under tempDir
	for _, sub := range []string{"models", "llama.cpp", "profiles", "cache", "benchmarks", "downloads"} {
		full := filepath.Join(tempDir, sub)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Errorf("expected directory %q to be created, but it was not", full)
		}
	}

	// Modify and save
	cfg.Theme = "light"
	cfg.Favorites = append(cfg.Favorites, "test-model")
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Reload and verify
	reloaded, err := loadWithDir(tempDir)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if reloaded.Theme != "light" {
		t.Errorf("expected theme to be 'light', got %q", reloaded.Theme)
	}
	if len(reloaded.Favorites) != 1 || reloaded.Favorites[0] != "test-model" {
		t.Errorf("expected favorites to contain 'test-model', got %v", reloaded.Favorites)
	}
}

func TestConfigHelpers(t *testing.T) {
	cfg := defaultConfig("")

	// Favorites
	modelPath := "models/Qwen/qwen2.5.gguf"
	if cfg.IsFavorite(modelPath) {
		t.Errorf("expected model to not be favorite initially")
	}
	cfg.ToggleFavorite(modelPath)
	if !cfg.IsFavorite(modelPath) {
		t.Errorf("expected model to be favorite after toggling")
	}
	cfg.ToggleFavorite(modelPath)
	if cfg.IsFavorite(modelPath) {
		t.Errorf("expected model to not be favorite after toggling again")
	}

	// RecentLaunches capped at 5
	for _, m := range []string{"m1", "m2", "m3", "m4", "m5", "m6"} {
		cfg.RecordLaunch(m)
	}
	if len(cfg.RecentLaunches) != 5 {
		t.Errorf("expected RecentLaunches capped at 5, got %d", len(cfg.RecentLaunches))
	}
	if cfg.RecentLaunches[0] != "m6" {
		t.Errorf("expected most recent launch to be 'm6', got %q", cfg.RecentLaunches[0])
	}
	cfg.RecordLaunch("m3")
	if cfg.RecentLaunches[0] != "m3" {
		t.Errorf("expected 'm3' to move to top, got %q", cfg.RecentLaunches[0])
	}
}

func TestAppDataDirMigration(t *testing.T) {
	tempBase, err := os.MkdirTemp("", "runora-migration-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempBase)

	var envVar string
	switch runtime.GOOS {
	case "windows":
		envVar = "APPDATA"
	case "linux":
		envVar = "XDG_CONFIG_HOME"
	case "darwin":
		envVar = "HOME"
	}

	if envVar == "" {
		t.Skip("skipping migration test on unsupported platform")
	}

	oldEnv := os.Getenv(envVar)
	defer os.Setenv(envVar, oldEnv)

	var userConfigBase string
	if runtime.GOOS == "darwin" {
		userConfigBase = filepath.Join(tempBase, "Library", "Application Support")
	} else {
		userConfigBase = tempBase
	}

	if err := os.MkdirAll(userConfigBase, 0755); err != nil {
		t.Fatalf("failed to create user config base: %v", err)
	}

	os.Setenv(envVar, tempBase)

	oldLlmgrDir := filepath.Join(userConfigBase, "llmgr")
	if err := os.MkdirAll(oldLlmgrDir, 0755); err != nil {
		t.Fatalf("failed to create old llmgr dir: %v", err)
	}
	fakeConfig := filepath.Join(oldLlmgrDir, "config.json")
	if err := os.WriteFile(fakeConfig, []byte(`{"theme":"forest"}`), 0644); err != nil {
		t.Fatalf("failed to write fake config: %v", err)
	}

	resolvedDir, err := AppDataDir()
	if err != nil {
		if strings.Contains(err.Error(), "not yet implemented") {
			t.Skipf("skipping migration test: %v", err)
		}
		t.Fatalf("unexpected error from AppDataDir: %v", err)
	}

	expectedDir := filepath.Join(userConfigBase, "runora")
	if resolvedDir != expectedDir {
		t.Errorf("expected AppDataDir to return %q, got %q", expectedDir, resolvedDir)
	}

	if _, err := os.Stat(oldLlmgrDir); !os.IsNotExist(err) {
		t.Errorf("expected old llmgr directory to be gone (renamed), but it exists")
	}

	newConfig := filepath.Join(expectedDir, "config.json")
	if _, err := os.Stat(newConfig); err != nil {
		t.Errorf("expected migrated config.json to exist at %q, but got err: %v", newConfig, err)
	}
}

func TestAtomicWriteFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "runora-atomic-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetFile := filepath.Join(tempDir, "test.txt")
	testData := []byte("hello atomic world")

	if err := AtomicWriteFile(targetFile, testData, 0600); err != nil {
		t.Fatalf("AtomicWriteFile failed: %v", err)
	}

	readData, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("failed to read back atomic file: %v", err)
	}
	if string(readData) != string(testData) {
		t.Errorf("expected %q, got %q", string(testData), string(readData))
	}

	// Overwrite atomically
	newData := []byte("overwritten atomic world")
	if err := AtomicWriteFile(targetFile, newData, 0600); err != nil {
		t.Fatalf("AtomicWriteFile overwrite failed: %v", err)
	}

	readNewData, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("failed to read back overwritten atomic file: %v", err)
	}
	if string(readNewData) != string(newData) {
		t.Errorf("expected %q, got %q", string(newData), string(readNewData))
	}
}

func TestCorruptedConfigRecovery(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "runora-corrupt-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, configFileName)
	corruptedData := []byte(`{ invalid json content !!! `)
	if err := os.WriteFile(configPath, corruptedData, 0644); err != nil {
		t.Fatalf("failed to write corrupted config: %v", err)
	}

	cfg, err := LoadFromDir(tempDir)
	if err != nil {
		t.Fatalf("LoadFromDir should recover gracefully from corrupted config, got err: %v", err)
	}

	if cfg == nil {
		t.Fatalf("expected non-nil default config after recovery")
	}

	// Verify that a backup corrupted file was created
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read tempDir: %v", err)
	}

	foundBackup := false
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "config.corrupted.") && strings.HasSuffix(f.Name(), ".json") {
			foundBackup = true
			backupPath := filepath.Join(tempDir, f.Name())
			data, err := os.ReadFile(backupPath)
			if err != nil {
				t.Errorf("failed to read backup file: %v", err)
			}
			if string(data) != string(corruptedData) {
				t.Errorf("backup file content mismatch: expected %q, got %q", string(corruptedData), string(data))
			}
			break
		}
	}

	if !foundBackup {
		t.Errorf("expected corrupted config backup file to exist in directory, found: %v", files)
	}

	// Verify new config is valid JSON
	var parsed Config
	validData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read new config.json: %v", err)
	}
	if err := json.Unmarshal(validData, &parsed); err != nil {
		t.Errorf("new config.json should be valid JSON: %v", err)
	}
}

func TestModelDirectories(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "runora-dirs-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := defaultConfig(tempDir)
	cfg.Paths.Models = filepath.Join(tempDir, "models")
	secDir1 := filepath.Join(tempDir, "secondary1")
	secDir2 := filepath.Join(tempDir, "secondary2")
	cfg.Paths.ModelDirectories = []string{secDir1, secDir2, filepath.Join(tempDir, "models")} // duplicate primary included

	allDirs := cfg.Paths.AllModelDirectories()
	if len(allDirs) != 3 {
		t.Fatalf("expected 3 deduplicated model directories, got %d: %v", len(allDirs), allDirs)
	}

	if allDirs[0] != filepath.Join(tempDir, "models") {
		t.Errorf("expected primary models directory first, got %s", allDirs[0])
	}
	if allDirs[1] != secDir1 || allDirs[2] != secDir2 {
		t.Errorf("expected secondary directories in order, got %v", allDirs)
	}

	if err := cfg.CreateDirectories(); err != nil {
		t.Fatalf("failed to create directories: %v", err)
	}

	for _, d := range allDirs {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			t.Errorf("expected directory %s to be created on disk", d)
		}
	}
}

