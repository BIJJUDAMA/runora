package ui

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/profile"
)

type ProfileCreatorMode int

const (
	ModeCreate ProfileCreatorMode = iota
	ModeEdit
	ModeDuplicate
)

type ProfileCreatorModel struct {
	profilesDir     string
	mode            ProfileCreatorMode
	origName        string
	nameInput       textinput.Model
	ctxInput        textinput.Model
	threadsInput    textinput.Model
	gpuInput        textinput.Model
	portInput       textinput.Model
	flashAttn       bool
	kvQuantIdx      int
	customArgsInput textinput.Model
	focusIndex      int
}

var kvQuantOptions = []string{"FP16", "Q8_0", "Q4_0", "FP8"}

func NewProfileCreatorModel(profilesDir string) *ProfileCreatorModel {
	nameTi := textinput.New()
	nameTi.Placeholder = "Enter name (e.g. Custom-8K, Coding-32K)..."
	nameTi.CharLimit = 50
	nameTi.Width = 40
	nameTi.Focus()

	ctxTi := textinput.New()
	ctxTi.Placeholder = "Enter context size (e.g. 8192, 16384)..."
	ctxTi.CharLimit = 10
	ctxTi.Width = 25

	threadsTi := textinput.New()
	threadsTi.Placeholder = fmt.Sprintf("Thread count (default %d)...", max(1, runtime.NumCPU()/2))
	threadsTi.CharLimit = 5
	threadsTi.Width = 25

	gpuTi := textinput.New()
	gpuTi.Placeholder = "Layers to offload (0 for CPU, 999 for max)..."
	gpuTi.CharLimit = 5
	gpuTi.Width = 25

	portTi := textinput.New()
	portTi.Placeholder = "Server port (e.g. 50505)..."
	portTi.CharLimit = 5
	portTi.Width = 25

	customArgsTi := textinput.New()
	customArgsTi.Placeholder = "Extra CLI flags (e.g. --temp 0.7 --top-p 0.9)..."
	customArgsTi.CharLimit = 200
	customArgsTi.Width = 55

	return &ProfileCreatorModel{
		profilesDir:     profilesDir,
		mode:            ModeCreate,
		nameInput:       nameTi,
		ctxInput:        ctxTi,
		threadsInput:    threadsTi,
		gpuInput:        gpuTi,
		portInput:       portTi,
		flashAttn:       true,
		kvQuantIdx:      0,
		customArgsInput: customArgsTi,
		focusIndex:      0,
	}
}

func NewProfileEditorModel(profilesDir string, p *profile.Profile, isDuplicate bool) *ProfileCreatorModel {
	nameTi := textinput.New()
	nameTi.Placeholder = "Enter profile name..."
	nameTi.CharLimit = 50
	nameTi.Width = 40

	if isDuplicate {
		nameTi.SetValue(p.Name + " (Copy)")
	} else {
		nameTi.SetValue(p.Name)
	}
	nameTi.Focus()

	ctxTi := textinput.New()
	ctxTi.Placeholder = "Enter context size (e.g. 8192, 16384)..."
	ctxTi.CharLimit = 10
	ctxTi.Width = 25
	ctxTi.SetValue(strconv.FormatUint(uint64(p.Context), 10))

	threadsTi := textinput.New()
	threadsTi.Placeholder = "Thread count..."
	threadsTi.CharLimit = 5
	threadsTi.Width = 25
	threadsTi.SetValue(strconv.Itoa(p.Threads))

	gpuTi := textinput.New()
	gpuTi.Placeholder = "Layers to offload (0 for CPU, 999 for max)..."
	gpuTi.CharLimit = 5
	gpuTi.Width = 25
	gpuTi.SetValue(strconv.Itoa(p.GPULayers))

	portTi := textinput.New()
	portTi.Placeholder = "Server port (e.g. 50505)..."
	portTi.CharLimit = 5
	portTi.Width = 25
	portTi.SetValue(strconv.Itoa(p.Port))

	customArgsTi := textinput.New()
	customArgsTi.Placeholder = "Extra CLI flags (e.g. --temp 0.7)..."
	customArgsTi.CharLimit = 200
	customArgsTi.Width = 55
	customArgsTi.SetValue(p.CustomArgs)

	kvIdx := 0
	switch strings.ToLower(p.CacheTypeK) {
	case "q8_0":
		kvIdx = 1
	case "q4_0":
		kvIdx = 2
	case "fp8":
		kvIdx = 3
	}

	mode := ModeEdit
	if isDuplicate {
		mode = ModeDuplicate
	}

	return &ProfileCreatorModel{
		profilesDir:     profilesDir,
		mode:            mode,
		origName:        p.Name,
		nameInput:       nameTi,
		ctxInput:        ctxTi,
		threadsInput:    threadsTi,
		gpuInput:        gpuTi,
		portInput:       portTi,
		flashAttn:       p.FlashAttention,
		kvQuantIdx:      kvIdx,
		customArgsInput: customArgsTi,
		focusIndex:      0,
	}
}

