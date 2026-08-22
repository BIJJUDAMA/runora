package runner

import (
	"archive/zip"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BIJJUDAMA/runora/hardware"
)

func TestResolveLlamaCppStableRelease(t *testing.T) {
	rel, err := fetchLatestRelease("https://api.github.com/repos/ggerganov/llama.cpp/releases/latest")
	if err != nil {
		t.Fatalf("fetchLatestRelease failed: %v", err)
	}
	t.Logf("Latest release: %s", rel.TagName)

	// Check if nightly-tag.txt exists
	var nightlyTagURL string
	for _, a := range rel.Assets {
		if a.Name == "nightly-tag.txt" {
			nightlyTagURL = a.BrowserDownloadURL
			break
		}
	}

	if nightlyTagURL != "" {
		resp, err := http.Get(nightlyTagURL)
		if err != nil {
			t.Fatalf("failed to fetch nightly-tag: %v", err)
		}
		defer resp.Body.Close()
		buf := make([]byte, 64)
		n, _ := resp.Body.Read(buf)
		nightlyTag := strings.TrimSpace(string(buf[:n]))
		t.Logf("Found nightly tag: %s", nightlyTag)

		tagRel, err := fetchLatestRelease("https://api.github.com/repos/ggerganov/llama.cpp/releases/tags/" + nightlyTag)
		if err != nil {
			t.Fatalf("failed to fetch tag release %s: %v", nightlyTag, err)
		}
		t.Logf("Tag release %s has %d assets", tagRel.TagName, len(tagRel.Assets))

		specsWinCUDA := &hardware.HardwareSpecs{
			OS: "Windows",
			GPU: hardware.GPUSpecs{
				Type:        "CUDA",
				CudaVersion: "13",
			},
		}
		mainAsset, cudartAsset, err := MatchAsset(tagRel, specsWinCUDA)
		if err != nil {
			t.Fatalf("MatchAsset failed: %v", err)
		}
		t.Logf("Matched main: %s, Cudart: %s", mainAsset.Name, cudartAsset.Name)
	}
}

func TestMatchAsset(t *testing.T) {
	release := &GithubRelease{
		TagName: "b3310",
		Name:    "llama.cpp b3310",
		Assets: []ReleaseAsset{
			{Name: "llama-b3310-bin-win-cu12.2.0-x64.zip", BrowserDownloadURL: "http://win-cuda-12.zip", Size: 100},
			{Name: "llama-b3310-bin-win-cu13.0.0-x64.zip", BrowserDownloadURL: "http://win-cuda-13.zip", Size: 100},
			{Name: "cudart-llama-bin-win-cuda-12.4-x64.zip", BrowserDownloadURL: "http://win-cudart-12.zip", Size: 50},
			{Name: "cudart-llama-bin-win-cuda-13.3-x64.zip", BrowserDownloadURL: "http://win-cudart-13.zip", Size: 50},
			{Name: "llama-b3310-bin-win-llvm-x64.zip", BrowserDownloadURL: "http://win-llvm.zip", Size: 80},
			{Name: "llama-b3310-bin-ubuntu-x64.zip", BrowserDownloadURL: "http://linux.zip", Size: 70},
			{Name: "llama-b3310-bin-macos-arm64.zip", BrowserDownloadURL: "http://mac-arm64.zip", Size: 60},
			{Name: "unrelated.txt", BrowserDownloadURL: "http://unrelated.txt", Size: 10},
		},
	}

	// 1. Windows with CUDA 12 GPU
	specsWinCUDA12 := &hardware.HardwareSpecs{
		OS: "Windows",
		GPU: hardware.GPUSpecs{
			Type:        "CUDA",
			CudaVersion: "12",
		},
	}
	asset, cudart, err := MatchAsset(release, specsWinCUDA12)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(asset.Name, "win-cu12.2.0") {
		t.Errorf("expected win-cu12.2.0 asset, got %s", asset.Name)
	}
	if cudart == nil || !strings.Contains(cudart.Name, "cudart-llama-bin-win-cuda-12.4-x64") {
		t.Errorf("expected cudart asset matching CUDA 12 DLLs, got %+v", cudart)
	}

	// 2. Windows with CUDA 13 GPU
	specsWinCUDA13 := &hardware.HardwareSpecs{
		OS: "Windows",
		GPU: hardware.GPUSpecs{
			Type:        "CUDA",
			CudaVersion: "13",
		},
	}
	asset, cudart, err = MatchAsset(release, specsWinCUDA13)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(asset.Name, "win-cu13.0.0") {
		t.Errorf("expected win-cu13.0.0 asset, got %s", asset.Name)
	}
	if cudart == nil || !strings.Contains(cudart.Name, "cudart-llama-bin-win-cuda-13.3-x64") {
		t.Errorf("expected cudart asset matching CUDA 13 DLLs, got %+v", cudart)
	}

	// 3. Windows with CPU only
	specsWinCPU := &hardware.HardwareSpecs{
		OS: "Windows",
		GPU: hardware.GPUSpecs{
			Type: "CPU",
		},
	}
	asset, cudart, err = MatchAsset(release, specsWinCPU)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(asset.Name, "win-llvm") {
		t.Errorf("expected win-llvm asset, got %s", asset.Name)
	}
	if cudart != nil {
		t.Errorf("expected no cudart asset for CPU, got %s", cudart.Name)
	}

	// 4. macOS
	specsMac := &hardware.HardwareSpecs{
		OS: "darwin",
		GPU: hardware.GPUSpecs{
			Type: "Metal",
		},
	}
	asset, cudart, err = MatchAsset(release, specsMac)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(asset.Name, "macos-arm64") {
		t.Errorf("expected macos-arm64 asset, got %s", asset.Name)
	}
	if cudart != nil {
		t.Errorf("expected no cudart asset for Metal, got %s", cudart.Name)
	}
}

