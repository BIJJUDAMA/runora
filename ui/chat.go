package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/chat"
	"github.com/BIJJUDAMA/runora/hardware"
	"github.com/BIJJUDAMA/runora/model"
	"github.com/BIJJUDAMA/runora/runner"
	"github.com/BIJJUDAMA/runora/ui/mouse"
)

type chatState int

const (
	chatStateIdle chatState = iota
	chatStateNoServer
	chatStateInstanceSelect
	chatStateStreaming
	chatStateCompacting
	chatStateParamOverlay
	chatStateRenaming
	chatStateDeleting
)

type ChatNavigateToLaunchMsg struct{}
type ChatNavigateToBrowserMsg struct{}

type chatTokenMsg struct {
	chunk chat.TokenChunk
}

type chatStreamDoneMsg struct {
	chunk chat.TokenChunk
}

type chatCompactDoneMsg struct {
	checkpoint *chat.CompactionCheckpoint
	err        error
}

type chatPollRunningMsg struct {
	running bool
	port    int
	model   string
}

// ChatModel is the Bubble Tea view for the integrated chat playground.
type ChatModel struct {
	service       chat.ChatService
	runtime       runner.ModelRuntime
	allModels     []*model.GGUFMetadata
	hardwareSpecs *hardware.HardwareSpecs
	sessions      []*chat.Session
	activeSession *chat.Session
	selectedSess  int

	width, height int
	sessionsFocus bool // true = left sessions list focused, false = chat/input focused

	state        chatState
	streamCancel context.CancelFunc
	streamBuf    strings.Builder
	streamChan   <-chan chat.TokenChunk
	statsLine    string

	contextUsed  int
	contextTotal int
	warnCompact  bool

	textarea         textarea.Model
	renameInput      string
	paramFocus       int // 0: Temp, 1: TopP, 2: TopK, 3: Context
	sysPromptIn      string
	instanceCursor   int
	runningInstances []runner.InstanceInfo
	toasts           *ToastManager
	mouseReg         *mouse.Registry

	chatHistoryScroll int
}

// NewChatModel initializes the chat playground component.
func NewChatModel(
	service chat.ChatService,
	rt runner.ModelRuntime,
	models []*model.GGUFMetadata,
	hw *hardware.HardwareSpecs,
	toasts *ToastManager,
	mouseReg *mouse.Registry,
) *ChatModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message... (Enter to send, Ctrl+Enter for newline)"
	ta.Focus()
	ta.CharLimit = 8192
	ta.SetWidth(60)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	m := &ChatModel{
		service:       service,
		runtime:       rt,
		allModels:     models,
		hardwareSpecs: hw,
		toasts:        toasts,
		mouseReg:      mouseReg,
		textarea:      ta,
		state:         chatStateIdle,
		contextTotal:  4096,
	}

	m.ReloadSessions()
	m.RefreshRunningInstances()
	return m
}

// getRunningGGUFInstances returns active server instances whose model is a GGUF file.
func (m *ChatModel) getRunningGGUFInstances() []runner.InstanceInfo {
	if m.runtime == nil {
		return nil
	}
	var list []runner.InstanceInfo
	for _, inst := range m.runtime.GetAllInstances() {
		ext := strings.ToLower(filepath.Ext(inst.ModelPath))
		if ext == ".gguf" {
			list = append(list, inst)
		}
	}
	return list
}

