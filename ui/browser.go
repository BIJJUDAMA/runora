package ui

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/benchmark"
	"github.com/BIJJUDAMA/runora/config"
	"github.com/BIJJUDAMA/runora/hardware"
	"github.com/BIJJUDAMA/runora/model"
	"github.com/BIJJUDAMA/runora/profile"
	"github.com/BIJJUDAMA/runora/runner"
)

type ServerUIStatus int

const (
	UIStatusStopped ServerUIStatus = iota
	UIStatusStarting
	UIStatusRunning
	UIStatusFailed
)

type ScreenMode int

const (
	ScreenBrowser ScreenMode = iota
	ScreenDashboard
	ScreenBenchmarkProgress
	ScreenPerformanceDashboard
	ScreenServerMonitor
	ScreenSettings
	ScreenDownloader
	ScreenProfileCreator
)

type OnboardingStep int

const (
	StepWelcome OnboardingStep = iota
	StepModelSidebar
	StepDetailsPanel
	StepLaunchDashboard
	StepDownloadLifecycle
	StepHFToken
	StepFinished
)

type SidebarItemType int

const (
	ItemSectionHeader SidebarItemType = iota
	ItemFolderHeader
	ItemModelEntry
)

type SidebarItem struct {
	Type           SidebarItemType
	Label          string
	ModelIdx       int
	ModelPath      string
}

type BrowserModel struct {
	config              *config.Config
	srvRunner           runner.ModelRuntime
	models              []*model.GGUFMetadata
	filtered            []int // indices in m.models
	selected            int   // index in m.sidebarItems
	scrollOffset        int
	loading             bool
	err                 error
	searchActive        bool
	searchInput         textinput.Model
	width, height       int
	sidebarItems        []SidebarItem
	selectedModelIdx    int // tracks which model is highlighted, independent of details toggle
	runningModelPath    string
	hardwareSpecs       *hardware.HardwareSpecs
	profiles            []*profile.Profile
	serverUIStatus      ServerUIStatus
	serverErr           error
	screenMode          ScreenMode
	dashboard           *DashboardModel
	perfDashboard       *PerformanceDashboardModel
	benchmarkProgress   *BenchmarkProgressModel
	monitorModel        *MonitorModel
	downloaderModel     *DownloaderModel
	downloadQueue       *model.DownloadQueue
	lifecycleModel      *LifecycleModel
	profileCreatorModel   *ProfileCreatorModel
	onboardingActive      bool
	onboardingStep        OnboardingStep
	onboardingTokenInput  textinput.Model
	focusRight            bool
	llamaCPPMissingActive bool
}

func NewBrowserModel(cfg *config.Config, srv runner.ModelRuntime) *BrowserModel {
	ti := textinput.New()
	ti.Placeholder = "Type to search..."
	ti.CharLimit = 156
	ti.Width = 30

	tokenTi := textinput.New()
	tokenTi.Placeholder = "Enter Hugging Face Token (optional)..."
	tokenTi.CharLimit = 128
	tokenTi.Width = 40
	tokenTi.SetValue(cfg.HFToken)

	q := model.NewDownloadQueue(cfg.Paths.Models, cfg.HFToken)

	// Apply theme colors
	ApplyTheme(cfg.Theme)

	// Check if llama.cpp is missing or empty
	llamaCPPMissing := false
	dir := cfg.Paths.LlamaCPP
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		llamaCPPMissing = true
	} else {
		files, err := os.ReadDir(dir)
		if err != nil || len(files) == 0 {
			llamaCPPMissing = true
		} else {
			binaryName := "llama-server"
			if runtime.GOOS == "windows" {
				binaryName = "llama-server.exe"
			}
			if _, err := os.Stat(filepath.Join(dir, binaryName)); os.IsNotExist(err) {
				llamaCPPMissing = true
			}
		}
	}

	return &BrowserModel{
		config:                cfg,
		srvRunner:             srv,
		loading:               true,
		searchInput:           ti,
		serverUIStatus:        UIStatusStopped,
		screenMode:            ScreenBrowser,
		sidebarItems:          []SidebarItem{},
		monitorModel:          NewMonitorModel(srv),
		lifecycleModel:        NewLifecycleModel(cfg, srv),
		downloadQueue:         q,
		downloaderModel:       NewDownloaderModel(cfg, q),
		onboardingActive:      !cfg.OnboardingCompleted && flag.Lookup("test.v") == nil,
		onboardingStep:        StepWelcome,
		onboardingTokenInput:  tokenTi,
		llamaCPPMissingActive: llamaCPPMissing && flag.Lookup("test.v") == nil,
	}
}

func (m *BrowserModel) isModelRunning(filePath string) bool {
	for _, inst := range m.srvRunner.GetAllInstances() {
		if inst.ModelPath == filePath {
			return true
		}
	}
	return false
}

type discoverMsg struct {
	models []*model.GGUFMetadata
	err    error
}

func discoverCmd(modelsDir string) tea.Cmd {
	return func() tea.Msg {
		models, err := model.DiscoverModels(modelsDir)
		return discoverMsg{models: models, err: err}
	}
}

type startServerMsg struct {
	err error
}

func startServerCmd(srv runner.ModelRuntime, llamaCppDir string, modelPath string, ctxSize uint32, threads int, gpuLayers int, batchSize int, host string, port int) tea.Cmd {
	return func() tea.Msg {
		err := srv.Start(modelPath, runner.StartOptions{
			LlamaCppDir: llamaCppDir,
			ContextSize: ctxSize,
			Threads:     threads,
			GPULayers:   gpuLayers,
			BatchSize:   batchSize,
			Host:        host,
			Port:        port,
		})
		return startServerMsg{err: err}
	}
}

type healthCheckMsg struct {
	online bool
}

func checkHealthCmd(port int) tea.Cmd {
	return func() tea.Msg {
		client := http.Client{
			Timeout: 200 * time.Millisecond,
		}
		// Poll for health up to 10 times (5 seconds total)
		for i := 0; i < 10; i++ {
			time.Sleep(500 * time.Millisecond)
			resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return healthCheckMsg{online: true}
				}
			}
		}
		return healthCheckMsg{online: false}
	}
}

type hardwareDetectMsg struct {
	specs *hardware.HardwareSpecs
	err   error
}

func detectHardwareCmd() tea.Msg {
	specs, err := hardware.DetectHardware()
	return hardwareDetectMsg{specs: specs, err: err}
}

type profilesMsg struct {
	profiles []*profile.Profile
	err      error
}

func (m *BrowserModel) loadProfilesCmd() tea.Cmd {
	return func() tea.Msg {
		profs, err := profile.LoadAll(m.config.Paths.Profiles)
		return profilesMsg{profiles: profs, err: err}
	}
}

type benchmarkMsg struct {
	step BenchmarkProgressStep
	res  *benchmark.BenchmarkResult
	err  error
	ch   chan benchmarkMsg
}

