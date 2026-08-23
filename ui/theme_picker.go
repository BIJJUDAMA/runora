package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ThemePickerModel is an interactive modal dialog for selecting and previewing themes.
type ThemePickerModel struct {
	themes          []Theme
	activeIdx       int
	originalThemeID string
	scrollOffset    int
	maxVisible      int
}

// NewThemePickerModel initializes a ThemePickerModel with the current active theme selected.
func NewThemePickerModel(currentThemeID string) *ThemePickerModel {
	themes := GetRegisteredThemes()
	activeIdx := 0
	for i, t := range themes {
		if strings.EqualFold(t.ID(), currentThemeID) {
			activeIdx = i
			break
		}
	}

	return &ThemePickerModel{
		themes:          themes,
		activeIdx:       activeIdx,
		originalThemeID: currentThemeID,
		maxVisible:      6,
	}
}

// Update handles keyboard input for navigating, previewing, confirming, or cancelling theme selection.
// Returns (cmd, done, applied, themeID) where:
// - done: true when the modal is closed
// - applied: true if confirmed, false if cancelled/reverted
// - themeID: the chosen or original theme ID
func (m *ThemePickerModel) Update(msg tea.Msg) (tea.Cmd, bool, bool, string) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.activeIdx > 0 {
				m.activeIdx--
			} else {
				m.activeIdx = len(m.themes) - 1
			}
			// Instant live preview on selection change
			ApplyTheme(m.themes[m.activeIdx].ID())
			return nil, false, false, m.themes[m.activeIdx].ID()

		case "down", "j":
			if m.activeIdx < len(m.themes)-1 {
				m.activeIdx++
			} else {
				m.activeIdx = 0
			}
			// Instant live preview on selection change
			ApplyTheme(m.themes[m.activeIdx].ID())
			return nil, false, false, m.themes[m.activeIdx].ID()

		case "enter", "space", "y", "Y":
			chosen := m.themes[m.activeIdx].ID()
			ApplyTheme(chosen)
			return nil, true, true, chosen

		case "esc", "q":
			// Revert to original theme
			ApplyTheme(m.originalThemeID)
			return nil, true, false, m.originalThemeID
		}
	}

	return nil, false, false, ""
}

// ActiveThemeItem returns the currently highlighted theme.
func (m *ThemePickerModel) ActiveThemeItem() Theme {
	if m.activeIdx >= 0 && m.activeIdx < len(m.themes) {
		return m.themes[m.activeIdx]
	}
	return m.themes[0]
}

// renderSwatches creates live colored blocks representing the palette tokens.
func renderSwatches(p ThemePalette) string {
	swatches := []struct {
		color lipgloss.TerminalColor
		char  string
	}{
		{p.Primary, "■"},
		{p.Secondary, "■"},
		{p.Accent, "■"},
		{p.Gold, "■"},
		{p.Success, "■"},
		{p.Warning, "■"},
		{p.Danger, "■"},
	}

	var sb strings.Builder
	for _, s := range swatches {
		if s.color != nil {
			sb.WriteString(lipgloss.NewStyle().Foreground(s.color).Render(s.char))
		}
	}
	return sb.String()
}

// View renders the theme picker modal overlay.
func (m *ThemePickerModel) View(width, height int) string {
	boxWidth := width - 10
	if boxWidth < 56 {
		boxWidth = 56
	}
	if boxWidth > 74 {
		boxWidth = 74
	}

	// Adjust scroll window for list
	if m.activeIdx < m.scrollOffset {
		m.scrollOffset = m.activeIdx
	} else if m.activeIdx >= m.scrollOffset+m.maxVisible {
		m.scrollOffset = m.activeIdx - m.maxVisible + 1
	}

	end := m.scrollOffset + m.maxVisible
	if end > len(m.themes) {
		end = len(m.themes)
	}

	var sb strings.Builder

	// Header
	headerTitle := RenderGradient("THEME SELECTOR", ThemeGradientStart, ThemeGradientEnd)
	sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Bold(true).Render(headerTitle)))
	sb.WriteString(fmt.Sprintf("  %s\n\n", StyleHelp.Render("Navigate with [↑/↓] or [J/K] — live preview is instant")))

	// List
	contentWidth := boxWidth - 6
	if contentWidth < 40 {
		contentWidth = 40
	}

	for i := m.scrollOffset; i < end; i++ {
		t := m.themes[i]
		p := t.Palette()
		swatches := renderSwatches(p)

		isCursor := i == m.activeIdx
		isActive := strings.EqualFold(t.ID(), ActiveTheme.ID())

		var cursorMarker string
		if isCursor {
			cursorMarker = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("▶ ")
		} else {
			cursorMarker = "  "
		}

		var activeTag string
		if isActive {
			activeTag = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render(" [ACTIVE]")
		}

		nameStr := fmt.Sprintf("%-20s", t.Name())
		if isCursor {
			nameStr = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render(nameStr)
		} else {
			nameStr = lipgloss.NewStyle().Foreground(ColorText).Bold(true).Render(nameStr)
		}

		desc := t.Description()
		if desc == "" {
			desc = "Theme palette"
		}
		maxDescLen := contentWidth - 6
		if maxDescLen < 20 {
			maxDescLen = 20
		}
		descTrunc := TruncateVisual(desc, maxDescLen, "...")
		descStyled := lipgloss.NewStyle().Foreground(ColorTextDim).Render("    " + descTrunc)

		rowTop := fmt.Sprintf("%s%s  %s%s", cursorMarker, nameStr, swatches, activeTag)
		rowBottom := descStyled

		if isCursor {
			itemBlock := lipgloss.NewStyle().
				Background(ColorSelected).
				Width(contentWidth).
				Padding(0, 1).
				Render(fmt.Sprintf("%s\n%s", rowTop, rowBottom))
			sb.WriteString(itemBlock + "\n")
		} else {
			itemBlock := lipgloss.NewStyle().
				Width(contentWidth).
				Padding(0, 1).
				Render(fmt.Sprintf("%s\n%s", rowTop, rowBottom))
			sb.WriteString(itemBlock + "\n")
		}
	}

	// Footer
	sb.WriteString("\n")
	footerItems := []string{
		fmt.Sprintf("%s Apply & Save", StyleHelpKey.Render("[Enter]")),
		fmt.Sprintf("%s Cancel", StyleHelpKey.Render("[Esc]")),
		fmt.Sprintf("%s Navigate", StyleHelpKey.Render("[↑/↓]")),
	}
	sb.WriteString("  " + strings.Join(footerItems, "   ") + "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorPrimary).
		Background(ColorDim).
		Padding(1, 2).
		Width(boxWidth).
		Render(sb.String())
}
