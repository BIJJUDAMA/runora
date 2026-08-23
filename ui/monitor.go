package ui

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/config"
	"github.com/BIJJUDAMA/runora/profile"
	"github.com/BIJJUDAMA/runora/runner"
)

type MonitorTickMsg struct{}

type MonitorMetricsMsg struct {
	Instances   []runner.InstanceInfo
	CachedMem   map[int]string
	CachedReqs  map[int]string
	CachedSlots map[int]*runner.ServerSlotMetrics
	CachedSpeed map[int]string
}

func MonitorTickCmd() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(t time.Time) tea.Msg {
		return MonitorTickMsg{}
	})
}

type MonitorModel struct {
	srvRunner   runner.ModelRuntime
	cfg         *config.Config
	profiles    []*profile.Profile
	instances   []runner.InstanceInfo
	selected    int
	cachedMem   map[int]string
	cachedReqs  map[int]string
	cachedSlots map[int]*runner.ServerSlotMetrics
	cachedSpeed map[int]string
	prevTokens  map[int]int
	prevTime    map[int]time.Time
}

func NewMonitorModel(srv runner.ModelRuntime) *MonitorModel {
	return &MonitorModel{
		srvRunner:   srv,
		instances:   []runner.InstanceInfo{},
		selected:    0,
		cachedMem:   make(map[int]string),
		cachedReqs:  make(map[int]string),
		cachedSlots: make(map[int]*runner.ServerSlotMetrics),
		cachedSpeed: make(map[int]string),
		prevTokens:  make(map[int]int),
		prevTime:    make(map[int]time.Time),
	}
}

func NewMonitorModelWithConfig(srv runner.ModelRuntime, cfg *config.Config, profiles []*profile.Profile) *MonitorModel {
	m := NewMonitorModel(srv)
	m.cfg = cfg
	m.profiles = profiles
	return m
}

func (m *MonitorModel) SetProfiles(profiles []*profile.Profile) {
	m.profiles = profiles
}

