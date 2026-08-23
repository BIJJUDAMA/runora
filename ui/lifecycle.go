package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/config"
	"github.com/BIJJUDAMA/runora/hardware"
	"github.com/BIJJUDAMA/runora/runner"
)

type LifecycleState int

const (
	StateIdle LifecycleState = iota
	StateChecking
	StateNoUpdate
	StateUpdateAvailable
	StateDownloading
	StateExtracting
	StateVerifying
	StateUpdateSuccess
	StateRollingBack
	StateRollbackSuccess
	StateError
)

type updateMsg struct {
	target   string // "llamacpp", "onnx", "app"
	state    LifecycleState
	progress float64
	msg      string
	err      error
	release  *runner.GithubRelease
	ch       chan updateMsg
}

type appCheckMsg struct {
	latestTag string
	err       error
}

type LifecycleModel struct {
	srvRunner        runner.ModelRuntime
	config           *config.Config
	specs            *hardware.HardwareSpecs
	state            LifecycleState
	localVersion     string
	localCommit      string
	localBuildInfo   string
	latestTagName    string
	latestRelease    *runner.GithubRelease
	matchedAsset     *runner.ReleaseAsset
	matchedCudart    *runner.ReleaseAsset
	downloadProgress float64
	actionMsg        string
	err              error
	hasBackup        bool
	hasLlamaBackup   bool
	hasOnnxBackup    bool
	width, height    int
	tokenInput       textinput.Model
	tokenEditActive  bool
	tokenEditTarget  string // "hf" or "github"
	// App self-update fields
	appVersion       string
	appLatestTag     string
	appCheckErr      error
	appChecking      bool
	appUpdating      bool
	appUpdateErr     error
	appUpdateSuccess bool
	appUpdateMsg     string

	// ONNX Runtime fields
	onnxLocalVersion  string
	onnxLatestVersion string
	onnxLatestRelease *runner.GithubRelease
	updatingRuntime   string // "llamacpp", "onnx", or "app"

	// Selected runtime for unified controls (0: llama.cpp, 1: ONNX, 2: Runora App)
	SelectedRuntime int

	// Release Channel & Backend Accelerator selection
	SelectedChannel   runner.ReleaseChannel
	SelectedBackend   runner.BackendType
	installedVersions []string
	activeSlot        string
}

func resolveAppVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return strings.TrimSuffix(v, "+dirty")
		}
	}
	return "dev"
}

func NewLifecycleModel(cfg *config.Config, srv runner.ModelRuntime) *LifecycleModel {
	specs, _ := hardware.DetectHardware()
	if specs == nil {
		specs = &hardware.HardwareSpecs{OS: runtime.GOOS}
	}

	ti := textinput.New()
	ti.Placeholder = "Enter HF_TOKEN (hf_...)"
	ti.CharLimit = 100
	ti.Width = 40
	ti.EchoMode = textinput.EchoPassword

	runner.SetGitHubToken(cfg.GitHubToken)

	m := &LifecycleModel{
		srvRunner:         srv,
		config:            cfg,
		specs:             specs,
		state:             StateIdle,
		tokenInput:        ti,
		tokenEditActive:   false,
		tokenEditTarget:   "hf",
		appVersion:        resolveAppVersion(),
		SelectedRuntime:   0,
		SelectedChannel:   runner.ChannelStable,
		SelectedBackend:   runner.BackendAuto,
		installedVersions: []string{},
	}
	m.RefreshLocalVersion()
	m.RefreshBackupStatus()
	m.RefreshVersionSlots()
	return m
}

func (m *LifecycleModel) ToggleChannel() {
	if m.SelectedChannel == runner.ChannelStable {
		m.SelectedChannel = runner.ChannelNightly
	} else {
		m.SelectedChannel = runner.ChannelStable
	}
	m.latestTagName = ""
	m.latestRelease = nil
	m.actionMsg = fmt.Sprintf("Switched to %s channel. Press [C/Enter] to check.", strings.Title(string(m.SelectedChannel)))
}

func (m *LifecycleModel) CycleBackend() {
	backends := []runner.BackendType{
		runner.BackendAuto,
		runner.BackendCUDA12,
		runner.BackendCUDA13,
		runner.BackendVulkan,
		runner.BackendCPU,
		runner.BackendROCm,
		runner.BackendMetal,
	}
	idx := 0
	for i, b := range backends {
		if b == m.SelectedBackend {
			idx = i
			break
		}
	}
	m.SelectedBackend = backends[(idx+1)%len(backends)]
	m.actionMsg = fmt.Sprintf("Selected target backend accelerator: %s", strings.ToUpper(string(m.SelectedBackend)))
}

func (m *LifecycleModel) RefreshVersionSlots() {
	slots, err := runner.ListInstalledVersions(m.config.Paths.LlamaCPP)
	if err == nil {
		m.installedVersions = slots
	} else {
		m.installedVersions = []string{}
	}
	active, _ := runner.GetActiveVersion(m.config.Paths.LlamaCPP)
	m.activeSlot = active
}

func (m *LifecycleModel) CycleVersionSlot() error {
	if len(m.installedVersions) == 0 {
		return fmt.Errorf("no version slots installed in llama.cpp/versions/")
	}
	idx := -1
	for i, v := range m.installedVersions {
		if v == m.activeSlot || v == m.localVersion {
			idx = i
			break
		}
	}
	nextIdx := (idx + 1) % len(m.installedVersions)
	nextVer := m.installedVersions[nextIdx]
	if nextVer == m.activeSlot && len(m.installedVersions) == 1 {
		m.actionMsg = fmt.Sprintf("Slot %s is already active.", nextVer)
		return nil
	}

	instances := m.srvRunner.GetAllInstances()
	if len(instances) > 0 {
		return fmt.Errorf("cannot switch version: active server instances are running. Stop servers first.")
	}

	err := runner.SwitchActiveVersion(m.config.Paths.LlamaCPP, nextVer)
	if err != nil {
		return err
	}
	m.RefreshLocalVersion()
	m.RefreshVersionSlots()
	m.actionMsg = fmt.Sprintf("Switched active llama.cpp version to %s", nextVer)
	return nil
}

