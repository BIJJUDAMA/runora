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