func (m *BrowserModel) startBenchmark(targetModel *model.GGUFMetadata) tea.Cmd {
	ch := make(chan benchmarkMsg)

	go func() {
		res, err := benchmark.RunBenchmark(
			m.config.Paths.LlamaCPP,
			targetModel,
			m.hardwareSpecs,
			m.config,
			func(stepNum int) {
				var step BenchmarkProgressStep
				switch stepNum {
				case 0:
					step = StepBooting
				case 1:
					step = StepRunningPrompt
				case 2:
					step = StepSavingData
				}
				ch <- benchmarkMsg{step: step, ch: ch}
			},
		)
		if err != nil {
			ch <- benchmarkMsg{step: StepError, err: err, ch: ch}
			return
		}

		err = benchmark.SaveResult(m.config.Paths.Benchmarks, res)
		if err != nil {
			ch <- benchmarkMsg{step: StepError, err: fmt.Errorf("failed to save result: %w", err), ch: ch}
			return
		}

		ch <- benchmarkMsg{step: StepDone, res: res, ch: ch}
	}()

	return m.readBenchmarkChan(ch)
}

func (m *BrowserModel) readBenchmarkChan(ch chan benchmarkMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

type downloadQueueMsg struct {
	task *model.DownloadTask
}

func (m *BrowserModel) readDownloadQueueChan() tea.Cmd {
	return func() tea.Msg {
		qChan := m.downloadQueue.GetChan()
		task, ok := <-qChan
		if !ok {
			return nil
		}
		return downloadQueueMsg{task: task}
	}
}

type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

func (m *BrowserModel) Init() tea.Cmd {
	return tea.Batch(
		discoverCmd(m.config.Paths.Models),
		m.loadProfilesCmd(),
		detectHardwareCmd,
		tickCmd(),
		m.readDownloadQueueChan(),
	)
}

func (m *BrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if m.onboardingActive {
		// StepHFToken is hoisted out first so that ALL message types (key events,
		// bracketed-paste PasteMsg, window resize, etc.) are trapped here and never
		// leak to the main browser handler where keys like "v" trigger navigation.
		if m.onboardingStep == StepHFToken {
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				switch keyMsg.String() {
				case "enter":
					tokenValue := strings.TrimSpace(m.onboardingTokenInput.Value())
					m.config.HFToken = tokenValue
					m.downloadQueue.UpdateToken(tokenValue)
					_ = m.config.Save()
					m.onboardingTokenInput.Blur()
					m.onboardingStep++
				case "esc":
					m.onboardingActive = false
					m.config.OnboardingCompleted = true
					_ = m.config.Save()
				case "ctrl+v":
					pasteFromClipboard(&m.onboardingTokenInput)
				case "ctrl+b":
					m.onboardingTokenInput.Blur()
					m.onboardingStep--
				default:
					// Forward all other keys to the input.
					var cmd tea.Cmd
					m.onboardingTokenInput, cmd = m.onboardingTokenInput.Update(msg)
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
			} else {
				// Forward all other messages to the textinput so the bubbles library can handle them.
				var cmd tea.Cmd
				m.onboardingTokenInput, cmd = m.onboardingTokenInput.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
			// Always return early during StepHFToken for all input/mouse messages
			// so they don't leak, but allow background messages to fall through.
			switch msg.(type) {
			case tea.KeyMsg, tea.MouseMsg:
				return m, tea.Batch(cmds...)
			}
		}

		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "enter", "space", "n", "N":
				if m.onboardingStep == StepFinished {
					m.onboardingActive = false
					m.config.OnboardingCompleted = true
					_ = m.config.Save()
				} else {
					m.onboardingStep++
					if m.onboardingStep == StepHFToken {
						m.onboardingTokenInput.Focus()
					}
				}
			case "p", "P", "b", "B":
				if m.onboardingStep > StepWelcome {
					m.onboardingStep--
					if m.onboardingStep == StepHFToken {
						m.onboardingTokenInput.Focus()
					}
				}
			case "esc", "q", "Q":
				m.onboardingActive = false
				m.config.OnboardingCompleted = true
				_ = m.config.Save()
			}
			return m, tea.Batch(cmds...)
		}
		// Catch-all: any user input event (key/mouse) must be swallowed
		// while onboarding is active to prevent accidental menu navigation,
		// but background system messages (like discoverMsg, tickMsg) must pass through.
		switch msg.(type) {
		case tea.KeyMsg, tea.MouseMsg:
			return m, tea.Batch(cmds...)
		}
	}

	if m.llamaCPPMissingActive {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "u", "U":
				m.llamaCPPMissingActive = false
				m.screenMode = ScreenSettings
				m.lifecycleModel.RefreshLocalVersion()
				m.lifecycleModel.RefreshBackupStatus()
				m.lifecycleModel.updatingRuntime = "llamacpp"
				cmds = append(cmds, m.lifecycleModel.StartCheckOnly())
			case "esc", "enter", "space":
				m.llamaCPPMissingActive = false
			case "q", "ctrl+c":
				_ = m.srvRunner.Stop()
				return m, tea.Quit
			}
		}
		// Always return early during missing warning popup for all input/mouse messages
		// so they don't leak, but allow background messages to fall through.
		switch msg.(type) {
		case tea.KeyMsg, tea.MouseMsg:
			return m, tea.Batch(cmds...)
		}
	}

	if m.screenMode == ScreenProfileCreator && m.profileCreatorModel != nil {
		if _, isSizeMsg := msg.(tea.WindowSizeMsg); !isSizeMsg {
			cmd, done, saved := m.profileCreatorModel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			if done {
				m.screenMode = ScreenDashboard
				m.profileCreatorModel = nil
				if saved {
					// Reload profiles from config paths
					profs, err := profile.LoadAll(m.config.Paths.Profiles)
					if err == nil {
						m.profiles = profs
						// Re-initialize dashboard with the newly added profile selected
						if m.dashboard != nil {
							m.dashboard.Profiles = profs
							// Select the newly added profile (which is the last one)
							m.dashboard.ActiveIdx = len(profs) - 1
						}
					}
				}
			}
			return m, tea.Batch(cmds...)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case discoverMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.models = msg.models
			for _, mod := range m.models {
				if override, ok := m.config.ModelTasks[mod.FilePath]; ok {
					mod.Task = override
				}
			}
			m.filterModels()

			// Select last selected model if found
			if m.config.LastSelectedModel != "" {
				for i, item := range m.sidebarItems {
					if item.Type == ItemModelEntry && item.ModelPath == m.config.LastSelectedModel {
						m.selected = i
						break
					}
				}
			}
		}

	case hardwareDetectMsg:
		if msg.err == nil {
			m.hardwareSpecs = msg.specs
		}

	case profilesMsg:
		if msg.err == nil {
			m.profiles = msg.profiles
			m.rebuildSidebar()
		}

	case downloadQueueMsg:
		if m.downloaderModel != nil {
			_, cmd := m.downloaderModel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if msg.task != nil && msg.task.Status == model.StatusCompleted {
			cmds = append(cmds, discoverCmd(m.config.Paths.Models))
		}
		cmds = append(cmds, m.readDownloadQueueChan())

	case hfResolveMsg:
		if m.downloaderModel != nil {
			_, cmd := m.downloaderModel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case updateMsg:
		if m.lifecycleModel != nil {
			_, cmd := m.lifecycleModel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case appCheckMsg:
		if m.lifecycleModel != nil {
			_, cmd := m.lifecycleModel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case benchmarkMsg:
		if m.benchmarkProgress == nil {
			break
		}
		if msg.err != nil {
			m.benchmarkProgress.Step = StepError
			m.benchmarkProgress.Err = msg.err
			break
		}

		m.benchmarkProgress.Step = msg.step
		if msg.step == StepDone {
			history, err := benchmark.LoadHistory(m.config.Paths.Benchmarks)
			if err == nil {
				m.perfDashboard = NewPerformanceDashboardModel(history)
			}
		} else {
			cmds = append(cmds, m.readBenchmarkChan(msg.ch))
		}

	case startServerMsg:
		if msg.err != nil {
			m.serverUIStatus = UIStatusFailed
			m.serverErr = msg.err
		}

	case healthCheckMsg:
		if msg.online {
			m.serverUIStatus = UIStatusRunning
		} else {
			// If it timed out, check if process is still running
			status, _, port := m.srvRunner.GetStatus()
			if status == runner.StatusRunning {
				m.serverUIStatus = UIStatusStarting
				// Retry health check
				cmds = append(cmds, checkHealthCmd(port))
			} else {
				m.serverUIStatus = UIStatusFailed
				m.serverErr = fmt.Errorf("server process terminated or failed to respond")
			}
		}

	case tickMsg:
		status, runModel, port := m.srvRunner.GetStatus()
		m.runningModelPath = runModel

		switch status {
		case runner.StatusRunning:
			if m.serverUIStatus == UIStatusStopped || m.serverUIStatus == UIStatusFailed {
				m.serverUIStatus = UIStatusStarting
				cmds = append(cmds, checkHealthCmd(port))
			}
		case runner.StatusFailed:
			m.serverUIStatus = UIStatusFailed
		case runner.StatusStopped:
			m.serverUIStatus = UIStatusStopped
		}

		cmds = append(cmds, tickCmd())

	case tea.KeyMsg:
		if m.screenMode == ScreenDashboard && m.dashboard != nil {
			switch msg.String() {
			case "left", "h":
				m.dashboard.CycleProfile(-1)
			case "right", "l":
				m.dashboard.CycleProfile(1)
			case "esc", "c", "C":
				m.screenMode = ScreenBrowser
			case "p", "P":
				m.profileCreatorModel = NewProfileCreatorModel(m.config.Paths.Profiles)
				m.screenMode = ScreenProfileCreator
			case "enter", "y", "Y":
				p := m.dashboard.ActiveProfile()
				if p != nil {
					// Persist profile selection
					m.config.ModelProfiles[m.dashboard.Model.FilePath] = p.Name
					
					// Record successful launch
					m.config.RecordLaunch(m.dashboard.Model.FilePath)
					
					_ = m.config.Save()
					m.rebuildSidebar()

					// Stop server on target port and launch with profile settings
					m.serverUIStatus = UIStatusStarting
					m.serverErr = nil
					m.runningModelPath = m.dashboard.Model.FilePath

					// Find an available port if this port is occupied by another model
					launchPort := findAvailablePort(p.Port, m.srvRunner, m.dashboard.Model.FilePath)
					_ = m.srvRunner.StopInstance(launchPort)

					cmds = append(cmds, startServerCmd(
						m.srvRunner,
						m.config.Paths.LlamaCPP,
						m.dashboard.Model.FilePath,
						p.Context,
						p.Threads,
						p.GPULayers,
						p.BatchSize,
						p.Host,
						launchPort,
					))
					cmds = append(cmds, checkHealthCmd(launchPort))
				}
				m.screenMode = ScreenBrowser
			}

		} else if m.screenMode == ScreenBenchmarkProgress && m.benchmarkProgress != nil {
			switch msg.String() {
			case "esc", "enter", "c", "C":
				if m.benchmarkProgress.Step == StepDone {
					m.screenMode = ScreenPerformanceDashboard
				} else if m.benchmarkProgress.Step == StepError {
					m.screenMode = ScreenBrowser
				}
			}
		} else if m.screenMode == ScreenPerformanceDashboard && m.perfDashboard != nil {
			switch msg.String() {
			case "esc", "q", "ctrl+c":
				m.screenMode = ScreenBrowser
			}
		} else if m.screenMode == ScreenServerMonitor && m.monitorModel != nil {
			switch msg.String() {
			case "esc", "c", "C":
				m.screenMode = ScreenBrowser
				m.rebuildSidebar()
			default:
				cmd := m.monitorModel.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		} else if m.screenMode == ScreenSettings && m.lifecycleModel != nil {
			if m.lifecycleModel.tokenEditActive {
				_, cmd := m.lifecycleModel.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			} else {
				switch msg.String() {
				case "esc":
					m.screenMode = ScreenBrowser
					m.rebuildSidebar()
				case "tab", "down", "j":
					m.lifecycleModel.NextRuntime()
				case "shift+tab", "up":
					m.lifecycleModel.PrevRuntime()
				case "enter", "c", "C":
					if m.lifecycleModel.state != StateChecking && m.lifecycleModel.state != StateDownloading && m.lifecycleModel.state != StateExtracting && m.lifecycleModel.state != StateVerifying && m.lifecycleModel.state != StateRollingBack {
						cmds = append(cmds, m.lifecycleModel.StartCheckSelected())
					}
				case " ", "u", "U":
					if m.lifecycleModel.state != StateChecking && m.lifecycleModel.state != StateDownloading && m.lifecycleModel.state != StateExtracting && m.lifecycleModel.state != StateVerifying && m.lifecycleModel.state != StateRollingBack {
						cmds = append(cmds, m.lifecycleModel.StartUpdateSelected())
					}
				case "r", "R":
					if m.lifecycleModel.hasBackup && m.lifecycleModel.state != StateChecking && m.lifecycleModel.state != StateDownloading && m.lifecycleModel.state != StateExtracting && m.lifecycleModel.state != StateVerifying && m.lifecycleModel.state != StateRollingBack {
						cmd := m.lifecycleModel.StartRollbackSelected()
						if cmd != nil {
							cmds = append(cmds, cmd)
						}
					}
				case "k", "K":
					// Direct ONNX check fallback
					if m.lifecycleModel.state != StateChecking && m.lifecycleModel.state != StateDownloading && m.lifecycleModel.state != StateExtracting && m.lifecycleModel.state != StateVerifying && m.lifecycleModel.state != StateRollingBack {
						m.lifecycleModel.SelectedRuntime = 1
						m.lifecycleModel.updatingRuntime = "onnx"
						cmds = append(cmds, m.lifecycleModel.StartOnnxCheckOnly())
					}
				case "o", "O":
					// Direct ONNX update fallback
					if m.lifecycleModel.state != StateChecking && m.lifecycleModel.state != StateDownloading && m.lifecycleModel.state != StateExtracting && m.lifecycleModel.state != StateVerifying && m.lifecycleModel.state != StateRollingBack {
						m.lifecycleModel.SelectedRuntime = 1
						m.lifecycleModel.updatingRuntime = "onnx"
						cmds = append(cmds, m.lifecycleModel.StartOnnxUpdate())
					}
				case "y", "Y":
					newTheme := "dracula"
					switch strings.ToLower(m.config.Theme) {
					case "dracula", "dark", "purple", "":
						newTheme = "sunset"
					case "sunset":
						newTheme = "nord"
					case "nord":
						newTheme = "cyberpunk"
					case "cyberpunk":
						newTheme = "forest"
					case "forest":
						newTheme = "monochrome"
					case "monochrome":
						newTheme = "dracula"
					}
					m.config.Theme = newTheme
					_ = m.config.Save()
					ApplyTheme(newTheme)
				case "t", "T":
					_, cmd := m.lifecycleModel.Update(msg)
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				case "v", "V":
					if !m.lifecycleModel.appChecking {
						m.lifecycleModel.SelectedRuntime = 2
						cmds = append(cmds, m.lifecycleModel.StartAppCheck())
					}
				case "a", "A":
					if m.lifecycleModel.appLatestTag != "" && m.lifecycleModel.appLatestTag != m.lifecycleModel.appVersion && !m.lifecycleModel.appUpdating {
						m.lifecycleModel.SelectedRuntime = 2
						cmds = append(cmds, m.lifecycleModel.StartAppUpdate())
					}
				case "n", "N":
					// Reset onboarding tour
					m.config.OnboardingCompleted = false
					_ = m.config.Save()
					m.onboardingActive = true
					m.onboardingStep = StepWelcome
					m.screenMode = ScreenBrowser
				}
			}
		} else if m.screenMode == ScreenDownloader && m.downloaderModel != nil {
			switch msg.String() {
			case "esc":
				if m.downloaderModel.focus == FocusFileList {
					m.downloaderModel.resolvedFiles = nil
					m.downloaderModel.repoID = ""
					m.downloaderModel.focus = FocusURL
					m.downloaderModel.urlInput.Focus()
				} else {
					m.downloaderModel.urlInput.Blur()
					m.downloaderModel.filenameInput.Blur()
					m.screenMode = ScreenBrowser
					m.rebuildSidebar()
				}
			default:
				_, cmd := m.downloaderModel.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		} else if m.searchActive {
			switch msg.String() {
			case "enter":
				m.searchActive = false
				m.searchInput.Blur()
			case "esc":
				m.searchActive = false
				m.searchInput.SetValue("")
				m.searchInput.Blur()
				m.filterModels()
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				cmds = append(cmds, cmd)
				m.filterModels()
			}
		} else if !m.onboardingActive {
			switch msg.String() {
			case "q", "ctrl+c":
				_ = m.srvRunner.Stop()
				return m, tea.Quit

			case "tab", "shift+tab":
				m.focusRight = !m.focusRight

			case "right", "l":
				m.focusRight = true

			case "left", "h":
				m.focusRight = false

			case "up", "k":
				m.moveSelection(-1)

			case "down", "j":
				m.moveSelection(1)

			case "/":
				m.searchActive = true
				m.searchInput.Focus()
				m.searchInput.SetValue("")
				m.filterModels()

			case "s", "S":
				m.serverUIStatus = UIStatusStopped
				m.serverErr = nil
				if m.selected >= 0 && m.selected < len(m.sidebarItems) {
					item := m.sidebarItems[m.selected]
					if item.Type == ItemModelEntry {
						selectedModel := m.models[item.ModelIdx]
						for _, inst := range m.srvRunner.GetAllInstances() {
							if inst.ModelPath == selectedModel.FilePath {
								_ = m.srvRunner.StopInstance(inst.Port)
							}
						}
					}
				}

			case "ctrl+s":
				m.serverUIStatus = UIStatusStopped
				m.serverErr = nil
				_ = m.srvRunner.Stop()

			case "e", "E":
				if m.selected >= 0 && m.selected < len(m.sidebarItems) {
					item := m.sidebarItems[m.selected]
					if item.Type == ItemModelEntry {
						selectedModel := m.models[item.ModelIdx]
						var nextTask string
						switch selectedModel.Task {
						case "TEXT_GENERATION":
							nextTask = "EMBEDDING"
						case "EMBEDDING":
							nextTask = "RERANKING"
						case "RERANKING":
							nextTask = "SPEECH_TO_TEXT"
						case "SPEECH_TO_TEXT":
							nextTask = "TEXT_TO_SPEECH"
						case "TEXT_TO_SPEECH":
							nextTask = "IMAGE_GENERATION"
						case "IMAGE_GENERATION":
							nextTask = "VISION"
						case "VISION":
							nextTask = "MULTIMODAL"
						default:
							nextTask = "TEXT_GENERATION"
						}
						selectedModel.Task = nextTask
						m.config.ModelTasks[selectedModel.FilePath] = nextTask
						_ = m.config.Save()
						m.rebuildSidebar()
					}
				}

			case "f", "F":
				if m.selected >= 0 && m.selected < len(m.sidebarItems) {
					item := m.sidebarItems[m.selected]
					if item.Type == ItemModelEntry {
						m.config.ToggleFavorite(item.ModelPath)
						_ = m.config.Save()
						m.rebuildSidebar()
					}
				}



			case "b", "B":
				if m.selected >= 0 && m.selected < len(m.sidebarItems) {
					item := m.sidebarItems[m.selected]
					if item.Type == ItemModelEntry {
						selectedModel := m.models[item.ModelIdx]
						m.benchmarkProgress = NewBenchmarkProgressModel(selectedModel.Name)
						m.screenMode = ScreenBenchmarkProgress
						_ = m.srvRunner.Stop()
						m.serverUIStatus = UIStatusStopped
						cmds = append(cmds, m.startBenchmark(selectedModel))
					}
				}

			case "v", "V":
				history, err := benchmark.LoadHistory(m.config.Paths.Benchmarks)
				if err == nil {
					m.perfDashboard = NewPerformanceDashboardModel(history)
					m.screenMode = ScreenPerformanceDashboard
				}

			case "m", "M":
				m.monitorModel.Refresh()
				m.screenMode = ScreenServerMonitor

			case "u", "U":
				m.lifecycleModel.RefreshLocalVersion()
				m.lifecycleModel.RefreshBackupStatus()
				m.lifecycleModel.updatingRuntime = "llamacpp"
				m.screenMode = ScreenSettings
				cmds = append(cmds, m.lifecycleModel.StartCheckOnly())

			case "d", "D":
				m.downloaderModel.focus = FocusURL
				m.downloaderModel.urlInput.Focus()
				m.screenMode = ScreenDownloader

			case "space", "enter":
				if m.selected >= 0 && m.selected < len(m.sidebarItems) {
					item := m.sidebarItems[m.selected]
					if item.Type == ItemModelEntry {
						selectedModel := m.models[item.ModelIdx]
						profName := m.config.ModelProfiles[selectedModel.FilePath]
						if profName == "" {
							profName = "Balanced"
						}
						m.dashboard = NewDashboardModel(selectedModel, m.hardwareSpecs, m.profiles, profName)
						m.screenMode = ScreenDashboard
					}
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *BrowserModel) filterModels() {
	query := strings.TrimSpace(strings.ToLower(m.searchInput.Value()))
	if query == "" {
		m.filtered = make([]int, len(m.models))
		for i := range m.models {
			m.filtered[i] = i
		}
	} else {
		m.filtered = []int{}
		for i, mod := range m.models {
			if strings.Contains(strings.ToLower(mod.Name), query) ||
				strings.Contains(strings.ToLower(mod.Architecture), query) ||
				strings.Contains(strings.ToLower(mod.FilePath), query) {
				m.filtered = append(m.filtered, i)
			}
		}
	}
	m.rebuildSidebar()
}

func (m *BrowserModel) saveLastSelected() {
	if m.selected >= 0 && m.selected < len(m.sidebarItems) {
		item := m.sidebarItems[m.selected]
		if item.Type == ItemModelEntry {
			m.config.LastSelectedModel = item.ModelPath
			_ = m.config.Save()
		}
	}
}

func (m *BrowserModel) rebuildSidebar() {
	m.sidebarItems = []SidebarItem{}

	modelPathMap := make(map[string]int)
	for idx, mod := range m.models {
		modelPathMap[mod.FilePath] = idx
	}

	query := strings.TrimSpace(m.searchInput.Value())
	if m.searchActive && query != "" {
		for _, idx := range m.filtered {
			mod := m.models[idx]
			m.sidebarItems = append(m.sidebarItems, SidebarItem{
				Type:      ItemModelEntry,
				Label:     mod.Name,
				ModelIdx:  idx,
				ModelPath: mod.FilePath,
			})
		}
		m.adjustSelection()
		return
	}

	// 1. Favorites
	var existingFavorites []SidebarItem
	for _, path := range m.config.Favorites {
		if idx, ok := modelPathMap[path]; ok {
			existingFavorites = append(existingFavorites, SidebarItem{
				Type:      ItemModelEntry,
				Label:     m.models[idx].Name,
				ModelIdx:  idx,
				ModelPath: path,
			})
		}
	}
	if len(existingFavorites) > 0 {
		m.sidebarItems = append(m.sidebarItems, SidebarItem{Type: ItemSectionHeader, Label: "★ FAVORITES"})
		m.sidebarItems = append(m.sidebarItems, existingFavorites...)
	}

	// 2. Recently Used
	var existingRecents []SidebarItem
	for _, path := range m.config.RecentLaunches {
		if idx, ok := modelPathMap[path]; ok {
			existingRecents = append(existingRecents, SidebarItem{
				Type:      ItemModelEntry,
				Label:     m.models[idx].Name,
				ModelIdx:  idx,
				ModelPath: path,
			})
		}
	}
	if len(existingRecents) > 0 {
		m.sidebarItems = append(m.sidebarItems, SidebarItem{Type: ItemSectionHeader, Label: "RECENTLY USED"})
		m.sidebarItems = append(m.sidebarItems, existingRecents...)
	}



	// 4. All Models
	if len(m.models) > 0 {
		m.sidebarItems = append(m.sidebarItems, SidebarItem{Type: ItemSectionHeader, Label: "ALL MODELS"})
		for idx, mod := range m.models {
			m.sidebarItems = append(m.sidebarItems, SidebarItem{
				Type:      ItemModelEntry,
				Label:     mod.Name,
				ModelIdx:  idx,
				ModelPath: mod.FilePath,
			})
		}
	}

	m.adjustSelection()
}

func (m *BrowserModel) moveSelection(direction int) {
	if len(m.sidebarItems) == 0 {
		return
	}
	next := m.selected
	for {
		next += direction
		if next < 0 || next >= len(m.sidebarItems) {
			return
		}
		if m.sidebarItems[next].Type != ItemSectionHeader {
			m.selected = next
			m.saveLastSelected()
			return
		}
	}
}

func (m *BrowserModel) adjustSelection() {
	if len(m.sidebarItems) == 0 {
		m.selected = 0
		return
	}
	if m.selected >= len(m.sidebarItems) {
		m.selected = len(m.sidebarItems) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.sidebarItems[m.selected].Type == ItemSectionHeader {
		found := false
		for i := m.selected; i < len(m.sidebarItems); i++ {
			if m.sidebarItems[i].Type != ItemSectionHeader {
				m.selected = i
				found = true
				break
			}
		}
		if !found {
			for i := m.selected; i >= 0; i-- {
				if m.sidebarItems[i].Type != ItemSectionHeader {
					m.selected = i
					break
				}
			}
		}
	}
}

func (m *BrowserModel) rightPanelView(width int, height int) string {
	if len(m.sidebarItems) == 0 || m.selected < 0 || m.selected >= len(m.sidebarItems) {
		return "\n  No model selected."
	}
	item := m.sidebarItems[m.selected]

	if item.Type != ItemModelEntry {
		return "\n  No model selected."
	}
	selectedModel := m.models[item.ModelIdx]

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n  %s\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(selectedModel.Name)))
	sb.WriteString(fmt.Sprintf("  %s\n\n", StyleHelp.Render(selectedModel.FilePath)))

	sb.WriteString(fmt.Sprintf("  %-16s %s\n", "Architecture:", selectedModel.Architecture))
	sb.WriteString(fmt.Sprintf("  %-16s %s\n", "Quantization:", selectedModel.Quantization))
	sb.WriteString(fmt.Sprintf("  %-16s %s\n", "Context Length:", fmt.Sprintf("%d tokens", selectedModel.ContextLength)))
	sb.WriteString(fmt.Sprintf("  %-16s %s\n", "Param Count:", formatParams(selectedModel.ParamCount)))
	sb.WriteString(fmt.Sprintf("  %-16s %s\n", "File Size:", formatSize(selectedModel.FileSize)))
	sb.WriteString(fmt.Sprintf("  %-16s %s\n", "Runtime:", selectedModel.Runtime))
	sb.WriteString(fmt.Sprintf("  %-16s %s (Press [E] to cycle)\n\n", "Task Type:", selectedModel.Task))




	// Memory Estimates
	if m.hardwareSpecs != nil {
		est := hardware.EstimateMemory(selectedModel, m.hardwareSpecs, 0)
		var suitStr string
		var suitabilityColor lipgloss.Color
		switch est.Suitability {
		case hardware.SuitabilityFits:
			suitStr = lipgloss.NewStyle().Background(ColorSecondary).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 1).Render(" FITS GPU ")
			suitabilityColor = ColorSecondary
		case hardware.SuitabilityPartial:
			suitStr = lipgloss.NewStyle().Background(ColorGold).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 1).Render(" PARTIAL ")
			suitabilityColor = ColorGold
		case hardware.SuitabilityExceeds:
			suitStr = lipgloss.NewStyle().Background(ColorDanger).Foreground(ColorWhite).Bold(true).Padding(0, 1).Render(" EXCEEDS ")
			suitabilityColor = ColorDanger
		}

		sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Bold(true).Render("Memory Suitability:")))
		sb.WriteString(fmt.Sprintf("  %-16s %s\n", "Status:", suitStr))

		// Visual progress bars
		if m.hardwareSpecs.IsUnified {
			var unifiedPct float64
			if m.hardwareSpecs.GPU.VRAM > 0 {
				unifiedPct = (float64(est.TotalMemory) / float64(m.hardwareSpecs.GPU.VRAM)) * 100
			}
			bar := RenderProgressBar(unifiedPct, 15, suitabilityColor)
			sb.WriteString(fmt.Sprintf("  %-16s %s %.0f%% (%s / %s)\n", "Unified Memory:", bar, unifiedPct, formatSize(int64(est.TotalMemory)), formatSize(int64(m.hardwareSpecs.GPU.VRAM))))
		} else {
			if m.hardwareSpecs.GPU.VRAM > 0 {
				vramUsage := (est.WeightSize * uint64(est.GPUOffloadPct) / 100)
				if est.GPUOffloadPct > 0 {
					vramUsage += est.KVCacheSize + est.Overhead
				}
				if vramUsage > m.hardwareSpecs.GPU.VRAM {
					vramUsage = m.hardwareSpecs.GPU.VRAM
				}
				vramPct := (float64(vramUsage) / float64(m.hardwareSpecs.GPU.VRAM)) * 100
				barColor := ColorSecondary
				if vramPct > 90 {
					barColor = ColorGold
				}
				if est.Suitability == hardware.SuitabilityExceeds {
					barColor = ColorDanger
				}
				bar := RenderProgressBar(vramPct, 15, barColor)
				sb.WriteString(fmt.Sprintf("  %-16s %s %.0f%% (%s / %s)\n", "GPU VRAM:", bar, vramPct, formatSize(int64(vramUsage)), formatSize(int64(m.hardwareSpecs.GPU.VRAM))))
			} else {
				sb.WriteString(fmt.Sprintf("  %-16s %s\n", "GPU VRAM:", lipgloss.NewStyle().Foreground(ColorMuted).Render("N/A (CPU Mode)")))
			}

			if m.hardwareSpecs.RAM.Total > 0 {
				vramUsage := (est.WeightSize * uint64(est.GPUOffloadPct) / 100)
				if est.GPUOffloadPct > 0 {
					vramUsage += est.KVCacheSize + est.Overhead
				}
				var ramUsage uint64
				if est.TotalMemory > vramUsage {
					ramUsage = est.TotalMemory - vramUsage
				}
				ramPct := (float64(ramUsage) / float64(m.hardwareSpecs.RAM.Total)) * 100
				barColor := ColorSecondary
				if ramPct > 80 {
					barColor = ColorGold
				}
				bar := RenderProgressBar(ramPct, 15, barColor)
				sb.WriteString(fmt.Sprintf("  %-16s %s %.0f%% (%s / %s)\n", "System RAM:", bar, ramPct, formatSize(int64(ramUsage)), formatSize(int64(m.hardwareSpecs.RAM.Total))))
			}
		}

		sb.WriteString(fmt.Sprintf("  %-16s %s\n", "KV Cache:", formatSize(int64(est.KVCacheSize))))
		sb.WriteString(fmt.Sprintf("  %-16s %s\n", "Overhead:", formatSize(int64(est.Overhead))))
		sb.WriteString(fmt.Sprintf("  %-16s %s (GPU offload: %d%%)\n", "Total Memory:", formatSize(int64(est.TotalMemory)), est.GPUOffloadPct))
		sb.WriteString(fmt.Sprintf("  %-16s %s\n", "Recommendation:", est.Reason))
		if est.Suitability == hardware.SuitabilityExceeds {
			sb.WriteString(fmt.Sprintf("                   %s %s\n",
				lipgloss.NewStyle().Foreground(ColorGold).Bold(true).Render("Press [Enter]"),
				lipgloss.NewStyle().Foreground(ColorMuted).Render("to choose a profile with a smaller context length."),
			))
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("  Detecting hardware requirements...\n\n")
	}

	sb.WriteString("  " + lipgloss.NewStyle().Bold(true).Render("Server Status:") + " ")
	var statusText string
	isRunning := m.isModelRunning(selectedModel.FilePath)

	if isRunning {
		port := 50505
		for _, inst := range m.srvRunner.GetAllInstances() {
			if inst.ModelPath == selectedModel.FilePath {
				port = inst.Port
				break
			}
		}
		statusText = StyleBadgeRunning.Render(" RUNNING ") + lipgloss.NewStyle().Foreground(ColorSecondary).Render(fmt.Sprintf(" on http://127.0.0.1:%d", port))
	} else if m.serverUIStatus == UIStatusStarting && m.runningModelPath == selectedModel.FilePath {
		statusText = StyleBadgeStarting.Render(" STARTING ")
	} else if m.serverUIStatus == UIStatusFailed && m.runningModelPath == selectedModel.FilePath {
		statusText = StyleBadgeFailed.Render(" FAILED ")
	} else {
		statusText = StyleBadgeStopped.Render(" STOPPED ")
	}
	sb.WriteString(statusText + "\n")

	if isRunning {
		sb.WriteString("\n  Active model is currently serving requests.\n")
	} else if m.serverUIStatus == UIStatusFailed && m.runningModelPath == selectedModel.FilePath && m.serverErr != nil {
		sb.WriteString(fmt.Sprintf("\n  %s\n", lipgloss.NewStyle().Foreground(lipgloss.Color("#FF3B30")).Render(fmt.Sprintf("Error: %v", m.serverErr))))
	}
	sb.WriteString("\n")

	// System specifications
	if m.hardwareSpecs != nil {
		gpuInfo := fmt.Sprintf("%s (%s VRAM)", m.hardwareSpecs.GPU.Name, formatSize(int64(m.hardwareSpecs.GPU.VRAM)))
		if m.hardwareSpecs.IsUnified {
			gpuInfo = fmt.Sprintf("%s (%s Unified)", m.hardwareSpecs.GPU.Name, formatSize(int64(m.hardwareSpecs.GPU.VRAM)))
		}
		sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Bold(true).Render("System Specifications:")))
		sb.WriteString(fmt.Sprintf("  %-16s %s\n", "OS:", m.hardwareSpecs.OS))
		sb.WriteString(fmt.Sprintf("  %-16s %s\n", "CPU Model:", m.hardwareSpecs.CPU.Model))
		sb.WriteString(fmt.Sprintf("  %-16s %d threads\n", "CPU Threads:", m.hardwareSpecs.CPU.Threads))
		sb.WriteString(fmt.Sprintf("  %-16s %s (%s available)\n", "RAM:", formatSize(int64(m.hardwareSpecs.RAM.Total)), formatSize(int64(m.hardwareSpecs.RAM.Available))))
		sb.WriteString(fmt.Sprintf("  %-16s %s\n", "GPU:", gpuInfo))
	}

	return sb.String()
}

func (m *BrowserModel) View() string {
	if m.loading {
		return "\n  Scanning models directory... Please wait."
	}

	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n  Press Q to quit.", m.err)
	}

	if m.screenMode == ScreenDashboard && m.dashboard != nil {
		dashView := m.dashboard.View(m.width, m.height)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dashView)
	}



	if m.screenMode == ScreenBenchmarkProgress && m.benchmarkProgress != nil {
		progressView := m.benchmarkProgress.View(m.width, m.height)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, progressView)
	}

	if m.screenMode == ScreenPerformanceDashboard && m.perfDashboard != nil {
		perfView := m.perfDashboard.View(m.width, m.height)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, perfView)
	}

	if m.screenMode == ScreenServerMonitor && m.monitorModel != nil {
		monitorView := m.monitorModel.View(m.width, m.height)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, monitorView)
	}

	if m.screenMode == ScreenSettings && m.lifecycleModel != nil {
		settingsView := m.lifecycleModel.View(m.width, m.height)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, settingsView)
	}

	if m.screenMode == ScreenDownloader && m.downloaderModel != nil {
		downView := m.downloaderModel.View(m.width, m.height)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, downView)
	}

	if m.screenMode == ScreenProfileCreator && m.profileCreatorModel != nil {
		creatorView := m.profileCreatorModel.View(m.width, m.height)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, creatorView)
	}

	totalWidth := m.width
	if totalWidth < 60 {
		totalWidth = 60
	}
	leftWidth := int(float64(totalWidth) * 0.40)
	if leftWidth < 25 {
		leftWidth = 25
	}
	rightWidth := totalWidth - leftWidth - 6

	panelHeight := m.height - 6
	if panelHeight < 10 {
		panelHeight = 10
	}

	var leftSb strings.Builder
	if len(m.sidebarItems) == 0 {
		leftSb.WriteString("\n  No models found.")
	} else {
		maxVisible := panelHeight - 2
		if maxVisible < 1 {
			maxVisible = 1
		}
		if m.selected < m.scrollOffset {
			m.scrollOffset = m.selected
		} else if m.selected >= m.scrollOffset+maxVisible {
			m.scrollOffset = m.selected - maxVisible + 1
		}

		end := m.scrollOffset + maxVisible
		if end > len(m.sidebarItems) {
			end = len(m.sidebarItems)
		}

		leftSb.WriteString("\n")
		for idx := m.scrollOffset; idx < end; idx++ {
			item := m.sidebarItems[idx]

			if item.Type == ItemSectionHeader {
				leftSb.WriteString(lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(fmt.Sprintf(" %s", item.Label)) + "\n")
				continue
			}

			if item.Type == ItemFolderHeader {
				folderLabel := item.Label
				if idx == m.selected {
					leftSb.WriteString(
						StyleSelectedListItem.Width(leftWidth - 2).Render(
							fmt.Sprintf("  %s", folderLabel),
						) + "\n",
					)
				} else {
					leftSb.WriteString(
						fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(folderLabel)),
					)
				}
				continue
			}

			// Model item entry
			mod := m.models[item.ModelIdx]

			bullet := "●"
			var bulletStyled string
			if m.hardwareSpecs != nil {
				est := hardware.EstimateMemory(mod, m.hardwareSpecs, 0)
				switch est.Suitability {
				case hardware.SuitabilityFits:
					bulletStyled = StyleSuccess.Render(bullet)
				case hardware.SuitabilityPartial:
					bulletStyled = StyleWarning.Render(bullet)
				case hardware.SuitabilityExceeds:
					bulletStyled = StyleDanger.Render(bullet)
				}
			} else {
				bulletStyled = StyleListItem.Render(bullet)
			}

			isRunningStr := ""
			if m.isModelRunning(mod.FilePath) {
				isRunningStr = "▶ "
			}

			displayName := item.Label
			if m.config.IsFavorite(mod.FilePath) {
				starSymbol := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB800")).Render("★ ")
				// Strip leading indent from nested elements to format properly
				if strings.HasPrefix(displayName, "  ") {
					displayName = "  " + starSymbol + strings.TrimPrefix(displayName, "  ")
				} else {
					displayName = starSymbol + displayName
				}
			}

			maxNameLen := leftWidth - 8
			if maxNameLen > 0 && len(displayName) > maxNameLen {
				displayName = displayName[:maxNameLen-3] + "..."
			}

			if idx == m.selected {
				leftSb.WriteString(
					StyleSelectedListItem.Width(leftWidth - 2).Render(
						fmt.Sprintf("%s%s %s", isRunningStr, bullet, displayName),
					) + "\n",
				)
			} else {
				leftSb.WriteString(
					fmt.Sprintf("  %s%s %s\n", isRunningStr, bulletStyled, StyleListItem.Render(displayName)),
				)
			}
		}
	}

	var leftTitle, rightTitle string
	leftBorderColor := ColorBorder
	rightBorderColor := ColorBorder

	if m.onboardingActive {
		if m.onboardingStep == StepModelSidebar {
			leftBorderColor = ColorPrimary
		} else if m.onboardingStep == StepDetailsPanel {
			rightBorderColor = ColorPrimary
		}
		leftTitle = StyleTitle.Render("Models")
		rightTitle = StyleTitle.Render("Details")
	} else {
		if m.focusRight {
			rightBorderColor = ColorPrimary
			leftTitle = StyleTitle.Render("Models")
			rightTitle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Padding(0, 1).Render("Details")
		} else {
			leftBorderColor = ColorPrimary
			leftTitle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Padding(0, 1).Render("Models")
			rightTitle = StyleTitle.Render("Details")
		}
	}

	leftPanelContent := fmt.Sprintf("%s\n%s", leftTitle, leftSb.String())

	leftView := StyleLeftPanel.Copy().
		BorderForeground(leftBorderColor).
		Width(leftWidth).
		Height(panelHeight).
		Render(leftPanelContent)

	rightPanelContent := fmt.Sprintf("%s\n%s", rightTitle, m.rightPanelView(rightWidth, panelHeight))
	rightView := StyleRightPanel.Copy().
		BorderForeground(rightBorderColor).
		Width(rightWidth).
		Height(panelHeight).
		Render(rightPanelContent)

	mainView := lipgloss.JoinHorizontal(lipgloss.Top, leftView, rightView)

	header := lipgloss.NewStyle().MarginBottom(1).Render(
		RenderGradient("RUNORA — Local AI Control Center", ThemeGradientStart, ThemeGradientEnd),
	)

	var footer string
	if m.searchActive {
		footer = fmt.Sprintf("Search: %s  %s", m.searchInput.View(), StyleHelp.Render("[Esc] Clear/Exit  [Enter] Confirm"))
	} else {
		footerItems := []string{
			fmt.Sprintf("%s Launch", StyleHelpKey.Render("[Enter]")),
			fmt.Sprintf("%s Favorite", StyleHelpKey.Render("[F]")),
			fmt.Sprintf("%s Benchmark", StyleHelpKey.Render("[B]")),
			fmt.Sprintf("%s Dashboard", StyleHelpKey.Render("[V]")),
			fmt.Sprintf("%s Monitor", StyleHelpKey.Render("[M]")),
			fmt.Sprintf("%s Settings", StyleHelpKey.Render("[U]")),
			fmt.Sprintf("%s Downloader", StyleHelpKey.Render("[D]")),
			fmt.Sprintf("%s Search", StyleHelpKey.Render("[/]")),
			fmt.Sprintf("%s Stop", StyleHelpKey.Render("[S]")),
			fmt.Sprintf("%s Quit", StyleHelpKey.Render("[Q]")),
		}
		footer = strings.Join(footerItems, " │ ")
	}

	bgView := lipgloss.JoinVertical(lipgloss.Left, header, mainView, StyleHelp.Render(footer))
	if m.onboardingActive {
		onboardingOverlay := m.onboardingOverlayView(m.width, m.height)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, onboardingOverlay)
	}
	if m.llamaCPPMissingActive {
		missingOverlay := m.llamaCPPMissingOverlayView(m.width, m.height)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, missingOverlay)
	}
	return bgView
}

