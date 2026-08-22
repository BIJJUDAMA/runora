package runner

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/BIJJUDAMA/runora/hardware"
)

type GithubRelease struct {
	TagName string         `json:"tag_name"`
	Name    string         `json:"name"`
	Body    string         `json:"body"`
	Assets  []ReleaseAsset `json:"assets"`
}

type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// QueryLocalVersion runs llama-server (or llama-cli) --version and parses output.
func QueryLocalVersion(llamaCppDir string) (version string, commit string, buildInfo string, err error) {
	binaryName := "llama-server"
	if runtime.GOOS == "windows" {
		binaryName = "llama-server.exe"
	}
	binaryPath := filepath.Join(llamaCppDir, binaryName)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return "Not Installed", "N/A", "N/A", fmt.Errorf("llama-server binary not found")
	}

	cmd := exec.Command(binaryPath, "--version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run() // run even if non-zero exit code, version commands sometimes fail but write version

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		output = strings.TrimSpace(stderr.String())
	}

	if output == "" {
		return "Unknown", "Unknown", "Unknown", fmt.Errorf("no output from version command")
	}

	// Typical output format:
	// version: 9707 (e1efd0991)
	// built with Clang 20.1.8 for Windows x86_64
	versionRegex := regexp.MustCompile(`version:\s*([^\s(]+)`)
	commitRegex := regexp.MustCompile(`\(([^)]+)\)`)

	version = "Unknown"
	commit = "Unknown"
	buildInfo = "Unknown"

	lines := strings.Split(output, "\n")
	if len(lines) > 0 {
		vMatch := versionRegex.FindStringSubmatch(lines[0])
		if len(vMatch) > 1 {
			version = vMatch[1]
		}
		cMatch := commitRegex.FindStringSubmatch(lines[0])
		if len(cMatch) > 1 {
			commit = cMatch[1]
		}
	}
	if len(lines) > 1 {
		buildInfo = strings.TrimSpace(lines[1])
	} else if len(lines) > 0 {
		buildInfo = strings.TrimSpace(lines[0])
	}

	return version, commit, buildInfo, nil
}

// CheckLatestRelease queries GitHub API for the latest llama.cpp release.
// Since llama.cpp publishes continuous builds (bXXXX) as releases, querying
// /releases?per_page=10 guarantees finding the latest build with release assets.
func CheckLatestRelease() (*GithubRelease, error) {
	releases, err := fetchReleasesList("https://api.github.com/repos/ggerganov/llama.cpp/releases?per_page=10")
	if err == nil && len(releases) > 0 {
		for _, r := range releases {
			if len(r.Assets) > 0 && strings.HasPrefix(strings.ToLower(r.TagName), "b") {
				return &r, nil
			}
		}
		// If no bXXXX tag found, pick the first release with assets
		for _, r := range releases {
			if len(r.Assets) > 0 {
				return &r, nil
			}
		}
		return &releases[0], nil
	}

	return fetchLatestRelease("https://api.github.com/repos/ggerganov/llama.cpp/releases/latest")
}

func fetchReleasesList(url string) ([]GithubRelease, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "runora-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned status %s", resp.Status)
	}

	var releases []GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	return releases, nil
}

// CheckAppRelease queries GitHub API for the latest runora release or tag.
func CheckAppRelease() (*GithubRelease, error) {
	rel, err := fetchLatestRelease("https://api.github.com/repos/BIJJUDAMA/runora/releases/latest")
	if err == nil {
		return rel, nil
	}

	// Fallback to checking tags if no formal GitHub Release exists
	tagRel, tagErr := fetchLatestTag("https://api.github.com/repos/BIJJUDAMA/runora/tags")
	if tagErr == nil {
		return tagRel, nil
	}

	return nil, fmt.Errorf("failed to check latest release: %w", err)
}

type GithubTag struct {
	Name string `json:"name"`
}

func fetchLatestTag(url string) (*GithubRelease, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "runora-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned status %s", resp.Status)
	}

	var tags []GithubTag
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("no tags found in repository")
	}

	return &GithubRelease{
		TagName: tags[0].Name,
		Name:    tags[0].Name,
	}, nil
}

func fetchLatestRelease(url string) (*GithubRelease, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "runora-updater")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned status %s", resp.Status)
	}

	var release GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}


