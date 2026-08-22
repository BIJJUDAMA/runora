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
		srvRunner:       srv,
		config:          cfg,
		specs:           specs,
		state:           StateIdle,
		tokenInput:      ti,
		tokenEditActive: false,
		tokenEditTarget: "hf",
		appVersion:      resolveAppVersion(),
		SelectedRuntime: 0,
	}
	m.RefreshLocalVersion()
	m.RefreshBackupStatus()
	return m
}

func (m *LifecycleModel) NextRuntime() {
	m.SelectedRuntime = (m.SelectedRuntime + 1) % 3
}

func (m *LifecycleModel) PrevRuntime() {
	m.SelectedRuntime = (m.SelectedRuntime + 2) % 3
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
		ch <- updateMsg{target: "llamacpp", state: StateChecking, msg: "Checking for llama.cpp updates...", ch: ch}
		release, err := runner.CheckLatestRelease()
		if err != nil {
			ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("failed to check for updates: %w", err), ch: ch}
			return
		}

		localV, _, _, _ := runner.QueryLocalVersion(m.config.Paths.LlamaCPP)
		state := StateUpdateAvailable
		cleanLocal := strings.TrimPrefix(strings.ToLower(localV), "b")
		cleanLatest := strings.TrimPrefix(strings.ToLower(release.TagName), "b")
		if cleanLocal == cleanLatest && cleanLocal != "unknown" && cleanLocal != "not installed" {
			state = StateNoUpdate
		}

		ch <- updateMsg{
			target:  "llamacpp",
			state:   state,
			msg:     fmt.Sprintf("Latest available release: %s", release.TagName),
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
		if m.latestRelease != nil && (strings.HasPrefix(m.latestTagName, "b") || strings.Contains(strings.ToLower(m.latestTagName), "llama")) {
			release = m.latestRelease
		} else {
			ch <- updateMsg{target: "llamacpp", state: StateChecking, msg: "Checking latest llama.cpp release on GitHub...", ch: ch}
			release, err = runner.CheckLatestRelease()
			if err != nil {
				ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("failed to check release: %w", err), ch: ch}
				return
			}
		}

		mainAsset, cudartAsset, err := runner.MatchAsset(release, m.specs)
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

		ch <- updateMsg{target: "llamacpp", state: StateExtracting, msg: "Creating backup of existing llama.cpp...", ch: ch}
		backupDir := m.config.Paths.LlamaCPP + ".backup"

		if _, err := os.Stat(m.config.Paths.LlamaCPP); err == nil {
			err = runner.CreateBackup(m.config.Paths.LlamaCPP, backupDir)
			if err != nil {
				ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("failed to create backup: %w", err), ch: ch}
				return
			}
		}

		ch <- updateMsg{target: "llamacpp", state: StateExtracting, msg: "Extracting updated binaries...", ch: ch}
		_ = os.MkdirAll(m.config.Paths.LlamaCPP, 0755)
		err = runner.ExtractArchive(destFile, m.config.Paths.LlamaCPP)
		if err != nil {
			_ = runner.RollbackBackup(backupDir, m.config.Paths.LlamaCPP)
			ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("extraction failed (rolled back): %w", err), ch: ch}
			return
		}
		_ = os.Remove(destFile)

		if destCudartFile != "" {
			ch <- updateMsg{target: "llamacpp", state: StateExtracting, msg: "Extracting CUDA runtime DLLs...", ch: ch}
			err = runner.ExtractArchive(destCudartFile, m.config.Paths.LlamaCPP)
			if err != nil {
				_ = runner.RollbackBackup(backupDir, m.config.Paths.LlamaCPP)
				ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("CUDA DLLs extraction failed (rolled back): %w", err), ch: ch}
				return
			}
			_ = os.Remove(destCudartFile)
		}

		ch <- updateMsg{target: "llamacpp", state: StateVerifying, msg: "Verifying installation...", ch: ch}
		version, commit, buildInfo, err := runner.QueryLocalVersion(m.config.Paths.LlamaCPP)
		if err != nil {
			_ = runner.RollbackBackup(backupDir, m.config.Paths.LlamaCPP)
			ch <- updateMsg{target: "llamacpp", state: StateError, err: fmt.Errorf("verification failed (rolled back): %w", err), ch: ch}
			return
		}

		ch <- updateMsg{
			target: "llamacpp",
			state:  StateUpdateSuccess,
			msg:    fmt.Sprintf("Update successful! Version: %s, Commit: %s (%s)", version, commit, buildInfo),
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

		onnxAsset, err := runner.MatchOnnxAsset(release, m.specs)
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
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "ctrl+v" {
			pasteFromClipboard(&m.tokenInput)
			return m, nil
		}

		var cmd tea.Cmd
		m.tokenInput, cmd = m.tokenInput.Update(msg)

		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
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
		case "t", "T":
			m.tokenEditActive = true
			m.tokenEditTarget = "hf"
			m.tokenInput.Placeholder = "Enter HF_TOKEN (hf_...)"
			m.tokenInput.Focus()
			m.tokenInput.SetValue(m.config.HFToken)
			return m, nil
		case "g", "G":
			m.tokenEditActive = true
			m.tokenEditTarget = "github"
			m.tokenInput.Placeholder = "Enter GITHUB_TOKEN (ghp_...)"
			m.tokenInput.Focus()
			m.tokenInput.SetValue(m.config.GitHubToken)
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
	if len(token) <= 10 {
		return "********"
	}
	return token[:5] + "..." + token[len(token)-5:]
}