func (pc *ProfileCreatorModel) Update(msg tea.Msg) (tea.Cmd, bool, bool) {
	var cmd tea.Cmd

	switch pc.focusIndex {
	case 0:
		pc.nameInput, cmd = pc.nameInput.Update(msg)
	case 1:
		pc.ctxInput, cmd = pc.ctxInput.Update(msg)
	case 2:
		pc.threadsInput, cmd = pc.threadsInput.Update(msg)
	case 3:
		pc.gpuInput, cmd = pc.gpuInput.Update(msg)
	case 4:
		pc.portInput, cmd = pc.portInput.Update(msg)
	case 7:
		pc.customArgsInput, cmd = pc.customArgsInput.Update(msg)
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "ctrl+v":
			switch pc.focusIndex {
			case 0:
				pasteFromClipboard(&pc.nameInput)
			case 1:
				pasteFromClipboard(&pc.ctxInput)
			case 2:
				pasteFromClipboard(&pc.threadsInput)
			case 3:
				pasteFromClipboard(&pc.gpuInput)
			case 4:
				pasteFromClipboard(&pc.portInput)
			case 7:
				pasteFromClipboard(&pc.customArgsInput)
			}
		case "tab", "down":
			pc.focusIndex = (pc.focusIndex + 1) % 8
			pc.updateFocus()
		case "shift+tab", "up":
			pc.focusIndex = (pc.focusIndex - 1 + 8) % 8
			pc.updateFocus()
		case "space", " ", "left", "right":
			if pc.focusIndex == 5 {
				pc.flashAttn = !pc.flashAttn
			} else if pc.focusIndex == 6 {
				if keyMsg.String() == "left" {
					pc.kvQuantIdx = (pc.kvQuantIdx - 1 + len(kvQuantOptions)) % len(kvQuantOptions)
				} else {
					pc.kvQuantIdx = (pc.kvQuantIdx + 1) % len(kvQuantOptions)
				}
			}
		case "esc":
			return nil, true, false
		case "enter", "ctrl+s":
			if pc.focusIndex == 5 {
				pc.flashAttn = !pc.flashAttn
				return nil, false, false
			}
			if pc.focusIndex == 6 {
				pc.kvQuantIdx = (pc.kvQuantIdx + 1) % len(kvQuantOptions)
				return nil, false, false
			}

			name := strings.TrimSpace(pc.nameInput.Value())
			if name == "" || profile.IsReservedWindowsName(name) {
				return nil, false, false
			}

			// Parse values with default fallbacks
			ctxVal := uint32(2048)
			if c, err := strconv.ParseUint(strings.TrimSpace(pc.ctxInput.Value()), 10, 32); err == nil && c >= 256 {
				ctxVal = uint32(c)
			}

			threads := runtime.NumCPU() / 2
			if threads < 1 {
				threads = 1
			}
			if t, err := strconv.Atoi(strings.TrimSpace(pc.threadsInput.Value())); err == nil && t >= 1 {
				threads = t
			}

			gpuVal := 999
			if g, err := strconv.Atoi(strings.TrimSpace(pc.gpuInput.Value())); err == nil && g >= 0 && g <= 999 {
				gpuVal = g
			}

			portVal := 50505
			if p, err := strconv.Atoi(strings.TrimSpace(pc.portInput.Value())); err == nil && p >= 1024 && p <= 65535 {
				portVal = p
			}

			kQuant := ""
			vQuant := ""
			switch kvQuantOptions[pc.kvQuantIdx] {
			case "Q8_0":
				kQuant = "q8_0"
				vQuant = "q8_0"
			case "Q4_0":
				kQuant = "q4_0"
				vQuant = "q4_0"
			case "FP8":
				kQuant = "fp8"
				vQuant = "fp8"
			}

			newProfile := &profile.Profile{
				Name:           name,
				Context:        ctxVal,
				Threads:        threads,
				GPULayers:      gpuVal,
				BatchSize:      512,
				Host:           "127.0.0.1",
				Port:           portVal,
				FlashAttention: pc.flashAttn,
				CacheTypeK:     kQuant,
				CacheTypeV:     vQuant,
				CustomArgs:     strings.TrimSpace(pc.customArgsInput.Value()),
			}

			if err := newProfile.Validate(); err != nil {
				return nil, false, false
			}

			// If editing and name changed, remove old profile
			if pc.mode == ModeEdit && pc.origName != "" && pc.origName != name {
				_ = profile.DeleteProfile(pc.profilesDir, pc.origName)
			}

			if err := profile.SaveProfile(pc.profilesDir, newProfile); err != nil {
				return nil, false, false
			}

			return nil, true, true
		}
	}

	return cmd, false, false
}

