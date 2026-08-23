package model

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func createMockGGUFAt(path string, name string, arch string, ctxLen uint32) error {
	var buf bytes.Buffer
	// Magic
	_, _ = buf.Write([]byte("GGUF"))
	// Version
	_ = binary.Write(&buf, binary.LittleEndian, uint32(3))
	// Tensor count
	_ = binary.Write(&buf, binary.LittleEndian, uint64(0))
	// Metadata KV count
	_ = binary.Write(&buf, binary.LittleEndian, uint64(3))

	// KV 1: general.name
	writeGGUFString(&buf, "general.name")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeString))
	writeGGUFString(&buf, name)

	// KV 2: general.architecture
	writeGGUFString(&buf, "general.architecture")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeString))
	writeGGUFString(&buf, arch)

	// KV 3: <arch>.context_length
	writeGGUFString(&buf, arch+".context_length")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeUInt32))
	_ = binary.Write(&buf, binary.LittleEndian, ctxLen)

	return os.WriteFile(path, buf.Bytes(), 0644)
}

func TestDiscoverModels(t *testing.T) {
	// Create a temporary directory structure
	tempDir, err := os.MkdirTemp("", "llama-models-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create nested folders
	dir1 := filepath.Join(tempDir, "Gemma")
	dir2 := filepath.Join(tempDir, "Qwen", "Coder")
	if err := os.MkdirAll(dir1, 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}
	if err := os.MkdirAll(dir2, 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	// Create mock files
	model1Path := filepath.Join(dir1, "gemma-2b.gguf")
	model2Path := filepath.Join(dir2, "qwen-coder-7b.gguf")
	nonModelPath := filepath.Join(tempDir, "readme.txt")

	if err := createMockGGUFAt(model1Path, "Gemma 2B", "gemma", 2048); err != nil {
		t.Fatalf("failed to create mock GGUF: %v", err)
	}
	if err := createMockGGUFAt(model2Path, "Qwen Coder 7B", "qwen2", 32768); err != nil {
		t.Fatalf("failed to create mock GGUF: %v", err)
	}
	if err := os.WriteFile(nonModelPath, []byte("plain text"), 0644); err != nil {
		t.Fatalf("failed to write dummy file: %v", err)
	}

	// Discover models
	models, err := DiscoverModels(tempDir)
	if err != nil {
		t.Fatalf("DiscoverModels failed: %v", err)
	}

	// Should discover exactly 2 models
	if len(models) != 2 {
		t.Errorf("expected 2 discovered models, got %d", len(models))
	}

	// Check model contents
	foundGemma := false
	foundQwen := false
	for _, m := range models {
		if m.Name == "Gemma 2B" {
			foundGemma = true
			if m.Architecture != "gemma" {
				t.Errorf("expected gemma arch, got %q", m.Architecture)
			}
			if m.ContextLength != 2048 {
				t.Errorf("expected 2048 context length, got %d", m.ContextLength)
			}
		} else if m.Name == "Qwen Coder 7B" {
			foundQwen = true
			if m.Architecture != "qwen2" {
				t.Errorf("expected qwen2 arch, got %q", m.Architecture)
			}
			if m.ContextLength != 32768 {
				t.Errorf("expected 32768 context length, got %d", m.ContextLength)
			}
		}
	}

	if !foundGemma {
		t.Errorf("Gemma 2B was not discovered")
	}
	if !foundQwen {
		t.Errorf("Qwen Coder 7B was not discovered")
	}
}

func TestDiscoverModelsCache(t *testing.T) {
	hasCache := false
	cacheFile := filepath.Join("cache", "metadata_cache.json")
	
	// Create cache dir if not exists so we can move cache there safely
	_ = os.MkdirAll("cache", 0755)
	
	if _, err := os.Stat(cacheFile); err == nil {
		hasCache = true
		_ = os.Rename(cacheFile, cacheFile+".tmp")
	}
	defer func() {
		_ = os.Remove(cacheFile)
		if hasCache {
			_ = os.Rename(cacheFile+".tmp", cacheFile)
		}
	}()

	tempDir, err := os.MkdirTemp("", "llama-models-cache-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	modelPath := filepath.Join(tempDir, "model.gguf")
	if err := createMockGGUFAt(modelPath, "Test Model", "llama", 2048); err != nil {
		t.Fatalf("failed to create mock GGUF: %v", err)
	}

	// First scan (should populate cache)
	models, err := DiscoverModels(tempDir)
	if err != nil {
		t.Fatalf("first scan failed: %v", err)
	}
	if len(models) != 1 || models[0].Name != "Test Model" {
		t.Errorf("expected 1 discovered model")
	}

	// Get original file info
	stat, err := os.Stat(modelPath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	origSize := stat.Size()
	origModTime := stat.ModTime()

	// Corrupt mock file on disk (so if cache fails, parsing it will fail)
	// We write garbage of the exact same size and restore the original modtime.
	garbage := make([]byte, origSize)
	copy(garbage, []byte("garbage content"))
	if err := os.WriteFile(modelPath, garbage, 0644); err != nil {
		t.Fatalf("failed to write garbage: %v", err)
	}
	if err := os.Chtimes(modelPath, origModTime, origModTime); err != nil {
		t.Fatalf("failed to restore modtime: %v", err)
	}


	// Second scan (should read from cache)
	models2, err := DiscoverModels(tempDir)
	if err != nil {
		t.Fatalf("second scan failed: %v", err)
	}
	if len(models2) != 1 || models2[0].Name != "Test Model" {
		t.Errorf("expected cache hit to return Test Model metadata despite corrupted file, got %v", models2)
	}

	// Change mod time on file to invalidate cache
	futureTime := time.Now().Add(1 * time.Hour)
	_ = os.Chtimes(modelPath, futureTime, futureTime)

	// Third scan (should miss cache and attempt re-parse, which will fail since file has garbage content)
	models3, err := DiscoverModels(tempDir)
	if err != nil {
		t.Fatalf("third scan failed: %v", err)
	}
	if len(models3) != 0 {
		t.Errorf("expected cache miss to fail parsing and return 0 models, got %d", len(models3))
	}
}

func TestDiscoverMultiFileGGUFShards(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llama-models-shards-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	shard1 := filepath.Join(tempDir, "qwen-72b-00001-of-00003.gguf")
	shard2 := filepath.Join(tempDir, "qwen-72b-00002-of-00003.gguf")
	shard3 := filepath.Join(tempDir, "qwen-72b-00003-of-00003.gguf")
	single := filepath.Join(tempDir, "single-model.gguf")

	// Shard 1 has GGUF metadata
	if err := createMockGGUFAt(shard1, "Qwen 72B", "qwen2", 32768); err != nil {
		t.Fatalf("failed to create shard 1: %v", err)
	}
	// Shards 2 and 3 have dummy bytes
	_ = os.WriteFile(shard2, bytes.Repeat([]byte("B"), 200), 0644)
	_ = os.WriteFile(shard3, bytes.Repeat([]byte("C"), 300), 0644)

	// Single model
	if err := createMockGGUFAt(single, "Single Model", "llama", 4096); err != nil {
		t.Fatalf("failed to create single model: %v", err)
	}

	shard1Info, _ := os.Stat(shard1)
	expectedTotalSize := shard1Info.Size() + 200 + 300

	models, err := DiscoverModels(tempDir)
	if err != nil {
		t.Fatalf("DiscoverModels failed: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 logical models, got %d", len(models))
	}

	var shardedModel *GGUFMetadata
	var singleModel *GGUFMetadata
	for _, m := range models {
		if m.ShardCount > 1 {
			shardedModel = m
		} else {
			singleModel = m
		}
	}

	if shardedModel == nil {
		t.Fatalf("expected sharded model to be discovered")
	}
	if shardedModel.ShardCount != 3 {
		t.Errorf("expected ShardCount 3, got %d", shardedModel.ShardCount)
	}
	if len(shardedModel.ShardFiles) != 3 {
		t.Errorf("expected 3 ShardFiles, got %d", len(shardedModel.ShardFiles))
	}
	if shardedModel.FileSize != expectedTotalSize {
		t.Errorf("expected aggregate FileSize %d, got %d", expectedTotalSize, shardedModel.FileSize)
	}
	if shardedModel.FilePath != shard1 {
		t.Errorf("expected FilePath to point to primary shard %s, got %s", shard1, shardedModel.FilePath)
	}

	if singleModel == nil {
		t.Fatalf("expected single model to be discovered")
	}
	if singleModel.ShardCount != 1 {
		t.Errorf("expected single model ShardCount 1, got %d", singleModel.ShardCount)
	}
}

func TestDiscoverMultiDirectory(t *testing.T) {
	tempDir1, err := os.MkdirTemp("", "llama-models-dir1")
	if err != nil {
		t.Fatalf("failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tempDir1)

	tempDir2, err := os.MkdirTemp("", "llama-models-dir2")
	if err != nil {
		t.Fatalf("failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tempDir2)

	m1 := filepath.Join(tempDir1, "model1.gguf")
	m2 := filepath.Join(tempDir2, "model2.gguf")

	_ = createMockGGUFAt(m1, "Model 1", "llama", 2048)
	_ = createMockGGUFAt(m2, "Model 2", "qwen2", 4096)

	models, err := DiscoverModels(tempDir1, tempDir2)
	if err != nil {
		t.Fatalf("DiscoverModels with multiple directories failed: %v", err)
	}

	if len(models) != 2 {
		t.Errorf("expected 2 discovered models from 2 directories, got %d", len(models))
	}
}

