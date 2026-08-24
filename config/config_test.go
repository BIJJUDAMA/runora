package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BIJJUDAMA/runora/credentials"
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

func TestConfigAPITokens(t *testing.T) {
	credentials.MockInit()
	_ = credentials.Delete(credentials.ProviderGitHub)
	_ = credentials.Delete(credentials.ProviderHuggingFace)

	// Test environment variable detection in defaultConfig
	t.Run("EnvVarDetection", func(t *testing.T) {
		t.Setenv("GITHUB_TOKEN", "ghp_env_test_1")
		t.Setenv("HF_TOKEN", "hf_env_test_1")

		cfg := defaultConfig("")
		if cfg.GitHubToken != "ghp_env_test_1" {
			t.Errorf("expected GitHubToken from GITHUB_TOKEN to be %q, got %q", "ghp_env_test_1", cfg.GitHubToken)
		}
		if cfg.HuggingFaceToken != "hf_env_test_1" {
			t.Errorf("expected HuggingFaceToken from HF_TOKEN to be %q, got %q", "hf_env_test_1", cfg.HuggingFaceToken)
		}

		// Test alternate env vars
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "ghp_alternate_token")
		t.Setenv("HF_TOKEN", "")
		t.Setenv("HUGGING_FACE_HUB_TOKEN", "hf_hub_alternate_token")

		cfgAlt := defaultConfig("")
		if cfgAlt.GitHubToken != "ghp_alternate_token" {
			t.Errorf("expected GitHubToken from GH_TOKEN to be %q, got %q", "ghp_alternate_token", cfgAlt.GitHubToken)
		}
		if cfgAlt.HuggingFaceToken != "hf_hub_alternate_token" {
			t.Errorf("expected HuggingFaceToken from HUGGING_FACE_HUB_TOKEN to be %q, got %q", "hf_hub_alternate_token", cfgAlt.HuggingFaceToken)
		}
	})

	// Test saving and loading with OS Keyring and plaintext purge
	t.Run("SaveAndLoadPersistence", func(t *testing.T) {
		credentials.MockInit()
		_ = credentials.Delete(credentials.ProviderGitHub)
		_ = credentials.Delete(credentials.ProviderHuggingFace)

		// Clean env vars for predictable persistence testing
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("GH_TOKEN", "")
		t.Setenv("HF_TOKEN", "")
		t.Setenv("HUGGING_FACE_HUB_TOKEN", "")

		tempDir, err := os.MkdirTemp("", "runora-tokens-test")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)

		cfg, err := LoadFromDir(tempDir)
		if err != nil {
			t.Fatalf("failed to load initial config: %v", err)
		}

		cfg.GitHubToken = "ghp_test_token_secret_123"
		cfg.HuggingFaceToken = "hf_token_secret_456"

		if err := cfg.Save(); err != nil {
			t.Fatalf("failed to save config with tokens: %v", err)
		}

		// Verify on-disk file content DOES NOT store plaintext secret tokens
		configPath := filepath.Join(tempDir, configFileName)
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("failed to read persisted config file: %v", err)
		}
		jsonStr := string(data)
		if strings.Contains(jsonStr, "ghp_test_token_secret_123") {
			t.Errorf("plaintext secret leaked into config.json:\n%s", jsonStr)
		}
		if strings.Contains(jsonStr, "hf_token_secret_456") {
			t.Errorf("plaintext secret leaked into config.json:\n%s", jsonStr)
		}

		// Verify tokens exist in OS keyring
		ghInKeyring, _ := credentials.Get(credentials.ProviderGitHub)
		if ghInKeyring != "ghp_test_token_secret_123" {
			t.Errorf("expected GitHub token in keyring %q, got %q", "ghp_test_token_secret_123", ghInKeyring)
		}
		hfInKeyring, _ := credentials.Get(credentials.ProviderHuggingFace)
		if hfInKeyring != "hf_token_secret_456" {
			t.Errorf("expected HF token in keyring %q, got %q", "hf_token_secret_456", hfInKeyring)
		}

		// Reload from disk and verify in-memory struct is populated from keyring
		reloaded, err := LoadFromDir(tempDir)
		if err != nil {
			t.Fatalf("failed to reload config: %v", err)
		}
		if reloaded.GitHubToken != "ghp_test_token_secret_123" {
			t.Errorf("expected reloaded GitHubToken %q, got %q", "ghp_test_token_secret_123", reloaded.GitHubToken)
		}
		if reloaded.HuggingFaceToken != "hf_token_secret_456" {
			t.Errorf("expected reloaded HuggingFaceToken %q, got %q", "hf_token_secret_456", reloaded.HuggingFaceToken)
		}
	})
}