// RefreshRunningInstances updates active instance bindings according to running GGUF models.
func (m *ChatModel) RefreshRunningInstances() {
	m.runningInstances = m.getRunningGGUFInstances()

	// 0 running GGUF instances
	if len(m.runningInstances) == 0 {
		if m.state != chatStateRenaming && m.state != chatStateDeleting && m.state != chatStateParamOverlay {
			m.state = chatStateNoServer
		}
		return
	}

	// Exactly 1 running GGUF instance -> automatically bind to it
	if len(m.runningInstances) == 1 {
		inst := m.runningInstances[0]
		if m.activeSession != nil {
			if m.activeSession.Port != inst.Port || m.activeSession.ModelPath != inst.ModelPath {
				m.activeSession.Port = inst.Port
				m.activeSession.ModelPath = inst.ModelPath
				if m.service != nil {
					_ = m.service.SaveSession(m.activeSession)
				}
			}
		}
		if m.state == chatStateNoServer || m.state == chatStateInstanceSelect {
			m.state = chatStateIdle
		}
		m.updateContextMetrics()
		return
	}

	// Multiple running GGUF instances
	if m.activeSession != nil {
		found := false
		for _, inst := range m.runningInstances {
			if inst.Port == m.activeSession.Port && inst.ModelPath == m.activeSession.ModelPath {
				found = true
				break
			}
		}
		if found {
			if m.state == chatStateNoServer {
				m.state = chatStateIdle
			}
			m.updateContextMetrics()
			return
		}
	}

	// If not currently bound to any valid running instance, show selector
	if m.state == chatStateNoServer || m.state == chatStateIdle {
		m.state = chatStateInstanceSelect
		m.instanceCursor = 0
	}
}

// ReloadSessions loads all saved sessions from disk.
func (m *ChatModel) ReloadSessions() {
	if m.service == nil {
		return
	}
	list, err := m.service.ListSessions()
	if err == nil {
		m.sessions = list
		if len(m.sessions) > 0 {
			if m.activeSession == nil {
				m.activeSession = m.sessions[0]
				m.selectedSess = 0
			}
		} else {
			// Auto create first session
			port := 50505
			modelPath := ""
			running := m.getRunningGGUFInstances()
			if len(running) > 0 {
				port = running[0].Port
				modelPath = running[0].ModelPath
			}
			newSess, err := m.service.NewSession("Chat 1", modelPath, port, chat.DefaultChatParams())
			if err == nil {
				m.sessions = []*chat.Session{newSess}
				m.activeSession = newSess
				m.selectedSess = 0
			}
		}
	}
	m.updateContextMetrics()
}

func (m *ChatModel) SetModels(models []*model.GGUFMetadata) {
	m.allModels = models
}

func (m *ChatModel) updateContextMetrics() {
	if m.activeSession == nil || m.service == nil {
		return
	}
	m.contextUsed = m.service.EstimateTokens(m.activeSession)
	ctxTotal, _ := m.service.ContextSizeFor(m.activeSession.Port)
	if ctxTotal > 0 {
		m.contextTotal = ctxTotal
	} else if m.activeSession.Params.ContextSize > 0 {
		m.contextTotal = m.activeSession.Params.ContextSize
	} else {
		m.contextTotal = 4096
	}

	if m.contextTotal > 0 && float64(m.contextUsed) >= (0.85*float64(m.contextTotal)) {
		m.warnCompact = true
	} else {
		m.warnCompact = false
	}
}

// Init implements tea.Model.
func (m *ChatModel) Init() tea.Cmd {
	return textarea.Blink
}

func waitForNextToken(ch <-chan chat.TokenChunk) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return chatStreamDoneMsg{}
		}
		chunk, ok := <-ch
		if !ok || chunk.Done || chunk.Err != nil {
			return chatStreamDoneMsg{chunk: chunk}
		}
		return chatTokenMsg{chunk: chunk}
	}
}

func pollServerStatusCmd(rt runner.ModelRuntime) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(500 * time.Millisecond)
		if rt == nil {
			return chatPollRunningMsg{running: false}
		}
		status, mp, port := rt.GetStatus()
		return chatPollRunningMsg{
			running: status == runner.StatusRunning,
			port:    port,
			model:   mp,
		}
	}
}

