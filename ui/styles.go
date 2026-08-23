package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ThemePalette defines semantic color tokens (analogous to CSS variables in a global stylesheet).
type ThemePalette struct {
	Primary        lipgloss.TerminalColor
	Secondary      lipgloss.TerminalColor
	Accent         lipgloss.TerminalColor
	Border         lipgloss.TerminalColor
	Selected       lipgloss.TerminalColor
	Muted          lipgloss.TerminalColor
	Text           lipgloss.TerminalColor
	TextMuted      lipgloss.TerminalColor
	TextDim        lipgloss.TerminalColor
	TextOnAccent   lipgloss.TerminalColor
	Gold           lipgloss.TerminalColor
	Focus          lipgloss.TerminalColor
	Dim            lipgloss.TerminalColor
	Success        lipgloss.TerminalColor
	Warning        lipgloss.TerminalColor
	Danger         lipgloss.TerminalColor
	ProgressFilled lipgloss.TerminalColor
	ProgressEmpty  lipgloss.TerminalColor
	GradientStart  string
	GradientEnd    string
}

// Theme defines the contract for any theme class in the application.
type Theme interface {
	ID() string
	Name() string
	Palette() ThemePalette
}

// --- Base Default Theme (Fallback / Standard Terminal Palette) ---
type BaseTheme struct{}

func (t BaseTheme) ID() string   { return "base" }
func (t BaseTheme) Name() string { return "Default" }
func (t BaseTheme) Palette() ThemePalette {
	return ThemePalette{
		Primary:        lipgloss.Color("#7D56F4"),
		Secondary:      lipgloss.Color("#04B575"),
		Accent:         lipgloss.Color("#FF5F87"),
		Border:         lipgloss.Color("#3C3C3C"),
		Selected:       lipgloss.Color("#2E2E3E"),
		Muted:          lipgloss.Color("#626262"),
		Text:           lipgloss.Color("#FAFAFA"),
		TextMuted:      lipgloss.Color("#D0D0D0"),
		TextDim:        lipgloss.Color("#808080"),
		TextOnAccent:   lipgloss.Color("#000000"),
		Gold:           lipgloss.Color("#FFB800"),
		Focus:          lipgloss.Color("#7D56F4"),
		Dim:            lipgloss.Color("#2E2E2E"),
		Success:        lipgloss.Color("#04B575"),
		Warning:        lipgloss.Color("#FFB800"),
		Danger:         lipgloss.Color("#FF3B30"),
		ProgressFilled: lipgloss.Color("#7D56F4"),
		ProgressEmpty:  lipgloss.Color("#2E2E2E"),
		GradientStart:  "#7D56F4",
		GradientEnd:    "#FF5F87",
	}
}

// --- Dracula / Dark Purple Theme ---
type DraculaTheme struct{}

func (t DraculaTheme) ID() string   { return "dracula" }
func (t DraculaTheme) Name() string { return "Dracula" }
func (t DraculaTheme) Palette() ThemePalette {
	return ThemePalette{
		Primary:        lipgloss.Color("#BD93F9"),
		Secondary:      lipgloss.Color("#50FA7B"),
		Accent:         lipgloss.Color("#FF79C6"),
		Border:         lipgloss.Color("#44475A"),
		Selected:       lipgloss.Color("#282A36"),
		Muted:          lipgloss.Color("#6272A4"),
		Text:           lipgloss.Color("#F8F8F2"),
		TextMuted:      lipgloss.Color("#BD93F9"),
		TextDim:        lipgloss.Color("#44475A"),
		TextOnAccent:   lipgloss.Color("#000000"),
		Gold:           lipgloss.Color("#F1FA8C"),
		Focus:          lipgloss.Color("#BD93F9"),
		Dim:            lipgloss.Color("#21222C"),
		Success:        lipgloss.Color("#50FA7B"),
		Warning:        lipgloss.Color("#F1FA8C"),
		Danger:         lipgloss.Color("#FF5555"),
		ProgressFilled: lipgloss.Color("#BD93F9"),
		ProgressEmpty:  lipgloss.Color("#282A36"),
		GradientStart:  "#BD93F9",
		GradientEnd:    "#FF79C6",
	}
}