func (m *MonitorModel) Refresh() {
	m.instances = m.srvRunner.GetAllInstances()
	if m.selected >= len(m.instances) {
		m.selected = len(m.instances) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m *MonitorModel) resolveStartOptions(inst runner.InstanceInfo) runner.StartOptions {
	llamaCppDir := "llama.cpp"
	threads := runtime.NumCPU()
	if threads > 8 {
		threads = 8
	}
	ctxSize := uint32(4096)
	gpuLayers := 33
	batchSize := 512
	host := "127.0.0.1"
	task := runner.TaskTextGeneration

	if m.cfg != nil {
		if m.cfg.Paths.LlamaCPP != "" {
			llamaCppDir = m.cfg.Paths.LlamaCPP
		}
		profName := m.cfg.ModelProfiles[inst.ModelPath]
		if profName == "" {
			profName = "Balanced"
		}
		for _, p := range m.profiles {
			if p.Name == profName {
				ctxSize = p.Context
				threads = p.Threads
				gpuLayers = p.GPULayers
				batchSize = p.BatchSize
				if p.Host != "" {
					host = p.Host
				}
				break
			}
		}
		if t, ok := m.cfg.ModelTasks[inst.ModelPath]; ok && t != "" {
			task = runner.TaskType(t)
		}
	}

	return runner.StartOptions{
		LlamaCppDir: llamaCppDir,
		ContextSize: ctxSize,
		Threads:     threads,
		GPULayers:   gpuLayers,
		BatchSize:   batchSize,
		Host:        host,
		Port:        inst.Port,
		Task:        task,
	}
}

func (m *MonitorModel) PollMetricsCmd() tea.Cmd {
	srv := m.srvRunner
	prevTokensMap := make(map[int]int, len(m.prevTokens))
	for k, v := range m.prevTokens {
		prevTokensMap[k] = v
	}
	prevTimeMap := make(map[int]time.Time, len(m.prevTime))
	for k, v := range m.prevTime {
		prevTimeMap[k] = v
	}

	return func() tea.Msg {
		instances := srv.GetAllInstances()
		memMap := make(map[int]string, len(instances))
		reqsMap := make(map[int]string, len(instances))
		slotsMap := make(map[int]*runner.ServerSlotMetrics, len(instances))
		speedMap := make(map[int]string, len(instances))

		now := time.Now()

		for _, inst := range instances {
			// Memory RSS
			mem, err := runner.GetMemoryUsage(inst.PID)
			if err == nil {
				memMap[inst.PID] = fmt.Sprintf("%.2f MB", mem)
			} else {
				memMap[inst.PID] = "N/A"
			}

			// Total requests
			reqs, err := runner.QueryServerRequests(inst.Port)
			if err == nil {
				reqsMap[inst.Port] = fmt.Sprintf("%d requests", reqs)
			} else {
				reqsMap[inst.Port] = "0 requests"
			}

			// Slots & Context window utilization
			slotMetrics, err := runner.QueryServerSlots(inst.Port)
			if err == nil && slotMetrics != nil {
				slotsMap[inst.Port] = slotMetrics
			}

			// Generation speed (tokens/sec)
			curTokens, err := runner.QueryServerTokens(inst.Port)
			if err == nil && curTokens > 0 {
				if lastTime, ok := prevTimeMap[inst.Port]; ok {
					lastTokens := prevTokensMap[inst.Port]
					elapsed := now.Sub(lastTime).Seconds()
					if elapsed > 0.1 && curTokens >= lastTokens {
						delta := curTokens - lastTokens
						tokPerSec := float64(delta) / elapsed
						if tokPerSec > 0 {
							speedMap[inst.Port] = fmt.Sprintf("%.1f tokens/sec", tokPerSec)
						} else {
							speedMap[inst.Port] = "0.0 tokens/sec (Idle)"
						}
					} else {
						speedMap[inst.Port] = "0.0 tokens/sec (Idle)"
					}
				} else {
					speedMap[inst.Port] = "0.0 tokens/sec"
				}
				prevTokensMap[inst.Port] = curTokens
				prevTimeMap[inst.Port] = now
			} else {
				speedMap[inst.Port] = "0.0 tokens/sec (Idle)"
			}
		}

		return MonitorMetricsMsg{
			Instances:   instances,
			CachedMem:   memMap,
			CachedReqs:  reqsMap,
			CachedSlots: slotsMap,
			CachedSpeed: speedMap,
		}
	}
}

func (m *MonitorModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case MonitorTickMsg:
		return tea.Batch(m.PollMetricsCmd(), MonitorTickCmd())

	case MonitorMetricsMsg:
		m.instances = msg.Instances
		for k, v := range msg.CachedMem {
			m.cachedMem[k] = v
		}
		for k, v := range msg.CachedReqs {
			m.cachedReqs[k] = v
		}
		for k, v := range msg.CachedSlots {
			m.cachedSlots[k] = v
		}
		for k, v := range msg.CachedSpeed {
			m.cachedSpeed[k] = v
		}
		if m.selected >= len(m.instances) {
			m.selected = len(m.instances) - 1
		}
		if m.selected < 0 {
			m.selected = 0
		}
		return nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.instances)-1 {
				m.selected++
			}
		case "s", "S":
			if len(m.instances) > 0 && m.selected >= 0 && m.selected < len(m.instances) {
				port := m.instances[m.selected].Port
				_ = m.srvRunner.StopInstance(port)
				return m.PollMetricsCmd()
			}
		case "ctrl+k":
			// Bulk terminate all running instances concurrently
			_ = m.srvRunner.Stop()
			return m.PollMetricsCmd()
		case "r", "R":
			// Hot-restart selected server with active profile
			if len(m.instances) > 0 && m.selected >= 0 && m.selected < len(m.instances) {
				inst := m.instances[m.selected]
				opts := m.resolveStartOptions(inst)
				_ = m.srvRunner.StopInstance(inst.Port)
				_ = m.srvRunner.Start(inst.ModelPath, opts)
				return m.PollMetricsCmd()
			}
		case "l", "L":
			if len(m.instances) > 0 && m.selected >= 0 && m.selected < len(m.instances) {
				port := m.instances[m.selected].Port
				return func() tea.Msg {
					return OpenLogStreamerMsg{Port: port, PrevScreen: ScreenServerMonitor}
				}
			}
		}
	}
	return nil
}