// StartAppCheck queries GitHub for the latest llama-manager release tag.
func (m *LifecycleModel) StartAppCheck() tea.Cmd {
	m.appChecking = true
	m.appCheckErr = nil
	m.updatingRuntime = "app"
	return func() tea.Msg {
		rel, err := runner.CheckAppRelease()
		if err != nil {
			return appCheckMsg{err: err}
		}
		return appCheckMsg{latestTag: rel.TagName}
	}
}

type appUpdateMsg struct {
	err error
	msg string
}

// StartAppUpdate runs go install to update the app.
func (m *LifecycleModel) StartAppUpdate() tea.Cmd {
	m.appUpdating = true
	m.appUpdateErr = nil
	m.appUpdateSuccess = false
	m.updatingRuntime = "app"
	return func() tea.Msg {
		cmd := exec.Command("go", "install", "github.com/BIJJUDAMA/runora/cmd/runora@latest")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return appUpdateMsg{err: fmt.Errorf("failed to run go install: %w (output: %s)", err, string(output))}
		}
		return appUpdateMsg{msg: "Update successful! Please restart the application."}
	}
}

func (m *LifecycleModel) RefreshLocalVersion() {
	version, commit, buildInfo, err := runner.QueryLocalVersion(m.config.Paths.LlamaCPP)
	if err == nil {
		m.localVersion = version
		m.localCommit = commit
		m.localBuildInfo = buildInfo
	} else {
		m.localVersion = "Not Installed"
		m.localCommit = "N/A"
		m.localBuildInfo = "N/A"
	}

	onnxVer, err := runner.QueryLocalOnnxVersion(m.config.Paths.OnnxRuntime)
	if err == nil {
		m.onnxLocalVersion = onnxVer
	} else {
		m.onnxLocalVersion = "Not Installed"
	}
}

func (m *LifecycleModel) RefreshBackupStatus() {
	llamaBackup := m.config.Paths.LlamaCPP + ".backup"
	onnxBackup := m.config.Paths.OnnxRuntime + ".backup"
	m.hasLlamaBackup = false
	m.hasOnnxBackup = false
	if _, err := os.Stat(llamaBackup); err == nil {
		m.hasLlamaBackup = true
	}
	if _, err := os.Stat(onnxBackup); err == nil {
		m.hasOnnxBackup = true
	}
	m.hasBackup = m.hasLlamaBackup || m.hasOnnxBackup
}

func (m *LifecycleModel) StartCheckOnly() tea.Cmd {
	ch := make(chan updateMsg)

	go func() {
		channelName := strings.Title(string(m.SelectedChannel))
		ch <- updateMsg{target: "llamacpp", state: StateChecking, msg: fmt.Sprintf("Checking for llama.cpp %s updates...", channelName), ch: ch}
		release, err := runner.CheckReleaseForChannel(m.SelectedChannel)
		if err != nil {
			ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("failed to check for updates: %w", err), ch: ch}
			return
		}

		localV, _, _, _ := runner.QueryLocalVersion(m.config.Paths.LlamaCPP)
		state := StateUpdateAvailable
		cleanLocal := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(localV), "v"), "b")
		cleanLatest := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(release.TagName), "v"), "b")
		cleanNightly := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(release.NightlyTag), "v"), "b")
		if (cleanLocal == cleanLatest || (cleanNightly != "" && cleanLocal == cleanNightly)) && cleanLocal != "unknown" && cleanLocal != "not installed" {
			state = StateNoUpdate
		}

		ch <- updateMsg{
			target:  "llamacpp",
			state:   state,
			msg:     fmt.Sprintf("Latest available %s release: %s", string(m.SelectedChannel), release.TagName),
			release: release,
			ch:      ch,
		}
	}()

	return m.ReadUpdateChan(ch)
}

func (m *LifecycleModel) StartOnnxCheckOnly() tea.Cmd {
	ch := make(chan updateMsg)

	go func() {
		ch <- updateMsg{target: "onnx", state: StateChecking, msg: "Checking for ONNX updates...", ch: ch}
		release, err := runner.CheckLatestOnnxRelease()
		if err != nil {
			ch <- updateMsg{target: "onnx", state: StateError, err: fmt.Errorf("failed to check for ONNX updates: %w", err), ch: ch}
			return
		}

		localV, _ := runner.QueryLocalOnnxVersion(m.config.Paths.OnnxRuntime)
		state := StateUpdateAvailable
		cleanLocal := strings.TrimPrefix(strings.ToLower(localV), "v")
		cleanLatest := strings.TrimPrefix(strings.ToLower(release.TagName), "v")
		if cleanLocal == cleanLatest && cleanLocal != "unknown" && cleanLocal != "not installed" {
			state = StateNoUpdate
		}

		ch <- updateMsg{
			target:  "onnx",
			state:   state,
			msg:     fmt.Sprintf("Latest available ONNX release: %s", release.TagName),
			release: release,
			ch:      ch,
		}
	}()

	return m.ReadUpdateChan(ch)
}

