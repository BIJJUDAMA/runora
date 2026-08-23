package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/BIJJUDAMA/runora/config"
	"github.com/BIJJUDAMA/runora/hardware"
)

func TestBentoLifecycleSettingsLayout(t *testing.T) {
	cfg := &config.Config{
		Paths: config.Paths{
			LlamaCPP:    "/app/llama.cpp",
			OnnxRuntime: "/app/onnxruntime",
			Downloads:   "/app/downloads",
		},
		GitHubToken: "ghp_1234567890abcdef123456",
		HFToken:     "hf_9876543210fedcba987654",
		Theme:       "forest",
	}
	mockSrv := &mockModelRuntime{}

	model := NewLifecycleModel(cfg, mockSrv)
	model.specs = &hardware.HardwareSpecs{
		OS: "windows",
		GPU: hardware.GPUSpecs{
			Type:        "CUDA",
			CudaVersion: "12.4",
			Name:        "NVIDIA GeForce RTX 4090",
		},
	}
	model.localVersion = "b3520"
	model.localCommit = "a1b2c3d"
	model.localBuildInfo = "MSVC 19.38 / CUDA 12.4"
	model.installedVersions = []string{"b3520", "b3500", "b3480"}
	model.activeSlot = "b3520"

	// 1. Standard Settings Bento Layout View
	view := model.View(100, 40)

	// Verify all 4 Bento Cards are present
	expectedCards := []struct {
		title string
		badge string
	}{
		{"API Credentials", "GitHub & Hugging Face"},
		{"1. Engine: llama.cpp Runtime", "llama.cpp"},
		{"2. Engine: ONNX Runtime", "ONNX"},
		{"3. Runora System & Tools", "System"},
	}

	for _, card := range expectedCards {
		if !strings.Contains(view, card.title) {
			t.Errorf("expected view to contain card title %q, got:\n%s", card.title, view)
		}
		if !strings.Contains(view, card.badge) {
			t.Errorf("expected view to contain card badge %q, got:\n%s", card.badge, view)
		}
	}

	// Verify llama.cpp card details
	if !strings.Contains(view, "Active Version Slot:") {
		t.Errorf("expected view to contain 'Active Version Slot:', got:\n%s", view)
	}
	if !strings.Contains(view, "Release Channel:") {
		t.Errorf("expected view to contain 'Release Channel:', got:\n%s", view)
	}
	if !strings.Contains(view, "Stable") {
		t.Errorf("expected view to contain 'Stable', got:\n%s", view)
	}
	if !strings.Contains(view, "Backend Accelerator:") {
		t.Errorf("expected view to contain 'Backend Accelerator:', got:\n%s", view)
	}
	if !strings.Contains(view, "Installed Slots:") {
		t.Errorf("expected view to contain 'Installed Slots:', got:\n%s", view)
	}

	// Verify ONNX card details
	if !strings.Contains(view, "Installed Version:") {
		t.Errorf("expected view to contain 'Installed Version:', got:\n%s", view)
	}

	// Verify Runora System card details
	if !strings.Contains(view, "Runora Version:") {
		t.Errorf("expected view to contain 'Runora Version:', got:\n%s", view)
	}

	// Verify API Credentials masked tokens and actions
	if !strings.Contains(view, "ghp_***") {
		t.Errorf("expected view to contain masked GitHub Token 'ghp_***', got:\n%s", view)
	}
	if !strings.Contains(view, "hf_***") {
		t.Errorf("expected view to contain masked HF Token 'hf_***', got:\n%s", view)
	}
	if !strings.Contains(view, "[G: Edit / Paste]") && !strings.Contains(view, "[G]") {
		t.Errorf("expected view to contain '[G]' action, got:\n%s", view)
	}
	if !strings.Contains(view, "[T: Edit / Paste]") && !strings.Contains(view, "[T]") {
		t.Errorf("expected view to contain '[T]' action, got:\n%s", view)
	}

	// 2. Strict Zero Emojis check in standard view
	for _, r := range view {
		if isEmoji(r) {
			t.Errorf("LifecycleModel.View contains emoji %q in rendered output", string(r))
		}
	}

	// 3. Test Token Edit Mode
	// Enter token edit mode via key 'g'
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if !model.tokenEditActive {
		t.Fatalf("expected tokenEditActive to be true after pressing 'g'")
	}
	editView := model.View(100, 40)

	if !strings.Contains(editView, "API Credentials") {
		t.Errorf("expected edit view to maintain API Credentials card, got:\n%s", editView)
	}
	if !strings.Contains(editView, "EDITING:") {
		t.Errorf("expected edit view to contain 'EDITING:', got:\n%s", editView)
	}
	if !strings.Contains(editView, "[Enter]") && !strings.Contains(editView, "Save") {
		t.Errorf("expected edit view to contain save prompt, got:\n%s", editView)
	}

	for _, r := range editView {
		if isEmoji(r) {
			t.Errorf("LifecycleModel.View in edit mode contains emoji %q in output", string(r))
		}
	}
}
