package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/ui/mouse"
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
	Description() string
	Palette() ThemePalette
	GradientStart() lipgloss.Color
	GradientEnd() lipgloss.Color
}

// --- Base Default Theme (Fallback / Standard Terminal Palette) ---
type BaseTheme struct{}

func (t BaseTheme) ID() string                          { return "base" }
func (t BaseTheme) Name() string                        { return "Default" }
func (t BaseTheme) Description() string                 { return "Standard terminal default palette" }
func (t BaseTheme) GradientStart() lipgloss.Color       { return lipgloss.Color(t.Palette().GradientStart) }
func (t BaseTheme) GradientEnd() lipgloss.Color         { return lipgloss.Color(t.Palette().GradientEnd) }
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

func (t DraculaTheme) ID() string                          { return "dracula" }
func (t DraculaTheme) Name() string                        { return "Dracula" }
func (t DraculaTheme) Description() string                 { return "Classic dark purple & neon vampire theme" }
func (t DraculaTheme) GradientStart() lipgloss.Color       { return lipgloss.Color(t.Palette().GradientStart) }
func (t DraculaTheme) GradientEnd() lipgloss.Color         { return lipgloss.Color(t.Palette().GradientEnd) }
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

func (t ForestTheme) ID() string                          { return "forest" }
func (t ForestTheme) Name() string                        { return "Forest" }
func (t ForestTheme) Description() string                 { return "Natural emerald greens and warm amber accents" }
func (t ForestTheme) GradientStart() lipgloss.Color       { return lipgloss.Color(t.Palette().GradientStart) }
func (t ForestTheme) GradientEnd() lipgloss.Color         { return lipgloss.Color(t.Palette().GradientEnd) }
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

func (t NordTheme) ID() string                          { return "nord" }
func (t NordTheme) Name() string                        { return "Nord" }
func (t NordTheme) Description() string                 { return "Arctic, north-bluish clean and elegant palette" }
func (t NordTheme) GradientStart() lipgloss.Color       { return lipgloss.Color(t.Palette().GradientStart) }
func (t NordTheme) GradientEnd() lipgloss.Color         { return lipgloss.Color(t.Palette().GradientEnd) }
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

func (t SunsetTheme) ID() string                          { return "sunset" }
func (t SunsetTheme) Name() string                        { return "Sunset" }
func (t SunsetTheme) Description() string                 { return "Warm dusk glow with coral and golden hues" }
func (t SunsetTheme) GradientStart() lipgloss.Color       { return lipgloss.Color(t.Palette().GradientStart) }
func (t SunsetTheme) GradientEnd() lipgloss.Color         { return lipgloss.Color(t.Palette().GradientEnd) }
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

func (t CyberpunkTheme) ID() string                          { return "cyberpunk" }
func (t CyberpunkTheme) Name() string                        { return "Cyberpunk" }
func (t CyberpunkTheme) Description() string                 { return "High-energy neon pink, cyan and electric yellow" }
func (t CyberpunkTheme) GradientStart() lipgloss.Color       { return lipgloss.Color(t.Palette().GradientStart) }
func (t CyberpunkTheme) GradientEnd() lipgloss.Color         { return lipgloss.Color(t.Palette().GradientEnd) }
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

func (t MonochromeTheme) ID() string                          { return "monochrome" }
func (t MonochromeTheme) Name() string                        { return "Monochrome" }
func (t MonochromeTheme) Description() string                 { return "Ultra-clean grayscale minimalist palette" }
func (t MonochromeTheme) GradientStart() lipgloss.Color       { return lipgloss.Color(t.Palette().GradientStart) }
func (t MonochromeTheme) GradientEnd() lipgloss.Color         { return lipgloss.Color(t.Palette().GradientEnd) }
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

// --- Solarized Light Theme (Clean Light Background) ---
type SolarizedLightTheme struct{}

