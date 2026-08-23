package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/BIJJUDAMA/runora/runner"
)

type mockLogStreamRuntime struct {
	instances []runner.InstanceInfo
}

func (m *mockLogStreamRuntime) Start(modelPath string, opts runner.StartOptions) error { return nil }
func (m *mockLogStreamRuntime) Stop() error                                           { return nil }
func (m *mockLogStreamRuntime) StopInstance(port int) error                           { return nil }
func (m *mockLogStreamRuntime) GetStatus() (runner.ServerStatus, string, int)         { return runner.StatusStopped, "", 0 }
func (m *mockLogStreamRuntime) GetAllInstances() []runner.InstanceInfo                { return m.instances }
func (m *mockLogStreamRuntime) Capabilities() []runner.TaskType                       { return nil }

func TestLogStreamerModel_LifecycleAndStreaming(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "runora-log-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "server-8080.log")
	initialContent := "line 1: server initializing\nline 2: loading model weights\nline 3: HTTP server listening on 127.0.0.1:8080\n"
	if err := os.WriteFile(logFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to write log file: %v", err)
	}

	mockRuntime := &mockLogStreamRuntime{
		instances: []runner.InstanceInfo{
			{
				PID:       1111,
				Port:      8080,
				ModelPath: "models/qwen.gguf",
				LogFile:   logFile,
			},
		},
	}

	model := NewLogStreamerModel(mockRuntime, ScreenBrowser, 8080)
	model.SetDimensions(100, 30)

	if len(model.allLines) != 3 {
		t.Fatalf("expected 3 initial log lines, got %d", len(model.allLines))
	}
	if !model.autoScroll {
		t.Errorf("expected autoScroll to be true initially")
	}
	if model.paused {
		t.Errorf("expected paused to be false initially")
	}

	// 1. Append more lines to the log file and simulate streaming read
	appendedContent := "line 4: [error] CUDA out of memory fallback to CPU\nline 5: warning: high context utilization\nline 6: all slots are idle\n"
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open log file for append: %v", err)
	}
	_, _ = f.WriteString(appendedContent)
	_ = f.Close()

	readCmd := model.readLogFileCmd()
	if readCmd == nil {
		t.Fatalf("readLogFileCmd returned nil")
	}
	msg := readCmd()
	dataMsg, ok := msg.(LogStreamDataMsg)
	if !ok {
		t.Fatalf("expected LogStreamDataMsg, got %T", msg)
	}
	if len(dataMsg.Lines) != 3 {
		t.Errorf("expected 3 new lines in LogStreamDataMsg, got %d", len(dataMsg.Lines))
	}

	// Update model with streamed data
	model, _ = model.Update(dataMsg)
	if len(model.allLines) != 6 {
		t.Errorf("expected 6 total lines after streaming, got %d", len(model.allLines))
	}

	// 2. Test Pause / Resume ([Space])
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !model.paused {
		t.Errorf("expected model to be paused after spacebar")
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if model.paused {
		t.Errorf("expected model to be resumed after second spacebar")
	}

	// 3. Test Search / Filter ([/])
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !model.searchActive {
		t.Errorf("expected searchActive to be true after /")
	}

	// Type query "error"
	for _, r := range "error" {
		model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if model.searchActive {
		t.Errorf("expected searchActive to be false after Enter")
	}
	if model.searchQuery != "error" {
		t.Errorf("expected searchQuery 'error', got %q", model.searchQuery)
	}

	filtered := model.getFilteredLines()
	if len(filtered) != 1 {
		t.Errorf("expected 1 line matching 'error', got %d", len(filtered))
	}

	// 4. Test View rendering and error styling
	view := model.View(100, 30)
	if !strings.Contains(view, "LIVE PROCESS LOG STREAMER") {
		t.Errorf("expected title in view")
	}
	if !strings.Contains(view, "error") {
		t.Errorf("expected error in view")
	}

	// 5. Test Clear Filter ([C])
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if model.searchQuery != "" {
		t.Errorf("expected searchQuery to be cleared after [C], got %q", model.searchQuery)
	}

	// 6. Test Esc closing modal
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !model.Closed() {
		t.Errorf("expected model to be closed after Esc")
	}
}

func TestLogStreamerModel_MultiInstanceSwitching(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "runora-log-multi-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	log1 := filepath.Join(tempDir, "server-8080.log")
	log2 := filepath.Join(tempDir, "server-8081.log")
	_ = os.WriteFile(log1, []byte("log from server 8080\n"), 0644)
	_ = os.WriteFile(log2, []byte("log from server 8081\n"), 0644)

	mockRuntime := &mockLogStreamRuntime{
		instances: []runner.InstanceInfo{
			{PID: 1001, Port: 8080, ModelPath: "models/qwen.gguf", LogFile: log1},
			{PID: 1002, Port: 8081, ModelPath: "models/llama.gguf", LogFile: log2},
		},
	}

	model := NewLogStreamerModel(mockRuntime, ScreenServerMonitor, 8080)
	if model.currentPort != 8080 {
		t.Errorf("expected initial port 8080, got %d", model.currentPort)
	}

	// Switch instance with Tab
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if model.currentPort != 8081 {
		t.Errorf("expected switched port 8081 after Tab, got %d", model.currentPort)
	}
	if len(model.allLines) != 1 || !strings.Contains(model.allLines[0], "8081") {
		t.Errorf("expected lines from 8081, got %v", model.allLines)
	}
}
