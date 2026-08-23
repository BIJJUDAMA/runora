package model

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"testing"
)

func writeGGUFString(w io.Writer, s string) {
	_ = binary.Write(w, binary.LittleEndian, uint64(len(s)))
	_, _ = w.Write([]byte(s))
}

func TestParseGGUF(t *testing.T) {
	// Create a temporary file to write mock GGUF content
	tempFile, err := os.CreateTemp("", "mock-gguf-*.gguf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Build GGUF mock data
	var buf bytes.Buffer

	// 1. Magic
	_, _ = buf.Write([]byte("GGUF"))

	// 2. Version (uint32)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(3))

	// 3. Tensor count (uint64)
	_ = binary.Write(&buf, binary.LittleEndian, uint64(0))

	// 4. Metadata KV count (uint64)
	_ = binary.Write(&buf, binary.LittleEndian, uint64(8))

	// KV 1: general.name (string)
	writeGGUFString(&buf, "general.name")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeString))
	writeGGUFString(&buf, "Mock Model 123")

	// KV 2: general.architecture (string)
	writeGGUFString(&buf, "general.architecture")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeString))
	writeGGUFString(&buf, "llama")

	// KV 3: llama.context_length (uint32)
	writeGGUFString(&buf, "llama.context_length")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeUInt32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(4096))

	// KV 4: general.file_type (uint32)
	writeGGUFString(&buf, "general.file_type")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeUInt32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(2)) // Q4_0

	// KV 5: llama.block_count (uint32)
	writeGGUFString(&buf, "llama.block_count")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeUInt32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(32))

	// KV 6: llama.attention.head_count (uint32)
	writeGGUFString(&buf, "llama.attention.head_count")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeUInt32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(32))

	// KV 7: llama.attention.head_count_kv (uint32)
	writeGGUFString(&buf, "llama.attention.head_count_kv")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeUInt32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(8))

	// KV 8: llama.embedding_length (uint32)
	writeGGUFString(&buf, "llama.embedding_length")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeUInt32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(4096))

	// Write to temp file
	_, err = tempFile.Write(buf.Bytes())
	if err != nil {
		t.Fatalf("failed to write mock GGUF data: %v", err)
	}
	_ = tempFile.Close()

	// Parse GGUF file
	meta, err := ParseGGUF(tempFile.Name())
	if err != nil {
		t.Fatalf("failed to parse GGUF: %v", err)
	}

	if meta.Name != "Mock Model 123" {
		t.Errorf("expected name to be 'Mock Model 123', got %q", meta.Name)
	}
	if meta.Architecture != "llama" {
		t.Errorf("expected architecture to be 'llama', got %q", meta.Architecture)
	}
	if meta.ContextLength != 4096 {
		t.Errorf("expected context length to be 4096, got %d", meta.ContextLength)
	}
	if meta.Quantization != "Q4_0" {
		t.Errorf("expected quantization to be 'Q4_0', got %q", meta.Quantization)
	}
	if meta.Layers != 32 {
		t.Errorf("expected layers to be 32, got %d", meta.Layers)
	}
	if meta.Heads != 32 {
		t.Errorf("expected heads to be 32, got %d", meta.Heads)
	}
	if meta.HeadsKV != 8 {
		t.Errorf("expected headsKV to be 8, got %d", meta.HeadsKV)
	}
	if meta.EmbeddingLen != 4096 {
		t.Errorf("expected embedding length to be 4096, got %d", meta.EmbeddingLen)
	}
}

func TestParseGGUFWithArray(t *testing.T) {
	tempFile, err := os.CreateTemp("", "mock-gguf-array-*.gguf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	var buf bytes.Buffer
	_, _ = buf.Write([]byte("GGUF"))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(3)) // Version 3
	_ = binary.Write(&buf, binary.LittleEndian, uint64(0)) // Tensor count
	_ = binary.Write(&buf, binary.LittleEndian, uint64(4)) // KV count

	// KV 1: general.architecture (string)
	writeGGUFString(&buf, "general.architecture")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeString))
	writeGGUFString(&buf, "gemma4")

	// KV 2: gemma4.block_count (uint32)
	writeGGUFString(&buf, "gemma4.block_count")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeUInt32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(48))

	// KV 3: gemma4.attention.head_count (uint32)
	writeGGUFString(&buf, "gemma4.attention.head_count")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeUInt32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))

	// KV 4: gemma4.attention.head_count_kv (array of TypeUInt32)
	writeGGUFString(&buf, "gemma4.attention.head_count_kv")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeArray))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeUInt32)) // elemType
	_ = binary.Write(&buf, binary.LittleEndian, uint64(48))         // lenVal (uint64 for version 3)

	// Write 48 values: 40 of 8s, 8 of 1s
	for i := 0; i < 48; i++ {
		val := uint32(8)
		if i%6 == 5 {
			val = uint32(1)
		}
		_ = binary.Write(&buf, binary.LittleEndian, val)
	}

	_, err = tempFile.Write(buf.Bytes())
	if err != nil {
		t.Fatalf("failed to write mock GGUF data: %v", err)
	}
	_ = tempFile.Close()

	meta, err := ParseGGUF(tempFile.Name())
	if err != nil {
		t.Fatalf("failed to parse GGUF: %v", err)
	}

	if meta.Architecture != "gemma4" {
		t.Errorf("expected architecture 'gemma4', got %q", meta.Architecture)
	}
	if meta.Layers != 48 {
		t.Errorf("expected layers 48, got %d", meta.Layers)
	}
	if meta.Heads != 16 {
		t.Errorf("expected heads 16, got %d", meta.Heads)
	}
	// Expected average of KV heads: (40*8 + 8*1) / 48 = 328/48 = 6.833 -> rounded to 7
	if meta.HeadsKV != 7 {
		t.Errorf("expected headsKV to be averaged to 7, got %d", meta.HeadsKV)
	}
}

