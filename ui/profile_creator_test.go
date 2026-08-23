package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/BIJJUDAMA/runora/profile"
)

func TestProfileCreatorModelFullNavigationAndSave(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "profile-creator-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	pc := NewProfileCreatorModel(tempDir)
	if pc.mode != ModeCreate {
		t.Errorf("expected ModeCreate, got %v", pc.mode)
	}
	if !pc.flashAttn {
		t.Errorf("expected flashAttn default to true")
	}

	// 1. Enter Name
	for _, ch := range "Super-LLM" {
		pc.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	// 2. Tab to Context
	pc.Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, ch := range "16384" {
		pc.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	// 3. Tab to Threads
	pc.Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, ch := range "12" {
		pc.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	// 4. Tab to GPU Layers
	pc.Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, ch := range "60" {
		pc.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	// 5. Tab to Port
	pc.Update(tea.KeyMsg{Type: tea.KeyTab})
	for _, ch := range "8080" {
		pc.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	// 6. Tab to Flash Attention toggle
	pc.Update(tea.KeyMsg{Type: tea.KeyTab})
	if pc.focusIndex != 5 {
		t.Fatalf("expected focusIndex 5, got %d", pc.focusIndex)
	}
	// Toggle off and on
	pc.Update(tea.KeyMsg{Type: tea.KeySpace})
	if pc.flashAttn != false {
		t.Errorf("expected flashAttn to be toggled to false")
	}
	pc.Update(tea.KeyMsg{Type: tea.KeySpace})
	if pc.flashAttn != true {
		t.Errorf("expected flashAttn to be toggled back to true")
	}

	// 7. Tab to KV Cache Quantization
	pc.Update(tea.KeyMsg{Type: tea.KeyTab})
	if pc.focusIndex != 6 {
		t.Fatalf("expected focusIndex 6, got %d", pc.focusIndex)
	}
	// Cycle to Q8_0 (idx 1)
	pc.Update(tea.KeyMsg{Type: tea.KeyRight})
	if pc.kvQuantIdx != 1 {
		t.Errorf("expected kvQuantIdx 1 (Q8_0), got %d", pc.kvQuantIdx)
	}

	// 8. Tab to Custom Arguments
	pc.Update(tea.KeyMsg{Type: tea.KeyTab})
	if pc.focusIndex != 7 {
		t.Fatalf("expected focusIndex 7, got %d", pc.focusIndex)
	}
	for _, ch := range "--temp 0.8 --top-k 40" {
		pc.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	// 9. Render view and assert 0 emojis
	rendered := pc.View(80, 25)
	if !strings.Contains(rendered, "Super-LLM") {
		t.Errorf("expected view to contain 'Super-LLM', got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Q8_0") {
		t.Errorf("expected view to contain 'Q8_0', got:\n%s", rendered)
	}
	for _, r := range rendered {
		if isEmoji(r) {
			t.Errorf("found emoji %q in profile creator view", string(r))
		}
	}

	// 10. Press Enter to Save
	_, done, saved := pc.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !done || !saved {
		t.Fatalf("expected save to succeed with done=true, saved=true, got done=%v saved=%v", done, saved)
	}

	// Verify saved file on disk
	profs, err := profile.LoadAll(tempDir)
	if err != nil {
		t.Fatalf("failed to load profiles: %v", err)
	}

	var found *profile.Profile
	for _, p := range profs {
		if p.Name == "Super-LLM" {
			found = p
			break
		}
	}

	if found == nil {
		t.Fatalf("saved profile 'Super-LLM' not found on disk")
	}
	if found.Context != 16384 || found.Threads != 12 || found.GPULayers != 60 || found.Port != 8080 {
		t.Errorf("profile fields mismatch: %+v", found)
	}
	if !found.FlashAttention {
		t.Errorf("expected FlashAttention to be true")
	}
	if found.CacheTypeK != "q8_0" || found.CacheTypeV != "q8_0" {
		t.Errorf("expected KV cache types 'q8_0', got K=%q V=%q", found.CacheTypeK, found.CacheTypeV)
	}
	if found.CustomArgs != "--temp 0.8 --top-k 40" {
		t.Errorf("expected CustomArgs '--temp 0.8 --top-k 40', got %q", found.CustomArgs)
	}
}

func TestProfileEditorModel_EditAndDuplicate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "profile-editor-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	orig := &profile.Profile{
		Name:           "Original",
		Context:        4096,
		Threads:        6,
		GPULayers:      40,
		BatchSize:      512,
		Host:           "127.0.0.1",
		Port:           50505,
		FlashAttention: true,
		CacheTypeK:     "q4_0",
		CacheTypeV:     "q4_0",
		CustomArgs:     "--rope-scaling-type linear",
	}
	_ = profile.SaveProfile(tempDir, orig)

	// Test Duplicate
	pcDup := NewProfileEditorModel(tempDir, orig, true)
	if pcDup.mode != ModeDuplicate {
		t.Errorf("expected ModeDuplicate, got %v", pcDup.mode)
	}
	if pcDup.nameInput.Value() != "Original (Copy)" {
		t.Errorf("expected name 'Original (Copy)', got %q", pcDup.nameInput.Value())
	}
	if pcDup.kvQuantIdx != 2 { // Q4_0
		t.Errorf("expected kvQuantIdx 2 (Q4_0), got %d", pcDup.kvQuantIdx)
	}

	_, done, saved := pcDup.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !done || !saved {
		t.Fatalf("expected duplicate save to succeed")
	}

	// Verify both exist
	if _, err := os.Stat(filepath.Join(tempDir, "original.json")); err != nil {
		t.Errorf("original profile was deleted during duplicate")
	}
	if _, err := os.Stat(filepath.Join(tempDir, "original_(copy).json")); err != nil {
		t.Errorf("duplicated profile was not saved to disk: %v", err)
	}
}
