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
	specs.CPU.PhysicalCores = getUnixCPUPhysicalCores()
	specs.CPU.Model = getUnixCPUModel()

	// 2. RAM Detection
	totalRAM, availRAM := getUnixRAM()
	specs.RAM.Total = totalRAM
	specs.RAM.Available = availRAM

	// 3. GPU Detection
	gpus := getUnixGPUs(totalRAM)
	specs.GPUs = gpus
	if len(gpus) > 0 {
		specs.GPU = gpus[0]
	} else {
		specs.GPU = GPUSpecs{
			Name: "Integrated Graphics / CPU",
			VRAM: 0,
			Type: "CPU",
		}
		specs.GPUs = []GPUSpecs{specs.GPU}
	}

	if specs.GPU.Type == "Metal" && runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		specs.IsUnified = true
	}

	return specs, nil
}


func getUnixCPUPhysicalCores() int {
	if runtime.GOOS == "linux" {
		return getLinuxPhysicalCores()
	} else if runtime.GOOS == "darwin" {
		return getDarwinPhysicalCores()
	}
	threads := runtime.NumCPU()
	if threads > 1 {
		return threads / 2
	}
	return 1
}

func getLinuxPhysicalCores() int {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		threads := runtime.NumCPU()
		if threads > 1 {
			return threads / 2
		}
		return 1
	}

	lines := strings.Split(string(data), "\n")
	type coreKey struct {
		physID string
		coreID string
	}
	coreMap := make(map[coreKey]bool)
	currentPhysID := "0"
	currentCoreID := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if currentCoreID != "" {
				coreMap[coreKey{physID: currentPhysID, coreID: currentCoreID}] = true
				currentPhysID = "0"
				currentCoreID = ""
			}
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "physical id":
			currentPhysID = val
		case "core id":
			currentCoreID = val
		case "processor":
			if currentCoreID == "" {
				currentCoreID = val
			}
		}
	}
	if currentCoreID != "" {
		coreMap[coreKey{physID: currentPhysID, coreID: currentCoreID}] = true
	}
	if len(coreMap) > 0 {
		return len(coreMap)
	}

	threads := runtime.NumCPU()
	if threads > 1 {
		return threads / 2
	}
	return 1
}

func getDarwinPhysicalCores() int {
	cmd := exec.Command("sysctl", "-n", "hw.physicalcpu")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		if val, err := strconv.Atoi(strings.TrimSpace(out.String())); err == nil && val > 0 {
			return val
		}
	}
	threads := runtime.NumCPU()
	if threads > 1 {
		return threads / 2
	}
	return 1
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

func getUnixGPUs(totalRAM uint64) []GPUSpecs {
	// Try running nvidia-smi for multi-GPU
	cmd := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		var gpus []GPUSpecs
		cudaVer := detectCudaVersion()
		for _, line := range lines {
			parts := strings.Split(strings.TrimSpace(line), ",")
			if len(parts) >= 2 {
				name := strings.TrimSpace(parts[0])
				vramMb, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
				if err == nil && vramMb > 0 {
					gpus = append(gpus, GPUSpecs{
						Name:        name,
						VRAM:        vramMb * 1024 * 1024,
						Type:        "CUDA",
						CudaVersion: cudaVer,
					})
				}
			}
		}
		if len(gpus) > 0 {
			return gpus
		}
	}

	if runtime.GOOS == "darwin" {
		if runtime.GOARCH == "arm64" {
			if totalRAM == 0 {
				totalRAM, _ = getUnixRAM()
			}
			vram := AppleSiliconMetalVRAM(totalRAM)
			if vram == 0 {
				vram = uint64(8 * 1024 * 1024 * 1024 * 67 / 100)
			}
			return []GPUSpecs{{
				Name: "Apple Silicon GPU",
				VRAM: vram,
				Type: "Metal",
			}}
		}
	}

	return []GPUSpecs{{
		Name: "Integrated Graphics / CPU",
		VRAM: 0,
		Type: "CPU",
	}}
}

func getUnixGPU(totalRAM uint64) (string, uint64, string) {
	gpus := getUnixGPUs(totalRAM)
	if len(gpus) > 0 {
		return gpus[0].Name, gpus[0].VRAM, gpus[0].Type
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
