package runner

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// TODO: Add MLX support when Apple Silicon/macOS support is implemented

type ServerStatus int

const (
	StatusStopped ServerStatus = iota
	StatusRunning
	StatusFailed
)

type TaskType string

const (
	TaskTextGeneration  TaskType = "TEXT_GENERATION"
	TaskEmbedding       TaskType = "EMBEDDING"
	TaskReranking       TaskType = "RERANKING"
	TaskSpeechToText    TaskType = "SPEECH_TO_TEXT"
	TaskTextToSpeech    TaskType = "TEXT_TO_SPEECH"
	TaskImageGeneration TaskType = "IMAGE_GENERATION"
	TaskVision          TaskType = "VISION"
	TaskMultimodal      TaskType = "MULTIMODAL"
)

type InstanceInfo struct {
	Port      int
	ModelPath string
	PID       int
	Uptime    time.Duration
	LogFile   string
}

type StartOptions struct {
	LlamaCppDir    string // Specific to llama.cpp binaries
	ContextSize    uint32
	Threads        int
	GPULayers      int
	BatchSize      int
	Host           string
	Port           int
	Task           TaskType
	Backend        BackendType
	VersionTag     string
	FlashAttention bool
	CacheTypeK     string
	CacheTypeV     string
	CustomArgs     string
}

type ModelRuntime interface {
	// Start launches the model server on the specified port.
	Start(modelPath string, opts StartOptions) error
	// Stop terminates all active server instances running under this runtime.
	Stop() error
	// StopInstance terminates the server running on a specific port.
	StopInstance(port int) error
	// GetStatus queries the state of the primary instance (e.g. on port 50505).
	GetStatus() (ServerStatus, string, int)
	// GetAllInstances returns all instances currently run by this backend.
	GetAllInstances() []InstanceInfo
	// Capabilities returns which task types this runtime is capable of running.
	Capabilities() []TaskType
}

type MultiRuntimeManager struct {
	supervisor *ProcessSupervisor
	llamaCpp   *LlamaCppRuntime
	onnx       *OnnxRuntime
}

func NewMultiRuntimeManager(logDir string) *MultiRuntimeManager {
	supervisor := NewProcessSupervisor(logDir)
	return &MultiRuntimeManager{
		supervisor: supervisor,
		llamaCpp:   NewLlamaCppRuntimeWithSupervisor(supervisor),
		onnx:       NewOnnxRuntimeWithSupervisor(supervisor),
	}
}

func (m *MultiRuntimeManager) Start(modelPath string, opts StartOptions) error {
	// TODO: Route to MLXRuntime on macOS/Apple Silicon machines when MLX support is implemented
	ext := strings.ToLower(filepath.Ext(modelPath))
	if ext == ".gguf" {
		return m.llamaCpp.Start(modelPath, opts)
	} else if ext == ".onnx" {
		return m.onnx.Start(modelPath, opts)
	}
	return fmt.Errorf("unsupported model format: %s", modelPath)
}

func (m *MultiRuntimeManager) Stop() error {
	return m.supervisor.Stop()
}

func (m *MultiRuntimeManager) StopInstance(port int) error {
	return m.supervisor.StopInstance(port)
}

func (m *MultiRuntimeManager) GetStatus() (ServerStatus, string, int) {
	return m.supervisor.GetStatus()
}

func (m *MultiRuntimeManager) GetAllInstances() []InstanceInfo {
	return m.supervisor.GetAllInstances()
}

func (m *MultiRuntimeManager) Capabilities() []TaskType {
	var list []TaskType
	list = append(list, m.llamaCpp.Capabilities()...)
	list = append(list, m.onnx.Capabilities()...)
	return list
}

func (m *MultiRuntimeManager) Supervisor() *ProcessSupervisor {
	return m.supervisor
}

func (m *MultiRuntimeManager) LogDir() string {
	if m.supervisor != nil {
		return m.supervisor.LogDir()
	}
	return ""
}