func (m *MonitorModel) View(width int, height int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("RUNTIME SERVER MONITOR")))

	if len(m.instances) == 0 {
		sb.WriteString("  No active server instances are currently running.\n\n")
		sb.WriteString("  " + StyleHelpKey.Render("[Esc]") + " Back to Browser\n")
	} else {
		sb.WriteString("  " + lipgloss.NewStyle().Bold(true).Render("Active Server Instances:") + "\n")
		sb.WriteString(fmt.Sprintf("  %-6s %-20s %-10s %-10s %-6s\n", "Port", "Model", "PID", "Uptime", "Status"))
		divWidth := width - 8
		if divWidth < 10 {
			divWidth = 10
		}
		sb.WriteString("  " + strings.Repeat("─", divWidth) + "\n")

		for idx, inst := range m.instances {
			modelName := filepath.Base(inst.ModelPath)
			modelName = TruncateVisual(modelName, 20, "...")

			uptimeSec := int(inst.Uptime.Seconds())
			uptimeStr := fmt.Sprintf("%dh %dm %ds", uptimeSec/3600, (uptimeSec%3600)/60, uptimeSec%60)

			statusStr := lipgloss.NewStyle().Foreground(ColorSecondary).Render("Serving")

			row := fmt.Sprintf("  %-6d %-20s %-10d %-10s %-6s",
				inst.Port, modelName, inst.PID, uptimeStr, statusStr,
			)

			if idx == m.selected {
				rowWidth := width - 8
				if rowWidth < 10 {
					rowWidth = 10
				}
				sb.WriteString(StyleSelectedListItem.Width(rowWidth).Render(row) + "\n")
			} else {
				sb.WriteString(row + "\n")
			}
		}

		sb.WriteString("\n")

		selectedIdx := m.selected
		if selectedIdx >= len(m.instances) {
			selectedIdx = len(m.instances) - 1
		}
		if selectedIdx < 0 {
			selectedIdx = 0
		}
		selectedInst := m.instances[selectedIdx]

		sb.WriteString("  " + lipgloss.NewStyle().Bold(true).Render("Selected Instance Performance Metrics:") + "\n")
		sb.WriteString("  " + strings.Repeat("─", divWidth) + "\n")

		memStr, hasMem := m.cachedMem[selectedInst.PID]
		if !hasMem {
			memStr = "Gathering..."
		}
		reqStr, hasReq := m.cachedReqs[selectedInst.Port]
		if !hasReq {
			reqStr = "Gathering..."
		}

		// Slot / context window utilization gauge: [████████░░░░] 65% (5324 / 8192)
		var contextGaugeStr string
		if slotMetrics, ok := m.cachedSlots[selectedInst.Port]; ok && slotMetrics != nil && slotMetrics.TotalNCtx > 0 {
			barColor := ColorSecondary
			if slotMetrics.PctUsed > 85 {
				barColor = ColorGold
			}
			if slotMetrics.PctUsed > 95 {
				barColor = ColorDanger
			}
			bar := RenderProgressBar(slotMetrics.PctUsed, 12, barColor)
			contextGaugeStr = fmt.Sprintf("[%s] %.0f%% (%d / %d)", bar, slotMetrics.PctUsed, slotMetrics.TotalNPast, slotMetrics.TotalNCtx)
		} else {
			contextGaugeStr = fmt.Sprintf("[%s] 0%% (Idle / No slot data)", RenderProgressBar(0, 12, ColorProgressEmpty))
		}

		// Token generation speed (tokens/sec)
		speedStr, hasSpeed := m.cachedSpeed[selectedInst.Port]
		if !hasSpeed {
			speedStr = "0.0 tokens/sec (Idle)"
		}

		sb.WriteString(fmt.Sprintf("  %-20s %d\n", "Process PID:", selectedInst.PID))
		sb.WriteString(fmt.Sprintf("  %-20s %d\n", "Server Port:", selectedInst.Port))
		sb.WriteString(fmt.Sprintf("  %-20s http://127.0.0.1:%d\n", "Endpoint:", selectedInst.Port))
		sb.WriteString(fmt.Sprintf("  %-20s %s\n", "Active Memory (RSS):", memStr))
		sb.WriteString(fmt.Sprintf("  %-20s %s\n", "Requests Handled:", reqStr))
		sb.WriteString(fmt.Sprintf("  %-20s %s\n", "Context Window:", contextGaugeStr))
		sb.WriteString(fmt.Sprintf("  %-20s %s\n", "Generation Speed:", speedStr))
		sb.WriteString(fmt.Sprintf("  %-20s %s\n\n", "Log File Path:", selectedInst.LogFile))

		helpStr := fmt.Sprintf("%s Stop  %s Restart  %s Stream Logs  %s Terminate All  %s Back",
			StyleHelpKey.Render("[S]"),
			StyleHelpKey.Render("[R]"),
			StyleHelpKey.Render("[L]"),
			StyleHelpKey.Render("[Ctrl+K]"),
			StyleHelpKey.Render("[Esc]"),
		)
		sb.WriteString("  " + helpStr + "\n")
	}

	boxWidth := width - 4
	if boxWidth < 50 {
		boxWidth = 50
	}
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorPrimary).
		Width(boxWidth).
		Render(sb.String())
}
