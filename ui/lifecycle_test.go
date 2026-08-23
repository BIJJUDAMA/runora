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

	// Verify all 3 Bento Cards are present
	expectedCards := []struct {
		title string
		badge string
	}{
		{"Runtime Version & Acceleration", "llama.cpp"},
		{"API Credentials", "GitHub / Hugging Face"},
		{"Backup & Rollback", "Recovery"},
	}

	for _, card := range expectedCards {
		if !strings.Contains(view, card.title) {
			t.Errorf("expected view to contain card title %q, got:\n%s", card.title, view)
		}
		if !strings.Contains(view, card.badge) {
			t.Errorf("expected view to contain card badge %q, got:\n%s", card.badge, view)
		}
	}

	// Verify Runtime Version & Acceleration card details
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

	// Verify API Credentials masked tokens and actions
	if !strings.Contains(view, "ghp_***") {
		t.Errorf("expected view to contain masked GitHub Token 'ghp_***', got:\n%s", view)
	}
	if !strings.Contains(view, "hf_***") {
		t.Errorf("expected view to contain masked HF Token 'hf_***', got:\n%s", view)
	}
	if !strings.Contains(view, "[E]") && !strings.Contains(view, "Edit") {
		t.Errorf("expected view to contain '[E] Edit' action, got:\n%s", view)
	}
	if !strings.Contains(view, "[S]") && !strings.Contains(view, "Save") {
		t.Errorf("expected view to contain '[S] Save' action, got:\n%s", view)
	}

	// Verify Backup & Rollback card details
	if !strings.Contains(view, "llama.cpp Backup:") {
		t.Errorf("expected view to contain 'llama.cpp Backup:', got:\n%s", view)
	}
	if !strings.Contains(view, "ONNX Runtime Backup:") {
		t.Errorf("expected view to contain 'ONNX Runtime Backup:', got:\n%s", view)
	}
	if !strings.Contains(view, "Recovery rollbacks") {
		t.Errorf("expected view to contain recovery information, got:\n%s", view)
	}

	// 2. Strict Zero Emojis check in standard view
	for _, r := range view {
		if isEmoji(r) {
			t.Errorf("LifecycleModel.View contains emoji %q in rendered output", string(r))
		}
	}

	// 3. Test Token Edit Mode
	// Enter token edit mode via key 'e'
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if !model.tokenEditActive {
		t.Fatalf("expected tokenEditActive to be true after pressing 'e'")
	}
	editView := model.View(100, 40)

	if !strings.Contains(editView, "API Credentials") {
		t.Errorf("expected edit view to maintain API Credentials card, got:\n%s", editView)
	}
	if !strings.Contains(editView, "Editing:") {
		t.Errorf("expected edit view to contain 'Editing:', got:\n%s", editView)
	}
	if !strings.Contains(editView, "[Enter/S]") && !strings.Contains(editView, "Save") {
		t.Errorf("expected edit view to contain save prompt, got:\n%s", editView)
	}

	for _, r := range editView {
		if isEmoji(r) {
			t.Errorf("LifecycleModel.View in edit mode contains emoji %q in output", string(r))
		}
	}

	// 4. Test ONNX Runtime selected view for zero emojis
	model.tokenEditActive = false
	model.SelectedRuntime = 1
	onnxView := model.View(100, 40)
	if !strings.Contains(onnxView, "ONNX Runtime") {
		t.Errorf("expected ONNX view to contain 'ONNX Runtime', got:\n%s", onnxView)
	}
	for _, r := range onnxView {
		if isEmoji(r) {
			t.Errorf("LifecycleModel.View (ONNX) contains emoji %q in output", string(r))
		}
	}

	// 5. Test Runora App selected view for zero emojis
	model.SelectedRuntime = 2
	appView := model.View(100, 40)
	if !strings.Contains(appView, "Runora CLI") {
		t.Errorf("expected App view to contain 'Runora CLI', got:\n%s", appView)
	}
	for _, r := range appView {
		if isEmoji(r) {
			t.Errorf("LifecycleModel.View (App) contains emoji %q in output", string(r))
		}
	}
}