// MatchAsset finds the most suitable assets (main binaries and optional cudart DLLs) for the user's OS, CPU/GPU architecture.
func MatchAsset(release *GithubRelease, specs *hardware.HardwareSpecs) (mainAsset *ReleaseAsset, cudartAsset *ReleaseAsset, err error) {
	if len(release.Assets) == 0 {
		return nil, nil, fmt.Errorf("no assets in release")
	}

	var bestAsset *ReleaseAsset
	bestScore := -1

	for _, asset := range release.Assets {
		nameLower := strings.ToLower(asset.Name)

		// 1. Extension check
		if !strings.HasSuffix(nameLower, ".zip") && !strings.HasSuffix(nameLower, ".tar.gz") && !strings.HasSuffix(nameLower, ".tgz") {
			continue
		}

		// Skip cudart assets when selecting the main llama binaries
		if strings.Contains(nameLower, "cudart") {
			continue
		}

		// 2. OS check
		osMatches := false
		switch strings.ToLower(specs.OS) {
		case "windows":
			if strings.Contains(nameLower, "win") || strings.Contains(nameLower, "windows") {
				osMatches = true
			}
		case "darwin", "macos":
			if strings.Contains(nameLower, "macos") || strings.Contains(nameLower, "osx") || strings.Contains(nameLower, "darwin") {
				osMatches = true
			}
		default: // assume linux
			if strings.Contains(nameLower, "linux") || strings.Contains(nameLower, "ubuntu") || strings.Contains(nameLower, "debian") {
				osMatches = true
			}
		}

		if !osMatches {
			continue
		}

		score := 100

		// 3. Architecture check
		if strings.ToLower(specs.OS) == "darwin" || strings.ToLower(specs.OS) == "macos" {
			if strings.Contains(nameLower, "arm64") {
				score += 50
			}
		} else {
			if strings.Contains(nameLower, "x64") || strings.Contains(nameLower, "x86_64") || strings.Contains(nameLower, "amd64") {
				score += 30
			}
		}

		// 4. GPU Backend check
		switch specs.GPU.Type {
		case "CUDA":
			if strings.Contains(nameLower, "cuda") || strings.Contains(nameLower, "cu") {
				score += 80
				cudaVer := specs.GPU.CudaVersion
				if cudaVer == "" {
					cudaVer = "12"
				}
				if strings.Contains(nameLower, "cu"+cudaVer) || strings.Contains(nameLower, "cuda-"+cudaVer) || strings.Contains(nameLower, "cuda"+cudaVer) {
					score += 50
				}
			} else if strings.Contains(nameLower, "llvm") || strings.Contains(nameLower, "cpu") {
				score += 10
			}
		case "ROCm":
			if strings.Contains(nameLower, "rocm") {
				score += 80
			} else if strings.Contains(nameLower, "llvm") || strings.Contains(nameLower, "cpu") {
				score += 10
			}
		case "Vulkan":
			if strings.Contains(nameLower, "vulkan") {
				score += 80
			} else if strings.Contains(nameLower, "llvm") || strings.Contains(nameLower, "cpu") {
				score += 10
			}
		default: // CPU
			if strings.Contains(nameLower, "llvm") || strings.Contains(nameLower, "cpu") {
				score += 80
			} else if strings.Contains(nameLower, "win-llvm") || strings.Contains(nameLower, "win64") {
				score += 50
			}
		}

		if score > bestScore {
			bestScore = score
			assetCopy := asset
			bestAsset = &assetCopy
		}
	}

	if bestAsset == nil {
		return nil, nil, fmt.Errorf("no matching asset found for OS %s and GPU type %s", specs.OS, specs.GPU.Type)
	}

	// 5. If Windows and CUDA, find the corresponding cudart DLLs asset
	if strings.ToLower(specs.OS) == "windows" && specs.GPU.Type == "CUDA" {
		bestCudartScore := -1
		for _, asset := range release.Assets {
			nameLower := strings.ToLower(asset.Name)
			if !strings.Contains(nameLower, "cudart") {
				continue
			}
			if !strings.HasSuffix(nameLower, ".zip") {
				continue
			}
			// Must match OS and arch
			if !strings.Contains(nameLower, "win") && !strings.Contains(nameLower, "windows") {
				continue
			}
			if !strings.Contains(nameLower, "x64") && !strings.Contains(nameLower, "x86_64") && !strings.Contains(nameLower, "amd64") {
				continue
			}

			score := 100
			cudaVer := specs.GPU.CudaVersion
			if cudaVer == "" {
				cudaVer = "12"
			}
			thisCudaVer := extractCudaVersion(asset.Name)
			if thisCudaVer != "" && strings.HasPrefix(thisCudaVer, cudaVer) {
				score += 50
			}

			if score > bestCudartScore {
				bestCudartScore = score
				assetCopy := asset
				cudartAsset = &assetCopy
			}
		}
	}

	return bestAsset, cudartAsset, nil
}

