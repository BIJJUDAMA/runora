package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/config"
	"github.com/BIJJUDAMA/runora/model"
	"github.com/BIJJUDAMA/runora/profile"
	"github.com/BIJJUDAMA/runora/runner"
)

func TestBrowserModelInit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llama-ui-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	modelsDir := filepath.Join(tempDir, "models")
	cacheDir := filepath.Join(tempDir, "cache")
	_ = os.MkdirAll(modelsDir, 0755)
	_ = os.MkdirAll(cacheDir, 0755)

	cfg := config.DefaultConfig()
	cfg.Paths.Models = modelsDir
	cfg.Paths.Cache = cacheDir

	srv := runner.NewMultiRuntimeManager(cacheDir)
	model := NewBrowserModel(cfg, srv)

	if model.loading != true {
		t.Errorf("expected model to start in loading state")
	}

	cmd := model.Init()
	if cmd == nil {
		t.Errorf("expected Init to return a batch command, got nil")
	}
}

func TestBrowserSidebarRebuild(t *testing.T) {
	cfg := config.DefaultConfig()
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// Add mock GGUF models
	bm.models = []*model.GGUFMetadata{
		{Name: "Qwen 2.5", FilePath: "models/qwen2.5.gguf"},
		{Name: "Gemma 2", FilePath: "models/gemma2.gguf"},
		{Name: "Llama 3", FilePath: "models/llama3.gguf"},
	}
	bm.filterModels()

	// 1. By default, there should only be "ALL MODELS" and the three models
	expectedInitialCount := 4
	if len(bm.sidebarItems) != expectedInitialCount {
		t.Errorf("expected %d sidebar items initially, got %d", expectedInitialCount, len(bm.sidebarItems))
	}
	if bm.sidebarItems[0].Type != ItemSectionHeader || bm.sidebarItems[0].Label != "ALL MODELS" {
		t.Errorf("expected first item to be ALL MODELS section header, got %+v", bm.sidebarItems[0])
	}

	// 2. Add Gemma 2 to Favorites
	cfg.ToggleFavorite("models/gemma2.gguf")
	bm.rebuildSidebar()
	expectedFavCount := 6
	if len(bm.sidebarItems) != expectedFavCount {
		t.Errorf("expected %d sidebar items after favoriting, got %d: %+v", expectedFavCount, len(bm.sidebarItems), bm.sidebarItems)
	}

	// 3. Test Navigation and selection adjustment
	bm.selected = 2
	bm.adjustSelection()
	if bm.sidebarItems[bm.selected].Type == ItemSectionHeader {
		t.Errorf("adjustSelection failed, selected is still on section header: %d", bm.selected)
	}
}

func TestBrowserBenchmarkTrigger(t *testing.T) {
	cfg := config.DefaultConfig()
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// Add mock GGUF models
	bm.models = []*model.GGUFMetadata{
		{Name: "Qwen 2.5", FilePath: "models/qwen2.5.gguf"},
	}
	bm.filterModels()

	// Initial screen mode is ScreenBrowser
	if bm.screenMode != ScreenBrowser {
		t.Errorf("expected screenMode to be ScreenBrowser, got %d", bm.screenMode)
	}

	// Press "b" to trigger benchmark
	var nextModel tea.Model
	var cmd tea.Cmd
	nextModel, cmd = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	updated := nextModel.(*BrowserModel)

	if updated.screenMode != ScreenBenchmarkProgress {
		t.Errorf("expected screenMode to transition to ScreenBenchmarkProgress, got %d", updated.screenMode)
	}
	if updated.benchmarkProgress == nil {
		t.Errorf("expected benchmarkProgress model to be initialized")
	}
	if cmd == nil {
		t.Errorf("expected benchmark launch command to be dispatched")
	}
}

func TestBrowserMonitorTrigger(t *testing.T) {
	cfg := config.DefaultConfig()
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// Initial screen mode is ScreenBrowser
	if bm.screenMode != ScreenBrowser {
		t.Errorf("expected screenMode to be ScreenBrowser, got %d", bm.screenMode)
	}

	// Press "m" to trigger monitor
	nextModel, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	updated := nextModel.(*BrowserModel)

	if updated.screenMode != ScreenServerMonitor {
		t.Errorf("expected screenMode to transition to ScreenServerMonitor, got %d", updated.screenMode)
	}
	if updated.monitorModel == nil {
		t.Errorf("expected monitorModel to be initialized")
	}
}

func TestBrowserSettingsTrigger(t *testing.T) {
	cfg := config.DefaultConfig()
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// Initial screen mode is ScreenBrowser
	if bm.screenMode != ScreenBrowser {
		t.Errorf("expected screenMode to be ScreenBrowser, got %d", bm.screenMode)
	}

	// Press "u" to trigger settings
	nextModel, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	updated := nextModel.(*BrowserModel)

	if updated.screenMode != ScreenSettings {
		t.Errorf("expected screenMode to transition to ScreenSettings, got %d", updated.screenMode)
	}
	if updated.lifecycleModel == nil {
		t.Errorf("expected lifecycleModel to be initialized")
	}
}

func TestBrowserTokenConfiguration(t *testing.T) {
	// Backup user config if exists
	hasUserConfig := false
	if _, err := os.Stat("config.json"); err == nil {
		hasUserConfig = true
		_ = os.Rename("config.json", "config.json.tmp")
	}
	defer func() {
		_ = os.Remove("config.json")
		if hasUserConfig {
			_ = os.Rename("config.json.tmp", "config.json")
		}
	}()

	cfg := config.DefaultConfig()
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// Transition to settings
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	bm = m.(*BrowserModel)

	// Trigger token editing mode
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	bm = m.(*BrowserModel)

	if !bm.lifecycleModel.tokenEditActive {
		t.Errorf("expected tokenEditActive to be true after pressing 't'")
	}

	// Simulate typing "hf_testtoken123"
	for _, char := range "hf_testtoken123" {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		bm = m.(*BrowserModel)
	}

	// Press enter to save
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	if bm.lifecycleModel.tokenEditActive {
		t.Errorf("expected tokenEditActive to be false after pressing Enter")
	}

	if bm.config.HFToken != "hf_testtoken123" {
		t.Errorf("expected HFToken in config to be hf_testtoken123, got %q", bm.config.HFToken)
	}
}

