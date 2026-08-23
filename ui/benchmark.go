package ui

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/benchmark"
	"github.com/BIJJUDAMA/runora/hardware"
)

type BenchmarkProgressStep int

const (
	StepBooting BenchmarkProgressStep = iota
	StepRunningPrompt
	StepSavingData
	StepDone
	StepError
)

type BenchmarkProgressModel struct {
	ModelName string
	Step      BenchmarkProgressStep
	Err       error
}

func NewBenchmarkProgressModel(modelName string) *BenchmarkProgressModel {
	return &BenchmarkProgressModel{
		ModelName: modelName,
		Step:      StepBooting,
	}
}

func (b *BenchmarkProgressModel) View(width int, height int) string {
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %s\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("MODEL BENCHMARK RUNNER")))
	modelName := TruncateVisual(b.ModelName, max(20, width-14), "...")
	sb.WriteString(fmt.Sprintf("  Model: %s\n\n", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(modelName)))

	renderStep := func(stepName string, step BenchmarkProgressStep) string {
		bullet := "[ ]"
		if b.Step > step {
			bullet = StyleSuccess.Render("[✓]")
		} else if b.Step == step {
			if b.Step == StepError {
				bullet = StyleDanger.Render("[X]")
			} else {
				bullet = lipgloss.NewStyle().Foreground(ColorAccent).Render("[●]")
			}
		}

		var style lipgloss.Style
		if b.Step == step {
			style = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
		} else if b.Step > step {
			style = lipgloss.NewStyle().Foreground(ColorWhite)
		} else {
			style = lipgloss.NewStyle().Foreground(ColorMuted)
		}
		return fmt.Sprintf("  %s %s\n", bullet, style.Render(stepName))
	}

	sb.WriteString(renderStep("Booting server instance on benchmark port...", StepBooting))
	sb.WriteString(renderStep("Running standard completion benchmark prompt...", StepRunningPrompt))
	sb.WriteString(renderStep("Saving performance evaluation records...", StepSavingData))

	sb.WriteString("\n")
	if b.Step == StepError && b.Err != nil {
		sb.WriteString(fmt.Sprintf("  %s\n\n", StyleDanger.Render(fmt.Sprintf("Error: %v", b.Err))))
		sb.WriteString("  " + StyleHelpKey.Render("[Esc]") + " Return to Browser\n")
	} else if b.Step == StepDone {
		sb.WriteString("  " + StyleSuccess.Render("Benchmark completed successfully!") + "\n\n")
		sb.WriteString("  " + StyleHelpKey.Render("[Esc/Enter]") + " View Performance Dashboard\n")
	} else {
		sb.WriteString("  " + lipgloss.NewStyle().Foreground(ColorMuted).Italic(true).Render("Please wait... This may take up to 20 seconds.") + "\n")
	}

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

type PerformanceDashboardModel struct {
	History []*benchmark.BenchmarkResult
	Cursor  int
	Specs   *hardware.HardwareSpecs
}

func NewPerformanceDashboardModel(history []*benchmark.BenchmarkResult) *PerformanceDashboardModel {
	specs, _ := hardware.DetectHardware()
	if specs == nil {
		specs = &hardware.HardwareSpecs{OS: runtime.GOOS}
	}
	return &PerformanceDashboardModel{
		History: history,
		Cursor:  0,
		Specs:   specs,
	}
}

func (d *PerformanceDashboardModel) GetStats() (fastest *benchmark.BenchmarkResult, efficient *benchmark.BenchmarkResult) {
	if len(d.History) == 0 {
		return nil, nil
	}
	var maxSpeed float64 = -1
	var maxEfficiency float64 = -1

	for _, r := range d.History {
		if r.TokensPerSec > maxSpeed {
			maxSpeed = r.TokensPerSec
			fastest = r
		}

		if r.RAMUsageMB > 0 {
			ramGB := r.RAMUsageMB / 1024.0
			eff := r.TokensPerSec / ramGB
			if eff > maxEfficiency {
				maxEfficiency = eff
				efficient = r
			}
		}
	}
	return fastest, efficient
}

