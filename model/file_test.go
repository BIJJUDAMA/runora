package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "replace-file-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	srcFile := filepath.Join(tempDir, "source.txt")
	dstFile := filepath.Join(tempDir, "dest.txt")

	if err := os.WriteFile(srcFile, []byte("initial source content"), 0644); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	// 1. Destination does not exist initially
	if err := ReplaceFile(srcFile, dstFile); err != nil {
		t.Fatalf("ReplaceFile failed when dst does not exist: %v", err)
	}

	data, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("failed to read dst file: %v", err)
	}
	if string(data) != "initial source content" {
		t.Errorf("expected 'initial source content', got %q", string(data))
	}

	// 2. Destination exists, overwrite with new source
	srcFile2 := filepath.Join(tempDir, "source2.txt")
	if err := os.WriteFile(srcFile2, []byte("updated content"), 0644); err != nil {
		t.Fatalf("failed to write source2 file: %v", err)
	}

	if err := ReplaceFile(srcFile2, dstFile); err != nil {
		t.Fatalf("ReplaceFile failed when overwriting existing dst: %v", err)
	}

	data2, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("failed to read dst file after overwrite: %v", err)
	}
	if string(data2) != "updated content" {
		t.Errorf("expected 'updated content', got %q", string(data2))
	}
}
