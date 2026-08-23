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
	ScreenLogStreamer
)

type OnboardingStep int

const (
	StepWelcome OnboardingStep = iota
	StepStorage
	StepTokens
	StepRuntime
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
	logStreamerModel      *LogStreamerModel
	onboardingActive      bool
	onboardingStep        OnboardingStep
	onboardingTokenInput  textinput.Model // Hugging Face token input
	onboardingGHTokenInput textinput.Model // GitHub token input
	onboardingTokenFocus  int             // 0 = GitHub Token, 1 = Hugging Face Token
	onboardingChannel     runner.ReleaseChannel
	onboardingBackend     runner.BackendType
	onboardingBackendIdx  int
	onboardingBackends    []runner.BackendType
	focusRight            bool
	llamaCPPMissingActive bool
	toasts                *ToastManager
	themePicker           *ThemePickerModel
	themePickerActive     bool
}

func NewBrowserModel(cfg *config.Config, srv runner.ModelRuntime) *BrowserModel {
	ti := textinput.New()
	ti.Placeholder = "Type to search..."
	ti.CharLimit = 156
	ti.Width = 30

	ghTokenTi := textinput.New()
	ghTokenTi.Placeholder = "Enter GitHub Token (optional)..."
	ghTokenTi.CharLimit = 128
	ghTokenTi.Width = 48
	ghTokenTi.EchoMode = textinput.EchoPassword
	ghTokenTi.EchoCharacter = '*'
	ghTokenTi.SetValue(cfg.GitHubToken)

	tokenTi := textinput.New()
	tokenTi.Placeholder = "Enter Hugging Face Token (optional)..."
	tokenTi.CharLimit = 128
	tokenTi.Width = 48
	tokenTi.EchoMode = textinput.EchoPassword
	tokenTi.EchoCharacter = '*'
	tokenTi.SetValue(cfg.HFToken)

	backends := []runner.BackendType{
		runner.BackendCUDA12,
		runner.BackendVulkan,
		runner.BackendMetal,
		runner.BackendCPU,
	}
	defaultBackendIdx := 0
	if runtime.GOOS == "darwin" {
		defaultBackendIdx = 2
	}

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
		monitorModel:          NewMonitorModelWithConfig(srv, cfg, nil),
		lifecycleModel:        NewLifecycleModel(cfg, srv),
		downloadQueue:         q,
		downloaderModel:       NewDownloaderModel(cfg, q),
		onboardingActive:      !cfg.OnboardingCompleted && flag.Lookup("test.v") == nil,
		onboardingStep:        StepWelcome,
		onboardingTokenInput:  tokenTi,
		onboardingGHTokenInput: ghTokenTi,
		onboardingTokenFocus:   0,
		onboardingChannel:      runner.ChannelStable,
		onboardingBackend:      backends[defaultBackendIdx],
		onboardingBackendIdx:   defaultBackendIdx,
		onboardingBackends:     backends,
		llamaCPPMissingActive: llamaCPPMissing && flag.Lookup("test.v") == nil,
		toasts:                NewToastManager(),
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

func discoverCmd(modelsDirs ...string) tea.Cmd {
	return func() tea.Msg {
		models, err := model.DiscoverModels(modelsDirs...)
		return discoverMsg{models: models, err: err}
	}
}

type startServerMsg struct {
	err error
}

func startServerCmd(srv runner.ModelRuntime, llamaCppDir string, modelPath string, ctxSize uint32, threads int, gpuLayers int, batchSize int, host string, port int, task runner.TaskType) tea.Cmd {
	return func() tea.Msg {
		err := srv.Start(modelPath, runner.StartOptions{
			LlamaCppDir: llamaCppDir,
			ContextSize: ctxSize,
			Threads:     threads,
			GPULayers:   gpuLayers,
			BatchSize:   batchSize,
			Host:        host,
			Port:        port,
			Task:        task,
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
		discoverCmd(m.config.Paths.AllModelDirectories()...),
		m.loadProfilesCmd(),
		detectHardwareCmd,
		tickCmd(),
		m.readDownloadQueueChan(),
	)
}

func (m *BrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if m.onboardingActive {
		// StepTokens has interactive text inputs for GitHub and Hugging Face tokens
		if m.onboardingStep == StepTokens {
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				switch keyMsg.String() {
				case "tab", "down":
					if m.onboardingTokenFocus == 0 {
						m.onboardingTokenFocus = 1
						m.onboardingGHTokenInput.Blur()
						m.onboardingTokenInput.Focus()
					} else {
						m.onboardingTokenFocus = 0
						m.onboardingTokenInput.Blur()
						m.onboardingGHTokenInput.Focus()
					}
					return m, nil
				case "shift+tab", "up":
					if m.onboardingTokenFocus == 1 {
						m.onboardingTokenFocus = 0
						m.onboardingTokenInput.Blur()
						m.onboardingGHTokenInput.Focus()
					} else {
						m.onboardingTokenFocus = 1
						m.onboardingGHTokenInput.Blur()
						m.onboardingTokenInput.Focus()
					}
					return m, nil
				case "enter":
					ghToken := strings.TrimSpace(m.onboardingGHTokenInput.Value())
					hfToken := strings.TrimSpace(m.onboardingTokenInput.Value())
					m.config.GitHubToken = ghToken
					m.config.HFToken = hfToken
					m.config.HuggingFaceToken = hfToken
					if m.downloadQueue != nil {
						m.downloadQueue.UpdateToken(hfToken)
					}
					_ = m.config.Save()
					m.onboardingGHTokenInput.Blur()
					m.onboardingTokenInput.Blur()
					m.onboardingStep++
					return m, nil
				case "esc":
					m.onboardingActive = false
					m.config.OnboardingCompleted = true
					_ = m.config.Save()
					return m, nil
				case "ctrl+v":
					if m.onboardingTokenFocus == 0 {
						pasteFromClipboard(&m.onboardingGHTokenInput)
					} else {
						pasteFromClipboard(&m.onboardingTokenInput)
					}
					return m, nil
				case "ctrl+b":
					m.onboardingGHTokenInput.Blur()
					m.onboardingTokenInput.Blur()
					m.onboardingStep--
					return m, nil
				default:
					var cmd tea.Cmd
					if m.onboardingTokenFocus == 0 {
						m.onboardingGHTokenInput, cmd = m.onboardingGHTokenInput.Update(msg)
					} else {
						m.onboardingTokenInput, cmd = m.onboardingTokenInput.Update(msg)
					}
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				}
			} else {
				var cmd1, cmd2 tea.Cmd
				if m.onboardingTokenFocus == 0 {
					m.onboardingGHTokenInput, cmd1 = m.onboardingGHTokenInput.Update(msg)
				} else {
					m.onboardingTokenInput, cmd2 = m.onboardingTokenInput.Update(msg)
				}
				if cmd1 != nil {
					cmds = append(cmds, cmd1)
				}
				if cmd2 != nil {
					cmds = append(cmds, cmd2)
				}
			}
			// Swallowing input/mouse events during StepTokens so they don't leak
			switch msg.(type) {
			case tea.KeyMsg, tea.MouseMsg:
				return m, tea.Batch(cmds...)
			}
		}

		if m.onboardingStep == StepRuntime {
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				switch keyMsg.String() {
				case "c", "C", "left", "right", "h", "l":
					if m.onboardingChannel == runner.ChannelStable {
						m.onboardingChannel = runner.ChannelNightly
					} else {
						m.onboardingChannel = runner.ChannelStable
					}
					if m.lifecycleModel != nil {
						m.lifecycleModel.SelectedChannel = m.onboardingChannel
					}
				case "a", "A", "up", "k":
					m.onboardingBackendIdx--
					if m.onboardingBackendIdx < 0 {
						m.onboardingBackendIdx = len(m.onboardingBackends) - 1
					}
					m.onboardingBackend = m.onboardingBackends[m.onboardingBackendIdx]
					if m.lifecycleModel != nil {
						m.lifecycleModel.SelectedBackend = m.onboardingBackend
					}
				case "down", "j":
					m.onboardingBackendIdx++
					if m.onboardingBackendIdx >= len(m.onboardingBackends) {
						m.onboardingBackendIdx = 0
					}
					m.onboardingBackend = m.onboardingBackends[m.onboardingBackendIdx]
					if m.lifecycleModel != nil {
						m.lifecycleModel.SelectedBackend = m.onboardingBackend
					}
				case "1":
					m.onboardingBackendIdx = 0
					m.onboardingBackend = m.onboardingBackends[m.onboardingBackendIdx]
					if m.lifecycleModel != nil {
						m.lifecycleModel.SelectedBackend = m.onboardingBackend
					}
				case "2":
					if len(m.onboardingBackends) > 1 {
						m.onboardingBackendIdx = 1
						m.onboardingBackend = m.onboardingBackends[m.onboardingBackendIdx]
						if m.lifecycleModel != nil {
							m.lifecycleModel.SelectedBackend = m.onboardingBackend
						}
					}
				case "3":
					if len(m.onboardingBackends) > 2 {
						m.onboardingBackendIdx = 2
						m.onboardingBackend = m.onboardingBackends[m.onboardingBackendIdx]
						if m.lifecycleModel != nil {
							m.lifecycleModel.SelectedBackend = m.onboardingBackend
						}
					}
				case "4":
					if len(m.onboardingBackends) > 3 {
						m.onboardingBackendIdx = 3
						m.onboardingBackend = m.onboardingBackends[m.onboardingBackendIdx]
						if m.lifecycleModel != nil {
							m.lifecycleModel.SelectedBackend = m.onboardingBackend
						}
					}
				case "enter", "space", "n", "N":
					if m.lifecycleModel != nil {
						m.lifecycleModel.SelectedChannel = m.onboardingChannel
						m.lifecycleModel.SelectedBackend = m.onboardingBackend
					}
					m.onboardingStep++
				case "p", "P", "b", "B":
					m.onboardingStep--
					if m.onboardingStep == StepTokens {
						if m.onboardingTokenFocus == 0 {
							m.onboardingGHTokenInput.Focus()
						} else {
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
					if m.onboardingStep == StepTokens {
						if m.onboardingTokenFocus == 0 {
							m.onboardingGHTokenInput.Focus()
						} else {
							m.onboardingTokenInput.Focus()
						}
					}
				}
			case "p", "P", "b", "B":
				if m.onboardingStep > StepWelcome {
					m.onboardingStep--
					if m.onboardingStep == StepTokens {
						if m.onboardingTokenFocus == 0 {
							m.onboardingGHTokenInput.Focus()
						} else {
							m.onboardingTokenInput.Focus()
						}
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

	if m.themePickerActive && m.themePicker != nil {
		if _, isSizeMsg := msg.(tea.WindowSizeMsg); !isSizeMsg {
			cmd, done, applied, themeID := m.themePicker.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			if done {
				m.themePickerActive = false
				m.themePicker = nil
				if applied {
					m.config.Theme = themeID
					_ = m.config.Save()
					ApplyTheme(themeID)
					if m.toasts != nil {
						cmds = append(cmds, m.toasts.ShowSuccess(fmt.Sprintf("Theme: %s applied", ActiveTheme.Name())))
					}
				} else {
					ApplyTheme(m.config.Theme)
				}
			}
			return m, tea.Batch(cmds...)
		}
	}

	if m.screenMode == ScreenLogStreamer && m.logStreamerModel != nil {
		if _, isSizeMsg := msg.(tea.WindowSizeMsg); !isSizeMsg {
			var cmd tea.Cmd
			m.logStreamerModel, cmd = m.logStreamerModel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			if m.logStreamerModel.Closed() {
				m.screenMode = m.logStreamerModel.PrevScreen()
				m.logStreamerModel = nil
			}
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
				savedProfileName := ""
				if m.profileCreatorModel != nil {
					savedProfileName = m.profileCreatorModel.nameInput.Value()
				}
				m.profileCreatorModel = nil
				if saved {
					// Reload profiles from config paths
					profs, err := profile.LoadAll(m.config.Paths.Profiles)
					if err == nil {
						m.profiles = profs
						// Re-initialize dashboard with the newly added profile selected
						if m.dashboard != nil {
							m.dashboard.Profiles = profs
							foundIdx := len(profs) - 1
							for i, p := range profs {
								if p.Name == savedProfileName {
									foundIdx = i
									break
								}
							}
							m.dashboard.ActiveIdx = foundIdx
							m.dashboard.SetToast("✓ Profile saved successfully!")
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

	case ToastExpireMsg:
		if m.toasts != nil {
			m.toasts.Update(msg)
		}

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
			if m.hardwareSpecs != nil {
				if m.hardwareSpecs.IsUnified || runtime.GOOS == "darwin" {
					m.onboardingBackendIdx = 2
					m.onboardingBackend = runner.BackendMetal
				} else if strings.Contains(strings.ToLower(m.hardwareSpecs.GPU.Name), "nvidia") || m.hardwareSpecs.GPU.Type == "CUDA" {
					m.onboardingBackendIdx = 0
					m.onboardingBackend = runner.BackendCUDA12
				} else if m.hardwareSpecs.GPU.VRAM > 0 {
					m.onboardingBackendIdx = 1
					m.onboardingBackend = runner.BackendVulkan
				} else {
					m.onboardingBackendIdx = 3
					m.onboardingBackend = runner.BackendCPU
				}
			}
		}

	case profilesMsg:
		if msg.err == nil {
			m.profiles = msg.profiles
			if m.monitorModel != nil {
				m.monitorModel.SetProfiles(msg.profiles)
			}
			m.rebuildSidebar()
		}

	case OpenLogStreamerMsg:
		m.logStreamerModel = NewLogStreamerModel(m.srvRunner, msg.PrevScreen, msg.Port)
		m.screenMode = ScreenLogStreamer
		cmds = append(cmds, m.logStreamerModel.Init())

	case LogStreamTickMsg:
		if m.screenMode == ScreenLogStreamer && m.logStreamerModel != nil {
			var cmd tea.Cmd
			m.logStreamerModel, cmd = m.logStreamerModel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case LogStreamDataMsg:
		if m.logStreamerModel != nil {
			var cmd tea.Cmd
			m.logStreamerModel, cmd = m.logStreamerModel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case downloadQueueMsg:
		if m.downloaderModel != nil {
			_, cmd := m.downloaderModel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if msg.task != nil && msg.task.Status == model.StatusCompleted {
			cmds = append(cmds, discoverCmd(m.config.Paths.AllModelDirectories()...))
		}
		cmds = append(cmds, m.readDownloadQueueChan())

	case hfResolveMsg:
		if m.downloaderModel != nil {
			_, cmd := m.downloaderModel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case MonitorTickMsg:
		if m.screenMode == ScreenServerMonitor && m.monitorModel != nil {
			cmd := m.monitorModel.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case MonitorMetricsMsg:
		if m.monitorModel != nil {
			cmd := m.monitorModel.Update(msg)
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

	case appUpdateMsg:
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
		if m.toasts != nil {
			m.toasts.PruneExpired()
		}
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
			case "esc":
				m.screenMode = ScreenBrowser
			case "c", "C":
				_ = m.dashboard.CopyCommandToClipboard()
				if m.toasts != nil {
					cmds = append(cmds, m.toasts.ShowSuccess("Command copied to clipboard!"))
				}
			case "p", "P":
				m.profileCreatorModel = NewProfileCreatorModel(m.config.Paths.Profiles)
				m.screenMode = ScreenProfileCreator
			case "e", "E":
				p := m.dashboard.ActiveProfile()
				if p != nil {
					m.profileCreatorModel = NewProfileEditorModel(m.config.Paths.Profiles, p, false)
					m.screenMode = ScreenProfileCreator
				}
			case "n", "N":
				p := m.dashboard.ActiveProfile()
				if p != nil {
					m.profileCreatorModel = NewProfileEditorModel(m.config.Paths.Profiles, p, true)
					m.screenMode = ScreenProfileCreator
				}
			case "d", "D":
				p := m.dashboard.ActiveProfile()
				if p != nil {
					if profile.IsDefaultProfile(p.Name) {
						m.dashboard.SetToast("Cannot delete built-in default profile: " + p.Name)
						if m.toasts != nil {
							cmds = append(cmds, m.toasts.ShowWarning("Cannot delete default profile"))
						}
					} else {
						if err := profile.DeleteProfile(m.config.Paths.Profiles, p.Name); err != nil {
							m.dashboard.SetToast(fmt.Sprintf("Failed to delete profile: %v", err))
							if m.toasts != nil {
								cmds = append(cmds, m.toasts.ShowDanger("Failed to delete profile"))
							}
						} else {
							profs, _ := profile.LoadAll(m.config.Paths.Profiles)
							m.profiles = profs
							m.dashboard.Profiles = profs
							if m.dashboard.ActiveIdx >= len(profs) {
								m.dashboard.ActiveIdx = max(0, len(profs)-1)
							}
							m.dashboard.SetToast(fmt.Sprintf("✓ Deleted custom profile '%s'", p.Name))
							if m.toasts != nil {
								cmds = append(cmds, m.toasts.ShowSuccess(fmt.Sprintf("Deleted profile '%s'", p.Name)))
							}
						}
					}
				}
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

					var taskType runner.TaskType
					if m.dashboard.Model != nil {
						taskType = runner.TaskType(m.dashboard.Model.Task)
					}
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
						taskType,
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
			case "up", "k":
				if m.perfDashboard.Cursor > 0 {
					m.perfDashboard.Cursor--
				}
			case "down", "j":
				if len(m.perfDashboard.History) > 0 && m.perfDashboard.Cursor < len(m.perfDashboard.History)-1 {
					m.perfDashboard.Cursor++
				}
			}
		} else if m.screenMode == ScreenServerMonitor && m.monitorModel != nil {
			switch msg.String() {
			case "esc", "c", "C":
				m.screenMode = ScreenBrowser
				m.rebuildSidebar()
			case "l", "L":
				var targetPort int
				if len(m.monitorModel.instances) > 0 && m.monitorModel.selected >= 0 && m.monitorModel.selected < len(m.monitorModel.instances) {
					targetPort = m.monitorModel.instances[m.monitorModel.selected].Port
				}
				m.logStreamerModel = NewLogStreamerModel(m.srvRunner, ScreenServerMonitor, targetPort)
				m.screenMode = ScreenLogStreamer
				cmds = append(cmds, m.logStreamerModel.Init())
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
				case "1":
					m.lifecycleModel.SelectedRuntime = 0
				case "2":
					m.lifecycleModel.SelectedRuntime = 1
				case "3":
					m.lifecycleModel.SelectedRuntime = 2
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
					m.themePicker = NewThemePickerModel(m.config.Theme)
					m.themePickerActive = true
				case "s", "S", "b", "B", "t", "T", "g", "G":
					_, cmd := m.lifecycleModel.Update(msg)
					if cmd != nil {
						cmds = append(cmds, cmd)
					}
				case "v", "V":
					if m.lifecycleModel.SelectedRuntime == 2 {
						if !m.lifecycleModel.appChecking {
							cmds = append(cmds, m.lifecycleModel.StartAppCheck())
						}
					} else {
						_, cmd := m.lifecycleModel.Update(msg)
						if cmd != nil {
							cmds = append(cmds, cmd)
						}
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

			case "right":
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
				if m.toasts != nil {
					cmds = append(cmds, m.toasts.ShowWarning("Model server stopped"))
				}

			case "ctrl+s":
				m.serverUIStatus = UIStatusStopped
				m.serverErr = nil
				_ = m.srvRunner.Stop()
				if m.toasts != nil {
					cmds = append(cmds, m.toasts.ShowWarning("All servers stopped"))
				}

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
						if m.toasts != nil {
							cmds = append(cmds, m.toasts.Show(fmt.Sprintf("Task set to: %s", nextTask)))
						}
					}
				}

			case "f", "F":
				if m.selected >= 0 && m.selected < len(m.sidebarItems) {
					item := m.sidebarItems[m.selected]
					if item.Type == ItemModelEntry {
						m.config.ToggleFavorite(item.ModelPath)
						_ = m.config.Save()
						m.rebuildSidebar()
						if m.toasts != nil {
							if m.config.IsFavorite(item.ModelPath) {
								cmds = append(cmds, m.toasts.ShowSuccess(fmt.Sprintf("★ Added %s to favorites", m.models[item.ModelIdx].Name)))
							} else {
								cmds = append(cmds, m.toasts.Show(fmt.Sprintf("Removed %s from favorites", m.models[item.ModelIdx].Name)))
							}
						}
					}
				}

			case "y", "Y":
				m.themePicker = NewThemePickerModel(m.config.Theme)
				m.themePickerActive = true

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
						if m.toasts != nil {
							cmds = append(cmds, m.toasts.Show(fmt.Sprintf("Benchmarking %s...", selectedModel.Name)))
						}
					}
				}

			case "v", "V":
				history, err := benchmark.LoadHistory(m.config.Paths.Benchmarks)
				if err == nil {
					m.perfDashboard = NewPerformanceDashboardModel(history)
					m.screenMode = ScreenPerformanceDashboard
				}

			case "m", "M":
				m.screenMode = ScreenServerMonitor
				cmds = append(cmds, m.monitorModel.PollMetricsCmd(), MonitorTickCmd())

			case "l", "L":
				var targetPort int
				if m.selected >= 0 && m.selected < len(m.sidebarItems) {
					item := m.sidebarItems[m.selected]
					if item.Type == ItemModelEntry {
						for _, inst := range m.srvRunner.GetAllInstances() {
							if inst.ModelPath == item.ModelPath {
								targetPort = inst.Port
								break
							}
						}
					}
				}
				if targetPort == 0 {
					instances := m.srvRunner.GetAllInstances()
					if len(instances) > 0 {
						targetPort = instances[0].Port
					}
				}
				m.logStreamerModel = NewLogStreamerModel(m.srvRunner, ScreenBrowser, targetPort)
				m.screenMode = ScreenLogStreamer
				cmds = append(cmds, m.logStreamerModel.Init())

			case "u", "U":
				m.lifecycleModel.RefreshLocalVersion()
				m.lifecycleModel.RefreshBackupStatus()
				m.screenMode = ScreenSettings

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
	if selectedModel.ShardCount > 1 {
		sb.WriteString(fmt.Sprintf("  %-16s %s (%d shards)\n", "File Size:", formatSize(selectedModel.FileSize), selectedModel.ShardCount))
		sb.WriteString(fmt.Sprintf("  %-16s %d shards (multi-file GGUF)\n", "Shards:", selectedModel.ShardCount))
	} else {
		sb.WriteString(fmt.Sprintf("  %-16s %s\n", "File Size:", formatSize(selectedModel.FileSize)))
	}
	sb.WriteString(fmt.Sprintf("  %-16s %s\n", "Runtime:", selectedModel.Runtime))
	sb.WriteString(fmt.Sprintf("  %-16s %s (Press [E] to cycle)\n\n", "Task Type:", selectedModel.Task))




	// Memory Estimates
	if m.hardwareSpecs != nil {
		est := hardware.EstimateMemory(selectedModel, m.hardwareSpecs, 0)
		var suitStr string
		var suitabilityColor lipgloss.TerminalColor
		switch est.Suitability {
		case hardware.SuitabilityFitsVRAM:
			suitStr = StyleBadgeFits.Render(" FITS VRAM ")
			suitabilityColor = ColorSecondary
		case hardware.SuitabilityPartialVRAM:
			suitStr = StyleBadgePartial.Render(" PARTIAL VRAM ")
			suitabilityColor = ColorGold
		case hardware.SuitabilityFitsRAM:
			suitStr = StyleBadgeFits.Render(" FITS RAM (CPU) ")
			suitabilityColor = ColorSecondary
		case hardware.SuitabilityExceeds:
			suitStr = StyleBadgeExceeds.Render(" EXCEEDS RAM ")
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
		if est.ActivationSize > 0 {
			sb.WriteString(fmt.Sprintf("  %-16s %s\n", "Activation:", formatSize(int64(est.ActivationSize))))
		}
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
		sb.WriteString(fmt.Sprintf("\n  %s\n", StyleDanger.Render(fmt.Sprintf("Error: %v", m.serverErr))))
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
	var baseView string

	if m.loading {
		baseView = "\n  Scanning models directory... Please wait."
	} else if m.err != nil {
		baseView = fmt.Sprintf("\n  Error: %v\n  Press Q to quit.", m.err)
	} else if m.onboardingActive {
		onboardingOverlay := m.onboardingOverlayView(m.width, m.height)
		baseView = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, onboardingOverlay)
	} else if m.llamaCPPMissingActive {
		missingOverlay := m.llamaCPPMissingOverlayView(m.width, m.height)
		baseView = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, missingOverlay)
	} else if m.screenMode == ScreenDashboard && m.dashboard != nil {
		dashView := m.dashboard.View(m.width, m.height)
		baseView = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dashView)
	} else if m.screenMode == ScreenBenchmarkProgress && m.benchmarkProgress != nil {
		progressView := m.benchmarkProgress.View(m.width, m.height)
		baseView = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, progressView)
	} else if m.screenMode == ScreenPerformanceDashboard && m.perfDashboard != nil {
		perfView := m.perfDashboard.View(m.width, m.height)
		baseView = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, perfView)
	} else if m.screenMode == ScreenServerMonitor && m.monitorModel != nil {
		monitorView := m.monitorModel.View(m.width, m.height)
		baseView = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, monitorView)
	} else if m.screenMode == ScreenSettings && m.lifecycleModel != nil {
		settingsView := m.lifecycleModel.View(m.width, m.height)
		baseView = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, settingsView)
	} else if m.screenMode == ScreenDownloader && m.downloaderModel != nil {
		downView := m.downloaderModel.View(m.width, m.height)
		baseView = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, downView)
	} else if m.screenMode == ScreenProfileCreator && m.profileCreatorModel != nil {
		creatorView := m.profileCreatorModel.View(m.width, m.height)
		baseView = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, creatorView)
	} else if m.screenMode == ScreenLogStreamer && m.logStreamerModel != nil {
		logView := m.logStreamerModel.View(m.width, m.height)
		baseView = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, logView)
	} else {
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
					selWidth := leftWidth - 2
					if selWidth < 10 {
						selWidth = 10
					}
					if idx == m.selected {
						leftSb.WriteString(
							StyleSelectedListItem.Width(selWidth).Render(
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
					case hardware.SuitabilityFitsVRAM:
						bulletStyled = StyleSuccess.Render(bullet)
					case hardware.SuitabilityPartialVRAM:
						bulletStyled = StyleWarning.Render(bullet)
					case hardware.SuitabilityFitsRAM:
						bulletStyled = lipgloss.NewStyle().Foreground(ColorSecondary).Render(bullet)
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

				rawName := item.Label
				isFavorite := m.config.IsFavorite(mod.FilePath)

				indent := ""
				if strings.HasPrefix(rawName, "  ") {
					indent = "  "
					rawName = strings.TrimPrefix(rawName, "  ")
				}

				maxNameLen := leftWidth - 10
				if isFavorite {
					maxNameLen -= 2
				}
				if maxNameLen < 4 {
					maxNameLen = 4
				}
				rawName = TruncateVisual(rawName, maxNameLen, "...")

				var displayName string
				if isFavorite {
					displayName = indent + StyleStar.Render("★ ") + rawName
				} else {
					displayName = indent + rawName
				}

				selWidth := leftWidth - 2
				if selWidth < 10 {
					selWidth = 10
				}
				if idx == m.selected {
					leftSb.WriteString(
						StyleSelectedListItem.Width(selWidth).Render(
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
				fmt.Sprintf("%s Logs", StyleHelpKey.Render("[L]")),
				fmt.Sprintf("%s Settings", StyleHelpKey.Render("[U]")),
				fmt.Sprintf("%s Downloader", StyleHelpKey.Render("[D]")),
				fmt.Sprintf("%s Themes", StyleHelpKey.Render("[Y]")),
				fmt.Sprintf("%s Search", StyleHelpKey.Render("[/]")),
				fmt.Sprintf("%s Stop", StyleHelpKey.Render("[S]")),
				fmt.Sprintf("%s Quit", StyleHelpKey.Render("[Q]")),
			}
			footer = strings.Join(footerItems, " │ ")
		}

		baseView = lipgloss.JoinVertical(lipgloss.Left, header, mainView, StyleHelp.Render(footer))
	}

	if m.themePickerActive && m.themePicker != nil {
		pickerView := m.themePicker.View(m.width, m.height)
		baseView = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, pickerView)
	}

	if m.toasts != nil && m.toasts.Active() {
		return m.toasts.Overlay(baseView, m.width, m.height)
	}
	return baseView
}

func (m *BrowserModel) onboardingOverlayView(width int, height int) string {
	var sb strings.Builder
	sb.WriteString("\n")

	boxWidth := 86
	if width > 0 && boxWidth > width-6 {
		boxWidth = width - 6
	}
	if boxWidth < 40 {
		boxWidth = 40
	}

	var stepTitle, stepSub, stepIndicator string
	switch m.onboardingStep {
	case StepWelcome:
		stepIndicator = "Step 1 of 5  ● ○ ○ ○ ○"
		stepTitle = "WELCOME TO RUNORA"
		stepSub = "Runora is your local AI control center for managing, profiling, and\n  running high-performance GGUF & ONNX models completely offline."

		sb.WriteString(fmt.Sprintf("  %s  %s\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(stepTitle), StyleMuted.Render(stepIndicator)))
		sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorWhite).Render(stepSub)))

		sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Bold(true).Render("◆ Detected System Hardware:")))
		if m.hardwareSpecs != nil {
			osStr := m.hardwareSpecs.OS
			if osStr == "" {
				osStr = runtime.GOOS
			}
			cpuModel := m.hardwareSpecs.CPU.Model
			if cpuModel == "" {
				cpuModel = "Detected CPU"
			}
			cores := m.hardwareSpecs.CPU.PhysicalCores
			threads := m.hardwareSpecs.CPU.Threads
			var coreInfo string
			if cores > 0 && threads > 0 {
				coreInfo = fmt.Sprintf("%d cores (%d threads)", cores, threads)
			} else if threads > 0 {
				coreInfo = fmt.Sprintf("%d threads", threads)
			} else {
				coreInfo = "Multi-core"
			}
			cpuStr := fmt.Sprintf("%s (%s)", cpuModel, coreInfo)

			ramStr := fmt.Sprintf("%s total (%s available)", formatSize(int64(m.hardwareSpecs.RAM.Total)), formatSize(int64(m.hardwareSpecs.RAM.Available)))
			if m.hardwareSpecs.RAM.Total == 0 {
				ramStr = "System RAM detected"
			}

			gpuName := m.hardwareSpecs.GPU.Name
			if gpuName == "" {
				gpuName = "Integrated / Host GPU"
			}
			var vramStr string
			if m.hardwareSpecs.IsUnified {
				vramStr = fmt.Sprintf("%s (Apple Silicon Unified Memory)", formatSize(int64(m.hardwareSpecs.GPU.VRAM)))
			} else if m.hardwareSpecs.GPU.VRAM > 0 {
				vramStr = fmt.Sprintf("%s VRAM", formatSize(int64(m.hardwareSpecs.GPU.VRAM)))
			} else {
				vramStr = "Shared System Memory"
			}

			sb.WriteString(fmt.Sprintf("  %-16s %s\n", "OS Platform:", osStr))
			sb.WriteString(fmt.Sprintf("  %-16s %s\n", "CPU Processor:", cpuStr))
			sb.WriteString(fmt.Sprintf("  %-16s %s\n", "System RAM:", ramStr))
			sb.WriteString(fmt.Sprintf("  %-16s %s\n", "GPU Device:", gpuName))
			sb.WriteString(fmt.Sprintf("  %-16s %s\n\n", "VRAM / Memory:", vramStr))
		} else {
			sb.WriteString("  Detecting hardware specifications...\n\n")
		}
		sb.WriteString("  Runora automatically benchmarks and tailors model context limits to your VRAM.\n\n")

	case StepStorage:
		stepIndicator = "Step 2 of 5  ● ● ○ ○ ○"
		stepTitle = "MODEL STORAGE & DIRECTORIES"
		stepSub = "Runora discovers and manages models directly from your local filesystem."

		sb.WriteString(fmt.Sprintf("  %s  %s\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(stepTitle), StyleMuted.Render(stepIndicator)))
		sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorWhite).Render(stepSub)))

		sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Bold(true).Render("◆ Configured Storage Locations:")))
		sb.WriteString(fmt.Sprintf("  %-18s %s\n", "Primary Models:", m.config.Paths.Models))
		sb.WriteString(fmt.Sprintf("  %-18s %s\n", "Model Cache:", m.config.Paths.Cache))
		sb.WriteString(fmt.Sprintf("  %-18s %s\n", "Profiles Path:", m.config.Paths.Profiles))
		sb.WriteString(fmt.Sprintf("  %-18s %s\n\n", "Downloads Path:", m.config.Paths.Downloads))

		sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Bold(true).Render("◆ Storage Architecture:")))
		sb.WriteString("  ● Recursive Discovery: Subfolders (e.g. models/llm, models/vlm) are indexed automatically.\n")
		sb.WriteString("  ● Zero Weight Loading: GGUF headers are parsed in milliseconds without consuming VRAM.\n")
		sb.WriteString("  ● Custom Directories:  Add secondary model folders at any time in Settings [U].\n\n")

	case StepTokens:
		stepIndicator = "Step 3 of 5  ● ● ● ○ ○"
		stepTitle = "API TOKENS CONFIGURATION"
		stepSub = "Configure optional API credentials for unlimited releases and gated model access."

		sb.WriteString(fmt.Sprintf("  %s  %s\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(stepTitle), StyleMuted.Render(stepIndicator)))
		sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorWhite).Render(stepSub)))

		ghPrefix := "  [1] "
		hfPrefix := "  [2] "
		ghLabel := "GitHub Token (Optional - increases engine download rate limits):"
		hfLabel := "Hugging Face Token (Optional - downloads gated models e.g. Llama-3, Gemma):"

		if m.onboardingTokenFocus == 0 {
			sb.WriteString(fmt.Sprintf("%s%s\n", ghPrefix, lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(ghLabel)))
			sb.WriteString(fmt.Sprintf("      %s\n\n", m.onboardingGHTokenInput.View()))
			sb.WriteString(fmt.Sprintf("%s%s\n", hfPrefix, StyleMuted.Render(hfLabel)))
			sb.WriteString(fmt.Sprintf("      %s\n\n", m.onboardingTokenInput.View()))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s\n", ghPrefix, StyleMuted.Render(ghLabel)))
			sb.WriteString(fmt.Sprintf("      %s\n\n", m.onboardingGHTokenInput.View()))
			sb.WriteString(fmt.Sprintf("%s%s\n", hfPrefix, lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(hfLabel)))
			sb.WriteString(fmt.Sprintf("      %s\n\n", m.onboardingTokenInput.View()))
		}

		sb.WriteString("  ● Press [Tab / Up / Down] to switch between token input fields.\n")
		sb.WriteString("  ● Press [Ctrl+V] to paste from clipboard (masked with * for security).\n")
		sb.WriteString("  ● Tokens are saved locally to config.json and never transmitted externally.\n\n")

	case StepRuntime:
		stepIndicator = "Step 4 of 5  ● ● ● ● ○"
		stepTitle = "RUNTIME ENGINE & ACCELERATOR"
		stepSub = "Configure the llama.cpp inference engine release channel and accelerator backend."

		sb.WriteString(fmt.Sprintf("  %s  %s\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(stepTitle), StyleMuted.Render(stepIndicator)))
		sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorWhite).Render(stepSub)))

		sb.WriteString("  ◆ Release Channel (Press [C] or [Left/Right] to toggle):\n")
		if m.onboardingChannel == runner.ChannelStable {
			sb.WriteString(fmt.Sprintf("    %s %-10s - Recommended, battle-tested official releases\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("[*]"), lipgloss.NewStyle().Bold(true).Render("Stable")))
			sb.WriteString(fmt.Sprintf("    %s %-10s - Bleeding-edge builds with latest architecture support\n\n", StyleMuted.Render("[ ]"), "Nightly"))
		} else {
			sb.WriteString(fmt.Sprintf("    %s %-10s - Recommended, battle-tested official releases\n", StyleMuted.Render("[ ]"), "Stable"))
			sb.WriteString(fmt.Sprintf("    %s %-10s - Bleeding-edge builds with latest architecture support\n\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("[*]"), lipgloss.NewStyle().Bold(true).Render("Nightly")))
		}

		sb.WriteString("  ◆ Hardware Accelerator (Press [A] or [Up/Down] to select):\n")
		backendDescriptions := []struct {
			backend runner.BackendType
			name    string
			desc    string
		}{
			{runner.BackendCUDA12, "CUDA", "NVIDIA GeForce, RTX, and Tesla GPUs (CUDA 12.x / 13.x)"},
			{runner.BackendVulkan, "Vulkan", "AMD Radeon, Intel Arc, and universal cross-vendor GPU acceleration"},
			{runner.BackendMetal, "Metal", "Apple Silicon M-Series unified memory GPU (macOS)"},
			{runner.BackendCPU, "CPU", "Universal x86_64 / ARM64 CPU execution (AVX2 / AVX-512)"},
		}

		for i, b := range backendDescriptions {
			if m.onboardingBackendIdx == i || m.onboardingBackend == b.backend {
				sb.WriteString(fmt.Sprintf("    %s %-8s - %s\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("[*]"), lipgloss.NewStyle().Bold(true).Render(b.name), b.desc))
			} else {
				sb.WriteString(fmt.Sprintf("    %s %-8s - %s\n", StyleMuted.Render("[ ]"), b.name, StyleMuted.Render(b.desc)))
			}
		}
		sb.WriteString("\n")

	case StepFinished:
		stepIndicator = "Step 5 of 5  ● ● ● ● ●"
		stepTitle = "SETUP COMPLETED"
		stepSub = "Runora is ready! Use the following keyboard shortcuts to manage local AI:"

		sb.WriteString(fmt.Sprintf("  %s  %s\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(stepTitle), StyleMuted.Render(stepIndicator)))
		sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorWhite).Render(stepSub)))

		sb.WriteString("  ◆ Quick Navigation Guide:\n")
		sb.WriteString(fmt.Sprintf("    ● %-22s Browse models and view parsed GGUF metadata\n", StyleHelpKey.Render("[Up / Down / j / k]")))
		sb.WriteString(fmt.Sprintf("    ● %-22s Open Launch Dashboard & choose inference profile\n", StyleHelpKey.Render("[Enter]")))
		sb.WriteString(fmt.Sprintf("    ● %-22s Model Downloader (Hugging Face repo browser & direct URL)\n", StyleHelpKey.Render("[D]")))
		sb.WriteString(fmt.Sprintf("    ● %-22s Lifecycle Manager & Engine Settings (updates / rollback)\n", StyleHelpKey.Render("[U]")))
		sb.WriteString(fmt.Sprintf("    ● %-22s Live Server Monitor & Resource Telemetry\n", StyleHelpKey.Render("[M]")))
		sb.WriteString(fmt.Sprintf("    ● %-22s Real-time Server Log Streamer\n", StyleHelpKey.Render("[L]")))
		sb.WriteString(fmt.Sprintf("    ● %-22s Theme Picker & Visual Color Schemes\n", StyleHelpKey.Render("[Y]")))
		sb.WriteString(fmt.Sprintf("    ● %-22s Filter and search models by name or tag\n", StyleHelpKey.Render("[/]")))
		sb.WriteString(fmt.Sprintf("    ● %-22s Toggle Help & Command Reference\n\n", StyleHelpKey.Render("[?]")))

		sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("[ Enter / Space ] Start Exploring Runora ->")))
	}

	// Navigation instructions footer
	var navHelp string
	if m.onboardingStep == StepWelcome {
		navHelp = fmt.Sprintf("  %s Next  %s Skip Setup", StyleHelpKey.Render("[Enter/Space]"), StyleHelpKey.Render("[Esc]"))
	} else if m.onboardingStep == StepTokens {
		navHelp = fmt.Sprintf("  %s Save & Next  %s Back  %s Skip Setup", StyleHelpKey.Render("[Enter]"), StyleHelpKey.Render("[Ctrl+B]"), StyleHelpKey.Render("[Esc]"))
	} else if m.onboardingStep == StepFinished {
		navHelp = fmt.Sprintf("  %s Finish Setup  %s Back", StyleHelpKey.Render("[Enter/Space/Esc]"), StyleHelpKey.Render("[P/B]"))
	} else {
		navHelp = fmt.Sprintf("  %s Next  %s Back  %s Skip Setup", StyleHelpKey.Render("[Enter/Space]"), StyleHelpKey.Render("[P/B]"), StyleHelpKey.Render("[Esc]"))
	}
	sb.WriteString(navHelp + "\n")

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