func (d *PerformanceDashboardModel) View(width int, height int) string {
	cardWidth := width
	if cardWidth < 50 {
		cardWidth = 50
	}

	// 1. Run History Bento Card
	var historySB strings.Builder
	if len(d.History) == 0 {
		historySB.WriteString("No benchmark records found. Run a benchmark with [B] in the model browser.")
	} else {
		fastest, efficient := d.GetStats()
		if fastest != nil {
			fastestName := TruncateVisual(fastest.ModelName, 24, "...")
			fastestStr := fmt.Sprintf("Fastest Model: %s (%.2f t/s)", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(fastestName), fastest.TokensPerSec)
			effStr := "N/A"
			if efficient != nil {
				ramGB := efficient.RAMUsageMB / 1024.0
				effName := TruncateVisual(efficient.ModelName, 24, "...")
				effStr = fmt.Sprintf("Most Efficient: %s (%.2f t/s per GB RAM)", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(effName), efficient.TokensPerSec/ramGB)
			}
			historySB.WriteString(fmt.Sprintf("%s  •  %s\n\n", fastestStr, effStr))
		}

		historySB.WriteString(fmt.Sprintf("%-2s %-12s %-20s %-14s %-10s %-12s\n",
			"", "Date", "Model", "Speed", "Startup", "RAM/VRAM",
		))
		divWidth := cardWidth - 8
		if divWidth < 20 {
			divWidth = 20
		}
		historySB.WriteString(strings.Repeat("─", divWidth) + "\n")

		if d.Cursor < 0 {
			d.Cursor = 0
		}
		if len(d.History) > 0 && d.Cursor >= len(d.History) {
			d.Cursor = len(d.History) - 1
		}

		maxVisible := 5
		startIdx := len(d.History) - 1 - d.Cursor
		if startIdx < 0 {
			startIdx = 0
		}

		count := 0
		for i := startIdx; i >= 0 && count < maxVisible; i-- {
			r := d.History[i]
			dateStr := r.RunDate.Format("01-02 15:04")
			modelName := TruncateVisual(r.ModelName, 18, "...")
			memInfo := fmt.Sprintf("%.1fG/%.1fG", r.RAMUsageMB/1024.0, r.VRAMUsageMB/1024.0)

			marker := "  "
			rowStyle := lipgloss.NewStyle()
			if i == d.Cursor || (d.Cursor == len(d.History)-1-i) {
				marker = "▶ "
				rowStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
			}

			line := fmt.Sprintf("%-2s %-12s %-20s %-14s %-10s %-12s",
				marker,
				dateStr,
				modelName,
				fmt.Sprintf("%.2f t/s", r.TokensPerSec),
				fmt.Sprintf("%.2fs", float64(r.StartupTimeMs)/1000.0),
				memInfo,
			)
			historySB.WriteString(rowStyle.Render(line) + "\n")
			count++
		}
	}
	historyContent := strings.TrimRight(historySB.String(), "\n")

	// 2. Throughput & Latency Bento Card
	var chartSB strings.Builder
	if len(d.History) == 0 {
		chartSB.WriteString("No throughput data recorded. Benchmarks record Prompt Evaluation (TTFT) and Token Decode rates.")
	} else {
		selIdx := d.Cursor
		if selIdx < 0 || selIdx >= len(d.History) {
			selIdx = 0
		}
		r := d.History[selIdx]

		modelLabel := TruncateVisual(r.ModelName, max(20, cardWidth-20), "...")
		chartSB.WriteString(fmt.Sprintf("Benchmark Target: %s\n\n", lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(modelLabel)))

		promptSpeed := r.PromptTokensPerSec
		if promptSpeed == 0 && r.StartupTimeMs > 0 {
			promptSpeed = r.TokensPerSec * 1.5
		}
		ttftText := ""
		if r.TTFTMs > 0 {
			ttftText = fmt.Sprintf("(TTFT: %.1f ms)", r.TTFTMs)
		} else if r.StartupTimeMs > 0 {
			ttftText = fmt.Sprintf("(Startup: %.2fs)", float64(r.StartupTimeMs)/1000.0)
		}
		promptBar := RenderProgressBar(min(100.0, promptSpeed*1.5), 18, ColorSecondary)
		chartSB.WriteString(fmt.Sprintf("  %-20s [%s] %6.2f t/s  %s\n", "Prompt Eval (TTFT):", promptBar, promptSpeed, lipgloss.NewStyle().Foreground(ColorMuted).Render(ttftText)))

		decodeBar := RenderProgressBar(min(100.0, r.TokensPerSec*2.0), 18, ColorPrimary)
		peakText := ""
		if r.PeakTokensPerSec > 0 {
			peakText = fmt.Sprintf("(Peak: %.2f t/s)", r.PeakTokensPerSec)
		}
		chartSB.WriteString(fmt.Sprintf("  %-20s [%s] %6.2f t/s  %s\n\n", "Decode Generation:", decodeBar, r.TokensPerSec, lipgloss.NewStyle().Foreground(ColorMuted).Render(peakText)))

		memBreakdown := fmt.Sprintf("Host RSS: %.1f MB  •  GPU Dedicated: %.1f MB  •  Total: %.1f MB",
			r.RAMUsageMB, r.VRAMUsageMB, r.RAMUsageMB+r.VRAMUsageMB)
		chartSB.WriteString(fmt.Sprintf("  %-20s %s", "Memory Footprint:", lipgloss.NewStyle().Foreground(ColorWhite).Render(memBreakdown)))
	}
	chartContent := strings.TrimRight(chartSB.String(), "\n")

	// 3. Hardware Configuration Bento Card
	var hwSB strings.Builder
	specs := d.Specs
	if specs == nil {
		specs, _ = hardware.DetectHardware()
		if specs == nil {
			specs = &hardware.HardwareSpecs{OS: runtime.GOOS}
		}
	}

	osArch := specs.OS

	cpuModel := specs.CPU.Model
	if cpuModel == "" {
		cpuModel = "Unknown CPU"
	}
	cpuInfo := fmt.Sprintf("%s (%d Cores, %d Threads)", cpuModel, specs.CPU.PhysicalCores, specs.CPU.Threads)
	if specs.CPU.PhysicalCores == 0 {
		cpuInfo = cpuModel
	}

	gpuInfo := specs.GPU.Type
	if specs.GPU.Name != "" && specs.GPU.Name != specs.GPU.Type {
		gpuInfo = fmt.Sprintf("%s (%s)", specs.GPU.Name, specs.GPU.Type)
	}
	if specs.GPU.VRAM > 0 {
		gpuInfo += fmt.Sprintf(" • %.1f GB VRAM", float64(specs.GPU.VRAM)/(1024.0*1024.0*1024.0))
	}

	ramInfo := "System RAM"
	if specs.RAM.Total > 0 {
		ramInfo = fmt.Sprintf("%.1f GB Total (%.1f GB Available)", float64(specs.RAM.Total)/(1024.0*1024.0*1024.0), float64(specs.RAM.Available)/(1024.0*1024.0*1024.0))
	}

	colWidth := max(35, (cardWidth-10)/2)
	row1Left := lipgloss.NewStyle().Width(colWidth).Render(fmt.Sprintf("OS / Platform: %s", lipgloss.NewStyle().Foreground(ColorWhite).Render(osArch)))
	row1Right := fmt.Sprintf("CPU: %s", lipgloss.NewStyle().Foreground(ColorWhite).Render(TruncateVisual(cpuInfo, colWidth-5, "...")))
	hwSB.WriteString(fmt.Sprintf("  %s │  %s\n", row1Left, row1Right))

	row2Left := lipgloss.NewStyle().Width(colWidth).Render(fmt.Sprintf("GPU Device:    %s", lipgloss.NewStyle().Foreground(ColorWhite).Render(TruncateVisual(gpuInfo, colWidth-15, "..."))))
	row2Right := fmt.Sprintf("RAM: %s", lipgloss.NewStyle().Foreground(ColorWhite).Render(ramInfo))
	hwSB.WriteString(fmt.Sprintf("  %s │  %s", row2Left, row2Right))

	hwContent := hwSB.String()

	// Assemble SurfaceCards
	historyBadge := fmt.Sprintf("%d runs", len(d.History))
	card1 := SurfaceCard("Benchmark Run History", historyContent, cardWidth, true, historyBadge)
	card2 := SurfaceCard("Throughput & Latency (Tokens/sec)", chartContent, cardWidth, false, "TTFT + Decode")
	card3 := SurfaceCard("Test Platform", hwContent, cardWidth, false, "Specs")

	helpFooter := fmt.Sprintf("  %s Back to Browser  %s Select Run  %s Run Benchmark",
		StyleHelpKey.Render("[Esc]"),
		StyleHelpKey.Render("[↑/↓]"),
		StyleHelpKey.Render("[B]"),
	)

	return lipgloss.JoinVertical(lipgloss.Left, card1, card2, card3, helpFooter)
}

