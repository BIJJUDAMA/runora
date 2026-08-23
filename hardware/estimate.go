package hardware

import (
	"fmt"
	"strings"

	"github.com/BIJJUDAMA/runora/model"
)

type MemoryEstimate struct {
	WeightSize     uint64
	KVCacheSize    uint64
	ActivationSize uint64
	Overhead       uint64
	TotalMemory    uint64
	Suitability    SuitabilityTier
	Reason         string
	GPUOffloadPct  int // Percentage of weights offloaded to GPU
}

// EstimateMemory computes estimated memory sizes and 4-tier suitability for a given model.
func EstimateMemory(meta *model.GGUFMetadata, specs *HardwareSpecs, contextLength uint32) *MemoryEstimate {
	if contextLength == 0 {
		contextLength = meta.ContextLength
		if contextLength == 0 {
			contextLength = 2048 // default fallback
		}
	}

	// Standard fallback dimensions if GGUF keys are missing
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

	kvCacheSize := kvBytesPerTokenLayer * uint64(layers) * uint64(contextLength)
	weightSize := uint64(meta.FileSize)

	// Dynamic compute graph activation memory: batchSize * embedLen * 4.5 (default batchSize = 512)
	batchSize := uint64(512)
	activationSize := uint64(float64(batchSize*uint64(embedLen)) * 4.5)

	// Base runtime overhead (512 MB)
	overhead := uint64(512 * 1024 * 1024)
	totalMemory := weightSize + kvCacheSize + activationSize + overhead

	est := &MemoryEstimate{
		WeightSize:     weightSize,
		KVCacheSize:    kvCacheSize,
		ActivationSize: activationSize,
		Overhead:       overhead,
		TotalMemory:    totalMemory,
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
