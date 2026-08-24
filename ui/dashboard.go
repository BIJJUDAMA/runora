package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/hardware"
	"github.com/BIJJUDAMA/runora/model"
	"github.com/BIJJUDAMA/runora/profile"
	"github.com/BIJJUDAMA/runora/ui/mouse"
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

func (d *DashboardModel) MoveProfile(dx, dy int) {
	if len(d.Profiles) == 0 {
		return
	}
	const colsPerRow = 5
	newIdx := d.ActiveIdx + dx + (dy * colsPerRow)
	if newIdx < 0 {
		newIdx = 0
	}
	if newIdx >= len(d.Profiles) {
		newIdx = len(d.Profiles) - 1
	}
	d.ActiveIdx = newIdx
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
	var parts []string
	parts = append(parts, "llama-server",
		"--model", d.Model.FilePath,
		"--host", p.Host,
		"--port", fmt.Sprintf("%d", p.Port),
		"--ctx-size", fmt.Sprintf("%d", p.Context),
		"--threads", fmt.Sprintf("%d", p.Threads),
		"--n-gpu-layers", fmt.Sprintf("%d", p.GPULayers),
		"--batch-size", fmt.Sprintf("%d", p.BatchSize),
	)
	if p.FlashAttention {
		parts = append(parts, "--flash-attn", "on")
	}
	if p.CacheTypeK != "" {
		parts = append(parts, "--cache-type-k", p.CacheTypeK)
	}
	if p.CacheTypeV != "" {
		parts = append(parts, "--cache-type-v", p.CacheTypeV)
	}
	if p.CustomArgs != "" {
		parts = append(parts, p.CustomArgs)
	}
	return strings.Join(parts, " ")
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
	return d.ViewWithRegistry(width, height, nil, nil, nil)
}

