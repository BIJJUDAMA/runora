package runner

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ManagedInstance tracks the runtime state of a supervised process.
type ManagedInstance struct {
	Port       int
	ModelPath  string
	Driver     EngineDriver
	Cmd        *exec.Cmd
	Process    *os.Process
	PID        int
	LaunchTime time.Time
	LogFile    string
	LogCloser  io.Closer
	CancelFunc context.CancelFunc
	ExitedChan chan struct{} // Closed exclusively by the single-owner wait goroutine upon process exit
	ExitErr    error
}

// ProcessSupervisor manages process lifecycles, guarantees zero double-Wait races,
// and orchestrates graceful multi-stage terminations.
type ProcessSupervisor struct {
	mu        sync.Mutex
	instances map[int]*ManagedInstance
	logDir    string
}

// NewProcessSupervisor initializes a new supervisor with the designated log directory.
func NewProcessSupervisor(logDir string) *ProcessSupervisor {
	if logDir == "" {
		logDir = "."
	}
	_ = os.MkdirAll(logDir, 0755)
	return &ProcessSupervisor{
		instances: make(map[int]*ManagedInstance),
		logDir:    logDir,
	}
}

func sanitizeDriverName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, ".", "")
	name = strings.ReplaceAll(name, " ", "-")
	if name == "" {
		return "server"
	}
	return name
}

// StartInstance starts and supervises a process instance using the given driver and options.
func (ps *ProcessSupervisor) StartInstance(driver EngineDriver, modelPath string, opts StartOptions) (*ManagedInstance, error) {
	ps.mu.Lock()
	if _, exists := ps.instances[opts.Port]; exists {
		ps.mu.Unlock()
		return nil, fmt.Errorf("a server is already running on port %d", opts.Port)
	}
	ps.mu.Unlock()

	// Check if port is already bound on OS TCP stack
	ln, netErr := net.Listen("tcp", fmt.Sprintf("%s:%d", opts.Host, opts.Port))
	if netErr != nil {
		return nil, fmt.Errorf("port %d is already in use by another process: %w", opts.Port, netErr)
	}
	_ = ln.Close()

	// Prepare log file
	logFileName := fmt.Sprintf("%s-%d.log", sanitizeDriverName(driver.Name()), opts.Port)
	logFilePath := filepath.Join(ps.logDir, logFileName)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", logFilePath, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cmd, err := driver.BuildCommand(ctx, modelPath, opts, logFile)
	if err != nil {
		_ = logFile.Close()
		cancel()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		cancel()
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	pid := 0
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}

	exitedChan := make(chan struct{})
	inst := &ManagedInstance{
		Port:       opts.Port,
		ModelPath:  modelPath,
		Driver:     driver,
		Cmd:        cmd,
		Process:    cmd.Process,
		PID:        pid,
		LaunchTime: time.Now(),
		LogFile:    logFilePath,
		LogCloser:  logFile,
		CancelFunc: cancel,
		ExitedChan: exitedChan,
	}

	ps.mu.Lock()
	ps.instances[opts.Port] = inst
	ps.mu.Unlock()

	// Single-owner cmd.Wait() goroutine:
	// Exclusively owns cmd.Wait() to prevent any double-Wait() race condition across the system.
	go func(managed *ManagedInstance) {
		defer func() {
			if managed.LogCloser != nil {
				_ = managed.LogCloser.Close()
			}
			close(managed.ExitedChan)
			ps.onInstanceExit(managed.Port)
		}()
		managed.ExitErr = managed.Cmd.Wait()
	}(inst)

	return inst, nil
}

func (ps *ProcessSupervisor) onInstanceExit(port int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.instances, port)
}

