package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/BIJJUDAMA/runora/benchmark"
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
			bullet = StyleSuccess.Render("[✔]")
		} else if b.Step == step {
			if b.Step == StepError {
				bullet = StyleDanger.Render("[✘]")
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
}

func NewPerformanceDashboardModel(history []*benchmark.BenchmarkResult) *PerformanceDashboardModel {
	return &PerformanceDashboardModel{
		History: history,
		Cursor:  0,
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
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  %s\n\n", lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("PERFORMANCE DASHBOARD")))

	fastest, efficient := d.GetStats()
	if fastest == nil {
		sb.WriteString("  No benchmark records found. Run a benchmark with " + StyleHelpKey.Render("[B]") + " in the browser.\n\n")
	} else {
		fastestName := TruncateVisual(fastest.ModelName, max(20, width-35), "...")
		sb.WriteString(fmt.Sprintf("  %-20s %s (%.2f tokens/sec)\n",
			"Fastest Model:",
			lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(fastestName),
			fastest.TokensPerSec,
		))

		effStr := "N/A"
		if efficient != nil {
			ramGB := efficient.RAMUsageMB / 1024.0
			effName := TruncateVisual(efficient.ModelName, max(20, width-45), "...")
			effStr = fmt.Sprintf("%s (%.2f t/s per GB RAM)",
				lipgloss.NewStyle().Foreground(ColorWhite).Bold(true).Render(effName),
				efficient.TokensPerSec/ramGB,
			)
		}
		sb.WriteString(fmt.Sprintf("  %-20s %s\n\n", "Most Efficient:", effStr))

		sb.WriteString("  " + lipgloss.NewStyle().Bold(true).Render("Benchmark History:") + "\n")

		// Header row
		sb.WriteString(fmt.Sprintf("  %-10s %-16s %-12s %-10s %-10s\n",
			"Date", "Model", "Speed", "Startup", "RAM/VRAM",
		))
		divWidth := width - 8
		if divWidth < 10 {
			divWidth = 10
		}
		sb.WriteString("  " + strings.Repeat("─", divWidth) + "\n")

		// Render rows
		maxVisible := height - 16
		if maxVisible < 1 {
			maxVisible = 1
		}

		if d.Cursor < 0 {
			d.Cursor = 0
		}
		if len(d.History) > 0 && d.Cursor >= len(d.History) {
			d.Cursor = len(d.History) - 1
		}

		startIdx := len(d.History) - 1 - d.Cursor
		if startIdx < 0 {
			startIdx = 0
		}

		count := 0
		for i := startIdx; i >= 0 && count < maxVisible; i-- {
			r := d.History[i]
			dateStr := r.RunDate.Format("01-02 15:04")

			modelName := TruncateVisual(r.ModelName, 16, "...")

			memInfo := fmt.Sprintf("%.1fG/%.1fG", r.RAMUsageMB/1024.0, r.VRAMUsageMB/1024.0)

			sb.WriteString(fmt.Sprintf("  %-10s %-16s %-12s %-10s %-10s\n",
				dateStr,
				modelName,
				fmt.Sprintf("%.2f t/s", r.TokensPerSec),
				fmt.Sprintf("%.2fs", float64(r.StartupTimeMs)/1000.0),
				memInfo,
			))
			count++
		}
		sb.WriteString("\n")
	}

	helpStr := fmt.Sprintf("%s Back to Browser", StyleHelpKey.Render("[Esc]"))
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
