package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/runner"
)

type LogStreamTickMsg struct{}

type LogStreamDataMsg struct {
	Port      int
	Lines     []string
	NewOffset int64
}

type OpenLogStreamerMsg struct {
	Port       int
	PrevScreen ScreenMode
}

func logStreamTickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return LogStreamTickMsg{}
	})
}

// LogStreamerModel presents a live terminal modal streaming stdout/stderr logs from supervised instances.
type LogStreamerModel struct {
	srvRunner    runner.ModelRuntime
	prevScreen   ScreenMode
	instances    []runner.InstanceInfo
	activeIdx    int
	currentPort  int
	logFilePath  string
	allLines     []string
	fileOffset   int64
	paused       bool
	autoScroll   bool
	scrollOffset int
	searchActive bool
	searchInput  textinput.Model
	searchQuery  string
	closed       bool
	width        int
	height       int
}

func NewLogStreamerModel(srv runner.ModelRuntime, prevScreen ScreenMode, targetPort int) *LogStreamerModel {
	ti := textinput.New()
	ti.Placeholder = "Search / filter logs..."
	ti.CharLimit = 100
	ti.Width = 30

	instances := srv.GetAllInstances()
	activeIdx := 0
	currentPort := 0
	logPath := ""

	if len(instances) > 0 {
		for i, inst := range instances {
			if targetPort > 0 && inst.Port == targetPort {
				activeIdx = i
				break
			}
		}
		currentPort = instances[activeIdx].Port
		logPath = instances[activeIdx].LogFile
	} else if targetPort > 0 {
		currentPort = targetPort
		logPath = filepath.Join("cache", fmt.Sprintf("server-%d.log", targetPort))
	} else {
		currentPort = 50505
		logPath = filepath.Join("cache", "llama-server-50505.log")
	}

	m := &LogStreamerModel{
		srvRunner:    srv,
		prevScreen:   prevScreen,
		instances:    instances,
		activeIdx:    activeIdx,
		currentPort:  currentPort,
		logFilePath:  logPath,
		allLines:     []string{},
		fileOffset:   0,
		paused:       false,
		autoScroll:   true,
		scrollOffset: 0,
		searchActive: false,
		searchInput:  ti,
		searchQuery:  "",
		closed:       false,
	}

	m.loadInitialLog()
	return m
}

func (m *LogStreamerModel) Closed() bool {
	return m.closed
}

func (m *LogStreamerModel) PrevScreen() ScreenMode {
	return m.prevScreen
}

func (m *LogStreamerModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
}

func (m *LogStreamerModel) switchInstance(newIdx int) {
	if newIdx < 0 || newIdx >= len(m.instances) {
		return
	}
	m.activeIdx = newIdx
	m.currentPort = m.instances[newIdx].Port
	m.logFilePath = m.instances[newIdx].LogFile
	m.allLines = nil
	m.fileOffset = 0
	m.scrollOffset = 0
	m.autoScroll = true
	m.loadInitialLog()
}

func (m *LogStreamerModel) loadInitialLog() {
	if m.logFilePath == "" {
		return
	}
	lines, newOffset, _ := readNewLogLines(m.logFilePath, 0, true)
	m.allLines = lines
	m.fileOffset = newOffset
	if m.autoScroll {
		m.scrollToBottom()
	}
}

func (m *LogStreamerModel) readLogFileCmd() tea.Cmd {
	filePath := m.logFilePath
	offset := m.fileOffset
	port := m.currentPort
	return func() tea.Msg {
		lines, newOffset, _ := readNewLogLines(filePath, offset, false)
		return LogStreamDataMsg{
			Port:      port,
			Lines:     lines,
			NewOffset: newOffset,
		}
	}
}

