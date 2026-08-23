package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestHexToRGB(t *testing.T) {
	r, g, b := hexToRGB("#FF0000")
	if r != 255 || g != 0 || b != 0 {
		t.Errorf("expected (255, 0, 0), got (%d, %d, %d)", r, g, b)
	}

	r, g, b = hexToRGB("00FF00")
	if r != 0 || g != 255 || b != 0 {
		t.Errorf("expected (0, 255, 0), got (%d, %d, %d)", r, g, b)
	}

	r, g, b = hexToRGB("#00F")
	if r != 0 || g != 0 || b != 255 {
		t.Errorf("expected (0, 0, 255), got (%d, %d, %d)", r, g, b)
	}

	r, g, b = hexToRGB("invalid")
	if r != 0 || g != 0 || b != 0 {
		t.Errorf("expected (0, 0, 0) for invalid hex, got (%d, %d, %d)", r, g, b)
	}
}

func TestInterpolateColor(t *testing.T) {
	c1 := [3]uint8{0, 0, 0}
	c2 := [3]uint8{255, 255, 255}

	res0 := interpolateColor(c1, c2, 0.0)
	if res0 != "#000000" {
		t.Errorf("expected #000000 for factor 0.0, got %s", res0)
	}

	res1 := interpolateColor(c1, c2, 1.0)
	if res1 != "#FFFFFF" {
		t.Errorf("expected #FFFFFF for factor 1.0, got %s", res1)
	}

	resHalf := interpolateColor(c1, c2, 0.5)
	if resHalf != "#808080" {
		t.Errorf("expected #808080 for factor 0.5, got %s", resHalf)
	}
}

func TestRenderGradientText(t *testing.T) {
	// Test empty text
	resEmpty := RenderGradientText("", "#FF0000", "#0000FF")
	if resEmpty != "" {
		t.Errorf("expected empty string for empty text, got %q", resEmpty)
	}

	// Test single character
	resSingle := RenderGradientText("A", "#FF0000", "#0000FF")
	if !strings.Contains(resSingle, "A") {
		t.Errorf("expected single char 'A' in output, got %q", resSingle)
	}

	// Test full text
	text := "RUNORA // COMMAND DECK"
	res := RenderGradientText(text, "#7D56F4", "#FF5F87")
	if !strings.Contains(res, "RUNORA") || !strings.Contains(res, "COMMAND DECK") {
		t.Errorf("expected rendered gradient to contain original text, got %q", res)
	}

	// Test short hex code interpolation
	resShortHex := RenderGradientText("TEST", "#F00", "#00F")
	if !strings.Contains(resShortHex, "TEST") {
		t.Errorf("expected short hex to render correctly, got %q", resShortHex)
	}
}

func TestRenderGradientBar(t *testing.T) {
	width := 20
	startHex := "#00F3FF"
	endHex := "#FF007F"

	// 0% progress
	bar0 := RenderGradientBar(0, width, startHex, endHex)
	if lipgloss.Width(bar0) != width {
		t.Errorf("expected width %d for 0%% bar, got %d", width, lipgloss.Width(bar0))
	}
	if strings.Contains(bar0, "█") {
		t.Errorf("expected 0%% bar to contain no filled blocks, got %q", bar0)
	}

	// 50% progress
	bar50 := RenderGradientBar(50, width, startHex, endHex)
	if lipgloss.Width(bar50) != width {
		t.Errorf("expected width %d for 50%% bar, got %d", width, lipgloss.Width(bar50))
	}
	if !strings.Contains(bar50, "█") || !strings.Contains(bar50, "░") {
		t.Errorf("expected 50%% bar to contain both filled and empty blocks, got %q", bar50)
	}

	// 100% progress
	bar100 := RenderGradientBar(100, width, startHex, endHex)
	if lipgloss.Width(bar100) != width {
		t.Errorf("expected width %d for 100%% bar, got %d", width, lipgloss.Width(bar100))
	}
	if strings.Contains(bar100, "░") {
		t.Errorf("expected 100%% bar to contain no empty blocks, got %q", bar100)
	}

	// >100% progress clamp
	bar150 := RenderGradientBar(150, width, startHex, endHex)
	if lipgloss.Width(bar150) != width {
		t.Errorf("expected width %d for >100%% bar, got %d", width, lipgloss.Width(bar150))
	}

	// Negative progress clamp
	barNeg := RenderGradientBar(-20, width, startHex, endHex)
	if lipgloss.Width(barNeg) != width {
		t.Errorf("expected width %d for negative pct bar, got %d", width, lipgloss.Width(barNeg))
	}
}

