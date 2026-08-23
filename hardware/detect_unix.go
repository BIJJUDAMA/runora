//go:build !windows

package hardware

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

func DetectHardware() (*HardwareSpecs, error) {
	specs := &HardwareSpecs{
		OS:        runtime.GOOS,
		IsUnified: false,
	}

	// 1. CPU Detection
	specs.CPU.Threads = runtime.NumCPU()
	specs.CPU.Model = getUnixCPUModel()

	// 2. RAM Detection
	totalRAM, availRAM := getUnixRAM()
	specs.RAM.Total = totalRAM
	specs.RAM.Available = availRAM

	// 3. GPU Detection
	gpuName, gpuVRAM, gpuType := getUnixGPU(totalRAM)
	specs.GPU.Name = gpuName
	specs.GPU.VRAM = gpuVRAM
	specs.GPU.Type = gpuType

	if gpuType == "Metal" && runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		specs.IsUnified = true
	}

	if gpuType == "CUDA" {
		specs.GPU.CudaVersion = detectCudaVersion()
	}

	return specs, nil
}

func getUnixCPUModel() string {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/cpuinfo")
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "model name") {
					parts := strings.Split(line, ":")
					if len(parts) >= 2 {
						return strings.TrimSpace(parts[1])
					}
				}
			}
		}
	} else if runtime.GOOS == "darwin" {
		cmd := exec.Command("sysctl", "-n", "machdep.cpu.brand_string")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			return strings.TrimSpace(out.String())
		}
	}
	return "Unknown Unix CPU"
}

func getUnixRAM() (uint64, uint64) {
	var total, avail uint64 = 8 * 1024 * 1024 * 1024, 4 * 1024 * 1024 * 1024
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			lines := strings.Split(string(data), "\n")
			var memTotal, memAvailable uint64
			for _, line := range lines {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					key := strings.TrimSuffix(parts[0], ":")
					val, err := strconv.ParseUint(parts[1], 10, 64)
					if err == nil {
						if key == "MemTotal" {
							memTotal = val * 1024 // KB to B
						} else if key == "MemAvailable" {
							memAvailable = val * 1024 // KB to B
						}
					}
				}
			}
			if memTotal > 0 {
				total = memTotal
				if memAvailable > 0 {
					avail = memAvailable
				} else {
					avail = memTotal / 2
				}
			}
		}
	} else if runtime.GOOS == "darwin" {
		cmd := exec.Command("sysctl", "-n", "hw.memsize")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil {
			val, err := strconv.ParseUint(strings.TrimSpace(out.String()), 10, 64)
			if err == nil {
				total = val
				avail = val / 2 // rough estimate
			}
		}
	}
	return total, avail
}

func getUnixGPU(totalRAM uint64) (string, uint64, string) {
	// Try running nvidia-smi
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		for _, line := range lines {
			parts := strings.Split(strings.TrimSpace(line), ",")
			if len(parts) >= 2 {
				name := strings.TrimSpace(parts[0])
				vramMb, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
				if err == nil && vramMb > 0 {
					return name, vramMb * 1024 * 1024, "CUDA"
				}
			}
		}
	}

	if runtime.GOOS == "darwin" {
		if runtime.GOARCH == "arm64" {
			if totalRAM == 0 {
				totalRAM, _ = getUnixRAM()
			}
			vram := AppleSiliconMetalVRAM(totalRAM)
			if vram == 0 {
				vram = uint64(float64(8*1024*1024*1024) * 0.67)
			}
			return "Apple Silicon GPU", vram, "Metal"
		}
	}

	return "Integrated Graphics / CPU", 0, "CPU"
}

func detectCudaVersion() string {
	// 1. Env variable check (e.g. CUDA_PATH or CUDA_HOME)
	for _, env := range []string{"CUDA_PATH", "CUDA_HOME"} {
		cudaPath := os.Getenv(env)
		if cudaPath != "" {
			base := filepath.Base(cudaPath)
			if strings.HasPrefix(base, "v") {
				parts := strings.Split(strings.TrimPrefix(base, "v"), ".")
				if len(parts) > 0 {
					return parts[0]
				}
			} else if strings.HasPrefix(base, "cuda-") {
				parts := strings.Split(strings.TrimPrefix(base, "cuda-"), ".")
				if len(parts) > 0 {
					return parts[0]
				}
			}
		}
	}

	// 2. Check /usr/local/cuda symlink or folder
	if link, err := os.Readlink("/usr/local/cuda"); err == nil {
		base := filepath.Base(link)
		if strings.HasPrefix(base, "cuda-") {
			parts := strings.Split(strings.TrimPrefix(base, "cuda-"), ".")
			if len(parts) > 0 {
				return parts[0]
			}
		}
	} else if fi, err := os.Stat("/usr/local/cuda"); err == nil && fi.IsDir() {
		if data, err := os.ReadFile("/usr/local/cuda/version.txt"); err == nil {
			re := regexp.MustCompile(`CUDA Version\s*(\d+)`)
			matches := re.FindStringSubmatch(string(data))
			if len(matches) > 1 {
				return matches[1]
			}
		}
	}

	// 3. Try nvcc
	cmd := exec.Command("nvcc", "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		re := regexp.MustCompile(`release (\d+)\.`)
		matches := re.FindStringSubmatch(out.String())
		if len(matches) > 1 {
			return matches[1]
		}
	}

	// 4. Try nvidia-smi
	cmdSmi := exec.Command("nvidia-smi")
	var outSmi bytes.Buffer
	cmdSmi.Stdout = &outSmi
	if err := cmdSmi.Run(); err == nil {
		re := regexp.MustCompile(`CUDA Version:\s*(\d+)`)
		matches := re.FindStringSubmatch(outSmi.String())
		if len(matches) > 1 {
			return matches[1]
		}
	}

	return ""
}
