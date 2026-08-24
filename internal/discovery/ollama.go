package discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BIJJUDAMA/runora/model"
)

// OllamaManifestLayer describes an individual layer in an Ollama JSON manifest.
type OllamaManifestLayer struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

// OllamaManifest describes the top-level schema of an Ollama model manifest.
type OllamaManifest struct {
	SchemaVersion int                   `json:"schemaVersion"`
	MediaType     string                `json:"mediaType"`
	Config        OllamaManifestLayer   `json:"config"`
	Layers        []OllamaManifestLayer `json:"layers"`
}

// OllamaScanner discovers and parses models from local Ollama installations.
type OllamaScanner struct{}

// NewOllamaScanner creates a new scanner for Ollama model libraries.
func NewOllamaScanner() *OllamaScanner {
	return &OllamaScanner{}
}

func (s *OllamaScanner) Type() SourceType {
	return SourceOllama
}

func (s *OllamaScanner) Name() string {
	return "Ollama"
}

// DetectPaths searches standard operating system locations for an Ollama models directory.
func (s *OllamaScanner) DetectPaths() []string {
	var candidates []string

	// 1. Check environment variable override
	if envPath := os.Getenv("OLLAMA_MODELS"); envPath != "" {
		candidates = append(candidates, filepath.Clean(envPath))
	}

	// 2. Platform-specific user home and system paths
	home, homeErr := os.UserHomeDir()
	if homeErr == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".ollama", "models"))
	}

	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates, filepath.Join(localAppData, "Ollama", "models"))
		}
		if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
			candidates = append(candidates, filepath.Join(userProfile, ".ollama", "models"))
		}
	} else {
		candidates = append(candidates, "/usr/share/ollama/.ollama/models")
		candidates = append(candidates, "/var/lib/ollama/.ollama/models")
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

// Scan parses all model manifests and resolves underlying GGUF model blobs.
func (s *OllamaScanner) Scan(rootDir string) ([]*DiscoveredModel, error) {
	manifestsDir := filepath.Join(rootDir, "manifests")
	blobsDir := filepath.Join(rootDir, "blobs")

	if _, err := os.Stat(manifestsDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("ollama manifests directory not found: %s", manifestsDir)
	}

	var discovered []*DiscoveredModel
	seenBlobs := make(map[string]bool)

	// Walk the manifests tree
	err := filepath.WalkDir(manifestsDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		// Read manifest file
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var manifest OllamaManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil
		}

		// Find the model layer blob digest
		var modelDigest string
		for _, layer := range manifest.Layers {
			if layer.MediaType == "application/vnd.ollama.image.model" {
				modelDigest = layer.Digest
				break
			}
		}

		if modelDigest == "" {
			return nil
		}

		// Convert digest format "sha256:abc1234..." to file name "sha256-abc1234..."
		blobFileName := strings.Replace(modelDigest, ":", "-", 1)
		blobPath := filepath.Join(blobsDir, blobFileName)

		if seenBlobs[blobPath] {
			return nil
		}

		fi, err := os.Stat(blobPath)
		if err != nil || fi.IsDir() {
			return nil
		}

		seenBlobs[blobPath] = true

		// Derive logical name from relative path in manifests directory
		// e.g. manifests/registry.ollama.ai/library/llama3.2/latest -> "llama3.2:latest"
		relPath, relErr := filepath.Rel(manifestsDir, path)
		logicalName := ""
		if relErr == nil {
			parts := strings.Split(filepath.ToSlash(relPath), "/")
			if len(parts) >= 3 {
				// Strip registry prefix if present
				parts = parts[1:]
			}
			if len(parts) >= 2 {
				modelName := parts[len(parts)-2]
				tagName := parts[len(parts)-1]
				logicalName = fmt.Sprintf("%s:%s", modelName, tagName)
			} else {
				logicalName = strings.Join(parts, ":")
			}
		}
		if logicalName == "" {
			logicalName = d.Name()
		}

		// Parse GGUF metadata from blob file
		meta, err := model.ParseGGUF(blobPath)
		if err != nil {
			// Create fallback entry if GGUF header parsing failed
			meta = &model.GGUFMetadata{
				FilePath:     blobPath,
				Name:         logicalName,
				Architecture: "unknown",
				Quantization: "unknown",
				FileSize:     fi.Size(),
			}
		} else {
			if meta.Name == "" || meta.Name == "unknown" {
				meta.Name = logicalName
			}
		}

		discovered = append(discovered, &DiscoveredModel{
			Source:      SourceOllama,
			LogicalName: logicalName,
			FilePath:    blobPath,
			Metadata:    meta,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to scan ollama manifests: %w", err)
	}

	return discovered, nil
}
