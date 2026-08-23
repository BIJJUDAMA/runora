package model

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type HFModelResult struct {
	ModelID   string `json:"modelId"`
	Downloads int    `json:"downloads"`
	Likes     int    `json:"likes"`
}

type HFSibling struct {
	Rpath string `json:"rfilename"`
	Size  int64  `json:"size"`
}

type HFModelDetail struct {
	ModelID  string      `json:"modelId"`
	Siblings []HFSibling `json:"siblings"`
}

type TaskStatus int

const (
	StatusQueued TaskStatus = iota
	StatusDownloading
	StatusPaused
	StatusCompleted
	StatusCanceled
	StatusFailed
)

type DownloadTask struct {
	URL        string
	DestPath   string
	ModelID    string
	FileName   string
	Downloaded int64
	TotalSize  int64
	Status     TaskStatus
	Error      error
	SpeedKBps  float64
	cancelFunc context.CancelFunc
	mu         sync.Mutex
	token      string
}

type DownloadQueue struct {
	mu           sync.Mutex
	tasks        []*DownloadTask
	activeTask   *DownloadTask
	ch           chan *DownloadTask
	configToken  string
	modelsDir    string
}

func NewDownloadQueue(modelsDir string, configToken string) *DownloadQueue {
	return &DownloadQueue{
		tasks:       []*DownloadTask{},
		ch:          make(chan *DownloadTask, 10),
		modelsDir:   modelsDir,
		configToken: configToken,
	}
}

func (q *DownloadQueue) AddTask(modelID string, fileName string, size int64, downloadURL string) *DownloadTask {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Safe name for sub-directory
	safeDirName := strings.ReplaceAll(modelID, "/", "_")
	invalidChars := []string{"\\", ":", "*", "?", "\"", "<", ">", "|"}
	for _, char := range invalidChars {
		safeDirName = strings.ReplaceAll(safeDirName, char, "_")
	}
	destDir := filepath.Join(q.modelsDir, safeDirName)

	task := &DownloadTask{
		URL:       downloadURL,
		DestPath:  filepath.Join(destDir, fileName),
		ModelID:   modelID,
		FileName:  fileName,
		TotalSize: size,
		Status:    StatusQueued,
		token:     q.configToken,
	}

	q.tasks = append(q.tasks, task)
	q.notify(task)

	q.processNext()
	return task
}

// AddFailedTask adds a pre-failed task directly to the queue.
func (q *DownloadQueue) AddFailedTask(modelID string, fileName string, err error) *DownloadTask {
	q.mu.Lock()
	defer q.mu.Unlock()

	task := &DownloadTask{
		URL:       "",
		DestPath:  "",
		ModelID:   modelID,
		FileName:  fileName,
		TotalSize: 0,
		Status:    StatusFailed,
		Error:     err,
		token:     q.configToken,
	}

	q.tasks = append(q.tasks, task)
	q.notify(task)
	return task
}

// UpdateToken updates the configuration token used by the queue.
func (q *DownloadQueue) UpdateToken(token string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.configToken = token
}

func (q *DownloadQueue) notify(task *DownloadTask) {
	select {
	case q.ch <- task:
	default:
	}
}

func (q *DownloadQueue) GetTasks() []*DownloadTask {
	q.mu.Lock()
	defer q.mu.Unlock()
	list := make([]*DownloadTask, len(q.tasks))
	copy(list, q.tasks)
	return list
}

func (q *DownloadQueue) GetChan() chan *DownloadTask {
	return q.ch
}

func (q *DownloadQueue) PauseTask(task *DownloadTask) {
	task.mu.Lock()
	if task.Status == StatusDownloading || task.Status == StatusQueued {
		task.Status = StatusPaused
		if task.cancelFunc != nil {
			task.cancelFunc()
		}
	}
	task.mu.Unlock()

	q.mu.Lock()
	defer q.mu.Unlock()
	q.notify(task)
	q.processNext()
}

