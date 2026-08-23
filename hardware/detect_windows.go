//go:build windows

package hardware

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type memoryStatusEx struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

// DXGI GUIDs & Structures
var (
	// IID_IDXGIFactory2: {50c83a1c-e072-4c48-87b0-3630fa36a6d0}
	iidIDXGIFactory2 = windows.GUID{
		Data1: 0x50c83a1c,
		Data2: 0xe072,
		Data3: 0x4c48,
		Data4: [8]byte{0x87, 0xb0, 0x36, 0x30, 0xfa, 0x36, 0xa6, 0xd0},
	}
	// IID_IDXGIFactory1: {770aae78-f26f-4944-a099-295b509f223e}
	iidIDXGIFactory1 = windows.GUID{
		Data1: 0x770aae78,
		Data2: 0xf26f,
		Data3: 0x4944,
		Data4: [8]byte{0xa0, 0x99, 0x29, 0x5b, 0x50, 0x9f, 0x22, 0x3e},
	}
	// IID_IDXGIFactory: {7b7166ec-21c7-44ae-b21a-c9ae321ae369}
	iidIDXGIFactory = windows.GUID{
		Data1: 0x7b7166ec,
		Data2: 0x21c7,
		Data3: 0x44ae,
		Data4: [8]byte{0xb2, 0x1a, 0xc9, 0xae, 0x32, 0x1a, 0xe3, 0x69},
	}
)

type dxgiAdapterDesc1 struct {
	Description           [128]uint16
	VendorId              uint32
	DeviceId              uint32
	SubSysId              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	AdapterLuid           struct {
		LowPart  uint32
		HighPart int32
	}
	Flags uint32
}

func DetectHardware() (*HardwareSpecs, error) {
	specs := &HardwareSpecs{
		OS:        "Windows",
		IsUnified: false, // Unified memory is macOS Apple Silicon specific
	}

	// 1. CPU Detection
	specs.CPU.Threads = runtime.NumCPU()
	specs.CPU.Model = getWindowsCPUModel()

	// 2. RAM Detection
	totalRAM, availRAM, err := getWindowsRAM()
	if err == nil {
		specs.RAM.Total = totalRAM
		specs.RAM.Available = availRAM
	} else {
		// Fallback to safe defaults if DLL call fails
		specs.RAM.Total = 8 * 1024 * 1024 * 1024
		specs.RAM.Available = 4 * 1024 * 1024 * 1024
	}

	// 3. GPU & VRAM Detection (Direct DXGI native syscalls <3ms, 64-bit VRAM)
	gpuName, gpuVRAM, gpuType := getWindowsGPU()
	specs.GPU.Name = gpuName
	specs.GPU.VRAM = gpuVRAM
	specs.GPU.Type = gpuType
	if gpuType == "CUDA" {
		specs.GPU.CudaVersion = detectCudaVersion()
	}

	return specs, nil
}

func getWindowsCPUModel() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err != nil {
		return "Unknown CPU"
	}
	defer k.Close()

	model, _, err := k.GetStringValue("ProcessorNameString")
	if err != nil {
		return "Unknown CPU"
	}
	return strings.TrimSpace(model)
}

func getWindowsRAM() (uint64, uint64, error) {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	globalMemoryStatusEx := kernel32.NewProc("GlobalMemoryStatusEx")

	var memStatus memoryStatusEx
	memStatus.dwLength = uint32(unsafe.Sizeof(memStatus))

	ret, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memStatus)))
	if ret == 0 {
		return 0, 0, err
	}

	return memStatus.ullTotalPhys, memStatus.ullAvailPhys, nil
}

func getWindowsGPU() (string, uint64, string) {
	// Primary: Native DXGI COM direct syscalls (<3ms, 64-bit VRAM without PowerShell/WMI overflow)
	if name, vram, gpuType, err := getWindowsGPUviaDXGI(); err == nil && name != "" {
		return name, vram, gpuType
	}

	// Fallback: Fast nvidia-smi check if DXGI fails
	nvSmiPath := "nvidia-smi"
	stdNvSmi := filepath.Join(os.Getenv("ProgramFiles"), "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe")
	if _, err := os.Stat(stdNvSmi); err == nil {
		nvSmiPath = stdNvSmi
	}

	cmd := exec.Command(nvSmiPath, "--query-gpu=name,memory.total", "--format=csv,noheader,nounits")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		for _, line := range lines {
			parts := strings.Split(strings.TrimSpace(line), ",")
			if len(parts) >= 2 {
				name := strings.TrimSpace(parts[0])
				var vramMb uint64
				if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &vramMb); err == nil && vramMb > 0 {
					return name, vramMb * 1024 * 1024, "CUDA"
				}
			}
		}
	}

	return "Integrated Graphics / CPU", 0, "CPU"
}