func TestParseGGUF_ExceedsMaxKVCount(t *testing.T) {
	tempFile, err := os.CreateTemp("", "gguf-max-kv-*.gguf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	var buf bytes.Buffer
	_, _ = buf.Write([]byte("GGUF"))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(3))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(0))          // Tensor count
	_ = binary.Write(&buf, binary.LittleEndian, uint64(MaxKVCount+1)) // Exceeds MaxKVCount

	_, _ = tempFile.Write(buf.Bytes())
	_ = tempFile.Close()

	_, err = ParseGGUF(tempFile.Name())
	if err == nil {
		t.Fatalf("expected error when kvCount exceeds MaxKVCount, got nil")
	}
}

func TestParseGGUF_ExceedsMaxTensorCount(t *testing.T) {
	tempFile, err := os.CreateTemp("", "gguf-max-tensor-*.gguf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	var buf bytes.Buffer
	_, _ = buf.Write([]byte("GGUF"))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(3))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(MaxTensorCount+1)) // Exceeds MaxTensorCount
	_ = binary.Write(&buf, binary.LittleEndian, uint64(1))

	_, _ = tempFile.Write(buf.Bytes())
	_ = tempFile.Close()

	_, err = ParseGGUF(tempFile.Name())
	if err == nil {
		t.Fatalf("expected error when tensorCount exceeds MaxTensorCount, got nil")
	}
}

func TestParseGGUF_ExceedsMaxStringLength(t *testing.T) {
	tempFile, err := os.CreateTemp("", "gguf-max-str-*.gguf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	var buf bytes.Buffer
	_, _ = buf.Write([]byte("GGUF"))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(3))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(1))

	// Write oversized string length
	_ = binary.Write(&buf, binary.LittleEndian, uint64(MaxStringLength+1))

	_, _ = tempFile.Write(buf.Bytes())
	_ = tempFile.Close()

	_, err = ParseGGUF(tempFile.Name())
	if err == nil {
		t.Fatalf("expected error when string length exceeds MaxStringLength, got nil")
	}
}

func TestParseGGUF_ExceedsMaxArrayLength(t *testing.T) {
	tempFile, err := os.CreateTemp("", "gguf-max-arr-*.gguf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	var buf bytes.Buffer
	_, _ = buf.Write([]byte("GGUF"))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(3))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(1))

	// KV 1: array key
	writeGGUFString(&buf, "some.array")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeArray))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeUInt32))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(MaxArrayLength+1)) // Exceeds MaxArrayLength

	_, _ = tempFile.Write(buf.Bytes())
	_ = tempFile.Close()

	_, err = ParseGGUF(tempFile.Name())
	if err == nil {
		t.Fatalf("expected error when array length exceeds MaxArrayLength, got nil")
	}
}

func TestParseGGUF_ExceedsMaxRecursionDepth(t *testing.T) {
	tempFile, err := os.CreateTemp("", "gguf-max-depth-*.gguf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	var buf bytes.Buffer
	_, _ = buf.Write([]byte("GGUF"))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(3))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(1))

	// KV 1: deeply nested array (exceeding MaxRecursionDepth=4)
	writeGGUFString(&buf, "nested.array")
	// Depth 1
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeArray))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeArray))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(1))
	// Depth 2
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeArray))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(1))
	// Depth 3
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeArray))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(1))
	// Depth 4
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeArray))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(1))
	// Depth 5 (Exceeds MaxRecursionDepth)
	_ = binary.Write(&buf, binary.LittleEndian, uint32(TypeUInt32))
	_ = binary.Write(&buf, binary.LittleEndian, uint64(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(42))

	_, _ = tempFile.Write(buf.Bytes())
	_ = tempFile.Close()

	_, err = ParseGGUF(tempFile.Name())
	if err == nil {
		t.Fatalf("expected error when recursion depth exceeds MaxRecursionDepth, got nil")
	}
}

func TestParseGGUF_CorruptedMagicOrVersion(t *testing.T) {
	// Bad magic
	tempFile, err := os.CreateTemp("", "gguf-bad-magic-*.gguf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	_, _ = tempFile.Write([]byte("BADM\x03\x00\x00\x00"))
	_ = tempFile.Close()

	_, err = ParseGGUF(tempFile.Name())
	if err == nil {
		t.Fatalf("expected error for bad magic, got nil")
	}

	// Bad version
	tempFile2, err := os.CreateTemp("", "gguf-bad-version-*.gguf")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile2.Name())
	_, _ = tempFile2.Write([]byte("GGUF\x09\x00\x00\x00")) // Version 9 unsupported
	_ = tempFile2.Close()

	_, err = ParseGGUF(tempFile2.Name())
	if err == nil {
		t.Fatalf("expected error for unsupported version, got nil")
	}
}

