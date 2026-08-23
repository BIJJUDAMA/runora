package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/runner"
)

type MonitorTickMsg struct{}

type MonitorMetricsMsg struct {
	Instances  []runner.InstanceInfo
	CachedMem  map[int]string
	CachedReqs map[int]string
}

func MonitorTickCmd() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(t time.Time) tea.Msg {
		return MonitorTickMsg{}
	})
}

type MonitorModel struct {
	srvRunner  runner.ModelRuntime
	instances  []runner.InstanceInfo
	selected   int
	cachedMem  map[int]string
	cachedReqs map[int]string
}

func NewMonitorModel(srv runner.ModelRuntime) *MonitorModel {
	return &MonitorModel{
		srvRunner:  srv,
		instances:  []runner.InstanceInfo{},
		selected:   0,
		cachedMem:  make(map[int]string),
		cachedReqs: make(map[int]string),
	}
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

func (m *MonitorModel) PollMetricsCmd() tea.Cmd {
	srv := m.srvRunner
	return func() tea.Msg {
		instances := srv.GetAllInstances()
		memMap := make(map[int]string, len(instances))
		reqsMap := make(map[int]string, len(instances))
		for _, inst := range instances {
			mem, err := runner.GetMemoryUsage(inst.PID)
			if err == nil {
				memMap[inst.PID] = fmt.Sprintf("%.2f MB", mem)
			} else {
				memMap[inst.PID] = "N/A"
			}
			reqs, err := runner.QueryServerRequests(inst.Port)
			if err == nil {
				reqsMap[inst.Port] = fmt.Sprintf("%d requests", reqs)
			} else {
				reqsMap[inst.Port] = "0 requests"
			}
		}
		return MonitorMetricsMsg{
			Instances:  instances,
			CachedMem:  memMap,
			CachedReqs: reqsMap,
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

		sb.WriteString(fmt.Sprintf("  %-20s %d\n", "Process PID:", selectedInst.PID))
		sb.WriteString(fmt.Sprintf("  %-20s %d\n", "Server Port:", selectedInst.Port))
		sb.WriteString(fmt.Sprintf("  %-20s http://127.0.0.1:%d\n", "Endpoint:", selectedInst.Port))
		sb.WriteString(fmt.Sprintf("  %-20s %s\n", "Active Memory (RSS):", memStr))
		sb.WriteString(fmt.Sprintf("  %-20s %s\n", "Requests Handled:", reqStr))
		sb.WriteString(fmt.Sprintf("  %-20s %s\n\n", "Log File Path:", selectedInst.LogFile))

		helpStr := fmt.Sprintf("%s Stop Selected Server  %s Back to Browser",
			StyleHelpKey.Render("[S]"),
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
