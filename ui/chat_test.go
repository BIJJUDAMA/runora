package ui

import (
	"context"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/BIJJUDAMA/runora/chat"
	"github.com/BIJJUDAMA/runora/model"
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
	view := cm.View(120, 35)

	for _, r := range view {
		if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2700 && r <= 0x27BF) {
			t.Errorf("found emoji character %q (%U) in Chat view", string(r), r)
		}
	}
}