func TestBrowserDownloaderTrigger(t *testing.T) {
	cfg := config.DefaultConfig()
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// Initial screen mode is ScreenBrowser
	if bm.screenMode != ScreenBrowser {
		t.Errorf("expected screenMode to be ScreenBrowser, got %d", bm.screenMode)
	}

	// Press "d" to trigger downloader
	nextModel, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	updated := nextModel.(*BrowserModel)

	if updated.screenMode != ScreenDownloader {
		t.Errorf("expected screenMode to transition to ScreenDownloader, got %d", updated.screenMode)
	}
	if updated.downloaderModel == nil {
		t.Errorf("expected downloaderModel to be initialized")
	}
}

func TestBrowserDownloaderDirectURL(t *testing.T) {
	// Backup user config if exists
	hasUserConfig := false
	if _, err := os.Stat("config.json"); err == nil {
		hasUserConfig = true
		_ = os.Rename("config.json", "config.json.tmp")
	}
	defer func() {
		_ = os.Remove("config.json")
		if hasUserConfig {
			_ = os.Rename("config.json.tmp", "config.json")
		}
	}()

	cfg := config.DefaultConfig()
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// 1. Transition to Downloader screen
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	bm = m.(*BrowserModel)

	if bm.screenMode != ScreenDownloader {
		t.Fatalf("expected screenMode to be ScreenDownloader, got %d", bm.screenMode)
	}

	// 2. By default, focus starts on FocusURL in our simplified downloader.
	if bm.downloaderModel.focus != FocusURL {
		t.Fatalf("expected initial focus to be FocusURL, got %d", bm.downloaderModel.focus)
	}

	// 3. Type URL: "http://example.com/models/test-model.gguf"
	for _, char := range "http://example.com/models/test-model.gguf" {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		bm = m.(*BrowserModel)
	}

	// Tab to switch to filename field
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	bm = m.(*BrowserModel)

	if bm.downloaderModel.focus != FocusFilename {
		t.Errorf("expected focus to switch to FocusFilename, got focus %d", bm.downloaderModel.focus)
	}

	// Type custom filename: "custom-name.gguf"
	for _, char := range "custom-name.gguf" {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		bm = m.(*BrowserModel)
	}

	// Press enter to queue download
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	// Focus should return to FocusURL
	if bm.downloaderModel.focus != FocusURL {
		t.Errorf("expected focus to return to FocusURL after submission, got %d", bm.downloaderModel.focus)
	}

	tasks := bm.downloadQueue.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task in queue, got %d", len(tasks))
	}

	task := tasks[0]
	if task.FileName != "custom-name.gguf" {
		t.Errorf("expected task filename to be 'custom-name.gguf', got %q", task.FileName)
	}
	if task.URL != "http://example.com/models/test-model.gguf" {
		t.Errorf("expected task URL to be correct, got %q", task.URL)
	}
}

func TestBrowserDownloaderHFRepo(t *testing.T) {
	// Backup user config if exists
	hasUserConfig := false
	if _, err := os.Stat("config.json"); err == nil {
		hasUserConfig = true
		_ = os.Rename("config.json", "config.json.tmp")
	}
	defer func() {
		_ = os.Remove("config.json")
		if hasUserConfig {
			_ = os.Rename("config.json.tmp", "config.json")
		}
	}()

	cfg := config.DefaultConfig()
	cfg.HFToken = "dummy_token"
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// 1. Transition to Downloader screen
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	bm = m.(*BrowserModel)

	// 2. Type Repo ID: "unsloth/gemma-4-E4B-it-GGUF"
	for _, char := range "unsloth/gemma-4-E4B-it-GGUF" {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		bm = m.(*BrowserModel)
	}

	// Tab to switch to filename field
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	bm = m.(*BrowserModel)

	// Type filename: "gemma-4-E4B-it-Q4_K_M.gguf"
	for _, char := range "gemma-4-E4B-it-Q4_K_M.gguf" {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		bm = m.(*BrowserModel)
	}

	// Press enter to queue download
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	tasks := bm.downloadQueue.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task in queue, got %d", len(tasks))
	}

	task := tasks[0]
	if task.FileName != "gemma-4-E4B-it-Q4_K_M.gguf" {
		t.Errorf("expected task filename to be 'gemma-4-E4B-it-Q4_K_M.gguf', got %q", task.FileName)
	}
	expectedURL := "https://huggingface.co/unsloth/gemma-4-E4B-it-GGUF/resolve/main/gemma-4-E4B-it-Q4_K_M.gguf"
	if task.URL != expectedURL {
		t.Errorf("expected task URL to be %q, got %q", expectedURL, task.URL)
	}
}