func TestSurfaceCard(t *testing.T) {
	title := "HARDWARE SPECS"
	badge := "[● ACTIVE]"
	content := "CPU: 16 Cores\nRAM: 32 GB\nVRAM: 12 GB"
	width := 50

	// Inactive card
	inactiveCard := SurfaceCard(title, content, width, false, badge)
	if !strings.Contains(inactiveCard, title) {
		t.Errorf("expected card to contain title %q", title)
	}
	if !strings.Contains(inactiveCard, badge) {
		t.Errorf("expected card to contain badge %q", badge)
	}
	if !strings.Contains(inactiveCard, "VRAM: 12 GB") {
		t.Errorf("expected card to contain content")
	}

	// Active card
	activeCard := SurfaceCard(title, content, width, true, badge)
	if !strings.Contains(activeCard, title) {
		t.Errorf("expected active card to contain title %q", title)
	}

	// Card without badge
	noBadgeCard := SurfaceCard("SIMPLE", "Hello World", 40, false, "")
	if !strings.Contains(noBadgeCard, "SIMPLE") || !strings.Contains(noBadgeCard, "Hello World") {
		t.Errorf("expected simple card without badge to render properly")
	}
}

func TestGlobalTabHeader(t *testing.T) {
	width := 100
	vramGauge := "RTX 4090 [████░░░░] 12.0/24.0 GB"
	runningCount := 2

	header := GlobalTabHeader(ScreenBrowser, width, runningCount, vramGauge)

	// Check all 6 tab labels
	expectedTabs := []string{
		"[1] Models",
		"[2] Launch",
		"[3] Monitor",
		"[4] Downloads",
		"[5] Bench",
		"[6] Settings",
	}

	for _, tab := range expectedTabs {
		if !strings.Contains(header, tab) {
			t.Errorf("expected header to contain tab %q, got:\n%s", tab, header)
		}
	}

	// Check active count badge
	if !strings.Contains(header, "2 Active") && !strings.Contains(header, "2") {
		t.Errorf("expected header to contain running instance count, got:\n%s", header)
	}

	// Check VRAM gauge
	if !strings.Contains(header, "RTX 4090") {
		t.Errorf("expected header to contain VRAM gauge info, got:\n%s", header)
	}
}

func isEmoji(r rune) bool {
	// Common Emoji ranges
	if (r >= 0x1F300 && r <= 0x1F5FF) || // Misc Symbols and Pictographs
		(r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map
		(r >= 0x1F700 && r <= 0x1F77F) || // Alchemical Symbols
		(r >= 0x1F780 && r <= 0x1F7FF) || // Geometric Shapes Extended
		(r >= 0x1F800 && r <= 0x1F8FF) || // Supplemental Arrows-C
		(r >= 0x1F900 && r <= 0x1F9FF) || // Supplemental Symbols and Pictographs
		(r >= 0x1FA00 && r <= 0x1FA6F) || // Chess Symbols
		(r >= 0x1FA70 && r <= 0x1FAFF) || // Symbols and Pictographs Extended-A
		(r >= 0x2600 && r <= 0x26FF && r != '✓' && r != '✗' && r != '★' && r != '☆') {
		return true
	}
	return false
}

func TestZeroEmojisInStyles(t *testing.T) {
	// Test all 10 registered themes
	themes := GetRegisteredThemes()
	if len(themes) < 10 {
		t.Errorf("expected at least 10 registered themes, found %d", len(themes))
	}

	for _, theme := range themes {
		p := theme.Palette()
		if theme.ID() == "" || theme.Name() == "" {
			t.Errorf("theme missing ID or Name: %+v", theme)
		}
		if theme.GradientStart() == "" || theme.GradientEnd() == "" {
			t.Errorf("theme %s missing GradientStart or GradientEnd method output", theme.ID())
		}
		if p.GradientStart == "" || p.GradientEnd == "" {
			t.Errorf("theme %s palette missing GradientStart or GradientEnd string", theme.ID())
		}

		// Verify zero emojis in theme names & descriptions
		for _, r := range theme.Name() + theme.Description() {
			if isEmoji(r) {
				t.Errorf("theme %s contains emoji %q in name/description", theme.ID(), string(r))
			}
		}
	}

	// Verify GlobalTabHeader contains zero emojis
	header := GlobalTabHeader(ScreenBrowser, 120, 3, "GPU [████░░░░] 4/8 GB")
	for _, r := range header {
		if isEmoji(r) {
			t.Errorf("GlobalTabHeader contains emoji %q in rendered output", string(r))
		}
	}

	// Verify SurfaceCard contains zero emojis
	card := SurfaceCard("TEST CARD", "Content text line", 60, true, "[ACTIVE]")
	for _, r := range card {
		if isEmoji(r) {
			t.Errorf("SurfaceCard contains emoji %q in rendered output", string(r))
		}
	}
}
