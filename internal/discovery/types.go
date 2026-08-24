package discovery

import (
	"time"

	"github.com/BIJJUDAMA/runora/model"
)

// SourceType identifies the origin runtime of an external model library.
type SourceType string

const (
	SourceOllama   SourceType = "ollama"
	SourceLMStudio SourceType = "lmstudio"
	SourceCustom   SourceType = "custom"
)

// SourceConfig represents configuration and detection state for a model library source.
type SourceConfig struct {
	Type         SourceType `json:"type"`
	Name         string     `json:"name"`
	Enabled      bool       `json:"enabled"`
	CustomPath   string     `json:"custom_path,omitempty"`
	DetectedPath string     `json:"detected_path,omitempty"`
	Detected     bool       `json:"detected"`
	LastScanTime time.Time  `json:"last_scan_time,omitempty"`
	ModelCount   int        `json:"model_count"`
}

// DiscoveredModel represents a single model located from an external library source.
type DiscoveredModel struct {
	Source       SourceType
	LogicalName  string
	FilePath     string
	Metadata     *model.GGUFMetadata
}

// ImportSummary aggregates results from scanning all configured model sources.
type ImportSummary struct {
	TotalDiscovered int
	TotalImported   int
	TotalSkipped    int
	SourcesScanned  int
	Duration        time.Duration
}

// SourceScanner is the common interface implemented by runtime-specific library scanners.
type SourceScanner interface {
	Type() SourceType
	Name() string
	DetectPaths() []string
	Scan(path string) ([]*DiscoveredModel, error)
}