func (m *BrowserModel) onboardingOverlayView(width int, height int) string {
	var sb strings.Builder
	sb.WriteString("\n")

	var stepTitle, stepDesc string
	switch m.onboardingStep {
	case StepWelcome:
		stepTitle = "Welcome to Runora!"
		stepDesc = "Runora is your local AI control center.\nThis quick tour will guide you through all the core features.\n\nPress [Enter / Space] to begin, or [Esc] to skip."
	case StepModelSidebar:
		stepTitle = "1. Model Discovery & Sidebar"
		stepDesc = "On the left is the Models Sidebar.\n- Models are recursively discovered under your models/ directory.\n- Navigate them using [Up / Down Arrow keys].\n- Press [F] to toggle favoriting a model for quick access."
	case StepDetailsPanel:
		stepTitle = "2. Model Specifications & Suitability"
		stepDesc = "On the right is the Details Panel.\n- Here you can see parsed GGUF metadata (architecture, parameters, etc.).\n- It automatically estimates VRAM and system memory usage.\n- If a model exceeds your hardware specs, a warning is displayed."
	case StepLaunchDashboard:
		stepTitle = "3. Profiles & Launching"
		stepDesc = "Press [Enter] on a selected model to open the Launch Dashboard.\n- Choose a context profile (Fast, Balanced, High, CPU, etc.).\n- View the exact llama.cpp command that will be launched.\n- Press [P] to create a custom profile."
	case StepDownloadLifecycle:
		stepTitle = "4. Downloader & Lifecycle Manager"
		stepDesc = "Additional utility panels are accessible via hotkeys:\n- Press [D] to open the Downloader to pull models directly via URL.\n- Press [U] to open the Lifecycle Manager (check/apply updates or rollback backups)."
	case StepHFToken:
		stepTitle = "5. Hugging Face Access (Optional)"
		stepDesc = "To download gated models (e.g. Llama, Gemma) or private repos, configure your HF Token here:\n\n" + m.onboardingTokenInput.View() + "\n\nPress [Enter] to save and continue."
	case StepFinished:
		stepTitle = "Tour Completed!"
		stepDesc = "You are all set to manage local AI models!\n\nPress [Enter / Space / Esc] to exit the tour and start exploring."
	}

	sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(stepTitle)))
	sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorWhite).Render(stepDesc)))

	// Navigation instructions footer
	var navHelp string
	if m.onboardingStep == StepWelcome {
		navHelp = fmt.Sprintf("  %s Next  %s Skip Tour", StyleHelpKey.Render("[Enter/Space]"), StyleHelpKey.Render("[Esc]"))
	} else if m.onboardingStep == StepFinished {
		navHelp = fmt.Sprintf("  %s Finish Tour", StyleHelpKey.Render("[Enter/Space]"))
	} else if m.onboardingStep == StepHFToken {
		navHelp = fmt.Sprintf("  %s Save & Next  %s Back  %s Skip Tour", StyleHelpKey.Render("[Enter]"), StyleHelpKey.Render("[Ctrl+B]"), StyleHelpKey.Render("[Esc]"))
	} else {
		navHelp = fmt.Sprintf("  %s Next  %s Back  %s Skip Tour", StyleHelpKey.Render("[Enter/Space]"), StyleHelpKey.Render("[P/B]"), StyleHelpKey.Render("[Esc]"))
	}
	sb.WriteString(navHelp + "\n")

	boxWidth := width - 8
	if boxWidth < 50 {
		boxWidth = 50
	}
	if boxWidth > 70 {
		boxWidth = 70
	}

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2).
		Width(boxWidth).
		Render(sb.String())
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffix := ""
	switch exp {
	case 0:
		suffix = "KB"
	case 1:
		suffix = "MB"
	case 2:
		suffix = "GB"
	default:
		suffix = "TB"
	}
	return fmt.Sprintf("%.2f %s", float64(bytes)/float64(div), suffix)
}

