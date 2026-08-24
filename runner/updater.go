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
	"sync"
	"time"

	"github.com/BIJJUDAMA/runora/hardware"
)

type ReleaseChannel string

const (
	ChannelStable  ReleaseChannel = "stable"
	ChannelNightly ReleaseChannel = "nightly"
)

type BackendType string

const (
	BackendAuto   BackendType = "auto"
	BackendCUDA12 BackendType = "cuda12"
	BackendCUDA13 BackendType = "cuda13"
	BackendVulkan BackendType = "vulkan"
	BackendCPU    BackendType = "cpu"
	BackendROCm   BackendType = "rocm"
	BackendMetal  BackendType = "metal"
)

type GithubRelease struct {
	TagName    string         `json:"tag_name"`
	NightlyTag string         `json:"nightly_tag,omitempty"`
	Name       string         `json:"name"`
	Body       string         `json:"body"`
	Assets     []ReleaseAsset `json:"assets"`
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

// CheckReleaseForChannel queries GitHub API for the latest llama.cpp release on the specified channel (Stable or Nightly).
func CheckReleaseForChannel(channel ReleaseChannel) (*GithubRelease, error) {
	if channel == ChannelNightly {
		releases, err := fetchReleasesList("https://api.github.com/repos/ggerganov/llama.cpp/releases?per_page=15")
		if err == nil && len(releases) > 0 {
			// Find the most recent release with binary assets (preferring nightly 'b...' build tags)
			for _, r := range releases {
				hasBin := false
				for _, a := range r.Assets {
					nameLower := strings.ToLower(a.Name)
					if strings.HasSuffix(nameLower, ".zip") || strings.HasSuffix(nameLower, ".tar.gz") || strings.HasSuffix(nameLower, ".tgz") {
						if !strings.Contains(nameLower, "source") {
							hasBin = true
							break
						}
					}
				}
				if hasBin && strings.HasPrefix(strings.ToLower(r.TagName), "b") {
					relCopy := r
					return &relCopy, nil
				}
			}
			// If no 'b' prefix found, return the first with binaries
			for _, r := range releases {
				if len(r.Assets) > 0 {
					relCopy := r
					return &relCopy, nil
				}
			}
			return &releases[0], nil
		}
		if err != nil {
			return nil, fmt.Errorf("failed to check nightly releases: %w", err)
		}
	}

	// ChannelStable (default)
	rel, err := fetchLatestRelease("https://api.github.com/repos/ggerganov/llama.cpp/releases/latest")
	if err == nil {
		hasBinaries := false
		var nightlyTagURL string
		for _, a := range rel.Assets {
			nameLower := strings.ToLower(a.Name)
			if strings.HasSuffix(nameLower, ".zip") || strings.HasSuffix(nameLower, ".tar.gz") || strings.HasSuffix(nameLower, ".tgz") {
				if !strings.Contains(nameLower, "source") {
					hasBinaries = true
				}
			}
			if a.Name == "nightly-tag.txt" {
				nightlyTagURL = a.BrowserDownloadURL
			}
		}

		if hasBinaries {
			return rel, nil
		}

		// If this is a stable semantic release referencing a nightly build via nightly-tag.txt
		if nightlyTagURL != "" {
			nightlyTag, tagErr := fetchTextFile(nightlyTagURL)
			if tagErr == nil && nightlyTag != "" {
				tagRel, tagRelErr := fetchLatestRelease("https://api.github.com/repos/ggerganov/llama.cpp/releases/tags/" + nightlyTag)
				if tagRelErr == nil && len(tagRel.Assets) > 0 {
					tagRel.NightlyTag = nightlyTag
					tagRel.TagName = rel.TagName
					tagRel.Name = fmt.Sprintf("%s (%s)", rel.TagName, nightlyTag)
					return tagRel, nil
				}
			}
		}
	}

	// Fallback to releases list if latest release could not be resolved
	releases, listErr := fetchReleasesList("https://api.github.com/repos/ggerganov/llama.cpp/releases?per_page=10")
	if listErr == nil && len(releases) > 0 {
		for _, r := range releases {
			if len(r.Assets) > 0 {
				return &r, nil
			}
		}
		return &releases[0], nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to check latest release: %w", err)
	}
	return rel, nil
}

// CheckLatestRelease queries GitHub API for the latest llama.cpp release.
func CheckLatestRelease() (*GithubRelease, error) {
	return CheckReleaseForChannel(ChannelStable)
}

var (
	customGitHubTokenMu sync.RWMutex
	customGitHubToken   string
)

// SetGitHubToken sets a user-configured GitHub personal access token to authenticate release API requests.
func SetGitHubToken(token string) {
	customGitHubTokenMu.Lock()
	defer customGitHubTokenMu.Unlock()
	customGitHubToken = strings.TrimSpace(token)
}

func applyAuthHeader(req *http.Request) {
	req.Header.Set("User-Agent", "runora-updater")
	customGitHubTokenMu.RLock()
	token := customGitHubToken
	customGitHubTokenMu.RUnlock()

	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func fetchTextFile(url string) (string, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	applyAuthHeader(req)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch text file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("GitHub rate limit exceeded (403). Set GITHUB_TOKEN or wait a moment.")
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github returned status %s", resp.Status)
	}

	buf := make([]byte, 128)
	n, _ := resp.Body.Read(buf)
	return strings.TrimSpace(string(buf[:n])), nil
}

func fetchReleasesList(url string) ([]GithubRelease, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	applyAuthHeader(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("GitHub API rate limit exceeded (403 Forbidden). Set GITHUB_TOKEN or wait a few minutes.")
	}

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
	applyAuthHeader(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("GitHub API rate limit exceeded (403 Forbidden). Set GITHUB_TOKEN or wait a few minutes.")
	}

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
	applyAuthHeader(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("GitHub API rate limit exceeded (403 Forbidden). Set GITHUB_TOKEN or wait a few minutes.")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned status %s", resp.Status)
	}

	var release GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}


// MatchAsset finds the most suitable assets (main binaries and optional cudart DLLs) for the user's OS, CPU/GPU architecture using Auto backend detection.
func MatchAsset(release *GithubRelease, specs *hardware.HardwareSpecs) (mainAsset *ReleaseAsset, cudartAsset *ReleaseAsset, err error) {
	return MatchAssetWithBackend(release, specs, BackendAuto)
}

// MatchAssetWithBackend finds the most suitable assets for the specified backend accelerator and hardware specs.
func MatchAssetWithBackend(release *GithubRelease, specs *hardware.HardwareSpecs, backend BackendType) (mainAsset *ReleaseAsset, cudartAsset *ReleaseAsset, err error) {
	if len(release.Assets) == 0 {
		return nil, nil, fmt.Errorf("no assets in release")
	}

	effectiveBackend := backend
	if effectiveBackend == "" || effectiveBackend == BackendAuto {
		switch specs.GPU.Type {
		case "CUDA":
			if specs.GPU.CudaVersion == "13" {
				effectiveBackend = BackendCUDA13
			} else {
				effectiveBackend = BackendCUDA12
			}
		case "ROCm":
			effectiveBackend = BackendROCm
		case "Vulkan":
			effectiveBackend = BackendVulkan
		case "Metal":
			effectiveBackend = BackendMetal
		default:
			effectiveBackend = BackendCPU
		}
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
		isArm := runtime.GOARCH == "arm64" || (specs != nil && strings.Contains(strings.ToLower(specs.CPU.Model), "apple"))
		if isArm {
			if strings.Contains(nameLower, "arm64") || strings.Contains(nameLower, "aarch64") {
				score += 50
			}
		} else {
			if strings.Contains(nameLower, "x64") || strings.Contains(nameLower, "x86_64") || strings.Contains(nameLower, "amd64") || strings.Contains(nameLower, "win64") {
				score += 30
			}
		}

		// 4. GPU Backend check
		switch effectiveBackend {
		case BackendCUDA12:
			if strings.Contains(nameLower, "cuda") || strings.Contains(nameLower, "cu") {
				score += 80
				if strings.Contains(nameLower, "cu12") || strings.Contains(nameLower, "cuda-12") || strings.Contains(nameLower, "cuda12") {
					score += 50
				}
			} else if strings.Contains(nameLower, "llvm") || strings.Contains(nameLower, "cpu") {
				score += 10
			}
		case BackendCUDA13:
			if strings.Contains(nameLower, "cuda") || strings.Contains(nameLower, "cu") {
				score += 80
				if strings.Contains(nameLower, "cu13") || strings.Contains(nameLower, "cuda-13") || strings.Contains(nameLower, "cuda13") {
					score += 50
				}
			} else if strings.Contains(nameLower, "llvm") || strings.Contains(nameLower, "cpu") {
				score += 10
			}
		case BackendROCm:
			if strings.Contains(nameLower, "rocm") {
				score += 80
			} else if strings.Contains(nameLower, "llvm") || strings.Contains(nameLower, "cpu") {
				score += 10
			}
		case BackendVulkan:
			if strings.Contains(nameLower, "vulkan") {
				score += 80
			} else if strings.Contains(nameLower, "llvm") || strings.Contains(nameLower, "cpu") {
				score += 10
			}
		case BackendMetal:
			if strings.Contains(nameLower, "macos") || strings.Contains(nameLower, "metal") {
				score += 80
			}
		default: // BackendCPU
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
		return nil, nil, fmt.Errorf("no matching asset found for OS %s and backend %s", specs.OS, effectiveBackend)
	}

	// 5. If Windows and CUDA, find the corresponding cudart DLLs asset
	if strings.ToLower(specs.OS) == "windows" && (effectiveBackend == BackendCUDA12 || effectiveBackend == BackendCUDA13) {
		bestCudartScore := -1
		targetCudaVer := "12"
		if effectiveBackend == BackendCUDA13 {
			targetCudaVer = "13"
		}

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
			thisCudaVer := extractCudaVersion(asset.Name)
			if thisCudaVer != "" && strings.HasPrefix(thisCudaVer, targetCudaVer) {
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

// MatchOnnxAsset finds the most suitable ONNX Runtime release package using Auto backend detection.
func MatchOnnxAsset(release *GithubRelease, specs *hardware.HardwareSpecs) (*ReleaseAsset, error) {
	return MatchOnnxAssetWithBackend(release, specs, BackendAuto)
}

// MatchOnnxAssetWithBackend finds the most suitable ONNX Runtime release package for a specific backend.
func MatchOnnxAssetWithBackend(release *GithubRelease, specs *hardware.HardwareSpecs, backend BackendType) (*ReleaseAsset, error) {
	if len(release.Assets) == 0 {
		return nil, fmt.Errorf("no assets in release")
	}

	effectiveBackend := backend
	if effectiveBackend == "" || effectiveBackend == BackendAuto {
		switch specs.GPU.Type {
		case "CUDA":
			if specs.GPU.CudaVersion == "13" {
				effectiveBackend = BackendCUDA13
			} else {
				effectiveBackend = BackendCUDA12
			}
		default:
			effectiveBackend = BackendCPU
		}
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
		if effectiveBackend == BackendCUDA12 || effectiveBackend == BackendCUDA13 {
			targetCuda := "12"
			if effectiveBackend == BackendCUDA13 {
				targetCuda = "13"
			}
			if strings.Contains(nameLower, "gpu_cuda"+targetCuda) {
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
		return nil, fmt.Errorf("no matching ONNX asset found for OS %s and backend %s", specs.OS, effectiveBackend)
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
	applyAuthHeader(req)
	req.Header.Set("User-Agent", "runora-updater")

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

	buf := make([]byte, 128*1024)
	lastEmission := time.Now()
	var lastPct float64

	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			_, werr := out.Write(buf[:n])
			if werr != nil {
				return werr
			}
			downloaded += int64(n)
			if totalSize > 0 && progressChan != nil {
				pct := float64(downloaded) / float64(totalSize)
				if time.Since(lastEmission) > 50*time.Millisecond || pct-lastPct >= 0.01 || pct >= 1.0 {
					select {
					case progressChan <- pct:
						lastEmission = time.Now()
						lastPct = pct
					default:
					}
				}
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return rerr
		}
	}

	if progressChan != nil {
		select {
		case progressChan <- 1.0:
		default:
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

// CreateBackup backs up the src folder to backupDst safely.
func CreateBackup(src string, backupDst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	_ = os.RemoveAll(backupDst)
	return copyDir(src, backupDst)
}

// RollbackBackup restores the backup directory safely without destroying the backup.
func RollbackBackup(backupSrc string, dst string) error {
	if _, err := os.Stat(backupSrc); os.IsNotExist(err) {
		return fmt.Errorf("backup does not exist")
	}

	_ = os.RemoveAll(dst)
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

// VersionsDir returns the path to the versions slot directory for llama.cpp.
func VersionsDir(llamaCppDir string) string {
	return filepath.Join(llamaCppDir, "versions")
}

// ListInstalledVersions returns a list of version tags installed under llama.cpp/versions/<tag>/.
func ListInstalledVersions(llamaCppDir string) ([]string, error) {
	vDir := VersionsDir(llamaCppDir)
	entries, err := os.ReadDir(vDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	binaryName := "llama-server"
	if runtime.GOOS == "windows" {
		binaryName = "llama-server.exe"
	}

	var versions []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slotPath := filepath.Join(vDir, entry.Name())
		binPath := filepath.Join(slotPath, binaryName)
		vTxtPath := filepath.Join(slotPath, "version.txt")

		if _, err := os.Stat(binPath); err == nil {
			versions = append(versions, entry.Name())
		} else if _, err := os.Stat(vTxtPath); err == nil {
			versions = append(versions, entry.Name())
		}
	}
	return versions, nil
}

// GetActiveVersion returns the active version tag for llama.cpp.
func GetActiveVersion(llamaCppDir string) (string, error) {
	activeTxt := filepath.Join(llamaCppDir, "active_version.txt")
	if data, err := os.ReadFile(activeTxt); err == nil {
		tag := strings.TrimSpace(string(data))
		if tag != "" {
			return tag, nil
		}
	}
	v, _, _, err := QueryLocalVersion(llamaCppDir)
	return v, err
}

// SwitchActiveVersion switches the active llama.cpp installation to the specified version slot.
func SwitchActiveVersion(llamaCppDir string, versionTag string) error {
	versionTag = strings.TrimSpace(versionTag)
	if versionTag == "" {
		return fmt.Errorf("version tag cannot be empty")
	}

	slotDir := filepath.Join(VersionsDir(llamaCppDir), versionTag)
	if _, err := os.Stat(slotDir); os.IsNotExist(err) {
		return fmt.Errorf("version slot %s does not exist at %s", versionTag, slotDir)
	}

	// Create backup of current active binaries if present
	backupDir := llamaCppDir + ".backup"
	if _, err := os.Stat(llamaCppDir); err == nil {
		_ = CreateBackup(llamaCppDir, backupDir)
	}

	// Copy files from version slot to root llama.cpp dir
	if err := os.MkdirAll(llamaCppDir, 0755); err != nil {
		return err
	}

	if err := copySlotToRoot(slotDir, llamaCppDir); err != nil {
		_ = RollbackBackup(backupDir, llamaCppDir)
		return fmt.Errorf("failed to switch active version to %s: %w", versionTag, err)
	}

	activeTxt := filepath.Join(llamaCppDir, "active_version.txt")
	_ = os.WriteFile(activeTxt, []byte(versionTag), 0644)

	return nil
}

// InstallVersionSlot extracts the release archive into llama.cpp/versions/<tag>/ and switches it to active.
func InstallVersionSlot(archivePath string, cudartArchivePath string, llamaCppDir string, versionTag string) error {
	versionTag = strings.TrimSpace(versionTag)
	if versionTag == "" {
		return fmt.Errorf("version tag cannot be empty")
	}

	slotDir := filepath.Join(VersionsDir(llamaCppDir), versionTag)
	if err := os.MkdirAll(slotDir, 0755); err != nil {
		return fmt.Errorf("failed to create version slot directory: %w", err)
	}

	if err := ExtractArchive(archivePath, slotDir); err != nil {
		return fmt.Errorf("failed to extract release into version slot: %w", err)
	}

	if cudartArchivePath != "" {
		if err := ExtractArchive(cudartArchivePath, slotDir); err != nil {
			return fmt.Errorf("failed to extract cudart DLLs into version slot: %w", err)
		}
	}

	versionFile := filepath.Join(slotDir, "version.txt")
	_ = os.WriteFile(versionFile, []byte(versionTag), 0644)

	return SwitchActiveVersion(llamaCppDir, versionTag)
}

// RemoveVersionSlot deletes a version directory from llama.cpp/versions/<tag>/.
func RemoveVersionSlot(llamaCppDir string, versionTag string) error {
	slotDir := filepath.Join(VersionsDir(llamaCppDir), versionTag)
	if _, err := os.Stat(slotDir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(slotDir)
}

func copySlotToRoot(slotDir string, rootDir string) error {
	entries, err := os.ReadDir(slotDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.Name() == "versions" || strings.HasSuffix(entry.Name(), ".backup") {
			continue
		}
		srcPath := filepath.Join(slotDir, entry.Name())
		dstPath := filepath.Join(rootDir, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// ListInstalledOnnxVersions returns a list of ONNX version slots installed under onnxruntime/versions/<tag>/.
func ListInstalledOnnxVersions(onnxDir string) ([]string, error) {
	vDir := filepath.Join(onnxDir, "versions")
	entries, err := os.ReadDir(vDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var versions []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slotPath := filepath.Join(vDir, entry.Name())
		vTxtPath := filepath.Join(slotPath, "version.txt")
		if _, err := os.Stat(vTxtPath); err == nil {
			versions = append(versions, entry.Name())
		}
	}
	return versions, nil
}

// SwitchActiveOnnxVersion switches the active ONNX runtime library to the specified version slot.
func SwitchActiveOnnxVersion(onnxDir string, versionTag string) error {
	versionTag = strings.TrimSpace(versionTag)
	if versionTag == "" {
		return fmt.Errorf("version tag cannot be empty")
	}

	slotDir := filepath.Join(onnxDir, "versions", versionTag)
	if _, err := os.Stat(slotDir); os.IsNotExist(err) {
		return fmt.Errorf("ONNX version slot %s does not exist", versionTag)
	}

	backupDir := onnxDir + ".backup"
	if _, err := os.Stat(onnxDir); err == nil {
		_ = CreateBackup(onnxDir, backupDir)
	}

	if err := os.MkdirAll(onnxDir, 0755); err != nil {
		return err
	}

	if err := copySlotToRoot(slotDir, onnxDir); err != nil {
		_ = RollbackBackup(backupDir, onnxDir)
		return fmt.Errorf("failed to switch active ONNX version to %s: %w", versionTag, err)
	}

	_ = os.WriteFile(filepath.Join(onnxDir, "version.txt"), []byte(versionTag), 0644)
	return nil
}

// InstallOnnxVersionSlot extracts an ONNX archive into onnxruntime/versions/<tag>/ and activates it.
func InstallOnnxVersionSlot(archivePath string, onnxDir string, versionTag string) error {
	versionTag = strings.TrimSpace(versionTag)
	if versionTag == "" {
		return fmt.Errorf("version tag cannot be empty")
	}

	slotDir := filepath.Join(onnxDir, "versions", versionTag)
	if err := os.MkdirAll(slotDir, 0755); err != nil {
		return err
	}

	if err := ExtractOnnxLibrary(archivePath, slotDir); err != nil {
		return err
	}

	_ = os.WriteFile(filepath.Join(slotDir, "version.txt"), []byte(versionTag), 0644)
	return SwitchActiveOnnxVersion(onnxDir, versionTag)
}

// MatchAppAsset finds the precompiled binary release asset corresponding to the current platform.
func MatchAppAsset(release *GithubRelease) (*ReleaseAsset, error) {
	if release == nil || len(release.Assets) == 0 {
		return nil, fmt.Errorf("release contains no downloadable assets")
	}

	targetOS := runtime.GOOS     // "windows", "darwin", "linux"
	targetArch := runtime.GOARCH // "amd64", "arm64", "386"

	var osTokens []string
	switch targetOS {
	case "windows":
		osTokens = []string{"windows", "win", ".exe"}
	case "darwin":
		osTokens = []string{"darwin", "macos", "osx", "apple"}
	case "linux":
		osTokens = []string{"linux"}
	}

	var archTokens []string
	switch targetArch {
	case "amd64":
		archTokens = []string{"amd64", "x86_64", "x64", "64bit"}
	case "arm64":
		archTokens = []string{"arm64", "aarch64"}
	}

	var bestAsset *ReleaseAsset
	bestScore := -1

	for i := range release.Assets {
		asset := &release.Assets[i]
		nameLower := strings.ToLower(asset.Name)

		hasAppName := strings.Contains(nameLower, "runora") || strings.Contains(nameLower, "llama-manager")
		if !hasAppName && len(release.Assets) > 2 {
			continue
		}

		score := 0
		if hasAppName {
			score += 10
		}

		matchedOS := false
		for _, tok := range osTokens {
			if strings.Contains(nameLower, tok) {
				matchedOS = true
				score += 20
				break
			}
		}
		if !matchedOS {
			continue
		}

		matchedArch := false
		for _, tok := range archTokens {
			if strings.Contains(nameLower, tok) {
				matchedArch = true
				score += 20
				break
			}
		}
		if !matchedArch && targetArch == "amd64" && !strings.Contains(nameLower, "arm") && !strings.Contains(nameLower, "aarch64") {
			matchedArch = true
			score += 10
		}
		if !matchedArch {
			continue
		}

		if strings.HasSuffix(nameLower, ".zip") || strings.HasSuffix(nameLower, ".tar.gz") || strings.HasSuffix(nameLower, ".tgz") || strings.HasSuffix(nameLower, ".exe") {
			score += 5
		}

		if score > bestScore {
			bestScore = score
			bestAsset = asset
		}
	}

	if bestAsset != nil {
		return bestAsset, nil
	}

	return nil, fmt.Errorf("no precompiled release asset found for %s-%s in release %s", targetOS, targetArch, release.TagName)
}

// DownloadAndInstallApp downloads the matching release asset from GitHub or falls back to go install.
func DownloadAndInstallApp(release *GithubRelease, downloadsDir string, progressChan chan float64) (string, error) {
	if downloadsDir == "" {
		downloadsDir = os.TempDir()
	}
	_ = os.MkdirAll(downloadsDir, 0755)

	asset, err := MatchAppAsset(release)
	if err != nil {
		// Fallback to go install if go toolchain is present
		if _, lookErr := exec.LookPath("go"); lookErr == nil {
			cmd := exec.Command("go", "install", "github.com/BIJJUDAMA/runora/cmd/runora@latest")
			output, goErr := cmd.CombinedOutput()
			if goErr != nil {
				return "", fmt.Errorf("go install failed: %w (output: %s)", goErr, string(output))
			}
			return "Upgraded successfully via go install! Please restart Runora.", nil
		}
		return "", fmt.Errorf("could not match release asset: %w", err)
	}

	// 1. Download asset into temporary directory
	destFile := filepath.Join(downloadsDir, asset.Name)
	if err := DownloadRelease(asset.BrowserDownloadURL, destFile, progressChan); err != nil {
		return "", fmt.Errorf("failed to download release asset: %w", err)
	}

	// 2. Extract or locate binary
	extractDir := filepath.Join(downloadsDir, "app-update-extracted")
	_ = os.RemoveAll(extractDir)
	_ = os.MkdirAll(extractDir, 0755)
	defer os.RemoveAll(extractDir)

	exeName := "runora"
	if runtime.GOOS == "windows" {
		exeName = "runora.exe"
	}

	var newBinaryPath string
	lowerName := strings.ToLower(asset.Name)

	if strings.HasSuffix(lowerName, ".zip") || strings.HasSuffix(lowerName, ".tar.gz") || strings.HasSuffix(lowerName, ".tgz") {
		if err := ExtractArchive(destFile, extractDir); err != nil {
			return "", fmt.Errorf("failed to extract release archive: %w", err)
		}
		// Search extracted directory for runora binary
		_ = filepath.WalkDir(extractDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return nil
			}
			base := strings.ToLower(d.Name())
			if base == exeName || strings.HasPrefix(base, "runora") || strings.HasPrefix(base, "llama-manager") {
				newBinaryPath = path
			}
			return nil
		})
	} else {
		// Standalone raw executable
		newBinaryPath = destFile
	}

	if newBinaryPath == "" {
		return "", fmt.Errorf("could not locate executable %s inside downloaded package", exeName)
	}

	// 3. Resolve current running executable location
	currExe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to resolve current executable path: %w", err)
	}
	currExe, err = filepath.EvalSymlinks(currExe)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlink for executable path: %w", err)
	}

	currDir := filepath.Dir(currExe)
	targetExe := filepath.Join(currDir, exeName)

	// 4. Perform atomic swap / rename replacement
	if runtime.GOOS == "windows" {
		oldExe := targetExe + ".old"
		_ = os.Remove(oldExe)
		if err := os.Rename(targetExe, oldExe); err != nil {
			oldExe = currExe + ".old"
			_ = os.Remove(oldExe)
			_ = os.Rename(currExe, oldExe)
		}

		data, err := os.ReadFile(newBinaryPath)
		if err != nil {
			return "", fmt.Errorf("failed to read new binary: %w", err)
		}
		if err := os.WriteFile(targetExe, data, 0755); err != nil {
			return "", fmt.Errorf("failed to write upgraded binary to %s: %w", targetExe, err)
		}
	} else {
		data, err := os.ReadFile(newBinaryPath)
		if err != nil {
			return "", fmt.Errorf("failed to read new binary: %w", err)
		}
		tmpExe := targetExe + ".tmp"
		if err := os.WriteFile(tmpExe, data, 0755); err != nil {
			return "", fmt.Errorf("failed to write temporary binary: %w", err)
		}
		if err := os.Rename(tmpExe, targetExe); err != nil {
			return "", fmt.Errorf("failed to replace binary: %w", err)
		}
	}

	tagName := release.TagName
	if tagName == "" {
		tagName = "latest"
	}
	return fmt.Sprintf("Successfully upgraded Runora to %s! Please restart the application.", tagName), nil
}
