package runner

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"
)

type mockScriptDriver struct {
	name         string
	capabilities []TaskType
	readyAttempts int
	readyCount    int
	mu            sync.Mutex
	sleepSec      int
}

func (m *mockScriptDriver) Name() string {
	return m.name
}

func (m *mockScriptDriver) Capabilities() []TaskType {
	return m.capabilities
}

func (m *mockScriptDriver) BuildCommand(ctx context.Context, modelPath string, opts StartOptions, logFile *os.File) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	sleepDuration := m.sleepSec
	if sleepDuration <= 0 {
		sleepDuration = 10
	}

	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", fmt.Sprintf("Start-Sleep -Seconds %d", sleepDuration))
	} else {
		cmd = exec.CommandContext(ctx, "sleep", fmt.Sprintf("%d", sleepDuration))
	}

	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	cmd.WaitDelay = 2 * time.Second
	configureSysProcAttr(cmd)
	return cmd, nil
}

func (m *mockScriptDriver) CheckReady(ctx context.Context, host string, port int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readyCount++
	if m.readyCount >= m.readyAttempts {
		return nil
	}
	return fmt.Errorf("still initializing")
}

func getFreePort(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestSupervisorSingleOwnerNoDoubleWait(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "supervisor-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sup := NewProcessSupervisor(tempDir)
	port := getFreePort(t)

	driver := &mockScriptDriver{name: "mock-runner", sleepSec: 15}
	inst, err := sup.StartInstance(driver, "test-model.gguf", StartOptions{
		Host: "127.0.0.1",
		Port: port,
	})
	if err != nil {
		t.Fatalf("failed to start instance: %v", err)
	}

	if inst.PID <= 0 {
		t.Errorf("expected positive PID, got %d", inst.PID)
	}

	// Concurrently call StopInstance from 15 goroutines to test for double-Wait() panics or race conditions
	var wg sync.WaitGroup
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sup.StopInstance(port)
		}()
	}
	wg.Wait()

	select {
	case <-inst.ExitedChan:
		// Exited successfully
	case <-time.After(3 * time.Second):
		t.Errorf("expected ExitedChan to be closed after StopInstance")
	}

	if len(sup.GetAllInstances()) != 0 {
		t.Errorf("expected 0 running instances, got %d", len(sup.GetAllInstances()))
	}
}

func TestSupervisorNoMutexHeldDuringStop(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "supervisor-lock-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sup := NewProcessSupervisor(tempDir)
	port1 := getFreePort(t)
	port2 := getFreePort(t)

	driver := &mockScriptDriver{name: "mock-runner", sleepSec: 15}
	_, err = sup.StartInstance(driver, "model1.gguf", StartOptions{Host: "127.0.0.1", Port: port1})
	if err != nil {
		t.Fatalf("failed to start instance 1: %v", err)
	}
	_, err = sup.StartInstance(driver, "model2.gguf", StartOptions{Host: "127.0.0.1", Port: port2})
	if err != nil {
		t.Fatalf("failed to start instance 2: %v", err)
	}

	stopDone := make(chan struct{})
	go func() {
		_ = sup.StopInstance(port1)
		close(stopDone)
	}()

	// Query status and list while StopInstance on port1 is in flight
	queryStart := time.Now()
	for i := 0; i < 10; i++ {
		_ = sup.GetAllInstances()
		status, _, _ := sup.GetStatus()
		if status == StatusStopped {
			// Should still have port2 running
		}
		time.Sleep(10 * time.Millisecond)
	}
	queryDuration := time.Since(queryStart)

	// Queries should not be blocked for 1.5s
	if queryDuration > 1000*time.Millisecond {
		t.Errorf("mutex contention detected: query duration %v exceeded threshold", queryDuration)
	}

	<-stopDone
	_ = sup.Stop()
}

func TestSupervisorWaitForReady(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "supervisor-ready-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sup := NewProcessSupervisor(tempDir)
	port := getFreePort(t)

	driver := &mockScriptDriver{
		name:          "mock-ready",
		readyAttempts: 3,
		sleepSec:      10,
	}
	inst, err := sup.StartInstance(driver, "test-ready.gguf", StartOptions{Host: "127.0.0.1", Port: port})
	if err != nil {
		t.Fatalf("failed to start instance: %v", err)
	}
	defer func() { _ = sup.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = sup.WaitForReady(ctx, inst, 2*time.Second)
	if err != nil {
		t.Errorf("expected WaitForReady to succeed, got %v", err)
	}
}

func TestSupervisorWaitForReadyPrematureExit(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "supervisor-exit-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sup := NewProcessSupervisor(tempDir)
	port := getFreePort(t)

	// Short-lived process (1 second) with unreachable readiness (100 attempts)
	driver := &mockScriptDriver{
		name:          "mock-fast-exit",
		readyAttempts: 100,
		sleepSec:      1,
	}
	inst, err := sup.StartInstance(driver, "test-exit.gguf", StartOptions{Host: "127.0.0.1", Port: port})
	if err != nil {
		t.Fatalf("failed to start instance: %v", err)
	}
	defer func() { _ = sup.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = sup.WaitForReady(ctx, inst, 4*time.Second)
	if err == nil {
		t.Errorf("expected WaitForReady to fail due to premature exit, got nil")
	}
}

func TestMultiRuntimeManagerUnified(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "multi-runtime-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mgr := NewMultiRuntimeManager(tempDir)
	if mgr.Supervisor() == nil {
		t.Fatal("expected non-nil supervisor")
	}

	caps := mgr.Capabilities()
	if len(caps) == 0 {
		t.Errorf("expected capabilities from multi-runtime manager")
	}

	// Verify unsupported model format returns error
	err = mgr.Start("model.invalid_format", StartOptions{Host: "127.0.0.1", Port: 50505})
	if err == nil {
		t.Errorf("expected error for invalid format, got nil")
	}
}

func TestMultiRuntimeRoutingByExtension(t *testing.T) {
	mgr := NewMultiRuntimeManager("")

	// Unsupported extension should fail immediately
	err := mgr.Start("models/unknown.bin", StartOptions{Host: "127.0.0.1", Port: 50505})
	if err == nil {
		t.Errorf("expected error starting .bin, got nil")
	}

	// Supported capabilities list should combine both engines
	caps := mgr.Capabilities()
	if len(caps) == 0 {
		t.Errorf("expected non-empty capabilities from MultiRuntimeManager")
	}
}