func (d *DashboardModel) ViewWithRegistry(width int, height int, reg *mouse.Registry, onLaunch func() tea.Cmd, onCopy func() tea.Cmd) string {
	d.Width = width
	d.Height = height

	if len(d.Profiles) == 0 {
		return "No profiles found. Press [P] to create a profile."
	}

	cardWidth := width
	if cardWidth < 60 {
		cardWidth = 60
	}

	leftColWidth := cardWidth / 2
	rightColWidth := cardWidth - leftColWidth
	cardHeight := height
	if cardHeight < 12 {
		cardHeight = 12
	}

	// ── Left Column: Hardware Fit & Memory Estimate ──
	var leftContent strings.Builder
	suitabilityBadge := "[SYSTEM RAM]"

	modelName := "No Model Selected"
	if d.Model != nil {
		modelName = d.Model.Name
	}
	modelNameTrunc := TruncateVisual(modelName, max(24, leftColWidth-18), "...")
	leftContent.WriteString(fmt.Sprintf("  Model: %s\n\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(modelNameTrunc)))

	// Toast notification banner if present and not expired
	if d.ToastMessage != "" && (d.ToastExpiry.IsZero() || time.Now().Before(d.ToastExpiry)) {
		toastStyle := lipgloss.NewStyle().
			Background(ColorPrimary).
			Foreground(ColorTextOnAccent).
			Bold(true).
			Padding(0, 2)
		leftContent.WriteString(fmt.Sprintf("  %s\n\n", toastStyle.Render(d.ToastMessage)))
	}

	p := d.Profiles[d.ActiveIdx]

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

	leftCard := SurfaceCardWithHeight("Hardware Fit & Memory Estimate", leftContent.String(), leftColWidth, cardHeight, true, suitabilityBadge)

	// ── Right Column: Top Card - Execution Profile ──
	var profileContent strings.Builder

	// 5-per-row grid layout (max 5 rows = 25 profiles)
	profileContent.WriteString("  Profiles (5 per row):\n")
	const colsPerRow = 5
	const maxRows = 5
	totalProfiles := len(d.Profiles)
	if totalProfiles > colsPerRow*maxRows {
		totalProfiles = colsPerRow * maxRows
	}

	numRows := (totalProfiles + colsPerRow - 1) / colsPerRow
	if numRows > maxRows {
		numRows = maxRows
	}

	for row := 0; row < numRows; row++ {
		var rowItems []string
		curX := leftColWidth + 3
		for col := 0; col < colsPerRow; col++ {
			idx := row*colsPerRow + col
			if idx >= totalProfiles {
				break
			}
			prof := d.Profiles[idx]
			isDefault := profile.IsDefaultProfile(prof.Name)
			profLabel := prof.Name
			if !isDefault {
				profLabel = prof.Name + "*"
			}
			var renderedItem string
			if idx == d.ActiveIdx {
				renderedItem = lipgloss.NewStyle().
					Background(ColorPrimary).
					Foreground(ColorTextOnAccent).
					Bold(true).
					Padding(0, 1).
					Render(profLabel)
			} else {
				renderedItem = lipgloss.NewStyle().
					Foreground(ColorMuted).
					Padding(0, 1).
					Render(profLabel)
			}
			rowItems = append(rowItems, renderedItem)

			if reg != nil {
				profW := lipgloss.Width(renderedItem)
				targetIdx := idx
				reg.Register(mouse.Region{
					ID:     fmt.Sprintf("profile-tile-%d", targetIdx),
					Bounds: mouse.Rect{X: curX, Y: 5 + row, W: profW, H: 1},
					ZIndex: 1,
					OnClick: func(msg tea.MouseMsg) tea.Cmd {
						d.ActiveIdx = targetIdx
						return nil
					},
					OnDblClick: func(msg tea.MouseMsg) tea.Cmd {
						d.ActiveIdx = targetIdx
						if onLaunch != nil {
							return onLaunch()
						}
						return nil
					},
				})
				curX += profW + 1
			}
		}
		profileContent.WriteString("  " + strings.Join(rowItems, " ") + "\n")
	}
	profileContent.WriteString("\n")

	profileContent.WriteString(fmt.Sprintf("  %-18s %d tokens\n", "Context Size:", p.Context))
	profileContent.WriteString(fmt.Sprintf("  %-18s %d threads\n", "Thread Count:", p.Threads))
	profileContent.WriteString(fmt.Sprintf("  %-18s %d layers\n", "GPU Layers:", p.GPULayers))

	faStatus := "Enabled (--flash-attn on)"
	if !p.FlashAttention {
		faStatus = "Disabled"
	}
	profileContent.WriteString(fmt.Sprintf("  %-18s %s\n", "Flash Attention:", faStatus))

	kvStatus := "FP16"
	if p.CacheTypeK != "" || p.CacheTypeV != "" {
		kType := p.CacheTypeK
		if kType == "" {
			kType = "f16"
		}
		vType := p.CacheTypeV
		if vType == "" {
			vType = "f16"
		}
		kvStatus = fmt.Sprintf("K: %s, V: %s", kType, vType)
	}
	profileContent.WriteString(fmt.Sprintf("  %-18s %s\n", "KV Quantization:", kvStatus))

	if p.CustomArgs != "" {
		profileContent.WriteString(fmt.Sprintf("  %-18s %s\n", "Custom Flags:", p.CustomArgs))
	}
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

	if reg != nil && onCopy != nil {
		reg.Register(mouse.Region{
			ID:     "dashboard-btn-copy",
			Bounds: mouse.Rect{X: leftColWidth + 3, Y: 3 + rightTopHeight + 3, W: 24, H: 1},
			ZIndex: 1,
			OnClick: func(msg tea.MouseMsg) tea.Cmd {
				return onCopy()
			},
		})
	}

	botRightCard := SurfaceCardWithHeight("Launch Command Preview", previewContent.String(), rightColWidth, rightBotHeight, false, "CLI")
	rightCol := lipgloss.JoinVertical(lipgloss.Left, topRightCard, botRightCard)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftCard, rightCol)
}
