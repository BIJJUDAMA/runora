package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/BIJJUDAMA/runora/config"
	"github.com/BIJJUDAMA/runora/hardware"
	"github.com/BIJJUDAMA/runora/model"
	"github.com/BIJJUDAMA/runora/runner"
	"github.com/BIJJUDAMA/runora/ui"
)

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return strings.TrimSuffix(v, "+dirty")
		}
	}
	return "dev"
}

func formatFileSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	if bytes >= gb {
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gb))
	}
	if bytes >= mb {
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(mb))
	}
	if bytes >= kb {
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(kb))
	}
	return fmt.Sprintf("%d B", bytes)
}

func main() {
	resetOnboarding := flag.Bool("reset-onboarding", false, "Reset and run the onboarding tour")
	showVersion := flag.Bool("version", false, "Print version and exit")
	listModels := flag.Bool("list-models", false, "List discovered models and exit")
	jsonOutput := flag.Bool("json", false, "Format output as JSON (for --list-models or --status)")
	statusFlag := flag.Bool("status", false, "Print current runtime and hardware status and exit")
	dataDir := flag.String("data-dir", "", "Custom application data directory path")
	modelsPath := flag.String("models", "", "Custom models directory path to scan")
	flag.Parse()

	if *showVersion {
		fmt.Println("runora", buildVersion())
		os.Exit(0)
	}

	var cfg *config.Config
	var err error

	if *dataDir != "" {
		cfg, err = config.LoadFromDir(*dataDir)
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	if *modelsPath != "" {
		cfg.Paths.Models = *modelsPath
	}

	if *resetOnboarding {
		cfg.OnboardingCompleted = false
		if err := cfg.Save(); err != nil {
			fmt.Printf("Error saving configuration: %v\n", err)
			os.Exit(1)
		}
	}

	// Headless --list-models
	if *listModels {
		allDirs := cfg.Paths.AllModelDirectories()
		models, err := model.DiscoverModels(allDirs...)
		if err != nil {
			if *jsonOutput {
				_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
			} else {
				fmt.Printf("Error discovering models: %v\n", err)
			}
			os.Exit(1)
		}
		if models == nil {
			models = []*model.GGUFMetadata{}
		}

		if *jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(models); err != nil {
				fmt.Printf("Error encoding JSON: %v\n", err)
				os.Exit(1)
			}
			os.Exit(0)
		}

		fmt.Printf("Discovered %d models across %d directories:\n\n", len(models), len(allDirs))
		if len(models) == 0 {
			fmt.Println("  No models found.")
			os.Exit(0)
		}

		fmt.Printf("  %-35s %-12s %-10s %-10s %-8s %-16s\n", "NAME", "ARCH", "QUANT", "SIZE", "SHARDS", "TASK")
		fmt.Println("  " + strings.Repeat("-", 95))
		for _, m := range models {
			shardStr := "-"
			if m.ShardCount > 1 {
				shardStr = fmt.Sprintf("%d shards", m.ShardCount)
			}
			name := m.Name
			if len(name) > 33 {
				name = name[:30] + "..."
			}
			sizeStr := formatFileSize(m.FileSize)
			fmt.Printf("  %-35s %-12s %-10s %-10s %-8s %-16s\n",
				name, m.Architecture, m.Quantization, sizeStr, shardStr, m.Task)
		}
		os.Exit(0)
	}

	// Headless --status
	if *statusFlag {
		srv := runner.NewMultiRuntimeManager(cfg.Paths.Cache)
		instances := srv.GetAllInstances()
		status, activeModel, port := srv.GetStatus()
		specs, _ := hardware.DetectHardware()

		var statusStr string
		switch status {
		case runner.StatusRunning:
			statusStr = "running"
		case runner.StatusFailed:
			statusStr = "failed"
		default:
			statusStr = "stopped"
		}

		if *jsonOutput {
			type StatusReport struct {
				ServerStatus string                   `json:"server_status"`
				ActiveModel  string                   `json:"active_model,omitempty"`
				Port         int                      `json:"port,omitempty"`
				Instances    []runner.InstanceInfo    `json:"instances"`
				Hardware     *hardware.HardwareSpecs `json:"hardware,omitempty"`
			}
			rep := StatusReport{
				ServerStatus: statusStr,
				ActiveModel:  activeModel,
				Port:         port,
				Instances:    instances,
				Hardware:     specs,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(rep)
			os.Exit(0)
		}

		fmt.Println("Runora Status Report")
		fmt.Println("====================")
		fmt.Printf("Server Status: %s\n", statusStr)
		if len(instances) > 0 {
			fmt.Println("\nRunning Instances:")
			for _, inst := range instances {
				fmt.Printf("  - Port %d: %s (PID: %d)\n", inst.Port, inst.ModelPath, inst.PID)
			}
		} else {
			fmt.Println("Active Instances: None")
		}

		if specs != nil {
			fmt.Println("\nHardware Specifications:")
			fmt.Printf("  OS:       %s\n", specs.OS)
			fmt.Printf("  CPU:      %s (%d threads)\n", specs.CPU.Model, specs.CPU.Threads)
			fmt.Printf("  RAM:      %s total / %s available\n", formatFileSize(int64(specs.RAM.Total)), formatFileSize(int64(specs.RAM.Available)))
			if specs.GPU.VRAM > 0 {
				fmt.Printf("  GPU:      %s (%s VRAM)\n", specs.GPU.Name, formatFileSize(int64(specs.GPU.VRAM)))
			}
		}
		os.Exit(0)
	}

	srv := runner.NewMultiRuntimeManager(cfg.Paths.Cache)
	defer func() {
		_ = srv.Stop()
	}()

	m := ui.NewBrowserModel(cfg, srv)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		log.Fatalf("Error running program: %v", err)
	}
}