func (t SolarizedLightTheme) ID() string                          { return "solarized-light" }
func (t SolarizedLightTheme) Name() string                        { return "Solarized Light" }
func (t SolarizedLightTheme) Description() string                 { return "Classic solarized warm light background palette" }
func (t SolarizedLightTheme) GradientStart() lipgloss.Color       { return lipgloss.Color(t.Palette().GradientStart) }
func (t SolarizedLightTheme) GradientEnd() lipgloss.Color         { return lipgloss.Color(t.Palette().GradientEnd) }
func (t SolarizedLightTheme) Palette() ThemePalette {
	return ThemePalette{
		Primary:        lipgloss.Color("#268BD2"),
		Secondary:      lipgloss.Color("#2AA198"),
		Accent:         lipgloss.Color("#D33682"),
		Border:         lipgloss.Color("#93A1A1"),
		Selected:       lipgloss.Color("#EEE8D5"),
		Muted:          lipgloss.Color("#657B83"),
		Text:           lipgloss.Color("#073642"),
		TextMuted:      lipgloss.Color("#586E75"),
		TextDim:        lipgloss.Color("#93A1A1"),
		TextOnAccent:   lipgloss.Color("#FDF6E3"),
		Gold:           lipgloss.Color("#B58900"),
		Focus:          lipgloss.Color("#268BD2"),
		Dim:            lipgloss.Color("#FDF6E3"),
		Success:        lipgloss.Color("#859900"),
		Warning:        lipgloss.Color("#CB4B16"),
		Danger:         lipgloss.Color("#DC322F"),
		ProgressFilled: lipgloss.Color("#268BD2"),
		ProgressEmpty:  lipgloss.Color("#EEE8D5"),
		GradientStart:  "#268BD2",
		GradientEnd:    "#2AA198",
	}
}

// --- Paper Light Theme (Warm Newsprint / Reader) ---
type PaperLightTheme struct{}

func (t PaperLightTheme) ID() string                          { return "paper-light" }
func (t PaperLightTheme) Name() string                        { return "Paper Light" }
func (t PaperLightTheme) Description() string                 { return "Warm ivory parchment background with dark ink text" }
func (t PaperLightTheme) GradientStart() lipgloss.Color       { return lipgloss.Color(t.Palette().GradientStart) }
func (t PaperLightTheme) GradientEnd() lipgloss.Color         { return lipgloss.Color(t.Palette().GradientEnd) }
func (t PaperLightTheme) Palette() ThemePalette {
	return ThemePalette{
		Primary:        lipgloss.Color("#1A5FB4"),
		Secondary:      lipgloss.Color("#26A269"),
		Accent:         lipgloss.Color("#C64600"),
		Border:         lipgloss.Color("#C0BFBC"),
		Selected:       lipgloss.Color("#E1DEDB"),
		Muted:          lipgloss.Color("#77767B"),
		Text:           lipgloss.Color("#1E1E1E"),
		TextMuted:      lipgloss.Color("#5E5C64"),
		TextDim:        lipgloss.Color("#9A9996"),
		TextOnAccent:   lipgloss.Color("#FFFFFF"),
		Gold:           lipgloss.Color("#D88900"),
		Focus:          lipgloss.Color("#1A5FB4"),
		Dim:            lipgloss.Color("#F6F5F4"),
		Success:        lipgloss.Color("#26A269"),
		Warning:        lipgloss.Color("#E5A50A"),
		Danger:         lipgloss.Color("#C01C28"),
		ProgressFilled: lipgloss.Color("#1A5FB4"),
		ProgressEmpty:  lipgloss.Color("#E1DEDB"),
		GradientStart:  "#1A5FB4",
		GradientEnd:    "#C64600",
	}
}

// --- High Contrast Theme (WCAG AAA Accessible) ---
type HighContrastTheme struct{}

func (t HighContrastTheme) ID() string                          { return "high-contrast" }
func (t HighContrastTheme) Name() string                        { return "High Contrast" }
func (t HighContrastTheme) Description() string                 { return "WCAG AAA accessible stark high-contrast palette" }
func (t HighContrastTheme) GradientStart() lipgloss.Color       { return lipgloss.Color(t.Palette().GradientStart) }
func (t HighContrastTheme) GradientEnd() lipgloss.Color         { return lipgloss.Color(t.Palette().GradientEnd) }
func (t HighContrastTheme) Palette() ThemePalette {
	return ThemePalette{
		Primary:        lipgloss.Color("#00FFFF"),
		Secondary:      lipgloss.Color("#00FF00"),
		Accent:         lipgloss.Color("#FFFF00"),
		Border:         lipgloss.Color("#FFFFFF"),
		Selected:       lipgloss.Color("#112233"),
		Muted:          lipgloss.Color("#CCCCCC"),
		Text:           lipgloss.Color("#FFFFFF"),
		TextMuted:      lipgloss.Color("#EEEEEE"),
		TextDim:        lipgloss.Color("#AAAAAA"),
		TextOnAccent:   lipgloss.Color("#000000"),
		Gold:           lipgloss.Color("#FFFF00"),
		Focus:          lipgloss.Color("#00FFFF"),
		Dim:            lipgloss.Color("#000000"),
		Success:        lipgloss.Color("#00FF00"),
		Warning:        lipgloss.Color("#FFFF00"),
		Danger:         lipgloss.Color("#FF3333"),
		ProgressFilled: lipgloss.Color("#00FFFF"),
		ProgressEmpty:  lipgloss.Color("#333333"),
		GradientStart:  "#00FFFF",
		GradientEnd:    "#FFFF00",
	}
}

