package hardware

// Hardware provides convenience helpers and high-level wrappers for hardware detection and management.

// GetSpecs is a convenience wrapper to detect system hardware specifications.
func GetSpecs() (*HardwareSpecs, error) {
	return DetectHardware()
}

// TensorSplit returns the recommended multi-GPU tensor split ratio string.
func (s *HardwareSpecs) TensorSplit() string {
	return TensorSplitAdvisorFromSpecs(s)
}

// PrimaryGPU returns the primary GPU specifications.
func (s *HardwareSpecs) PrimaryGPU() GPUSpecs {
	if len(s.GPUs) > 0 {
		return s.GPUs[0]
	}
	return s.GPU
}

// GPUCount returns the number of detected GPUs.
func (s *HardwareSpecs) GPUCount() int {
	if len(s.GPUs) > 0 {
		return len(s.GPUs)
	}
	if s.GPU.VRAM > 0 {
		return 1
	}
	return 0
}
