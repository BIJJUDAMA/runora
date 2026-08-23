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

	// 1. Standard Settings 2-Panel Inspector Layout View (Section 0: API Credentials)
	view := model.View(100, 40)

	// Verify Components list and API Credentials inspector are present
	if !strings.Contains(view, "Components") {
		t.Errorf("expected view to contain 'Components' selector, got:\n%s", view)
	}
	if !strings.Contains(view, "API Credentials Inspector") {
		t.Errorf("expected view to contain 'API Credentials Inspector', got:\n%s", view)
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

	// 3. Switch to Section 1 (llama.cpp Runtime Inspector)
	model.SelectedRuntime = 1
	llamaView := model.View(100, 40)
	if !strings.Contains(llamaView, "llama.cpp Runtime Inspector") {
		t.Errorf("expected view to contain 'llama.cpp Runtime Inspector', got:\n%s", llamaView)
	}
	if !strings.Contains(llamaView, "Active Version Slot:") {
		t.Errorf("expected llama view to contain 'Active Version Slot:', got:\n%s", llamaView)
	}
	if !strings.Contains(llamaView, "Release Channel:") {
		t.Errorf("expected llama view to contain 'Release Channel:', got:\n%s", llamaView)
	}
	if !strings.Contains(llamaView, "Stable") {
		t.Errorf("expected llama view to contain 'Stable', got:\n%s", llamaView)
	}
	if !strings.Contains(llamaView, "Backend Accelerator:") {
		t.Errorf("expected llama view to contain 'Backend Accelerator:', got:\n%s", llamaView)
	}
	if !strings.Contains(llamaView, "Installed Slots:") {
		t.Errorf("expected llama view to contain 'Installed Slots:', got:\n%s", llamaView)
	}

	// 4. Switch to Section 2 (ONNX Runtime Inspector)
	model.SelectedRuntime = 2
	onnxView := model.View(100, 40)
	if !strings.Contains(onnxView, "ONNX Runtime Inspector") {
		t.Errorf("expected view to contain 'ONNX Runtime Inspector', got:\n%s", onnxView)
	}
	if !strings.Contains(onnxView, "Installed Version:") {
		t.Errorf("expected onnx view to contain 'Installed Version:', got:\n%s", onnxView)
	}

	// 5. Switch to Section 3 (Runora System & Tools Inspector)
	model.SelectedRuntime = 3
	appView := model.View(100, 40)
	if !strings.Contains(appView, "Runora System & Tools Inspector") {
		t.Errorf("expected view to contain 'Runora System & Tools Inspector', got:\n%s", appView)
	}
	if !strings.Contains(appView, "Runora CLI Version:") {
		t.Errorf("expected app view to contain 'Runora CLI Version:', got:\n%s", appView)
	}

	// 6. Test Token Edit Mode
	// Enter token edit mode via key 'g'
	model.SelectedRuntime = 0
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if !model.tokenEditActive {
		t.Fatalf("expected tokenEditActive to be true after pressing 'g'")
	}
	editView := model.View(100, 40)

	if !strings.Contains(editView, "API Credentials Inspector") {
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
