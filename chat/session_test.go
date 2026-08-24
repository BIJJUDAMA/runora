package chat

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionMarshalRoundtrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runora-chat-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	params := ChatParams{
		Temperature:  0.8,
		TopP:         0.95,
		TopK:         50,
		ContextSize:  8192,
		SystemPrompt: "You are a helpful assistant.",
	}

	sess, err := NewSession(tmpDir, "Test Session", "models/test.gguf", 50505, params)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	sess.Messages = append(sess.Messages,
		Message{
			ID:        "msg-1",
			Role:      "user",
			Content:   "Hello model!",
			Timestamp: time.Now(),
			Tokens:    5,
		},
		Message{
			ID:        "msg-2",
			Role:      "assistant",
			Content:   "Hello! How can I assist you?",
			Timestamp: time.Now(),
			Tokens:    8,
		},
	)

	sess.Checkpoints = append(sess.Checkpoints, CompactionCheckpoint{
		ID:           "cp-1",
		Summary:      "User greeted assistant.",
		CoveredRange: [2]int{0, 1},
		TokensBefore: 13,
		TokensAfter:  4,
		CreatedAt:    time.Now(),
		OriginalMessages: []Message{
			{ID: "orig-1", Role: "user", Content: "Earlier msg"},
		},
	})

	if err := SaveSession(tmpDir, sess); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	loaded, err := LoadSession(tmpDir, sess.ID)
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}

	if loaded.Name != "Test Session" {
		t.Errorf("expected name %q, got %q", "Test Session", loaded.Name)
	}
	if loaded.ModelPath != "models/test.gguf" {
		t.Errorf("expected model path %q, got %q", "models/test.gguf", loaded.ModelPath)
	}
	if loaded.Port != 50505 {
		t.Errorf("expected port 50505, got %d", loaded.Port)
	}
	if loaded.Params.Temperature != 0.8 || loaded.Params.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("params mismatch: %+v", loaded.Params)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "Hello model!" {
		t.Errorf("msg 0 mismatch: %s", loaded.Messages[0].Content)
	}
	if len(loaded.Checkpoints) != 1 {
		t.Fatalf("expected 1 checkpoint, got %d", len(loaded.Checkpoints))
	}
	if loaded.Checkpoints[0].Summary != "User greeted assistant." {
		t.Errorf("checkpoint summary mismatch: %s", loaded.Checkpoints[0].Summary)
	}
	if len(loaded.Checkpoints[0].OriginalMessages) != 1 {
		t.Errorf("original messages count mismatch: %d", len(loaded.Checkpoints[0].OriginalMessages))
	}
}

func TestListSessionsOrdering(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runora-chat-list-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	s1, _ := NewSession(tmpDir, "First", "", 50505, DefaultChatParams())
	time.Sleep(10 * time.Millisecond)
	s2, _ := NewSession(tmpDir, "Second", "", 50505, DefaultChatParams())
	time.Sleep(10 * time.Millisecond)
	s3, _ := NewSession(tmpDir, "Third", "", 50505, DefaultChatParams())

	// Touch s1 to make it most recently updated
	time.Sleep(10 * time.Millisecond)
	_ = SaveSession(tmpDir, s1)

	list, err := ListSessions(tmpDir)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(list) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(list))
	}
	if list[0].ID != s1.ID {
		t.Errorf("expected newest updated session to be %s, got %s (%s)", s1.ID, list[0].ID, list[0].Name)
	}
	if list[1].ID != s3.ID {
		t.Errorf("expected 2nd session to be %s, got %s", s3.ID, list[1].ID)
	}
	if list[2].ID != s2.ID {
		t.Errorf("expected 3rd session to be %s, got %s", s2.ID, list[2].ID)
	}
}

func TestDeleteAndRenameSession(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runora-chat-del-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := NewSession(tmpDir, "Old Name", "", 50505, DefaultChatParams())
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := RenameSession(tmpDir, s.ID, "Renamed Session"); err != nil {
		t.Fatalf("failed to rename session: %v", err)
	}

	loaded, err := LoadSession(tmpDir, s.ID)
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if loaded.Name != "Renamed Session" {
		t.Errorf("expected name 'Renamed Session', got %q", loaded.Name)
	}

	if err := DeleteSession(tmpDir, s.ID); err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}

	target := filepath.Join(tmpDir, s.ID+".json")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("expected file %s to be deleted", target)
	}
}
