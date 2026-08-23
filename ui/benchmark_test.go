package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/BIJJUDAMA/runora/benchmark"
	"github.com/BIJJUDAMA/runora/hardware"
)

func TestBentoBenchmarkDashboardLayout(t *testing.T) {
	// 1. Test with populated benchmark history
	now := time.Now()
	sampleResults := []*benchmark.BenchmarkResult{
		{
			ModelPath:          "/models/Meta-Llama-3-8B-Instruct.Q4_K_M.gguf",
			ModelName:          "Llama-3-8B-Instruct-Q4",
			RunDate:            now.Add(-10 * time.Minute),
			StartupTimeMs:      1250,
			PromptTokensPerSec: 145.5,
			TokensPerSec:       38.2,
			PeakTokensPerSec:   42.0,
			TTFTMs:             45.2,
			RAMUsageMB:         4200.0,
			VRAMUsageMB:        5600.0,
		},
		{
			ModelPath:          "/models/Mistral-7B-Instruct-v0.3.Q5_K_M.gguf",
			ModelName:          "Mistral-7B-Instruct-Q5",
			RunDate:            now,
			StartupTimeMs:      1100,
			PromptTokensPerSec: 160.0,
			TokensPerSec:       45.8,
			PeakTokensPerSec:   48.5,
			TTFTMs:             38.5,
			RAMUsageMB:         3800.0,
			VRAMUsageMB:        5200.0,
		},
	}

	model := NewPerformanceDashboardModel(sampleResults)
	model.Specs = &hardware.HardwareSpecs{
		OS: "windows",
		CPU: hardware.CPUSpecs{
			Model:         "AMD Ryzen 9 7950X",
			PhysicalCores: 16,
			Threads:       32,
		},
		GPU: hardware.GPUSpecs{
			Type:        "CUDA",
			Name:        "NVIDIA GeForce RTX 4090",
			VRAM:        24 * 1024 * 1024 * 1024,
			CudaVersion: "12.4",
		},
		RAM: hardware.RAMSpecs{
			Total:     64 * 1024 * 1024 * 1024,
			Available: 48 * 1024 * 1024 * 1024,
		},
	}

	view := model.View(100, 40)

	// Verify Bento Cards are rendered with standard titles and badges
	expectedCards := []struct {
		title string
		badge string
	}{
		{"Benchmark Run History", "2 runs"},
		{"Throughput & Latency (Tokens/sec)", "TTFT + Decode"},
		{"Test Platform", "Specs"},
	}

	for _, card := range expectedCards {
		if !strings.Contains(view, card.title) {
			t.Errorf("expected view to contain card title %q, got:\n%s", card.title, view)
		}
		if !strings.Contains(view, card.badge) {
			t.Errorf("expected view to contain badge %q, got:\n%s", card.badge, view)
		}
	}

	// Verify history content items
	if !strings.Contains(view, "Llama-3-8B-Instruct") {
		t.Errorf("expected view to contain 'Llama-3-8B-Instruct', got:\n%s", view)
	}
	if !strings.Contains(view, "Mistral-7B-Instruct") {
		t.Errorf("expected view to contain 'Mistral-7B-Instruct', got:\n%s", view)
	}
	if !strings.Contains(view, "Fastest Model:") {
		t.Errorf("expected view to contain 'Fastest Model:', got:\n%s", view)
	}
	if !strings.Contains(view, "Most Efficient:") {
		t.Errorf("expected view to contain 'Most Efficient:', got:\n%s", view)
	}

	// Verify throughput details
	if !strings.Contains(view, "Prompt Eval (TTFT):") {
		t.Errorf("expected view to contain 'Prompt Eval (TTFT):', got:\n%s", view)
	}
	if !strings.Contains(view, "Decode Generation:") {
		t.Errorf("expected view to contain 'Decode Generation:', got:\n%s", view)
	}
	if !strings.Contains(view, "Memory Footprint:") {
		t.Errorf("expected view to contain 'Memory Footprint:', got:\n%s", view)
	}

	// Verify test platform specs
	if !strings.Contains(view, "RTX 4090") {
		t.Errorf("expected view to contain GPU model 'RTX 4090', got:\n%s", view)
	}
	if !strings.Contains(view, "Ryzen 9 7950X") {
		t.Errorf("expected view to contain CPU model 'Ryzen 9 7950X', got:\n%s", view)
	}

	// 2. Strict Zero Emojis check across dashboard view
	for _, r := range view {
		if isEmoji(r) {
			t.Errorf("PerformanceDashboardModel.View contains emoji %q in output", string(r))
		}
	}

	// 3. Test empty history view
	emptyModel := NewPerformanceDashboardModel([]*benchmark.BenchmarkResult{})
	emptyView := emptyModel.View(100, 40)
	if !strings.Contains(emptyView, "Benchmark Run History") {
		t.Errorf("empty dashboard missing 'Benchmark Run History'")
	}
	if !strings.Contains(emptyView, "No benchmark records found") {
		t.Errorf("empty dashboard missing 'No benchmark records found'")
	}
	for _, r := range emptyView {
		if isEmoji(r) {
			t.Errorf("Empty PerformanceDashboardModel.View contains emoji %q in output", string(r))
		}
	}

	// 4. Test BenchmarkProgressModel Views across all steps for zero emojis
	steps := []BenchmarkProgressStep{
		StepBooting,
		StepRunningPrompt,
		StepSavingData,
		StepDone,
		StepError,
	}
	for _, step := range steps {
		pm := NewBenchmarkProgressModel("Test-Model-7B")
		pm.Step = step
		pView := pm.View(80, 20)
		for _, r := range pView {
			if isEmoji(r) {
				t.Errorf("BenchmarkProgressModel.View step %d contains emoji %q in output", step, string(r))
			}
		}
	}
}
