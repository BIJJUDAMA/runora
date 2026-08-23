package model

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadQueueFlow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llama-manager-download-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	q := NewDownloadQueue(tempDir, "hf_mock_token")

	// Initially queue should be empty
	if len(q.GetTasks()) != 0 {
		t.Errorf("expected empty task list initially")
	}

	// Add a task
	task := q.AddTask("org/repo", "model.gguf", 1000, "http://example.com/model.gguf")

	// Verify task details
	tasks := q.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task in queue, got %d", len(tasks))
	}

	t0 := tasks[0]
	if t0.ModelID != "org/repo" {
		t.Errorf("expected ModelID org/repo, got %s", t0.ModelID)
	}
	if t0.FileName != "model.gguf" {
		t.Errorf("expected FileName model.gguf, got %s", t0.FileName)
	}
	if t0.TotalSize != 1000 {
		t.Errorf("expected TotalSize 1000, got %d", t0.TotalSize)
	}
	expectedPath := filepath.Join(tempDir, "org_repo", "model.gguf")
	if t0.DestPath != expectedPath {
		t.Errorf("expected DestPath %q, got %q", expectedPath, t0.DestPath)
	}

	// Test notify channel contains task update
	select {
	case notifiedTask := <-q.GetChan():
		if notifiedTask != task {
			t.Errorf("expected to be notified with added task")
		}
	default:
		t.Errorf("expected to receive task notification on channel")
	}

	// Test Pause Task
	q.PauseTask(task)
	task.mu.Lock()
	pausedStatus := task.Status
	task.mu.Unlock()
	if pausedStatus != StatusPaused {
		t.Errorf("expected task status to be StatusPaused, got %d", pausedStatus)
	}

	// Test Resume Task
	q.ResumeTask(task)
	task.mu.Lock()
	resumedStatus := task.Status
	task.mu.Unlock()
	if resumedStatus != StatusQueued && resumedStatus != StatusDownloading {
		t.Errorf("expected task status to be StatusQueued or StatusDownloading, got %d", resumedStatus)
	}

	// Test Cancel Task cleans up both DestPath and .part file
	if err := os.MkdirAll(filepath.Dir(task.DestPath), 0755); err != nil {
		t.Fatalf("failed to create dest dir: %v", err)
	}
	partPath := task.DestPath + ".part"
	if err := os.WriteFile(partPath, []byte("partial download data"), 0644); err != nil {
		t.Fatalf("failed to write dummy part file: %v", err)
	}
	if err := os.WriteFile(task.DestPath, []byte("dest file data"), 0644); err != nil {
		t.Fatalf("failed to write dummy dest file: %v", err)
	}

	q.CancelTask(task)
	if len(q.GetTasks()) != 0 {
		t.Errorf("expected task to be removed from queue after Cancel")
	}
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Errorf("expected .part file to be removed after CancelTask")
	}
	if _, err := os.Stat(task.DestPath); !os.IsNotExist(err) {
		t.Errorf("expected DestPath to be removed after CancelTask")
	}
}

func TestClearAndRemoveTasks(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llama-manager-clear-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	q := NewDownloadQueue(tempDir, "hf_mock_token")

	// Add several tasks
	t1 := q.AddTask("org/repo1", "model1.gguf", 1000, "http://example.com/model1.gguf")
	t2 := q.AddTask("org/repo2", "model2.gguf", 2000, "http://example.com/model2.gguf")
	t3 := q.AddTask("org/repo3", "model3.gguf", 3000, "http://example.com/model3.gguf")

	// Set their statuses manually for testing
	t1.mu.Lock()
	t1.Status = StatusCompleted
	t1.mu.Unlock()

	t2.mu.Lock()
	t2.Status = StatusDownloading
	t2.mu.Unlock()

	t3.mu.Lock()
	t3.Status = StatusFailed
	t3.mu.Unlock()

	// Clear finished should remove t1 and t3, but keep t2
	q.ClearFinishedTasks()

	tasks := q.GetTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task remaining, got %d", len(tasks))
	}
	if tasks[0] != t2 {
		t.Errorf("expected remaining task to be t2")
	}

	// Remove task t2 using RemoveTask
	q.RemoveTask(t2)
	if len(q.GetTasks()) != 0 {
		t.Errorf("expected 0 tasks remaining after RemoveTask")
	}
}

func TestDownloaderHTTPRangeResumptionEndToEnd(t *testing.T) {
	fullPayload := []byte("The quick brown fox jumps over the lazy dog. 1234567890! Local GGUF streaming works.")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fullPayload)))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(fullPayload)
			return
		}

		// Parse "bytes=X-"
		var start int
		if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-", &start); err != nil || start >= len(fullPayload) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(fullPayload)-1, len(fullPayload)))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(fullPayload)-start))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(fullPayload[start:])
	}))
	defer server.Close()

	tempDir, err := os.MkdirTemp("", "llama-range-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	q := NewDownloadQueue(tempDir, "")

	// 1. Pre-seed partial .part file with first 10 bytes
	destDir := filepath.Join(tempDir, "test_repo")
	_ = os.MkdirAll(destDir, 0755)
	destPath := filepath.Join(destDir, "sample.gguf")
	partPath := destPath + ".part"
	_ = os.WriteFile(partPath, fullPayload[:10], 0644)

	task := q.AddTask("test/repo", "sample.gguf", int64(len(fullPayload)), server.URL+"/sample.gguf")

	// Wait for task to complete (or fail)
	done := false
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		task.mu.Lock()
		st := task.Status
		task.mu.Unlock()
		if st == StatusCompleted || st == StatusFailed {
			done = true
			break
		}
	}

	if !done {
		t.Fatalf("download task did not complete in time")
	}

	task.mu.Lock()
	finalStatus := task.Status
	task.mu.Unlock()

	if finalStatus != StatusCompleted {
		t.Fatalf("expected task StatusCompleted, got %d (err: %v)", finalStatus, task.Error)
	}

	// Verify complete file on disk
	downloadedBytes, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(downloadedBytes) != string(fullPayload) {
		t.Errorf("downloaded content mismatch:\nGot:  %q\nWant: %q", string(downloadedBytes), string(fullPayload))
	}
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Errorf("expected .part file to be removed upon download completion")
	}
}

