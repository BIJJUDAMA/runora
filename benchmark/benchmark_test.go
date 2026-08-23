package benchmark

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BIJJUDAMA/runora/config"
	"github.com/BIJJUDAMA/runora/model"
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

func TestBenchmarkHistoryPersistenceOrderingAndCorruptRecovery(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llama-bench-history-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test empty directory load
	emptyHist, err := LoadHistory(tempDir)
	if err != nil {
		t.Fatalf("LoadHistory on empty dir failed: %v", err)
	}
	if len(emptyHist) != 0 {
		t.Errorf("expected 0 results from empty dir, got %d", len(emptyHist))
	}

	// Append 3 sequential benchmark results
	models := []string{"Qwen-7B", "Llama-8B", "Mistral-7B"}
	speeds := []float64{62.5, 48.3, 55.1}

	for i := range models {
		r := &BenchmarkResult{
			ModelPath:    fmt.Sprintf("models/%s.gguf", models[i]),
			ModelName:    models[i],
			RunDate:      time.Now().Add(time.Duration(i) * time.Minute),
			TokensPerSec: speeds[i],
		}
		if err := SaveResult(tempDir, r); err != nil {
			t.Fatalf("SaveResult %d failed: %v", i, err)
		}
	}

	loaded, err := LoadHistory(tempDir)
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 results, got %d", len(loaded))
	}

	for i := range models {
		if loaded[i].ModelName != models[i] || loaded[i].TokensPerSec != speeds[i] {
			t.Errorf("history item %d mismatch: got name=%s speed=%f, expected name=%s speed=%f",
				i, loaded[i].ModelName, loaded[i].TokensPerSec, models[i], speeds[i])
		}
	}

	// Corrupt the history.json file with malformed JSON
	histPath := filepath.Join(tempDir, "history.json")
	_ = os.WriteFile(histPath, []byte("INVALID_JSON_CORRUPT{{{"), 0644)

	// LoadHistory must recover gracefully and return empty slice rather than crashing
	corruptedLoaded, err := LoadHistory(tempDir)
	if err != nil {
		t.Fatalf("expected graceful return on corrupt json, got error: %v", err)
	}
	if len(corruptedLoaded) != 0 {
		t.Errorf("expected 0 results from corrupted file, got %d", len(corruptedLoaded))
	}
}

func TestBenchmarkCalculationAndFallbacks(t *testing.T) {
	// 1. Full timing fields populated from llama.cpp /completion
	compResp := LlamaCompletionResponse{
		Content: "Local AI in the terminal runs fast and free.",
		Timings: LlamaTimings{
			PromptN:            32,
			PromptMs:           128.0,
			PromptPerSecond:    250.0,
			PredictedN:         64,
			PredictedMs:        1280.0,
			PredictedPerSecond: 50.0,
		},
	}

	promptTokensPerSec := compResp.Timings.PromptPerSecond
	if promptTokensPerSec == 0 && compResp.Timings.PromptN > 0 && compResp.Timings.PromptMs > 0 {
		promptTokensPerSec = float64(compResp.Timings.PromptN) / (compResp.Timings.PromptMs / 1000.0)
	}
	if promptTokensPerSec != 250.0 {
		t.Errorf("expected promptTokensPerSec 250.0, got %f", promptTokensPerSec)
	}

	ttftMs := compResp.Timings.PromptMs
	if ttftMs != 128.0 {
		t.Errorf("expected ttftMs 128.0, got %f", ttftMs)
	}

	tokensPerSec := compResp.Timings.PredictedPerSecond
	if tokensPerSec != 50.0 {
		t.Errorf("expected tokensPerSec 50.0, got %f", tokensPerSec)
	}

	// 2. Test fallback when PromptPerSecond is 0 but PromptN and PromptMs exist
	compRespFallback := LlamaCompletionResponse{
		Timings: LlamaTimings{
			PromptN:     40,
			PromptMs:    200.0, // 40 tokens / 0.2s = 200 tps
			PredictedN:  100,
			PredictedMs: 2000.0, // 100 tokens / 2.0s = 50 tps
		},
	}
	calcPromptTPS := compRespFallback.Timings.PromptPerSecond
	if calcPromptTPS == 0 && compRespFallback.Timings.PromptN > 0 && compRespFallback.Timings.PromptMs > 0 {
		calcPromptTPS = float64(compRespFallback.Timings.PromptN) / (compRespFallback.Timings.PromptMs / 1000.0)
	}
	if calcPromptTPS != 200.0 {
		t.Errorf("expected calculated prompt TPS 200.0, got %f", calcPromptTPS)
	}

	calcDecodeTPS := compRespFallback.Timings.PredictedPerSecond
	if calcDecodeTPS == 0 && compRespFallback.Timings.PredictedN > 0 && compRespFallback.Timings.PredictedMs > 0 {
		calcDecodeTPS = float64(compRespFallback.Timings.PredictedN) / (compRespFallback.Timings.PredictedMs / 1000.0)
	}
	if calcDecodeTPS != 50.0 {
		t.Errorf("expected calculated decode TPS 50.0, got %f", calcDecodeTPS)
	}
}

func TestBenchmarkRunFailurePaths(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llama-bench-fail-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := config.DefaultConfig()
	cfg.Paths.Cache = tempDir

	m := &model.GGUFMetadata{
		FilePath: "non_existent_model.gguf",
		Name:     "Non-Existent",
	}

	stepSequence := []int{}
	stepRecorder := func(step int) {
		stepSequence = append(stepSequence, step)
	}

	// Missing binary directory must fail immediately
	res, err := RunBenchmark(filepath.Join(tempDir, "missing_bin"), m, nil, cfg, stepRecorder)
	if err == nil {
		t.Errorf("expected error running benchmark with missing binary dir, got nil")
	}
	if res != nil {
		t.Errorf("expected nil result on failure, got: %+v", res)
	}

	// Step 0 (StepBooting) must have been recorded
	if len(stepSequence) == 0 || stepSequence[0] != 0 {
		t.Errorf("expected step 0 to be recorded, got: %v", stepSequence)
	}
}