func TestBrowserDownloaderHFRepoResolve(t *testing.T) {
	// Backup user config if exists
	hasUserConfig := false
	if _, err := os.Stat("config.json"); err == nil {
		hasUserConfig = true
		_ = os.Rename("config.json", "config.json.tmp")
	}
	defer func() {
		_ = os.Remove("config.json")
		if hasUserConfig {
			_ = os.Rename("config.json.tmp", "config.json")
		}
	}()

	cfg := config.DefaultConfig()
	cfg.HFToken = "dummy_token"
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// 1. Transition to Downloader screen
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	bm = m.(*BrowserModel)

	// 2. Type Repo ID: "unsloth/gemma-4-E4B-it-GGUF"
	for _, char := range "unsloth/gemma-4-E4B-it-GGUF" {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		bm = m.(*BrowserModel)
	}

	// 3. Press Enter without typing a filename
	m, cmd := bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	if cmd == nil {
		t.Fatalf("expected an async resolving command to be returned, got nil")
	}
	if !bm.downloaderModel.resolving {
		t.Errorf("expected resolving to be true after Enter on repository")
	}

	// 4. Send hfResolveMsg to simulate resolving completion
	files := []model.HFSibling{
		{Rpath: "file1.gguf", Size: 100},
		{Rpath: "file2.gguf", Size: 200},
	}
	m, _ = bm.Update(hfResolveMsg{repoID: "unsloth/gemma-4-E4B-it-GGUF", files: files})
	bm = m.(*BrowserModel)

	if bm.downloaderModel.resolving {
		t.Errorf("expected resolving to be false after completion")
	}
	if bm.downloaderModel.focus != FocusFileList {
		t.Errorf("expected downloader focus to be FocusFileList, got %v", bm.downloaderModel.focus)
	}
	if len(bm.downloaderModel.resolvedFiles) != 2 {
		t.Errorf("expected 2 resolved files, got %d", len(bm.downloaderModel.resolvedFiles))
	}

	// 5. Navigate Down/j to choose file2.gguf
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	bm = m.(*BrowserModel)
	if bm.downloaderModel.selectedFileIdx != 1 {
		t.Errorf("expected selectedFileIdx to be 1 after pressing j, got %d", bm.downloaderModel.selectedFileIdx)
	}

	// 6. Press Enter to select file2.gguf
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	if bm.downloaderModel.focus != FocusURL {
		t.Errorf("expected focus to return to FocusURL, got %v", bm.downloaderModel.focus)
	}

	tasks := bm.downloadQueue.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task in queue, got %d", len(tasks))
	}
	task := tasks[0]
	if task.FileName != "file2.gguf" {
		t.Errorf("expected task filename to be 'file2.gguf', got %q", task.FileName)
	}
	expectedURL := "https://huggingface.co/unsloth/gemma-4-E4B-it-GGUF/resolve/main/file2.gguf"
	if task.URL != expectedURL {
		t.Errorf("expected task URL to be %q, got %q", expectedURL, task.URL)
	}
}

func TestBrowserCreateCustomProfile(t *testing.T) {
	tempProfilesDir, err := os.MkdirTemp("", "llama-manager-profiles-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempProfilesDir)

	cfg := config.DefaultConfig()
	cfg.Paths.Profiles = tempProfilesDir
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// Set some mock models so we can enter Dashboard
	bm.models = []*model.GGUFMetadata{
		{
			Name:     "Test Model",
			FilePath: "models/test.gguf",
		},
	}
	bm.rebuildSidebar()

	// Select the model entry (index 1 is the model since index 0 is Section Header)
	bm.selected = 1

	// 1. Enter Dashboard by pressing Enter
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	if bm.screenMode != ScreenDashboard {
		t.Fatalf("expected screenMode to be ScreenDashboard, got %d", bm.screenMode)
	}

	// 2. Press 'P' to open Profile Creator
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	bm = m.(*BrowserModel)

	if bm.screenMode != ScreenProfileCreator {
		t.Fatalf("expected screenMode to be ScreenProfileCreator, got %d", bm.screenMode)
	}

	// 3. Type Name: "Custom-Test-Profile"
	for _, char := range "Custom-Test-Profile" {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		bm = m.(*BrowserModel)
	}

	// Tab to Context size
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	bm = m.(*BrowserModel)

	// Type context size: "8192"
	for _, char := range "8192" {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		bm = m.(*BrowserModel)
	}

	// Tab to GPU layers
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	bm = m.(*BrowserModel)

	// Type GPU layers: "99"
	for _, char := range "99" {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		bm = m.(*BrowserModel)
	}

	// Tab to Port
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	bm = m.(*BrowserModel)

	// Type Port: "8085"
	for _, char := range "8085" {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{char}})
		bm = m.(*BrowserModel)
	}

	// Press Enter to save
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	// Should return to Dashboard
	if bm.screenMode != ScreenDashboard {
		t.Errorf("expected to return to ScreenDashboard, got %d", bm.screenMode)
	}

	// Verify profile is created and loaded
	found := false
	for _, p := range bm.profiles {
		if p.Name == "Custom-Test-Profile" {
			found = true
			if p.Context != 8192 {
				t.Errorf("expected context size 8192, got %d", p.Context)
			}
			if p.GPULayers != 99 {
				t.Errorf("expected GPU layers 99, got %d", p.GPULayers)
			}
			if p.Port != 8085 {
				t.Errorf("expected port 8085, got %d", p.Port)
			}
		}
	}
	if !found {
		t.Errorf("created custom profile 'Custom-Test-Profile' was not found in loaded profiles")
	}
}