func (pc *ProfileCreatorModel) updateFocus() {
	pc.nameInput.Blur()
	pc.ctxInput.Blur()
	pc.threadsInput.Blur()
	pc.gpuInput.Blur()
	pc.portInput.Blur()
	pc.customArgsInput.Blur()

	switch pc.focusIndex {
	case 0:
		pc.nameInput.Focus()
	case 1:
		pc.ctxInput.Focus()
	case 2:
		pc.threadsInput.Focus()
	case 3:
		pc.gpuInput.Focus()
	case 4:
		pc.portInput.Focus()
	case 7:
		pc.customArgsInput.Focus()
	}
}

func (pc *ProfileCreatorModel) View(width int, height int) string {
	var sb strings.Builder

	labels := []string{
		"1. Profile Name:",
		"2. Context Size (tokens):",
		"3. CPU Threads:",
		"4. GPU Layers (0 for CPU, 999 for max):",
		"5. Server Port:",
		"6. Flash Attention:",
		"7. KV Cache Quantization:",
		"8. Custom CLI Arguments:",
	}

	styles := make([]lipgloss.Style, 8)
	for i := range styles {
		if i == pc.focusIndex {
			styles[i] = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
		} else {
			styles[i] = lipgloss.NewStyle().Foreground(ColorWhite)
		}
	}

	sb.WriteString("  " + styles[0].Render(labels[0]) + "\n")
	sb.WriteString("  " + pc.nameInput.View() + "\n\n")

	sb.WriteString("  " + styles[1].Render(labels[1]) + "\n")
	sb.WriteString("  " + pc.ctxInput.View() + "\n\n")

	sb.WriteString("  " + styles[2].Render(labels[2]) + "\n")
	sb.WriteString("  " + pc.threadsInput.View() + "\n\n")

	sb.WriteString("  " + styles[3].Render(labels[3]) + "\n")
	sb.WriteString("  " + pc.gpuInput.View() + "\n\n")

	sb.WriteString("  " + styles[4].Render(labels[4]) + "\n")
	sb.WriteString("  " + pc.portInput.View() + "\n\n")

	faPill := "[x] Enabled (--flash-attn)"
	if !pc.flashAttn {
		faPill = "[ ] Disabled"
	}
	if pc.focusIndex == 5 {
		faPill = lipgloss.NewStyle().Background(ColorPrimary).Foreground(ColorTextOnAccent).Bold(true).Padding(0, 1).Render(faPill)
	} else {
		faPill = lipgloss.NewStyle().Foreground(ColorTextDim).Padding(0, 1).Render(faPill)
	}
	sb.WriteString("  " + styles[5].Render(labels[5]) + "  " + faPill + "  " + StyleHelpKey.Render("[Space/Enter to toggle]") + "\n\n")

	var kvPills []string
	for i, opt := range kvQuantOptions {
		if i == pc.kvQuantIdx {
			kvPills = append(kvPills, lipgloss.NewStyle().Background(ColorPrimary).Foreground(ColorTextOnAccent).Bold(true).Padding(0, 1).Render(opt))
		} else {
			kvPills = append(kvPills, lipgloss.NewStyle().Foreground(ColorMuted).Padding(0, 1).Render(opt))
		}
	}
	sb.WriteString("  " + styles[6].Render(labels[6]) + "  " + strings.Join(kvPills, " ") + "  " + StyleHelpKey.Render("[Left/Right to cycle]") + "\n\n")

	sb.WriteString("  " + styles[7].Render(labels[7]) + "\n")
	sb.WriteString("  " + pc.customArgsInput.View() + "\n\n")

	sb.WriteString(fmt.Sprintf("  %s Save Profile | %s Navigate | %s Cancel",
		StyleHelpKey.Render("[Enter/Ctrl+S]"),
		StyleHelpKey.Render("[Tab/Shift+Tab]"),
		StyleHelpKey.Render("[Esc]"),
	))

	var title string
	switch pc.mode {
	case ModeEdit:
		title = fmt.Sprintf("Edit Profile: %s", pc.origName)
	case ModeDuplicate:
		title = fmt.Sprintf("Duplicate Profile: %s", pc.origName)
	default:
		title = "Create Custom Profile"
	}

	cardHeight := height
	if cardHeight < 16 {
		cardHeight = 16
	}
	return SurfaceCardWithHeight(title, sb.String(), width, cardHeight, true, "Profile Config")
}