var registeredThemes = []Theme{
	ForestTheme{},
	DraculaTheme{},
	NordTheme{},
	SunsetTheme{},
	CyberpunkTheme{},
	MonochromeTheme{},
	SolarizedLightTheme{},
	PaperLightTheme{},
	HighContrastTheme{},
	BaseTheme{},
}

// GetRegisteredThemes returns all available theme implementations.
func GetRegisteredThemes() []Theme {
	return registeredThemes
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
	case "solarized-light", "solarizedlight", "solarized_light", "solarized":
		return SolarizedLightTheme{}
	case "paper-light", "paperlight", "paper_light", "paper":
		return PaperLightTheme{}
	case "high-contrast", "highcontrast", "high_contrast", "accessibility", "wcag":
		return HighContrastTheme{}
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
	ColorTextDim        lipgloss.TerminalColor
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
	StyleMuted            lipgloss.Style

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
	ColorTextDim = p.TextDim
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

	StyleMuted = lipgloss.NewStyle().
		Foreground(ColorMuted)

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

func hexToRGB(hex string) (uint8, uint8, uint8) {
	hex = strings.TrimSpace(strings.TrimPrefix(hex, "#"))
	if len(hex) == 3 {
		r, errR := strconv.ParseUint(string(hex[0])+string(hex[0]), 16, 8)
		g, errG := strconv.ParseUint(string(hex[1])+string(hex[1]), 16, 8)
		b, errB := strconv.ParseUint(string(hex[2])+string(hex[2]), 16, 8)
		if errR == nil && errG == nil && errB == nil {
			return uint8(r), uint8(g), uint8(b)
		}
	} else if len(hex) == 6 {
		r, errR := strconv.ParseUint(hex[0:2], 16, 8)
		g, errG := strconv.ParseUint(hex[2:4], 16, 8)
		b, errB := strconv.ParseUint(hex[4:6], 16, 8)
		if errR == nil && errG == nil && errB == nil {
			return uint8(r), uint8(g), uint8(b)
		}
	}
	return 0, 0, 0
}

func parseHexColor(hex string) (r, g, b int) {
	uR, uG, uB := hexToRGB(hex)
	return int(uR), int(uG), int(uB)
}

func interpolateColor(c1, c2 [3]uint8, factor float64) string {
	if factor < 0 {
		factor = 0
	}
	if factor > 1 {
		factor = 1
	}
	r := uint8(math.Round(float64(c1[0]) + float64(int(c2[0])-int(c1[0]))*factor))
	g := uint8(math.Round(float64(c1[1]) + float64(int(c2[1])-int(c1[1]))*factor))
	b := uint8(math.Round(float64(c1[2]) + float64(int(c2[2])-int(c1[2]))*factor))
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}

// RenderGradientText computes a smooth linear gradient across individual characters.
func RenderGradientText(text string, startHex, endHex string) string {
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return ""
	}
	if n == 1 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(startHex)).Bold(true).Render(text)
	}

	r1, g1, b1 := hexToRGB(startHex)
	r2, g2, b2 := hexToRGB(endHex)
	c1 := [3]uint8{r1, g1, b1}
	c2 := [3]uint8{r2, g2, b2}

	var sb strings.Builder
	for i, char := range runes {
		factor := float64(i) / float64(n-1)
		cHex := interpolateColor(c1, c2, factor)
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(cHex)).Bold(true).Render(string(char)))
	}
	return sb.String()
}

// RenderGradient is an alias for RenderGradientText for backwards compatibility.
func RenderGradient(text string, startHex, endHex string) string {
	return RenderGradientText(text, startHex, endHex)
}

