package hardware

import (
	"fmt"
	"strings"

	"github.com/BIJJUDAMA/runora/model"
)

type MemoryEstimate struct {
	WeightSize        uint64
	KVCacheSize       uint64
	ActivationSize    uint64
	Overhead          uint64
	TotalMemory       uint64
	Suitability       SuitabilityTier
	Reason            string
	GPUOffloadPct     int // Percentage of weights offloaded to GPU
	KVQuant           string
	KVCacheMultiplier float64
	ExactGPULayers    int // Exact integer layer offload count fitting in available VRAM
}

// KVCacheMultiplier returns the memory scaling factor for a given KV cache quantization type.
// Supported: FP16 (1.0x), Q8_0 (0.56x), Q4_0 (0.28x), FP8 (0.50x).
func KVCacheMultiplier(quant string) float64 {
	switch strings.ToLower(strings.TrimSpace(quant)) {
	case "q8_0", "q80", "8bit", "int8", "q8":
		return 0.56
	case "q4_0", "q40", "4bit", "int4", "q4":
		return 0.28
	case "fp8", "f8", "q8_e4m3", "q8_e5m2", "float8", "e4m3", "e5m2":
		return 0.50
	case "f16", "fp16", "float16", "f32", "fp32", "":
		return 1.0
	default:
		return 1.0
	}
}

// ComputeKVCacheSize calculates the KV cache memory size in bytes for a given model, context length, and quantization format.
// Accurately accounts for MHA, GQA, MQA, and DeepSeek MLA architectures.
func ComputeKVCacheSize(meta *model.GGUFMetadata, contextLength uint32, kvQuant ...string) uint64 {
	if meta == nil {
		return 0
	}
	if contextLength == 0 {
		contextLength = meta.ContextLength
		if contextLength == 0 {
			contextLength = 2048 // default fallback
		}
	}

	layers := meta.Layers
	if layers == 0 {
		layers = 32
	}
	heads := meta.Heads
	if heads == 0 {
		heads = 32
	}
	headsKV := meta.HeadsKV
	if headsKV == 0 {
		headsKV = heads // Fallback to MHA (headsKV = heads)
	}
	embedLen := meta.EmbeddingLen
	if embedLen == 0 {
		embedLen = 4096
	}

	headDim := meta.HeadDim
	if headDim == 0 {
		headDim = embedLen / heads
		if headDim == 0 {
			headDim = 128 // default fallback
		}
	}

	// Universal KV Cache Math: MHA, GQA, MQA, and MLA (DeepSeek)
	var kvBytesPerTokenLayer uint64
	archLower := strings.ToLower(meta.Architecture)
	isMLA := strings.Contains(archLower, "deepseek") || archLower == "deepseek2" || archLower == "deepseek3"

	if isMLA {
		// DeepSeek MLA compresses KV cache to low-rank latent representation + decoupled RoPE
		// Standard DeepSeek MLA uses kv_lora_rank (512) + qk_rope_head_dim (64) = 576 floats @ 2 bytes = 1152 bytes/layer
		kvBytesPerTokenLayer = uint64(576 * 2)
	} else {
		// Standard MHA / GQA / MQA: 2 (K and V) * 2 bytes (FP16) * headsKV * headDim
		kvBytesPerTokenLayer = uint64(4) * uint64(headsKV) * uint64(headDim)
	}

	baseKVCacheSize := kvBytesPerTokenLayer * uint64(layers) * uint64(contextLength)

	quant := ""
	if len(kvQuant) > 0 {
		quant = kvQuant[0]
	}
	mult := KVCacheMultiplier(quant)
	return uint64(float64(baseKVCacheSize) * mult)
}

// EstimateMemory computes estimated memory sizes and 4-tier suitability for a given model with default FP16 KV cache.
func EstimateMemory(meta *model.GGUFMetadata, specs *HardwareSpecs, contextLength uint32) *MemoryEstimate {
	return EstimateMemoryWithKVQuant(meta, specs, contextLength, "")
}