func readNewLogLines(filePath string, fromOffset int64, isInitial bool) ([]string, int64, error) {
	if filePath == "" {
		return nil, fromOffset, nil
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fromOffset, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fromOffset, err
	}

	fileSize := fi.Size()
	if fileSize < fromOffset {
		// Log file was rotated or truncated
		fromOffset = 0
	}

	if isInitial && fromOffset == 0 && fileSize > 256*1024 {
		// For very large log files, seek to the last 128KB to avoid reading massive history synchronously
		fromOffset = fileSize - 128*1024
		_, _ = f.Seek(fromOffset, io.SeekStart)
		reader := bufio.NewReader(f)
		_, _ = reader.ReadString('\n')
		fromOffset, _ = f.Seek(0, io.SeekCurrent)
	}

	if fileSize == fromOffset {
		return nil, fromOffset, nil
	}

	_, err = f.Seek(fromOffset, io.SeekStart)
	if err != nil {
		return nil, fromOffset, err
	}

	bytesToRead := fileSize - fromOffset
	buf := make([]byte, bytesToRead)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fromOffset, err
	}

	newOffset := fromOffset + int64(n)
	content := string(buf[:n])
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	rawLines := strings.Split(content, "\n")

	var lines []string
	for _, l := range rawLines {
		if l != "" {
			lines = append(lines, l)
		}
	}

	return lines, newOffset, nil
}

func (m *LogStreamerModel) scrollToBottom() {
	visibleCount := m.getVisibleLinesCount()
	viewportHeight := m.getViewportHeight()
	if visibleCount > viewportHeight {
		m.scrollOffset = visibleCount - viewportHeight
	} else {
		m.scrollOffset = 0
	}
}

func (m *LogStreamerModel) getViewportHeight() int {
	h := m.height - 10
	if h < 5 {
		return 5
	}
	return h
}