// Update handles UI events, streaming tokens, and keybindings.
func (m *ChatModel) Update(msg tea.Msg) (*ChatModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		inputWidth := max(30, m.width-30)
		m.textarea.SetWidth(inputWidth)

	case chatTokenMsg:
		if msg.chunk.Token != "" {
			m.streamBuf.WriteString(msg.chunk.Token)
		}
		if msg.chunk.GenTokens > 0 {
			m.statsLine = fmt.Sprintf("Generating... %d tokens", msg.chunk.GenTokens)
		}
		cmds = append(cmds, waitForNextToken(m.streamChan))

	case chatStreamDoneMsg:
		m.state = chatStateIdle
		m.streamCancel = nil
		m.streamChan = nil
		if msg.chunk.Err != nil && msg.chunk.Err != context.Canceled {
			m.showToast("Generation error: "+msg.chunk.Err.Error(), ToastDanger)
		}
		if msg.chunk.TokensPerSec > 0 {
			m.statsLine = fmt.Sprintf("%.1f tok/s | %d prompt | %d gen", msg.chunk.TokensPerSec, msg.chunk.PromptTokens, msg.chunk.GenTokens)
		} else {
			m.statsLine = fmt.Sprintf("%d prompt | %d gen", msg.chunk.PromptTokens, msg.chunk.GenTokens)
		}
		m.streamBuf.Reset()
		m.ReloadSessions()
		m.updateContextMetrics()

	case chatCompactDoneMsg:
		m.state = chatStateIdle
		if msg.err != nil {
			m.showToast("Compaction failed: "+msg.err.Error(), ToastDanger)
		} else if msg.checkpoint != nil {
			m.showToast(fmt.Sprintf("Compacted %d tokens into %d tokens", msg.checkpoint.TokensBefore, msg.checkpoint.TokensAfter), ToastSuccess)
			m.ReloadSessions()
			m.updateContextMetrics()
		} else {
			m.showToast("Nothing to compact - context already compact", ToastInfo)
		}

	case chatPollRunningMsg:
		if msg.running {
			m.state = chatStateIdle
			if m.activeSession != nil {
				m.activeSession.Port = msg.port
				m.activeSession.ModelPath = msg.model
				_ = m.service.SaveSession(m.activeSession)
			}
			m.showToast("Model server ready for chat", ToastSuccess)
		} else if m.state == chatStateNoServer {
			cmds = append(cmds, pollServerStatusCmd(m.runtime))
		}

	case tea.KeyMsg:
		switch m.state {
		case chatStateStreaming:
			if msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC {
				if m.streamCancel != nil {
					m.streamCancel()
				}
				m.state = chatStateIdle
				m.showToast("Generation stopped", ToastInfo)
				return m, nil
			}

		case chatStateParamOverlay:
			switch msg.String() {
			case "esc":
				m.state = chatStateIdle
				return m, nil
			case "up", "k":
				m.paramFocus = (m.paramFocus - 1 + 4) % 4
				return m, nil
			case "down", "j":
				m.paramFocus = (m.paramFocus + 1) % 4
				return m, nil
			case "+", "=", "right", "l":
				m.adjustParam(1)
				return m, nil
			case "-", "_", "left", "h":
				m.adjustParam(-1)
				return m, nil
			}

		case chatStateRenaming:
			switch msg.Type {
			case tea.KeyEsc:
				m.state = chatStateIdle
				return m, nil
			case tea.KeyEnter:
				if m.activeSession != nil && strings.TrimSpace(m.renameInput) != "" {
					_ = m.service.RenameSession(m.activeSession.ID, strings.TrimSpace(m.renameInput))
					m.ReloadSessions()
					m.showToast("Session renamed", ToastSuccess)
				}
				m.state = chatStateIdle
				return m, nil
			case tea.KeyBackspace:
				if len(m.renameInput) > 0 {
					m.renameInput = m.renameInput[:len(m.renameInput)-1]
				}
				return m, nil
			default:
				if len(msg.Runes) > 0 {
					m.renameInput += string(msg.Runes)
				}
				return m, nil
			}

		case chatStateDeleting:
			switch strings.ToLower(msg.String()) {
			case "y", "enter":
				if m.activeSession != nil {
					_ = m.service.DeleteSession(m.activeSession.ID)
					m.activeSession = nil
					m.ReloadSessions()
					m.showToast("Session deleted", ToastSuccess)
				}
				m.state = chatStateIdle
				return m, nil
			case "n", "esc":
				m.state = chatStateIdle
				return m, nil
			}

		case chatStateNoServer:
			switch msg.String() {
			case "enter", "2":
				return m, func() tea.Msg { return ChatNavigateToLaunchMsg{} }
			case "1":
				return m, func() tea.Msg { return ChatNavigateToBrowserMsg{} }
			case "tab":
				m.sessionsFocus = !m.sessionsFocus
				if !m.sessionsFocus {
					m.textarea.Focus()
				} else {
					m.textarea.Blur()
				}
				return m, nil
			}
			if m.sessionsFocus {
				switch msg.String() {
				case "up", "k":
					if m.selectedSess > 0 {
						m.selectedSess--
						m.activeSession = m.sessions[m.selectedSess]
					}
					return m, nil
				case "down", "j":
					if m.selectedSess < len(m.sessions)-1 {
						m.selectedSess++
						m.activeSession = m.sessions[m.selectedSess]
					}
					return m, nil
				case "n", "N":
					sessName := fmt.Sprintf("Chat %d", len(m.sessions)+1)
					newSess, err := m.service.NewSession(sessName, "", 50505, chat.DefaultChatParams())
					if err == nil {
						m.activeSession = newSess
						m.ReloadSessions()
						for idx, s := range m.sessions {
							if s.ID == newSess.ID {
								m.selectedSess = idx
								break
							}
						}
						m.showToast("New session created", ToastSuccess)
					}
					return m, nil
				case "d", "D":
					if m.activeSession != nil {
						m.state = chatStateDeleting
					}
					return m, nil
				case "r", "R":
					if m.activeSession != nil {
						m.renameInput = m.activeSession.Name
						m.state = chatStateRenaming
					}
					return m, nil
				}
			}

		case chatStateInstanceSelect:
			switch msg.String() {
			case "esc":
				if m.activeSession != nil && m.activeSession.Port > 0 {
					m.state = chatStateIdle
				}
				return m, nil
			case "up", "k":
				if m.instanceCursor > 0 {
					m.instanceCursor--
				}
				return m, nil
			case "down", "j":
				if m.instanceCursor < len(m.runningInstances)-1 {
					m.instanceCursor++
				}
				return m, nil
			case "enter":
				if len(m.runningInstances) > 0 && m.instanceCursor < len(m.runningInstances) {
					target := m.runningInstances[m.instanceCursor]
					if m.activeSession != nil {
						m.activeSession.Port = target.Port
						m.activeSession.ModelPath = target.ModelPath
						if m.service != nil {
							_ = m.service.SaveSession(m.activeSession)
						}
					}
					m.state = chatStateIdle
					m.updateContextMetrics()
					m.showToast(fmt.Sprintf("Connected to %s (Port %d)", filepath.Base(target.ModelPath), target.Port), ToastSuccess)
				}
				return m, nil
			}

		case chatStateIdle:
			// Tab toggles panel focus
			if msg.Type == tea.KeyTab {
				m.sessionsFocus = !m.sessionsFocus
				if !m.sessionsFocus {
					m.textarea.Focus()
				} else {
					m.textarea.Blur()
				}
				return m, nil
			}

			if m.sessionsFocus {
				switch msg.String() {
				case "up", "k":
					if m.selectedSess > 0 {
						m.selectedSess--
						m.activeSession = m.sessions[m.selectedSess]
						m.updateContextMetrics()
					}
					return m, nil
				case "down", "j":
					if m.selectedSess < len(m.sessions)-1 {
						m.selectedSess++
						m.activeSession = m.sessions[m.selectedSess]
						m.updateContextMetrics()
					}
					return m, nil
				case "n", "N":
					port := 50505
					mp := ""
					running := m.getRunningGGUFInstances()
					if len(running) > 0 {
						port = running[0].Port
						mp = running[0].ModelPath
					}
					sessName := fmt.Sprintf("Chat %d", len(m.sessions)+1)
					newSess, err := m.service.NewSession(sessName, mp, port, chat.DefaultChatParams())
					if err == nil {
						m.activeSession = newSess
						m.ReloadSessions()
						for idx, s := range m.sessions {
							if s.ID == newSess.ID {
								m.selectedSess = idx
								break
							}
						}
						m.showToast("New session created", ToastSuccess)
					}
					return m, nil
				case "d", "D":
					if m.activeSession != nil {
						m.state = chatStateDeleting
					}
					return m, nil
				case "r", "R":
					if m.activeSession != nil {
						m.renameInput = m.activeSession.Name
						m.state = chatStateRenaming
					}
					return m, nil
				}
			} else {
				// Chat View / Input focused
				switch msg.String() {
				case "ctrl+enter":
					m.textarea.InsertString("\n")
					return m, nil
				case "enter":
					text := strings.TrimSpace(m.textarea.Value())
					if text != "" {
						m.textarea.Reset()
						return m, m.sendMessageCmd(text)
					}
					return m, nil
				case "m", "M":
					if msg.Type == tea.KeyRunes && len(m.textarea.Value()) == 0 {
						instances := m.getRunningGGUFInstances()
						if len(instances) > 1 {
							m.runningInstances = instances
							m.state = chatStateInstanceSelect
							m.instanceCursor = 0
							return m, nil
						} else if len(instances) == 1 {
							m.showToast(fmt.Sprintf("Using active model: %s (Port %d)", filepath.Base(instances[0].ModelPath), instances[0].Port), ToastInfo)
							return m, nil
						} else {
							m.state = chatStateNoServer
							return m, nil
						}
					}
				case "k", "K":
					if msg.Type == tea.KeyRunes && len(m.textarea.Value()) == 0 {
						return m, m.triggerCompactCmd()
					}
				case "p", "P":
					if msg.Type == tea.KeyRunes && len(m.textarea.Value()) == 0 {
						m.state = chatStateParamOverlay
						if m.activeSession != nil {
							m.sysPromptIn = m.activeSession.Params.SystemPrompt
						}
						return m, nil
					}
				case "c", "C":
					if msg.Type == tea.KeyRunes && len(m.textarea.Value()) == 0 {
						m.copyLastAssistantMessage()
						return m, nil
					}
				case "pgup":
					if m.chatHistoryScroll > 0 {
						m.chatHistoryScroll -= 5
						if m.chatHistoryScroll < 0 {
							m.chatHistoryScroll = 0
						}
					}
					return m, nil
				case "pgdown":
					m.chatHistoryScroll += 5
					return m, nil
				}

				// Forward typing to textarea
				var taCmd tea.Cmd
				m.textarea, taCmd = m.textarea.Update(msg)
				cmds = append(cmds, taCmd)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *ChatModel) adjustParam(dir int) {
	if m.activeSession == nil {
		return
	}
	switch m.paramFocus {
	case 0: // Temperature
		m.activeSession.Params.Temperature += float64(dir) * 0.05
		if m.activeSession.Params.Temperature < 0.0 {
			m.activeSession.Params.Temperature = 0.0
		}
		if m.activeSession.Params.Temperature > 2.0 {
			m.activeSession.Params.Temperature = 2.0
		}
	case 1: // TopP
		m.activeSession.Params.TopP += float64(dir) * 0.05
		if m.activeSession.Params.TopP < 0.0 {
			m.activeSession.Params.TopP = 0.0
		}
		if m.activeSession.Params.TopP > 1.0 {
			m.activeSession.Params.TopP = 1.0
		}
	case 2: // TopK
		m.activeSession.Params.TopK += dir * 5
		if m.activeSession.Params.TopK < 1 {
			m.activeSession.Params.TopK = 1
		}
		if m.activeSession.Params.TopK > 200 {
			m.activeSession.Params.TopK = 200
		}
	case 3: // ContextSize
		m.activeSession.Params.ContextSize += dir * 512
		if m.activeSession.Params.ContextSize < 512 {
			m.activeSession.Params.ContextSize = 512
		}
		if m.activeSession.Params.ContextSize > 131072 {
			m.activeSession.Params.ContextSize = 131072
		}
	}
	_ = m.service.SaveSession(m.activeSession)
}

func (m *ChatModel) showToast(message string, toastType ToastType) tea.Cmd {
	if m.toasts != nil {
		return m.toasts.Add(message, toastType, 2500*time.Millisecond)
	}
	return nil
}

func (m *ChatModel) copyLastAssistantMessage() {
	if m.activeSession == nil || len(m.activeSession.Messages) == 0 {
		return
	}
	for i := len(m.activeSession.Messages) - 1; i >= 0; i-- {
		msg := m.activeSession.Messages[i]
		if msg.Role == "assistant" && strings.TrimSpace(msg.Content) != "" {
			_ = clipboard.WriteAll(msg.Content)
			m.showToast("Copied assistant message to clipboard", ToastSuccess)
			return
		}
	}
}

func (m *ChatModel) triggerCompactCmd() tea.Cmd {
	return func() tea.Msg {
		if m.activeSession == nil || m.service == nil {
			return chatCompactDoneMsg{err: fmt.Errorf("no active session")}
		}
		cp, err := m.service.Compact(context.Background(), m.activeSession)
		return chatCompactDoneMsg{checkpoint: cp, err: err}
	}
}

func (m *ChatModel) sendMessageCmd(text string) tea.Cmd {
	return func() tea.Msg {
		if m.activeSession == nil {
			return chatStreamDoneMsg{chunk: chat.TokenChunk{Err: fmt.Errorf("no active session")}}
		}

		instances := m.getRunningGGUFInstances()
		if len(instances) == 0 {
			m.state = chatStateNoServer
			return chatStreamDoneMsg{chunk: chat.TokenChunk{Err: fmt.Errorf("no GGUF model server running - launch a model in the Launch tab first")}}
		}

		if len(instances) == 1 {
			m.activeSession.Port = instances[0].Port
			m.activeSession.ModelPath = instances[0].ModelPath
		} else {
			// Multiple running instances: check if activeSession matches one
			found := false
			for _, inst := range instances {
				if inst.Port == m.activeSession.Port && inst.ModelPath == m.activeSession.ModelPath {
					found = true
					break
				}
			}
			if !found {
				m.runningInstances = instances
				m.state = chatStateInstanceSelect
				m.instanceCursor = 0
				return chatStreamDoneMsg{chunk: chat.TokenChunk{Err: fmt.Errorf("multiple models running - please select which model to use")}}
			}
		}

		// Auto-compact before send if above 85% threshold
		if m.warnCompact {
			_, _ = m.service.Compact(context.Background(), m.activeSession)
		}

		ctx, cancel := context.WithCancel(context.Background())
		m.streamCancel = cancel
		m.state = chatStateStreaming
		m.streamBuf.Reset()

		stream, err := m.service.Stream(ctx, m.activeSession, text)
		if err != nil {
			m.state = chatStateIdle
			return chatStreamDoneMsg{chunk: chat.TokenChunk{Err: err}}
		}

		m.streamChan = stream
		return waitForNextToken(stream)()
	}
}

// View renders the dual-column Bento chat interface.
func (m *ChatModel) View(width, height int) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	m.width = width
	m.height = height

	cardHeight := max(16, height-3)
	leftWidth := max(22, int(float64(width)*0.22))
	rightWidth := max(40, width-leftWidth-2)

	leftCard := m.renderSessionsCard(leftWidth, cardHeight)
	rightCard := m.renderChatCard(rightWidth, cardHeight)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftCard, rightCard)
}