// EstimateMemoryWithKVQuant computes estimated memory sizes and suitability with optional quantized KV cache.
func EstimateMemoryWithKVQuant(meta *model.GGUFMetadata, specs *HardwareSpecs, contextLength uint32, kvQuant string) *MemoryEstimate {
	if meta == nil {
		return &MemoryEstimate{
			Suitability: SuitabilityFitsRAM,
			Reason:      "No model metadata provided",
		}
	}

	if contextLength == 0 {
		contextLength = meta.ContextLength
		if contextLength == 0 {
			contextLength = 2048 // default fallback
		}
	}

	embedLen := meta.EmbeddingLen
	if embedLen == 0 {
		embedLen = 4096
	}

	mult := KVCacheMultiplier(kvQuant)
	kvCacheSize := ComputeKVCacheSize(meta, contextLength, kvQuant)
	weightSize := uint64(meta.FileSize)

	// Dynamic compute graph activation memory: batchSize * embedLen * 4.5 (default batchSize = 512)
	batchSize := uint64(512)
	activationSize := uint64(float64(batchSize*uint64(embedLen)) * 4.5)

	// Base runtime overhead (512 MB)
	overhead := uint64(512 * 1024 * 1024)
	totalMemory := weightSize + kvCacheSize + activationSize + overhead

	gpuLayers := ExactGPULayers(meta, specs, contextLength, kvQuant)

	est := &MemoryEstimate{
		WeightSize:        weightSize,
		KVCacheSize:       kvCacheSize,
		ActivationSize:    activationSize,
		Overhead:          overhead,
		TotalMemory:       totalMemory,
		KVQuant:           kvQuant,
		KVCacheMultiplier: mult,
		ExactGPULayers:    gpuLayers,
	}

	if specs == nil {
		est.Suitability = SuitabilityFitsRAM
		est.Reason = "No hardware specifications available"
		return est
	}

	// 1. macOS Apple Silicon Unified Memory
	if specs.IsUnified {
		if totalMemory <= specs.GPU.VRAM {
			est.Suitability = SuitabilityFitsVRAM
			est.GPUOffloadPct = 100
			est.Reason = "Fits fully in Unified Memory (Metal accelerated)"
		} else if totalMemory <= specs.RAM.Total {
			est.Suitability = SuitabilityPartialVRAM
			est.GPUOffloadPct = int((float64(specs.GPU.VRAM) / float64(totalMemory)) * 100)
			if est.GPUOffloadPct > 99 {
				est.GPUOffloadPct = 99
			}
			if est.GPUOffloadPct < 5 {
				est.GPUOffloadPct = 5
			}
			est.Reason = "Fits in system RAM; partial Unified GPU offload"
		} else {
			est.Suitability = SuitabilityExceeds
			est.GPUOffloadPct = 0
			est.Reason = "Exceeds total system memory; severe performance lag expected"
		}
		return est
	}

	// 2. Dedicated GPU / System RAM (Windows & Linux)
	if specs.GPU.VRAM > 0 {
		if totalMemory <= specs.GPU.VRAM {
			est.Suitability = SuitabilityFitsVRAM
			est.GPUOffloadPct = 100
			est.Reason = fmt.Sprintf("Fits fully in GPU VRAM (%s)", specs.GPU.Name)
		} else if specs.GPU.VRAM > (overhead + activationSize + 100*1024*1024) {
			// Some capacity to offload to GPU
			if totalMemory > specs.GPU.VRAM+specs.RAM.Total {
				est.Suitability = SuitabilityExceeds
				est.GPUOffloadPct = 0
				est.Reason = "Exceeds combined GPU VRAM and System RAM"
			} else {
				est.Suitability = SuitabilityPartialVRAM
				vramAvailableForWeights := int64(specs.GPU.VRAM) - int64(kvCacheSize) - int64(activationSize) - int64(overhead)
				if vramAvailableForWeights > 0 {
					est.GPUOffloadPct = int((float64(vramAvailableForWeights) / float64(weightSize)) * 100)
				} else {
					est.GPUOffloadPct = int((float64(specs.GPU.VRAM) / float64(totalMemory)) * 100)
				}
				if est.GPUOffloadPct > 99 {
					est.GPUOffloadPct = 99
				}
				if est.GPUOffloadPct < 5 {
					est.GPUOffloadPct = 5
				}
				est.Reason = "Partial GPU offload; remaining layers will run on CPU"
			}
		} else {
			// Insufficient VRAM to offload weights, falls back to CPU
			if totalMemory <= specs.RAM.Total {
				est.Suitability = SuitabilityFitsRAM
				est.GPUOffloadPct = 0
				est.Reason = "Fits system RAM (Runs on CPU-only, insufficient VRAM)"
			} else {
				est.Suitability = SuitabilityExceeds
				est.GPUOffloadPct = 0
				est.Reason = "Exceeds system memory limits"
			}
		}
	} else {
		// CPU-only system (no GPU VRAM)
		if totalMemory <= specs.RAM.Total {
			est.Suitability = SuitabilityFitsRAM
			est.GPUOffloadPct = 0
			est.Reason = "Fits system RAM (Runs on CPU-only)"
		} else {
			est.Suitability = SuitabilityExceeds
			est.GPUOffloadPct = 0
			est.Reason = "Exceeds total system RAM"
		}
	}

	return est
}

