package benchmark

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBenchmarkDatabase(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llama-benchmark-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save benchmark result
	res := &BenchmarkResult{
		ModelPath:          "models/test.gguf",
		ModelName:          "Test Model",
		RunDate:            time.Now(),
		StartupTimeMs:      1200,
		PromptTokensPerSec: 150.5,
		TokensPerSec:       45.2,
		PeakTokensPerSec:   48.0,
		TTFTMs:             105.2,
		RAMUsageMB:         2048.5,
		VRAMUsageMB:        512.0,
		MemoryBreakdown: MemoryBreakdown{
			HostRSSBytes: 2048 * 1024 * 1024,
			GPUDedicated: 512 * 1024 * 1024,
		},
	}

	err = SaveResult(tempDir, res)
	if err != nil {
		t.Fatalf("failed to save result: %v", err)
	}

	// Verify history file exists
	historyFile := filepath.Join(tempDir, "history.json")
	if _, err := os.Stat(historyFile); os.IsNotExist(err) {
		t.Errorf("history.json was not created")
	}

	// Load history
	history, err := LoadHistory(tempDir)
	if err != nil {
		t.Fatalf("failed to load history: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("expected 1 result, got %d", len(history))
	}

	loaded := history[0]
	if loaded.ModelName != "Test Model" || loaded.TokensPerSec != 45.2 || loaded.StartupTimeMs != 1200 {
		t.Errorf("loaded data mismatch: %+v", loaded)
	}
	if loaded.PromptTokensPerSec != 150.5 || loaded.TTFTMs != 105.2 {
		t.Errorf("loaded decoupled prompt metrics mismatch: %+v", loaded)
	}
	if loaded.MemoryBreakdown.HostRSSBytes != 2048*1024*1024 || loaded.MemoryBreakdown.GPUDedicated != 512*1024*1024 {
		t.Errorf("loaded memory breakdown mismatch: %+v", loaded.MemoryBreakdown)
	}
}
