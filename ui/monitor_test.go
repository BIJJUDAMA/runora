package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/BIJJUDAMA/runora/config"
	"github.com/BIJJUDAMA/runora/profile"
	"github.com/BIJJUDAMA/runora/runner"
)

type mockMonitorRuntime struct {
	instances  []runner.InstanceInfo
	stopped    []int
	allStopped bool
	started    []string
}

func (m *mockMonitorRuntime) Start(modelPath string, opts runner.StartOptions) error {
	m.started = append(m.started, modelPath)
	return nil
}
func (m *mockMonitorRuntime) Stop() error {
	m.allStopped = true
	m.instances = nil
	return nil
}
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

func TestMonitorModel_SlotsGaugesAndControls(t *testing.T) {
	mockRuntime := &mockMonitorRuntime{
		instances: []runner.InstanceInfo{
			{
				PID:       2222,
				Port:      8080,
				ModelPath: "models/qwen2.5.gguf",
				Uptime:    100 * time.Second,
				LogFile:   "logs/server-8080.log",
			},
		},
	}

	cfg := config.DefaultConfig()
	profs := []*profile.Profile{
		{Name: "High", Context: 8192, Threads: 8, GPULayers: 33, BatchSize: 512, Port: 8080},
	}
	model := NewMonitorModelWithConfig(mockRuntime, cfg, profs)
	model.Refresh()

	// 1. Inject slots metrics and generation speed metrics
	metricsMsg := MonitorMetricsMsg{
		Instances: mockRuntime.instances,
		CachedMem: map[int]string{2222: "1500.00 MB"},
		CachedReqs: map[int]string{8080: "42 requests"},
		CachedSlots: map[int]*runner.ServerSlotMetrics{
			8080: {
				TotalNCtx:   8192,
				TotalNPast:  5324,
				PctUsed:     65.0,
				ActiveSlots: 1,
				TotalSlots:  1,
			},
		},
		CachedSpeed: map[int]string{
			8080: "28.5 tokens/sec",
		},
	}
	_ = model.Update(metricsMsg)

	// Render view and check context window gauge and speed
	view := model.View(100, 30)
	if !strings.Contains(view, "65% (5324 / 8192)") {
		t.Errorf("expected context slot gauge '65%% (5324 / 8192)' in view, got:\n%s", view)
	}
	if !strings.Contains(view, "28.5 tokens/sec") {
		t.Errorf("expected speed '28.5 tokens/sec' in view, got:\n%s", view)
	}

	// 2. Test Hot Restart ([R])
	restartCmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if restartCmd == nil {
		t.Errorf("expected restart command to return PollMetricsCmd")
	}
	if len(mockRuntime.started) != 1 || mockRuntime.started[0] != "models/qwen2.5.gguf" {
		t.Errorf("expected server to be restarted with models/qwen2.5.gguf, got %v", mockRuntime.started)
	}

	// 3. Test Log streamer trigger ([L])
	logCmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if logCmd == nil {
		t.Fatalf("expected log streamer command from [L], got nil")
	}
	msg := logCmd()
	openMsg, ok := msg.(OpenLogStreamerMsg)
	if !ok {
		t.Fatalf("expected OpenLogStreamerMsg from [L], got %T", msg)
	}
	if openMsg.Port != 8080 || openMsg.PrevScreen != ScreenServerMonitor {
		t.Errorf("unexpected OpenLogStreamerMsg content: %+v", openMsg)
	}

	// 4. Test Bulk Terminate (Ctrl+K)
	bulkCmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if bulkCmd == nil {
		t.Errorf("expected bulk terminate command from Ctrl+K")
	}
	if !mockRuntime.allStopped {
		t.Errorf("expected allStopped to be true after Ctrl+K")
	}
}