func TestMatchOnnxAsset(t *testing.T) {
	release := &GithubRelease{
		TagName: "v1.27.0",
		Name:    "onnxruntime v1.27.0",
		Assets: []ReleaseAsset{
			{Name: "onnxruntime-win-x64-1.27.0.zip", BrowserDownloadURL: "http://win-cpu.zip", Size: 100},
			{Name: "onnxruntime-win-x64-gpu_cuda12-1.27.0.zip", BrowserDownloadURL: "http://win-cuda12.zip", Size: 100},
			{Name: "onnxruntime-win-x64-gpu_cuda13-1.27.0.zip", BrowserDownloadURL: "http://win-cuda13.zip", Size: 100},
			{Name: "onnxruntime-linux-x64-1.27.0.tgz", BrowserDownloadURL: "http://linux-cpu.tgz", Size: 100},
			{Name: "onnxruntime-linux-x64-gpu_cuda12-1.27.0.tgz", BrowserDownloadURL: "http://linux-cuda12.tgz", Size: 100},
			{Name: "onnxruntime-linux-x64-gpu_cuda13-1.27.0.tgz", BrowserDownloadURL: "http://linux-cuda13.tgz", Size: 100},
			{Name: "onnxruntime-osx-arm64-1.27.0.tgz", BrowserDownloadURL: "http://mac-arm64.tgz", Size: 100},
		},
	}

	// Windows CPU
	specsWinCPU := &hardware.HardwareSpecs{
		OS: "Windows",
		GPU: hardware.GPUSpecs{
			Type: "CPU",
		},
	}
	asset, err := MatchOnnxAsset(release, specsWinCPU)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(asset.Name, "win-x64-1.27.0.zip") {
		t.Errorf("expected win-cpu, got %s", asset.Name)
	}

	// Windows CUDA 12
	specsWinCUDA12 := &hardware.HardwareSpecs{
		OS: "Windows",
		GPU: hardware.GPUSpecs{
			Type:        "CUDA",
			CudaVersion: "12",
		},
	}
	asset, err = MatchOnnxAsset(release, specsWinCUDA12)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(asset.Name, "gpu_cuda12") {
		t.Errorf("expected win gpu_cuda12, got %s", asset.Name)
	}

	// Windows CUDA 13
	specsWinCUDA13 := &hardware.HardwareSpecs{
		OS: "Windows",
		GPU: hardware.GPUSpecs{
			Type:        "CUDA",
			CudaVersion: "13",
		},
	}
	asset, err = MatchOnnxAsset(release, specsWinCUDA13)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(asset.Name, "gpu_cuda13") {
		t.Errorf("expected win gpu_cuda13, got %s", asset.Name)
	}

	// Linux CUDA 13
	specsLinuxCUDA13 := &hardware.HardwareSpecs{
		OS: "linux",
		GPU: hardware.GPUSpecs{
			Type:        "CUDA",
			CudaVersion: "13",
		},
	}
	asset, err = MatchOnnxAsset(release, specsLinuxCUDA13)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(asset.Name, "linux-x64-gpu_cuda13") {
		t.Errorf("expected linux gpu_cuda13, got %s", asset.Name)
	}

	// macOS
	specsMac := &hardware.HardwareSpecs{
		OS: "darwin",
		GPU: hardware.GPUSpecs{
			Type: "Metal",
		},
	}
	asset, err = MatchOnnxAsset(release, specsMac)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(asset.Name, "osx-arm64") {
		t.Errorf("expected osx-arm64, got %s", asset.Name)
	}
}

