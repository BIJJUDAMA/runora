package hardware

type CPUSpecs struct {
	Model         string
	Threads       int // Logical cores (hyperthreads)
	PhysicalCores int // Physical CPU cores
}

type RAMSpecs struct {
	Total     uint64 // in bytes
	Available uint64 // in bytes
}

type GPUSpecs struct {
	Name        string
	VRAM        uint64 // in bytes
	Type        string // e.g. CUDA, Metal, ROCm, Intel, CPU
	CudaVersion string
}

type HardwareSpecs struct {
	CPU       CPUSpecs
	RAM       RAMSpecs
	GPU       GPUSpecs   // Primary GPU (for backward compatibility)
	GPUs      []GPUSpecs // All detected GPUs
	OS        string
	IsUnified bool // True if system has unified memory (Apple Silicon Mac)
}

// TotalVRAM returns the sum of VRAM across all detected GPUs, or primary GPU VRAM.
func (s *HardwareSpecs) TotalVRAM() uint64 {
	if len(s.GPUs) > 0 {
		var total uint64
		for _, g := range s.GPUs {
			total += g.VRAM
		}
		return total
	}
	return s.GPU.VRAM
}

// RecommendedThreads returns the recommended thread count for llama.cpp CPU inference.
// It prioritizes physical core count over logical threads to avoid hyperthreading cache contention.
func (s *HardwareSpecs) RecommendedThreads() int {
	if s.CPU.PhysicalCores > 0 {
		return s.CPU.PhysicalCores
	}
	if s.CPU.Threads > 0 {
		if s.CPU.Threads > 2 {
			return s.CPU.Threads / 2
		}
		return s.CPU.Threads
	}
	return 4
}


// AppleSiliconMetalVRAM computes the dynamic piecewise Apple Silicon Metal memory limit.
// Curves: 67% on 8GB up to 92% on 192GB Studio.
func AppleSiliconMetalVRAM(totalRAM uint64) uint64 {
	gb := float64(totalRAM) / (1024 * 1024 * 1024)
	var ratio float64
	switch {
	case gb <= 8:
		ratio = 0.67
	case gb <= 16:
		// 8GB -> 67%, 16GB -> 75%
		ratio = 0.67 + (gb-8)*(0.75-0.67)/(16-8)
	case gb <= 36:
		// 16GB -> 75%, 36GB -> 80%
		ratio = 0.75 + (gb-16)*(0.80-0.75)/(36-16)
	case gb <= 64:
		// 36GB -> 80%, 64GB -> 85%
		ratio = 0.80 + (gb-36)*(0.85-0.80)/(64-36)
	case gb <= 128:
		// 64GB -> 85%, 128GB -> 88%
		ratio = 0.85 + (gb-64)*(0.88-0.85)/(128-64)
	default:
		// 128GB -> 88%, 192GB+ -> 92%
		if gb >= 192 {
			ratio = 0.92
		} else {
			ratio = 0.88 + (gb-128)*(0.92-0.88)/(192-128)
		}
	}
	return uint64(float64(totalRAM) * ratio)
}
