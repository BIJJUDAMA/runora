package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/BIJJUDAMA/runora/config"
	"github.com/BIJJUDAMA/runora/model"
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

	// Press Enter to advance to next step
	m, _ := bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)
	if bm.onboardingStep != StepModelSidebar {
		t.Errorf("expected onboarding to advance to StepModelSidebar, got %d", bm.onboardingStep)
	}

	// Press 'b' to go back
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	bm = m.(*BrowserModel)
	if bm.onboardingStep != StepWelcome {
		t.Errorf("expected onboarding to go back to StepWelcome, got %d", bm.onboardingStep)
	}

	// Advance to StepHFToken (Welcome -> Sidebar -> Details -> Launch -> Download/Lifecycle -> HFToken)
	for i := 0; i < 5; i++ {
		m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
		bm = m.(*BrowserModel)
	}
	if bm.onboardingStep != StepHFToken {
		t.Fatalf("expected step to be StepHFToken (5), got %v", bm.onboardingStep)
	}

	// Simulate typing a token
	bm.onboardingTokenInput.SetValue("test_token_123")
	// Press enter on StepHFToken to submit it
	m, _ = bm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	bm = m.(*BrowserModel)

	if bm.onboardingStep != StepFinished {
		t.Errorf("expected step to advance to StepFinished, got %v", bm.onboardingStep)
	}
	if bm.config.HFToken != "test_token_123" {
		t.Errorf("expected config HFToken to be test_token_123, got %q", bm.config.HFToken)
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







