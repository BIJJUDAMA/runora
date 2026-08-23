package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/BIJJUDAMA/runora/runner"
)

type mockMonitorRuntime struct {
	instances []runner.InstanceInfo
	stopped   []int
}

func (m *mockMonitorRuntime) Start(modelPath string, opts runner.StartOptions) error { return nil }
func (m *mockMonitorRuntime) Stop() error { return nil }
func (m *mockMonitorRuntime) StopInstance(port int) error {
	m.stopped = append(m.stopped, port)
	for i, inst := range m.instances {
		if inst.Port == port {
			m.instances = append(m.instances[:i], m.instances[i+1:]...)
			break
		}
	}
	return nil
}
func (m *mockMonitorRuntime) GetStatus() (runner.ServerStatus, string, int) { return runner.StatusStopped, "", 0 }
func (m *mockMonitorRuntime) GetAllInstances() []runner.InstanceInfo { return m.instances }
func (m *mockMonitorRuntime) Capabilities() []runner.TaskType { return nil }

func TestMonitorModel_PureView(t *testing.T) {
	mockRuntime := &mockMonitorRuntime{
		instances: []runner.InstanceInfo{
			{
				PID:       12345,
				Port:      8080,
				ModelPath: "models/test-qwen-2.5-7b.gguf",
				Uptime:    125 * time.Second,
				LogFile:   "logs/test.log",
			},
		},
	}

	model := NewMonitorModel(mockRuntime)
	// Refresh manually for test setup
	model.Refresh()

	if len(model.instances) != 1 {
		t.Fatalf("expected 1 instance initially, got %d", len(model.instances))
	}

	// Change mock runtime underlying instances to test that View() does NOT synchronously refresh
	mockRuntime.instances = append(mockRuntime.instances, runner.InstanceInfo{
		PID:       67890,
		Port:      8081,
		ModelPath: "models/test-llama-3.gguf",
		Uptime:    30 * time.Second,
		LogFile:   "logs/test2.log",
	})

	// Render view
	viewOutput := model.View(80, 24)

	// Since View() is pure and does not call Refresh(), model.instances should still be 1
	if len(model.instances) != 1 {
		t.Errorf("expected View() to be 100%% pure with zero state mutation or synchronous I/O, got %d instances", len(model.instances))
	}

	if viewOutput == "" {
		t.Errorf("expected non-empty view output")
	}
}

func TestMonitorModel_AsyncMetricsPolling(t *testing.T) {
	mockRuntime := &mockMonitorRuntime{
		instances: []runner.InstanceInfo{
			{
				PID:       12345,
				Port:      8080,
				ModelPath: "models/test-model.gguf",
				Uptime:    60 * time.Second,
				LogFile:   "logs/test.log",
			},
		},
	}

	model := NewMonitorModel(mockRuntime)

	// Test Tick command generation
	tickCmd := MonitorTickCmd()
	if tickCmd == nil {
		t.Errorf("expected MonitorTickCmd to return non-nil tea.Cmd")
	}

	// Test PollMetricsCmd
	pollCmd := model.PollMetricsCmd()
	if pollCmd == nil {
		t.Fatalf("expected PollMetricsCmd to return non-nil tea.Cmd")
	}

	msg := pollCmd()
	metricsMsg, ok := msg.(MonitorMetricsMsg)
	if !ok {
		t.Fatalf("expected PollMetricsCmd to produce MonitorMetricsMsg, got %T", msg)
	}

	if len(metricsMsg.Instances) != 1 {
		t.Errorf("expected 1 instance in metricsMsg, got %d", len(metricsMsg.Instances))
	}

	// Update model with metricsMsg
	cmd := model.Update(metricsMsg)
	if cmd != nil {
		t.Errorf("expected nil cmd on metrics update, got %v", cmd)
	}
	if len(model.instances) != 1 {
		t.Errorf("expected model instances to be updated to 1, got %d", len(model.instances))
	}

	// Test keyboard navigation
	_ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})

	// Test stopping instance
	stopCmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if stopCmd == nil {
		t.Errorf("expected stop command to return PollMetricsCmd")
	}
	if len(mockRuntime.stopped) != 1 || mockRuntime.stopped[0] != 8080 {
		t.Errorf("expected port 8080 to be stopped, got %v", mockRuntime.stopped)
	}
}