// WaitForReady polls the driver's readiness probe until success, process exit, or timeout.
func (ps *ProcessSupervisor) WaitForReady(ctx context.Context, inst *ManagedInstance, timeout time.Duration) error {
	if timeout <= 0 {
		return nil
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-inst.ExitedChan:
			if inst.ExitErr != nil {
				return fmt.Errorf("process exited prematurely with error: %w", inst.ExitErr)
			}
			return fmt.Errorf("process exited prematurely before becoming ready")
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			if now.After(deadline) {
				return fmt.Errorf("timed out waiting for instance on port %d to become ready", inst.Port)
			}
			if err := inst.Driver.CheckReady(ctx, "127.0.0.1", inst.Port); err == nil {
				return nil
			}
		}
	}
}

// StopInstance performs a two-stage graceful stop:
// Stage 1: Signals OS interrupt (process group on Unix, CTRL_BREAK/interrupt on Windows) and waits up to 1.5s on ExitedChan.
// Stage 2: If the process did not exit, sends SIGKILL / force kill.
// Crucially, no mutex is held during synchronous waits.
func (ps *ProcessSupervisor) StopInstance(port int) error {
	ps.mu.Lock()
	inst, exists := ps.instances[port]
	if !exists {
		ps.mu.Unlock()
		return nil
	}
	// Unlock immediately so no mutex is held during synchronous waits
	ps.mu.Unlock()

	// Stage 1: Graceful interrupt
	if inst.CancelFunc != nil {
		inst.CancelFunc()
	}
	if inst.Process != nil {
		_ = interruptProcess(inst.Process)
	}

	select {
	case <-inst.ExitedChan:
		return nil
	case <-time.After(1500 * time.Millisecond):
		// Stage 1 timed out, escalate to Stage 2
	}

	// Stage 2: Force kill
	if inst.Process != nil {
		_ = killProcess(inst.Process)
	}

	select {
	case <-inst.ExitedChan:
		return nil
	case <-time.After(2 * time.Second):
		return fmt.Errorf("process on port %d failed to terminate after kill", port)
	}
}

// Stop terminates all managed instances concurrently.
func (ps *ProcessSupervisor) Stop() error {
	ps.mu.Lock()
	instances := make([]*ManagedInstance, 0, len(ps.instances))
	for _, inst := range ps.instances {
		instances = append(instances, inst)
	}
	ps.mu.Unlock()

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var lastErr error

	for _, inst := range instances {
		wg.Add(1)
		go func(in *ManagedInstance) {
			defer wg.Done()
			if err := ps.StopInstance(in.Port); err != nil {
				errMu.Lock()
				lastErr = err
				errMu.Unlock()
			}
		}(inst)
	}
	wg.Wait()
	return lastErr
}

// GetStatus returns the status, running model path, and port of the primary running server.
func (ps *ProcessSupervisor) GetStatus() (ServerStatus, string, int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if len(ps.instances) == 0 {
		return StatusStopped, "", 50505
	}

	if inst, exists := ps.instances[50505]; exists {
		return StatusRunning, inst.ModelPath, 50505
	}

	for port, inst := range ps.instances {
		return StatusRunning, inst.ModelPath, port
	}

	return StatusStopped, "", 50505
}

// GetAllInstances returns status information for all active servers.
func (ps *ProcessSupervisor) GetAllInstances() []InstanceInfo {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	list := make([]InstanceInfo, 0, len(ps.instances))
	for port, inst := range ps.instances {
		list = append(list, InstanceInfo{
			Port:      port,
			ModelPath: inst.ModelPath,
			PID:       inst.PID,
			Uptime:    time.Since(inst.LaunchTime),
			LogFile:   inst.LogFile,
		})
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].Port < list[j].Port
	})
	return list
}

// GetInstance returns a snapshot of a managed instance by port.
func (ps *ProcessSupervisor) GetInstance(port int) (*ManagedInstance, bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	inst, exists := ps.instances[port]
	return inst, exists
}

// LogDir returns the configured supervisor log directory.
func (ps *ProcessSupervisor) LogDir() string {
	return ps.logDir
}

// GetLogPath returns the path to the log file for an instance on the given port.
func (ps *ProcessSupervisor) GetLogPath(port int) string {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if inst, ok := ps.instances[port]; ok {
		return inst.LogFile
	}
	return filepath.Join(ps.logDir, fmt.Sprintf("server-%d.log", port))
}
