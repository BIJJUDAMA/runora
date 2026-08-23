package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/BIJJUDAMA/runora/hardware"
	"github.com/BIJJUDAMA/runora/model"
	"github.com/BIJJUDAMA/runora/profile"
	"github.com/charmbracelet/lipgloss"
)

func createTestModelAndSpecs() (*model.GGUFMetadata, *hardware.HardwareSpecs, []*profile.Profile) {
	meta := &model.GGUFMetadata{
		ID:            "meta-llama-3-8b",
		Name:          "Meta-Llama-3-8B-Instruct-Q4_K_M.gguf",
		Architecture:  "llama",
		ContextLength: 8192,
		Quantization:  "Q4_K_M",
		ParamCount:    8030000000,
		FileSize:      4920721408, // ~4.58 GB
		FilePath:      "models/Meta-Llama-3-8B-Instruct-Q4_K_M.gguf",
		Layers:        32,
		Heads:         32,
		HeadsKV:       8,
		EmbeddingLen:  4096,
		HeadDim:       128,
	}

	specs := &hardware.HardwareSpecs{
		CPU: hardware.CPUSpecs{
			PhysicalCores: 8,
			Threads:       16,
			Model:         "AMD Ryzen 7 7800X3D",
		},
		RAM: hardware.RAMSpecs{
			Total:     34359738368, // 32 GB
			Available: 25769803776,
		},
		GPU: hardware.GPUSpecs{
			Name: "NVIDIA GeForce RTX 4080",
			VRAM: 17179869184, // 16 GB
			Type: "CUDA",
		},
		GPUs: []hardware.GPUSpecs{
			{
				Name: "NVIDIA GeForce RTX 4080",
				VRAM: 17179869184,
				Type: "CUDA",
			},
		},
	}

	profiles := profile.DefaultProfiles()
	return meta, specs, profiles
}