func TestBrowserOnboardingTour(t *testing.T) {
	// Backup user config if exists
	hasUserConfig := false
	if _, err := os.Stat("config.json"); err == nil {
		hasUserConfig = true
		_ = os.Rename("config.json", "config.json.tmp")
	}
	defer func() {
		_ = os.Remove("config.json")
		if hasUserConfig {
			_ = os.Rename("config.json.tmp", "config.json")
		}
	}()

	cfg := config.DefaultConfig()
	cfg.OnboardingCompleted = false // force onboarding
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)
	bm.onboardingActive = true // force onboarding in test environment

	if !bm.onboardingActive {
		t.Errorf("expected onboarding to be active initially")
	}
	if bm.onboardingStep != StepWelcome {
		t.Errorf("expected onboarding to start at StepWelcome, got %d", bm.onboardingStep)
	}

	// Press Enter to advance to StepStorage
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)
	if bm.onboardingStep != StepStorage {
		t.Errorf("expected onboarding to advance to StepStorage, got %d", bm.onboardingStep)
	}

	// Press 'b' to go back
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	bm = m.(*BrowserModel)
	if bm.onboardingStep != StepWelcome {
		t.Errorf("expected onboarding to go back to StepWelcome, got %d", bm.onboardingStep)
	}

	// Advance to StepTokens (Welcome -> Storage -> Tokens)
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)
	if bm.onboardingStep != StepTokens {
		t.Fatalf("expected step to be StepTokens, got %v", bm.onboardingStep)
	}

	// Simulate typing tokens
	bm.onboardingGHTokenInput.SetValue("ghp_test_token_123")
	bm.onboardingTokenInput.SetValue("hf_test_token_456")
	// Press enter on StepTokens to submit
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	if bm.onboardingStep != StepRuntime {
		t.Errorf("expected step to advance to StepRuntime, got %v", bm.onboardingStep)
	}
	if bm.config.GitHubToken != "ghp_test_token_123" {
		t.Errorf("expected config GitHubToken to be ghp_test_token_123, got %q", bm.config.GitHubToken)
	}
	if bm.config.HFToken != "hf_test_token_456" {
		t.Errorf("expected config HFToken to be hf_test_token_456, got %q", bm.config.HFToken)
	}

	// Advance to StepFinished
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)
	if bm.onboardingStep != StepFinished {
		t.Errorf("expected step to advance to StepFinished, got %v", bm.onboardingStep)
	}

	// Test that background messages like discoverMsg fall through during onboarding
	bm.onboardingActive = true
	bm.loading = true
	m, _ = bm.Update(discoverMsg{models: []*model.GGUFMetadata{}})
	bm = m.(*BrowserModel)
	if bm.loading {
		t.Errorf("expected discoverMsg to not be swallowed and loading to be false during onboarding")
	}

	// Skip tour
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	bm = m.(*BrowserModel)
	if bm.onboardingActive {
		t.Errorf("expected onboarding to be deactivated after pressing Esc")
	}
	if !bm.config.OnboardingCompleted {
		t.Errorf("expected OnboardingCompleted to be set to true in config")
	}
}

func TestOnboardingWizardFlowWide(t *testing.T) {
	// Backup user config if exists
	hasUserConfig := false
	if _, err := os.Stat("config.json"); err == nil {
		hasUserConfig = true
		_ = os.Rename("config.json", "config.json.tmp")
	}
	defer func() {
		_ = os.Remove("config.json")
		if hasUserConfig {
			_ = os.Rename("config.json.tmp", "config.json")
		}
	}()

	cfg := config.DefaultConfig()
	cfg.OnboardingCompleted = false
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)
	bm.onboardingActive = true
	bm.width = 120
	bm.height = 36

	// 1. Verify 86-cell width bounds
	renderedWide := bm.onboardingOverlayView(120, 36)
	wideWidth := lipgloss.Width(renderedWide)
	// Box style has Width(86) plus DoubleBorder (2) and Padding(1, 2) (4) = 92
	if wideWidth < 86 {
		t.Errorf("expected wide width (%d) to be at least 86 cells", wideWidth)
	}

	// When width is constrained to 70 cells:
	renderedNarrow := bm.onboardingOverlayView(70, 36)
	narrowWidth := lipgloss.Width(renderedNarrow)
	if narrowWidth >= wideWidth {
		t.Errorf("expected narrow width (%d) to be smaller than wide width (%d)", narrowWidth, wideWidth)
	}

	// Helper to assert zero emojis in rendered string
	assertZeroEmojis := func(stepName string, content string) {
		for _, r := range content {
			if (r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
				(r >= 0x1F300 && r <= 0x1F5FF) || // Misc Symbols and Pictographs
				(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map
				(r >= 0x1F700 && r <= 0x1F77F) || // Alchemical Symbols
				(r >= 0x1F780 && r <= 0x1F7FF) || // Geometric Shapes Extended
				(r >= 0x1F800 && r <= 0x1F8FF) || // Supplemental Arrows-C
				(r >= 0x1F900 && r <= 0x1F9FF) || // Supplemental Symbols and Pictographs
				(r >= 0x1FA00 && r <= 0x1FA6F) || // Chess Symbols
				(r >= 0x1FA70 && r <= 0x1FAFF) || // Symbols and Pictographs Extended-A
				(r >= 0x2600 && r <= 0x26FF && r != 0x2605 && r != 0x2606) || // Misc symbols
				(r >= 0x2700 && r <= 0x27BF && r != 0x2713 && r != 0x2717) { // Dingbats
				t.Errorf("found emoji %q (code: %U) in step %s", r, r, stepName)
			}
		}
	}

	// Check Step 1: StepWelcome
	if bm.onboardingStep != StepWelcome {
		t.Fatalf("expected StepWelcome (0), got %v", bm.onboardingStep)
	}
	assertZeroEmojis("StepWelcome", bm.onboardingOverlayView(120, 36))

	// Advance to Step 2: StepStorage
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)
	if bm.onboardingStep != StepStorage {
		t.Fatalf("expected StepStorage (1), got %v", bm.onboardingStep)
	}
	assertZeroEmojis("StepStorage", bm.onboardingOverlayView(120, 36))

	// Advance to Step 3: StepTokens
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)
	if bm.onboardingStep != StepTokens {
		t.Fatalf("expected StepTokens (2), got %v", bm.onboardingStep)
	}
	assertZeroEmojis("StepTokens", bm.onboardingOverlayView(120, 36))

	// Test back navigation from StepTokens with Ctrl+B
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	bm = m.(*BrowserModel)
	if bm.onboardingStep != StepStorage {
		t.Fatalf("expected StepStorage after Ctrl+B, got %v", bm.onboardingStep)
	}

	// Return to StepTokens
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)
	if bm.onboardingStep != StepTokens {
		t.Fatalf("expected StepTokens, got %v", bm.onboardingStep)
	}

	// Test tab switching in StepTokens
	if bm.onboardingTokenFocus != 0 {
		t.Errorf("expected initial token focus 0 (GitHub token), got %d", bm.onboardingTokenFocus)
	}
	// Type into GitHub token
	for _, ch := range "ghp_securetoken999" {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		bm = m.(*BrowserModel)
	}
	if bm.onboardingGHTokenInput.Value() != "ghp_securetoken999" {
		t.Errorf("expected gh token input to be ghp_securetoken999, got %q", bm.onboardingGHTokenInput.Value())
	}

	// Switch focus to Hugging Face token with Tab
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	bm = m.(*BrowserModel)
	if bm.onboardingTokenFocus != 1 {
		t.Errorf("expected token focus to switch to 1 (HF token), got %d", bm.onboardingTokenFocus)
	}
	for _, ch := range "hf_supersecret888" {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		bm = m.(*BrowserModel)
	}
	if bm.onboardingTokenInput.Value() != "hf_supersecret888" {
		t.Errorf("expected hf token input to be hf_supersecret888, got %q", bm.onboardingTokenInput.Value())
	}

	// Submit tokens with Enter
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	// Step 4: StepRuntime
	if bm.onboardingStep != StepRuntime {
		t.Fatalf("expected StepRuntime (3), got %v", bm.onboardingStep)
	}
	assertZeroEmojis("StepRuntime", bm.onboardingOverlayView(120, 36))

	// Verify config saved tokens
	if bm.config.GitHubToken != "ghp_securetoken999" {
		t.Errorf("expected config.GitHubToken to be saved as ghp_securetoken999, got %q", bm.config.GitHubToken)
	}
	if bm.config.HFToken != "hf_supersecret888" {
		t.Errorf("expected config.HFToken to be saved as hf_supersecret888, got %q", bm.config.HFToken)
	}

	// Test toggling release channel with 'c'
	initChannel := bm.onboardingChannel
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	bm = m.(*BrowserModel)
	if bm.onboardingChannel == initChannel {
		t.Errorf("expected release channel to toggle after pressing 'c'")
	}

	// Test cycling accelerator with 'a'
	initBackend := bm.onboardingBackend
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	bm = m.(*BrowserModel)
	if bm.onboardingBackend == initBackend && len(bm.onboardingBackends) > 1 {
		t.Errorf("expected backend to change after pressing 'a'")
	}

	// Advance to Step 5: StepFinished
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)
	if bm.onboardingStep != StepFinished {
		t.Fatalf("expected StepFinished (4), got %v", bm.onboardingStep)
	}
	assertZeroEmojis("StepFinished", bm.onboardingOverlayView(120, 36))

	// Complete onboarding with Enter
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)
	if bm.onboardingActive {
		t.Errorf("expected onboarding to be inactive after finishing")
	}
	if !bm.config.OnboardingCompleted {
		t.Errorf("expected config.OnboardingCompleted to be true")
	}
}

