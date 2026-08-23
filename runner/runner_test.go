package runner

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRunnerBinaryNotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llama-runner-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	runner := NewLlamaCppRuntime(tempDir)

	// Try to start a server with a non-existent llama.cpp dir
	err = runner.Start("some-model.gguf", StartOptions{
		LlamaCppDir: filepath.Join(tempDir, "missing-dir"),
		ContextSize: 2048,
		Threads:     4,
		GPULayers:   999,
		BatchSize:   512,
		Host:        "127.0.0.1",
		Port:        50505,
	})
	if err == nil {
		t.Errorf("expected error starting server with missing directory, got nil")
	}
}

func TestMultiInstanceTracking(t *testing.T) {
	runner := NewLlamaCppRuntime("")

	// Initially, it should have no active instances
	if len(runner.GetAllInstances()) != 0 {
		t.Errorf("expected 0 running instances initially")
	}

	status, model, port := runner.GetStatus()
	if status != StatusStopped || model != "" || port != 50505 {
		t.Errorf("incorrect stopped status values: %d, %q, %d", status, model, port)
	}
}

func TestQueryServerSlotsAndTokens(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/slots", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id": 0, "n_ctx": 8192, "n_past": 5324, "is_processing": true, "state": 1},
			{"id": 1, "n_ctx": 8192, "n_past": 0, "is_processing": false, "state": 0}
		]`))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(`
# HELP llama_tokens_predicted_total Total number of predicted tokens.
llama_tokens_predicted_total 420
# HELP llama_tokens_evaluated_total Total number of evaluated tokens.
llama_tokens_evaluated_total 1000
`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	var port int
	_, _ = fmt.Sscanf(server.URL, "http://127.0.0.1:%d", &port)

	slots, err := QueryServerSlots(port)
	if err != nil {
		t.Fatalf("QueryServerSlots failed: %v", err)
	}
	if slots.TotalNCtx != 16384 {
		t.Errorf("expected total context 16384, got %d", slots.TotalNCtx)
	}
	if slots.TotalNPast != 5324 {
		t.Errorf("expected total past tokens 5324, got %d", slots.TotalNPast)
	}
	if slots.ActiveSlots != 1 {
		t.Errorf("expected 1 active slot, got %d", slots.ActiveSlots)
	}

	tokens, err := QueryServerTokens(port)
	if err != nil {
		t.Fatalf("QueryServerTokens failed: %v", err)
	}
	if tokens != 1420 {
		t.Errorf("expected 1420 total tokens, got %d", tokens)
	}
}