func (m *LogStreamerModel) getFilteredLines() []string {
	if m.searchQuery == "" {
		return m.allLines
	}
	query := strings.ToLower(m.searchQuery)
	var filtered []string
	for _, line := range m.allLines {
		if strings.Contains(strings.ToLower(line), query) {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

func (m *LogStreamerModel) getVisibleLinesCount() int {
	return len(m.getFilteredLines())
}

func (m *LogStreamerModel) Init() tea.Cmd {
	return tea.Batch(m.readLogFileCmd(), logStreamTickCmd())
}

func (m *LogStreamerModel) Update(msg tea.Msg) (*LogStreamerModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.autoScroll {
			m.scrollToBottom()
		}

	case LogStreamTickMsg:
		if m.closed {
			return m, nil
		}
		// Refresh running instances snapshot
		m.instances = m.srvRunner.GetAllInstances()
		if len(m.instances) > 0 {
			if m.activeIdx >= len(m.instances) {
				m.activeIdx = 0
			}
			if m.currentPort == 0 || m.logFilePath == "" {
				m.currentPort = m.instances[m.activeIdx].Port
				m.logFilePath = m.instances[m.activeIdx].LogFile
			}
		}

		if !m.paused && m.logFilePath != "" {
			cmds = append(cmds, m.readLogFileCmd())
		}
		cmds = append(cmds, logStreamTickCmd())

	case LogStreamDataMsg:
		if msg.Port == m.currentPort && len(msg.Lines) > 0 {
			m.fileOffset = msg.NewOffset
			m.allLines = append(m.allLines, msg.Lines...)
			// Keep buffer from growing beyond 20,000 lines
			if len(m.allLines) > 20000 {
				m.allLines = m.allLines[len(m.allLines)-15000:]
			}
			if m.autoScroll {
				m.scrollToBottom()
			}
		}

	case tea.KeyMsg:
		if m.searchActive {
			switch msg.String() {
			case "enter":
				m.searchActive = false
				m.searchQuery = strings.TrimSpace(m.searchInput.Value())
				m.searchInput.Blur()
				if m.autoScroll {
					m.scrollToBottom()
				}
			case "esc":
				m.searchActive = false
				m.searchQuery = ""
				m.searchInput.SetValue("")
				m.searchInput.Blur()
				if m.autoScroll {
					m.scrollToBottom()
				}
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				m.searchQuery = strings.TrimSpace(m.searchInput.Value())
				if m.autoScroll {
					m.scrollToBottom()
				}
			}
			return m, tea.Batch(cmds...)
		}

		switch msg.String() {
		case "esc", "q", "Q":
			if m.searchQuery != "" {
				m.searchQuery = ""
				m.searchInput.SetValue("")
			} else {
				m.closed = true
			}

		case "/":
			m.searchActive = true
			m.searchInput.Focus()
			m.searchInput.SetValue(m.searchQuery)

		case "c", "C":
			if m.searchQuery != "" {
				m.searchQuery = ""
				m.searchInput.SetValue("")
			} else {
				m.allLines = nil
				m.scrollOffset = 0
			}

		case " ":
			m.paused = !m.paused

		case "up", "k":
			m.autoScroll = false
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}

		case "down", "j":
			maxScroll := m.getVisibleLinesCount() - m.getViewportHeight()
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scrollOffset < maxScroll {
				m.scrollOffset++
			}
			if m.scrollOffset >= maxScroll {
				m.autoScroll = true
			}

		case "pgup":
			m.autoScroll = false
			m.scrollOffset -= 15
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}

		case "pgdown":
			maxScroll := m.getVisibleLinesCount() - m.getViewportHeight()
			if maxScroll < 0 {
				maxScroll = 0
			}
			m.scrollOffset += 15
			if m.scrollOffset >= maxScroll {
				m.scrollOffset = maxScroll
				m.autoScroll = true
			}

		case "home", "g":
			m.autoScroll = false
			m.scrollOffset = 0

		case "end", "G":
			m.autoScroll = true
			m.scrollToBottom()

		case "tab", "right", "l":
			if len(m.instances) > 1 {
				m.switchInstance((m.activeIdx + 1) % len(m.instances))
			}

		case "shift+tab", "left", "h":
			if len(m.instances) > 1 {
				m.switchInstance((m.activeIdx - 1 + len(m.instances)) % len(m.instances))
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *LogStreamerModel) View(width, height int) string {
	m.width = width
	m.height = height

	boxWidth := width - 4
	if boxWidth < 60 {
		boxWidth = 60
	}
	contentWidth := boxWidth - 4
	if contentWidth < 40 {
		contentWidth = 40
	}

	var sb strings.Builder
	sb.WriteString("\n")

	// Header line
	titleStr := RenderGradient("RUNORA — LIVE PROCESS LOG STREAMER", ThemeGradientStart, ThemeGradientEnd)
	var statusBadge string
	if m.paused {
		statusBadge = StyleBadgeStarting.Render(" ❚❚ PAUSED ")
	} else {
		statusBadge = StyleBadgeRunning.Render(" ● STREAMING ")
	}

	sb.WriteString(fmt.Sprintf("  %s    %s\n", titleStr, statusBadge))

	// Instance Tabs Bar
	if len(m.instances) > 0 {
		sb.WriteString("  ")
		for i, inst := range m.instances {
			modelName := filepath.Base(inst.ModelPath)
			modelName = TruncateVisual(modelName, 18, "...")
			tabText := fmt.Sprintf(" Port %d: %s (PID %d) ", inst.Port, modelName, inst.PID)
			if i == m.activeIdx {
				sb.WriteString(lipgloss.NewStyle().Background(ColorPrimary).Foreground(ColorTextOnAccent).Bold(true).Render(tabText) + " ")
			} else {
				sb.WriteString(lipgloss.NewStyle().Background(ColorDim).Foreground(ColorTextMuted).Render(tabText) + " ")
			}
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString(fmt.Sprintf("  %s %s\n",
			lipgloss.NewStyle().Foreground(ColorTextDim).Render("Log File:"),
			lipgloss.NewStyle().Foreground(ColorTextMuted).Render(m.logFilePath),
		))
	}

	// Filter / Search Status line
	if m.searchActive {
		sb.WriteString(fmt.Sprintf("  %s %s\n",
			lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("Search:"),
			m.searchInput.View(),
		))
	} else if m.searchQuery != "" {
		filteredCount := m.getVisibleLinesCount()
		sb.WriteString(fmt.Sprintf("  %s %s %s\n",
			lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("Filter:"),
			lipgloss.NewStyle().Foreground(ColorGold).Render(fmt.Sprintf("%q", m.searchQuery)),
			lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("(%d matching lines)", filteredCount)),
		))
	}

	divWidth := contentWidth
	sb.WriteString("  " + strings.Repeat("─", divWidth) + "\n")

	// Viewport lines
	visibleLines := m.getFilteredLines()
	viewportHeight := m.getViewportHeight()

	if len(visibleLines) == 0 {
		if len(m.allLines) == 0 {
			sb.WriteString("\n  " + lipgloss.NewStyle().Foreground(ColorMuted).Italic(true).Render("Waiting for live log stream output from process...") + "\n\n")
		} else {
			sb.WriteString(fmt.Sprintf("\n  %s\n\n", lipgloss.NewStyle().Foreground(ColorMuted).Render("No log lines match the active search query.")))
		}
		for i := 0; i < viewportHeight-3; i++ {
			sb.WriteString("\n")
		}
	} else {
		maxScroll := len(visibleLines) - viewportHeight
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.scrollOffset > maxScroll {
			m.scrollOffset = maxScroll
		}
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}

		endIdx := m.scrollOffset + viewportHeight
		if endIdx > len(visibleLines) {
			endIdx = len(visibleLines)
		}

		for idx := m.scrollOffset; idx < endIdx; idx++ {
			line := visibleLines[idx]
			formattedLine := formatLogLine(line, m.searchQuery, contentWidth-8)
			lineNum := fmt.Sprintf("%4d │ ", idx+1)
			sb.WriteString(lipgloss.NewStyle().Foreground(ColorTextDim).Render("  "+lineNum) + formattedLine + "\n")
		}

		// Pad remaining height if lines fewer than viewport
		linesRendered := endIdx - m.scrollOffset
		for i := linesRendered; i < viewportHeight; i++ {
			sb.WriteString("\n")
		}
	}

	sb.WriteString("  " + strings.Repeat("─", divWidth) + "\n")

	// Footer Help Bar
	scrollStatus := "Auto-Scroll: ON"
	if !m.autoScroll {
		scrollStatus = "Auto-Scroll: OFF (Scrolled)"
	}
	helpLeft := fmt.Sprintf("%s Pause/Resume  %s Filter  %s Clear  %s Switch Server  %s Close",
		StyleHelpKey.Render("[Space]"),
		StyleHelpKey.Render("[/]"),
		StyleHelpKey.Render("[C]"),
		StyleHelpKey.Render("[Tab]"),
		StyleHelpKey.Render("[Esc]"),
	)
	helpRight := lipgloss.NewStyle().Foreground(ColorMuted).Render(scrollStatus)

	sb.WriteString(fmt.Sprintf("  %-*s %s\n", divWidth-len(scrollStatus)-2, helpLeft, helpRight))

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorPrimary).
		Padding(0, 1).
		Width(boxWidth).
		Render(sb.String())
}

func formatLogLine(line string, query string, maxLen int) string {
	line = strings.TrimRight(line, "\r\n")
	line = TruncateVisual(line, maxLen, "…")

	lower := strings.ToLower(line)
	var baseStyle lipgloss.Style

	switch {
	case strings.Contains(lower, "error") || strings.Contains(lower, "err:") || strings.Contains(lower, "failed") || strings.Contains(lower, "fatal") || strings.Contains(lower, "panic") || strings.Contains(lower, "exception"):
		baseStyle = lipgloss.NewStyle().Foreground(ColorDanger).Bold(true)
	case strings.Contains(lower, "warn") || strings.Contains(lower, "warning") || strings.Contains(lower, "deprecated"):
		baseStyle = lipgloss.NewStyle().Foreground(ColorGold)
	case strings.Contains(lower, "listening") || strings.Contains(lower, "ready") || strings.Contains(lower, "loaded model") || strings.Contains(lower, "http://") || strings.Contains(lower, "all slots are idle"):
		baseStyle = lipgloss.NewStyle().Foreground(ColorSecondary)
	case strings.Contains(lower, "debug") || strings.Contains(lower, "trace"):
		baseStyle = lipgloss.NewStyle().Foreground(ColorTextDim)
	default:
		baseStyle = lipgloss.NewStyle().Foreground(ColorText)
	}

	if query == "" {
		return baseStyle.Render(line)
	}

	// Highlight matching search terms
	qLower := strings.ToLower(query)
	startIdx := strings.Index(lower, qLower)
	if startIdx < 0 {
		return baseStyle.Render(line)
	}

	matchEnd := startIdx + len(query)
	if matchEnd > len(line) {
		matchEnd = len(line)
	}

	before := line[:startIdx]
	matched := line[startIdx:matchEnd]
	after := line[matchEnd:]

	highlightStyle := lipgloss.NewStyle().Background(ColorPrimary).Foreground(ColorTextOnAccent).Bold(true)
	return baseStyle.Render(before) + highlightStyle.Render(matched) + baseStyle.Render(after)
}
