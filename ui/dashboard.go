package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/hardware"
	"github.com/BIJJUDAMA/runora/model"
	"github.com/BIJJUDAMA/runora/profile"
)

type DashboardModel struct {
	Model         *model.GGUFMetadata
	Specs         *hardware.HardwareSpecs
	Profiles      []*profile.Profile
	ActiveIdx     int
	Width, Height int
	ToastMessage  string
	ToastExpiry   time.Time
}

func NewDashboardModel(m *model.GGUFMetadata, specs *hardware.HardwareSpecs, profiles []*profile.Profile, activeProfile string) *DashboardModel {
	activeIdx := 0
	for i, p := range profiles {
		if p.Name == activeProfile {
			activeIdx = i
			break
		}
	}

	return &DashboardModel{
		Model:     m,
		Specs:     specs,
		Profiles:  profiles,
		ActiveIdx: activeIdx,
	}
}

func (d *DashboardModel) ActiveProfile() *profile.Profile {
	if len(d.Profiles) == 0 {
		return nil
	}
	if d.ActiveIdx >= len(d.Profiles) {
		d.ActiveIdx = len(d.Profiles) - 1
	}
	if d.ActiveIdx < 0 {
		d.ActiveIdx = 0
	}
	return d.Profiles[d.ActiveIdx]
}

func (d *DashboardModel) CycleProfile(direction int) {
	if len(d.Profiles) == 0 {
		return
	}
	d.ActiveIdx = (d.ActiveIdx + direction + len(d.Profiles)) % len(d.Profiles)
}

func (d *DashboardModel) SetToast(msg string) {
	d.ToastMessage = msg
	d.ToastExpiry = time.Now().Add(4 * time.Second)
}

func (d *DashboardModel) GetLaunchCommand() string {
	p := d.ActiveProfile()
	if p == nil || d.Model == nil {
		return ""
	}
	return fmt.Sprintf("llama-server --model %s --host %s --port %d --ctx-size %d --threads %d --n-gpu-layers %d --batch-size %d",
		d.Model.FilePath, p.Host, p.Port, p.Context, p.Threads, p.GPULayers, p.BatchSize)
}

func (d *DashboardModel) CopyCommandToClipboard() error {
	cmdStr := d.GetLaunchCommand()
	if cmdStr == "" {
		d.SetToast("No launch command available to copy")
		return fmt.Errorf("no command available")
	}
	err := clipboard.WriteAll(cmdStr)
	if err == nil {
		d.SetToast("✓ Command copied to clipboard!")
	} else {
		d.SetToast(fmt.Sprintf("Failed to copy to clipboard: %v", err))
	}
	return err
}