// RenderGradientBar computes a multi-stop TrueColor gradient progress bar.
func RenderGradientBar(pct float64, width int, startHex, endHex string) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	if width <= 0 {
		return ""
	}

	filledCount := int(math.Round(pct / 100.0 * float64(width)))
	if filledCount < 0 {
		filledCount = 0
	}
	if filledCount > width {
		filledCount = width
	}
	emptyCount := width - filledCount

	r1, g1, b1 := hexToRGB(startHex)
	r2, g2, b2 := hexToRGB(endHex)
	c1 := [3]uint8{r1, g1, b1}
	c2 := [3]uint8{r2, g2, b2}

	var sb strings.Builder
	for i := 0; i < filledCount; i++ {
		factor := 0.0
		if width > 1 {
			factor = float64(i) / float64(width-1)
		}
		colorHex := interpolateColor(c1, c2, factor)
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorHex)).Render("█"))
	}
	if emptyCount > 0 {
		emptyColor := ColorProgressEmpty
		if emptyColor == nil {
			emptyColor = lipgloss.Color("#2E2E2E")
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(emptyColor).Render(strings.Repeat("░", emptyCount)))
	}
	return sb.String()
}

// SurfaceCardWithHeight renders a standardized Bento card surface with explicit width and height, centering content vertically when height > 0.
func SurfaceCardWithHeight(title string, content string, width int, height int, active bool, badge string) string {
	borderColor := ColorBorder
	if borderColor == nil {
		borderColor = lipgloss.Color("#3C3C3C")
	}
	titleColor := ColorText
	if titleColor == nil {
		titleColor = lipgloss.Color("#FAFAFA")
	}
	if active {
		if ColorFocus != nil {
			borderColor = ColorFocus
		} else if ColorPrimary != nil {
			borderColor = ColorPrimary
		}
		if ColorPrimary != nil {
			titleColor = ColorPrimary
		}
	}

	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)

	var innerWidth int
	if width > 4 {
		cardStyle = cardStyle.Width(width - 2)
		innerWidth = width - 4
	}

	var headerLine string
	if title != "" || badge != "" {
		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(titleColor)
		badgeStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
		if ColorAccent == nil {
			badgeStyle = badgeStyle.Foreground(lipgloss.Color("#FF5F87"))
		}

		if title != "" && badge != "" {
			if innerWidth > 0 {
				titleW := lipgloss.Width(title)
				badgeW := lipgloss.Width(badge)
				spaces := innerWidth - titleW - badgeW
				if spaces < 1 {
					spaces = 1
				}
				headerLine = titleStyle.Render(title) + strings.Repeat(" ", spaces) + badgeStyle.Render(badge)
			} else {
				headerLine = titleStyle.Render(title) + " " + badgeStyle.Render(badge)
			}
		} else if title != "" {
			headerLine = titleStyle.Render(title)
		} else {
			headerLine = badgeStyle.Render(badge)
		}
	}

	if height > 2 {
		innerHeight := height - 2
		cardStyle = cardStyle.Height(innerHeight)
	}

	var cardBody string
	if headerLine != "" && content != "" {
		cardBody = headerLine + "\n" + content
	} else if headerLine != "" {
		cardBody = headerLine
	} else {
		cardBody = content
	}

	return cardStyle.Render(cardBody)
}

// SurfaceCard renders a standardized Bento card surface with auto height.
func SurfaceCard(title string, content string, width int, active bool, badge string) string {
	return SurfaceCardWithHeight(title, content, width, 0, active, badge)
}

// GlobalTabHeader renders the standardized top navigation bar across all screens.
func GlobalTabHeader(activeScreen ScreenMode, width int, runningCount int, vramGauge string) string {
	return GlobalTabHeaderWithRegistry(activeScreen, width, runningCount, vramGauge, nil, nil)
}