// --- Forest Theme ---
type ForestTheme struct{}

func (t ForestTheme) ID() string   { return "forest" }
func (t ForestTheme) Name() string { return "Forest" }
func (t ForestTheme) Palette() ThemePalette {
	return ThemePalette{
		Primary:        lipgloss.Color("#2E7D32"),
		Secondary:      lipgloss.Color("#81C784"),
		Accent:         lipgloss.Color("#FFB74D"),
		Border:         lipgloss.Color("#2E3B2E"),
		Selected:       lipgloss.Color("#1B2E1B"),
		Muted:          lipgloss.Color("#6B8E23"),
		Text:           lipgloss.Color("#F1F8E9"),
		TextMuted:      lipgloss.Color("#C8E6C9"),
		TextDim:        lipgloss.Color("#2E3B2E"),
		TextOnAccent:   lipgloss.Color("#000000"),
		Gold:           lipgloss.Color("#FFB74D"),
		Focus:          lipgloss.Color("#81C784"),
		Dim:            lipgloss.Color("#121A12"),
		Success:        lipgloss.Color("#81C784"),
		Warning:        lipgloss.Color("#FFB74D"),
		Danger:         lipgloss.Color("#E57373"),
		ProgressFilled: lipgloss.Color("#2E7D32"),
		ProgressEmpty:  lipgloss.Color("#121A12"),
		GradientStart:  "#2E7D32",
		GradientEnd:    "#81C784",
	}
}

// --- Nord Theme ---
type NordTheme struct{}

func (t NordTheme) ID() string   { return "nord" }
func (t NordTheme) Name() string { return "Nord" }
func (t NordTheme) Palette() ThemePalette {
	return ThemePalette{
		Primary:        lipgloss.Color("#88C0D0"),
		Secondary:      lipgloss.Color("#A3BE8C"),
		Accent:         lipgloss.Color("#EBCB8B"),
		Border:         lipgloss.Color("#3B4252"),
		Selected:       lipgloss.Color("#2E3440"),
		Muted:          lipgloss.Color("#65728A"),
		Text:           lipgloss.Color("#ECEFF4"),
		TextMuted:      lipgloss.Color("#D8DEE9"),
		TextDim:        lipgloss.Color("#3B4252"),
		TextOnAccent:   lipgloss.Color("#000000"),
		Gold:           lipgloss.Color("#EBCB8B"),
		Focus:          lipgloss.Color("#88C0D0"),
		Dim:            lipgloss.Color("#242933"),
		Success:        lipgloss.Color("#A3BE8C"),
		Warning:        lipgloss.Color("#EBCB8B"),
		Danger:         lipgloss.Color("#BF616A"),
		ProgressFilled: lipgloss.Color("#88C0D0"),
		ProgressEmpty:  lipgloss.Color("#242933"),
		GradientStart:  "#88C0D0",
		GradientEnd:    "#5E81AC",
	}
}

// --- Sunset Theme ---
type SunsetTheme struct{}

func (t SunsetTheme) ID() string   { return "sunset" }
func (t SunsetTheme) Name() string { return "Sunset" }
func (t SunsetTheme) Palette() ThemePalette {
	return ThemePalette{
		Primary:        lipgloss.Color("#FF5F6D"),
		Secondary:      lipgloss.Color("#04B575"),
		Accent:         lipgloss.Color("#FFC371"),
		Border:         lipgloss.Color("#4A2E2E"),
		Selected:       lipgloss.Color("#3D2222"),
		Muted:          lipgloss.Color("#8A6F6F"),
		Text:           lipgloss.Color("#FAFAFA"),
		TextMuted:      lipgloss.Color("#E0C0C0"),
		TextDim:        lipgloss.Color("#4A2E2E"),
		TextOnAccent:   lipgloss.Color("#000000"),
		Gold:           lipgloss.Color("#FFB800"),
		Focus:          lipgloss.Color("#FF5F6D"),
		Dim:            lipgloss.Color("#2E1E1E"),
		Success:        lipgloss.Color("#04B575"),
		Warning:        lipgloss.Color("#FFB800"),
		Danger:         lipgloss.Color("#FF3B30"),
		ProgressFilled: lipgloss.Color("#FF5F6D"),
		ProgressEmpty:  lipgloss.Color("#2E1E1E"),
		GradientStart:  "#FF5F6D",
		GradientEnd:    "#FFC371",
	}
}