func TestDualColumnBentoDashboardLayout(t *testing.T) {
	meta, specs, profiles := createTestModelAndSpecs()
	dashboard := NewDashboardModel(meta, specs, profiles, "Balanced")

	width := 120
	height := 35
	rendered := dashboard.View(width, height)

	// 1. Verify Model Name in Left Card
	if !strings.Contains(rendered, "Meta-Llama-3-8B") {
		t.Errorf("expected view to contain model name 'Meta-Llama-3-8B', got:\n%s", rendered)
	}

	// 2. Verify Left Column Bento Card: "Hardware Fit & Memory Estimate"
	if !strings.Contains(rendered, "Hardware Fit & Memory Estimate") {
		t.Errorf("expected left Bento card title 'Hardware Fit & Memory Estimate', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Model Weight:") {
		t.Errorf("expected left card to contain 'Model Weight:', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Quantized KV Cache:") || !strings.Contains(rendered, "multiplier") {
		t.Errorf("expected left card to contain 'Quantized KV Cache:' with multiplier, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Activation Graph:") {
		t.Errorf("expected left card to contain 'Activation Graph:', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Total Required:") {
		t.Errorf("expected left card to contain 'Total Required:', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Fit Suitability:") {
		t.Errorf("expected left card to contain 'Fit Suitability:', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[FITS VRAM]") {
		t.Errorf("expected left card suitability badge '[FITS VRAM]', got:\n%s", rendered)
	}

	// 3. Verify Gradient Memory Bar in Left Card
	if !strings.Contains(rendered, "Memory Allocation:") {
		t.Errorf("expected left card to contain 'Memory Allocation:', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "█") {
		t.Errorf("expected left card to render gradient progress bar filled blocks '█', got:\n%s", rendered)
	}

	// 4. Verify Right Column Top Card: "Execution Profile"
	if !strings.Contains(rendered, "Execution Profile") {
		t.Errorf("expected right top Bento card title 'Execution Profile', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Balanced") {
		t.Errorf("expected right top card to display active profile badge 'Balanced', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[<]") || !strings.Contains(rendered, "[>]") {
		t.Errorf("expected right top card to display carousel navigation '[<]' and '[>]', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Context Size:") || !strings.Contains(rendered, "4096 tokens") {
		t.Errorf("expected right top card to display Context Size '4096 tokens', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Thread Count:") {
		t.Errorf("expected right top card to display 'Thread Count:', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "GPU Layers:") {
		t.Errorf("expected right top card to display 'GPU Layers:', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Flash Attention:") {
		t.Errorf("expected right top card to display 'Flash Attention:', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "KV Quantization:") || !strings.Contains(rendered, "Q8_0/Q4_0/FP8") {
		t.Errorf("expected right top card to display 'KV Quantization:' mentioning Q8_0/Q4_0/FP8, got:\n%s", rendered)
	}

	// 5. Verify Right Column Bottom Card: "Launch Command Preview"
	if !strings.Contains(rendered, "Launch Command Preview") {
		t.Errorf("expected right bottom Bento card title 'Launch Command Preview', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "CLI") {
		t.Errorf("expected right bottom card to display badge 'CLI', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "llama-server") || !strings.Contains(rendered, "--model") {
		t.Errorf("expected right bottom card to render generated llama-server command, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[C]") || !strings.Contains(rendered, "Copy to clipboard") {
		t.Errorf("expected right bottom card to display '[C] Copy to clipboard' hint, got:\n%s", rendered)
	}

	// 6. Verify Height matches exactly height
	if h := lipgloss.Height(rendered); h != height {
		t.Errorf("expected rendered height to match %d, got %d", height, h)
	}

	// 7. Verify STRICT ZERO EMOJIS across the entire dashboard view
	for _, r := range rendered {
		if isEmoji(r) {
			t.Errorf("found disallowed emoji character %q (rune: 0x%X) in dashboard view:\n%s", string(r), r, rendered)
		}
	}
}

func TestDashboardModel_CycleProfiles(t *testing.T) {
	meta, specs, profiles := createTestModelAndSpecs()
	dashboard := NewDashboardModel(meta, specs, profiles, "Fast")

	if dashboard.ActiveProfile().Name != "Fast" {
		t.Fatalf("expected initial profile to be 'Fast', got %s", dashboard.ActiveProfile().Name)
	}

	// Cycle forward
	dashboard.CycleProfile(1)
	if dashboard.ActiveProfile().Name != "Balanced" {
		t.Errorf("expected active profile after cycle +1 to be 'Balanced', got %s", dashboard.ActiveProfile().Name)
	}

	// Cycle backward
	dashboard.CycleProfile(-1)
	if dashboard.ActiveProfile().Name != "Fast" {
		t.Errorf("expected active profile after cycle -1 to be 'Fast', got %s", dashboard.ActiveProfile().Name)
	}

	// Cycle backward wrap-around
	dashboard.CycleProfile(-1)
	expectedLast := profiles[len(profiles)-1].Name
	if dashboard.ActiveProfile().Name != expectedLast {
		t.Errorf("expected wrap-around cycle -1 to be %q, got %s", expectedLast, dashboard.ActiveProfile().Name)
	}
}

func TestDashboardModel_GetLaunchCommand(t *testing.T) {
	meta, specs, profiles := createTestModelAndSpecs()
	dashboard := NewDashboardModel(meta, specs, profiles, "High")

	cmd := dashboard.GetLaunchCommand()
	expectedSubstrings := []string{
		"llama-server",
		"--model " + meta.FilePath,
		"--host 127.0.0.1",
		"--port 50505",
		"--ctx-size 8192",
		"--threads",
		"--n-gpu-layers 999",
		"--batch-size 512",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(cmd, sub) {
			t.Errorf("expected launch command to contain %q, got %q", sub, cmd)
		}
	}
}

func TestDashboardModel_ToastMessage(t *testing.T) {
	meta, specs, profiles := createTestModelAndSpecs()
	dashboard := NewDashboardModel(meta, specs, profiles, "Balanced")

	dashboard.SetToast("Test Toast Notification")
	rendered := dashboard.View(100, 30)

	if !strings.Contains(rendered, "Test Toast Notification") {
		t.Errorf("expected rendered view to contain active toast notification, got:\n%s", rendered)
	}

	// Expire toast
	dashboard.ToastExpiry = time.Now().Add(-1 * time.Minute)
	renderedExpired := dashboard.View(100, 30)
	if strings.Contains(renderedExpired, "Test Toast Notification") {
		t.Errorf("expected expired toast notification to not appear, got:\n%s", renderedExpired)
	}
}

func TestDashboardModel_ZeroEmojisAllProfilesAndThemes(t *testing.T) {
	meta, specs, profiles := createTestModelAndSpecs()

	themes := GetRegisteredThemes()
	for _, theme := range themes {
		ApplyTheme(theme.ID())

		for i := 0; i < len(profiles); i++ {
			dashboard := NewDashboardModel(meta, specs, profiles, profiles[i].Name)
			rendered := dashboard.View(100, 30)

			for _, r := range rendered {
				if isEmoji(r) {
					t.Errorf("theme %s profile %s contains emoji %q in dashboard view", theme.ID(), profiles[i].Name, string(r))
				}
			}
		}
	}
}

func TestDashboardModel_NilSpecsAndModelFallbacks(t *testing.T) {
	_, _, profiles := createTestModelAndSpecs()

	// Nil model and nil specs
	dashboardNil := NewDashboardModel(nil, nil, profiles, "Balanced")
	renderedNil := dashboardNil.View(80, 25)

	if !strings.Contains(renderedNil, "No model selected") {
		t.Errorf("expected fallback text for nil model, got:\n%s", renderedNil)
	}

	for _, r := range renderedNil {
		if isEmoji(r) {
			t.Errorf("nil dashboard view contains emoji %q", string(r))
		}
	}
}
