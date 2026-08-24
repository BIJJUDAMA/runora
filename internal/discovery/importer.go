package discovery

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/BIJJUDAMA/runora/model"
)

// Importer coordinates discovery scanners, handles deduplication, and indexes models.
type Importer struct {
	scanners map[SourceType]SourceScanner
}

// NewImporter initializes an Importer with default runtime scanners.
func NewImporter() *Importer {
	imp := &Importer{
		scanners: make(map[SourceType]SourceScanner),
	}
	imp.RegisterScanner(NewOllamaScanner())
	imp.RegisterScanner(NewLMStudioScanner())
	return imp
}

// RegisterScanner registers a custom or runtime-specific scanner.
func (imp *Importer) RegisterScanner(s SourceScanner) {
	imp.scanners[s.Type()] = s
}

// DetectAllSources probes the environment and operating system paths for installed runtimes.
func (imp *Importer) DetectAllSources() []SourceConfig {
	var sources []SourceConfig

	// 1. Ollama
	if scanner, ok := imp.scanners[SourceOllama]; ok {
		paths := scanner.DetectPaths()
		detected := len(paths) > 0
		detectedPath := ""
		if detected {
			detectedPath = paths[0]
		}
		sources = append(sources, SourceConfig{
			Type:         SourceOllama,
			Name:         "Ollama",
			Enabled:      detected,
			Detected:     detected,
			DetectedPath: detectedPath,
		})
	}

	// 2. LM Studio
	if scanner, ok := imp.scanners[SourceLMStudio]; ok {
		paths := scanner.DetectPaths()
		detected := len(paths) > 0
		detectedPath := ""
		if detected {
			detectedPath = paths[0]
		}
		sources = append(sources, SourceConfig{
			Type:         SourceLMStudio,
			Name:         "LM Studio",
			Enabled:      detected,
			Detected:     detected,
			DetectedPath: detectedPath,
		})
	}

	return sources
}

// ScanSource scans a single source configuration and returns discovered models.
func (imp *Importer) ScanSource(cfg *SourceConfig) ([]*DiscoveredModel, error) {
	scanner, ok := imp.scanners[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported source type: %s", cfg.Type)
	}

	targetPath := cfg.CustomPath
	if targetPath == "" {
		targetPath = cfg.DetectedPath
	}
	if targetPath == "" {
		return nil, nil
	}

	models, err := scanner.Scan(targetPath)
	if err != nil {
		return nil, err
	}

	cfg.LastScanTime = time.Now()
	cfg.ModelCount = len(models)
	cfg.Detected = true

	return models, nil
}

// ScanAll scans all enabled source configurations, applies deduplication, and returns discovered models.
func (imp *Importer) ScanAll(configs []SourceConfig, existingModelPaths map[string]bool) ([]*DiscoveredModel, ImportSummary, error) {
	startTime := time.Now()
	summary := ImportSummary{}

	seenPaths := make(map[string]bool)
	if existingModelPaths != nil {
		for k, v := range existingModelPaths {
			if v {
				seenPaths[filepath.Clean(k)] = true
			}
		}
	}

	var allDiscovered []*DiscoveredModel

	for i := range configs {
		cfg := &configs[i]
		if !cfg.Enabled {
			continue
		}

		models, err := imp.ScanSource(cfg)
		if err != nil {
			continue
		}

		summary.SourcesScanned++
		summary.TotalDiscovered += len(models)

		for _, m := range models {
			clean := filepath.Clean(m.FilePath)
			if seenPaths[clean] {
				summary.TotalSkipped++
				continue
			}
			seenPaths[clean] = true
			allDiscovered = append(allDiscovered, m)
			summary.TotalImported++
		}
	}

	summary.Duration = time.Since(startTime)
	return allDiscovered, summary, nil
}

// ConvertToGGUFMetadata converts discovered models into Runora GGUFMetadata entries.
func ConvertToGGUFMetadata(discovered []*DiscoveredModel) []*model.GGUFMetadata {
	var result []*model.GGUFMetadata
	for _, dm := range discovered {
		if dm.Metadata != nil {
			result = append(result, dm.Metadata)
		}
	}
	return result
}