// --- Cyberpunk Theme ---
type CyberpunkTheme struct{}

func (t CyberpunkTheme) ID() string   { return "cyberpunk" }
func (t CyberpunkTheme) Name() string { return "Cyberpunk" }
func (t CyberpunkTheme) Palette() ThemePalette {
	return ThemePalette{
		Primary:        lipgloss.Color("#FF007F"),
		Secondary:      lipgloss.Color("#00F3FF"),
		Accent:         lipgloss.Color("#FFE600"),
		Border:         lipgloss.Color("#2A0845"),
		Selected:       lipgloss.Color("#2A0033"),
		Muted:          lipgloss.Color("#7F5A83"),
		Text:           lipgloss.Color("#FFFFFF"),
		TextMuted:      lipgloss.Color("#E0B0FF"),
		TextDim:        lipgloss.Color("#2A0845"),
		TextOnAccent:   lipgloss.Color("#000000"),
		Gold:           lipgloss.Color("#FFE600"),
		Focus:          lipgloss.Color("#FFE600"),
		Dim:            lipgloss.Color("#1A001A"),
		Success:        lipgloss.Color("#00F3FF"),
		Warning:        lipgloss.Color("#FFE600"),
		Danger:         lipgloss.Color("#FF003C"),
		ProgressFilled: lipgloss.Color("#FF007F"),
		ProgressEmpty:  lipgloss.Color("#1A001A"),
		GradientStart:  "#FF007F",
		GradientEnd:    "#00F3FF",
	}
}

// --- Monochrome / Clean Minimalist Theme ---
type MonochromeTheme struct{}

func (t MonochromeTheme) ID() string   { return "monochrome" }
func (t MonochromeTheme) Name() string { return "Monochrome" }
func (t MonochromeTheme) Palette() ThemePalette {
	return ThemePalette{
		Primary:        lipgloss.Color("#FAFAFA"),
		Secondary:      lipgloss.Color("#A0A0A0"),
		Accent:         lipgloss.Color("#FAFAFA"),
		Border:         lipgloss.Color("#404040"),
		Selected:       lipgloss.Color("#2A2A2A"),
		Muted:          lipgloss.Color("#707070"),
		Text:           lipgloss.Color("#FFFFFF"),
		TextMuted:      lipgloss.Color("#C0C0C0"),
		TextDim:        lipgloss.Color("#404040"),
		TextOnAccent:   lipgloss.Color("#000000"),
		Gold:           lipgloss.Color("#FFFFFF"),
		Focus:          lipgloss.Color("#FAFAFA"),
		Dim:            lipgloss.Color("#181818"),
		Success:        lipgloss.Color("#FAFAFA"),
		Warning:        lipgloss.Color("#A0A0A0"),
		Danger:         lipgloss.Color("#707070"),
		ProgressFilled: lipgloss.Color("#FAFAFA"),
		ProgressEmpty:  lipgloss.Color("#181818"),
		GradientStart:  "#FAFAFA",
		GradientEnd:    "#606060",
	}
}

var registeredThemes = []Theme{
	ForestTheme{},
	DraculaTheme{},
	NordTheme{},
	SunsetTheme{},
	CyberpunkTheme{},
	MonochromeTheme{},
	BaseTheme{},
}

func NextThemeName(current string) string {
	current = strings.ToLower(current)
	for i, t := range registeredThemes {
		if t.ID() == current || (current == "" && t.ID() == "forest") {
			nextIdx := (i + 1) % len(registeredThemes)
			return registeredThemes[nextIdx].ID()
		}
	}
	return registeredThemes[0].ID()
}

func resolveTheme(themeName string) Theme {
	switch strings.ToLower(themeName) {
	case "sunset":
		return SunsetTheme{}
	case "nord":
		return NordTheme{}
	case "cyberpunk":
		return CyberpunkTheme{}
	case "forest":
		return ForestTheme{}
	case "monochrome":
		return MonochromeTheme{}
	case "base", "default":
		return BaseTheme{}
	default:
		return DraculaTheme{}
	}
}