func TestBrowserDownloaderClearQueue(t *testing.T) {
	// Backup user config if exists
	hasUserConfig := false
	if _, err := os.Stat("config.json"); err == nil {
		hasUserConfig = true
		_ = os.Rename("config.json", "config.json.tmp")
	}
	defer func() {
		_ = os.Remove("config.json")
		if hasUserConfig {
			_ = os.Rename("config.json.tmp", "config.json")
		}
	}()

	cfg := config.DefaultConfig()
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// Transition to Downloader screen
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	bm = m.(*BrowserModel)

	// Add two mock tasks manually for testing
	t1 := bm.downloadQueue.AddTask("org/repo1", "m1.gguf", 100, "http://example.com/m1.gguf")
	t2 := bm.downloadQueue.AddTask("org/repo2", "m2.gguf", 200, "http://example.com/m2.gguf")

	t1.Status = model.StatusCompleted
	t2.Status = model.StatusFailed

	// Move focus to download queue
	// Focus transitions: FocusURL -> FocusFilename -> FocusQueue
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	bm = m.(*BrowserModel)
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	bm = m.(*BrowserModel)

	if bm.downloaderModel.focus != FocusQueue {
		t.Fatalf("expected focus to be FocusQueue, got %d", bm.downloaderModel.focus)
	}

	// 1. Remove t1 individually by selecting it and pressing 'c'
	bm.downloaderModel.selectedTaskIdx = 0
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	bm = m.(*BrowserModel)

	tasks := bm.downloadQueue.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task remaining, got %d", len(tasks))
	}
	if tasks[0] != t2 {
		t.Errorf("expected remaining task to be t2")
	}

	// 2. Add t1 back (as completed) and test clearing all finished tasks with 'x'
	t1 = bm.downloadQueue.AddTask("org/repo1", "m1.gguf", 100, "http://example.com/m1.gguf")
	t1.Status = model.StatusCompleted

	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	bm = m.(*BrowserModel)

	if len(bm.downloadQueue.GetTasks()) != 0 {
		t.Errorf("expected all finished tasks to be cleared, got %d tasks remaining", len(bm.downloadQueue.GetTasks()))
	}
}

type mockModelRuntime struct {
	instances []runner.InstanceInfo
}

func (m *mockModelRuntime) Start(modelPath string, opts runner.StartOptions) error { return nil }
func (m *mockModelRuntime) Stop() error { return nil }
func (m *mockModelRuntime) StopInstance(port int) error { return nil }
func (m *mockModelRuntime) GetStatus() (runner.ServerStatus, string, int) { return runner.StatusStopped, "", 0 }
func (m *mockModelRuntime) GetAllInstances() []runner.InstanceInfo { return m.instances }
func (m *mockModelRuntime) Capabilities() []runner.TaskType { return nil }