// ExactGPULayers calculates the exact integer layer offload count fitting in available GPU VRAM.
func ExactGPULayers(meta *model.GGUFMetadata, specs *HardwareSpecs, contextLength uint32, kvQuant ...string) int {
	if meta == nil || specs == nil {
		return 0
	}

	var availableVRAM uint64
	if specs.IsUnified {
		availableVRAM = specs.GPU.VRAM
	} else {
		availableVRAM = specs.GPU.VRAM
	}

	quant := ""
	if len(kvQuant) > 0 {
		quant = kvQuant[0]
	}

	return ExactGPULayersForVRAM(meta, availableVRAM, contextLength, quant)
}

// ExactGPULayersForVRAM calculates the exact integer layer offload count fitting in a specified amount of VRAM (in bytes).
func ExactGPULayersForVRAM(meta *model.GGUFMetadata, availableVRAM uint64, contextLength uint32, kvQuant ...string) int {
	if meta == nil || meta.FileSize == 0 || availableVRAM == 0 {
		return 0
	}

	if contextLength == 0 {
		contextLength = meta.ContextLength
		if contextLength == 0 {
			contextLength = 2048
		}
	}

	totalLayers := int(meta.Layers)
	if totalLayers <= 0 {
		totalLayers = 32
	}

	embedLen := meta.EmbeddingLen
	if embedLen == 0 {
		embedLen = 4096
	}

	quant := ""
	if len(kvQuant) > 0 {
		quant = kvQuant[0]
	}

	kvCacheSize := ComputeKVCacheSize(meta, contextLength, quant)
	kvPerLayer := kvCacheSize / uint64(totalLayers)
	weightPerLayer := uint64(meta.FileSize) / uint64(totalLayers)
	vramPerLayer := weightPerLayer + kvPerLayer
	if vramPerLayer == 0 {
		return 0
	}

	// Compute graph activations & base CUDA/Metal context overhead
	batchSize := uint64(512)
	activationSize := uint64(float64(batchSize*uint64(embedLen)) * 4.5)
	overhead := uint64(256 * 1024 * 1024) // 256MB baseline runtime buffer
	fixedVRAM := activationSize + overhead

	totalMemoryNeeded := uint64(meta.FileSize) + kvCacheSize + fixedVRAM
	if availableVRAM >= totalMemoryNeeded {
		return totalLayers
	}

	if availableVRAM <= fixedVRAM {
		return 0
	}

	vramForLayers := availableVRAM - fixedVRAM
	offloadedLayers := int(vramForLayers / vramPerLayer)

	if offloadedLayers > totalLayers {
		offloadedLayers = totalLayers
	}
	if offloadedLayers < 0 {
		offloadedLayers = 0
	}

	return offloadedLayers
}

// TensorSplitAdvisor computes the recommended multi-GPU tensor split ratio string (e.g. "3,1" or "1,1").
// It accepts a slice of GPUSpecs and calculates simplified integer proportion ratios based on each GPU's VRAM.
// Returns an empty string if there are fewer than 2 GPUs or if total VRAM is 0.
func TensorSplitAdvisor(gpus []GPUSpecs) string {
	if len(gpus) < 2 {
		return ""
	}

	hasNonZero := false
	for _, g := range gpus {
		if g.VRAM > 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		return ""
	}

	// 256 MB granularity for clean proportion ratios (handles 6GB, 8GB, 10GB, 12GB, 16GB, 24GB, etc.)
	chunkSize := uint64(256 * 1024 * 1024)
	units := make([]int64, len(gpus))
	for i, g := range gpus {
		if g.VRAM == 0 {
			units[i] = 0
		} else {
			u := int64((g.VRAM + chunkSize/2) / chunkSize)
			if u < 1 {
				u = 1
			}
			units[i] = u
		}
	}

	var g int64 = 0
	for _, u := range units {
		if u > 0 {
			if g == 0 {
				g = u
			} else {
				g = gcd(g, u)
			}
		}
	}

	if g == 0 {
		return ""
	}

	parts := make([]string, len(units))
	for i, u := range units {
		parts[i] = fmt.Sprintf("%d", u/g)
	}

	return strings.Join(parts, ",")
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// TensorSplitAdvisorFromSpecs computes tensor split ratios from HardwareSpecs.
func TensorSplitAdvisorFromSpecs(specs *HardwareSpecs) string {
	if specs == nil {
		return ""
	}
	return TensorSplitAdvisor(specs.GPUs)
}