// GlobalTabHeaderWithRegistry renders the standardized top navigation bar and registers clickable tab regions.
func GlobalTabHeaderWithRegistry(activeScreen ScreenMode, width int, runningCount int, vramGauge string, onTabClick func(ScreenMode) tea.Cmd, reg *mouse.Registry) string {
	var activeIndex int = -1
	switch activeScreen {
	case ScreenBrowser:
		activeIndex = 0
	case ScreenDashboard, ScreenProfileCreator:
		activeIndex = 1
	case ScreenServerMonitor, ScreenLogStreamer:
		activeIndex = 2
	case ScreenDownloader:
		activeIndex = 3
	case ScreenPerformanceDashboard, ScreenBenchmarkProgress:
		activeIndex = 4
	case ScreenSettings:
		activeIndex = 5
	}

	tab3Label := "[3] Monitor"
	if runningCount > 0 {
		tab3Label = fmt.Sprintf("[3] Monitor (%d Active)", runningCount)
	}

	tabs := []struct {
		index  int
		screen ScreenMode
		label  string
		id     string
	}{
		{0, ScreenBrowser, "[1] Models", "tab-browser"},
		{1, ScreenDashboard, "[2] Launch", "tab-dashboard"},
		{2, ScreenServerMonitor, tab3Label, "tab-monitor"},
		{3, ScreenDownloader, "[4] Downloads", "tab-downloader"},
		{4, ScreenPerformanceDashboard, "[5] Bench", "tab-benchmark"},
		{5, ScreenSettings, "[6] Settings", "tab-settings"},
	}

	activeBg := ColorSelected
	if activeBg == nil {
		activeBg = lipgloss.Color("#2E2E3E")
	}
	activeFg := ColorSecondary
	if activeFg == nil {
		activeFg = lipgloss.Color("#04B575")
	}
	textMuted := ColorTextMuted
	if textMuted == nil {
		textMuted = lipgloss.Color("#D0D0D0")
	}
	borderColor := ColorBorder
	if borderColor == nil {
		borderColor = lipgloss.Color("#3C3C3C")
	}

	var renderedTabs []string
	for _, tab := range tabs {
		if tab.index == activeIndex {
			tabStyle := lipgloss.NewStyle().
				Background(activeBg).
				Foreground(activeFg).
				Bold(true).
				Padding(0, 1)
			renderedTabs = append(renderedTabs, tabStyle.Render("● "+tab.label))
		} else {
			tabStyle := lipgloss.NewStyle().
				Foreground(textMuted).
				Padding(0, 1)
			renderedTabs = append(renderedTabs, tabStyle.Render(tab.label))
		}
	}

	// Register clickable tab regions if registry is provided
	if reg != nil && onTabClick != nil {
		curX := 2
		for i, tab := range tabs {
			rendered := renderedTabs[i]
			tabW := lipgloss.Width(rendered)
			targetScreen := tab.screen
			reg.Register(mouse.Region{
				ID:     tab.id,
				Bounds: mouse.Rect{X: curX, Y: 2, W: tabW, H: 1},
				ZIndex: 0,
				OnClick: func(msg tea.MouseMsg) tea.Cmd {
					return onTabClick(targetScreen)
				},
			})
			curX += tabW + 1 // +1 for sep "│"
		}
	}

	sep := lipgloss.NewStyle().Foreground(borderColor).Render("│")
	tabsRow := strings.Join(renderedTabs, sep)

	var vramStr string
	if vramGauge != "" {
		vramColor := ColorTextDim
		if vramColor == nil {
			vramColor = lipgloss.Color("#808080")
		}
		vramStr = lipgloss.NewStyle().Foreground(vramColor).Render(vramGauge)
	}

	brand := "RUNORA // RUNTIME MANAGER"
	if ThemeGradientStart != "" && ThemeGradientEnd != "" {
		brand = RenderGradientText(brand, ThemeGradientStart, ThemeGradientEnd)
	} else {
		primary := ColorPrimary
		if primary == nil {
			primary = lipgloss.Color("#7D56F4")
		}
		brand = lipgloss.NewStyle().Bold(true).Foreground(primary).Render(brand)
	}

	var innerWidth int
	headerBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)

	if width > 4 {
		headerBox = headerBox.Width(width - 2)
		innerWidth = width - 4
	}

	if vramStr != "" && innerWidth > 0 {
		tabsW := lipgloss.Width(tabsRow)
		vramW := lipgloss.Width(vramStr)
		spaces := innerWidth - tabsW - vramW
		if spaces > 0 {
			tabsRow = tabsRow + strings.Repeat(" ", spaces) + vramStr
		} else {
			tabsRow = tabsRow + "  " + vramStr
		}
	} else if vramStr != "" {
		tabsRow = tabsRow + "  " + vramStr
	}

	body := brand + "\n" + tabsRow
	return headerBox.Render(body)
}