func extractCudaVersion(name string) string {
	nameLower := strings.ToLower(name)
	re := regexp.MustCompile(`(?:cuda-?|cu)(\d+(?:\.\d+)*)`)
	matches := re.FindStringSubmatch(nameLower)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// CheckLatestOnnxRelease queries GitHub API for the latest ONNX Runtime release.
func CheckLatestOnnxRelease() (*GithubRelease, error) {
	return fetchLatestRelease("https://api.github.com/repos/microsoft/onnxruntime/releases/latest")
}

// MatchOnnxAsset finds the most suitable ONNX Runtime release package.
func MatchOnnxAsset(release *GithubRelease, specs *hardware.HardwareSpecs) (*ReleaseAsset, error) {
	if len(release.Assets) == 0 {
		return nil, fmt.Errorf("no assets in release")
	}

	var bestAsset *ReleaseAsset
	bestScore := -1

	for _, asset := range release.Assets {
		nameLower := strings.ToLower(asset.Name)

		// 1. Extension check
		if !strings.HasSuffix(nameLower, ".zip") && !strings.HasSuffix(nameLower, ".tar.gz") && !strings.HasSuffix(nameLower, ".tgz") {
			continue
		}

		// 2. OS check
		osMatches := false
		switch strings.ToLower(specs.OS) {
		case "windows":
			if strings.Contains(nameLower, "win") || strings.Contains(nameLower, "windows") {
				osMatches = true
			}
		case "darwin", "macos":
			if strings.Contains(nameLower, "osx") || strings.Contains(nameLower, "mac") || strings.Contains(nameLower, "darwin") {
				osMatches = true
			}
		default: // assume linux
			if strings.Contains(nameLower, "linux") {
				osMatches = true
			}
		}

		if !osMatches {
			continue
		}

		score := 100

		// 3. Architecture check
		if strings.ToLower(specs.OS) == "darwin" || strings.ToLower(specs.OS) == "macos" {
			if strings.Contains(nameLower, "arm64") {
				score += 50
			}
		} else {
			if strings.Contains(nameLower, "x64") || strings.Contains(nameLower, "x86_64") || strings.Contains(nameLower, "amd64") {
				score += 30
			} else if strings.Contains(nameLower, "arm64") {
				if strings.Contains(strings.ToLower(runtime.GOARCH), "arm64") {
					score += 30
				}
			}
		}

		// 4. GPU Backend check
		if specs.GPU.Type == "CUDA" {
			cudaVer := specs.GPU.CudaVersion
			if cudaVer == "" {
				cudaVer = "12"
			}
			if strings.Contains(nameLower, "gpu_cuda"+cudaVer) {
				score += 100
			} else if strings.Contains(nameLower, "gpu_cuda") {
				score += 50
			} else if strings.Contains(nameLower, "gpu") {
				score += 30
			} else if !strings.Contains(nameLower, "gpu") && !strings.Contains(nameLower, "cuda") {
				score += 10
			}
		} else {
			if !strings.Contains(nameLower, "gpu") && !strings.Contains(nameLower, "cuda") && !strings.Contains(nameLower, "training") {
				score += 80
			}
		}

		if score > bestScore {
			bestScore = score
			assetCopy := asset
			bestAsset = &assetCopy
		}
	}

	if bestAsset == nil {
		return nil, fmt.Errorf("no matching ONNX asset found for OS %s and GPU type %s", specs.OS, specs.GPU.Type)
	}

	return bestAsset, nil
}

// QueryLocalOnnxVersion checks the presence of ONNX library files and reads the version.txt if available.
func QueryLocalOnnxVersion(onnxDir string) (string, error) {
	libName := "libonnxruntime.so"
	if runtime.GOOS == "windows" {
		libName = "onnxruntime.dll"
	} else if runtime.GOOS == "darwin" {
		libName = "libonnxruntime.dylib"
	}
	libPath := filepath.Join(onnxDir, libName)
	if _, err := os.Stat(libPath); os.IsNotExist(err) {
		return "Not Installed", fmt.Errorf("ONNX Runtime library not found")
	}

	versionPath := filepath.Join(onnxDir, "version.txt")
	if data, err := os.ReadFile(versionPath); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	return "Installed (Unknown Version)", nil
}

func detectArchiveType(archivePath string) string {
	ext := strings.ToLower(filepath.Ext(archivePath))
	if ext == ".zip" || ext == ".nupkg" || ext == ".jar" {
		return "zip"
	}
	if ext == ".tar.gz" || ext == ".tgz" || strings.HasSuffix(strings.ToLower(archivePath), ".tar.gz") {
		return "targz"
	}

	// Sniff magic bytes
	f, err := os.Open(archivePath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var header [4]byte
	n, err := io.ReadFull(f, header[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return ""
	}
	if n >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		return "targz"
	}
	if n >= 4 && header[0] == 'P' && header[1] == 'K' && (header[2] == 3 || header[2] == 5 || header[2] == 7) && (header[3] == 4 || header[3] == 6 || header[3] == 8) {
		return "zip"
	}

	return ""
}

// ExtractOnnxLibrary extracts only the ONNX Runtime library from zip/tar.gz files.
func ExtractOnnxLibrary(archivePath string, destDir string) error {
	format := detectArchiveType(archivePath)
	switch format {
	case "zip":
		return extractOnnxZip(archivePath, destDir)
	case "targz":
		return extractOnnxTarGz(archivePath, destDir)
	default:
		ext := filepath.Ext(archivePath)
		return fmt.Errorf("unsupported archive format: %s", ext)
	}
}

func matchesOnnxLibName(name string) bool {
	base := filepath.Base(name)
	baseLower := strings.ToLower(base)
	if runtime.GOOS == "windows" {
		return strings.HasPrefix(baseLower, "onnxruntime") && (strings.HasSuffix(baseLower, ".dll") || strings.HasSuffix(baseLower, ".lib"))
	}
	if runtime.GOOS == "darwin" {
		return strings.HasPrefix(baseLower, "libonnxruntime") && strings.HasSuffix(baseLower, ".dylib")
	}
	return strings.HasPrefix(baseLower, "libonnxruntime") && (strings.HasSuffix(baseLower, ".so") || strings.Contains(baseLower, ".so."))
}

func extractOnnxZip(archivePath string, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	found := false
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if matchesOnnxLibName(f.Name) {
			targetPath := filepath.Join(destDir, filepath.Base(f.Name))
			rc, err := f.Open()
			if err != nil {
				return err
			}
			out, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				rc.Close()
				return err
			}
			_, err = io.Copy(out, rc)
			out.Close()
			rc.Close()
			if err != nil {
				return err
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("could not find onnxruntime library in zip")
	}
	return nil
}

func extractOnnxTarGz(archivePath string, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	found := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if matchesOnnxLibName(header.Name) {
			targetPath := filepath.Join(destDir, filepath.Base(header.Name))
			out, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				return err
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("could not find onnxruntime library in tar.gz")
	}
	return nil
}

// DownloadAndInstallOnnxRuntime downloads the ONNX release asset, extracts the library files, and writes a version.txt.
func DownloadAndInstallOnnxRuntime(url string, destDir string, version string, downloadsDir string, progressChan chan float64) error {
	ext := ".zip"
	urlLower := strings.ToLower(url)
	if strings.HasSuffix(urlLower, ".tar.gz") || strings.HasSuffix(urlLower, ".tgz") {
		ext = ".tar.gz"
	} else if strings.HasSuffix(urlLower, ".nupkg") {
		ext = ".nupkg"
	} else if strings.HasSuffix(urlLower, ".zip") {
		ext = ".zip"
	}

	tempFile := filepath.Join(downloadsDir, fmt.Sprintf("onnxruntime-%s%s", version, ext))
	if err := os.MkdirAll(downloadsDir, 0755); err != nil {
		return err
	}

	err := DownloadRelease(url, tempFile, progressChan)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer os.Remove(tempFile)

	// Backup existing installation if present
	backupDir := destDir + ".backup"
	if _, statErr := os.Stat(destDir); statErr == nil {
		_ = CreateBackup(destDir, backupDir)
	}

	err = ExtractOnnxLibrary(tempFile, destDir)
	if err != nil {
		if _, statErr := os.Stat(backupDir); statErr == nil {
			_ = RollbackBackup(backupDir, destDir)
		}
		return fmt.Errorf("failed to extract library: %w", err)
	}

	versionPath := filepath.Join(destDir, "version.txt")
	err = os.WriteFile(versionPath, []byte(version), 0644)
	if err != nil {
		return fmt.Errorf("failed to write version.txt: %w", err)
	}

	return nil
}

// DownloadRelease downloads an asset URL and writes progress fraction (0.0 to 1.0) to progressChan.
func DownloadRelease(url string, destPath string, progressChan chan float64) error {
	defer func() {
		if progressChan != nil {
			close(progressChan)
		}
	}()

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "llama-manager-updater")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: server returned status %s", resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	totalSize := resp.ContentLength
	var downloaded int64

	buf := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			_, werr := out.Write(buf[:n])
			if werr != nil {
				return werr
			}
			downloaded += int64(n)
			if totalSize > 0 && progressChan != nil {
				progressChan <- float64(downloaded) / float64(totalSize)
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return rerr
		}
	}

	return nil
}