func TestConfigLegacySecretMigration(t *testing.T) {
	credentials.MockInit()
	_ = credentials.Delete(credentials.ProviderGitHub)
	_ = credentials.Delete(credentials.ProviderHuggingFace)

	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("HF_TOKEN", "")
	t.Setenv("HUGGING_FACE_HUB_TOKEN", "")

	tempDir, err := os.MkdirTemp("", "runora-migration-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, configFileName)
	legacyJSON := `{
  "theme": "dracula",
  "github_token": "ghp_legacy_migrated_123",
  "hf_token": "hf_legacy_migrated_456"
}`
	if err := os.WriteFile(configPath, []byte(legacyJSON), 0644); err != nil {
		t.Fatalf("failed to write legacy config: %v", err)
	}

	// Loading must trigger migration into OS keyring and purge plaintext secrets from config.json
	cfg, err := LoadFromDir(tempDir)
	if err != nil {
		t.Fatalf("LoadFromDir failed on legacy config: %v", err)
	}

	if cfg.GitHubToken != "ghp_legacy_migrated_123" {
		t.Errorf("expected migrated GitHubToken %q, got %q", "ghp_legacy_migrated_123", cfg.GitHubToken)
	}
	if cfg.HuggingFaceToken != "hf_legacy_migrated_456" {
		t.Errorf("expected migrated HuggingFaceToken %q, got %q", "hf_legacy_migrated_456", cfg.HuggingFaceToken)
	}

	// Verify tokens are in OS keyring
	ghKeyring, _ := credentials.Get(credentials.ProviderGitHub)
	if ghKeyring != "ghp_legacy_migrated_123" {
		t.Errorf("keyring missing migrated GitHubToken, got %q", ghKeyring)
	}
	hfKeyring, _ := credentials.Get(credentials.ProviderHuggingFace)
	if hfKeyring != "hf_legacy_migrated_456" {
		t.Errorf("keyring missing migrated HuggingFaceToken, got %q", hfKeyring)
	}

	// Verify on-disk file was rewritten without plaintext tokens
	purgedData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read migrated config: %v", err)
	}
	purgedStr := string(purgedData)
	if strings.Contains(purgedStr, "ghp_legacy_migrated_123") || strings.Contains(purgedStr, "github_token") {
		t.Errorf("config.json still contains plaintext github_token after migration:\n%s", purgedStr)
	}
	if strings.Contains(purgedStr, "hf_legacy_migrated_456") || strings.Contains(purgedStr, "hf_token") {
		t.Errorf("config.json still contains plaintext hf_token after migration:\n%s", purgedStr)
	}
}

func TestCorruptedConfigSelfHealingAndBackup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "runora-corrupt-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, configFileName)
	corruptData := []byte("INVALID_JSON_CORRUPT{broken_syntax: true,")
	if err := os.WriteFile(configPath, corruptData, 0644); err != nil {
		t.Fatalf("failed to write corrupt config: %v", err)
	}

	// LoadFromDir should heal itself, create .corrupted backup, and return valid config
	cfg, err := LoadFromDir(tempDir)
	if err != nil {
		t.Fatalf("expected LoadFromDir to self-heal without error, got: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected non-nil config returned")
	}

	// Verify .corrupted backup file was generated
	files, _ := os.ReadDir(tempDir)
	foundBackup := false
	for _, f := range files {
		if strings.Contains(f.Name(), ".corrupted") {
			foundBackup = true
			backupData, _ := os.ReadFile(filepath.Join(tempDir, f.Name()))
			if string(backupData) != string(corruptData) {
				t.Errorf("backup data mismatch, got %q, expected %q", string(backupData), string(corruptData))
			}
			break
		}
	}
	if !foundBackup {
		t.Errorf("expected .corrupted backup file to be created in %s", tempDir)
	}

	// Verify new valid config was saved
	savedData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read restored config: %v", err)
	}
	var testUnmarshal map[string]interface{}
	if err := json.Unmarshal(savedData, &testUnmarshal); err != nil {
		t.Errorf("restored config is not valid JSON: %v", err)
	}
}

func TestAtomicWriteFileConcurrentSafety(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "runora-atomic-write-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetFile := filepath.Join(tempDir, "concurrent_target.json")

	doneCh := make(chan error, 20)
	for i := 0; i < 20; i++ {
		go func(idx int) {
			data := []byte(fmt.Sprintf(`{"worker": %d, "timestamp": %d}`, idx, idx*1000))
			doneCh <- AtomicWriteFile(targetFile, data, 0644)
		}(i)
	}

	for i := 0; i < 20; i++ {
		if err := <-doneCh; err != nil {
			t.Errorf("concurrent AtomicWriteFile %d failed: %v", i, err)
		}
	}

	// Verify final file is valid JSON and not corrupt/torn
	finalData, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("failed to read target file: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(finalData, &parsed); err != nil {
		t.Errorf("final file is corrupt after concurrent writes: %v, content:\n%s", err, string(finalData))
	}
}

func TestFavoriteTogglingAndRecentLaunchesLimits(t *testing.T) {
	cfg := defaultConfig("")

	// 1. Test ToggleFavorite
	if cfg.IsFavorite("model-a") {
		t.Errorf("expected model-a to not be favorite initially")
	}
	cfg.ToggleFavorite("model-a")
	if !cfg.IsFavorite("model-a") {
		t.Errorf("expected model-a to be favorite after toggle")
	}
	cfg.ToggleFavorite("model-a")
	if cfg.IsFavorite("model-a") {
		t.Errorf("expected model-a to not be favorite after second toggle")
	}

	// 2. Test RecordLaunch and deduplication
	for i := 0; i < 15; i++ {
		cfg.RecordLaunch(fmt.Sprintf("model-%d", i))
	}
	// Re-record recent model
	cfg.RecordLaunch("model-5")

	if len(cfg.RecentLaunches) == 0 {
		t.Fatalf("expected non-empty recent launches")
	}
	// Latest launched model must be at index 0
	if cfg.RecentLaunches[0] != "model-5" {
		t.Errorf("expected most recent launch to be 'model-5' at index 0, got %s", cfg.RecentLaunches[0])
	}
}