func (q *DownloadQueue) ResumeTask(task *DownloadTask) {
	task.mu.Lock()
	if task.Status == StatusPaused || task.Status == StatusFailed || task.Status == StatusQueued {
		task.Status = StatusQueued
	}
	task.mu.Unlock()

	q.mu.Lock()
	defer q.mu.Unlock()
	q.notify(task)
	q.processNext()
}

func (q *DownloadQueue) CancelTask(task *DownloadTask) {
	task.mu.Lock()
	if task.cancelFunc != nil {
		task.cancelFunc()
	}
	task.Status = StatusCanceled
	task.mu.Unlock()

	q.mu.Lock()
	defer q.mu.Unlock()

	// Remove from list
	for i, t := range q.tasks {
		if t == task {
			q.tasks = append(q.tasks[:i], q.tasks[i+1:]...)
			break
		}
	}

	// Remove file and part file if partially downloaded
	_ = os.Remove(task.DestPath)
	_ = os.Remove(task.DestPath + ".part")

	q.notify(task)
	q.processNext()
}

// RemoveTask removes a task from the queue list without deleting its model file.
func (q *DownloadQueue) RemoveTask(task *DownloadTask) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, t := range q.tasks {
		if t == task {
			q.tasks = append(q.tasks[:i], q.tasks[i+1:]...)
			break
		}
	}

	if q.activeTask == task {
		q.activeTask = nil
	}

	q.notify(nil)
}

// ClearFinishedTasks removes all tasks that are Completed, Failed, or Canceled.
func (q *DownloadQueue) ClearFinishedTasks() {
	q.mu.Lock()
	defer q.mu.Unlock()

	var activeTaskRemaining bool
	newTasks := []*DownloadTask{}
	for _, t := range q.tasks {
		t.mu.Lock()
		status := t.Status
		t.mu.Unlock()

		if status == StatusQueued || status == StatusDownloading || status == StatusPaused {
			newTasks = append(newTasks, t)
			if t == q.activeTask {
				activeTaskRemaining = true
			}
		}
	}
	q.tasks = newTasks
	if !activeTaskRemaining {
		q.activeTask = nil
	}

	q.notify(nil)
}


func (q *DownloadQueue) processNext() {
	if q.activeTask != nil {
		q.activeTask.mu.Lock()
		activeStatus := q.activeTask.Status
		q.activeTask.mu.Unlock()
		if activeStatus == StatusDownloading {
			return // Already busy
		}
	}

	// Find next queued task
	for _, task := range q.tasks {
		task.mu.Lock()
		status := task.Status
		task.mu.Unlock()

		if status == StatusQueued {
			q.activeTask = task
			go q.runDownload(task)
			return
		}
	}
}

func (q *DownloadQueue) runDownload(task *DownloadTask) {
	task.mu.Lock()
	task.Status = StatusDownloading
	ctx, cancel := context.WithCancel(context.Background())
	task.cancelFunc = cancel
	task.mu.Unlock()

	q.notify(task)

	err := q.downloadLoop(ctx, task)

	task.mu.Lock()
	if task.Status == StatusDownloading {
		if err != nil {
			task.Status = StatusFailed
			task.Error = err
		} else {
			task.Status = StatusCompleted
		}
	}
	task.cancelFunc = nil
	task.mu.Unlock()

	q.notify(task)

	q.mu.Lock()
	q.processNext()
	q.mu.Unlock()
}