func TestFindAvailablePort(t *testing.T) {
	srv := &mockModelRuntime{
		instances: []runner.InstanceInfo{
			{Port: 50505, ModelPath: "models/model1.gguf"},
			{Port: 50506, ModelPath: "models/model2.gguf"},
		},
	}

	// 1. If starting model1.gguf on 50505 (same model/port), it should return 50505 (no change)
	port := findAvailablePort(50505, srv, "models/model1.gguf")
	if port != 50505 {
		t.Errorf("expected port 50505, got %d", port)
	}

	// 2. If starting model3.gguf on 50505, since 50505 and 50506 are busy by other models, it should return 50507
	port = findAvailablePort(50505, srv, "models/model3.gguf")
	if port != 50507 {
		t.Errorf("expected port 50507, got %d", port)
	}
}

func TestUnifiedLifecycleNavigation(t *testing.T) {
	cfg := config.DefaultConfig()
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// Transition to settings screen
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")})
	bm = m.(*BrowserModel)

	if bm.screenMode != ScreenSettings {
		t.Fatalf("expected screenMode to be ScreenSettings, got %d", bm.screenMode)
	}

	if bm.lifecycleModel.SelectedRuntime != 0 {
		t.Errorf("expected initial SelectedRuntime to be 0 (llama.cpp), got %d", bm.lifecycleModel.SelectedRuntime)
	}

	// Press Tab to cycle to ONNX Runtime (1)
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	bm = m.(*BrowserModel)
	if bm.lifecycleModel.SelectedRuntime != 1 {
		t.Errorf("expected SelectedRuntime to be 1 after Tab, got %d", bm.lifecycleModel.SelectedRuntime)
	}

	// Press Tab to cycle to Runora App (2)
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	bm = m.(*BrowserModel)
	if bm.lifecycleModel.SelectedRuntime != 2 {
		t.Errorf("expected SelectedRuntime to be 2 after Tab, got %d", bm.lifecycleModel.SelectedRuntime)
	}

	// Press Tab again to wrap back to llama.cpp (0)
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyTab})
	bm = m.(*BrowserModel)
	if bm.lifecycleModel.SelectedRuntime != 0 {
		t.Errorf("expected SelectedRuntime to wrap to 0, got %d", bm.lifecycleModel.SelectedRuntime)
	}

	// Press Up arrow to move backwards to Runora App (2)
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyUp})
	bm = m.(*BrowserModel)
	if bm.lifecycleModel.SelectedRuntime != 2 {
		t.Errorf("expected SelectedRuntime to be 2 after Up arrow, got %d", bm.lifecycleModel.SelectedRuntime)
	}
}

func TestThemeRegistryAndSwitching(t *testing.T) {
	for _, theme := range registeredThemes {
		if theme.ID() == "" {
			t.Errorf("theme missing ID")
		}
		if theme.Name() == "" {
			t.Errorf("theme missing Name")
		}
		if theme.Description() == "" {
			t.Errorf("theme %s missing Description", theme.Name())
		}
		p := theme.Palette()
		if p.Primary == nil || p.Secondary == nil || p.Border == nil {
			t.Errorf("theme %s has invalid palette", theme.Name())
		}
		ApplyTheme(theme.ID())
		if ActiveTheme.ID() != theme.ID() {
			t.Errorf("expected active theme %s, got %s", theme.ID(), ActiveTheme.ID())
		}
	}

	next := NextThemeName("forest")
	if next != "dracula" {
		t.Errorf("expected next theme after forest to be dracula, got %s", next)
	}
}

func TestAccessibilityAndLightThemes(t *testing.T) {
	// Test Solarized Light
	sol := resolveTheme("solarized-light")
	if sol.ID() != "solarized-light" || sol.Name() != "Solarized Light" {
		t.Errorf("expected Solarized Light theme, got ID=%s Name=%s", sol.ID(), sol.Name())
	}
	solPal := sol.Palette()
	if solPal.Primary == nil || solPal.Secondary == nil || solPal.Text == nil {
		t.Errorf("invalid Solarized Light palette")
	}

	// Test Paper Light
	paper := resolveTheme("paper-light")
	if paper.ID() != "paper-light" || paper.Name() != "Paper Light" {
		t.Errorf("expected Paper Light theme, got ID=%s Name=%s", paper.ID(), paper.Name())
	}
	paperPal := paper.Palette()
	if paperPal.Primary == nil || paperPal.Text == nil {
		t.Errorf("invalid Paper Light palette")
	}

	// Test High Contrast (WCAG AAA)
	hc := resolveTheme("high-contrast")
	if hc.ID() != "high-contrast" || hc.Name() != "High Contrast" {
		t.Errorf("expected High Contrast theme, got ID=%s Name=%s", hc.ID(), hc.Name())
	}
	hcPal := hc.Palette()
	if hcPal.Primary == nil || hcPal.Text == nil || hcPal.Border == nil {
		t.Errorf("invalid High Contrast palette")
	}
}

