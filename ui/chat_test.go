package ui

import (
	"context"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/BIJJUDAMA/runora/chat"
	"github.com/BIJJUDAMA/runora/model"
	"github.com/BIJJUDAMA/runora/runner"
)

type mockChatService struct {
	sessions []*chat.Session
}

func (m *mockChatService) Stream(ctx context.Context, session *chat.Session, userMsg string) (<-chan chat.TokenChunk, error) {
	ch := make(chan chat.TokenChunk, 2)
	ch <- chat.TokenChunk{Token: "Hello from mock!"}
	ch <- chat.TokenChunk{Done: true, GenTokens: 3}
	close(ch)
	return ch, nil
}

func (m *mockChatService) Compact(ctx context.Context, session *chat.Session) (*chat.CompactionCheckpoint, error) {
	return &chat.CompactionCheckpoint{
		Summary:      "Mock summary",
		TokensBefore: 100,
		TokensAfter:  20,
	}, nil
}

func (m *mockChatService) EstimateTokens(session *chat.Session) int {
	return 100
}

func (m *mockChatService) ContextSizeFor(port int) (int, error) {
	return 4096, nil
}

func (m *mockChatService) NewSession(name, modelPath string, port int, params chat.ChatParams) (*chat.Session, error) {
	s := &chat.Session{
		ID:        chat.GenerateID("sess"),
		Name:      name,
		ModelPath: modelPath,
		Port:      port,
		Params:    params,
	}
	m.sessions = append(m.sessions, s)
	return s, nil
}

func (m *mockChatService) ListSessions() ([]*chat.Session, error) {
	return m.sessions, nil
}

