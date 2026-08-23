package runner

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// EngineDriver abstracts command preparation, readiness polling, and capability detection for an execution backend.
type EngineDriver interface {
	// Name returns the driver backend name (e.g. "llama.cpp", "onnx").
	Name() string
	// Capabilities returns the task types supported by this driver.
	Capabilities() []TaskType
	// BuildCommand constructs the exec.Cmd to launch the server for this model and options.
	BuildCommand(ctx context.Context, modelPath string, opts StartOptions, logFile *os.File) (*exec.Cmd, error)
	// CheckReady polls or probes if the server at host:port is ready to accept requests.
	CheckReady(ctx context.Context, host string, port int) error
}

// LlamaCppDriver implements EngineDriver for llama.cpp binary servers.
type LlamaCppDriver struct{}

func NewLlamaCppDriver() *LlamaCppDriver {
	return &LlamaCppDriver{}
}

func (d *LlamaCppDriver) Name() string {
	return "llama.cpp"
}

func (d *LlamaCppDriver) Capabilities() []TaskType {
	return []TaskType{TaskTextGeneration, TaskEmbedding}
}

func (d *LlamaCppDriver) BuildCommand(ctx context.Context, modelPath string, opts StartOptions, logFile *os.File) (*exec.Cmd, error) {
	binaryName := "llama-server"
	if runtime.GOOS == "windows" {
		binaryName = "llama-server.exe"
	}
	binaryPath := filepath.Join(opts.LlamaCppDir, binaryName)

	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("llama-server binary not found at %s", binaryPath)
	}

	args := []string{
		"--model", modelPath,
		"--host", opts.Host,
		"--port", fmt.Sprintf("%d", opts.Port),
	}
	if opts.ContextSize > 0 {
		args = append(args, "--ctx-size", fmt.Sprintf("%d", opts.ContextSize))
	}
	if opts.Threads > 0 {
		args = append(args, "--threads", fmt.Sprintf("%d", opts.Threads))
	}
	if opts.GPULayers >= 0 {
		args = append(args, "--n-gpu-layers", fmt.Sprintf("%d", opts.GPULayers))
	}
	if opts.BatchSize > 0 {
		args = append(args, "--batch-size", fmt.Sprintf("%d", opts.BatchSize))
	}
	if opts.Task == TaskEmbedding {
		args = append(args, "--embedding")
	} else if opts.Task == TaskReranking {
		args = append(args, "--reranking")
	}

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	// Go 1.20+ WaitDelay ensures pipe I/O cleanly terminates after process exit without hanging
	cmd.WaitDelay = 2 * time.Second
	configureSysProcAttr(cmd)

	return cmd, nil
}

func (d *LlamaCppDriver) CheckReady(ctx context.Context, host string, port int) error {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s:%d/health", host, port), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return fmt.Errorf("server loading weights (HTTP 503)")
	}
	return fmt.Errorf("unexpected health status: %d", resp.StatusCode)
}

// OnnxDriver implements EngineDriver for ONNX runtime servers.
type OnnxDriver struct{}

func NewOnnxDriver() *OnnxDriver {
	return &OnnxDriver{}
}

func (d *OnnxDriver) Name() string {
	return "onnx"
}

func (d *OnnxDriver) Capabilities() []TaskType {
	return []TaskType{TaskEmbedding, TaskReranking, TaskSpeechToText, TaskVision, TaskImageGeneration}
}

func (d *OnnxDriver) BuildCommand(ctx context.Context, modelPath string, opts StartOptions, logFile *os.File) (*exec.Cmd, error) {
	// TODO: Integrate native ONNX runtime server wrapper when binary is provided.
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", "Start-Sleep -Seconds 86400")
	} else {
		cmd = exec.CommandContext(ctx, "sleep", "86400")
	}
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	cmd.WaitDelay = 2 * time.Second
	configureSysProcAttr(cmd)
	return cmd, nil
}

func (d *OnnxDriver) CheckReady(ctx context.Context, host string, port int) error {
	return nil
}