func TestExtractOnnxLibraryZip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "onnx-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	zipPath := filepath.Join(tempDir, "onnx.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}

	zw := zip.NewWriter(zipFile)
	w, err := zw.Create("onnxruntime-win-x64-1.27.0/lib/onnxruntime.dll")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	_, _ = w.Write([]byte("mock-dll-content"))

	// Add an unrelated file that should not be extracted
	wUnrelated, err := zw.Create("onnxruntime-win-x64-1.27.0/include/onnxruntime_c_api.h")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	_, _ = wUnrelated.Write([]byte("mock-header-content"))

	_ = zw.Close()
	_ = zipFile.Close()

	destDir := filepath.Join(tempDir, "extracted")
	err = ExtractOnnxLibrary(zipPath, destDir)
	if err != nil {
		t.Fatalf("ExtractOnnxLibrary failed: %v", err)
	}

	// Verify onnxruntime.dll exists at the root of destDir
	dllPath := filepath.Join(destDir, "onnxruntime.dll")
	if _, err := os.Stat(dllPath); os.IsNotExist(err) {
		t.Errorf("expected onnxruntime.dll to be extracted, not found")
	}

	// Verify header does not exist
	headerPath := filepath.Join(destDir, "onnxruntime_c_api.h")
	if _, err := os.Stat(headerPath); !os.IsNotExist(err) {
		t.Errorf("expected onnxruntime_c_api.h not to be extracted")
	}
}

func TestExtractOnnxLibraryArchiveFormatSniffing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "onnx-archive-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create file with .archive extension (which caused the user's error previously)
	archivePath := filepath.Join(tempDir, "onnxruntime-v1.27.0.archive")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create archive file: %v", err)
	}

	zw := zip.NewWriter(archiveFile)
	w, err := zw.Create("onnxruntime-win-x64-1.27.0/lib/onnxruntime.dll")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	_, _ = w.Write([]byte("mock-dll-content"))
	_ = zw.Close()
	_ = archiveFile.Close()

	destDir := filepath.Join(tempDir, "extracted")
	err = ExtractOnnxLibrary(archivePath, destDir)
	if err != nil {
		t.Fatalf("ExtractOnnxLibrary failed on .archive extension with magic bytes: %v", err)
	}

	dllPath := filepath.Join(destDir, "onnxruntime.dll")
	if _, err := os.Stat(dllPath); os.IsNotExist(err) {
		t.Errorf("expected onnxruntime.dll to be extracted from .archive, not found")
	}
}

func TestExtractAndFlattenZip(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llama-manager-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	zipPath := filepath.Join(tempDir, "test.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}

	zw := zip.NewWriter(zipFile)

	// Create files inside a single root directory in zip
	subFolder := "llama-b3310-bin-win-llvm-x64/"
	filesToCreate := map[string]string{
		subFolder + "llama-server.exe": "server-binary-content",
		subFolder + "llama-cli.exe":    "cli-binary-content",
		subFolder + "README.md":        "readme-content",
	}

	for name, content := range filesToCreate {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry: %v", err)
		}
		_, _ = w.Write([]byte(content))
	}
	_ = zw.Close()
	_ = zipFile.Close()

	// Extract
	destDir := filepath.Join(tempDir, "extracted")
	err = ExtractArchive(zipPath, destDir)
	if err != nil {
		t.Fatalf("ExtractArchive failed: %v", err)
	}

	// Verify that files are flattened directly under destDir
	expectedFiles := []string{"llama-server.exe", "llama-cli.exe", "README.md"}
	for _, f := range expectedFiles {
		path := filepath.Join(destDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to be extracted and flattened, but not found", f)
		}
	}

	// Check content of one of the files
	contentBytes, err := os.ReadFile(filepath.Join(destDir, "llama-server.exe"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(contentBytes) != "server-binary-content" {
		t.Errorf("incorrect file content: got %q, expected %q", string(contentBytes), "server-binary-content")
	}
}

func TestBackupAndRollback(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "llama-manager-backup-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	srcDir := filepath.Join(tempDir, "llama.cpp")
	_ = os.MkdirAll(srcDir, 0755)
	_ = os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("original-content"), 0644)

	backupDir := filepath.Join(tempDir, "llama.cpp.backup")

	// Test Backup
	err = CreateBackup(srcDir, backupDir)
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	// Verify backup files exist
	if _, err := os.Stat(filepath.Join(backupDir, "test.txt")); os.IsNotExist(err) {
		t.Errorf("backup did not copy test.txt")
	}

	// Test Rollback
	// Modify src first
	_ = os.MkdirAll(srcDir, 0755)
	_ = os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("modified-content"), 0644)

	err = RollbackBackup(backupDir, srcDir)
	if err != nil {
		t.Fatalf("RollbackBackup failed: %v", err)
	}

	// Verify rollback content is original
	content, err := os.ReadFile(filepath.Join(srcDir, "test.txt"))
	if err != nil {
		t.Fatalf("failed to read text file: %v", err)
	}
	if string(content) != "original-content" {
		t.Errorf("expected rolled back content %q, got %q", "original-content", string(content))
	}
}

func TestQueryLocalVersionMissing(t *testing.T) {
	_, _, _, err := QueryLocalVersion("missing-directory-path")
	if err == nil {
		t.Errorf("expected error for missing directory path, got nil")
	}
}

// Helper functions for copying folders under tests
// (Already defined at package level in updater.go)
