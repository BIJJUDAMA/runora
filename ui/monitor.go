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
	cardWidth := width
	if cardWidth < 50 {
		cardWidth = 50
	}
	innerWidth := cardWidth - 6
	if innerWidth < 20 {
		innerWidth = 20
	}

	startHex := ThemeGradientStart
	if startHex == "" {
		startHex = "#7D56F4"
	}
	endHex := ThemeGradientEnd
	if endHex == "" {
		endHex = "#04B575"
	}

	// Calculate height distribution among cards
	var hSummary, hInstance, hContext, hControl int
	if height >= 16 {
		hSummary = (height * 7) / 30
		hInstance = (height * 9) / 30
		hContext = (height * 8) / 30
		hControl = height - hSummary - hInstance - hContext
	}

	// 1. Top Summary Bento Card
	var summaryContent string
	var summaryBadge string

	if len(m.instances) == 0 {
		summaryBadge = "0 running"
		var sumSb strings.Builder
		sumSb.WriteString("  No active server instances are currently running.\n")
		sumSb.WriteString("  Start or launch a model from the Models screen [1] or Launch Dashboard [2].")
		summaryContent = sumSb.String()
	} else {
		summaryBadge = fmt.Sprintf("%d running", len(m.instances))
		var sumSb strings.Builder

		// Multi-instance summary table header
		headerRow := fmt.Sprintf("  %-6s %-22s %-8s %-12s %-18s %-8s", "PORT", "MODEL", "PID", "UPTIME", "GENERATION SPEED", "STATUS")
		sumSb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ColorTextMuted).Render(headerRow) + "\n")
		divW := innerWidth
		if divW > 80 {
			divW = 80
		}
		sumSb.WriteString("  " + strings.Repeat("─", divW) + "\n")

		for idx, inst := range m.instances {
			modelName := filepath.Base(inst.ModelPath)
			modelName = TruncateVisual(modelName, 22, "...")

			uptimeSec := int(inst.Uptime.Seconds())
			uptimeStr := fmt.Sprintf("%dh %dm %ds", uptimeSec/3600, (uptimeSec%3600)/60, uptimeSec%60)

			statusStr := lipgloss.NewStyle().Foreground(ColorSecondary).Render("Serving")

			speedStr, hasSpeed := m.cachedSpeed[inst.Port]
			if !hasSpeed {
				speedStr = "0.0 tokens/sec"
			}

			row := fmt.Sprintf("  %-6d %-22s %-8d %-12s %-18s %-8s",
				inst.Port, modelName, inst.PID, uptimeStr, speedStr, statusStr,
			)

			if idx == m.selected {
				sumSb.WriteString(StyleSelectedListItem.Width(innerWidth).Render(row) + "\n")
			} else {
				sumSb.WriteString(row + "\n")
			}
		}
		summaryContent = strings.TrimRight(sumSb.String(), "\n")
	}

	// If no instances running, render Top Summary Card + Control Actions Card
	if len(m.instances) == 0 {
		var hEmptySum, hEmptyCtrl int
		if height >= 8 {
			hEmptySum = height / 2
			hEmptyCtrl = height - hEmptySum
		}
		summaryCard := SurfaceCardWithHeight("Active Server Instances", summaryContent, cardWidth, hEmptySum, false, summaryBadge)
		controlShortcuts := fmt.Sprintf("  %s Back to Browser  %s Models  %s Launch",
			StyleHelpKey.Render("[Esc]"),
			StyleHelpKey.Render("[1]"),
			StyleHelpKey.Render("[2]"),
		)
		controlCard := SurfaceCardWithHeight("Control Actions", controlShortcuts, cardWidth, hEmptyCtrl, false, "")
		return summaryCard + "\n" + controlCard
	}

	summaryCard := SurfaceCardWithHeight("Active Server Instances", summaryContent, cardWidth, hSummary, false, summaryBadge)

	// Active selected instance resolution
	selectedIdx := m.selected
	if selectedIdx >= len(m.instances) {
		selectedIdx = len(m.instances) - 1
	}
	if selectedIdx < 0 {
		selectedIdx = 0
	}
	selectedInst := m.instances[selectedIdx]

	modelBase := filepath.Base(selectedInst.ModelPath)
	modelBaseTrunc := TruncateVisual(modelBase, max(24, innerWidth-25), "...")

	uptimeSec := int(selectedInst.Uptime.Seconds())
	uptimeStr := fmt.Sprintf("%dh %dm %ds", uptimeSec/3600, (uptimeSec%3600)/60, uptimeSec%60)

	memStr, hasMem := m.cachedMem[selectedInst.PID]
	if !hasMem {
		memStr = "Gathering..."
	}
	reqStr, hasReq := m.cachedReqs[selectedInst.Port]
	if !hasReq {
		reqStr = "Gathering..."
	}
	speedStr, hasSpeed := m.cachedSpeed[selectedInst.Port]
	if !hasSpeed {
		speedStr = "0.0 tokens/sec (Idle)"
	}

	// 2. Live Instance List / Selected Instance Bento Card
	var instSb strings.Builder
	if innerWidth >= 70 {
		colW := innerWidth / 2
		instSb.WriteString(fmt.Sprintf("  %-18s %s\n", "Model Name:", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(modelBaseTrunc)))

		row2L := fmt.Sprintf("  %-18s %d", "Server Port:", selectedInst.Port)
		row2R := fmt.Sprintf("%-18s %d", "Process PID:", selectedInst.PID)
		instSb.WriteString(fmt.Sprintf("%-*s %s\n", colW, row2L, row2R))

		row3L := fmt.Sprintf("  %-18s %s", "Host / Endpoint:", fmt.Sprintf("http://127.0.0.1:%d", selectedInst.Port))
		row3R := fmt.Sprintf("%-18s %s", "Uptime:", uptimeStr)
		instSb.WriteString(fmt.Sprintf("%-*s %s\n", colW, row3L, row3R))

		row4L := fmt.Sprintf("  %-18s %s", "Generation Speed:", speedStr)
		row4R := fmt.Sprintf("%-18s %s", "Active Memory (RSS):", memStr)
		instSb.WriteString(fmt.Sprintf("%-*s %s\n", colW, row4L, row4R))

		row5L := fmt.Sprintf("  %-18s %s", "HTTP Status:", lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render("HTTP 200 OK (Serving)"))
		row5R := fmt.Sprintf("%-18s %s", "Requests Handled:", reqStr)
		instSb.WriteString(fmt.Sprintf("%-*s %s", colW, row5L, row5R))

		if selectedInst.LogFile != "" {
			instSb.WriteString(fmt.Sprintf("\n  %-18s %s", "Log File Path:", TruncateVisual(selectedInst.LogFile, max(30, innerWidth-22), "...")))
		}
	} else {
		instSb.WriteString(fmt.Sprintf("  %-18s %s\n", "Model Name:", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(modelBaseTrunc)))
		instSb.WriteString(fmt.Sprintf("  %-18s %d\n", "Server Port:", selectedInst.Port))
		instSb.WriteString(fmt.Sprintf("  %-18s %d\n", "Process PID:", selectedInst.PID))
		instSb.WriteString(fmt.Sprintf("  %-18s %s\n", "Host / Endpoint:", fmt.Sprintf("http://127.0.0.1:%d", selectedInst.Port)))
		instSb.WriteString(fmt.Sprintf("  %-18s %s\n", "Uptime:", uptimeStr))
		instSb.WriteString(fmt.Sprintf("  %-18s %s\n", "Generation Speed:", speedStr))
		instSb.WriteString(fmt.Sprintf("  %-18s %s\n", "HTTP Status:", lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render("HTTP 200 OK (Serving)")))
		instSb.WriteString(fmt.Sprintf("  %-18s %s\n", "Active Memory (RSS):", memStr))
		instSb.WriteString(fmt.Sprintf("  %-18s %s", "Requests Handled:", reqStr))
		if selectedInst.LogFile != "" {
			instSb.WriteString(fmt.Sprintf("\n  %-18s %s", "Log File Path:", TruncateVisual(selectedInst.LogFile, max(30, innerWidth-22), "...")))
		}
	}

	instanceBadge := fmt.Sprintf("Port %d [Active]", selectedInst.Port)
	instanceCard := SurfaceCardWithHeight("Live Instance Details", instSb.String(), cardWidth, hInstance, true, instanceBadge)

	// 3. Live Context Slot Utilization Bento Card
	gaugeWidth := 20
	if innerWidth > 60 {
		gaugeWidth = 24
	}

	var contextGaugeStr string
	var contextBadge string
	var slotSb strings.Builder

	slotMetrics, hasSlots := m.cachedSlots[selectedInst.Port]
	if hasSlots && slotMetrics != nil && slotMetrics.TotalNCtx > 0 {
		slotPct := slotMetrics.PctUsed
		bar := RenderGradientBar(slotPct, gaugeWidth, startHex, endHex)
		contextGaugeStr = fmt.Sprintf("[%s] %.0f%% (%d / %d tokens)", bar, slotPct, slotMetrics.TotalNPast, slotMetrics.TotalNCtx)
		contextBadge = fmt.Sprintf("%.0f%% Used", slotPct)

		if innerWidth >= 70 {
			colW := innerWidth / 2
			slotSb.WriteString(fmt.Sprintf("  %-18s %s\n", "Context Window:", contextGaugeStr))
			row2L := fmt.Sprintf("  %-18s %d / %d slots active", "Slot Allocations:", slotMetrics.ActiveSlots, slotMetrics.TotalSlots)
			row2R := fmt.Sprintf("%-18s %d tokens", "Processed (n_past):", slotMetrics.TotalNPast)
			slotSb.WriteString(fmt.Sprintf("%-*s %s\n", colW, row2L, row2R))
			slotSb.WriteString(fmt.Sprintf("  %-18s %d tokens", "Capacity (n_ctx):", slotMetrics.TotalNCtx))
		} else {
			slotSb.WriteString(fmt.Sprintf("  %-18s %s\n", "Context Window:", contextGaugeStr))
			slotSb.WriteString(fmt.Sprintf("  %-18s %d / %d slots active\n", "Slot Allocations:", slotMetrics.ActiveSlots, slotMetrics.TotalSlots))
			slotSb.WriteString(fmt.Sprintf("  %-18s %d tokens\n", "Processed (n_past):", slotMetrics.TotalNPast))
			slotSb.WriteString(fmt.Sprintf("  %-18s %d tokens", "Capacity (n_ctx):", slotMetrics.TotalNCtx))
		}
	} else {
		bar := RenderGradientBar(0, gaugeWidth, startHex, endHex)
		contextGaugeStr = fmt.Sprintf("[%s] 0%% (Idle / No slot data)", bar)
		contextBadge = "Idle"

		slotSb.WriteString(fmt.Sprintf("  %-18s %s\n", "Context Window:", contextGaugeStr))
		slotSb.WriteString(fmt.Sprintf("  %-18s %s", "Status:", "Waiting for active inference requests / metrics query..."))
	}

	contextCard := SurfaceCardWithHeight("Live Context Slot Utilization", slotSb.String(), cardWidth, hContext, false, contextBadge)

	// 4. Control Actions Card
	helpShortcuts := fmt.Sprintf("  %s Restart  %s Stop  %s Stop All  %s Stream Logs  %s Back",
		StyleHelpKey.Render("[R]"),
		StyleHelpKey.Render("[S]"),
		StyleHelpKey.Render("[Ctrl+K]"),
		StyleHelpKey.Render("[L]"),
		StyleHelpKey.Render("[Esc]"),
	)
	navShortcuts := fmt.Sprintf("  Navigation: %s Select Instance  %s Auto Refresh (1.5s)",
		StyleHelpKey.Render("[Up/Down/j/k]"),
		StyleHelpKey.Render("[Tick]"),
	)
	controlCard := SurfaceCardWithHeight("Control Actions", helpShortcuts+"\n"+navShortcuts, cardWidth, hControl, false, "")

	return summaryCard + "\n" + instanceCard + "\n" + contextCard + "\n" + controlCard
}