func TestThemePickerModel(t *testing.T) {
	picker := NewThemePickerModel("forest")
	if picker.ActiveThemeItem().ID() != "forest" {
		t.Errorf("expected initial active theme to be forest, got %s", picker.ActiveThemeItem().ID())
	}

	// Navigate Down (j)
	_, done, applied, themeID := picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if done || applied {
		t.Errorf("expected done=false, applied=false on navigation")
	}
	if themeID != "dracula" {
		t.Errorf("expected next theme to be dracula, got %s", themeID)
	}
	if ActiveTheme.ID() != "dracula" {
		t.Errorf("expected live preview to activate dracula theme, got %s", ActiveTheme.ID())
	}

	// Navigate Up (k)
	_, done, applied, themeID = picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if themeID != "forest" {
		t.Errorf("expected theme to return to forest, got %s", themeID)
	}

	// Test View rendering
	v := picker.View(80, 24)
	if len(v) == 0 {
		t.Errorf("expected non-empty View from ThemePickerModel")
	}

	// Test Confirmation (Enter)
	_, done, applied, chosen := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !done || !applied || chosen != "forest" {
		t.Errorf("expected done=true, applied=true, chosen=forest on Enter, got done=%v applied=%v chosen=%s", done, applied, chosen)
	}

	// Test Cancellation (Esc reverts to original theme)
	picker2 := NewThemePickerModel("forest")
	picker2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}) // moves to dracula
	if ActiveTheme.ID() != "dracula" {
		t.Errorf("expected live preview to be dracula")
	}
	_, done, applied, original := picker2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !done || applied || original != "forest" {
		t.Errorf("expected done=true, applied=false, original=forest on Esc, got done=%v applied=%v original=%s", done, applied, original)
	}
	if ActiveTheme.ID() != "forest" {
		t.Errorf("expected theme to revert to forest after Esc, got %s", ActiveTheme.ID())
	}
}

func TestToastManagerLifecycle(t *testing.T) {
	tm := NewToastManager()
	if tm.Active() {
		t.Errorf("expected empty ToastManager to not be active")
	}

	// Add informational toast
	cmd := tm.Show("Test notification")
	if cmd == nil {
		t.Errorf("expected non-nil tea.Cmd from Show")
	}
	if !tm.Active() || tm.Count() != 1 {
		t.Fatalf("expected 1 active toast, got %d", tm.Count())
	}
	if tm.GetToasts()[0].Message != "Test notification" {
		t.Errorf("expected toast message 'Test notification', got %q", tm.GetToasts()[0].Message)
	}

	// Add success, warning, danger toasts
	tm.ShowSuccess("Theme: Nord applied")
	tm.ShowWarning("Low disk space")
	tm.ShowDanger("Server failed")

	if tm.Count() != 4 {
		t.Errorf("expected 4 active toasts, got %d", tm.Count())
	}

	rendered := tm.RenderToasts()
	if len(rendered) == 0 {
		t.Errorf("expected non-empty RenderToasts output")
	}

	// Test Overlay
	base := "Line 1: Main View Content\nLine 2: Models Sidebar\nLine 3: Details Panel"
	overlaid := tm.Overlay(base, 80, 24)
	if len(overlaid) == 0 || overlaid == base {
		t.Errorf("expected overlaid output to differ from base")
	}

	// Test Remove by ID
	firstID := tm.GetToasts()[0].ID
	tm.Remove(firstID)
	if tm.Count() != 3 {
		t.Errorf("expected 3 toasts after removal, got %d", tm.Count())
	}

	// Test ToastExpireMsg
	remainingID := tm.GetToasts()[0].ID
	tm.Update(ToastExpireMsg{ID: remainingID})
	if tm.Count() != 2 {
		t.Errorf("expected 2 toasts after ToastExpireMsg, got %d", tm.Count())
	}

	// Test Clear
	tm.Clear()
	if tm.Active() || tm.Count() != 0 {
		t.Errorf("expected 0 toasts after Clear")
	}
}

func TestBrowserThemePickerIntegration(t *testing.T) {
	// Backup user config if exists
	hasUserConfig := false
	if _, err := os.Stat("config.json"); err == nil {
		hasUserConfig = true
		_ = os.Rename("config.json", "config.json.tmp")
	}
	defer func() {
		_ = os.Remove("config.json")
		if hasUserConfig {
			_ = os.Rename("config.json.tmp", "config.json")
		}
	}()

	cfg := config.DefaultConfig()
	cfg.Theme = "forest"
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// Press 'y' in ScreenBrowser to open ThemePickerModel
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	bm = m.(*BrowserModel)

	if !bm.themePickerActive || bm.themePicker == nil {
		t.Fatalf("expected themePickerActive to be true and themePicker initialized")
	}

	// Navigate to Dracula with 'j'
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	bm = m.(*BrowserModel)

	if ActiveTheme.ID() != "dracula" {
		t.Errorf("expected live preview to switch to dracula, got %s", ActiveTheme.ID())
	}

	// Confirm selection with Enter
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	if bm.themePickerActive {
		t.Errorf("expected themePickerActive to be false after confirmation")
	}
	if bm.config.Theme != "dracula" {
		t.Errorf("expected config theme to be dracula, got %s", bm.config.Theme)
	}
	if bm.toasts.Count() != 1 {
		t.Errorf("expected 1 toast queued after applying theme, got %d", bm.toasts.Count())
	}
}

func TestBrowserFloatingToastsOnActions(t *testing.T) {
	// Backup user config if exists
	hasUserConfig := false
	if _, err := os.Stat("config.json"); err == nil {
		hasUserConfig = true
		_ = os.Rename("config.json", "config.json.tmp")
	}
	defer func() {
		_ = os.Remove("config.json")
		if hasUserConfig {
			_ = os.Rename("config.json.tmp", "config.json")
		}
	}()

	cfg := config.DefaultConfig()
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	// Add mock GGUF models
	bm.models = []*model.GGUFMetadata{
		{Name: "Qwen 2.5", FilePath: "models/qwen2.5.gguf", Task: "TEXT_GENERATION"},
	}
	bm.rebuildSidebar()
	bm.selected = 1 // select model entry

	// 1. Toggle favorite with 'f'
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	bm = m.(*BrowserModel)

	if bm.toasts.Count() < 1 {
		t.Errorf("expected toast after toggling favorite")
	}

	// 2. Cycle task with 'e'
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	bm = m.(*BrowserModel)

	if bm.models[0].Task != "EMBEDDING" {
		t.Errorf("expected task to cycle to EMBEDDING, got %s", bm.models[0].Task)
	}

	// 3. Stop server with 's'
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	bm = m.(*BrowserModel)

	if bm.toasts.Count() < 3 {
		t.Errorf("expected multiple toasts after actions, got %d", bm.toasts.Count())
	}

	// 4. Test View rendering with floating toasts
	bm.width = 80
	bm.height = 24
	bm.loading = false
	viewOutput := bm.View()
	if len(viewOutput) == 0 {
		t.Errorf("expected non-empty view output with floating toasts")
	}
}

