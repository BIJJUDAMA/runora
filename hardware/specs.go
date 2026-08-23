package hardware

// SuitabilityTier represents the 4-tier suitability classification for a model.
type SuitabilityTier int

const (
	SuitabilityFitsVRAM    SuitabilityTier = iota // Fits fully in GPU VRAM (or Apple Silicon unified limit)
	SuitabilityPartialVRAM                        // Fits with partial GPU VRAM offload
	SuitabilityFitsRAM                            // Fits in system RAM (runs on CPU only)
	SuitabilityExceeds                            // Exceeds total system memory
)

// Backward compatibility aliases
type Suitability = SuitabilityTier

const (
	SuitabilityFits    = SuitabilityFitsVRAM
	SuitabilityPartial = SuitabilityPartialVRAM
)

// KVCacheQuantType represents the quantization format for the KV cache.
type KVCacheQuantType string

const (
	KVQuantFP16 KVCacheQuantType = "f16"  // 1.0x base FP16 KV cache
	KVQuantQ8_0 KVCacheQuantType = "q8_0" // 0.56x (8-bit quantization with block scale overhead)
	KVQuantQ4_0 KVCacheQuantType = "q4_0" // 0.28x (4-bit quantization with block scale overhead)
	KVQuantFP8  KVCacheQuantType = "fp8"  // 0.50x (8-bit float quantization, e.g. E4M3 / E5M2)
)

