package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BIJJUDAMA/runora/model"
)

// LMStudioScanner discovers and parses models from local LM Studio installations.
type LMStudioScanner struct{}

// NewLMStudioScanner creates a new scanner for LM Studio model libraries.
func NewLMStudioScanner() *LMStudioScanner {
	return &LMStudioScanner{}
}

func (s *LMStudioScanner) Type() SourceType {
	return SourceLMStudio
}

func (s *LMStudioScanner) Name() string {
	return "LM Studio"
}

// DetectPaths searches standard operating system locations for an LM Studio models directory.
func (s *LMStudioScanner) DetectPaths() []string {
	var candidates []string

	// 1. Check environment variable override
	if envPath := os.Getenv("LM_STUDIO_MODELS"); envPath != "" {
		candidates = append(candidates, filepath.Clean(envPath))
	}

	// 2. Platform-specific user home and system paths
	home, homeErr := os.UserHomeDir()
	if homeErr == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".cache", "lm-studio", "models"))
		candidates = append(candidates, filepath.Join(home, ".lmstudio", "models"))
	}

	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			candidates = append(candidates, filepath.Join(appData, "LM Studio", "models"))
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates, filepath.Join(localAppData, "LM Studio", "models"))
		}
		if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
			candidates = append(candidates, filepath.Join(userProfile, ".cache", "lm-studio", "models"))
			candidates = append(candidates, filepath.Join(userProfile, ".lmstudio", "models"))
		}
	}

	// Return deduplicated valid directory paths
	var valid []string
	seen := make(map[string]bool)
	for _, c := range candidates {
		c = filepath.Clean(c)
		if seen[c] {
			continue
		}
		seen[c] = true
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			valid = append(valid, c)
		}
	}

	return valid
}

// Scan recursively walks the LM Studio models directory for .gguf and .onnx model files.
func (s *LMStudioScanner) Scan(rootDir string) ([]*DiscoveredModel, error) {
	if _, err := os.Stat(rootDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("lm studio models directory not found: %s", rootDir)
	}

	var discovered []*DiscoveredModel
	seenFiles := make(map[string]bool)

	err := filepath.WalkDir(rootDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		cleanPath := filepath.Clean(path)
		if seenFiles[cleanPath] {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(cleanPath))
		if ext != ".gguf" && ext != ".onnx" {
			return nil
		}

		fi, err := d.Info()
		if err != nil {
			return nil
		}

		seenFiles[cleanPath] = true

		// Derive publisher/model logical name
		relPath, relErr := filepath.Rel(rootDir, cleanPath)
		logicalName := ""
		if relErr == nil {
			parts := strings.Split(filepath.ToSlash(relPath), "/")
			if len(parts) >= 2 {
				// e.g. "publisher/model-name/model.gguf" -> "publisher/model-name"
				logicalName = strings.Join(parts[:len(parts)-1], "/")
			} else {
				logicalName = strings.TrimSuffix(filepath.Base(cleanPath), ext)
			}
		}
		if logicalName == "" {
			logicalName = strings.TrimSuffix(filepath.Base(cleanPath), ext)
		}

		var meta *model.GGUFMetadata
		if ext == ".gguf" {
			var parseErr error
			meta, parseErr = model.ParseGGUF(cleanPath)
			if parseErr != nil {
				meta = &model.GGUFMetadata{
					FilePath:     cleanPath,
					Name:         logicalName,
					Architecture: "unknown",
					Quantization: "unknown",
					FileSize:     fi.Size(),
					Runtime:      "llama.cpp",
					Task:         "TEXT_GENERATION",
				}
			}
		} else {
			meta = &model.GGUFMetadata{
				FilePath:     cleanPath,
				Name:         logicalName,
				Architecture: "onnx",
				Quantization: "fp32",
				FileSize:     fi.Size(),
				Runtime:      "ONNX Runtime",
				Task:         "TEXT_GENERATION",
			}
		}

		if meta.Name == "" || meta.Name == "unknown" {
			meta.Name = logicalName
		}

		discovered = append(discovered, &DiscoveredModel{
			Source:      SourceLMStudio,
			LogicalName: logicalName,
			FilePath:    cleanPath,
			Metadata:    meta,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan lm studio models: %w", err)
	}

	return discovered, nil
}
