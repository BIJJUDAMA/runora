package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/config"
	"github.com/BIJJUDAMA/runora/profile"
)

type ProfileCreatorModel struct {
	profilesDir string
	nameInput   textinput.Model
	ctxInput    textinput.Model
	gpuInput    textinput.Model
	portInput   textinput.Model
	focusIndex  int
}

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

	gpuTi := textinput.New()
	gpuTi.Placeholder = "Layers to offload (0 for CPU, 999 for max)..."
	gpuTi.CharLimit = 5
	gpuTi.Width = 25

	portTi := textinput.New()
	portTi.Placeholder = "Server port (e.g. 50505)..."
	portTi.CharLimit = 5
	portTi.Width = 25

	return &ProfileCreatorModel{
		profilesDir: profilesDir,
		nameInput:   nameTi,
		ctxInput:    ctxTi,
		gpuInput:    gpuTi,
		portInput:   portTi,
		focusIndex:  0,
	}
}

func (pc *ProfileCreatorModel) Update(msg tea.Msg) (tea.Cmd, bool, bool) {
	// 3 elements returned: cmd tea.Cmd, done bool, saved bool
	var cmd tea.Cmd

	switch pc.focusIndex {
	case 0:
		pc.nameInput, cmd = pc.nameInput.Update(msg)
	case 1:
		pc.ctxInput, cmd = pc.ctxInput.Update(msg)
	case 2:
		pc.gpuInput, cmd = pc.gpuInput.Update(msg)
	case 3:
		pc.portInput, cmd = pc.portInput.Update(msg)
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
				pasteFromClipboard(&pc.gpuInput)
			case 3:
				pasteFromClipboard(&pc.portInput)
			}
		case "tab":
			pc.focusIndex = (pc.focusIndex + 1) % 4
			pc.updateFocus()
		case "shift+tab":
			pc.focusIndex = (pc.focusIndex - 1 + 4) % 4
			pc.updateFocus()
		case "esc":
			return nil, true, false
		case "enter":
			name := strings.TrimSpace(pc.nameInput.Value())
			if name == "" || profile.IsReservedWindowsName(name) {
				return nil, false, false
			}

			// Parse values with default fallbacks
			ctxVal := uint32(2048)
			if c, err := strconv.ParseUint(strings.TrimSpace(pc.ctxInput.Value()), 10, 32); err == nil && c >= 256 {
				ctxVal = uint32(c)
			}

			gpuVal := 999
			if g, err := strconv.Atoi(strings.TrimSpace(pc.gpuInput.Value())); err == nil && g >= 0 && g <= 999 {
				gpuVal = g
			}

			portVal := 50505
			if p, err := strconv.Atoi(strings.TrimSpace(pc.portInput.Value())); err == nil && p >= 1024 && p <= 65535 {
				portVal = p
			}

			threads := runtime.NumCPU() / 2
			if threads < 1 {
				threads = 1
			}

			newProfile := &profile.Profile{
				Name:      name,
				Context:   ctxVal,
				Threads:   threads,
				GPULayers: gpuVal,
				BatchSize: 512,
				Host:      "127.0.0.1",
				Port:      portVal,
			}

			if err := newProfile.Validate(); err != nil {
				return nil, false, false
			}

			// Sanitize filename of reserved characters & Windows device names
			cleanName := profile.SanitizeProfileName(name)
			fileName := strings.ReplaceAll(strings.ToLower(cleanName), " ", "_") + ".json"
			_ = os.MkdirAll(pc.profilesDir, 0755)
			filePath := filepath.Join(pc.profilesDir, fileName)

			data, err := json.MarshalIndent(newProfile, "", "  ")
			if err == nil {
				if werr := config.AtomicWriteFile(filePath, data, 0644); werr != nil {
					return nil, false, false
				}
			}

			return nil, true, true
		}
	}

	return cmd, false, false
}

func (pc *ProfileCreatorModel) updateFocus() {
	pc.nameInput.Blur()
	pc.ctxInput.Blur()
	pc.gpuInput.Blur()
	pc.portInput.Blur()

	switch pc.focusIndex {
	case 0:
		pc.nameInput.Focus()
	case 1:
		pc.ctxInput.Focus()
	case 2:
		pc.gpuInput.Focus()
	case 3:
		pc.portInput.Focus()
	}
}

func (pc *ProfileCreatorModel) View(width int, height int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("CREATE CUSTOM PROFILE")))

	nameStyle := lipgloss.NewStyle().Foreground(ColorWhite)
	ctxStyle := lipgloss.NewStyle().Foreground(ColorWhite)
	gpuStyle := lipgloss.NewStyle().Foreground(ColorWhite)
	portStyle := lipgloss.NewStyle().Foreground(ColorWhite)

	switch pc.focusIndex {
	case 0:
		nameStyle = nameStyle.Foreground(ColorSecondary).Bold(true)
	case 1:
		ctxStyle = ctxStyle.Foreground(ColorSecondary).Bold(true)
	case 2:
		gpuStyle = gpuStyle.Foreground(ColorSecondary).Bold(true)
	case 3:
		portStyle = portStyle.Foreground(ColorSecondary).Bold(true)
	}

	sb.WriteString("  " + nameStyle.Render("Profile Name:") + "\n")
	sb.WriteString("  " + pc.nameInput.View() + "\n\n")

	sb.WriteString("  " + ctxStyle.Render("Context Size (tokens):") + "\n")
	sb.WriteString("  " + pc.ctxInput.View() + "\n\n")

	sb.WriteString("  " + gpuStyle.Render("GPU Layers to Offload (0 for CPU, 999 for max):") + "\n")
	sb.WriteString("  " + pc.gpuInput.View() + "\n\n")

	sb.WriteString("  " + portStyle.Render("Port Number:") + "\n")
	sb.WriteString("  " + pc.portInput.View() + "\n\n")

	helpStr := fmt.Sprintf("%s Switch fields  %s Save profile  %s Cancel",
		StyleHelpKey.Render("[Tab / Shift+Tab]"),
		StyleHelpKey.Render("[Enter]"),
		StyleHelpKey.Render("[Esc]"),
	)
	sb.WriteString("  " + helpStr + "\n")

	boxWidth := width - 4
	if boxWidth < 66 {
		boxWidth = 66
	}
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorPrimary).
		Width(boxWidth).
		Render(sb.String())
}