func (d *DashboardModel) View(width int, height int) string {
	p := d.ActiveProfile()
	if p == nil {
		return "No profiles found."
	}

	boxWidth := width - 4
	if boxWidth < 70 {
		boxWidth = 70
	}

	gridWidth := boxWidth - 4
	if gridWidth < 60 {
		gridWidth = 60
	}

	leftColWidth := gridWidth / 2
	rightColWidth := gridWidth - leftColWidth

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("LAUNCH DASHBOARD")))
	modelName := "No Model Selected"
	if d.Model != nil {
		modelName = d.Model.Name
	}
	modelName = TruncateVisual(modelName, max(30, width-14), "...")
	sb.WriteString(fmt.Sprintf("  Model: %s\n\n", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(modelName)))

	// Toast notification banner if present and not expired
	if d.ToastMessage != "" && (d.ToastExpiry.IsZero() || time.Now().Before(d.ToastExpiry)) {
		toastStyle := lipgloss.NewStyle().
			Background(ColorPrimary).
			Foreground(ColorTextOnAccent).
			Bold(true).
			Padding(0, 2)
		sb.WriteString(fmt.Sprintf("  %s\n\n", toastStyle.Render(d.ToastMessage)))
	}

	// ── Left Column: Hardware Fit & Memory Estimate ──
	var leftContent strings.Builder
	suitabilityBadge := "[SYSTEM RAM]"

	if d.Model != nil {
		weightStr := formatSize(d.Model.FileSize)
		if d.Specs != nil {
			est := hardware.EstimateMemory(d.Model, d.Specs, p.Context)

			switch est.Suitability {
			case hardware.SuitabilityFitsVRAM:
				suitabilityBadge = "[FITS VRAM]"
			case hardware.SuitabilityPartialVRAM:
				suitabilityBadge = "[PARTIAL VRAM]"
			case hardware.SuitabilityFitsRAM:
				suitabilityBadge = "[FITS RAM]"
			case hardware.SuitabilityExceeds:
				suitabilityBadge = "[EXCEEDS]"
			}

			var suitStr string
			switch est.Suitability {
			case hardware.SuitabilityFitsVRAM:
				suitStr = StyleSuccess.Render("Fits GPU VRAM (Full Offload)")
			case hardware.SuitabilityPartialVRAM:
				suitStr = StyleWarning.Render(fmt.Sprintf("Partial VRAM Offload (%d%%)", est.GPUOffloadPct))
			case hardware.SuitabilityFitsRAM:
				suitStr = lipgloss.NewStyle().Foreground(ColorSecondary).Render("Fits System RAM (CPU-only)")
			case hardware.SuitabilityExceeds:
				suitStr = StyleDanger.Render("Exceeds Memory Limits")
			}

			leftContent.WriteString(fmt.Sprintf("  %-20s %s\n", "Model Weight:", StyleTitle.Copy().Padding(0).Render(weightStr)))
			leftContent.WriteString(fmt.Sprintf("  %-20s %s (%.2fx multiplier)\n", "Quantized KV Cache:", formatSize(int64(est.KVCacheSize)), est.KVCacheMultiplier))
			leftContent.WriteString(fmt.Sprintf("  %-20s %s\n", "Activation Graph:", formatSize(int64(est.ActivationSize))))
			leftContent.WriteString(fmt.Sprintf("  %-20s %s (GPU offload: %d%%)\n", "Total Required:", formatSize(int64(est.TotalMemory)), est.GPUOffloadPct))
			leftContent.WriteString(fmt.Sprintf("  %-20s %s\n", "Fit Suitability:", suitStr))
			if est.Reason != "" {
				leftContent.WriteString(fmt.Sprintf("  %-20s %s\n\n", "Recommendation:", lipgloss.NewStyle().Foreground(ColorTextDim).Render(est.Reason)))
			} else {
				leftContent.WriteString("\n")
			}

			// Gradient Memory Bar
			barWidth := leftColWidth - 8
			if barWidth < 10 {
				barWidth = 10
			}
			if barWidth > 28 {
				barWidth = 28
			}
			startHex := ThemeGradientStart
			if startHex == "" {
				startHex = "#7D56F4"
			}
			endHex := ThemeGradientEnd
			if endHex == "" {
				endHex = "#FF5F87"
			}
			gradBar := RenderGradientBar(float64(est.GPUOffloadPct), barWidth, startHex, endHex)
			leftContent.WriteString(fmt.Sprintf("  Memory Allocation:\n  [%s] %d%% GPU Offload\n", gradBar, est.GPUOffloadPct))
		} else {
			leftContent.WriteString(fmt.Sprintf("  %-20s %s\n", "Model Weight:", weightStr))
			leftContent.WriteString(fmt.Sprintf("  %-20s %s\n", "Quantized KV Cache:", "1.00x multiplier"))
			leftContent.WriteString(fmt.Sprintf("  %-20s %s\n", "Activation Graph:", "512 MB (Estimated)"))
			leftContent.WriteString(fmt.Sprintf("  %-20s %s\n", "Total Required:", weightStr))
			leftContent.WriteString(fmt.Sprintf("  %-20s %s\n\n", "Fit Suitability:", "Hardware specs unavailable"))

			barWidth := leftColWidth - 8
			if barWidth < 10 {
				barWidth = 10
			}
			if barWidth > 28 {
				barWidth = 28
			}
			gradBar := RenderGradientBar(0, barWidth, ThemeGradientStart, ThemeGradientEnd)
			leftContent.WriteString(fmt.Sprintf("  Memory Allocation:\n  [%s] 0%% GPU Offload\n", gradBar))
		}
	} else {
		leftContent.WriteString("  No model selected.\n")
	}

	cardHeight := max(16, height-3)
	leftCard := SurfaceCardWithHeight("Hardware Fit & Memory Estimate", leftContent.String(), leftColWidth, cardHeight, true, suitabilityBadge)

	// ── Right Column: Top Card - Execution Profile ──
	var profileContent strings.Builder

	// Profile Carousel line with [<] / [>]
	var profItems []string
	for i, prof := range d.Profiles {
		isDefault := profile.IsDefaultProfile(prof.Name)
		profLabel := prof.Name
		if !isDefault {
			profLabel = prof.Name + "*"
		}
		if i == d.ActiveIdx {
			profItems = append(profItems, lipgloss.NewStyle().
				Background(ColorPrimary).
				Foreground(ColorTextOnAccent).
				Bold(true).
				Padding(0, 1).
				Render(profLabel))
		} else {
			profItems = append(profItems, lipgloss.NewStyle().
				Foreground(ColorMuted).
				Padding(0, 1).
				Render(profLabel))
		}
	}
	carouselLine := fmt.Sprintf("  Profiles: %s %s %s\n\n",
		StyleHelpKey.Render("[<]"),
		strings.Join(profItems, " "),
		StyleHelpKey.Render("[>]"),
	)
	profileContent.WriteString(carouselLine)

	profileContent.WriteString(fmt.Sprintf("  %-18s %d tokens\n", "Context Size:", p.Context))
	profileContent.WriteString(fmt.Sprintf("  %-18s %d threads\n", "Thread Count:", p.Threads))
	profileContent.WriteString(fmt.Sprintf("  %-18s %d layers\n", "GPU Layers:", p.GPULayers))
	profileContent.WriteString(fmt.Sprintf("  %-18s %s\n", "Flash Attention:", "Enabled (Auto)"))
	profileContent.WriteString(fmt.Sprintf("  %-18s %s\n", "KV Quantization:", "FP16 (Q8_0/Q4_0/FP8 supported)"))
	profileContent.WriteString(fmt.Sprintf("  %-18s %s:%d\n", "Host / Port:", p.Host, p.Port))

	activeProfileBadge := p.Name
	if !profile.IsDefaultProfile(p.Name) {
		activeProfileBadge = p.Name + "*"
	}
	rightTopHeight := (cardHeight * 6) / 10
	rightBotHeight := cardHeight - rightTopHeight
	topRightCard := SurfaceCardWithHeight("Execution Profile", profileContent.String(), rightColWidth, rightTopHeight, false, activeProfileBadge)

	// ── Right Column: Bottom Card - Launch Command Preview ──
	var previewContent strings.Builder
	cmdPreview := d.GetLaunchCommand()
	cmdWidth := rightColWidth - 6
	if cmdWidth < 20 {
		cmdWidth = 20
	}
	wrappedCmd := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Italic(true).
		Width(cmdWidth).
		Render(cmdPreview)

	previewContent.WriteString(wrappedCmd)
	previewContent.WriteString("\n\n")
	previewContent.WriteString(fmt.Sprintf("  %s Copy to clipboard", StyleHelpKey.Render("[C]")))

	botRightCard := SurfaceCardWithHeight("Launch Command Preview", previewContent.String(), rightColWidth, rightBotHeight, false, "CLI")
	rightCol := lipgloss.JoinVertical(lipgloss.Left, topRightCard, botRightCard)

	bentoGrid := lipgloss.JoinHorizontal(lipgloss.Top, leftCard, rightCol)
	sb.WriteString(bentoGrid)
	sb.WriteString("\n\n")

	// Help prompts
	helpStr := fmt.Sprintf("  %s Launch  %s Cycle  %s New  %s Edit  %s Dupl  %s Del  %s Copy Cmd  %s Back",
		StyleHelpKey.Render("[Enter]"),
		StyleHelpKey.Render("[←/→]"),
		StyleHelpKey.Render("[P]"),
		StyleHelpKey.Render("[E]"),
		StyleHelpKey.Render("[N]"),
		StyleHelpKey.Render("[D]"),
		StyleHelpKey.Render("[C]"),
		StyleHelpKey.Render("[Esc]"),
	)
	sb.WriteString(helpStr + "\n")

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorPrimary).
		Width(boxWidth).
		Render(sb.String())
}
