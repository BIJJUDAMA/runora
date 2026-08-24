package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Helper to create mock GGUF file with minimum header
func createMockGGUF(t *testing.T, targetPath string) {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	// Minimal GGUF v3 header: Magic "GGUF", Version 3, TensorCount 0, KVCount 0
	header := []byte{
		'G', 'G', 'U', 'F',
		0x03, 0x00, 0x00, 0x00, // Version 3
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Tensor count 0
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // KV count 0
	}

	if err := os.WriteFile(targetPath, header, 0644); err != nil {
		t.Fatalf("failed to write mock gguf: %v", err)
	}
}

func TestOllamaManifestParsingAndBlobResolution(t *testing.T) {
	tempDir := t.TempDir()
	manifestsDir := filepath.Join(tempDir, "manifests", "registry.ollama.ai", "library", "llama3.2")
	blobsDir := filepath.Join(tempDir, "blobs")
	_ = os.MkdirAll(manifestsDir, 0755)
	_ = os.MkdirAll(blobsDir, 0755)

	blobDigest := "sha256:7b5eb123456789abcdef"
	blobFileName := "sha256-7b5eb123456789abcdef"
	blobPath := filepath.Join(blobsDir, blobFileName)
	createMockGGUF(t, blobPath)

	// Create Ollama manifest JSON
	manifest := OllamaManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.docker.distribution.manifest.v2+json",
		Layers: []OllamaManifestLayer{
			{
				MediaType: "application/vnd.ollama.image.model",
				Digest:    blobDigest,
				Size:      int64(len(blobFileName)),
			},
			{
				MediaType: "application/vnd.ollama.image.license",
				Digest:    "sha256:1111111111",
				Size:      128,
			},
		},
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	manifestFile := filepath.Join(manifestsDir, "latest")
	if err := os.WriteFile(manifestFile, manifestData, 0644); err != nil {
		t.Fatalf("failed to write manifest file: %v", err)
	}

	scanner := NewOllamaScanner()
	models, err := scanner.Scan(tempDir)
	if err != nil {
		t.Fatalf("unexpected scan error: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("expected 1 model discovered, got %d", len(models))
	}

	m := models[0]
	if m.Source != SourceOllama {
		t.Errorf("expected source %s, got %s", SourceOllama, m.Source)
	}
	if m.LogicalName != "llama3.2:latest" {
		t.Errorf("expected logical name 'llama3.2:latest', got %q", m.LogicalName)
	}
	if filepath.Clean(m.FilePath) != filepath.Clean(blobPath) {
		t.Errorf("expected blob path %q, got %q", blobPath, m.FilePath)
	}
}

func TestLMStudioRecursiveDiscovery(t *testing.T) {
	tempDir := t.TempDir()
	model1Path := filepath.Join(tempDir, "TheBloke", "Mistral-7B", "mistral-7b.Q4_K_M.gguf")
	model2Path := filepath.Join(tempDir, "microsoft", "phi-3", "phi-3-mini.onnx")
	ignoredFile := filepath.Join(tempDir, "temp", "notes.txt")

	createMockGGUF(t, model1Path)
	createMockGGUF(t, model2Path)
	_ = os.MkdirAll(filepath.Dir(ignoredFile), 0755)
	_ = os.WriteFile(ignoredFile, []byte("ignore me"), 0644)

	scanner := NewLMStudioScanner()
	models, err := scanner.Scan(tempDir)
	if err != nil {
		t.Fatalf("unexpected scan error: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models discovered, got %d", len(models))
	}

	names := make(map[string]bool)
	for _, m := range models {
		if m.Source != SourceLMStudio {
			t.Errorf("expected source %s, got %s", SourceLMStudio, m.Source)
		}
		names[m.LogicalName] = true
	}

	if !names["TheBloke/Mistral-7B"] {
		t.Errorf("expected to find logical name 'TheBloke/Mistral-7B', got %v", names)
	}
	if !names["microsoft/phi-3"] {
		t.Errorf("expected to find logical name 'microsoft/phi-3', got %v", names)
	}
}

func TestImporterDeduplicationAndSummary(t *testing.T) {
	tempDir := t.TempDir()
	ollamaDir := filepath.Join(tempDir, "ollama")
	lmStudioDir := filepath.Join(tempDir, "lmstudio")

	// Setup mock Ollama
	manifestsDir := filepath.Join(ollamaDir, "manifests", "registry.ollama.ai", "library", "qwen")
	blobsDir := filepath.Join(ollamaDir, "blobs")
	_ = os.MkdirAll(manifestsDir, 0755)
	_ = os.MkdirAll(blobsDir, 0755)

	blobDigest := "sha256:qwenblob12345"
	blobPath := filepath.Join(blobsDir, "sha256-qwenblob12345")
	createMockGGUF(t, blobPath)

	manifest := OllamaManifest{
		SchemaVersion: 2,
		Layers: []OllamaManifestLayer{
			{MediaType: "application/vnd.ollama.image.model", Digest: blobDigest, Size: 100},
		},
	}
	mData, _ := json.Marshal(manifest)
	_ = os.WriteFile(filepath.Join(manifestsDir, "7b"), mData, 0644)

	// Setup mock LM Studio
	lmModelPath := filepath.Join(lmStudioDir, "meta", "llama-3-8b.gguf")
	createMockGGUF(t, lmModelPath)

	importer := NewImporter()
	configs := []SourceConfig{
		{
			Type:         SourceOllama,
			Name:         "Ollama",
			Enabled:      true,
			DetectedPath: ollamaDir,
		},
		{
			Type:         SourceLMStudio,
			Name:         "LM Studio",
			Enabled:      true,
			DetectedPath: lmStudioDir,
		},
	}

	// 1. First scan without pre-existing paths -> all 2 models imported
	discovered, summary, err := importer.ScanAll(configs, nil)
	if err != nil {
		t.Fatalf("unexpected ScanAll error: %v", err)
	}
	if len(discovered) != 2 {
		t.Fatalf("expected 2 discovered models, got %d", len(discovered))
	}
	if summary.TotalImported != 2 {
		t.Errorf("expected 2 total imported, got %d", summary.TotalImported)
	}
	if summary.TotalSkipped != 0 {
		t.Errorf("expected 0 skipped, got %d", summary.TotalSkipped)
	}

	// 2. Second scan with lmModelPath already in existing paths -> 1 imported, 1 skipped
	existing := map[string]bool{
		filepath.Clean(lmModelPath): true,
	}
	discovered2, summary2, err := importer.ScanAll(configs, existing)
	if err != nil {
		t.Fatalf("unexpected ScanAll error: %v", err)
	}
	if len(discovered2) != 1 {
		t.Fatalf("expected 1 discovered model (Ollama only), got %d", len(discovered2))
	}
	if summary2.TotalImported != 1 {
		t.Errorf("expected 1 imported, got %d", summary2.TotalImported)
	}
	if summary2.TotalSkipped != 1 {
		t.Errorf("expected 1 skipped, got %d", summary2.TotalSkipped)
	}

	// 3. ConvertToGGUFMetadata
	ggufMetas := ConvertToGGUFMetadata(discovered)
	if len(ggufMetas) != 2 {
		t.Errorf("expected 2 GGUFMetadata converted, got %d", len(ggufMetas))
	}
}

func TestDetectAllSourcesFallback(t *testing.T) {
	importer := NewImporter()
	sources := importer.DetectAllSources()

	if len(sources) < 2 {
		t.Fatalf("expected at least 2 default source configurations, got %d", len(sources))
	}

	types := make(map[SourceType]bool)
	for _, s := range sources {
		types[s.Type] = true
	}

	if !types[SourceOllama] {
		t.Errorf("expected Ollama source configured")
	}
	if !types[SourceLMStudio] {
		t.Errorf("expected LM Studio source configured")
	}
}