func TestDashboardProfileManagementAndClipboard(t *testing.T) {
	tempProfilesDir, err := os.MkdirTemp("", "runora-dash-prof-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempProfilesDir)

	cfg := config.DefaultConfig()
	cfg.Favorites = []string{}
	cfg.RecentLaunches = []string{}
	cfg.OnboardingCompleted = true
	cfg.Paths.Profiles = tempProfilesDir
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)
	bm.onboardingActive = false

	// Load default profiles
	profs, _ := profile.LoadAll(tempProfilesDir)
	bm.profiles = profs

	// Add mock model and load profiles
	bm.models = []*model.GGUFMetadata{
		{Name: "Llama-3-8B", FilePath: "models/llama-3-8b.gguf"},
	}
	bm.rebuildSidebar()
	bm.selected = 1 // select model

	// 1. Open Dashboard
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	if bm.screenMode != ScreenDashboard || bm.dashboard == nil {
		t.Fatalf("expected to be in ScreenDashboard")
	}

	// 2. Test Clipboard Copy [C]
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	bm = m.(*BrowserModel)

	if bm.dashboard.ToastMessage == "" {
		t.Errorf("expected dashboard ToastMessage to be set after pressing 'c'")
	}
	cmdPreview := bm.dashboard.GetLaunchCommand()
	if !strings.Contains(cmdPreview, "llama-server") || !strings.Contains(cmdPreview, "models/llama-3-8b.gguf") {
		t.Errorf("unexpected command generated: %q", cmdPreview)
	}

	// 3. Test Duplicating profile [N]
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	bm = m.(*BrowserModel)

	if bm.screenMode != ScreenProfileCreator || bm.profileCreatorModel == nil {
		t.Fatalf("expected to be in ScreenProfileCreator after pressing 'n'")
	}
	if bm.profileCreatorModel.mode != ModeDuplicate {
		t.Errorf("expected mode to be ModeDuplicate")
	}
	if !strings.Contains(bm.profileCreatorModel.nameInput.Value(), "(Copy)") {
		t.Errorf("expected duplicate name input to contain '(Copy)', got %q", bm.profileCreatorModel.nameInput.Value())
	}

	// Save duplicate profile (Press Enter)
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	if bm.screenMode != ScreenDashboard {
		t.Errorf("expected to return to ScreenDashboard after saving duplicate")
	}

	// Verify duplicate exists
	foundCopy := false
	for _, p := range bm.profiles {
		if strings.Contains(p.Name, "(Copy)") {
			foundCopy = true
			break
		}
	}
	if !foundCopy {
		t.Errorf("expected duplicated profile to be in loaded profiles")
	}

	// 4. Test Deleting default profile (must fail/show toast)
	// Switch to default profile "Fast"
	for i, p := range bm.dashboard.Profiles {
		if p.Name == "Fast" {
			bm.dashboard.ActiveIdx = i
			break
		}
	}
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	bm = m.(*BrowserModel)

	if !strings.Contains(bm.dashboard.ToastMessage, "Cannot delete") {
		t.Errorf("expected cannot delete warning toast when attempting to delete Fast profile, got %q", bm.dashboard.ToastMessage)
	}

	// 5. Test Deleting custom duplicate profile
	// Switch to the duplicate profile
	for i, p := range bm.dashboard.Profiles {
		if strings.Contains(p.Name, "(Copy)") {
			bm.dashboard.ActiveIdx = i
			break
		}
	}
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	bm = m.(*BrowserModel)

	if !strings.Contains(bm.dashboard.ToastMessage, "Deleted custom profile") {
		t.Errorf("expected deleted toast when deleting custom profile, got %q", bm.dashboard.ToastMessage)
	}
}

func TestBrowserLogStreamerTrigger(t *testing.T) {
	cfg := config.DefaultConfig()
	srv := runner.NewMultiRuntimeManager("")
	bm := NewBrowserModel(cfg, srv)

	bm.models = []*model.GGUFMetadata{
		{Name: "Qwen 2.5", FilePath: "models/qwen2.5.gguf"},
	}
	bm.filterModels()

	// Initial screen mode is ScreenBrowser
	if bm.screenMode != ScreenBrowser {
		t.Errorf("expected screenMode to be ScreenBrowser, got %d", bm.screenMode)
	}

	// Press [L] to open LogStreamerModel
	m, cmd := bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	bm = m.(*BrowserModel)

	if bm.screenMode != ScreenLogStreamer {
		t.Errorf("expected screenMode to transition to ScreenLogStreamer, got %d", bm.screenMode)
	}
	if bm.logStreamerModel == nil {
		t.Fatalf("expected logStreamerModel to be initialized")
	}
	if cmd == nil {
		t.Errorf("expected Init cmd batch from log streamer")
	}

	// Press [Esc] in LogStreamerModel to return to ScreenBrowser
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	bm = m.(*BrowserModel)

	if bm.screenMode != ScreenBrowser {
		t.Errorf("expected screenMode to return to ScreenBrowser on Esc, got %d", bm.screenMode)
	}
}