func formatParams(params uint64) string {
	if params == 0 {
		return "Unknown"
	}
	if params >= 1e9 {
		return fmt.Sprintf("%.2f B", float64(params)/1e9)
	}
	if params >= 1e6 {
		return fmt.Sprintf("%.2f M", float64(params)/1e6)
	}
	return fmt.Sprintf("%d", params)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m *BrowserModel) llamaCPPMissingOverlayView(width, height int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("RUNTIME NOT FOUND")))
	sb.WriteString("  Runora requires the llama.cpp inference runtime to run models,\n")
	sb.WriteString("  but no installation was found in your llama.cpp folder.\n\n")
	sb.WriteString("  " + StyleHelpKey.Render("[U]") + " Go to Settings to automatically download & install\n")
	sb.WriteString("  " + StyleHelpKey.Render("[Esc / Enter]") + " Dismiss this warning\n")
	sb.WriteString("  " + StyleHelpKey.Render("[Q]") + " Quit Runora\n")

	boxWidth := width - 8
	if boxWidth < 50 {
		boxWidth = 50
	}
	if boxWidth > 75 {
		boxWidth = 75
	}

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2).
		Width(boxWidth).
		Render(sb.String())
}

func findAvailablePort(startPort int, srv runner.ModelRuntime, currentModelPath string) int {
	instances := srv.GetAllInstances()
	port := startPort
	for {
		busy := false
		for _, inst := range instances {
			if inst.Port == port && inst.ModelPath != currentModelPath {
				busy = true
				break
			}
		}
		if !busy {
			return port
		}
		port++
	}
}