func (m *mockChatService) LoadSession(id string) (*chat.Session, error) {
	for _, s := range m.sessions {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, os.ErrNotExist
}

func (m *mockChatService) SaveSession(session *chat.Session) error {
	return nil
}

func (m *mockChatService) DeleteSession(id string) error {
	for i, s := range m.sessions {
		if s.ID == id {
			m.sessions = append(m.sessions[:i], m.sessions[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockChatService) RenameSession(id, name string) error {
	for _, s := range m.sessions {
		if s.ID == id {
			s.Name = name
			return nil
		}
	}
	return nil
}

func (m *mockChatService) ChatsDir() string {
	return ""
}

func TestChatModelBasicRendering(t *testing.T) {
	ApplyTheme("forest")
	svc := &mockChatService{
		sessions: []*chat.Session{
			{
				ID:   "sess-1",
				Name: "General Chat",
				Port: 50505,
				Messages: []chat.Message{
					{Role: "user", Content: "Explain concurrency"},
					{Role: "assistant", Content: "Concurrency is the composition of independently executing processes."},
				},
				Checkpoints: []chat.CompactionCheckpoint{
					{
						Summary:      "Initial greeting exchange.",
						CoveredRange: [2]int{0, 1},
						TokensBefore: 50,
						TokensAfter:  10,
					},
				},
			},
		},
	}

	cm := NewChatModel(svc, nil, []*model.GGUFMetadata{}, nil, nil, nil)
	cm.state = chatStateIdle
	rendered := cm.View(100, 30)

	if !strings.Contains(rendered, "Sessions") {
		t.Errorf("expected Sessions card in view")
	}
	if !strings.Contains(rendered, "General Chat") {
		t.Errorf("expected session name in view")
	}
	if !strings.Contains(rendered, "USER: Explain concurrency") {
		t.Errorf("expected user message in view")
	}
	if !strings.Contains(rendered, "ASSISTANT: Concurrency is the composition") {
		t.Errorf("expected assistant message in view")
	}
	if !strings.Contains(rendered, "COMPACTED: 2 turns") {
		t.Errorf("expected compaction separator in view")
	}
}

func TestChatModelParameterOverlay(t *testing.T) {
	ApplyTheme("dracula")
	svc := &mockChatService{
		sessions: []*chat.Session{
			{
				ID:     "sess-1",
				Name:   "General Chat",
				Params: chat.DefaultChatParams(),
			},
		},
	}

	cm := NewChatModel(svc, nil, nil, nil, nil, nil)
	cm.state = chatStateIdle

	// Press P to open parameters overlay
	cm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	if cm.state != chatStateParamOverlay {
		t.Fatalf("expected state chatStateParamOverlay, got %v", cm.state)
	}

	overlayView := cm.View(100, 30)
	if !strings.Contains(overlayView, "Parameters") {
		t.Errorf("expected Parameters card in view")
	}
	if !strings.Contains(overlayView, "Temperature") {
		t.Errorf("expected Temperature in overlay")
	}

	// Press Esc to close
	cm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cm.state != chatStateIdle {
		t.Fatalf("expected state chatStateIdle after Esc, got %v", cm.state)
	}
}

func TestChatModelCompactionWarningBanner(t *testing.T) {
	ApplyTheme("nord")
	svc := &mockChatService{
		sessions: []*chat.Session{
			{
				ID:   "sess-1",
				Name: "High Pressure Chat",
				Params: chat.ChatParams{
					ContextSize: 1000,
				},
			},
		},
	}

	cm := NewChatModel(svc, nil, nil, nil, nil, nil)
	cm.state = chatStateIdle
	cm.contextUsed = 900
	cm.contextTotal = 1000
	cm.warnCompact = true

	view := cm.View(100, 30)
	if !strings.Contains(view, "Context pressure at 90%") {
		t.Errorf("expected context pressure warning banner, got:\n%s", view)
	}
}

func TestZeroEmojisInChatView(t *testing.T) {
	ApplyTheme("forest")
	svc := &mockChatService{
		sessions: []*chat.Session{
			{
				ID:   "sess-1",
				Name: "Emoji-Free Chat",
				Port: 50505,
				Messages: []chat.Message{
					{Role: "user", Content: "Test message"},
					{Role: "assistant", Content: "Test response"},
				},
			},
		},
	}

	cm := NewChatModel(svc, nil, nil, nil, nil, nil)
	cm.state = chatStateIdle
	view := cm.View(120, 35)

	for _, r := range view {
		if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2700 && r <= 0x27BF) {
			t.Errorf("found emoji character %q (%U) in Chat view", string(r), r)
		}
	}
}

type mockChatRuntime struct {
	status    runner.ServerStatus
	modelPath string
	port      int
	instances []runner.InstanceInfo
}

func (m *mockChatRuntime) Start(modelPath string, opts runner.StartOptions) error { return nil }
func (m *mockChatRuntime) Stop() error                                           { return nil }
func (m *mockChatRuntime) StopInstance(port int) error                           { return nil }
func (m *mockChatRuntime) GetStatus() (runner.ServerStatus, string, int) {
	return m.status, m.modelPath, m.port
}
func (m *mockChatRuntime) GetAllInstances() []runner.InstanceInfo {
	return m.instances
}
func (m *mockChatRuntime) Capabilities() []runner.TaskType {
	return []runner.TaskType{runner.TaskTextGeneration}
}

func TestChatModelNoRunningServer(t *testing.T) {
	ApplyTheme("forest")
	svc := &mockChatService{
		sessions: []*chat.Session{
			{ID: "sess-1", Name: "Chat 1", Port: 50505},
		},
	}
	rt := &mockChatRuntime{
		status:    runner.StatusStopped,
		instances: []runner.InstanceInfo{},
	}

	cm := NewChatModel(svc, rt, nil, nil, nil, nil)
	if cm.state != chatStateNoServer {
		t.Fatalf("expected state chatStateNoServer when 0 instances running, got %v", cm.state)
	}

	view := cm.View(100, 30)
	if !strings.Contains(view, "No GGUF Server Running") {
		t.Errorf("expected 'No GGUF Server Running' in view, got:\n%s", view)
	}
	if !strings.Contains(view, "[2] Launch") {
		t.Errorf("expected reference to '[2] Launch' in view")
	}

	// Pressing Enter should emit ChatNavigateToLaunchMsg
	_, cmd := cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected command on Enter in chatStateNoServer")
	}
	msg := cmd()
	if _, ok := msg.(ChatNavigateToLaunchMsg); !ok {
		t.Errorf("expected ChatNavigateToLaunchMsg, got %T", msg)
	}
}

func TestChatModelNonGGUFFilteredOut(t *testing.T) {
	svc := &mockChatService{
		sessions: []*chat.Session{
			{ID: "sess-1", Name: "Chat 1", Port: 50505},
		},
	}
	// ONNX model running (not GGUF)
	rt := &mockChatRuntime{
		status:    runner.StatusRunning,
		modelPath: "models/bge-large.onnx",
		port:      50505,
		instances: []runner.InstanceInfo{
			{Port: 50505, ModelPath: "models/bge-large.onnx"},
		},
	}

	cm := NewChatModel(svc, rt, nil, nil, nil, nil)
	if cm.state != chatStateNoServer {
		t.Fatalf("expected state chatStateNoServer when only ONNX model is running, got %v", cm.state)
	}
}

func TestChatModelSingleGGUFServerAutoConnect(t *testing.T) {
	svc := &mockChatService{
		sessions: []*chat.Session{
			{ID: "sess-1", Name: "Chat 1", Port: 0},
		},
	}
	rt := &mockChatRuntime{
		status:    runner.StatusRunning,
		modelPath: "models/llama-3.1-8b.gguf",
		port:      50505,
		instances: []runner.InstanceInfo{
			{Port: 50505, ModelPath: "models/llama-3.1-8b.gguf"},
		},
	}

	cm := NewChatModel(svc, rt, nil, nil, nil, nil)
	if cm.state != chatStateIdle {
		t.Fatalf("expected state chatStateIdle when 1 GGUF server is running, got %v", cm.state)
	}
	if cm.activeSession.Port != 50505 || cm.activeSession.ModelPath != "models/llama-3.1-8b.gguf" {
		t.Errorf("expected session auto-bound to 50505 and models/llama-3.1-8b.gguf, got port %d, path %s",
			cm.activeSession.Port, cm.activeSession.ModelPath)
	}
}

func TestChatModelMultipleGGUFServersSelect(t *testing.T) {
	svc := &mockChatService{
		sessions: []*chat.Session{
			{ID: "sess-1", Name: "Chat 1", Port: 0},
		},
	}
	rt := &mockChatRuntime{
		status: runner.StatusRunning,
		instances: []runner.InstanceInfo{
			{Port: 50505, ModelPath: "models/llama-3.1-8b.gguf"},
			{Port: 50506, ModelPath: "models/qwen-2.5-7b.gguf"},
		},
	}

	cm := NewChatModel(svc, rt, nil, nil, nil, nil)
	if cm.state != chatStateInstanceSelect {
		t.Fatalf("expected state chatStateInstanceSelect when multiple GGUF servers running, got %v", cm.state)
	}

	view := cm.View(100, 30)
	if !strings.Contains(view, "Multiple GGUF Servers Running") {
		t.Errorf("expected 'Multiple GGUF Servers Running' in view, got:\n%s", view)
	}
	if !strings.Contains(view, "llama-3.1-8b.gguf") || !strings.Contains(view, "qwen-2.5-7b.gguf") {
		t.Errorf("expected both models listed in view")
	}

	// Press Down to select second model (qwen)
	cm.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cm.instanceCursor != 1 {
		t.Fatalf("expected instanceCursor 1, got %d", cm.instanceCursor)
	}

	// Press Enter to connect
	cm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cm.state != chatStateIdle {
		t.Fatalf("expected state chatStateIdle after selection, got %v", cm.state)
	}
	if cm.activeSession.Port != 50506 || cm.activeSession.ModelPath != "models/qwen-2.5-7b.gguf" {
		t.Errorf("expected session bound to qwen on port 50506, got port %d, path %s",
			cm.activeSession.Port, cm.activeSession.ModelPath)
	}

	// Press M to switch models
	cm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if cm.state != chatStateInstanceSelect {
		t.Fatalf("expected state chatStateInstanceSelect after pressing M, got %v", cm.state)
	}
}