// ── Global Semantic Tokens (CSS Variables) ────────────────────────────────────
var (
	ActiveTheme Theme

	ColorPrimary        lipgloss.TerminalColor
	ColorSecondary      lipgloss.TerminalColor
	ColorBorder         lipgloss.TerminalColor
	ColorSelected       lipgloss.TerminalColor
	ColorAccent         lipgloss.TerminalColor
	ColorMuted          lipgloss.TerminalColor
	ColorWhite          lipgloss.TerminalColor // Alias for ColorText
	ColorText           lipgloss.TerminalColor
	ColorTextMuted      lipgloss.TerminalColor
	ColorTextOnAccent   lipgloss.TerminalColor
	ColorGold           lipgloss.TerminalColor
	ColorFocus          lipgloss.TerminalColor
	ColorDim            lipgloss.TerminalColor
	ColorProgressFilled lipgloss.TerminalColor
	ColorProgressEmpty  lipgloss.TerminalColor

	ColorSuccess lipgloss.TerminalColor
	ColorWarning lipgloss.TerminalColor
	ColorDanger  lipgloss.TerminalColor

	ThemeGradientStart string
	ThemeGradientEnd   string

	// ── Global Component Styles (Global CSS Classes) ──────────────────────────
	StyleHeader           lipgloss.Style
	StyleTitle            lipgloss.Style
	StyleLeftPanel        lipgloss.Style
	StyleRightPanel       lipgloss.Style
	StyleSelectedListItem lipgloss.Style
	StyleListItem         lipgloss.Style
	StyleHelp             lipgloss.Style
	StyleHelpKey          lipgloss.Style
	StyleSearchActive     lipgloss.Style
	StyleStar             lipgloss.Style

	// Status Badges
	StyleBadgeRunning  lipgloss.Style
	StyleBadgeStopped  lipgloss.Style
	StyleBadgeFailed   lipgloss.Style
	StyleBadgeStarting lipgloss.Style
	StyleBadgeFits     lipgloss.Style
	StyleBadgePartial  lipgloss.Style
	StyleBadgeExceeds   lipgloss.Style
	StyleTagPill       lipgloss.Style

	// Severity / Status
	StyleSecondary lipgloss.Style
	StyleSuccess   lipgloss.Style
	StyleWarning   lipgloss.Style
	StyleDanger    lipgloss.Style
)

func init() {
	// Initialize default theme
	ApplyTheme("forest")
}

