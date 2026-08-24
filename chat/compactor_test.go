package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestBuildContextThreeLayers(t *testing.T) {
	session := &Session{
		Params: ChatParams{
			SystemPrompt: "System instruction here",
		},
		Checkpoints: []CompactionCheckpoint{
			{
				Summary: "User asked about Go routines.",
			},
		},
		Messages: []Message{
			{Role: "user", Content: "How do channels work?"},
			{Role: "assistant", Content: "Channels allow goroutines to communicate."},
		},
	}

	msgs := BuildContext(session)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 context messages, got %d", len(msgs))
	}

	// Layer 1
	if msgs[0].Role != "system" || msgs[0].Content != "System instruction here" {
		t.Errorf("layer 1 mismatch: %+v", msgs[0])
	}

	// Layer 2
	if msgs[1].Role != "assistant" || msgs[1].Content != "[Previous Conversation Summary: User asked about Go routines.]" {
		t.Errorf("layer 2 mismatch: %+v", msgs[1])
	}

	// Layer 3
	if msgs[2].Role != "user" || msgs[2].Content != "How do channels work?" {
		t.Errorf("layer 3 message 1 mismatch: %+v", msgs[2])
	}
}

func TestEstimateTokens(t *testing.T) {
	session := &Session{
		Params: ChatParams{
			SystemPrompt: "12345678", // ~2 tokens
		},
		Checkpoints: []CompactionCheckpoint{
			{
				TokensAfter: 10,
			},
		},
		Messages: []Message{
			{Tokens: 15, Content: "some msg"},
			{Tokens: 0, Content: "12345678"}, // ~2 tokens fallback
		},
	}

	count := EstimateTokens(session)
	expected := 2 + 10 + 15 + 2
	if count != expected {
		t.Errorf("expected %d tokens, got %d", expected, count)
	}
}

func TestContextSizeFor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/props" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"default_generation_settings": map[string]interface{}{
					"n_ctx": 8192,
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())

	ctxSize, err := ContextSizeFor(port)
	if err != nil {
		t.Fatalf("failed to query context size: %v", err)
	}
	if ctxSize != 8192 {
		t.Errorf("expected context size 8192, got %d", ctxSize)
	}
}

func TestCompactWithMockServer(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runora-compact-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/props" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"default_generation_settings": map[string]interface{}{
					"n_ctx": 500,
				},
			})
			return
		}
		if r.URL.Path == "/v1/chat/completions" {
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"message": map[string]interface{}{
							"content": "Compacted summary of early discussion.",
						},
					},
				},
				"usage": map[string]interface{}{
					"completion_tokens": 12,
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	u, _ := url.Parse(mockServer.URL)
	port, _ := strconv.Atoi(u.Port())

	session, err := NewSession(tmpDir, "Compaction Test", "model.gguf", port, ChatParams{
		Temperature: 0.7,
		TopP:        0.9,
		TopK:        40,
		ContextSize: 500,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Add 6 turns
	for i := 0; i < 6; i++ {
		session.Messages = append(session.Messages, Message{
			ID:        GenerateID("m"),
			Role:      "user",
			Content:   "This is a long message turn that occupies memory in the context window.",
			Tokens:    50,
			Timestamp: time.Now(),
		})
	}

	cp, err := Compact(context.Background(), tmpDir, session, port)
	if err != nil {
		t.Fatalf("compaction failed: %v", err)
	}
	if cp == nil {
		t.Fatalf("expected non-nil compaction checkpoint")
	}

	if cp.Summary != "Compacted summary of early discussion." {
		t.Errorf("checkpoint summary mismatch: %s", cp.Summary)
	}
	if len(cp.OriginalMessages) == 0 {
		t.Errorf("expected original messages in checkpoint")
	}
	if len(session.Checkpoints) != 1 {
		t.Errorf("expected 1 checkpoint on session, got %d", len(session.Checkpoints))
	}
	if len(session.Messages) >= 6 {
		t.Errorf("expected fewer messages in session after compaction, got %d", len(session.Messages))
	}

	// Verify persistence
	loaded, err := LoadSession(tmpDir, session.ID)
	if err != nil {
		t.Fatalf("failed to load session after compact: %v", err)
	}
	if len(loaded.Checkpoints) != 1 {
		t.Errorf("expected persisted checkpoint count 1, got %d", len(loaded.Checkpoints))
	}
}