func (m *LifecycleModel) StartCheckAll() tea.Cmd {
	return tea.Batch(m.StartCheckOnly(), m.StartOnnxCheckOnly(), m.StartAppCheck())
}

func (m *LifecycleModel) StartUpdate() tea.Cmd {
	ch := make(chan updateMsg)

	go func() {
		instances := m.srvRunner.GetAllInstances()
		if len(instances) > 0 {
			ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("cannot update: active server instances are running. Please stop all servers first."), ch: ch}
			return
		}

		var release *runner.GithubRelease
		var err error
		if m.latestRelease != nil && (strings.HasPrefix(m.latestTagName, "b") || strings.Contains(strings.ToLower(m.latestTagName), "llama") || strings.HasPrefix(m.latestTagName, "v")) {
			release = m.latestRelease
		} else {
			ch <- updateMsg{target: "llamacpp", state: StateChecking, msg: fmt.Sprintf("Checking latest llama.cpp (%s) on GitHub...", m.SelectedChannel), ch: ch}
			release, err = runner.CheckReleaseForChannel(m.SelectedChannel)
			if err != nil {
				ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("failed to check release: %w", err), ch: ch}
				return
			}
		}

		mainAsset, cudartAsset, err := runner.MatchAssetWithBackend(release, m.specs, m.SelectedBackend)
		if err != nil {
			ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("failed to match release asset: %w", err), ch: ch}
			return
		}

		var mainScale float64 = 1.0
		if cudartAsset != nil {
			mainScale = 0.7
		}

		ch <- updateMsg{target: "llamacpp", state: StateDownloading, progress: 0.0, msg: fmt.Sprintf("Downloading %s...", mainAsset.Name), ch: ch}

		destFile := filepath.Join(m.config.Paths.Downloads, mainAsset.Name)
		progressChan := make(chan float64, 5)

		downloadErrChan := make(chan error, 1)
		go func() {
			downloadErrChan <- runner.DownloadRelease(mainAsset.BrowserDownloadURL, destFile, progressChan)
		}()

		for p := range progressChan {
			ch <- updateMsg{target: "llamacpp", state: StateDownloading, progress: p * mainScale, msg: fmt.Sprintf("Downloading %s (%.1f%%)...", mainAsset.Name, p*100.0), ch: ch}
		}

		if derr := <-downloadErrChan; derr != nil {
			ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("failed to download release: %w", derr), ch: ch}
			return
		}

		var destCudartFile string
		if cudartAsset != nil {
			ch <- updateMsg{target: "llamacpp", state: StateDownloading, progress: 0.7, msg: fmt.Sprintf("Downloading %s...", cudartAsset.Name), ch: ch}
			destCudartFile = filepath.Join(m.config.Paths.Downloads, cudartAsset.Name)
			cudartProgressChan := make(chan float64, 5)

			cudartDownloadErrChan := make(chan error, 1)
			go func() {
				cudartDownloadErrChan <- runner.DownloadRelease(cudartAsset.BrowserDownloadURL, destCudartFile, cudartProgressChan)
			}()

			for p := range cudartProgressChan {
				combinedProgress := 0.7 + (p * 0.3)
				ch <- updateMsg{target: "llamacpp", state: StateDownloading, progress: combinedProgress, msg: fmt.Sprintf("Downloading %s (%.1f%%)...", cudartAsset.Name, p*100.0), ch: ch}
			}

			if derr := <-cudartDownloadErrChan; derr != nil {
				ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("failed to download cudart release: %w", derr), ch: ch}
				return
			}
		}

		versionTag := release.TagName
		if versionTag == "" {
			versionTag = "latest"
		}

		ch <- updateMsg{target: "llamacpp", state: StateExtracting, msg: fmt.Sprintf("Installing into version slot (%s)...", versionTag), ch: ch}

		err = runner.InstallVersionSlot(destFile, destCudartFile, m.config.Paths.LlamaCPP, versionTag)
		_ = os.Remove(destFile)
		if destCudartFile != "" {
			_ = os.Remove(destCudartFile)
		}
		if err != nil {
			ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("failed to install version slot: %w", err), ch: ch}
			return
		}

		ch <- updateMsg{target: "llamacpp", state: StateVerifying, msg: "Verifying installation...", ch: ch}
		version, commit, buildInfo, err := runner.QueryLocalVersion(m.config.Paths.LlamaCPP)
		if err != nil {
			ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("verification failed: %w", err), ch: ch}
			return
		}

		ch <- updateMsg{
			target: "llamacpp",
			state:  StateUpdateSuccess,
			msg:    fmt.Sprintf("Update successful! Activated slot: %s (version: %s, commit: %s, %s)", versionTag, version, commit, buildInfo),
			ch:     ch,
		}
	}()

	return m.ReadUpdateChan(ch)
}

