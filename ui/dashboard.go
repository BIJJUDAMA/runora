package ui

import (
	"fmt"
	"strings"

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
	return d.Profiles[d.ActiveIdx]
}

func (d *DashboardModel) CycleProfile(direction int) {
	if len(d.Profiles) == 0 {
		return
	}
	d.ActiveIdx = (d.ActiveIdx + direction + len(d.Profiles)) % len(d.Profiles)
}

func (d *DashboardModel) View(width int, height int) string {
	p := d.ActiveProfile()
	if p == nil {
		return "No profiles found."
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("LAUNCH DASHBOARD")))
	modelName := TruncateVisual(d.Model.Name, max(30, width-14), "...")
	sb.WriteString(fmt.Sprintf("  Model: %s\n\n", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(modelName)))

	// Profiles selector row
	var profsRow []string
	for i, prof := range d.Profiles {
		if i == d.ActiveIdx {
			profsRow = append(profsRow, lipgloss.NewStyle().
				Background(ColorPrimary).
				Foreground(ColorTextOnAccent).
				Bold(true).
				Padding(0, 1).
				Render(prof.Name))
		} else {
			profsRow = append(profsRow, lipgloss.NewStyle().
				Foreground(ColorMuted).
				Padding(0, 1).
				Render(prof.Name))
		}
	}
	sb.WriteString("  Profile:  " + strings.Join(profsRow, "  ") + "\n\n")

	// Details
	sb.WriteString(fmt.Sprintf("  %-16s %d tokens\n", "Context Size:", p.Context))
	sb.WriteString(fmt.Sprintf("  %-16s %d threads\n", "CPU Threads:", p.Threads))
	sb.WriteString(fmt.Sprintf("  %-16s %d\n", "GPU Layers:", p.GPULayers))
	sb.WriteString(fmt.Sprintf("  %-16s %d\n", "Batch Size:", p.BatchSize))
	sb.WriteString(fmt.Sprintf("  %-16s %s:%d\n\n", "Host/Port:", p.Host, p.Port))

	// Dynamic Memory Estimates based on the active profile's context
	if d.Specs != nil {
		est := hardware.EstimateMemory(d.Model, d.Specs, p.Context)
		var suitStr string
		switch est.Suitability {
		case hardware.SuitabilityFitsVRAM:
			suitStr = StyleSuccess.Render("Fits GPU VRAM")
		case hardware.SuitabilityPartialVRAM:
			suitStr = StyleWarning.Render("Partial VRAM Offload")
		case hardware.SuitabilityFitsRAM:
			suitStr = lipgloss.NewStyle().Foreground(ColorSecondary).Render("Fits System RAM (CPU)")
		case hardware.SuitabilityExceeds:
			suitStr = StyleDanger.Render("Exceeds Memory Limits")
		}

		sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Bold(true).Render("Dynamic Memory Suitability:")))
		sb.WriteString(fmt.Sprintf("  %-16s %s\n", "Status:", suitStr))
		sb.WriteString(fmt.Sprintf("  %-16s %s\n", "KV Cache Size:", formatSize(int64(est.KVCacheSize))))
		sb.WriteString(fmt.Sprintf("  %-16s %s (GPU offload: %d%%)\n", "Total Memory:", formatSize(int64(est.TotalMemory)), est.GPUOffloadPct))
		sb.WriteString(fmt.Sprintf("  %-16s %s\n\n", "Recommendation:", est.Reason))
	}

	// Command preview
	cmdPreview := fmt.Sprintf("llama-server --model %s --host %s --port %d --ctx-size %d --threads %d --n-gpu-layers %d --batch-size %d",
		d.Model.FilePath, p.Host, p.Port, p.Context, p.Threads, p.GPULayers, p.BatchSize)
	
	cmdWidth := width - 6
	if cmdWidth < 20 {
		cmdWidth = 20
	}

	wrappedCmd := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Italic(true).
		Width(cmdWidth).
		Render(cmdPreview)

	sb.WriteString(fmt.Sprintf("  %s\n  %s\n\n", lipgloss.NewStyle().Bold(true).Render("Launch Command Preview:"), wrappedCmd))

	// Help prompts
	helpStr := fmt.Sprintf("%s Launch  %s Cycle profiles  %s Create profile  %s Cancel",
		StyleHelpKey.Render("[Enter/Y]"),
		StyleHelpKey.Render("[Left/Right]"),
		StyleHelpKey.Render("[P]"),
		StyleHelpKey.Render("[Esc/C]"),
	)
	sb.WriteString("  " + helpStr + "\n")

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