func (q *DownloadQueue) downloadLoop(ctx context.Context, task *DownloadTask) error {
	destDir := filepath.Dir(task.DestPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	partPath := task.DestPath + ".part"

	// Get local size from part file if resuming
	var startBytes int64 = 0
	info, err := os.Stat(partPath)
	if err == nil {
		startBytes = info.Size()
	}

	// Don't resume if file is already full size
	if task.TotalSize > 0 && startBytes >= task.TotalSize {
		task.mu.Lock()
		task.Downloaded = task.TotalSize
		task.mu.Unlock()
		_ = ReplaceFile(partPath, task.DestPath)
		return nil
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   0,
	}
	req, err := http.NewRequestWithContext(ctx, "GET", task.URL, nil)
	if err != nil {
		return err
	}

	// Inject secure token
	token := q.configToken
	if token == "" {
		token = os.Getenv("HF_TOKEN")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	isResuming := false
	if startBytes > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startBytes))
		isResuming = true
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("server returned HTTP error %d: %s", resp.StatusCode, resp.Status)
	}

	var file *os.File
	if isResuming && resp.StatusCode == http.StatusPartialContent {
		file, err = os.OpenFile(partPath, os.O_WRONLY|os.O_APPEND, 0644)
		task.mu.Lock()
		task.Downloaded = startBytes
		task.mu.Unlock()
	} else {
		// Truncate/create new part file
		file, err = os.OpenFile(partPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		task.mu.Lock()
		task.Downloaded = 0
		task.mu.Unlock()
	}

	if err != nil {
		return err
	}
	defer file.Close()

	if task.TotalSize <= 0 && resp.ContentLength > 0 {
		task.mu.Lock()
		if isResuming && resp.StatusCode == http.StatusPartialContent {
			task.TotalSize = startBytes + resp.ContentLength
		} else {
			task.TotalSize = resp.ContentLength
		}
		task.mu.Unlock()
	}

	buf := make([]byte, 64*1024)
	startTime := time.Now()
	var bytesSinceLastReport int64 = 0
	lastReportTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			_, werr := file.Write(buf[:n])
			if werr != nil {
				return werr
			}

			task.mu.Lock()
			task.Downloaded += int64(n)
			task.mu.Unlock()

			bytesSinceLastReport += int64(n)

			// Calculate speed every 500ms
			now := time.Now()
			elapsed := now.Sub(lastReportTime)
			if elapsed >= 500*time.Millisecond {
				speed := float64(bytesSinceLastReport) / 1024.0 / elapsed.Seconds()
				task.mu.Lock()
				task.SpeedKBps = speed
				task.mu.Unlock()

				bytesSinceLastReport = 0
				lastReportTime = now
				q.notify(task)
			}
		}

		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return rerr
		}
	}

	// Final speed calculate
	totalElapsed := time.Since(startTime)
	if totalElapsed > 0 {
		task.mu.Lock()
		task.SpeedKBps = 0
		task.mu.Unlock()
		q.notify(task)
	}

	// Close file explicitly before atomic rename to destination
	_ = file.Close()
	return ReplaceFile(partPath, task.DestPath)
}

// SearchHFModels queries Hugging Face API for repositories matching query.
func SearchHFModels(query string, token string) ([]HFModelResult, error) {
	if query == "" {
		return []HFModelResult{}, nil
	}

	escapedQuery := url.QueryEscape(query)
	apiURL := fmt.Sprintf("https://huggingface.co/api/models?search=%s&limit=20", escapedQuery)

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	if token == "" {
		token = os.Getenv("HF_TOKEN")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hf api returned status %s", resp.Status)
	}

	var results []HFModelResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}

// ListHFModelFiles lists GGUF files for a repository.
func ListHFModelFiles(modelID string, token string) ([]HFSibling, error) {
	apiURL := fmt.Sprintf("https://huggingface.co/api/models/%s", modelID)

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	if token == "" {
		token = os.Getenv("HF_TOKEN")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hf api returned status %s", resp.Status)
	}

	var detail HFModelDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, err
	}

	// Filter siblings to only .gguf and .onnx files
	var files []HFSibling
	for _, sibling := range detail.Siblings {
		ext := strings.ToLower(filepath.Ext(sibling.Rpath))
		if ext == ".gguf" || ext == ".onnx" {
			files = append(files, sibling)
		}
	}

	return files, nil
}