func (m *LifecycleModel) StartOnnxUpdate() tea.Cmd {
	ch := make(chan updateMsg)

	go func() {
		instances := m.srvRunner.GetAllInstances()
		if len(instances) > 0 {
			ch <- updateMsg{target: "onnx", state: StateError, err: fmt.Errorf("cannot install ONNX: active server instances are running. Please stop all servers first."), ch: ch}
			return
		}

		var release *runner.GithubRelease
		var err error
		if m.onnxLatestRelease != nil {
			release = m.onnxLatestRelease
		} else {
			ch <- updateMsg{target: "onnx", state: StateChecking, msg: "Checking latest ONNX release on GitHub...", ch: ch}
			release, err = runner.CheckLatestOnnxRelease()
			if err != nil {
				ch <- updateMsg{target: "onnx", state: StateError, err: fmt.Errorf("failed to check ONNX release: %w", err), ch: ch}
				return
			}
		}

		onnxAsset, err := runner.MatchOnnxAssetWithBackend(release, m.specs, m.SelectedBackend)
		if err != nil {
			ch <- updateMsg{target: "onnx", state: StateError, err: fmt.Errorf("failed to match ONNX release asset: %w", err), ch: ch}
			return
		}

		ch <- updateMsg{target: "onnx", state: StateDownloading, progress: 0.0, msg: fmt.Sprintf("Downloading %s...", onnxAsset.Name), ch: ch}

		progressChan := make(chan float64, 5)
		downloadErrChan := make(chan error, 1)
		go func() {
			downloadErrChan <- runner.DownloadAndInstallOnnxRuntime(
				onnxAsset.BrowserDownloadURL,
				m.config.Paths.OnnxRuntime,
				release.TagName,
				m.config.Paths.Downloads,
				progressChan,
			)
		}()

		for p := range progressChan {
			ch <- updateMsg{target: "onnx", state: StateDownloading, progress: p, msg: fmt.Sprintf("Downloading %s (%.1f%%)...", onnxAsset.Name, p*100.0), ch: ch}
		}

		if derr := <-downloadErrChan; derr != nil {
			ch <- updateMsg{target: "onnx", state: StateError, err: fmt.Errorf("failed to install ONNX Runtime: %w", derr), ch: ch}
			return
		}

		ch <- updateMsg{
			target: "onnx",
			state:  StateUpdateSuccess,
			msg:    fmt.Sprintf("ONNX Runtime library installed successfully! Version: %s", release.TagName),
			ch:     ch,
		}
	}()

	return m.ReadUpdateChan(ch)
}

func (m *LifecycleModel) StartRollback() tea.Cmd {
	ch := make(chan updateMsg)

	go func() {
		instances := m.srvRunner.GetAllInstances()
		if len(instances) > 0 {
			ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("cannot rollback: active server instances are running. Please stop all servers first."), ch: ch}
			return
		}

		ch <- updateMsg{target: "llamacpp", state: StateRollingBack, msg: "Restoring backup version...", ch: ch}
		backupDir := m.config.Paths.LlamaCPP + ".backup"

		err := runner.RollbackBackup(backupDir, m.config.Paths.LlamaCPP)
		if err != nil {
			ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("rollback failed: %w", err), ch: ch}
			return
		}

		ch <- updateMsg{target: "llamacpp", state: StateRollbackSuccess, msg: "Rollback completed successfully!", ch: ch}
	}()

	return m.ReadUpdateChan(ch)
}

func (m *LifecycleModel) StartOnnxRollback() tea.Cmd {
	ch := make(chan updateMsg)

	go func() {
		instances := m.srvRunner.GetAllInstances()
		if len(instances) > 0 {
			ch <- updateMsg{target: "onnx", state: StateError, err: fmt.Errorf("cannot rollback ONNX: active server instances are running. Please stop all servers first."), ch: ch}
			return
		}

		ch <- updateMsg{target: "onnx", state: StateRollingBack, msg: "Restoring ONNX backup version...", ch: ch}
		backupDir := m.config.Paths.OnnxRuntime + ".backup"

		err := runner.RollbackBackup(backupDir, m.config.Paths.OnnxRuntime)
		if err != nil {
			ch <- updateMsg{target: "onnx", state: StateError, err: fmt.Errorf("ONNX rollback failed: %w", err), ch: ch}
			return
		}

		ch <- updateMsg{target: "onnx", state: StateRollbackSuccess, msg: "ONNX rollback completed successfully!", ch: ch}
	}()

	return m.ReadUpdateChan(ch)
}

// StartCheckSelected checks updates for the currently focused runtime component.
func (m *LifecycleModel) StartCheckSelected() tea.Cmd {
	switch m.SelectedRuntime {
	case 1: // ONNX
		m.updatingRuntime = "onnx"
		return m.StartOnnxCheckOnly()
	case 2: // App
		m.updatingRuntime = "app"
		return m.StartAppCheck()
	default: // llama.cpp
		m.updatingRuntime = "llamacpp"
		return m.StartCheckOnly()
	}
}

// StartUpdateSelected downloads and updates the currently focused runtime component.
func (m *LifecycleModel) StartUpdateSelected() tea.Cmd {
	switch m.SelectedRuntime {
	case 1: // ONNX
		m.updatingRuntime = "onnx"
		return m.StartOnnxUpdate()
	case 2: // App
		m.updatingRuntime = "app"
		return m.StartAppUpdate()
	default: // llama.cpp
		m.updatingRuntime = "llamacpp"
		return m.StartUpdate()
	}
}

// StartRollbackSelected rolls back the currently focused runtime component.
func (m *LifecycleModel) NextRuntime() {
	m.SelectedRuntime = (m.SelectedRuntime + 1) % 4
}

func (m *LifecycleModel) PrevRuntime() {
	m.SelectedRuntime = (m.SelectedRuntime + 3) % 4
}

func (m *LifecycleModel) StartRollbackSelected() tea.Cmd {
	switch m.SelectedRuntime {
	case 1: // ONNX
		if m.hasOnnxBackup {
			m.updatingRuntime = "onnx"
			return m.StartOnnxRollback()
		}
	case 2: // App
		return nil
	default: // llama.cpp
		if m.hasLlamaBackup {
			m.updatingRuntime = "llamacpp"
			return m.StartRollback()
		}
	}
	return nil
}