// ExtractArchive extracts zip/tar.gz into destDir and flattens single directories.
func ExtractArchive(archivePath string, destDir string) error {
	format := detectArchiveType(archivePath)
	var err error

	switch format {
	case "zip":
		err = extractZip(archivePath, destDir)
	case "targz":
		err = extractTarGz(archivePath, destDir)
	default:
		ext := filepath.Ext(archivePath)
		return fmt.Errorf("unsupported archive format: %s", ext)
	}

	if err != nil {
		return err
	}

	// Flatten root directory if single directory found
	return flattenIfSingleFolder(destDir)
}

func extractZip(archivePath string, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for _, f := range r.File {
		filePath := filepath.Join(destDir, filepath.FromSlash(f.Name))

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, f.Mode()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func extractTarGz(archivePath string, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		filePath := filepath.Join(destDir, filepath.FromSlash(header.Name))

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(filePath, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
				return err
			}

			out, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			_, err = io.Copy(out, tr)
			out.Close()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func flattenIfSingleFolder(destDir string) error {
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return err
	}

	// Filter out files/directories to find if there is a single directory
	var dirEntry os.DirEntry
	dirCount := 0
	fileCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			dirEntry = entry
			dirCount++
		} else {
			fileCount++
		}
	}

	if dirCount == 1 && fileCount == 0 {
		subDir := filepath.Join(destDir, dirEntry.Name())
		subEntries, err := os.ReadDir(subDir)
		if err != nil {
			return err
		}

		for _, se := range subEntries {
			srcPath := filepath.Join(subDir, se.Name())
			dstPath := filepath.Join(destDir, se.Name())
			if err := os.Rename(srcPath, dstPath); err != nil {
				// Fallback if rename fails
				if err := copyDirOrFile(srcPath, dstPath); err != nil {
					return err
				}
			}
		}
		_ = os.RemoveAll(subDir)
	}

	return nil
}

func copyDirOrFile(src string, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

// CreateBackup backs up the src folder to backupDst atomically.
func CreateBackup(src string, backupDst string) error {
	_ = os.RemoveAll(backupDst)

	if err := os.Rename(src, backupDst); err == nil {
		return nil
	}

	return copyDir(src, backupDst)
}

// RollbackBackup restores the backup directory.
func RollbackBackup(backupSrc string, dst string) error {
	if _, err := os.Stat(backupSrc); os.IsNotExist(err) {
		return fmt.Errorf("backup does not exist")
	}

	_ = os.RemoveAll(dst)

	if err := os.Rename(backupSrc, dst); err == nil {
		return nil
	}

	return copyDir(backupSrc, dst)
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		return copyFile(path, dstPath)
	})
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
