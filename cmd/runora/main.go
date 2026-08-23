package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/BIJJUDAMA/runora/config"
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

func main() {
	resetOnboarding := flag.Bool("reset-onboarding", false, "Reset and run the onboarding tour")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("runora", buildVersion())
		os.Exit(0)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	if *resetOnboarding {
		cfg.OnboardingCompleted = false
		if err := cfg.Save(); err != nil {
			fmt.Printf("Error saving configuration: %v\n", err)
			os.Exit(1)
		}
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