func (m *LifecycleModel) ReadUpdateChan(ch chan updateMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *LifecycleModel) Update(msg tea.Msg) (*LifecycleModel, tea.Cmd) {
	if m.tokenEditActive {
		// Handle ctrl+v before delegating to the textinput Update
		if keyMsg, ok := msg.(tea.KeyMsg); ok && (keyMsg.String() == "ctrl+v" || keyMsg.Type == tea.KeyCtrlV) {
			pasteFromClipboard(&m.tokenInput)
			return m, nil
		}

		var cmd tea.Cmd
		m.tokenInput, cmd = m.tokenInput.Update(msg)

		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter", "ctrl+s":
				val := strings.TrimSpace(m.tokenInput.Value())
				if m.tokenEditTarget == "github" {
					m.config.GitHubToken = val
					runner.SetGitHubToken(val)
				} else {
					m.config.HFToken = val
				}
				_ = m.config.Save()
				m.tokenInput.Blur()
				m.tokenEditActive = false
			case "esc":
				m.tokenInput.Blur()
				m.tokenEditActive = false
			}
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.PrevRuntime()
		case "down", "j":
			m.NextRuntime()
		case "g", "G":
			m.SelectedRuntime = 0
			m.tokenEditActive = true
			m.tokenEditTarget = "github"
			m.tokenInput.Placeholder = "Enter GITHUB_TOKEN (ghp_...)"
			m.tokenInput.Focus()
			m.tokenInput.SetValue(m.config.GitHubToken)
			return m, nil
		case "t", "T", "e", "E":
			m.SelectedRuntime = 0
			m.tokenEditActive = true
			m.tokenEditTarget = "hf"
			m.tokenInput.Placeholder = "Enter HF_TOKEN (hf_...)"
			m.tokenInput.Focus()
			m.tokenInput.SetValue(m.config.HFToken)
			return m, nil
		case "s", "S":
			if m.SelectedRuntime == 1 {
				m.ToggleChannel()
			}
			return m, nil
		case "b", "B":
			m.CycleBackend()
			return m, nil
		case "v", "V":
			if m.SelectedRuntime == 1 {
				if err := m.CycleVersionSlot(); err != nil {
					m.actionMsg = err.Error()
				}
			}
			return m, nil
		case "1":
			m.SelectedRuntime = 0
			return m, nil
		case "2":
			m.SelectedRuntime = 1
			return m, nil
		case "3":
			m.SelectedRuntime = 2
			return m, nil
		case "4":
			m.SelectedRuntime = 3
			return m, nil
		}

	case appCheckMsg:
		m.appChecking = false
		if msg.err != nil {
			m.appCheckErr = msg.err
		} else {
			m.appCheckErr = nil
			m.appLatestTag = msg.latestTag
		}

	case appUpdateMsg:
		m.appUpdating = false
		if msg.err != nil {
			m.appUpdateErr = msg.err
			m.appUpdateSuccess = false
		} else {
			m.appUpdateErr = nil
			m.appUpdateSuccess = true
			m.appUpdateMsg = msg.msg
			m.appVersion = m.appLatestTag
		}

	case updateMsg:
		m.state = msg.state
		if msg.err != nil {
			m.err = msg.err
			m.actionMsg = ""
		} else {
			m.err = nil
			m.actionMsg = msg.msg
		}

		if msg.progress > 0 {
			m.downloadProgress = msg.progress
		}

		if msg.release != nil {
			if msg.target == "onnx" || (msg.target == "" && m.updatingRuntime == "onnx") {
				m.onnxLatestRelease = msg.release
				m.onnxLatestVersion = msg.release.TagName
			} else if msg.target == "app" || (msg.target == "" && m.updatingRuntime == "app") {
				m.appLatestTag = msg.release.TagName
			} else if msg.target == "llamacpp" || (msg.target == "" && m.updatingRuntime == "llamacpp") {
				m.latestRelease = msg.release
				m.latestTagName = msg.release.TagName
			}
		}

		if m.state == StateUpdateSuccess || m.state == StateRollbackSuccess || m.state == StateError {
			m.RefreshLocalVersion()
			m.RefreshBackupStatus()
			m.RefreshVersionSlots()
			m.updatingRuntime = ""
		}

		if m.state != StateUpdateSuccess && m.state != StateRollbackSuccess && m.state != StateError && m.state != StateNoUpdate && m.state != StateUpdateAvailable {
			return m, m.ReadUpdateChan(msg.ch)
		}
	}
	return m, nil
}

func maskToken(token string) string {
	if token == "" {
		return lipgloss.NewStyle().Foreground(ColorMuted).Render("Not Configured")
	}
	if strings.HasPrefix(token, "ghp_") {
		if len(token) > 8 {
			return "ghp_***" + token[len(token)-4:]
		}
		return "ghp_***"
	}
	if strings.HasPrefix(token, "hf_") {
		if len(token) > 7 {
			return "hf_***" + token[len(token)-4:]
		}
		return "hf_***"
	}
	if len(token) <= 8 {
		return "********"
	}
	return token[:4] + "***" + token[len(token)-4:]
}