// ApplyTheme sets the active theme class and updates all global CSS variable styles.
func ApplyTheme(themeName string) {
	theme := resolveTheme(themeName)
	ActiveTheme = theme
	p := theme.Palette()

	// Update semantic variables
	ColorPrimary = p.Primary
	ColorSecondary = p.Secondary
	ColorBorder = p.Border
	ColorSelected = p.Selected
	ColorAccent = p.Accent
	ColorMuted = p.Muted
	ColorWhite = p.Text
	ColorText = p.Text
	ColorTextMuted = p.TextMuted
	ColorTextOnAccent = p.TextOnAccent
	ColorGold = p.Gold
	ColorFocus = p.Focus
	ColorDim = p.Dim
	ColorProgressFilled = p.ProgressFilled
	ColorProgressEmpty = p.ProgressEmpty

	ColorSuccess = p.Success
	ColorWarning = p.Warning
	ColorDanger = p.Danger

	ThemeGradientStart = p.GradientStart
	ThemeGradientEnd = p.GradientEnd

	// Rebuild Central Component Styles (Global Stylesheet)
	StyleHeader = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true).
		MarginBottom(1)

	StyleTitle = lipgloss.NewStyle().
		Foreground(ColorText).
		Bold(true).
		Padding(0, 1)

	StyleLeftPanel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1)

	StyleRightPanel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1)

	StyleSelectedListItem = lipgloss.NewStyle().
		Background(ColorSelected).
		Foreground(ColorSecondary).
		Bold(true)

	StyleListItem = lipgloss.NewStyle().
		Foreground(ColorText)

	StyleHelp = lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1)

	StyleHelpKey = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true)

	StyleSearchActive = lipgloss.NewStyle().
		Foreground(ColorAccent).
		Bold(true)

	StyleStar = lipgloss.NewStyle().
		Foreground(ColorGold)

	StyleBadgeRunning = lipgloss.NewStyle().
		Background(ColorSecondary).
		Foreground(ColorTextOnAccent).
		Bold(true).
		Padding(0, 1)

	StyleBadgeStopped = lipgloss.NewStyle().
		Background(ColorMuted).
		Foreground(ColorText).
		Bold(true).
		Padding(0, 1)

	StyleBadgeFailed = lipgloss.NewStyle().
		Background(ColorDanger).
		Foreground(ColorText).
		Bold(true).
		Padding(0, 1)

	StyleBadgeStarting = lipgloss.NewStyle().
		Background(ColorWarning).
		Foreground(ColorTextOnAccent).
		Bold(true).
		Padding(0, 1)

	StyleBadgeFits = lipgloss.NewStyle().
		Background(ColorSecondary).
		Foreground(ColorTextOnAccent).
		Bold(true).
		Padding(0, 1)

	StyleBadgePartial = lipgloss.NewStyle().
		Background(ColorGold).
		Foreground(ColorTextOnAccent).
		Bold(true).
		Padding(0, 1)

	StyleBadgeExceeds = lipgloss.NewStyle().
		Background(ColorDanger).
		Foreground(ColorText).
		Bold(true).
		Padding(0, 1)

	StyleTagPill = lipgloss.NewStyle().
		Background(ColorDim).
		Foreground(ColorTextMuted).
		Padding(0, 1)

	StyleSecondary = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	StyleSuccess = lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	StyleWarning = lipgloss.NewStyle().Foreground(ColorWarning).Bold(true)
	StyleDanger = lipgloss.NewStyle().Foreground(ColorDanger).Bold(true)
}

func RenderProgressBar(percent float64, width int, filledColor lipgloss.TerminalColor) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	if width < 5 {
		width = 5
	}
	filledCount := int(percent / 100.0 * float64(width))
	if filledCount < 0 {
		filledCount = 0
	}
	if filledCount > width {
		filledCount = width
	}
	emptyCount := width - filledCount

	filledStr := ""
	if filledCount > 0 {
		filledStr = strings.Repeat("█", filledCount)
	}
	emptyStr := ""
	if emptyCount > 0 {
		emptyStr = strings.Repeat("░", emptyCount)
	}

	if filledColor == nil {
		filledColor = ColorProgressFilled
	}
	filledStyle := lipgloss.NewStyle().Foreground(filledColor)
	emptyStyle := lipgloss.NewStyle().Foreground(ColorProgressEmpty)

	return filledStyle.Render(filledStr) + emptyStyle.Render(emptyStr)
}

func parseHexColor(hex string) (r, g, b int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 3 {
		fmt.Sscanf(string(hex[0])+string(hex[0])+string(hex[1])+string(hex[1])+string(hex[2])+string(hex[2]), "%02x%02x%02x", &r, &g, &b)
	} else if len(hex) == 6 {
		fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	}
	return
}

func interpolateColor(startHex, endHex string, fraction float64) lipgloss.Color {
	r1, g1, b1 := parseHexColor(startHex)
	r2, g2, b2 := parseHexColor(endHex)

	r := int(float64(r1) + float64(r2-r1)*fraction)
	g := int(float64(g1) + float64(g2-g1)*fraction)
	b := int(float64(b1) + float64(b2-b1)*fraction)

	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", r, g, b))
}

func RenderGradient(text string, startHex, endHex string) string {
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return ""
	}
	if n == 1 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(startHex)).Bold(true).Render(text)
	}

	var sb strings.Builder
	for i, char := range runes {
		fraction := float64(i) / float64(n-1)
		c := interpolateColor(startHex, endHex, fraction)
		sb.WriteString(lipgloss.NewStyle().Foreground(c).Bold(true).Render(string(char)))
	}
	return sb.String()
}
