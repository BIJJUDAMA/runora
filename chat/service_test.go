package chat

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStreamTokensDelivered(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runora-stream-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatalf("expected flusher")
			}

			tokens := []string{"Hello", " ", "world", "!", " How", " are", " you?"}
			for _, tok := range tokens {
				data := fmt.Sprintf(`data: {"choices":[{"delta":{"content":%q}}],"usage":{"prompt_tokens":10}}`+"\n\n", tok)
				_, _ = w.Write([]byte(data))
				flusher.Flush()
				time.Sleep(10 * time.Millisecond)
			}

			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			flusher.Flush()
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	u, _ := url.Parse(mockServer.URL)
	port, _ := strconv.Atoi(u.Port())

	svc := NewService(tmpDir)
	sess, err := svc.NewSession("Stream Test", "model.gguf", port, DefaultChatParams())
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	stream, err := svc.Stream(context.Background(), sess, "Greetings!")
	if err != nil {
		t.Fatalf("stream call failed: %v", err)
	}

	var received strings.Builder
	doneReceived := false
	var finalChunk TokenChunk

	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("unexpected chunk error: %v", chunk.Err)
		}
		if chunk.Token != "" {
			received.WriteString(chunk.Token)
		}
		if chunk.Done {
			doneReceived = true
			finalChunk = chunk
		}
	}

	if !doneReceived {
		t.Errorf("expected done chunk to be received")
	}
	if received.String() != "Hello world! How are you?" {
		t.Errorf("expected %q, got %q", "Hello world! How are you?", received.String())
	}
	if finalChunk.GenTokens != 7 {
		t.Errorf("expected 7 gen tokens, got %d", finalChunk.GenTokens)
	}
	if finalChunk.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", finalChunk.PromptTokens)
	}

	// Verify that user message and assistant message are both persisted in session
	loaded, err := svc.LoadSession(sess.ID)
	if err != nil {
		t.Fatalf("failed to reload session: %v", err)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages in session, got %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Role != "user" || loaded.Messages[0].Content != "Greetings!" {
		t.Errorf("user turn mismatch: %+v", loaded.Messages[0])
	}
	if loaded.Messages[1].Role != "assistant" || loaded.Messages[1].Content != "Hello world! How are you?" {
		t.Errorf("assistant turn mismatch: %+v", loaded.Messages[1])
	}
	if loaded.Messages[1].Partial {
		t.Errorf("completed turn should not be marked partial")
	}
}

func TestStreamContextCancellation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "runora-cancel-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)

			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"First"}}]}` + "\n\n"))
			flusher.Flush()
			time.Sleep(100 * time.Millisecond)

			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"Second"}}]}` + "\n\n"))
			flusher.Flush()
			time.Sleep(500 * time.Millisecond)
			return
		}
	}))
	defer mockServer.Close()

	u, _ := url.Parse(mockServer.URL)
	port, _ := strconv.Atoi(u.Port())

	svc := NewService(tmpDir)
	sess, _ := svc.NewSession("Cancel Test", "model.gguf", port, DefaultChatParams())

	ctx, cancel := context.WithCancel(context.Background())

	stream, err := svc.Stream(ctx, sess, "Hello")
	if err != nil {
		t.Fatalf("stream failed: %v", err)
	}

	for chunk := range stream {
		if chunk.Token == "First" {
			cancel() // Cancel immediately after first token
		}
	}

	// Session should have saved the partial assistant response
	loaded, err := svc.LoadSession(sess.ID)
	if err != nil {
		t.Fatalf("failed to load session: %v", err)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("expected 2 messages (user + partial assistant), got %d", len(loaded.Messages))
	}
	if !loaded.Messages[1].Partial {
		t.Errorf("expected assistant message to be marked partial")
	}
	if !strings.Contains(loaded.Messages[1].Content, "First") {
		t.Errorf("expected partial content to contain 'First', got %q", loaded.Messages[1].Content)
	}
}