func (m *LifecycleModel) View(width int, height int) string {
	m.width = width
	m.height = height

	cardWidth := width
	if cardWidth < 50 {
		cardWidth = 50
	}

	leftWidth := 28
	if cardWidth < 80 {
		leftWidth = 24
	}
	rightWidth := cardWidth - leftWidth - 1
	if rightWidth < 40 {
		rightWidth = 40
	}

	// Status short pills for left panel
	ghStatusShort := lipgloss.NewStyle().Foreground(ColorMuted).Render("[Not Set]")
	if m.config.GitHubToken != "" && m.config.HFToken != "" {
		ghStatusShort = StyleSuccess.Render("[✓ Both Set]")
	} else if m.config.GitHubToken != "" || m.config.HFToken != "" {
		ghStatusShort = StyleSuccess.Render("[✓ 1 Set]")
	}

	llamaStatusShort := StyleSuccess.Render("[Active]")
	if m.localVersion == "Not Installed" {
		llamaStatusShort = StyleDanger.Render("[Missing]")
	}

	onnxStatusShort := StyleSuccess.Render("[v1.29]")
	if m.onnxLocalVersion == "Not Installed" {
		onnxStatusShort = StyleDanger.Render("[Missing]")
	} else if m.onnxLocalVersion != "" && m.onnxLocalVersion != "Unknown" {
		onnxStatusShort = lipgloss.NewStyle().Foreground(ColorWhite).Render("[" + m.onnxLocalVersion + "]")
	}

	appStatusShort := lipgloss.NewStyle().Foreground(ColorWhite).Render("[" + m.appVersion + "]")

	// 1. Left Column: Components Navigation List
	var leftSB strings.Builder
	leftSB.WriteString("\n")
	sections := []struct {
		idx   int
		name  string
		badge string
	}{
		{0, "1. API Tokens", ghStatusShort},
		{1, "2. llama.cpp", llamaStatusShort},
		{2, "3. ONNX Runtime", onnxStatusShort},
		{3, "4. Runora App", appStatusShort},
	}

	for _, s := range sections {
		isSelected := m.SelectedRuntime == s.idx
		prefix := "  "
		if isSelected {
			prefix = "▶ "
		}

		nameStyle := lipgloss.NewStyle().Foreground(ColorWhite)
		if isSelected {
			nameStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
		}

		leftSB.WriteString(fmt.Sprintf("%s%-14s %s\n\n", prefix, nameStyle.Render(s.name), s.badge))
	}
	leftSB.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Render("  [↑/↓: Select Section]"))
	leftContent := strings.TrimRight(leftSB.String(), "\n")
	leftCard := SurfaceCard("Components", leftContent, leftWidth, true, "4 Sections")

	// 2. Right Column: Dynamic Section Inspector
	var rightCard string

	if m.SelectedRuntime == 0 {
		// API Credentials Inspector
		var tokenSB strings.Builder
		ghTokenStr := maskToken(m.config.GitHubToken)
		hfTokenStr := maskToken(m.config.HFToken)

		ghStatus := lipgloss.NewStyle().Foreground(ColorMuted).Render("[Not Configured]")
		if m.config.GitHubToken != "" {
			ghStatus = StyleSuccess.Render("[✓ Configured]")
		}
		hfStatus := lipgloss.NewStyle().Foreground(ColorMuted).Render("[Not Configured]")
		if m.config.HFToken != "" {
			hfStatus = StyleSuccess.Render("[✓ Configured]")
		}

		if m.tokenEditActive {
			targetName := "Hugging Face API Token (HF_TOKEN)"
			targetKey := "hf_..."
			if m.tokenEditTarget == "github" {
				targetName = "GitHub API Token (GITHUB_TOKEN)"
				targetKey = "ghp_..."
			}
			tokenSB.WriteString(fmt.Sprintf("  %s  %s\n\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("EDITING: "+targetName), lipgloss.NewStyle().Foreground(ColorMuted).Render("("+targetKey+")")))
			tokenSB.WriteString(fmt.Sprintf("  Input: > %s\n\n", m.tokenInput.View()))
			tokenSB.WriteString(fmt.Sprintf("  Controls: %s Save  │  %s Paste from Clipboard  │  %s Cancel\n", StyleHelpKey.Render("[Enter]"), StyleHelpKey.Render("[Ctrl+V]"), StyleHelpKey.Render("[Esc]")))
		} else {
			tokenSB.WriteString(fmt.Sprintf("  %-28s %-16s %-16s %s\n", "GitHub Token (ghp_***):", ghTokenStr, ghStatus, StyleHelpKey.Render("[G: Edit / Paste]")))
			tokenSB.WriteString(fmt.Sprintf("  %-28s %-16s %-16s %s\n\n", "Hugging Face Token (hf_***):", hfTokenStr, hfStatus, StyleHelpKey.Render("[T: Edit / Paste]")))
			tokenSB.WriteString("  Information:\n")
			tokenSB.WriteString("  • GitHub Token unlocks 5,000 req/hr rate limits for seamless version updates.\n")
			tokenSB.WriteString("  • Hugging Face Token enables downloading gated, private, and research models.\n\n")
			tokenSB.WriteString(fmt.Sprintf("  Actions: Press %s to edit GitHub Token, %s to edit Hugging Face Token\n", StyleHelpKey.Render("[G]"), StyleHelpKey.Render("[T]")))
		}
		tokenContent := strings.TrimRight(tokenSB.String(), "\n")
		rightCard = SurfaceCard("API Credentials Inspector", tokenContent, rightWidth, m.tokenEditActive, "GitHub & Hugging Face")

	} else if m.SelectedRuntime == 1 {
		// llama.cpp Runtime Inspector
		var llamaSB strings.Builder
		localVerStr := m.localVersion
		if localVerStr != "Not Installed" && localVerStr != "Unknown" {
			localVerStr = StyleSuccess.Render(localVerStr)
		} else if localVerStr == "Not Installed" {
			localVerStr = StyleDanger.Render(localVerStr)
		}
		llamaSB.WriteString(fmt.Sprintf("  %-22s %s\n", "Active Version Slot:", localVerStr))
		llamaSB.WriteString(fmt.Sprintf("  %-22s %s\n", "Folder Path:", m.config.Paths.LlamaCPP))
		if m.localCommit != "" && m.localCommit != "N/A" {
			llamaSB.WriteString(fmt.Sprintf("  %-22s %s (%s)\n", "Commit / Build:", m.localCommit, m.localBuildInfo))
		}

		channelStr := ""
		if m.SelectedChannel == runner.ChannelStable {
			channelStr = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("[●] Stable") + "   " + lipgloss.NewStyle().Foreground(ColorMuted).Render("[○] Nightly")
		} else {
			channelStr = lipgloss.NewStyle().Foreground(ColorMuted).Render("[○] Stable") + "   " + lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render("[●] Nightly")
		}
		llamaSB.WriteString(fmt.Sprintf("  %-22s %s  %s\n", "Release Channel:", channelStr, lipgloss.NewStyle().Foreground(ColorMuted).Render("[S: Toggle Channel]")))

		backendDisplay := strings.ToUpper(string(m.SelectedBackend))
		if m.SelectedBackend == runner.BackendAuto {
			detected := m.specs.GPU.Type
			if detected == "CUDA" && m.specs.GPU.CudaVersion != "" {
				detected += " " + m.specs.GPU.CudaVersion
			}
			backendDisplay = fmt.Sprintf("Auto (Detected: %s)", detected)
		}
		llamaSB.WriteString(fmt.Sprintf("  %-22s %s  %s\n", "Backend Accelerator:", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(backendDisplay), lipgloss.NewStyle().Foreground(ColorMuted).Render("[B: Cycle Accelerator]")))

		slotsDisplay := lipgloss.NewStyle().Foreground(ColorMuted).Render("None installed in versions/")
		if len(m.installedVersions) > 0 {
			var slotItems []string
			for _, s := range m.installedVersions {
				if s == m.localVersion || s == m.activeSlot {
					slotItems = append(slotItems, StyleSuccess.Render(s+" [Active]"))
				} else {
					slotItems = append(slotItems, lipgloss.NewStyle().Foreground(ColorWhite).Render(s))
				}
			}
			slotsDisplay = strings.Join(slotItems, " • ")
		}
		llamaSB.WriteString(fmt.Sprintf("  %-22s %s  %s\n", "Installed Slots:", slotsDisplay, lipgloss.NewStyle().Foreground(ColorMuted).Render("[V: Switch Slot]")))

		updateLlamaStatus := lipgloss.NewStyle().Foreground(ColorMuted).Render("Not checked ([U] to check updates)")
		if m.latestTagName != "" {
			if m.latestTagName == m.localVersion {
				updateLlamaStatus = StyleSuccess.Render("✓ Up-to-date (" + m.latestTagName + ")")
			} else {
				updateLlamaStatus = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(m.latestTagName + " (Update Available)  [Enter/U: Install]")
			}
		}
		llamaSB.WriteString(fmt.Sprintf("  %-22s %s\n", "Update Status:", updateLlamaStatus))

		llamaBackupStr := lipgloss.NewStyle().Foreground(ColorMuted).Render("None")
		if m.hasLlamaBackup {
			llamaBackupStr = StyleSuccess.Render("Available (llama.cpp.backup/)") + "  " + lipgloss.NewStyle().Foreground(ColorMuted).Render("[R: Rollback]")
		}
		llamaSB.WriteString(fmt.Sprintf("  %-22s %s\n\n", "Backup & Recovery:", llamaBackupStr))
		llamaSB.WriteString(fmt.Sprintf("  Actions: %s Check & Install Update  │  %s Rollback Backup\n", StyleHelpKey.Render("[Enter/U]"), StyleHelpKey.Render("[R]")))
		llamaContent := strings.TrimRight(llamaSB.String(), "\n")
		rightCard = SurfaceCard("llama.cpp Runtime Inspector", llamaContent, rightWidth, true, "llama.cpp")

	} else if m.SelectedRuntime == 2 {
		// ONNX Runtime Inspector
		var onnxSB strings.Builder
		onnxVerStr := m.onnxLocalVersion
		if onnxVerStr != "Not Installed" && !strings.Contains(onnxVerStr, "Unknown") {
			onnxVerStr = StyleSuccess.Render(onnxVerStr)
		} else if onnxVerStr == "Not Installed" {
			onnxVerStr = StyleDanger.Render(onnxVerStr)
		}
		onnxSB.WriteString(fmt.Sprintf("  %-22s %s\n", "Installed Version:", onnxVerStr))
		onnxSB.WriteString(fmt.Sprintf("  %-22s %s\n", "Folder Path:", m.config.Paths.OnnxRuntime))

		onnxBackendDisplay := strings.ToUpper(string(m.SelectedBackend))
		if m.SelectedBackend == runner.BackendAuto {
			detected := m.specs.GPU.Type
			if detected == "CUDA" && m.specs.GPU.CudaVersion != "" {
				detected += " " + m.specs.GPU.CudaVersion
			}
			onnxBackendDisplay = fmt.Sprintf("Auto (Detected: %s)", detected)
		}
		onnxSB.WriteString(fmt.Sprintf("  %-22s %s  %s\n", "Backend Accelerator:", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(onnxBackendDisplay), lipgloss.NewStyle().Foreground(ColorMuted).Render("[B: Cycle Accelerator]")))

		updateOnnxStatus := lipgloss.NewStyle().Foreground(ColorMuted).Render("Not checked ([U] to check updates)")
		if m.onnxLatestVersion != "" {
			if m.onnxLatestVersion == m.onnxLocalVersion {
				updateOnnxStatus = StyleSuccess.Render("✓ Up-to-date (" + m.onnxLatestVersion + ")")
			} else {
				updateOnnxStatus = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(m.onnxLatestVersion + " (Update Available)  [Enter/O: Install]")
			}
		}
		onnxSB.WriteString(fmt.Sprintf("  %-22s %s\n", "Update Status:", updateOnnxStatus))

		onnxBackupStr := lipgloss.NewStyle().Foreground(ColorMuted).Render("None")
		if m.hasOnnxBackup {
			onnxBackupStr = StyleSuccess.Render("Available (onnxruntime.backup/)") + "  " + lipgloss.NewStyle().Foreground(ColorMuted).Render("[R: Rollback]")
		}
		onnxSB.WriteString(fmt.Sprintf("  %-22s %s\n\n", "Backup & Recovery:", onnxBackupStr))
		onnxSB.WriteString(fmt.Sprintf("  Actions: %s Install/Update ONNX  │  %s Rollback Backup\n", StyleHelpKey.Render("[Enter/O]"), StyleHelpKey.Render("[R]")))
		onnxContent := strings.TrimRight(onnxSB.String(), "\n")
		rightCard = SurfaceCard("ONNX Runtime Inspector", onnxContent, rightWidth, true, "ONNX")

	} else {
		// Runora App & Tools Inspector
		var appSB strings.Builder
		appVerStr := lipgloss.NewStyle().Foreground(ColorWhite).Render(m.appVersion)
		appSB.WriteString(fmt.Sprintf("  %-22s %s\n", "Runora CLI Version:", appVerStr))

		updateAppStatus := StyleSuccess.Render("✓ Up-to-date")
		if m.appChecking {
			updateAppStatus = lipgloss.NewStyle().Foreground(ColorMuted).Render("Checking...")
		} else if m.appCheckErr != nil {
			updateAppStatus = StyleDanger.Render("Check failed")
		} else if m.appLatestTag != "" {
			if m.appLatestTag == m.appVersion || m.appUpdateSuccess {
				updateAppStatus = StyleSuccess.Render("✓ Up-to-date (" + m.appLatestTag + ")")
			} else {
				updateAppStatus = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(m.appLatestTag + " (Update Available)  [Enter/A: Self-Update]")
			}
		}
		appSB.WriteString(fmt.Sprintf("  %-22s %s\n", "App Update Status:", updateAppStatus))

		themeName := strings.Title(m.config.Theme)
		if themeName == "" {
			themeName = "Forest"
		}
		appSB.WriteString(fmt.Sprintf("  %-22s %s  %s\n", "UI Theme Palette:", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(themeName), lipgloss.NewStyle().Foreground(ColorMuted).Render("[Y: Switch Theme]")))
		appSB.WriteString(fmt.Sprintf("  %-22s %s\n\n", "Onboarding Tour:", lipgloss.NewStyle().Foreground(ColorMuted).Render("[N: Re-run Welcome Tour]")))
		appSB.WriteString(fmt.Sprintf("  Actions: %s Self-Update App  │  %s Themes  │  %s Welcome Tour\n", StyleHelpKey.Render("[Enter/A]"), StyleHelpKey.Render("[Y]"), StyleHelpKey.Render("[N]")))
		appContent := strings.TrimRight(appSB.String(), "\n")
		rightCard = SurfaceCard("Runora System & Tools Inspector", appContent, rightWidth, true, "System")
	}

	// Join Left & Right Columns side by side
	mainDeck := lipgloss.JoinHorizontal(lipgloss.Top, leftCard, rightCard)

	var elements []string
	elements = append(elements, mainDeck)

	// Status line if active
	if m.state != StateIdle {
		var statusSB strings.Builder
		statusText := ""
		targetName := "Inference runtime"
		if m.updatingRuntime == "onnx" {
			targetName = "ONNX Runtime"
		} else if m.updatingRuntime == "llamacpp" {
			targetName = "llama.cpp"
		} else if m.updatingRuntime == "app" {
			targetName = "Runora App"
		}

		switch m.state {
		case StateChecking:
			statusText = fmt.Sprintf("Checking latest %s release...", targetName)
		case StateNoUpdate:
			statusText = StyleSuccess.Render(fmt.Sprintf("%s is up-to-date.", targetName))
		case StateUpdateAvailable:
			statusText = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(fmt.Sprintf("%s update available - press [U] or [Space] to install.", targetName))
		case StateDownloading:
			statusText = fmt.Sprintf("Downloading %s: %s", targetName, m.actionMsg)
		case StateExtracting:
			statusText = fmt.Sprintf("Extracting %s: %s", targetName, m.actionMsg)
		case StateVerifying:
			statusText = fmt.Sprintf("Verifying %s installation...", targetName)
		case StateUpdateSuccess:
			statusText = StyleSuccess.Render(fmt.Sprintf("%s updated successfully.", targetName))
		case StateRollingBack:
			statusText = fmt.Sprintf("Rolling back %s...", targetName)
		case StateRollbackSuccess:
			statusText = StyleSuccess.Render(fmt.Sprintf("%s rollback completed successfully.", targetName))
		case StateError:
			statusText = StyleDanger.Render(fmt.Sprintf("Error: %v", m.err))
		}

		if statusText != "" {
			statusSB.WriteString("  " + statusText + "\n")
		}
		if m.state == StateDownloading {
			bar := RenderProgressBar(m.downloadProgress*100.0, max(20, width-18), ColorProgressFilled)
			statusSB.WriteString(fmt.Sprintf("  %s %3.0f%%\n", bar, m.downloadProgress*100.0))
		}
		elements = append(elements, statusSB.String())
	}

	return lipgloss.JoinVertical(lipgloss.Left, elements...)
}