func (m *LifecycleModel) View(width int, height int) string {
	m.width = width
	m.height = height

	if m.tokenEditActive {
		title := "CONFIGURE HUGGING FACE TOKEN"
		desc1 := "Please enter or paste your Hugging Face API token (HF_TOKEN)."
		desc2 := "This token is used for downloading gated/private models and avoiding API limits."
		if m.tokenEditTarget == "github" {
			title = "CONFIGURE GITHUB API TOKEN"
			desc1 = "Please enter or paste your GitHub Personal Access Token (GITHUB_TOKEN / GH_TOKEN)."
			desc2 = "This token increases release check rate limits from 60 to 5,000 requests per hour."
		}

		var sb strings.Builder
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(title)))
		sb.WriteString("  " + desc1 + "\n")
		sb.WriteString("  " + desc2 + "\n\n")
		sb.WriteString("  " + m.tokenInput.View() + "\n\n")
		sb.WriteString("  " + StyleHelpKey.Render("[Enter]") + " Save Token  " + StyleHelpKey.Render("[Esc]") + " Cancel / Exit\n")

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

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("SETTINGS & RUNTIME LIFECYCLE")))

	// Helper styling for focus
	renderHeader := func(index int, numStr, title string) string {
		if m.SelectedRuntime == index {
			return lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(fmt.Sprintf("  ▶ [●] %s. %s (Focused)", numStr, title))
		}
		return lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("    [○] %s. %s", numStr, title))
	}

	// ── 1. llama.cpp Runtime ──────────────────────────────────────────────────
	sb.WriteString(renderHeader(0, "1", "Inference Runtime (llama.cpp)") + "\n")
	sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Folder Path:", m.config.Paths.LlamaCPP))

	localVerStr := m.localVersion
	if localVerStr != "Not Installed" && localVerStr != "Unknown" {
		localVerStr = StyleSuccess.Render(localVerStr)
	} else if localVerStr == "Not Installed" {
		localVerStr = StyleDanger.Render(localVerStr)
	}
	sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Version Tag:", localVerStr))
	sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Commit Hash:", m.localCommit))
	sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Compiler/Build:", m.localBuildInfo))

	backupStr := lipgloss.NewStyle().Foreground(ColorMuted).Render("Not Available")
	if m.hasLlamaBackup {
		backupStr = StyleSuccess.Render("Available (llama.cpp.backup/)")
	}
	sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Local Backup:", backupStr))

	if m.latestTagName != "" {
		sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Latest Release:", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(m.latestTagName)))
	} else {
		sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Latest Release:", lipgloss.NewStyle().Foreground(ColorMuted).Render("Not checked  ([C] / [Enter] to check)")))
	}
	sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Available Actions:", lipgloss.NewStyle().Foreground(ColorMuted).Render("[C/Enter] Check  •  [U/Space] Install/Update  •  [R] Rollback")))
	sb.WriteString("\n")

	// ── 2. ONNX Runtime Library ──────────────────────────────────────────────
	sb.WriteString(renderHeader(1, "2", "ONNX Runtime Library") + "\n")
	sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Folder Path:", m.config.Paths.OnnxRuntime))

	onnxVerStr := m.onnxLocalVersion
	if onnxVerStr != "Not Installed" && !strings.Contains(onnxVerStr, "Unknown") {
		onnxVerStr = StyleSuccess.Render(onnxVerStr)
	} else if onnxVerStr == "Not Installed" {
		onnxVerStr = StyleDanger.Render(onnxVerStr)
	}
	sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Installed Version:", onnxVerStr))

	onnxBackupStr := lipgloss.NewStyle().Foreground(ColorMuted).Render("Not Available")
	if m.hasOnnxBackup {
		onnxBackupStr = StyleSuccess.Render("Available (onnxruntime.backup/)")
	}
	sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Local Backup:", onnxBackupStr))

	if m.onnxLatestVersion != "" {
		sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Latest Release:", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(m.onnxLatestVersion)))
	} else {
		sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Latest Release:", lipgloss.NewStyle().Foreground(ColorMuted).Render("Not checked  ([C] / [Enter] to check)")))
	}
	sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Available Actions:", lipgloss.NewStyle().Foreground(ColorMuted).Render("[C/Enter] Check  •  [U/Space] Install/Update  •  [R] Rollback")))
	sb.WriteString("\n")

	// ── 3. Runora App ────────────────────────────────────────────────────────
	sb.WriteString(renderHeader(2, "3", "Runora CLI Application") + "\n")
	appVerStr := lipgloss.NewStyle().Foreground(ColorWhite).Render(m.appVersion)
	sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Installed Version:", appVerStr))
	if m.appChecking {
		sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Latest Release:", lipgloss.NewStyle().Foreground(ColorMuted).Render("Checking...")))
	} else if m.appCheckErr != nil {
		sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Latest Release:", StyleDanger.Render("Check failed")))
	} else if m.appLatestTag != "" {
		if m.appUpdateSuccess {
			sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Latest Release:", StyleSuccess.Render(m.appLatestTag+" (up-to-date)")))
			sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Status:", StyleSuccess.Render(m.appUpdateMsg)))
		} else if m.appLatestTag == m.appVersion {
			sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Latest Release:", StyleSuccess.Render(m.appLatestTag+" (up-to-date)")))
		} else {
			sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Latest Release:", lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(m.appLatestTag+" — update available")))
			if m.appUpdating {
				sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Status:", lipgloss.NewStyle().Foreground(ColorAccent).Render("Installing update...")))
			} else if m.appUpdateErr != nil {
				sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Status:", StyleDanger.Render(fmt.Sprintf("Update failed: %v", m.appUpdateErr))))
			}
		}
	} else {
		sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Latest Release:", lipgloss.NewStyle().Foreground(ColorMuted).Render("Not checked  ([C] / [Enter] to check)")))
	}
	sb.WriteString(fmt.Sprintf("      %-20s %s\n", "Available Actions:", lipgloss.NewStyle().Foreground(ColorMuted).Render("[C/Enter] Check  •  [U/Space] Update App")))
	sb.WriteString("\n")

	// ── Preferences & Hardware ───────────────────────────────────────────────
	sb.WriteString("  " + lipgloss.NewStyle().Bold(true).Render("Preferences & Hardware:") + "\n")
	themeStr := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(strings.Title(m.config.Theme))
	tokenStr := maskToken(m.config.HFToken)
	ghTokenStr := maskToken(m.config.GitHubToken)
	onboardStr := lipgloss.NewStyle().Foreground(ColorMuted).Render("Completed")
	if !m.config.OnboardingCompleted {
		onboardStr = lipgloss.NewStyle().Foreground(ColorAccent).Render("Not completed")
	}

	gpuDesc := m.specs.GPU.Type
	if m.specs.GPU.Type == "CUDA" && m.specs.GPU.CudaVersion != "" {
		gpuDesc += fmt.Sprintf(" (CUDA %s)", m.specs.GPU.CudaVersion)
	}

	col1Left := lipgloss.NewStyle().Width(38).Render(fmt.Sprintf("      %-18s %s", "Color Theme:", themeStr))
	col1Right := fmt.Sprintf("%-20s %s", "Operating System:", m.specs.OS)
	sb.WriteString(fmt.Sprintf("%s │   %s\n", col1Left, col1Right))

	col2Left := lipgloss.NewStyle().Width(38).Render(fmt.Sprintf("      %-18s %s", "HF Token [T]:", tokenStr))
	col2Right := fmt.Sprintf("%-20s %s", "GPU Accelerator:", gpuDesc)
	sb.WriteString(fmt.Sprintf("%s │   %s\n", col2Left, col2Right))

	col3Left := lipgloss.NewStyle().Width(38).Render(fmt.Sprintf("      %-18s %s", "GitHub Token [G]:", ghTokenStr))
	col3Right := fmt.Sprintf("%-20s %s", "Onboarding Tour:", onboardStr)
	sb.WriteString(fmt.Sprintf("%s │   %s\n", col3Left, col3Right))
	sb.WriteString("\n")

	// ── Runtime status ───────────────────────────────────────────────────────
	if m.state != StateIdle {
		sb.WriteString("  " + lipgloss.NewStyle().Bold(true).Render("Status:") + "\n")
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
			statusText = lipgloss.NewStyle().Foreground(ColorAccent).Bold(true).Render(fmt.Sprintf("%s update available — press [U] or [Space] to install.", targetName))
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
			sb.WriteString("    " + statusText + "\n")
		}
		if m.state == StateDownloading {
			sb.WriteString("    " + renderProgressBar(width-8, m.downloadProgress) + "\n")
		}
		sb.WriteString("\n")
	}

	// ── Unified Help footer ───────────────────────────────────────────────────
	var helpKeys []string
	if m.state != StateDownloading && m.state != StateExtracting && m.state != StateVerifying && m.state != StateRollingBack {
		helpKeys = append(helpKeys, fmt.Sprintf("%s Switch Focus", StyleHelpKey.Render("[1-3 / Tab/↑↓]")))
		helpKeys = append(helpKeys, fmt.Sprintf("%s Check", StyleHelpKey.Render("[C/Enter]")))
		helpKeys = append(helpKeys, fmt.Sprintf("%s Update/Install", StyleHelpKey.Render("[U/Space]")))
		if m.hasBackup {
			helpKeys = append(helpKeys, fmt.Sprintf("%s Rollback", StyleHelpKey.Render("[R]")))
		}
		helpKeys = append(helpKeys, fmt.Sprintf("%s Theme", StyleHelpKey.Render("[Y]")))
		helpKeys = append(helpKeys, fmt.Sprintf("%s HF Token", StyleHelpKey.Render("[T]")))
		helpKeys = append(helpKeys, fmt.Sprintf("%s GH Token", StyleHelpKey.Render("[G]")))
		helpKeys = append(helpKeys, fmt.Sprintf("%s Reset Tour", StyleHelpKey.Render("[N]")))
	}
	helpKeys = append(helpKeys, fmt.Sprintf("%s Back", StyleHelpKey.Render("[Esc]")))

	sb.WriteString("  " + strings.Join(helpKeys, " │ ") + "\n")

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

func renderProgressBar(width int, fraction float64) string {
	barWidth := width - 10
	if barWidth < 10 {
		barWidth = 10
	}
	filledWidth := int(float64(barWidth) * fraction)
	if filledWidth > barWidth {
		filledWidth = barWidth
	}
	emptyWidth := barWidth - filledWidth

	filled := lipgloss.NewStyle().Foreground(ColorSecondary).Render(strings.Repeat("█", filledWidth))
	empty := lipgloss.NewStyle().Foreground(ColorMuted).Render(strings.Repeat("░", emptyWidth))
	percent := fmt.Sprintf(" %3.0f%%", fraction*100.0)

	return filled + empty + percent
}