func getWindowsGPUviaDXGI() (string, uint64, string, error) {
	dxgiDLL := windows.NewLazySystemDLL("dxgi.dll")
	createDXGIFactory1 := dxgiDLL.NewProc("CreateDXGIFactory1")
	if err := createDXGIFactory1.Find(); err != nil {
		return "", 0, "", err
	}

	var factory uintptr
	// Try CreateDXGIFactory1 with IID_IDXGIFactory2 first (Windows 8+)
	hr, _, _ := createDXGIFactory1.Call(
		uintptr(unsafe.Pointer(&iidIDXGIFactory2)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if hr != 0 || factory == 0 {
		// Fallback to IID_IDXGIFactory1
		hr, _, _ = createDXGIFactory1.Call(
			uintptr(unsafe.Pointer(&iidIDXGIFactory1)),
			uintptr(unsafe.Pointer(&factory)),
		)
	}
	if hr != 0 || factory == 0 {
		// Fallback to CreateDXGIFactory with IID_IDXGIFactory
		createDXGIFactory := dxgiDLL.NewProc("CreateDXGIFactory")
		if createDXGIFactory.Find() == nil {
			hr, _, _ = createDXGIFactory.Call(
				uintptr(unsafe.Pointer(&iidIDXGIFactory)),
				uintptr(unsafe.Pointer(&factory)),
			)
		}
	}

	if hr != 0 || factory == 0 {
		return "", 0, "", fmt.Errorf("failed to create DXGI factory (hr=0x%x)", hr)
	}

	factoryVtbl := *(*[32]uintptr)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(factory))))
	releaseFactory := factoryVtbl[2]
	enumAdapters1 := factoryVtbl[12]

	defer syscall.SyscallN(releaseFactory, factory)

	var bestName string
	var bestVRAM uint64
	var bestType string
	var foundHardware bool

	for i := uint32(0); ; i++ {
		var adapter uintptr
		hr, _, _ = syscall.SyscallN(enumAdapters1, factory, uintptr(i), uintptr(unsafe.Pointer(&adapter)))
		if hr != 0 || adapter == 0 {
			break // DXGI_ERROR_NOT_FOUND (0x887A0002)
		}

		adapterVtbl := *(*[32]uintptr)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(adapter))))
		releaseAdapter := adapterVtbl[2]
		getDesc1 := adapterVtbl[10]

		var desc dxgiAdapterDesc1
		hrDesc, _, _ := syscall.SyscallN(getDesc1, adapter, uintptr(unsafe.Pointer(&desc)))
		syscall.SyscallN(releaseAdapter, adapter)

		if hrDesc != 0 {
			continue
		}

		name := syscall.UTF16ToString(desc.Description[:])
		vram := uint64(desc.DedicatedVideoMemory)
		isSoftware := (desc.Flags & 2) != 0 // DXGI_ADAPTER_FLAG_SOFTWARE = 2

		gpuType := "CPU"
		if desc.VendorId == 0x10DE { // NVIDIA
			gpuType = "CUDA"
		} else if desc.VendorId == 0x1002 || desc.VendorId == 0x1022 { // AMD
			gpuType = "ROCm"
		} else if desc.VendorId == 0x8086 { // Intel
			gpuType = "Intel"
		} else {
			lowerName := strings.ToLower(name)
			if strings.Contains(lowerName, "nvidia") || strings.Contains(lowerName, "geforce") || strings.Contains(lowerName, "quadro") || strings.Contains(lowerName, "tesla") || strings.Contains(lowerName, "rtx") {
				gpuType = "CUDA"
			} else if strings.Contains(lowerName, "amd") || strings.Contains(lowerName, "radeon") {
				gpuType = "ROCm"
			} else if strings.Contains(lowerName, "intel") {
				gpuType = "Intel"
			}
		}

		if !isSoftware {
			if !foundHardware || vram > bestVRAM {
				foundHardware = true
				bestName = name
				bestVRAM = vram
				bestType = gpuType
			}
		} else if !foundHardware && bestName == "" {
			bestName = name
			bestVRAM = vram
			bestType = gpuType
		}
	}

	if bestName != "" {
		return bestName, bestVRAM, bestType, nil
	}
	return "", 0, "", fmt.Errorf("no DXGI adapters found")
}

func detectCudaVersion() string {
	// 1. Env variable check (e.g. C:\Program Files\NVIDIA GPU Computing Toolkit\CUDA\v12.4)
	cudaPath := os.Getenv("CUDA_PATH")
	if cudaPath != "" {
		base := filepath.Base(cudaPath)
		if strings.HasPrefix(base, "v") {
			parts := strings.Split(strings.TrimPrefix(base, "v"), ".")
			if len(parts) > 0 {
				return parts[0]
			}
		}
	}

	// 2. Try nvcc
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

	// 3. Try nvidia-smi
	nvSmiPath := "nvidia-smi"
	stdNvSmi := filepath.Join(os.Getenv("ProgramFiles"), "NVIDIA Corporation", "NVSMI", "nvidia-smi.exe")
	if _, err := os.Stat(stdNvSmi); err == nil {
		nvSmiPath = stdNvSmi
	}
	cmdSmi := exec.Command(nvSmiPath)
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