func (m *ChatModel) renderSessionsCard(width, height int) string {
	var b strings.Builder

	if len(m.sessions) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("No chat sessions."))
	} else {
		for idx, s := range m.sessions {
			name := s.Name
			if len(name) > width-6 {
				name = name[:width-9] + "..."
			}

			if idx == m.selectedSess {
				marker := "> "
				style := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
				b.WriteString(style.Render(fmt.Sprintf("%s%s\n", marker, name)))
			} else {
				style := lipgloss.NewStyle().Foreground(ColorText)
				b.WriteString(style.Render(fmt.Sprintf("  %s\n", name)))
			}
		}
	}

	// Action shortcuts footer inside card
	actions := lipgloss.NewStyle().Foreground(ColorTextMuted).Render("[N] New | [D] Del | [R] Rename")
	content := lipgloss.JoinVertical(lipgloss.Left, b.String(), "\n", actions)

	badge := fmt.Sprintf("%d Chats", len(m.sessions))
	return SurfaceCardWithHeight("Sessions", content, width, height, m.sessionsFocus, badge)
}

func (m *ChatModel) renderChatCard(width, height int) string {
	if m.state == chatStateNoServer {
		return m.renderNoServerCard(width, height)
	}
	if m.state == chatStateInstanceSelect {
		return m.renderInstanceSelectCard(width, height)
	}
	if m.state == chatStateParamOverlay {
		return m.renderParametersOverlay(width, height)
	}

	var histBuilder strings.Builder

	if m.activeSession != nil {
		// Render checkpoints summary separators
		for _, cp := range m.activeSession.Checkpoints {
			sepText := fmt.Sprintf("--- [COMPACTED: %d turns | %d -> %d tokens | %s] ---",
				cp.CoveredRange[1]-cp.CoveredRange[0]+1,
				cp.TokensBefore,
				cp.TokensAfter,
				cp.CreatedAt.Format("15:04:05"),
			)
			sep := lipgloss.NewStyle().Foreground(ColorWarning).Italic(true).Render(sepText)
			histBuilder.WriteString(sep + "\n\n")
		}

		// Render message turns
		for _, msg := range m.activeSession.Messages {
			if msg.Role == "user" {
				prefix := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("USER: ")
				histBuilder.WriteString(prefix + msg.Content + "\n\n")
			} else if msg.Role == "assistant" {
				prefix := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render("ASSISTANT: ")
				content := msg.Content
				if msg.Partial {
					content += " [stopped]"
				}
				histBuilder.WriteString(prefix + content + "\n\n")
			}
		}
	}

	// Real-time streaming buffer
	if m.state == chatStateStreaming && m.streamBuf.Len() > 0 {
		prefix := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render("ASSISTANT: ")
		streamContent := m.streamBuf.String() + " █"
		histBuilder.WriteString(prefix + streamContent + "\n\n")
	}

	// Compaction warning banner if >= 85% context pressure
	var warningBanner string
	if m.warnCompact {
		pct := 0
		if m.contextTotal > 0 {
			pct = (m.contextUsed * 100) / m.contextTotal
		}
		warnStyle := lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
		warningBanner = warnStyle.Render(fmt.Sprintf("Context pressure at %d%% (%d/%d tokens) - Press [K] to compact history now\n", pct, m.contextUsed, m.contextTotal))
	}

	// Rename prompt modal
	if m.state == chatStateRenaming {
		renamePrompt := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render("Rename Session: ") + m.renameInput + " █"
		warningBanner += renamePrompt + "\n"
	}

	// Delete confirmation modal
	if m.state == chatStateDeleting {
		delPrompt := lipgloss.NewStyle().Foreground(ColorDanger).Bold(true).Render("Delete this session? [Y/N]")
		warningBanner += delPrompt + "\n"
	}

	// Bottom status metrics line
	pct := 0
	if m.contextTotal > 0 {
		pct = (m.contextUsed * 100) / m.contextTotal
	}
	modelName := "None"
	port := 50505
	if m.activeSession != nil {
		port = m.activeSession.Port
		if m.activeSession.ModelPath != "" {
			modelName = filepath.Base(m.activeSession.ModelPath)
		}
	}
	stats := m.statsLine
	if stats == "" {
		stats = "Ready"
	}

	metricsLine := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		fmt.Sprintf("Model: %s | Port: %d | Context: %d/%d (%d%%) | %s",
			modelName, port, m.contextUsed, m.contextTotal, pct, stats,
		),
	)

	// Action hints
	hints := lipgloss.NewStyle().Foreground(ColorTextMuted).Render(
		"[Enter] Send | [Ctrl+Enter] Newline | [M] Switch Model | [K] Compact | [P] Params | [C] Copy | [Tab] Switch Pane",
	)

	inputArea := lipgloss.JoinVertical(lipgloss.Left,
		warningBanner,
		m.textarea.View(),
		hints,
		metricsLine,
	)

	fullContent := lipgloss.JoinVertical(lipgloss.Left,
		histBuilder.String(),
		"\n",
		inputArea,
	)

	title := "Chat Playground"
	if m.activeSession != nil {
		title = m.activeSession.Name
	}
	badge := "Interactive"
	if m.state == chatStateStreaming {
		badge = "Streaming..."
	}

	return SurfaceCardWithHeight(title, fullContent, width, height, !m.sessionsFocus, badge)
}

func (m *ChatModel) renderNoServerCard(width, height int) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(ColorWarning).Bold(true).Render("No GGUF Server Running\n\n"))
	b.WriteString("Chat Playground connects directly to an active local GGUF model server.\n\n")
	b.WriteString("Currently, no GGUF server instance is running.\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(ColorSecondary).Render("-> Go to the [2] Launch tab to configure an execution profile and launch a model.\n\n"))
	b.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("Note: Only GGUF models are supported for interactive chat.\n\n"))
	b.WriteString(lipgloss.NewStyle().Foreground(ColorTextMuted).Render("[Enter] / [2] Go to Launch Tab | [1] Browse Models | [Tab] Switch Pane"))
	return SurfaceCardWithHeight("Chat Playground", b.String(), width, height, !m.sessionsFocus, "No Active Model")
}

func (m *ChatModel) renderInstanceSelectCard(width, height int) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render("Multiple GGUF Servers Running\n\n"))
	b.WriteString("Select which active model instance to chat with:\n\n")

	for i, inst := range m.runningInstances {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(ColorText)
		if i == m.instanceCursor {
			prefix = "> "
			style = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
		}
		modelName := filepath.Base(inst.ModelPath)
		uptimeStr := inst.Uptime.Round(time.Second).String()
		b.WriteString(style.Render(fmt.Sprintf("%s%s (Port: %d, PID: %d, Uptime: %s)\n", prefix, modelName, inst.Port, inst.PID, uptimeStr)))
	}

	b.WriteString("\n" + lipgloss.NewStyle().Foreground(ColorTextMuted).Render("[Enter] Connect to Model | [Esc] Cancel | [Up/Down] Navigate"))
	return SurfaceCardWithHeight("Select Model Server", b.String(), width, height, true, "Select Instance")
}

func (m *ChatModel) renderParametersOverlay(width, height int) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render("Inference Generation Parameters\n\n"))

	params := chat.DefaultChatParams()
	if m.activeSession != nil {
		params = m.activeSession.Params
	}

	fields := []struct {
		label string
		val   string
	}{
		{"Temperature", fmt.Sprintf("%.2f  ([+] / [-])", params.Temperature)},
		{"Top-P", fmt.Sprintf("%.2f  ([+] / [-])", params.TopP)},
		{"Top-K", fmt.Sprintf("%d  ([+] / [-])", params.TopK)},
		{"Context Size", fmt.Sprintf("%d tokens  ([+] / [-])", params.ContextSize)},
	}

	for i, f := range fields {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(ColorText)
		if i == m.paramFocus {
			prefix = "> "
			style = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
		}
		b.WriteString(style.Render(fmt.Sprintf("%s%-16s: %s\n", prefix, f.label, f.val)))
	}

	b.WriteString("\n" + lipgloss.NewStyle().Foreground(ColorTextMuted).Render("[Up/Down] Navigate | [+/-] Adjust Value | [Esc] Close"))
	return SurfaceCardWithHeight("Parameters", b.String(), width, height, true, "Settings")
}
